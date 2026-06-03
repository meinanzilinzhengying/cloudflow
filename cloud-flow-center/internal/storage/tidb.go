// Package storage 提供基于 TiDB 的数据持久化
// 支持分布式存储、分区管理和SQL聚合查询
package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/go-sql-driver/mysql"

	edge "github.com/meinanzilinzhengying/cloudflow/proto"
	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
	"github.com/meinanzilinzhengying/cloudflow/pkg/safety"
	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
)

// 随机数生成器在 Go 1.20+ 中自动种子化，不需要手动调用 Seed
// func init() {
// 	rand.Seed(time.Now().UnixNano())
// }

// ============================================================================
// M1 修复: 类型安全结构体定义
// ============================================================================

// Business 业务实体
type Business struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BusinessCreateRequest 创建业务请求
type BusinessCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

// BusinessUpdateRequest 更新业务请求
type BusinessUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// Service 服务实体
type Service struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	BusinessID  string    `json:"business_id"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Owner       string    `json:"owner"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Collector 采集器实体
type Collector struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Type     string                 `json:"type"`
	Config   map[string]interface{} `json:"config,omitempty"`
	Status   string                 `json:"status"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// DataNode 数据节点实体
type DataNode struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Endpoint   string    `json:"endpoint"`
	Status     string    `json:"status"`
	Tags       []string  `json:"tags,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ProbeNode 探针节点实体（M1 修复: 为 GetNodes 定义结构体）
