// Package anomaly 拓扑异常检测与可视化标记
//
// 功能：
//   - 节点异常检测：错误率、延迟、流量异常
//   - 边异常检测：流量异常、超时、断链
//   - 异常传播路径标记
//   - 异常节点/边可视化标记（颜色、闪烁、大小等）
//   - 异常根因分析
//
package anomaly

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	graph "github.com/meinanzilinzhengying/cloudflow/services/topology-engine/graph"
)

// ============================================================================
// 一、异常检测器
// ============================================================================

// AnomalyDetector 拓扑异常检测器
type AnomalyDetector struct {
	config AnomalyConfig
	mu     sync.RWMutex

	// 历史数据窗口（用于动态阈值）
	history     map[graph.NodeID]*MetricHistory
	edgeHistory map[graph.EdgeKey]*MetricHistory
}

// AnomalyConfig 异常检测配置
type AnomalyConfig struct {
	// 错误率阈值
	ErrorRateCritical float64 // 如 0.1 (10%)
	ErrorRateWarning  float64 // 如 0.01 (1%)

	// 延迟阈值 (ms)
	LatencyCriticalMs float64 // 如 1000ms
	LatencyWarningMs  float64 // 如 500ms

	// 流量异常阈值（相对变化率）
	TrafficDropCritical float64 // 如 0.8 (下降80%)
	TrafficDropWarning  float64 // 如 0.5 (下降50%)
	TrafficSpikeFactor  float64 // 如 5.0 (增长5倍)

	// 动态阈值（基于历史数据）
	EnableDynamicThreshold bool
	HistoryWindowSize      int // 历史窗口大小

	// 检测窗口
	DetectionWindow time.Duration
}

// DefaultAnomalyConfig 返回默认异常检测配置
func DefaultAnomalyConfig() AnomalyConfig {
	return AnomalyConfig{
		ErrorRateCritical:  0.10,
		ErrorRateWarning:   0.01,
		LatencyCriticalMs:  1000.0,
		LatencyWarningMs:   500.0,
		TrafficDropCritical: 0.80,
		TrafficDropWarning:  0.50,
		TrafficSpikeFactor:  5.0,
		EnableDynamicThreshold: true,
		HistoryWindowSize:      60,
		DetectionWindow:        5 * time.Minute,
	}
}

// NewAnomalyDetector 创建异常检测器
func NewAnomalyDetector(config AnomalyConfig) *AnomalyDetector {
	return &AnomalyDetector{
		config:      config,
		history:     make(map[graph.NodeID]*MetricHistory),
		edgeHistory: make(map[graph.EdgeKey]*MetricHistory),
	}
}

// ============================================================================
// 二、异常类型定义
// ============================================================================

// AnomalySeverity 异常严重级别
type AnomalySeverity string

const (
	SeverityCritical AnomalySeverity = "critical" // 红色
	SeverityWarning  AnomalySeverity = "warning"  // 橙色
	SeverityInfo     AnomalySeverity = "info"     // 黄色
)

// AnomalyType 异常类型
type AnomalyType string

const (
	TypeErrorRate     AnomalyType = "error_rate"      // 错误率异常
	TypeLatency       AnomalyType = "latency"         // 延迟异常
	TypeTrafficDrop   AnomalyType = "traffic_drop"    // 流量下降
	TypeTrafficSpike  AnomalyType = "traffic_spike"   // 流量突增
	TypeNodeOffline   AnomalyType = "node_offline"    // 节点离线
	TypeEdgeTimeout   AnomalyType = "edge_timeout"    // 边超时
	TypeEdgeBroken    AnomalyType = "edge_broken"     // 边断链
	TypeCircuitBreak  AnomalyType = "circuit_break"   // 熔断
)

// Anomaly 异常记录
type Anomaly struct {
	ID          string
	Type        AnomalyType
	Severity    AnomalySeverity
	NodeID      graph.NodeID  // 节点异常时
	EdgeKey     graph.EdgeKey // 边异常时
	Message     string
	Value       float64       // 当前值
	Threshold   float64       // 阈值
	DetectedAt  time.Time
	Duration    time.Duration // 持续时间

	// 可视化标记
	VisualMarker *VisualMarker
}

