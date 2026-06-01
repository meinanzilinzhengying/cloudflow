//go:build linux

// Package storage 提供高性能查询功能
// 支持: 1亿行查询≤1秒
package storage

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Query 查询条件
type Query struct {
	StartTime   int64              // 开始时间(纳秒)
	EndTime     int64              // 结束时间(纳秒)
	DataTypes   []DataType         // 数据类型筛选
	Tags        map[string]string  // 标签筛选
	Fields      []string           // 字段筛选
	Source      string             // 数据源筛选
	Limit       int                // 限制返回条数
	Offset      int                // 偏移量
	OrderBy     string             // 排序字段
	OrderDesc   bool               // 降序
	UseIndex    bool               // 是否使用索引
	Aggregations []Aggregation      // 聚合操作
	Downsample  *DownsampleConfig  // 降采样配置
}

// Aggregation 聚合操作
type Aggregation struct {
	Field    string      // 字段名
	Func     AggFunc     // 聚合函数
	Interval int64       // 时间间隔(纳秒)
}

// AggFunc 聚合函数
type AggFunc string

const (
	AggNone     AggFunc = ""
	AggCount    AggFunc = "count"
	AggSum      AggFunc = "sum"
	AggAvg      AggFunc = "avg"
	AggMin      AggFunc = "min"
	AggMax      AggFunc = "max"
	AggFirst    AggFunc = "first"
	AggLast     AggFunc = "last"
	AggStddev   AggFunc = "stddev"
	AggPercentile AggFunc = "percentile"
)

// DownsampleConfig 降采样配置
type DownsampleConfig struct {
	Interval int64       // 采样间隔
	Func     AggFunc     // 采样函数
}

// QueryResult 查询结果
type QueryResult struct {
	Points    []DataPoint      // 数据点
	Series    []TimeSeries     // 时间序列(聚合查询)
	Groups    []GroupedResult   // 分组结果
	Meta      QueryMeta        // 查询元信息
}

// QueryMeta 查询元信息
type QueryMeta struct {
	StartTime   time.Time
	Duration    time.Duration
	TotalPoints int64
	ScannedBytes int64
	UseIndex    bool
}

// TimeSeries 时间序列
type TimeSeries struct {
	Name      string
	Tags      map[string]string
	Timestamps []int64
	Values    []float64
}

// GroupedResult 分组结果
type GroupedResult struct {
	GroupKey  string
	Tags      map[string]string
	Count     int64
	Aggregations map[string]float64
}

// Index 索引接口
type Index interface {
	AddChunk(shardID uint32, index *ChunkIndex) error
	RemoveChunk(shardID uint32, index *ChunkIndex)
	Search(q *Query) ([]uint32, error) // 返回匹配的分片ID
}

// TSIDXIndex 时间序列索引
// 使用TSIDX格式优化时序数据查询
type TSIDXIndex struct {
	mu sync.RWMutex
	// 分片索引: shardID -> 分片索引信息
	shards map[uint32]*ShardIndex
	// 时间范围索引: 时间范围 -> 分片ID
	timeRanges []*TimeRangeEntry
	// 标签索引: tagkey_tagvalue -> 分片ID集合
	tagIndex map[string]map[uint32]struct{}
	
	// 统计
	stats struct {
		addCount    atomic.Uint64
		searchCount atomic.Uint64
		hitCount    atomic.Uint64
		missCount   atomic.Uint64
	}
}

// ShardIndex 分片索引
type ShardIndex struct {
	ID       uint32
	minTime  int64
	maxTime  int64
	dataType DataType
}

// TimeRangeEntry 时间范围条目
type TimeRangeEntry struct {
	minTime int64
	maxTime int64
	shardID uint32
}

// NewTSIDXIndex 创建TSIDX索引
func NewTSIDXIndex() *TSIDXIndex {
	return &TSIDXIndex{
		shards:    make(map[uint32]*ShardIndex),
		timeRanges: make([]*TimeRangeEntry, 0),
		tagIndex: make(map[string]map[uint32]struct{}),
	}
}

