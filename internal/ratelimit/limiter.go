// Package ratelimit 提供基于 Redis 的滑动窗口速率限制器
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config 速率限制配置
type Config struct {
	// Redis 客户端
	Redis *redis.Client
	
	// 默认限流规则（请求数/时间窗口）
	DefaultLimit  int
	DefaultWindow time.Duration
	
	// 自定义限流规则（key -> limit, window）
	CustomRules map[string]Rule
}

// Rule 限流规则
type Rule struct {
	Limit  int
	Window time.Duration
}

// Limiter 速率限制器
type Limiter struct {
	config Config
}

// NewLimiter 创建速率限制器
func NewLimiter(config Config) *Limiter {
	if config.DefaultLimit == 0 {
		config.DefaultLimit = 100 // 默认 100 请求/分钟
	}
	if config.DefaultWindow == 0 {
		config.DefaultWindow = time.Minute
	}
	if config.CustomRules == nil {
		config.CustomRules = make(map[string]Rule)
	}
	
	return &Limiter{config: config}
}

// Allow 检查是否允许请求（滑动窗口算法）
func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	rule := l.getRule(key)
	now := time.Now()
	windowStart := now.Add(-rule.Window).UnixNano()
	
	// 使用 Redis pipeline 提高性能
	pipe := l.config.Redis.Pipeline()
	
	// 1. 移除过期记录
	pipe.ZRemRangeByScore(ctx, "rate:"+key, "0", strconv.FormatInt(windowStart, 10))
	
	// 2. 获取当前窗口内的请求数
	pipe.ZCard(ctx, "rate:"+key)
	
	// 3. 添加新记录
	pipe.ZAdd(ctx, "rate:"+key, &redis.Z{
		Score:  float64(now.UnixNano()),
		Member: strconv.FormatInt(now.UnixNano(), 10) + ":" + generateRandomID(),
	})
	
	// 4. 设置过期时间
	pipe.Expire(ctx, "rate:"+key, rule.Window)
	
	// 执行 pipeline
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, fmt.Errorf("pipeline exec failed: %w", err)
	}
	
	// 获取当前请求数
	cardCmd := cmds[1].(*redis.IntCmd)
	count, err := cardCmd.Result()
	if err != nil {
		return false, fmt.Errorf("get count failed: %w", err)
	}
	
	// 判断是否超限（注意：count 是添加前的数量）
	if count >= int64(rule.Limit) {
		// 超限，移除刚才添加的记录
		l.config.Redis.ZRem(ctx, "rate:"+key, 
			strconv.FormatInt(now.UnixNano(), 10)+":*")
		return false, nil
	}
	
	return true, nil
}

// AllowWithInfo 检查并返回详细信息
func (l *Limiter) AllowWithInfo(ctx context.Context, key string) (*AllowInfo, error) {
	rule := l.getRule(key)
	now := time.Now()
	windowStart := now.Add(-rule.Window).UnixNano()
	
	// 获取当前请求数
	count, err := l.config.Redis.ZCount(ctx, "rate:"+key, 
		strconv.FormatInt(windowStart, 10), 
		strconv.FormatInt(now.UnixNano(), 10)).Result()
	if err != nil {
		return nil, fmt.Errorf("get count failed: %w", err)
	}
	
	remaining := rule.Limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	
	info := &AllowInfo{
		Allowed:   remaining > 0,
		Limit:     rule.Limit,
		Remaining: remaining,
		ResetAt:   now.Add(rule.Window),
	}
	
	if info.Allowed {
		// 添加新记录
		l.config.Redis.ZAdd(ctx, "rate:"+key, &redis.Z{
			Score:  float64(now.UnixNano()),
			Member: strconv.FormatInt(now.UnixNano(), 10) + ":" + generateRandomID(),
		})
		l.config.Redis.Expire(ctx, "rate:"+key, rule.Window)
	}
	
	return info, nil
}

// getRule 获取限流规则
func (l *Limiter) getRule(key string) Rule {
	// 先查找自定义规则
	for pattern, rule := range l.config.CustomRules {
		if matchPattern(key, pattern) {
			return rule
		}
	}
	
	// 返回默认规则
	return Rule{
		Limit:  l.config.DefaultLimit,
		Window: l.config.DefaultWindow,
	}
}

// Reset 重置指定 key 的限流计数
func (l *Limiter) Reset(ctx context.Context, key string) error {
	return l.config.Redis.Del(ctx, "rate:"+key).Err()
}

// GetUsage 获取当前使用情况
func (l *Limiter) GetUsage(ctx context.Context, key string) (*UsageInfo, error) {
	rule := l.getRule(key)
	now := time.Now()
	windowStart := now.Add(-rule.Window).UnixNano()
	
	count, err := l.config.Redis.ZCount(ctx, "rate:"+key,
		strconv.FormatInt(windowStart, 10),
		strconv.FormatInt(now.UnixNano(), 10)).Result()
	if err != nil {
		return nil, fmt.Errorf("get usage failed: %w", err)
	}
	
	return &UsageInfo{
		Current:   int(count),
		Limit:     rule.Limit,
		Remaining: rule.Limit - int(count),
		Window:    rule.Window,
	}, nil
}

// AllowInfo 允许请求的详细信息
type AllowInfo struct {
	Allowed   bool      `json:"allowed"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}

// UsageInfo 使用情况信息
type UsageInfo struct {
	Current   int           `json:"current"`
	Limit     int           `json:"limit"`
	Remaining int           `json:"remaining"`
	Window    time.Duration `json:"window"`
}

// matchPattern 简单的模式匹配
func matchPattern(key, pattern string) bool {
	// 支持通配符 *
	if pattern == "*" {
		return true
	}
	
	// 精确匹配
	if key == pattern {
		return true
	}
	
	// 前缀匹配（pattern 以 * 结尾）
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(key) >= len(prefix) && key[:len(prefix)] == prefix
	}
	
	return false
}

// generateRandomID 生成随机 ID（简化版）
func generateRandomID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