// VisualMarker 可视化标记
type VisualMarker struct {
	// 颜色 (hex)
	Color string
	// 边框颜色
	BorderColor string
	// 大小倍率 (1.0 = 正常)
	SizeScale float64
	// 是否闪烁
	Blinking bool
	// 闪烁频率 (Hz)
	BlinkRate float64
	// 标签文本
	Label string
	// 图标
	Icon string
	// 透明度 (0.0 - 1.0)
	Opacity float64
}

// ============================================================================
// 三、历史数据
// ============================================================================

// MetricHistory 指标历史数据
type MetricHistory struct {
	NodeID    graph.NodeID
	EdgeKey   graph.EdgeKey

	ErrorRates    []float64
	Latencies     []float64
	TrafficBytes  []uint64
	RequestCounts []uint64

	Timestamps []int64
	MaxSize    int
}

// AddRecord 添加历史记录
func (mh *MetricHistory) AddRecord(errorRate, latencyMs float64, bytes, requests uint64, ts int64) {
	mh.ErrorRates = append(mh.ErrorRates, errorRate)
	mh.Latencies = append(mh.Latencies, latencyMs)
	mh.TrafficBytes = append(mh.TrafficBytes, bytes)
	mh.RequestCounts = append(mh.RequestCounts, requests)
	mh.Timestamps = append(mh.Timestamps, ts)

	if mh.MaxSize > 0 && len(mh.ErrorRates) > mh.MaxSize {
		mh.ErrorRates = mh.ErrorRates[len(mh.ErrorRates)-mh.MaxSize:]
		mh.Latencies = mh.Latencies[len(mh.Latencies)-mh.MaxSize:]
		mh.TrafficBytes = mh.TrafficBytes[len(mh.TrafficBytes)-mh.MaxSize:]
		mh.RequestCounts = mh.RequestCounts[len(mh.RequestCounts)-mh.MaxSize:]
		mh.Timestamps = mh.Timestamps[len(mh.Timestamps)-mh.MaxSize:]
	}
}

// AvgErrorRate 平均错误率
func (mh *MetricHistory) AvgErrorRate() float64 {
	if len(mh.ErrorRates) == 0 {
		return 0
	}
	var sum float64
	for _, v := range mh.ErrorRates {
		sum += v
	}
	return sum / float64(len(mh.ErrorRates))
}

// AvgLatency 平均延迟
func (mh *MetricHistory) AvgLatency() float64 {
	if len(mh.Latencies) == 0 {
		return 0
	}
	var sum float64
	for _, v := range mh.Latencies {
		sum += v
	}
	return sum / float64(len(mh.Latencies))
}

// AvgTraffic 平均流量
func (mh *MetricHistory) AvgTraffic() uint64 {
	if len(mh.TrafficBytes) == 0 {
		return 0
	}
	var sum uint64
	for _, v := range mh.TrafficBytes {
		sum += v
	}
	return sum / uint64(len(mh.TrafficBytes))
}

// StdDevLatency 延迟标准差
func (mh *MetricHistory) StdDevLatency() float64 {
	if len(mh.Latencies) < 2 {
		return 0
	}
	avg := mh.AvgLatency()
	var sumSq float64
	for _, v := range mh.Latencies {
		diff := v - avg
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(mh.Latencies)))
}

// ============================================================================
// 四、节点异常检测
// ============================================================================

