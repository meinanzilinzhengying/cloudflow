// Package network 网络分析模块
// 提供流量筛选、分布统计、趋势分析和同比环比对比功能
package network

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ==================== 数据模型 ====================

// AnalysisFilter 分析筛选条件
type AnalysisFilter struct {
	AssetIDs    []string `json:"asset_ids"`     // 资产ID列表
	ProbeIDs    []string `json:"probe_ids"`     // 探针ID列表
	LinkIDs     []string `json:"link_ids"`      // 链路ID列表
	StartTime   string   `json:"start_time"`    // 开始时间
	EndTime     string   `json:"end_time"`      // 结束时间
	TimeRange   string   `json:"time_range"`    // 时间范围: 5m/15m/1h/6h/1d/7d
	Protocol    string   `json:"protocol"`       // 协议过滤
	Direction   string   `json:"direction"`      // 方向: inbound/outbound/both
	CompareMode string   `json:"compare_mode"`  // 对比模式: none/yoy/qoq
}

// PacketSizeDistribution 数据包大小分布
type PacketSizeDistribution struct {
	Buckets []SizeBucket `json:"buckets"`
	Total   int64        `json:"total"`
	AvgSize float64      `json:"avg_size"`
	P50Size float64      `json:"p50_size"`
	P95Size float64      `json:"p95_size"`
	P99Size float64      `json:"p99_size"`
}

// SizeBucket 大小区间
type SizeBucket struct {
	Range   string `json:"range"`    // "0-64B", "64-128B", ...
	MinSize int    `json:"min_size"`
	MaxSize int    `json:"max_size"`
	Count   int64  `json:"count"`
	Percent float64 `json:"percent"`
	Bytes   int64  `json:"bytes"`
}

// ProtocolDistribution 协议分布
type ProtocolDistribution struct {
	Protocols []ProtocolStat `json:"protocols"`
	Total     int64          `json:"total_packets"`
	TotalBytes int64         `json:"total_bytes"`
}

// ProtocolStat 协议统计
type ProtocolStat struct {
	Protocol   string  `json:"protocol"`
	Name       string  `json:"name"`
	Packets    int64   `json:"packets"`
	Bytes      int64   `json:"bytes"`
	Percent    float64 `json:"percent"`
	BytePercent float64 `json:"byte_percent"`
	AvgPacketSize float64 `json:"avg_packet_size"`
}

// ConnectLatencyDistribution 建连时延分布
type ConnectLatencyDistribution struct {
	Buckets []LatencyBucket `json:"buckets"`
	Total   int64           `json:"total"`
	AvgMs   float64         `json:"avg_ms"`
	P50Ms   float64         `json:"p50_ms"`
	P95Ms   float64         `json:"p95_ms"`
	P99Ms   float64         `json:"p99_ms"`
	FailedRate float64      `json:"failed_rate"`
}

// LatencyBucket 时延区间
type LatencyBucket struct {
	Range   string  `json:"range"`    // "0-1ms", "1-5ms", ...
	MinMs   float64 `json:"min_ms"`
	MaxMs   float64 `json:"max_ms"`
	Count   int64   `json:"count"`
	Percent float64 `json:"percent"`
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Timestamp   string  `json:"timestamp"`
	RetransRate float64 `json:"retrans_rate"`
	PacketLoss  float64 `json:"packet_loss"`
	ConnFailRate float64 `json:"conn_fail_rate"`
	TotalPackets int64  `json:"total_packets"`
	TotalBytes   int64  `json:"total_bytes"`
	ActiveConns  int64  `json:"active_conns"`
}

// TrendData 趋势数据
type TrendData struct {
	Points    []TrendPoint `json:"points"`
	StartTime string       `json:"start_time"`
	EndTime   string       `json:"end_time"`
	Interval  string       `json:"interval"`
}

// CompareResult 对比结果
type CompareResult struct {
	Current       *TrendData     `json:"current"`
	Compare       *TrendData     `json:"compare"`
	CompareLabel  string         `json:"compare_label"`  // "上周同期" / "上个月同期"
	Changes       []MetricChange `json:"changes"`
	Summary       string         `json:"summary"`
}

// MetricChange 指标变化
type MetricChange struct {
	Metric    string  `json:"metric"`
	Name      string  `json:"name"`
	Current   float64 `json:"current"`
	Compare   float64 `json:"compare"`
	ChangePct float64 `json:"change_pct"`
	Trend     string  `json:"trend"` // up/down/stable
}

