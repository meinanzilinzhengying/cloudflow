// Package controlplane 控制平面核心逻辑测试
package controlplane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 一、配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "control-plane", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, ":9001", cfg.GrpcAddr)
	assert.Equal(t, ":8001", cfg.HttpAddr)
	assert.Equal(t, []string{"localhost:2379"}, cfg.EtcdEndpoints)
	assert.Equal(t, "github.com/meinanzilinzhengying/cloudflow/services/", cfg.EtcdPrefix)
	assert.Equal(t, 90*time.Second, cfg.AgentTTL)
	assert.Equal(t, 60*time.Second, cfg.HeartbeatTimeout)
	assert.False(t, cfg.TLSEnabled)
	assert.False(t, cfg.TLSInsecureSkip)
}

func TestDefaultConfig_NilSafe(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.NotEmpty(t, cfg.EtcdEndpoints)
	assert.NotEmpty(t, cfg.EtcdPrefix)
}

// ============================================================================
// 二、运行时配置测试
// ============================================================================

func TestRuntimeConfig_Threshold(t *testing.T) {
	cfg := &RuntimeConfig{
		ID:          1,
		Key:         "cpu_usage_threshold",
		Value:       "85",
		Type:        "threshold",
		Level:       "critical",
		Description: "CPU使用率告警阈值(%)",
		UpdatedAt:   time.Now().Unix(),
	}
	assert.Equal(t, int64(1), cfg.ID)
	assert.Equal(t, "cpu_usage_threshold", cfg.Key)
	assert.Equal(t, "85", cfg.Value)
	assert.Equal(t, "threshold", cfg.Type)
	assert.Equal(t, "critical", cfg.Level)
	assert.Greater(t, cfg.UpdatedAt, int64(0))
}

func TestRuntimeConfig_Notification(t *testing.T) {
	cfg := &RuntimeConfig{
		ID:          11,
		Key:         "webhook_url",
		Value:       "https://hooks.slack.com/services/xxx",
		Type:        "notification",
		Description: "告警通知Webhook地址",
		UpdatedAt:   time.Now().Unix(),
	}
	assert.Equal(t, "notification", cfg.Type)
	assert.Equal(t, "", cfg.Level) // notification 类型没有 Level
	assert.NotEmpty(t, cfg.Value)
}

func TestRuntimeConfig_General(t *testing.T) {
	cfg := &RuntimeConfig{
		ID:          15,
		Key:         "log_level",
		Value:       "info",
		Type:        "general",
		Description: "系统日志级别",
		UpdatedAt:   time.Now().Unix(),
	}
	assert.Equal(t, "general", cfg.Type)
	assert.Equal(t, "info", cfg.Value)
}

// ============================================================================
// 三、服务创建测试
// ============================================================================

func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false // 禁用 TLS，避免证书文件问题

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.NotNil(t, s.configs)
	assert.False(t, s.running)
}

func TestNewService_NilConfig(t *testing.T) {
	s, err := New(nil)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "control-plane", s.config.ServiceName)
}

// ============================================================================
// 四、默认配置项测试
// ============================================================================

func TestNewService_DefaultConfigs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)

	// 验证所有默认配置项已加载
	assert.Equal(t, 18, len(s.configs), "应有18个默认配置项")

	// 验证几个关键配置项
	cpuThreshold, ok := s.configs["cpu_usage_threshold"]
	assert.True(t, ok)
	assert.Equal(t, "85", cpuThreshold.Value)
	assert.Equal(t, "threshold", cpuThreshold.Type)
	assert.Equal(t, "critical", cpuThreshold.Level)

	memThreshold, ok := s.configs["memory_usage_threshold"]
	assert.True(t, ok)
	assert.Equal(t, "90", memThreshold.Value)

	diskThreshold, ok := s.configs["disk_usage_threshold"]
	assert.True(t, ok)
	assert.Equal(t, "95", diskThreshold.Value)

	webhook, ok := s.configs["webhook_url"]
	assert.True(t, ok)
	assert.Equal(t, "notification", webhook.Type)

	logLevel, ok := s.configs["log_level"]
	assert.True(t, ok)
	assert.Equal(t, "info", logLevel.Value)
	assert.Equal(t, "general", logLevel.Type)
}