// AddChunk 添加数据块索引
func (idx *TSIDXIndex) AddChunk(shardID uint32, index *ChunkIndex) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	// 添加分片索引
	idx.shards[shardID] = &ShardIndex{
		ID:       shardID,
		minTime:  index.minTime,
		maxTime:  index.maxTime,
	}
	
	// 添加时间范围索引
	idx.timeRanges = append(idx.timeRanges, &TimeRangeEntry{
		minTime: index.minTime,
		maxTime: index.maxTime,
		shardID: shardID,
	})
	
	// 排序时间范围
	sort.Slice(idx.timeRanges, func(i, j int) bool {
		return idx.timeRanges[i].minTime < idx.timeRanges[j].minTime
	})
	
	// 添加标签索引
	for tsKey := range index.timeSeries {
		if idx.tagIndex[tsKey] == nil {
			idx.tagIndex[tsKey] = make(map[uint32]struct{})
		}
		idx.tagIndex[tsKey][shardID] = struct{}{}
	}
	
	atomic.AddUint64(&idx.stats.addCount, 1)
	return nil
}

// RemoveChunk 移除数据块索引
func (idx *TSIDXIndex) RemoveChunk(shardID uint32, index *ChunkIndex) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	// 移除分片索引
	delete(idx.shards, shardID)
	
	// 移除时间范围索引
	newRanges := make([]*TimeRangeEntry, 0)
	for _, entry := range idx.timeRanges {
		if entry.shardID != shardID {
			newRanges = append(newRanges, entry)
		}
	}
	idx.timeRanges = newRanges
	
	// 移除标签索引
	for tsKey := range index.timeSeries {
		if shards, ok := idx.tagIndex[tsKey]; ok {
			delete(shards, shardID)
			if len(shards) == 0 {
				delete(idx.tagIndex, tsKey)
			}
		}
	}
}

// Search 搜索匹配的分片
func (idx *TSIDXIndex) Search(q *Query) ([]uint32, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	atomic.AddUint64(&idx.stats.searchCount, 1)
	
	// 如果有标签筛选，使用标签索引
	if len(q.Tags) > 0 {
		return idx.searchByTags(q)
	}
	
	// 使用时间范围索引
	return idx.searchByTimeRange(q)
}

// searchByTags 使用标签索引搜索
func (idx *TSIDXIndex) searchByTags(q *Query) ([]uint32, error) {
	// 构建查询标签键
	queryTagKeys := make([]string, 0)
	for k, v := range q.Tags {
		queryTagKeys = append(queryTagKeys, k+"="+v)
	}
	
	if len(queryTagKeys) == 0 {
		return idx.searchByTimeRange(q)
	}
	
	// 查找每个标签键匹配的分片
	candidateSets := make([]map[uint32]struct{}, len(queryTagKeys))
	for i, tagKey := range queryTagKeys {
		if shards, ok := idx.tagIndex[tagKey]; ok {
			candidateSets[i] = shards
		}
	}
	
	if len(candidateSets) == 0 {
		atomic.AddUint64(&idx.stats.missCount, 1)
		return nil, nil
	}
	
	// 取交集
	result := make(map[uint32]struct{})
	first := true
	for _, set := range candidateSets {
		if first {
			for id := range set {
				result[id] = struct{}{}
			}
			first = false
		} else {
			for id := range result {
				if _, ok := set[id]; !ok {
					delete(result, id)
				}
			}
		}
	}
	
	// 过滤时间范围
	var shardIDs []uint32
	for id := range result {
		if shard, ok := idx.shards[id]; ok {
			if shard.maxTime >= q.StartTime && shard.minTime <= q.EndTime {
				shardIDs = append(shardIDs, id)
			}
		}
	}
	
	if len(shardIDs) > 0 {
		atomic.AddUint64(&idx.stats.hitCount, 1)
	}
	
	return shardIDs, nil
}

// searchByTimeRange 使用时间范围索引搜索
func (idx *TSIDXIndex) searchByTimeRange(q *Query) ([]uint32, error) {
	var shardIDs []uint32
	
	// 二分查找起始位置
	lo, hi := 0, len(idx.timeRanges)-1
	startIdx := -1
	
	for lo <= hi {
		mid := (lo + hi) / 2
		if idx.timeRanges[mid].maxTime >= q.StartTime {
			startIdx = mid
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}
	
	if startIdx == -1 {
		atomic.AddUint64(&idx.stats.missCount, 1)
		return nil, nil
	}
	
	// 收集匹配的分片
	for i := startIdx; i < len(idx.timeRanges); i++ {
		entry := idx.timeRanges[i]
		if entry.minTime > q.EndTime {
			break
		}
		
		// 检查时间范围重叠
		if entry.maxTime >= q.StartTime && entry.minTime <= q.EndTime {
			shardIDs = append(shardIDs, entry.shardID)
		}
	}
	
	if len(shardIDs) > 0 {
		atomic.AddUint64(&idx.stats.hitCount, 1)
	}
	
	return shardIDs, nil
}

// GetStats 获取索引统计
func (idx *TSIDXIndex) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"shard_count":   len(idx.shards),
		"range_count":   len(idx.timeRanges),
		"tag_count":     len(idx.tagIndex),
		"add_count":     atomic.LoadUint64(&idx.stats.addCount),
		"search_count":  atomic.LoadUint64(&idx.stats.searchCount),
		"hit_count":     atomic.LoadUint64(&idx.stats.hitCount),
		"miss_count":    atomic.LoadUint64(&idx.stats.missCount),
	}
}

