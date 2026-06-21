// P24: 业务指标埋点 — 用户行为、业务流程、租户级别指标
//
// 解决业务层面可观测性不足的问题：
//   - 用户行为指标缺失
//   - 业务流程追踪不完整
//   - 租户级别指标不够细
//
package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ============================================================================
// 一、用户行为指标
// ============================================================================

var (
	// UserLoginTotal 用户登录/登出次数
	// Labels: tenant_id, action (login/logout), status (success/failure)
	UserLoginTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_user_login_total",
			Help: "Total number of user login/logout attempts",
		},
		[]string{"tenant_id", "action", "status"},
	)

	// UserActionsTotal 用户操作次数
	// Labels: tenant_id, user_id, action_type (create/query/update/delete/export/...)
	UserActionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_user_actions_total",
			Help: "Total number of user actions",
		},
		[]string{"tenant_id", "user_id", "action_type"},
	)

	// UserActiveSessions 当前活跃会话数（Gauge）
	// Labels: tenant_id
	UserActiveSessions = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_user_active_sessions",
			Help: "Current number of active user sessions",
		},
		[]string{"tenant_id"},
	)

	// UserSessionDurationSeconds 用户会话时长分布
	// Labels: tenant_id
	UserSessionDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudflow_user_session_duration_seconds",
			Help:    "User session duration in seconds",
			Buckets: []float64{60, 300, 600, 1800, 3600, 7200, 14400, 28800},
		},
		[]string{"tenant_id"},
	)
)

// ============================================================================
// 二、业务流程追踪指标
// ============================================================================

var (
	// BusinessOperationTotal 关键业务操作执行次数
	// Labels: tenant_id, operation (alert_evaluate/flow_ingest/query_execute/...), status (success/failure)
	BusinessOperationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_business_operation_total",
			Help: "Total number of business operations",
		},
		[]string{"tenant_id", "operation", "status"},
	)

	// BusinessOperationDurationSeconds 关键业务操作耗时
	// Labels: tenant_id, operation, stage (total/validation/processing/storage)
	BusinessOperationDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudflow_business_operation_duration_seconds",
			Help:    "Business operation duration in seconds by stage",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"tenant_id", "operation", "stage"},
	)

	// BusinessPipelineStageTotal 业务流程各阶段执行次数
	// Labels: tenant_id, pipeline (flow_processing/alert_pipeline/...), stage, status
	BusinessPipelineStageTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_business_pipeline_stage_total",
			Help: "Total number of pipeline stage executions",
		},
		[]string{"tenant_id", "pipeline", "stage", "status"},
	)

	// BusinessPipelineStageLatencySeconds 业务流程各阶段延迟
	// Labels: tenant_id, pipeline, stage
	BusinessPipelineStageLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudflow_business_pipeline_stage_latency_seconds",
			Help:    "Pipeline stage latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"tenant_id", "pipeline", "stage"},
	)
)

// ============================================================================
// 三、租户级别指标
// ============================================================================

var (
	// TenantAPICallsTotal 租户 API 调用次数
	// Labels: tenant_id, method (GET/POST/...), endpoint (/api/flows/...)
	TenantAPICallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_tenant_api_calls_total",
			Help: "Total number of API calls per tenant",
		},
		[]string{"tenant_id", "method", "endpoint"},
	)

	// TenantResourceUsage 租户资源使用量（Gauge）
	// Labels: tenant_id, resource_type (flows/min/flows_hour/metrics/storage_bytes/agent_count/...)
	TenantResourceUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_resource_usage",
			Help: "Current resource usage per tenant",
		},
		[]string{"tenant_id", "resource_type"},
	)

	// TenantAlertCount 租户告警数量（Gauge）
	// Labels: tenant_id, severity (critical/warning/info)
	TenantAlertCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_alert_count",
			Help: "Current number of active alerts per tenant",
		},
		[]string{"tenant_id", "severity"},
	)

	// TenantActiveUsers 租户活跃用户数（Gauge）
	// Labels: tenant_id
	TenantActiveUsers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_active_users",
			Help: "Current number of active users per tenant",
		},
		[]string{"tenant_id"},
	)

	// TenantQuotaUsageRatio 租户配额使用率（0.0~1.0 Gauge）
	// Labels: tenant_id, quota_type (flows/min/metrics/min/storage/retention/agent_count/...)
	TenantQuotaUsageRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_quota_usage_ratio",
			Help: "Current quota usage ratio (0.0~1.0) per tenant",
		},
		[]string{"tenant_id", "quota_type"},
	)

	// TenantDataIngestRateBytes 租户数据摄入速率
	// Labels: tenant_id
	TenantDataIngestRateBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_data_ingest_rate_bytes",
			Help: "Current data ingest rate in bytes per second per tenant",
		},
		[]string{"tenant_id"},
	)
)

