package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) *redis.Client {
	// 使用本地 Redis 进行测试
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // 使用独立的 DB 避免干扰
	})
	
	// 测试连接
	err := client.Ping(context.Background()).Err()
	if err != nil {
		t.Skip("Redis not available, skipping test")
	}
	
	// 清理测试数据
	client.FlushDB(context.Background())
	
	return client
}

func TestLimiterAllow(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  5,
		DefaultWindow: time.Second,
	})
	
	ctx := context.Background()
	key := "test:user:123"
	
	// 前 5 次请求应该允许
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}
	
	// 第 6 次请求应该被拒绝
	allowed, err := limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.False(t, allowed, "6th request should be denied")
	
	// 等待窗口过期
	time.Sleep(time.Second + 100*time.Millisecond)
	
	// 再次请求应该允许
	allowed, err = limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, allowed, "Request after window should be allowed")
}

func TestLimiterAllowWithInfo(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
	})
	
	ctx := context.Background()
	key := "test:user:456"
	
	info, err := limiter.AllowWithInfo(ctx, key)
	require.NoError(t, err)
	
	assert.True(t, info.Allowed)
	assert.Equal(t, 10, info.Limit)
	assert.Equal(t, 9, info.Remaining)
	assert.WithinDuration(t, time.Now().Add(time.Minute), info.ResetAt, time.Second)
}

func TestLimiterCustomRules(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		CustomRules: map[string]Rule{
			"api:login": {Limit: 5, Window: time.Minute},    // 登录接口更严格
			"api:*":     {Limit: 50, Window: time.Minute},   // API 通用规则
		},
	})
	
	ctx := context.Background()
	
	// 测试登录接口限流
	loginKey := "api:login:user:123"
	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow(ctx, loginKey)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
	
	// 第 6 次应该被拒绝
	allowed, err := limiter.Allow(ctx, loginKey)
	require.NoError(t, err)
	assert.False(t, allowed)
	
	// 测试其他 API（使用通用规则）
	apiKey := "api:query:user:456"
	for i := 0; i < 50; i++ {
		allowed, err := limiter.Allow(ctx, apiKey)
		require.NoError(t, err)
		assert.True(t, allowed)
	}
}

func TestLimiterReset(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  5,
		DefaultWindow: time.Minute,
	})
	
	ctx := context.Background()
	key := "test:user:reset"
	
	// 消耗所有配额
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, key)
	}
	
	// 验证已被限流
	allowed, _ := limiter.Allow(ctx, key)
	assert.False(t, allowed)
	
	// 重置
	err := limiter.Reset(ctx, key)
	require.NoError(t, err)
	
	// 再次请求应该允许
	allowed, err = limiter.Allow(ctx, key)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestLimiterGetUsage(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  10,
		DefaultWindow: time.Minute,
	})
	
	ctx := context.Background()
	key := "test:user:usage"
	
	// 初始状态
	usage, err := limiter.GetUsage(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, 0, usage.Current)
	assert.Equal(t, 10, usage.Limit)
	assert.Equal(t, 10, usage.Remaining)
	
	// 发送 3 个请求
	for i := 0; i < 3; i++ {
		limiter.Allow(ctx, key)
	}
	
	// 检查使用情况
	usage, err = limiter.GetUsage(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, 3, usage.Current)
	assert.Equal(t, 7, usage.Remaining)
}

func TestLimiterConcurrent(t *testing.T) {
	redisClient := setupTestRedis(t)
	defer redisClient.Close()
	
	limiter := NewLimiter(Config{
		Redis:         redisClient,
		DefaultLimit:  100,
		DefaultWindow: time.Second,
	})
	
	ctx := context.Background()
	key := "test:concurrent"
	
	// 并发发送 150 个请求
	done := make(chan bool, 150)
	for i := 0; i < 150; i++ {
		go func() {
			limiter.Allow(ctx, key)
			done <- true
		}()
	}
	
	// 等待完成
	for i := 0; i < 150; i++ {
		<-done
	}
	
	// 验证最终状态
	usage, err := limiter.GetUsage(ctx, key)
	require.NoError(t, err)
	assert.LessOrEqual(t, usage.Current, 100)
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		key     string
		pattern string
		want    bool
	}{
		{"api:login", "*", true},
		{"api:login", "api:login", true},
		{"api:login", "api:*", true},
		{"api:query", "api:*", true},
		{"other:endpoint", "api:*", false},
		{"api:login:user", "api:login", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.key+"_"+tt.pattern, func(t *testing.T) {
			got := matchPattern(tt.key, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}
