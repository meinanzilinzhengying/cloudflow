package blacklist

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   2, // 使用独立的 DB
	})
	
	err := client.Ping(context.Background()).Err()
	if err != nil {
		t.Skip("Redis not available, skipping test")
	}
	
	client.FlushDB(context.Background())
	return client
}

func TestBlacklistAddAndCheck(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	tokenID := "token-123"
	expireAt := time.Now().Add(30 * time.Minute)
	
	// 添加到黑名单
	err := bl.Add(ctx, tokenID, expireAt)
	require.NoError(t, err)
	
	// 检查是否在黑名单中
	blacklisted, err := bl.IsBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	assert.True(t, blacklisted)
}

func TestBlacklistExpiredToken(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	tokenID := "token-expired"
	expireAt := time.Now().Add(-10 * time.Minute) // 已过期
	
	// 添加已过期的 token
	err := bl.Add(ctx, tokenID, expireAt)
	require.NoError(t, err)
	
	// 不应该在黑名单中
	blacklisted, err := bl.IsBlacklisted(ctx, tokenID)
	require.NoError(t, err)
	assert.False(t, blacklisted)
}

func TestBlacklistWithReason(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	tokenID := "token-reason"
	expireAt := time.Now().Add(30 * time.Minute)
	reason := "user logout"
	
	// 添加带原因的黑名单
	err := bl.AddWithReason(ctx, tokenID, expireAt, reason)
	require.NoError(t, err)
	
	// 检查原因
	gotReason, err := bl.GetReason(ctx, tokenID)
	require.NoError(t, err)
	assert.Equal(t, reason, gotReason)
}

func TestBlacklistRemove(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	tokenID := "token-remove"
	expireAt := time.Now().Add(30 * time.Minute)
	
	// 添加
	err := bl.Add(ctx, tokenID, expireAt)
	require.NoError(t, err)
	
	// 验证存在
	blacklisted, _ := bl.IsBlacklisted(ctx, tokenID)
	assert.True(t, blacklisted)
	
	// 移除
	err = bl.Remove(ctx, tokenID)
	require.NoError(t, err)
	
	// 验证已移除
	blacklisted, _ = bl.IsBlacklisted(ctx, tokenID)
	assert.False(t, blacklisted)
}

func TestBlacklistRevokeUserTokens(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	userID := "user-123"
	
	// 添加用户的多个 token
	for i := 0; i < 5; i++ {
		tokenID := "user:" + userID + ":token-" + string(rune('0'+i))
		expireAt := time.Now().Add(30 * time.Minute)
		err := bl.AddWithReason(ctx, tokenID, expireAt, "active")
		require.NoError(t, err)
	}
	
	// 撤销所有 token
	err := bl.RevokeUserTokens(ctx, userID, "password changed")
	require.NoError(t, err)
	
	// 验证所有 token 的原因已更新
	for i := 0; i < 5; i++ {
		tokenID := "user:" + userID + ":token-" + string(rune('0'+i))
		reason, err := bl.GetReason(ctx, tokenID)
		require.NoError(t, err)
		assert.Equal(t, "password changed", reason)
	}
}

func TestBlacklistGetStats(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	
	// 添加一些 token
	for i := 0; i < 10; i++ {
		tokenID := "token-stat-" + string(rune('0'+i))
		expireAt := time.Now().Add(30 * time.Minute)
		bl.Add(ctx, tokenID, expireAt)
	}
	
	// 获取统计
	stats, err := bl.GetStats(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.TotalCount, 10)
}

func TestBlacklistDefaultTTL(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "test:blacklist:",
		DefaultTTL: 2 * time.Second, // 很短的 TTL 用于测试
	})
	
	ctx := context.Background()
	tokenID := "token-ttl"
	expireAt := time.Now().Add(1 * time.Hour) // 很长，但会被 DefaultTTL 限制
	
	err := bl.Add(ctx, tokenID, expireAt)
	require.NoError(t, err)
	
	// 验证存在
	blacklisted, _ := bl.IsBlacklisted(ctx, tokenID)
	assert.True(t, blacklisted)
	
	// 等待 TTL 过期
	time.Sleep(3 * time.Second)
	
	// 应该已过期
	blacklisted, _ = bl.IsBlacklisted(ctx, tokenID)
	assert.False(t, blacklisted)
}

func TestBlacklistKeyPrefix(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	bl1 := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "prefix1:",
		DefaultTTL: time.Hour,
	})
	
	bl2 := NewBlacklist(Config{
		Redis:      redisClient,
		KeyPrefix:  "prefix2:",
		DefaultTTL: time.Hour,
	})
	
	ctx := context.Background()
	tokenID := "shared-token"
	expireAt := time.Now().Add(30 * time.Minute)
	
	// 在 prefix1 中添加
	err := bl1.Add(ctx, tokenID, expireAt)
	require.NoError(t, err)
	
	// prefix1 中应该存在
	blacklisted, _ := bl1.IsBlacklisted(ctx, tokenID)
	assert.True(t, blacklisted)
	
	// prefix2 中应该不存在
	blacklisted, _ = bl2.IsBlacklisted(ctx, tokenID)
	assert.False(t, blacklisted)
}
