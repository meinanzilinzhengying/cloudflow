// P4: 链路追踪功能测试
package trace

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ============================================================================
// 一、W3C Trace ID 生成器测试
// ============================================================================

func TestW3CTraceIDGenerator_GenerateTraceID(t *testing.T) {
	gen := NewW3CTraceIDGenerator()
	traceID := gen.GenerateTraceID()
	assert.Equal(t, 32, len(traceID))
	assert.Regexp(t, "^[0-9a-f]{32}$", string(traceID))
}

func TestW3CTraceIDGenerator_GenerateSpanID(t *testing.T) {
	gen := NewW3CTraceIDGenerator()
	spanID := gen.GenerateSpanID()
	assert.Equal(t, 16, len(spanID))
	assert.Regexp(t, "^[0-9a-f]{16}$", string(spanID))
}

func TestW3CTraceIDGenerator_GenerateTraceParent(t *testing.T) {
	gen := NewW3CTraceIDGenerator()
	traceparent := gen.GenerateTraceParent(true)
	assert.Equal(t, 55, len(traceparent))
	assert.True(t, traceparent[:2] == "00")
	assert.True(t, traceparent[52:55] == "-01")

	traceparent2 := gen.GenerateTraceParent(false)
	assert.True(t, traceparent2[52:55] == "-00")
}

func TestParseTraceParent(t *testing.T) {
	// 有效格式
	sc, err := ParseTraceParent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	assert.NoError(t, err)
	assert.Equal(t, TraceID("4bf92f3577b34da6a3ce929d0e0e4736"), sc.TraceID)
	assert.Equal(t, SpanID("00f067aa0ba902b7"), sc.SpanID)
	assert.True(t, sc.Sampled)

	// 未采样
	sc2, err := ParseTraceParent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	assert.NoError(t, err)
	assert.False(t, sc2.Sampled)

	// 无效格式
	_, err = ParseTraceParent("invalid")
	assert.Error(t, err)

	// 长度错误
	_, err = ParseTraceParent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7")
	assert.Error(t, err)
}

func TestW3CTraceIDGenerator_Uniqueness(t *testing.T) {
	gen := NewW3CTraceIDGenerator()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		traceID := string(gen.GenerateTraceID())
		assert.False(t, seen[traceID], "Trace ID 应唯一")
		seen[traceID] = true
	}
}

// ============================================================================
// 二、eBPF Trace 关联测试
// ============================================================================

func TestEBPFTraceCorrelator_ExtractTraceFromHTTP(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)

	// W3C traceparent
	headers := map[string][]string{
		"traceparent": {"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := etc.ExtractTraceFromHTTP(headers, connKey)
	assert.NotNil(t, sc)
	assert.Equal(t, TraceID("4bf92f3577b34da6a3ce929d0e0e4736"), sc.TraceID)
	assert.Equal(t, SpanID("00f067aa0ba902b7"), sc.SpanID)

	// 验证映射已建立
	retrieved, ok := etc.GetSpanContextByConn(connKey)
	assert.True(t, ok)
	assert.Equal(t, sc.TraceID, retrieved.TraceID)
}

func TestEBPFTraceCorrelator_ExtractCustomTraceID(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)
	headers := map[string][]string{
		"x-trace-id":        {"abc123"},
		"x-span-id":         {"span456"},
		"x-parent-span-id":  {"parent789"},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := etc.ExtractTraceFromHTTP(headers, connKey)
	assert.NotNil(t, sc)
	assert.Equal(t, TraceID("abc123"), sc.TraceID)
	assert.Equal(t, SpanID("span456"), sc.SpanID)
	assert.Equal(t, SpanID("parent789"), sc.ParentID)
}

func TestEBPFTraceCorrelator_ExtractB3TraceID(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)
	headers := map[string][]string{
		"x-b3-traceid": {"abc123"},
		"x-b3-spanid":  {"span456"},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := etc.ExtractTraceFromHTTP(headers, connKey)
	assert.NotNil(t, sc)
	assert.Equal(t, TraceID("abc123"), sc.TraceID)
	assert.Equal(t, SpanID("span456"), sc.SpanID)
}

func TestEBPFTraceCorrelator_ExtractJaegerTraceID(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)
	headers := map[string][]string{
		"uber-trace-id": {"abc123:span456:parent789:1"},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := etc.ExtractTraceFromHTTP(headers, connKey)
	assert.NotNil(t, sc)
	assert.Equal(t, TraceID("abc123"), sc.TraceID)
	assert.Equal(t, SpanID("span456"), sc.SpanID)
	assert.Equal(t, SpanID("parent789"), sc.ParentID)
	assert.True(t, sc.Sampled)
}

func TestEBPFTraceCorrelator_AssociateFlow(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)
	flow := &FlowRecord{
		ConnKey:   ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80},
		TraceID:   TraceID("trace-abc"),
		SpanID:    SpanID("span-123"),
		Timestamp: time.Now(),
		Bytes:     1000,
	}
	etc.AssociateFlowWithTrace(flow)

	flows := etc.GetFlowsByTraceID(TraceID("trace-abc"))
	assert.Equal(t, 1, len(flows))
	assert.Equal(t, uint64(1000), flows[0].Bytes)
}

