//go:build linux

package gateway

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/servicemesh"
)

// ============================================================================
// 一、路由配置
// ============================================================================

// Route 路由配置
type Route struct {
	ID            string              `json:"id"`
	Path          string              `json:"path"`           // 请求路径前缀
	Methods       []string            `json:"methods"`        // HTTP方法
	ServiceName   string              `json:"service_name"`   // 目标服务名
	StripPrefix   bool                `json:"strip_prefix"`   // 是否去除前缀
	RewritePath   string              `json:"rewrite_path"`   // 路径重写
	LoadBalance   servicemesh.LoadBalanceStrategy `json:"load_balance"`
	Timeout       time.Duration       `json:"timeout"`
	RetryCount    int                 `json:"retry_count"`
	RequireAuth   bool                `json:"require_auth"`   // 是否需要认证
	AllowedRoles  []string            `json:"allowed_roles"`  // 允许的角色
	RateLimit     *RateLimitConfig    `json:"rate_limit,omitempty"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	QPS       int           // 每秒请求数
	Burst     int           // 突发流量
	Window    time.Duration // 时间窗口
	KeyPrefix string        // Redis key前缀
}

// RouteRegistry 路由注册表
type RouteRegistry struct {
	mu     sync.RWMutex
	routes map[string]*Route // path -> route
	order  []*Route          // 有序列表（用于前缀匹配）
}

// NewRouteRegistry 创建路由注册表
func NewRouteRegistry() *RouteRegistry {
	return &RouteRegistry{
		routes: make(map[string]*Route),
		order:  make([]*Route, 0),
	}
}

// Register 注册路由
func (rr *RouteRegistry) Register(route *Route) error {
	if route.ID == "" || route.Path == "" || route.ServiceName == "" {
		return fmt.Errorf("route id, path, service_name required")
	}
	
	rr.mu.Lock()
	defer rr.mu.Unlock()
	
	// 检查是否已存在
	if _, exists := rr.routes[route.ID]; exists {
		return fmt.Errorf("route %s already exists", route.ID)
	}
	
	// 设置默认值
	if route.Methods == nil {
		route.Methods = []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	}
	if route.Timeout == 0 {
		route.Timeout = 30 * time.Second
	}
	if route.LoadBalance == "" {
		route.LoadBalance = servicemesh.LBStrategyRandom
	}
	
	rr.routes[route.ID] = route
	
	// 按路径长度排序（长路径优先匹配）
	inserted := false
	for i, r := range rr.order {
		if len(route.Path) > len(r.Path) {
			rr.order = append(rr.order[:i], append([]*Route{route}, rr.order[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		rr.order = append(rr.order, route)
	}
	
	return nil
}

// Remove 移除路由
func (rr *RouteRegistry) Remove(routeID string) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	
	delete(rr.routes, routeID)
	
	var newOrder []*Route
	for _, r := range rr.order {
		if r.ID != routeID {
			newOrder = append(newOrder, r)
		}
	}
	rr.order = newOrder
}

// Match 匹配路由
func (rr *RouteRegistry) Match(path string, method string) *Route {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	
	for _, route := range rr.order {
		if strings.HasPrefix(path, route.Path) {
			// 检查方法
			if len(route.Methods) == 0 {
				return route
			}
			for _, m := range route.Methods {
				if m == method || m == "*" {
					return route
				}
			}
		}
	}
	return nil
}

// GetAll 获取所有路由
func (rr *RouteRegistry) GetAll() []*Route {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	
	result := make([]*Route, len(rr.order))
	copy(result, rr.order)
	return result
}

// ============================================================================
// 二、限流器
// ============================================================================

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(key string, limit int, window time.Duration) bool
	GetRemaining(key string, limit int, window time.Duration) int
}

// MemoryRateLimiter 内存限流器（滑动窗口）
type MemoryRateLimiter struct {
	mu     sync.RWMutex
	windows map[string]*slidingWindow
}

// slidingWindow 滑动窗口
type slidingWindow struct {
	limit    int
	window   time.Duration
	requests []time.Time
	mu       sync.Mutex
}

// NewMemoryRateLimiter 创建内存限流器
func NewMemoryRateLimiter() RateLimiter {
	return &MemoryRateLimiter{
		windows: make(map[string]*slidingWindow),
	}
}

// Allow 是否允许请求
func (mrl *MemoryRateLimiter) Allow(key string, limit int, window time.Duration) bool {
	mrl.mu.Lock()
	w, exists := mrl.windows[key]
	if !exists {
		w = &slidingWindow{
			limit:  limit,
			window: window,
		}
		mrl.windows[key] = w
	}
	mrl.mu.Unlock()
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-window)
	
	// 清理过期请求
	var valid []time.Time
	for _, t := range w.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	w.requests = valid
	
	if len(w.requests) < limit {
		w.requests = append(w.requests, now)
		return true
	}
	return false
}

// GetRemaining 获取剩余配额
func (mrl *MemoryRateLimiter) GetRemaining(key string, limit int, window time.Duration) int {
	mrl.mu.RLock()
	w, exists := mrl.windows[key]
	mrl.mu.RUnlock()
	
	if !exists {
		return limit
	}
	
	w.mu.Lock()
	defer w.mu.Unlock()
	
	cutoff := time.Now().Add(-window)
	var valid []time.Time
	for _, t := range w.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	w.requests = valid
	
	remaining := limit - len(w.requests)
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// ============================================================================
// 三、认证中间件
// ============================================================================

// AuthProvider 认证提供者接口
type AuthProvider interface {
	Authenticate(r *http.Request) (*AuthResult, error)
}

// AuthResult 认证结果
type AuthResult struct {
	UserID   string            `json:"user_id"`
	Username string            `json:"username"`
	Roles    []string          `json:"roles"`
	TenantID string            `json:"tenant_id"`
	Claims   map[string]string `json:"claims,omitempty"`
}

// JWTAuthProvider JWT认证提供者
type JWTAuthProvider struct {
	secretKey []byte
	issuer    string
}

// NewJWTAuthProvider 创建JWT认证提供者
func NewJWTAuthProvider(secretKey string, issuer string) AuthProvider {
	// 简化版：实际应使用 jwt-go 库
	return &JWTAuthProvider{
		secretKey: []byte(secretKey),
		issuer:    issuer,
	}
}

// Authenticate JWT认证
func (jap *JWTAuthProvider) Authenticate(r *http.Request) (*AuthResult, error) {
	// 简化实现：实际应解析JWT token
	token := r.Header.Get("Authorization")
	if token == "" {
		return nil, fmt.Errorf("missing authorization header")
	}
	
	// 移除 "Bearer " 前缀
	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	}
	
	// 模拟解析（实际应验证签名和claims）
	if token == "invalid" {
		return nil, fmt.Errorf("invalid token")
	}
	
	return &AuthResult{
		UserID:   "user-123",
		Username: "admin",
		Roles:    []string{"admin"},
		TenantID: "tenant-1",
	}, nil
}

// ============================================================================
// 四、网关核心
// ============================================================================

// Gateway 服务网关
type Gateway struct {
	mu           sync.RWMutex
	addr         string
	registry     servicemesh.Registry
	routes       *RouteRegistry
	rateLimiter  RateLimiter
	authProvider AuthProvider
	clients      map[string]*servicemesh.ServiceClient
	server       *http.Server
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewGateway 创建网关
func NewGateway(addr string, registry servicemesh.Registry) *Gateway {
	return &Gateway{
		addr:        addr,
		registry:    registry,
		routes:      NewRouteRegistry(),
		rateLimiter: NewMemoryRateLimiter(),
		clients:     make(map[string]*servicemesh.ServiceClient),
		stopCh:      make(chan struct{}),
	}
}

// SetAuthProvider 设置认证提供者
func (g *Gateway) SetAuthProvider(provider AuthProvider) {
	g.authProvider = provider
}

// SetRateLimiter 设置限流器
func (g *Gateway) SetRateLimiter(limiter RateLimiter) {
	g.rateLimiter = limiter
}

// RegisterRoute 注册路由
func (g *Gateway) RegisterRoute(route *Route) error {
	if err := g.routes.Register(route); err != nil {
		return err
	}
	
	// 创建/更新服务客户端
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if _, exists := g.clients[route.ServiceName]; !exists {
		client, err := servicemesh.NewServiceClient(route.ServiceName, g.registry, route.LoadBalance)
		if err != nil {
			return err
		}
		client.SetTimeout(route.Timeout)
		g.clients[route.ServiceName] = client
	}
	
	return nil
}

// RemoveRoute 移除路由
func (g *Gateway) RemoveRoute(routeID string) {
	g.routes.Remove(routeID)
}

// ServeHTTP HTTP处理器
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			http.Error(w, fmt.Sprintf("Internal Server Error: %v", rec), http.StatusInternalServerError)
		}
	}()
	
	// 1. 匹配路由
	route := g.routes.Match(r.URL.Path, r.Method)
	if route == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	
	// 2. 认证检查
	if route.RequireAuth && g.authProvider != nil {
		_, err := g.authProvider.Authenticate(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// 角色检查（简化版）
		if len(route.AllowedRoles) > 0 {
			// 实际应检查用户角色
		}
	}
	
	// 3. 限流检查
	if route.RateLimit != nil {
		key := r.RemoteAddr + ":" + route.ID
		if !g.rateLimiter.Allow(key, route.RateLimit.QPS, route.RateLimit.Window) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		remaining := g.rateLimiter.GetRemaining(key, route.RateLimit.QPS, route.RateLimit.Window)
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	}
	
	// 4. 获取服务客户端并转发请求
	g.mu.RLock()
	client, exists := g.clients[route.ServiceName]
	g.mu.RUnlock()
	
	if !exists {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	
	// 5. 构建目标URL并转发
	inst, err := client.GetInstance()
	if err != nil {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	
	// 构建目标路径
	targetPath := r.URL.Path
	if route.StripPrefix {
		targetPath = strings.TrimPrefix(targetPath, route.Path)
	}
	if route.RewritePath != "" {
		targetPath = route.RewritePath
	}
	
	// 确保路径以 / 开头
	if targetPath != "" && !strings.HasPrefix(targetPath, "/") {
		targetPath = "/" + targetPath
	}
	
	targetURL := fmt.Sprintf("http://%s%s", inst.Address(), targetPath)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}
	
	// 6. 创建反向代理请求
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	
	// 复制 headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	
	// 7. 执行请求（带超时）
	clientHTTP := &http.Client{Timeout: route.Timeout}
	resp, err := clientHTTP.Do(proxyReq)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	
	// 8. 复制响应
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	
	// 复制 body
	io.Copy(w, resp.Body)
}

// Start 启动网关
func (g *Gateway) Start() error {
	g.server = &http.Server{
		Addr:    g.addr,
		Handler: g,
	}
	
	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// log error
		}
	}()
	
	return nil
}

// Stop 停止网关
func (g *Gateway) Stop() error {
	g.stopOnce.Do(func() {
		close(g.stopCh)
	})
	
	if g.server != nil {
		return g.server.Close()
	}
	return nil
}

// GetStats 获取网关统计
func (g *Gateway) GetStats() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	return map[string]interface{}{
		"addr":            g.addr,
		"route_count":     len(g.routes.GetAll()),
		"client_count":    len(g.clients),
		"has_auth":        g.authProvider != nil,
		"has_rate_limit":  g.rateLimiter != nil,
	}
}

// GetRoutes 获取所有路由
func (g *Gateway) GetRoutes() []*Route {
	return g.routes.GetAll()
}

// GetServiceClient 获取服务客户端
func (g *Gateway) GetServiceClient(name string) (*servicemesh.ServiceClient, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	client, ok := g.clients[name]
	return client, ok
}
