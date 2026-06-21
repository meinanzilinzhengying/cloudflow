package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// JWT 黑名单测试
// ============================================================================

func TestJWTManager_ValidateToken_WithBlacklist(t *testing.T) {
	// 创建带内存黑名单的 JWT 管理器
	blacklist := NewInMemoryBlacklist(1 * time.Hour)
	
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  1 * time.Hour,
		refreshDuration: 24 * time.Hour,
		blacklist:       blacklist,
	}
	
	// 签发 token
	token, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	
	// 验证通过
	claims, err := manager.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("validate token before revoke: %v", err)
	}
	if claims.Subject != "user1" {
		t.Errorf("expected userID=user1, got %s", claims.Subject)
	}
	
	// 撤销 token
	err = manager.RevokeToken(context.Background(), token)
	if err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	
	// 再次验证应该失败
	_, err = manager.ValidateToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected token to be invalid after revocation")
	}
	if err.Error() != "token has been revoked" {
		t.Errorf("expected 'token has been revoked', got: %v", err)
	}
}

func TestJWTManager_ValidateToken_WithoutBlacklist(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	// 无黑名单的 JWT 管理器
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  1 * time.Hour,
		refreshDuration: 24 * time.Hour,
		blacklist:       nil,
	}
	
	token, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	
	// RevokeToken 应该报错（黑名单未配置）
	err = manager.RevokeToken(context.Background(), token)
	if err == nil {
		t.Fatal("expected error when blacklist not configured")
	}
}

func TestJWTManager_RevokeToken_Expired(t *testing.T) {
	blacklist := NewInMemoryBlacklist(1 * time.Hour)
	
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  -1 * time.Hour, // 已经过期
		refreshDuration: 24 * time.Hour,
		blacklist:       blacklist,
	}
	
	token, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	
	// 撤销已过期 token 应该静默成功（无需加入黑名单）
	err = manager.RevokeToken(context.Background(), token)
	if err != nil {
		t.Fatalf("revoke expired token: %v", err)
	}
}

func TestJWTManager_RefreshToken_AfterRevoke(t *testing.T) {
	blacklist := NewInMemoryBlacklist(1 * time.Hour)
	
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  1 * time.Hour,
		refreshDuration: 24 * time.Hour,
		blacklist:       blacklist,
	}
	
	// 签发 access token 和 refresh token
	accessToken, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
	if err != nil {
		t.Fatalf("generate access token: %v", err)
	}
	
	refreshToken, err := manager.GenerateRefreshToken("user1")
	if err != nil {
		t.Fatalf("generate refresh token: %v", err)
	}
	
	// 撤销 refresh token
	err = manager.RevokeToken(context.Background(), refreshToken)
	if err != nil {
		t.Fatalf("revoke refresh token: %v", err)
	}
	
	// 使用已撤销的 refresh token 刷新应该失败
	_, err = manager.RefreshToken(context.Background(), refreshToken)
	if err == nil {
		t.Fatal("expected refresh to fail after revoke")
	}
	
	// access token 仍然有效（未撤销）
	claims, err := manager.ValidateToken(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("access token should still be valid: %v", err)
	}
	if claims.Subject != "user1" {
		t.Errorf("expected userID=user1, got %s", claims.Subject)
	}
}

func TestInMemoryBlacklist_Cleanup(t *testing.T) {
	bl := NewInMemoryBlacklist(50 * time.Millisecond)
	
	ctx := context.Background()
	
	// 添加一条记录
	err := bl.AddToBlacklist(ctx, "token1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("add to blacklist: %v", err)
	}
	
	// 检查存在
	isDup, err := bl.IsBlacklisted(ctx, "token1")
	if err != nil {
		t.Fatalf("check blacklist: %v", err)
	}
	if !isDup {
		t.Fatal("expected token to be blacklisted")
	}
	
	// 等待过期
	time.Sleep(200 * time.Millisecond)
	
	// 清理循环每小时运行，但读取时会检查过期
	isDup, err = bl.IsBlacklisted(ctx, "token1")
	if err != nil {
		t.Fatalf("check after expiry: %v", err)
	}
	if isDup {
		t.Fatal("expected token to be expired from blacklist")
	}
}

func TestJWTManager_JTIInClaims(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  1 * time.Hour,
		refreshDuration: 24 * time.Hour,
	}
	
	token, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	
	claims, err := manager.ValidateToken(context.Background(), token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	
	// JTI 应该不为空
	if claims.ID == "" {
		t.Fatal("expected JTI to be set")
	}
	if len(claims.ID) != 32 {
		t.Errorf("expected JTI length 32 (hex of 16 bytes), got %d: %s", len(claims.ID), claims.ID)
	}
}

func TestJWTManager_JTIUniqueness(t *testing.T) {
	keyPair, err := GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	
	manager := &JWTManager{
		keyPair:         keyPair,
		keyID:           "test",
		issuer:          "cloudflow",
		expireDuration:  1 * time.Hour,
		refreshDuration: 24 * time.Hour,
	}
	
	jtis := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := manager.GenerateToken("user1", "tenant1", "admin", "admin", "", nil, false)
		if err != nil {
			t.Fatalf("generate token %d: %v", i, err)
		}
		
		claims, err := manager.ValidateToken(context.Background(), token)
		if err != nil {
			t.Fatalf("validate token %d: %v", i, err)
		}
		
		if jtis[claims.ID] {
			t.Fatalf("duplicate JTI: %s", claims.ID)
		}
		jtis[claims.ID] = true
	}
	
	if len(jtis) != 100 {
		t.Errorf("expected 100 unique JTIs, got %d", len(jtis))
	}
}

func TestClaims_GetJTI(t *testing.T) {
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID: "test-jti-123",
		},
	}
	
	if claims.GetJTI() != "test-jti-123" {
		t.Errorf("expected JTI=test-jti-123, got %s", claims.GetJTI())
	}
	
	// 空 ID
	claims.ID = ""
	if claims.GetJTI() != "" {
		t.Errorf("expected empty JTI, got %s", claims.GetJTI())
	}
}
