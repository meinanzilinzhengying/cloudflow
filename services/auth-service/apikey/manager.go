// Package apikey API Key 持久化管理
//
// P1-04 修复: API Key 持久化到 TiDB
//
// 提供 API Key 的创建、验证、撤销功能，数据持久化到 TiDB。
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Manager API Key 管理器
type Manager struct {
	db         *sql.DB
	inMemory   sync.Map // keyHash -> *APIKeyInfo（内存缓存）
	cacheTTL   time.Duration
}

// Config 数据库配置
type Config struct {
	Addr         string
	User         string
	Password     string
	Database     string
	CacheTTL     time.Duration // 缓存过期时间，默认 5 分钟
}

// APIKeyInfo API Key 信息
type APIKeyInfo struct {
	KeyHash   string    `json:"key_hash"`   // SHA-256 哈希
	KeyPrefix string    `json:"key_prefix"` // 前缀用于展示
	UserID    string    `json:"user_id"`     // 用户 ID
	TenantID  string    `json:"tenant_id"`   // 租户 ID
	Name      string    `json:"name"`        // Key 名称
	CreatedAt time.Time `json:"created_at"`  // 创建时间
	ExpiresAt time.Time `json:"expires_at"`  // 过期时间
	Revoked   bool      `json:"revoked"`     // 是否已撤销
}

// NewManager 创建 API Key 管理器
func NewManager(cfg *Config) (*Manager, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4",
		cfg.User, cfg.Password, cfg.Addr, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	m := &Manager{
		db:       db,
		cacheTTL: cfg.CacheTTL,
	}

	if m.cacheTTL == 0 {
		m.cacheTTL = 5 * time.Minute
	}

	// 初始化表
	if err := m.initTable(); err != nil {
		return nil, fmt.Errorf("init table: %w", err)
	}

	// 加载现有 Keys 到缓存
	if err := m.loadToCache(); err != nil {
		return nil, fmt.Errorf("load to cache: %w", err)
	}

	// 启动缓存清理
	go m.cleanupCache()

	return m, nil
}

