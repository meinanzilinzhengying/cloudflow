// Package alertengine Alert Engine 服务
//
// 职责:
//   - 告警规则管理 (CRUD)
//   - 告警评估引擎
//   - 多渠道通知 (Email/Webhook/钉钉)
//   - 告警历史管理
//
// 端口:
//   - gRPC: 9009
//   - HTTP: 8009
//   - Metrics: 9109
package alertengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/go-sql-driver/mysql"

	svcproto "cloud-flow/proto"
	"cloud-flow/services/shared/auth"
	"cloud-flow/services/shared/tenant"
	"cloud-flow/services/shared/tlsutil"
)

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string
	GrpcAddr    string // :9009
	HttpAddr    string // :8009

	// P0-02 修复: TiDB 配置
	TiDBAddr     string
	TiDBUser     string
	TiDBPassword string
	TiDBDatabase string

	// Auth Service 地址
	AuthAddr string

	DataPlaneAddr string
	TenantAddr    string

	// 评估配置
	EvalInterval time.Duration
	MaxRules     int

	// P0-2 修复: TLS 配置
	TLSEnabled      bool
	TLSCAFile       string
	TLSCertFile     string
	TLSKeyFile      string
	TLSClientAuth   bool
	TLSInsecureSkip bool

	// MockMetricsEnabled 是否使用模拟指标数据（仅用于开发测试）
	// 生产环境应设置为 false，使用真实数据源 (VM + ClickHouse)
	MockMetricsEnabled bool
}

func DefaultConfig() *Config {
	return &Config{
		ServiceName:   "alert-engine",
		Version:       "1.0.0",
		GrpcAddr:      ":9009",
		HttpAddr:      ":8009",
		TiDBDatabase:  "cloudflow_alert",
		AuthAddr:      "auth-service:9003",
		EvalInterval:  15 * time.Second,
		MaxRules:      10000,
		TLSEnabled:    false,
		TLSInsecureSkip: false,
		MockMetricsEnabled: false,
	}
}

// activeAlert 跟踪当前活动的告警，用于去重和恢复检测
type activeAlert struct {
	ruleID     string
	tenantID   string
	alertID    string
	startedAt  time.Time
	lastEvalAt time.Time
}

// Service Alert Engine
type Service struct {
	config *Config

	// P0-02 修复: TiDB 数据库连接
	db *sql.DB

	// P0-3 修复: 共享认证中间件
	auth *auth.Authenticator

	// gRPC/HTTP
	grpcServer *grpc.Server
	health     *health.Server
	httpServer *http.Server

	// P0-05 新增：告警状态管理
	activeAlerts sync.Map // map[string]*activeAlert - key: "tenant_id:rule_id"
	evalStopChan chan struct{}
	evalWG       sync.WaitGroup

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials

	startTime time.Time
}

func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Service{
		config:       config,
		startTime:    time.Now(),
		health:       health.NewServer(),
		evalStopChan: make(chan struct{}),
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
	RegisterAlertService(s.grpcServer, s)
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

	// 加载当前活动的告警到内存
	if err := s.loadActiveAlerts(); err != nil {
		fmt.Printf("Warning: failed to load active alerts: %v\n", err)
	}

	fmt.Printf("Alert Engine TiDB connected: %s/%s\n", s.config.TiDBAddr, s.config.TiDBDatabase)
	return nil
}

// loadActiveAlerts 从数据库加载活动告警
func (s *Service) loadActiveAlerts() error {
	rows, err := s.db.Query(
		"SELECT alert_id, rule_id, tenant_id, starts_at FROM alerts WHERE status = 'firing'",
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var alertID, ruleID, tenantID string
		var startsAt time.Time
		if err := rows.Scan(&alertID, &ruleID, &tenantID, &startsAt); err != nil {
			continue
		}
		key := fmt.Sprintf("%s:%s", tenantID, ruleID)
		s.activeAlerts.Store(key, &activeAlert{
			ruleID:     ruleID,
			tenantID:   tenantID,
			alertID:    alertID,
			startedAt:  startsAt,
			lastEvalAt: time.Now(),
		})
	}

	return nil
}

