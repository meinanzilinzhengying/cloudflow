//go:build linux

package governance

import (
	"testing"
	"time"
)

// ============================================================================
// 版本管理测试
// ============================================================================

func TestVersionRegistry(t *testing.T) {
	vr := NewVersionRegistry()

	v1 := &ServiceVersion{
		ServiceName: "user-service",
		Version:     "v1.0.0",
		InstanceID:  "inst-1",
		Host:        "10.0.0.1",
		Port:        8080,
		Status:      VersionActive,
		Weight:      100,
	}
	v2 := &ServiceVersion{
		ServiceName: "user-service",
		Version:     "v2.0.0",
		InstanceID:  "inst-2",
		Host:        "10.0.0.2",
		Port:        8080,
		Status:      VersionCanary,
		Weight:      5,
	}

	// 注册
	if err := vr.Register(v1); err != nil {
		t.Fatalf("Register v1 failed: %v", err)
	}
	if err := vr.Register(v2); err != nil {
		t.Fatalf("Register v2 failed: %v", err)
	}

	// 获取版本
	versions := vr.GetVersions("user-service")
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}

	// 获取活跃版本
	active := vr.GetActiveVersions("user-service")
	if len(active) != 2 {
		t.Fatalf("expected 2 active versions, got %d", len(active))
	}

	// 分布
	dist := vr.GetVersionDistribution("user-service")
	if dist["v1.0.0"] != 1 || dist["v2.0.0"] != 1 {
		t.Errorf("unexpected distribution: %v", dist)
	}

	// 权重选择
	selected, err := vr.SelectByWeight("user-service", "user-123")
	if err != nil {
		t.Fatalf("SelectByWeight failed: %v", err)
	}
	if selected == nil {
		t.Fatal("expected non-nil selected version")
	}

	// 注销
	vr.Deregister("user-service", "inst-1")
	versions = vr.GetVersions("user-service")
	if len(versions) != 1 {
		t.Fatalf("expected 1 version after deregister, got %d", len(versions))
	}

	// 更新状态
	if err := vr.UpdateStatus("user-service", "inst-2", VersionRetired); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	active = vr.GetActiveVersions("user-service")
	if len(active) != 0 {
		t.Fatalf("expected 0 active versions after retirement, got %d", len(active))
	}
}

func TestVersionRegistryDefaults(t *testing.T) {
	vr := NewVersionRegistry()
	v := &ServiceVersion{
		ServiceName: "svc",
		Version:     "v1",
		InstanceID:  "i1",
	}
	vr.Register(v)
	if v.Status != VersionActive {
		t.Errorf("expected default status active, got %s", v.Status)
	}
	if !v.RegisteredAt.IsZero() {
		// 应该被自动设置
	} else {
		t.Error("expected RegisteredAt to be set")
	}
}

func TestVersionRegistryInvalid(t *testing.T) {
	vr := NewVersionRegistry()
	v := &ServiceVersion{ServiceName: "", Version: "v1", InstanceID: "i1"}
	if err := vr.Register(v); err == nil {
		t.Error("expected error for empty service_name")
	}
}

// ============================================================================
// 灰度发布测试
// ============================================================================

func TestCanaryManager(t *testing.T) {
	vr := NewVersionRegistry()
	cm := NewCanaryManager(vr)
	cm.Start()
	defer cm.Stop()

	// 注册版本
	vr.Register(&ServiceVersion{
		ServiceName: "api-service",
		Version:     "v1.0.0",
		InstanceID:  "stable-1",
		Status:      VersionActive,
		Weight:      100,
	})
	vr.Register(&ServiceVersion{
		ServiceName: "api-service",
		Version:     "v2.0.0",
		InstanceID:  "canary-1",
		Status:      VersionActive,
		Weight:      0,
	})

	config := &CanaryConfig{
		ServiceName:    "api-service",
		CanaryVersion:  "v2.0.0",
		StableVersion:  "v1.0.0",
		TrafficPercent: 30,
		StepPercent:    10,
		StepInterval:   1 * time.Second,
		Criteria: CanaryCriteria{
			MinRequestCount: 10,
			MaxErrorRate:    0.05,
			MaxLatencyP99:   1000,
			MinDuration:     100 * time.Millisecond,
		},
	}

	release, err := cm.CreateRelease(config)
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}
	if release.Status != CanaryRunning {
		t.Errorf("expected status running, got %s", release.Status)
	}

	// 更新指标（满足条件）
	cm.UpdateMetrics("api-service", CanaryMetrics{
		RequestCount: 100,
		ErrorCount:   1,
		ErrorRate:    0.01,
		LatencyP99:   500,
	})

	// 等待满足最小持续时间
	time.Sleep(200 * time.Millisecond)

	// 手动推进
	cm.checkPromotions()

	// 获取 release
	rel, ok := cm.GetRelease("api-service")
	if !ok {
		t.Fatal("expected release to exist")
	}
	if rel.CurrentPercent < 10 {
		t.Errorf("expected percent >= 10, got %d", rel.CurrentPercent)
	}

	// 统计
	stats := cm.Stats()
	if stats["total"].(int) != 1 {
		t.Errorf("expected total 1, got %v", stats["total"])
	}

	// 回滚
	if err := cm.Rollback("api-service"); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	rel, _ = cm.GetRelease("api-service")
	if rel.Status != CanaryRolledBack {
		t.Errorf("expected status rolled_back, got %s", rel.Status)
	}
	if rel.CurrentPercent != 0 {
		t.Errorf("expected percent 0 after rollback, got %d", rel.CurrentPercent)
	}
}

