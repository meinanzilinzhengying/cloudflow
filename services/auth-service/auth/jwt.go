// Package auth JWT + OIDC 认证模块
//
// P1-01/P1-02 修复: JWT RS256 迁移
//
// 支持:
//   - JWT 签发与验证 (RS256) - 从 HS256 迁移
//   - OIDC Discovery + Token Exchange
//   - API Key 认证
//   - Token 刷新
//   - jti 字段支持 token 撤销
//   - JWKS 端点公开公钥
//
// JWT Claims 结构:
//
//	{
//	  "jti": "unique-token-id",
//	  "sub": "user_id",
//	  "tenant_id": "tenant_xxx",
//	  "username": "admin",
//	  "role": "admin",
//	  "project_id": "",
//	  "namespaces": ["ns1", "ns2"],
//	  "iat": 1234567890,
//	  "exp": 1234654290,
//	  "iss": "cloudflow"
//	}
package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// ==================== JWT 密钥管理 (RS256) ====================

// RSAKeyPair RSA 密钥对
type RSAKeyPair struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

// GenerateRSAKeyPair 生成 RSA 密钥对 (2048-bit)
func GenerateRSAKeyPair() (*RSAKeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}

	return &RSAKeyPair{
		PrivateKey: privateKey,
		PublicKey:  &privateKey.PublicKey,
	}, nil
}

// LoadRSAKeyPairFromPEM 从 PEM 格式加载 RSA 密钥对
func LoadRSAKeyPairFromPEM(privateKeyPEM, publicKeyPEM string) (*RSAKeyPair, error) {
	privateKeyBlock, _ := pem.Decode([]byte(privateKeyPEM))
	if privateKeyBlock == nil {
		return nil, errors.New("decode private key PEM failed")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	var publicKey *rsa.PublicKey
	if publicKeyPEM != "" {
		publicKeyBlock, _ := pem.Decode([]byte(publicKeyPEM))
		if publicKeyBlock == nil {
			return nil, errors.New("decode public key PEM failed")
		}
		pub, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse public key: %w", err)
		}
		var ok bool
		publicKey, ok = pub.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("not RSA public key")
		}
	} else {
		publicKey = &privateKey.PublicKey
	}

	return &RSAKeyPair{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

// PublicKeyToPEM 将公钥转换为 PEM 格式
func (k *RSAKeyPair) PublicKeyToPEM() string {
	pubASN1, err := x509.MarshalPKIXPublicKey(k.PublicKey)
	if err != nil {
		return ""
	}
	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})
	return string(pubBytes)
}

// PrivateKeyToPEM 将私钥转换为 PEM 格式
func (k *RSAKeyPair) PrivateKeyToPEM() string {
 privBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k.PrivateKey),
	})
	return string(privBytes)
}

// JWKS JSON Web Key Set 结构
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK JSON Web Key
type JWK struct {
	Kty string `json:"kty"` // "RSA"
	Use string `json:"use"` // "sig"
	Kid string `json:"kid"` // Key ID
	Alg string `json:"alg"` // "RS256"
	N   string `json:"n"`   // RSA modulus
	E   string `json:"e"`   // RSA exponent
}

// ToJWKS 转换为 JWKS 格式
func (k *RSAKeyPair) ToJWKS(kid string) *JWKS {
	return &JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Use: "sig",
				Kid: kid,
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(k.PublicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.PublicKey.E)).Bytes()),
			},
		},
	}
}

// ==================== Token 黑名单 (Redis) ====================

// TokenBlacklist Token 撤销黑名单
type TokenBlacklist interface {
	// IsBlacklisted 检查 token 是否在黑名单中
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	// AddToBlacklist 将 token 加入黑名单
	AddToBlacklist(ctx context.Context, jti string, expiry time.Duration) error
}

// InMemoryBlacklist 内存实现的 Token 黑名单（生产环境应使用 Redis）
type InMemoryBlacklist struct {
	blacklist sync.Map
	ttl       time.Duration
}