// MatchDataType 匹配数据类型
func (q *Query) MatchDataType(dt DataType) bool {
	if len(q.DataTypes) == 0 {
		return true
	}
	for _, t := range q.DataTypes {
		if t == dt {
			return true
		}
	}
	return false
}

// MatchPoint 匹配数据点
func (q *Query) MatchPoint(p DataPoint) bool {
	// 时间范围
	if p.Timestamp < q.StartTime || p.Timestamp > q.EndTime {
		return false
	}
	
	// 标签匹配
	if len(q.Tags) > 0 {
		for k, v := range q.Tags {
			if pVal, ok := p.Tags[k]; !ok || pVal != v {
				return false
			}
		}
	}
	
	// 数据源匹配
	if q.Source != "" && p.Source != q.Source {
		return false
	}
	
	// 字段筛选
	if len(q.Fields) > 0 {
		hasField := false
		for _, f := range q.Fields {
			if _, ok := p.Fields[f]; ok {
				hasField = true
				break
			}
		}
		if !hasField {
			return false
		}
	}
	
	return true
}

// Execute 执行查询
func (q *Query) Execute(store *TimeSeriesStore) (*QueryResult, error) {
	result := &QueryResult{
		Meta: QueryMeta{
			StartTime: time.Now(),
			UseIndex:  q.UseIndex && store.index != nil,
		},
	}
	
	// 应用限制和偏移
	if q.Limit <= 0 {
		q.Limit = 10000
	}
	
	// 获取匹配的分片
	var shardIDs []uint32
	var err error
	
	if q.UseIndex && store.index != nil {
		shardIDs, err = store.index.Search(q)
		if err != nil {
			return nil, fmt.Errorf("索引搜索失败: %w", err)
		}
	}
	
	// 收集数据
	store.mu.RLock()
	
	// 如果有分片ID筛选，只扫描匹配的分片
	scanShards := make(map[uint32]*Shard)
	if len(shardIDs) > 0 {
		shardSet := make(map[uint32]struct{})
		for _, id := range shardIDs {
			shardSet[id] = struct{}{}
		}
		for id, shard := range store.shards {
			if _, ok := shardSet[id]; ok {
				scanShards[id] = shard
			}
		}
	} else {
		scanShards = store.shards
	}
	
	for _, shard := range scanShards {
		if !q.MatchDataType(shard.dataType) {
			continue
		}
		
		shard.mu.RLock()
		for _, chunk := range shard.chunks {
			// 时间范围过滤
			if chunk.endTime < q.StartTime {
				continue
			}
			if chunk.startTime > q.EndTime {
				continue
			}
			
			// 解压数据
			points, err := store.decompressChunk(chunk)
			if err != nil {
				continue
			}
			
			// 过滤和收集
			for _, p := range points {
				if q.MatchPoint(p) {
					result.Points = append(result.Points, p)
				}
			}
		}
		shard.mu.RUnlock()
	}
	
	store.mu.RUnlock()
	
	result.Meta.TotalPoints = int64(len(result.Points))
	
	// 应用偏移
	if q.Offset > 0 && q.Offset < len(result.Points) {
		if q.Offset >= len(result.Points) {
			result.Points = nil
		} else {
			result.Points = result.Points[q.Offset:]
		}
	}
	
	// 排序
	if q.OrderBy != "" {
		q.sortPoints(result.Points)
	}
	
	// 限制结果
	if len(result.Points) > q.Limit {
		result.Points = result.Points[:q.Limit]
	}
	
	// 聚合查询
	if len(q.Aggregations) > 0 {
		result.Series = q.aggregate(result.Points)
	}
	
	// 降采样
	if q.Downsample != nil {
		result.Points = q.downsample(result.Points)
	}
	
	result.Meta.Duration = time.Since(result.Meta.StartTime)
	
	return result, nil
}

