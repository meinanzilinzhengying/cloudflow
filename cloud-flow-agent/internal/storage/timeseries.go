//go:build linux

// Package storage 提供高性能历史数据存储功能
// 支持时序数据存储、压缩、查询
// - 历史数据默认存60天，支持自定义周期
// - 结构化压缩≥20:1、日志≥6:1
// - 写入≥5万条/秒、1亿行查询≤1秒
package storage

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// DataType 数据类型
type DataType uint8

const (
	DataTypeUnknown DataType = iota
	DataTypeMetric       // 结构化指标数据
	DataTypeLog          // 日志数据
	DataTypeTrace        // 追踪数据
	DataTypeEvent        // 事件数据
)

// String 返回数据类型名称
func (t DataType) String() string {
	switch t {
	case DataTypeMetric:
		return "metric"
	case DataTypeLog:
		return "log"
	case DataTypeTrace:
		return "trace"
	case DataTypeEvent:
		return "event"
	default:
		return "unknown"
	}
}

// CompressionType 压缩类型
type CompressionType uint8

const (
	CompressionNone CompressionType = iota
	CompressionZSTD     // ZSTD压缩，高压缩率
	CompressionLZ4      // LZ4压缩，高速度
	CompressionDelta    // 增量编码压缩
	CompressionSnappy   // Snappy压缩，日志数据专用，高速+合理压缩比
)

// DataPoint 数据点
type DataPoint struct {
	Timestamp int64     // 时间戳(纳秒)
	Tags      map[string]string // 标签
	Fields    map[string]interface{} // 字段值
	DataType  DataType // 数据类型
	Source    string   // 数据源
}

// Chunk 数据块
type Chunk struct {
	mu        sync.RWMutex
	id        uint64
	startTime int64
	endTime   int64
	dataType  DataType
	compressed bool
	compressionType CompressionType // 记录本块使用的压缩算法
	data      []byte
	index     *ChunkIndex
	refCount  atomic.Int32
}

// ChunkIndex 数据块索引
type ChunkIndex struct {
	minTime int64
	maxTime int64
	count   int64
	// 时间序列索引: tagkey_tagvalue -> []byte offset
	timeSeries map[string][]byte
	// 字段统计
	fields map[string]struct {
		min, max, sum float64
		count int64
	}
}

// RetentionConfig 保留期配置
type RetentionConfig struct {
	Enabled      bool          // 启用保留策略
	DefaultDays  int           // 默认保留天数
	CustomPeriod map[DataType]int // 自定义类型保留天数
}

// GetRetentionDays 获取指定数据类型的保留天数
func (r *RetentionConfig) GetRetentionDays(dt DataType) int {
	if days, ok := r.CustomPeriod[dt]; ok && days > 0 {
		return days
	}
	return r.DefaultDays
}

// StorageOptions 存储配置选项
type StorageOptions struct {
	BaseDir           string          // 基础存储目录
	Retention         RetentionConfig // 保留期配置
	ChunkSize         int             // 数据块大小(条数)
	WriteBufferSize   int             // 写缓冲区大小
	CompressionType   CompressionType // 压缩类型
	IndexEnabled      bool            // 启用索引
	RetentionInterval time.Duration   // 保留期检查间隔
	MaxOpenFiles      int             // 最大打开文件数
}

// DefaultStorageOptions 返回默认配置
func DefaultStorageOptions(baseDir string) *StorageOptions {
	return &StorageOptions{
		BaseDir: baseDir,
		Retention: RetentionConfig{
			Enabled:     true,
			DefaultDays: 60, // 默认60天
			CustomPeriod: map[DataType]int{
				DataTypeMetric: 60, // 指标数据60天
				DataTypeLog:    30, // 日志数据30天
				DataTypeTrace:  7,  // 追踪数据7天
				DataTypeEvent:  90, // 事件数据90天
			},
		},
		ChunkSize:         10000,           // 每块1万条
		WriteBufferSize:   50000,           // 缓冲区5万条
		CompressionType:   CompressionZSTD, // 默认ZSTD压缩
		IndexEnabled:      true,            // 启用索引
		RetentionInterval: time.Hour,       // 每小时检查一次
		MaxOpenFiles:     1024,
	}
}

