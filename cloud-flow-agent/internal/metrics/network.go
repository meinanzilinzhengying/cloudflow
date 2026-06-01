package metrics

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// NetworkMetrics 网络指标数据
type NetworkMetrics struct {
	Timestamp   time.Time `json:"timestamp"`
	BytesIn     uint64    `json:"bytes_in"`      // 入站字节数
	BytesOut    uint64    `json:"bytes_out"`     // 出站字节数
	PacketsIn   uint64    `json:"packets_in"`    // 入站包数
	PacketsOut  uint64    `json:"packets_out"`   // 出站包数
	Connections uint64    `json:"connections"`   // 连接数
	Retransmits uint64    `json:"retransmits"`   // 重传次数
	LatencyMs   float64   `json:"latency_ms"`    // 平均时延(ms)
	LatencyMax  float64   `json:"latency_max_ms"` // 最大时延(ms)
	LatencyP99  float64   `json:"latency_p99_ms"` // P99时延(ms)
}

// TrafficDistribution 流量分布
type TrafficDistribution struct {
	TotalBytes   uint64                 `json:"total_bytes"`
	TotalPackets uint64                 `json:"total_packets"`
	ByProtocol   map[string]uint64      `json:"by_protocol"`   // 按协议分布
	ByPort       map[string]uint64      `json:"by_port"`        // 按端口分布
	ByDirection  map[string]uint64      `json:"by_direction"`   // 按方向分布
	TopTalkers   []TopTalker            `json:"top_talkers"`    // Top流量来源
}

// TopTalker Top流量来源
type TopTalker struct {
	IP        string `json:"ip"`
	Bytes     uint64 `json:"bytes"`
	Packets   uint64 `json:"packets"`
	Direction string `json:"direction"`
}

// LatencyTrend 时延趋势
type LatencyTrend struct {
	Timestamps []time.Time `json:"timestamps"`
	AvgLatency []float64   `json:"avg_latency"`   // 平均时延
	P50Latency []float64   `json:"p50_latency"`   // P50时延
	P95Latency []float64   `json:"p95_latency"`   // P95时延
	P99Latency []float64   `json:"p99_latency"`   // P99时延
	MaxLatency []float64   `json:"max_latency"`   // 最大时延
}

// RetransmitTrend 重传趋势
type RetransmitTrend struct {
	Timestamps      []time.Time `json:"timestamps"`
	RetransmitCount []uint64    `json:"retransmit_count"`   // 重传次数
	RetransmitRate  []float64   `json:"retransmit_rate"`    // 重传率(%)
	TimeoutCount    []uint64    `json:"timeout_count"`      // 超时次数
	FastRetransmit  []uint64    `json:"fast_retransmit"`    // 快速重传次数
}

// NetworkCollector 网络指标采集器
type NetworkCollector struct {
	mu       sync.RWMutex
	metrics  []NetworkMetrics
	maxSize  int
	interval time.Duration
	
	// 实时统计
	trafficDist    TrafficDistribution
	latencyTrend   LatencyTrend
	retransmitTrend RetransmitTrend
}

// NewNetworkCollector 创建网络指标采集器
func NewNetworkCollector(maxSize int, interval time.Duration) *NetworkCollector {
	if maxSize <= 0 {
		maxSize = 1440 // 默认保存24小时(1分钟间隔)
	}
	if interval <= 0 {
		interval = time.Minute
	}
	
	return &NetworkCollector{
		maxSize:  maxSize,
		interval: interval,
		metrics:  make([]NetworkMetrics, 0, maxSize),
		trafficDist: TrafficDistribution{
			ByProtocol:  make(map[string]uint64),
			ByPort:      make(map[string]uint64),
			ByDirection: make(map[string]uint64),
			TopTalkers:  make([]TopTalker, 0),
		},
		latencyTrend: LatencyTrend{
			Timestamps: make([]time.Time, 0),
			AvgLatency: make([]float64, 0),
			P50Latency: make([]float64, 0),
			P95Latency: make([]float64, 0),
			P99Latency: make([]float64, 0),
			MaxLatency: make([]float64, 0),
		},
		retransmitTrend: RetransmitTrend{
			Timestamps:      make([]time.Time, 0),
			RetransmitCount: make([]uint64, 0),
			RetransmitRate:  make([]float64, 0),
			TimeoutCount:    make([]uint64, 0),
			FastRetransmit:  make([]uint64, 0),
		},
	}
}

// RecordMetrics 记录网络指标
func (c *NetworkCollector) RecordMetrics(m NetworkMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 添加新指标
	c.metrics = append(c.metrics, m)
	
	// 限制大小
	if len(c.metrics) > c.maxSize {
		c.metrics = c.metrics[len(c.metrics)-c.maxSize:]
	}
	
	// 更新流量分布
	c.updateTrafficDistribution(m)
	
	// 更新时延趋势
	c.updateLatencyTrend(m)
	
	// 更新重传趋势
	c.updateRetransmitTrend(m)
}

