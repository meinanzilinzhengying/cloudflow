// Package cache 拓扑图缓存
//
// 设计目标:
//   - LRU + TTL 双重淘汰策略
//   - 多租户隔离
//   - 多图类型 (service/process/pod/namespace)
//   - 版本化快照 (避免读时修改)
//   - 百万级 edge 缓存支持
package cache

import (
	"context"
	"container/list"
	"fmt"
	"sync"
	"time"

	graph "cloud-flow/services/topology-engine/graph"
)

// CacheKey 缓存键，按租户 + 图类型隔离
type CacheKey struct {
	TenantID  string
	GraphType string // service / process / pod / namespace
}

// Key 返回序列化后的缓存键，格式: "tenantID:graphType"
func (k CacheKey) Key() string {
	return k.TenantID + ":" + k.GraphType
}

// CacheEntry 缓存条目，持有不可变快照
type CacheEntry struct {
	Key        CacheKey
	Snapshot   *graph.GraphSnapshot // 不可变快照，来自 graph.Snapshot()
	Version    uint64               // 版本号，用于快速失效判断
	CreatedAt  time.Time            // 写入时间
	LastAccess time.Time            // 最近访问时间 (LRU 依据)
	Size       int                  // 近似内存占用: nodes*256 + edges*128
}

// CacheConfig 缓存配置
type CacheConfig struct {
	MaxEntries      int           // 最大缓存条目数 (默认 1000)
	MaxMemoryMB     int           // 最大内存占用 MB (默认 512)
	DefaultTTL      time.Duration // 默认 TTL (默认 5 分钟)
	CleanupInterval time.Duration // 后台清理间隔 (默认 30 秒)
}

// DefaultCacheConfig 返回默认缓存配置
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxEntries:      1000,
		MaxMemoryMB:     512,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 30 * time.Second,
	}
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Entries     int     // 当前缓存条目数
	MemoryBytes int64   // 当前近似内存占用 (字节)
	Hits        uint64  // 缓存命中次数
	Misses      uint64  // 缓存未命中次数
	HitRate     float64 // 命中率 (0.0 ~ 1.0)
}

// Cache 拓扑图缓存，基于 LRU + TTL 双重淘汰策略
type Cache struct {
	mu             sync.RWMutex
	entries        map[string]*CacheEntry // key → entry
	order          *list.List             // 双向链表，维护 LRU 顺序 (front=最近, back=最久)
	entryMap       map[string]*list.Element // key → list element
	config         CacheConfig
	currentMemory  int64 // 近似内存占用 (字节)
	hits           uint64
	misses         uint64
}

// NewCache 创建一个新的拓扑图缓存实例。
// 如果 config 为 nil，则使用默认配置。
func NewCache(config *CacheConfig) *Cache {
	if config == nil {
		config = DefaultCacheConfig()
	}
	return &Cache{
		entries:  make(map[string]*CacheEntry),
		order:    list.New(),
		entryMap: make(map[string]*list.Element),
		config:   *config,
	}
}

// Get 获取缓存中的拓扑图快照。
// 命中时更新 LRU 顺序并返回 (snapshot, true)；
// 未命中或已过期时返回 (nil, false)。
func (c *Cache) Get(tenantID, graphType string) (*graph.GraphSnapshot, bool) {
	key := CacheKey{TenantID: tenantID, GraphType: graphType}
	keyStr := key.Key()

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entryMap[keyStr]
	if !ok {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*CacheEntry)

	// TTL 过期检查
	if c.isExpired(entry) {
		c.removeEntry(keyStr, elem)
		c.misses++
		return nil, false
	}

	// 命中: 移动到链表前端 (最近访问)
	c.order.MoveToFront(elem)
	entry.LastAccess = time.Now()
	c.hits++

	return entry.Snapshot, true
}

// Put 将拓扑图快照写入缓存。
// 如果缓存已满，会先触发 LRU 淘汰。
// 如果已存在同 key 的条目，则更新快照并重置 TTL。
func (c *Cache) Put(tenantID, graphType string, snapshot *graph.GraphSnapshot) {
	key := CacheKey{TenantID: tenantID, GraphType: graphType}
	keyStr := key.Key()

	now := time.Now()
	size := c.estimateSize(snapshot)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已存在，先移除旧条目
	if elem, ok := c.entryMap[keyStr]; ok {
		c.removeEntry(keyStr, elem)
	}

	// 淘汰直到满足容量限制
	for c.order.Len() >= c.config.MaxEntries ||
		c.currentMemory+int64(size) > int64(c.config.MaxMemoryMB)*1024*1024 {
		if !c.evict() {
			break // 无法继续淘汰
		}
	}

	entry := &CacheEntry{
		Key:        key,
		Snapshot:   snapshot,
		Version:    snapshot.Version,
		CreatedAt:  now,
		LastAccess: now,
		Size:       size,
	}

	elem := c.order.PushFront(entry)
	c.entries[keyStr] = entry
	c.entryMap[keyStr] = elem
	c.currentMemory += int64(size)
}

