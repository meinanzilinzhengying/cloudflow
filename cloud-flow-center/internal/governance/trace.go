//go:build linux

package governance

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、调用链监控（Span Collector）
// 与 agent trace 模块对接，收集并聚合调用链数据
// ============================================================================

// TraceSpan 调用链 Span（简化版，与 agent trace 结构对齐）
type TraceSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentID     string            `json:"parent_id,omitempty"`
	ServiceName  string            `json:"service_name"`
	Operation    string            `json:"operation"`  // 操作名如 GET /api/users
	StartTime    int64             `json:"start_time"` // 微秒时间戳
	Duration     int64             `json:"duration"`   // 微秒
	Status       int               `json:"status"`     // HTTP 状态码
	Error        bool              `json:"error"`
	Tags         map[string]string `json:"tags,omitempty"`
}

// TraceCollector 调用链收集器
type TraceCollector struct {
	mu         sync.RWMutex
	spans      []*TraceSpan
	traces     map[string][]*TraceSpan // traceID -> spans
	services   map[string]*ServiceStats // serviceName -> stats
	sampleRate float64              // 采样率 0.0-1.0
	maxSpans   int                  // 最大缓存 span 数
	stopCh     chan struct{}
}

// ServiceStats 服务统计
type ServiceStats struct {
	ServiceName     string    `json:"service_name"`
	RequestCount    int64     `json:"request_count"`
	ErrorCount      int64     `json:"error_count"`
	ErrorRate       float64   `json:"error_rate"`
	AvgLatency      int64     `json:"avg_latency"`     // 微秒
	P99Latency      int64     `json:"p99_latency"`     // 微秒
	P95Latency      int64     `json:"p95_latency"`     // 微秒
	P50Latency      int64     `json:"p50_latency"`     // 微秒
	LastUpdated     time.Time `json:"last_updated"`
}

// NewTraceCollector 创建调用链收集器
func NewTraceCollector(sampleRate float64, maxSpans int) *TraceCollector {
	if sampleRate <= 0 || sampleRate > 1 {
		sampleRate = 1.0
	}
	if maxSpans <= 0 {
		maxSpans = 10000
	}
	return &TraceCollector{
		spans:      make([]*TraceSpan, 0, maxSpans),
		traces:     make(map[string][]*TraceSpan),
		services:   make(map[string]*ServiceStats),
		sampleRate: sampleRate,
		maxSpans:   maxSpans,
		stopCh:     make(chan struct{}),
	}
}

// Collect 收集单个 Span
func (tc *TraceCollector) Collect(span *TraceSpan) bool {
	if span == nil {
		return false
	}
	// 采样判断
	if tc.sampleRate < 1.0 && !tc.shouldSample() {
		return false
	}

	if span.TraceID == "" || span.SpanID == "" {
		span.TraceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
		span.SpanID = fmt.Sprintf("span_%d", time.Now().UnixNano())
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 缓存控制
	if len(tc.spans) >= tc.maxSpans {
		tc.spans = tc.spans[tc.maxSpans/10:] // 淘汰前 10%
	}

	tc.spans = append(tc.spans, span)
	tc.traces[span.TraceID] = append(tc.traces[span.TraceID], span)
	tc.updateServiceStats(span)
	return true
}

// CollectBatch 批量收集
func (tc *TraceCollector) CollectBatch(spans []*TraceSpan) int {
	count := 0
	for _, span := range spans {
		if tc.Collect(span) {
			count++
		}
	}
	return count
}

// shouldSample 采样判断
func (tc *TraceCollector) shouldSample() bool {
	return time.Now().UnixNano()%1000 < int64(tc.sampleRate*1000)
}

// updateServiceStats 更新服务统计
func (tc *TraceCollector) updateServiceStats(span *TraceSpan) {
	stats, exists := tc.services[span.ServiceName]
	if !exists {
		stats = &ServiceStats{ServiceName: span.ServiceName}
		tc.services[span.ServiceName] = stats
	}

	stats.RequestCount++
	if span.Error || span.Status >= 500 {
		stats.ErrorCount++
	}
	stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.RequestCount)
	stats.AvgLatency = (stats.AvgLatency*(stats.RequestCount-1) + span.Duration) / stats.RequestCount
	stats.LastUpdated = time.Now()

	// 更新 P50/P95/P99（简化版：用最近 100 个请求）
	recentSpans := tc.getRecentSpans(span.ServiceName, 100)
	latencies := make([]int64, len(recentSpans))
	for i, s := range recentSpans {
		latencies[i] = s.Duration
	}
	stats.P50Latency = percentile(latencies, 0.5)
	stats.P95Latency = percentile(latencies, 0.95)
	stats.P99Latency = percentile(latencies, 0.99)
}

