package authservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"github.com/meinanzilinzhengying/cloudflow/pkg/metrics"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/meinanzilinzhengying/cloudflow/services/auth-service/auth"
	"github.com/meinanzilinzhengying/cloudflow/services/auth-service/rbac"
	rbacadapter "github.com/meinanzilinzhengying/cloudflow/services/auth-service/rbac/adapter"
	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/tenant"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/tlsutil"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/security"
)

// ============================================================================
// 配置
// ============================================================================

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string
	GrpcAddr    string // :9006
	HttpAddr    string // :8006

	// JWT
	JWTSecret      string
	JWTIssuer      string
	JWTExpireSec   int64
	JWTRefreshSec  int64

	// OIDC (可选)
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	OIDCScopes       string

	// RBAC
	SuperAdminRole string

	// 关系型数据库配置
	RelationalDBType     storage.DatabaseType
	RelationalDBHost     string
	RelationalDBPort     int
	RelationalDBUser     string
	RelationalDBPassword string
	RelationalDBDatabase string

	// P0-2 修复: TLS 配置
	TLSEnabled      bool
	TLSCAFile       string
	TLSCertFile     string
	TLSKeyFile      string
	TLSClientAuth   bool
	TLSInsecureSkip bool
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	cfg := &Config{
		ServiceName:         "auth-service",
		Version:             "1.0.0",
		GrpcAddr:            ":9006",
		HttpAddr:            ":8006",
		JWTIssuer:           "cloudflow",
		JWTExpireSec:        86400,   // 24h
		JWTRefreshSec:       604800,  // 7d
		SuperAdminRole:      "super_admin",
		RelationalDBType:    storage.DBMySQL,
		RelationalDBHost:    "mysql",
		RelationalDBPort:    3306,
		RelationalDBUser:    "root",
		RelationalDBDatabase: "cloudflow_auth",
		TLSEnabled:          false,
		TLSInsecureSkip:     false,
	}

	// 从环境变量读取配置
	if v := os.Getenv("RELATIONAL_DB_TYPE"); v != "" {
		cfg.RelationalDBType = storage.DatabaseType(v)
	}
	if v := os.Getenv("RELATIONAL_DB_HOST"); v != "" {
		cfg.RelationalDBHost = v
	}
	if v := os.Getenv("RELATIONAL_DB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.RelationalDBPort = p
		}
	}
	if v := os.Getenv("RELATIONAL_DB_USER"); v != "" {
		cfg.RelationalDBUser = v
	}
	cfg.RelationalDBPassword = os.Getenv("RELATIONAL_DB_PASSWORD")
	if v := os.Getenv("RELATIONAL_DB_DATABASE"); v != "" {
		cfg.RelationalDBDatabase = v
	}
	return cfg
}

// ============================================================================
// 服务
// ============================================================================

// Service Auth Service
type Service struct {
	config *Config

	// 认证
	authenticator *auth.Authenticator

	// RBAC
	rbacEngine *rbac.RBACEngine

	// 关系型数据库存储
	db     storage.RelationalStorage
	gormDB *gorm.DB

	// 用户缓存 (热路径优化，但数据来源于 TiDB)
	usersCache sync.Map // username -> *UserInfo
	cacheTTL   time.Duration
	cacheMu    sync.RWMutex

	// gRPC/HTTP
	grpcServer *grpc.Server
	health     *health.Server
	httpServer *http.Server

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials

	// 安全管理
	securityManager *security.SecurityManager

	startTime time.Time
}

// HealthCheck 健康检查
func (s *Service) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{
		Healthy: true,
		Version: s.config.Version,
		Uptime:  int64(time.Since(s.startTime).Seconds()),
	}, nil
}