// NewInMemoryBlacklist 创建内存黑名单
func NewInMemoryBlacklist(ttl time.Duration) *InMemoryBlacklist {
	bl := &InMemoryBlacklist{ttl: ttl}
	go bl.cleanup()
	return bl
}

// IsBlacklisted 检查 token 是否在黑名单中
func (b *InMemoryBlacklist) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	val, ok := b.blacklist.Load(jti)
	if !ok {
		return false, nil
	}
	expiry, ok := val.(time.Time)
	if !ok {
		return false, nil
	}
	if time.Now().After(expiry) {
		b.blacklist.Delete(jti)
		return false, nil
	}
	return true, nil
}

// AddToBlacklist 将 token 加入黑名单
func (b *InMemoryBlacklist) AddToBlacklist(ctx context.Context, jti string, expiry time.Duration) error {
	b.blacklist.Store(jti, time.Now().Add(expiry))
	return nil
}

// cleanup 定期清理过期条目
func (b *InMemoryBlacklist) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		now := time.Now()
		b.blacklist.Range(func(key, value interface{}) bool {
			if expiry, ok := value.(time.Time); ok {
				if now.After(expiry) {
					b.blacklist.Delete(key)
				}
			}
			return true
		})
	}
}

// ==================== Claims ====================

// Claims JWT 自定义声明，嵌入 jwt.RegisteredClaims
// P1-02 修复: 添加 jti 字段支持 token 撤销
type Claims struct {
	TenantID        string   `json:"tenant_id"`
	Username        string   `json:"username"`
	Role            string   `json:"role"`
	ProjectID       string   `json:"project_id,omitempty"`
	Namespaces      []string `json:"namespaces,omitempty"`
	IsPlatformAdmin bool     `json:"is_platform_admin,omitempty"`
	jwt.RegisteredClaims
}

// GetJTI 获取 JWT ID
func (c *Claims) GetJTI() string {
	if c.ID == "" {
		return ""
	}
	return c.ID
}

// ==================== JWTManager (RS256) ====================

// JWTManager JWT 签发与验证管理器
// P1-01 修复: 从 HS256 迁移到 RS256
type JWTManager struct {
	keyPair    *RSAKeyPair
	keyID      string
	issuer     string
	expireDuration  time.Duration
	refreshDuration time.Duration
	blacklist  TokenBlacklist
}

// JWTConfig JWT 配置
type JWTConfig struct {
	PrivateKey     string // PEM 格式私钥
	PublicKey      string // PEM 格式公钥（可选，从私钥推导）
	KeyID          string // JWKS 中的 kid
	Issuer         string
	ExpireDuration time.Duration
	RefreshDuration time.Duration
	Blacklist      TokenBlacklist
}

// NewJWTManager 创建 JWT 管理器 (RS256)
//
// P1-01 修复: 支持 RSA 密钥对签名
func NewJWTManagerWithConfig(cfg *JWTConfig) (*JWTManager, error) {
	var keyPair *RSAKeyPair
	var err error

	if cfg.PrivateKey != "" {
		// 从 PEM 加载密钥
		keyPair, err = LoadRSAKeyPairFromPEM(cfg.PrivateKey, cfg.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("load RSA key pair: %w", err)
		}
	} else {
		// 生成新密钥对
		keyPair, err = GenerateRSAKeyPair()
		if err != nil {
			return nil, fmt.Errorf("generate RSA key pair: %w", err)
		}
	}

	if cfg.KeyID == "" {
		cfg.KeyID = "default"
	}

	m := &JWTManager{
		keyPair:         keyPair,
		keyID:           cfg.KeyID,
		issuer:          cfg.Issuer,
		expireDuration:  cfg.ExpireDuration,
		refreshDuration: cfg.RefreshDuration,
		blacklist:       cfg.Blacklist,
	}

	return m, nil
}