func TestEBPFTraceCorrelator_NoTraceID(t *testing.T) {
	etc := NewEBPFTraceCorrelator(nil)
	headers := map[string][]string{
		"content-type": {"application/json"},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := etc.ExtractTraceFromHTTP(headers, connKey)
	assert.Nil(t, sc)
}

func TestConnKey_String(t *testing.T) {
	key := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	assert.Equal(t, "10.0.0.1:12345→10.0.0.2:80", key.String())
}

// ============================================================================
// 三、调用链重建测试
// ============================================================================

func buildTestSpans() []Span {
	now := time.Now()
	traceID := TraceID("test-trace-123")
	return []Span{
		{TraceID: traceID, SpanID: SpanID("root"), ParentID: SpanID(""), Name: "root", Service: "gateway", StartTime: now, Duration: 100 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: traceID, SpanID: SpanID("s1"), ParentID: SpanID("root"), Name: "service-a", Service: "service-a", StartTime: now.Add(5 * time.Millisecond), Duration: 80 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: traceID, SpanID: SpanID("s2"), ParentID: SpanID("s1"), Name: "service-b", Service: "service-b", StartTime: now.Add(10 * time.Millisecond), Duration: 30 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: traceID, SpanID: SpanID("s3"), ParentID: SpanID("s1"), Name: "service-c", Service: "service-c", StartTime: now.Add(15 * time.Millisecond), Duration: 60 * time.Millisecond, Status: SpanStatusError},
		{TraceID: traceID, SpanID: SpanID("s4"), ParentID: SpanID("root"), Name: "service-d", Service: "service-d", StartTime: now.Add(20 * time.Millisecond), Duration: 20 * time.Millisecond, Status: SpanStatusOK},
	}
}

func TestChainRebuilder_Rebuild(t *testing.T) {
	cr := NewChainRebuilder()
	spans := buildTestSpans()
	trace := cr.Rebuild(TraceID("test-trace-123"), spans)

	assert.NotNil(t, trace)
	assert.Equal(t, SpanID("root"), trace.RootSpanID)
	assert.Equal(t, 5, len(trace.Spans))
	assert.GreaterOrEqual(t, trace.Depth, 2)
	assert.Equal(t, 5, trace.ServiceCount)
	assert.True(t, trace.HasError)
	assert.NotNil(t, trace.Tree)
}

func TestChainRebuilder_SpanTree(t *testing.T) {
	cr := NewChainRebuilder()
	spans := buildTestSpans()
	trace := cr.Rebuild(TraceID("test-trace-123"), spans)

	// 检查 root 的子节点
	rootSpan := trace.Spans[0]
	for _, span := range trace.Spans {
		if span.SpanID == trace.RootSpanID {
			rootSpan = span
			break
		}
	}
	assert.True(t, rootSpan.IsRoot)
	assert.GreaterOrEqual(t, len(rootSpan.Children), 2)

	// 检查 service-a 的子节点
	for _, span := range trace.Spans {
		if span.SpanID == SpanID("s1") {
			assert.Equal(t, 2, len(span.Children))
			assert.Equal(t, 2, span.ChildCount)
		}
	}
}

func TestChainRebuilder_CriticalPath(t *testing.T) {
	cr := NewChainRebuilder()
	spans := buildTestSpans()
	trace := cr.Rebuild(TraceID("test-trace-123"), spans)

	// 检查关键路径上的节点
	for _, span := range trace.Spans {
		if span.SpanID == SpanID("s3") { // 最慢的子节点
			assert.True(t, span.IsCritical || span.IsLeaf, "高延迟节点应在关键路径或是叶子")
		}
	}
}

func TestChainRebuilder_RebuildEmpty(t *testing.T) {
	cr := NewChainRebuilder()
	trace := cr.Rebuild(TraceID("empty"), []Span{})
	assert.Nil(t, trace)
}

// ============================================================================
// 四、自动标记测试
// ============================================================================

func TestAutoMarker_MarkSpans(t *testing.T) {
	am := NewAutoMarker(nil)

	spans := []Span{
		{TraceID: TraceID("t1"), SpanID: SpanID("s1"), Name: "fast", Service: "svc-a", Duration: 10 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: TraceID("t1"), SpanID: SpanID("s2"), Name: "slow", Service: "svc-a", Duration: 600 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: TraceID("t1"), SpanID: SpanID("s3"), Name: "error", Service: "svc-b", Duration: 100 * time.Millisecond, Status: SpanStatusError},
		{TraceID: TraceID("t1"), SpanID: SpanID("s4"), Name: "timeout", Service: "svc-b", Duration: 2 * time.Second, Status: SpanStatusTimeout},
	}

	marked := am.MarkSpans(spans)
	assert.Equal(t, 4, len(marked))

	// 检查慢调用标记
	var slowFound, errorFound, timeoutFound bool
	for _, ms := range marked {
		for _, mark := range ms.Marks {
			switch mark.Type {
			case MarkTypeSlowCall:
				slowFound = true
			case MarkTypeErrorCall:
				if ms.Status == SpanStatusError {
					errorFound = true
				}
				if ms.Status == SpanStatusTimeout {
					timeoutFound = true
				}
			}
		}
	}
	assert.True(t, slowFound, "应检测到慢调用")
	assert.True(t, errorFound, "应检测到错误调用")
	assert.True(t, timeoutFound, "应检测到超时调用")
}

func TestAutoMarker_MarkTrace(t *testing.T) {
	am := NewAutoMarker(nil)
	cr := NewChainRebuilder()
	spans := buildTestSpans()
	trace := cr.Rebuild(TraceID("test-trace-123"), spans)

	marked := am.MarkTrace(trace)
	assert.NotNil(t, marked)
	assert.True(t, marked.HasError)
	assert.GreaterOrEqual(t, marked.MarkedSpanCount, 1)
	assert.NotNil(t, marked.SpanMarks)
	assert.GreaterOrEqual(t, len(marked.TraceMarks), 1)
}

func TestLatencyHistory(t *testing.T) {
	lh := &LatencyHistory{
		ServiceName: "svc-a",
		SpanName:    "op1",
		MaxSize:     10,
	}

	for i := 0; i < 15; i++ {
		lh.Add(float64(i * 10))
	}
	assert.Equal(t, 10, len(lh.Latencies))
	assert.Greater(t, lh.P99(), 0.0)
	assert.Greater(t, lh.Avg(), 0.0)
}

func TestAutoMarker_DynamicThreshold(t *testing.T) {
	am := NewAutoMarker(&AutoMarkerConfig{
		SlowCallThresholdMs:  500,
		SlowCallP99Factor:      2.0,
		MarkHighLatencyCalls:   true,
		MarkErrorCalls:         true,
		HistoryWindowSize:      50,
		MinLatencyToMarkMs:     50,
	})

	// 先建立历史基线
	spans := []Span{
		{TraceID: TraceID("t1"), SpanID: SpanID("s1"), Name: "op", Service: "svc", Duration: 10 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: TraceID("t1"), SpanID: SpanID("s2"), Name: "op", Service: "svc", Duration: 20 * time.Millisecond, Status: SpanStatusOK},
		{TraceID: TraceID("t1"), SpanID: SpanID("s3"), Name: "op", Service: "svc", Duration: 15 * time.Millisecond, Status: SpanStatusOK},
	}
	am.MarkSpans(spans)

	// 然后发送一个远超 P99 的异常延迟
	anomalySpans := []Span{
		{TraceID: TraceID("t1"), SpanID: SpanID("s4"), Name: "op", Service: "svc", Duration: 2000 * time.Millisecond, Status: SpanStatusOK},
	}
	marked := am.MarkSpans(anomalySpans)
	assert.Equal(t, 1, len(marked))

	// 检查是否有异常标记
	var hasAnomaly bool
	for _, mark := range marked[0].Marks {
		if mark.Type == MarkTypeAnomaly || mark.Type == MarkTypeSlowCall {
			hasAnomaly = true
		}
	}
	assert.True(t, hasAnomaly, "应检测到异常延迟")
}

func TestMaxSeverity(t *testing.T) {
	assert.Equal(t, MarkSeverityCritical, maxSeverity(MarkSeverityWarning, MarkSeverityCritical))
	assert.Equal(t, MarkSeverityWarning, maxSeverity(MarkSeverityInfo, MarkSeverityWarning))
	assert.Equal(t, MarkSeverityInfo, maxSeverity(MarkSeverityInfo, MarkSeverityInfo))
	assert.Equal(t, MarkSeverityCritical, maxSeverity(MarkSeverityCritical, MarkSeverityWarning))
}

// ============================================================================
// 五、增强引擎测试
// ============================================================================

func TestEnhancedTraceEngine(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.w3cGen)
	assert.NotNil(t, engine.correlator)
	assert.NotNil(t, engine.rebuilder)
	assert.NotNil(t, engine.autoMarker)
}

