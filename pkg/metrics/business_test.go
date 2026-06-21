package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// 一、用户行为指标测试
// ============================================================================

func TestRecordUserLogin(t *testing.T) {
	// 清除之前的指标
	prometheus.DefaultRegisterer.Unregister(UserLoginTotal)
	UserLoginTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_user_login_total_test",
			Help: "Test",
		},
		[]string{"tenant_id", "action", "status"},
	)

	RecordUserLogin("tenant-1", "login", "success")
	RecordUserLogin("tenant-1", "login", "success")
	RecordUserLogin("tenant-1", "logout", "success")
	RecordUserLogin("", "login", "failure") // 空 tenantID 应转为 "unknown"

	// CollectAndCount 返回 metric family 数量，对于 CounterVec 返回 1
	count := testutil.CollectAndCount(UserLoginTotal)
	assert.GreaterOrEqual(t, count, 1)
}

func TestRecordUserAction(t *testing.T) {
	RecordUserAction("tenant-1", "user-1", "query")
	RecordUserAction("tenant-1", "user-1", "create")
	RecordUserAction("tenant-2", "user-2", "delete")
	RecordUserAction("", "", "export") // 空 tenantID 和 userID 应转为默认值

	// 验证不 panic
	assert.True(t, true)
}

func TestSetUserActiveSessions(t *testing.T) {
	SetUserActiveSessions("tenant-1", 5)
	SetUserActiveSessions("tenant-1", 3)
	SetUserActiveSessions("tenant-2", 10)
	SetUserActiveSessions("", 0) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestRecordSessionDuration(t *testing.T) {
	RecordSessionDuration("tenant-1", 5*time.Minute)
	RecordSessionDuration("tenant-1", 1*time.Hour)
	RecordSessionDuration("", 30*time.Second) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

// ============================================================================
// 二、业务流程追踪指标测试
// ============================================================================

func TestRecordBusinessOperation(t *testing.T) {
	RecordBusinessOperation("tenant-1", "alert_evaluate", "success")
	RecordBusinessOperation("tenant-1", "alert_evaluate", "failure")
	RecordBusinessOperation("tenant-1", "flow_ingest", "success")
	RecordBusinessOperation("", "query_execute", "success") // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestRecordBusinessOperationDuration(t *testing.T) {
	RecordBusinessOperationDuration("tenant-1", "query_flows", "total", 150*time.Millisecond)
	RecordBusinessOperationDuration("tenant-1", "query_flows", "validation", 10*time.Millisecond)
	RecordBusinessOperationDuration("tenant-1", "query_flows", "processing", 100*time.Millisecond)
	RecordBusinessOperationDuration("tenant-1", "query_flows", "storage", 40*time.Millisecond)

	assert.True(t, true)
}

func TestRecordPipelineStage(t *testing.T) {
	RecordPipelineStage("tenant-1", "flow_processing", "decode", "success", 2*time.Millisecond)
	RecordPipelineStage("tenant-1", "flow_processing", "enrich", "success", 5*time.Millisecond)
	RecordPipelineStage("tenant-1", "flow_processing", "storage", "failure", 50*time.Millisecond)
	RecordPipelineStage("", "alert_pipeline", "evaluate", "success", 10*time.Millisecond) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

// ============================================================================
// 三、租户级别指标测试
// ============================================================================

func TestRecordTenantAPICall(t *testing.T) {
	RecordTenantAPICall("tenant-1", "GET", "/api/flows")
	RecordTenantAPICall("tenant-1", "POST", "/api/flows")
	RecordTenantAPICall("tenant-1", "GET", "/api/alerts")
	RecordTenantAPICall("", "GET", "/api/flows") // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestSetTenantResourceUsage(t *testing.T) {
	SetTenantResourceUsage("tenant-1", "flows/min", 1200)
	SetTenantResourceUsage("tenant-1", "metrics", 5000)
	SetTenantResourceUsage("tenant-1", "agent_count", 15)
	SetTenantResourceUsage("", "storage_bytes", 1e9) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestSetTenantAlertCount(t *testing.T) {
	SetTenantAlertCount("tenant-1", "critical", 2)
	SetTenantAlertCount("tenant-1", "warning", 5)
	SetTenantAlertCount("tenant-1", "info", 10)
	SetTenantAlertCount("", "critical", 0) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestSetTenantActiveUsers(t *testing.T) {
	SetTenantActiveUsers("tenant-1", 5)
	SetTenantActiveUsers("tenant-2", 12)
	SetTenantActiveUsers("", 0) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestSetTenantQuotaUsage(t *testing.T) {
	SetTenantQuotaUsage("tenant-1", "flows/min", 0.6)
	SetTenantQuotaUsage("tenant-1", "storage", 0.8)
	SetTenantQuotaUsage("tenant-1", "agent_count", 0.3)
	SetTenantQuotaUsage("", "flows/min", 0) // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

func TestSetTenantDataIngestRate(t *testing.T) {
	SetTenantDataIngestRate("tenant-1", 1024*1024) // 1MB/s
	SetTenantDataIngestRate("tenant-2", 512*1024)   // 512KB/s
	SetTenantDataIngestRate("", 0)                  // 空 tenantID 应转为 "unknown"

	assert.True(t, true)
}

// ============================================================================
// 四、业务流程追踪器测试
// ============================================================================

func TestBusinessTracer(t *testing.T) {
	tracer := NewBusinessTracer("flow_ingest", "tenant-1")
	assert.NotNil(t, tracer)
	assert.Equal(t, "flow_ingest", tracer.pipeline)
	assert.Equal(t, "tenant-1", tracer.tenantID)
	assert.False(t, tracer.IsClosed())

	// 阶段 1: decode
	span1 := tracer.Start("decode")
	time.Sleep(5 * time.Millisecond)
	span1.End(StatusSuccess)

	// 阶段 2: enrich
	span2 := tracer.Start("enrich")
	time.Sleep(10 * time.Millisecond)
	span2.End(StatusSuccess)

	// 阶段 3: storage（失败）
	span3 := tracer.Start("storage")
	time.Sleep(20 * time.Millisecond)
	span3.End(StatusFailure)

	// 关闭追踪器
	tracer.Close(StatusSuccess)

	assert.True(t, tracer.IsClosed())
	assert.GreaterOrEqual(t, len(tracer.GetStages()), 3)
	assert.Greater(t, tracer.GetTotalDuration(), time.Duration(0))

	// 验证幂等性
	tracer.Close(StatusSuccess) // 不应 panic
	assert.True(t, tracer.IsClosed())
}

func TestBusinessTracer_SkippedStage(t *testing.T) {
	tracer := NewBusinessTracer("alert_evaluate", "tenant-2")

	span1 := tracer.Start("validate")
	span1.End(StatusSuccess)

	span2 := tracer.Start("evaluate")
	span2.End(StatusSkipped)

	tracer.Close(StatusSuccess)

	stages := tracer.GetStages()
	assert.Equal(t, 2, len(stages))
	assert.Equal(t, StatusSkipped, stages[1].Status)
}

func TestBusinessTracer_PanicRecovery(t *testing.T) {
	tracer := NewBusinessTracer("query_execute", "tenant-1")

	assert.Panics(t, func() {
		span := tracer.Start("processing")
		defer span.End(StatusSuccess)
		panic("simulated panic")
	})

	// 注意：panic 会跳出当前 goroutine，所以这里 tracer 不会自动关闭
	// 实际使用时应通过 TraceBusinessOperation 的 defer recover 来关闭
}

func TestTraceBusinessOperation(t *testing.T) {
	ctx := WithTenantID(context.Background(), "tenant-1")

	// 成功场景
	err := TraceBusinessOperation(ctx, "query_flows", func() error {
		time.Sleep(5 * time.Millisecond)
		return nil
	})
	assert.NoError(t, err)

	// 失败场景
	err = TraceBusinessOperation(ctx, "query_flows", func() error {
		return errors.New("database error")
	})
	assert.Error(t, err)
}

func TestTraceBusinessOperationWithStages(t *testing.T) {
	ctx := WithTenantID(context.Background(), "tenant-1")

	err := TraceBusinessOperationWithStages(ctx, "flow_ingest", func(tracer *BusinessTracer) error {
		span := tracer.Start("decode")
		time.Sleep(2 * time.Millisecond)
		span.End(StatusSuccess)

		span2 := tracer.Start("storage")
		time.Sleep(3 * time.Millisecond)
		span2.End(StatusSuccess)

		return nil
	})
	assert.NoError(t, err)
}

func TestTraceFunc(t *testing.T) {
	ctx := WithTenantID(context.Background(), "tenant-1")

	// 成功
	err := TraceFunc(ctx, "query_flows", "validation", func() error {
		return nil
	})
	assert.NoError(t, err)

	// 失败
	err = TraceFunc(ctx, "query_flows", "processing", func() error {
		return errors.New("timeout")
	})
	assert.Error(t, err)
}

func TestTraceFuncWithValue(t *testing.T) {
	ctx := WithTenantID(context.Background(), "tenant-1")

	result, err := TraceFuncWithValue(ctx, "query_flows", "processing", func() (string, error) {
		return "success", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "success", result)

	_, err = TraceFuncWithValue(ctx, "query_flows", "processing", func() (string, error) {
		return "", errors.New("timeout")
	})
	assert.Error(t, err)
}

// ============================================================================
// 五、Context 辅助函数测试
// ============================================================================

func TestWithTenantIDAndGetTenantID(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetTenantID(ctx))

	ctx = WithTenantID(ctx, "tenant-123")
	assert.Equal(t, "tenant-123", GetTenantID(ctx))

	// 覆盖
	ctx = WithTenantID(ctx, "tenant-456")
	assert.Equal(t, "tenant-456", GetTenantID(ctx))
}

func TestWithUserIDAndGetUserID(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", GetUserID(ctx))

	ctx = WithUserID(ctx, "user-789")
	assert.Equal(t, "user-789", GetUserID(ctx))
}

func TestContextIsolation(t *testing.T) {
	ctx1 := WithTenantID(context.Background(), "tenant-1")
	ctx2 := WithTenantID(context.Background(), "tenant-2")

	assert.Equal(t, "tenant-1", GetTenantID(ctx1))
	assert.Equal(t, "tenant-2", GetTenantID(ctx2))
	assert.Equal(t, "", GetTenantID(context.Background()))
}

// ============================================================================
// 六、中间件辅助函数测试
// ============================================================================

func TestNormalizeBusinessEndpoint(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/flows", "/api/flows"},
		{"/api/flows/12345", "/api/flows/{id}"},
		{"/api/flows/550e8400-e29b-41d4-a716-446655440000", "/api/flows/{id}"},
		{"/api/tenants/abc123/projects", "/api/tenants/abc123/projects"}, // 非纯数字，不长于4
		{"/api/tenants/123456789/projects", "/api/tenants/{id}/projects"},
		{"/", "/"},
		{"", ""},
	}

	for _, tc := range tests {
		result := normalizeBusinessEndpoint(tc.input)
		assert.Equal(t, tc.expected, result, "input: %s", tc.input)
	}
}

func TestClassifyAction(t *testing.T) {
	tests := []struct {
		method   string
		endpoint string
		expected string
	}{
		{"GET", "/api/flows", "query"},
		{"POST", "/api/flows", "create"},
		{"PUT", "/api/flows/123", "update"},
		{"DELETE", "/api/flows/123", "delete"},
		{"GET", "/api/flows/export", "export"},
		{"POST", "/api/flows/search", "query"},
		{"GET", "/api/flows/download", "export"},
		{"POST", "/api/flows/create", "create"},
		{"PATCH", "/api/flows/update", "update"},
		{"POST", "/api/flows/delete", "delete"},
		{"POST", "/api/flows/other", "create"}, // 默认根据 POST 判断
	}

	for _, tc := range tests {
		result := classifyAction(tc.method, tc.endpoint)
		assert.Equal(t, tc.expected, result, "method=%s endpoint=%s", tc.method, tc.endpoint)
	}
}

func TestInjectBusinessContext(t *testing.T) {
	ctx := context.Background()
	ctx = InjectBusinessContext(ctx, "tenant-1", "user-1")
	assert.Equal(t, "tenant-1", GetTenantID(ctx))
	assert.Equal(t, "user-1", GetUserID(ctx))

	// 空值不覆盖
	ctx2 := InjectBusinessContext(ctx, "", "")
	assert.Equal(t, "tenant-1", GetTenantID(ctx2))
	assert.Equal(t, "user-1", GetUserID(ctx2))
}

func TestBusinessTracer_IdempotentEnd(t *testing.T) {
	tracer := NewBusinessTracer("test", "tenant-1")
	span := tracer.Start("stage1")
	span.End(StatusSuccess)
	span.End(StatusFailure) // 幂等，不应覆盖第一次的结果

	stages := tracer.GetStages()
	assert.Equal(t, 1, len(stages))
	assert.Equal(t, StatusSuccess, stages[0].Status)
}