type ProbeNode struct {
	EdgeNodeID   string                 `json:"edge_node_id"`
	Hostname     string                 `json:"hostname,omitempty"`
	HostIP       string                 `json:"host_ip,omitempty"`
	Status       string                 `json:"status,omitempty"`
	Version      string                 `json:"version,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// SystemOverview 系统概览（M1 修复: 为 GetOverview 定义结构体）
type SystemOverview struct {
	TotalNodes    int `json:"total_nodes"`
	OnlineNodes   int `json:"online_nodes"`
	OfflineNodes  int `json:"offline_nodes"`
	TotalServices int `json:"total_services"`
	Storage       string `json:"storage"`
	Nodes         int `json:"nodes"`
	Days          int `json:"days"`
	TodayMetrics  int `json:"today_metrics"`
	TodayTraces   int `json:"today_traces"`
	TodayProfs    int `json:"today_profs"`
}

// UserPreferences 用户偏好设置
type UserPreferences struct {
	Username       string `json:"username"`
	Theme         string `json:"theme"`
	Language      string `json:"language"`
	PageSize      int    `json:"page_size"`
	RefreshInterval int   `json:"refresh_interval"`
}

// PaginationResult 分页结果
type PaginationResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// ToMap 将 Business 转换为 map（向后兼容）
func (b *Business) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":          b.ID,
		"name":        b.Name,
		"description": b.Description,
		"status":      b.Status,
		"owner":       b.Owner,
		"created_at":   b.CreatedAt.Unix(),
		"updated_at":   b.UpdatedAt.Unix(),
	}
}

// ToMap 将 Service 转换为 map（向后兼容）
func (s *Service) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":           s.ID,
		"name":         s.Name,
		"business_id":  s.BusinessID,
		"description":  s.Description,
		"status":       s.Status,
		"owner":        s.Owner,
		"created_at":   s.CreatedAt.Unix(),
		"updated_at":   s.UpdatedAt.Unix(),
	}
}

// ToMap 将 Collector 转换为 map（向后兼容）
func (c *Collector) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":         c.ID,
		"name":       c.Name,
		"type":       c.Type,
		"config":     c.Config,
		"status":     c.Status,
		"created_at": c.CreatedAt.Unix(),
		"updated_at": c.UpdatedAt.Unix(),
	}
}

// ToMap 将 DataNode 转换为 map（向后兼容）
func (d *DataNode) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":         d.ID,
		"name":       d.Name,
		"type":       d.Type,
		"endpoint":   d.Endpoint,
		"status":     d.Status,
		"tags":       d.Tags,
		"created_at": d.CreatedAt.Unix(),
		"updated_at": d.UpdatedAt.Unix(),
	}
}

// ToMap 将 ProbeNode 转换为 map（向后兼容）
func (p *ProbeNode) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"edge_node_id": p.EdgeNodeID,
		"hostname":     p.Hostname,
		"host_ip":      p.HostIP,
		"status":       p.Status,
		"version":      p.Version,
		"tags":         p.Tags,
		"metadata":     p.Metadata,
		"payload":      p.Payload,
		"updated_at":   p.UpdatedAt.Unix(),
	}
}

// ToMap 将 SystemOverview 转换为 map（向后兼容）
func (o *SystemOverview) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"status":        "running",
		"total_nodes":   o.TotalNodes,
		"online_nodes":  o.OnlineNodes,
		"offline_nodes": o.OfflineNodes,
		"total_services": o.TotalServices,
		"storage":       o.Storage,
		"nodes":         o.Nodes,
		"days":          o.Days,
		"today_metrics": o.TodayMetrics,
		"today_traces":  o.TodayTraces,
		"today_profs":   o.TodayProfs,
	}
}

// ToMap 将 UserPreferences 转换为 map（向后兼容）
func (u *UserPreferences) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"username":        u.Username,
		"theme":          u.Theme,
		"language":       u.Language,
		"page_size":      u.PageSize,
		"refresh_interval": u.RefreshInterval,
	}
}

// TiDBStorage TiDB存储引擎实现
type TiDBStorage struct {
	db            *sql.DB
	retDays       int
	logger        *logger.Logger
	stopCh        chan struct{}
	muCache       sync.RWMutex
	overviewCache map[string]interface{}
	nodesCache    []map[string]interface{}
	overviewCacheExpiry time.Time // overview 缓存独立过期时间
	nodesCacheExpiry    time.Time // nodes 缓存独立过期时间
	// cacheInvalidationTime 记录每个缓存 key 的失效时间，实现按 key 粒度失效
	// key 为缓存类型（"overview" 或 "nodes"），value 为该 key 被标记失效的时间
	cacheInvalidationTime map[string]time.Time
	stopped       sync.Once

	// H6 修复: 缓存大小限制和统计
	maxNodesCacheSize   int           // nodesCache 最大条目数
	cacheStats          CacheStats    // 缓存统计信息

	// 批量写入引擎
	batchEngine *BatchWriteEngine
}

// CacheStats 缓存统计信息
type CacheStats struct {
	NodesCacheSize    int           // 当前 nodes 缓存条目数
	NodesCacheBytes   int64         // 当前 nodes 缓存占用字节数（估算）
	OverviewCacheSize int           // 当前 overview 缓存条目数
	CacheHits         uint64        // 缓存命中次数
	CacheMisses       uint64        // 缓存未命中次数
	LastUpdated       time.Time     // 最后更新时间
}

// NewTiDB 创建TiDB存储引擎实例
func NewTiDB(dsn string, retDays int, log *logger.Logger) (*TiDBStorage, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("连接 TiDB 失败: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 优化连接池（TiDB 分布式特性调优）
	OptimizeConnectionPool(db, DefaultConnectionPoolConfig())

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("TiDB Ping 失败: %w", err)
	}

	// 初始化用户表
	if err := initUserTable(db, log); err != nil {
		log.Warnf("初始化用户表失败: %v", err)
	}

	// 初始化用户偏好设置表
	if err := initUserPreferencesTable(db, log); err != nil {
		log.Warnf("初始化用户偏好设置表失败: %v", err)
	}

	// 初始化业务和服务表
	if err := initBusinessTable(db, log); err != nil {
		log.Warnf("初始化业务表失败: %v", err)
	}
	if err := initServiceTable(db, log); err != nil {
		log.Warnf("初始化服务表失败: %v", err)
	}
	if err := initCollectorTable(db, log); err != nil {
		log.Warnf("初始化采集器表失败: %v", err)
	}
	if err := initDataNodeTable(db, log); err != nil {
		log.Warnf("初始化数据节点表失败: %v", err)
	}

	// 初始化优化后的表结构（分区 + 索引）
	if err := InitOptimizedTables(db, log); err != nil {
		log.Warnf("初始化优化表结构失败: %v", err)
	}

	// 创建批量写入引擎
	batchEngine := NewBatchWriteEngine(db, DefaultBatchWriterConfig(), log)
	batchEngine.Start()

	log.Infof("TiDB 存储引擎已连接: %s", maskDSN(dsn))
	return &TiDBStorage{
		db:            db,
		retDays:       retDays,
		logger:        log,
		stopCh:        make(chan struct{}),
		overviewCache: make(map[string]interface{}),
		nodesCache:    []map[string]interface{}{},
		overviewCacheExpiry: time.Now().Add(1 * time.Minute),
		nodesCacheExpiry:    time.Now().Add(1 * time.Minute),
		cacheInvalidationTime: make(map[string]time.Time),
		// H6 修复: 初始化缓存大小限制（默认 10000 条，约 10-50MB）
		maxNodesCacheSize:   10000,
		cacheStats:          CacheStats{LastUpdated: time.Now()},
		batchEngine:          batchEngine,
	}, nil
}

// initUserTable 初始化用户表
func initUserTable(db *sql.DB, log *logger.Logger) error {
	// 创建用户表
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(50) UNIQUE NOT NULL,
		password VARCHAR(255) NOT NULL,
		role VARCHAR(20) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建用户表失败: %w", err)
	}
	
	// 检查是否存在默认管理员用户
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = 'admin'").Scan(&count)
	if err != nil {
		return fmt.Errorf("检查默认用户失败: %w", err)
	}
	
	// 如果不存在，创建默认管理员用户
	if count == 0 {
		// 从环境变量读取默认密码，或生成随机密码
		defaultPassword := os.Getenv("CLOUD_FLOW_ADMIN_PASSWORD")
		if defaultPassword == "" {
			// 生成随机密码
			var err error
			defaultPassword, err = generateRandomPassword(12)
			if err != nil {
				return fmt.Errorf("生成随机密码失败: %w", err)
			}
			log.Warnf("未设置 CLOUD_FLOW_ADMIN_PASSWORD 环境变量，已生成随机默认管理员密码（请在首次登录后立即修改）")
		}
		
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("生成密码哈希失败: %w", err)
		}
		
		_, err = db.Exec(
			"INSERT INTO users (username, password, role) VALUES (?, ?, ?)",
			"admin", string(hashedPassword), "admin",
		)
		if err != nil {
			return fmt.Errorf("创建默认管理员用户失败: %w", err)
		}
		
		log.Info("已创建默认管理员用户: admin/[密码从环境变量或随机生成]")
	}
	
	return nil
}

// initUserPreferencesTable 初始化用户偏好设置表
func initUserPreferencesTable(db *sql.DB, log *logger.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS user_preferences (
		username VARCHAR(50) PRIMARY KEY,
		theme VARCHAR(20) DEFAULT 'light',
		language VARCHAR(10) DEFAULT 'zh-CN',
		page_size INT DEFAULT 20,
		refresh_interval INT DEFAULT 30,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建用户偏好设置表失败: %w", err)
	}
	return nil
}

// SaveUserPreferences 保存用户偏好设置
func (s *TiDBStorage) SaveUserPreferences(username string, prefs map[string]interface{}) error {
	theme, _ := prefs["theme"].(string)
	language, _ := prefs["language"].(string)
	pageSize, _ := prefs["page_size"].(int)
	refreshInterval, _ := prefs["refresh_interval"].(int)

	if theme == "" {
		theme = "light"
	}
	if language == "" {
		language = "zh-CN"
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if refreshInterval <= 0 {
		refreshInterval = 30
	}

	_, err := s.db.Exec(
		`INSERT INTO user_preferences (username, theme, language, page_size, refresh_interval)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE theme = VALUES(theme), language = VALUES(language), page_size = VALUES(page_size), refresh_interval = VALUES(refresh_interval)`,
		username, theme, language, pageSize, refreshInterval,
	)
	if err != nil {
		return fmt.Errorf("保存用户偏好设置失败: %w", err)
	}
	return nil
}

// GetUserPreferences 获取用户偏好设置
func (s *TiDBStorage) GetUserPreferences(username string) (map[string]interface{}, error) {
	row := s.db.QueryRow(
		"SELECT theme, language, page_size, refresh_interval FROM user_preferences WHERE username = ?",
		username,
	)
	var theme, language string
	var pageSize, refreshInterval int
	err := row.Scan(&theme, &language, &pageSize, &refreshInterval)
	if err != nil {
		if err == sql.ErrNoRows {
			// 返回默认值
			return map[string]interface{}{
				"theme":            "light",
				"language":         "zh-CN",
				"page_size":        20,
				"refresh_interval": 30,
			}, nil
		}
		return nil, fmt.Errorf("获取用户偏好设置失败: %w", err)
	}
	return map[string]interface{}{
		"theme":            theme,
		"language":         language,
		"page_size":        pageSize,
		"refresh_interval": refreshInterval,
	}, nil
}

// ==================== 业务管理 ====================

func initBusinessTable(db *sql.DB, log *logger.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS businesses (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		status VARCHAR(20) DEFAULT 'active',
		owner VARCHAR(50),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建业务表失败: %w", err)
	}
	return nil
}

func (s *TiDBStorage) ListBusiness(page, pageSize int) ([]map[string]interface{}, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM businesses").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("查询业务总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query("SELECT id, name, description, status, owner, created_at, updated_at FROM businesses ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("查询业务列表失败: %w", err)
	}
	defer rows.Close()
	var items []map[string]interface{}
	for rows.Next() {
		var id, name, status string
		var description, owner sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &description, &status, &owner, &createdAt, &updatedAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "name": name, "description": description.String,
			"status": status, "owner": owner.String,
			"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return items, total, nil
}

func (s *TiDBStorage) CreateBusiness(data map[string]interface{}) error {
	id, _ := data["id"].(string)
	name, _ := data["name"].(string)
	desc, _ := data["description"].(string)
	owner, _ := data["owner"].(string)
	if id == "" || name == "" {
		return fmt.Errorf("业务 ID 和名称不能为空")
	}
	_, err := s.db.Exec("INSERT INTO businesses (id, name, description, owner) VALUES (?, ?, ?, ?)", id, name, desc, owner)
	return err
}

func (s *TiDBStorage) GetBusiness(id string) (map[string]interface{}, error) {
	row := s.db.QueryRow("SELECT id, name, description, status, owner, created_at, updated_at FROM businesses WHERE id = ?", id)
	var name, status string
	var description, owner sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &name, &description, &status, &owner, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return map[string]interface{}{
		"id": id, "name": name, "description": description.String,
		"status": status, "owner": owner.String,
		"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
	}, nil
}

func (s *TiDBStorage) UpdateBusiness(id string, data map[string]interface{}) error {
	name, _ := data["name"].(string)
	desc, _ := data["description"].(string)
	status, _ := data["status"].(string)
	_, err := s.db.Exec("UPDATE businesses SET name=COALESCE(NULLIF(?,''),name), description=COALESCE(NULLIF(?,''),description), status=COALESCE(NULLIF(?,''),status) WHERE id=?",
		name, desc, status, id)
	return err
}

func (s *TiDBStorage) DeleteBusiness(id string) error {
	result, err := s.db.Exec("DELETE FROM businesses WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("业务不存在: %s", id)
	}
	return nil
}

// ==================== 服务管理 ====================

func initServiceTable(db *sql.DB, log *logger.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS services (
		id VARCHAR(64) PRIMARY KEY,
		business_id VARCHAR(64),
		name VARCHAR(255) NOT NULL,
		description TEXT,
		status VARCHAR(20) DEFAULT 'running',
		endpoints JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_business_id (business_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建服务表失败: %w", err)
	}
	return nil
}

func (s *TiDBStorage) ListService(page, pageSize int) ([]map[string]interface{}, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM services").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("查询服务总数失败: %w", err)
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query("SELECT id, business_id, name, description, status, endpoints, created_at, updated_at FROM services ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("查询服务列表失败: %w", err)
	}
	defer rows.Close()
	var items []map[string]interface{}
	for rows.Next() {
		var id, name, status string
		var businessID, description sql.NullString
		var endpoints sql.NullString
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &businessID, &name, &description, &status, &endpoints, &createdAt, &updatedAt); err != nil {
			continue
		}
		var eps interface{} = []interface{}{}
		if endpoints.Valid && endpoints.String != "" {
			safety.CheckAndWarn(s.logger, json.Unmarshal([]byte(endpoints.String), &eps), "解析 endpoints JSON 失败")
		}
		items = append(items, map[string]interface{}{
			"id": id, "business_id": businessID.String, "name": name,
			"description": description.String, "status": status, "endpoints": eps,
			"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return items, total, nil
}

func (s *TiDBStorage) CreateService(data map[string]interface{}) error {
	id, _ := data["id"].(string)
	name, _ := data["name"].(string)
	bizID, _ := data["business_id"].(string)
	desc, _ := data["description"].(string)
	endpoints, err := json.Marshal(data["endpoints"])
	if err != nil {
		s.logger.Warnf("json marshal endpoints failed: %v", err)
		endpoints = []byte("null")
	}
	if id == "" || name == "" {
		return fmt.Errorf("服务 ID 和名称不能为空")
	}
	_, err = s.db.Exec("INSERT INTO services (id, business_id, name, description, endpoints) VALUES (?, ?, ?, ?, ?)",
		id, bizID, name, desc, string(endpoints))
	return err
}

func (s *TiDBStorage) GetService(id string) (map[string]interface{}, error) {
	row := s.db.QueryRow("SELECT id, business_id, name, description, status, endpoints, created_at, updated_at FROM services WHERE id = ?", id)
	var name, status string
	var businessID, description sql.NullString
	var endpoints sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &businessID, &name, &description, &status, &endpoints, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var eps interface{} = []interface{}{}
	if endpoints.Valid && endpoints.String != "" {
		safety.CheckAndWarn(s.logger, json.Unmarshal([]byte(endpoints.String), &eps), "解析 endpoints JSON 失败")
	}
	return map[string]interface{}{
		"id": id, "business_id": businessID.String, "name": name,
		"description": description.String, "status": status, "endpoints": eps,
		"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
	}, nil
}

func (s *TiDBStorage) UpdateService(id string, data map[string]interface{}) error {
	name, _ := data["name"].(string)
	bizID, _ := data["business_id"].(string)
	desc, _ := data["description"].(string)
	status, _ := data["status"].(string)
	endpoints, err := json.Marshal(data["endpoints"])
	if err != nil {
		s.logger.Warnf("json marshal endpoints failed: %v", err)
		endpoints = []byte("null")
	}
	_, err = s.db.Exec(
		"UPDATE services SET name=COALESCE(NULLIF(?,''),name), business_id=COALESCE(NULLIF(?,''),business_id), description=COALESCE(NULLIF(?,''),description), status=COALESCE(NULLIF(?,''),status), endpoints=COALESCE(NULLIF(?,'null'),endpoints) WHERE id=?",
		name, bizID, desc, status, string(endpoints), id)
	return err
}

func (s *TiDBStorage) DeleteService(id string) error {
	result, err := s.db.Exec("DELETE FROM services WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("服务不存在: %s", id)
	}
	return nil
}

// ==================== 采集器管理 ====================

func initCollectorTable(db *sql.DB, log *logger.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS collectors (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		host VARCHAR(255),
		port INT,
		status VARCHAR(20) DEFAULT 'running',
		config JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建采集器表失败: %w", err)
	}
	return nil
}

func (s *TiDBStorage) ListCollector(page, pageSize int) ([]map[string]interface{}, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM collectors").Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query("SELECT id, name, host, port, status, created_at, updated_at FROM collectors ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []map[string]interface{}
	for rows.Next() {
		var id, name, status string
		var host sql.NullString
		var port sql.NullInt64
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &host, &port, &status, &createdAt, &updatedAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "name": name, "host": host.String, "port": port.Int64,
			"status": status, "created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return items, total, nil
}

func (s *TiDBStorage) CreateCollector(data map[string]interface{}) error {
	id, _ := data["id"].(string)
	name, _ := data["name"].(string)
	host, _ := data["host"].(string)
	port, _ := data["port"].(float64)
	if id == "" || name == "" {
		return fmt.Errorf("采集器 ID 和名称不能为空")
	}
	_, err := s.db.Exec("INSERT INTO collectors (id, name, host, port) VALUES (?, ?, ?, ?)", id, name, host, int(port))
	return err
}

func (s *TiDBStorage) UpdateCollector(id string, data map[string]interface{}) error {
	name, _ := data["name"].(string)
	host, _ := data["host"].(string)
	port, _ := data["port"].(float64)
	status, _ := data["status"].(string)
	config, err := json.Marshal(data["config"])
	if err != nil {
		s.logger.Warnf("json marshal config failed: %v", err)
		config = []byte("null")
	}
	_, err = s.db.Exec(
		"UPDATE collectors SET name=COALESCE(NULLIF(?,''),name), host=COALESCE(NULLIF(?,''),host), port=COALESCE(NULLIF(?,0),port), status=COALESCE(NULLIF(?,''),status), config=COALESCE(NULLIF(?,'null'),config) WHERE id=?",
		name, host, int(port), status, string(config), id)
	return err
}

// GetCollector 获取采集器详情
func (s *TiDBStorage) GetCollector(id string) (map[string]interface{}, error) {
	row := s.db.QueryRow("SELECT id, name, host, port, status, config, created_at, updated_at FROM collectors WHERE id = ?", id)
	var name, status string
	var host sql.NullString
	var port sql.NullInt64
	var config sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &name, &host, &port, &status, &config, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var cfg interface{} = map[string]interface{}{}
	if config.Valid && config.String != "" {
		safety.CheckAndWarn(s.logger, json.Unmarshal([]byte(config.String), &cfg), "解析 config JSON 失败")
	}
	return map[string]interface{}{
		"id": id, "name": name, "host": host.String, "port": port.Int64,
		"status": status, "config": cfg,
		"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
	}, nil
}

func (s *TiDBStorage) DeleteCollector(id string) error {
	result, err := s.db.Exec("DELETE FROM collectors WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("采集器不存在: %s", id)
	}
	return nil
}

// ==================== 数据节点管理 ====================

func initDataNodeTable(db *sql.DB, log *logger.Logger) error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS data_nodes (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		host VARCHAR(255),
		port INT,
		type VARCHAR(50) DEFAULT 'tidb',
		status VARCHAR(20) DEFAULT 'online',
		config JSON,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("创建数据节点表失败: %w", err)
	}
	return nil
}

func (s *TiDBStorage) ListDataNode(page, pageSize int) ([]map[string]interface{}, int, error) {
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM data_nodes").Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := s.db.Query("SELECT id, name, host, port, type, status, created_at, updated_at FROM data_nodes ORDER BY created_at DESC LIMIT ? OFFSET ?", pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []map[string]interface{}
	for rows.Next() {
		var id, name, nodeType, status string
		var host sql.NullString
		var port sql.NullInt64
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &host, &port, &nodeType, &status, &createdAt, &updatedAt); err != nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"id": id, "name": name, "host": host.String, "port": port.Int64,
			"type": nodeType, "status": status,
			"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
		})
	}
	if items == nil {
		items = []map[string]interface{}{}
	}
	return items, total, nil
}

func (s *TiDBStorage) CreateDataNode(data map[string]interface{}) error {
	id, _ := data["id"].(string)
	name, _ := data["name"].(string)
	host, _ := data["host"].(string)
	port, _ := data["port"].(float64)
	nodeType, _ := data["type"].(string)
	if id == "" || name == "" {
		return fmt.Errorf("数据节点 ID 和名称不能为空")
	}
	if nodeType == "" {
		nodeType = "tidb"
	}
	_, err := s.db.Exec("INSERT INTO data_nodes (id, name, host, port, type) VALUES (?, ?, ?, ?, ?)", id, name, host, int(port), nodeType)
	return err
}

func (s *TiDBStorage) UpdateDataNode(id string, data map[string]interface{}) error {
	name, _ := data["name"].(string)
	host, _ := data["host"].(string)
	port, _ := data["port"].(float64)
	nodeType, _ := data["type"].(string)
	status, _ := data["status"].(string)
	config, err := json.Marshal(data["config"])
	if err != nil {
		s.logger.Warnf("json marshal config failed: %v", err)
		config = []byte("null")
	}
	_, err = s.db.Exec(
		"UPDATE data_nodes SET name=COALESCE(NULLIF(?,''),name), host=COALESCE(NULLIF(?,''),host), port=COALESCE(NULLIF(?,0),port), type=COALESCE(NULLIF(?,''),type), status=COALESCE(NULLIF(?,''),status), config=COALESCE(NULLIF(?,'null'),config) WHERE id=?",
		name, host, int(port), nodeType, status, string(config), id)
	return err
}

// GetDataNode 获取数据节点详情
func (s *TiDBStorage) GetDataNode(id string) (map[string]interface{}, error) {
	row := s.db.QueryRow("SELECT id, name, host, port, type, status, config, created_at, updated_at FROM data_nodes WHERE id = ?", id)
	var name, nodeType, status string
	var host sql.NullString
	var port sql.NullInt64
	var config sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(&id, &name, &host, &port, &nodeType, &status, &config, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var cfg interface{} = map[string]interface{}{}
	if config.Valid && config.String != "" {
		safety.CheckAndWarn(s.logger, json.Unmarshal([]byte(config.String), &cfg), "解析 config JSON 失败")
	}
	return map[string]interface{}{
		"id": id, "name": name, "host": host.String, "port": port.Int64,
		"type": nodeType, "status": status, "config": cfg,
		"created_at": createdAt.Format(time.RFC3339), "updated_at": updatedAt.Format(time.RFC3339),
	}, nil
}

func (s *TiDBStorage) DeleteDataNode(id string) error {
	result, err := s.db.Exec("DELETE FROM data_nodes WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("数据节点不存在: %s", id)
	}
	return nil
}

// maskDSN 遮蔽DSN中的敏感信息
func maskDSN(dsn string) string {
	parts := strings.Split(dsn, "@")
	if len(parts) > 1 {
		return "***@" + parts[1]
	}
	return "***"
}

// generateUUID 使用 crypto/rand 生成 UUID v4
func generateUUID() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("crypto/rand 不可用，无法生成 UUID: %w", err)
	}
	// 设置版本 4 和变体 10
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// generateRandomPassword 生成指定长度的随机密码
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"
	password := make([]byte, length)
	charsetMax := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		idx, err := rand.Int(rand.Reader, charsetMax)
		if err != nil {
			return "", fmt.Errorf("生成随机密码失败: crypto/rand 不可用: %w", err)
		}
		password[i] = charset[idx.Int64()]
	}
	return string(password), nil
}

// SaveMetrics 保存指标数据
func (s *TiDBStorage) SaveMetrics(probeID string, metrics interface{}) error {
	if metrics == nil {
		return nil
	}

	// 优先使用批量写入引擎（高性能路径）
	if s.batchEngine != nil {
		switch m := metrics.(type) {
		case []*edge.MetricData:
			return s.batchEngine.EnqueueMetrics(probeID, m)
		}
	}

	// 回退到逐条写入（兼容路径）
	switch m := metrics.(type) {
	case []*edge.MetricData:
		return s.saveMetricDataSlice(probeID, m)
	case []interface{}:
		return s.saveInterfaceSlice(probeID, m)
	default:
		return fmt.Errorf("不支持的metrics类型: %T", metrics)
	}
}

// saveMetricDataSlice 保存强类型的MetricData切片
func (s *TiDBStorage) saveMetricDataSlice(probeID string, metrics []*edge.MetricData) error {
	if len(metrics) == 0 {
		return nil
	}

	const batchSize = 200
	for i := 0; i < len(metrics); i += batchSize {
		end := i + batchSize
		if end > len(metrics) {
			end = len(metrics)
		}
		batch := metrics[i:end]

		if err := s.insertMetricsBatch(probeID, batch); err != nil {
			return err
		}
	}
	return nil
}

// saveInterfaceSlice 保存interface{}类型的切片（兼容旧接口）
func (s *TiDBStorage) saveInterfaceSlice(probeID string, metrics []interface{}) error {
	if len(metrics) == 0 {
		return nil
	}

	dataSlice := make([]*edge.MetricData, 0, len(metrics))
	for _, m := range metrics {
		if md, ok := m.(*edge.MetricData); ok {
			dataSlice = append(dataSlice, md)
		}
	}

	return s.saveMetricDataSlice(probeID, dataSlice)
}

// parseTagFloat 从 tags map 中解析 float64 值，解析失败返回 0
func parseTagFloat(tags map[string]string, key string) float64 {
	if tags == nil {
		return 0
	}
	val, ok := tags[key]
	if !ok || val == "" {
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return f
}

// insertMetricsBatch 批量插入指标数据
func (s *TiDBStorage) insertMetricsBatch(probeID string, batch []*edge.MetricData) error {
	if len(batch) == 0 {
		return nil
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			s.logger.Errorf("insertMetricsBatch panic: %v", p)
			tx.Rollback()
		} else if err != nil {
			tx.Rollback()
		}
	}()

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*13)

	for _, m := range batch {
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		ts := time.Unix(m.Timestamp, 0)
		// 使用 MetricData 中的 probe_id，如果为空则使用参数中的 probeID
		mdProbeID := m.ProbeId
		if mdProbeID == "" {
			mdProbeID = probeID
		}
		// 从 Tags 中提取系统指标（cpu_usage, memory_usage, disk_usage），
		// 确保与 QueryMetrics/GetRecentMetrics 查询的列一致
		cpuUsage := parseTagFloat(m.Tags, "cpu_usage")
		memoryUsage := parseTagFloat(m.Tags, "memory_usage")
		diskUsage := parseTagFloat(m.Tags, "disk_usage")
		valueArgs = append(valueArgs,
			mdProbeID, ts, m.SrcIp, m.DstIp,
			m.SrcPort, m.DstPort, m.Protocol,
			m.Bytes, m.Packets, m.Latency,
			cpuUsage, memoryUsage, diskUsage,
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO metrics (probe_id, ts, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency, cpu_usage, memory_usage, disk_usage)
		VALUES %s`, strings.Join(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		return fmt.Errorf("批量写入 metrics 失败: %w", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.invalidateCacheKey("overview")
	return nil
}

// SaveTraces 保存链路追踪数据
func (s *TiDBStorage) SaveTraces(probeID string, spans interface{}) error {
	if spans == nil {
		return nil
	}

	// 优先使用批量写入引擎（高性能路径）
	if s.batchEngine != nil {
		switch m := spans.(type) {
		case []*edge.TraceSpanData:
			return s.batchEngine.EnqueueTraces(probeID, m)
		}
	}

	// 回退到逐条写入（兼容路径）
	switch m := spans.(type) {
	case []*edge.TraceSpanData:
		return s.saveTraceSpanDataSlice(probeID, m)
	case []interface{}:
		return s.saveTracesBatch(probeID, m)
	default:
		return fmt.Errorf("不支持的spans类型: %T", spans)
	}
}

// saveTraceSpanDataSlice 保存强类型的TraceSpanData切片
func (s *TiDBStorage) saveTraceSpanDataSlice(probeID string, spans []*edge.TraceSpanData) error {
	if len(spans) == 0 {
		return nil
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			s.logger.Errorf("saveTraceSpanDataSlice panic: %v", p)
			tx.Rollback()
		} else if err != nil {
			tx.Rollback()
		}
	}()

	valueStrings := make([]string, 0, len(spans))
	valueArgs := make([]interface{}, 0, len(spans)*4)

	for _, span := range spans {
		valueStrings = append(valueStrings, "(?, ?, ?, ?)")
		ts := span.StartTime
		if ts == 0 {
			ts = time.Now().Unix()
		}
		// 使用 TraceSpanData 中的 probe_id，如果为空则使用参数中的 probeID
		spanProbeID := span.ProbeId
		if spanProbeID == "" {
			spanProbeID = probeID
		}
		spanMap := map[string]interface{}{
			"trace_id":   span.TraceId,
			"span_id":    span.SpanId,
			"parent_id":  span.ParentId,
			"service":    span.Service,
			"operation":  span.Operation,
			"start_time": span.StartTime,
			"end_time":   span.EndTime,
			"duration":   span.Duration,
			"status":     span.Status,
			"tags":       span.Tags,
		}
		payload, err := json.Marshal(spanMap)
		if err != nil {
			s.logger.Warnf("json marshal span failed: %v", err)
			continue
		}
		valueArgs = append(valueArgs, spanProbeID, ts, string(payload), span.SpanId)
	}

	query := fmt.Sprintf(
		`INSERT INTO traces (probe_id, ts, payload, span_id) 
		VALUES %s`, strings.Join(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		return fmt.Errorf("批量写入 traces 失败: %w", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.invalidateCacheKey("overview")
	return nil
}

// saveTracesBatch 批量保存链路追踪数据
func (s *TiDBStorage) saveTracesBatch(probeID string, spans []interface{}) error {
	if len(spans) == 0 {
		return nil
	}

	const batchSize = 200
	for i := 0; i < len(spans); i += batchSize {
		end := i + batchSize
		if end > len(spans) {
			end = len(spans)
		}
		batch := spans[i:end]

		if err := s.insertTracesBatch(probeID, batch); err != nil {
			return err
		}
	}
	return nil
}

// insertTracesBatch 批量插入链路追踪数据
func (s *TiDBStorage) insertTracesBatch(probeID string, batch []interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			s.logger.Errorf("insertTracesBatch panic: %v", p)
			tx.Rollback()
		} else if err != nil {
			tx.Rollback()
		}
	}()

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*4)

	for _, span := range batch {
		// 假设 span 是 map[string]interface{} 类型
		if spanMap, ok := span.(map[string]interface{}); ok {
			valueStrings = append(valueStrings, "(?, ?, ?, ?)")
			ts := time.Now().Unix()
			if timestamp, ok := spanMap["timestamp"].(int64); ok {
				ts = timestamp
			}
			payload, err := json.Marshal(spanMap)
			if err != nil {
				s.logger.Warnf("json marshal span failed: %v", err)
				continue
			}
			// 检查 span_id 是否为空，为空时生成默认 UUID
			spanID := ""
			if sid, ok := spanMap["span_id"].(string); ok && sid != "" {
				spanID = sid
			}
			if spanID == "" {
				generatedID, genErr := generateUUID()
				if genErr != nil {
					return fmt.Errorf("生成 span ID 失败: %w", genErr)
				}
				spanID = generatedID
			}
			valueArgs = append(valueArgs, probeID, ts, string(payload), spanID)
		}
	}

	if len(valueStrings) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO traces (probe_id, ts, payload, span_id)
		VALUES %s`, strings.Join(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		return fmt.Errorf("批量写入 traces 失败: %w", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.invalidateCacheKey("overview")
	return nil
}

// SaveProfiling 保存性能分析数据
func (s *TiDBStorage) SaveProfiling(probeID string, profiles interface{}) error {
	if profiles == nil {
		return nil
	}

	// 优先使用批量写入引擎（高性能路径）
	if s.batchEngine != nil {
		switch m := profiles.(type) {
		case []*edge.ProfilingData:
			return s.batchEngine.EnqueueProfiling(probeID, m)
		}
	}

	// 回退到逐条写入（兼容路径）
	switch m := profiles.(type) {
	case []*edge.ProfilingData:
		return s.saveProfilingDataSlice(probeID, m)
	case []interface{}:
		return s.saveProfilingBatch(probeID, m)
	default:
		return fmt.Errorf("不支持的profiles类型: %T", profiles)
	}
}

// saveProfilingDataSlice 保存强类型的ProfilingData切片
func (s *TiDBStorage) saveProfilingDataSlice(probeID string, profiles []*edge.ProfilingData) error {
	if len(profiles) == 0 {
		return nil
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			s.logger.Errorf("saveProfilingDataSlice panic: %v", p)
			tx.Rollback()
		} else if err != nil {
			tx.Rollback()
		}
	}()

	valueStrings := make([]string, 0, len(profiles))
	valueArgs := make([]interface{}, 0, len(profiles)*4)

	for _, profile := range profiles {
		valueStrings = append(valueStrings, "(?, ?, ?, ?)")
		ts := time.Now().Unix()
		// 使用 ProfilingData 中的 probe_id，如果为空则使用参数中的 probeID
		profileProbeID := profile.ProbeId
		if profileProbeID == "" {
			profileProbeID = probeID
		}
		profileMap := map[string]interface{}{
			"type":       profile.Type,
			"stack":      profile.Stack,
			"count":      profile.Count,
			"total_time": profile.TotalTime,
			"labels":     profile.Labels,
		}
		payload, err := json.Marshal(profileMap)
		if err != nil {
			s.logger.Warnf("json marshal profile failed: %v", err)
			continue
		}
		valueArgs = append(valueArgs, profileProbeID, ts, string(payload), profile.Type)
	}

	query := fmt.Sprintf(
		`INSERT INTO profiling (probe_id, ts, payload, type) 
		VALUES %s`, strings.Join(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		return fmt.Errorf("批量写入 profiling 失败: %w", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.invalidateCacheKey("overview")
	return nil
}

// saveProfilingBatch 批量保存性能分析数据
func (s *TiDBStorage) saveProfilingBatch(probeID string, profiles []interface{}) error {
	if len(profiles) == 0 {
		return nil
	}

	const batchSize = 200
	for i := 0; i < len(profiles); i += batchSize {
		end := i + batchSize
		if end > len(profiles) {
			end = len(profiles)
		}
		batch := profiles[i:end]

		if err := s.insertProfilingBatch(probeID, batch); err != nil {
			return err
		}
	}
	return nil
}

// insertProfilingBatch 批量插入性能分析数据
func (s *TiDBStorage) insertProfilingBatch(probeID string, batch []interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			s.logger.Errorf("insertProfilingBatch panic: %v", p)
			tx.Rollback()
		} else if err != nil {
			tx.Rollback()
		}
	}()

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*4)

	for _, profile := range batch {
		// 假设 profile 是 map[string]interface{} 类型
		if profileMap, ok := profile.(map[string]interface{}); ok {
			valueStrings = append(valueStrings, "(?, ?, ?, ?)")
			ts := time.Now().Unix()
			if timestamp, ok := profileMap["timestamp"].(int64); ok {
				ts = timestamp
			}
			payload, err := json.Marshal(profileMap)
			if err != nil {
				s.logger.Warnf("json marshal profile failed: %v", err)
				continue
			}
			profileType := "cpu"
			if pType, ok := profileMap["type"].(string); ok {
				profileType = pType
			}
			valueArgs = append(valueArgs, probeID, ts, string(payload), profileType)
		}
	}

	if len(valueStrings) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`INSERT INTO profiling (probe_id, ts, payload, type)
		VALUES %s`, strings.Join(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		return fmt.Errorf("批量写入 profiling 失败: %w", err)
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	s.invalidateCacheKey("overview")
	return nil
}

// SaveProbeInfo 保存探针信息
func (s *TiDBStorage) SaveProbeInfo(edgeNodeID string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO probes (edge_node_id, payload, updated_at) 
		VALUES (?, ?, ?) 
		ON DUPLICATE KEY UPDATE payload = VALUES(payload), updated_at = VALUES(updated_at)`,
		edgeNodeID, string(payload), time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("保存探针信息失败: %w", err)
	}

	// 探针信息变更同时影响 overview（节点数）和 nodes（探针列表）
	s.invalidateCacheKey("overview")
	s.invalidateCacheKey("nodes")
	return nil
}