// UserInfo 用户信息
type UserInfo struct {
	UserID   string
	Username string
	Password string // bcrypt hash
	TenantID string
	Role     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New 创建服务
func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 初始化安全管理器
	secConfig := security.DefaultSecurityConfig()
	secManager := security.NewSecurityManager(secConfig)
	
	// 初始化认证器（使用安全管理器的黑名单）
	authConfig := &auth.JWTConfig{
		PrivateKey:      config.JWTSecret,
		Issuer:          config.JWTIssuer,
		ExpireDuration:  time.Duration(config.JWTExpireSec) * time.Second,
		RefreshDuration: time.Duration(config.JWTRefreshSec) * time.Second,
		Blacklist:       auth.NewInMemoryBlacklist(time.Duration(config.JWTExpireSec) * time.Second),
	}
	
	authenticator, err := auth.NewAuthenticatorWithConfig(authConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	s := &Service{
		config:        config,
		authenticator: authenticator,
		cacheTTL:      5 * time.Minute,
		securityManager: secManager,
		startTime:     time.Now(),
		health:        health.NewServer(),
	}

	// P0-2 修复: 初始化 TLS 凭证
	if config.TLSEnabled {
		tlsCfg := tlsutil.Config{
			Enabled:      config.TLSEnabled,
			CAFile:       config.TLSCAFile,
			CertFile:     config.TLSCertFile,
			KeyFile:      config.TLSKeyFile,
			ClientAuth:   config.TLSClientAuth,
			InsecureSkip: config.TLSInsecureSkip,
		}
		var err error
		s.grpcCreds, err = tlsutil.ServerCredentials(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("TLS credentials init failed: %w", err)
		}
	}

	// 初始化关系型数据库连接
	if err := s.initDatabase(); err != nil {
		return nil, fmt.Errorf("database init failed: %w", err)
	}

	// 使用 GormAdapter 初始化 RBAC 引擎
	gormAdapter, err := rbacadapter.NewGormAdapter(s.gormDB)
	if err != nil {
		return nil, fmt.Errorf("RBAC adapter init failed: %w", err)
	}

	s.rbacEngine = rbac.NewRBACEngineWithAdapter(rbac.RBACConfig{
		SuperAdminRole:         config.SuperAdminRole,
		DefaultPoliciesEnabled: true,
	}, gormAdapter)

	// 初始化 gRPC 服务器
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.UnaryInterceptor(tenant.GRPCUnaryServerInterceptor()))
	if s.grpcCreds != nil {
		grpcOptions = append(grpcOptions, grpc.Creds(s.grpcCreds))
	}
	s.grpcServer = grpc.NewServer(grpcOptions...)
	svcproto.RegisterAuthServiceServer(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	return s, nil
}

// initDatabase 初始化关系型数据库连接和用户表
func (s *Service) initDatabase() error {
	cfg := &storage.Config{
		Type:     s.config.RelationalDBType,
		Host:     s.config.RelationalDBHost,
		Port:     s.config.RelationalDBPort,
		User:     s.config.RelationalDBUser,
		Password: s.config.RelationalDBPassword,
		Database: s.config.RelationalDBDatabase,
	}

	db, err := storage.OpenRelational(cfg)
	if err != nil {
		return fmt.Errorf("database open failed: %w", err)
	}

	s.db = db

	// 初始化 GORM DB (用于 RBAC 持久化)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_bin",
		s.config.RelationalDBUser,
		s.config.RelationalDBPassword,
		s.config.RelationalDBHost,
		s.config.RelationalDBPort,
		s.config.RelationalDBDatabase,
	)
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("GORM open failed: %w", err)
	}
	s.gormDB = gormDB

	// 初始化用户表
	if err := s.initUserTable(); err != nil {
		return fmt.Errorf("init user table failed: %w", err)
	}

	// 加载用户到缓存
	if err := s.loadUsersToCache(); err != nil {
		return fmt.Errorf("load users to cache failed: %w", err)
	}

	fmt.Printf("Auth Service connected: database=%s, host=%s:%d/%s\n", 
		s.config.RelationalDBType, s.config.RelationalDBHost, s.config.RelationalDBPort, s.config.RelationalDBDatabase)
	return nil
}

