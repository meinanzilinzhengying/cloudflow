package trace

import (
	"time"
)

// TraceID 追踪ID (128-bit, 32 hex chars)
type TraceID string

// SpanID Span ID (64-bit, 16 hex chars)
type SpanID string

// SpanContext Span上下文，用于传播
type SpanContext struct {
	TraceID  TraceID `json:"trace_id"`
	SpanID   SpanID  `json:"span_id"`
	ParentID SpanID  `json:"parent_id,omitempty"`
	Sampled  bool    `json:"sampled"`
	Baggage  map[string]string `json:"baggage,omitempty"`
}

// Span 表示一个追踪跨度
type Span struct {
	TraceID   TraceID            `json:"trace_id"`
	SpanID    SpanID             `json:"span_id"`
	ParentID  SpanID             `json:"parent_id,omitempty"`
	Name      string             `json:"name"`          // 操作名称
	Service   string             `json:"service"`       // 服务名
	StartTime time.Time          `json:"start_time"`
	Duration  time.Duration      `json:"duration"`
	Status    SpanStatus         `json:"status"`
	Kind      SpanKind           `json:"kind"`          // client/server/producer/consumer
	Tags      map[string]string  `json:"tags,omitempty"`
	Logs      []SpanLog          `json:"logs,omitempty"`
}

// SpanStatus Span状态
type SpanStatus string

const (
	SpanStatusOK       SpanStatus = "ok"
	SpanStatusError    SpanStatus = "error"
	SpanStatusTimeout  SpanStatus = "timeout"
)

// SpanKind Span类型
type SpanKind string

const (
	SpanKindClient   SpanKind = "client"
	SpanKindServer   SpanKind = "server"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
	SpanKindInternal SpanKind = "internal"
)

// SpanLog Span日志
type SpanLog struct {
	Timestamp time.Time         `json:"timestamp"`
	Fields    map[string]string `json:"fields"`
}

