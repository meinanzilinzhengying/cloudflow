// Package ratelimit 提供 HTTP API 速率限制功能
//
// 支持两种限流策略：
//  1. 内存令牌桶（按用户/租户）— 单机场景
//  2. Redis 滑动窗口（按 IP）— 分布式/DDoS 防护
package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 内存令牌桶限流器（按用户/租户）
// ============================================================================

// Limiter 令牌桶限流器接口
type Limiter interface {
	Allow(key string) bool
	AllowN(key string, n int) bool
}

// TokenBucket 内存令牌桶（按 key 隔离）
type TokenBucket struct {
	mu       sync.RWMutex
	buckets  map[string]*bucketEntry
	rate     float64 // 每秒补充令牌数
	burst    int     // 桶容量
	interval time.Duration
}

type bucketEntry struct {
	tokens  int
	max     int
	last    time.Time
	rateDur time.Duration
}

// NewTokenBucket 创建内存令牌桶限流器
// rate: 每秒允许的请求数
// burst: 突发容量（桶大小）
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	tb := &TokenBucket{
		buckets:  make(map[string]*bucketEntry),
		rate:     rate,
		burst:    burst,
		interval: time.Duration(float64(time.Second) / rate),
	}
	go tb.cleanupLoop()
	return tb
}

// Allow 检查 key 是否允许请求
func (tb *TokenBucket) Allow(key string) bool {
	return tb.AllowN(key, 1)
}

// AllowN 检查 key 是否允许 N 个请求
func (tb *TokenBucket) AllowN(key string, n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	b, ok := tb.buckets[key]
	if !ok {
		b = &bucketEntry{
			tokens:  tb.burst,
			max:     tb.burst,
			last:    now,
			rateDur: tb.interval,
		}
		tb.buckets[key] = b
	}

	elapsed := now.Sub(b.last)
	b.last = now
	if b.rateDur > 0 {
		newTokens := int(elapsed / b.rateDur)
		if newTokens > 0 {
			b.tokens += newTokens
			if b.tokens > b.max {
				b.tokens = b.max
			}
		}
	}

	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

// cleanupLoop 定期清理 5 分钟未使用的 bucket
func (tb *TokenBucket) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tb.mu.Lock()
		now := time.Now()
		for key, b := range tb.buckets {
			if now.Sub(b.last) > 5*time.Minute {
				delete(tb.buckets, key)
			}
		}
		tb.mu.Unlock()
	}
}

// ============================================================================
// 滑动窗口限流器（基于 Redis，用于 DDoS 防护）
// ============================================================================

// RedisLimiter Redis 滑动窗口限流器
type RedisLimiter struct {
	client RedisClient
	window time.Duration
	limit  int
}

// RedisClient Redis 客户端接口
type RedisClient interface {
	Incr(key string) (int64, error)
	Expire(key string, seconds int) error
	Get(key string) (string, error)
}

// NewRedisLimiter 创建 Redis 滑动窗口限流器
func NewRedisLimiter(client RedisClient, window time.Duration, limit int) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		window: window,
		limit:  limit,
	}
}

// Allow 检查 key 是否允许请求
func (rl *RedisLimiter) Allow(key string) bool {
	return rl.AllowN(key, 1)
}

// AllowN 检查 key 是否允许 N 个请求
func (rl *RedisLimiter) AllowN(key string, n int) bool {
	windowKey := fmt.Sprintf("ratelimit:%s", key)
	count, err := rl.client.Incr(windowKey)
	if err != nil {
		return true // Redis 故障时降级
	}
	if count == 1 {
		_ = rl.client.Expire(windowKey, int(rl.window.Seconds()))
	}
	if int(count) > rl.limit {
		return false
	}
	return true
}

// ============================================================================
// 辅助函数
// ============================================================================

// ExtractClientIP 从请求中提取客户端 IP
func ExtractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	xri := r.Header.Get("X-Real-Ip")
	if xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// MakeKey 组合限流 key
func MakeKey(parts ...string) string {
	return strings.Join(parts, ":")
}
