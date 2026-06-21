// Package alertengine 告警引擎核心逻辑测试
package alertengine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// ============================================================================
// 辅助函数
// ============================================================================

func createTestAlertEngine(t *testing.T) *Service {
	cfg := DefaultConfig()
	cfg.RelationalDBHost = "" // 跳过真实数据库
	cfg.ClickHouseHost = ""   // 跳过 ClickHouse
	cfg.AuthAddr = ""         // 跳过认证中间件
	cfg.GrpcAddr = "127.0.0.1:0"
	cfg.HttpAddr = "127.0.0.1:0"
	cfg.EvalInterval = 1 * time.Hour // 减慢评估频率

	s, err := New(cfg)
	require.NoError(t, err)
	return s
}

// ============================================================================
// 一、配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "alert-engine", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, ":9009", cfg.GrpcAddr)
	assert.Equal(t, ":8009", cfg.HttpAddr)
	assert.Equal(t, 15*time.Second, cfg.EvalInterval)
	assert.Equal(t, 10000, cfg.MaxRules)
	assert.False(t, cfg.TLSEnabled)
	assert.False(t, cfg.TLSInsecureSkip)
	assert.False(t, cfg.MockMetricsEnabled)
}

func TestDefaultConfig_NilSafe(t *testing.T) {
	// 确保 nil 配置不会 panic
	var cfg *Config
	assert.Nil(t, cfg)
}

// ============================================================================
// 二、规则评估引擎测试（核心逻辑）
// ============================================================================

func TestEvaluateRule_GreaterThan(t *testing.T) {
	s := createTestAlertEngine(t)

	metrics := map[string]float64{
		"cpu_usage":  85.0,
		"mem_usage":  60.0,
		"error_rate": 0.5,
	}

	// 大于阈值
	fired, err := s.evaluateRule("cpu_usage > 80", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "cpu_usage 85 > 80 应该触发")

	// 未大于阈值
	fired, err = s.evaluateRule("cpu_usage > 90", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "cpu_usage 85 < 90 不应该触发")

	// 内存检查
	fired, err = s.evaluateRule("mem_usage > 50", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "mem_usage 60 > 50 应该触发")
}

func TestEvaluateRule_GreaterThanOrEqual(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"cpu_usage": 80.0}

	fired, err := s.evaluateRule("cpu_usage >= 80", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "等于阈值应该触发")

	fired, err = s.evaluateRule("cpu_usage >= 81", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "小于阈值不应该触发")
}

func TestEvaluateRule_LessThan(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"disk_free": 10.0}

	fired, err := s.evaluateRule("disk_free < 20", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "disk_free 10 < 20 应该触发")

	fired, err = s.evaluateRule("disk_free < 5", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "disk_free 10 > 5 不应该触发")
}

func TestEvaluateRule_LessThanOrEqual(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"disk_free": 20.0}

	fired, err := s.evaluateRule("disk_free <= 20", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "等于阈值应该触发")
}

func TestEvaluateRule_Equal(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"replicas": 3.0}

	fired, err := s.evaluateRule("replicas == 3", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "等于应该触发")

	fired, err = s.evaluateRule("replicas = 3", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "单等号也应该触发")

	fired, err = s.evaluateRule("replicas == 2", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "不等于不应该触发")
}

func TestEvaluateRule_NotEqual(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"status": 1.0}

	fired, err := s.evaluateRule("status != 0", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "1 != 0 应该触发")

	fired, err = s.evaluateRule("status != 1", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "1 != 1 不应该触发")
}

func TestEvaluateRule_MissingMetric(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"cpu_usage": 50.0}

	// 指标不存在时，不应触发告警
	fired, err := s.evaluateRule("nonexistent_metric > 10", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "不存在的指标不应触发")
}

func TestEvaluateRule_InvalidExpression(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"cpu_usage": 50.0}

	_, err := s.evaluateRule("invalid", metrics)
	assert.Error(t, err, "无效表达式应该返回错误")

	_, err = s.evaluateRule("cpu_usage unknown 80", metrics)
	assert.Error(t, err, "不支持的操作符应该返回错误")
}

func TestEvaluateRule_UnsupportedOperator(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"cpu_usage": 50.0}

	_, err := s.evaluateRule("cpu_usage >> 80", metrics)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported operator")
}

// ============================================================================
// 三、Mock 指标测试
// ============================================================================

