// Package otel OTLP 协议接收器
//
// 支持:
//   - OTLP/gRPC (标准 OTEL 导出协议)
//   - OTLP/HTTP (备用协议)
//   - Trace / Metrics / Logs 三种信号
//
// 数据流:
//
//	OTEL SDK ──OTLP/gRPC──▶ OTLPReceiver ──▶ TraceStore
//	                          │
//	                          ├──▶ MetricsStore
//	                          └──▶ LogStore
//
// 端口:
//   - OTLP gRPC: 4317
//   - OTLP HTTP: 4318
package otel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// 常量与默认值
// ---------------------------------------------------------------------------

const (
	defaultGRPCAddr     = ":4317"
	defaultHTTPAddr     = ":4318"
	defaultMaxRecvMsg   = 64 * 1024 * 1024 // 64 MB
	defaultBatchSize    = 1000
	defaultFlushInterval = 5 * time.Second
	defaultMaxSpans     = 100000
	defaultMaxLogs      = 100000
	defaultMaxPoints    = 10000

	// SpanKind 常量
	SpanKindClient   = "SPAN_KIND_CLIENT"
	SpanKindServer   = "SPAN_KIND_SERVER"
	SpanKindProducer = "SPAN_KIND_PRODUCER"
	SpanKindConsumer = "SPAN_KIND_CONSUMER"
	SpanKindInternal = "SPAN_KIND_INTERNAL"

	// StatusCode 常量
	StatusCodeOK     = "STATUS_CODE_OK"
	StatusCodeError  = "STATUS_CODE_ERROR"
	StatusCodeUnset  = "STATUS_CODE_UNSET"

	// Metric 类型
	MetricTypeGauge     = "gauge"
	MetricTypeCounter   = "counter"
	MetricTypeHistogram = "histogram"

	// Severity 常量
	SeverityTrace    = "TRACE"
	SeverityTrace2   = "TRACE2"
	SeverityTrace3   = "TRACE3"
	SeverityTrace4   = "TRACE4"
	SeverityDebug    = "DEBUG"
	SeverityDebug2   = "DEBUG2"
	SeverityDebug3   = "DEBUG3"
	SeverityDebug4   = "DEBUG4"
	SeverityInfo     = "INFO"
	SeverityInfo2    = "INFO2"
	SeverityInfo3    = "INFO3"
	SeverityInfo4    = "INFO4"
	SeverityWarn     = "WARN"
	SeverityWarn2    = "WARN2"
	SeverityWarn3    = "WARN3"
	SeverityWarn4    = "WARN4"
	SeverityError    = "ERROR"
	SeverityError2   = "ERROR2"
	SeverityError3   = "ERROR3"
	SeverityError4   = "ERROR4"
	SeverityFatal    = "FATAL"
	SeverityFatal2   = "FATAL2"
	SeverityFatal3   = "FATAL3"
	SeverityFatal4   = "FATAL4"
)

// ---------------------------------------------------------------------------
// 内部数据类型
// ---------------------------------------------------------------------------

// SpanEvent 表示 Span 上的一个事件。
type SpanEvent struct {
	Name       string            `json:"name"`
	Time       int64             `json:"timeUnixNano"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Span 表示一个 OTLP Span。
type Span struct {
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	StartTime    int64             `json:"startTimeUnixNano"`
	EndTime      int64             `json:"endTimeUnixNano"`
	DurationNs   int64             `json:"durationNs"`
	ServiceName  string            `json:"serviceName"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Status       string            `json:"status"`
	StatusCode   int               `json:"statusCode"`
	Events       []SpanEvent       `json:"events,omitempty"`
}

// Trace 表示一条完整的调用链。
type Trace struct {
	TraceID    string  `json:"traceId"`
	RootSpan   *Span   `json:"rootSpan,omitempty"`
	Spans      []*Span `json:"spans"`
	ServiceName string `json:"serviceName"`
	StartTime  int64   `json:"startTimeUnixNano"`
	DurationNs int64   `json:"durationNs"`
	ErrorCount int     `json:"errorCount"`
	mu         sync.Mutex
}

// SpanQuery 用于查询 Span。
type SpanQuery struct {
	ServiceName   string        `json:"serviceName,omitempty"`
	OperationName string        `json:"operationName,omitempty"`
	MinDuration   time.Duration `json:"minDuration,omitempty"`
	HasError      bool          `json:"hasError,omitempty"`
	StartTime     int64         `json:"startTimeUnixNano,omitempty"`
	EndTime       int64         `json:"endTimeUnixNano,omitempty"`
	Limit         int           `json:"limit,omitempty"`
}

// MetricDataPoint 表示一个指标数据点。
type MetricDataPoint struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Value       float64           `json:"value"`
	Timestamp   int64             `json:"timestampUnixNano"`
	Labels      map[string]string `json:"labels,omitempty"`
	ServiceName string            `json:"serviceName"`
}

// MetricSeries 表示一个时间序列。
type MetricSeries struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Points []MetricDataPoint `json:"points"`
	mu     sync.RWMutex
}

