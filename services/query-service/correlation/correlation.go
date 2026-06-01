// Package correlation 多维关联引擎
//
// 关联维度:
//   1. Trace-Flow Correlation:   trace_id 关联 eBPF flow 和 OTEL span
//   2. Process-Trace Correlation: pid/comm 关联 process 和 trace
//   3. Service-Trace Correlation: service_name 关联 service topology 和 trace
//
// 数据融合:
//
//	eBPF Network Flow ──┐
//	                     ├──▶ Correlation Engine ──▶ Unified Telemetry
//	OTEL Trace ─────────┤
//	                     │
//	K8S Metadata ───────┤
//	Process Metadata ───┘
//
// Root Cause Analysis 基础:
//   - 从异常 trace → 关联 flow → 找到慢调用链路
//   - 从异常 flow → 关联 trace → 找到完整调用栈
//   - 从 service topology → 关联 trace → 找到故障服务
package correlation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// CorrelationKey - 关联索引键
// ---------------------------------------------------------------------------

// CorrelationKey 用于索引关联记录的键。
// Type 可选值: trace_id, service_name, process_pid, namespace_pod
type CorrelationKey struct {
	Type  string
	Value string
}

// String 返回 "type:value" 格式的字符串表示。
func (k CorrelationKey) String() string {
	return k.Type + ":" + k.Value
}

// ---------------------------------------------------------------------------
// CorrelationRecord - 关联记录
// ---------------------------------------------------------------------------

// CorrelationRecord 表示一条关联记录。
// Source 标识数据来源: ebpf / otel / k8s / process
type CorrelationRecord struct {
	Key       CorrelationKey
	Source    string
	Timestamp int64
	Data      interface{}
	TTL       time.Duration
}

// ---------------------------------------------------------------------------
// FlowTraceLink - Flow 与 Trace 的关联链路
// ---------------------------------------------------------------------------

// FlowTraceLink 将一条 eBPF 网络流与一条 OTEL Trace/Span 关联起来。
type FlowTraceLink struct {
	TraceID       string
	SpanID        string
	FlowSrcIP     string
	FlowDstIP     string
	FlowSrcPort   uint16
	FlowDstPort   uint16
	FlowProtocol  string
	FlowLatencyNs uint64
	FlowBytes     uint64
	FlowTimestamp int64
	ServiceName   string
	Namespace     string
	Pod           string
	ProcessName   string
	PID           uint32
}

// ---------------------------------------------------------------------------
// EndpointLatency - 端点延迟统计
// ---------------------------------------------------------------------------

// EndpointLatency 描述某个端点的延迟和错误率统计。
type EndpointLatency struct {
	Endpoint     string
	AvgLatencyMs float64
	ErrorRate    float64
	CallCount    int
}

// ---------------------------------------------------------------------------
// ServiceTraceSummary - 服务维度 Trace 汇总
// ---------------------------------------------------------------------------

// ServiceTraceSummary 对一个服务的所有 Trace 做聚合统计。
type ServiceTraceSummary struct {
	ServiceName  string
	Namespace    string
	TraceCount   int
	ErrorCount   int
	P50LatencyMs float64
	P95LatencyMs float64
	P99LatencyMs float64
	AvgLatencyMs float64
	TopEndpoints []EndpointLatency
}

// ---------------------------------------------------------------------------
// ProcessTraceSummary - 进程维度 Trace 汇总
// ---------------------------------------------------------------------------

// ProcessTraceSummary 对一个进程的所有 Trace 做聚合统计。
type ProcessTraceSummary struct {
	ProcessName  string
	PID          uint32
	Hostname     string
	TraceCount   int
	ErrorCount   int
	AvgLatencyMs float64
}

// ---------------------------------------------------------------------------
// 数据输入结构体
// ---------------------------------------------------------------------------

// FlowData 表示从 eBPF 采集的网络流数据。
type FlowData struct {
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	Protocol    string
	Bytes       uint64
	Packets     uint64
	LatencyNs   uint64
	TraceID     string
	ServiceName string
	Namespace   string
	Pod         string
	ProcessName string
	PID         uint32
	Timestamp   int64
	TenantID    string
}