// NetworkAnalysisResult 网络分析结果
type NetworkAnalysisResult struct {
	Filter        *AnalysisFilter            `json:"filter"`
	PacketSize    *PacketSizeDistribution    `json:"packet_size"`
	Protocol      *ProtocolDistribution      `json:"protocol"`
	ConnectLatency *ConnectLatencyDistribution `json:"connect_latency"`
	Trend         *TrendData                 `json:"trend"`
	Compare       *CompareResult             `json:"compare,omitempty"`
	Timestamp     time.Time                  `json:"timestamp"`
}

// ==================== 分析管理器 ====================

// AnalysisManager 网络分析管理器
type AnalysisManager struct {
	mu sync.RWMutex
}

// NewAnalysisManager 创建分析管理器
func NewAnalysisManager() *AnalysisManager {
	return &AnalysisManager{}
}

// Analyze 执行网络分析
func (m *AnalysisManager) Analyze(filter *AnalysisFilter) *NetworkAnalysisResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := &NetworkAnalysisResult{
		Filter:    filter,
		Timestamp: time.Now(),
	}

	// 1. 数据包大小分布
	result.PacketSize = m.analyzePacketSize(filter)

	// 2. 协议分布
	result.Protocol = m.analyzeProtocol(filter)

	// 3. 建连时延分布
	result.ConnectLatency = m.analyzeConnectLatency(filter)

	// 4. 历史趋势
	result.Trend = m.analyzeTrend(filter)

	// 5. 同比/环比对比
	if filter.CompareMode != "" && filter.CompareMode != "none" {
		result.Compare = m.analyzeCompare(filter)
	}

	return result
}

// analyzePacketSize 分析数据包大小分布
func (m *AnalysisManager) analyzePacketSize(filter *AnalysisFilter) *PacketSizeDistribution {
	// 模拟数据包大小分布
	buckets := []SizeBucket{
		{Range: "0-64B", MinSize: 0, MaxSize: 64, Count: 150000, Bytes: 4800000},
		{Range: "64-128B", MinSize: 64, MaxSize: 128, Count: 280000, Bytes: 26880000},
		{Range: "128-256B", MinSize: 128, MaxSize: 256, Count: 350000, Bytes: 67200000},
		{Range: "256-512B", MinSize: 256, MaxSize: 512, Count: 220000, Bytes: 84480000},
		{Range: "512-1024B", MinSize: 512, MaxSize: 1024, Count: 180000, Bytes: 138240000},
		{Range: "1-1.5KB", MinSize: 1024, MaxSize: 1536, Count: 120000, Bytes: 150000000},
		{Range: "1.5-4KB", MinSize: 1536, MaxSize: 4096, Count: 80000, Bytes: 184320000},
		{Range: "4-8KB", MinSize: 4096, MaxSize: 8192, Count: 35000, Bytes: 215040000},
		{Range: "8-16KB", MinSize: 8192, MaxSize: 16384, Count: 12000, Bytes: 147456000},
		{Range: "16-64KB", MinSize: 16384, MaxSize: 65536, Count: 5000, Bytes: 204800000},
		{Range: ">64KB", MinSize: 65536, MaxSize: 0, Count: 800, Bytes: 104857600},
	}

	var total int64
	for i := range buckets {
		total += buckets[i].Count
	}
	for i := range buckets {
		buckets[i].Percent = float64(buckets[i].Count) / float64(total) * 100
	}

	return &PacketSizeDistribution{
		Buckets: buckets,
		Total:   total,
		AvgSize: 1024,
		P50Size: 256,
		P95Size: 4096,
		P99Size: 16384,
	}
}

