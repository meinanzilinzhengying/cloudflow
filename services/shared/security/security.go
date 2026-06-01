// Package security 安全增强模块
//
// 提供以下安全功能:
//   - JWT 黑名单 (持久化 Redis/内存)
//   - 密钥轮换
//   - Token 速率限制
//   - 登录失败锁定
//   - 安全事件日志
package security

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ==================== 安全配置 ====================

// SecurityConfig 安全配置
type SecurityConfig struct {
	// 黑名单配置
	BlacklistEnabled bool
	BlacklistTTL     time.Duration

	// 密钥轮换配置
	KeyRotationEnabled bool
	KeyRotationInterval time.Duration
	KeyRotationGracePeriod time.Duration

	// Token 限速配置
	TokenRateLimitEnabled bool
	TokenRateLimitPerSecond float64
	TokenRateLimitBurst int

	// 登录失败锁定配置
	LoginLockoutEnabled bool
	LoginMaxAttempts int
	LoginLockoutDuration time.Duration

	// API 限流配置
	APIRateLimitEnabled bool
	APIRateLimitPerSecond float64
	APIRateLimitBurst int
}

// DefaultSecurityConfig 默认安全配置
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		BlacklistEnabled:      true,
		BlacklistTTL:          7 * 24 * time.Hour,
		KeyRotationEnabled:    true,
		KeyRotationInterval:   30 * 24 * time.Hour, // 30天轮换
		KeyRotationGracePeriod: 24 * time.Hour,     // 24小时宽限期
		TokenRateLimitEnabled: true,
		TokenRateLimitPerSecond: 100,
		TokenRateLimitBurst:   200,
		LoginLockoutEnabled:    true,
		LoginMaxAttempts:       5,
		LoginLockoutDuration:  30 * time.Minute,
		APIRateLimitEnabled:    true,
		APIRateLimitPerSecond: 1000,
		APIRateLimitBurst:      2000,
	}
}

// ==================== JWT 黑名单 ====================

// BlacklistEntry 黑名单条目
type BlacklistEntry struct {
	JTI      string    `json:"jti"`
	Reason   string    `json:"reason"`    // 撤销原因
	RevokedAt time.Time `json:"revoked_at"` // 撤销时间
	ExpiresAt time.Time `json:"expires_at"` // 过期时间
	UserID   string    `json:"user_id"`   // 用户 ID (可选)
}

// TokenBlacklist Token 黑名单接口
type TokenBlacklist interface {
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	AddToBlacklist(ctx context.Context, jti string, entry *BlacklistEntry) error
	RemoveFromBlacklist(ctx context.Context, jti string) error
	GetBlacklistEntry(ctx context.Context, jti string) (*BlacklistEntry, error)
	ClearExpired(ctx context.Context) (int, error)
}

// InMemoryTokenBlacklist 内存实现的黑名单
type InMemoryTokenBlacklist struct {
	entries sync.Map
	ttl    time.Duration
}

// NewInMemoryTokenBlacklist 创建内存黑名单
func NewInMemoryTokenBlacklist(ttl time.Duration) *InMemoryTokenBlacklist {
	bl := &InMemoryTokenBlacklist{ttl: ttl}
	go bl.cleanup()
	return bl
}

// IsBlacklisted 检查 token 是否在黑名单中
func (b *InMemoryTokenBlacklist) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	val, ok := b.entries.Load(jti)
	if !ok {
		return false, nil
	}
	entry, ok := val.(*BlacklistEntry)
	if !ok {
		return false, nil
	}
	if time.Now().After(entry.ExpiresAt) {
		b.entries.Delete(jti)
		return false, nil
	}
	return true, nil
}

// AddToBlacklist 将 token 加入黑名单
func (b *InMemoryTokenBlacklist) AddToBlacklist(ctx context.Context, jti string, entry *BlacklistEntry) error {
	if entry.ExpiresAt.IsZero() {
		entry.ExpiresAt = time.Now().Add(b.ttl)
	}
	if entry.RevokedAt.IsZero() {
		entry.RevokedAt = time.Now()
	}
	b.entries.Store(jti, entry)
	return nil
}

