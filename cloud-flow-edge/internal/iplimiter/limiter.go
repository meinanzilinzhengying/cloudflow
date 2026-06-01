// Package iplimiter provides per-IP token bucket rate limiting for gRPC requests.
// It uses sync.Map for concurrent per-IP buckets and auto-cleans stale entries.
package iplimiter

import (
	"sync"
	"sync/atomic"
	"time"
)

// TokenBucket implements a simple counter-based token bucket for rate limiting.
type TokenBucket struct {
	tokens     int64
	maxTokens  int64
	lastRefill int64 // Unix nanoseconds of last refill
	qps        int64 // tokens added per second
}

// newTokenBucket creates a TokenBucket with the given QPS and burst size.
func newTokenBucket(qps, burst int64) *TokenBucket {
	now := time.Now().UnixNano()
	return &TokenBucket{
		tokens:     burst,
		maxTokens:  burst,
		lastRefill: now,
		qps:        qps,
	}
}

// allow attempts to consume one token. Returns true if allowed.
func (tb *TokenBucket) allow() bool {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&tb.lastRefill)
	elapsed := now - last

	// Calculate tokens to add based on elapsed time
	if elapsed > 0 && tb.qps > 0 {
		tokensToAdd := (elapsed * tb.qps) / int64(time.Second)
		if tokensToAdd > 0 {
			for {
				oldLast := atomic.LoadInt64(&tb.lastRefill)
				newLast := oldLast + (tokensToAdd * int64(time.Second) / tb.qps)
				if atomic.CompareAndSwapInt64(&tb.lastRefill, oldLast, newLast) {
					break
				}
			}
			for {
				current := atomic.LoadInt64(&tb.tokens)
				desired := current + tokensToAdd
				if desired > tb.maxTokens {
					desired = tb.maxTokens
				}
				if atomic.CompareAndSwapInt64(&tb.tokens, current, desired) {
					break
				}
			}
		}
	}

	// Try to consume one token
	for {
		current := atomic.LoadInt64(&tb.tokens)
		if current <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt64(&tb.tokens, current, current-1) {
			return true
		}
	}
}

// ipEntry holds the token bucket and last access time for a single IP.
type ipEntry struct {
	bucket    *TokenBucket
	lastSeen  int64 // Unix nanoseconds
}

// Stats holds statistics about the rate limiter.
type Stats struct {
	TrackedIPs    int       `json:"tracked_ips"`
	TopConsumers  []IPStat  `json:"top_consumers"`
}

// IPStat holds per-IP statistics.
type IPStat struct {
	IP          string `json:"ip"`
	Remaining   int64  `json:"remaining"`
	MaxTokens   int64  `json:"max_tokens"`
	LastSeen    string `json:"last_seen"`
}

// Config holds configuration for the rate limiter.
type Config struct {
	// MaxQPS is the maximum queries per second allowed per IP (default: 100)
	MaxQPS int64
	// BurstSize is the maximum burst of requests allowed per IP (default: MaxQPS)
	BurstSize int64
	// CleanupInterval is how often stale IPs are cleaned up (default: 1 minute)
	CleanupInterval time.Duration
	// StaleDuration is how long an IP can be inactive before cleanup (default: 10 minutes)
	StaleDuration time.Duration
}

// Limiter provides per-IP rate limiting using token buckets.
type Limiter struct {
	config Config
	ips    sync.Map // map[string]*ipEntry

	stopCh   chan struct{}
	stopped  int32
	mu       sync.Mutex
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxQPS:          100,
		BurstSize:       100,
		CleanupInterval: 1 * time.Minute,
		StaleDuration:   10 * time.Minute,
	}
}

// New creates a new per-IP rate limiter with the given configuration.
// If config.MaxQPS is 0, defaults are used.
func New(config Config) *Limiter {
	if config.MaxQPS <= 0 {
		config.MaxQPS = 100
	}
	if config.BurstSize <= 0 {
		config.BurstSize = config.MaxQPS
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 1 * time.Minute
	}
	if config.StaleDuration <= 0 {
		config.StaleDuration = 10 * time.Minute
	}

	l := &Limiter{
		config: config,
		stopCh: make(chan struct{}),
	}

	go l.cleanupLoop()

	return l
}

// getOrCreateBucket returns the token bucket for the given IP, creating one if needed.
func (l *Limiter) getOrCreateBucket(ip string) *TokenBucket {
	if val, ok := l.ips.Load(ip); ok {
		entry := val.(*ipEntry)
		atomic.StoreInt64(&entry.lastSeen, time.Now().UnixNano())
		return entry.bucket
	}

	bucket := newTokenBucket(l.config.MaxQPS, l.config.BurstSize)
	entry := &ipEntry{
		bucket:   bucket,
		lastSeen: time.Now().UnixNano(),
	}

	actual, loaded := l.ips.LoadOrStore(ip, entry)
	if loaded {
		// Another goroutine created it first; update lastSeen on the existing entry
		existing := actual.(*ipEntry)
		atomic.StoreInt64(&existing.lastSeen, time.Now().UnixNano())
		return existing.bucket
	}
	return bucket
}

// Allow checks whether a request from the given IP is allowed under the rate limit.
// Returns true if the request should be processed, false if it should be rejected.
func (l *Limiter) Allow(ip string) bool {
	bucket := l.getOrCreateBucket(ip)
	return bucket.allow()
}

// cleanupLoop periodically removes IPs that have been inactive for StaleDuration.
func (l *Limiter) cleanupLoop() {
	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stopCh:
			return
		}
	}
}

// cleanup removes all IP entries that have not been seen within StaleDuration.
func (l *Limiter) cleanup() {
	staleThreshold := time.Now().Add(-l.config.StaleDuration).UnixNano()

	l.ips.Range(func(key, value interface{}) bool {
		entry := value.(*ipEntry)
		lastSeen := atomic.LoadInt64(&entry.lastSeen)
		if lastSeen < staleThreshold {
			l.ips.Delete(key)
		}
		return true
	})
}

// GetStats returns statistics about the rate limiter including the number of
// tracked IPs and the top consumers by remaining token count.
func (l *Limiter) GetStats() Stats {
	var count int
	var consumers []IPStat

	now := time.Now()

	l.ips.Range(func(key, value interface{}) bool {
		count++
		ip := key.(string)
		entry := value.(*ipEntry)
		bucket := entry.bucket
		lastSeen := time.Unix(0, atomic.LoadInt64(&entry.lastSeen))

		consumers = append(consumers, IPStat{
			IP:        ip,
			Remaining: atomic.LoadInt64(&bucket.tokens),
			MaxTokens: bucket.maxTokens,
			LastSeen:  lastSeen.Format(time.RFC3339),
		})
		return true
	})

	// Sort by remaining tokens descending to show top consumers
	// (fewer remaining tokens = higher consumption)
	for i := 0; i < len(consumers)-1; i++ {
		for j := i + 1; j < len(consumers); j++ {
			if consumers[j].Remaining < consumers[i].Remaining {
				consumers[i], consumers[j] = consumers[j], consumers[i]
			}
		}
	}

	// Limit top consumers to 20 entries
	if len(consumers) > 20 {
		consumers = consumers[:20]
	}

	return Stats{
		TrackedIPs:   count,
		TopConsumers: consumers,
	}
}

// Stop shuts down the cleanup goroutine. The limiter can no longer be used after Stop.
func (l *Limiter) Stop() {
	if atomic.CompareAndSwapInt32(&l.stopped, 0, 1) {
		close(l.stopCh)
	}
}