// updateTrafficDistribution 更新流量分布
func (c *NetworkCollector) updateTrafficDistribution(m NetworkMetrics) {
	c.trafficDist.TotalBytes += m.BytesIn + m.BytesOut
	c.trafficDist.TotalPackets += m.PacketsIn + m.PacketsOut
	
	// 按方向分布
	c.trafficDist.ByDirection["in"] += m.BytesIn
	c.trafficDist.ByDirection["out"] += m.BytesOut
}

// updateLatencyTrend 更新时延趋势
func (c *NetworkCollector) updateLatencyTrend(m NetworkMetrics) {
	c.latencyTrend.Timestamps = append(c.latencyTrend.Timestamps, m.Timestamp)
	c.latencyTrend.AvgLatency = append(c.latencyTrend.AvgLatency, m.LatencyMs)
	c.latencyTrend.P99Latency = append(c.latencyTrend.P99Latency, m.LatencyP99)
	c.latencyTrend.MaxLatency = append(c.latencyTrend.MaxLatency, m.LatencyMax)
	
	// 限制大小
	if len(c.latencyTrend.Timestamps) > c.maxSize {
		c.latencyTrend.Timestamps = c.latencyTrend.Timestamps[1:]
		c.latencyTrend.AvgLatency = c.latencyTrend.AvgLatency[1:]
		c.latencyTrend.P99Latency = c.latencyTrend.P99Latency[1:]
		c.latencyTrend.MaxLatency = c.latencyTrend.MaxLatency[1:]
	}
}

// updateRetransmitTrend 更新重传趋势
func (c *NetworkCollector) updateRetransmitTrend(m NetworkMetrics) {
	c.retransmitTrend.Timestamps = append(c.retransmitTrend.Timestamps, m.Timestamp)
	c.retransmitTrend.RetransmitCount = append(c.retransmitTrend.RetransmitCount, m.Retransmits)
	
	// 计算重传率
	var rate float64
	if m.PacketsOut > 0 {
		rate = float64(m.Retransmits) / float64(m.PacketsOut) * 100
	}
	c.retransmitTrend.RetransmitRate = append(c.retransmitTrend.RetransmitRate, rate)
	
	// 限制大小
	if len(c.retransmitTrend.Timestamps) > c.maxSize {
		c.retransmitTrend.Timestamps = c.retransmitTrend.Timestamps[1:]
		c.retransmitTrend.RetransmitCount = c.retransmitTrend.RetransmitCount[1:]
		c.retransmitTrend.RetransmitRate = c.retransmitTrend.RetransmitRate[1:]
	}
}

// GetTrafficDistribution 获取流量分布
func (c *NetworkCollector) GetTrafficDistribution(ctx context.Context, duration time.Duration) (*TrafficDistribution, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 复制当前分布
	dist := TrafficDistribution{
		TotalBytes:   c.trafficDist.TotalBytes,
		TotalPackets: c.trafficDist.TotalPackets,
		ByProtocol:   make(map[string]uint64),
		ByPort:       make(map[string]uint64),
		ByDirection:  make(map[string]uint64),
		TopTalkers:   make([]TopTalker, len(c.trafficDist.TopTalkers)),
	}
	
	for k, v := range c.trafficDist.ByProtocol {
		dist.ByProtocol[k] = v
	}
	for k, v := range c.trafficDist.ByPort {
		dist.ByPort[k] = v
	}
	for k, v := range c.trafficDist.ByDirection {
		dist.ByDirection[k] = v
	}
	copy(dist.TopTalkers, c.trafficDist.TopTalkers)
	
	return &dist, nil
}

// GetLatencyTrend 获取时延趋势
func (c *NetworkCollector) GetLatencyTrend(ctx context.Context, duration time.Duration) (*LatencyTrend, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 根据duration过滤数据
	cutoff := time.Now().Add(-duration)
	startIdx := 0
	for i, ts := range c.latencyTrend.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	trend := LatencyTrend{
		Timestamps: c.latencyTrend.Timestamps[startIdx:],
		AvgLatency: c.latencyTrend.AvgLatency[startIdx:],
		P99Latency: c.latencyTrend.P99Latency[startIdx:],
		MaxLatency: c.latencyTrend.MaxLatency[startIdx:],
	}
	
	return &trend, nil
}

// GetRetransmitTrend 获取重传趋势
func (c *NetworkCollector) GetRetransmitTrend(ctx context.Context, duration time.Duration) (*RetransmitTrend, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 根据duration过滤数据
	cutoff := time.Now().Add(-duration)
	startIdx := 0
	for i, ts := range c.retransmitTrend.Timestamps {
		if ts.After(cutoff) {
			startIdx = i
			break
		}
	}
	
	trend := RetransmitTrend{
		Timestamps:      c.retransmitTrend.Timestamps[startIdx:],
		RetransmitCount: c.retransmitTrend.RetransmitCount[startIdx:],
		RetransmitRate:  c.retransmitTrend.RetransmitRate[startIdx:],
	}
	
	return &trend, nil
}