// SpanData 表示从 OTEL 采集的 Span 数据。
type SpanData struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	ServiceName  string
	DurationNs   int64
	Status       string
	Attributes   map[string]string
	StartTime    int64
	TenantID     string
}

// K8SMetadata 表示 Kubernetes 资源元数据。
type K8SMetadata struct {
	Pod           string
	Namespace     string
	Service       string
	Deployment    string
	Node          string
	ContainerID   string
	ContainerName string
	Labels        map[string]string
	Timestamp     int64
}

// ProcessMetadata 表示进程元数据。
type ProcessMetadata struct {
	PID         uint32
	ProcessName string
	Comm        string
	Hostname    string
	ContainerID string
	StartTime   int64
}

// ---------------------------------------------------------------------------
// Root Cause Analysis 结构体
// ---------------------------------------------------------------------------

// ErrorSpanInfo 描述一个出错的 Span。
type ErrorSpanInfo struct {
	SpanID        string
	ServiceName   string
	OperationName string
	Error         string
	DurationNs    int64
}

// SlowSpanInfo 描述一个慢 Span（延迟超过 P99 基线）。
type SlowSpanInfo struct {
	SpanID        string
	ServiceName   string
	OperationName string
	DurationNs    int64
	P99BaselineNs int64
}

// RootCauseReport 是根因分析报告。
type RootCauseReport struct {
	TraceID          string
	ErrorSpans       []*ErrorSpanInfo
	SlowSpans        []*SlowSpanInfo
	RelatedFlows     []*FlowTraceLink
	AffectedServices []string
	SuggestedCauses  []string
	GeneratedAt      int64
}

// ---------------------------------------------------------------------------
// CorrelationEngine - 多维关联引擎
// ---------------------------------------------------------------------------

// CorrelationEngine 是核心关联引擎，负责接收多维数据、建立关联索引、
// 并提供根因分析能力。
type CorrelationEngine struct {
	index            map[string][]*CorrelationRecord // key string → records
	flowTraceLinks   []*FlowTraceLink                // ring buffer
	serviceSummaries map[string]*ServiceTraceSummary // "namespace/service" → summary
	processSummaries map[string]*ProcessTraceSummary // "hostname/process:pid" → summary
	mu               sync.RWMutex
	maxLinks         int
	ttl              time.Duration
	cancel           context.CancelFunc
}