// DetectNodeAnomalies 检测所有节点异常
func (ad *AnomalyDetector) DetectNodeAnomalies(g *graph.Graph) []*Anomaly {
	var anomalies []*Anomaly

	nodes := g.Nodes()
	now := time.Now()

	for _, n := range nodes {
		if !n.Active {
			continue
		}

		latencyMs := float64(n.AvgLatencyNs) / 1e6
		errRate := n.ErrorRate()

		// 更新历史
		ad.updateNodeHistory(n.ID, errRate, latencyMs, n.BytesIn+n.BytesOut, n.RequestCount, now.Unix())

		// 检测错误率异常
		if errRate > ad.config.ErrorRateCritical {
			anomalies = append(anomalies, ad.createNodeAnomaly(
				n.ID, TypeErrorRate, SeverityCritical,
				errRate, ad.config.ErrorRateCritical,
				fmt.Sprintf("节点 %s 错误率 %.1f%%，超过临界阈值 %.1f%%", n.Name, errRate*100, ad.config.ErrorRateCritical*100),
			))
		} else if errRate > ad.config.ErrorRateWarning {
			anomalies = append(anomalies, ad.createNodeAnomaly(
				n.ID, TypeErrorRate, SeverityWarning,
				errRate, ad.config.ErrorRateWarning,
				fmt.Sprintf("节点 %s 错误率 %.1f%%，超过警告阈值 %.1f%%", n.Name, errRate*100, ad.config.ErrorRateWarning*100),
			))
		}

		// 检测延迟异常
		if latencyMs > ad.config.LatencyCriticalMs {
			anomalies = append(anomalies, ad.createNodeAnomaly(
				n.ID, TypeLatency, SeverityCritical,
				latencyMs, ad.config.LatencyCriticalMs,
				fmt.Sprintf("节点 %s 延迟 %.1fms，超过临界阈值 %.1fms", n.Name, latencyMs, ad.config.LatencyCriticalMs),
			))
		} else if latencyMs > ad.config.LatencyWarningMs {
			anomalies = append(anomalies, ad.createNodeAnomaly(
				n.ID, TypeLatency, SeverityWarning,
				latencyMs, ad.config.LatencyWarningMs,
				fmt.Sprintf("节点 %s 延迟 %.1fms，超过警告阈值 %.1fms", n.Name, latencyMs, ad.config.LatencyWarningMs),
			))
		}

		// 检测流量异常（动态阈值）
		if ad.config.EnableDynamicThreshold {
			if history, ok := ad.history[n.ID]; ok && len(history.TrafficBytes) > 5 {
				avgTraffic := history.AvgTraffic()
				currentTraffic := n.BytesIn + n.BytesOut
				if avgTraffic > 0 {
					dropRate := 1.0 - float64(currentTraffic)/float64(avgTraffic)
					if dropRate > ad.config.TrafficDropCritical {
						anomalies = append(anomalies, ad.createNodeAnomaly(
							n.ID, TypeTrafficDrop, SeverityCritical,
							dropRate, ad.config.TrafficDropCritical,
							fmt.Sprintf("节点 %s 流量下降 %.1f%%", n.Name, dropRate*100),
						))
					} else if dropRate > ad.config.TrafficDropWarning {
						anomalies = append(anomalies, ad.createNodeAnomaly(
							n.ID, TypeTrafficDrop, SeverityWarning,
							dropRate, ad.config.TrafficDropWarning,
							fmt.Sprintf("节点 %s 流量下降 %.1f%%", n.Name, dropRate*100),
						))
					}

					spikeFactor := float64(currentTraffic) / float64(avgTraffic)
					if spikeFactor > ad.config.TrafficSpikeFactor {
						anomalies = append(anomalies, ad.createNodeAnomaly(
							n.ID, TypeTrafficSpike, SeverityWarning,
							spikeFactor, ad.config.TrafficSpikeFactor,
							fmt.Sprintf("节点 %s 流量突增 %.1f 倍", n.Name, spikeFactor),
						))
					}
				}
			}
		}
	}

	return anomalies
}

// ============================================================================
// 五、边异常检测
// ============================================================================

