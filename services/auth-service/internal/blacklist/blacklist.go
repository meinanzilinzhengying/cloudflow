// Package blacklist 提供 JWT Token 黑名单功能
package blacklist

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// Config 黑名单配置
type Config struct {
	// Redis 客户端
	Redis *redis.Client
	
	// Key 前缀
	KeyPrefix string
	
	// 默认 TTL（如果 token 没有过期时间）
	DefaultTTL time.Duration
}

// Blacklist JWT Token 黑名单管理器
type Blacklist struct {
	config Config
}

// NewBlacklist 创建黑名单管理器
func NewBlacklist(config Config) *Blacklist {
	if config.KeyPrefix == "" {
		config.KeyPrefix = "jwt:blacklist:"
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 24 * time.Hour // 默认 24 小时
	}
	
	return &Blacklist{config: config}
}

// Add 将 token 加入黑名单
func (bl *Blacklist) Add(ctx context.Context, tokenID string, expireAt time.Time) error {
	key := bl.config.KeyPrefix + tokenID
	
	// 计算 TTL
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		// token 已过期，不需要加入黑名单
		return nil
	}
	
	// 如果 TTL 超过默认值，使用默认值（避免长期占用 Redis）
	if ttl > bl.config.DefaultTTL {
		ttl = bl.config.DefaultTTL
	}
	
	// 存储到 Redis
	err := bl.config.Redis.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		return fmt.Errorf("add to blacklist failed: %w", err)
	}
	
	return nil
}

// AddWithReason 将 token 加入黑名单并记录原因
func (bl *Blacklist) AddWithReason(ctx context.Context, tokenID string, expireAt time.Time, reason string) error {
	key := bl.config.KeyPrefix + tokenID
	
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		return nil
	}
	if ttl > bl.config.DefaultTTL {
		ttl = bl.config.DefaultTTL
	}
	
	// 存储 token ID 和原因
	data := map[string]interface{}{
		"reason":    reason,
		"blacklisted_at": time.Now().Unix(),
	}
	
	err := bl.config.Redis.HSet(ctx, key, data).Err()
	if err != nil {
		return fmt.Errorf("add to blacklist with reason failed: %w", err)
	}
	
	bl.config.Redis.Expire(ctx, key, ttl)
	
	return nil
}

// IsBlacklisted 检查 token 是否在黑名单中
func (bl *Blacklist) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := bl.config.KeyPrefix + tokenID
	
	exists, err := bl.config.Redis.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("check blacklist failed: %w", err)
	}
	
	return exists > 0, nil
}

// GetReason 获取 token 被加入黑名单的原因
func (bl *Blacklist) GetReason(ctx context.Context, tokenID string) (string, error) {
	key := bl.config.KeyPrefix + tokenID
	
	reason, err := bl.config.Redis.HGet(ctx, key, "reason").Result()
	if err == redis.Nil {
		// 没有记录原因，可能在黑名单中但没有原因字段
		exists, err := bl.config.Redis.Exists(ctx, key).Result()
		if err != nil {
			return "", err
		}
		if exists > 0 {
			return "unknown", nil
		}
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get reason failed: %w", err)
	}
	
	return reason, nil
}

// Remove 从黑名单中移除 token
func (bl *Blacklist) Remove(ctx context.Context, tokenID string) error {
	key := bl.config.KeyPrefix + tokenID
	
	err := bl.config.Redis.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("remove from blacklist failed: %w", err)
	}
	
	return nil
}

// RevokeUserTokens 撤销用户的所有 token
func (bl *Blacklist) RevokeUserTokens(ctx context.Context, userID string, reason string) error {
	// 查找该用户的所有 token
	pattern := bl.config.KeyPrefix + "user:" + userID + ":*"
	
	iter := bl.config.Redis.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		tokenKey := iter.Val()
		
		// 更新原因
		bl.config.Redis.HSet(ctx, tokenKey, "reason", reason)
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan user tokens failed: %w", err)
	}
	
	return nil
}

// GetStats 获取黑名单统计信息
func (bl *Blacklist) GetStats(ctx context.Context) (*Stats, error) {
	pattern := bl.config.KeyPrefix + "*"
	
	var count int64
	iter := bl.config.Redis.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		count++
	}
	
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan blacklist failed: %w", err)
	}
	
	return &Stats{
		TotalCount: int(count),
	}, nil
}

// Cleanup 清理过期的黑名单条目（通常由 Redis 自动处理）
func (bl *Blacklist) Cleanup(ctx context.Context) error {
	// Redis 会自动删除过期的 key
	// 这里可以添加额外的清理逻辑，如清理孤儿数据
	return nil
}

// Stats 黑名单统计信息
type Stats struct {
	TotalCount int `json:"total_count"`
}

// MiddlewareAuthInterceptor JWT 认证中间件（集成黑名单检查）
func (bl *Blacklist) MiddlewareAuthInterceptor(validateToken func(string) (string, time.Time, error)) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		// TODO: 从 context 或 header 获取 token
		// 这里仅提供框架，实际使用时需要根据具体场景实现
		
		// tokenString := getTokenFromContext(ctx)
		// userID, expireAt, err := validateToken(tokenString)
		// if err != nil {
		//     return ctx, err
		// }
		
		// // 检查黑名单
		// blacklisted, err := bl.IsBlacklisted(ctx, tokenString)
		// if err != nil {
		//     return ctx, err
		// }
		// if blacklisted {
		//     return ctx, errors.New("token has been revoked")
		// }
		
		// // 将用户信息存入 context
		// ctx = context.WithValue(ctx, "userID", userID)
		
		return ctx, nil
	}
}