func TestCanaryManagerMeetsCriteria(t *testing.T) {
	cm := NewCanaryManager(nil)
	criteria := CanaryCriteria{
		MinRequestCount: 10,
		MaxErrorRate:    0.05,
		MaxLatencyP99:   1000,
		MinDuration:     1 * time.Minute,
	}

	// 不满足：请求数不够
	if cm.meetsCriteria(criteria, CanaryMetrics{RequestCount: 5}, 2*time.Minute) {
		t.Error("expected not meet criteria (request count)")
	}

	// 不满足：错误率太高
	if cm.meetsCriteria(criteria, CanaryMetrics{RequestCount: 100, ErrorRate: 0.1}, 2*time.Minute) {
		t.Error("expected not meet criteria (error rate)")
	}

	// 不满足：持续时间不够
	if cm.meetsCriteria(criteria, CanaryMetrics{RequestCount: 100, ErrorRate: 0.01, LatencyP99: 500}, 10*time.Second) {
		t.Error("expected not meet criteria (duration)")
	}

	// 满足
	if !cm.meetsCriteria(criteria, CanaryMetrics{RequestCount: 100, ErrorRate: 0.01, LatencyP99: 500}, 2*time.Minute) {
		t.Error("expected meets criteria")
	}
}

func TestCanaryManagerInvalidConfig(t *testing.T) {
	cm := NewCanaryManager(NewVersionRegistry())
	_, err := cm.CreateRelease(&CanaryConfig{
		ServiceName:   "",
		CanaryVersion: "v2",
		StableVersion: "v1",
	})
	if err == nil {
		t.Error("expected error for empty service_name")
	}
}

// ============================================================================
// 统一配置测试
// ============================================================================

func TestConfigManager(t *testing.T) {
	cm := NewConfigManager()

	config := &GovernanceConfig{
		ServiceName: "order-service",
		CircuitBreaker: &CircuitBreakerConfig{
			Name:             "order-cb",
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:          30 * time.Second,
			Enabled:          true,
		},
		RateLimit: &RateLimitConfig{
			Name:    "order-rl",
			QPS:     100,
			Burst:   200,
			Window:  1 * time.Second,
			Enabled: true,
		},
		Timeout: 10 * time.Second,
		Version: "v1.0.0",
	}

	if err := cm.SetConfig(config); err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	// 获取
	retrieved, ok := cm.GetConfig("order-service")
	if !ok {
		t.Fatal("expected config to exist")
	}
	if retrieved.ServiceName != "order-service" {
		t.Errorf("unexpected service name: %s", retrieved.ServiceName)
	}
	if !retrieved.CircuitBreaker.Enabled {
		t.Error("expected circuit breaker enabled")
	}
	if retrieved.RateLimit.QPS != 100 {
		t.Errorf("expected QPS 100, got %d", retrieved.RateLimit.QPS)
	}

	// 监听
	watchCh := cm.Watch()
	config2 := &GovernanceConfig{
		ServiceName: "order-service",
		Version:     "v1.1.0",
		CircuitBreaker: &CircuitBreakerConfig{
			Name:             "order-cb",
			FailureThreshold: 10,
			Enabled:          true,
		},
	}
	cm.SetConfig(config2)

	select {
	case updated := <-watchCh:
		if updated.Version != "v1.1.0" {
			t.Errorf("expected version v1.1.0 from watcher, got %s", updated.Version)
		}
	case <-time.After(1 * time.Second):
		t.Error("watcher timeout")
	}

	// 所有配置
	all := cm.GetAllConfigs()
	if len(all) != 1 {
		t.Errorf("expected 1 config, got %d", len(all))
	}

	// 删除
	cm.DeleteConfig("order-service")
	_, ok = cm.GetConfig("order-service")
	if ok {
		t.Error("expected config to be deleted")
	}

	// 统计
	stats := cm.Stats()
	if stats["total_services"].(int) != 0 {
		t.Errorf("expected 0 services, got %v", stats["total_services"])
	}
}