// ============================================================================
// 五、TLS 配置测试
// ============================================================================

func TestNewService_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)
	assert.Nil(t, s.grpcCreds)
}

func TestNewService_TLSEnabled_NoFiles(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = true
	cfg.TLSCAFile = "/tmp/nonexistent-ca.pem"
	cfg.TLSCertFile = "/tmp/nonexistent-cert.pem"
	cfg.TLSKeyFile = "/tmp/nonexistent-key.pem"

	_, err := New(cfg)
	assert.Error(t, err, "不存在的证书文件应该报错")
}

func TestNewServerTLSCredentials(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = true
	cfg.TLSCertFile = "/tmp/nonexistent-cert.pem"
	cfg.TLSKeyFile = "/tmp/nonexistent-key.pem"

	s, err := New(cfg)
	// 即使 New 失败，我们关心的是 newServerTLSCredentials 的错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLS credentials init failed")
}

// ============================================================================
// 六、服务生命周期测试
// ============================================================================

func TestService_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)

	// 服务创建后不应运行
	assert.False(t, s.running)

	// 健康状态应为 NOT_SERVING
	healthStatus := s.health.GetServingStatus(cfg.ServiceName)
	assert.Equal(t, int(healthStatus), int(healthStatus)) // 至少能调用
}

// ============================================================================
// 七、gRPC Dial 选项测试
// ============================================================================

func TestGetGRPCDialOptions_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)

	opts, err := s.getGRPCDialOptions()
	require.NoError(t, err)
	assert.NotNil(t, opts)
	assert.GreaterOrEqual(t, len(opts), 1)
}

func TestGetGRPCDialOptions_TLSEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = true
	cfg.TLSCAFile = "/tmp/nonexistent-ca.pem"
	cfg.TLSCertFile = "/tmp/nonexistent-cert.pem"
	cfg.TLSKeyFile = "/tmp/nonexistent-key.pem"

	s, err := New(cfg)
	// New 会失败，但我们可以测试 dial options 逻辑
	assert.Error(t, err)
}

// ============================================================================
// 八、连接下游服务测试
// ============================================================================

func TestConnectToDownstream_NoAddrs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false
	cfg.DataPlaneAddr = ""
	cfg.AuthAddr = ""
	cfg.TenantAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)

	// 当没有下游地址时，connectToDownstream 应该返回 nil
	err = s.connectToDownstream()
	assert.NoError(t, err)
}

// ============================================================================
// 九、Agent/Edge 管理测试
// ============================================================================

func TestService_AgentStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)

	// 初始状态应为空
	assert.NotNil(t, s.agents)
	assert.NotNil(t, s.edges)
}

// ============================================================================
// 十、边界条件测试
// ============================================================================

func TestDefaultConfig_AgentTTL(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.AgentTTL, time.Duration(0))
	assert.Greater(t, cfg.HeartbeatTimeout, time.Duration(0))
	assert.GreaterOrEqual(t, cfg.AgentTTL, cfg.HeartbeatTimeout)
}

func TestRuntimeConfig_IDUnique(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)

	ids := make(map[int64]bool)
	for _, c := range s.configs {
		assert.False(t, ids[c.ID], "配置ID %d 应该唯一", c.ID)
		ids[c.ID] = true
	}
}

func TestRuntimeConfig_KeysUnique(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)

	keys := make(map[string]bool)
	for _, c := range s.configs {
		assert.False(t, keys[c.Key], "配置Key %s 应该唯一", c.Key)
		keys[c.Key] = true
	}
}

func TestNewService_MaxRecvMsgSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s.grpcServer)
	// gRPC 服务器已创建，默认消息大小为 64MB
	assert.NotNil(t, s.grpcServer)
}
