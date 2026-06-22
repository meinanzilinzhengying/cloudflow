//go:build linux

package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
)

// ============================================================================
// PostgreSQL 元数据存储适配器
// 替代 TiDB 的轻量级方案，适用于元数据量不大的场景
// ============================================================================

// PostgresConfig PostgreSQL 配置
type PostgresConfig struct {
	DSN         string
	Database    string
	MaxOpenConn int           // 最大连接数
	MaxIdleConn int           // 最大空闲连接数
	ConnMaxLife time.Duration // 连接最大生命周期
}

// DefaultPostgresConfig 返回默认 PostgreSQL 配置
func DefaultPostgresConfig() *PostgresConfig {
	return &PostgresConfig{
		Database:    "cloudflow",
		MaxOpenConn: 25,
		MaxIdleConn: 5,
		ConnMaxLife: 5 * time.Minute,
	}
}

// PostgresEngine PostgreSQL 存储引擎
type PostgresEngine struct {
	db     *sql.DB
	config *PostgresConfig
	logger *logger.Logger
	mu     sync.RWMutex
	closed bool
}

// NewPostgresEngine 创建 PostgreSQL 存储引擎
// 依赖外部注入的 *sql.DB，驱动由调用方通过 _ 导入注册（如 github.com/lib/pq）
func NewPostgresEngine(db *sql.DB, cfg *PostgresConfig, log *logger.Logger) (*PostgresEngine, error) {
	if cfg == nil {
		cfg = DefaultPostgresConfig()
	}
	if db == nil {
		return nil, fmt.Errorf("db connection required")
	}

	pe := &PostgresEngine{
		db:     db,
		config: cfg,
		logger: log,
	}

	db.SetMaxOpenConns(cfg.MaxOpenConn)
	db.SetMaxIdleConns(cfg.MaxIdleConn)
	db.SetConnMaxLifetime(cfg.ConnMaxLife)

	if err := pe.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema failed: %w", err)
	}

	return pe, nil
}

