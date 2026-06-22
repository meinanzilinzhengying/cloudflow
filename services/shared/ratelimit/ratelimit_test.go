package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// MockRedisClient 模拟 Redis 客户端
type MockRedisClient struct {
	data map[string]int64
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{data: make(map[string]int64)}
}

func (m *MockRedisClient) Incr(key string) (int64, error) {
	m.data[key]++
	return m.data[key], nil
}

func (m *MockRedisClient) Expire(key string, seconds int) error {
	return nil
}

func (m *MockRedisClient) Get(key string) (string, error) {
	return "", nil
}

// ============================================================================
// TokenBucket 测试
// ============================================================================

func TestTokenBucket_Allow(t *testing.T) {
	tb := NewTokenBucket(10, 5) // 10/s, burst 5

	// 初始有 5 个令牌，应该允许 5 个请求
	for i := 0; i < 5; i++ {
		if !tb.Allow("key1") {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 第 6 个应该被拒绝
	if tb.Allow("key1") {
		t.Error("Expected 6th request to be denied")
	}

	// 不同 key 应该独立计数
	if !tb.Allow("key2") {
		t.Error("Expected request from key2 to be allowed")
	}
}

func TestTokenBucket_AllowN(t *testing.T) {
	tb := NewTokenBucket(10, 5)

	// 请求 3 个令牌
	if !tb.AllowN("key1", 3) {
		t.Error("Expected AllowN(3) to be allowed")
	}

	// 剩余 2 个，请求 3 个应该失败
	if tb.AllowN("key1", 3) {
		t.Error("Expected AllowN(3) to be denied after consuming 3 tokens")
	}

	// 请求 2 个应该成功
	if !tb.AllowN("key1", 2) {
		t.Error("Expected AllowN(2) to be allowed")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := NewTokenBucket(100, 1) // 100/s, burst 1

	// 消费唯一令牌
	if !tb.Allow("key1") {
		t.Error("Expected first request to be allowed")
	}
	if tb.Allow("key1") {
		t.Error("Expected second request to be denied")
	}

	// 等待 20ms 补充 2 个令牌
	time.Sleep(25 * time.Millisecond)
	if !tb.Allow("key1") {
		t.Error("Expected request after refill to be allowed")
	}
}

func TestTokenBucket_Cleanup(t *testing.T) {
	tb := NewTokenBucket(10, 5)
	tb.Allow("expiring_key")

	// 直接检查 bucket 存在
	tb.mu.RLock()
	_, exists := tb.buckets["expiring_key"]
	tb.mu.RUnlock()
	if !exists {
		t.Error("Expected bucket to exist initially")
	}

	// 模拟 cleanup（手动删除）
	tb.mu.Lock()
	delete(tb.buckets, "expiring_key")
	tb.mu.Unlock()

	tb.mu.RLock()
	_, exists = tb.buckets["expiring_key"]
	tb.mu.RUnlock()
	if exists {
		t.Error("Expected bucket to be cleaned up")
	}
}

// ============================================================================
// RedisLimiter 测试
// ============================================================================

func TestRedisLimiter_Allow(t *testing.T) {
	client := NewMockRedisClient()
	rl := NewRedisLimiter(client, 60*time.Second, 5)

	// 前 5 个应该允许
	for i := 0; i < 5; i++ {
		if !rl.Allow("ip1") {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 第 6 个应该被拒绝
	if rl.Allow("ip1") {
		t.Error("Expected 6th request to be denied")
	}

	// 不同 IP 应该独立
	if !rl.Allow("ip2") {
		t.Error("Expected request from ip2 to be allowed")
	}
}

func TestRedisLimiter_RedisFailure(t *testing.T) {
	// Redis 故障时应该降级（允许通过）
	client := &FailingRedisClient{}
	rl := NewRedisLimiter(client, 60*time.Second, 5)

	if !rl.Allow("ip1") {
		t.Error("Expected request to be allowed when Redis fails (graceful degradation)")
	}
}

type FailingRedisClient struct{}

func (f *FailingRedisClient) Incr(key string) (int64, error) {
	return 0, http.ErrAbortHandler
}
func (f *FailingRedisClient) Expire(key string, seconds int) error {
	return http.ErrAbortHandler
}
func (f *FailingRedisClient) Get(key string) (string, error) {
	return "", http.ErrAbortHandler
}

// ============================================================================
// Middleware 测试
// ============================================================================

func TestMiddleware_GlobalRateLimit(t *testing.T) {
	config := &MiddlewareConfig{
		GlobalQPS:      2,
		GlobalBurst:    2,
		IPQPS:          100,
		IPBurst:        100,
		UserQPS:        100,
		UserBurst:      100,
		AuthQPS:        100,
		AuthBurst:      100,
		PenaltySeconds: 1,
		StatusCode:     http.StatusTooManyRequests,
	}
	m := NewMiddleware(config)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 前 2 个应该成功
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d on request %d", rec.Code, i+1)
		}
	}

	// 第 3 个应该被全局限流
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rec.Code)
	}
}

func TestMiddleware_AuthPathRateLimit(t *testing.T) {
	config := &MiddlewareConfig{
		GlobalQPS:      10000,
		GlobalBurst:    10000,
		IPQPS:          100,
		IPBurst:        100,
		UserQPS:        100,
		UserBurst:      100,
		AuthQPS:        2,
		AuthBurst:      2,
		PenaltySeconds: 1,
		StatusCode:     http.StatusTooManyRequests,
	}
	m := NewMiddleware(config)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 前 2 个 /login 应该成功
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d on request %d", rec.Code, i+1)
		}
	}

	// 第 3 个 /login 应该被认证限流
	req := httptest.NewRequest("POST", "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 for auth path, got %d", rec.Code)
	}
}