// Trace 完整追踪
type Trace struct {
	TraceID   TraceID  `json:"trace_id"`
	Spans     []Span   `json:"spans"`
	Service   string   `json:"service"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	RootSpan  *Span    `json:"root_span,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// TraceSummary 追踪摘要
type TraceSummary struct {
	TraceID      TraceID        `json:"trace_id"`
	Service      string         `json:"service"`
	Name         string         `json:"name"`
	StartTime    time.Time      `json:"start_time"`
	Duration     time.Duration  `json:"duration"`
	SpanCount    int            `json:"span_count"`
	ErrorCount   int            `json:"error_count"`
	HasError     bool           `json:"has_error"`
	Status       SpanStatus     `json:"status"`
	Services     []string       `json:"services"`       // 涉及的服务列表
}

// TraceQuery 追踪查询条件
type TraceQuery struct {
	TraceID    TraceID         `json:"trace_id,omitempty"`
	Service    string          `json:"service,omitempty"`
	Name       string          `json:"name,omitempty"`
	MinDuration time.Duration  `json:"min_duration,omitempty"`
	MaxDuration time.Duration  `json:"max_duration,omitempty"`
	StartTime  time.Time       `json:"start_time,omitempty"`
	EndTime    time.Time       `json:"end_time,omitempty"`
	Status     SpanStatus      `json:"status,omitempty"`
	Tags       map[string]string `json:"tags,omitempty"`
	Limit      int             `json:"limit,omitempty"`
	Offset     int             `json:"offset,omitempty"`
	OrderBy    string          `json:"order_by,omitempty"` // duration/start_time
	OrderDesc  bool            `json:"order_desc,omitempty"`
}

// TraceQueryResult 追踪查询结果
type TraceQueryResult struct {
	Total    int             `json:"total"`
	Traces   []TraceSummary  `json:"traces"`
}

// SpanQuery Span查询条件
type SpanQuery struct {
	TraceID   TraceID        `json:"trace_id"`
	Service   string         `json:"service,omitempty"`
	Name      string         `json:"name,omitempty"`
	Status    SpanStatus     `json:"status,omitempty"`
	MinDuration time.Duration `json:"min_duration,omitempty"`
}

// SpanQueryResult Span查询结果
type SpanQueryResult struct {
	TraceID  TraceID `json:"trace_id"`
	Spans    []Span  `json:"spans"`
}

// CorrelationType 关联类型
type CorrelationType string

const (
	CorrTypeMetric  CorrelationType = "metric"   // 追踪→指标
	CorrTypeLog     CorrelationType = "log"      // 追踪→日志
	CorrTypeAlert   CorrelationType = "alert"    // 追踪→告警
	CorrTypeTrace   CorrelationType = "trace"    // 追踪→追踪(因果链)
	CorrTypeProfile CorrelationType = "profile"  // 追踪→火焰图
)

// Correlation 关联记录
type Correlation struct {
	TraceID    TraceID        `json:"trace_id"`
	SpanID     SpanID         `json:"span_id,omitempty"`
	Type       CorrelationType `json:"type"`
	TargetID   string         `json:"target_id"`     // 关联目标ID
	TargetType string         `json:"target_type"`   // 目标类型
	Timestamp  time.Time      `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// CorrelationQuery 关联查询
type CorrelationQuery struct {
	TraceID   TraceID        `json:"trace_id"`
	SpanID    SpanID         `json:"span_id,omitempty"`
	Types     []CorrelationType `json:"types,omitempty"`
	StartTime time.Time      `json:"start_time,omitempty"`
	EndTime   time.Time      `json:"end_time,omitempty"`
}

// CorrelationResult 关联查询结果
type CorrelationResult struct {
	TraceID     TraceID       `json:"trace_id"`
	Correlations []Correlation `json:"correlations"`
	Metrics     []MetricLink  `json:"metrics,omitempty"`
	Logs        []LogLink     `json:"logs,omitempty"`
	Alerts      []AlertLink   `json:"alerts,omitempty"`
}

// MetricLink 指标关联
type MetricLink struct {
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
	Timestamp  time.Time `json:"timestamp"`
	Labels     map[string]string `json:"labels"`
}

// LogLink 日志关联
type LogLink struct {
	LogID     string            `json:"log_id"`
	Message   string            `json:"message"`
	Level     string            `json:"level"`
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
}

// AlertLink 告警关联
type AlertLink struct {
	AlertID   string    `json:"alert_id"`
	Name      string    `json:"name"`
	Level     string    `json:"level"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// TraceStats 追踪统计
type TraceStats struct {
	TotalTraces   int64 `json:"total_traces"`
	TotalSpans    int64 `json:"total_spans"`
	ErrorRate     float64 `json:"error_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	P50DurationMs float64 `json:"p50_duration_ms"`
	P99DurationMs float64 `json:"p99_duration_ms"`
	StorageSize   int64 `json:"storage_size"`
}

// GenerateTraceID 生成TraceID (32 hex chars)
func GenerateTraceID() TraceID {
	// 简化实现，生产环境应使用 crypto/rand
	b := make([]byte, 16)
	now := time.Now().UnixNano()
	for i := range b {
		b[i] = byte(now >> (uint(i) * 4))
		if i > 7 {
			b[i] = byte(now >> (uint(i-8) * 4))
		}
	}
	return TraceID(formatHex(b))
}

// GenerateSpanID 生成SpanID (16 hex chars)
func GenerateSpanID() SpanID {
	b := make([]byte, 8)
	now := time.Now().UnixNano()
	for i := range b {
		b[i] = byte(now >> (uint(i) * 8))
	}
	return SpanID(formatHex(b))
}

func formatHex(b []byte) string {
	const hex = "0123456789abcdef"
	s := make([]byte, len(b)*2)
	for i, v := range b {
		s[i*2] = hex[v>>4]
		s[i*2+1] = hex[v&0x0f]
	}
	return string(s)
}
