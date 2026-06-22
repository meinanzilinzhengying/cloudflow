package kafkaconsumer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DedupChecker 消息去重检查器接口
type DedupChecker interface {
	// IsDuplicate 检查消息是否已处理过
	// 返回 true 表示是重复消息，应跳过
	IsDuplicate(topic string, partition int32, offset int64) (bool, error)
	// Close 关闭去重器
	Close() error
}

// NoOpDedup 空实现（不做去重，用于测试或禁用场景）
type NoOpDedup struct{}

func (n *NoOpDedup) IsDuplicate(topic string, partition int32, offset int64) (bool, error) {
	return false, nil
}

func (n *NoOpDedup) Close() error { return nil }

// ============================================================================
// Redis 去重器
// ============================================================================

// RedisDedup 基于 Redis 的消息去重器
type RedisDedup struct {
	client redis.UniversalClient
	ttl    time.Duration
	prefix string
	ctx    context.Context
	cancel context.CancelFunc
}

// RedisDedupConfig Redis 去重器配置
type RedisDedupConfig struct {
	Addr     string
	Password string
	DB       int
	TTL      time.Duration
	Prefix   string
}

// DefaultRedisDedupConfig 默认配置
func DefaultRedisDedupConfig() RedisDedupConfig {
	return RedisDedupConfig{
		Addr:   "localhost:6379",
		TTL:    24 * time.Hour,
		Prefix: "cloudflow:dedup",
	}
}

// NewRedisDedup 创建 Redis 去重器
func NewRedisDedup(config RedisDedupConfig) (*RedisDedup, error) {
	if config.TTL == 0 {
		config.TTL = 24 * time.Hour
	}
	if config.Prefix == "" {
		config.Prefix = "cloudflow:dedup"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// 测试连接
	if err := client.Ping(ctx).Err(); err != nil {
		cancel()
		client.Close()
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}

	d := &RedisDedup{
		client: client,
		ttl:    config.TTL,
		prefix: config.Prefix,
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动定期清理 goroutine
	go d.cleanupLoop()

	return d, nil
}

// IsDuplicate 检查消息是否已处理
// 使用 SETNX 原子操作：如果 key 不存在则设置成功（返回 true），否则失败（返回 false）
func (d *RedisDedup) IsDuplicate(topic string, partition int32, offset int64) (bool, error) {
	key := d.key(topic, partition, offset)

	// SET key "1" NX EX ttl_seconds
	ok, err := d.client.SetNX(d.ctx, key, "1", d.ttl).Result()
	if err != nil {
		// Redis 故障时降级：视为非重复消息（允许通过，避免阻塞）
		return false, nil
	}

	// SETNX 返回 true 表示 key 是新创建的（非重复）
	// 返回 false 表示 key 已存在（重复消息）
	return !ok, nil
}

// Close 关闭 Redis 连接
func (d *RedisDedup) Close() error {
	d.cancel()
	return d.client.Close()
}

// key 生成 Redis key
func (d *RedisDedup) key(topic string, partition int32, offset int64) string {
	return fmt.Sprintf("%s:%s:%d:%d", d.prefix, topic, partition, offset)
}

// cleanupLoop 定期扫描并清理过期 key（Redis 有自动过期，这里主要是扫描异常）
func (d *RedisDedup) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// Redis 自动处理过期，无需手动清理
		case <-d.ctx.Done():
			return
		}
	}
}

// ============================================================================
// 内存去重器（用于单机测试，无 Redis 依赖）
// ============================================================================

// MemoryDedup 基于内存的消息去重器（带 TTL 清理）
type MemoryDedup struct {
	mu      sync.RWMutex
	records map[string]time.Time
	ttl     time.Duration
}

// NewMemoryDedup 创建内存去重器
func NewMemoryDedup(ttl time.Duration) *MemoryDedup {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	d := &MemoryDedup{
		records: make(map[string]time.Time),
		ttl:     ttl,
	}
	go d.cleanupLoop()
	return d
}

// IsDuplicate 检查消息是否已处理
func (d *MemoryDedup) IsDuplicate(topic string, partition int32, offset int64) (bool, error) {
	key := fmt.Sprintf("%s:%d:%d", topic, partition, offset)

	d.mu.RLock()
	lastSeen, exists := d.records[key]
	d.mu.RUnlock()

	if exists && time.Since(lastSeen) < d.ttl {
		return true, nil
	}

	d.mu.Lock()
	d.records[key] = time.Now()
	d.mu.Unlock()
	return false, nil
}

// Close 关闭
func (d *MemoryDedup) Close() error { return nil }

// cleanupLoop 定期清理过期记录
func (d *MemoryDedup) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for key, ts := range d.records {
			if now.Sub(ts) > d.ttl {
				delete(d.records, key)
			}
		}
		d.mu.Unlock()
	}
}

// Stats 返回去重统计（仅内存模式）
func (d *MemoryDedup) Stats() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.records)
}
