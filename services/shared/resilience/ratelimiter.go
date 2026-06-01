package resilience

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiterConfig struct {
	Name string

	RequestsPerSecond float64
	BurstSize         int

	RequestsPerMinute float64
	RequestsPerHour   float64

	BlockOnLimitExceeded bool

	OnLimitExceeded func(limitType string)
}

func DefaultRateLimiterConfig(name string) RateLimiterConfig {
	return RateLimiterConfig{
		Name:                name,
		RequestsPerSecond:   100,
		BurstSize:           200,
		RequestsPerMinute:   0,
		RequestsPerHour:     0,
		BlockOnLimitExceeded: true,
		OnLimitExceeded:     nil,
	}
}

type RateLimiterMetrics struct {
	Name              string  `json:"name"`
	TotalRequests     int64   `json:"total_requests"`
	AllowedRequests   int64   `json:"allowed_requests"`
	RejectedRequests  int64   `json:"rejected_requests"`
	BlockedRequests   int64   `json:"blocked_requests"`
	CurrentRPS        float64 `json:"current_rps"`
	RejectionRate     float64 `json:"rejection_rate"`
	LastRejectionTime string  `json:"last_rejection_time,omitempty"`
	LastResetTime     string  `json:"last_reset_time"`
	CurrentBurstUsed  int     `json:"current_burst_used"`
}

type RateLimiter struct {
	config RateLimiterConfig

	limiter *rate.Limiter

	totalRequests   int64
	allowedRequests int64
	rejectedRequests int64
	blockedRequests  int64
	lastRejection   int64
	lastReset       int64

	burstUsed int64

	mu sync.RWMutex
}

func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if config.Name == "" {
		config.Name = "default"
	}
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 100
	}
	if config.BurstSize <= 0 {
		config.BurstSize = int(config.RequestsPerSecond * 2)
	}

	rl := &RateLimiter{
		config:     config,
		limiter:    rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize),
		totalRequests: 0,
		allowedRequests: 0,
		rejectedRequests: 0,
		blockedRequests: 0,
		lastReset: time.Now().UnixNano(),
	}

	return rl
}

func (rl *RateLimiter) Allow() bool {
	return rl.AllowN(1)
}

func (rl *RateLimiter) AllowN(n int) bool {
	atomic.AddInt64(&rl.totalRequests, 1)

	if rl.config.BlockOnLimitExceeded {
		if rl.limiter.AllowN(time.Now(), 1) {
			atomic.AddInt64(&rl.allowedRequests, 1)
			return true
		}
		atomic.AddInt64(&rl.rejectedRequests, 1)
		atomic.StoreInt64(&rl.lastRejection, time.Now().UnixNano())

		if rl.config.OnLimitExceeded != nil {
			go rl.config.OnLimitExceeded("per_second")
		}
		return false
	}

	if rl.limiter.AllowN(time.Now(), 1) {
		atomic.AddInt64(&rl.allowedRequests, 1)
		return true
	}

	atomic.AddInt64(&rl.rejectedRequests, 1)
	atomic.StoreInt64(&rl.lastRejection, time.Now().UnixNano())

	if rl.config.OnLimitExceeded != nil {
		go rl.config.OnLimitExceeded("per_second")
	}

	return false
}

func (rl *RateLimiter) AllowContext(ctx context.Context) error {
	return rl.AllowContextN(ctx, 1)
}

func (rl *RateLimiter) AllowContextN(ctx context.Context, n int) error {
	atomic.AddInt64(&rl.totalRequests, 1)

	err := rl.limiter.WaitN(ctx, n)
	if err != nil {
		atomic.AddInt64(&rl.rejectedRequests, 1)
		atomic.StoreInt64(&rl.lastRejection, time.Now().UnixNano())

		if rl.config.OnLimitExceeded != nil {
			go rl.config.OnLimitExceeded("per_second")
		}
		return err
	}

	atomic.AddInt64(&rl.allowedRequests, 1)
	return nil
}

func (rl *RateLimiter) Reserve() *rate.Reservation {
	atomic.AddInt64(&rl.totalRequests, 1)

	reservation := rl.limiter.Reserve()
	if !reservation.OK() {
		atomic.AddInt64(&rl.rejectedRequests, 1)
		atomic.StoreInt64(&rl.lastRejection, time.Now().UnixNano())

		if rl.config.OnLimitExceeded != nil {
			go rl.config.OnLimitExceeded("per_second")
		}
	}

	atomic.AddInt64(&rl.allowedRequests, 1)
	return reservation
}

func (rl *RateLimiter) SetLimit(newLimit rate.Limit) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiter.SetLimit(newLimit)
}

func (rl *RateLimiter) SetBurst(newBurst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiter.SetBurst(newBurst)
}

func (rl *RateLimiter) SetLimitAt(now time.Time, newLimit rate.Limit) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiter.SetLimitAt(now, newLimit)
}

func (rl *RateLimiter) SetBurstAt(now time.Time, newBurst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limiter.SetBurstAt(now, newBurst)
}