func TestEnhancedTraceEngine_StartEnhancedSpan(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	ctx := context.Background()

	ctx, span := engine.StartEnhancedSpan(ctx, "test-operation", WithService("test-service"))
	assert.NotNil(t, span)
	assert.Equal(t, 32, len(span.TraceID))
	assert.Equal(t, 16, len(span.SpanID))
	assert.Equal(t, "test-operation", span.Name)
	assert.Equal(t, "test-service", span.Service)

	// 验证上下文注入
	spanCtx := engine.propagator.Extract(ctx)
	assert.NotNil(t, spanCtx)
	assert.Equal(t, span.TraceID, spanCtx.TraceID)
	assert.Equal(t, span.SpanID, spanCtx.SpanID)
}

func TestEnhancedTraceEngine_ParentSpan(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	ctx := context.Background()

	// 第一个 span
	ctx, parent := engine.StartEnhancedSpan(ctx, "parent-op", WithService("svc-a"))
	parent.Duration = 100 * time.Millisecond
	engine.FinishSpan(parent)

	// 第二个 span (子 span)
	ctx, child := engine.StartEnhancedSpan(ctx, "child-op", WithService("svc-b"))
	assert.Equal(t, parent.TraceID, child.TraceID)
	assert.Equal(t, parent.SpanID, child.ParentID)
}