// TimeSeriesStore 时序数据存储
type TimeSeriesStore struct {
	opts        *StorageOptions
	log         *logger.Logger
	mu          sync.RWMutex
	shards      map[uint32]*Shard // 按数据源分片
	writeBuffer map[DataType]*WriteBuffer
	compressor  Compressor       // 默认压缩器（向后兼容）
	compressors map[DataType]Compressor // 按数据类型路由的压缩器
	index       Index
	stopCh      chan struct{}
	wg          sync.WaitGroup
	
	// 统计信息
	stats struct {
		写入统计   atomic.Uint64
		读取统计   atomic.Uint64
		压缩前字节 atomic.Uint64
		压缩后字节 atomic.Uint64
		丢弃统计   atomic.Uint64
	}
}

// Shard 数据分片
type Shard struct {
	id       uint32
	dataType DataType
	source   string
	chunks   []*Chunk
	mu       sync.RWMutex
}

// WriteBuffer 写缓冲区
type WriteBuffer struct {
	mu      sync.Mutex
	data    []DataPoint
	size    int
	cond    *sync.Cond
	closed  bool
}

// NewTimeSeriesStore 创建时序数据存储
func NewTimeSeriesStore(opts *StorageOptions, log *logger.Logger) (*TimeSeriesStore, error) {
	if opts == nil {
		opts = DefaultStorageOptions("/var/lib/cloud-flow-agent/storage")
	}
	
	// 确保目录存在
	if err := os.MkdirAll(opts.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}
	
	// 创建压缩器
	compressor, err := NewCompressor(opts.CompressionType)
	if err != nil {
		return nil, fmt.Errorf("创建压缩器失败: %w", err)
	}

	// 按数据类型创建专用压缩器
	compressors := make(map[DataType]Compressor)
	compressors[DataTypeMetric] = NewMetricCompressor() // 结构化指标 → Zstandard 高压缩
	compressors[DataTypeLog] = NewSnappyCompressor()    // 日志数据 → Snappy 快速压缩
	compressors[DataTypeTrace] = compressor             // 追踪数据 → 使用默认压缩器
	compressors[DataTypeEvent] = compressor             // 事件数据 → 使用默认压缩器
	
	// 创建索引
	var index Index
	if opts.IndexEnabled {
		index = NewTSIDXIndex()
	}
	
	store := &TimeSeriesStore{
		opts:        opts,
		log:         log,
		shards:      make(map[uint32]*Shard),
		writeBuffer: make(map[DataType]*WriteBuffer),
		compressor:  compressor,
		compressors: compressors,
		index:       index,
		stopCh:      make(chan struct{}),
	}
	
	// 初始化写缓冲区
	for dt := DataTypeMetric; dt <= DataTypeEvent; dt++ {
		store.writeBuffer[dt] = &WriteBuffer{
			data: make([]DataPoint, 0, opts.WriteBufferSize),
			size: opts.WriteBufferSize,
		}
		store.writeBuffer[dt].cond = sync.NewCond(&store.writeBuffer[dt].mu)
	}
	
	// 启动后台任务
	store.wg.Add(1)
	go store.flushLoop()
	
	store.wg.Add(1)
	go store.retentionLoop()
	
	store.log.Infof("时序存储初始化完成: 目录=%s, 保留期=%d天, 压缩=%s", 
		opts.BaseDir, opts.Retention.DefaultDays, opts.CompressionType.String())
	
	return store, nil
}

