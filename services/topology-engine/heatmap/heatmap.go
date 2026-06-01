// Package heatmap 拓扑热力图引擎
//
// 支持两种热力图:
//   - Latency Heatmap: 边延迟分布，用于识别慢调用链路
//   - Error Heatmap:   边错误率分布，用于识别故障链路
//
// 数据来源:
//   - 实时: 从内存 Graph 快照计算
//   - 历史: 从 ClickHouse topology 表聚合查询
package heatmap

import (
	"math"
	"sort"
	"sync"
	"time"

	graph "cloud-flow/services/topology-engine/graph"
	svcproto "cloud-flow/services/proto"
)

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// HeatmapConfig 热力图引擎配置
type HeatmapConfig struct {
	// TimeInterval 时间桶大小（秒），默认 60
	TimeInterval int64
	// MaxBuckets 每条边最大保留桶数量，默认 1440（24h @1min）
	MaxBuckets int
	// ErrorThreshold 错误率阈值，低于此值的边不展示在错误热力图中，默认 0.01（1%）
	ErrorThreshold float64
}

// applyDefaults 对零值字段填充默认值并返回副本。
func (c HeatmapConfig) applyDefaults() HeatmapConfig {
	if c.TimeInterval <= 0 {
		c.TimeInterval = 60
	}
	if c.MaxBuckets <= 0 {
		c.MaxBuckets = 1440
	}
	if c.ErrorThreshold <= 0 {
		c.ErrorThreshold = 0.01
	}
	return c
}

// ---------------------------------------------------------------------------
// 时间桶
// ---------------------------------------------------------------------------

// TimeBucket 热力图时间桶，存储单个时间窗口内的聚合指标
type TimeBucket struct {
	StartTime  int64   // 桶起始时间（Unix 秒）
	EndTime    int64   // 桶结束时间（Unix 秒）
	Value      float64 // 平均延迟（ms）或错误率（0~1）
	Count      uint64  // 采样数
	SumValue   float64 // 延迟累加值（用于计算平均）
	ErrorCount uint64  // 错误计数（用于错误热力图）
	TotalCount uint64  // 总请求计数（用于错误热力图）
}

// ---------------------------------------------------------------------------
// HeatmapEngine
// ---------------------------------------------------------------------------

// HeatmapEngine 热力图计算引擎
//
// 从 GraphSnapshot 增量更新延迟和错误率时间序列，
// 支持按边查询、全量查询以及统计聚合。
type HeatmapEngine struct {
	config HeatmapConfig

	// latencyHistory 延迟热力图历史数据，key = "source→target"
	latencyHistory map[string][]*TimeBucket
	// errorHistory 错误热力图历史数据，key = "source→target"
	errorHistory map[string][]*TimeBucket

	mu sync.RWMutex
}

// NewHeatmapEngine 创建热力图引擎实例
func NewHeatmapEngine(config HeatmapConfig) *HeatmapEngine {
	return &HeatmapEngine{
		config:         config.applyDefaults(),
		latencyHistory: make(map[string][]*TimeBucket),
		errorHistory:   make(map[string][]*TimeBucket),
	}
}

// ---------------------------------------------------------------------------
// 写入
// ---------------------------------------------------------------------------

// UpdateFromGraph 从拓扑图快照更新热力图数据。
//
// 遍历快照中所有活跃边，将每条边的延迟和错误率写入对应的时间桶。
// 如果当前时间窗口尚无桶则自动创建；已有桶则累加指标。
func (e *HeatmapEngine) UpdateFromGraph(g *graph.GraphSnapshot) {
	if g == nil || len(g.Edges) == 0 {
		return
	}

	now := time.Now().Unix()
	interval := e.config.TimeInterval
	bucketStart := (now / interval) * interval
	bucketEnd := bucketStart + interval

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, edge := range g.Edges {
		if edge == nil {
			continue
		}

		key := edge.Source + "\u2192" + edge.Target

		// ---- 延迟热力图 ----
		e.updateBucket(e.latencyHistory, key, bucketStart, bucketEnd, edge)

		// ---- 错误热力图 ----
		e.updateBucket(e.errorHistory, key, bucketStart, bucketEnd, edge)
	}
}

