// Package aggregator 提供边缘数据聚合功能
//
// 功能：
// 1. 按1分钟粒度聚合原始数据
// 2. 按资产、协议、指标维度分组聚合
// 3. 去除重复、无效数据
// 4. 减少向Center上报的数据量≥50%
//
// 聚合维度：
//   - 资产维度：ProbeID + 源IP/目的IP
//   - 协议维度：TCP/UDP/HTTP等
//   - 指标维度：流量、延迟、错误率等
package aggregator

import (
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"cloudflow-edge/pkg/logger"
	edge "cloudflow/proto"
)

// ============================================================================
// 配置
// ============================================================================

// Config 聚合器配置
type Config struct {
	WindowSize      time.Duration // 聚合窗口大小（默认1分钟）
	MaxWindowCount  int           // 最大窗口数量（默认10个）
	Deduplication   bool          // 是否启用去重（默认启用）
	FilterInvalid   bool          // 是否过滤无效数据（默认启用）
	AggregationKeys []string      // 聚合维度（默认["probe_id", "src_ip", "dst_ip", "protocol"]）
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		WindowSize:      1 * time.Minute,
		MaxWindowCount:  10,
		Deduplication:   true,
		FilterInvalid:   true,
		AggregationKeys: []string{"probe_id", "src_ip", "dst_ip", "protocol"},
	}
}

// ============================================================================
// 数据结构
// ============================================================================

// AggregationKey 聚合键
type AggregationKey struct {
	ProbeID  string
	SrcIP    string
	DstIP    string
	Protocol string
}

// String 返回聚合键的字符串表示
func (k AggregationKey) String() string {
	return fmt.Sprintf("%s|%s|%s|%s", k.ProbeID, k.SrcIP, k.DstIP, k.Protocol)
}

// Hash 返回聚合键的哈希值
func (k AggregationKey) Hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(k.String()))
	return h.Sum64()
}

// AggregatedMetrics 聚合后的指标数据
type AggregatedMetrics struct {
	Key           AggregationKey `json:"key"`
	WindowStart   int64          `json:"window_start"`   // 窗口开始时间戳
	WindowEnd     int64          `json:"window_end"`     // 窗口结束时间戳
	Count         int64          `json:"count"`          // 原始数据条数
	TotalBytes    int64          `json:"total_bytes"`    // 总字节数
	TotalPackets  int64          `json:"total_packets"`  // 总包数
	TotalLatency  int64          `json:"total_latency"`  // 总延迟（用于计算平均延迟）
	MinLatency    int64          `json:"min_latency"`    // 最小延迟
	MaxLatency    int64          `json:"max_latency"`    // 最大延迟
	AvgLatency    int64          `json:"avg_latency"`    // 平均延迟
	ErrorCount    int64          `json:"error_count"`    // 错误计数
	SrcPorts      map[int32]int  `json:"src_ports"`      // 源端口分布
	DstPorts      map[int32]int  `json:"dst_ports"`      // 目的端口分布
	Tags          map[string]string `json:"tags"`        // 标签（合并）
}

// Stats 聚合统计
type Stats struct {
	TotalInputRecords    int64         `json:"total_input_records"`     // 输入总记录数
	TotalOutputRecords   int64         `json:"total_output_records"`    // 输出总记录数
	DedupRemovedRecords  int64         `json:"dedup_removed_records"`   // 去重移除记录数
	FilteredRecords      int64         `json:"filtered_records"`        // 过滤记录数
	AggregationRatio     float64       `json:"aggregation_ratio"`       // 聚合比例（输出/输入）
	CompressionRatio     float64       `json:"compression_ratio"`       // 压缩比例（1 - 输出/输入）
	ActiveWindows        int           `json:"active_windows"`          // 活跃窗口数
	LastWindowTime       time.Time     `json:"last_window_time"`        // 最后窗口时间
}

// ============================================================================
// 聚合窗口
// ============================================================================

