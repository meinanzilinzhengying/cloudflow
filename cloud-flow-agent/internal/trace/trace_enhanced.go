// Package trace 链路追踪功能增强
//
// P4 增强内容：
//   - W3C 标准 Trace ID 生成和注入
//   - eBPF 网络流量与 Trace 关联
//   - 跨服务调用链完整还原
//   - 慢调用/错误调用自动标记
//
package trace

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 一、W3C 标准 Trace ID 生成器
// ============================================================================

// W3CTraceIDGenerator W3C trace-context 标准 Trace ID 生成器
//
// 格式规范：
//   - Trace ID: 16 bytes (32 hex chars), version 00
//   - Span ID: 8 bytes (16 hex chars)
//   - traceparent: 00-{trace-id}-{span-id}-{flags}
type W3CTraceIDGenerator struct {
	mu sync.Mutex
}

// NewW3CTraceIDGenerator 创建 W3C 标准 Trace ID 生成器
func NewW3CTraceIDGenerator() *W3CTraceIDGenerator {
	return &W3CTraceIDGenerator{}
}

// GenerateTraceID 生成符合 W3C 标准的 Trace ID (32 hex chars)
func (g *W3CTraceIDGenerator) GenerateTraceID() TraceID {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback: 使用时间和随机组合
		now := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	return TraceID(formatHex(b))
}

// GenerateSpanID 生成符合 W3C 标准的 Span ID (16 hex chars)
func (g *W3CTraceIDGenerator) GenerateSpanID() SpanID {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		now := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(now >> (uint(i) * 8))
		}
	}
	return SpanID(formatHex(b))
}

// GenerateTraceParent 生成 W3C traceparent 头值
// 格式: 00-{trace-id}-{span-id}-{flags}
// flags: 00=未采样, 01=已采样
func (g *W3CTraceIDGenerator) GenerateTraceParent(sampled bool) string {
	traceID := g.GenerateTraceID()
	spanID := g.GenerateSpanID()
	flags := "00"
	if sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", traceID, spanID, flags)
}

// ParseTraceParent 解析 W3C traceparent 头
func ParseTraceParent(traceparent string) (*SpanContext, error) {
	if len(traceparent) != 55 {
		return nil, fmt.Errorf("invalid traceparent length: %d", len(traceparent))
	}
	// 格式: 00-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx-xxxxxxxxxxxxxxxx-01
	version := traceparent[:2]
	if version != "00" && version != "01" {
		return nil, fmt.Errorf("unsupported traceparent version: %s", version)
	}
	if traceparent[2] != '-' || traceparent[35] != '-' || traceparent[52] != '-' {
		return nil, fmt.Errorf("invalid traceparent format")
	}

	traceID := TraceID(traceparent[3:35])
	spanID := SpanID(traceparent[36:52])
	flags := traceparent[53:55]

	sampled := flags == "01"

	return &SpanContext{
		TraceID:  traceID,
		SpanID:   spanID,
		Sampled:  sampled,
		Baggage:  make(map[string]string),
	}, nil
}

// ============================================================================
// 二、eBPF 网络流量与 Trace 关联
// ============================================================================

// EBPFTraceCorrelator eBPF 流量与 Trace 关联器
//
// 职责：
//   - 从 eBPF 采集的 HTTP 请求中提取 Trace ID
//   - 将网络流量与调用链关联
//   - 建立 socket 连接 ↔ Span 的映射
type EBPFTraceCorrelator struct {
	mu sync.RWMutex

	// 连接标识 → Span 上下文映射
	connSpanMap map[ConnKey]*SpanContext

	// Trace ID → 关联的流量记录
	traceFlows map[TraceID][]*FlowRecord

	// 配置
	config *EBPFTraceConfig
}

// ConnKey 连接标识 (四元组)
type ConnKey struct {
	SrcIP   string
	DstIP   string
	SrcPort uint16
	DstPort uint16
}

// String 返回连接标识字符串
func (k ConnKey) String() string {
	return fmt.Sprintf("%s:%d→%s:%d", k.SrcIP, k.SrcPort, k.DstIP, k.DstPort)
}

// FlowRecord 流量记录 (与 Trace 关联)
type FlowRecord struct {
	ConnKey
	Timestamp  time.Time
	Bytes      uint64
	Packets    uint64
	LatencyNs  uint64
	StatusCode int

	// Trace 关联信息
	TraceID  TraceID
	SpanID   SpanID
	ParentID SpanID

	// HTTP 信息
	Method string
	Path   string
	Host   string
}