// NewCorrelationEngine 创建一个新的关联引擎实例。
// maxLinks 控制 flowTraceLinks 环形缓冲区大小（默认 1000000）。
// ttl 控制关联记录的生存时间（默认 30 分钟）。
func NewCorrelationEngine(maxLinks int, ttl time.Duration) *CorrelationEngine {
	if maxLinks <= 0 {
		maxLinks = 1000000
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &CorrelationEngine{
		index:            make(map[string][]*CorrelationRecord),
		flowTraceLinks:   make([]*FlowTraceLink, 0, maxLinks),
		serviceSummaries: make(map[string]*ServiceTraceSummary),
		processSummaries: make(map[string]*ProcessTraceSummary),
		maxLinks:         maxLinks,
		ttl:              ttl,
	}
}

// Start 启动后台清理协程。
func (e *CorrelationEngine) Start(ctx context.Context) {
	ctx, e.cancel = context.WithCancel(ctx)
	go e.cleanupLoop(ctx)
}

// Stop 停止后台清理协程。
func (e *CorrelationEngine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// ---------------------------------------------------------------------------
// 数据接入 (Ingestion)
// ---------------------------------------------------------------------------

// IngestFlow 接入 eBPF 网络流数据，建立关联记录。
func (e *CorrelationEngine) IngestFlow(flow FlowData) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UnixNano()

	// 按 trace_id 建立关联
	if flow.TraceID != "" {
		key := CorrelationKey{Type: "trace_id", Value: flow.TraceID}
		e.addRecord(key.String(), "ebpf", now, flow, e.ttl)
	}

	// 按 service_name 建立关联
	if flow.ServiceName != "" {
		key := CorrelationKey{Type: "service_name", Value: flow.ServiceName}
		e.addRecord(key.String(), "ebpf", now, flow, e.ttl)
	}

	// 按 process_pid 建立关联
	if flow.ProcessName != "" && flow.PID > 0 {
		key := CorrelationKey{Type: "process_pid", Value: fmt.Sprintf("%s:%d", flow.ProcessName, flow.PID)}
		e.addRecord(key.String(), "ebpf", now, flow, e.ttl)
	}

	// 按 namespace_pod 建立关联
	if flow.Namespace != "" && flow.Pod != "" {
		key := CorrelationKey{Type: "namespace_pod", Value: fmt.Sprintf("%s/%s", flow.Namespace, flow.Pod)}
		e.addRecord(key.String(), "ebpf", now, flow, e.ttl)
	}

	// 维护 flowTraceLinks 环形缓冲区
	link := &FlowTraceLink{
		TraceID:       flow.TraceID,
		FlowSrcIP:     flow.SrcIP,
		FlowDstIP:     flow.DstIP,
		FlowSrcPort:   flow.SrcPort,
		FlowDstPort:   flow.DstPort,
		FlowProtocol:  flow.Protocol,
		FlowLatencyNs: flow.LatencyNs,
		FlowBytes:     flow.Bytes,
		FlowTimestamp: flow.Timestamp,
		ServiceName:   flow.ServiceName,
		Namespace:     flow.Namespace,
		Pod:           flow.Pod,
		ProcessName:   flow.ProcessName,
		PID:           flow.PID,
	}
	e.addFlowTraceLink(link)
}

// IngestSpan 接入 OTEL Span 数据，建立关联记录并更新聚合统计。
func (e *CorrelationEngine) IngestSpan(span SpanData) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UnixNano()

	// 按 trace_id 建立关联
	key := CorrelationKey{Type: "trace_id", Value: span.TraceID}
	e.addRecord(key.String(), "otel", now, span, e.ttl)

	// 按 service_name 建立关联
	if span.ServiceName != "" {
		svcKey := CorrelationKey{Type: "service_name", Value: span.ServiceName}
		e.addRecord(svcKey.String(), "otel", now, span, e.ttl)
	}

	// 按 process_pid 建立关联（从 Attributes 中提取）
	if pid, ok := span.Attributes["process.pid"]; ok {
		comm := span.Attributes["process.executable.name"]
		if comm == "" {
			comm = span.ServiceName
		}
		procKey := CorrelationKey{Type: "process_pid", Value: fmt.Sprintf("%s:%s", comm, pid)}
		e.addRecord(procKey.String(), "otel", now, span, e.ttl)
	}

	// 更新服务汇总
	if span.ServiceName != "" {
		e.updateServiceSummaryLocked(span.ServiceName, &span)
	}

	// 更新进程汇总
	if comm, ok := span.Attributes["process.executable.name"]; ok {
		if pid, ok2 := span.Attributes["process.pid"]; ok2 {
			hostname := span.Attributes["host.name"]
			e.updateProcessSummaryLocked(comm, pid, hostname, &span)
		}
	}
}

// IngestK8SMetadata 接入 Kubernetes 元数据。
func (e *CorrelationEngine) IngestK8SMetadata(k8s K8SMetadata) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UnixNano()

	// 按 namespace_pod 建立关联
	if k8s.Pod != "" && k8s.Namespace != "" {
		key := CorrelationKey{Type: "namespace_pod", Value: fmt.Sprintf("%s/%s", k8s.Namespace, k8s.Pod)}
		e.addRecord(key.String(), "k8s", now, k8s, e.ttl)
	}

	// 按 service_name 建立关联
	if k8s.Service != "" {
		key := CorrelationKey{Type: "service_name", Value: k8s.Service}
		e.addRecord(key.String(), "k8s", now, k8s, e.ttl)
	}
}