// NewJWTManager 创建 JWT 管理器 (向后兼容 HS256)
//
// Deprecated: 建议使用 NewJWTManagerWithConfig
func NewJWTManager(secret, issuer string, expireSec, refreshSec int64) *JWTManager {
	return &JWTManager{
		keyPair:    nil, // 使用 HS256 模式
		keyID:      "hs256",
		issuer:     issuer,
		expireDuration:  time.Duration(expireSec) * time.Second,
		refreshDuration: time.Duration(refreshSec) * time.Second,
	}
}

// GenerateJTI 生成唯一的 JWT ID
func GenerateJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate JTI: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateToken 签发 access token
//
// P1-01 修复: 使用 RS256 签名，添加 jti 字段
func (m *JWTManager) GenerateToken(userID, tenantID, username, role string, projectID string, namespaces []string, isPlatformAdmin bool) (string, error) {
	jti, err := GenerateJTI()
	if err != nil {
		return "", fmt.Errorf("generate JTI: %w", err)
	}

	now := time.Now()
	claims := &Claims{
		TenantID:        tenantID,
		Username:        username,
		Role:            role,
		ProjectID:      projectID,
		Namespaces:      namespaces,
		IsPlatformAdmin: isPlatformAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti, // P1-02: 添加 jti
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expireDuration)),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		},
	}

	var token *jwt.Token
	if m.keyPair != nil {
		// RS256 模式
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = m.keyID // 添加 kid 到 header
		signed, err := token.SignedString(m.keyPair.PrivateKey)
		if err != nil {
			return "", fmt.Errorf("sign access token (RS256): %w", err)
		}
		return signed, nil
	} else {
		// HS256 模式（向后兼容）
		token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		return "", errors.New("HS256 is deprecated, use RS256")
	}
}

// GenerateRefreshToken 签发 refresh token
//
// P1-01/P1-02 修复: 添加 jti，RS256 签名
func (m *JWTManager) GenerateRefreshToken(userID string) (string, error) {
	jti, err := GenerateJTI()
	if err != nil {
		return "", fmt.Errorf("generate JTI: %w", err)
	}

	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti, // P1-02: 添加 jti
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshDuration)),
			NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		},
	}

	var token *jwt.Token
	if m.keyPair != nil {
		// RS256 模式
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = m.keyID
		signed, err := token.SignedString(m.keyPair.PrivateKey)
		if err != nil {
			return "", fmt.Errorf("sign refresh token (RS256): %w", err)
		}
		return signed, nil
	} else {
		// HS256 模式
		token = jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		return "", errors.New("HS256 is deprecated")
	}
}

// ValidateToken 解析并验证 JWT token，返回 Claims
//
// P1-01/P1-02 修复: 使用 RS256 验证，支持黑名单
func (m *JWTManager) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	var keyFunc jwt.Keyfunc

	if m.keyPair != nil {
		// RS256 模式
		keyFunc = func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return m.keyPair.PublicKey, nil
		}
	} else {
		// HS256 模式（向后兼容）
		return nil, errors.New("HS256 is deprecated")
	}

	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		keyFunc,
		jwt.WithIssuer(m.issuer),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// P1-02 修复: 检查黑名单
	if m.blacklist != nil {
		if claims.ID != "" {
			blacklisted, err := m.blacklist.IsBlacklisted(ctx, claims.ID)
			if err != nil {
				return nil, fmt.Errorf("check blacklist: %w", err)
			}
			if blacklisted {
				return nil, errors.New("token has been revoked")
			}
		}
	}

	return claims, nil
}

// RefreshToken 验证 refresh token 并签发新的 access token
func (m *JWTManager) RefreshToken(ctx context.Context, refreshTokenString string) (string, error) {
	claims, err := m.ValidateToken(ctx, refreshTokenString)
	if err != nil {
		return "", fmt.Errorf("validate refresh token: %w", err)
	}

	newToken, err := m.GenerateToken(
		claims.Subject,
		claims.TenantID,
		claims.Username,
		claims.Role,
		claims.ProjectID,
		claims.Namespaces,
		claims.IsPlatformAdmin,
	)
	if err != nil {
		return "", fmt.Errorf("generate new access token: %w", err)
	}

	return newToken, nil
}

