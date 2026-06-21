//go:build linux

package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ============================================================================
// Redis 职责分离管理器
// 将 Redis 的缓存、队列、状态存储职责分离到不同前缀/数据库
// ============================================================================

// CacheRole Redis 职责角色
type CacheRole string

const (
	RoleCache  CacheRole = "cache"  // 缓存：短期TTL，高频读取
	RoleQueue  CacheRole = "queue"  // 队列：LPush/RPush，消费后删除
	RoleState  CacheRole = "state"  // 状态：长期持久，分布式锁/会话
)

// CacheConfig Redis 配置
type CacheConfig struct {
	Addr         string
	Password     string
	DB           int           // 默认数据库
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultCacheConfig 返回默认缓存配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Addr:         "localhost:6379",
		DB:           0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}

// CacheManager Redis 职责分离管理器
type CacheManager struct {
	client *redis.Client
	config *CacheConfig
	mu     sync.RWMutex

	// 各角色使用不同 key 前缀隔离
	rolePrefixes map[CacheRole]string
}

// NewCacheManager 创建 Redis 职责分离管理器
func NewCacheManager(cfg *CacheConfig) (*CacheManager, error) {
	if cfg == nil {
		cfg = DefaultCacheConfig()
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connect failed: %w", err)
	}

	cm := &CacheManager{
		client:       client,
		config:       cfg,
		rolePrefixes: defaultRolePrefixes(),
	}
	return cm, nil
}

func defaultRolePrefixes() map[CacheRole]string {
	return map[CacheRole]string{
		RoleCache: "cf:cache:",
		RoleQueue: "cf:queue:",
		RoleState: "cf:state:",
	}
}

// SetRolePrefix 设置角色前缀
func (cm *CacheManager) SetRolePrefix(role CacheRole, prefix string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.rolePrefixes[role] = prefix
}

// prefixKey 生成带角色前缀的 key
func (cm *CacheManager) prefixKey(role CacheRole, key string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	prefix := cm.rolePrefixes[role]
	if prefix == "" {
		prefix = string(role) + ":"
	}
	return prefix + key
}

// ============================================================================
// 缓存操作 (Cache Role)
// 短期 TTL，高频读取
// ============================================================================

// CacheGet 缓存读取
func (cm *CacheManager) CacheGet(ctx context.Context, key string) (string, error) {
	return cm.client.Get(ctx, cm.prefixKey(RoleCache, key)).Result()
}

// CacheSet 缓存写入
func (cm *CacheManager) CacheSet(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return cm.client.Set(ctx, cm.prefixKey(RoleCache, key), value, ttl).Err()
}

// CacheDelete 缓存删除
func (cm *CacheManager) CacheDelete(ctx context.Context, key string) error {
	return cm.client.Del(ctx, cm.prefixKey(RoleCache, key)).Err()
}

// CacheSetNX 缓存条件写入（仅当 key 不存在时）
func (cm *CacheManager) CacheSetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	return cm.client.SetNX(ctx, cm.prefixKey(RoleCache, key), value, ttl).Result()
}

// CacheBatchGet 批量缓存读取
func (cm *CacheManager) CacheBatchGet(ctx context.Context, keys []string) (map[string]string, error) {
	prefixedKeys := make([]string, len(keys))
	for i, k := range keys {
		prefixedKeys[i] = cm.prefixKey(RoleCache, k)
	}
	result, err := cm.client.MGet(ctx, prefixedKeys...).Result()
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(keys))
	for i, v := range result {
		if v != nil {
			m[keys[i]] = fmt.Sprint(v)
		}
	}
	return m, nil
}