// IngestProcessMetadata 接入进程元数据。
func (e *CorrelationEngine) IngestProcessMetadata(proc ProcessMetadata) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UnixNano()

	// 按 process_pid 建立关联
	key := CorrelationKey{Type: "process_pid", Value: fmt.Sprintf("%s:%d", proc.ProcessName, proc.PID)}
	e.addRecord(key.String(), "process", now, proc, e.ttl)

	// 按 namespace_pod 建立关联（如果有容器 ID）
	if proc.ContainerID != "" {
		containerKey := CorrelationKey{Type: "namespace_pod", Value: proc.ContainerID}
		e.addRecord(containerKey.String(), "process", now, proc, e.ttl)
	}
}

// ---------------------------------------------------------------------------
// 查询 (Query)
// ---------------------------------------------------------------------------

// GetFlowsByTraceID 根据 traceID 查找所有关联的 FlowTraceLink。
func (e *CorrelationEngine) GetFlowsByTraceID(traceID string) []*FlowTraceLink {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var links []*FlowTraceLink
	for _, l := range e.flowTraceLinks {
		if l.TraceID == traceID {
			links = append(links, l)
		}
	}
	return links
}

// GetTracesByService 根据 serviceName 查找所有关联的 traceID。
func (e *CorrelationEngine) GetTracesByService(serviceName string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := CorrelationKey{Type: "service_name", Value: serviceName}.String()
	records := e.index[key]
	seen := make(map[string]struct{})
	var traceIDs []string
	for _, r := range records {
		switch v := r.Data.(type) {
		case SpanData:
			if _, ok := seen[v.TraceID]; !ok {
				seen[v.TraceID] = struct{}{}
				traceIDs = append(traceIDs, v.TraceID)
			}
		case FlowData:
			if v.TraceID != "" {
				if _, ok := seen[v.TraceID]; !ok {
					seen[v.TraceID] = struct{}{}
					traceIDs = append(traceIDs, v.TraceID)
				}
			}
		}
	}
	return traceIDs
}

// GetTracesByProcess 根据进程名和 PID 查找所有关联的 traceID。
func (e *CorrelationEngine) GetTracesByProcess(processName string, pid uint32) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := CorrelationKey{Type: "process_pid", Value: fmt.Sprintf("%s:%d", processName, pid)}.String()
	records := e.index[key]
	seen := make(map[string]struct{})
	var traceIDs []string
	for _, r := range records {
		switch v := r.Data.(type) {
		case SpanData:
			if _, ok := seen[v.TraceID]; !ok {
				seen[v.TraceID] = struct{}{}
				traceIDs = append(traceIDs, v.TraceID)
			}
		case FlowData:
			if v.TraceID != "" {
				if _, ok := seen[v.TraceID]; !ok {
					seen[v.TraceID] = struct{}{}
					traceIDs = append(traceIDs, v.TraceID)
				}
			}
		}
	}
	return traceIDs
}

// GetServiceSummary 获取服务的聚合统计摘要。
func (e *CorrelationEngine) GetServiceSummary(serviceName string) *ServiceTraceSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 尝试多种 key 格式
	for _, k := range []string{serviceName, "default/" + serviceName} {
		if s, ok := e.serviceSummaries[k]; ok {
			return s
		}
	}
	return nil
}

// GetProcessSummary 获取进程的聚合统计摘要。
func (e *CorrelationEngine) GetProcessSummary(processName string, pid uint32) *ProcessTraceSummary {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 尝试多种 key 格式
	for _, k := range []string{
		fmt.Sprintf("%s:%d", processName, pid),
		fmt.Sprintf(":%s:%d", processName, pid),
	} {
		if s, ok := e.processSummaries[k]; ok {
			return s
		}
	}
	return nil
}