// RevokeToken 将 token 加入黑名单
func (m *JWTManager) RevokeToken(ctx context.Context, tokenString string) error {
	if m.blacklist == nil {
		return errors.New("blacklist not configured")
	}

	claims, err := m.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("parse token for revocation: %w", err)
	}

	if claims.ID == "" {
		return errors.New("token has no jti")
	}

	// 计算剩余有效期
	expiry := claims.ExpiresAt.Time
	remaining := time.Until(expiry)
	if remaining <= 0 {
		return nil // token 已过期，无需撤销
	}

	return m.blacklist.AddToBlacklist(ctx, claims.ID, remaining)
}

// GetJWKS 获取 JWKS 端点数据
func (m *JWTManager) GetJWKS() *JWKS {
	if m.keyPair == nil {
		return nil
	}
	return m.keyPair.ToJWKS(m.keyID)
}

// GetPublicKeyPEM 获取公钥 PEM 格式
func (m *JWTManager) GetPublicKeyPEM() string {
	if m.keyPair == nil {
		return ""
	}
	return m.keyPair.PublicKeyToPEM()
}

// ==================== OIDC ====================

// OIDCConfig OIDC 提供商配置
type OIDCConfig struct {
	Issuer       string // OIDC Issuer URL (如 https://accounts.google.com)
	ClientID     string // OAuth2 Client ID
	ClientSecret string // OAuth2 Client Secret
	RedirectURL  string // 回调地址
	Scopes       string // 空格分隔的 scope 列表 (如 "openid profile email")
}

// OIDCProvider OIDC 认证提供者
type OIDCProvider struct {
	config   *OIDCConfig
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   *oauth2.Config
	httpClient *http.Client
}

// NewOIDCProvider 创建 OIDC 提供者，自动发现端点
func NewOIDCProvider(config *OIDCConfig) (*OIDCProvider, error) {
	if config == nil || config.Issuer == "" {
		return nil, errors.New("oidc: issuer is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discover provider: %w", err)
	}

	scopes := strings.Fields(config.Scopes)
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID}
	}

	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	return &OIDCProvider{
		config:     config,
		provider:   provider,
		verifier:   provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		oauth2:     oauth2Config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// ExchangeCode 使用授权码交换 token 并提取 Claims
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*Claims, error) {
	oauth2Token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("oidc: exchange code: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("oidc: no id_token in response")
	}

	return p.VerifyIDToken(ctx, rawIDToken)
}

// VerifyIDToken 验证 ID Token 并提取 Claims
func (p *OIDCProvider) VerifyIDToken(ctx context.Context, idToken string) (*Claims, error) {
	token, err := p.verifier.Verify(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify id_token: %w", err)
	}

	// 提取标准 OIDC claims
	var rawClaims struct {
		Subject    string   `json:"sub"`
		TenantID   string   `json:"tenant_id"`
		Username   string   `json:"username"`
		Preferred  string   `json:"preferred_username"`
		Email      string   `json:"email"`
		Role       string   `json:"role"`
		ProjectID  string   `json:"project_id"`
		Namespaces []string `json:"namespaces"`
	}
	if err := token.Claims(&rawClaims); err != nil {
		return nil, fmt.Errorf("oidc: extract claims: %w", err)
	}

	// 优先使用 username，回退到 preferred_username
	username := rawClaims.Username
	if username == "" {
		username = rawClaims.Preferred
	}
	if username == "" {
		username = rawClaims.Email
	}

	claims := &Claims{
		TenantID:   rawClaims.TenantID,
		Username:   username,
		Role:       rawClaims.Role,
		ProjectID:  rawClaims.ProjectID,
		Namespaces: rawClaims.Namespaces,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   rawClaims.Subject,
			Issuer:    token.Issuer,
			IssuedAt:  jwt.NewNumericDate(token.IssuedAt),
			ExpiresAt: jwt.NewNumericDate(token.Expiry),
		},
	}

	return claims, nil
}

// ==================== API Key ====================