// DetectEdgeAnomalies 检测所有边异常
func (ad *AnomalyDetector) DetectEdgeAnomalies(g *graph.Graph) []*Anomaly {
	var anomalies []*Anomaly

	edges := g.Edges()
	now := time.Now()

	for _, e := range edges {
		if !e.Active {
			continue
		}

		latencyMs := float64(e.Latency) / 1e6
		errRate := e.ErrorRate()

		// 更新历史
		ad.updateEdgeHistory(e.Key(), errRate, latencyMs, e.Bytes, e.RequestCount, now.Unix())

		// 检测错误率异常
		if errRate > ad.config.ErrorRateCritical {
			anomalies = append(anomalies, ad.createEdgeAnomaly(
				e.Key(), TypeErrorRate, SeverityCritical,
				errRate, ad.config.ErrorRateCritical,
				fmt.Sprintf("边 %s→%s 错误率 %.1f%%", e.Source, e.Target, errRate*100),
			))
		} else if errRate > ad.config.ErrorRateWarning {
			anomalies = append(anomalies, ad.createEdgeAnomaly(
				e.Key(), TypeErrorRate, SeverityWarning,
				errRate, ad.config.ErrorRateWarning,
				fmt.Sprintf("边 %s→%s 错误率 %.1f%%", e.Source, e.Target, errRate*100),
			))
		}

		// 检测延迟异常
		if latencyMs > ad.config.LatencyCriticalMs {
			anomalies = append(anomalies, ad.createEdgeAnomaly(
				e.Key(), TypeLatency, SeverityCritical,
				latencyMs, ad.config.LatencyCriticalMs,
				fmt.Sprintf("边 %s→%s 延迟 %.1fms", e.Source, e.Target, latencyMs),
			))
		} else if latencyMs > ad.config.LatencyWarningMs {
			anomalies = append(anomalies, ad.createEdgeAnomaly(
				e.Key(), TypeLatency, SeverityWarning,
				latencyMs, ad.config.LatencyWarningMs,
				fmt.Sprintf("边 %s→%s 延迟 %.1fms", e.Source, e.Target, latencyMs),
			))
		}
	}

	return anomalies
}

// ============================================================================
// 六、异常创建
// ============================================================================

func (ad *AnomalyDetector) createNodeAnomaly(
	nodeID graph.NodeID, anomalyType AnomalyType, severity AnomalySeverity,
	value, threshold float64, message string,
) *Anomaly {
	anomaly := &Anomaly{
		ID:         fmt.Sprintf("anomaly-%s-%s-%d", nodeID, anomalyType, time.Now().Unix()),
		Type:       anomalyType,
		Severity:   severity,
		NodeID:     nodeID,
		Message:    message,
		Value:      value,
		Threshold:  threshold,
		DetectedAt: time.Now(),
		VisualMarker: ad.createVisualMarker(severity, anomalyType),
	}
	return anomaly
}

func (ad *AnomalyDetector) createEdgeAnomaly(
	edgeKey graph.EdgeKey, anomalyType AnomalyType, severity AnomalySeverity,
	value, threshold float64, message string,
) *Anomaly {
	anomaly := &Anomaly{
		ID:         fmt.Sprintf("anomaly-%s-%s-%d", edgeKey.String(), anomalyType, time.Now().Unix()),
		Type:       anomalyType,
		Severity:   severity,
		EdgeKey:    edgeKey,
		Message:    message,
		Value:      value,
		Threshold:  threshold,
		DetectedAt: time.Now(),
		VisualMarker: ad.createVisualMarker(severity, anomalyType),
	}
	return anomaly
}

// ============================================================================
// 七、可视化标记
// ============================================================================

// CreateVisualMarker 创建可视化标记（公开方法）
func (ad *AnomalyDetector) CreateVisualMarker(severity AnomalySeverity, anomalyType AnomalyType) *VisualMarker {
	return ad.createVisualMarker(severity, anomalyType)
}

func (ad *AnomalyDetector) createVisualMarker(severity AnomalySeverity, anomalyType AnomalyType) *VisualMarker {
	marker := &VisualMarker{
		Opacity: 1.0,
	}

	// 根据严重级别设置颜色
	switch severity {
	case SeverityCritical:
		marker.Color = "#FF4444"
		marker.BorderColor = "#FF0000"
		marker.SizeScale = 1.5
		marker.Blinking = true
		marker.BlinkRate = 2.0
	case SeverityWarning:
		marker.Color = "#FF8800"
		marker.BorderColor = "#FF6600"
		marker.SizeScale = 1.2
		marker.Blinking = true
		marker.BlinkRate = 1.0
	case SeverityInfo:
		marker.Color = "#FFCC00"
		marker.BorderColor = "#FFAA00"
		marker.SizeScale = 1.1
		marker.Blinking = false
	}

	// 根据异常类型设置图标
	switch anomalyType {
	case TypeErrorRate:
		marker.Icon = "⚠️"
		marker.Label = "错误"
	case TypeLatency:
		marker.Icon = "⏱️"
		marker.Label = "延迟"
	case TypeTrafficDrop:
		marker.Icon = "📉"
		marker.Label = "流量下降"
	case TypeTrafficSpike:
		marker.Icon = "📈"
		marker.Label = "流量突增"
	case TypeNodeOffline:
		marker.Icon = "❌"
		marker.Label = "离线"
	case TypeEdgeTimeout:
		marker.Icon = "⌛"
		marker.Label = "超时"
	case TypeEdgeBroken:
		marker.Icon = "💔"
		marker.Label = "断链"
	case TypeCircuitBreak:
		marker.Icon = "🔒"
		marker.Label = "熔断"
	}

	return marker
}