// Write 写入数据点
func (s *TimeSeriesStore) Write(points ...DataPoint) error {
	if len(points) == 0 {
		return nil
	}
	
	// 按数据类型分组
	buffers := make(map[DataType][]DataPoint)
	for _, p := range points {
		dt := p.DataType
		if dt == DataTypeUnknown {
			dt = DataTypeMetric
		}
		buffers[dt] = append(buffers[dt], p)
	}
	
	// 写入缓冲区
	for dt, pts := range buffers {
		if err := s.writeToBuffer(dt, pts); err != nil {
			return err
		}
	}
	
	atomic.AddUint64(&s.stats.写入统计, uint64(len(points)))
	return nil
}

// writeToBuffer 写入缓冲区
func (s *TimeSeriesStore) writeToBuffer(dt DataType, points []DataPoint) error {
	buf := s.writeBuffer[dt]
	if buf == nil {
		return errors.New("缓冲区未初始化")
	}
	
	buf.mu.Lock()
	defer buf.mu.Unlock()
	
	if buf.closed {
		return errors.New("缓冲区已关闭")
	}
	
	buf.data = append(buf.data, points...)
	
	// 检查是否需要自动刷新
	if len(buf.data) >= buf.size {
		select {
		case s.stopCh <- struct{}{}: // 触发刷新
		default:
		}
	}
	
	return nil
}

// flushLoop 刷新循环
func (s *TimeSeriesStore) flushLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.stopCh:
			s.flushAll()
			return
		case <-ticker.C:
			s.flushAll()
		}
	}
}

// flushAll 刷新所有缓冲区
func (s *TimeSeriesStore) flushAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for dt, buf := s.writeBuffer {
		buf.mu.Lock()
		if len(buf.data) > 0 {
			// 交换缓冲区
			data := buf.data
			buf.data = make([]DataPoint, 0, buf.size)
			buf.mu.Unlock()
			
			// 写入存储
			if err := s.writeChunk(dt, data); err != nil {
				s.log.Warnf("写入数据块失败: %v", err)
				atomic.AddUint64(&s.stats.丢弃统计, uint64(len(data)))
			}
		} else {
			buf.mu.Unlock()
		}
	}
}

// writeChunk 写入数据块
func (s *TimeSeriesStore) writeChunk(dt DataType, points []DataPoint) error {
	if len(points) == 0 {
		return nil
	}
	
	// 按时间排序
	sortDataPoints(points)
	
	// 计算分片ID
	shardID := calculateShardID(points[0].Source, dt)
	
	s.mu.Lock()
	shard, ok := s.shards[shardID]
	if !ok {
		shard = &Shard{
			id:       shardID,
			dataType: dt,
			source:   points[0].Source,
			chunks:   make([]*Chunk, 0),
		}
		s.shards[shardID] = shard
	}
	// 在持有主锁的情况下锁定 shard，防止在解锁后 shard 被其他 goroutine 删除
	shard.mu.Lock()
	s.mu.Unlock()
	defer shard.mu.Unlock()
	
	// 创建数据块
	chunk := &Chunk{
		id:        atomic.AddUint64(&s.stats.写入统计, 0),
		startTime: points[0].Timestamp,
		endTime:   points[len(points)-1].Timestamp,
		dataType:  dt,
	}
	
	// 序列化数据
	data, err := s.serializePoints(points)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}
	
	// 按数据类型选择压缩器
	cmp := s.getCompressor(dt)
	
	// 压缩数据
	compressed, err := cmp.Compress(data)
	if err != nil {
		return fmt.Errorf("压缩数据失败: %w", err)
	}
	
	atomic.AddUint64(&s.stats.压缩前字节, uint64(len(data)))
	atomic.AddUint64(&s.stats.压缩后字节, uint64(len(compressed)))
	
	chunk.data = compressed
	chunk.compressed = true
	chunk.compressionType = cmp.Type()
	
	// 创建索引
	if s.index != nil {
		chunk.index = s.createChunkIndex(chunk, points)
		if err := s.index.AddChunk(shardID, chunk.index); err != nil {
			s.log.Warnf("添加索引失败: %v", err)
		}
	}
	
	shard.chunks = append(shard.chunks, chunk)
	
	// 持久化
	if err := s.persistChunk(shard, chunk); err != nil {
		return fmt.Errorf("持久化数据块失败: %w", err)
	}
	
	s.log.Debugf("写入数据块: 类型=%s, 条数=%d, 大小=%d/%d字节, 压缩比=%.1f:1",
		dt.String(), len(points), len(data), len(compressed), 
		float64(len(data))/float64(len(compressed)))
	
	return nil
}