// CacheBatchSet 批量缓存写入
func (cm *CacheManager) CacheBatchSet(ctx context.Context, kv map[string]interface{}, ttl time.Duration) error {
	pipe := cm.client.Pipeline()
	for k, v := range kv {
		pipe.Set(ctx, cm.prefixKey(RoleCache, k), v, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// CacheStats 缓存统计
func (cm *CacheManager) CacheStats(ctx context.Context) (map[string]interface{}, error) {
	prefix := cm.rolePrefixes[RoleCache]
	iter := cm.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	count := 0
	for iter.Next(ctx) {
		count++
	}
	return map[string]interface{}{
		"prefix": prefix,
		"keys":   count,
		"role":   "cache",
	}, iter.Err()
}

// ============================================================================
// 队列操作 (Queue Role)
// 消息队列，LPush/RPush，消费后删除
// ============================================================================

// QueuePush 入队
func (cm *CacheManager) QueuePush(ctx context.Context, queueName string, values ...interface{}) error {
	return cm.client.LPush(ctx, cm.prefixKey(RoleQueue, queueName), values...).Err()
}

// QueuePop 出队（阻塞弹出）
func (cm *CacheManager) QueuePop(ctx context.Context, queueName string, timeout time.Duration) (string, error) {
	result, err := cm.client.BRPop(ctx, timeout, cm.prefixKey(RoleQueue, queueName)).Result()
	if err != nil {
		return "", err
	}
	if len(result) >= 2 {
		return result[1], nil
	}
	return "", fmt.Errorf("empty result")
}

// QueueLen 队列长度
func (cm *CacheManager) QueueLen(ctx context.Context, queueName string) (int64, error) {
	return cm.client.LLen(ctx, cm.prefixKey(RoleQueue, queueName)).Result()
}

// QueuePeek 查看队首（不删除）
func (cm *CacheManager) QueuePeek(ctx context.Context, queueName string) (string, error) {
	result, err := cm.client.LIndex(ctx, cm.prefixKey(RoleQueue, queueName), -1).Result()
	if err == redis.Nil {
		return "", nil
	}
	return result, err
}

// QueueRange 查看队列范围
func (cm *CacheManager) QueueRange(ctx context.Context, queueName string, start, stop int64) ([]string, error) {
	return cm.client.LRange(ctx, cm.prefixKey(RoleQueue, queueName), start, stop).Result()
}

// QueueStats 队列统计
func (cm *CacheManager) QueueStats(ctx context.Context) (map[string]interface{}, error) {
	prefix := cm.rolePrefixes[RoleQueue]
	iter := cm.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	queues := make(map[string]int64)
	for iter.Next(ctx) {
		key := iter.Val()
		len, _ := cm.client.LLen(ctx, key).Result()
		queues[key] = len
	}
	return map[string]interface{}{
		"prefix":  prefix,
		"queues":  queues,
		"role":    "queue",
	}, iter.Err()
}

// ============================================================================
// 状态存储 (State Role)
// 长期持久，分布式锁/会话/配置
// ============================================================================

// StateGet 状态读取
func (cm *CacheManager) StateGet(ctx context.Context, key string) (string, error) {
	return cm.client.Get(ctx, cm.prefixKey(RoleState, key)).Result()
}

// StateSet 状态写入（长期持久，不设置 TTL 或设置很长的 TTL）
func (cm *CacheManager) StateSet(ctx context.Context, key string, value interface{}) error {
	return cm.client.Set(ctx, cm.prefixKey(RoleState, key), value, 0).Err()
}

// StateSetWithTTL 状态写入带 TTL
func (cm *CacheManager) StateSetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return cm.client.Set(ctx, cm.prefixKey(RoleState, key), value, ttl).Err()
}

// StateDelete 状态删除
func (cm *CacheManager) StateDelete(ctx context.Context, key string) error {
	return cm.client.Del(ctx, cm.prefixKey(RoleState, key)).Err()
}

// StateExists 状态是否存在
func (cm *CacheManager) StateExists(ctx context.Context, key string) (bool, error) {
	n, err := cm.client.Exists(ctx, cm.prefixKey(RoleState, key)).Result()
	return n > 0, err
}

// AcquireLock 分布式锁获取
func (cm *CacheManager) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	return cm.client.SetNX(ctx, cm.prefixKey(RoleState, "lock:"+lockKey), "1", ttl).Result()
}

// ReleaseLock 分布式锁释放
func (cm *CacheManager) ReleaseLock(ctx context.Context, lockKey string) error {
	return cm.client.Del(ctx, cm.prefixKey(RoleState, "lock:"+lockKey)).Err()
}

// StateStats 状态统计
func (cm *CacheManager) StateStats(ctx context.Context) (map[string]interface{}, error) {
	prefix := cm.rolePrefixes[RoleState]
	iter := cm.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	count := 0
	for iter.Next(ctx) {
		count++
	}
	return map[string]interface{}{
		"prefix": prefix,
		"keys":   count,
		"role":   "state",
	}, iter.Err()
}

// ============================================================================
// 全局管理
// ============================================================================

// Health 健康检查
func (cm *CacheManager) Health(ctx context.Context) error {
	return cm.client.Ping(ctx).Err()
}

// Close 关闭连接
func (cm *CacheManager) Close() error {
	return cm.client.Close()
}

// Stats 获取所有角色统计
func (cm *CacheManager) Stats(ctx context.Context) (map[string]interface{}, error) {
	cacheStats, _ := cm.CacheStats(ctx)
	queueStats, _ := cm.QueueStats(ctx)
	stateStats, _ := cm.StateStats(ctx)
	info := cm.client.Info(ctx, "memory").Val()

	return map[string]interface{}{
		"cache":  cacheStats,
		"queue":  queueStats,
		"state":  stateStats,
		"memory": info,
	}, nil
}

// FlushRole 清空指定角色的数据
func (cm *CacheManager) FlushRole(ctx context.Context, role CacheRole) error {
	prefix := cm.rolePrefixes[role]
	iter := cm.client.Scan(ctx, 0, prefix+"*", 0).Iterator()
	pipe := cm.client.Pipeline()
	for iter.Next(ctx) {
		pipe.Del(ctx, iter.Val())
	}
	_, err := pipe.Exec(ctx)
	return err
}