// Window 单个时间窗口的聚合数据
type Window struct {
	StartTime int64                        `json:"start_time"`
	EndTime   int64                        `json:"end_time"`
	Metrics   map[uint64]*AggregatedMetrics `json:"metrics"` // key hash -> aggregated data
	SeenKeys  map[string]bool              `json:"seen_keys"` // 用于去重
}

// newWindow 创建新窗口
func newWindow(startTime int64, windowSize time.Duration) *Window {
	return &Window{
		StartTime: startTime,
		EndTime:   startTime + int64(windowSize.Seconds()*1e9),
		Metrics:   make(map[uint64]*AggregatedMetrics),
		SeenKeys:  make(map[string]bool),
	}
}

// ============================================================================
// 聚合器
// ============================================================================

// Aggregator 数据聚合器
type Aggregator struct {
	config Config
	logger *logger.Logger

	// 窗口管理
	mu       sync.RWMutex
	windows  []*Window          // 按时间排序的窗口列表
	windowMu sync.Mutex         // 窗口操作的互斥锁

	// 统计
	statsMu          sync.RWMutex
	totalInput       int64
	totalOutput      int64
	dedupRemoved     int64
	filtered         int64

	// 控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewAggregator 创建聚合器
func NewAggregator(cfg Config, log *logger.Logger) *Aggregator {
	if cfg.WindowSize == 0 {
		cfg = DefaultConfig()
	}

	return &Aggregator{
		config: cfg,
		logger: log,
		windows: make([]*Window, 0, cfg.MaxWindowCount),
		stopCh: make(chan struct{}),
	}
}

// Start 启动聚合器
func (a *Aggregator) Start() {
	// 启动窗口清理协程
	a.wg.Add(1)
	go a.cleanupLoop()
	a.logger.Infof("[aggregator] 聚合器已启动: 窗口大小=%v, 最大窗口数=%d", a.config.WindowSize, a.config.MaxWindowCount)
}

// Stop 停止聚合器
func (a *Aggregator) Stop() {
	close(a.stopCh)
	a.wg.Wait()
	a.logger.Info("[aggregator] 聚合器已停止")
}

// cleanupLoop 定期清理过期窗口
func (a *Aggregator) cleanupLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.WindowSize / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.cleanupExpiredWindows()
		case <-a.stopCh:
			return
		}
	}
}

// cleanupExpiredWindows 清理过期窗口
func (a *Aggregator) cleanupExpiredWindows() {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()

	if len(a.windows) == 0 {
		return
	}

	now := time.Now().UnixNano()
	maxAge := int64(a.config.WindowSize) * int64(a.config.MaxWindowCount)

	var newWindows []*Window
	for _, w := range a.windows {
		if now-w.EndTime < maxAge {
			newWindows = append(newWindows, w)
		}
	}

	if len(newWindows) != len(a.windows) {
		removed := len(a.windows) - len(newWindows)
		a.windows = newWindows
		a.logger.Debugf("[aggregator] 清理 %d 个过期窗口", removed)
	}
}

// getOrCreateWindow 获取或创建窗口 (调用方需持有 windowMu)
func (a *Aggregator) getOrCreateWindow(timestamp int64) *Window {
	windowSize := int64(a.config.WindowSize.Seconds() * 1e9)
	windowStart := (timestamp / windowSize) * windowSize

	// 查找现有窗口
	for _, w := range a.windows {
		if w.StartTime == windowStart {
			return w
		}
	}

	// 创建新窗口
	window := newWindow(windowStart, a.config.WindowSize)
	a.windows = append(a.windows, window)

	// 如果窗口过多，移除最旧的
	if len(a.windows) > a.config.MaxWindowCount {
		a.windows = a.windows[1:]
	}

	return window
}

// AddMetrics 添加指标数据到聚合器
func (a *Aggregator) AddMetrics(metrics []*edge.MetricData) {
	for _, m := range metrics {
		a.addMetric(m)
	}
}