// serializePoints 序列化数据点
func (s *TimeSeriesStore) serializePoints(points []DataPoint) ([]byte, error) {
	// 使用简单高效的二进制格式
	buf := make([]byte, 0, len(points)*64) // 预估每条64字节
	
	for _, p := range points {
		// 时间戳(8字节)
		var timestamp [8]byte
		putUint64(timestamp[:], uint64(p.Timestamp))
		buf = append(buf, timestamp[:]...)
		
		// 标签数量(2字节) + 标签数据
		tags := p.Tags
		if tags == nil {
			tags = make(map[string]string)
		}
		var tagCount [2]byte
		putUint16(tagCount[:], uint16(len(tags)))
		buf = append(buf, tagCount[:]...)
		
		for k, v := range tags {
			buf = append(buf, byte(len(k)))
			buf = append(buf, k...)
			buf = append(buf, byte(len(v)))
			buf = append(buf, v...)
		}
		
		// 字段数量(2字节) + 字段数据
		fields := p.Fields
		if fields == nil {
			fields = make(map[string]interface{})
		}
		putUint16(tagCount[:], uint16(len(fields)))
		buf = append(buf, tagCount[:]...)
		
		for k, v := range fields {
			buf = append(buf, byte(len(k)))
			buf = append(buf, k...)
			
			// 写入值
			switch val := v.(type) {
			case float64:
				var fval [8]byte
				putFloat64(fval[:], val)
				buf = append(buf, 1) // 类型标记
				buf = append(buf, fval[:]...)
			case int64:
				var ival [8]byte
				putUint64(ival[:], uint64(val))
				buf = append(buf, 2) // 类型标记
				buf = append(buf, ival[:]...)
			case string:
				buf = append(buf, 3) // 类型标记
				buf = append(buf, byte(len(val)))
				buf = append(buf, val...)
			default:
				buf = append(buf, 0) // 未知类型
			}
		}
	}
	
	return buf, nil
}

// createChunkIndex 创建块索引
func (s *TimeSeriesStore) createChunkIndex(chunk *Chunk, points []DataPoint) *ChunkIndex {
	index := &ChunkIndex{
		minTime: chunk.startTime,
		maxTime: chunk.endTime,
		count:   int64(len(points)),
		timeSeries: make(map[string][]byte),
		fields: make(map[string]struct {
			min, max, sum float64
			count int64
		}),
	}
	
	// 构建时间序列索引
	for _, p := range points {
		tsKey := buildTimeSeriesKey(p.Tags)
		if _, ok := index.timeSeries[tsKey]; !ok {
			// 存储偏移量
			var offset [8]byte
			putUint64(offset[:], 0) // 简化，实际应存储真实偏移
			index.timeSeries[tsKey] = offset[:]
		}
		
		// 字段统计
		for k, v := range p.Fields {
			if fv, ok := v.(float64); ok {
				stat := index.fields[k]
				if stat.count == 0 {
					stat.min = fv
					stat.max = fv
				} else {
					if fv < stat.min {
						stat.min = fv
					}
					if fv > stat.max {
						stat.max = fv
					}
				}
				stat.sum += fv
				stat.count++
				index.fields[k] = stat
			}
		}
	}
	
	return index
}

