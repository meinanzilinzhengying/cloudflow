//go:build linux

package gateway_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/gateway"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/servicemesh"
)

func TestRouteRegistry(t *testing.T) {
	registry := gateway.NewRouteRegistry()

	route1 := &gateway.Route{
		ID:          "route-1",
		Path:        "/api/users",
		Methods:     []string{"GET", "POST"},
		ServiceName: "user-service",
		StripPrefix: false,
		Timeout:     30 * time.Second,
	}

	route2 := &gateway.Route{
		ID:          "route-2",
		Path:        "/api/users/v2",
		Methods:     []string{"GET"},
		ServiceName: "user-service-v2",
		StripPrefix: true,
		Timeout:     10 * time.Second,
	}

	// 测试注册
	if err := registry.Register(route1); err != nil {
		t.Fatalf("Register route1 failed: %v", err)
	}
	if err := registry.Register(route2); err != nil {
		t.Fatalf("Register route2 failed: %v", err)
	}

	// 测试重复注册
	if err := registry.Register(route1); err == nil {
		t.Fatal("Expected error for duplicate route")
	}

	// 测试匹配（长路径优先）
	matched := registry.Match("/api/users/v2/profile", "GET")
	if matched == nil {
		t.Fatal("Expected match for /api/users/v2/profile")
	}
	if matched.ID != "route-2" {
		t.Fatalf("Expected route-2, got %s", matched.ID)
	}

	// 测试前缀匹配
	matched = registry.Match("/api/users/list", "GET")
	if matched == nil {
		t.Fatal("Expected match for /api/users/list")
	}
	if matched.ID != "route-1" {
		t.Fatalf("Expected route-1, got %s", matched.ID)
	}

	// 测试方法不匹配
	matched = registry.Match("/api/users/list", "DELETE")
	if matched != nil {
		t.Fatalf("Expected no match for DELETE, got %s", matched.ID)
	}

	// 测试移除
	registry.Remove("route-1")
	matched = registry.Match("/api/users/list", "GET")
	if matched != nil {
		t.Fatalf("Expected no match after remove, got %s", matched.ID)
	}

	// 测试获取所有路由
	all := registry.GetAll()
	if len(all) != 1 {
		t.Fatalf("Expected 1 route after remove, got %d", len(all))
	}
}

func TestRateLimiter(t *testing.T) {
	limiter := gateway.NewMemoryRateLimiter()

	key := "test-key"
	limit := 5
	window := 1 * time.Second

	// 前5次应该允许
	for i := 0; i < 5; i++ {
		if !limiter.Allow(key, limit, window) {
			t.Fatalf("Expected allow for request %d", i+1)
		}
	}

	// 第6次应该拒绝
	if limiter.Allow(key, limit, window) {
		t.Fatal("Expected deny for 6th request")
	}

	// 检查剩余配额
	remaining := limiter.GetRemaining(key, limit, window)
	if remaining != 0 {
		t.Fatalf("Expected 0 remaining, got %d", remaining)
	}

	// 等待窗口过期
	time.Sleep(1100 * time.Millisecond)

	// 应该再次允许
	if !limiter.Allow(key, limit, window) {
		t.Fatal("Expected allow after window expiration")
	}
}

func TestJWTAuthProvider(t *testing.T) {
	provider := gateway.NewJWTAuthProvider("secret", "cloudflow")

	// 测试有效token
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	result, err := provider.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if result.UserID != "user-123" {
		t.Fatalf("Expected user-123, got %s", result.UserID)
	}
	if result.Username != "admin" {
		t.Fatalf("Expected admin, got %s", result.Username)
	}

	// 测试缺少header
	req2 := httptest.NewRequest("GET", "/api/test", nil)
	_, err = provider.Authenticate(req2)
	if err == nil {
		t.Fatal("Expected error for missing authorization")
	}

	// 测试无效token
	req3 := httptest.NewRequest("GET", "/api/test", nil)
	req3.Header.Set("Authorization", "Bearer invalid")
	_, err = provider.Authenticate(req3)
	if err == nil {
		t.Fatal("Expected error for invalid token")
	}
}

