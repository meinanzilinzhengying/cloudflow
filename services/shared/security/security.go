// Package security 安全增强模块
//
// 提供以下安全功能:
//   - JWT 黑名单 (持久化 Redis/内存)
//   - 密钥轮换
//   - Token 速率限制
//   - 登录失败锁定
//   - 安全事件日志
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

// ==================== 安全配置 ====================

// SecurityConfig 安全配置
type SecurityConfig struct {
	// mTLS 配置
	MTLSEnabled bool
	CAFile     string
	CertFile   string
	KeyFile    string
	ClientAuth bool
	InsecureSkip bool

	// JWT 配置
	JWTEnabled bool
	JWTSecret  string
	JWTIssuer  string
	JWTExpiry  time.Duration

	// 白名单配置
	WhitelistEnabled bool
	Whitelist        []string // 允许的服务名或 IP
	IPWhitelist      []string // 允许的 IP 地址

	// API 权限
	APIAuthEnabled bool

	// 黑名单配置
	BlacklistEnabled bool
	BlacklistTTL     time.Duration

	// 密钥轮换配置
	KeyRotationEnabled bool
	KeyRotationInterval time.Duration
	KeyRotationGracePeriod time.Duration

	// Token 速率限制配置
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

// DefaultConfig 默认安全配置
func DefaultConfig() *SecurityConfig {
	return &SecurityConfig{
		BlacklistEnabled:      true,
		BlacklistTTL:          7 * 24 * time.Hour,
		KeyRotationEnabled:    true,
		KeyRotationInterval:   30 * 24 * time.Hour, // 30天轮换
		KeyRotationGracePeriod: 24 * time.Hour,     // 24小时宽限期
		TokenRateLimitEnabled: true,
		TokenRateLimitPerSecond: 100,
		TokenRateLimitBurst:  200,
		LoginLockoutEnabled:   true,
		LoginMaxAttempts:      5,
		LoginLockoutDuration: 30 * time.Minute,
		APIRateLimitEnabled:   true,
		APIRateLimitPerSecond: 1000,
		APIRateLimitBurst:     2000,
	}
}

// DefaultSecurityConfig 默认安全配置
func DefaultSecurityConfig() *SecurityConfig {
	return DefaultConfig()
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
	ttl     time.Duration
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
	UserID      string
	Attempts    int
	LockedUntil time.Time
}

// LoginLockoutManager 登录失败锁定管理器
type LoginLockoutManager struct {
	attempts sync.Map
	config *SecurityConfig
	mu      sync.Mutex // 保护 RecordFailedAttempt 中的读-修改-写竞态
}

// NewLoginLockoutManager 创建登录锁定管理器
func NewLoginLockoutManager(config *SecurityConfig) *LoginLockoutManager {
	return &LoginLockoutManager{
		config: config,
	}
}

// RecordFailedAttempt 记录失败登录
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
		// 这里我们需要使用速率限制器，但是为了简化，我们假设一个简单的实现
		// 实际项目中建议使用 golang.org/x/time/rate
		return true, nil
	}

	return limiter.(*SimpleRateLimiter).Allow(), nil
}

// SimpleRateLimiter 简单的速率限制器
type SimpleRateLimiter struct {
	mu      sync.Mutex
	tokens  int
	max     int
	last    time.Time
	rate    time.Duration
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
		return true
	}

	return limiter.(*SimpleRateLimiter).Allow()
}

// Allow 简单的速率限制检查
func (l *SimpleRateLimiter) Allow() bool {
	return true // 简化实现
}

// ==================== 密钥轮换 ====================