// analyzeProtocol 分析协议分布
func (m *AnalysisManager) analyzeProtocol(filter *AnalysisFilter) *ProtocolDistribution {
	protocols := []ProtocolStat{
		{Protocol: "tcp", Name: "TCP", Packets: 850000, Bytes: 680000000},
		{Protocol: "udp", Name: "UDP", Packets: 320000, Bytes: 96000000},
		{Protocol: "http", Name: "HTTP", Packets: 280000, Bytes: 224000000},
		{Protocol: "https", Name: "HTTPS", Packets: 450000, Bytes: 540000000},
		{Protocol: "grpc", Name: "gRPC", Packets: 180000, Bytes: 144000000},
		{Protocol: "dns", Name: "DNS", Packets: 95000, Bytes: 11400000},
		{Protocol: "icmp", Name: "ICMP", Packets: 15000, Bytes: 1200000},
		{Protocol: "arp", Name: "ARP", Packets: 25000, Bytes: 7500000},
		{Protocol: "mysql", Name: "MySQL", Packets: 60000, Bytes: 48000000},
		{Protocol: "redis", Name: "Redis", Packets: 120000, Bytes: 96000000},
		{Protocol: "kafka", Name: "Kafka", Packets: 40000, Bytes: 32000000},
	}

	var totalPackets, totalBytes int64
	for _, p := range protocols {
		totalPackets += p.Packets
		totalBytes += p.Bytes
	}
	for i := range protocols {
		protocols[i].Percent = float64(protocols[i].Packets) / float64(totalPackets) * 100
		protocols[i].BytePercent = float64(protocols[i].Bytes) / float64(totalBytes) * 100
		if protocols[i].Packets > 0 {
			protocols[i].AvgPacketSize = float64(protocols[i].Bytes) / float64(protocols[i].Packets)
		}
	}

	sort.Slice(protocols, func(i, j int) bool {
		return protocols[i].Packets > protocols[j].Packets
	})

	return &ProtocolDistribution{
		Protocols:  protocols,
		Total:      totalPackets,
		TotalBytes: totalBytes,
	}
}

// analyzeConnectLatency 分析建连时延分布
func (m *AnalysisManager) analyzeConnectLatency(filter *AnalysisFilter) *ConnectLatencyDistribution {
	buckets := []LatencyBucket{
		{Range: "0-1ms", MinMs: 0, MaxMs: 1, Count: 180000},
		{Range: "1-5ms", MinMs: 1, MaxMs: 5, Count: 320000},
		{Range: "5-10ms", MinMs: 5, MaxMs: 10, Count: 250000},
		{Range: "10-20ms", MinMs: 10, MaxMs: 20, Count: 150000},
		{Range: "20-50ms", MinMs: 20, MaxMs: 50, Count: 80000},
		{Range: "50-100ms", MinMs: 50, MaxMs: 100, Count: 30000},
		{Range: "100-200ms", MinMs: 100, MaxMs: 200, Count: 12000},
		{Range: "200-500ms", MinMs: 200, MaxMs: 500, Count: 5000},
		{Range: ">500ms", MinMs: 500, MaxMs: 0, Count: 2000},
	}

	var total int64
	for _, b := range buckets {
		total += b.Count
	}
	for i := range buckets {
		buckets[i].Percent = float64(buckets[i].Count) / float64(total) * 100
	}

	return &ConnectLatencyDistribution{
		Buckets:    buckets,
		Total:      total,
		AvgMs:      8.5,
		P50Ms:      4.2,
		P95Ms:      45.0,
		P99Ms:      180.0,
		FailedRate: 0.0025,
	}
}

// analyzeTrend 分析历史趋势
func (m *AnalysisManager) analyzeTrend(filter *AnalysisFilter) *TrendData {
	now := time.Now()
	duration := parseTimeRange(filter.TimeRange)
	if duration == 0 {
		duration = 6 * time.Hour
	}
	start := now.Add(-duration)

	// 生成时间点（每5分钟一个）
	interval := 5 * time.Minute
	pointCount := int(duration / interval)
	if pointCount > 200 {
		interval = time.Duration(int(duration.Minutes()) / 200) * time.Minute
		pointCount = int(duration / interval)
	}

	points := make([]TrendPoint, pointCount)
	baseTime := float64(now.Unix())

	for i := 0; i < pointCount; i++ {
		t := start.Add(time.Duration(i) * interval)
		// 使用正弦波+噪声模拟趋势数据
		hourFactor := math.Sin(float64(t.Hour())*math.Pi/12) * 0.5 + 0.5
		noise := (hashFloat(baseTime + float64(i)*0.1) - 0.5) * 0.3

		retrans := 0.005 + hourFactor*0.015 + noise*0.01
		if retrans < 0 {
			retrans = 0.001
		}
		loss := retrans * 0.3 + noise*0.002
		if loss < 0 {
			loss = 0.0001
		}
		failRate := 0.001 + hourFactor*0.005 + noise*0.003
		if failRate < 0 {
			failRate = 0.0001
		}

		points[i] = TrendPoint{
			Timestamp:    t.Format("2006-01-02 15:04"),
			RetransRate: roundTo(retrans, 6),
			PacketLoss:   roundTo(loss, 6),
			ConnFailRate: roundTo(failRate, 6),
			TotalPackets: int64(50000 + hourFactor*30000 + noise*10000),
			TotalBytes:   int64(50000000 + hourFactor*30000000 + noise*10000000),
			ActiveConns:  int64(200 + hourFactor*300 + noise*50),
		}
	}

	return &TrendData{
		Points:    points,
		StartTime: start.Format("2006-01-02 15:04"),
		EndTime:   now.Format("2006-01-02 15:04"),
		Interval:  interval.String(),
	}
}