// EBPFTraceConfig eBPF Trace 关联配置
type EBPFTraceConfig struct {
	// 从 HTTP 头提取 Trace ID 的优先级列表
	TraceIDHeaders []string
	// 连接映射过期时间
	ConnMapTTL time.Duration
	// 最大映射条目数
	MaxConnMapSize int
}

// DefaultEBPFTraceConfig 返回默认配置
func DefaultEBPFTraceConfig() *EBPFTraceConfig {
	return &EBPFTraceConfig{
		TraceIDHeaders: []string{
			"traceparent",         // W3C 标准
			"x-trace-id",          // 自定义
			"x-b3-traceid",        // B3
			"uber-trace-id",       // Jaeger
			"sw8",                 // SkyWalking
		},
		ConnMapTTL:     5 * time.Minute,
		MaxConnMapSize: 100000,
	}
}

// NewEBPFTraceCorrelator 创建 eBPF Trace 关联器
func NewEBPFTraceCorrelator(config *EBPFTraceConfig) *EBPFTraceCorrelator {
	if config == nil {
		config = DefaultEBPFTraceConfig()
	}
	return &EBPFTraceCorrelator{
		connSpanMap: make(map[ConnKey]*SpanContext),
		traceFlows:  make(map[TraceID][]*FlowRecord),
		config:      config,
	}
}

// ExtractTraceFromHTTP 从 HTTP 请求头中提取 Trace 信息
//
// 参数:
//   - headers: HTTP 请求头 (map[string][]string 格式)
//   - connKey: 连接标识
//
// 返回提取到的 SpanContext，如果未找到则返回 nil
func (etc *EBPFTraceCorrelator) ExtractTraceFromHTTP(
	headers map[string][]string,
	connKey ConnKey,
) *SpanContext {
	var sc *SpanContext

	for _, headerName := range etc.config.TraceIDHeaders {
		values, exists := headers[headerName]
		if !exists || len(values) == 0 {
			continue
		}
		value := values[0]

		switch headerName {
		case "traceparent":
			parsed, err := ParseTraceParent(value)
			if err == nil {
				sc = parsed
				break
			}
		case "x-trace-id":
			sc = etc.parseCustomTraceID(value, headers)
			break
		case "x-b3-traceid":
			sc = etc.parseB3TraceID(value, headers)
			break
		case "uber-trace-id":
			sc = etc.parseJaegerTraceID(value)
			break
		case "sw8":
			sc = etc.parseSkyWalkingTraceID(value)
			break
		}

		if sc != nil {
			break
		}
	}

	if sc != nil {
		etc.mu.Lock()
		etc.connSpanMap[connKey] = sc
		etc.mu.Unlock()
	}

	return sc
}

// AssociateFlowWithTrace 将流量记录与 Trace 关联
func (etc *EBPFTraceCorrelator) AssociateFlowWithTrace(flow *FlowRecord) {
	if flow.TraceID == "" {
		return
	}

	etc.mu.Lock()
	defer etc.mu.Unlock()

	etc.traceFlows[flow.TraceID] = append(etc.traceFlows[flow.TraceID], flow)

	// 限制大小
	if len(etc.traceFlows[flow.TraceID]) > 1000 {
		etc.traceFlows[flow.TraceID] = etc.traceFlows[flow.TraceID][len(etc.traceFlows[flow.TraceID])-1000:]
	}
}

// GetFlowsByTraceID 获取 Trace ID 关联的所有流量记录
func (etc *EBPFTraceCorrelator) GetFlowsByTraceID(traceID TraceID) []*FlowRecord {
	etc.mu.RLock()
	defer etc.mu.RUnlock()
	return etc.traceFlows[traceID]
}

// GetSpanContextByConn 根据连接获取 Span 上下文
func (etc *EBPFTraceCorrelator) GetSpanContextByConn(connKey ConnKey) (*SpanContext, bool) {
	etc.mu.RLock()
	defer etc.mu.RUnlock()
	sc, ok := etc.connSpanMap[connKey]
	return sc, ok
}