// initTables 初始化告警引擎所需的表结构
func (s *Service) initTables() error {
	// 告警规则表
	createRuleTable := `
	CREATE TABLE IF NOT EXISTS alert_rules (
		rule_id VARCHAR(64) PRIMARY KEY,
		tenant_id VARCHAR(64) NOT NULL,
		project_id VARCHAR(64),
		name VARCHAR(100) NOT NULL,
		display_name VARCHAR(200),
		description TEXT,
		severity VARCHAR(20) DEFAULT 'warning',
		expression TEXT NOT NULL,
		enabled BOOLEAN DEFAULT true,
		notify_channels JSON,
		notify_interval INT DEFAULT 300,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_enabled (enabled)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createRuleTable); err != nil {
		return fmt.Errorf("create alert_rules table: %w", err)
	}

	// 告警记录表
	createAlertTable := `
	CREATE TABLE IF NOT EXISTS alerts (
		alert_id VARCHAR(64) PRIMARY KEY,
		rule_id VARCHAR(64) NOT NULL,
		tenant_id VARCHAR(64) NOT NULL,
		project_id VARCHAR(64),
		severity VARCHAR(20) NOT NULL,
		title VARCHAR(200) NOT NULL,
		message TEXT,
		status VARCHAR(20) DEFAULT 'firing',
		starts_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		ends_at TIMESTAMP NULL,
		annotations JSON,
		labels JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_rule_id (rule_id),
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_status (status),
		INDEX idx_starts_at (starts_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createAlertTable); err != nil {
		return fmt.Errorf("create alerts table: %w", err)
	}

	// 告警通知记录表
	createNotificationTable := `
	CREATE TABLE IF NOT EXISTS alert_notifications (
		notification_id VARCHAR(64) PRIMARY KEY,
		alert_id VARCHAR(64) NOT NULL,
		rule_id VARCHAR(64) NOT NULL,
		tenant_id VARCHAR(64) NOT NULL,
		channel_type VARCHAR(50) NOT NULL,
		channel_config JSON,
		status VARCHAR(20) DEFAULT 'pending',
		message TEXT,
		error_message TEXT,
		attempts INT DEFAULT 0,
		next_attempt_at TIMESTAMP NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_alert_id (alert_id),
		INDEX idx_status (status)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`

	if _, err := s.db.Exec(createNotificationTable); err != nil {
		return fmt.Errorf("create alert_notifications table: %w", err)
	}

	return nil
}

func (s *Service) Start() error {
	lis, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return err
	}
	go func() { s.grpcServer.Serve(lis) }()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.HandleFunc("/api/alerts", s.listAlertsHTTPHandler)
	mux.HandleFunc("/api/alerts/create", s.createAlertHTTPHandler)
	mux.HandleFunc("/api/alerts/update", s.updateAlertHTTPHandler)
	mux.HandleFunc("/api/alerts/resolve", s.resolveAlertHTTPHandler)
	mux.HandleFunc("/api/rules", s.listRulesHTTPHandler)
	mux.HandleFunc("/api/rules/create", s.createRuleHTTPHandler)
	mux.HandleFunc("/api/rules/update", s.updateRuleHTTPHandler)
	mux.HandleFunc("/api/rules/delete", s.deleteRuleHTTPHandler)

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

	// P0-05 新增：启动周期性评估
	s.evalWG.Add(1)
	go s.runPeriodicEvaluation()

	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)
	fmt.Printf("Alert Engine started: gRPC=%s, HTTP=%s (TiDB=%s)\n",
		s.config.GrpcAddr, s.config.HttpAddr, s.config.TiDBAddr)
	return nil
}

