// Package tenantservice Tenant Service 服务
//
// 职责:
//   - 租户 CRUD
//   - 项目管理 (Project)
//   - 配额管理
//   - Plan 管理 (free/pro/enterprise)
//   - 租户隔离
//   - tenant_id 全链路透传
//
// 端口:
//   - gRPC: 9010
//   - HTTP: 8010
//   - Metrics: 9110
package tenantservice

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/go-sql-driver/mysql"

	svcproto "cloud-flow/services/proto"
	"cloud-flow/services/shared/auth"
	"cloud-flow/services/shared/tenant"
	"cloud-flow/services/shared/tlsutil"
)

// ============================================================================
// 配置
// ============================================================================

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string
	GrpcAddr    string // :9010
	HttpAddr    string // :8010

	// Auth Service 地址
	AuthAddr string

	// P0-02 修复: TiDB 配置
	TiDBAddr     string
	TiDBUser     string
	TiDBPassword string
	TiDBDatabase string

	DefaultRetentionDays   int
	DefaultMaxAgents       int
	DefaultMaxFlowsPerDay  int64
	DefaultMaxStorageGB    int
	DefaultMaxAlertRules   int

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
	return &Config{
		ServiceName:           "tenant-service",
		Version:               "1.0.0",
		GrpcAddr:              ":9010",
		HttpAddr:              ":8010",
		AuthAddr:              "auth-service:9006",
		TiDBDatabase:          "cloudflow_tenant",
		DefaultRetentionDays:  30,
		DefaultMaxAgents:      100,
		DefaultMaxFlowsPerDay: 10_000_000,
		DefaultMaxStorageGB:   100,
		DefaultMaxAlertRules:  100,
		TLSEnabled:            false,
		TLSInsecureSkip:       false,
	}
}

// ============================================================================
// 服务
// ============================================================================

// Service Tenant Service
type Service struct {
	config *Config

	// P0-02 修复: TiDB 数据库连接
	db *sql.DB

	// Auth Service 连接（旧字段保留用于兼容）
	authConn *grpc.ClientConn

	// P0-3 修复: 共享认证中间件
	auth *auth.Authenticator

	// gRPC/HTTP
	grpcServer *grpc.Server
	health     *health.Server
	httpServer *http.Server

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials

	startTime time.Time
}