// ============================================================================
// 四、便捷埋点函数
// ============================================================================

// RecordUserLogin 记录用户登录/登出事件
func RecordUserLogin(tenantID, action, status string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	UserLoginTotal.WithLabelValues(tenantID, action, status).Inc()
}

// RecordUserAction 记录用户操作
func RecordUserAction(tenantID, userID, actionType string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	if userID == "" {
		userID = "anonymous"
	}
	UserActionsTotal.WithLabelValues(tenantID, userID, actionType).Inc()
}

// RecordBusinessOperation 记录业务操作完成
func RecordBusinessOperation(tenantID, operation, status string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	BusinessOperationTotal.WithLabelValues(tenantID, operation, status).Inc()
}

// RecordBusinessOperationDuration 记录业务操作耗时
func RecordBusinessOperationDuration(tenantID, operation, stage string, duration time.Duration) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	BusinessOperationDurationSeconds.WithLabelValues(tenantID, operation, stage).Observe(duration.Seconds())
}

// RecordPipelineStage 记录业务流程阶段执行
func RecordPipelineStage(tenantID, pipeline, stage, status string, latency time.Duration) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	BusinessPipelineStageTotal.WithLabelValues(tenantID, pipeline, stage, status).Inc()
	BusinessPipelineStageLatencySeconds.WithLabelValues(tenantID, pipeline, stage).Observe(latency.Seconds())
}

// RecordTenantAPICall 记录租户 API 调用
func RecordTenantAPICall(tenantID, method, endpoint string) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantAPICallsTotal.WithLabelValues(tenantID, method, endpoint).Inc()
}

// SetTenantResourceUsage 设置租户资源使用量
func SetTenantResourceUsage(tenantID, resourceType string, value float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantResourceUsage.WithLabelValues(tenantID, resourceType).Set(value)
}

// SetTenantAlertCount 设置租户告警数量
func SetTenantAlertCount(tenantID, severity string, count float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantAlertCount.WithLabelValues(tenantID, severity).Set(count)
}

// SetTenantActiveUsers 设置租户活跃用户数
func SetTenantActiveUsers(tenantID string, count float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantActiveUsers.WithLabelValues(tenantID).Set(count)
}

// SetTenantQuotaUsage 设置租户配额使用率
func SetTenantQuotaUsage(tenantID, quotaType string, ratio float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantQuotaUsageRatio.WithLabelValues(tenantID, quotaType).Set(ratio)
}

// SetTenantDataIngestRate 设置租户数据摄入速率
func SetTenantDataIngestRate(tenantID string, bytesPerSec float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	TenantDataIngestRateBytes.WithLabelValues(tenantID).Set(bytesPerSec)
}

// SetUserActiveSessions 设置活跃会话数
func SetUserActiveSessions(tenantID string, count float64) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	UserActiveSessions.WithLabelValues(tenantID).Set(count)
}

// RecordSessionDuration 记录用户会话时长
func RecordSessionDuration(tenantID string, duration time.Duration) {
	if tenantID == "" {
		tenantID = "unknown"
	}
	UserSessionDurationSeconds.WithLabelValues(tenantID).Observe(duration.Seconds())
}

// ============================================================================
// 五、从 Context 提取租户 ID 的辅助函数
// ============================================================================

// tenantIDKey 是 context 中存储 tenant_id 的 key 类型
type tenantIDKey struct{}

// WithTenantID 将租户 ID 注入 context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, tenantID)
}

// GetTenantID 从 context 提取租户 ID
func GetTenantID(ctx context.Context) string {
	if v := ctx.Value(tenantIDKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// userIDKey 是 context 中存储 user_id 的 key 类型
type userIDKey struct{}

// WithUserID 将用户 ID 注入 context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

// GetUserID 从 context 提取用户 ID
func GetUserID(ctx context.Context) string {
	if v := ctx.Value(userIDKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