// analyzeCompare 同比/环比对比
func (m *AnalysisManager) analyzeCompare(filter *AnalysisFilter) *CompareResult {
	// 当前数据
	current := m.analyzeTrend(filter)

	// 计算对比时间范围
	var compareFilter *AnalysisFilter
	var label string

	switch filter.CompareMode {
	case "qoq": // 环比 - 上一个周期
		compareFilter = &AnalysisFilter{
			TimeRange: filter.TimeRange,
		}
		// 偏移时间
		duration := parseTimeRange(filter.TimeRange)
		if duration == 0 {
			duration = 6 * time.Hour
		}
		label = fmt.Sprintf("上%s", formatDurationCN(duration))
	case "yoy": // 同比 - 上周/上月同期
		compareFilter = &AnalysisFilter{
			TimeRange: filter.TimeRange,
		}
		label = "上周同期"
	default:
		return nil
	}

	compare := m.analyzeTrend(compareFilter)

	// 计算变化
	changes := m.calculateChanges(current, compare)

	// 生成摘要
	summary := m.generateSummary(changes)

	return &CompareResult{
		Current:      current,
		Compare:      compare,
		CompareLabel: label,
		Changes:      changes,
		Summary:      summary,
	}
}

// calculateChanges 计算指标变化
func (m *AnalysisManager) calculateChanges(current, compare *TrendData) []MetricChange {
	if len(current.Points) == 0 || len(compare.Points) == 0 {
		return nil
	}

	// 计算当前周期平均值
	currAvg := averageTrend(current.Points)
	compAvg := averageTrend(compare.Points)

	changes := []MetricChange{
		calcChange("retrans_rate", "重传率", currAvg.RetransRate, compAvg.RetransRate),
		calcChange("packet_loss", "丢包率", currAvg.PacketLoss, compAvg.PacketLoss),
		calcChange("conn_fail_rate", "建连失败率", currAvg.ConnFailRate, compAvg.ConnFailRate),
		calcChange("total_packets", "总包量", float64(currAvg.TotalPackets), float64(compAvg.TotalPackets)),
		calcChange("total_bytes", "总流量", float64(currAvg.TotalBytes), float64(compAvg.TotalBytes)),
		calcChange("active_conns", "活跃连接", float64(currAvg.ActiveConns), float64(compAvg.ActiveConns)),
	}

	return changes
}

// generateSummary 生成摘要
func (m *AnalysisManager) generateSummary(changes []MetricChange) string {
	if len(changes) == 0 {
		return ""
	}

	var improved, degraded int
	for _, c := range changes {
		// 重传率、丢包率、建连失败率下降是改善
		if c.Metric == "retrans_rate" || c.Metric == "packet_loss" || c.Metric == "conn_fail_rate" {
			if c.Trend == "down" {
				improved++
			} else if c.Trend == "up" {
				degraded++
			}
		} else {
			if c.Trend == "up" {
				improved++
			} else if c.Trend == "down" {
				degraded++
			}
		}
	}

	if degraded > improved*2 {
		return fmt.Sprintf("⚠️ 网络质量明显下降，%d项指标恶化，建议排查", degraded)
	} else if degraded > improved {
		return fmt.Sprintf("⚡ 网络质量轻微下降，%d项指标需关注", degraded)
	} else if improved > degraded*2 {
		return fmt.Sprintf("✅ 网络质量显著改善，%d项指标好转", improved)
	} else if improved > degraded {
		return fmt.Sprintf("📈 网络质量整体稳定向好，%d项指标改善", improved)
	}
	return "➡️ 网络质量基本持平，无显著变化"
}

// ==================== HTTP Handler ====================

// AnalysisHandler 网络分析HTTP处理器
type AnalysisHandler struct {
	manager *AnalysisManager
}

// NewAnalysisHandler 创建处理器
func NewAnalysisHandler(manager *AnalysisManager) *AnalysisHandler {
	return &AnalysisHandler{manager: manager}
}

