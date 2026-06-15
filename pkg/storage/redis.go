// Package storage 数据库抽象层
// Redis KV存储驱动实现
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDriver Redis驱动实现
type RedisDriver struct{}

// RedisStorage Redis KV存储实现
type RedisStorage struct {
	client *redis.Client
	cfg    *Config
}

func init() {
	// Redis驱动注册（独立实现，不通过通用驱动注册）
}

// ==================== Redis驱动 ====================

func (d *RedisDriver) Type() DatabaseType {
	return "redis"
}

func (d *RedisDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	return nil, fmt.Errorf("redis does not support relational storage")
}

func (d *RedisDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("redis does not support time series storage")
}

func (d *RedisDriver) OpenKV(cfg *Config) (KVStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           0,
		PoolSize:     cfg.MaxOpenConns,
		MinIdleConns: cfg.MaxIdleConns,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis failed: %w", err)
	}

	return &RedisStorage{
		client: client,
		cfg:    cfg,
	}, nil
}

// ==================== Redis KV存储实现 ====================

func (s *RedisStorage) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	var data []byte
	var err error

	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}

	return s.client.Set(ctx, key, data, ttl).Err()
}

func (s *RedisStorage) Get(ctx context.Context, key string, value interface{}) error {
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}

	switch v := value.(type) {
	case *string:
		*v = string(data)
	case *[]byte:
		*v = data
	default:
		return json.Unmarshal(data, value)
	}
	return nil
}

func (s *RedisStorage) Del(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

func (s *RedisStorage) Exists(ctx context.Context, key string) (bool, error) {
	cnt, err := s.client.Exists(ctx, key).Result()
	return cnt > 0, err
}

func (s *RedisStorage) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

// Hash操作

func (s *RedisStorage) HSet(ctx context.Context, key, field string, value interface{}) error {
	return s.client.HSet(ctx, key, field, value).Err()
}

func (s *RedisStorage) HGet(ctx context.Context, key, field string, value interface{}) error {
	data, err := s.client.HGet(ctx, key, field).Bytes()
	if err != nil {
		return err
	}

	switch v := value.(type) {
	case *string:
		*v = string(data)
	case *[]byte:
		*v = data
	default:
		return json.Unmarshal(data, value)
	}
	return nil
}

func (s *RedisStorage) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

// 连接管理

func (s *RedisStorage) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStorage) Close() error {
	return s.client.Close()
}

// ==================== KV存储工厂函数 ====================

// OpenRedis 打开Redis KV存储
func OpenRedis(cfg *Config) (KVStorage, error) {
	driver := &RedisDriver{}
	return driver.OpenKV(cfg)
}