// GetRootCauseAnalysis 对指定 traceID 执行根因分析，生成报告。
func (e *CorrelationEngine) GetRootCauseAnalysis(traceID string) *RootCauseReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	report := &RootCauseReport{
		TraceID:     traceID,
		GeneratedAt: time.Now().UnixNano(),
	}

	// 1. 获取该 trace 的所有 span
	key := CorrelationKey{Type: "trace_id", Value: traceID}.String()
	records := e.index[key]
	if len(records) == 0 {
		return report
	}

	var spans []SpanData
	serviceLatencies := make(map[string][]float64) // service → []latencyMs
	serviceErrors := make(map[string]int)          // service → error count
	serviceTotal := make(map[string]int)           // service → total count

	for _, r := range records {
		if span, ok := r.Data.(SpanData); ok {
			spans = append(spans, span)
			latencyMs := float64(span.DurationNs) / 1e6
			serviceLatencies[span.ServiceName] = append(serviceLatencies[span.ServiceName], latencyMs)
			serviceTotal[span.ServiceName]++
			if span.Status == "ERROR" {
				serviceErrors[span.ServiceName]++
			}
		}
	}

	// 2. 计算每个服务的 P99 基线（基于已聚合的 serviceSummary）
	serviceBaselines := make(map[string]float64) // service → P99 latency in ms
	for svcName := range serviceLatencies {
		if summary, ok := e.serviceSummaries[svcName]; ok {
			serviceBaselines[svcName] = summary.P99LatencyMs
		} else {
			// 从当前 trace 的 span 计算近似 P99
			serviceBaselines[svcName] = percentile(serviceLatencies[svcName], 99)
		}
	}

	// 3. 识别 error spans 和 slow spans
	affectedSet := make(map[string]struct{})
	for _, span := range spans {
		if span.Status == "ERROR" {
			errMsg := ""
			if span.Attributes != nil {
				errMsg = span.Attributes["error.message"]
				if errMsg == "" {
					errMsg = span.Attributes["error"]
				}
			}
			report.ErrorSpans = append(report.ErrorSpans, &ErrorSpanInfo{
				SpanID:        span.SpanID,
				ServiceName:   span.ServiceName,
				OperationName: span.Name,
				Error:         errMsg,
				DurationNs:    span.DurationNs,
			})
			affectedSet[span.ServiceName] = struct{}{}
			report.SuggestedCauses = append(report.SuggestedCauses,
				fmt.Sprintf("Error in %s: %s", span.ServiceName, errMsg))
		}

		latencyMs := float64(span.DurationNs) / 1e6
		p99Baseline := serviceBaselines[span.ServiceName]
		if p99Baseline > 0 && latencyMs > p99Baseline {
			report.SlowSpans = append(report.SlowSpans, &SlowSpanInfo{
				SpanID:        span.SpanID,
				ServiceName:   span.ServiceName,
				OperationName: span.Name,
				DurationNs:    span.DurationNs,
				P99BaselineNs: int64(p99Baseline * 1e6),
			})
			affectedSet[span.ServiceName] = struct{}{}
			report.SuggestedCauses = append(report.SuggestedCauses,
				fmt.Sprintf("High latency in %s (%s)", span.ServiceName, span.Name))
		}
	}

	// 4. 查找关联的 flows
	report.RelatedFlows = e.getFlowsByTraceIDLocked(traceID)
	for _, link := range report.RelatedFlows {
		if link.ServiceName != "" {
			affectedSet[link.ServiceName] = struct{}{}
		}
		// 检测网络重传（通过 bytes/packets 比值异常或延迟异常高）
		if link.FlowLatencyNs > 0 && link.FlowBytes > 0 {
			// 如果延迟超过 1 秒且 bytes 较少，可能是连接超时
			if link.FlowLatencyNs > uint64(time.Second) && link.FlowBytes < 1024 {
				report.SuggestedCauses = append(report.SuggestedCauses,
					fmt.Sprintf("Connection timeout between %s and %s",
						fmt.Sprintf("%s:%d", link.FlowSrcIP, link.FlowSrcPort),
						fmt.Sprintf("%s:%d", link.FlowDstIP, link.FlowDstPort)))
			}
		}
		// 检测重传：延迟高且协议为 TCP
		if link.FlowProtocol == "tcp" && link.FlowLatencyNs > uint64(500*time.Millisecond) {
			report.SuggestedCauses = append(report.SuggestedCauses,
				fmt.Sprintf("Network retransmission detected between %s and %s",
					fmt.Sprintf("%s:%d", link.FlowSrcIP, link.FlowSrcPort),
					fmt.Sprintf("%s:%d", link.FlowDstIP, link.FlowDstPort)))
		}
	}

	// 5. 检查服务错误率
	for svcName, total := range serviceTotal {
		if errs, ok := serviceErrors[svcName]; ok && total > 0 {
			errorRate := float64(errs) / float64(total) * 100
			if errorRate > 5.0 {
				report.SuggestedCauses = append(report.SuggestedCauses,
					fmt.Sprintf("High error rate in %s (%.1f%%)", svcName, errorRate))
			}
		}
	}

	// 6. 构建受影响服务列表
	for svc := range affectedSet {
		if svc != "" {
			report.AffectedServices = append(report.AffectedServices, svc)
		}
	}
	sort.Strings(report.AffectedServices)

	// 去重 SuggestedCauses
	report.SuggestedCauses = dedupStrings(report.SuggestedCauses)

	return report
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