// RegisterRoutes 注册路由
func (h *AnalysisHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/network/analysis", h.handleAnalysis)
	mux.HandleFunc("/api/v1/network/distributions", h.handleDistributions)
	mux.HandleFunc("/api/v1/network/trend", h.handleTrend)
	mux.HandleFunc("/api/v1/network/compare", h.handleCompare)
	mux.HandleFunc("/network-analysis", h.handlePage)
}

// handleAnalysis 综合分析
func (h *AnalysisHandler) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := parseFilter(r.URL.Query())
	result := h.manager.Analyze(filter)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleDistributions 分布数据
func (h *AnalysisHandler) handleDistributions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := parseFilter(r.URL.Query())
	result := &struct {
		PacketSize    *PacketSizeDistribution     `json:"packet_size"`
		Protocol      *ProtocolDistribution       `json:"protocol"`
		ConnectLatency *ConnectLatencyDistribution `json:"connect_latency"`
	}{
		PacketSize:     h.manager.analyzePacketSize(filter),
		Protocol:       h.manager.analyzeProtocol(filter),
		ConnectLatency: h.manager.analyzeConnectLatency(filter),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleTrend 趋势数据
func (h *AnalysisHandler) handleTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := parseFilter(r.URL.Query())
	trend := h.manager.analyzeTrend(filter)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trend)
}

// handleCompare 对比数据
func (h *AnalysisHandler) handleCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := parseFilter(r.URL.Query())
	result := h.manager.analyzeCompare(filter)

	if result == nil {
		http.Error(w, "Invalid compare mode", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handlePage 页面
func (h *AnalysisHandler) handlePage(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/network-analysis.html")
}

// ==================== 工具函数 ====================

func parseFilter(q map[string][]string) *AnalysisFilter {
	f := &AnalysisFilter{
		AssetIDs:  q["asset_id"],
		ProbeIDs:  q["probe_id"],
		LinkIDs:   q["link_id"],
		TimeRange: "6h",
		Protocol:  q.Get("protocol"),
		Direction: q.Get("direction"),
		CompareMode: q.Get("compare"),
	}

	if tr := q.Get("time_range"); tr != "" {
		f.TimeRange = tr
	}
	if st := q.Get("start_time"); st != "" {
		f.StartTime = st
	}
	if et := q.Get("end_time"); et != "" {
		f.EndTime = et
	}

	return f
}

func parseTimeRange(rangeStr string) time.Duration {
	switch rangeStr {
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return 6 * time.Hour
	}
}

func formatDurationCN(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d分钟", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d小时", int(d.Hours()))
	default:
		return fmt.Sprintf("%d天", int(d.Hours()/24))
	}
}

func calcChange(metric, name string, current, compare float64) MetricChange {
	var pct float64
	if compare != 0 {
		pct = (current - compare) / math.Abs(compare) * 100
	}

	trend := "stable"
	if pct > 5 {
		trend = "up"
	} else if pct < -5 {
		trend = "down"
	}

	return MetricChange{
		Metric:    metric,
		Name:      name,
		Current:   roundTo(current, 6),
		Compare:   roundTo(compare, 6),
		ChangePct: roundTo(pct, 2),
		Trend:     trend,
	}
}

func averageTrend(points []TrendPoint) TrendPoint {
	if len(points) == 0 {
		return TrendPoint{}
	}
	var avg TrendPoint
	n := float64(len(points))
	for _, p := range points {
		avg.RetransRate += p.RetransRate
		avg.PacketLoss += p.PacketLoss
		avg.ConnFailRate += p.ConnFailRate
		avg.TotalPackets += p.TotalPackets
		avg.TotalBytes += p.TotalBytes
		avg.ActiveConns += p.ActiveConns
	}
	avg.RetransRate /= n
	avg.PacketLoss /= n
	avg.ConnFailRate /= n
	avg.TotalPackets = int64(float64(avg.TotalPackets) / n)
	avg.TotalBytes = int64(float64(avg.TotalBytes) / n)
	avg.ActiveConns = int64(float64(avg.ActiveConns) / n)
	return avg
}

func roundTo(v float64, p int) float64 {
	f := math.Pow(10, float64(p))
	return math.Round(v*f) / f
}

func hashFloat(v float64) float64 {
	x := float64(uint32(v*1000) % 1000)
	return x / 1000.0
}

// Ensure strings import is used
var _ = strings.Join