// ============================================================================
// 八、历史更新
// ============================================================================

func (ad *AnomalyDetector) updateNodeHistory(
	nodeID graph.NodeID, errorRate, latencyMs float64, bytes, requests uint64, ts int64,
) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	history, exists := ad.history[nodeID]
	if !exists {
		history = &MetricHistory{
			NodeID:  nodeID,
			MaxSize: ad.config.HistoryWindowSize,
		}
		ad.history[nodeID] = history
	}
	history.AddRecord(errorRate, latencyMs, bytes, requests, ts)
}

func (ad *AnomalyDetector) updateEdgeHistory(
	edgeKey graph.EdgeKey, errorRate, latencyMs float64, bytes, requests uint64, ts int64,
) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	history, exists := ad.edgeHistory[edgeKey]
	if !exists {
		history = &MetricHistory{
			EdgeKey: edgeKey,
			MaxSize: ad.config.HistoryWindowSize,
		}
		ad.edgeHistory[edgeKey] = history
	}
	history.AddRecord(errorRate, latencyMs, bytes, requests, ts)
}

// ============================================================================
// 九、异常聚合报告
// ============================================================================

// AnomalyReport 异常聚合报告
type AnomalyReport struct {
	DetectedAt      time.Time
	TotalAnomalies  int
	CriticalCount   int
	WarningCount    int
	InfoCount       int
	NodeAnomalies   []*Anomaly
	EdgeAnomalies   []*Anomaly
	TopIssues       []*Anomaly
	PropagationPath *AnomalyPropagationPath
}

// GenerateReport 生成异常报告
func (ad *AnomalyDetector) GenerateReport(g *graph.Graph) *AnomalyReport {
	nodeAnomalies := ad.DetectNodeAnomalies(g)
	edgeAnomalies := ad.DetectEdgeAnomalies(g)

	allAnomalies := append(nodeAnomalies, edgeAnomalies...)

	report := &AnomalyReport{
		DetectedAt:     time.Now(),
		TotalAnomalies: len(allAnomalies),
		NodeAnomalies:  nodeAnomalies,
		EdgeAnomalies:  edgeAnomalies,
	}

	for _, a := range allAnomalies {
		switch a.Severity {
		case SeverityCritical:
			report.CriticalCount++
		case SeverityWarning:
			report.WarningCount++
		case SeverityInfo:
			report.InfoCount++
		}
	}

	// 提取 top 问题
	sort.Slice(allAnomalies, func(i, j int) bool {
		// 按严重级别排序
		severityOrder := map[AnomalySeverity]int{
			SeverityCritical: 3,
			SeverityWarning:  2,
			SeverityInfo:     1,
		}
		if severityOrder[allAnomalies[i].Severity] != severityOrder[allAnomalies[j].Severity] {
			return severityOrder[allAnomalies[i].Severity] > severityOrder[allAnomalies[j].Severity]
		}
		return allAnomalies[i].Value > allAnomalies[j].Value
	})

	if len(allAnomalies) > 10 {
		report.TopIssues = allAnomalies[:10]
	} else {
		report.TopIssues = allAnomalies
	}

	// 分析异常传播路径
	report.PropagationPath = ad.buildPropagationPath(g, allAnomalies)

	return report
}

// ============================================================================
// 十、异常传播路径分析
// ============================================================================

// AnomalyPropagationPath 异常传播路径
type AnomalyPropagationPath struct {
	RootNodes []*AnomalyPathNode
}

// AnomalyPathNode 传播路径节点
type AnomalyPathNode struct {
	Anomaly      *Anomaly
	Depth        int
	Children     []*AnomalyPathNode
	IsRootCause  bool
	Confidence   float64
}