// CleanupExpired 清理过期连接映射
func (etc *EBPFTraceCorrelator) CleanupExpired() {
	etc.mu.Lock()
	defer etc.mu.Unlock()
	// 简化：只限制映射大小
	if len(etc.connSpanMap) > etc.config.MaxConnMapSize {
		etc.connSpanMap = make(map[ConnKey]*SpanContext)
	}
	if len(etc.traceFlows) > etc.config.MaxConnMapSize {
		etc.traceFlows = make(map[TraceID][]*FlowRecord)
	}
}

// 解析各种格式的 Trace ID

func (etc *EBPFTraceCorrelator) parseCustomTraceID(traceID string, headers map[string][]string) *SpanContext {
	sc := &SpanContext{
		TraceID: TraceID(traceID),
		Baggage: make(map[string]string),
		Sampled: true,
	}
	if spanIDs, ok := headers["x-span-id"]; ok && len(spanIDs) > 0 {
		sc.SpanID = SpanID(spanIDs[0])
	}
	if parentIDs, ok := headers["x-parent-span-id"]; ok && len(parentIDs) > 0 {
		sc.ParentID = SpanID(parentIDs[0])
	}
	return sc
}

func (etc *EBPFTraceCorrelator) parseB3TraceID(traceID string, headers map[string][]string) *SpanContext {
	sc := &SpanContext{
		TraceID: TraceID(traceID),
		Baggage: make(map[string]string),
		Sampled: true,
	}
	if spanIDs, ok := headers["x-b3-spanid"]; ok && len(spanIDs) > 0 {
		sc.SpanID = SpanID(spanIDs[0])
	}
	if parentIDs, ok := headers["x-b3-parentspanid"]; ok && len(parentIDs) > 0 {
		sc.ParentID = SpanID(parentIDs[0])
	}
	if sampled, ok := headers["x-b3-sampled"]; ok && len(sampled) > 0 && sampled[0] == "0" {
		sc.Sampled = false
	}
	return sc
}

func (etc *EBPFTraceCorrelator) parseJaegerTraceID(value string) *SpanContext {
	// 格式: {trace-id}:{span-id}:{parent-span-id}:{flags}
	// trace-id 可以是 64 位或 128 位 (16 或 32 hex chars)
	parts := splitByChar(value, ':')
	if len(parts) < 4 {
		return nil
	}
	sc := &SpanContext{
		TraceID: TraceID(parts[0]),
		SpanID:  SpanID(parts[1]),
		Baggage: make(map[string]string),
	}
	if parts[2] != "0" {
		sc.ParentID = SpanID(parts[2])
	}
	if parts[3] == "0" {
		sc.Sampled = false
	} else {
		sc.Sampled = true
	}
	return sc
}

func (etc *EBPFTraceCorrelator) parseSkyWalkingTraceID(value string) *SpanContext {
	// sw8 格式: {trace-id}-{span-id}-... (复杂格式，简化处理)
	parts := splitByChar(value, '-')
	if len(parts) < 2 {
		return nil
	}
	return &SpanContext{
		TraceID: TraceID(parts[0]),
		SpanID:  SpanID(parts[1]),
		Baggage: make(map[string]string),
		Sampled: true,
	}
}