func TestEnhancedTraceEngine_RebuildTrace(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	traceID := TraceID("test-rebuild-trace")
	now := time.Now()

	// 手动添加 spans
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("root"), ParentID: SpanID(""), Name: "root", Service: "gateway", StartTime: now, Duration: 100 * time.Millisecond, Status: SpanStatusOK})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s1"), ParentID: SpanID("root"), Name: "svc-a", Service: "svc-a", StartTime: now.Add(5 * time.Millisecond), Duration: 80 * time.Millisecond, Status: SpanStatusOK})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s2"), ParentID: SpanID("s1"), Name: "svc-b", Service: "svc-b", StartTime: now.Add(10 * time.Millisecond), Duration: 30 * time.Millisecond, Status: SpanStatusOK})

	rebuilt, err := engine.RebuildTrace(traceID)
	assert.NoError(t, err)
	assert.NotNil(t, rebuilt)
	assert.Equal(t, SpanID("root"), rebuilt.RootSpanID)
	assert.Equal(t, 3, len(rebuilt.Spans))
	assert.NotNil(t, rebuilt.Tree)
}

func TestEnhancedTraceEngine_MarkTrace(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	traceID := TraceID("test-mark-trace")
	now := time.Now()

	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("root"), ParentID: SpanID(""), Name: "root", Service: "gateway", StartTime: now, Duration: 100 * time.Millisecond, Status: SpanStatusOK})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s1"), ParentID: SpanID("root"), Name: "slow", Service: "svc-a", StartTime: now.Add(5 * time.Millisecond), Duration: 600 * time.Millisecond, Status: SpanStatusOK})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s2"), ParentID: SpanID("root"), Name: "error", Service: "svc-b", StartTime: now.Add(10 * time.Millisecond), Duration: 50 * time.Millisecond, Status: SpanStatusError})

	marked, err := engine.MarkTrace(traceID)
	assert.NoError(t, err)
	assert.NotNil(t, marked)
	assert.True(t, marked.HasError)
	assert.GreaterOrEqual(t, marked.MarkedSpanCount, 2)
	assert.GreaterOrEqual(t, len(marked.TraceMarks), 1)
}

func TestEnhancedTraceEngine_GenerateW3CTraceParent(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	traceparent := engine.GenerateW3CTraceParent(true)
	assert.Equal(t, 55, len(traceparent))
	assert.Equal(t, "-01", traceparent[52:55])
}

func TestEnhancedTraceEngine_GetServiceDependencyMap(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	traceID := TraceID("test-deps")
	now := time.Now()

	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("root"), ParentID: SpanID(""), Name: "root", Service: "gateway", StartTime: now, Duration: 100 * time.Millisecond})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s1"), ParentID: SpanID("root"), Name: "a", Service: "svc-a", StartTime: now.Add(5 * time.Millisecond), Duration: 50 * time.Millisecond})
	engine.store.AddSpan(Span{TraceID: traceID, SpanID: SpanID("s2"), ParentID: SpanID("s1"), Name: "b", Service: "svc-b", StartTime: now.Add(10 * time.Millisecond), Duration: 30 * time.Millisecond})

	deps := engine.GetServiceDependencyMap(traceID)
	assert.NotNil(t, deps)
	assert.Contains(t, deps, "gateway")
	assert.Contains(t, deps, "svc-a")
}