// initSchema 初始化元数据表
func (pe *PostgresEngine) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id          SERIAL PRIMARY KEY,
    username    VARCHAR(64) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    role        VARCHAR(32) NOT NULL DEFAULT 'viewer',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS businesses (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    description TEXT,
    status      VARCHAR(32) DEFAULT 'active',
    owner       VARCHAR(64),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS services (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    business_id INTEGER REFERENCES businesses(id) ON DELETE SET NULL,
    description TEXT,
    status      VARCHAR(32) DEFAULT 'active',
    owner       VARCHAR(64),
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS collectors (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    type        VARCHAR(32) NOT NULL,
    config      JSONB DEFAULT '{}',
    status      VARCHAR(32) DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS data_nodes (
    id          SERIAL PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    address     VARCHAR(255) NOT NULL,
    status      VARCHAR(32) DEFAULT 'active',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_businesses_status ON businesses(status);
CREATE INDEX IF NOT EXISTS idx_services_business_id ON services(business_id);
CREATE INDEX IF NOT EXISTS idx_collectors_status ON collectors(status);
`
	_, err := pe.db.Exec(schema)
	return err
}

// ============================================================================
// 用户管理
// ============================================================================

// CreateUser 创建用户
func (pe *PostgresEngine) CreateUser(username, password, role string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `INSERT INTO users (username, password, role) VALUES ($1, $2, $3)`
	_, err := pe.db.Exec(query, username, password, role)
	if err != nil {
		return fmt.Errorf("create user failed: %w", err)
	}
	return nil
}

// GetUser 获取用户
func (pe *PostgresEngine) GetUser(username string) (map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, username, role, created_at, updated_at FROM users WHERE username = $1`
	row := pe.db.QueryRow(query, username)

	var id int
	var uname, role string
	var createdAt, updatedAt time.Time

	err := row.Scan(&id, &uname, &role, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %s", username)
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":         id,
		"username":   uname,
		"role":       role,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}, nil
}

// ListUsers 列出用户
func (pe *PostgresEngine) ListUsers() ([]map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, username, role, created_at FROM users ORDER BY id`
	rows, err := pe.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var username, role string
		var createdAt time.Time
		if err := rows.Scan(&id, &username, &role, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":         id,
			"username":   username,
			"role":       role,
			"created_at": createdAt,
		})
	}
	return result, rows.Err()
}

// DeleteUser 删除用户
func (pe *PostgresEngine) DeleteUser(username string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `DELETE FROM users WHERE username = $1`
	result, err := pe.db.Exec(query, username)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %s", username)
	}
	return nil
}

// UpdateUserRole 更新用户角色
func (pe *PostgresEngine) UpdateUserRole(username, role string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `UPDATE users SET role = $1, updated_at = CURRENT_TIMESTAMP WHERE username = $2`
	_, err := pe.db.Exec(query, role, username)
	return err
}

// ============================================================================
// 业务管理
// ============================================================================

// CreateBusiness 创建业务
func (pe *PostgresEngine) CreateBusiness(data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	description := getString(data, "description")
	owner := getString(data, "owner")

	query := `INSERT INTO businesses (name, description, owner) VALUES ($1, $2, $3)`
	_, err := pe.db.Exec(query, name, description, owner)
	return err
}

// ListBusiness 列出业务
func (pe *PostgresEngine) ListBusiness(page, pageSize int) ([]map[string]interface{}, int, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	if err := pe.db.QueryRow(`SELECT COUNT(*) FROM businesses`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT id, name, description, status, owner, created_at FROM businesses ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := pe.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var name, description, status, owner string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &description, &status, &owner, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"status":      status,
			"owner":       owner,
			"created_at":  createdAt,
		})
	}
	return result, total, rows.Err()
}

// GetBusiness 获取业务
func (pe *PostgresEngine) GetBusiness(id string) (map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, name, description, status, owner, created_at FROM businesses WHERE id = $1`
	var bid int
	var name, description, status, owner string
	var createdAt time.Time

	err := pe.db.QueryRow(query, id).Scan(&bid, &name, &description, &status, &owner, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("business not found: %s", id)
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":          bid,
		"name":        name,
		"description": description,
		"status":      status,
		"owner":       owner,
		"created_at":  createdAt,
	}, nil
}

// UpdateBusiness 更新业务
func (pe *PostgresEngine) UpdateBusiness(id string, data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	description := getString(data, "description")
	status := getString(data, "status")

	query := `UPDATE businesses SET name = $1, description = $2, status = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4`
	_, err := pe.db.Exec(query, name, description, status, id)
	return err
}

// DeleteBusiness 删除业务
func (pe *PostgresEngine) DeleteBusiness(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `DELETE FROM businesses WHERE id = $1`
	_, err := pe.db.Exec(query, id)
	return err
}

// ============================================================================
// 服务管理
// ============================================================================

// CreateService 创建服务
func (pe *PostgresEngine) CreateService(data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	businessID := getString(data, "business_id")
	description := getString(data, "description")
	owner := getString(data, "owner")

	query := `INSERT INTO services (name, business_id, description, owner) VALUES ($1, NULLIF($2,'')::int, $3, $4)`
	_, err := pe.db.Exec(query, name, businessID, description, owner)
	return err
}

// ListService 列出服务
func (pe *PostgresEngine) ListService(page, pageSize int) ([]map[string]interface{}, int, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	if err := pe.db.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT id, name, business_id, description, status, owner, created_at FROM services ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := pe.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var name, description, status, owner string
		var businessID sql.NullInt32
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &businessID, &description, &status, &owner, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":           id,
			"name":         name,
			"business_id":  businessID.Int32,
			"description":  description,
			"status":       status,
			"owner":        owner,
			"created_at":   createdAt,
		})
	}
	return result, total, rows.Err()
}

// GetService 获取服务
func (pe *PostgresEngine) GetService(id string) (map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, name, business_id, description, status, owner, created_at FROM services WHERE id = $1`
	var bid int
	var name, description, status, owner string
	var businessID sql.NullInt32
	var createdAt time.Time

	err := pe.db.QueryRow(query, id).Scan(&bid, &name, &businessID, &description, &status, &owner, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("service not found: %s", id)
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":          bid,
		"name":        name,
		"business_id": businessID.Int32,
		"description": description,
		"status":      status,
		"owner":       owner,
		"created_at":  createdAt,
	}, nil
}

// UpdateService 更新服务
func (pe *PostgresEngine) UpdateService(id string, data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	status := getString(data, "status")
	query := `UPDATE services SET name = $1, status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`
	_, err := pe.db.Exec(query, name, status, id)
	return err
}

// DeleteService 删除服务
func (pe *PostgresEngine) DeleteService(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `DELETE FROM services WHERE id = $1`
	_, err := pe.db.Exec(query, id)
	return err
}

// ============================================================================
// 采集器管理
// ============================================================================

// CreateCollector 创建采集器
func (pe *PostgresEngine) CreateCollector(data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	colType := getString(data, "type")
	config := getString(data, "config")

	query := `INSERT INTO collectors (name, type, config) VALUES ($1, $2, $3::jsonb)`
	_, err := pe.db.Exec(query, name, colType, config)
	return err
}

// ListCollector 列出采集器
func (pe *PostgresEngine) ListCollector(page, pageSize int) ([]map[string]interface{}, int, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	if err := pe.db.QueryRow(`SELECT COUNT(*) FROM collectors`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT id, name, type, config, status, created_at FROM collectors ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := pe.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var name, colType, status string
		var config []byte
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &colType, &config, &status, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":         id,
			"name":       name,
			"type":       colType,
			"config":     string(config),
			"status":     status,
			"created_at": createdAt,
		})
	}
	return result, total, rows.Err()
}

// GetCollector 获取采集器
func (pe *PostgresEngine) GetCollector(id string) (map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, name, type, config, status, created_at FROM collectors WHERE id = $1`
	var bid int
	var name, colType, status string
	var config []byte
	var createdAt time.Time

	err := pe.db.QueryRow(query, id).Scan(&bid, &name, &colType, &config, &status, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("collector not found: %s", id)
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":         bid,
		"name":       name,
		"type":       colType,
		"config":     string(config),
		"status":     status,
		"created_at": createdAt,
	}, nil
}