// RemoveFromBlacklist 从黑名单中移除
func (b *InMemoryTokenBlacklist) RemoveFromBlacklist(ctx context.Context, jti string) error {
	b.entries.Delete(jti)
	return nil
}

// GetBlacklistEntry 获取黑名单条目
func (b *InMemoryTokenBlacklist) GetBlacklistEntry(ctx context.Context, jti string) (*BlacklistEntry, error) {
	val, ok := b.entries.Load(jti)
	if !ok {
		return nil, nil
	}
	entry, ok := val.(*BlacklistEntry)
	if !ok {
		return nil, nil
	}
	if time.Now().After(entry.ExpiresAt) {
		b.entries.Delete(jti)
		return nil, nil
	}
	return entry, nil
}

// ClearExpired 清理过期条目
func (b *InMemoryTokenBlacklist) ClearExpired(ctx context.Context) (int, error) {
	count := 0
	b.entries.Range(func(key, value interface{}) bool {
		if entry, ok := value.(*BlacklistEntry); ok {
			if time.Now().After(entry.ExpiresAt) {
				b.entries.Delete(key)
				count++
			}
		}
		return true
	})
	return count, nil
}

// cleanup 定期清理
func (b *InMemoryTokenBlacklist) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		b.ClearExpired(ctx)
		cancel()
	}
}

// ==================== 登录失败锁定 ====================

// LoginAttempt 登录尝试记录
type LoginAttempt struct {
	UserID     string
	Attempts   int
	LockedUntil time.Time
}

// LoginLockoutManager 登录失败锁定管理器
type LoginLockoutManager struct {
	attempts sync.Map
	config *SecurityConfig
	mu     sync.Mutex // FIX: 保护 RecordFailedAttempt 中的读-修改-写竞态
}

// NewLoginLockoutManager 创建登录锁定管理器
func NewLoginLockoutManager(config *SecurityConfig) *LoginLockoutManager {
	return &LoginLockoutManager{
		config: config,
	}
}

// RecordFailedAttempt 记录失败登录
// FIX: 使用 mutex 保护读-修改-写操作，防止并发竞态
func (m *LoginLockoutManager) RecordFailedAttempt(userID string) error {
	if !m.config.LoginLockoutEnabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	val, _ := m.attempts.LoadOrStore(userID, &LoginAttempt{
		UserID: userID,
		Attempts: 1,
	})

	attempt := val.(*LoginAttempt)
	if now.Before(attempt.LockedUntil) {
		return fmt.Errorf("account locked until %s", attempt.LockedUntil.Format(time.RFC3339))
	}

	// 更新尝试次数
	attempt.Attempts++
	if attempt.Attempts >= m.config.LoginMaxAttempts {
		attempt.LockedUntil = now.Add(m.config.LoginLockoutDuration)
		attempt.Attempts = m.config.LoginMaxAttempts // 防止溢出
	}
	m.attempts.Store(userID, attempt)

	if now.Before(attempt.LockedUntil) {
		return fmt.Errorf("account locked until %s", attempt.LockedUntil.Format(time.RFC3339))
	}

	return nil
}

// RecordSuccessfulAttempt 记录成功登录（清除失败计数）
func (m *LoginLockoutManager) RecordSuccessfulAttempt(userID string) {
	m.attempts.Delete(userID)
}

// IsLocked 检查账户是否被锁定
func (m *LoginLockoutManager) IsLocked(userID string) bool {
	val, ok := m.attempts.Load(userID)
	if !ok {
		return false
	}
	attempt := val.(*LoginAttempt)
	return time.Now().Before(attempt.LockedUntil)
}

// GetRemainingAttempts 获取剩余尝试次数
func (m *LoginLockoutManager) GetRemainingAttempts(userID string) int {
	val, ok := m.attempts.Load(userID)
	if !ok {
		return m.config.LoginMaxAttempts
	}
	attempt := val.(*LoginAttempt)
	if time.Now().Before(attempt.LockedUntil) {
		return 0
	}
	return m.config.LoginMaxAttempts - attempt.Attempts
}