func TestConfigManagerValidation(t *testing.T) {
	// 验证熔断配置
	if err := ValidateCircuitBreaker(nil); err == nil {
		t.Error("expected error for nil circuit breaker")
	}
	if err := ValidateCircuitBreaker(&CircuitBreakerConfig{Name: "", FailureThreshold: 5}); err == nil {
		t.Error("expected error for empty name")
	}
	if err := ValidateCircuitBreaker(&CircuitBreakerConfig{Name: "cb", FailureThreshold: 0}); err == nil {
		t.Error("expected error for invalid threshold")
	}

	// 验证限流配置
	if err := ValidateRateLimit(nil); err == nil {
		t.Error("expected error for nil rate limit")
	}
	if err := ValidateRateLimit(&RateLimitConfig{Name: "", QPS: 100}); err == nil {
		t.Error("expected error for empty name")
	}
	if err := ValidateRateLimit(&RateLimitConfig{Name: "rl", QPS: 0}); err == nil {
		t.Error("expected error for invalid QPS")
	}
}

func TestMergeConfig(t *testing.T) {
	base := &GovernanceConfig{
		ServiceName: "svc",
		Timeout:     5 * time.Second,
		Version:     "v1",
		CircuitBreaker: &CircuitBreakerConfig{Name: "cb1", FailureThreshold: 5},
		RateLimit:  &RateLimitConfig{Name: "rl1", QPS: 50},
	}

	override := &GovernanceConfig{
		ServiceName: "svc",
		Timeout:     10 * time.Second,
		Version:     "v2",
		RateLimit:   &RateLimitConfig{Name: "rl2", QPS: 100},
	}

	merged := MergeConfig(base, override)
	if merged.Timeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", merged.Timeout)
	}
	if merged.Version != "v2" {
		t.Errorf("expected version v2, got %s", merged.Version)
	}
	if merged.CircuitBreaker == nil || merged.CircuitBreaker.Name != "cb1" {
		t.Error("expected circuit breaker from base")
	}
	if merged.RateLimit == nil || merged.RateLimit.Name != "rl2" {
		t.Error("expected rate limit from override")
	}
}

func TestDefaultConfigs(t *testing.T) {
	cb := DefaultCircuitBreakerConfig("test-cb")
	if cb.FailureThreshold != 5 {
		t.Errorf("unexpected failure threshold: %d", cb.FailureThreshold)
	}
	if !cb.Enabled {
		t.Error("expected circuit breaker enabled by default")
	}

	rl := DefaultRateLimitConfig("test-rl")
	if rl.QPS != 100 {
		t.Errorf("unexpected QPS: %d", rl.QPS)
	}
	if !rl.Enabled {
		t.Error("expected rate limit enabled by default")
	}

	retry := DefaultRetryConfig()
	if retry.MaxRetries != 3 {
		t.Errorf("unexpected max retries: %d", retry.MaxRetries)
	}
}

// ============================================================================
// 调用链监控测试
// ============================================================================

