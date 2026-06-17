// Package storage 数据库抽象层
// GaussDB(for Redis) 适配实现
// GaussDB Redis 100%兼容Redis协议，直接复用Redis客户端
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// GaussRedisDriver GaussDB(for Redis)驱动实现
type GaussRedisDriver struct{}

// GaussRedisStorage GaussDB(for Redis) KV存储实现
type GaussRedisStorage struct {
	client *redis.Client
	cfg    *Config
}

func init() {
	RegisterDriver(DatabaseGaussRedis, &GaussRedisDriver{})
}

// ==================== GaussDB Redis驱动 ====================

func (d *GaussRedisDriver) Type() DatabaseType {
	return DatabaseGaussRedis
}

func (d *GaussRedisDriver) OpenRelational(cfg *Config) (RelationalStorage, error) {
	return nil, fmt.Errorf("gauss redis does not support relational storage")
}

func (d *GaussRedisDriver) OpenTimeSeries(cfg *Config) (TimeSeriesStorage, error) {
	return nil, fmt.Errorf("gauss redis does not support time series storage")
}

func (d *GaussRedisDriver) OpenKV(cfg *Config) (KVStorage, error) {
	// GaussDB(for Redis) 100%兼容Redis协议
	// 仅需特殊配置优化
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           0,
		PoolSize:     cfg.MaxOpenConns,
		MinIdleConns: cfg.MaxIdleConns,
		// GaussDB Redis 特定优化
		DialTimeout:  10 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		PoolTimeout:  30 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping gauss redis failed: %w", err)
	}

	return &GaussRedisStorage{
		client: client,
		cfg:    cfg,
	}, nil
}

// ==================== GaussDB Redis KV存储实现 ====================
// 由于GaussDB Redis完全兼容Redis协议，所有方法直接复用Redis实现

func (s *GaussRedisStorage) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
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

func (s *GaussRedisStorage) Get(ctx context.Context, key string, value interface{}) error {
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

func (s *GaussRedisStorage) Del(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

func (s *GaussRedisStorage) Exists(ctx context.Context, key string) (bool, error) {
	cnt, err := s.client.Exists(ctx, key).Result()
	return cnt > 0, err
}

func (s *GaussRedisStorage) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

// Hash操作

func (s *GaussRedisStorage) HSet(ctx context.Context, key, field string, value interface{}) error {
	return s.client.HSet(ctx, key, field, value).Err()
}

func (s *GaussRedisStorage) HGet(ctx context.Context, key, field string, value interface{}) error {
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

func (s *GaussRedisStorage) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

// 连接管理

func (s *GaussRedisStorage) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *GaussRedisStorage) Close() error {
	return s.client.Close()
}

// ==================== KV存储工厂函数 ====================

// OpenGaussRedis 打开GaussDB(for Redis) KV存储
func OpenGaussRedis(cfg *Config) (KVStorage, error) {
	driver := &GaussRedisDriver{}
	return driver.OpenKV(cfg)
}
