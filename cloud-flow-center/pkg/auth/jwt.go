// Package auth 提供认证相关功能
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateRandomString 生成指定长度的随机字符串
func GenerateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

// Claims JWT声明

type Claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// JWTManager JWT管理器
type JWTManager struct {
	secretKey string
	issuer    string
	audience  []string
}

// JWT 常量
const (
	defaultIssuer   = "cloud-flow-center"
	defaultAudience = "cloud-flow-portal"
)

// NewJWTManager 创建JWT管理器
// N2 修复: 移除死代码（config.Load() 已强制要求 JWT Secret，此处 secretKey 必不为空）
func NewJWTManager(secretKey string) *JWTManager {
	if secretKey == "" {
		log.Fatalf("JWT Secret 为空，这不应发生。请检查 CLOUD_FLOW_JWT_SECRET 环境变量或配置文件 center.jwt.secret_key")
	}
	return &JWTManager{
		secretKey: secretKey,
		issuer:    defaultIssuer,
		audience:  []string{defaultAudience},
	}
}

// GenerateToken 生成JWT令牌
func (m *JWTManager) GenerateToken(userID, role string, duration time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings(m.audience),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-5 * time.Second)), // 允许 5 秒时钟偏差
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.secretKey))
}

// ValidateToken 验证JWT令牌
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(m.secretKey), nil
		},
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience...),
	)

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