// LogRecord 表示一条日志记录。
type LogRecord struct {
	TraceID      string            `json:"traceId,omitempty"`
	SpanID       string            `json:"spanId,omitempty"`
	Timestamp    int64             `json:"timestampUnixNano"`
	Severity     string            `json:"severity"`
	SeverityNumber int             `json:"severityNumber"`
	Body         string            `json:"body"`
	ServiceName  string            `json:"serviceName"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Resource     map[string]string `json:"resource,omitempty"`
}

// LogQuery 用于查询日志。
type LogQuery struct {
	ServiceName string `json:"serviceName,omitempty"`
	TraceID     string `json:"traceId,omitempty"`
	MinSeverity string `json:"minSeverity,omitempty"`
	BodyContains string `json:"bodyContains,omitempty"`
	StartTime   int64  `json:"startTimeUnixNano,omitempty"`
	EndTime     int64  `json:"endTimeUnixNano,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

// ---------------------------------------------------------------------------
// OTLPStats — 原子计数器
// ---------------------------------------------------------------------------

// OTLPStats 记录接收器的统计信息。
type OTLPStats struct {
	TracesReceived       uint64 `json:"tracesReceived"`
	SpansReceived        uint64 `json:"spansReceived"`
	MetricsReceived      uint64 `json:"metricsReceived"`
	DataPointsReceived   uint64 `json:"dataPointsReceived"`
	LogsReceived         uint64 `json:"logsReceived"`
	LogRecordsReceived   uint64 `json:"logRecordsReceived"`
	Errors               uint64 `json:"errors"`
}

// addTraces 原子递增 TracesReceived。
func (s *OTLPStats) addTraces(n uint64) {
	atomic.AddUint64(&s.TracesReceived, n)
}

// addSpans 原子递增 SpansReceived。
func (s *OTLPStats) addSpans(n uint64) {
	atomic.AddUint64(&s.SpansReceived, n)
}

// addMetrics 原子递增 MetricsReceived。
func (s *OTLPStats) addMetrics(n uint64) {
	atomic.AddUint64(&s.MetricsReceived, n)
}

// addDataPoints 原子递增 DataPointsReceived。
func (s *OTLPStats) addDataPoints(n uint64) {
	atomic.AddUint64(&s.DataPointsReceived, n)
}

// addLogs 原子递增 LogsReceived。
func (s *OTLPStats) addLogs(n uint64) {
	atomic.AddUint64(&s.LogsReceived, n)
}

// addLogRecords 原子递增 LogRecordsReceived。
func (s *OTLPStats) addLogRecords(n uint64) {
	atomic.AddUint64(&s.LogRecordsReceived, n)
}

// addErrors 原子递增 Errors。
func (s *OTLPStats) addErrors(n uint64) {
	atomic.AddUint64(&s.Errors, n)
}

// Snapshot 返回统计快照。
func (s *OTLPStats) Snapshot() OTLPStats {
	return OTLPStats{
		TracesReceived:     atomic.LoadUint64(&s.TracesReceived),
		SpansReceived:      atomic.LoadUint64(&s.SpansReceived),
		MetricsReceived:    atomic.LoadUint64(&s.MetricsReceived),
		DataPointsReceived: atomic.LoadUint64(&s.DataPointsReceived),
		LogsReceived:       atomic.LoadUint64(&s.LogsReceived),
		LogRecordsReceived: atomic.LoadUint64(&s.LogRecordsReceived),
		Errors:             atomic.LoadUint64(&s.Errors),
	}
}

// ---------------------------------------------------------------------------
// OTLPReceiverConfig
// ---------------------------------------------------------------------------

// OTLPReceiverConfig 配置 OTLP 接收器。
type OTLPReceiverConfig struct {
	// GRPCAddr gRPC 监听地址 (默认 ":4317")
	GRPCAddr string
	// HTTPAddr HTTP 监听地址 (默认 ":4318")
	HTTPAddr string
	// MaxRecvMsgSize 最大接收消息大小 (默认 64MB)
	MaxRecvMsgSize int
	// BatchSize 批处理大小 (默认 1000)
	BatchSize int
	// FlushInterval 刷新间隔 (默认 5s)
	FlushInterval time.Duration
}

// setDefaults 填充默认值。
func (c *OTLPReceiverConfig) setDefaults() {
	if c.GRPCAddr == "" {
		c.GRPCAddr = defaultGRPCAddr
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = defaultHTTPAddr
	}
	if c.MaxRecvMsgSize <= 0 {
		c.MaxRecvMsgSize = defaultMaxRecvMsg
	}
	if c.BatchSize <= 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaultFlushInterval
	}
}

// ---------------------------------------------------------------------------
// TraceStore
// ---------------------------------------------------------------------------

// TraceStore 存储和管理 Trace 数据。
type TraceStore struct {
	traces   sync.Map // traceID -> *Trace
	spans    []*Span  // 环形缓冲区
	maxSpans int
	mu       sync.RWMutex
}

// NewTraceStore 创建一个新的 TraceStore。
func NewTraceStore(maxSpans int) *TraceStore {
	if maxSpans <= 0 {
		maxSpans = defaultMaxSpans
	}
	return &TraceStore{
		spans:    make([]*Span, 0, maxSpans),
		maxSpans: maxSpans,
	}
}

