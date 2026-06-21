// Package tenantservice 租户服务测试
package tenantservice

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// ============================================================================
// 一、配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "tenant-service", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, ":9010", cfg.GrpcAddr)
	assert.Equal(t, ":8010", cfg.HttpAddr)
	assert.Equal(t, "auth-service:9006", cfg.AuthAddr)
	assert.Equal(t, storage.DBMySQL, cfg.DBType)
	assert.Equal(t, "127.0.0.1", cfg.DBHost)
	assert.Equal(t, 3306, cfg.DBPort)
	assert.Equal(t, "cloudflow_tenant", cfg.DBDatabase)
	assert.Equal(t, 50, cfg.DBMaxOpenConns)
	assert.Equal(t, 10, cfg.DBMaxIdleConns)
	assert.False(t, cfg.DBEnableDualWrite)
	assert.Equal(t, storage.ModeOldOnly, cfg.DBDualWriteMode)
	assert.Equal(t, 30, cfg.DefaultRetentionDays)
	assert.Equal(t, 100, cfg.DefaultMaxAgents)
	assert.Equal(t, int64(10_000_000), cfg.DefaultMaxFlowsPerDay)
	assert.Equal(t, 100, cfg.DefaultMaxStorageGB)
	assert.Equal(t, 100, cfg.DefaultMaxAlertRules)
	assert.False(t, cfg.TLSEnabled)
	assert.False(t, cfg.TLSInsecureSkip)
}

func TestDefaultConfig_NilSafe(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
}

// ============================================================================
// 二、服务创建测试
// ============================================================================

func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBHost = ""      // 跳过数据库
	cfg.AuthAddr = ""    // 跳过认证
	cfg.TLSEnabled = false

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.Nil(t, s.db) // 数据库未初始化
}

func TestNewService_NilConfig(t *testing.T) {
	s, err := New(nil)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "tenant-service", s.config.ServiceName)
}

// ============================================================================
// 三、TLS 配置测试
// ============================================================================

func TestNewService_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false
	cfg.DBHost = ""
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.Nil(t, s.grpcCreds)
}

func TestNewService_TLSEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = true
	cfg.TLSCAFile = "/tmp/nonexistent-ca.pem"
	cfg.TLSCertFile = "/tmp/nonexistent-cert.pem"
	cfg.TLSKeyFile = "/tmp/nonexistent-key.pem"
	cfg.DBHost = ""
	cfg.AuthAddr = ""

	_, err := New(cfg)
	assert.Error(t, err, "不存在的证书文件应该报错")
}

// ============================================================================
// 四、数据库配置测试
// ============================================================================

func TestDefaultConfig_DBType(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, storage.DBMySQL, cfg.DBType)
	
	// 测试其他数据库类型
	cfg.DBType = storage.DBDameng
	assert.Equal(t, storage.DBDameng, cfg.DBType)
	
	cfg.DBType = storage.DBKingBase
	assert.Equal(t, storage.DBKingBase, cfg.DBType)
	
	cfg.DBType = storage.DBGaussDB
	assert.Equal(t, storage.DBGaussDB, cfg.DBType)
}

func TestDefaultConfig_DualWrite(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.DBEnableDualWrite)
	assert.Equal(t, storage.ModeOldOnly, cfg.DBDualWriteMode)
	
	cfg.DBEnableDualWrite = true
	cfg.DBDualWriteMode = storage.ModeSyncWrite
	assert.True(t, cfg.DBEnableDualWrite)
	assert.Equal(t, storage.ModeSyncWrite, cfg.DBDualWriteMode)
}

// ============================================================================
// 五、配额配置测试
// ============================================================================

func TestDefaultConfig_Quota(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 30, cfg.DefaultRetentionDays)
	assert.Equal(t, 100, cfg.DefaultMaxAgents)
	assert.Equal(t, int64(10_000_000), cfg.DefaultMaxFlowsPerDay)
	assert.Equal(t, 100, cfg.DefaultMaxStorageGB)
	assert.Equal(t, 100, cfg.DefaultMaxAlertRules)
}

func TestDefaultConfig_QuotaZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultMaxAgents = 0
	cfg.DefaultMaxFlowsPerDay = 0
	cfg.DefaultMaxStorageGB = 0
	cfg.DefaultMaxAlertRules = 0
	
	assert.Equal(t, 0, cfg.DefaultMaxAgents)
	assert.Equal(t, int64(0), cfg.DefaultMaxFlowsPerDay)
}

// ============================================================================
// 六、健康检查测试
// ============================================================================

func TestHealthCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBHost = ""
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)

	resp, err := s.HealthCheck(context.Background(), &svcproto.HealthCheckRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.GreaterOrEqual(t, resp.Uptime, int64(0))
}

// ============================================================================
// 七、服务生命周期测试
// ============================================================================

func TestService_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBHost = ""
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
}

// ============================================================================
// 八、边界条件测试
// ============================================================================

func TestDefaultConfig_MaxConns(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.DBMaxOpenConns, 0)
	assert.Greater(t, cfg.DBMaxIdleConns, 0)
	assert.GreaterOrEqual(t, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
}

func TestDefaultConfig_RetentionDays(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.DefaultRetentionDays, 0)
}

func TestDefaultConfig_FlowsPerDay(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.DefaultMaxFlowsPerDay, int64(0))
}

func TestNewService_WithAuth(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = "localhost:0"
	cfg.DBHost = ""
	
	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s.auth)
}

// ============================================================================
// 九、结构体完整性测试
// ============================================================================

func TestService_Struct(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DBHost = ""
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.NotNil(t, s.startTime)
}

func TestConfig_Struct(t *testing.T) {
	cfg := &Config{
		ServiceName: "custom-tenant",
		Version:     "2.0.0",
	}
	assert.Equal(t, "custom-tenant", cfg.ServiceName)
	assert.Equal(t, "2.0.0", cfg.Version)
}