// persistChunk 持久化数据块
func (s *TimeSeriesStore) persistChunk(shard *Shard, chunk *Chunk) error {
	// 构建文件路径
	filename := fmt.Sprintf("%s_%d_%d_%d.chunk", 
		shard.dataType.String(), shard.id, chunk.startTime, chunk.endTime)
	path := filepath.Join(s.opts.BaseDir, filename)
	
	// 写入文件
	if err := os.WriteFile(path, chunk.data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	
	// 写入索引文件
	if chunk.index != nil {
		indexPath := path + ".idx"
		if err := s.persistIndex(indexPath, chunk.index); err != nil {
			s.log.Warnf("写入索引文件失败: %v", err)
		}
	}
	
	return nil
}

// persistIndex 持久化索引
func (s *TimeSeriesStore) persistIndex(path string, index *ChunkIndex) error {
	// 简化的索引持久化
	data := make([]byte, 0)
	
	// 写入元数据
	var meta [24]byte
	putInt64(meta[:8], index.minTime)
	putInt64(meta[8:16], index.maxTime)
	putInt64(meta[16:24], index.count)
	data = append(data, meta[:]...)
	
	return os.WriteFile(path, data, 0644)
}

// retentionLoop 保留期管理循环
func (s *TimeSeriesStore) retentionLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.opts.RetentionInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if s.opts.Retention.Enabled {
				if err := s.cleanupExpiredData(); err != nil {
					s.log.Warnf("清理过期数据失败: %v", err)
				}
			}
		}
	}
}

// cleanupExpiredData 清理过期数据
func (s *TimeSeriesStore) cleanupExpiredData() error {
	now := time.Now().UnixNano()
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	var deletedCount int
	for shardID, shard := range s.shards {
		shard.mu.Lock()
		
		retention := s.opts.Retention.GetRetentionDays(shard.dataType)
		cutoffTime := now - int64(retention)*24*60*60*1e9
		
		var validChunks []*Chunk
		for _, chunk := range shard.chunks {
			if chunk.endTime < cutoffTime {
				// 删除过期块
				if err := s.deleteChunk(shard, chunk); err != nil {
					s.log.Warnf("删除过期数据块失败: %v", err)
					validChunks = append(validChunks, chunk)
				} else {
					deletedCount++
				}
			} else {
				validChunks = append(validChunks, chunk)
			}
		}
		
		shard.chunks = validChunks
		
		// 删除空分片
		if len(shard.chunks) == 0 {
			delete(s.shards, shardID)
		}
		
		shard.mu.Unlock()
	}
	
	if deletedCount > 0 {
		s.log.Infof("清理过期数据: 删除%d个数据块", deletedCount)
	}
	
	return nil
}

// deleteChunk 删除数据块
func (s *TimeSeriesStore) deleteChunk(shard *Shard, chunk *Chunk) error {
	filename := fmt.Sprintf("%s_%d_%d_%d.chunk", 
		shard.dataType.String(), shard.id, chunk.startTime, chunk.endTime)
	path := filepath.Join(s.opts.BaseDir, filename)
	
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	
	// 删除索引文件
	indexPath := path + ".idx"
	os.Remove(indexPath)
	
	// 从索引中移除
	if s.index != nil {
		s.index.RemoveChunk(shard.id, chunk.index)
	}
	
	return nil
}

// Query 查询数据
func (s *TimeSeriesStore) Query(q *Query) (*QueryResult, error) {
	if q == nil {
		return nil, errors.New("查询条件为空")
	}
	
	start := time.Now()
	atomic.AddUint64(&s.stats.读取统计, 1)
	
	// 使用索引加速查询
	if s.index != nil && q.UseIndex {
		return s.queryWithIndex(q)
	}
	
	return s.queryWithoutIndex(q, start)
}

