// Package auth 统一认证中间件
//
// 设计目标:
//   - 统一 JWT Token 和 API Key 验证逻辑
//   - 自动注入 TenantContext 到请求链路
//   - gRPC 和 HTTP 双端点支持
//   - 所有服务引用，避免代码重复
//
// 使用方式:
//
//	1. 在 New() 中创建认证器
//		authMiddleware := auth.NewAuthenticator(auth.Config{
//			AuthAddr:  config.AuthAddr,
//			TLSConfig: tlsCfg,
//		})
//
//	2. HTTP 中使用
//		protectedMux := authMiddleware.HTTPHandler(mux)
//
//	3. gRPC 中使用
//		grpcServer := grpc.NewServer(
//			grpc.UnaryInterceptor(authMiddleware.GRPCInterceptor()),
//		)
package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	svcproto "cloud-flow/services/proto"
)

// Config 认证器配置
type Config struct {
	AuthAddr     string
	TLSEnabled   bool
	CAFile       string
	CertFile     string
	KeyFile      string
	InsecureSkip bool
}

// Authenticator 统一认证器
type Authenticator struct {
	authConn *grpc.ClientConn
}

// NewAuthenticator 创建认证器
func NewAuthenticator(config Config) (*Authenticator, error) {
	if config.AuthAddr == "" {
		return nil, fmt.Errorf("auth address is required")
	}

	var dialOpts []grpc.DialOption

	if config.TLSEnabled {
		tlsConfig, err := buildTLSConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to build TLS config: %w", err)
		}
		creds := credentials.NewTLS(tlsConfig)
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}

	conn, err := grpc.Dial(config.AuthAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial auth service: %w", err)
	}

	return &Authenticator{authConn: conn}, nil
}