// UpdateCollector 更新采集器
func (pe *PostgresEngine) UpdateCollector(id string, data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	status := getString(data, "status")
	query := `UPDATE collectors SET name = $1, status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`
	_, err := pe.db.Exec(query, name, status, id)
	return err
}

// DeleteCollector 删除采集器
func (pe *PostgresEngine) DeleteCollector(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `DELETE FROM collectors WHERE id = $1`
	_, err := pe.db.Exec(query, id)
	return err
}

// ============================================================================
// 数据节点管理
// ============================================================================

// CreateDataNode 创建数据节点
func (pe *PostgresEngine) CreateDataNode(data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	address := getString(data, "address")

	query := `INSERT INTO data_nodes (name, address) VALUES ($1, $2)`
	_, err := pe.db.Exec(query, name, address)
	return err
}

// ListDataNode 列出数据节点
func (pe *PostgresEngine) ListDataNode(page, pageSize int) ([]map[string]interface{}, int, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	var total int
	if err := pe.db.QueryRow(`SELECT COUNT(*) FROM data_nodes`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := `SELECT id, name, address, status, created_at FROM data_nodes ORDER BY id LIMIT $1 OFFSET $2`
	rows, err := pe.db.Query(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]interface{}
	for rows.Next() {
		var id int
		var name, address, status string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &address, &status, &createdAt); err != nil {
			continue
		}
		result = append(result, map[string]interface{}{
			"id":         id,
			"name":       name,
			"address":    address,
			"status":     status,
			"created_at": createdAt,
		})
	}
	return result, total, rows.Err()
}

// GetDataNode 获取数据节点
func (pe *PostgresEngine) GetDataNode(id string) (map[string]interface{}, error) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	query := `SELECT id, name, address, status, created_at FROM data_nodes WHERE id = $1`
	var bid int
	var name, address, status string
	var createdAt time.Time

	err := pe.db.QueryRow(query, id).Scan(&bid, &name, &address, &status, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("data node not found: %s", id)
		}
		return nil, err
	}

	return map[string]interface{}{
		"id":         bid,
		"name":       name,
		"address":    address,
		"status":     status,
		"created_at": createdAt,
	}, nil
}

// UpdateDataNode 更新数据节点
func (pe *PostgresEngine) UpdateDataNode(id string, data map[string]interface{}) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	name := getString(data, "name")
	status := getString(data, "status")
	query := `UPDATE data_nodes SET name = $1, status = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`
	_, err := pe.db.Exec(query, name, status, id)
	return err
}

// DeleteDataNode 删除数据节点
func (pe *PostgresEngine) DeleteDataNode(id string) error {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	query := `DELETE FROM data_nodes WHERE id = $1`
	_, err := pe.db.Exec(query, id)
	return err
}

// ============================================================================
// 辅助函数
// ============================================================================

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// DB 返回底层数据库连接
func (pe *PostgresEngine) DB() interface{} {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.db
}

// Stop 关闭引擎
func (pe *PostgresEngine) Stop() {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.closed {
		return
	}
	pe.closed = true
	if pe.db != nil {
		pe.db.Close()
	}
}

// Health 健康检查
func (pe *PostgresEngine) Health() error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	if pe.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return pe.db.Ping()
}

// Stats 获取统计信息
func (pe *PostgresEngine) Stats() map[string]interface{} {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	if pe.db == nil {
		return map[string]interface{}{"status": "not_connected"}
	}
	stats := pe.db.Stats()
	return map[string]interface{}{
		"status":           "connected",
		"open_connections": stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"wait_count":       stats.WaitCount,
		"wait_duration":    stats.WaitDuration.String(),
	}
}