// addMetric 添加单条指标数据
func (a *Aggregator) addMetric(m *edge.MetricData) {
	// 统计输入
	a.statsMu.Lock()
	a.totalInput++
	a.statsMu.Unlock()

	// 过滤无效数据
	if a.config.FilterInvalid && !a.isValidMetric(m) {
		a.statsMu.Lock()
		a.filtered++
		a.statsMu.Unlock()
		return
	}

	// 构建聚合键
	key := AggregationKey{
		ProbeID:  m.GetProbeId(),
		SrcIP:    m.GetSrcIp(),
		DstIP:    m.GetDstIp(),
		Protocol: m.GetProtocol(),
	}

	// 获取窗口 — 先持有 windowMu，再操作 statsMu
	a.windowMu.Lock()
	window := a.getOrCreateWindow(m.GetTimestamp())

	// 去重检查
	if a.config.Deduplication {
		uniqueKey := fmt.Sprintf("%s|%d|%d", key.String(), m.GetSrcPort(), m.GetDstPort())
		if window.SeenKeys[uniqueKey] {
			a.statsMu.Lock()
			a.dedupRemoved++
			a.statsMu.Unlock()
			a.windowMu.Unlock()
			return
		}
		window.SeenKeys[uniqueKey] = true
	}

	// 聚合数据
	hash := key.Hash()
	agg, exists := window.Metrics[hash]
	if !exists {
		agg = &AggregatedMetrics{
			Key:          key,
			WindowStart:  window.StartTime,
			WindowEnd:    window.EndTime,
			MinLatency:   m.GetLatency(),
			MaxLatency:   m.GetLatency(),
			SrcPorts:     make(map[int32]int),
			DstPorts:     make(map[int32]int),
			Tags:         make(map[string]string),
		}
		window.Metrics[hash] = agg
	}

	// 更新聚合值
	agg.Count++
	agg.TotalBytes += m.GetBytes()
	agg.TotalPackets += m.GetPackets()
	agg.TotalLatency += m.GetLatency()

	if m.GetLatency() < agg.MinLatency {
		agg.MinLatency = m.GetLatency()
	}
	if m.GetLatency() > agg.MaxLatency {
		agg.MaxLatency = m.GetLatency()
	}

	// 端口分布
	agg.SrcPorts[m.GetSrcPort()]++
	agg.DstPorts[m.GetDstPort()]++

	// 合并标签
	for k, v := range m.GetTags() {
		if _, exists := agg.Tags[k]; !exists {
			agg.Tags[k] = v
		}
	}

	// 计算平均延迟
	agg.AvgLatency = agg.TotalLatency / agg.Count

	a.windowMu.Unlock()
}

// isValidMetric 检查指标数据是否有效
func (a *Aggregator) isValidMetric(m *edge.MetricData) bool {
	// 检查必需字段
	if m.GetProbeId() == "" {
		return false
	}
	if m.GetTimestamp() <= 0 {
		return false
	}
	if m.GetSrcIp() == "" && m.GetDstIp() == "" {
		return false
	}
	// 过滤异常值
	if m.GetBytes() < 0 || m.GetPackets() < 0 {
		return false
	}
	if m.GetLatency() < 0 || m.GetLatency() > 3600000 { // 延迟超过1小时视为异常
		return false
	}
	return true
}

// GetAggregatedMetrics 获取指定时间窗口的聚合结果
func (a *Aggregator) GetAggregatedMetrics(windowStart int64) []*AggregatedMetrics {
	a.windowMu.RLock()
	defer a.windowMu.RUnlock()

	for _, w := range a.windows {
		if w.StartTime == windowStart {
			result := make([]*AggregatedMetrics, 0, len(w.Metrics))
			for _, agg := range w.Metrics {
				result = append(result, agg)
			}

			// 统计输出
			a.statsMu.Lock()
			a.totalOutput += int64(len(result))
			a.statsMu.Unlock()

			return result
		}
	}
	return nil
}