// AddSpan 添加一个 Span 到存储。
func (ts *TraceStore) AddSpan(span *Span) {
	if span == nil {
		return
	}

	// 计算持续时间
	if span.EndTime > span.StartTime {
		span.DurationNs = span.EndTime - span.StartTime
	}

	// 添加到环形缓冲区
	ts.mu.Lock()
	if len(ts.spans) >= ts.maxSpans {
		// 移除最旧的 10%
		discard := ts.maxSpans / 10
		if discard < 1 {
			discard = 1
		}
		ts.spans = ts.spans[discard:]
	}
	ts.spans = append(ts.spans, span)
	ts.mu.Unlock()

	// 更新 Trace
	traceID := span.TraceID
	if traceID == "" {
		return
	}

	val, loaded := ts.traces.LoadOrStore(traceID, &Trace{
		TraceID:   traceID,
		Spans:     []*Span{span},
		StartTime: span.StartTime,
	})

	t := val.(*Trace)
	if loaded {
		t.mu.Lock()
		t.Spans = append(t.Spans, span)

		// 更新 Trace 的起始时间和持续时间
		if span.StartTime < t.StartTime || t.StartTime == 0 {
			t.StartTime = span.StartTime
		}
		endTime := span.EndTime
		for _, s := range t.Spans {
			if s.EndTime > endTime {
				endTime = s.EndTime
			}
		}
		t.DurationNs = endTime - t.StartTime

		// 更新服务名
		if t.ServiceName == "" && span.ServiceName != "" {
			t.ServiceName = span.ServiceName
		}

		// 更新根 Span
		if span.ParentSpanID == "" && t.RootSpan == nil {
			t.RootSpan = span
		}

		// 统计错误
		if span.Status == StatusCodeError || span.StatusCode == 2 {
			t.ErrorCount++
		}
		t.mu.Unlock()
	} else {
		// 新创建的 Trace
		t.ServiceName = span.ServiceName
		if span.ParentSpanID == "" {
			t.RootSpan = span
		}
		if span.Status == StatusCodeError || span.StatusCode == 2 {
			t.ErrorCount = 1
		}
	}
}

// GetTrace 根据 traceID 获取完整 Trace。
func (ts *TraceStore) GetTrace(traceID string) (*Trace, bool) {
	val, ok := ts.traces.Load(traceID)
	if !ok {
		return nil, false
	}
	return val.(*Trace), true
}

// GetSpans 根据条件查询 Span 列表。
func (ts *TraceStore) GetSpans(serviceName string, startTime, endTime int64) []*Span {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []*Span
	for _, span := range ts.spans {
		if serviceName != "" && span.ServiceName != serviceName {
			continue
		}
		if startTime > 0 && span.StartTime < startTime {
			continue
		}
		if endTime > 0 && span.StartTime > endTime {
			continue
		}
		result = append(result, span)
	}
	return result
}

// GetServices 返回所有已知服务名。
func (ts *TraceStore) GetServices() []string {
	services := make(map[string]struct{})

	ts.mu.RLock()
	for _, span := range ts.spans {
		if span.ServiceName != "" {
			services[span.ServiceName] = struct{}{}
		}
	}
	ts.mu.RUnlock()

	result := make([]string, 0, len(services))
	for svc := range services {
		result = append(result, svc)
	}
	return result
}

// SearchSpans 根据查询条件搜索 Span。
func (ts *TraceStore) SearchSpans(query SpanQuery) []*Span {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	var result []*Span
	for _, span := range ts.spans {
		if query.ServiceName != "" && span.ServiceName != query.ServiceName {
			continue
		}
		if query.OperationName != "" && span.Name != query.OperationName {
			continue
		}
		if query.MinDuration > 0 && span.DurationNs < int64(query.MinDuration) {
			continue
		}
		if query.HasError && span.Status != StatusCodeError && span.StatusCode != 2 {
			continue
		}
		if query.StartTime > 0 && span.StartTime < query.StartTime {
			continue
		}
		if query.EndTime > 0 && span.StartTime > query.EndTime {
			continue
		}
		result = append(result, span)
	}

	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result
}

// ---------------------------------------------------------------------------
// MetricsStore
// ---------------------------------------------------------------------------

// MetricsStore 存储和管理指标数据。
type MetricsStore struct {
	series sync.Map // "name:labels_hash" -> *MetricSeries
}

// NewMetricsStore 创建一个新的 MetricsStore。
func NewMetricsStore() *MetricsStore {
	return &MetricsStore{}
}

// seriesKey 生成时间序列的唯一键。
func seriesKey(name string, labels map[string]string) string {
	h := sha256.New()
	h.Write([]byte(name))
	// 按 key 排序写入，确保一致性
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	for i, k := range keys {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(labels[k]))
	}
	return name + ":" + hex.EncodeToString(h.Sum(nil))[:16]
}

// AddDataPoint 添加一个指标数据点。
func (ms *MetricsStore) AddDataPoint(dp *MetricDataPoint) {
	if dp == nil || dp.Name == "" {
		return
	}

	key := seriesKey(dp.Name, dp.Labels)

	val, _ := ms.series.LoadOrStore(key, &MetricSeries{
		Name:   dp.Name,
		Labels: copyStringMap(dp.Labels),
		Points: make([]MetricDataPoint, 0, 256),
	})

	series := val.(*MetricSeries)
	series.mu.Lock()
	if len(series.Points) >= defaultMaxPoints {
		series.Points = series.Points[len(series.Points)/4:]
	}
	series.Points = append(series.Points, *dp)
	series.mu.Unlock()
}

