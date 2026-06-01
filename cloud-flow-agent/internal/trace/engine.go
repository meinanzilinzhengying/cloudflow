package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// TraceEngine 追踪引擎
type TraceEngine struct {
	mu          sync.RWMutex
	store       *TraceStore
	corrStore   *CorrelationStore
	config      *TraceConfig
	propagator  *Propagator
}

// TraceStore 追踪存储
type TraceStore struct {
	mu       sync.RWMutex
	traces   map[TraceID]*Trace          // 按TraceID索引
	spans    map[TraceID][]Span          // 按TraceID→Spans索引
	bySpan   map[SpanID]*Span            // 按SpanID索引
	byService map[string][]TraceSummary  // 按服务索引
	byTime   []TraceSummary              // 按时间索引
	maxSize  int
}

// CorrelationStore 关联存储
type CorrelationStore struct {
	mu           sync.RWMutex
	byTrace      map[TraceID][]Correlation  // 按TraceID索引
	byTarget     map[string][]Correlation   // 按目标ID反向索引
	byType       map[CorrelationType][]Correlation // 按类型索引
	maxSize      int
}

// TraceConfig 追踪配置
type TraceConfig struct {
	Enabled           bool          `yaml:"enabled" json:"enabled"`
	SampleRate        float64       `yaml:"sample_rate" json:"sample_rate"`           // 采样率 0.0-1.0
	MaxTraces         int           `yaml:"max_traces" json:"max_traces"`             // 最大存储追踪数
	MaxSpansPerTrace  int           `yaml:"max_spans_per_trace" json:"max_spans_per_trace"`
	RetentionTime     time.Duration `yaml:"retention_time" json:"retention_time"`     // 保留时间
	EnableCorrelation bool          `yaml:"enable_correlation" json:"enable_correlation"` // 启用关联
	PropagateHeaders  []string      `yaml:"propagate_headers" json:"propagate_headers"` // 传播的HTTP头
	BaggageMaxKeys    int           `yaml:"baggage_max_keys" json:"baggage_max_keys"` // 最大Baggage数
}

// NewTraceEngine 创建追踪引擎
func NewTraceEngine(config *TraceConfig) *TraceEngine {
	if config == nil {
		config = DefaultTraceConfig()
	}
	maxSize := config.MaxTraces
	if maxSize <= 0 {
		maxSize = 10000
	}

	return &TraceEngine{
		store: &TraceStore{
			traces:     make(map[TraceID]*Trace),
			spans:      make(map[TraceID][]Span),
			bySpan:     make(map[SpanID]*Span),
			byService:  make(map[string][]TraceSummary),
			byTime:     make([]TraceSummary, 0, maxSize),
			maxSize:    maxSize,
		},
		corrStore: &CorrelationStore{
			byTrace:  make(map[TraceID][]Correlation),
			byTarget: make(map[string][]Correlation),
			byType:   make(map[CorrelationType][]Correlation),
			maxSize:  maxSize * 5,
		},
		config:     config,
		propagator: NewPropagator(config.PropagateHeaders),
	}
}

// DefaultTraceConfig 默认追踪配置
func DefaultTraceConfig() *TraceConfig {
	return &TraceConfig{
		Enabled:           true,
		SampleRate:        1.0,
		MaxTraces:         10000,
		MaxSpansPerTrace:  1000,
		RetentionTime:     time.Hour * 24,
		EnableCorrelation: true,
		PropagateHeaders:  []string{"x-trace-id", "x-span-id", "x-parent-span-id", "traceparent", "b3"},
		BaggageMaxKeys:    32,
	}
}

// StartSpan 开始一个Span
func (e *TraceEngine) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	// 从上下文提取父SpanContext
	parentCtx := e.propagator.Extract(ctx)

	var traceID TraceID
	var parentID SpanID

	if parentCtx != nil && parentCtx.TraceID != "" {
		traceID = parentCtx.TraceID
		parentID = parentCtx.SpanID
	} else {
		traceID = GenerateTraceID()
	}

	spanID := GenerateSpanID()

	// 采样决策
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

	// 应用选项
	for _, opt := range opts {
		opt(span)
	}

	// 注入到上下文
	spanCtx := &SpanContext{
		TraceID: traceID,
		SpanID:  spanID,
		ParentID: parentID,
		Sampled: sampled,
		Baggage: parentCtx.Baggage,
	}
	ctx = context.WithValue(ctx, traceContextKey{}, spanCtx)

	return ctx, span
}

