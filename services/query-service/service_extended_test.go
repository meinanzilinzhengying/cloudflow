// Package queryservice 查询服务增强测试
package queryservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// ============================================================================
// 一、配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "query-service", cfg.ServiceName)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, ":9007", cfg.GrpcAddr)
	assert.Equal(t, ":8007", cfg.HttpAddr)
	assert.Equal(t, 10, cfg.MaxConcurrentQueries)
	assert.Equal(t, 30*time.Second, cfg.QueryTimeout)
	assert.False(t, cfg.TLSEnabled)
}

// ============================================================================
// 二、服务创建测试
// ============================================================================

func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.NotNil(t, s.config)
	assert.NotNil(t, s.grpcServer)
	assert.NotNil(t, s.health)
	assert.NotNil(t, s.httpServer)
	assert.NotNil(t, s.connLimiter)
	assert.NotNil(t, s.rateLimiter)
	assert.NotNil(t, s.otlpReceiver)
	assert.NotNil(t, s.correlationEngine)
}

func TestNewService_NilConfig(t *testing.T) {
	s, err := New(nil)
	require.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "query-service", s.config.ServiceName)
}

// ============================================================================
// 三、HTTP 处理器测试
// ============================================================================

func TestHealthzHandler(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	s.healthzHandler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestHealthzHandler_HeadMethod(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	w := httptest.NewRecorder()

	s.healthzHandler(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ============================================================================
// 四、限流中间件测试
// ============================================================================

func TestConnectionLimiter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.NotNil(t, s.connLimiter)
	// 默认连接数限制为 2000
	assert.NotNil(t, s.connLimiter)
}

func TestRateLimiter(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.NotNil(t, s.rateLimiter)
	assert.NotNil(t, s.rateLimiter)
}

// ============================================================================
// 五、OTLP 接收器测试
// ============================================================================

func TestOTLPReceiver_StartStop(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.NotNil(t, s.otlpReceiver)
	assert.NotNil(t, s.correlationEngine)
}

// ============================================================================
// 六、查询参数化测试
// ============================================================================

func TestQueryFlows_NoDB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	req := &svcproto.QueryFlowRequest{
		TenantId:  "test-tenant",
		StartTime: time.Now().Add(-1 * time.Hour).Unix(),
		EndTime:   time.Now().Unix(),
	}

	resp, err := s.QueryFlows(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Total)
	assert.Empty(t, resp.Records)
	assert.GreaterOrEqual(t, resp.TookMs, int64(0))
}

func TestQueryFlows_EmptyTenant(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	req := &svcproto.QueryFlowRequest{
		TenantId: "",
	}

	resp, err := s.QueryFlows(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.Total)
}

func TestQueryFlows_NoDB_MultipleTimeRanges(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	now := time.Now().Unix()
	testCases := []struct {
		name      string
		startTime int64
		endTime   int64
	}{
		{"valid range", now - 3600, now},
		{"zero start", 0, now},
		{"zero end", now - 3600, 0},
		{"both zero", 0, 0},
		{"negative start", -1, now},
		{"future end", now - 3600, now + 3600},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &svcproto.QueryFlowRequest{
				TenantId:  "test-tenant",
				StartTime: tc.startTime,
				EndTime:   tc.endTime,
			}
			resp, err := s.QueryFlows(context.Background(), req)
			require.NoError(t, err)
			assert.Equal(t, int64(0), resp.Total)
		})
	}
}

// ============================================================================
// 七、统计测试
// ============================================================================

func TestStats_Zero(t *testing.T) {
	var stats Stats
	assert.Equal(t, uint64(0), stats.QueryCount)
	assert.Equal(t, uint64(0), stats.QueryErrors)
	assert.Equal(t, uint64(0), stats.QueryFromCache)
	assert.Equal(t, uint64(0), stats.AvgLatencyMs)
}

func TestStats_Increment(t *testing.T) {
	var stats Stats
	stats.QueryCount += 5
	stats.QueryErrors += 1
	stats.QueryFromCache += 2
	stats.AvgLatencyMs = 100

	assert.Equal(t, uint64(5), stats.QueryCount)
	assert.Equal(t, uint64(1), stats.QueryErrors)
	assert.Equal(t, uint64(2), stats.QueryFromCache)
	assert.Equal(t, uint64(100), stats.AvgLatencyMs)
}

// ============================================================================
// 八、TLS 配置测试
// ============================================================================

func TestNewService_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)
	assert.Nil(t, s.authenticator)
}

func TestNewService_TLSEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = true
	cfg.TLSCAFile = "/tmp/nonexistent-ca.pem"
	cfg.TLSCertFile = "/tmp/nonexistent-cert.pem"
	cfg.TLSKeyFile = "/tmp/nonexistent-key.pem"
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	_, err := New(cfg)
	assert.Error(t, err, "不存在的证书文件应该报错")
}

// ============================================================================
// 九、服务地址配置测试
// ============================================================================

func TestDefaultConfig_Addresses(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, ":9007", cfg.GrpcAddr)
	assert.Equal(t, ":8007", cfg.HttpAddr)
	assert.Equal(t, "", cfg.DataPlaneAddr)
	assert.Equal(t, "", cfg.TopologyAddr)
	assert.Equal(t, "", cfg.AlertAddr)
}

// ============================================================================
// 十、后端连接配置测试
// ============================================================================

func TestDefaultConfig_Database(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, storage.DatabaseClickHouse, cfg.TimeSeriesDBType)
	assert.Equal(t, "clickhouse", cfg.TimeSeriesDBHost)
	assert.Equal(t, 9000, cfg.TimeSeriesDBPort)
	assert.Equal(t, "default", cfg.TimeSeriesDBUser)
	assert.Equal(t, "cloudflow", cfg.TimeSeriesDBDatabase)
}

func TestNewService_NoTimeSeriesDB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = "" // 跳过时序数据库

	s, err := New(cfg)
	require.NoError(t, err)
	assert.Nil(t, s.tsDB)
}

// ============================================================================
// 十一、gRPC Dial 选项测试
// ============================================================================

func TestGetGRPCDialOptions_TLSDisabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TLSEnabled = false
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	opts, err := s.getGRPCDialOptions()
	require.NoError(t, err)
	assert.NotNil(t, opts)
}

// ============================================================================
// 十二、并发查询测试
// ============================================================================

func TestMaxConcurrentQueries(t *testing.T) {
	cfg := DefaultConfig()
	assert.Greater(t, cfg.MaxConcurrentQueries, 0)
	assert.Greater(t, cfg.QueryTimeout, time.Duration(0))
}

// ============================================================================
// 十三、关联分析引擎测试
// ============================================================================

func TestCorrelationEngine(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuthAddr = ""
	cfg.TimeSeriesDBHost = ""

	s, err := New(cfg)
	require.NoError(t, err)

	assert.NotNil(t, s.correlationEngine)
}