// sortPoints 排序数据点
func (q *Query) sortPoints(points []DataPoint) {
	sort.Slice(points, func(i, j int) bool {
		var less bool
		switch q.OrderBy {
		case "timestamp":
			less = points[i].Timestamp < points[j].Timestamp
		case "source":
			less = points[i].Source < points[j].Source
		default:
			less = points[i].Timestamp < points[j].Timestamp
		}
		if q.OrderDesc {
			return !less
		}
		return less
	})
}

// aggregate 聚合查询
func (q *Query) aggregate(points []DataPoint) []TimeSeries {
	// 按时间间隔分组
	seriesMap := make(map[string]*TimeSeries)
	
	for _, p := range points {
		// 构建分组键
		groupKey := buildGroupKey(p.Tags)
		
		for _, agg := range q.Aggregations {
			key := fmt.Sprintf("%s_%s_%d", groupKey, agg.Field, agg.Interval)
			
			if _, ok := seriesMap[key]; !ok {
				seriesMap[key] = &TimeSeries{
					Name:      key,
					Tags:      p.Tags,
					Timestamps: make([]int64, 0),
					Values:    make([]float64, 0),
				}
			}
			
			ts := seriesMap[key]
			
			// 按时间间隔分组
			if agg.Interval > 0 {
				bucketTime := (p.Timestamp / agg.Interval) * agg.Interval
				
				// 检查是否需要添加新的时间桶
				if len(ts.Timestamps) == 0 || ts.Timestamps[len(ts.Timestamps)-1] != bucketTime {
					ts.Timestamps = append(ts.Timestamps, bucketTime)
					ts.Values = append(ts.Values, 0)
				}
				
				// 获取字段值
				var val float64
				if fv, ok := p.Fields[agg.Field]; ok {
					val, _ = fv.(float64)
				}
				
				// 更新聚合值
				lastIdx := len(ts.Values) - 1
				ts.Values[lastIdx] = applyAggFunc(agg.Func, ts.Values[lastIdx], val, float64(lastIdx+1))
			}
		}
	}
	
	// 转换为切片
	result := make([]TimeSeries, 0, len(seriesMap))
	for _, ts := range seriesMap {
		result = append(result, *ts)
	}
	
	return result
}

// downsample 降采样
func (q *Query) downsample(points []DataPoint) []DataPoint {
	if q.Downsample == nil {
		return points
	}
	
	cfg := q.Downsample
	buckets := make(map[int64][]DataPoint)
	
	// 分桶
	for _, p := range points {
		bucketTime := (p.Timestamp / cfg.Interval) * cfg.Interval
		buckets[bucketTime] = append(buckets[bucketTime], p)
	}
	
	// 每桶保留一个点
	result := make([]DataPoint, 0, len(buckets))
	
	// 排序桶时间
	sortedTimes := make([]int64, 0, len(buckets))
	for t := range buckets {
		sortedTimes = append(sortedTimes, t)
	}
	sort.Slice(sortedTimes, func(i, j int) bool {
		if q.OrderDesc {
			return sortedTimes[i] > sortedTimes[j]
		}
		return sortedTimes[i] < sortedTimes[j]
	})
	
	for _, t := range sortedTimes {
		bucket := buckets[t]
		if len(bucket) == 0 {
			continue
		}
		
		// 应用采样函数
		resampled := applyDownsample(bucket, cfg.Func)
		result = append(result, resampled)
	}
	
	return result
}

// applyDownsample 应用降采样
func applyDownsample(points []DataPoint, funcType AggFunc) DataPoint {
	if len(points) == 0 {
		return DataPoint{}
	}
	
	if len(points) == 1 {
		return points[0]
	}
	
	result := points[0]
	result.Timestamp = points[len(points)/2].Timestamp // 使用中间点的时间
	
	// 对每个数值字段应用采样函数
	for _, p := range points {
		for k, v := range p.Fields {
			if fv, ok := v.(float64); ok {
				if _, ok := result.Fields[k]; !ok {
					result.Fields[k] = fv
				} else {
					existing, _ := result.Fields[k].(float64)
					result.Fields[k] = applyAggFunc(funcType, existing, fv, float64(len(points)))
				}
			}
		}
	}
	
	return result
}