// initUserTable 初始化用户表
func (s *Service) initUserTable() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		user_id VARCHAR(64) PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		tenant_id VARCHAR(64) DEFAULT 'default',
		role VARCHAR(20) NOT NULL DEFAULT 'user',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_username (username),
		INDEX idx_tenant (tenant_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	if _, err := s.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	// 检查是否存在默认管理员用户
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&count)
	if err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}

	if count == 0 {
		// 创建默认管理员
		defaultPassword := os.Getenv("ADMIN_INITIAL_PASSWORD")
		if defaultPassword == "" {
			return fmt.Errorf("ADMIN_INITIAL_PASSWORD 环境变量未设置，禁止使用默认密码创建管理员用户")
		}
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("generate password hash: %w", err)
		}

		_, err = s.db.Exec(
			"INSERT INTO users (user_id, username, password, tenant_id, role) VALUES (?, ?, ?, ?, ?)",
			"admin-001",
			"admin",
			string(hashedPassword),
			"default",
			"admin",
		)
		if err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}

		fmt.Println("默认管理员用户已创建")
	}

	return nil
}

// loadUsersToCache 从 TiDB 加载用户到内存缓存
func (s *Service) loadUsersToCache() error {
	rows, err := s.db.Query("SELECT user_id, username, password, tenant_id, role FROM users")
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var user UserInfo
		if err := rows.Scan(&user.UserID, &user.Username, &user.Password, &user.TenantID, &user.Role); err != nil {
			continue
		}
		s.usersCache.Store(user.Username, &user)
	}

	return nil
}

// Start 启动服务
func (s *Service) Start() error {
	// 启动 RBAC 引擎
	// FIX: 检查 RBAC 引擎启动错误
	if err := s.rbacEngine.Start(context.Background()); err != nil {
		return fmt.Errorf("RBAC engine start failed: %w", err)
	}

	// 注册内置角色
	s.rbacEngine.AddBuiltinRoles()

	// 启动 gRPC (带租户拦截器)
	listener, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listen failed: %w", err)
	}

	go func() { s.grpcServer.Serve(listener) }()

	// 启动 HTTP (带租户中间件)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/login", s.loginHandler)
	mux.HandleFunc("/verify", s.verifyHandler)
	mux.HandleFunc("/refresh", s.refreshHandler)
	mux.HandleFunc("/revoke", s.revokeHandler)
	mux.HandleFunc("/oidc/callback", s.oidcCallbackHandler)
	mux.HandleFunc("/oidc/auth", s.oidcAuthHandler)
	mux.HandleFunc("/roles", s.rolesHandler)
	mux.HandleFunc("/permissions/check", s.checkPermissionHandler)
	mux.HandleFunc("/apikeys", s.apiKeyHandler)
	
	// P0-02 修复: 添加用户管理 API
	mux.HandleFunc("/users", s.usersHandler)
	mux.HandleFunc("/users/create", s.createUserHandler)
	mux.HandleFunc("/users/update", s.updateUserHandler)
	mux.HandleFunc("/users/delete", s.deleteUserHandler)

	s.httpServer = &http.Server{
		Addr:    s.config.HttpAddr,
		Handler: tenant.HTTPMiddleware(mux),
	}
	go func() { s.httpServer.ListenAndServe() }()

	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)
	fmt.Printf("Auth Service started: gRPC=%s, HTTP=%s (database=%s)\n", 
		s.config.GrpcAddr, s.config.HttpAddr, s.config.RelationalDBType)
	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	s.rbacEngine.Stop()
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	// P1-04 修复: 使用优雅关闭等待请求完成
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			fmt.Printf("HTTP server shutdown error: %v\n", err)
		}
	}

	// P1-04 修复: gRPC 使用带超时的 GracefulStop
	if s.grpcServer != nil {
		stopped := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(30 * time.Second):
			fmt.Println("gRPC graceful stop timeout, forcing stop")
			s.grpcServer.Stop()
		}
	}

	if s.db != nil {
		s.db.Close()
	}
}