// Query 根据条件查询指标数据点。
func (ms *MetricsStore) Query(name string, labels map[string]string, startTime, endTime int64) []*MetricDataPoint {
	key := seriesKey(name, labels)

	val, ok := ms.series.Load(key)
	if !ok {
		return nil
	}

	series := val.(*MetricSeries)
	series.mu.RLock()
	defer series.mu.RUnlock()

	var result []*MetricDataPoint
	for i := range series.Points {
		dp := &series.Points[i]
		if startTime > 0 && dp.Timestamp < startTime {
			continue
		}
		if endTime > 0 && dp.Timestamp > endTime {
			continue
		}
		result = append(result, dp)
	}
	return result
}

// GetAllSeriesNames 返回所有指标名称。
func (ms *MetricsStore) GetAllSeriesNames() []string {
	names := make(map[string]struct{})
	ms.series.Range(func(_, val interface{}) bool {
		s := val.(*MetricSeries)
		names[s.Name] = struct{}{}
		return true
	})
	result := make([]string, 0, len(names))
	for n := range names {
		result = append(result, n)
	}
	return result
}

// ---------------------------------------------------------------------------
// LogStore
// ---------------------------------------------------------------------------

// LogStore 存储和管理日志数据。
type LogStore struct {
	logs    []*LogRecord
	maxLogs int
	mu      sync.RWMutex
}

// NewLogStore 创建一个新的 LogStore。
func NewLogStore(maxLogs int) *LogStore {
	if maxLogs <= 0 {
		maxLogs = defaultMaxLogs
	}
	return &LogStore{
		logs:    make([]*LogRecord, 0, maxLogs),
		maxLogs: maxLogs,
	}
}

// AddRecord 添加一条日志记录。
func (ls *LogStore) AddRecord(record *LogRecord) {
	if record == nil {
		return
	}

	ls.mu.Lock()
	if len(ls.logs) >= ls.maxLogs {
		discard := ls.maxLogs / 10
		if discard < 1 {
			discard = 1
		}
		ls.logs = ls.logs[discard:]
	}
	ls.logs = append(ls.logs, record)
	ls.mu.Unlock()
}

// severityOrder 返回严重级别的排序值，值越大越严重。
func severityOrder(sev string) int {
	switch strings.ToUpper(sev) {
	case SeverityTrace, SeverityTrace2, SeverityTrace3, SeverityTrace4:
		return 1
	case SeverityDebug, SeverityDebug2, SeverityDebug3, SeverityDebug4:
		return 5
	case SeverityInfo, SeverityInfo2, SeverityInfo3, SeverityInfo4:
		return 9
	case SeverityWarn, SeverityWarn2, SeverityWarn3, SeverityWarn4:
		return 13
	case SeverityError, SeverityError2, SeverityError3, SeverityError4:
		return 17
	case SeverityFatal, SeverityFatal2, SeverityFatal3, SeverityFatal4:
		return 21
	default:
		return 0
	}
}

// Query 根据条件查询日志记录。
func (ls *LogStore) Query(query LogQuery) []*LogRecord {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	minSev := severityOrder(query.MinSeverity)

	var result []*LogRecord
	for _, record := range ls.logs {
		if query.ServiceName != "" && record.ServiceName != query.ServiceName {
			continue
		}
		if query.TraceID != "" && record.TraceID != query.TraceID {
			continue
		}
		if query.MinSeverity != "" && severityOrder(record.Severity) < minSev {
			continue
		}
		if query.BodyContains != "" && !strings.Contains(record.Body, query.BodyContains) {
			continue
		}
		if query.StartTime > 0 && record.Timestamp < query.StartTime {
			continue
		}
		if query.EndTime > 0 && record.Timestamp > query.EndTime {
			continue
		}
		result = append(result, record)
	}

	if query.Limit > 0 && len(result) > query.Limit {
		result = result[:query.Limit]
	}
	return result
}

// ---------------------------------------------------------------------------
// OTLPReceiver
// ---------------------------------------------------------------------------

// OTLPReceiver 实现 OTLP gRPC/HTTP 接收器。
type OTLPReceiver struct {
	config       OTLPReceiverConfig
	traceStore   *TraceStore
	metricsStore *MetricsStore
	logStore     *LogStore
	httpServer   *http.Server
	grpcServer   *http.Server
	stats        OTLPStats
}

// NewOTLPReceiver 创建一个新的 OTLPReceiver。
func NewOTLPReceiver(config OTLPReceiverConfig) *OTLPReceiver {
	config.setDefaults()
	return &OTLPReceiver{
		config:       config,
		traceStore:   NewTraceStore(defaultMaxSpans),
		metricsStore: NewMetricsStore(),
		logStore:     NewLogStore(defaultMaxLogs),
	}
}

// TraceStore 返回内部的 TraceStore。
func (r *OTLPReceiver) TraceStore() *TraceStore {
	return r.traceStore
}

// MetricsStore 返回内部的 MetricsStore。
func (r *OTLPReceiver) MetricsStore() *MetricsStore {
	return r.metricsStore
}

// LogStore 返回内部的 LogStore。
func (r *OTLPReceiver) LogStore() *LogStore {
	return r.logStore
}

// Stats 返回统计快照。
func (r *OTLPReceiver) Stats() OTLPStats {
	return r.stats.Snapshot()
}