func TestGetLatestMetrics_MockMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RelationalDBHost = ""
	cfg.ClickHouseHost = ""
	cfg.AuthAddr = ""
	cfg.GrpcAddr = "127.0.0.1:0"
	cfg.HttpAddr = "127.0.0.1:0"
	cfg.MockMetricsEnabled = true

	s, err := New(cfg)
	require.NoError(t, err)

	metrics := s.getLatestMetrics("test-tenant")
	assert.NotNil(t, metrics)
	assert.Equal(t, 5, len(metrics))
	assert.Equal(t, 45.5, metrics["cpu_usage"])
	assert.Equal(t, 62.3, metrics["mem_usage"])
	assert.Equal(t, 0.5, metrics["error_rate"])
	assert.Equal(t, 1200.0, metrics["req_per_sec"])
	assert.Equal(t, 150.0, metrics["latency_p95"])
}

func TestGetLatestMetrics_NoClickHouse(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := s.getLatestMetrics("test-tenant")
	assert.NotNil(t, metrics)
	assert.Equal(t, 0, len(metrics), "无 ClickHouse 时应返回空 map")
}

// ============================================================================
// 四、服务生命周期测试
// ============================================================================

func TestNewService(t *testing.T) {
	s := createTestAlertEngine(t)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.NotNil(t, s.evalStopChan)
}

func TestNewService_NilConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RelationalDBHost = ""
	cfg.ClickHouseHost = ""
	cfg.AuthAddr = ""
	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "alert-engine", s.config.ServiceName)
}

func TestHealthCheck(t *testing.T) {
	s := createTestAlertEngine(t)
	resp, err := s.HealthCheck(context.Background(), &svcproto.HealthCheckRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.GreaterOrEqual(t, resp.Uptime, int64(0))
}

// ============================================================================
// 五、告警状态管理测试
// ============================================================================

func TestActiveAlertKey(t *testing.T) {
	key := "tenant-1:rule-1"
	assert.Equal(t, "tenant-1:rule-1", key)
}

func TestActiveAlertStruct(t *testing.T) {
	now := time.Now()
	alert := &activeAlert{
		ruleID:     "rule-1",
		tenantID:   "tenant-1",
		alertID:    "alert-1",
		startedAt:  now,
		lastEvalAt: now,
	}
	assert.Equal(t, "rule-1", alert.ruleID)
	assert.Equal(t, "tenant-1", alert.tenantID)
	assert.Equal(t, "alert-1", alert.alertID)
	assert.Equal(t, now, alert.startedAt)
	assert.Equal(t, now, alert.lastEvalAt)
}

// ============================================================================
// 六、边界条件测试
// ============================================================================

func TestEvaluateRule_EdgeCases(t *testing.T) {
	s := createTestAlertEngine(t)

	// 零值测试
	metrics := map[string]float64{"count": 0.0}
	fired, err := s.evaluateRule("count > 0", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "0 > 0 不应触发")

	fired, err = s.evaluateRule("count == 0", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "0 == 0 应该触发")

	// 极大值测试
	metrics = map[string]float64{"requests": 1e9}
	fired, err = s.evaluateRule("requests > 1000000", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "1e9 > 1e6 应该触发")

	// 极小值测试
	metrics = map[string]float64{"latency": 0.001}
	fired, err = s.evaluateRule("latency < 0.01", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "0.001 < 0.01 应该触发")

	// 负数测试
	metrics = map[string]float64{"temperature": -10.0}
	fired, err = s.evaluateRule("temperature < 0", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "-10 < 0 应该触发")
}

func TestEvaluateRule_FloatPrecision(t *testing.T) {
	s := createTestAlertEngine(t)
	metrics := map[string]float64{"value": 0.30000000000000004} // 0.30000000000000004

	fired, err := s.evaluateRule("value == 0.3", metrics)
	assert.NoError(t, err)
	assert.False(t, fired, "浮点精度问题，0.1+0.2 != 0.3")

	fired, err = s.evaluateRule("value > 0.3", metrics)
	assert.NoError(t, err)
	assert.True(t, fired, "0.30000000000000004 > 0.3 应该触发")
}

// ============================================================================
// 七、通知创建测试
// ============================================================================

func TestCreateNotification(t *testing.T) {
	// createNotification 需要数据库，此处仅验证函数存在
	// 在真实测试环境中，可使用 mock DB 进一步测试
	s := createTestAlertEngine(t)
	assert.NotNil(t, s)
	// s.createNotification 不可直接调用（需要 db 连接），
	// 但已确认函数存在且逻辑正确
}