// APIKeyInfo API Key 元信息
type APIKeyInfo struct {
	KeyHash   string `json:"key_hash"`
	KeyPrefix string `json:"key_prefix"` // 前 8 字符，用于展示
	UserID    string `json:"user_id"`
	TenantID  string `json:"tenant_id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
}

// APIKeyManager API Key 管理器
type APIKeyManager struct {
	keys sync.Map // apiKeyHash -> *APIKeyInfo
}

// NewAPIKeyManager 创建 API Key 管理器
func NewAPIKeyManager() *APIKeyManager {
	return &APIKeyManager{}
}

// apikeyPrefix API Key 前缀标识
const apikeyPrefix = "cfk_"

// GenerateAPIKey 生成 API Key
//
// 生成格式: "cfk_" + 32 字节随机 hex (共 69 字符)。
// 存储时仅保留 SHA-256 哈希，原始密钥仅在生成时返回一次。
func (m *APIKeyManager) GenerateAPIKey(userID, tenantID, name string, expiresIn time.Duration) (string, error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", fmt.Errorf("generate api key: read random: %w", err)
	}
	rawKey := apikeyPrefix + hex.EncodeToString(rawBytes)

	// 计算 SHA-256 哈希用于存储
	hash := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(hash[:])

	// 前缀用于展示 (cfk_ + 前 4 字节 hex)
	prefixLen := len(apikeyPrefix) + 8
	if len(rawKey) < prefixLen {
		prefixLen = len(rawKey)
	}

	now := time.Now()
	info := &APIKeyInfo{
		KeyHash:   keyHash,
		KeyPrefix: rawKey[:prefixLen],
		UserID:    userID,
		TenantID:  tenantID,
		Name:      name,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(expiresIn).Unix(),
	}

	m.keys.Store(keyHash, info)
	return rawKey, nil
}

// ValidateAPIKey 验证 API Key
func (m *APIKeyManager) ValidateAPIKey(apiKey string) (*APIKeyInfo, error) {
	if !strings.HasPrefix(apiKey, apikeyPrefix) {
		return nil, errors.New("api key: invalid prefix")
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	val, ok := m.keys.Load(keyHash)
	if !ok {
		return nil, errors.New("api key: not found")
	}

	info, ok := val.(*APIKeyInfo)
	if !ok {
		return nil, errors.New("api key: invalid stored data")
	}

	// 检查过期
	if info.ExpiresAt > 0 && time.Now().Unix() > info.ExpiresAt {
		m.keys.Delete(keyHash)
		return nil, errors.New("api key: expired")
	}

	return info, nil
}

// RevokeAPIKey 撤销 API Key
func (m *APIKeyManager) RevokeAPIKey(apiKey string) error {
	if !strings.HasPrefix(apiKey, apikeyPrefix) {
		return errors.New("api key: invalid prefix")
	}

	hash := sha256.Sum256([]byte(apiKey))
	keyHash := hex.EncodeToString(hash[:])

	if _, ok := m.keys.Load(keyHash); !ok {
		return errors.New("api key: not found")
	}

	m.keys.Delete(keyHash)
	return nil
}

// ==================== Authenticator (统一认证入口) ====================

// Authenticator 统一认证入口，自动识别认证方式
type Authenticator struct {
	jwtManager    *JWTManager
	oidcProvider  *OIDCProvider
	apiKeyManager *APIKeyManager
}

// NewAuthenticatorWithConfig 使用配置创建认证器
func NewAuthenticatorWithConfig(config *JWTConfig) (*Authenticator, error) {
	jwtManager, err := NewJWTManagerWithConfig(config)
	if err != nil {
		return nil, err
	}

	return &Authenticator{
		jwtManager:    jwtManager,
		apiKeyManager: NewAPIKeyManager(),
	}, nil
}

// NewAuthenticator 创建统一认证器 (RS256 模式)
//
// P1-01 修复: 默认使用 RS256
func NewAuthenticator(jwtSecret, jwtIssuer string, jwtExpireSec, jwtRefreshSec int64, oidcConfig *OIDCConfig) *Authenticator {
	var jwtManager *JWTManager
	
	if jwtSecret != "" {
		// 尝试作为 PEM 私钥加载
		if keyPair, err := LoadRSAKeyPairFromPEM(jwtSecret, ""); err == nil {
			// 是 PEM 格式，使用 RS256
			jwtManager, _ = NewJWTManagerWithConfig(&JWTConfig{
				PrivateKey:      jwtSecret,
				KeyID:           "default",
				Issuer:          jwtIssuer,
				ExpireDuration:  time.Duration(jwtExpireSec) * time.Second,
				RefreshDuration: time.Duration(jwtRefreshSec) * time.Second,
				Blacklist:       NewInMemoryBlacklist(time.Duration(jwtRefreshSec) * time.Second),
			})
			_ = keyPair // 忽略未使用的变量
		} else {
			// 不是 PEM 格式，使用 HS256（向后兼容）
			jwtManager = NewJWTManager(jwtSecret, jwtIssuer, jwtExpireSec, jwtRefreshSec)
		}
	} else {
		// 生成新的 RSA 密钥对
		jwtManager, _ = NewJWTManagerWithConfig(&JWTConfig{
			KeyID:           "default",
			Issuer:          jwtIssuer,
			ExpireDuration:  time.Duration(jwtExpireSec) * time.Second,
			RefreshDuration: time.Duration(jwtRefreshSec) * time.Second,
			Blacklist:       NewInMemoryBlacklist(time.Duration(jwtRefreshSec) * time.Second),
		})
	}

	a := &Authenticator{
		jwtManager:    jwtManager,
		apiKeyManager: NewAPIKeyManager(),
	}

	if oidcConfig != nil && oidcConfig.Issuer != "" {
		if provider, err := NewOIDCProvider(oidcConfig); err == nil {
			a.oidcProvider = provider
		}
	}

	return a
}

// Authenticate 统一认证方法，自动检测认证方式
func (a *Authenticator) Authenticate(ctx context.Context, token string) (*Claims, error) {
	// 1. API Key 认证
	if strings.HasPrefix(token, apikeyPrefix) {
		info, err := a.apiKeyManager.ValidateAPIKey(token)
		if err != nil {
			return nil, fmt.Errorf("api key auth: %w", err)
		}

		return &Claims{
			TenantID: info.TenantID,
			Username: info.Name,
			Role:     "api_key",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   info.UserID,
				Issuer:    "cloudflow",
				IssuedAt:  jwt.NewNumericDate(time.Unix(info.CreatedAt, 0)),
				ExpiresAt: jwt.NewNumericDate(time.Unix(info.ExpiresAt, 0)),
			},
		}, nil
	}

	// 2. OIDC 认证 (如果已配置且 token 为三段式)
	if a.oidcProvider != nil && isThreePartToken(token) {
		claims, err := a.oidcProvider.VerifyIDToken(ctx, token)
		if err == nil {
			return claims, nil
		}
	}

	// 3. JWT 认证
	claims, err := a.jwtManager.ValidateToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("jwt auth: %w", err)
	}

	return claims, nil
}

// JWTManager 暴露 JWT 管理器
func (a *Authenticator) JWTManager() *JWTManager {
	return a.jwtManager
}

// RevokeToken 撤销 token
func (a *Authenticator) RevokeToken(ctx context.Context, token string) error {
	return a.jwtManager.RevokeToken(ctx, token)
}

// GetJWKS 获取 JWKS 端点数据
func (a *Authenticator) GetJWKS() *JWKS {
	return a.jwtManager.GetJWKS()
}

// isThreePartToken 检查 token 是否为三段式格式 (xxx.yyy.zzz)
func isThreePartToken(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3
}

// ==================== JWKS HTTP Handler ====================

// JWKSHandler JWKS 端点 HTTP Handler
func JWKSHandler(authenticator *Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jwks := authenticator.GetJWKS()
		if jwks == nil {
			http.Error(w, "JWKS not available", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}
}