// initTable 初始化 API Key 表
func (m *Manager) initTable() error {
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS api_keys (
		key_hash VARCHAR(64) PRIMARY KEY,
		key_prefix VARCHAR(16) NOT NULL,
		user_id VARCHAR(64) NOT NULL,
		tenant_id VARCHAR(64) NOT NULL,
		name VARCHAR(100) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		revoked BOOLEAN DEFAULT FALSE,
		INDEX idx_user_id (user_id),
		INDEX idx_tenant_id (tenant_id),
		INDEX idx_expires_at (expires_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`

	if _, err := m.db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("create api_keys table: %w", err)
	}

	return nil
}

// loadToCache 从数据库加载 API Keys 到内存缓存
func (m *Manager) loadToCache() error {
	rows, err := m.db.Query(`
		SELECT key_hash, key_prefix, user_id, tenant_id, name, created_at, expires_at, revoked
		FROM api_keys
		WHERE revoked = FALSE AND expires_at > NOW()
	`)
	if err != nil {
		return fmt.Errorf("load api keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var info APIKeyInfo
		if err := rows.Scan(
			&info.KeyHash, &info.KeyPrefix, &info.UserID,
			&info.TenantID, &info.Name, &info.CreatedAt, &info.ExpiresAt, &info.Revoked,
		); err != nil {
			continue
		}
		m.inMemory.Store(info.KeyHash, &info)
	}

	return nil
}

// cleanupCache 定期清理过期缓存
func (m *Manager) cleanupCache() {
	ticker := time.NewTicker(1 * time.Hour)
	now := time.Now()

	for range ticker.C {
		m.inMemory.Range(func(key, value interface{}) bool {
			if info, ok := value.(*APIKeyInfo); ok {
				if now.After(info.ExpiresAt) || info.Revoked {
					m.inMemory.Delete(key)
				}
			}
			return true
		})
	}
}

// Prefix API Key 前缀标识
const Prefix = "cfk_"

// Generate 生成 API Key
//
// P1-04 修复: 持久化到 TiDB
func (m *Manager) Generate(ctx context.Context, userID, tenantID, name string, expiresIn time.Duration) (string, error) {
	// 生成随机 Key
	rawBytes := make([]byte, 32)
	if _, err := generateRandomBytes(rawBytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	rawKey := Prefix + hex.EncodeToString(rawBytes)

	// 计算 SHA-256 哈希
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	// 前缀用于展示
	prefixLen := len(Prefix) + 8
	if len(rawKey) < prefixLen {
		prefixLen = len(rawKey)
	}
	keyPrefix := rawKey[:prefixLen]

	now := time.Now()
	expiresAt := now.Add(expiresIn)

	// 插入数据库
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO api_keys (key_hash, key_prefix, user_id, tenant_id, name, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		keyHash, keyPrefix, userID, tenantID, name, now, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert api key: %w", err)
	}

	// 写入缓存
	info := &APIKeyInfo{
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		UserID:    userID,
		TenantID:  tenantID,
		Name:      name,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Revoked:   false,
	}
	m.inMemory.Store(keyHash, info)

	return rawKey, nil
}

// Validate 验证 API Key
//
// P1-04 修复: 优先从缓存验证，缓存未命中从数据库查询
func (m *Manager) Validate(ctx context.Context, apiKey string) (*APIKeyInfo, error) {
	if !isValidPrefix(apiKey) {
		return nil, errors.New("invalid api key prefix")
	}

	// 计算哈希
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	// 1. 先从缓存查找
	if cached, ok := m.inMemory.Load(keyHash); ok {
		info := cached.(*APIKeyInfo)
		if !info.Revoked && time.Now().Before(info.ExpiresAt) {
			return info, nil
		}
		// 已过期或已撤销，从缓存删除
		m.inMemory.Delete(keyHash)
	}

	// 2. 缓存未命中，从数据库查询
	var info APIKeyInfo
	err := m.db.QueryRowContext(ctx,
		`SELECT key_hash, key_prefix, user_id, tenant_id, name, created_at, expires_at, revoked
		 FROM api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(
		&info.KeyHash, &info.KeyPrefix, &info.UserID,
		&info.TenantID, &info.Name, &info.CreatedAt, &info.ExpiresAt, &info.Revoked,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("api key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query api key: %w", err)
	}

	// 检查过期和撤销
	if info.Revoked {
		return nil, errors.New("api key has been revoked")
	}
	if time.Now().After(info.ExpiresAt) {
		return nil, errors.New("api key has expired")
	}

	// 更新缓存
	m.inMemory.Store(keyHash, &info)

	return &info, nil
}

// Revoke 撤销 API Key
//
// P1-04 修复: 持久化撤销状态
func (m *Manager) Revoke(ctx context.Context, apiKey string) error {
	if !isValidPrefix(apiKey) {
		return errors.New("invalid api key prefix")
	}

	// 计算哈希
	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	// 更新数据库
	result, err := m.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked = TRUE WHERE key_hash = ? AND revoked = FALSE`,
		keyHash,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("api key not found or already revoked")
	}

	// 从缓存删除
	m.inMemory.Delete(keyHash)

	return nil
}

// ListByUser 列出用户的所有 API Keys
func (m *Manager) ListByUser(ctx context.Context, userID string) ([]*APIKeyInfo, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT key_hash, key_prefix, user_id, tenant_id, name, created_at, expires_at, revoked
		 FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*APIKeyInfo
	for rows.Next() {
		var info APIKeyInfo
		if err := rows.Scan(
			&info.KeyHash, &info.KeyPrefix, &info.UserID,
			&info.TenantID, &info.Name, &info.CreatedAt, &info.ExpiresAt, &info.Revoked,
		); err != nil {
			continue
		}
		keys = append(keys, &info)
	}

	return keys, nil
}

// CleanupExpired 清理过期 API Keys
func (m *Manager) CleanupExpired(ctx context.Context) (int64, error) {
	// 从缓存清理
	m.inMemory.Range(func(key, value interface{}) bool {
		if info, ok := value.(*APIKeyInfo); ok {
			if time.Now().After(info.ExpiresAt) {
				m.inMemory.Delete(key)
			}
		}
		return true
	})

	// 从数据库删除过期 Keys
	result, err := m.db.ExecContext(ctx,
		`DELETE FROM api_keys WHERE expires_at < NOW() OR (revoked = TRUE AND expires_at < DATE_SUB(NOW(), INTERVAL 30 DAY))`,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired keys: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	return rowsAffected, nil
}

// Close 关闭数据库连接
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// ============================================================================
// 辅助函数
// ============================================================================

// generateRandomBytes 生成随机字节
// FIX: 使用 crypto/rand 替代 time.Now() 生成密码学安全的随机数
func generateRandomBytes(b []byte) error {
	_, err := rand.Read(b)
	return err
}

// isValidPrefix 检查 API Key 前缀
func isValidPrefix(apiKey string) bool {
	return len(apiKey) > len(Prefix) && apiKey[:len(Prefix)] == Prefix
}
