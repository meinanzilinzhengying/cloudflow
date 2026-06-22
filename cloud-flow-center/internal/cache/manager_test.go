//go:build linux

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestDefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()
	if cfg.Addr != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", cfg.Addr)
	}
	if cfg.DB != 0 {
		t.Errorf("expected DB 0, got %d", cfg.DB)
	}
}

func TestCacheManagerRoles(t *testing.T) {
	// 尝试连接 Redis，如果不可用则跳过测试
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping: " + err.Error())
	}
	client.Close()

	cm, err := NewCacheManager(nil)
	if err != nil {
		t.Fatalf("NewCacheManager failed: %v", err)
	}
	defer cm.Close()

	ctx2 := context.Background()

	// 测试缓存操作
	if err := cm.CacheSet(ctx2, "test_key", "test_value", 5*time.Second); err != nil {
		t.Fatalf("CacheSet failed: %v", err)
	}
	val, err := cm.CacheGet(ctx2, "test_key")
	if err != nil {
		t.Fatalf("CacheGet failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %s", val)
	}

	// 测试队列操作
	if err := cm.QueuePush(ctx2, "test_queue", "msg1", "msg2"); err != nil {
		t.Fatalf("QueuePush failed: %v", err)
	}
	len, err := cm.QueueLen(ctx2, "test_queue")
	if err != nil {
		t.Fatalf("QueueLen failed: %v", err)
	}
	if len != 2 {
		t.Errorf("expected queue len 2, got %d", len)
	}

	// 测试状态操作
	if err := cm.StateSet(ctx2, "config", `{"version":"1.0"}`); err != nil {
		t.Fatalf("StateSet failed: %v", err)
	}
	stateVal, err := cm.StateGet(ctx2, "config")
	if err != nil {
		t.Fatalf("StateGet failed: %v", err)
	}
	if stateVal != `{"version":"1.0"}` {
		t.Errorf("unexpected state value: %s", stateVal)
	}

	// 测试角色前缀隔离
	// 不同角色的 key 不应该互相干扰
	cacheStats, err := cm.CacheStats(ctx2)
	if err != nil {
		t.Fatalf("CacheStats failed: %v", err)
	}
	if cacheStats["role"] != "cache" {
		t.Errorf("expected role 'cache', got %v", cacheStats["role"])
	}

	queueStats, err := cm.QueueStats(ctx2)
	if err != nil {
		t.Fatalf("QueueStats failed: %v", err)
	}
	if queueStats["role"] != "queue" {
		t.Errorf("expected role 'queue', got %v", queueStats["role"])
	}
}

func TestCacheManagerHealth(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping")
	}
	client.Close()

	cm, err := NewCacheManager(nil)
	if err != nil {
		t.Fatalf("NewCacheManager failed: %v", err)
	}
	defer cm.Close()

	ctx2 := context.Background()
	if err := cm.Health(ctx2); err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
}

func TestCacheManagerNoRedis(t *testing.T) {
	cfg := &CacheConfig{Addr: "localhost:99999", DialTimeout: 1 * time.Second}
	_, err := NewCacheManager(cfg)
	if err == nil {
		t.Fatal("expected error for invalid redis address")
	}
}

func TestRolePrefix(t *testing.T) {
	cfg := DefaultCacheConfig()
	cm := &CacheManager{
		config:       cfg,
		rolePrefixes: defaultRolePrefixes(),
	}

	if cm.prefixKey(RoleCache, "test") != "cf:cache:test" {
		t.Errorf("unexpected cache prefix: %s", cm.prefixKey(RoleCache, "test"))
	}
	if cm.prefixKey(RoleQueue, "test") != "cf:queue:test" {
		t.Errorf("unexpected queue prefix: %s", cm.prefixKey(RoleQueue, "test"))
	}
	if cm.prefixKey(RoleState, "test") != "cf:state:test" {
		t.Errorf("unexpected state prefix: %s", cm.prefixKey(RoleState, "test"))
	}
}