// GetNetworkSummary 获取网络指标汇总
func (c *NetworkCollector) GetNetworkSummary(ctx context.Context) (map[string]interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if len(c.metrics) == 0 {
		return map[string]interface{}{
			"status": "no_data",
		}, nil
	}
	
	// 计算最新值
	latest := c.metrics[len(c.metrics)-1]
	
	// 计算平均值
	var totalBytes, totalPackets, totalRetransmits uint64
	var totalLatency float64
	for _, m := range c.metrics {
		totalBytes += m.BytesIn + m.BytesOut
		totalPackets += m.PacketsIn + m.PacketsOut
		totalRetransmits += m.Retransmits
		totalLatency += m.LatencyMs
	}
	
	count := float64(len(c.metrics))
	avgLatency := totalLatency / count
	
	// 计算重传率
	var retransmitRate float64
	if totalPackets > 0 {
		retransmitRate = float64(totalRetransmits) / float64(totalPackets) * 100
	}
	
	return map[string]interface{}{
		"status":            "ok",
		"latest":            latest,
		"avg_latency_ms":    avgLatency,
		"retransmit_rate":   retransmitRate,
		"total_bytes":       totalBytes,
		"total_packets":     totalPackets,
		"data_points":       len(c.metrics),
	}, nil
}

// UpdateProtocolStats 更新协议统计
func (c *NetworkCollector) UpdateProtocolStats(protocol string, bytes uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.trafficDist.ByProtocol[protocol] += bytes
}

// UpdatePortStats 更新端口统计
func (c *NetworkCollector) UpdatePortStats(port string, bytes uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.trafficDist.ByPort[port] += bytes
}

// UpdateTopTalkers 更新Top流量来源
func (c *NetworkCollector) UpdateTopTalkers(talkers []TopTalker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 合并并排序
	talkerMap := make(map[string]*TopTalker)
	
	// 添加现有数据
	for i := range c.trafficDist.TopTalkers {
		t := &c.trafficDist.TopTalkers[i]
		talkerMap[t.IP] = t
	}
	
	// 添加新数据
	for i := range talkers {
		t := &talkers[i]
		if existing, ok := talkerMap[t.IP]; ok {
			existing.Bytes += t.Bytes
			existing.Packets += t.Packets
		} else {
			talkerMap[t.IP] = t
		}
	}
	
	// 转换为切片并排序
	result := make([]TopTalker, 0, len(talkerMap))
	for _, t := range talkerMap {
		result = append(result, *t)
	}
	
	sort.Slice(result, func(i, j int) bool {
		return result[i].Bytes > result[j].Bytes
	})
	
	// 只保留Top 10
	if len(result) > 10 {
		result = result[:10]
	}
	
	c.trafficDist.TopTalkers = result
}

// CalculatePercentiles 计算时延分位数
func CalculatePercentiles(latencies []float64) (p50, p95, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0
	}
	
	// 复制并排序
	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)
	
	p50 = percentile(sorted, 0.50)
	p95 = percentile(sorted, 0.95)
	p99 = percentile(sorted, 0.99)
	
	return
}

// percentile 计算分位数
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	
	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))
	
	if lower == upper {
		return sorted[lower]
	}
	
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// NetworkMetricsAPI 网络指标API
type NetworkMetricsAPI struct {
	collector *NetworkCollector
}

// NewNetworkMetricsAPI 创建网络指标API
func NewNetworkMetricsAPI(collector *NetworkCollector) *NetworkMetricsAPI {
	return &NetworkMetricsAPI{collector: collector}
}

// RegisterRoutes 注册路由
func (api *NetworkMetricsAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/metrics/network/traffic", api.handleTrafficDistribution)
	mux.HandleFunc("/api/v1/metrics/network/latency", api.handleLatencyTrend)
	mux.HandleFunc("/api/v1/metrics/network/retransmit", api.handleRetransmitTrend)
	mux.HandleFunc("/api/v1/metrics/network/summary", api.handleNetworkSummary)
}

// handleTrafficDistribution 处理流量分布请求
func (api *NetworkMetricsAPI) handleTrafficDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	dist, err := api.collector.GetTrafficDistribution(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, dist)
}

// handleLatencyTrend 处理时延趋势请求
func (api *NetworkMetricsAPI) handleLatencyTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	trend, err := api.collector.GetLatencyTrend(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, trend)
}

// handleRetransmitTrend 处理重传趋势请求
func (api *NetworkMetricsAPI) handleRetransmitTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	duration := parseDuration(r.URL.Query().Get("duration"), time.Hour)
	
	trend, err := api.collector.GetRetransmitTrend(r.Context(), duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, trend)
}

// handleNetworkSummary 处理网络汇总请求
func (api *NetworkMetricsAPI) handleNetworkSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	summary, err := api.collector.GetNetworkSummary(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, summary)
}

// parseDuration 解析时间参数
func parseDuration(value string, defaultDuration time.Duration) time.Duration {
	if value == "" {
		return defaultDuration
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return defaultDuration
	}
	return d
}

// writeJSON 写入JSON响应
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