// QueryMetrics 查询指标数据
func (s *TiDBStorage) QueryMetrics(day string, probeID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	startTime, err := time.Parse("2006-01-02", day)
	if err != nil {
		return nil, fmt.Errorf("日期格式错误: %w", err)
	}
	start := startTime.Unix()
	end := start + 86400

	query := `SELECT id, probe_id, UNIX_TIMESTAMP(ts) as timestamp, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency, cpu_usage, memory_usage, disk_usage 
			FROM metrics 
			WHERE ts >= FROM_UNIXTIME(?) AND ts < FROM_UNIXTIME(?)`
	args := []interface{}{start, end}

	if probeID != "" {
		query += " AND probe_id = ?"
		args = append(args, probeID)
	}

	query += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var scanProbeID string
		var ts int64
		var srcIP, dstIP, protocol string
		var srcPort, dstPort int
		var bytes, packets, latency int64
		var cpuUsage, memoryUsage, diskUsage float64

		if err := rows.Scan(&id, &scanProbeID, &ts, &srcIP, &dstIP, &srcPort, &dstPort, &protocol, &bytes, &packets, &latency, &cpuUsage, &memoryUsage, &diskUsage); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"id":         id,
			"probe_id":   scanProbeID,
			"timestamp":  ts,
			"src_ip":     srcIP,
			"dst_ip":     dstIP,
			"src_port":   srcPort,
			"dst_port":   dstPort,
			"protocol":   protocol,
			"bytes":      bytes,
			"packets":    packets,
			"latency":    latency,
			"cpu_usage":  cpuUsage,
			"memory_usage": memoryUsage,
			"disk_usage": diskUsage,
			"tags":       map[string]string{},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	return results, nil
}

