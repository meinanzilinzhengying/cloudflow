package portal

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore 基于 Redis 的外部存储
type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

// NewRedisStore 创建 Redis 存储
func NewRedisStore(addr string) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "",
		DB:       0,
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisStore{client: client, ctx: ctx}, nil
}

// SetCSRFToken 设置 CSRF token
func (rs *RedisStore) SetCSRFToken(token string, expiry time.Duration) error {
	return rs.client.Set(rs.ctx, "csrf:"+token, "1", expiry).Err()
}

// ValidateCSRFToken 验证 CSRF token（一次性使用）
func (rs *RedisStore) ValidateCSRFToken(token string) bool {
	key := "csrf:" + token
	result, err := rs.client.GetDel(rs.ctx, key).Result()
	return err == nil && result == "1"
}

// IncrLoginFailure 增加登录失败计数
func (rs *RedisStore) IncrLoginFailure(username string) (int, error) {
	key := "login_fail:" + username
	count, err := rs.client.Incr(rs.ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if _, err := rs.client.Expire(rs.ctx, key, 30*time.Minute).Result(); err != nil {
	}
	return int(count), nil
}

// GetLoginFailureCount 获取登录失败计数
func (rs *RedisStore) GetLoginFailureCount(username string) (int, error) {
	key := "login_fail:" + username
	count, err := rs.client.Get(rs.ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return count, err
}

// ResetLoginFailure 重置登录失败计数
func (rs *RedisStore) ResetLoginFailure(username string) error {
	return rs.client.Del(rs.ctx, "login_fail:"+username).Err()
}

// Close 关闭连接
func (rs *RedisStore) Close() error {
	return rs.client.Close()
}