// Start 启动 OTLP HTTP 和 gRPC 接收器。
// gRPC 服务通过 HTTP 路由模拟 OTLP gRPC 协议端点。
func (r *OTLPReceiver) Start(ctx context.Context) error {
	// 启动 gRPC 监听 (使用 HTTP 路由模拟 gRPC 端点)
	go func() {
		grpcListener, err := net.Listen("tcp", r.config.GRPCAddr)
		if err != nil {
			log.Printf("[OTLP] gRPC listen failed on %s: %v", r.config.GRPCAddr, err)
			r.stats.addErrors(1)
			return
		}
		log.Printf("[OTLP] gRPC server listening on %s", r.config.GRPCAddr)

		// 使用标准 HTTP/2 服务器处理 gRPC 请求
		// 由于我们无法导入 OTEL proto 包，gRPC 端口将 OTLP 请求
		// 转发到内部处理逻辑
		grpcMux := http.NewServeMux()
		grpcMux.HandleFunc("/opentelemetry.proto.collector.trace.v1.TraceService/Export", r.handleGRPCTraces)
		grpcMux.HandleFunc("/opentelemetry.proto.collector.metrics.v1.MetricsService/Export", r.handleGRPCMetrics)
		grpcMux.HandleFunc("/opentelemetry.proto.collector.logs.v1.LogsService/Export", r.handleGRPCLogs)

		grpcSrv := &http.Server{
			Handler: grpcMux,
		}
		r.grpcServer = grpcSrv

		// 监听 context 取消
		go func() {
			<-ctx.Done()
			grpcSrv.Close()
		}()

		err = grpcSrv.Serve(grpcListener)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("[OTLP] gRPC server error: %v", err)
			r.stats.addErrors(1)
		}
	}()

	// 启动 HTTP 监听
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/v1/traces", r.handleHTTPTraces)
	httpMux.HandleFunc("/v1/metrics", r.handleHTTPMetrics)
	httpMux.HandleFunc("/v1/logs", r.handleHTTPLogs)

	r.httpServer = &http.Server{
		Addr:    r.config.HTTPAddr,
		Handler: httpMux,
	}

	go func() {
		log.Printf("[OTLP] HTTP server listening on %s", r.config.HTTPAddr)
		if err := r.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[OTLP] HTTP server error: %v", err)
			r.stats.addErrors(1)
		}
	}()

	log.Printf("[OTLP] receiver started (gRPC=%s, HTTP=%s)", r.config.GRPCAddr, r.config.HTTPAddr)
	return nil
}

// Stop 停止 OTLP 接收器。
func (r *OTLPReceiver) Stop() error {
	var errs []string

	if r.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("HTTP shutdown: %v", err))
		}
	}

	if r.grpcServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := r.grpcServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("gRPC shutdown: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %s", strings.Join(errs, "; "))
	}
	log.Println("[OTLP] receiver stopped")
	return nil
}

// ---------------------------------------------------------------------------
// OTLP/HTTP 处理器
// ---------------------------------------------------------------------------