func TestGateway(t *testing.T) {
	// 创建后端服务
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "true")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Backend response: %s %s", r.Method, r.URL.Path)
	}))
	defer backend.Close()

	// 创建注册中心和网关
	registry := servicemesh.NewMemoryRegistry()
	
	// 注册后端服务实例
	backendInst := &servicemesh.ServiceInstance{
		ID:     "backend-1",
		Name:   "backend-service",
		Host:   "127.0.0.1",
		Port:   parsePort(backend.URL),
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(backendInst)

	gw := gateway.NewGateway("127.0.0.1:0", registry)
	
	// 注册路由
	route := &gateway.Route{
		ID:          "backend-route",
		Path:        "/api/",
		Methods:     []string{"GET", "POST"},
		ServiceName: "backend-service",
		StripPrefix: false,
		Timeout:     5 * time.Second,
		RequireAuth: false,
	}
	if err := gw.RegisterRoute(route); err != nil {
		t.Fatalf("RegisterRoute failed: %v", err)
	}

	// 测试路由匹配
	routes := gw.GetRoutes()
	if len(routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(routes))
	}

	// 测试服务客户端
	client, ok := gw.GetServiceClient("backend-service")
	if !ok {
		t.Fatal("Expected service client for backend-service")
	}
	if client == nil {
		t.Fatal("Expected non-nil service client")
	}

	// 测试统计
	stats := gw.GetStats()
	if stats["route_count"] != 1 {
		t.Fatalf("Expected route_count 1, got %v", stats["route_count"])
	}
	if stats["client_count"] != 1 {
		t.Fatalf("Expected client_count 1, got %v", stats["client_count"])
	}

	// 测试网关HTTP处理
	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	if string(body) != "Backend response: GET /api/test" {
		t.Fatalf("Unexpected body: %s", string(body))
	}

	// 测试未匹配路由
	req2 := httptest.NewRequest("GET", "/unknown/path", nil)
	rec2 := httptest.NewRecorder()
	gw.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", rec2.Code)
	}
}

func TestGatewayWithAuth(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	registry := servicemesh.NewMemoryRegistry()
	backendInst := &servicemesh.ServiceInstance{
		ID:     "backend-1",
		Name:   "backend-service",
		Host:   "127.0.0.1",
		Port:   parsePort(backend.URL),
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(backendInst)

	gw := gateway.NewGateway("127.0.0.1:0", registry)
	gw.SetAuthProvider(gateway.NewJWTAuthProvider("secret", "cloudflow"))

	route := &gateway.Route{
		ID:          "auth-route",
		Path:        "/api/secure",
		Methods:     []string{"GET"},
		ServiceName: "backend-service",
		RequireAuth: true,
		Timeout:     5 * time.Second,
	}
	gw.RegisterRoute(route)

	// 测试无认证
	req := httptest.NewRequest("GET", "/api/secure", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401, got %d", rec.Code)
	}

	// 测试有认证
	req2 := httptest.NewRequest("GET", "/api/secure", nil)
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	gw.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec2.Code)
	}
}

func TestGatewayWithRateLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	registry := servicemesh.NewMemoryRegistry()
	backendInst := &servicemesh.ServiceInstance{
		ID:     "backend-1",
		Name:   "backend-service",
		Host:   "127.0.0.1",
		Port:   parsePort(backend.URL),
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(backendInst)

	gw := gateway.NewGateway("127.0.0.1:0", registry)

	route := &gateway.Route{
		ID:          "ratelimit-route",
		Path:        "/api/limited",
		Methods:     []string{"GET"},
		ServiceName: "backend-service",
		RequireAuth: false,
		Timeout:     5 * time.Second,
		RateLimit: &gateway.RateLimitConfig{
			QPS:    2,
			Window: 1 * time.Second,
		},
	}
	gw.RegisterRoute(route)

	// 前2次应该成功
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/limited", nil)
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 for request %d, got %d", i+1, rec.Code)
		}
	}

	// 第3次应该被限流
	req := httptest.NewRequest("GET", "/api/limited", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429, got %d", rec.Code)
	}

	// 检查限流响应头
	remaining := rec.Header().Get("X-RateLimit-Remaining")
	if remaining != "0" {
		t.Fatalf("Expected remaining 0, got %s", remaining)
	}
}

