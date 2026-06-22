// Package dataplane 数据平面核心逻辑测试
package dataplane

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
	assert.Equal(t, "data-plane", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, ":9002", cfg.GrpcAddr)
	assert.Equal(t, ":9102", cfg.MetricsAddr)
	assert.Equal(t, 10000, cfg.BatchSize)
	assert.Equal(t, time.Second, cfg.FlushInterval)
	assert.Equal(t, 100000, cfg.QueueSize)
	assert.Equal(t, 4, cfg.WorkerCount)
	assert.False(t, cfg.TLSEnabled)
	assert.False(t, cfg.TLSInsecureSkip)
	assert.NotNil(t, cfg.Sampling)
	assert.Equal(t, "http://victoriametrics:8428", cfg.VictoriaMetricsAddr)
	assert.Equal(t, "http://loki:3100", cfg.LokiAddr)
}

func TestDefaultConfig_NilSafe(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Sampling)
}

// ============================================================================
// 二、HTTP 方法转换测试
// ============================================================================

func TestHTTPMethodToString(t *testing.T) {
	tests := []struct {
		method   uint8
		expected string
	}{
		{1, "GET"},
		{2, "POST"},
		{3, "PUT"},
		{4, "DELETE"},
		{5, "HEAD"},
		{6, "OPTIONS"},
		{7, "PATCH"},
		{0, "UNKNOWN"},
		{8, "UNKNOWN"},
		{255, "UNKNOWN"},
	}

	for _, tt := range tests {
		result := httpMethodToString(tt.method)
		assert.Equal(t, tt.expected, result, "method %d should be %s", tt.method, tt.expected)
	}
}

func TestHTTPMethodToString_Exhaustive(t *testing.T) {
	// 验证所有方法值
	assert.Equal(t, "GET", httpMethodToString(1))
	assert.Equal(t, "POST", httpMethodToString(2))
	assert.Equal(t, "PUT", httpMethodToString(3))
	assert.Equal(t, "DELETE", httpMethodToString(4))
	assert.Equal(t, "HEAD", httpMethodToString(5))
	assert.Equal(t, "OPTIONS", httpMethodToString(6))
	assert.Equal(t, "PATCH", httpMethodToString(7))
	// 边界值
	assert.Equal(t, "UNKNOWN", httpMethodToString(0))
	assert.Equal(t, "UNKNOWN", httpMethodToString(9))
	assert.Equal(t, "UNKNOWN", httpMethodToString(100))
}

// ============================================================================
// 三、服务创建测试
// ============================================================================

func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = "" // 跳过认证中间件

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.NotNil(t, s.samplingEngine)
	assert.NotNil(t, s.flowQueue)
	assert.NotNil(t, s.metricQueue)
	assert.NotNil(t, s.traceQueue)
	assert.NotNil(t, s.logQueue)
}

func TestNewService_NilConfig(t *testing.T) {
	s, err := New(nil)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "data-plane", s.config.ServiceName)
}

func TestNewService_NilSampling(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sampling = nil
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config.Sampling)
}

// ============================================================================
// 四、Stats 统计测试
// ============================================================================

func TestStats_Zero(t *testing.T) {
	var stats Stats
	assert.Equal(t, uint64(0), stats.FlowsIngested)
	assert.Equal(t, uint64(0), stats.MetricsIngested)
	assert.Equal(t, uint64(0), stats.TracesIngested)
	assert.Equal(t, uint64(0), stats.LogsIngested)
	assert.Equal(t, uint64(0), stats.FlowsDropped)
	assert.Equal(t, uint64(0), stats.FlowsSampled)
	assert.Equal(t, uint64(0), stats.FlowsWritten)
	assert.Equal(t, uint64(0), stats.WriteErrors)
	assert.Equal(t, uint64(0), stats.AvgLatencyMs)
}

func TestStats_Increment(t *testing.T) {
	var stats Stats
	stats.FlowsIngested++
	stats.MetricsIngested += 10
	stats.FlowsWritten += 5
	stats.WriteErrors++
	stats.AvgLatencyMs = 25

	assert.Equal(t, uint64(1), stats.FlowsIngested)
	assert.Equal(t, uint64(10), stats.MetricsIngested)
	assert.Equal(t, uint64(5), stats.FlowsWritten)
	assert.Equal(t, uint64(1), stats.WriteErrors)
	assert.Equal(t, uint64(25), stats.AvgLatencyMs)
}

// ============================================================================
// 五、TLS 配置测试
// ============================================================================

func TestNewService_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false
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
	cfg.AuthAddr = ""

	_, err := New(cfg)
	assert.Error(t, err, "不存在的 TLS 文件应该报错")
}

// ============================================================================
// 六、服务生命周期测试
// ============================================================================

func TestService_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.GrpcAddr = "127.0.0.1:0"
	cfg.MetricsAddr = "127.0.0.1:0"

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)

	// 验证服务创建后 running 为 false
	assert.False(t, s.running.Load())
}

// ============================================================================
// 七、队列容量测试
// ============================================================================

func TestQueueCapacity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.QueueSize = 100
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.Equal(t, 100, cap(s.flowQueue))
	assert.Equal(t, 100, cap(s.metricQueue))
	assert.Equal(t, 100, cap(s.traceQueue))
	assert.Equal(t, 100, cap(s.logQueue))
}

// ============================================================================
// 八、HTTP 客户端测试
// ============================================================================

func TestNewService_VMClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VictoriaMetricsAddr = "http://vm:8428"
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s.vmHTTPClient)
	assert.Equal(t, 30*time.Second, s.vmHTTPClient.Timeout)
}

func TestNewService_LokiClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LokiAddr = "http://loki:3100"
	cfg.AuthAddr = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s.lokiHTTPClient)
	assert.Equal(t, 30*time.Second, s.lokiHTTPClient.Timeout)
}

// ============================================================================
// 九、配置边界值测试
// ============================================================================

func TestDefaultConfig_BatchSize(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.BatchSize, 0)
	assert.Greater(t, cfg.FlushInterval, time.Duration(0))
	assert.Greater(t, cfg.QueueSize, 0)
	assert.Greater(t, cfg.WorkerCount, 0)
}

func TestDefaultConfig_DatabaseDefaults(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "clickhouse", cfg.TimeSeriesDBHost)
	assert.Equal(t, 9000, cfg.TimeSeriesDBPort)
	assert.Equal(t, "default", cfg.TimeSeriesDBUser)
	assert.Equal(t, "cloudflow", cfg.TimeSeriesDBDatabase)
}