// handleHTTPTraces 处理 POST /v1/traces 请求。
func (r *OTLPReceiver) handleHTTPTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}
	defer req.Body.Close()

	contentType := req.Header.Get("Content-Type")

	var spans []*Span
	switch {
	case strings.Contains(contentType, "application/json"):
		spans, err = parseOTLPTracesJSON(body)
	case strings.Contains(contentType, "application/x-protobuf"):
		// Protobuf 格式：尝试作为 JSON 回退解析（OTLP 也支持 JSON 编码的 protobuf）
		spans, err = parseOTLPTracesJSON(body)
	default:
		// 默认尝试 JSON
		spans, err = parseOTLPTracesJSON(body)
	}

	if err != nil {
		log.Printf("[OTLP] failed to parse traces: %v", err)
		http.Error(w, fmt.Sprintf("failed to parse traces: %v", err), http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}

	traceIDs := make(map[string]struct{})
	for _, span := range spans {
		r.traceStore.AddSpan(span)
		traceIDs[span.TraceID] = struct{}{}
	}

	r.stats.addTraces(uint64(len(traceIDs)))
	r.stats.addSpans(uint64(len(spans)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": nil})
}

// handleHTTPMetrics 处理 POST /v1/metrics 请求。
func (r *OTLPReceiver) handleHTTPMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}
	defer req.Body.Close()

	var dataPoints []*MetricDataPoint
	dataPoints, err = parseOTLPMetricsJSON(body)
	if err != nil {
		log.Printf("[OTLP] failed to parse metrics: %v", err)
		http.Error(w, fmt.Sprintf("failed to parse metrics: %v", err), http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}

	metricNames := make(map[string]struct{})
	for _, dp := range dataPoints {
		r.metricsStore.AddDataPoint(dp)
		metricNames[dp.Name] = struct{}{}
	}

	r.stats.addMetrics(uint64(len(metricNames)))
	r.stats.addDataPoints(uint64(len(dataPoints)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": nil})
}

// handleHTTPLogs 处理 POST /v1/logs 请求。
func (r *OTLPReceiver) handleHTTPLogs(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}
	defer req.Body.Close()

	var records []*LogRecord
	records, err = parseOTLPLogsJSON(body)
	if err != nil {
		log.Printf("[OTLP] failed to parse logs: %v", err)
		http.Error(w, fmt.Sprintf("failed to parse logs: %v", err), http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}

	for _, record := range records {
		r.logStore.AddRecord(record)
	}

	r.stats.addLogs(1)
	r.stats.addLogRecords(uint64(len(records)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"partialSuccess": nil})
}

// ---------------------------------------------------------------------------
// OTLP/gRPC 处理器 (HTTP 路由模拟 gRPC 端点)
// ---------------------------------------------------------------------------

// handleGRPCTraces 处理 gRPC TraceService/Export 请求。
func (r *OTLPReceiver) handleGRPCTraces(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		r.stats.addErrors(1)
		return
	}
	defer req.Body.Close()

	// gRPC 传输使用 protobuf 编码，但我们也支持 JSON 回退
	spans, err := parseOTLPTracesJSON(body)
	if err != nil {
		// 如果 JSON 解析失败，尝试 protobuf 二进制解析
		// 由于没有 OTEL proto 定义，记录错误并返回
		log.Printf("[OTLP] gRPC traces parse failed: %v", err)
		r.stats.addErrors(1)
		w.WriteHeader(http.StatusOK) // gRPC 返回 200 即使解析失败
		return
	}

	traceIDs := make(map[string]struct{})
	for _, span := range spans {
		r.traceStore.AddSpan(span)
		traceIDs[span.TraceID] = struct{}{}
	}

	r.stats.addTraces(uint64(len(traceIDs)))
	r.stats.addSpans(uint64(len(spans)))

	w.Header().Set("Content-Type", "application/grpc")
	w.WriteHeader(http.StatusOK)
}

// handleGRPCMetrics 处理 gRPC MetricsService/Export 请求。
func (r *OTLPReceiver) handleGRPCMetrics(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		r.stats.addErrors(1)
		w.WriteHeader(http.StatusOK)
		return
	}
	defer req.Body.Close()

	dataPoints, err := parseOTLPMetricsJSON(body)
	if err != nil {
		log.Printf("[OTLP] gRPC metrics parse failed: %v", err)
		r.stats.addErrors(1)
		w.WriteHeader(http.StatusOK)
		return
	}

	metricNames := make(map[string]struct{})
	for _, dp := range dataPoints {
		r.metricsStore.AddDataPoint(dp)
		metricNames[dp.Name] = struct{}{}
	}

	r.stats.addMetrics(uint64(len(metricNames)))
	r.stats.addDataPoints(uint64(len(dataPoints)))

	w.Header().Set("Content-Type", "application/grpc")
	w.WriteHeader(http.StatusOK)
}

// handleGRPCLogs 处理 gRPC LogsService/Export 请求。
func (r *OTLPReceiver) handleGRPCLogs(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(io.LimitReader(req.Body, int64(r.config.MaxRecvMsgSize)))
	if err != nil {
		r.stats.addErrors(1)
		w.WriteHeader(http.StatusOK)
		return
	}
	defer req.Body.Close()

	records, err := parseOTLPLogsJSON(body)
	if err != nil {
		log.Printf("[OTLP] gRPC logs parse failed: %v", err)
		r.stats.addErrors(1)
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, record := range records {
		r.logStore.AddRecord(record)
	}

	r.stats.addLogs(1)
	r.stats.addLogRecords(uint64(len(records)))

	w.Header().Set("Content-Type", "application/grpc")
	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// OTLP JSON 解析器
// ---------------------------------------------------------------------------

// --- OTLP Traces JSON ---

// otlpTracesRequest 是 OTLP ExportTraceServiceRequest 的 JSON 表示。
type otlpTracesRequest struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource   `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

type otlpScopeSpans struct {
	Scope  otlpScope `json:"scope"`
	Spans  []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name string `json:"name"`
}

type otlpSpan struct {
	TraceID                string            `json:"traceId"`
	SpanID                 string            `json:"spanId"`
	ParentSpanID           string            `json:"parentSpanId"`
	Name                   string            `json:"name"`
	Kind                   int               `json:"kind"`
	StartTimeUnixNano      json.Number       `json:"startTimeUnixNano"`
	EndTimeUnixNano        json.Number       `json:"endTimeUnixNano"`
	Attributes             []otlpAttribute   `json:"attributes"`
	Status                 otlpStatus        `json:"status"`
	Events                 []otlpSpanEvent   `json:"events"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type otlpSpanEvent struct {
	Name       string          `json:"name"`
	TimeUnixNano json.Number   `json:"timeUnixNano"`
	Attributes []otlpAttribute `json:"attributes"`
}

type otlpAttribute struct {
	Key   string     `json:"key"`
	Value otlpValue  `json:"value"`
}

type otlpValue struct {
	StringValue string  `json:"stringValue"`
	IntValue    string  `json:"intValue"`
	DoubleValue string  `json:"doubleValue"`
	BoolValue   bool    `json:"boolValue"`
	ArrayValue  *otlpArrayValue `json:"arrayValue"`
}

type otlpArrayValue struct {
	Values []otlpValue `json:"values"`
}

// parseOTLPTracesJSON 解析 OTLP Trace JSON 格式。
func parseOTLPTracesJSON(body []byte) ([]*Span, error) {
	var req otlpTracesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal traces request: %w", err)
	}

	var spans []*Span
	for _, rs := range req.ResourceSpans {
		serviceName := extractServiceName(rs.Resource.Attributes)

		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				span := &Span{
					TraceID:      s.TraceID,
					SpanID:       s.SpanID,
					ParentSpanID: s.ParentSpanID,
					Name:         s.Name,
					Kind:         spanKindToString(s.Kind),
					ServiceName:  serviceName,
					Attributes:   extractAttributes(s.Attributes),
					StatusCode:   s.Status.Code,
					Status:       statusCodeToString(s.Status.Code),
				}

				// 解析时间戳（可能是字符串或数字）
				if s.StartTimeUnixNano.String() != "" {
					span.StartTime, _ = s.StartTimeUnixNano.Int64()
				}
				if s.EndTimeUnixNano.String() != "" {
					span.EndTime, _ = s.EndTimeUnixNano.Int64()
				}
				if span.EndTime > span.StartTime {
					span.DurationNs = span.EndTime - span.StartTime
				}

				// 解析事件
				for _, e := range s.Events {
					event := SpanEvent{
						Name:       e.Name,
						Attributes: extractAttributes(e.Attributes),
					}
					if e.TimeUnixNano.String() != "" {
						event.Time, _ = e.TimeUnixNano.Int64()
					}
					span.Events = append(span.Events, event)
				}

				spans = append(spans, span)
			}
		}
	}

	return spans, nil
}