// addRecord 向索引中添加一条关联记录。
// 调用方必须持有 e.mu 写锁。
func (e *CorrelationEngine) addRecord(key string, source string, ts int64, data interface{}, ttl time.Duration) {
	record := &CorrelationRecord{
		Key:       CorrelationKey{Type: strings.SplitN(key, ":", 2)[0], Value: strings.SplitN(key, ":", 2)[1]},
		Source:    source,
		Timestamp: ts,
		Data:      data,
		TTL:       ttl,
	}
	e.index[key] = append(e.index[key], record)
}

// addFlowTraceLink 向环形缓冲区添加一条 FlowTraceLink。
// 调用方必须持有 e.mu 写锁。
func (e *CorrelationEngine) addFlowTraceLink(link *FlowTraceLink) {
	if len(e.flowTraceLinks) >= e.maxLinks {
		// 环形缓冲区: 移除最旧的记录
		e.flowTraceLinks = append(e.flowTraceLinks[1:], link)
	} else {
		e.flowTraceLinks = append(e.flowTraceLinks, link)
	}
}

// getFlowsByTraceIDLocked 内部方法，调用方必须持有读锁。
func (e *CorrelationEngine) getFlowsByTraceIDLocked(traceID string) []*FlowTraceLink {
	var links []*FlowTraceLink
	for _, l := range e.flowTraceLinks {
		if l.TraceID == traceID {
			links = append(links, l)
		}
	}
	return links
}

// updateServiceSummaryLocked 更新服务聚合统计。
// 调用方必须持有 e.mu 写锁。
func (e *CorrelationEngine) updateServiceSummaryLocked(serviceName string, span *SpanData) {
	summary, ok := e.serviceSummaries[serviceName]
	if !ok {
		summary = &ServiceTraceSummary{
			ServiceName:  serviceName,
			TopEndpoints: make([]EndpointLatency, 0),
		}
		e.serviceSummaries[serviceName] = summary
	}

	summary.TraceCount++
	latencyMs := float64(span.DurationNs) / 1e6

	if span.Status == "ERROR" {
		summary.ErrorCount++
	}

	// 更新延迟分位数（增量近似）
	summary.AvgLatencyMs = (summary.AvgLatencyMs*float64(summary.TraceCount-1) + latencyMs) / float64(summary.TraceCount)

	// 收集所有延迟值用于计算分位数
	// 为了性能，我们使用 serviceSummaries 中存储的延迟列表
	// 这里通过 index 获取该服务的所有 span 来计算
	latencies := e.getServiceLatenciesLocked(serviceName)
	if len(latencies) > 0 {
		summary.P50LatencyMs = percentile(latencies, 50)
		summary.P95LatencyMs = percentile(latencies, 95)
		summary.P99LatencyMs = percentile(latencies, 99)
	}

	// 更新端点统计
	e.updateEndpointStats(summary, span)
}