// FinishSpan 完成一个Span
func (e *TraceEngine) FinishSpan(span *Span) {
	span.Duration = time.Since(span.StartTime)

	if span.Duration <= 0 {
		span.Duration = time.Nanosecond
	}

	e.store.AddSpan(*span)
}

// shouldSample 采样决策
func (e *TraceEngine) shouldSample() bool {
	if e.config.SampleRate >= 1.0 {
		return true
	}
	if e.config.SampleRate <= 0 {
		return false
	}
	// 简化采样: 基于时间纳秒的确定性采样
	return time.Now().UnixNano()%10000 < int64(e.config.SampleRate*10000)
}

// GetTrace 获取完整追踪
func (e *TraceEngine) GetTrace(traceID TraceID) (*Trace, error) {
	return e.store.GetTrace(traceID)
}

// QueryTraces 查询追踪列表
func (e *TraceEngine) QueryTraces(query TraceQuery) (*TraceQueryResult, error) {
	return e.store.Query(query)
}

// GetSpanBySpanID 按SpanID获取Span (反向查询)
func (e *TraceEngine) GetSpanBySpanID(spanID SpanID) (*Span, error) {
	return e.store.GetSpan(spanID)
}

// GetTraceBySpanID 通过SpanID找到所属Trace
func (e *TraceEngine) GetTraceBySpanID(spanID SpanID) (*Trace, error) {
	span, err := e.store.GetSpan(spanID)
	if err != nil {
		return nil, err
	}
	return e.store.GetTrace(span.TraceID)
}

// QuerySpans 查询Span列表
func (e *TraceEngine) QuerySpans(query SpanQuery) (*SpanQueryResult, error) {
	spans, err := e.store.GetSpans(query.TraceID)
	if err != nil {
		return nil, err
	}

	// 过滤
	filtered := make([]Span, 0)
	for _, s := range spans {
		if query.Service != "" && s.Service != query.Service {
			continue
		}
		if query.Name != "" && s.Name != query.Name {
			continue
		}
		if query.Status != "" && s.Status != query.Status {
			continue
		}
		if query.MinDuration > 0 && s.Duration < query.MinDuration {
			continue
		}
		filtered = append(filtered, s)
	}

	return &SpanQueryResult{
		TraceID: query.TraceID,
		Spans:   filtered,
	}, nil
}

// AddCorrelation 添加关联
func (e *TraceEngine) AddCorrelation(corr Correlation) {
	if !e.config.EnableCorrelation {
		return
	}
	e.corrStore.Add(corr)
}

// GetCorrelations 获取关联
func (e *TraceEngine) GetCorrelations(query CorrelationQuery) (*CorrelationResult, error) {
	corrs := e.corrStore.GetByTraceID(query.TraceID)

	// 按类型过滤
	if len(query.Types) > 0 {
		typeSet := make(map[CorrelationType]bool)
		for _, t := range query.Types {
			typeSet[t] = true
		}
		filtered := make([]Correlation, 0)
		for _, c := range corrs {
			if typeSet[c.Type] {
				filtered = append(filtered, c)
			}
		}
		corrs = filtered
	}

	result := &CorrelationResult{
		TraceID:      query.TraceID,
		Correlations: corrs,
	}

	return result, nil
}

// GetCorrelationsByTarget 反向查询: 通过目标ID找到关联的Trace
func (e *TraceEngine) GetCorrelationsByTarget(targetID string) []Correlation {
	return e.corrStore.GetByTargetID(targetID)
}

// InjectHTTP 注入追踪上下文到HTTP请求头
func (e *TraceEngine) InjectHTTP(ctx context.Context, headers http.Header) {
	e.propagator.Inject(ctx, headers)
}

// ExtractHTTP 从HTTP请求头提取追踪上下文
func (e *TraceEngine) ExtractHTTP(r *http.Request) context.Context {
	return e.propagator.ExtractHTTP(r)
}

// GetStats 获取统计信息
func (e *TraceEngine) GetStats() TraceStats {
	return e.store.Stats()
}

// --- TraceStore 实现 ---