func TestEnhancedTraceEngine_QuerySlowTraces(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	now := time.Now()

	// 添加快追踪
	engine.store.AddSpan(Span{TraceID: TraceID("fast"), SpanID: SpanID("r1"), Name: "fast", Service: "svc", StartTime: now, Duration: 10 * time.Millisecond})
	// 添加慢追踪
	engine.store.AddSpan(Span{TraceID: TraceID("slow"), SpanID: SpanID("r2"), Name: "slow", Service: "svc", StartTime: now.Add(time.Second), Duration: 2 * time.Second})

	traces := engine.QuerySlowTraces(500, 10)
	assert.GreaterOrEqual(t, len(traces), 1)
}

func TestEnhancedTraceEngine_QueryErrorTraces(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	now := time.Now()

	// 正常追踪
	engine.store.AddSpan(Span{TraceID: TraceID("ok"), SpanID: SpanID("r1"), Name: "ok", Service: "svc", StartTime: now, Duration: 10 * time.Millisecond, Status: SpanStatusOK})
	// 错误追踪
	engine.store.AddSpan(Span{TraceID: TraceID("err"), SpanID: SpanID("r2"), Name: "err", Service: "svc", StartTime: now.Add(time.Second), Duration: 100 * time.Millisecond, Status: SpanStatusError})

	traces := engine.QueryErrorTraces(10)
	assert.GreaterOrEqual(t, len(traces), 1)
}

// ============================================================================
// 六、集成测试
// ============================================================================

func TestTraceEngine_Integration(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	ctx := context.Background()

	// 1. 生成 W3C Trace ID
	traceparent := engine.GenerateW3CTraceParent(true)
	assert.Equal(t, 55, len(traceparent))

	// 2. 从 HTTP 请求提取 Trace
	headers := map[string][]string{
		"traceparent": {traceparent},
	}
	connKey := ConnKey{SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 12345, DstPort: 80}
	sc := engine.ExtractTraceFromEBPF(headers, connKey)
	assert.NotNil(t, sc)

	// 3. 创建 Span 树
	ctx, rootSpan := engine.StartEnhancedSpan(ctx, "root", WithService("gateway"))
	ctx, spanA := engine.StartEnhancedSpan(ctx, "service-a", WithService("svc-a"))
	ctx, spanB := engine.StartEnhancedSpan(ctx, "service-b", WithService("svc-b"))

	spanB.Duration = 30 * time.Millisecond
	engine.FinishSpan(spanB)
	spanA.Duration = 80 * time.Millisecond
	engine.FinishSpan(spanA)
	rootSpan.Duration = 100 * time.Millisecond
	engine.FinishSpan(rootSpan)

	// 4. 重建调用链
	rebuilt, err := engine.RebuildTrace(rootSpan.TraceID)
	assert.NoError(t, err)
	assert.NotNil(t, rebuilt)
	assert.GreaterOrEqual(t, len(rebuilt.Spans), 3)

	// 5. 自动标记
	marked, err := engine.MarkTrace(rootSpan.TraceID)
	assert.NoError(t, err)
	assert.NotNil(t, marked)

	// 6. 服务依赖图
	deps := engine.GetServiceDependencyMap(rootSpan.TraceID)
	assert.NotNil(t, deps)
}

func TestPropagator_W3CCompatibility(t *testing.T) {
	engine := NewEnhancedTraceEngine(nil)
	ctx := context.Background()

	// 创建带有 Trace 上下文的请求
	ctx, span := engine.StartEnhancedSpan(ctx, "test", WithService("svc"))
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	engine.InjectHTTP(ctx, req.Header)

	// 验证 W3C traceparent
	traceparent := req.Header.Get("traceparent")
	assert.NotEmpty(t, traceparent)
	assert.Equal(t, 55, len(traceparent))

	// 验证 B3
	assert.NotEmpty(t, req.Header.Get("X-B3-TraceId"))
	assert.NotEmpty(t, req.Header.Get("X-B3-SpanId"))

	// 反向提取
	extractedCtx := engine.ExtractHTTP(req)
	extractedSC := engine.propagator.Extract(extractedCtx)
	assert.NotNil(t, extractedSC)
	assert.Equal(t, span.TraceID, extractedSC.TraceID)
	assert.Equal(t, span.SpanID, extractedSC.SpanID)
}