// New 创建服务
func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Service{
		config:    config,
		startTime: time.Now(),
		health:    health.NewServer(),
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

	// P0-3 修复: 使用共享认证中间件
	if config.AuthAddr != "" {
		authMiddleware, err := auth.NewAuthenticator(auth.Config{
			AuthAddr:     config.AuthAddr,
			TLSEnabled:   config.TLSEnabled,
			CAFile:       config.TLSCAFile,
			CertFile:     config.TLSCertFile,
			KeyFile:      config.TLSKeyFile,
			InsecureSkip: config.TLSInsecureSkip,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to init auth middleware: %w", err)
		}
		s.auth = authMiddleware
	}

	// P0-02 修复: 初始化 TiDB 连接
	if config.TiDBAddr != "" {
		if err := s.initTiDB(); err != nil {
			return nil, fmt.Errorf("TiDB init failed: %w", err)
		}
	}

	// 初始化 gRPC 服务器
	var grpcOptions []grpc.ServerOption
	if s.grpcCreds != nil {
		grpcOptions = append(grpcOptions, grpc.Creds(s.grpcCreds))
	}
	s.grpcServer = grpc.NewServer(grpcOptions...)
	RegisterTenantService(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	return s, nil
}

// initTiDB P0-02 修复: 初始化 TiDB 连接和表结构
func (s *Service) initTiDB() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_bin",
		s.config.TiDBUser,
		s.config.TiDBPassword,
		s.config.TiDBAddr,
		s.config.TiDBDatabase,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("TiDB open failed: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("TiDB ping failed: %w", err)
	}

	s.db = db

	// 初始化表结构
	if err := s.initTables(); err != nil {
		return fmt.Errorf("init tables failed: %w", err)
	}

	fmt.Printf("Tenant Service TiDB connected: %s/%s\n", s.config.TiDBAddr, s.config.TiDBDatabase)
	return nil
}

// initTables 初始化租户服务所需的表结构
func (s *Service) initTables() error {
	// 租户表
	createTenantTable := `
	CREATE TABLE IF NOT EXISTS tenants (
		tenant_id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		display_name VARCHAR(200),
		description TEXT,
		plan VARCHAR(20) DEFAULT 'free',
		status VARCHAR(20) DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_name (name),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createTenantTable); err != nil {
		return fmt.Errorf("create tenants table: %w", err)
	}

	// 项目表
	createProjectTable := `
	CREATE TABLE IF NOT EXISTS projects (
		project_id VARCHAR(64) PRIMARY KEY,
		tenant_id VARCHAR(64) NOT NULL,
		name VARCHAR(100) NOT NULL,
		display_name VARCHAR(200),
		description TEXT,
		status VARCHAR(20) DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_name (name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createProjectTable); err != nil {
		return fmt.Errorf("create projects table: %w", err)
	}

	// 配额表
	createQuotaTable := `
	CREATE TABLE IF NOT EXISTS quotas (
		quota_id VARCHAR(64) PRIMARY KEY,
		tenant_id VARCHAR(64) NOT NULL,
		project_id VARCHAR(64),
		max_agents INT DEFAULT 100,
		max_flows_per_day BIGINT DEFAULT 10000000,
		max_storage_gb INT DEFAULT 100,
		max_alert_rules INT DEFAULT 100,
		retention_days INT DEFAULT 30,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_project_id (project_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createQuotaTable); err != nil {
		return fmt.Errorf("create quotas table: %w", err)
	}

	// 租户成员表
	createMemberTable := `
	CREATE TABLE IF NOT EXISTS tenant_members (
		member_id VARCHAR(64) PRIMARY KEY,
		tenant_id VARCHAR(64) NOT NULL,
		user_id VARCHAR(64) NOT NULL,
		role VARCHAR(20) DEFAULT 'user',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_user_id (user_id),
		UNIQUE INDEX idx_tenant_user (tenant_id, user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createMemberTable); err != nil {
		return fmt.Errorf("create tenant_members table: %w", err)
	}

	// 创建默认租户（如果不存在）
	return s.createDefaultTenant()
}

// createDefaultTenant 创建默认租户
func (s *Service) createDefaultTenant() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tenants WHERE tenant_id = 'default'").Scan(&count)
	if err != nil {
		return fmt.Errorf("check default tenant: %w", err)
	}

	if count == 0 {
		_, err = s.db.Exec(
			"INSERT INTO tenants (tenant_id, name, display_name, description, plan) VALUES (?, ?, ?, ?, ?)",
			"default",
			"default",
			"默认租户",
			"系统默认租户",
			"free",
		)
		if err != nil {
			return fmt.Errorf("create default tenant: %w", err)
		}

		// 创建默认租户的配额
		_, err = s.db.Exec(
			"INSERT INTO quotas (quota_id, tenant_id, max_agents, max_flows_per_day, max_storage_gb, max_alert_rules, retention_days) VALUES (?, ?, ?, ?, ?, ?, ?)",
			"quota-default",
			"default",
			s.config.DefaultMaxAgents,
			s.config.DefaultMaxFlowsPerDay,
			s.config.DefaultMaxStorageGB,
			s.config.DefaultMaxAlertRules,
			s.config.DefaultRetentionDays,
		)
		if err != nil {
			return fmt.Errorf("create default quota: %w", err)
		}

		fmt.Printf("Default tenant created\n")
	}

	return nil
}

// Start 启动服务
func (s *Service) Start() error {
	// 启动 gRPC
	lis, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listen failed: %w", err)
	}

	go func() { s.grpcServer.Serve(lis) }()

	// 启动 HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.HandleFunc("/api/tenants", s.tenantsHandler)
	mux.HandleFunc("/api/tenants/create", s.createTenantHandler)
	mux.HandleFunc("/api/tenants/update", s.updateTenantHandler)
	mux.HandleFunc("/api/tenants/delete", s.deleteTenantHandler)
	mux.HandleFunc("/api/projects", s.projectsHandler)
	mux.HandleFunc("/api/quotas", s.quotasHandler)

	var handler http.Handler = mux
	// P0-3 修复: 应用共享认证中间件
	if s.auth != nil {
		handler = s.auth.Middleware("/healthz")(handler)
	}
	// 始终应用租户中间件（在认证后）
	handler = tenant.HTTPMiddleware(handler)

	s.httpServer = &http.Server{
		Addr:         s.config.HttpAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() { s.httpServer.ListenAndServe() }()

	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)
	fmt.Printf("Tenant Service started: gRPC=%s, HTTP=%s (TiDB=%s)\n",
		s.config.GrpcAddr, s.config.HttpAddr, s.config.TiDBAddr)
	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
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

	// P0-3 修复: 清理认证中间件资源
	if s.auth != nil {
		s.auth.Close()
	}
}

// ============================================================================
// gRPC 服务方法
// ============================================================================

// CreateTenant 创建租户
func (s *Service) CreateTenant(ctx context.Context, req *svcproto.CreateTenantRequest) (*svcproto.CreateTenantResponse, error) {
	tenantID := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO tenants (tenant_id, name, display_name, description, plan) VALUES (?, ?, ?, ?, ?)",
		tenantID,
		req.Name,
		req.DisplayName,
		req.Description,
		req.Plan,
	)
	if err != nil {
		return &svcproto.CreateTenantResponse{Success: false, Message: err.Error()}, nil
	}

	// 创建默认配额
	_, err = s.db.Exec(
		"INSERT INTO quotas (quota_id, tenant_id, max_agents, max_flows_per_day, max_storage_gb, max_alert_rules, retention_days) VALUES (?, ?, ?, ?, ?, ?, ?)",
		fmt.Sprintf("quota-%s", tenantID),
		tenantID,
		s.config.DefaultMaxAgents,
		s.config.DefaultMaxFlowsPerDay,
		s.config.DefaultMaxStorageGB,
		s.config.DefaultMaxAlertRules,
		s.config.DefaultRetentionDays,
	)
	if err != nil {
		return &svcproto.CreateTenantResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.CreateTenantResponse{
		Success: true,
		TenantId: tenantID,
	}, nil
}

// GetTenant 获取租户信息
func (s *Service) GetTenant(ctx context.Context, req *svcproto.GetTenantRequest) (*svcproto.GetTenantResponse, error) {
	var tenant svcproto.Tenant
	err := s.db.QueryRow(
		"SELECT tenant_id, name, display_name, description, plan, status, created_at, updated_at FROM tenants WHERE tenant_id = ?",
		req.TenantId,
	).Scan(
		&tenant.TenantId,
		&tenant.Name,
		&tenant.DisplayName,
		&tenant.Description,
		&tenant.Plan,
		&tenant.Status,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &svcproto.GetTenantResponse{Tenant: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}

	return &svcproto.GetTenantResponse{Tenant: &tenant}, nil
}

// UpdateTenant 更新租户信息
func (s *Service) UpdateTenant(ctx context.Context, req *svcproto.UpdateTenantRequest) (*svcproto.UpdateTenantResponse, error) {
	result, err := s.db.Exec(
		"UPDATE tenants SET display_name = ?, description = ?, plan = ?, status = ? WHERE tenant_id = ?",
		req.DisplayName,
		req.Description,
		req.Plan,
		req.Status,
		req.TenantId,
	)
	if err != nil {
		return &svcproto.UpdateTenantResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	return &svcproto.UpdateTenantResponse{Success: rowsAffected > 0}, nil
}

// DeleteTenant 删除租户
func (s *Service) DeleteTenant(ctx context.Context, req *svcproto.DeleteTenantRequest) (*svcproto.DeleteTenantResponse, error) {
	// 不能删除默认租户
	if req.TenantId == "default" {
		return &svcproto.DeleteTenantResponse{Success: false, Message: "cannot delete default tenant"}, nil
	}

	// 开启事务
	tx, err := s.db.Begin()
	if err != nil {
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	// 删除配额
	_, err = tx.Exec("DELETE FROM quotas WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	// 删除项目
	_, err = tx.Exec("DELETE FROM projects WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	// 删除成员
	_, err = tx.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	// 删除租户
	_, err = tx.Exec("DELETE FROM tenants WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	err = tx.Commit()
	if err != nil {
		return &svcproto.DeleteTenantResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.DeleteTenantResponse{Success: true}, nil
}

// ListTenants 列出所有租户
func (s *Service) ListTenants(ctx context.Context, req *svcproto.ListTenantsRequest) (*svcproto.ListTenantsResponse, error) {
	rows, err := s.db.Query("SELECT tenant_id, name, display_name, description, plan, status, created_at FROM tenants")
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*svcproto.Tenant
	for rows.Next() {
		var tenant svcproto.Tenant
		if err := rows.Scan(
			&tenant.TenantId,
			&tenant.Name,
			&tenant.DisplayName,
			&tenant.Description,
			&tenant.Plan,
			&tenant.Status,
			&tenant.CreatedAt,
		); err != nil {
			continue
		}
		tenants = append(tenants, &tenant)
	}

	return &svcproto.ListTenantsResponse{Tenants: tenants}, nil
}

// CreateProject 创建项目
func (s *Service) CreateProject(ctx context.Context, req *svcproto.CreateProjectRequest) (*svcproto.CreateProjectResponse, error) {
	projectID := fmt.Sprintf("project-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO projects (project_id, tenant_id, name, display_name, description) VALUES (?, ?, ?, ?, ?)",
		projectID,
		req.TenantId,
		req.Name,
		req.DisplayName,
		req.Description,
	)
	if err != nil {
		return &svcproto.CreateProjectResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.CreateProjectResponse{
		Success:  true,
		ProjectId: projectID,
	}, nil
}

// GetProject 获取项目信息
func (s *Service) GetProject(ctx context.Context, req *svcproto.GetProjectRequest) (*svcproto.GetProjectResponse, error) {
	var project svcproto.Project
	err := s.db.QueryRow(
		"SELECT project_id, tenant_id, name, display_name, description, status, created_at FROM projects WHERE project_id = ?",
		req.ProjectId,
	).Scan(
		&project.ProjectId,
		&project.TenantId,
		&project.Name,
		&project.DisplayName,
		&project.Description,
		&project.Status,
		&project.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return &svcproto.GetProjectResponse{Project: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	return &svcproto.GetProjectResponse{Project: &project}, nil
}

// ListProjects 列出租户下的项目
func (s *Service) ListProjects(ctx context.Context, req *svcproto.ListProjectsRequest) (*svcproto.ListProjectsResponse, error) {
	rows, err := s.db.Query("SELECT project_id, tenant_id, name, display_name, description, status, created_at FROM projects WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []*svcproto.Project
	for rows.Next() {
		var project svcproto.Project
		if err := rows.Scan(
			&project.ProjectId,
			&project.TenantId,
			&project.Name,
			&project.DisplayName,
			&project.Description,
			&project.Status,
			&project.CreatedAt,
		); err != nil {
			continue
		}
		projects = append(projects, &project)
	}

	return &svcproto.ListProjectsResponse{Projects: projects}, nil
}

// GetQuota 获取配额信息
func (s *Service) GetQuota(ctx context.Context, req *svcproto.GetQuotaRequest) (*svcproto.GetQuotaResponse, error) {
	var quota svcproto.Quota
	err := s.db.QueryRow(
		"SELECT quota_id, tenant_id, project_id, max_agents, max_flows_per_day, max_storage_gb, max_alert_rules, retention_days FROM quotas WHERE tenant_id = ?",
		req.TenantId,
	).Scan(
		&quota.QuotaId,
		&quota.TenantId,
		&quota.ProjectId,
		&quota.MaxAgents,
		&quota.MaxFlowsPerDay,
		&quota.MaxStorageGb,
		&quota.MaxAlertRules,
		&quota.RetentionDays,
	)
	if err == sql.ErrNoRows {
		// 返回默认配额
		return &svcproto.GetQuotaResponse{
			Quota: &svcproto.Quota{
				TenantId:        req.TenantId,
				MaxAgents:       int32(s.config.DefaultMaxAgents),
				MaxFlowsPerDay:  s.config.DefaultMaxFlowsPerDay,
				MaxStorageGb:    int32(s.config.DefaultMaxStorageGB),
				MaxAlertRules:   int32(s.config.DefaultMaxAlertRules),
				RetentionDays:   int32(s.config.DefaultRetentionDays),
			},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get quota: %w", err)
	}

	return &svcproto.GetQuotaResponse{Quota: &quota}, nil
}

// UpdateQuota 更新配额信息
func (s *Service) UpdateQuota(ctx context.Context, req *svcproto.UpdateQuotaRequest) (*svcproto.UpdateQuotaResponse, error) {
	result, err := s.db.Exec(
		"UPDATE quotas SET max_agents = ?, max_flows_per_day = ?, max_storage_gb = ?, max_alert_rules = ?, retention_days = ? WHERE tenant_id = ?",
		req.MaxAgents,
		req.MaxFlowsPerDay,
		req.MaxStorageGb,
		req.MaxAlertRules,
		req.RetentionDays,
		req.TenantId,
	)
	if err != nil {
		return &svcproto.UpdateQuotaResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	return &svcproto.UpdateQuotaResponse{Success: rowsAffected > 0}, nil
}

// AddTenantMember 添加租户成员
func (s *Service) AddTenantMember(ctx context.Context, req *svcproto.AddTenantMemberRequest) (*svcproto.AddTenantMemberResponse, error) {
	memberID := fmt.Sprintf("member-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO tenant_members (member_id, tenant_id, user_id, role) VALUES (?, ?, ?, ?)",
		memberID,
		req.TenantId,
		req.UserId,
		req.Role,
	)
	if err != nil {
		return &svcproto.AddTenantMemberResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.AddTenantMemberResponse{Success: true}, nil
}

// RemoveTenantMember 移除租户成员
func (s *Service) RemoveTenantMember(ctx context.Context, req *svcproto.RemoveTenantMemberRequest) (*svcproto.RemoveTenantMemberResponse, error) {
	result, err := s.db.Exec(
		"DELETE FROM tenant_members WHERE tenant_id = ? AND user_id = ?",
		req.TenantId,
		req.UserId,
	)
	if err != nil {
		return &svcproto.RemoveTenantMemberResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	return &svcproto.RemoveTenantMemberResponse{Success: rowsAffected > 0}, nil
}

// ListTenantMembers 列出租户成员
func (s *Service) ListTenantMembers(ctx context.Context, req *svcproto.ListTenantMembersRequest) (*svcproto.ListTenantMembersResponse, error) {
	rows, err := s.db.Query("SELECT member_id, tenant_id, user_id, role, created_at FROM tenant_members WHERE tenant_id = ?", req.TenantId)
	if err != nil {
		return nil, fmt.Errorf("list tenant members: %w", err)
	}
	defer rows.Close()

	var members []*svcproto.TenantMember
	for rows.Next() {
		var member svcproto.TenantMember
		if err := rows.Scan(
			&member.MemberId,
			&member.TenantId,
			&member.UserId,
			&member.Role,
			&member.CreatedAt,
		); err != nil {
			continue
		}
		members = append(members, &member)
	}

	return &svcproto.ListTenantMembersResponse{Members: members}, nil
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Service) tenantsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		resp, _ := s.ListTenants(r.Context(), &svcproto.ListTenantsRequest{})
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (s *Service) createTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req svcproto.CreateTenantRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp, _ := s.CreateTenant(r.Context(), &req)
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (s *Service) updateTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req svcproto.UpdateTenantRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp, _ := s.UpdateTenant(r.Context(), &req)
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (s *Service) deleteTenantHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req svcproto.DeleteTenantRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp, _ := s.DeleteTenant(r.Context(), &req)
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (s *Service) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tenantID := r.URL.Query().Get("tenant_id")
		resp, _ := s.ListProjects(r.Context(), &svcproto.ListProjectsRequest{TenantId: tenantID})
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}

func (s *Service) quotasHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tenantID := r.URL.Query().Get("tenant_id")
		resp, _ := s.GetQuota(r.Context(), &svcproto.GetQuotaRequest{TenantId: tenantID})
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
}