// updateProcessSummaryLocked 更新进程聚合统计。
// 调用方必须持有 e.mu 写锁。
func (e *CorrelationEngine) updateProcessSummaryLocked(processName string, pid string, hostname string, span *SpanData) {
	key := fmt.Sprintf("%s:%s", processName, pid)
	summary, ok := e.processSummaries[key]
	if !ok {
		summary = &ProcessTraceSummary{
			ProcessName: processName,
			Hostname:    hostname,
		}
		// 解析 PID
		var pidVal uint32
		fmt.Sscanf(pid, "%d", &pidVal)
		summary.PID = pidVal
		e.processSummaries[key] = summary
	}

	summary.TraceCount++
	latencyMs := float64(span.DurationNs) / 1e6
	summary.AvgLatencyMs = (summary.AvgLatencyMs*float64(summary.TraceCount-1) + latencyMs) / float64(summary.TraceCount)

	if span.Status == "ERROR" {
		summary.ErrorCount++
	}
}

// getServiceLatenciesLocked 获取指定服务的所有 span 延迟（毫秒）。
// 调用方必须持有读锁或写锁。
func (e *CorrelationEngine) getServiceLatenciesLocked(serviceName string) []float64 {
	key := CorrelationKey{Type: "service_name", Value: serviceName}.String()
	records := e.index[key]
	var latencies []float64
	for _, r := range records {
		if span, ok := r.Data.(SpanData); ok {
			latencies = append(latencies, float64(span.DurationNs)/1e6)
		}
	}
	return latencies
}

// updateEndpointStats 更新服务摘要中的端点统计。
// 调用方必须持有 e.mu 写锁。
func (e *CorrelationEngine) updateEndpointStats(summary *ServiceTraceSummary, span *SpanData) {
	endpoint := span.Name
	latencyMs := float64(span.DurationNs) / 1e6
	isError := span.Status == "ERROR"

	found := false
	for i, ep := range summary.TopEndpoints {
		if ep.Endpoint == endpoint {
			// 增量更新平均值
			total := float64(ep.CallCount)
			summary.TopEndpoints[i].AvgLatencyMs = (ep.AvgLatencyMs*total + latencyMs) / (total + 1)
			summary.TopEndpoints[i].CallCount++
			if isError {
				errCount := int(ep.ErrorRate * float64(ep.CallCount-1) / 100.0)
				summary.TopEndpoints[i].ErrorRate = float64(errCount+1) / float64(ep.CallCount) * 100.0
			}
			found = true
			break
		}
	}
	if !found {
		summary.TopEndpoints = append(summary.TopEndpoints, EndpointLatency{
			Endpoint:     endpoint,
			AvgLatencyMs: latencyMs,
			ErrorRate:    0,
			CallCount:    1,
		})
		if isError {
			summary.TopEndpoints[len(summary.TopEndpoints)-1].ErrorRate = 100.0
		}
	}

	// 保持 TopEndpoints 按调用次数降序排列，最多保留 20 个
	sort.Slice(summary.TopEndpoints, func(i, j int) bool {
		return summary.TopEndpoints[i].CallCount > summary.TopEndpoints[j].CallCount
	})
	if len(summary.TopEndpoints) > 20 {
		summary.TopEndpoints = summary.TopEndpoints[:20]
	}
}

// ---------------------------------------------------------------------------
// 后台清理
// ---------------------------------------------------------------------------

// cleanupLoop 定期清理过期的关联记录。
func (e *CorrelationEngine) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.cleanup()
		}
	}
}

// cleanup 移除所有已过期的关联记录。
func (e *CorrelationEngine) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now().UnixNano()

	for key, records := range e.index {
		var valid []*CorrelationRecord
		for _, r := range records {
			expiry := r.Timestamp + int64(r.TTL)
			if expiry > now {
				valid = append(valid, r)
			}
		}
		if len(valid) == 0 {
			delete(e.index, key)
		} else {
			e.index[key] = valid
		}
	}
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// percentile 计算给定数据切片的百分位数。
// p 的取值范围为 0-100。
func percentile(data []float64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	index := (p / 100.0) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	frac := index - float64(lower)
	return sorted[lower] + frac*(sorted[upper]-sorted[lower])
}

// dedupStrings 对字符串切片去重，保持原始顺序。
func dedupStrings(items []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, item := range items {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}