// GetCompletedWindows 获取已完成的窗口（可以上报的）
func (a *Aggregator) GetCompletedWindows() []int64 {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()

	now := time.Now().UnixNano()
	var completed []int64

	for _, w := range a.windows {
		// 窗口结束时间已过，且不是当前窗口
		if w.EndTime < now {
			completed = append(completed, w.StartTime)
		}
	}

	return completed
}

// RemoveWindow 移除指定窗口（上报成功后调用）
func (a *Aggregator) RemoveWindow(windowStart int64) {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()

	for i, w := range a.windows {
		if w.StartTime == windowStart {
			// 从列表中移除
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			a.logger.Debugf("[aggregator] 移除窗口: start=%d", windowStart)
			return
		}
	}
}

// ConvertToMetricsBatch 将聚合结果转换为MetricsBatch
func (a *Aggregator) ConvertToMetricsBatch(aggregated []*AggregatedMetrics, probeID string) *edge.MetricsBatch {
	metrics := make([]*edge.MetricData, 0, len(aggregated))

	for _, agg := range aggregated {
		// 选择最常见的源端口和目的端口
		srcPort := a.getMostCommonPort(agg.SrcPorts)
		dstPort := a.getMostCommonPort(agg.DstPorts)

		m := &edge.MetricData{
			ProbeId:   probeID,
			Timestamp: agg.WindowStart,
			SrcIp:     agg.Key.SrcIP,
			DstIp:     agg.Key.DstIP,
			SrcPort:   srcPort,
			DstPort:   dstPort,
			Protocol:  agg.Key.Protocol,
			Bytes:     agg.TotalBytes,
			Packets:   agg.TotalPackets,
			Latency:   agg.AvgLatency,
			Tags:      agg.Tags,
		}
		metrics = append(metrics, m)
	}

	return &edge.MetricsBatch{
		ProbeId:   probeID,
		Metrics:   metrics,
		Timestamp: time.Now().UnixNano(),
	}
}

// getMostCommonPort 获取最常见的端口
func (a *Aggregator) getMostCommonPort(portMap map[int32]int) int32 {
	var maxPort int32
	var maxCount int
	for port, count := range portMap {
		if count > maxCount {
			maxCount = count
			maxPort = port
		}
	}
	return maxPort
}

// GetStats 获取聚合统计
func (a *Aggregator) GetStats() Stats {
	a.statsMu.RLock()
	defer a.statsMu.RUnlock()

	a.windowMu.RLock()
	activeWindows := len(a.windows)
	var lastWindowTime time.Time
	if len(a.windows) > 0 {
		lastWindowTime = time.Unix(0, a.windows[len(a.windows)-1].StartTime)
	}
	a.windowMu.RUnlock()

	stats := Stats{
		TotalInputRecords:   a.totalInput,
		TotalOutputRecords:  a.totalOutput,
		DedupRemovedRecords: a.dedupRemoved,
		FilteredRecords:     a.filtered,
		ActiveWindows:       activeWindows,
		LastWindowTime:      lastWindowTime,
	}

	if a.totalInput > 0 {
		stats.AggregationRatio = float64(a.totalOutput) / float64(a.totalInput)
		stats.CompressionRatio = 1.0 - stats.AggregationRatio
	}

	return stats
}

// ResetStats 重置统计
func (a *Aggregator) ResetStats() {
	a.statsMu.Lock()
	defer a.statsMu.Unlock()

	a.totalInput = 0
	a.totalOutput = 0
	a.dedupRemoved = 0
	a.filtered = 0
}

// Clear 清空所有窗口
func (a *Aggregator) Clear() {
	a.windowMu.Lock()
	defer a.windowMu.Unlock()

	a.windows = make([]*Window, 0, a.config.MaxWindowCount)
	a.logger.Info("[aggregator] 所有聚合窗口已清空")
}