// queryWithIndex 使用索引查询
func (s *TimeSeriesStore) queryWithIndex(q *Query) (*QueryResult, error) {
	result := &QueryResult{
		Points: make([]DataPoint, 0),
		Meta: QueryMeta{
			StartTime: time.Now(),
		},
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// 查找匹配的分片
	for shardID, shard := range s.shards {
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
			
			// 解压并过滤数据
			points, err := s.decompressChunk(chunk)
			if err != nil {
				continue
			}
			
			// 应用查询条件过滤
			for _, p := range points {
				if q.MatchPoint(p) {
					result.Points = append(result.Points, p)
				}
			}
		}
		shard.mu.RUnlock()
	}
	
	result.Meta.Duration = time.Since(result.Meta.StartTime)
	result.Meta.TotalPoints = len(result.Points)
	
	return result, nil
}

// queryWithoutIndex 全表扫描查询
func (s *TimeSeriesStore) queryWithoutIndex(q *Query, start time.Time) (*QueryResult, error) {
	result := &QueryResult{
		Points: make([]DataPoint, 0),
		Meta: QueryMeta{
			StartTime: start,
		},
	}
	
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	for _, shard := range s.shards {
		if !q.MatchDataType(shard.dataType) {
			continue
		}
		
		shard.mu.RLock()
		for _, chunk := range shard.chunks {
			if chunk.endTime < q.StartTime || chunk.startTime > q.EndTime {
				continue
			}
			
			points, err := s.decompressChunk(chunk)
			if err != nil {
				continue
			}
			
			for _, p := range points {
				if q.MatchPoint(p) {
					result.Points = append(result.Points, p)
				}
			}
		}
		shard.mu.RUnlock()
	}
	
	result.Meta.Duration = time.Since(result.Meta.StartTime)
	result.Meta.TotalPoints = len(result.Points)
	
	return result, nil
}

// decompressChunk 解压数据块
func (s *TimeSeriesStore) decompressChunk(chunk *Chunk) ([]DataPoint, error) {
	if !chunk.compressed {
		return nil, errors.New("数据块未压缩")
	}
	
	// 按数据类型选择解压器
	cmp := s.getCompressor(chunk.dataType)
	data, err := cmp.Decompress(chunk.data)
	if err != nil {
		return nil, fmt.Errorf("解压失败: %w", err)
	}
	
	return s.deserializePoints(data)
}

// deserializePoints 反序列化数据点
func (s *TimeSeriesStore) deserializePoints(data []byte) ([]DataPoint, error) {
	points := make([]DataPoint, 0)
	offset := 0
	
	for offset < len(data) {
		p := DataPoint{}
		
		// 读取时间戳
		if offset+8 > len(data) {
			break
		}
		p.Timestamp = int64(getUint64(data[offset:offset+8]))
		offset += 8
		
		// 读取标签
		if offset+2 > len(data) {
			break
		}
		tagCount := int(getUint16(data[offset:offset+2]))
		offset += 2
		
		p.Tags = make(map[string]string)
		for i := 0; i < tagCount; i++ {
			if offset+1 > len(data) {
				break
			}
			kLen := int(data[offset])
			offset++
			
			if offset+kLen > len(data) {
				break
			}
			k := string(data[offset : offset+kLen])
			offset += kLen
			
			if offset+1 > len(data) {
				break
			}
			vLen := int(data[offset])
			offset++
			
			if offset+vLen > len(data) {
				break
			}
			v := string(data[offset : offset+vLen])
			offset += vLen
			
			p.Tags[k] = v
		}
		
		// 读取字段
		if offset+2 > len(data) {
			break
		}
		fieldCount := int(getUint16(data[offset:offset+2]))
		offset += 2
		
		p.Fields = make(map[string]interface{})
		for i := 0; i < fieldCount; i++ {
			if offset+1 > len(data) {
				break
			}
			fLen := int(data[offset])
			offset++
			
			if offset+fLen > len(data) {
				break
			}
			fk := string(data[offset : offset+fLen])
			offset += fLen
			
			if offset+1 > len(data) {
				break
			}
			typeMark := data[offset]
			offset++
			
			switch typeMark {
			case 1: // float64
				if offset+8 > len(data) {
					break
				}
				p.Fields[fk] = getFloat64(data[offset : offset+8])
				offset += 8
			case 2: // int64
				if offset+8 > len(data) {
					break
				}
				p.Fields[fk] = int64(getUint64(data[offset:offset+8]))
				offset += 8
			case 3: // string
				if offset+1 > len(data) {
					break
				}
				sLen := int(data[offset])
				offset++
				if offset+sLen > len(data) {
					break
				}
				p.Fields[fk] = string(data[offset : offset+sLen])
				offset += sLen
			}
		}
		
		points = append(points, p)
	}
	
	return points, nil
}