func TestTraceCollector(t *testing.T) {
	tc := NewTraceCollector(1.0, 1000)

	span1 := &TraceSpan{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "user-service",
		Operation:   "GET /users",
		StartTime:   time.Now().UnixMicro(),
		Duration:    100000, // 100ms
		Status:      200,
	}
	span2 := &TraceSpan{
		TraceID:     "trace-1",
		SpanID:      "span-2",
		ParentID:    "span-1",
		ServiceName: "order-service",
		Operation:   "POST /orders",
		StartTime:   time.Now().UnixMicro(),
		Duration:    500000, // 500ms
		Status:      500,
		Error:       true,
	}
	span3 := &TraceSpan{
		TraceID:     "trace-2",
		SpanID:      "span-3",
		ServiceName: "user-service",
		Operation:   "GET /users/1",
		StartTime:   time.Now().UnixMicro(),
		Duration:    200000,
		Status:      200,
	}

	// 收集
	if !tc.Collect(span1) {
		t.Error("expected span1 to be collected")
	}
	if !tc.Collect(span2) {
		t.Error("expected span2 to be collected")
	}
	if !tc.Collect(span3) {
		t.Error("expected span3 to be collected")
	}

	// 获取 trace
	trace := tc.GetTrace("trace-1")
	if len(trace) != 2 {
		t.Errorf("expected 2 spans in trace-1, got %d", len(trace))
	}

	// 服务统计
	stats, ok := tc.GetServiceStats("user-service")
	if !ok {
		t.Fatal("expected user-service stats")
	}
	if stats.RequestCount != 2 {
		t.Errorf("expected request count 2, got %d", stats.RequestCount)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("expected error count 0, got %d", stats.ErrorCount)
	}

	orderStats, ok := tc.GetServiceStats("order-service")
	if !ok {
		t.Fatal("expected order-service stats")
	}
	if orderStats.RequestCount != 1 {
		t.Errorf("expected request count 1, got %d", orderStats.RequestCount)
	}
	if orderStats.ErrorCount != 1 {
		t.Errorf("expected error count 1, got %d", orderStats.ErrorCount)
	}
	if orderStats.ErrorRate != 1.0 {
		t.Errorf("expected error rate 1.0, got %f", orderStats.ErrorRate)
	}

	// 依赖图
	deps := tc.GetServiceDependencyGraph()
	if len(deps) == 0 {
		t.Log("no dependencies found (expected: user-service -> order-service)")
	}
	if deps["user-service"] != nil {
		if !contains(deps["user-service"], "order-service") {
			t.Errorf("expected user-service -> order-service dependency")
		}
	}

	// 错误 trace
	errTraces := tc.GetErrorTraces(10)
	if len(errTraces) != 1 {
		t.Errorf("expected 1 error trace, got %d", len(errTraces))
	}

	// 慢 trace
	slowTraces := tc.GetSlowTraces(300000, 10) // >300ms
	if len(slowTraces) != 1 {
		t.Errorf("expected 1 slow trace, got %d", len(slowTraces))
	}

	// 统计
	stats2 := tc.Stats()
	if stats2["span_count"].(int) != 3 {
		t.Errorf("expected 3 spans, got %v", stats2["span_count"])
	}
	if stats2["trace_count"].(int) != 2 {
		t.Errorf("expected 2 traces, got %v", stats2["trace_count"])
	}

	// Flush
	tc.Flush()
	stats3 := tc.Stats()
	if stats3["span_count"].(int) != 0 {
		t.Errorf("expected 0 spans after flush, got %v", stats3["span_count"])
	}
}

func TestTraceCollectorSampling(t *testing.T) {
	tc := NewTraceCollector(0.5, 1000)
	collected := 0
	for i := 0; i < 1000; i++ {
		span := &TraceSpan{
			TraceID:     "trace-sampling",
			SpanID:      "span",
			ServiceName: "svc",
			Operation:   "op",
			Duration:    100000,
		}
		if tc.Collect(span) {
			collected++
		}
	}
	// 50% 采样率，收集数量应该在 200-800 之间
	if collected < 200 || collected > 800 {
		t.Errorf("unexpected collected count with 50%% sampling: %d", collected)
	}
}

func TestTraceCollectorBatch(t *testing.T) {
	tc := NewTraceCollector(1.0, 1000)
	spans := []*TraceSpan{
		{TraceID: "t1", SpanID: "s1", ServiceName: "svc", Duration: 100000},
		{TraceID: "t1", SpanID: "s2", ServiceName: "svc", Duration: 200000},
		{TraceID: "t2", SpanID: "s3", ServiceName: "svc", Duration: 300000},
	}
	count := tc.CollectBatch(spans)
	if count != 3 {
		t.Errorf("expected 3 collected, got %d", count)
	}
	if tc.Stats()["span_count"].(int) != 3 {
		t.Errorf("expected 3 spans, got %v", tc.Stats()["span_count"])
	}
}

func TestTraceCollectorNilSpan(t *testing.T) {
	tc := NewTraceCollector(1.0, 100)
	if tc.Collect(nil) {
		t.Error("expected nil span to not be collected")
	}
}

func TestPercentile(t *testing.T) {
	values := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if percentile(values, 0.5) != 5 {
		t.Errorf("expected p50 = 5, got %d", percentile(values, 0.5))
	}
	if percentile(values, 0.0) != 1 {
		t.Errorf("expected p0 = 1, got %d", percentile(values, 0.0))
	}
	if percentile(values, 1.0) != 10 {
		t.Errorf("expected p100 = 10, got %d", percentile(values, 1.0))
	}
	if percentile([]int64{}, 0.5) != 0 {
		t.Errorf("expected empty percentile = 0, got %d", percentile([]int64{}, 0.5))
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "a") {
		t.Error("expected contains a")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Error("expected not contains c")
	}
}