// UnlockUser 解锁用户
func (m *LoginLockoutManager) UnlockUser(userID string) {
	m.attempts.Delete(userID)
}

// ==================== Token 速率限制 ====================

// TokenRateLimiter Token 速率限制器（按用户）
type TokenRateLimiter struct {
	users sync.Map
	config *SecurityConfig
}

// NewTokenRateLimiter 创建 Token 速率限制器
func NewTokenRateLimiter(config *SecurityConfig) *TokenRateLimiter {
	return &TokenRateLimiter{
		config: config,
	}
}

// Allow 检查是否允许请求
func (l *TokenRateLimiter) Allow(userID string) (bool, error) {
	if !l.config.TokenRateLimitEnabled {
		return true, nil
	}

	limiter, ok := l.users.Load(userID)
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(l.config.TokenRateLimitPerSecond), l.config.TokenRateLimitBurst)
		l.users.Store(userID, limiter)
	}

	return limiter.(*rate.Limiter).Allow(), nil
}

// CheckAndConsume 检查并消费一次请求
func (l *TokenRateLimiter) CheckAndConsume(userID string) (bool, error) {
	if !l.config.TokenRateLimitEnabled {
		return false, nil
	}

	limiter, ok := l.users.Load(userID)
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(l.config.TokenRateLimitPerSecond), l.config.TokenRateLimitBurst)
		l.users.Store(userID, limiter)
	}

	allowed := limiter.(*rate.Limiter).Allow()
	return !allowed, nil
}

// Wait 等待直到允许请求
func (l *TokenRateLimiter) Wait(ctx context.Context, userID string) error {
	if !l.config.TokenRateLimitEnabled {
		return nil
	}

	limiter, ok := l.users.Load(userID)
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(l.config.TokenRateLimitPerSecond), l.config.TokenRateLimitBurst)
		l.users.Store(userID, limiter)
	}

	return limiter.(*rate.Limiter).Wait(ctx)
}

// ==================== API 速率限制 ====================

// APIRateLimiter API 速率限制器（按用户）
type APIRateLimiter struct {
	users sync.Map
	config *SecurityConfig
}

// NewAPIRateLimiter 创建 API 速率限制器
func NewAPIRateLimiter(config *SecurityConfig) *APIRateLimiter {
	return &APIRateLimiter{config: config}
}

// Allow 检查用户是否允许请求
func (l *APIRateLimiter) Allow(userID string) bool {
	if !l.config.APIRateLimitEnabled {
		return true
	}

	limiter, ok := l.users.Load(userID)
	if !ok {
		limiter = rate.NewLimiter(rate.Limit(l.config.APIRateLimitPerSecond), l.config.APIRateLimitBurst)
		l.users.Store(userID, limiter)
	}

	return limiter.(*rate.Limiter).Allow()
}

// ==================== 密钥轮换 ====================

// RotatingKey 轮换密钥
type RotatingKey struct {
	KeyID string
	Key interface{}
	ActiveUntil time.Time
}

// KeyRotationManager 密钥轮换管理器
type KeyRotationManager struct {
	keys sync.Map // keyID -> RotatingKey
	config *SecurityConfig
	activeKeyID string
	mu sync.RWMutex
}

// NewKeyRotationManager 创建密钥轮换管理器
func NewKeyRotationManager(config *SecurityConfig) *KeyRotationManager {
	return &KeyRotationManager{
		config: config,
	}
}

// AddKey 添加密钥
func (m *KeyRotationManager) AddKey(keyID string, key interface{}, activeUntil time.Time) {
	m.keys.Store(keyID, &RotatingKey{
		KeyID: keyID,
		Key: key,
		ActiveUntil: activeUntil,
	})
}

// SetActiveKey 设置当前活动密钥
func (m *KeyRotationManager) SetActiveKey(keyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.keys.Load(keyID); ok {
		m.activeKeyID = keyID
	}
}

