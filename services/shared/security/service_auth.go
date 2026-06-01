// Package security 微服务安全认证模块
//
// 提供完整的服务间安全认证机制：
//   - 双向 TLS (mTLS) 认证
//   - JWT 令牌认证
//   - 服务白名单访问控制
//   - API 权限校验中间件
//   - 服务身份识别与授权
package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// =============================================================================
// 安全配置
// =============================================================================

// SecurityConfig 安全配置
type SecurityConfig struct {
	// mTLS 配置
	MTLSEnabled bool
	CAFile      string
	CertFile    string
	KeyFile     string
	ClientAuth  bool
	InsecureSkip bool

	// JWT 配置
	JWTEnabled   bool
	JWTSecret    string
	JWTIssuer    string
	JWTExpiry    time.Duration

	// 白名单配置
	WhitelistEnabled bool
	Whitelist        []string // 允许的服务名或 IP
	IPWhitelist      []string // 允许的 IP 地址

	// API 权限
	APIAuthEnabled bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *SecurityConfig {
	return &SecurityConfig{
		MTLSEnabled:      false,
		ClientAuth:       false,
		JWTEnabled:       false,
		JWTSecret:        "change-me-in-production",
		JWTIssuer:        "cloudflow",
		JWTExpiry:        24 * time.Hour,
		WhitelistEnabled: false,
		APIAuthEnabled:   false,
	}
}

// =============================================================================
// 服务身份与 JWT 令牌
// =============================================================================

// ServiceClaims JWT 服务身份声明
type ServiceClaims struct {
	ServiceName string   `json:"service_name"`
	ServiceID   string   `json:"service_id"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// ServiceIdentity 服务身份
type ServiceIdentity struct {
	Name        string
	ID          string
	Permissions []string
	Cert        *x509.Certificate // mTLS 证书
	IsAuthenticated bool
}

// contextKey 上下文键
type contextKey string

const (
	IdentityContextKey contextKey = "service_identity"
	AuthTokenContextKey contextKey = "auth_token"
)

// TokenManager JWT 令牌管理器
type TokenManager struct {
	config *SecurityConfig
	signingKey []byte
}

// NewTokenManager 创建令牌管理器
func NewTokenManager(cfg *SecurityConfig) *TokenManager {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &TokenManager{
		config:     cfg,
		signingKey: []byte(cfg.JWTSecret),
	}
}

// GenerateToken 生成服务间调用令牌
func (tm *TokenManager) GenerateToken(serviceName, serviceID string, permissions []string) (string, error) {
	if !tm.config.JWTEnabled {
		return "", nil
	}

	claims := ServiceClaims{
		ServiceName: serviceName,
		ServiceID:   serviceID,
		Permissions: permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.config.JWTIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tm.config.JWTExpiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.signingKey)
}

// ValidateToken 验证服务间调用令牌
func (tm *TokenManager) ValidateToken(tokenString string) (*ServiceClaims, error) {
	if !tm.config.JWTEnabled {
		return &ServiceClaims{IsAuthenticated: true}, nil
	}

	token, err := jwt.ParseWithClaims(tokenString, &ServiceClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return tm.signingKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*ServiceClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// =============================================================================
// 白名单管理
// =============================================================================

// WhitelistManager 白名单管理器
type WhitelistManager struct {
	config     *SecurityConfig
	serviceMap map[string]bool
	ipMap      map[string]bool
	mu         sync.RWMutex
}

// NewWhitelistManager 创建白名单管理器
func NewWhitelistManager(cfg *SecurityConfig) *WhitelistManager {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	wm := &WhitelistManager{
		config:     cfg,
		serviceMap: make(map[string]bool),
		ipMap:      make(map[string]bool),
	}

	// 初始化白名单
	for _, name := range cfg.Whitelist {
		wm.serviceMap[name] = true
	}
	for _, ip := range cfg.IPWhitelist {
		wm.ipMap[ip] = true
	}

	return wm
}

// IsServiceAllowed 检查服务是否在白名单中
func (wm *WhitelistManager) IsServiceAllowed(serviceName string) bool {
	if !wm.config.WhitelistEnabled {
		return true
	}

	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.serviceMap[serviceName]
}

// IsIPAllowed 检查 IP 是否在白名单中
func (wm *WhitelistManager) IsIPAllowed(ip string) bool {
	if !wm.config.WhitelistEnabled {
		return true
	}

	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.ipMap[ip]
}

// IsAddressAllowed 检查网络地址是否在白名单中
func (wm *WhitelistManager) IsAddressAllowed(addr string) bool {
	if !wm.config.WhitelistEnabled {
		return true
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // 可能是不带端口的地址
	}

	return wm.IsIPAllowed(host)
}

// AddService 添加服务到白名单
func (wm *WhitelistManager) AddService(serviceName string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.serviceMap[serviceName] = true
}

// AddIP 添加 IP 到白名单
func (wm *WhitelistManager) AddIP(ip string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.ipMap[ip] = true
}

// RemoveService 从白名单移除服务
func (wm *WhitelistManager) RemoveService(serviceName string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	delete(wm.serviceMap, serviceName)
}

// RemoveIP 从白名单移除 IP
func (wm *WhitelistManager) RemoveIP(ip string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	delete(wm.ipMap, ip)
}

// =============================================================================
// TLS/mTLS 凭证
// =============================================================================

// mTLSCredentials mTLS 凭证
type mTLSCredentials struct {
	config *SecurityConfig
	tlsConfig *tls.Config
}

// ServerTLSCredentials 创建服务端 TLS 凭证
func ServerTLSCredentials(cfg *SecurityConfig) (credentials.TransportCredentials, error) {
	if !cfg.MTLSEnabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.ClientAuth && cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		serverCert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load server cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// ClientTLSCredentials 创建客户端 TLS 凭证
func ClientTLSCredentials(cfg *SecurityConfig) (credentials.TransportCredentials, error) {
	if !cfg.MTLSEnabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if cfg.InsecureSkip {
		tlsConfig.InsecureSkipVerify = true
		return credentials.NewTLS(tlsConfig), nil
	}

	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// DialOptions 获取 gRPC Dial 选项
func DialOptions(cfg *SecurityConfig) ([]grpc.DialOption, error) {
	creds, err := ClientTLSCredentials(cfg)
	if err != nil {
		return nil, err
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(creds)}, nil
}

// =============================================================================
// gRPC 拦截器
// =============================================================================

// InterceptorManager 拦截器管理器
type InterceptorManager struct {
	config      *SecurityConfig
	tokenMgr    *TokenManager
	whitelistMgr *WhitelistManager
}

// NewInterceptorManager 创建拦截器管理器
func NewInterceptorManager(cfg *SecurityConfig, tokenMgr *TokenManager, whitelistMgr *WhitelistManager) *InterceptorManager {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if tokenMgr == nil {
		tokenMgr = NewTokenManager(cfg)
	}
	if whitelistMgr == nil {
		whitelistMgr = NewWhitelistManager(cfg)
	}

	return &InterceptorManager{
		config:      cfg,
		tokenMgr:    tokenMgr,
		whitelistMgr: whitelistMgr,
	}
}

// UnaryServerInterceptor 一元拦截器
func (im *InterceptorManager) UnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 1. 检查 mTLS 证书身份
	identity, err := im.extractAndVerifyIdentity(ctx)
	if err != nil {
		log.Printf("Authentication failed for %s: %v", info.FullMethod, err)
		return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	// 2. 检查白名单
	if !im.checkWhitelist(ctx, identity) {
		log.Printf("Access denied to %s for service %s", info.FullMethod, identity.Name)
		return nil, status.Errorf(codes.PermissionDenied, "access denied")
	}

	// 3. 检查 API 权限
	if err := im.checkAPIPermission(ctx, info.FullMethod, identity); err != nil {
		log.Printf("API permission denied for %s: %v", info.FullMethod, err)
		return nil, err
	}

	// 将身份信息存入上下文
	newCtx := context.WithValue(ctx, IdentityContextKey, identity)

	// 调用处理器
	return handler(newCtx, req)
}

// StreamServerInterceptor 流拦截器
func (im *InterceptorManager) StreamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// 提取和验证身份
	identity, err := im.extractAndVerifyIdentity(ss.Context())
	if err != nil {
		log.Printf("Authentication failed for %s: %v", info.FullMethod, err)
		return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	// 检查白名单
	if !im.checkWhitelist(ss.Context(), identity) {
		log.Printf("Access denied to %s for service %s", info.FullMethod, identity.Name)
		return status.Errorf(codes.PermissionDenied, "access denied")
	}

	// 包装流以传递身份
	wrapped := &wrappedServerStream{
		ServerStream: ss,
		ctx: context.WithValue(ss.Context(), IdentityContextKey, identity),
	}

	return handler(srv, wrapped)
}

// extractAndVerifyIdentity 从上下文提取并验证身份
func (im *InterceptorManager) extractAndVerifyIdentity(ctx context.Context) (*ServiceIdentity, error) {
	identity := &ServiceIdentity{}

	// 1. 从 mTLS 证书提取身份
	if p, ok := peer.FromContext(ctx); ok && im.config.MTLSEnabled {
		if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			if len(tlsInfo.State.VerifiedChains) > 0 && len(tlsInfo.State.VerifiedChains[0]) > 0 {
				cert := tlsInfo.State.VerifiedChains[0][0]
				identity.Cert = cert
				identity.Name = cert.Subject.CommonName
				identity.IsAuthenticated = true
			}
		}
	}

	// 2. 从 JWT 令牌提取身份
	if im.config.JWTEnabled {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			authHeaders := md.Get("authorization")
			for _, h := range authHeaders {
				if strings.HasPrefix(h, "Bearer ") {
					token := strings.TrimPrefix(h, "Bearer ")
					claims, err := im.tokenMgr.ValidateToken(token)
					if err == nil && claims != nil {
						identity.Name = claims.ServiceName
						identity.ID = claims.ServiceID
						identity.Permissions = claims.Permissions
						identity.IsAuthenticated = true
						return identity, nil
					}
				}
			}
		}
	}

	// 如果没有启用安全检查，返回未认证但允许访问
	if !im.config.JWTEnabled && !im.config.MTLSEnabled {
		identity.IsAuthenticated = true
		return identity, nil
	}

	// 如果启用了安全检查但未认证，返回错误
	if !identity.IsAuthenticated {
		return nil, fmt.Errorf("no valid authentication credentials")
	}

	return identity, nil
}

// checkWhitelist 检查白名单
func (im *InterceptorManager) checkWhitelist(ctx context.Context, identity *ServiceIdentity) bool {
	if !im.config.WhitelistEnabled {
		return true
	}

	// 检查服务名白名单
	if identity.Name != "" && im.whitelistMgr.IsServiceAllowed(identity.Name) {
		return true
	}

	// 检查 IP 白名单
	if p, ok := peer.FromContext(ctx); ok {
		return im.whitelistMgr.IsAddressAllowed(p.Addr.String())
	}

	return false
}

// checkAPIPermission 检查 API 权限
func (im *InterceptorManager) checkAPIPermission(ctx context.Context, method string, identity *ServiceIdentity) error {
	if !im.config.APIAuthEnabled {
		return nil
	}

	// 简单的权限检查：所有认证的服务都可以访问所有 API
	// 在生产环境中，可以在这里实现更复杂的权限逻辑
	if identity.IsAuthenticated {
		return nil
	}

	return status.Error(codes.PermissionDenied, "permission denied")
}

// wrappedServerStream 包装的 ServerStream
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context 返回上下文
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// =============================================================================
// HTTP 中间件
// =============================================================================

// HTTPMiddleware HTTP 安全中间件
type HTTPMiddleware struct {
	config      *SecurityConfig
	tokenMgr    *TokenManager
	whitelistMgr *WhitelistManager
}

// NewHTTPMiddleware 创建 HTTP 中间件
func NewHTTPMiddleware(cfg *SecurityConfig, tokenMgr *TokenManager, whitelistMgr *WhitelistManager) *HTTPMiddleware {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if tokenMgr == nil {
		tokenMgr = NewTokenManager(cfg)
	}
	if whitelistMgr == nil {
		whitelistMgr = NewWhitelistManager(cfg)
	}

	return &HTTPMiddleware{
		config:      cfg,
		tokenMgr:    tokenMgr,
		whitelistMgr: whitelistMgr,
	}
}

// SecurityMiddleware 安全中间件
func (hm *HTTPMiddleware) SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 验证认证
		identity, err := hm.authenticateHTTPRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 2. 检查白名单
		if !hm.checkHTTPWhitelist(r, identity) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// 3. 将身份存入上下文
		ctx := context.WithValue(r.Context(), IdentityContextKey, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticateHTTPRequest 认证 HTTP 请求
func (hm *HTTPMiddleware) authenticateHTTPRequest(r *http.Request) (*ServiceIdentity, error) {
	identity := &ServiceIdentity{}

	// 检查 mTLS 证书
	if hm.config.MTLSEnabled && r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		identity.Cert = cert
		identity.Name = cert.Subject.CommonName
		identity.IsAuthenticated = true
	}

	// 检查 JWT 令牌
	if hm.config.JWTEnabled {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := hm.tokenMgr.ValidateToken(token)
			if err == nil && claims != nil {
				identity.Name = claims.ServiceName
				identity.ID = claims.ServiceID
				identity.Permissions = claims.Permissions
				identity.IsAuthenticated = true
				return identity, nil
			}
		}
	}

	// 如果没有启用安全检查
	if !hm.config.JWTEnabled && !hm.config.MTLSEnabled {
		identity.IsAuthenticated = true
		return identity, nil
	}

	// 如果启用了安全检查但未认证
	if !identity.IsAuthenticated {
		return nil, fmt.Errorf("unauthorized")
	}

	return identity, nil
}

// checkHTTPWhitelist 检查 HTTP 白名单
func (hm *HTTPMiddleware) checkHTTPWhitelist(r *http.Request, identity *ServiceIdentity) bool {
	if !hm.config.WhitelistEnabled {
		return true
	}

	// 检查服务名白名单
	if identity.Name != "" && hm.whitelistMgr.IsServiceAllowed(identity.Name) {
		return true
	}

	// 检查 IP 白名单
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return hm.whitelistMgr.IsIPAllowed(host)
}

// =============================================================================
// 安全管理器 - 统一入口
// =============================================================================

// SecurityManager 安全管理器
type SecurityManager struct {
	Config          *SecurityConfig
	TokenManager    *TokenManager
	WhitelistManager *WhitelistManager
	InterceptorMgr *InterceptorManager
	HTTPMiddleware  *HTTPMiddleware
}

// NewSecurityManager 创建安全管理器
func NewSecurityManager(cfg *SecurityConfig) *SecurityManager {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	tokenMgr := NewTokenManager(cfg)
	whitelistMgr := NewWhitelistManager(cfg)
	interceptorMgr := NewInterceptorManager(cfg, tokenMgr, whitelistMgr)
	httpMiddleware := NewHTTPMiddleware(cfg, tokenMgr, whitelistMgr)

	return &SecurityManager{
		Config:          cfg,
		TokenManager:    tokenMgr,
		WhitelistManager: whitelistMgr,
		InterceptorMgr:  interceptorMgr,
		HTTPMiddleware:  httpMiddleware,
	}
}

// GetIdentityFromContext 从上下文获取服务身份
func GetIdentityFromContext(ctx context.Context) (*ServiceIdentity, bool) {
	identity, ok := ctx.Value(IdentityContextKey).(*ServiceIdentity)
	return identity, ok
}

// ServerOptions 获取 gRPC 服务端选项
func (sm *SecurityManager) ServerOptions() ([]grpc.ServerOption, error) {
	opts := []grpc.ServerOption{}

	// 添加 TLS 凭证
	tlsCreds, err := ServerTLSCredentials(sm.Config)
	if err != nil {
		return nil, err
	}
	opts = append(opts, grpc.Creds(tlsCreds))

	// 添加拦截器
	opts = append(opts, grpc.UnaryInterceptor(sm.InterceptorMgr.UnaryServerInterceptor))
	opts = append(opts, grpc.StreamInterceptor(sm.InterceptorMgr.StreamServerInterceptor))

	return opts, nil
}

// ClientOptions 获取 gRPC 客户端选项（带认证令牌）
func (sm *SecurityManager) ClientOptions(serviceName, serviceID string, permissions []string) ([]grpc.DialOption, error) {
	opts, err := DialOptions(sm.Config)
	if err != nil {
		return nil, err
	}

	// 添加 JWT 令牌拦截器
	if sm.Config.JWTEnabled {
		token, err := sm.TokenManager.GenerateToken(serviceName, serviceID, permissions)
		if err != nil {
			return nil, err
		}
		opts = append(opts, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
			return invoker(ctx, method, req, reply, cc, opts...)
		}))
		opts = append(opts, grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
			return streamer(ctx, desc, cc, method, opts...)
		}))
	}

	return opts, nil
}

// =============================================================================
// JSON 配置解析
// =============================================================================

// LoadConfigFromJSON 从 JSON 加载配置
func LoadConfigFromJSON(data []byte) (*SecurityConfig, error) {
	var cfg SecurityConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfigToJSON 保存配置到 JSON
func SaveConfigToJSON(cfg *SecurityConfig) ([]byte, error) {
	return json.MarshalIndent(cfg, "", "  ")
}
