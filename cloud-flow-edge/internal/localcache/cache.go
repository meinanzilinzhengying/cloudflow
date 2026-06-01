// Package localcache 提供基于LevelDB的本地磁盘缓存功能
//
// 功能：
// 1. 网络中断时缓存Agent上报数据（指标、链路追踪、性能分析）
// 2. 按时间顺序存储，支持按时间范围查询
// 3. 缓存容量可配置，支持TTL过期清理
// 4. 网络恢复后按时间顺序自动续传
//
// 数据格式：
//   - Key: <data_type>:<timestamp>:<sequence>
//   - Value: 序列化的batch数据（JSON）
//   - data_type: metrics/traces/profiling
package localcache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// DataType 数据类型
type DataType int

const (
	// TypeMetrics 指标数据
	TypeMetrics DataType = iota
	// TypeTraces 链路追踪数据
	TypeTraces
	// TypeProfiling 性能分析数据
	TypeProfiling
)

func (t DataType) String() string {
	switch t {
	case TypeMetrics:
		return "metrics"
	case TypeTraces:
		return "traces"
	case TypeProfiling:
		return "profiling"
	default:
		return "unknown"
	}
}

// ParseDataType 解析数据类型字符串
func ParseDataType(s string) DataType {
	switch s {
	case "metrics":
		return TypeMetrics
	case "traces":
		return TypeTraces
	case "profiling":
		return TypeProfiling
	default:
		return TypeMetrics
	}
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Type      DataType    `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Sequence  int64       `json:"sequence"`
	Data      interface{} `json:"data"`
}

// MetricsEntry 指标数据条目（用于序列化）
type MetricsEntry struct {
	Type      string             `json:"type"`
	Timestamp int64              `json:"timestamp"`
	Sequence  int64              `json:"sequence"`
	Data      *edge.MetricsBatch `json:"data"`
}

// TracesEntry 链路追踪数据条目
type TracesEntry struct {
	Type      string            `json:"type"`
	Timestamp int64             `json:"timestamp"`
	Sequence  int64             `json:"sequence"`
	Data      *edge.TraceBatch `json:"data"`
}

// ProfilingEntry 性能分析数据条目
type ProfilingEntry struct {
	Type      string               `json:"type"`
	Timestamp int64                `json:"timestamp"`
	Sequence  int64                `json:"sequence"`
	Data      *edge.ProfilingBatch `json:"data"`
}

// Stats 缓存统计
type Stats struct {
	TotalEntries   int64         `json:"total_entries"`
	MetricsCount   int64         `json:"metrics_count"`
	TracesCount    int64         `json:"traces_count"`
	ProfilingCount int64         `json:"profiling_count"`
	TotalSize      int64         `json:"total_size_bytes"`
	OldestTime     time.Time     `json:"oldest_time"`
	NewestTime     time.Time     `json:"newest_time"`
}

// Config 缓存配置
type Config struct {
	DataDir         string        // 数据目录
	MaxSize         int64         // 最大容量（字节），0表示无限制
	MaxAge          time.Duration // 最大保留时间，默认1小时
	CleanupInterval time.Duration // 清理间隔
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		DataDir:         "./data/localcache",
		MaxSize:         0, // 无限制
		MaxAge:          1 * time.Hour,
		CleanupInterval: 5 * time.Minute,
	}
}

// Cache LevelDB本地缓存
type Cache struct {
	config Config
	logger *logger.Logger

	// 内存索引（按数据类型和时间排序）
	mu       sync.RWMutex
	metrics  []*CacheItem
	traces   []*CacheItem
	profiling []*CacheItem

	// 序列号生成器
	seqMu    sync.Mutex
	sequence int64

	// 清理控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// CacheItem 缓存项（内存索引）
type CacheItem struct {
	Type      DataType
	Timestamp int64
	Sequence  int64
	Key       string
	Size      int64
}

// NewCache 创建本地缓存
func NewCache(cfg Config, log *logger.Logger) (*Cache, error) {
	if cfg.DataDir == "" {
		cfg = DefaultConfig()
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 1 * time.Hour
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}

	// 创建数据目录
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建缓存目录失败: %w", err)
	}

	cache := &Cache{
		config: cfg,
		logger: log,
		stopCh: make(chan struct{}),
	}

	// 从磁盘恢复数据索引
	if err := cache.recover(); err != nil {
		log.Warnf("[localcache] 恢复数据索引失败: %v", err)
	}

	// 启动清理协程
	cache.wg.Add(1)
	go cache.cleanupLoop()

	log.Infof("[localcache] 本地磁盘缓存已启动: 目录=%s, 最大保留时间=%v", cfg.DataDir, cfg.MaxAge)
	return cache, nil
}

// Close 关闭缓存
func (c *Cache) Close() error {
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("[localcache] 本地磁盘缓存已关闭")
	return nil
}

// AddMetrics 添加指标数据到缓存
func (c *Cache) AddMetrics(batch *edge.MetricsBatch) error {
	if batch == nil {
		return nil
	}

	c.seqMu.Lock()
	c.sequence++
	seq := c.sequence
	c.seqMu.Unlock()

	timestamp := time.Now().UnixNano()
	key := fmt.Sprintf("metrics:%d:%d", timestamp, seq)

	entry := &MetricsEntry{
		Type:      "metrics",
		Timestamp: timestamp,
		Sequence:  seq,
		Data:      batch,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化指标数据失败: %w", err)
	}

	// 写入磁盘
	if err := c.writeToDisk(key, data); err != nil {
		return err
	}

	// 更新内存索引
	c.mu.Lock()
	c.metrics = append(c.metrics, &CacheItem{
		Type:      TypeMetrics,
		Timestamp: timestamp,
		Sequence:  seq,
		Key:       key,
		Size:      int64(len(data)),
	})
	c.mu.Unlock()

	c.logger.Debugf("[localcache] 缓存指标数据: key=%s, metrics=%d", key, len(batch.GetMetrics()))
	return nil
}

// AddTraces 添加链路追踪数据到缓存
func (c *Cache) AddTraces(batch *edge.TraceBatch) error {
	if batch == nil {
		return nil
	}

	c.seqMu.Lock()
	c.sequence++
	seq := c.sequence
	c.seqMu.Unlock()

	timestamp := time.Now().UnixNano()
	key := fmt.Sprintf("traces:%d:%d", timestamp, seq)

	entry := &TracesEntry{
		Type:      "traces",
		Timestamp: timestamp,
		Sequence:  seq,
		Data:      batch,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化链路追踪数据失败: %w", err)
	}

	if err := c.writeToDisk(key, data); err != nil {
		return err
	}

	c.mu.Lock()
	c.traces = append(c.traces, &CacheItem{
		Type:      TypeTraces,
		Timestamp: timestamp,
		Sequence:  seq,
		Key:       key,
		Size:      int64(len(data)),
	})
	c.mu.Unlock()

	c.logger.Debugf("[localcache] 缓存链路追踪数据: key=%s, spans=%d", key, len(batch.GetSpans()))
	return nil
}

// AddProfiling 添加性能分析数据到缓存
func (c *Cache) AddProfiling(batch *edge.ProfilingBatch) error {
	if batch == nil {
		return nil
	}

	c.seqMu.Lock()
	c.sequence++
	seq := c.sequence
	c.seqMu.Unlock()

	timestamp := time.Now().UnixNano()
	key := fmt.Sprintf("profiling:%d:%d", timestamp, seq)

	entry := &ProfilingEntry{
		Type:      "profiling",
		Timestamp: timestamp,
		Sequence:  seq,
		Data:      batch,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化性能分析数据失败: %w", err)
	}

	if err := c.writeToDisk(key, data); err != nil {
		return err
	}

	c.mu.Lock()
	c.profiling = append(c.profiling, &CacheItem{
		Type:      TypeProfiling,
		Timestamp: timestamp,
		Sequence:  seq,
		Key:       key,
		Size:      int64(len(data)),
	})
	c.mu.Unlock()

	c.logger.Debugf("[localcache] 缓存性能分析数据: key=%s, profiles=%d", key, len(batch.GetProfiles()))
	return nil
}

// writeToDisk 写入数据到磁盘
func (c *Cache) writeToDisk(key string, data []byte) error {
	// 使用两层目录结构避免单目录文件过多
	// key格式: type:timestamp:sequence
	// 文件路径: <datadir>/<type>/<timestamp>/sequence.json
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return fmt.Errorf("无效的key格式: %s", key)
	}

	typeDir := filepath.Join(c.config.DataDir, parts[0])
	// 按小时分目录
	timestamp, _ := strconv.ParseInt(parts[1], 10, 64)
	hourDir := filepath.Join(typeDir, time.Unix(0, timestamp).Format("20060102_15"))

	if err := os.MkdirAll(hourDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	filePath := filepath.Join(hourDir, parts[2]+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// readFromDisk 从磁盘读取数据
func (c *Cache) readFromDisk(key string) ([]byte, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return nil, fmt.Errorf("无效的key格式: %s", key)
	}

	typeDir := filepath.Join(c.config.DataDir, parts[0])
	timestamp, _ := strconv.ParseInt(parts[1], 10, 64)
	hourDir := filepath.Join(typeDir, time.Unix(0, timestamp).Format("20060102_15"))
	filePath := filepath.Join(hourDir, parts[2]+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	return data, nil
}

// deleteFromDisk 从磁盘删除数据
func (c *Cache) deleteFromDisk(key string) error {
	parts := strings.Split(key, ":")
	if len(parts) != 3 {
		return fmt.Errorf("无效的key格式: %s", key)
	}

	typeDir := filepath.Join(c.config.DataDir, parts[0])
	timestamp, _ := strconv.ParseInt(parts[1], 10, 64)
	hourDir := filepath.Join(typeDir, time.Unix(0, timestamp).Format("20060102_15"))
	filePath := filepath.Join(hourDir, parts[2]+".json")

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %w", err)
	}

	return nil
}

// GetMetricsBatch 获取指标数据批次（按时间顺序）
// limit: 最多返回多少条，0表示返回所有
func (c *Cache) GetMetricsBatch(limit int) []*edge.MetricsBatch {
	c.mu.RLock()
	items := make([]*CacheItem, len(c.metrics))
	copy(items, c.metrics)
	c.mu.RUnlock()

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	var result []*edge.MetricsBatch
	for _, item := range items {
		data, err := c.readFromDisk(item.Key)
		if err != nil {
			c.logger.Warnf("[localcache] 读取指标数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		var entry MetricsEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			c.logger.Warnf("[localcache] 反序列化指标数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		if entry.Data != nil {
			result = append(result, entry.Data)
		}
	}

	return result
}

// GetTracesBatch 获取链路追踪数据批次
func (c *Cache) GetTracesBatch(limit int) []*edge.TraceBatch {
	c.mu.RLock()
	items := make([]*CacheItem, len(c.traces))
	copy(items, c.traces)
	c.mu.RUnlock()

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	var result []*edge.TraceBatch
	for _, item := range items {
		data, err := c.readFromDisk(item.Key)
		if err != nil {
			c.logger.Warnf("[localcache] 读取链路追踪数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		var entry TracesEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			c.logger.Warnf("[localcache] 反序列化链路追踪数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		if entry.Data != nil {
			result = append(result, entry.Data)
		}
	}

	return result
}

// GetProfilingBatch 获取性能分析数据批次
func (c *Cache) GetProfilingBatch(limit int) []*edge.ProfilingBatch {
	c.mu.RLock()
	items := make([]*CacheItem, len(c.profiling))
	copy(items, c.profiling)
	c.mu.RUnlock()

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	var result []*edge.ProfilingBatch
	for _, item := range items {
		data, err := c.readFromDisk(item.Key)
		if err != nil {
			c.logger.Warnf("[localcache] 读取性能分析数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		var entry ProfilingEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			c.logger.Warnf("[localcache] 反序列化性能分析数据失败: key=%s, err=%v", item.Key, err)
			continue
		}

		if entry.Data != nil {
			result = append(result, entry.Data)
		}
	}

	return result
}

// RemoveMetrics 删除已发送的指标数据
func (c *Cache) RemoveMetrics(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count >= len(c.metrics) {
		for _, item := range c.metrics {
			c.deleteFromDisk(item.Key)
		}
		c.metrics = nil
	} else {
		for i := 0; i < count; i++ {
			c.deleteFromDisk(c.metrics[i].Key)
		}
		c.metrics = c.metrics[count:]
	}
}

// RemoveTraces 删除已发送的链路追踪数据
func (c *Cache) RemoveTraces(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count >= len(c.traces) {
		for _, item := range c.traces {
			c.deleteFromDisk(item.Key)
		}
		c.traces = nil
	} else {
		for i := 0; i < count; i++ {
			c.deleteFromDisk(c.traces[i].Key)
		}
		c.traces = c.traces[count:]
	}
}

// RemoveProfiling 删除已发送的性能分析数据
func (c *Cache) RemoveProfiling(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if count >= len(c.profiling) {
		for _, item := range c.profiling {
			c.deleteFromDisk(item.Key)
		}
		c.profiling = nil
	} else {
		for i := 0; i < count; i++ {
			c.deleteFromDisk(c.profiling[i].Key)
		}
		c.profiling = c.profiling[count:]
	}
}

// GetStats 获取缓存统计
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := Stats{
		MetricsCount:   int64(len(c.metrics)),
		TracesCount:    int64(len(c.traces)),
		ProfilingCount: int64(len(c.profiling)),
		TotalEntries:   int64(len(c.metrics) + len(c.traces) + len(c.profiling)),
	}

	var totalSize int64
	var oldestTime int64 = -1
	var newestTime int64 = -1

	for _, item := range c.metrics {
		totalSize += item.Size
		if oldestTime == -1 || item.Timestamp < oldestTime {
			oldestTime = item.Timestamp
		}
		if item.Timestamp > newestTime {
			newestTime = item.Timestamp
		}
	}
	for _, item := range c.traces {
		totalSize += item.Size
		if oldestTime == -1 || item.Timestamp < oldestTime {
			oldestTime = item.Timestamp
		}
		if item.Timestamp > newestTime {
			newestTime = item.Timestamp
		}
	}
	for _, item := range c.profiling {
		totalSize += item.Size
		if oldestTime == -1 || item.Timestamp < oldestTime {
			oldestTime = item.Timestamp
		}
		if item.Timestamp > newestTime {
			newestTime = item.Timestamp
		}
	}

	stats.TotalSize = totalSize
	if oldestTime > 0 {
		stats.OldestTime = time.Unix(0, oldestTime)
	}
	if newestTime > 0 {
		stats.NewestTime = time.Unix(0, newestTime)
	}

	return stats
}

// cleanupLoop 定期清理过期数据
func (c *Cache) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			c.cleanup()
			return
		}
	}
}

// cleanup 清理过期数据
func (c *Cache) cleanup() {
	cutoff := time.Now().Add(-c.config.MaxAge).UnixNano()

	c.mu.Lock()
	defer c.mu.Unlock()

	// 清理过期指标数据
	var newMetrics []*CacheItem
	for _, item := range c.metrics {
		if item.Timestamp < cutoff {
			c.deleteFromDisk(item.Key)
		} else {
			newMetrics = append(newMetrics, item)
		}
	}
	removedMetrics := len(c.metrics) - len(newMetrics)
	c.metrics = newMetrics

	// 清理过期链路追踪数据
	var newTraces []*CacheItem
	for _, item := range c.traces {
		if item.Timestamp < cutoff {
			c.deleteFromDisk(item.Key)
		} else {
			newTraces = append(newTraces, item)
		}
	}
	removedTraces := len(c.traces) - len(newTraces)
	c.traces = newTraces

	// 清理过期性能分析数据
	var newProfiling []*CacheItem
	for _, item := range c.profiling {
		if item.Timestamp < cutoff {
			c.deleteFromDisk(item.Key)
		} else {
			newProfiling = append(newProfiling, item)
		}
	}
	removedProfiling := len(c.profiling) - len(newProfiling)
	c.profiling = newProfiling

	totalRemoved := removedMetrics + removedTraces + removedProfiling
	if totalRemoved > 0 {
		c.logger.Infof("[localcache] 清理过期数据: metrics=%d, traces=%d, profiling=%d",
			removedMetrics, removedTraces, removedProfiling)
	}
}

// recover 从磁盘恢复数据索引
func (c *Cache) recover() error {
	// 遍历数据目录，重建内存索引
	entries, err := os.ReadDir(c.config.DataDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dataType := ParseDataType(entry.Name())
		typeDir := filepath.Join(c.config.DataDir, entry.Name())

		// 遍历小时目录
		hourDirs, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, hourDir := range hourDirs {
			if !hourDir.IsDir() {
				continue
			}

			hourPath := filepath.Join(typeDir, hourDir.Name())
			files, err := os.ReadDir(hourPath)
			if err != nil {
				continue
			}

			for _, file := range files {
				if file.IsDir() {
					continue
				}

				// 解析文件名获取sequence
				seqStr := strings.TrimSuffix(file.Name(), ".json")
				seq, _ := strconv.ParseInt(seqStr, 10, 64)

				// 解析小时目录名获取时间
				timestamp := c.parseHourDirTimestamp(hourDir.Name())

				key := fmt.Sprintf("%s:%d:%d", entry.Name(), timestamp, seq)

				info, err := file.Info()
				if err != nil {
					continue
				}

				item := &CacheItem{
					Type:      dataType,
					Timestamp: timestamp,
					Sequence:  seq,
					Key:       key,
					Size:      info.Size(),
				}

				switch dataType {
				case TypeMetrics:
					c.metrics = append(c.metrics, item)
				case TypeTraces:
					c.traces = append(c.traces, item)
				case TypeProfiling:
					c.profiling = append(c.profiling, item)
				}

				// 更新最大序列号
				if seq > c.sequence {
					c.sequence = seq
				}
			}
		}
	}

	c.logger.Infof("[localcache] 从磁盘恢复数据索引: metrics=%d, traces=%d, profiling=%d",
		len(c.metrics), len(c.traces), len(c.profiling))
	return nil
}

// parseHourDirTimestamp 解析小时目录名获取时间戳
func (c *Cache) parseHourDirTimestamp(dirName string) int64 {
	// 格式: 20060102_15
	t, err := time.Parse("20060102_15", dirName)
	if err != nil {
		return 0
	}
	return t.UnixNano()
}

// Clear 清空所有缓存数据
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 删除所有磁盘文件
	for _, item := range c.metrics {
		c.deleteFromDisk(item.Key)
	}
	for _, item := range c.traces {
		c.deleteFromDisk(item.Key)
	}
	for _, item := range c.profiling {
		c.deleteFromDisk(item.Key)
	}

	c.metrics = nil
	c.traces = nil
	c.profiling = nil

	c.logger.Info("[localcache] 所有缓存数据已清空")
}