// Invalidate 使指定租户 + 图类型的缓存条目失效
func (c *Cache) Invalidate(tenantID, graphType string) {
	key := CacheKey{TenantID: tenantID, GraphType: graphType}
	keyStr := key.Key()

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entryMap[keyStr]; ok {
		c.removeEntry(keyStr, elem)
	}
}

// InvalidateTenant 使指定租户的所有缓存条目失效
func (c *Cache) InvalidateTenant(tenantID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := tenantID + ":"
	for keyStr, elem := range c.entryMap {
		if len(keyStr) >= len(prefix) && keyStr[:len(prefix)] == prefix {
			c.removeEntry(keyStr, elem)
		}
	}
}

// InvalidateAll 清空所有缓存条目
func (c *Cache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.order.Init()
	c.entryMap = make(map[string]*list.Element)
	c.currentMemory = 0
}

// Stats 返回缓存统计信息 (快照，无锁读取)
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Entries:     len(c.entries),
		MemoryBytes: c.currentMemory,
		Hits:        c.hits,
		Misses:      c.misses,
		HitRate:     hitRate,
	}
}

// StartCleanup 启动后台清理 goroutine，定期淘汰过期条目。
// 通过 ctx 取消来停止清理。
func (c *Cache) StartCleanup(ctx context.Context) {
	interval := c.config.CleanupInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.cleanupExpired()
			}
		}
	}()
}

// evict 执行一次 LRU 淘汰，移除链表尾部 (最久未访问) 的条目。
// 必须在持有写锁的情况下调用。
// 返回 true 表示成功淘汰一个条目，false 表示缓存为空。
func (c *Cache) evict() bool {
	// 链表尾部是最久未访问的条目
	back := c.order.Back()
	if back == nil {
		return false
	}

	entry := back.Value.(*CacheEntry)
	keyStr := entry.Key.Key()

	c.removeEntry(keyStr, back)
	return true
}

// isExpired 检查缓存条目是否已过期
func (c *Cache) isExpired(entry *CacheEntry) bool {
	ttl := c.config.DefaultTTL
	if ttl <= 0 {
		return false
	}
	return time.Since(entry.CreatedAt) > ttl
}

// removeEntry 从缓存中移除指定条目 (内部方法，必须持有写锁)
func (c *Cache) removeEntry(keyStr string, elem *list.Element) {
	entry, ok := elem.Value.(*CacheEntry)
	if !ok {
		return
	}

	c.currentMemory -= int64(entry.Size)
	if c.currentMemory < 0 {
		c.currentMemory = 0
	}

	c.order.Remove(elem)
	delete(c.entries, keyStr)
	delete(c.entryMap, keyStr)
}

// cleanupExpired 清理所有已过期的缓存条目
func (c *Cache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 从尾部向前遍历，移除过期条目
	var next *list.Element
	for elem := c.order.Back(); elem != nil; elem = next {
		next = elem.Prev() // 先保存前驱，因为当前 elem 可能被移除
		entry := elem.Value.(*CacheEntry)
		if c.isExpired(entry) {
			c.removeEntry(entry.Key.Key(), elem)
		}
	}
}

// estimateSize 估算 GraphSnapshot 的近似内存占用 (字节)
// 公式: nodes * 256 + edges * 128
func (c *Cache) estimateSize(snapshot *graph.GraphSnapshot) int {
	if snapshot == nil {
		return 0
	}
	return snapshot.NodeCount*256 + snapshot.EdgeCount*128
}

// String 返回缓存状态的调试字符串
func (s CacheStats) String() string {
	return fmt.Sprintf(
		"CacheStats{entries=%d, memory=%.1fMB, hits=%d, misses=%d, hitRate=%.2f%%}",
		s.Entries,
		float64(s.MemoryBytes)/(1024*1024),
		s.Hits,
		s.Misses,
		s.HitRate*100,
	)
}