// buildTLSConfig 构建 TLS 配置
func buildTLSConfig(config Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkip,
	}

	if config.CAFile != "" {
		caCert, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// Close 关闭认证器
func (a *Authenticator) Close() {
	if a.authConn != nil {
		a.authConn.Close()
	}
}

// ValidateResult 验证结果
type ValidateResult struct {
	Valid      bool
	UserID     string
	Username   string
	Role       string
	TenantID   string
	AuthMethod string
}

// ValidateToken 验证 Token
func (a *Authenticator) ValidateToken(ctx context.Context, token string) (*ValidateResult, error) {
	if a.authConn == nil {
		return nil, fmt.Errorf("auth connection not initialized")
	}

	client := svcproto.NewAuthServiceClient(a.authConn)
	resp, err := client.ValidateToken(ctx, &svcproto.ValidateTokenRequest{Token: token})
	if err != nil {
		return nil, err
	}

	return &ValidateResult{
		Valid:      resp.Valid,
		UserID:     resp.UserId,
		Username:   resp.Username,
		Role:       resp.Role,
		TenantID:   resp.TenantId,
		AuthMethod: "jwt",
	}, nil
}

// ValidateAPIKey 验证 API Key
func (a *Authenticator) ValidateAPIKey(ctx context.Context, apiKey string) (*ValidateResult, error) {
	if a.authConn == nil {
		return nil, fmt.Errorf("auth connection not initialized")
	}

	client := svcproto.NewAuthServiceClient(a.authConn)
	resp, err := client.ValidateAPIKey(ctx, &svcproto.ValidateAPIKeyRequest{ApiKey: apiKey})
	if err != nil {
		return nil, err
	}

	return &ValidateResult{
		Valid:      resp.Valid,
		UserID:     resp.UserId,
		Username:   resp.Username,
		Role:       resp.Role,
		TenantID:   resp.TenantId,
		AuthMethod: "apikey",
	}, nil
}

// ValidateRequest 验证 HTTP 请求
// 优先级: X-API-Key > Bearer Token
func (a *Authenticator) ValidateRequest(r *http.Request) (*ValidateResult, error) {
	ctx := r.Context()

	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		result, err := a.ValidateAPIKey(ctx, apiKey)
		if err != nil {
			return nil, err
		}
		if result.Valid {
			return result, nil
		}
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && strings.ToLower(authHeader[:7]) == "bearer " {
		token := authHeader[7:]
		result, err := a.ValidateToken(ctx, token)
		if err != nil {
			return nil, err
		}
		if result.Valid {
			return result, nil
		}
	}

	return nil, fmt.Errorf("missing authentication")
}

// HTTPHandler HTTP 认证中间件处理器
func (a *Authenticator) HTTPHandler(mux *http.ServeMux) *http.ServeMux {
	protectedMux := http.NewServeMux()

	protectedMux.HandleFunc("/healthz", mux.HandlerFunc("/healthz"))

	protectedMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			mux.ServeHTTP(w, r)
			return
		}

		result, err := a.ValidateRequest(r)
		if err != nil {
			if strings.Contains(err.Error(), "missing authentication") {
				http.Error(w, "Missing authentication", http.StatusUnauthorized)
				return
			}
			if strings.Contains(err.Error(), "not initialized") {
				http.Error(w, "Auth service unavailable", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := WithContext(r.Context(), result)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})

	return protectedMux
}

// Middleware HTTP 中间件包装器
func (a *Authenticator) Middleware(publicPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isPublic := false
			for _, path := range publicPaths {
				if r.URL.Path == path {
					isPublic = true
					break
				}
			}

			if isPublic {
				next.ServeHTTP(w, r)
				return
			}

			result, err := a.ValidateRequest(r)
			if err != nil {
				if strings.Contains(err.Error(), "missing authentication") {
					http.Error(w, "Missing authentication", http.StatusUnauthorized)
					return
				}
				if strings.Contains(err.Error(), "not initialized") {
					http.Error(w, "Auth service unavailable", http.StatusServiceUnavailable)
					return
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := WithContext(r.Context(), result)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ============================================================================
// Context 管理
// ============================================================================

type authCtxKey struct{}

type AuthContext struct {
	UserID     string
	Username   string
	Role       string
	TenantID   string
	AuthMethod string
}

func WithContext(ctx context.Context, result *ValidateResult) context.Context {
	authCtx := &AuthContext{
		UserID:     result.UserID,
		Username:   result.Username,
		Role:       result.Role,
		TenantID:   result.TenantID,
		AuthMethod: result.AuthMethod,
	}
	return context.WithValue(ctx, authCtxKey{}, authCtx)
}

func FromContext(ctx context.Context) (*AuthContext, bool) {
	authCtx, ok := ctx.Value(authCtxKey{}).(*AuthContext)
	return authCtx, ok
}

func MustFromContext(ctx context.Context) *AuthContext {
	authCtx, ok := FromContext(ctx)
	if !ok {
		panic("auth: context does not contain AuthContext")
	}
	return authCtx
}

// ============================================================================
// gRPC Metadata 注入和提取
// ============================================================================

// InjectToMetadata 将认证信息注入 gRPC metadata
func (a *Authenticator) InjectToMetadata(ctx context.Context, result *ValidateResult) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New()
	}

	md.Set("x-user-id", result.UserID)
	md.Set("x-tenant-id", result.TenantID)
	md.Set("x-username", result.Username)
	md.Set("x-role", result.Role)
	md.Set("x-auth-method", result.AuthMethod)

	return metadata.NewOutgoingContext(ctx, md)
}

// ExtractFromMetadata 从 gRPC metadata 提取认证信息
func ExtractFromMetadata(ctx context.Context) (*AuthContext, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, false
	}

	authCtx := &AuthContext{}

	if userIDs := md.Get("x-user-id"); len(userIDs) > 0 {
		authCtx.UserID = userIDs[0]
	}
	if tenantIDs := md.Get("x-tenant-id"); len(tenantIDs) > 0 {
		authCtx.TenantID = tenantIDs[0]
	}
	if usernames := md.Get("x-username"); len(usernames) > 0 {
		authCtx.Username = usernames[0]
	}
	if roles := md.Get("x-role"); len(roles) > 0 {
		authCtx.Role = roles[0]
	}
	if authMethods := md.Get("x-auth-method"); len(authMethods) > 0 {
		authCtx.AuthMethod = authMethods[0]
	}

	return authCtx, authCtx.UserID != "" || authCtx.TenantID != ""
}

// ============================================================================
// HTTP Header 注入和提取
// ============================================================================

const (
	HeaderTenantID = "X-Tenant-ID"
	HeaderUserID   = "X-User-ID"
	HeaderUsername = "X-Username"
	HeaderRole     = "X-Role"
	HeaderAuth     = "X-Auth-Method"
)

// InjectToHeaders 将认证信息注入 HTTP headers
func InjectToHeaders(r *http.Request, result *ValidateResult) {
	r.Header.Set(HeaderTenantID, result.TenantID)
	r.Header.Set(HeaderUserID, result.UserID)
	r.Header.Set(HeaderUsername, result.Username)
	r.Header.Set(HeaderRole, result.Role)
	r.Header.Set(HeaderAuth, result.AuthMethod)
}

// ExtractFromHeaders 从 HTTP headers 提取认证信息
func ExtractFromHeaders(r *http.Request) *AuthContext {
	return &AuthContext{
		TenantID:   r.Header.Get(HeaderTenantID),
		UserID:     r.Header.Get(HeaderUserID),
		Username:   r.Header.Get(HeaderUsername),
		Role:       r.Header.Get(HeaderRole),
		AuthMethod: r.Header.Get(HeaderAuth),
	}
}

// ============================================================================
// gRPC Interceptors
// ============================================================================

// GRPCInterceptor gRPC 一元拦截器
func (a *Authenticator) GRPCInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !requiresAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, fmt.Errorf("missing metadata")
		}

		var result *ValidateResult
		var err error

		if apiKeys := md.Get("x-api-key"); len(apiKeys) > 0 && apiKeys[0] != "" {
			result, err = a.ValidateAPIKey(ctx, apiKeys[0])
		} else if authHeaders := md.Get("authorization"); len(authHeaders) > 0 {
			authHeader := authHeaders[0]
			if len(authHeader) > 7 && strings.ToLower(authHeader[:7]) == "bearer " {
				result, err = a.ValidateToken(ctx, authHeader[7:])
			}
		}

		if err != nil {
			return nil, fmt.Errorf("auth service error: %w", err)
		}

		if result == nil || !result.Valid {
			return nil, fmt.Errorf("unauthorized")
		}

		newCtx := WithContext(ctx, result)
		return handler(newCtx, req)
	}
}