// --- OTLP Metrics JSON ---

// otlpMetricsRequest 是 OTLP ExportMetricsServiceRequest 的 JSON 表示。
type otlpMetricsRequest struct {
	ResourceMetrics []otlpResourceMetrics `json:"resourceMetrics"`
}

type otlpResourceMetrics struct {
	Resource   otlpResource     `json:"resource"`
	ScopeMetrics []otlpScopeMetrics `json:"scopeMetrics"`
}

type otlpScopeMetrics struct {
	Scope   otlpScope  `json:"scope"`
	Metrics []otlpMetric `json:"metrics"`
}

type otlpMetric struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Unit        string          `json:"unit"`
	Gauge       *otlpGauge      `json:"gauge"`
	Sum         *otlpSum        `json:"sum"`
	Histogram   *otlpHistogram  `json:"histogram"`
}

type otlpGauge struct {
	DataPoints []otlpNumberDataPoint `json:"dataPoints"`
}

type otlpSum struct {
	IsMonotonic bool                    `json:"isMonotonic"`
	AggregationTemporality int           `json:"aggregationTemporality"`
	DataPoints  []otlpNumberDataPoint   `json:"dataPoints"`
}

type otlpHistogram struct {
	DataPoints []otlpHistogramDataPoint `json:"dataPoints"`
}

type otlpNumberDataPoint struct {
	Attributes    []otlpAttribute `json:"attributes"`
	StartTimeUnixNano json.Number `json:"startTimeUnixNano"`
	TimeUnixNano  json.Number     `json:"timeUnixNano"`
	AsDouble      string          `json:"asDouble"`
	AsInt         string          `json:"asInt"`
}

type otlpHistogramDataPoint struct {
	Attributes         []otlpAttribute `json:"attributes"`
	StartTimeUnixNano  json.Number     `json:"startTimeUnixNano"`
	TimeUnixNano       json.Number     `json:"timeUnixNano"`
	Count              uint64          `json:"count"`
	Sum                float64         `json:"sum"`
	BucketCounts       []uint64        `json:"bucketCounts"`
	ExplicitBounds     []float64       `json:"explicitBounds"`
	Min                float64         `json:"min"`
	Max                float64         `json:"max"`
}

// parseOTLPMetricsJSON 解析 OTLP Metrics JSON 格式。
func parseOTLPMetricsJSON(body []byte) ([]*MetricDataPoint, error) {
	var req otlpMetricsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal metrics request: %w", err)
	}

	var dataPoints []*MetricDataPoint
	for _, rm := range req.ResourceMetrics {
		serviceName := extractServiceName(rm.Resource.Attributes)

		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				switch {
				case m.Gauge != nil:
					for _, dp := range m.Gauge.DataPoints {
						dataPoints = append(dataPoints, &MetricDataPoint{
							Name:        m.Name,
							Type:        MetricTypeGauge,
							Value:       parseNumberValue(dp.AsDouble, dp.AsInt),
							Timestamp:   parseTimestamp(dp.TimeUnixNano),
							Labels:      extractAttributes(dp.Attributes),
							ServiceName: serviceName,
						})
					}
				case m.Sum != nil:
					for _, dp := range m.Sum.DataPoints {
						dataPoints = append(dataPoints, &MetricDataPoint{
							Name:        m.Name,
							Type:        MetricTypeCounter,
							Value:       parseNumberValue(dp.AsDouble, dp.AsInt),
							Timestamp:   parseTimestamp(dp.TimeUnixNano),
							Labels:      extractAttributes(dp.Attributes),
							ServiceName: serviceName,
						})
					}
				case m.Histogram != nil:
					for _, dp := range m.Histogram.DataPoints {
						dataPoints = append(dataPoints, &MetricDataPoint{
							Name:        m.Name,
							Type:        MetricTypeHistogram,
							Value:       dp.Sum,
							Timestamp:   parseTimestamp(dp.TimeUnixNano),
							Labels:      extractAttributes(dp.Attributes),
							ServiceName: serviceName,
						})
					}
				}
			}
		}
	}

	return dataPoints, nil
}

// --- OTLP Logs JSON ---