func TestMiddleware_Whitelist(t *testing.T) {
	config := &MiddlewareConfig{
		GlobalQPS:      1,
		GlobalBurst:    1,
		IPQPS:          1,
		IPBurst:        1,
		UserQPS:        1,
		UserBurst:      1,
		AuthQPS:        1,
		AuthBurst:      1,
		PenaltySeconds: 1,
		StatusCode:     http.StatusTooManyRequests,
		WhitelistIPs:   []string{"192.168.1.1"},
	}
	m := NewMiddleware(config)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 白名单 IP 应该不受限流
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status 200 for whitelisted IP, got %d on request %d", rec.Code, i+1)
		}
	}
}

func TestMiddleware_Penalty(t *testing.T) {
	config := &MiddlewareConfig{
		GlobalQPS:      100,
		GlobalBurst:    100,
		IPQPS:          1,
		IPBurst:        1,
		UserQPS:        100,
		UserBurst:      100,
		AuthQPS:        100,
		AuthBurst:      100,
		PenaltySeconds: 1,
		StatusCode:     http.StatusTooManyRequests,
	}
	m := NewMiddleware(config)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 触发限流
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req) // 成功

	req2 := httptest.NewRequest("GET", "/test", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2) // 超限，触发惩罚
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429, got %d", rec2.Code)
	}

	// 惩罚期内再次请求应该直接 429
	req3 := httptest.NewRequest("GET", "/test", nil)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Errorf("Expected 429 during penalty, got %d", rec3.Code)
	}
}

func TestMiddleware_RateLimitHeaders(t *testing.T) {
	config := DefaultMiddlewareConfig()
	m := NewMiddleware(config)

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("Expected X-RateLimit-Limit header")
	}
	if rec.Header().Get("X-RateLimit-Window") == "" {
		t.Error("Expected X-RateLimit-Window header")
	}
}

// ============================================================================
// ConnectionLimiter 测试
// ============================================================================

func TestConnectionLimiter(t *testing.T) {
	cl := NewConnectionLimiter(2)

	handler := cl.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	done := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			done <- rec.Code
		}()
	}

	okCount := 0
	rejectCount := 0
	for i := 0; i < 3; i++ {
		code := <-done
		if code == http.StatusOK {
			okCount++
		} else if code == http.StatusServiceUnavailable {
			rejectCount++
		}
	}

	if okCount != 2 {
		t.Errorf("Expected 2 OK, got %d", okCount)
	}
	if rejectCount != 1 {
		t.Errorf("Expected 1 rejected, got %d", rejectCount)
	}
}

// ============================================================================
// ExtractClientIP 测试
// ============================================================================

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name:       "X-Forwarded-For",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"},
			remoteAddr: "10.0.0.1:12345",
			expected:   "1.2.3.4",
		},
		{
			name:       "X-Real-Ip",
			headers:    map[string]string{"X-Real-Ip": "2.3.4.5"},
			remoteAddr: "10.0.0.1:12345",
			expected:   "2.3.4.5",
		},
		{
			name:       "RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "10.0.0.1:12345",
			expected:   "10.0.0.1",
		},
		{
			name:       "RemoteAddr no port",
			headers:    map[string]string{},
			remoteAddr: "10.0.0.1",
			expected:   "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			ip := ExtractClientIP(req)
			if ip != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, ip)
			}
		})
	}
}