// ============================================================================
// gRPC 服务方法
// ============================================================================

// Authenticate 认证 (用户名+密码 → JWT)
func (s *Service) Authenticate(ctx context.Context, req *svcproto.AuthenticateRequest) (*svcproto.AuthenticateResponse, error) {
	// 1. 检查账户是否被锁定
	if s.securityManager != nil && s.securityManager.Lockout().IsLocked(req.Username) {
		return nil, fmt.Errorf("account temporarily locked due to too many failed attempts")
	}

	// 2. 从 TiDB 查找用户
	user, err := s.findUserFromDB(req.Username)
	if err != nil || user == nil {
		// 记录失败尝试（即使用户名不存在也要记录，防止用户名枚举攻击）
		if s.securityManager != nil {
			s.securityManager.Lockout().RecordFailedAttempt(req.Username)
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	// 3. 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		// 记录失败尝试
		if s.securityManager != nil {
			s.securityManager.Lockout().RecordFailedAttempt(user.UserID)
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	// 4. 登录成功，清除失败记录
	if s.securityManager != nil {
		s.securityManager.Lockout().RecordSuccessfulAttempt(user.UserID)
	}

	// 5. 检查 token 生成速率限制
	if s.securityManager != nil {
		limited, err := s.securityManager.TokenRateLimiter().Allow(user.UserID)
		if err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}
		if !limited {
			return nil, fmt.Errorf("too many token requests, please try again later")
		}
	}

	// 6. 签发 JWT
	token, err := s.authenticator.JWTManager().GenerateToken(
		user.UserID, user.TenantID, user.Username, user.Role, "", nil, false,
	)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %w", err)
	}

	return &svcproto.AuthenticateResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(s.config.JWTExpireSec) * time.Second).Unix(),
		UserId:    user.UserID,
		Role:      user.Role,
	}, nil
}

// ValidateToken 验证 Token
func (s *Service) ValidateToken(ctx context.Context, req *svcproto.ValidateTokenRequest) (*svcproto.ValidateTokenResponse, error) {
	claims, err := s.authenticator.Authenticate(ctx, req.Token)
	if err != nil {
		return &svcproto.ValidateTokenResponse{Valid: false}, nil
	}

	return &svcproto.ValidateTokenResponse{
		Valid:    true,
		UserId:   claims.Subject,
		Username: claims.Username,
		Role:     claims.Role,
		TenantId: claims.TenantID,
	}, nil
}

// Authorize 鉴权 (Casbin RBAC)
func (s *Service) Authorize(ctx context.Context, req *svcproto.AuthorizeRequest) (*svcproto.AuthorizeResponse, error) {
	tc, ok := tenant.FromContext(ctx)
	if !ok {
		return &svcproto.AuthorizeResponse{Allowed: false, Reason: "no tenant context"}, nil
	}

	allowed, reason := s.rbacEngine.CheckPermission(
		req.UserId, tc.TenantID, tc.ProjectID, req.Resource, req.Action,
	)

	return &svcproto.AuthorizeResponse{
		Allowed: allowed,
		Reason:  reason,
	}, nil
}

// CreateRole 创建角色
func (s *Service) CreateRole(ctx context.Context, req *svcproto.CreateRoleRequest) (*svcproto.CreateRoleResponse, error) {
	tc := tenant.MustFromContext(ctx)

	role := &svcproto.Role{
		Id:          fmt.Sprintf("role_%d", time.Now().UnixNano()),
		TenantId:    req.TenantId,
		ProjectId:   req.ProjectId,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Permissions: req.Permissions,
		IsBuiltin:   false,
		CreatedAt:   time.Now().Unix(),
	}

	for _, perm := range req.Permissions {
		action, resource := parsePermission(perm)
		if action != "" {
			s.rbacEngine.AddPolicy(req.Name, tc.TenantID, req.ProjectId, resource, action, "allow")
		}
	}

	return &svcproto.CreateRoleResponse{Role: role}, nil
}