// AddSpan 添加Span
func (s *TraceStore) AddSpan(span Span) {
	s.mu.Lock()
	defer s.mu.Unlock()

	traceID := span.TraceID

	// 添加到Spans索引
	s.spans[traceID] = append(s.spans[traceID], span)
	s.bySpan[span.SpanID] = &s.spans[traceID][len(s.spans[traceID])-1]

	// 更新Trace
	trace, exists := s.traces[traceID]
	if !exists {
		trace = &Trace{
			TraceID: traceID,
			Spans:   make([]Span, 0),
			Tags:    make(map[string]string),
		}
		s.traces[traceID] = trace
	}

	trace.Spans = append(trace.Spans, span)
	if span.Service != "" {
		trace.Service = span.Service
	}
	if span.ParentID == "" {
		trace.RootSpan = &span
	}

	// 更新时间
	if trace.StartTime.IsZero() || span.StartTime.Before(trace.StartTime) {
		trace.StartTime = span.StartTime
	}
	endTime := span.StartTime.Add(span.Duration)
	if trace.EndTime.IsZero() || endTime.After(trace.EndTime) {
		trace.EndTime = endTime
	}
	trace.Duration = trace.EndTime.Sub(trace.StartTime)

	// 限制大小
	if len(s.spans[traceID]) > 1000 {
		s.spans[traceID] = s.spans[traceID][len(s.spans[traceID])-1000:]
	}
}

// GetTrace 获取完整追踪
func (s *TraceStore) GetTrace(traceID TraceID) (*Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trace, ok := s.traces[traceID]
	if !ok {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}
	return trace, nil
}

// GetSpan 按SpanID获取
func (s *TraceStore) GetSpan(spanID SpanID) (*Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	span, ok := s.bySpan[spanID]
	if !ok {
		return nil, fmt.Errorf("span not found: %s", spanID)
	}
	return span, nil
}

// GetSpans 获取Trace的所有Span
func (s *TraceStore) GetSpans(traceID TraceID) ([]Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	spans, ok := s.spans[traceID]
	if !ok {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}
	return spans, nil
}

// Query 查询追踪
func (s *TraceStore) Query(query TraceQuery) (*TraceQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]TraceSummary, 0)

	for _, trace := range s.traces {
		// 按TraceID精确查询
		if query.TraceID != "" && trace.TraceID != query.TraceID {
			continue
		}
		// 按服务过滤
		if query.Service != "" && trace.Service != query.Service {
			continue
		}
		// 按时间过滤
		if !query.StartTime.IsZero() && trace.StartTime.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && trace.StartTime.After(query.EndTime) {
			continue
		}
		// 按时长过滤
		if query.MinDuration > 0 && trace.Duration < query.MinDuration {
			continue
		}
		if query.MaxDuration > 0 && trace.Duration > query.MaxDuration {
			continue
		}

		summary := s.buildSummary(trace)
		results = append(results, *summary)
	}

	// 排序
	sort.Slice(results, func(i, j int) bool {
		switch query.OrderBy {
		case "duration":
			if query.OrderDesc {
				return results[i].Duration > results[j].Duration
			}
			return results[i].Duration < results[j].Duration
		default:
			if query.OrderDesc {
				return results[i].StartTime.After(results[j].StartTime)
			}
			return results[i].StartTime.Before(results[j].StartTime)
		}
	})

	// 分页
	total := len(results)
	if query.Offset > 0 {
		if query.Offset >= total {
			return &TraceQueryResult{Total: total}, nil
		}
		results = results[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return &TraceQueryResult{
		Total:  total,
		Traces: results,
	}, nil
}

// buildSummary 构建摘要
func (s *TraceStore) buildSummary(trace *Trace) *TraceSummary {
	summary := &TraceSummary{
		TraceID:   trace.TraceID,
		Service:   trace.Service,
		StartTime: trace.StartTime,
		Duration:  trace.Duration,
		SpanCount: len(trace.Spans),
	}

	if trace.RootSpan != nil {
		summary.Name = trace.RootSpan.Name
	}

	// 收集服务和错误
	services := make(map[string]bool)
	for _, span := range trace.Spans {
		services[span.Service] = true
		if span.Status == SpanStatusError || span.Status == SpanStatusTimeout {
			summary.ErrorCount++
		}
	}
	summary.HasError = summary.ErrorCount > 0
	if summary.HasError {
		summary.Status = SpanStatusError
	}

	for svc := range services {
		summary.Services = append(summary.Services, svc)
	}
	sort.Strings(summary.Services)

	return summary
}

// Stats 统计信息
func (s *TraceStore) Stats() TraceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalSpans int64
	var totalDuration float64
	var errorCount int64
	durations := make([]float64, 0, len(s.traces))

	for _, trace := range s.traces {
		spanCount := len(trace.Spans)
		totalSpans += int64(spanCount)
		totalDuration += trace.Duration.Seconds() * 1000
		durations = append(durations, trace.Duration.Seconds()*1000)

		for _, span := range trace.Spans {
			if span.Status == SpanStatusError || span.Status == SpanStatusTimeout {
				errorCount++
			}
		}
	}

	stats := TraceStats{
		TotalTraces: int64(len(s.traces)),
		TotalSpans:  totalSpans,
	}

	if totalSpans > 0 {
		stats.ErrorRate = float64(errorCount) / float64(totalSpans) * 100
	}
	if len(durations) > 0 {
		sort.Float64s(durations)
		stats.AvgDurationMs = totalDuration / float64(len(durations))
		stats.P50DurationMs = percentile(durations, 0.50)
		stats.P99DurationMs = percentile(durations, 0.99)
	}

	return stats
}

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