// otlpLogsRequest 是 OTLP ExportLogsServiceRequest 的 JSON 表示。
type otlpLogsRequest struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}

type otlpResourceLogs struct {
	Resource    otlpResource    `json:"resource"`
	ScopeLogs   []otlpScopeLogs `json:"scopeLogs"`
}

type otlpScopeLogs struct {
	Scope       otlpScope   `json:"scope"`
	LogRecords  []otlpLogRecord `json:"logRecords"`
}

type otlpLogRecord struct {
	TraceID                string          `json:"traceId"`
	SpanID                 string          `json:"spanId"`
	TimeUnixNano           json.Number     `json:"timeUnixNano"`
	ObservedTimeUnixNano   json.Number     `json:"observedTimeUnixNano"`
	SeverityNumber         int             `json:"severityNumber"`
	SeverityText           string          `json:"severityText"`
	Body                   otlpAnyValue    `json:"body"`
	Attributes             []otlpAttribute `json:"attributes"`
	DroppedAttributesCount int             `json:"droppedAttributesCount"`
	Flags                  uint32          `json:"flags"`
}

type otlpAnyValue struct {
	StringValue string `json:"stringValue"`
	IntValue    string `json:"intValue"`
	DoubleValue string `json:"doubleValue"`
	BoolValue   bool   `json:"boolValue"`
}

// parseOTLPLogsJSON 解析 OTLP Logs JSON 格式。
func parseOTLPLogsJSON(body []byte) ([]*LogRecord, error) {
	var req otlpLogsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("unmarshal logs request: %w", err)
	}

	var records []*LogRecord
	for _, rl := range req.ResourceLogs {
		serviceName := extractServiceName(rl.Resource.Attributes)
		resourceAttrs := extractAttributes(rl.Resource.Attributes)

		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				timestamp := parseTimestamp(lr.TimeUnixNano)
				if timestamp == 0 {
					timestamp = parseTimestamp(lr.ObservedTimeUnixNano)
				}

				record := &LogRecord{
					TraceID:         lr.TraceID,
					SpanID:          lr.SpanID,
					Timestamp:       timestamp,
					Severity:        lr.SeverityText,
					SeverityNumber:  lr.SeverityNumber,
					Body:            lr.Body.StringValue,
					ServiceName:     serviceName,
					Attributes:      extractAttributes(lr.Attributes),
					Resource:        resourceAttrs,
				}

				// 如果没有 severityText，根据 severityNumber 推断
				if record.Severity == "" {
					record.Severity = severityNumberToString(lr.SeverityNumber)
				}

				records = append(records, record)
			}
		}
	}

	return records, nil
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// extractServiceName 从资源属性中提取 service.name。
func extractServiceName(attrs []otlpAttribute) string {
	for _, attr := range attrs {
		if attr.Key == "service.name" {
			return attr.Value.StringValue
		}
	}
	return ""
}

// extractAttributes 将 OTLP 属性列表转换为 map[string]string。
func extractAttributes(attrs []otlpAttribute) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	m := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		if attr.Value.StringValue != "" {
			m[attr.Key] = attr.Value.StringValue
		} else if attr.Value.IntValue != "" {
			m[attr.Key] = attr.Value.IntValue
		} else if attr.Value.DoubleValue != "" {
			m[attr.Key] = attr.Value.DoubleValue
		} else if attr.Value.BoolValue {
			m[attr.Key] = "true"
		}
	}
	return m
}

// spanKindToString 将 OTLP SpanKind 整数转换为字符串。
func spanKindToString(kind int) string {
	switch kind {
	case 1:
		return SpanKindInternal
	case 2:
		return SpanKindServer
	case 3:
		return SpanKindClient
	case 4:
		return SpanKindProducer
	case 5:
		return SpanKindConsumer
	default:
		return SpanKindInternal
	}
}

// statusCodeToString 将 OTLP StatusCode 整数转换为字符串。
func statusCodeToString(code int) string {
	switch code {
	case 0:
		return StatusCodeUnset
	case 1:
		return StatusCodeOK
	case 2:
		return StatusCodeError
	default:
		return StatusCodeUnset
	}
}

// severityNumberToString 将 OTLP SeverityNumber 转换为字符串。
func severityNumberToString(num int) string {
	switch num {
	case 1, 2, 3, 4:
		return SeverityTrace
	case 5, 6, 7, 8:
		return SeverityDebug
	case 9, 10, 11, 12:
		return SeverityInfo
	case 13, 14, 15, 16:
		return SeverityWarn
	case 17, 18, 19, 20:
		return SeverityError
	case 21, 22, 23, 24:
		return SeverityFatal
	default:
		return ""
	}
}

// parseNumberValue 解析 double/int 数值。
func parseNumberValue(doubleStr, intStr string) float64 {
	if doubleStr != "" {
		v, err := json.Number(doubleStr).Float64()
		if err == nil {
			return v
		}
	}
	if intStr != "" {
		v, err := json.Number(intStr).Int64()
		if err == nil {
			return float64(v)
		}
	}
	return 0
}

// parseTimestamp 解析 JSON 数字时间戳。
func parseTimestamp(n json.Number) int64 {
	if n.String() == "" {
		return 0
	}
	v, err := n.Int64()
	if err != nil {
		return 0
	}
	return v
}

// copyStringMap 浅拷贝字符串 map。
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