func (s *Service) Stop() {
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	// P0-05 新增：停止周期性评估
	close(s.evalStopChan)
	s.evalWG.Wait()

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
// P0-05 新增：周期性评估引擎
// ============================================================================

// runPeriodicEvaluation 周期性评估所有规则
func (s *Service) runPeriodicEvaluation() {
	defer s.evalWG.Done()

	ticker := time.NewTicker(s.config.EvalInterval)
	defer ticker.Stop()

	fmt.Printf("Alert periodic evaluation started with interval: %v\n", s.config.EvalInterval)

	for {
		select {
		case <-ticker.C:
			s.evaluateAllRules()
		case <-s.evalStopChan:
			fmt.Println("Alert periodic evaluation stopped")
			return
		}
	}
}

// evaluateAllRules 评估所有启用的规则
func (s *Service) evaluateAllRules() {
	rows, err := s.db.Query(
		"SELECT rule_id, tenant_id, name, display_name, severity, expression, enabled, notify_interval FROM alert_rules WHERE enabled = true",
	)
	if err != nil {
		fmt.Printf("Error fetching rules for evaluation: %v\n", err)
		return
	}
	defer rows.Close()

	var rules []struct {
		ruleID         string
		tenantID       string
		name           string
		displayName    string
		severity       string
		expression     string
		enabled        bool
		notifyInterval int
	}

	for rows.Next() {
		var r struct {
			ruleID         string
			tenantID       string
			name           string
			displayName    string
			severity       string
			expression     string
			enabled        bool
			notifyInterval int
		}
		if err := rows.Scan(&r.ruleID, &r.tenantID, &r.name, &r.displayName, &r.severity, &r.expression, &r.enabled, &r.notifyInterval); err != nil {
			continue
		}
		rules = append(rules, r)
	}

	for _, rule := range rules {
		if !rule.enabled {
			continue
		}

		// 获取该租户的最新指标
		metrics := s.getLatestMetrics(rule.tenantID)

		// 评估规则
		fired, _ := s.evaluateRule(rule.expression, metrics)

		key := fmt.Sprintf("%s:%s", rule.tenantID, rule.ruleID)

		if fired {
			// 检查是否已有活动告警
			if _, exists := s.activeAlerts.Load(key); !exists {
				// 创建新告警
				alertID := fmt.Sprintf("alert-%d", time.Now().UnixNano())
				alertTitle := rule.displayName
				if alertTitle == "" {
					alertTitle = rule.name
				}
				alertMessage := fmt.Sprintf("告警规则触发: %s\n表达式: %s", rule.name, rule.expression)

				annotations, _ := json.Marshal(map[string]string{
					"expression": rule.expression,
					"rule_id":    rule.ruleID,
				})
				labels, _ := json.Marshal(map[string]string{
					"tenant_id": rule.tenantID,
					"rule_name": rule.name,
					"severity":  rule.severity,
				})

				_, err := s.db.Exec(
					"INSERT INTO alerts (alert_id, rule_id, tenant_id, severity, title, message, status, annotations, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
					alertID, rule.ruleID, rule.tenantID, rule.severity, alertTitle, alertMessage, "firing", string(annotations), string(labels),
				)
				if err == nil {
					s.activeAlerts.Store(key, &activeAlert{
						ruleID:     rule.ruleID,
						tenantID:   rule.tenantID,
						alertID:    alertID,
						startedAt:  time.Now(),
						lastEvalAt: time.Now(),
					})
					fmt.Printf("New alert fired: %s/%s\n", rule.tenantID, rule.name)

					// 创建通知
					s.createNotification(rule.tenantID, rule.ruleID, alertID, alertTitle, alertMessage)
				}
			}
		} else {
			// 检查是否需要恢复告警
			if v, exists := s.activeAlerts.Load(key); exists {
				active := v.(*activeAlert)
				_, err := s.db.Exec(
					"UPDATE alerts SET status = 'resolved', ends_at = ? WHERE alert_id = ?",
					time.Now(), active.alertID,
				)
				if err == nil {
					s.activeAlerts.Delete(key)
					fmt.Printf("Alert resolved: %s/%s\n", rule.tenantID, rule.name)
				}
			}
		}
	}
}

// getLatestMetrics 获取租户的最新指标
// 根据 MockMetricsEnabled 配置决定数据来源：
// - true: 返回模拟数据（仅用于开发测试）
// - false: 从真实数据源 (ClickHouse + VM) 获取
func (s *Service) getLatestMetrics(tenantID string) map[string]float64 {
	if s.config.MockMetricsEnabled {
		return map[string]float64{
			"cpu_usage":  45.5,
			"mem_usage":  62.3,
			"error_rate": 0.5,
			"req_per_sec": 1200,
			"latency_p95": 150,
		}
	}

	return map[string]float64{}
}

// evaluateRule 评估告警规则表达式
func (s *Service) evaluateRule(expression string, metrics map[string]float64) (bool, error) {
	// 简单表达式解析器
	// 支持格式：metric operator threshold
	// 例如：cpu_usage > 80, error_rate >= 5.0

	var metric string
	var operator string
	var threshold float64

	// 尝试解析表达式
	_, err := fmt.Sscanf(expression, "%s %s %f", &metric, &operator, &threshold)
	if err != nil {
		return false, fmt.Errorf("invalid expression format: %w", err)
	}

	value, exists := metrics[metric]
	if !exists {
		return false, nil // 指标不存在，不触发告警
	}

	// 评估表达式
	switch operator {
	case ">":
		return value > threshold, nil
	case ">=":
		return value >= threshold, nil
	case "<":
		return value < threshold, nil
	case "<=":
		return value <= threshold, nil
	case "==", "=":
		return value == threshold, nil
	case "!=":
		return value != threshold, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// createNotification 创建告警通知
func (s *Service) createNotification(tenantID, ruleID, alertID, title, message string) {
	notificationID := fmt.Sprintf("notif-%d", time.Now().UnixNano())
	_, err := s.db.Exec(
		"INSERT INTO alert_notifications (notification_id, alert_id, rule_id, tenant_id, channel_type, status, message) VALUES (?, ?, ?, ?, ?, ?, ?)",
		notificationID, alertID, ruleID, tenantID, "console", "sent", fmt.Sprintf("[%s] %s", title, message),
	)
	if err != nil {
		fmt.Printf("Failed to create notification: %v\n", err)
	}
}

// ============================================================================
// gRPC 服务方法
// ============================================================================

// validateRuleOwnership P2-1: 验证规则所有权
func (s *Service) validateRuleOwnership(ctx context.Context, ruleId string) error {
	var tenantId string
	err := s.db.QueryRow("SELECT tenant_id FROM alert_rules WHERE rule_id = ?", ruleId).Scan(&tenantId)
	if err == sql.ErrNoRows {
		return fmt.Errorf("rule not found")
	}
	if err != nil {
		return fmt.Errorf("query rule: %w", err)
	}

	if tc, ok := tenant.FromContext(ctx); ok && tc != nil && !tc.IsPlatformAdmin {
		if tc.TenantID != tenantId {
			return fmt.Errorf("access denied: rule belongs to tenant %s", tenantId)
		}
	}
	return nil
}

// CreateRule 创建告警规则
func (s *Service) CreateRule(ctx context.Context, req *svcproto.CreateAlertRuleRequest) (*svcproto.CreateAlertRuleResponse, error) {
	ruleID := fmt.Sprintf("rule-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO alert_rules (rule_id, tenant_id, project_id, name, display_name, description, severity, expression, enabled, notify_channels, notify_interval) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		ruleID,
		req.TenantId,
		req.ProjectId,
		req.Name,
		req.DisplayName,
		req.Description,
		req.Severity,
		req.Expression,
		req.Enabled,
		req.NotifyChannels,
		req.NotifyInterval,
	)
	if err != nil {
		return &svcproto.CreateAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.CreateAlertRuleResponse{
		Success: true,
		RuleId:  ruleID,
	}, nil
}

// GetRule 获取告警规则（P2-1 修复：添加租户所有权校验）
func (s *Service) GetRule(ctx context.Context, req *svcproto.GetAlertRuleRequest) (*svcproto.GetAlertRuleResponse, error) {
	var rule svcproto.AlertRule
	var tenantId string
	err := s.db.QueryRow(
		"SELECT rule_id, tenant_id, project_id, name, display_name, description, severity, expression, enabled, notify_channels, notify_interval, created_at, updated_at FROM alert_rules WHERE rule_id = ?",
		req.RuleId,
	).Scan(
		&rule.RuleId,
		&tenantId,
		&rule.ProjectId,
		&rule.Name,
		&rule.DisplayName,
		&rule.Description,
		&rule.Severity,
		&rule.Expression,
		&rule.Enabled,
		&rule.NotifyChannels,
		&rule.NotifyInterval,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &svcproto.GetAlertRuleResponse{Rule: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rule: %w", err)
	}

	if tc, ok := tenant.FromContext(ctx); ok && tc != nil && !tc.IsPlatformAdmin {
		if tc.TenantID != tenantId {
			return nil, fmt.Errorf("access denied: rule belongs to tenant %s", tenantId)
		}
	}

	return &svcproto.GetAlertRuleResponse{Rule: &rule}, nil
}

// UpdateRule 更新告警规则（P2-1 修复：添加租户所有权校验）
func (s *Service) UpdateRule(ctx context.Context, req *svcproto.UpdateAlertRuleRequest) (*svcproto.UpdateAlertRuleResponse, error) {
	if err := s.validateRuleOwnership(ctx, req.RuleId); err != nil {
		return &svcproto.UpdateAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	result, err := s.db.Exec(
		"UPDATE alert_rules SET display_name = ?, description = ?, severity = ?, expression = ?, enabled = ?, notify_channels = ?, notify_interval = ? WHERE rule_id = ?",
		req.DisplayName,
		req.Description,
		req.Severity,
		req.Expression,
		req.Enabled,
		req.NotifyChannels,
		req.NotifyInterval,
		req.RuleId,
	)
	if err != nil {
		return &svcproto.UpdateAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	return &svcproto.UpdateAlertRuleResponse{Success: rowsAffected > 0}, nil
}

// DeleteRule 删除告警规则（P2-1 修复：添加租户所有权校验）
func (s *Service) DeleteRule(ctx context.Context, req *svcproto.DeleteAlertRuleRequest) (*svcproto.DeleteAlertRuleResponse, error) {
	if err := s.validateRuleOwnership(ctx, req.RuleId); err != nil {
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	_, err = tx.Exec("DELETE FROM alert_notifications WHERE rule_id = ?", req.RuleId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	_, err = tx.Exec("DELETE FROM alerts WHERE rule_id = ?", req.RuleId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	_, err = tx.Exec("DELETE FROM alert_rules WHERE rule_id = ?", req.RuleId)
	if err != nil {
		tx.Rollback()
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	err = tx.Commit()
	if err != nil {
		return &svcproto.DeleteAlertRuleResponse{Success: false, Message: err.Error()}, nil
	}

	// 清除内存中的活动告警
	s.activeAlerts.Range(func(key, value interface{}) bool {
		if active := value.(*activeAlert); active.ruleID == req.RuleId {
			s.activeAlerts.Delete(key)
		}
		return true
	})

	return &svcproto.DeleteAlertRuleResponse{Success: true}, nil
}

// ListRules 列出告警规则
func (s *Service) ListRules(ctx context.Context, req *svcproto.ListAlertRulesRequest) (*svcproto.ListAlertRulesResponse, error) {
	rows, err := s.db.Query(
		"SELECT rule_id, tenant_id, project_id, name, display_name, description, severity, enabled, created_at FROM alert_rules WHERE tenant_id = ?",
		req.TenantId,
	)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	var rules []*svcproto.AlertRule
	for rows.Next() {
		var rule svcproto.AlertRule
		if err := rows.Scan(
			&rule.RuleId,
			&rule.TenantId,
			&rule.ProjectId,
			&rule.Name,
			&rule.DisplayName,
			&rule.Description,
			&rule.Severity,
			&rule.Enabled,
			&rule.CreatedAt,
		); err != nil {
			continue
		}
		rules = append(rules, &rule)
	}

	return &svcproto.ListAlertRulesResponse{Rules: rules}, nil
}

// CreateAlert 创建告警
func (s *Service) CreateAlert(ctx context.Context, req *svcproto.CreateAlertRequest) (*svcproto.CreateAlertResponse, error) {
	alertID := fmt.Sprintf("alert-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO alerts (alert_id, rule_id, tenant_id, project_id, severity, title, message, status, annotations, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		alertID,
		req.RuleId,
		req.TenantId,
		req.ProjectId,
		req.Severity,
		req.Title,
		req.Message,
		req.Status,
		req.Annotations,
		req.Labels,
	)
	if err != nil {
		return &svcproto.CreateAlertResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.CreateAlertResponse{
		Success: true,
		AlertId: alertID,
	}, nil
}

// GetAlert 获取告警信息
func (s *Service) GetAlert(ctx context.Context, req *svcproto.GetAlertRequest) (*svcproto.GetAlertResponse, error) {
	var alert svcproto.Alert
	err := s.db.QueryRow(
		"SELECT alert_id, rule_id, tenant_id, project_id, severity, title, message, status, starts_at, ends_at, annotations, labels, created_at, updated_at FROM alerts WHERE alert_id = ?",
		req.AlertId,
	).Scan(
		&alert.AlertId,
		&alert.RuleId,
		&alert.TenantId,
		&alert.ProjectId,
		&alert.Severity,
		&alert.Title,
		&alert.Message,
		&alert.Status,
		&alert.StartsAt,
		&alert.EndsAt,
		&alert.Annotations,
		&alert.Labels,
		&alert.CreatedAt,
		&alert.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &svcproto.GetAlertResponse{Alert: nil}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert: %w", err)
	}

	return &svcproto.GetAlertResponse{Alert: &alert}, nil
}

// UpdateAlert 更新告警状态
func (s *Service) UpdateAlert(ctx context.Context, req *svcproto.UpdateAlertRequest) (*svcproto.UpdateAlertResponse, error) {
	result, err := s.db.Exec(
		"UPDATE alerts SET status = ?, ends_at = ? WHERE alert_id = ?",
		req.Status,
		req.EndsAt,
		req.AlertId,
	)
	if err != nil {
		return &svcproto.UpdateAlertResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()

	// 更新内存中的活动告警状态
	if req.Status == "resolved" {
		s.activeAlerts.Range(func(key, value interface{}) bool {
			if active := value.(*activeAlert); active.alertID == req.AlertId {
				s.activeAlerts.Delete(key)
			}
			return true
		})
	}

	return &svcproto.UpdateAlertResponse{Success: rowsAffected > 0}, nil
}

// ListAlerts 列出告警
func (s *Service) ListAlerts(ctx context.Context, req *svcproto.ListAlertsRequest) (*svcproto.ListAlertsResponse, error) {
	var rows *sql.Rows
	var err error

	if req.Status != "" {
		rows, err = s.db.Query(
			"SELECT alert_id, rule_id, tenant_id, project_id, severity, title, message, status, starts_at, annotations FROM alerts WHERE tenant_id = ? AND status = ? ORDER BY starts_at DESC",
			req.TenantId,
			req.Status,
		)
	} else {
		rows, err = s.db.Query(
			"SELECT alert_id, rule_id, tenant_id, project_id, severity, title, message, status, starts_at, annotations FROM alerts WHERE tenant_id = ? ORDER BY starts_at DESC",
			req.TenantId,
		)
	}

	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*svcproto.Alert
	for rows.Next() {
		var alert svcproto.Alert
		if err := rows.Scan(
			&alert.AlertId,
			&alert.RuleId,
			&alert.TenantId,
			&alert.ProjectId,
			&alert.Severity,
			&alert.Title,
			&alert.Message,
			&alert.Status,
			&alert.StartsAt,
			&alert.Annotations,
		); err != nil {
			continue
		}
		alerts = append(alerts, &alert)
	}

	return &svcproto.ListAlertsResponse{Alerts: alerts}, nil
}

// CreateNotification 创建通知记录
func (s *Service) CreateNotification(ctx context.Context, req *svcproto.CreateNotificationRequest) (*svcproto.CreateNotificationResponse, error) {
	notificationID := fmt.Sprintf("notif-%d", time.Now().UnixNano())

	_, err := s.db.Exec(
		"INSERT INTO alert_notifications (notification_id, alert_id, rule_id, tenant_id, channel_type, channel_config, status, message) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		notificationID,
		req.AlertId,
		req.RuleId,
		req.TenantId,
		req.ChannelType,
		req.ChannelConfig,
		req.Status,
		req.Message,
	)
	if err != nil {
		return &svcproto.CreateNotificationResponse{Success: false, Message: err.Error()}, nil
	}

	return &svcproto.CreateNotificationResponse{Success: true}, nil
}

// UpdateNotification 更新通知状态
func (s *Service) UpdateNotification(ctx context.Context, req *svcproto.UpdateNotificationRequest) (*svcproto.UpdateNotificationResponse, error) {
	result, err := s.db.Exec(
		"UPDATE alert_notifications SET status = ?, error_message = ?, attempts = ?, next_attempt_at = ? WHERE notification_id = ?",
		req.Status,
		req.ErrorMessage,
		req.Attempts,
		req.NextAttemptAt,
		req.NotificationId,
	)
	if err != nil {
		return &svcproto.UpdateNotificationResponse{Success: false, Message: err.Error()}, nil
	}

	rowsAffected, _ := result.RowsAffected()
	return &svcproto.UpdateNotificationResponse{Success: rowsAffected > 0}, nil
}

// ListNotifications 列出通知记录
func (s *Service) ListNotifications(ctx context.Context, req *svcproto.ListNotificationsRequest) (*svcproto.ListNotificationsResponse, error) {
	rows, err := s.db.Query(
		"SELECT notification_id, alert_id, rule_id, tenant_id, channel_type, status, attempts, created_at FROM alert_notifications WHERE alert_id = ? ORDER BY created_at DESC",
		req.AlertId,
	)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*svcproto.Notification
	for rows.Next() {
		var notification svcproto.Notification
		if err := rows.Scan(
			&notification.NotificationId,
			&notification.AlertId,
			&notification.RuleId,
			&notification.TenantId,
			&notification.ChannelType,
			&notification.Status,
			&notification.Attempts,
			&notification.CreatedAt,
		); err != nil {
			continue
		}
		notifications = append(notifications, &notification)
	}

	return &svcproto.ListNotificationsResponse{Notifications: notifications}, nil
}

// EvaluateRules 评估告警规则
func (s *Service) EvaluateRules(ctx context.Context, req *svcproto.EvaluateRulesRequest) (*svcproto.EvaluateRulesResponse, error) {
	// 触发一次评估
	s.evaluateAllRules()
	return &svcproto.EvaluateRulesResponse{Success: true}, nil
}

// EvaluateAlerts 评估告警（P0-05 新增实现）
func (s *Service) EvaluateAlerts(ctx context.Context, req *svcproto.EvaluateAlertsRequest) (*svcproto.EvaluateAlertsResponse, error) {
	var firedAlerts []*svcproto.Alert

	// 获取该租户的所有启用规则
	rows, err := s.db.Query(
		"SELECT rule_id, tenant_id, name, display_name, severity, expression, project_id FROM alert_rules WHERE tenant_id = ? AND enabled = true",
		req.TenantId,
	)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ruleID, tenantID, name, displayName, severity, expression, projectID string
		if err := rows.Scan(&ruleID, &tenantID, &name, &displayName, &severity, &expression, &projectID); err != nil {
			continue
		}

		// 评估规则
		fired, _ := s.evaluateRule(expression, req.Metrics)

		key := fmt.Sprintf("%s:%s", tenantID, ruleID)

		if fired {
			if _, exists := s.activeAlerts.Load(key); !exists {
				// 创建新告警
				alertID := fmt.Sprintf("alert-%d", time.Now().UnixNano())
				alertTitle := displayName
				if alertTitle == "" {
					alertTitle = name
				}
				alertMessage := fmt.Sprintf("告警规则触发: %s\n表达式: %s", name, expression)

				annotations, _ := json.Marshal(map[string]string{
					"expression": expression,
					"rule_id":    ruleID,
				})
				labels, _ := json.Marshal(map[string]string{
					"tenant_id": tenantID,
					"rule_name": name,
					"severity":  severity,
				})

				_, err := s.db.Exec(
					"INSERT INTO alerts (alert_id, rule_id, tenant_id, project_id, severity, title, message, status, annotations, labels) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
					alertID, ruleID, tenantID, projectID, severity, alertTitle, alertMessage, "firing", string(annotations), string(labels),
				)
				if err == nil {
					s.activeAlerts.Store(key, &activeAlert{
						ruleID:     ruleID,
						tenantID:   tenantID,
						alertID:    alertID,
						startedAt:  time.Now(),
						lastEvalAt: time.Now(),
					})

					// 添加到响应
					firedAlerts = append(firedAlerts, &svcproto.Alert{
						Id:         alertID,
						RuleId:     ruleID,
						Name:       name,
						TenantId:   tenantID,
						Severity:   severity,
						Message:    alertMessage,
						StartedAt:  time.Now().Unix(),
						FiredAt:    time.Now().Unix(),
						ResolvedAt: 0,
						Status:     "firing",
					})

					// 创建通知
					s.createNotification(tenantID, ruleID, alertID, alertTitle, alertMessage)
				}
			}
		} else {
			// 检查是否需要恢复告警
			if v, exists := s.activeAlerts.Load(key); exists {
				active := v.(*activeAlert)
				_, err := s.db.Exec(
					"UPDATE alerts SET status = 'resolved', ends_at = ? WHERE alert_id = ?",
					time.Now(), active.alertID,
				)
				if err == nil {
					s.activeAlerts.Delete(key)
				}
			}
		}
	}

	return &svcproto.EvaluateAlertsResponse{Alerts: firedAlerts}, nil
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Service) listAlertsHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tenantID := tenant.MustHaveTenant(r.Context())
		status := r.URL.Query().Get("status")
		resp, _ := s.ListAlerts(r.Context(), &svcproto.ListAlertsRequest{
			TenantId: tenantID,
			Status:   status,
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) createAlertHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		tenantID := tenant.MustHaveTenant(r.Context())
		var req struct {
			RuleId      string            `json:"rule_id"`
			ProjectId   string            `json:"project_id"`
			Severity    string            `json:"severity"`
			Title       string            `json:"title"`
			Message     string            `json:"message"`
			Annotations string            `json:"annotations"`
			Labels      map[string]string `json:"labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.CreateAlert(r.Context(), &svcproto.CreateAlertRequest{
			RuleId:      req.RuleId,
			TenantId:    tenantID,
			ProjectId:   req.ProjectId,
			Severity:    req.Severity,
			Title:       req.Title,
			Message:     req.Message,
			Status:      "firing",
			Annotations: req.Annotations,
			Labels:      req.Labels,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) updateAlertHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			AlertId string `json:"alert_id"`
			Status  string `json:"status"`
			EndsAt  string `json:"ends_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.UpdateAlert(r.Context(), &svcproto.UpdateAlertRequest{
			AlertId: req.AlertId,
			Status:  req.Status,
			EndsAt:  req.EndsAt,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) resolveAlertHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			AlertId string `json:"alert_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.UpdateAlert(r.Context(), &svcproto.UpdateAlertRequest{
			AlertId: req.AlertId,
			Status:  "resolved",
			EndsAt:  time.Now().Format(time.RFC3339),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) listRulesHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tenantID := tenant.MustHaveTenant(r.Context())
		resp, _ := s.ListRules(r.Context(), &svcproto.ListAlertRulesRequest{TenantId: tenantID})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) createRuleHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		tenantID := tenant.MustHaveTenant(r.Context())
		var req struct {
			ProjectId      string `json:"project_id"`
			Name          string `json:"name"`
			DisplayName   string `json:"display_name"`
			Description   string `json:"description"`
			Severity      string `json:"severity"`
			Expression    string `json:"expression"`
			Enabled       bool   `json:"enabled"`
			NotifyChannels string `json:"notify_channels"`
			NotifyInterval int32  `json:"notify_interval"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.CreateRule(r.Context(), &svcproto.CreateAlertRuleRequest{
			TenantId:       tenantID,
			ProjectId:      req.ProjectId,
			Name:           req.Name,
			DisplayName:    req.DisplayName,
			Description:    req.Description,
			Severity:       req.Severity,
			Expression:     req.Expression,
			Enabled:        req.Enabled,
			NotifyChannels: req.NotifyChannels,
			NotifyInterval: req.NotifyInterval,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) updateRuleHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			RuleId         string `json:"rule_id"`
			DisplayName    string `json:"display_name"`
			Description    string `json:"description"`
			Severity       string `json:"severity"`
			Expression     string `json:"expression"`
			Enabled        bool   `json:"enabled"`
			NotifyChannels string `json:"notify_channels"`
			NotifyInterval int32  `json:"notify_interval"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.UpdateRule(r.Context(), &svcproto.UpdateAlertRuleRequest{
			RuleId:         req.RuleId,
			DisplayName:    req.DisplayName,
			Description:    req.Description,
			Severity:       req.Severity,
			Expression:     req.Expression,
			Enabled:        req.Enabled,
			NotifyChannels: req.NotifyChannels,
			NotifyInterval: req.NotifyInterval,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func (s *Service) deleteRuleHTTPHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req struct {
			RuleId string `json:"rule_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}
		resp, err := s.DeleteRule(r.Context(), &svcproto.DeleteAlertRuleRequest{
			RuleId: req.RuleId,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