func TestGatewayStripPrefix(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Path: %s", r.URL.Path)
	}))
	defer backend.Close()

	registry := servicemesh.NewMemoryRegistry()
	backendInst := &servicemesh.ServiceInstance{
		ID:     "backend-1",
		Name:   "backend-service",
		Host:   "127.0.0.1",
		Port:   parsePort(backend.URL),
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(backendInst)

	gw := gateway.NewGateway("127.0.0.1:0", registry)

	route := &gateway.Route{
		ID:          "strip-route",
		Path:        "/api/v1/",
		Methods:     []string{"GET"},
		ServiceName: "backend-service",
		StripPrefix: true,
		Timeout:     5 * time.Second,
	}
	gw.RegisterRoute(route)

	req := httptest.NewRequest("GET", "/api/v1/users/list", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("Expected 200, got %d, body: %s", rec.Code, string(body))
	}

	body, _ := io.ReadAll(rec.Body)
	expected := "Path: /users/list"
	if string(body) != expected {
		t.Fatalf("Expected body '%s', got '%s'", expected, string(body))
	}
}

func TestGatewayRewritePath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Path: %s", r.URL.Path)
	}))
	defer backend.Close()

	registry := servicemesh.NewMemoryRegistry()
	backendInst := &servicemesh.ServiceInstance{
		ID:     "backend-1",
		Name:   "backend-service",
		Host:   "127.0.0.1",
		Port:   parsePort(backend.URL),
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(backendInst)

	gw := gateway.NewGateway("127.0.0.1:0", registry)

	route := &gateway.Route{
		ID:          "rewrite-route",
		Path:        "/api/old",
		Methods:     []string{"GET"},
		ServiceName: "backend-service",
		RewritePath: "/api/new",
		Timeout:     5 * time.Second,
	}
	gw.RegisterRoute(route)

	req := httptest.NewRequest("GET", "/api/old", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Body)
	expected := "Path: /api/new"
	if string(body) != expected {
		t.Fatalf("Expected body '%s', got '%s'", expected, string(body))
	}
}

func TestGatewayMultipleMethods(t *testing.T) {
	registry := gateway.NewRouteRegistry()

	route := &gateway.Route{
		ID:          "multi-method",
		Path:        "/api/resource",
		Methods:     []string{"GET", "POST"},
		ServiceName: "test-service",
		Timeout:     5 * time.Second,
	}
	registry.Register(route)

	// GET 应该匹配
	matched := registry.Match("/api/resource", "GET")
	if matched == nil {
		t.Fatal("Expected match for GET")
	}

	// POST 应该匹配
	matched = registry.Match("/api/resource", "POST")
	if matched == nil {
		t.Fatal("Expected match for POST")
	}

	// DELETE 不应该匹配
	matched = registry.Match("/api/resource", "DELETE")
	if matched != nil {
		t.Fatal("Expected no match for DELETE")
	}
}

func TestGatewayDefaultValues(t *testing.T) {
	registry := gateway.NewRouteRegistry()

	// 不设置默认值的路由
	route := &gateway.Route{
		ID:          "minimal",
		Path:        "/api/min",
		ServiceName: "min-service",
	}
	registry.Register(route)

	matched := registry.Match("/api/min", "GET")
	if matched == nil {
		t.Fatal("Expected match")
	}
	if matched.Timeout != 30*time.Second {
		t.Fatalf("Expected default timeout 30s, got %v", matched.Timeout)
	}
	if matched.LoadBalance != servicemesh.LBStrategyRandom {
		t.Fatalf("Expected default strategy random, got %s", matched.LoadBalance)
	}
	if len(matched.Methods) != 5 {
		t.Fatalf("Expected 5 default methods, got %d", len(matched.Methods))
	}
}

func parsePort(url string) int {
	var port int
	fmt.Sscanf(url, "http://127.0.0.1:%d", &port)
	return port
}