// updateBucket 将单条边的指标写入（或累加到）对应时间桶。
// 调用方需持有 e.mu 写锁。
func (e *HeatmapEngine) updateBucket(
	history map[string][]*TimeBucket,
	key string,
	bucketStart, bucketEnd int64,
	edge *graph.Edge,
) {
	buckets := history[key]

	// 查找或创建当前时间桶
	var bucket *TimeBucket
	if len(buckets) > 0 {
		last := buckets[len(buckets)-1]
		if last.StartTime == bucketStart {
			bucket = last
		}
	}
	if bucket == nil {
		bucket = &TimeBucket{
			StartTime: bucketStart,
			EndTime:   bucketEnd,
		}
		buckets = append(buckets, bucket)
	}

	// 累加延迟指标
	if edge.LatencyCount > 0 {
		// 将 ns 转为 ms
		latencyMs := float64(edge.Latency) / float64(1e6)
		bucket.SumValue += latencyMs * float64(edge.LatencyCount)
		bucket.Count += edge.LatencyCount
		bucket.Value = bucket.SumValue / float64(bucket.Count)
	}

	// 累加错误指标
	bucket.ErrorCount += edge.Errors
	bucket.TotalCount += edge.RequestCount
	if bucket.TotalCount > 0 {
		bucket.ErrorCount = bucket.ErrorCount // 已累加
	}

	// 限制桶数量
	if len(buckets) > e.config.MaxBuckets {
		buckets = buckets[len(buckets)-e.config.MaxBuckets:]
	}

	history[key] = buckets
}

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------

// GetLatencyHeatmap 获取延迟热力图数据点。
//
// source / target 为空时返回所有边的热力图数据。
// startTime / endTime 为 Unix 秒，0 表示不限制。
func (e *HeatmapEngine) GetLatencyHeatmap(source, target string, startTime, endTime int64) []*svcproto.HeatmapPoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var points []*svcproto.HeatmapPoint

	if source != "" && target != "" {
		// 查询单条边
		key := source + "\u2192" + target
		points = append(points, e.collectPoints(e.latencyHistory, key, source, target, startTime, endTime)...)
	} else {
		// 查询所有边
		for key, buckets := range e.latencyHistory {
			src, tgt := splitEdgeKey(key)
			points = append(points, e.collectPointsFromBuckets(buckets, src, tgt, startTime, endTime)...)
		}
	}

	return points
}

// GetErrorHeatmap 获取错误率热力图数据点。
//
// source / target 为空时返回所有边的热力图数据。
// startTime / endTime 为 Unix 秒，0 表示不限制。
// 仅返回错误率 >= ErrorThreshold 的数据点。
func (e *HeatmapEngine) GetErrorHeatmap(source, target string, startTime, endTime int64) []*svcproto.HeatmapPoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var points []*svcproto.HeatmapPoint

	if source != "" && target != "" {
		key := source + "\u2192" + target
		points = append(points, e.collectErrorPoints(e.errorHistory, key, source, target, startTime, endTime)...)
	} else {
		for key, buckets := range e.errorHistory {
			src, tgt := splitEdgeKey(key)
			points = append(points, e.collectErrorPointsFromBuckets(buckets, src, tgt, startTime, endTime)...)
		}
	}

	return points
}

// GetLatencyStats 获取指定时间范围内的延迟统计（min, max, avg），单位 ms。
func (e *HeatmapEngine) GetLatencyStats(startTime, endTime int64) (min, max, avg float64) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	min = math.MaxFloat64
	max = -math.MaxFloat64
	var totalCount uint64
	var totalSum float64

	for _, buckets := range e.latencyHistory {
		for _, b := range buckets {
			if !inTimeRange(b, startTime, endTime) {
				continue
			}
			if b.Count == 0 {
				continue
			}
			if b.Value < min {
				min = b.Value
			}
			if b.Value > max {
				max = b.Value
			}
			totalSum += b.Value * float64(b.Count)
			totalCount += b.Count
		}
	}

	if totalCount > 0 {
		avg = totalSum / float64(totalCount)
	}
	if min == math.MaxFloat64 {
		min = 0
	}
	if max == -math.MaxFloat64 {
		max = 0
	}

	return min, max, avg
}

// GetErrorStats 获取指定时间范围内的错误率统计（min, max, avg）。
func (e *HeatmapEngine) GetErrorStats(startTime, endTime int64) (min, max, avg float64) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	min = math.MaxFloat64
	max = -math.MaxFloat64
	var totalCount uint64
	var totalSum float64

	for _, buckets := range e.errorHistory {
		for _, b := range buckets {
			if !inTimeRange(b, startTime, endTime) {
				continue
			}
			if b.TotalCount == 0 {
				continue
			}
			rate := float64(b.ErrorCount) / float64(b.TotalCount)
			if rate < min {
				min = rate
			}
			if rate > max {
				max = rate
			}
			totalSum += rate
			totalCount++
		}
	}

	if totalCount > 0 {
		avg = totalSum / float64(totalCount)
	}
	if min == math.MaxFloat64 {
		min = 0
	}
	if max == -math.MaxFloat64 {
		max = 0
	}

	return min, max, avg
}

// ---------------------------------------------------------------------------
// 维护
// ---------------------------------------------------------------------------

// Cleanup 清理 beforeTime 之前的时间桶，释放内存。
func (e *HeatmapEngine) Cleanup(beforeTime int64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.cleanupMap(e.latencyHistory, beforeTime)
	e.cleanupMap(e.errorHistory, beforeTime)
}