// BindUserRole 绑定用户角色
func (s *Service) BindUserRole(ctx context.Context, req *svcproto.BindUserRoleRequest) (*svcproto.BindUserRoleResponse, error) {
	_ = tenant.MustFromContext(ctx)

	err := s.rbacEngine.AddRoleForUser(req.UserId, req.RoleId, req.TenantId, req.ProjectId)
	if err != nil {
		return &svcproto.BindUserRoleResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.BindUserRoleResponse{Success: true, Message: "role bound"}, nil
}

// CreatePolicy 创建策略
func (s *Service) CreatePolicy(ctx context.Context, req *svcproto.CreatePolicyRequest) (*svcproto.CreatePolicyResponse, error) {
	tc := tenant.MustFromContext(ctx)

	policy := &svcproto.Policy{
		Id:          fmt.Sprintf("policy_%d", time.Now().UnixNano()),
		TenantId:    req.TenantId,
		ProjectId:   req.ProjectId,
		Name:        req.Name,
		Description: req.Description,
		Effect:      req.Effect,
		Actions:     req.Actions,
		Resources:   req.Resources,
		Conditions:  req.Conditions,
		Priority:    req.Priority,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}

	for _, action := range req.Actions {
		for _, resource := range req.Resources {
			s.rbacEngine.AddPolicy("*", tc.TenantID, req.ProjectId, resource, action, req.Effect)
		}
	}

	return &svcproto.CreatePolicyResponse{Policy: policy}, nil
}

// CheckPermission 检查权限
func (s *Service) CheckPermission(ctx context.Context, req *svcproto.CheckPermissionRequest) (*svcproto.CheckPermissionResponse, error) {
	tc, ok := tenant.FromContext(ctx)
	if !ok {
		return &svcproto.CheckPermissionResponse{Allowed: false, Reason: "no tenant context"}, nil
	}

	allowed, reason := s.rbacEngine.CheckPermission(
		req.UserId, req.TenantId, tc.ProjectID, req.Resource, req.Action,
	)

	return &svcproto.CheckPermissionResponse{Allowed: allowed, Reason: reason}, nil
}

// OIDCCallback OIDC 回调
func (s *Service) OIDCCallback(ctx context.Context, req *svcproto.OIDCCallbackRequest) (*svcproto.AuthenticateResponse, error) {
	claims, err := s.authenticator.Authenticate(ctx, req.Code)
	if err != nil {
		return nil, fmt.Errorf("OIDC callback failed: %w", err)
	}

	token, err := s.authenticator.JWTManager().GenerateToken(
		claims.Subject, claims.TenantID, claims.Username, claims.Role,
		claims.ProjectID, claims.Namespaces, claims.IsPlatformAdmin,
	)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %w", err)
	}

	return &svcproto.AuthenticateResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(time.Duration(s.config.JWTExpireSec) * time.Second).Unix(),
		UserId:    claims.Subject,
		Role:      claims.Role,
	}, nil
}

// RevokeToken 撤销 Token
func (s *Service) RevokeToken(ctx context.Context, req *svcproto.RevokeTokenRequest) (*svcproto.RevokeTokenResponse, error) {
	// 验证 token 并获取 claims
	claims, err := s.authenticator.Authenticate(ctx, req.Token)
	if err != nil {
		return &svcproto.RevokeTokenResponse{Success: false, Message: "invalid token"}, nil
	}

	// 撤销 token
	if s.securityManager != nil {
		err = s.securityManager.RevokeToken(ctx, claims.GetJTI(), req.Reason, claims.Subject, claims.ExpiresAt.Time)
		if err != nil {
			return &svcproto.RevokeTokenResponse{Success: false, Message: err.Error()}, nil
		}
	} else {
		// 回退到原始的黑名单方法
		err = s.authenticator.RevokeToken(ctx, req.Token)
		if err != nil {
			return &svcproto.RevokeTokenResponse{Success: false, Message: err.Error()}, nil
		}
	}

	return &svcproto.RevokeTokenResponse{Success: true}, nil
}

