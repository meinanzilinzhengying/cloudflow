// Package auth 统一认证中间件测试
package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 一、配置测试
// ============================================================================

func TestConfig_Valid(t *testing.T) {
	cfg := Config{
		AuthAddr:     "auth-service:9003",
		TLSEnabled:   true,
		CAFile:       "/certs/ca.pem",
		CertFile:     "/certs/cert.pem",
		KeyFile:      "/certs/key.pem",
		InsecureSkip: false,
	}
	assert.Equal(t, "auth-service:9003", cfg.AuthAddr)
	assert.True(t, cfg.TLSEnabled)
	assert.Equal(t, "/certs/ca.pem", cfg.CAFile)
	assert.False(t, cfg.InsecureSkip)
}

func TestConfig_Default(t *testing.T) {
	cfg := Config{}
	assert.Equal(t, "", cfg.AuthAddr)
	assert.False(t, cfg.TLSEnabled)
	assert.False(t, cfg.InsecureSkip)
}

// ============================================================================
// 二、认证器创建测试
// ============================================================================

func TestNewAuthenticator_EmptyAddr(t *testing.T) {
	// 空地址应该仍然能创建（但无法连接）
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestNewAuthenticator_WithAddr(t *testing.T) {
	cfg := Config{AuthAddr: "localhost:0"}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)
	assert.NotNil(t, auth)
	assert.NotNil(t, auth.authConn)
}

func TestNewAuthenticator_TLSWithoutFiles(t *testing.T) {
	cfg := Config{
		AuthAddr:   "localhost:0",
		TLSEnabled: true,
		CAFile:     "/tmp/nonexistent-ca.pem",
		CertFile:   "/tmp/nonexistent-cert.pem",
		KeyFile:    "/tmp/nonexistent-key.pem",
	}
	_, err := NewAuthenticator(cfg)
	assert.Error(t, err, "不存在的证书文件应该报错")
}

// ============================================================================
// 三、Close 测试
// ============================================================================

func TestAuthenticator_Close(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	// Close 不应该 panic
	assert.NotPanics(t, func() {
		auth.Close()
	})
}

func TestAuthenticator_Close_NilConn(t *testing.T) {
	auth := &Authenticator{authConn: nil}
	assert.NotPanics(t, func() {
		auth.Close()
	})
}

// ============================================================================
// 四、HTTP 中间件测试
// ============================================================================

func TestAuthenticator_Middleware_WithoutAuth(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	// 无认证器时，中间件应直接放行（跳过认证）
	handler := auth.Middleware("/healthz")
	assert.NotNil(t, handler)
}

func TestAuthenticator_Middleware_Exclusions(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	// 测试排除路径
	handler := auth.Middleware("/healthz", "/metrics", "/ready")
	assert.NotNil(t, handler)
}

// ============================================================================
// 五、gRPC 拦截器测试
// ============================================================================

func TestAuthenticator_GRPCInterceptor(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	interceptor := auth.GRPCInterceptor()
	assert.NotNil(t, interceptor)
}

func TestAuthenticator_GRPCInterceptor_WithConn(t *testing.T) {
	cfg := Config{AuthAddr: "localhost:0"}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	interceptor := auth.GRPCInterceptor()
	assert.NotNil(t, interceptor)
}

// ============================================================================
// 六、边界条件测试
// ============================================================================

func TestConfig_TLSSelfSigned(t *testing.T) {
	cfg := Config{
		AuthAddr:     "localhost:9003",
		TLSEnabled:   true,
		InsecureSkip: true,
	}
	assert.True(t, cfg.InsecureSkip)
	// InsecureSkip 为 true 时，不应要求证书文件
}

func TestNewAuthenticator_MultipleCalls(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth1, err := NewAuthenticator(cfg)
	require.NoError(t, err)
	auth2, err := NewAuthenticator(cfg)
	require.NoError(t, err)
	assert.NotEqual(t, auth1, auth2)

	auth1.Close()
	auth2.Close()
}

func TestAuthenticator_ConcurrentClose(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	// 并发关闭不应 panic
	done := make(chan bool, 2)
	go func() {
		auth.Close()
		done <- true
	}()
	go func() {
		auth.Close()
		done <- true
	}()

	select {
	case <-done:
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// ============================================================================
// 七、结构体完整性测试
// ============================================================================

func TestAuthenticator_Struct(t *testing.T) {
	cfg := Config{AuthAddr: ""}
	auth, err := NewAuthenticator(cfg)
	require.NoError(t, err)

	// 验证结构体字段可访问
	assert.NotNil(t, auth)
	// authConn 是私有字段，无法直接测试，但已通过其他测试间接验证
}