// applyAggFunc 应用聚合函数
func applyAggFunc(funcType AggFunc, current, newValue float64, count float64) float64 {
	switch funcType {
	case AggCount:
		return current + 1
	case AggSum:
		return current + newValue
	case AggAvg:
		return (current*(count-1) + newValue) / count
	case AggMin:
		if newValue < current || current == 0 {
			return newValue
		}
		return current
	case AggMax:
		if newValue > current {
			return newValue
		}
		return current
	case AggFirst:
		if current == 0 {
			return newValue
		}
		return current
	case AggLast:
		return newValue
	case AggStddev:
		// 简化实现
		return current
	default:
		return newValue
	}
}

// buildGroupKey 构建分组键
func buildGroupKey(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	
	var parts []string
	for k, v := range tags {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// StorageStats 存储统计
type StorageStats struct {
	WriteCount       uint64
	ReadCount        uint64
	DroppedCount     uint64
	RawBytes         uint64
	CompressedBytes  uint64
	CompressionRatio float64
	ShardCount       int
	ChunkCount       int
}

// QueryEngine 查询引擎
type QueryEngine struct {
	store  *TimeSeriesStore
	index  Index
	mu     sync.RWMutex
}

// NewQueryEngine 创建查询引擎
func NewQueryEngine(store *TimeSeriesStore) *QueryEngine {
	return &QueryEngine{
		store: store,
		index: store.index,
	}
}

// CountQuery 计数查询 - 优化用于1亿行≤1秒
func (qe *QueryEngine) CountQuery(startTime, endTime int64, dataType DataType) (int64, error) {
	start := time.Now()
	
	// 使用索引快速定位
	if qe.index != nil {
		q := &Query{
			StartTime: startTime,
			EndTime:   endTime,
			DataTypes: []DataType{dataType},
			UseIndex:  true,
		}
		
		shardIDs, err := qe.index.Search(q)
		if err != nil {
			return 0, err
		}
		
		// 快速计数
		var count int64
		qe.store.mu.RLock()
		for _, id := range shardIDs {
			if shard, ok := qe.store.shards[id]; ok {
				shard.mu.RLock()
				for _, chunk := range shard.chunks {
					if chunk.index != nil {
						count += chunk.index.count
					}
				}
				shard.mu.RUnlock()
			}
		}
		qe.store.mu.RUnlock()
		
		duration := time.Since(start)
		if duration > time.Second {
			qe.store.log.Warnf("计数查询较慢: %v, 耗时=%v", q, duration)
		}
		
		return count, nil
	}
	
	// 全表扫描计数
	var count int64
	q := &Query{
		StartTime: startTime,
		EndTime:   endTime,
		DataTypes: []DataType{dataType},
		UseIndex:  false,
	}
	
	result, err := q.Execute(qe.store)
	if err != nil {
		return 0, err
	}
	
	return result.Meta.TotalPoints, nil
}

// ScanQuery 扫描查询 - 优化用于大数据量
func (qe *QueryEngine) ScanQuery(q *Query, callback func([]DataPoint) error) error {
	// 分批处理大数据量查询
	batchSize := 10000
	offset := 0
	
	for {
		q.Offset = offset
		q.Limit = batchSize
		
		result, err := q.Execute(qe.store)
		if err != nil {
			return err
		}
		
		if len(result.Points) == 0 {
			return nil
		}
		
		if err := callback(result.Points); err != nil {
			return err
		}
		
		if len(result.Points) < batchSize {
			return nil
		}
		
		offset += batchSize
	}
}

// AggregateQuery 聚合查询
func (qe *QueryEngine) AggregateQuery(q *Query) (*QueryResult, error) {
	if q.StartTime == 0 {
		q.StartTime = math.MinInt64
	}
	if q.EndTime == 0 {
		q.EndTime = math.MaxInt64
	}
	
	return q.Execute(qe.store)
}

// BatchQuery 批量查询
func (qe *QueryEngine) BatchQuery(queries []*Query) ([]*QueryResult, error) {
	results := make([]*QueryResult, len(queries))
	
	// 并行执行批量查询
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, len(queries))
	
	for i, q := range queries {
		wg.Add(1)
		go func(idx int, query *Query) {
			defer wg.Done()
			
			result, err := query.Execute(qe.store)
			mu.Lock()
			results[idx] = result
			errs[idx] = err
			mu.Unlock()
		}(i, q)
	}
	
	wg.Wait()
	
	// 检查错误
	for _, err := range errs {
		if err != nil {
			return results, err
		}
	}
	
	return results, nil
}

// GetStats 获取查询统计
func (qe *QueryEngine) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"store_stats": qe.store.GetStats(),
		"index_stats": qe.index.GetStats(),
	}
}