// buildPropagationPath 构建异常传播路径
func (ad *AnomalyDetector) buildPropagationPath(g *graph.Graph, anomalies []*Anomaly) *AnomalyPropagationPath {
	path := &AnomalyPropagationPath{}

	// 找出根因节点（异常的上游节点，没有异常的上游调用者）
	anomalyNodeSet := make(map[graph.NodeID]bool)
	for _, a := range anomalies {
		if a.NodeID != "" {
			anomalyNodeSet[a.NodeID] = true
		}
	}

	for _, a := range anomalies {
		if a.NodeID == "" {
			continue
		}

		// 检查此节点是否有异常的上游
		hasAnomalousUpstream := false
		edges := g.Edges()
		for _, e := range edges {
			if e.Target == a.NodeID && e.Active && anomalyNodeSet[e.Source] {
				hasAnomalousUpstream = true
				break
			}
		}

		if !hasAnomalousUpstream {
			// 可能是根因
			root := &AnomalyPathNode{
				Anomaly:     a,
				Depth:       0,
				IsRootCause: true,
				Confidence:  0.8,
			}
			path.RootNodes = append(path.RootNodes, root)
		}
	}

	return path
}

// ============================================================================
// 十一、拓扑标记器
// ============================================================================

// TopologyMarker 拓扑标记器（将异常标记应用到图上）
type TopologyMarker struct{}

// NewTopologyMarker 创建拓扑标记器
func NewTopologyMarker() *TopologyMarker {
	return &TopologyMarker{}
}

// MarkedTopology 带标记的拓扑图
type MarkedTopology struct {
	Nodes       []*MarkedNode
	Edges       []*MarkedEdge
	AnomalyCount int
}

// MarkedNode 带标记的节点
type MarkedNode struct {
	*graph.Node
	Marker    *VisualMarker
	Anomalies []*Anomaly
}

// MarkedEdge 带标记的边
type MarkedEdge struct {
	*graph.Edge
	Marker    *VisualMarker
	Anomalies []*Anomaly
}

// ApplyMarks 将异常标记应用到拓扑图
func (tm *TopologyMarker) ApplyMarks(g *graph.Graph, anomalies []*Anomaly) *MarkedTopology {
	marked := &MarkedTopology{}

	// 按节点/边分组异常
	nodeAnomalies := make(map[graph.NodeID][]*Anomaly)
	edgeAnomalies := make(map[graph.EdgeKey][]*Anomaly)

	for _, a := range anomalies {
		if a.NodeID != "" {
			nodeAnomalies[a.NodeID] = append(nodeAnomalies[a.NodeID], a)
		} else if a.EdgeKey.Source != "" && a.EdgeKey.Target != "" {
			edgeAnomalies[a.EdgeKey] = append(edgeAnomalies[a.EdgeKey], a)
		}
	}

	// 标记节点
	nodes := g.Nodes()
	for _, n := range nodes {
		mn := &MarkedNode{Node: n}
		if anoms, ok := nodeAnomalies[n.ID]; ok {
			mn.Anomalies = anoms
			mn.Marker = mergeMarkers(anoms)
			marked.AnomalyCount += len(anoms)
		}
		marked.Nodes = append(marked.Nodes, mn)
	}

	// 标记边
	edges := g.Edges()
	for _, e := range edges {
		me := &MarkedEdge{Edge: e}
		key := e.Key()
		if anoms, ok := edgeAnomalies[key]; ok {
			me.Anomalies = anoms
			me.Marker = mergeMarkers(anoms)
			marked.AnomalyCount += len(anoms)
		}
		marked.Edges = append(marked.Edges, me)
	}

	return marked
}

// mergeMarkers 合并多个异常的标记（取最高级别）
func mergeMarkers(anomalies []*Anomaly) *VisualMarker {
	if len(anomalies) == 0 {
		return nil
	}

	// 找到最高严重级别的异常
	var highest *Anomaly
	severityOrder := map[AnomalySeverity]int{
		SeverityCritical: 3,
		SeverityWarning:  2,
		SeverityInfo:     1,
	}

	for _, a := range anomalies {
		if highest == nil || severityOrder[a.Severity] > severityOrder[highest.Severity] {
			highest = a
		}
	}

	if highest != nil && highest.VisualMarker != nil {
		marker := *highest.VisualMarker
		// 添加异常数量标签
		if len(anomalies) > 1 {
			marker.Label = fmt.Sprintf("%s (%d)", marker.Label, len(anomalies))
		}
		return &marker
	}
	return nil
}