// getRecentSpans 获取最近 n 个 span
func (tc *TraceCollector) getRecentSpans(serviceName string, n int) []*TraceSpan {
	var result []*TraceSpan
	for i := len(tc.spans) - 1; i >= 0 && len(result) < n; i-- {
		if tc.spans[i].ServiceName == serviceName {
			result = append(result, tc.spans[i])
		}
	}
	return result
}

// GetTrace 获取完整调用链
func (tc *TraceCollector) GetTrace(traceID string) []*TraceSpan {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	spans := tc.traces[traceID]
	if len(spans) == 0 {
		return nil
	}
	result := make([]*TraceSpan, len(spans))
	copy(result, spans)
	return result
}

// GetServiceStats 获取服务统计
func (tc *TraceCollector) GetServiceStats(serviceName string) (*ServiceStats, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	stats, ok := tc.services[serviceName]
	if !ok {
		return nil, false
	}
	// 深拷贝
	copy := *stats
	return &copy, true
}

// GetAllServiceStats 获取所有服务统计
func (tc *TraceCollector) GetAllServiceStats() []*ServiceStats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	result := make([]*ServiceStats, 0, len(tc.services))
	for _, stats := range tc.services {
		copy := *stats
		result = append(result, &copy)
	}
	return result
}

// GetServiceDependencyGraph 获取服务依赖图
func (tc *TraceCollector) GetServiceDependencyGraph() map[string][]string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	dependencies := make(map[string][]string)
	for _, spans := range tc.traces {
		for _, span := range spans {
			if span.ParentID != "" {
				// 找到 parent span 确定服务间依赖
				for _, other := range spans {
					if other.SpanID == span.ParentID && other.ServiceName != span.ServiceName {
						if !contains(dependencies[other.ServiceName], span.ServiceName) {
							dependencies[other.ServiceName] = append(dependencies[other.ServiceName], span.ServiceName)
						}
					}
				}
			}
		}
	}
	return dependencies
}

// GetErrorTraces 获取错误调用链
func (tc *TraceCollector) GetErrorTraces(limit int) []string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	var result []string
	for traceID, spans := range tc.traces {
		for _, span := range spans {
			if span.Error || span.Status >= 500 {
				result = append(result, traceID)
				break
			}
		}
		if len(result) >= limit {
			break
		}
	}
	return result
}

// GetSlowTraces 获取慢调用链
func (tc *TraceCollector) GetSlowTraces(threshold int64, limit int) []string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	var result []string
	for traceID, spans := range tc.traces {
		for _, span := range spans {
			if span.Duration > threshold {
				result = append(result, traceID)
				break
			}
		}
		if len(result) >= limit {
			break
		}
	}
	return result
}

// Flush 清空数据
func (tc *TraceCollector) Flush() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.spans = tc.spans[:0]
	tc.traces = make(map[string][]*TraceSpan)
	tc.services = make(map[string]*ServiceStats)
}

// Stats 获取统计
func (tc *TraceCollector) Stats() map[string]interface{} {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	return map[string]interface{}{
		"span_count":      len(tc.spans),
		"trace_count":     len(tc.traces),
		"service_count":   len(tc.services),
		"sample_rate":     tc.sampleRate,
		"max_spans":       tc.maxSpans,
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	// 简单冒泡排序
	sorted := make([]int64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