// ValidateAPIKey 验证 API Key
func (s *Service) ValidateAPIKey(ctx context.Context, req *svcproto.ValidateAPIKeyRequest) (*svcproto.ValidateAPIKeyResponse, error) {
	if req.ApiKey == "" {
		return &svcproto.ValidateAPIKeyResponse{Valid: false}, nil
	}

	// 使用 authenticator 的 apiKeyManager 验证 API Key
	claims, err := s.authenticator.Authenticate(ctx, req.ApiKey)
	if err != nil {
		return &svcproto.ValidateAPIKeyResponse{Valid: false}, nil
	}

	return &svcproto.ValidateAPIKeyResponse{
		Valid:    true,
		UserId:   claims.Subject,
		Username: claims.Username,
		Role:     claims.Role,
		TenantId: claims.TenantID,
	}, nil
}

// ============================================================================
// P0-02 修复: 用户管理 (TiDB 持久化)
// ============================================================================

// findUserFromDB 从 TiDB 查找用户
func (s *Service) findUserFromDB(username string) (*UserInfo, error) {
	// 先从缓存查找
	if cached, ok := s.usersCache.Load(username); ok {
		if userInfo, ok := cached.(*UserInfo); ok {
			return userInfo, nil
		}
	}

	// 缓存未命中，从 TiDB 查询
	var user UserInfo
	err := s.db.QueryRow(
		"SELECT user_id, username, password, tenant_id, role FROM users WHERE username = ?",
		username,
	).Scan(&user.UserID, &user.Username, &user.Password, &user.TenantID, &user.Role)
	
	if storage.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	// 更新缓存
	s.usersCache.Store(username, &user)
	return &user, nil
}

// CreateUser 创建用户
func (s *Service) CreateUser(username, password, role, tenantID string) error {
	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	userID := fmt.Sprintf("user-%d", time.Now().UnixNano())

	_, err = s.db.Exec(
		"INSERT INTO users (user_id, username, password, tenant_id, role) VALUES (?, ?, ?, ?, ?)",
		userID, username, string(hashedPassword), tenantID, role,
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	// 更新缓存
	user := &UserInfo{
		UserID:   userID,
		Username: username,
		Password: string(hashedPassword),
		TenantID: tenantID,
		Role:     role,
	}
	s.usersCache.Store(username, user)

	return nil
}

// UpdateUser 更新用户
func (s *Service) UpdateUser(username, password, role string) error {
	if password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash password: %w", err)
		}
		_, err = s.db.Exec(
			"UPDATE users SET password = ?, role = ? WHERE username = ?",
			string(hashedPassword), role, username,
		)
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}
	} else {
		_, err := s.db.Exec(
			"UPDATE users SET role = ? WHERE username = ?",
			role, username,
		)
		if err != nil {
			return fmt.Errorf("update user role: %w", err)
		}
	}

	// 清除缓存，强制下次从 DB 读取
	s.usersCache.Delete(username)

	return nil
}

// DeleteUser 删除用户
func (s *Service) DeleteUser(username string) error {
	_, err := s.db.Exec("DELETE FROM users WHERE username = ?", username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	// 清除缓存
	s.usersCache.Delete(username)

	return nil
}

// ListUsers 列出用户
func (s *Service) ListUsers() ([]*UserInfo, error) {
	rows, err := s.db.Query("SELECT user_id, username, tenant_id, role FROM users")
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*UserInfo
	for rows.Next() {
		var user UserInfo
		if err := rows.Scan(&user.UserID, &user.Username, &user.TenantID, &user.Role); err != nil {
			continue
		}
		users = append(users, &user)
	}

	return users, nil
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s","version":"%s"}`,
		s.config.ServiceName, s.config.Version)
}

func (s *Service) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req svcproto.AuthenticateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := s.Authenticate(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	writeJSON(w, resp)
}

func (s *Service) verifyHandler(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	resp, err := s.ValidateToken(r.Context(), &svcproto.ValidateTokenRequest{Token: token})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	writeJSON(w, resp)
}