func (rl *RateLimiter) GetMetrics() RateLimiterMetrics {
	total := atomic.LoadInt64(&rl.totalRequests)
	allowed := atomic.LoadInt64(&rl.allowedRequests)
	rejected := atomic.LoadInt64(&rl.rejectedRequests)
	lastRej := atomic.LoadInt64(&rl.lastRejection)
	lastRst := atomic.LoadInt64(&rl.lastReset)

	var rejectionRate float64
	if total > 0 {
		rejectionRate = mathRound(float64(rejected)/float64(total)*10000) / 100
	}

	metrics := RateLimiterMetrics{
		Name:             rl.config.Name,
		TotalRequests:    total,
		AllowedRequests:  allowed,
		RejectedRequests: rejected,
		BlockedRequests:  atomic.LoadInt64(&rl.blockedRequests),
		CurrentRPS:       rate.Limit(atomic.LoadInt64(&rl.totalRequests)) / rate.Limit(time.Since(time.Unix(0, lastRst)).Seconds()),
		RejectionRate:    rejectionRate,
		LastResetTime:    time.Unix(0, lastRst).Format(time.RFC3339),
	}

	if lastRej > 0 {
		metrics.LastRejectionTime = time.Unix(0, lastRej).Format(time.RFC3339)
	}

	return metrics
}

func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	atomic.StoreInt64(&rl.totalRequests, 0)
	atomic.StoreInt64(&rl.allowedRequests, 0)
	atomic.StoreInt64(&rl.rejectedRequests, 0)
	atomic.StoreInt64(&rl.blockedRequests, 0)
	atomic.StoreInt64(&rl.lastRejection, 0)
	atomic.StoreInt64(&rl.lastReset, time.Now().UnixNano())

	rl.limiter = rate.NewLimiter(rate.Limit(rl.config.RequestsPerSecond), rl.config.BurstSize)
}

func (rl *RateLimiter) IsAvailable() bool {
	return rl.Allow()
}

type MultiTierRateLimiter struct {
	config RateLimiterConfig

	perSecond  *RateLimiter
	perMinute  *RateLimiter
	perHour    *RateLimiter

	mu sync.RWMutex
}

func NewMultiTierRateLimiter(config RateLimiterConfig) *MultiTierRateLimiter {
	mt := &MultiTierRateLimiter{
		config:    config,
		perSecond: NewRateLimiter(RateLimiterConfig{
			Name:                  fmt.Sprintf("%s-per-second", config.Name),
			RequestsPerSecond:     config.RequestsPerSecond,
			BurstSize:             config.BurstSize,
			BlockOnLimitExceeded:  config.BlockOnLimitExceeded,
			OnLimitExceeded:       nil,
		}),
	}

	if config.RequestsPerMinute > 0 {
		mt.perMinute = NewRateLimiter(RateLimiterConfig{
			Name:                 fmt.Sprintf("%s-per-minute", config.Name),
			RequestsPerSecond:    config.RequestsPerMinute / 60,
			BurstSize:            int(config.RequestsPerMinute / 60 * 2),
			BlockOnLimitExceeded: config.BlockOnLimitExceeded,
			OnLimitExceeded:      nil,
		})
	}

	if config.RequestsPerHour > 0 {
		mt.perHour = NewRateLimiter(RateLimiterConfig{
			Name:                 fmt.Sprintf("%s-per-hour", config.Name),
			RequestsPerSecond:    config.RequestsPerHour / 3600,
			BurstSize:           int(config.RequestsPerHour / 3600 * 2),
			BlockOnLimitExceeded: config.BlockOnLimitExceeded,
			OnLimitExceeded:     nil,
		})
	}

	return mt
}

func (mt *MultiTierRateLimiter) Allow() bool {
	if !mt.perSecond.Allow() {
		if mt.config.OnLimitExceeded != nil {
			go mt.config.OnLimitExceeded("per_second")
		}
		return false
	}

	if mt.perMinute != nil && !mt.perMinute.Allow() {
		if mt.config.OnLimitExceeded != nil {
			go mt.config.OnLimitExceeded("per_minute")
		}
		return false
	}

	if mt.perHour != nil && !mt.perHour.Allow() {
		if mt.config.OnLimitExceeded != nil {
			go mt.config.OnLimitExceeded("per_hour")
		}
		return false
	}

	return true
}

func (mt *MultiTierRateLimiter) AllowContext(ctx context.Context) error {
	if err := mt.perSecond.AllowContext(ctx); err != nil {
		if mt.config.OnLimitExceeded != nil {
			go mt.config.OnLimitExceeded("per_second")
		}
		return fmt.Errorf("per-second limit exceeded: %w", err)
	}

	if mt.perMinute != nil {
		if err := mt.perMinute.AllowContext(ctx); err != nil {
			if mt.config.OnLimitExceeded != nil {
				go mt.config.OnLimitExceeded("per_minute")
			}
			return fmt.Errorf("per-minute limit exceeded: %w", err)
		}
	}

	if mt.perHour != nil {
		if err := mt.perHour.AllowContext(ctx); err != nil {
			if mt.config.OnLimitExceeded != nil {
				go mt.config.OnLimitExceeded("per_hour")
			}
			return fmt.Errorf("per-hour limit exceeded: %w", err)
		}
	}

	return nil
}

func (mt *MultiTierRateLimiter) GetMetrics() map[string]RateLimiterMetrics {
	metrics := make(map[string]RateLimiterMetrics)
	metrics["per_second"] = mt.perSecond.GetMetrics()

	if mt.perMinute != nil {
		metrics["per_minute"] = mt.perMinute.GetMetrics()
	}
	if mt.perHour != nil {
		metrics["per_hour"] = mt.perHour.GetMetrics()
	}

	return metrics
}

func (mt *MultiTierRateLimiter) Reset() {
	mt.perSecond.Reset()
	if mt.perMinute != nil {
		mt.perMinute.Reset()
	}
	if mt.perHour != nil {
		mt.perHour.Reset()
	}
}

func mathRound(x float64) float64 {
	return float64(int(x + 0.5))
}