// QueryMetricsByAlert 按告警规则查询指标
func (s *TiDBStorage) QueryMetricsByAlert(day string, metricName string, operator string, threshold float64, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	column, err := storage.MetricNameToColumn(metricName)
	if err != nil {
		return nil, fmt.Errorf("不支持的指标类型: %s: %w", metricName, err)
	}

	// 验证操作符，防止 SQL 注入
	validOperators := map[string]bool{
		">":  true,
		">=": true,
		"<":  true,
		"<=": true,
		"=":  true,
		"!=": true,
	}
	if !validOperators[operator] {
		return nil, fmt.Errorf("无效的操作符: %s", operator)
	}

	// 验证列名，防止 SQL 注入
	validColumns := map[string]bool{
		"bytes": true, "packets": true, "latency": true,
		"cpu_usage": true, "memory_usage": true, "disk_usage": true,
	}
	if !validColumns[column] {
		return nil, fmt.Errorf("无效的列名: %s", column)
	}

	query := fmt.Sprintf(
		`SELECT id, probe_id, UNIX_TIMESTAMP(ts) as timestamp, %s, protocol
		FROM metrics
		WHERE ts >= NOW() - INTERVAL 1 DAY
		AND %s %s ?
		ORDER BY ts DESC LIMIT ?`,
		column, column, operator)

	rows, err := s.db.Query(query, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var probeID string
		var ts int64
		var value float64
		var protocol string

		if err := rows.Scan(&id, &probeID, &ts, &value, &protocol); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"id":        id,
			"probe_id":  probeID,
			"timestamp": ts,
			"value":     value,
			"protocol":  protocol,
			"data":      map[string]interface{}{"value": value},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	return results, nil
}



// QueryTraces 查询链路追踪数据
func (s *TiDBStorage) QueryTraces(day string, probeID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	startTime, err := time.Parse("2006-01-02", day)
	if err != nil {
		return nil, fmt.Errorf("日期格式错误: %w", err)
	}
	start := startTime.Unix()
	end := start + 86400

	query := `SELECT id, probe_id, UNIX_TIMESTAMP(ts) as timestamp, span_id, payload 
			FROM traces 
			WHERE ts >= FROM_UNIXTIME(?) AND ts < FROM_UNIXTIME(?)`
	args := []interface{}{start, end}

	if probeID != "" {
		query += " AND probe_id = ?"
		args = append(args, probeID)
	}

	query += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var scanProbeID, spanID, payloadStr string
		var ts int64

		if err := rows.Scan(&id, &scanProbeID, &ts, &spanID, &payloadStr); err != nil {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			s.logger.Warnf("反序列化 traces payload 失败 (id=%d, span_id=%s): %v", id, spanID, err)
			payload = make(map[string]interface{})
		}

		results = append(results, map[string]interface{}{
			"id":        id,
			"probe_id":  scanProbeID,
			"timestamp": ts,
			"span_id":   spanID,
			"payload":   payload,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	return results, nil
}

// GetOverview 获取系统概览
// M1 修复: 使用 SystemOverview 结构体，确保类型安全
func (s *TiDBStorage) GetOverview() (map[string]interface{}, error) {
	// M1: 使用结构化版本
	overview, err := s.GetOverviewTyped()
	if err != nil {
		return nil, err
	}
	return overview.ToMap(), nil
}

// GetOverviewTyped 获取系统概览（类型安全版本，M1 修复）
func (s *TiDBStorage) GetOverviewTyped() (*SystemOverview, error) {
	s.muCache.RLock()
	cachedAt := s.overviewCacheExpiry.Add(-1 * time.Minute)
	if time.Now().Before(s.overviewCacheExpiry) && len(s.overviewCache) > 0 && !s.isCacheKeyInvalidatedLocked("overview", cachedAt) {
		result := mapToSystemOverview(s.overviewCache)
		s.muCache.RUnlock()
		return result, nil
	}
	s.muCache.RUnlock()

	// M1: 缓存过期，重新查询，使用结构体
	var metricsToday, tracesToday, profsToday int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM metrics WHERE ts >= DATE(NOW())").Scan(&metricsToday); err != nil {
		s.logger.Warnf("查询今日指标数失败: %v", err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM traces WHERE ts >= DATE(NOW())").Scan(&tracesToday); err != nil {
		s.logger.Warnf("查询今日追踪数失败: %v", err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM profiling WHERE ts >= DATE(NOW())").Scan(&profsToday); err != nil {
		s.logger.Warnf("查询今日性能分析数失败: %v", err)
	}

	var nodeCount int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM probes").Scan(&nodeCount); err != nil {
		s.logger.Warnf("查询节点数失败: %v", err)
	}

	var dayCount int
	if err := s.db.QueryRow("SELECT COUNT(DISTINCT DATE(ts)) FROM metrics").Scan(&dayCount); err != nil {
		s.logger.Warnf("查询天数失败: %v", err)
	}

	// M1: 构建结构体（类型安全）
	result := &SystemOverview{
		TotalNodes:   nodeCount,
		OnlineNodes:  0, // 需要从 probes 表获取在线状态
		OfflineNodes: 0,
		Storage:      "tidb",
		Nodes:        nodeCount,
		Days:         dayCount,
		TodayMetrics: metricsToday,
		TodayTraces:  tracesToday,
		TodayProfs:   profsToday,
	}

	s.muCache.Lock()
	s.overviewCache = result.ToMap()
	s.overviewCacheExpiry = time.Now().Add(1 * time.Minute)
	delete(s.cacheInvalidationTime, "overview")
	s.muCache.Unlock()

	return result, nil
}

// mapToSystemOverview 将 map 转换为 SystemOverview（M1 修复）
func mapToSystemOverview(m map[string]interface{}) *SystemOverview {
	overview := &SystemOverview{Storage: "tidb"}

	if v, ok := m["total_nodes"].(int); ok {
		overview.TotalNodes = v
	}
	if v, ok := m["total_nodes"].(int64); ok {
		overview.TotalNodes = int(v)
	}
	if v, ok := m["online_nodes"].(int); ok {
		overview.OnlineNodes = v
	}
	if v, ok := m["online_nodes"].(int64); ok {
		overview.OnlineNodes = int(v)
	}
	if v, ok := m["offline_nodes"].(int); ok {
		overview.OfflineNodes = v
	}
	if v, ok := m["offline_nodes"].(int64); ok {
		overview.OfflineNodes = int(v)
	}
	if v, ok := m["total_services"].(int); ok {
		overview.TotalServices = v
	}
	if v, ok := m["nodes"].(int); ok {
		overview.Nodes = v
	}
	if v, ok := m["nodes"].(int64); ok {
		overview.Nodes = int(v)
	}
	if v, ok := m["days"].(int); ok {
		overview.Days = v
	}
	if v, ok := m["days"].(int64); ok {
		overview.Days = int(v)
	}
	if v, ok := m["today_metrics"].(int); ok {
		overview.TodayMetrics = v
	}
	if v, ok := m["today_metrics"].(int64); ok {
		overview.TodayMetrics = int(v)
	}
	if v, ok := m["today_traces"].(int); ok {
		overview.TodayTraces = v
	}
	if v, ok := m["today_traces"].(int64); ok {
		overview.TodayTraces = int(v)
	}
	if v, ok := m["today_profs"].(int); ok {
		overview.TodayProfs = v
	}
	if v, ok := m["today_profs"].(int64); ok {
		overview.TodayProfs = int(v)
	}

	return overview
}

// GetNodes 获取探针列表
// H6 修复: 添加缓存大小限制和统计
// M1 修复: 内部使用 ProbeNode 结构体，确保类型安全
func (s *TiDBStorage) GetNodes() ([]map[string]interface{}, error) {
	// M1: 使用结构化版本获取数据
	nodes, err := s.GetNodesTyped()
	if err != nil {
		return nil, err
	}
	// M1: 转换为 map 以保持向后兼容
	result := make([]map[string]interface{}, len(nodes))
	for i, node := range nodes {
		result[i] = node.ToMap()
	}
	return result, nil
}

// GetNodesTyped 获取探针列表（类型安全版本，M1 修复）
func (s *TiDBStorage) GetNodesTyped() ([]ProbeNode, error) {
	s.muCache.RLock()
	// 检查缓存是否过期（TTL）或被按 key 粒度标记失效
	cachedAt := s.nodesCacheExpiry.Add(-1 * time.Minute)
	if time.Now().Before(s.nodesCacheExpiry) && len(s.nodesCache) > 0 && !s.isCacheKeyInvalidatedLocked("nodes", cachedAt) {
		// M1: 将缓存的 map 转换为结构体
		result := make([]ProbeNode, len(s.nodesCache))
		for i, node := range s.nodesCache {
			result[i] = mapToProbeNode(node)
		}
		s.cacheStats.CacheHits++
		s.muCache.RUnlock()
		return result, nil
	}
	s.cacheStats.CacheMisses++
	s.muCache.RUnlock()

	rows, err := s.db.Query("SELECT edge_node_id, payload, updated_at FROM probes ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// M1: 使用结构体替代 map
	var nodes []ProbeNode
	var totalBytes int64
	for rows.Next() {
		var nodeID, payloadStr string
		var updatedAt int64
		if err := rows.Scan(&nodeID, &payloadStr, &updatedAt); err != nil {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			s.logger.Warnf("反序列化 probes payload 失败 (node_id=%s): %v", nodeID, err)
			payload = make(map[string]interface{})
		}

		// M1: 构建结构体（类型安全）
		node := ProbeNode{
			EdgeNodeID: nodeID,
			UpdatedAt:  time.Unix(updatedAt, 0),
			Payload:    payload,
		}

		// M1: 提取常见字段到独立属性（IDE 自动补全）
		if hostname, ok := payload["hostname"].(string); ok {
			node.Hostname = hostname
		}
		if hostIP, ok := payload["host_ip"].(string); ok {
			node.HostIP = hostIP
		}
		if status, ok := payload["status"].(string); ok {
			node.Status = status
		}
		if version, ok := payload["version"].(string); ok {
			node.Version = version
		}

		nodes = append(nodes, node)
		totalBytes += int64(len(payloadStr))

		// H6: 限制缓存条目数，防止内存无限增长
		if len(nodes) >= s.maxNodesCacheSize {
			s.logger.Warnf("探针数量超过缓存上限 %d，仅缓存前 %d 条", s.maxNodesCacheSize, s.maxNodesCacheSize)
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	// M1: 将结构体转换回 map 用于缓存（保持现有缓存逻辑）
	cacheData := make([]map[string]interface{}, len(nodes))
	for i, node := range nodes {
		cacheData[i] = node.ToMap()
	}

	s.muCache.Lock()
	s.nodesCache = cacheData
	s.nodesCacheExpiry = time.Now().Add(1 * time.Minute)
	s.cacheStats.NodesCacheSize = len(nodes)
	s.cacheStats.NodesCacheBytes = totalBytes
	s.cacheStats.LastUpdated = time.Now()
	delete(s.cacheInvalidationTime, "nodes")
	s.muCache.Unlock()

	if totalBytes > 50*1024*1024 {
		s.logger.Warnf("[H6] nodesCache 内存占用过高: %d MB，建议降低 maxNodesCacheSize 或优化探针 payload", totalBytes/1024/1024)
	}

	return nodes, nil
}

// mapToProbeNode 将 map 转换为 ProbeNode（M1 修复）
func mapToProbeNode(m map[string]interface{}) ProbeNode {
	node := ProbeNode{}

	if edgeNodeID, ok := m["edge_node_id"].(string); ok {
		node.EdgeNodeID = edgeNodeID
	}
	if hostname, ok := m["hostname"].(string); ok {
		node.Hostname = hostname
	}
	if hostIP, ok := m["host_ip"].(string); ok {
		node.HostIP = hostIP
	}
	if status, ok := m["status"].(string); ok {
		node.Status = status
	}
	if version, ok := m["version"].(string); ok {
		node.Version = version
	}
	if tags, ok := m["tags"].([]interface{}); ok {
		node.Tags = make([]string, len(tags))
		for i, t := range tags {
			if s, ok := t.(string); ok {
				node.Tags[i] = s
			}
		}
	}
	if metadata, ok := m["metadata"].(map[string]interface{}); ok {
		node.Metadata = metadata
	}
	if payload, ok := m["payload"].(map[string]interface{}); ok {
		node.Payload = payload
	}
	if updatedAt, ok := m["updated_at"].(int64); ok {
		node.UpdatedAt = time.Unix(updatedAt, 0)
	}

	return node
}

// StartCleanup 启动清理协程
func (s *TiDBStorage) StartCleanup() {
	s.StartPartitionManager()
}

// StartPartitionManager 启动分区管理
func (s *TiDBStorage) StartPartitionManager() {
	go func() {
		// 每天凌晨 2:00 执行
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
			timer := time.NewTimer(next.Sub(now))

			select {
			case <-timer.C:
				s.managePartitions()
			case <-s.stopCh:
				timer.Stop()
				return
			}
		}
	}()
	s.logger.Infof("分区管理协程已启动，保留 %d 天", s.retDays)
}

// managePartitions 管理分区
func (s *TiDBStorage) managePartitions() {
	// 检查 TiDB 版本是否支持分区表
	if err := s.checkTiDBVersion(); err != nil {
		s.logger.Warnf("跳过分区管理: %v", err)
		return
	}

	tables := []string{"metrics", "traces", "profiling", "alert_history"}

	for _, table := range tables {
		// 表名白名单校验，防止 SQL 注入
		allowedTables := map[string]bool{
			"metrics": true, "traces": true, "profiling": true, "alert_history": true,
		}
		if !allowedTables[table] {
			s.logger.Errorf("不允许的表名: %s", table)
			continue
		}
		// 检查表是否存在
		if !s.tableExists(table) {
			// alert_history 表可能不存在，跳过该表的分区管理
			s.logger.Debugf("表 %s 不存在，跳过分区管理", table)
			continue
		}

		// 创建未来 7 天的分区
		for d := 1; d <= 7; d++ {
			future := time.Now().AddDate(0, 0, d)
			partName := fmt.Sprintf("p_%s", future.Format("20060102"))
			nextDay := future.AddDate(0, 0, 1).Format("2006-01-02")

			query := fmt.Sprintf(
				`ALTER TABLE %s ADD PARTITION (PARTITION %s VALUES LESS THAN (UNIX_TIMESTAMP('%s')))`,
				table, partName, nextDay)
			// NOTE: 分区名 partName 由日期格式化生成（格式: p_YYYYMMDD），
			// 不接受外部输入，因此无需额外的分区名校验。
			// 如果未来分区名来自用户输入，必须添加校验防止 SQL 注入。
			if _, err := s.db.Exec(query); err != nil {
				// 分区已存在则忽略
				if !strings.Contains(err.Error(), "Duplicate") {
					s.logger.Warnf("创建分区失败: %s: %v", table, err)
				}
			}
		}

		// 删除过期分区
		expired := time.Now().AddDate(0, 0, -s.retDays).Format("20060102")
		partName := fmt.Sprintf("p_%s", expired)
		query := fmt.Sprintf("ALTER TABLE %s DROP PARTITION IF EXISTS %s", table, partName)
		if _, err := s.db.Exec(query); err != nil {
			s.logger.Warnf("删除过期分区失败: %s: %v", table, err)
		} else {
			s.logger.Infof("已删除过期分区: %s.%s", table, partName)
		}
	}
}

// checkTiDBVersion 检查 TiDB 版本是否支持分区表
func (s *TiDBStorage) checkTiDBVersion() error {
	var version string
	err := s.db.QueryRow("SELECT tidb_version()").Scan(&version)
	if err != nil {
		// 如果 tidb_version() 函数不存在，可能是普通 MySQL
		s.logger.Warn("无法获取 TiDB 版本信息，分区管理功能将被禁用")
		return fmt.Errorf("非 TiDB 环境")
	}
	s.logger.Infof("检测到 TiDB 版本: %s", version)
	return nil
}

// tableExists 检查表是否存在
func (s *TiDBStorage) tableExists(tableName string) bool {
	query := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`
	var count int
	if err := s.db.QueryRow(query, tableName).Scan(&count); err != nil {
		s.logger.Warnf("检查表 %s 是否存在失败: %v", tableName, err)
		return false
	}
	return count > 0
}

// invalidateCache 使所有缓存失效
func (s *TiDBStorage) invalidateCache() {
	s.muCache.Lock()
	s.overviewCache = make(map[string]interface{})
	s.nodesCache = []map[string]interface{}{}
	s.overviewCacheExpiry = time.Now()
	s.nodesCacheExpiry = time.Now()
	s.cacheInvalidationTime["overview"] = time.Now()
	s.cacheInvalidationTime["nodes"] = time.Now()
	s.muCache.Unlock()
}

// invalidateCacheKey 使指定缓存 key 失效（按 key 粒度失效）
func (s *TiDBStorage) invalidateCacheKey(key string) {
	s.muCache.Lock()
	defer s.muCache.Unlock()

	s.cacheInvalidationTime[key] = time.Now()

	switch key {
	case "overview":
		s.overviewCache = make(map[string]interface{})
		s.overviewCacheExpiry = time.Now()
	case "nodes":
		s.nodesCache = []map[string]interface{}{}
		s.nodesCacheExpiry = time.Now()
	}
}

// GetCacheStats 获取缓存统计信息（H6 修复）
func (s *TiDBStorage) GetCacheStats() CacheStats {
	s.muCache.RLock()
	defer s.muCache.RUnlock()

	// 计算 overview cache 大小
	overviewSize := len(s.overviewCache)

	return CacheStats{
		NodesCacheSize:    s.cacheStats.NodesCacheSize,
		NodesCacheBytes:   s.cacheStats.NodesCacheBytes,
		OverviewCacheSize: overviewSize,
		CacheHits:         s.cacheStats.CacheHits,
		CacheMisses:       s.cacheStats.CacheMisses,
		LastUpdated:       s.cacheStats.LastUpdated,
	}
}

// SetMaxNodesCacheSize 设置 nodesCache 最大条目数（H6 修复）
func (s *TiDBStorage) SetMaxNodesCacheSize(size int) {
	if size > 0 {
		s.muCache.Lock()
		s.maxNodesCacheSize = size
		s.muCache.Unlock()
		s.logger.Infof("设置 nodesCache 最大条目数为 %d", size)
	}
}

// isCacheKeyInvalidated 检查指定缓存 key 是否在给定时间之后被标记失效
func (s *TiDBStorage) isCacheKeyInvalidated(key string, cachedAfter time.Time) bool {
	s.muCache.RLock()
	defer s.muCache.RUnlock()

	if invalidatedAt, ok := s.cacheInvalidationTime[key]; ok {
		return invalidatedAt.After(cachedAfter)
	}
	return false
}

// isCacheKeyInvalidatedLocked 检查指定缓存 key 是否在给定时间之后被标记失效（调用方已持有锁）
func (s *TiDBStorage) isCacheKeyInvalidatedLocked(key string, cachedAfter time.Time) bool {
	if invalidatedAt, ok := s.cacheInvalidationTime[key]; ok {
		return invalidatedAt.After(cachedAfter)
	}
	return false
}

// Stop 停止存储引擎
func (s *TiDBStorage) Stop() {
	s.stopped.Do(func() {
		// 停止批量写入引擎
		if s.batchEngine != nil {
			s.batchEngine.Stop()
		}
		close(s.stopCh)
		s.db.Close()
		s.logger.Info("TiDB 存储引擎已关闭")
	})
}

// DB 获取底层数据库连接
func (s *TiDBStorage) DB() interface{} {
	return s.db
}

// GetDB 保留原有的 *sql.DB 版本，供内部使用
func (s *TiDBStorage) GetDB() *sql.DB {
	return s.db
}

// GetRecentMetrics 获取最近的指标数据
// M4 修复: 使用参数化查询避免 SQL 注入风险
func (s *TiDBStorage) GetRecentMetrics(metricType string, limit int, timeWindow time.Duration) ([]*edge.MetricData, error) {
	if limit <= 0 {
		limit = 5
	}
	if timeWindow <= 0 {
		timeWindow = 5 * time.Minute
	}

	// M4 修复: 使用 DATE_SUB 配合参数化查询，避免 fmt.Sprintf 拼接
	// 计算截止时间戳，使用参数化查询
	cutoffTime := time.Now().Add(-timeWindow)
	query := `SELECT probe_id, UNIX_TIMESTAMP(ts) as timestamp, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency, cpu_usage, memory_usage, disk_usage
			FROM metrics
			WHERE ts >= ? `
	args := []interface{}{cutoffTime}

	// 根据 metricType 过滤数据
	switch metricType {
	case "network", "traffic":
		// 网络相关指标：需要有网络字段
		query += " AND src_ip IS NOT NULL AND dst_ip IS NOT NULL"
	case "cpu", "memory", "disk":
		// 系统相关指标：网络字段为 NULL
		query += " AND src_ip IS NULL AND dst_ip IS NULL"
	}

	query += " ORDER BY ts DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()

	var metrics []*edge.MetricData
	for rows.Next() {
		var probeID, srcIP, dstIP, protocol string
		var timestamp int64
		var srcPort, dstPort int
		var bytes, packets, latency int64
		var cpuUsage, memoryUsage, diskUsage float64

		if err := rows.Scan(&probeID, &timestamp, &srcIP, &dstIP, &srcPort, &dstPort, &protocol, &bytes, &packets, &latency, &cpuUsage, &memoryUsage, &diskUsage); err != nil {
			continue
		}

		// 将系统指标值填充到 Tags map，确保 alerting/manager.go 的
		// extractMetricValue 能从 metric.Tags["cpu_usage"] 等读取到值
		tags := make(map[string]string)
		if cpuUsage > 0 {
			tags["cpu_usage"] = strconv.FormatFloat(cpuUsage, 'f', 2, 64)
		}
		if memoryUsage > 0 {
			tags["memory_usage"] = strconv.FormatFloat(memoryUsage, 'f', 2, 64)
		}
		if diskUsage > 0 {
			tags["disk_usage"] = strconv.FormatFloat(diskUsage, 'f', 2, 64)
		}

		metric := &edge.MetricData{
			ProbeId:      probeID,
			Timestamp:    timestamp,
			SrcIp:        srcIP,
			DstIp:        dstIP,
			SrcPort:      int32(srcPort),
			DstPort:      int32(dstPort),
			Protocol:     edge.ProtocolType(protocol),
			Bytes:        bytes,
			Packets:      packets,
			Latency:      latency,
			CpuUsage:     cpuUsage,
			MemoryUsage:  memoryUsage,
			DiskUsage:    diskUsage,
			Tags:         tags,
		}

		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	return metrics, nil
}

// CreateUser 创建用户
func (s *TiDBStorage) CreateUser(username, password, role string) error {
	// 哈希密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	// 插入用户
	_, err = s.db.Exec(
		"INSERT INTO users (username, password, role) VALUES (?, ?, ?)",
		username, string(hashedPassword), role,
	)
	if err != nil {
		return fmt.Errorf("创建用户失败: %w", err)
	}

	return nil
}

// GetUser 获取用户信息
func (s *TiDBStorage) GetUser(username string) (map[string]interface{}, error) {
	row := s.db.QueryRow(
		"SELECT id, username, role, created_at, updated_at FROM users WHERE username = ?",
		username,
	)

	var id int
	var user, role string
	var createdAt, updatedAt time.Time

	err := row.Scan(&id, &user, &role, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	return map[string]interface{}{
		"id":         id,
		"username":   user,
		"role":       role,
		"created_at": createdAt.Unix(),
		"updated_at": updatedAt.Unix(),
	}, nil
}

// UpdateUser 更新用户信息
func (s *TiDBStorage) UpdateUser(username, password, role string) error {
	// 检查用户是否存在
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return fmt.Errorf("检查用户是否存在失败: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("用户不存在: %s", username)
	}

	// 准备更新语句
	var query string
	var args []interface{}

	if password != "" {
		// 哈希新密码
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("生成密码哈希失败: %w", err)
		}

		query = "UPDATE users SET password = ?, role = ? WHERE username = ?"
		args = []interface{}{string(hashedPassword), role, username}
	} else {
		query = "UPDATE users SET role = ? WHERE username = ?"
		args = []interface{}{role, username}
	}

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("更新用户失败: %w", err)
	}

	return nil
}

// UpdateUserRole 仅更新用户角色（不修改密码）
func (s *TiDBStorage) UpdateUserRole(username, role string) error {
	// 检查用户是否存在
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return fmt.Errorf("检查用户是否存在失败: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("用户不存在: %s", username)
	}

	_, err = s.db.Exec("UPDATE users SET role = ? WHERE username = ?", role, username)
	if err != nil {
		return fmt.Errorf("更新用户角色失败: %w", err)
	}

	return nil
}

// DeleteUser 删除用户
func (s *TiDBStorage) DeleteUser(username string) error {
	// 检查用户是否存在
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil {
		return fmt.Errorf("检查用户失败: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("用户不存在")
	}

	// 检查是否是最后一个管理员用户
	var adminCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
	if err != nil {
		return fmt.Errorf("检查管理员用户失败: %w", err)
	}

	// 检查当前用户是否是管理员
	var isAdmin bool
	err = s.db.QueryRow("SELECT role = 'admin' FROM users WHERE username = ?", username).Scan(&isAdmin)
	if err != nil {
		return fmt.Errorf("检查用户角色失败: %w", err)
	}

	// 如果是最后一个管理员用户，不允许删除
	if isAdmin && adminCount <= 1 {
		return fmt.Errorf("不能删除最后一个管理员用户")
	}

	// 执行删除操作
	_, err = s.db.Exec("DELETE FROM users WHERE username = ?", username)
	if err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}

	return nil
}

// ListUsers 列出所有用户
func (s *TiDBStorage) ListUsers() ([]map[string]interface{}, error) {
	rows, err := s.db.Query("SELECT id, username, role, created_at, updated_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("查询用户列表失败: %w", err)
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id int
		var username, role string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &username, &role, &createdAt, &updatedAt); err != nil {
			continue
		}

		users = append(users, map[string]interface{}{
			"id":         id,
			"username":   username,
			"role":       role,
			"created_at": createdAt.Unix(),
			"updated_at": updatedAt.Unix(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("查询结果处理失败: %w", err)
	}

	return users, nil
}

// VerifyUser 验证用户登录
func (s *TiDBStorage) VerifyUser(username, password string) (bool, string, error) {
	row := s.db.QueryRow("SELECT password, role FROM users WHERE username = ?", username)

	var hashedPassword, role string
	err := row.Scan(&hashedPassword, &role)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, "", nil
		}
		return false, "", fmt.Errorf("验证用户失败: %w", err)
	}

	// 验证密码
	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return false, "", nil
	}

	return true, role, nil
}

// ChangePassword 修改用户密码（验证旧密码后更新）
func (s *TiDBStorage) ChangePassword(username, oldPassword, newPassword string) error {
	// 验证旧密码
	row := s.db.QueryRow("SELECT password FROM users WHERE username = ?", username)
	var hashedPassword string
	if err := row.Scan(&hashedPassword); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("用户不存在: %s", username)
		}
		return fmt.Errorf("查询用户失败: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(oldPassword)); err != nil {
		return fmt.Errorf("旧密码不正确")
	}

	// 哈希新密码
	newHashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成密码哈希失败: %w", err)
	}

	result, err := s.db.Exec("UPDATE users SET password = ? WHERE username = ?", string(newHashed), username)
	if err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("用户不存在: %s", username)
	}
	return nil
}