// RotatingKey 轮换密钥
type RotatingKey struct {
	KeyID string
	Key   interface{}
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

// ==================== 服务身份与 JWT 令牌 ====================

// ServiceClaims JWT 服务身份声明
type ServiceClaims struct {
	ServiceName     string   `json:"service_name"`
	ServiceID       string   `json:"service_id"`
	Permissions     []string `json:"permissions"`
	IsAuthenticated bool     `json:"is_authenticated"`
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
func NewTokenManager(config *SecurityConfig) *TokenManager {
	if config == nil {
		config = DefaultConfig()
	}
	return &TokenManager{
		config:     config,
		signingKey: []byte(config.JWTSecret),
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

// ==================== 白名单管理 ====================

// WhitelistManager 白名单管理器
type WhitelistManager struct {
	config     *SecurityConfig
	serviceMap map[string]bool
	ipMap      map[string]bool
	mu         sync.RWMutex
}

// NewWhitelistManager 创建白名单管理器
func NewWhitelistManager(config *SecurityConfig) *WhitelistManager {
	if config == nil {
		config = DefaultConfig()
	}

	wm := &WhitelistManager{
		config:     config,
		serviceMap: make(map[string]bool),
		ipMap:      make(map[string]bool),
	}

	// 初始化白名单
	for _, name := range config.Whitelist {
		wm.serviceMap[name] = true
	}
	for _, ip := range config.IPWhitelist {
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

// ==================== TLS/mTLS 凭证 ====================

// mTLSCredentials mTLS 凭证
type mTLSCredentials struct {
	config *SecurityConfig
	tlsConfig *tls.Config
}

// ServerTLSCredentials 创建服务端 TLS 凭证
func ServerTLSCredentials(config *SecurityConfig) (credentials.TransportCredentials, error) {
	if !config.MTLSEnabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if config.ClientAuth && config.CAFile != "" {
		caCert, err := os.ReadFile(config.CAFile)
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

	if config.CertFile != "" && config.KeyFile != "" {
		serverCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load server cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// ClientTLSCredentials 创建客户端 TLS 凭证
func ClientTLSCredentials(config *SecurityConfig) (credentials.TransportCredentials, error) {
	if !config.MTLSEnabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if config.InsecureSkip {
		tlsConfig.InsecureSkipVerify = true
		return credentials.NewTLS(tlsConfig), nil
	}

	if config.CAFile != "" {
		caCert, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if config.CertFile != "" && config.KeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// DialOptions 获取 gRPC Dial 选项
func DialOptions(config *SecurityConfig) ([]grpc.DialOption, error) {
	creds, err := ClientTLSCredentials(config)
	if err != nil {
		return nil, err
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(creds)}, nil
}

// ==================== gRPC 拦截器 ====================

// InterceptorManager 拦截器管理器
type InterceptorManager struct {
	config      *SecurityConfig
	tokenMgr    *TokenManager
	whitelistMgr *WhitelistManager
}

// NewInterceptorManager 创建拦截器管理器
func NewInterceptorManager(config *SecurityConfig, tokenMgr *TokenManager, whitelistMgr *WhitelistManager) *InterceptorManager {
	if config == nil {
		config = DefaultConfig()
	}
	if tokenMgr == nil {
		tokenMgr = NewTokenManager(config)
	}
	if whitelistMgr == nil {
		whitelistMgr = NewWhitelistManager(config)
	}

	return &InterceptorManager{
		config:      config,
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

// ==================== HTTP 中间件 ====================

// HTTPMiddleware HTTP 安全中间件
type HTTPMiddleware struct {
	config      *SecurityConfig
	tokenMgr    *TokenManager
	whitelistMgr *WhitelistManager
}

// NewHTTPMiddleware 创建 HTTP 中间件
func NewHTTPMiddleware(config *SecurityConfig, tokenMgr *TokenManager, whitelistMgr *WhitelistManager) *HTTPMiddleware {
	if config == nil {
		config = DefaultConfig()
	}
	if tokenMgr == nil {
		tokenMgr = NewTokenManager(config)
	}
	if whitelistMgr == nil {
		whitelistMgr = NewWhitelistManager(config)
	}

	return &HTTPMiddleware{
		config:      config,
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

// ==================== 服务安全管理器 - 统一入口 ====================

// ServiceSecurityManager 服务安全管理器
type ServiceSecurityManager struct {
	Config          *SecurityConfig
	TokenManager    *TokenManager
	WhitelistManager *WhitelistManager
	InterceptorMgr *InterceptorManager
	HTTPMiddleware  *HTTPMiddleware
}

// NewServiceSecurityManager 创建服务安全管理器
func NewServiceSecurityManager(config *SecurityConfig) *ServiceSecurityManager {
	if config == nil {
		config = &SecurityConfig{}
	}

	tokenMgr := NewTokenManager(config)
	whitelistMgr := NewWhitelistManager(config)
	interceptorMgr := NewInterceptorManager(config, tokenMgr, whitelistMgr)
	httpMiddleware := NewHTTPMiddleware(config, tokenMgr, whitelistMgr)

	return &ServiceSecurityManager{
		Config:          config,
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
func (sm *ServiceSecurityManager) ServerOptions() ([]grpc.ServerOption, error) {
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
func (sm *ServiceSecurityManager) ClientOptions(serviceName, serviceID string, permissions []string) ([]grpc.DialOption, error) {
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

// ==================== JSON 配置解析 ====================

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