func splitByChar(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// ============================================================================
// 三、跨服务调用链完整还原
// ============================================================================

// ChainRebuilder 调用链重建器
//
// 职责：
//   - 从分散的 Span 记录中重建完整的调用链
//   - 处理跨服务边界的 Span 关联
//   - 构建父子关系树
//   - 计算端到端延迟
type ChainRebuilder struct {
	mu sync.RWMutex
}

// NewChainRebuilder 创建调用链重建器
func NewChainRebuilder() *ChainRebuilder {
	return &ChainRebuilder{}
}

// RebuiltTrace 重建后的完整追踪
type RebuiltTrace struct {
	TraceID      TraceID
	RootSpanID   SpanID
	Spans        []*RebuiltSpan
	Tree         *SpanTree
	Duration     time.Duration
	ServiceCount int
	Depth        int
	HasError     bool
	HasTimeout   bool
}

// RebuiltSpan 重建后的 Span (包含计算字段)
type RebuiltSpan struct {
	Span
	// 计算字段
	Children    []*RebuiltSpan
	ChildCount  int
	SubtreeDuration time.Duration
	Path        []SpanID // 从 root 到当前的路径
	Depth       int
	IsLeaf      bool
	IsRoot      bool
	IsCritical  bool // 关键路径上的节点
}

// SpanTree Span 树节点
type SpanTree struct {
	SpanID    SpanID
	Name      string
	Service   string
	Duration  time.Duration
	Status    SpanStatus
	Children  []*SpanTree
	Depth     int
}

// Rebuild 从 Span 列表重建完整调用链
func (cr *ChainRebuilder) Rebuild(traceID TraceID, spans []Span) *RebuiltTrace {
	if len(spans) == 0 {
		return nil
	}

	// 构建 Span 映射和父子关系
	spanMap := make(map[SpanID]*RebuiltSpan)
	for i := range spans {
		rs := &RebuiltSpan{Span: spans[i]}
		spanMap[rs.SpanID] = rs
	}

	// 建立父子关系
	var rootSpan *RebuiltSpan
	for _, rs := range spanMap {
		if rs.ParentID == "" {
			rootSpan = rs
			rs.IsRoot = true
		} else if parent, ok := spanMap[rs.ParentID]; ok {
			parent.Children = append(parent.Children, rs)
			parent.ChildCount++
		}
	}

	if rootSpan == nil {
		// 找不到 root，取第一个作为 root
		rootSpan = spanMap[spans[0].SpanID]
		rootSpan.IsRoot = true
	}

	// 构建树和计算路径
	maxDepth := 0
	serviceSet := make(map[string]bool)
	var buildTree func(rs *RebuiltSpan, depth int, path []SpanID)
	buildTree = func(rs *RebuiltSpan, depth int, path []SpanID) {
		rs.Depth = depth
		rs.Path = append([]SpanID{}, path...)
		rs.Path = append(rs.Path, rs.SpanID)
		if depth > maxDepth {
			maxDepth = depth
		}
		if rs.Service != "" {
			serviceSet[rs.Service] = true
		}
		if len(rs.Children) == 0 {
			rs.IsLeaf = true
		}
		for _, child := range rs.Children {
			buildTree(child, depth+1, rs.Path)
		}
	}
	buildTree(rootSpan, 0, []SpanID{})

	// 计算子树持续时间
	var calcSubtreeDuration func(rs *RebuiltSpan) time.Time
	calcSubtreeDuration = func(rs *RebuiltSpan) time.Time {
		maxEnd := rs.StartTime.Add(rs.Duration)
		for _, child := range rs.Children {
			childEnd := calcSubtreeDuration(child)
			if childEnd.After(maxEnd) {
				maxEnd = childEnd
			}
		}
		rs.SubtreeDuration = maxEnd.Sub(rs.StartTime)
		return maxEnd
	}
	calcSubtreeDuration(rootSpan)

	// 识别关键路径（延迟贡献最大的路径）
	cr.markCriticalPath(rootSpan)

	// 构建树结构
	tree := cr.buildSpanTree(rootSpan)

	// 计算总时长
	var hasError, hasTimeout bool
	for _, rs := range spanMap {
		if rs.Status == SpanStatusError {
			hasError = true
		}
		if rs.Status == SpanStatusTimeout {
			hasTimeout = true
		}
	}

	rebuilt := &RebuiltTrace{
		TraceID:      traceID,
		RootSpanID:   rootSpan.SpanID,
		Duration:     rootSpan.SubtreeDuration,
		ServiceCount: len(serviceSet),
		Depth:        maxDepth,
		HasError:     hasError,
		HasTimeout:   hasTimeout,
		Tree:         tree,
	}

	// 收集所有 span
	for _, rs := range spanMap {
		rebuilt.Spans = append(rebuilt.Spans, rs)
	}

	// 按开始时间排序
	sort.Slice(rebuilt.Spans, func(i, j int) bool {
		return rebuilt.Spans[i].StartTime.Before(rebuilt.Spans[j].StartTime)
	})

	return rebuilt
}

// markCriticalPath 标记关键路径上的 Span
// 关键路径 = 每个父节点的子节点中持续时间最长的路径
func (cr *ChainRebuilder) markCriticalPath(root *RebuiltSpan) {
	root.IsCritical = true
	for _, child := range root.Children {
		cr.markCriticalPath(child)
	}

	// 标记每个父节点的最长子路径
	for _, rs := range []*RebuiltSpan{root} {
		if len(rs.Children) == 0 {
			continue
		}
		var maxChild *RebuiltSpan
		var maxDuration time.Duration
		for _, child := range rs.Children {
			if child.SubtreeDuration > maxDuration {
				maxDuration = child.SubtreeDuration
				maxChild = child
			}
		}
		if maxChild != nil {
			maxChild.IsCritical = true
		}
	}
}

func (cr *ChainRebuilder) buildSpanTree(root *RebuiltSpan) *SpanTree {
	if root == nil {
		return nil
	}
	tree := &SpanTree{
		SpanID:   root.SpanID,
		Name:     root.Name,
		Service:  root.Service,
		Duration: root.Duration,
		Status:   root.Status,
		Depth:    root.Depth,
	}
	for _, child := range root.Children {
		tree.Children = append(tree.Children, cr.buildSpanTree(child))
	}
	return tree
}

// ============================================================================
// 四、慢调用/错误调用自动标记
// ============================================================================

// AutoMarker 自动标记器
//
// 职责：
//   - 根据延迟阈值自动标记慢调用
//   - 根据错误状态自动标记错误调用
//   - 根据异常模式自动标记问题调用
//   - 生成标记事件
type AutoMarker struct {
	config *AutoMarkerConfig
	mu     sync.RWMutex

	// 历史数据用于动态阈值
	history map[string]*LatencyHistory
}

// AutoMarkerConfig 自动标记配置
type AutoMarkerConfig struct {
	// 慢调用阈值 (绝对值 ms)
	SlowCallThresholdMs float64
	// 慢调用 P99 倍数阈值 (动态)
	SlowCallP99Factor float64
	// 错误调用标记
	MarkErrorCalls bool
	// 超时调用标记
	MarkTimeoutCalls bool
	// 高延迟调用标记
	MarkHighLatencyCalls bool
	// 历史窗口大小
	HistoryWindowSize int
	// 标记的延迟基线 (低于此值不标记)
	MinLatencyToMarkMs float64
}

// DefaultAutoMarkerConfig 返回默认配置
func DefaultAutoMarkerConfig() *AutoMarkerConfig {
	return &AutoMarkerConfig{
		SlowCallThresholdMs:  500,  // 500ms
		SlowCallP99Factor:      2.0,  // 超过 P99 的 2 倍
		MarkErrorCalls:         true,
		MarkTimeoutCalls:       true,
		MarkHighLatencyCalls:   true,
		HistoryWindowSize:      100,
		MinLatencyToMarkMs:     50,   // 低于 50ms 不标记
	}
}

// NewAutoMarker 创建自动标记器
func NewAutoMarker(config *AutoMarkerConfig) *AutoMarker {
	if config == nil {
		config = DefaultAutoMarkerConfig()
	}
	return &AutoMarker{
		config:  config,
		history: make(map[string]*LatencyHistory),
	}
}

// LatencyHistory 延迟历史
type LatencyHistory struct {
	ServiceName string
	SpanName    string
	Latencies   []float64 // ms
	MaxSize     int
}

// Add 添加延迟记录
func (lh *LatencyHistory) Add(latencyMs float64) {
	lh.Latencies = append(lh.Latencies, latencyMs)
	if lh.MaxSize > 0 && len(lh.Latencies) > lh.MaxSize {
		lh.Latencies = lh.Latencies[len(lh.Latencies)-lh.MaxSize:]
	}
}

// P99 返回 P99 延迟
func (lh *LatencyHistory) P99() float64 {
	if len(lh.Latencies) == 0 {
		return 0
	}
	sorted := make([]float64, len(lh.Latencies))
	copy(sorted, lh.Latencies)
	sort.Float64s(sorted)
	idx := int(math.Ceil(0.99 * float64(len(sorted)-1)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Avg 返回平均延迟
func (lh *LatencyHistory) Avg() float64 {
	if len(lh.Latencies) == 0 {
		return 0
	}
	var sum float64
	for _, v := range lh.Latencies {
		sum += v
	}
	return sum / float64(len(lh.Latencies))
}

// MarkedSpan 标记后的 Span
type MarkedSpan struct {
	Span
	Marks       []SpanMark    // 标记列表
	Severity    MarkSeverity  // 最高严重级别
	LatencyMs   float64       // 延迟 (ms)
	P99Latency  float64       // P99 延迟
	AvgLatency  float64       // 平均延迟
}

// SpanMark Span 标记
type SpanMark struct {
	Type        MarkType
	Severity    MarkSeverity
	Message     string
	LatencyMs   float64
	ThresholdMs float64
	Timestamp   time.Time
}

// MarkType 标记类型
type MarkType string

const (
	MarkTypeSlowCall   MarkType = "slow_call"    // 慢调用
	MarkTypeErrorCall  MarkType = "error_call"   // 错误调用
	MarkTypeTimeout    MarkType = "timeout"      // 超时
	MarkTypeHighLatency MarkType = "high_latency" // 高延迟
	MarkTypeAnomaly    MarkType = "anomaly"      // 异常模式
)

// MarkSeverity 标记严重级别
type MarkSeverity string

const (
	MarkSeverityInfo     MarkSeverity = "info"
	MarkSeverityWarning  MarkSeverity = "warning"
	MarkSeverityCritical MarkSeverity = "critical"
)

// MarkSpans 对 Span 列表进行自动标记
func (am *AutoMarker) MarkSpans(spans []Span) []*MarkedSpan {
	var markedSpans []*MarkedSpan

	// 第一遍：收集历史数据
	for _, span := range spans {
		latencyMs := span.Duration.Seconds() * 1000
		key := fmt.Sprintf("%s:%s", span.Service, span.Name)
		am.updateHistory(key, latencyMs)
	}

	// 第二遍：标记
	for _, span := range spans {
		ms := am.markSpan(span)
		markedSpans = append(markedSpans, ms)
	}

	return markedSpans
}

// MarkTrace 对重建后的追踪进行标记
func (am *AutoMarker) MarkTrace(trace *RebuiltTrace) *MarkedTrace {
	if trace == nil {
		return nil
	}

	mt := &MarkedTrace{
		TraceID:   trace.TraceID,
		Duration:  trace.Duration,
		HasError:  trace.HasError,
		SpanMarks: make(map[SpanID][]SpanMark),
	}

	// 标记每个 Span
	for _, span := range trace.Spans {
		ms := am.markSpan(span.Span)
		mt.SpanMarks[span.SpanID] = ms.Marks
		if ms.Severity == MarkSeverityCritical {
			mt.CriticalSpanCount++
		} else if ms.Severity == MarkSeverityWarning {
			mt.WarningSpanCount++
		}
		if len(ms.Marks) > 0 {
			mt.MarkedSpanCount++
		}
	}

	// 标记整体 Trace
	if trace.HasError {
		mt.TraceMarks = append(mt.TraceMarks, SpanMark{
			Type:     MarkTypeErrorCall,
			Severity: MarkSeverityCritical,
			Message:  "Trace 包含错误调用",
		})
	}
	if trace.Duration.Milliseconds() > int64(am.config.SlowCallThresholdMs) {
		mt.TraceMarks = append(mt.TraceMarks, SpanMark{
			Type:     MarkTypeSlowCall,
			Severity: MarkSeverityWarning,
			Message:  fmt.Sprintf("Trace 总延迟 %.1fms 超过阈值 %.0fms", trace.Duration.Seconds()*1000, am.config.SlowCallThresholdMs),
		})
	}

	return mt
}

// MarkedTrace 标记后的追踪
type MarkedTrace struct {
	TraceID            TraceID
	Duration           time.Duration
	HasError           bool
	SpanMarks          map[SpanID][]SpanMark
	TraceMarks         []SpanMark
	MarkedSpanCount    int
	CriticalSpanCount  int
	WarningSpanCount   int
}

func (am *AutoMarker) markSpan(span Span) *MarkedSpan {
	ms := &MarkedSpan{
		Span:      span,
		LatencyMs: span.Duration.Seconds() * 1000,
		Marks:     []SpanMark{},
	}

	key := fmt.Sprintf("%s:%s", span.Service, span.Name)

	// 检查历史数据
	history := am.getHistory(key)
	if history != nil {
		ms.P99Latency = history.P99()
		ms.AvgLatency = history.Avg()
	}

	// 标记规则 1: 错误调用
	if am.config.MarkErrorCalls && (span.Status == SpanStatusError || span.Status == SpanStatusTimeout) {
		severity := MarkSeverityWarning
		msg := "错误调用"
		if span.Status == SpanStatusTimeout {
			severity = MarkSeverityCritical
			msg = "调用超时"
		}
		ms.Marks = append(ms.Marks, SpanMark{
			Type:      MarkTypeErrorCall,
			Severity:  severity,
			Message:   fmt.Sprintf("Span %s/%s: %s", span.Service, span.Name, msg),
			LatencyMs: ms.LatencyMs,
			Timestamp: time.Now(),
		})
		ms.Severity = maxSeverity(ms.Severity, severity)
	}

	// 标记规则 2: 慢调用 (绝对阈值)
	if am.config.MarkHighLatencyCalls && ms.LatencyMs > am.config.SlowCallThresholdMs {
		if ms.LatencyMs >= am.config.MinLatencyToMarkMs {
			ms.Marks = append(ms.Marks, SpanMark{
				Type:        MarkTypeSlowCall,
				Severity:    MarkSeverityWarning,
				Message:     fmt.Sprintf("Span %s/%s 延迟 %.1fms 超过阈值 %.0fms", span.Service, span.Name, ms.LatencyMs, am.config.SlowCallThresholdMs),
				LatencyMs:   ms.LatencyMs,
				ThresholdMs: am.config.SlowCallThresholdMs,
				Timestamp:   time.Now(),
			})
			ms.Severity = maxSeverity(ms.Severity, MarkSeverityWarning)
		}
	}

	// 标记规则 3: 动态阈值 (超过 P99 的 N 倍)
	if history != nil && history.P99() > 0 {
		dynamicThreshold := history.P99() * am.config.SlowCallP99Factor
		if am.config.MarkHighLatencyCalls && ms.LatencyMs > dynamicThreshold {
			ms.Marks = append(ms.Marks, SpanMark{
				Type:        MarkTypeAnomaly,
				Severity:    MarkSeverityCritical,
				Message:     fmt.Sprintf("Span %s/%s 延迟 %.1fms 超过 P99 (%.1fms) 的 %.1f 倍", span.Service, span.Name, ms.LatencyMs, history.P99(), am.config.SlowCallP99Factor),
				LatencyMs:   ms.LatencyMs,
				ThresholdMs: dynamicThreshold,
				Timestamp:   time.Now(),
			})
			ms.Severity = maxSeverity(ms.Severity, MarkSeverityCritical)
		}
	}

	return ms
}

func (am *AutoMarker) updateHistory(key string, latencyMs float64) {
	am.mu.Lock()
	defer am.mu.Unlock()

	history, exists := am.history[key]
	if !exists {
		history = &LatencyHistory{
			ServiceName: key,
			MaxSize:     am.config.HistoryWindowSize,
		}
		am.history[key] = history
	}
	history.Add(latencyMs)
}

func (am *AutoMarker) getHistory(key string) *LatencyHistory {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.history[key]
}

func maxSeverity(a, b MarkSeverity) MarkSeverity {
	order := map[MarkSeverity]int{
		MarkSeverityInfo:     1,
		MarkSeverityWarning:  2,
		MarkSeverityCritical: 3,
		"":                   0,
	}
	if order[a] > order[b] {
		return a
	}
	return b
}

// ============================================================================
// 五、Trace 增强 API
// ============================================================================

// EnhancedTraceEngine 增强版追踪引擎
type EnhancedTraceEngine struct {
	*TraceEngine
	w3cGen      *W3CTraceIDGenerator
	correlator  *EBPFTraceCorrelator
	rebuilder   *ChainRebuilder
	autoMarker  *AutoMarker
}

// NewEnhancedTraceEngine 创建增强版追踪引擎
func NewEnhancedTraceEngine(config *TraceConfig) *EnhancedTraceEngine {
	base := NewTraceEngine(config)
	return &EnhancedTraceEngine{
		TraceEngine: base,
		w3cGen:      NewW3CTraceIDGenerator(),
		correlator:  NewEBPFTraceCorrelator(nil),
		rebuilder:   NewChainRebuilder(),
		autoMarker:  NewAutoMarker(nil),
	}
}

// StartEnhancedSpan 开始增强版 Span（支持 W3C 标准 Trace ID）
func (e *EnhancedTraceEngine) StartEnhancedSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	// 先尝试提取父上下文
	parentCtx := e.propagator.Extract(ctx)

	var traceID TraceID
	var parentID SpanID

	if parentCtx != nil && parentCtx.TraceID != "" {
		traceID = parentCtx.TraceID
		parentID = parentCtx.SpanID
	} else {
		traceID = e.w3cGen.GenerateTraceID()
	}

	spanID := e.w3cGen.GenerateSpanID()

	sampled := e.shouldSample()

	span := &Span{
		TraceID:   traceID,
		SpanID:    spanID,
		ParentID:  parentID,
		Name:      name,
		StartTime: time.Now(),
		Status:    SpanStatusOK,
		Tags:      make(map[string]string),
	}

	for _, opt := range opts {
		opt(span)
	}

	spanCtx := &SpanContext{
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: parentID,
		Sampled:  sampled,
		Baggage:  make(map[string]string),
	}
	if parentCtx != nil && parentCtx.Baggage != nil {
		spanCtx.Baggage = parentCtx.Baggage
	}
	ctx = context.WithValue(ctx, traceContextKey{}, spanCtx)

	return ctx, span
}

// RebuildTrace 重建完整调用链
func (e *EnhancedTraceEngine) RebuildTrace(traceID TraceID) (*RebuiltTrace, error) {
	spans, err := e.store.GetSpans(traceID)
	if err != nil {
		return nil, err
	}
	return e.rebuilder.Rebuild(traceID, spans), nil
}

// MarkTrace 自动标记追踪
func (e *EnhancedTraceEngine) MarkTrace(traceID TraceID) (*MarkedTrace, error) {
	rebuilt, err := e.RebuildTrace(traceID)
	if err != nil {
		return nil, err
	}
	return e.autoMarker.MarkTrace(rebuilt), nil
}

// GetMarkedTrace 获取带标记的完整追踪
func (e *EnhancedTraceEngine) GetMarkedTrace(traceID TraceID) (*MarkedTrace, error) {
	return e.MarkTrace(traceID)
}

// ExtractTraceFromEBPF 从 eBPF 流量中提取 Trace 信息
func (e *EnhancedTraceEngine) ExtractTraceFromEBPF(
	headers map[string][]string,
	connKey ConnKey,
) *SpanContext {
	return e.correlator.ExtractTraceFromHTTP(headers, connKey)
}

// GetFlowsByTraceID 获取 Trace 关联的流量
func (e *EnhancedTraceEngine) GetFlowsByTraceID(traceID TraceID) []*FlowRecord {
	return e.correlator.GetFlowsByTraceID(traceID)
}

// GenerateW3CTraceParent 生成 W3C traceparent 头
func (e *EnhancedTraceEngine) GenerateW3CTraceParent(sampled bool) string {
	return e.w3cGen.GenerateTraceParent(sampled)
}

// GetTraceStats 获取追踪统计（带标记信息）
func (e *EnhancedTraceEngine) GetTraceStats(traceID TraceID) (*TraceStats, *MarkedTrace, error) {
	stats := e.store.Stats()
	marked, err := e.MarkTrace(traceID)
	return &stats, marked, err
}

// QuerySlowTraces 查询慢追踪
func (e *EnhancedTraceEngine) QuerySlowTraces(thresholdMs float64, limit int) []TraceSummary {
	query := TraceQuery{
		MinDuration: time.Duration(thresholdMs) * time.Millisecond,
		OrderBy:     "duration",
		OrderDesc:   true,
		Limit:       limit,
	}
	result, _ := e.QueryTraces(query)
	if result == nil {
		return nil
	}
	return result.Traces
}

// QueryErrorTraces 查询错误追踪
func (e *EnhancedTraceEngine) QueryErrorTraces(limit int) []TraceSummary {
	query := TraceQuery{
		Status:    SpanStatusError,
		OrderBy:   "start_time",
		OrderDesc: true,
		Limit:     limit,
	}
	result, _ := e.QueryTraces(query)
	if result == nil {
		return nil
	}
	return result.Traces
}

// GetServiceDependencyMap 获取服务依赖图（基于 Trace 数据）
func (e *EnhancedTraceEngine) GetServiceDependencyMap(traceID TraceID) map[string][]string {
	spans, err := e.store.GetSpans(traceID)
	if err != nil {
		return nil
	}

	deps := make(map[string][]string)
	spanMap := make(map[SpanID]Span)
	for _, span := range spans {
		spanMap[span.SpanID] = span
	}

	for _, span := range spans {
		if span.ParentID != "" {
			if parent, ok := spanMap[span.ParentID]; ok {
				if parent.Service != "" && span.Service != "" && parent.Service != span.Service {
					if !contains(deps[parent.Service], span.Service) {
						deps[parent.Service] = append(deps[parent.Service], span.Service)
					}
				}
			}
		}
	}
	return deps
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