func (s *Service) refreshHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newToken, err := s.authenticator.JWTManager().RefreshToken(r.Context(), body.RefreshToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	writeJSON(w, map[string]interface{}{
		"token":      newToken,
		"expires_at": time.Now().Add(time.Duration(s.config.JWTExpireSec) * time.Second).Unix(),
	})
}

func (s *Service) revokeHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token  string `json:"token"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.Token == "" {
		body.Token = extractBearerToken(r)
	}

	if body.Token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	resp, err := s.RevokeToken(r.Context(), &svcproto.RevokeTokenRequest{
		Token:  body.Token,
		Reason: body.Reason,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp)
}

func (s *Service) oidcCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	resp, err := s.OIDCCallback(r.Context(), &svcproto.OIDCCallbackRequest{Code: code, State: state})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *Service) oidcAuthHandler(w http.ResponseWriter, r *http.Request) {
	if s.config.OIDCIssuer == "" {
		http.Error(w, "OIDC not configured", http.StatusNotImplemented)
		return
	}
	writeJSON(w, map[string]string{
		"issuer":            s.config.OIDCIssuer,
		"authorization_url": s.config.OIDCIssuer + "/auth",
		"client_id":         s.config.OIDCClientID,
		"redirect_url":      s.config.OIDCRedirectURL,
		"scopes":            s.config.OIDCScopes,
	})
}

func (s *Service) rolesHandler(w http.ResponseWriter, r *http.Request) {
	tc := tenant.MustFromContext(r.Context())
	roles, _ := s.rbacEngine.GetRolesForUser(tc.UserID, tc.TenantID, tc.ProjectID)
	writeJSON(w, map[string]interface{}{"roles": roles})
}

func (s *Service) checkPermissionHandler(w http.ResponseWriter, r *http.Request) {
	tc := tenant.MustFromContext(r.Context())
	action := r.URL.Query().Get("action")
	resource := r.URL.Query().Get("resource")

	allowed, reason := s.rbacEngine.CheckPermission(tc.UserID, tc.TenantID, tc.ProjectID, resource, action)
	writeJSON(w, map[string]interface{}{"allowed": allowed, "reason": reason})
}

func (s *Service) apiKeyHandler(w http.ResponseWriter, r *http.Request) {
	tc := tenant.MustFromContext(r.Context())

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		key, err := s.authenticator.APIKeyManager().GenerateAPIKey(tc.UserID, tc.TenantID, body.Name, 30*24*time.Hour)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"api_key": key})

	case http.MethodDelete:
		key := r.URL.Query().Get("key")
		s.authenticator.APIKeyManager().RevokeAPIKey(key)
		writeJSON(w, map[string]string{"status": "revoked"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// P0-02 修复: 用户管理 HTTP Handler
func (s *Service) usersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users, err := s.ListUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{"users": users})
}

func (s *Service) createUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
		TenantID string `json:"tenant_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.Username == "" || body.Password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}

	if body.Role == "" {
		body.Role = "user"
	}
	if body.TenantID == "" {
		body.TenantID = "default"
	}

	if err := s.CreateUser(body.Username, body.Password, body.Role, body.TenantID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "created"})
}

func (s *Service) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if body.Username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}

	if err := s.UpdateUser(body.Username, body.Password, body.Role); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "updated"})
}

func (s *Service) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}

	if err := s.DeleteUser(username); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "deleted"})
}

// ============================================================================
// 辅助函数
// ============================================================================

// findUser 保留旧方法用于向后兼容，但优先使用 TiDB
func (s *Service) findUser(username string) (*UserInfo, bool) {
	user, err := s.findUserFromDB(username)
	if err != nil || user == nil {
		return nil, false
	}
	return user, true
}

// parsePermission 解析权限字符串 "flow:read" → (action="read", resource="flow")
func parsePermission(perm string) (action, resource string) {
	parts := strings.SplitN(perm, ":", 2)
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return perm, "*"
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
