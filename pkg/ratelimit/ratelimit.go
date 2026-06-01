package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket 令牌桶限流器
type TokenBucket struct {
	mu         sync.Mutex
	bucketSize int
	refillRate int // 每秒填充的令牌数
	tokens     int
	lastRefill time.Time
}

// NewTokenBucket 创建令牌桶
func NewTokenBucket(bucketSize, refillRate int) *TokenBucket {
	return &TokenBucket{
		bucketSize: bucketSize,
		refillRate: refillRate,
		tokens:     bucketSize,
		lastRefill: time.Now(),
	}
}

// Allow 尝试消费一个令牌
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	tb.lastRefill = now

	// 按时间比例填充令牌
	refillTokens := int(elapsed.Seconds()) * tb.refillRate
	if refillTokens > 0 {
		tb.tokens += refillTokens
		if tb.tokens > tb.bucketSize {
			tb.tokens = tb.bucketSize
		}
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// IPRateLimiter IP 级别限流器
type IPRateLimiter struct {
	mu              sync.RWMutex
	limiters        map[string]*TokenBucket
	defaultBucketSize int
	defaultRefillRate int
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

// NewIPRateLimiter 创建 IP 级别限流器
func NewIPRateLimiter(defaultBucketSize, defaultRefillRate int, cleanupInterval time.Duration) *IPRateLimiter {
	rl := &IPRateLimiter{
		limiters:        make(map[string]*TokenBucket),
		defaultBucketSize: defaultBucketSize,
		defaultRefillRate: defaultRefillRate,
		cleanupInterval: cleanupInterval,
		lastCleanup:     time.Now(),
	}
	return rl
}

// getLimiter 获取或创建 IP 对应的限流器
func (rl *IPRateLimiter) getLimiter(ip string) *TokenBucket {
	rl.mu.RLock()
	limiter, exists := rl.limiters[ip]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// 再次检查，避免竞态条件
		if limiter, exists = rl.limiters[ip]; !exists {
			limiter = NewTokenBucket(rl.defaultBucketSize, rl.defaultRefillRate)
			rl.limiters[ip] = limiter
			// 检查是否需要清理
			rl.cleanupIfNeeded()
		}
		rl.mu.Unlock()
	}
	return limiter
}

// AllowIP 检查 IP 是否允许请求
func (rl *IPRateLimiter) AllowIP(ip string) bool {
	limiter := rl.getLimiter(ip)
	return limiter.Allow()
}

// cleanupIfNeeded 清理长时间未使用的限流器
func (rl *IPRateLimiter) cleanupIfNeeded() {
	if time.Since(rl.lastCleanup) < rl.cleanupInterval {
		return
	}

	// 简单清理：删除所有限流器，下次请求时重新创建
	// 更精细的清理可以记录最后使用时间
	rl.limiters = make(map[string]*TokenBucket)
	rl.lastCleanup = time.Now()
}

// SetIPLimiter 为特定 IP 设置自定义限流器配置
func (rl *IPRateLimiter) SetIPLimiter(ip string, bucketSize, refillRate int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiters[ip] = NewTokenBucket(bucketSize, refillRate)
}

// RemoveIPLimiter 移除特定 IP 的限流器
func (rl *IPRateLimiter) RemoveIPLimiter(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limiters, ip)
}

// ClearAll 清空所有限流器
func (rl *IPRateLimiter) ClearAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiters = make(map[string]*TokenBucket)
}

// MultiLevelRateLimiter 多级限流器（支持不同接口不同策略）
type MultiLevelRateLimiter struct {
	mu          sync.RWMutex
	limiters    map[string]*IPRateLimiter
}

// NewMultiLevelRateLimiter 创建多级限流器
func NewMultiLevelRateLimiter() *MultiLevelRateLimiter {
	return &MultiLevelRateLimiter{
		limiters: make(map[string]*IPRateLimiter),
	}
}

// RegisterLevel 注册一个限流级别
func (m *MultiLevelRateLimiter) RegisterLevel(name string, bucketSize, refillRate int, cleanupInterval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limiters[name] = NewIPRateLimiter(bucketSize, refillRate, cleanupInterval)
}

// Allow 检查特定级别和 IP 是否允许请求
func (m *MultiLevelRateLimiter) Allow(level, ip string) bool {
	m.mu.RLock()
	limiter, exists := m.limiters[level]
	m.mu.RUnlock()

	if !exists {
		// 默认允许
		return true
	}
	return limiter.AllowIP(ip)
}

// GetLimiter 获取特定级别的限流器
func (m *MultiLevelRateLimiter) GetLimiter(level string) *IPRateLimiter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.limiters[level]
}