// GetActiveKey 获取当前活动密钥
func (m *KeyRotationManager) GetActiveKey() (string, interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if m.activeKeyID == "" {
		return "", nil, false
	}
	
	val, ok := m.keys.Load(m.activeKeyID)
	if !ok {
		return "", nil, false
	}
	
	key := val.(*RotatingKey)
	if !time.Now().Before(key.ActiveUntil) {
		return "", nil, false
	}
	
	return key.KeyID, key.Key, true
}

// GetKeyByID 获取指定密钥
func (m *KeyRotationManager) GetKeyByID(keyID string) (interface{}, bool) {
	val, ok := m.keys.Load(keyID)
	if !ok {
		return nil, false
	}
	
	key := val.(*RotatingKey)
	// 在宽限期内仍然可以使用旧密钥验证
	if m.config.KeyRotationEnabled && 
	   time.Now().After(key.ActiveUntil) && 
	   time.Now().Before(key.ActiveUntil.Add(m.config.KeyRotationGracePeriod)) {
		return key.Key, true
	}
	
	if !time.Now().Before(key.ActiveUntil) {
		return nil, false
	}
	
	return key.Key, true
}

// ListKeyIDs 列出所有密钥 ID
func (m *KeyRotationManager) ListKeyIDs() []string {
	var keyIDs []string
	m.keys.Range(func(key, value interface{}) bool {
		keyIDs = append(keyIDs, key.(string))
		return true
	})
	return keyIDs
}

// RemoveExpiredKeys 清理过期密钥
func (m *KeyRotationManager) RemoveExpiredKeys() {
	m.keys.Range(func(key, value interface{}) bool {
		k := value.(*RotatingKey)
		// 超过宽限期的密钥删除
		if time.Now().After(k.ActiveUntil.Add(m.config.KeyRotationGracePeriod)) {
			m.keys.Delete(key)
		}
		return true
	})
}

// ==================== 统一安全管理器 ====================

// SecurityManager 统一安全管理器
type SecurityManager struct {
	config *SecurityConfig
	
	blacklist TokenBlacklist
	lockout *LoginLockoutManager
	tokenRateLimiter *TokenRateLimiter
	apiRateLimiter *APIRateLimiter
	keyRotation *KeyRotationManager
}

// NewSecurityManager 创建统一安全管理器
func NewSecurityManager(config *SecurityConfig) *SecurityManager {
	m := &SecurityManager{
		config: config,
		lockout: NewLoginLockoutManager(config),
		tokenRateLimiter: NewTokenRateLimiter(config),
		apiRateLimiter: NewAPIRateLimiter(config),
		keyRotation: NewKeyRotationManager(config),
	}
	
	if config.BlacklistEnabled {
		m.blacklist = NewInMemoryTokenBlacklist(config.BlacklistTTL)
	}
	
	return m
}

// Blacklist 获致黑名单
func (m *SecurityManager) Blacklist() TokenBlacklist {
	return m.blacklist
}

// Lockout 获致登录锁定管理器
func (m *SecurityManager) Lockout() *LoginLockoutManager {
	return m.lockout
}

// TokenRateLimiter 获致 Token 速率限制器
func (m *SecurityManager) TokenRateLimiter() *TokenRateLimiter {
	return m.tokenRateLimiter
}

// APIRateLimiter 获致 API 速率限制器
func (m *SecurityManager) APIRateLimiter() *APIRateLimiter {
	return m.apiRateLimiter
}

// KeyRotation 获致密钥轮换管理器
func (m *SecurityManager) KeyRotation() *KeyRotationManager {
	return m.keyRotation
}

// RevokeToken 撤销 Token
func (m *SecurityManager) RevokeToken(ctx context.Context, jti, reason, userID string, expiresAt time.Time) error {
	if m.blacklist == nil {
		return fmt.Errorf("blacklist not enabled")
	}
	
	return m.blacklist.AddToBlacklist(ctx, jti, &BlacklistEntry{
		JTI: jti,
		Reason: reason,
		RevokedAt: time.Now(),
		ExpiresAt: expiresAt,
		UserID: userID,
	})
}

// IsTokenRevoked 检查 Token 是否已撤销
func (m *SecurityManager) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	if m.blacklist == nil {
		return false, nil
	}
	return m.blacklist.IsBlacklisted(ctx, jti)
}