// ============================================================================
// 十二、根因分析
// ============================================================================

// RootCauseAnalyzer 根因分析器
type RootCauseAnalyzer struct{}

// NewRootCauseAnalyzer 创建根因分析器
func NewRootCauseAnalyzer() *RootCauseAnalyzer {
	return &RootCauseAnalyzer{}
}

// RootCauseResult 根因分析结果
type RootCauseResult struct {
	RootCauses   []*Anomaly
	Confidence   float64
	AnalysisTime time.Time
	Suggestions  []string
}

// AnalyzeRootCause 分析异常的根因
func (rca *RootCauseAnalyzer) AnalyzeRootCause(g *graph.Graph, anomalies []*Anomaly) *RootCauseResult {
	result := &RootCauseResult{
		AnalysisTime: time.Now(),
		Confidence:  0.0,
	}

	if len(anomalies) == 0 {
		result.Suggestions = append(result.Suggestions, "未发现异常")
		return result
	}

	// 按节点 ID 分组
	nodeAnomalies := make(map[graph.NodeID][]*Anomaly)
	for _, a := range anomalies {
		if a.NodeID != "" {
			nodeAnomalies[a.NodeID] = append(nodeAnomalies[a.NodeID], a)
		}
	}

	// 找出根因节点（没有异常上游的节点）
	anomalyNodeSet := make(map[graph.NodeID]bool)
	for id := range nodeAnomalies {
		anomalyNodeSet[id] = true
	}

	for nodeID, anoms := range nodeAnomalies {
		// 检查此节点是否有异常的上游
		hasAnomalousUpstream := false
		edges := g.Edges()
		for _, e := range edges {
			if e.Target == nodeID && e.Active && anomalyNodeSet[e.Source] {
				hasAnomalousUpstream = true
				break
			}
		}

		if !hasAnomalousUpstream && len(anoms) > 0 {
			// 找到根因
			result.RootCauses = append(result.RootCauses, anoms[0])
			result.Confidence += 0.2
		}
	}

	if len(result.RootCauses) == 0 {
		// 如果找不到根因，可能是外部原因
		result.Suggestions = append(result.Suggestions, "可能是外部依赖或基础设施问题")
		result.Confidence = 0.3
	} else {
		result.Confidence = math.Min(1.0, result.Confidence)
		result.Suggestions = append(result.Suggestions,
			fmt.Sprintf("发现 %d 个可能的根因节点", len(result.RootCauses)),
			"建议优先检查根因节点的资源使用情况",
			"检查根因节点的上游依赖是否正常",
		)
	}

	return result
}

// ============================================================================
// 十三、辅助函数
// ============================================================================

// String 返回异常的字符串表示
func (a *Anomaly) String() string {
	if a.NodeID != "" {
		return fmt.Sprintf("Anomaly{%s node=%s severity=%s value=%.2f}", a.Type, a.NodeID, a.Severity, a.Value)
	}
	return fmt.Sprintf("Anomaly{%s edge=%s severity=%s value=%.2f}", a.Type, a.EdgeKey.String(), a.Severity, a.Value)
}

// SeverityOrder 返回严重级别的排序值
func SeverityOrder(s AnomalySeverity) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// IsNodeAnomaly 判断是否为节点异常
func (a *Anomaly) IsNodeAnomaly() bool {
	return a.NodeID != ""
}

// IsEdgeAnomaly 判断是否为边异常
func (a *Anomaly) IsEdgeAnomaly() bool {
	return a.EdgeKey.Source != "" && a.EdgeKey.Target != ""
}

// GetMarkerColor 获取标记颜色（用于前端渲染）
func (a *Anomaly) GetMarkerColor() string {
	if a.VisualMarker != nil {
		return a.VisualMarker.Color
	}
	return "#FF0000"
}