// --- CorrelationStore 实现 ---

// Add 添加关联
func (s *CorrelationStore) Add(corr Correlation) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byTrace[corr.TraceID] = append(s.byTrace[corr.TraceID], corr)
	s.byTarget[corr.TargetID] = append(s.byTarget[corr.TargetID], corr)
	s.byType[corr.Type] = append(s.byType[corr.Type], corr)
}

// GetByTraceID 按TraceID获取关联
func (s *CorrelationStore) GetByTraceID(traceID TraceID) []Correlation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byTrace[traceID]
}

// GetByTargetID 按目标ID反向查询
func (s *CorrelationStore) GetByTargetID(targetID string) []Correlation {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byTarget[targetID]
}

// --- Propagator ---

type traceContextKey struct{}

// Propagator 追踪上下文传播器
type Propagator struct {
	headers []string
}

// NewPropagator 创建传播器
func NewPropagator(headers []string) *Propagator {
	if len(headers) == 0 {
		headers = []string{"x-trace-id", "x-span-id", "x-parent-span-id"}
	}
	return &Propagator{headers: headers}
}

// Inject 注入追踪上下文到HTTP头
func (p *Propagator) Inject(ctx context.Context, headers http.Header) {
	spanCtx := ctx.Value(traceContextKey{})
	if spanCtx == nil {
		return
	}
	sc := spanCtx.(*SpanContext)
	if sc == nil {
		return
	}

	headers.Set("x-trace-id", string(sc.TraceID))
	headers.Set("x-span-id", string(sc.SpanID))
	if sc.ParentID != "" {
		headers.Set("x-parent-span-id", string(sc.ParentID))
	}
	// W3C traceparent 格式
	headers.Set("traceparent", fmt.Sprintf("00-%s-%s-01", sc.TraceID, sc.SpanID))
	// B3 格式
	headers.Set("X-B3-TraceId", string(sc.TraceID))
	headers.Set("X-B3-SpanId", string(sc.SpanID))
	if sc.ParentID != "" {
		headers.Set("X-B3-ParentSpanId", string(sc.ParentID))
	}
}

// Extract 从上下文提取 (已存在的context)
func (p *Propagator) Extract(ctx context.Context) *SpanContext {
	spanCtx := ctx.Value(traceContextKey{})
	if spanCtx == nil {
		return nil
	}
	return spanCtx.(*SpanContext)
}

// ExtractHTTP 从HTTP请求提取
func (p *Propagator) ExtractHTTP(r *http.Request) context.Context {
	traceID := SpanID(r.Header.Get("x-trace-id"))
	spanID := SpanID(r.Header.Get("x-span-id"))
	parentID := SpanID(r.Header.Get("x-parent-span-id"))

	// W3C traceparent
	if traceID == "" {
		tp := r.Header.Get("traceparent")
		if tp != "" {
			parts := strings.Split(tp, "-")
			if len(parts) >= 3 {
				traceID = TraceID(parts[1])
				spanID = SpanID(parts[2])
			}
		}
	}

	// B3
	if traceID == "" {
		traceID = TraceID(r.Header.Get("X-B3-TraceId"))
		spanID = SpanID(r.Header.Get("X-B3-SpanId"))
		parentID = SpanID(r.Header.Get("X-B3-ParentSpanId"))
	}

	if traceID == "" {
		return r.Context()
	}

	sc := &SpanContext{
		TraceID: TraceID(traceID),
		SpanID:  spanID,
		ParentID: parentID,
		Sampled: true,
		Baggage: make(map[string]string),
	}

	return context.WithValue(r.Context(), traceContextKey{}, sc)
}