// GRPCStreamInterceptor gRPC 流拦截器
func (a *Authenticator) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !requiresAuth(info.FullMethod) {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return fmt.Errorf("missing metadata")
		}

		var result *ValidateResult
		var err error

		if apiKeys := md.Get("x-api-key"); len(apiKeys) > 0 && apiKeys[0] != "" {
			result, err = a.ValidateAPIKey(ctx, apiKeys[0])
		} else if authHeaders := md.Get("authorization"); len(authHeaders) > 0 {
			authHeader := authHeaders[0]
			if len(authHeader) > 7 && strings.ToLower(authHeader[:7]) == "bearer " {
				result, err = a.ValidateToken(ctx, authHeader[7:])
			}
		}

		if err != nil {
			return fmt.Errorf("auth service error: %w", err)
		}

		if result == nil || !result.Valid {
			return fmt.Errorf("unauthorized")
		}

		newCtx := WithContext(ctx, result)
		wrapped := &grpcServerStreamWrapper{
			ServerStream: ss,
			ctx:          newCtx,
		}
		return handler(srv, wrapped)
	}
}

// requiresAuth 判断 gRPC 方法是否需要认证
func requiresAuth(method string) bool {
	publicMethods := []string{
		"/proto.HealthService/Check",
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
	}
	for _, m := range publicMethods {
		if strings.HasPrefix(method, m) {
			return false
		}
	}
	return true
}

// grpcServerStreamWrapper gRPC 流包装器
type grpcServerStreamWrapper struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *grpcServerStreamWrapper) Context() context.Context {
	return w.ctx
}