// getCompressor 按数据类型获取压缩器
// 优先使用该类型专用压缩器，未配置时回退到默认压缩器
func (s *TimeSeriesStore) getCompressor(dt DataType) Compressor {
	if cmp, ok := s.compressors[dt]; ok {
		return cmp
	}
	return s.compressor
}

// GetCompressor 获取指定数据类型的压缩器（外部接口）
func (s *TimeSeriesStore) GetCompressor(dt DataType) Compressor {
	return s.getCompressor(dt)
}

// SetCompressor 设置指定数据类型的压缩器（支持运行时替换）
func (s *TimeSeriesStore) SetCompressor(dt DataType, cmp Compressor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compressors[dt] = cmp
}

// GetCompressionRatio 获取指定数据类型的压缩比
func (s *TimeSeriesStore) GetCompressionRatio(dt DataType) float64 {
	return s.getCompressor(dt).Ratio()
}

// GetStats 获取存储统计
func (s *TimeSeriesStore) GetStats() StorageStats {
	stats := StorageStats{
		WriteCount:    atomic.LoadUint64(&s.stats.写入统计),
		ReadCount:     atomic.LoadUint64(&s.stats.读取统计),
		DroppedCount:  atomic.LoadUint64(&s.stats.丢弃统计),
		RawBytes:      atomic.LoadUint64(&s.stats.压缩前字节),
		CompressedBytes: atomic.LoadUint64(&s.stats.压缩后字节),
	}
	
	if stats.RawBytes > 0 {
		stats.CompressionRatio = float64(stats.RawBytes) / float64(stats.CompressedBytes)
	}
	
	// 统计分片和块数量
	s.mu.RLock()
	stats.ShardCount = len(s.shards)
	for _, shard := range s.shards {
		shard.mu.RLock()
		stats.ChunkCount += len(shard.chunks)
		shard.mu.RUnlock()
	}
	s.mu.RUnlock()
	
	return stats
}

// Close 关闭存储
func (s *TimeSeriesStore) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	
	// 刷新剩余数据
	s.flushAll()
	
	s.log.Infof("时序存储已关闭: 统计=%+v", s.GetStats())
	return nil
}

// 辅助函数

func sortDataPoints(points []DataPoint) {
	// 简单插入排序
	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j-1].Timestamp > points[j].Timestamp; j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}
}

func calculateShardID(source string, dt DataType) uint32 {
	// 简单哈希
	hash := uint32(dt) * 31
	for _, c := range source {
		hash = hash*31 + uint32(c)
	}
	return hash
}

func buildTimeSeriesKey(tags map[string]string) string {
	// 构建时间序列键
	result := ""
	for k, v := range tags {
		result += k + "=" + v + ","
	}
	return result
}

// 字节序操作
func putUint64(b []byte, v uint64) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
}

func getUint64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func putUint16(b []byte, v uint16) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
}

func getUint16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func putFloat64(b []byte, v float64) {
	putUint64(b, floatToUint64(v))
}

func getFloat64(b []byte) float64 {
	return uint64ToFloat(getUint64(b))
}

func floatToUint64(f float64) uint64 {
	return math.Float64bits(f)
}

func uint64ToFloat(u uint64) float64 {
	return math.Float64frombits(u)
}

func putInt64(b []byte, v int64) {
	putUint64(b, uint64(v))
}

func getInt64(b []byte) int64 {
	return int64(getUint64(b))
}