// --- SpanOption ---

type SpanOption func(*Span)

// WithService 设置服务名
func WithService(service string) SpanOption {
	return func(s *Span) { s.Service = service }
}

// WithKind 设置Span类型
func WithKind(kind SpanKind) SpanOption {
	return func(s *Span) { s.Kind = kind }
}

// WithTag 设置标签
func WithTag(key, value string) SpanOption {
	return func(s *Span) {
		if s.Tags == nil {
			s.Tags = make(map[string]string)
		}
		s.Tags[key] = value
	}
}

// WithTags 批量设置标签
func WithTags(tags map[string]string) SpanOption {
	return func(s *Span) {
		if s.Tags == nil {
			s.Tags = make(map[string]string)
		}
		for k, v := range tags {
			s.Tags[k] = v
		}
	}
}

// --- TraceAPI ---

// TraceAPI 追踪HTTP API
type TraceAPI struct {
	engine *TraceEngine
}

// NewTraceAPI 创建追踪API
func NewTraceAPI(engine *TraceEngine) *TraceAPI {
	return &TraceAPI{engine: engine}
}

// RegisterRoutes 注册路由
func (api *TraceAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/traces", api.handleQueryTraces)
	mux.HandleFunc("/api/v1/traces/", api.handleGetTrace)
	mux.HandleFunc("/api/v1/spans/", api.handleGetSpan)
	mux.HandleFunc("/api/v1/traces/", api.handleQuerySpans)
	mux.HandleFunc("/api/v1/traces/correlations/", api.handleGetCorrelations)
	mux.HandleFunc("/api/v1/traces/stats", api.handleGetStats)
}

// handleQueryTraces 查询追踪列表
func (api *TraceAPI) handleQueryTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := TraceQuery{
		Service:   r.URL.Query().Get("service"),
		Name:      r.URL.Query().Get("name"),
		Status:    SpanStatus(r.URL.Query().Get("status")),
		OrderBy:   r.URL.Query().Get("order_by"),
		Limit:     20,
		OrderDesc: r.URL.Query().Get("order") == "desc",
	}

	result, err := api.engine.QueryTraces(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result)
}

// handleGetTrace 获取完整追踪
func (api *TraceAPI) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceID := extractTraceIDFromPath(r.URL.Path)
	if traceID == "" {
		http.Error(w, "Missing trace ID", http.StatusBadRequest)
		return
	}

	trace, err := api.engine.GetTrace(traceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, trace)
}

// handleGetSpan 按SpanID查询 (反向: Span→Trace)
func (api *TraceAPI) handleGetSpan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 路径: /api/v1/spans/{spanID}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/spans/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Missing span ID", http.StatusBadRequest)
		return
	}
	spanID := SpanID(parts[0])

	// 反向查询: SpanID → Trace
	trace, err := api.engine.GetTraceBySpanID(spanID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, trace)
}

// handleQuerySpans 查询Span列表
func (api *TraceAPI) handleQuerySpans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceID := extractTraceIDFromPath(r.URL.Path)
	if traceID == "" {
		http.Error(w, "Missing trace ID", http.StatusBadRequest)
		return
	}

	query := SpanQuery{
		TraceID: traceID,
		Service: r.URL.Query().Get("service"),
		Name:    r.URL.Query().Get("name"),
		Status:  SpanStatus(r.URL.Query().Get("status")),
	}

	result, err := api.engine.QuerySpans(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, result)
}

// handleGetCorrelations 获取关联
func (api *TraceAPI) handleGetCorrelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceID := extractTraceIDFromPath(r.URL.Path)
	if traceID == "" {
		http.Error(w, "Missing trace ID", http.StatusBadRequest)
		return
	}

	query := CorrelationQuery{TraceID: traceID}
	result, err := api.engine.GetCorrelations(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, result)
}

// handleGetStats 获取统计
func (api *TraceAPI) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := api.engine.GetStats()
	writeJSON(w, stats)
}

func extractTraceIDFromPath(path string) TraceID {
	// /api/v1/traces/{traceID}/spans 或 /api/v1/traces/{traceID}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/traces/"), "/")
	if len(parts) > 0 && parts[0] != "" {
		return TraceID(parts[0])
	}
	return ""
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