// cleanupMap 清理 map 中所有 key 下早于 beforeTime 的桶。
func (e *HeatmapEngine) cleanupMap(history map[string][]*TimeBucket, beforeTime int64) {
	for key, buckets := range history {
		idx := sort.Search(len(buckets), func(i int) bool {
			return buckets[i].StartTime >= beforeTime
		})
		if idx >= len(buckets) {
			// 所有桶都早于 beforeTime，删除整条边
			delete(history, key)
		} else if idx > 0 {
			history[key] = buckets[idx:]
		}
	}
}

// ---------------------------------------------------------------------------
// 响应构建
// ---------------------------------------------------------------------------

// BuildResponse 将热力图数据点组装为 proto 响应，同时计算统计值。
func (e *HeatmapEngine) BuildResponse(points []*svcproto.HeatmapPoint, startTime, endTime int64) *svcproto.HeatmapResponse {
	if len(points) == 0 {
		return &svcproto.HeatmapResponse{
			Points:    points,
			MinValue:  0,
			MaxValue:  0,
			AvgValue:  0,
			StartTime: startTime,
			EndTime:   endTime,
			Interval:  e.config.TimeInterval,
		}
	}

	minVal := math.MaxFloat64
	maxVal := -math.MaxFloat64
	var sumVal float64

	for _, p := range points {
		if p.Value < minVal {
			minVal = p.Value
		}
		if p.Value > maxVal {
			maxVal = p.Value
		}
		sumVal += p.Value
	}

	avgVal := sumVal / float64(len(points))

	if minVal == math.MaxFloat64 {
		minVal = 0
	}
	if maxVal == -math.MaxFloat64 {
		maxVal = 0
	}

	return &svcproto.HeatmapResponse{
		Points:    points,
		MinValue:  minVal,
		MaxValue:  maxVal,
		AvgValue:  avgVal,
		StartTime: startTime,
		EndTime:   endTime,
		Interval:  e.config.TimeInterval,
	}
}

// ---------------------------------------------------------------------------
// 内部辅助方法
// ---------------------------------------------------------------------------

// collectPoints 从指定边的延迟桶中收集数据点。
func (e *HeatmapEngine) collectPoints(
	history map[string][]*TimeBucket,
	key, source, target string,
	startTime, endTime int64,
) []*svcproto.HeatmapPoint {
	buckets, ok := history[key]
	if !ok {
		return nil
	}
	return e.collectPointsFromBuckets(buckets, source, target, startTime, endTime)
}

// collectPointsFromBuckets 从桶切片中收集延迟数据点。
func (e *HeatmapEngine) collectPointsFromBuckets(
	buckets []*TimeBucket,
	source, target string,
	startTime, endTime int64,
) []*svcproto.HeatmapPoint {
	points := make([]*svcproto.HeatmapPoint, 0, len(buckets))
	for _, b := range buckets {
		if !inTimeRange(b, startTime, endTime) {
			continue
		}
		if b.Count == 0 {
			continue
		}
		points = append(points, &svcproto.HeatmapPoint{
			Source:    source,
			Target:    target,
			Timestamp: b.StartTime,
			Value:     b.Value,
			Count:     b.Count,
		})
	}
	return points
}

// collectErrorPoints 从指定边的错误桶中收集数据点（仅含超过阈值的）。
func (e *HeatmapEngine) collectErrorPoints(
	history map[string][]*TimeBucket,
	key, source, target string,
	startTime, endTime int64,
) []*svcproto.HeatmapPoint {
	buckets, ok := history[key]
	if !ok {
		return nil
	}
	return e.collectErrorPointsFromBuckets(buckets, source, target, startTime, endTime)
}

// collectErrorPointsFromBuckets 从桶切片中收集错误率数据点。
func (e *HeatmapEngine) collectErrorPointsFromBuckets(
	buckets []*TimeBucket,
	source, target string,
	startTime, endTime int64,
) []*svcproto.HeatmapPoint {
	points := make([]*svcproto.HeatmapPoint, 0, len(buckets))
	threshold := e.config.ErrorThreshold

	for _, b := range buckets {
		if !inTimeRange(b, startTime, endTime) {
			continue
		}
		if b.TotalCount == 0 {
			continue
		}
		rate := float64(b.ErrorCount) / float64(b.TotalCount)
		if rate < threshold {
			continue
		}
		points = append(points, &svcproto.HeatmapPoint{
			Source:    source,
			Target:    target,
			Timestamp: b.StartTime,
			Value:     rate,
			Count:     b.TotalCount,
		})
	}
	return points
}

// inTimeRange 判断桶是否在查询时间范围内。
func inTimeRange(b *TimeBucket, startTime, endTime int64) bool {
	if startTime > 0 && b.EndTime <= startTime {
		return false
	}
	if endTime > 0 && b.StartTime >= endTime {
		return false
	}
	return true
}

// splitEdgeKey 将 "source→target" 格式的 key 拆分为 source 和 target。
func splitEdgeKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '\u2192' { // Unicode arrow →
			return key[:i], key[i+3:] // → is 3 bytes in UTF-8
		}
	}
	return key, ""
}
