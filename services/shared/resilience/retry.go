package resilience

import (
	"context"
	"fmt"
	"time"
)

type RetryConfig struct {
	MaxAttempts       int
	InitialInterval   time.Duration
	MaxInterval       time.Duration
	Multiplier        float64
	Jitter            float64
	RetryableErrors  []error

	OnRetry func(attempt int, err error, nextInterval time.Duration)
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:     3,
		InitialInterval:  100 * time.Millisecond,
		MaxInterval:      30 * time.Second,
		Multiplier:       2.0,
		Jitter:           0.1,
		RetryableErrors:  nil,
		OnRetry:          nil,
	}
}

type RetryableFunc func() error
type RetryableFuncWithContext func(ctx context.Context) error

func IsRetryable(err error, cfg RetryConfig) bool {
	if err == nil {
		return false
	}

	for _, retryable := range cfg.RetryableErrors {
		if err == retryable {
			return true
		}
	}

	return true
}

func WithRetry(fn RetryableFunc, cfg RetryConfig) error {
	var lastErr error
	attempt := 0

	for attempt < cfg.MaxAttempts {
		attempt++

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if attempt >= cfg.MaxAttempts {
			break
		}

		if !IsRetryable(lastErr, cfg) {
			return lastErr
		}

		interval := CalculateBackoff(attempt, cfg)

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, lastErr, interval)
		}

		time.Sleep(interval)
	}

	return fmt.Errorf("retry exhausted after %d attempts: %w", attempt, lastErr)
}

func WithRetryContext(ctx context.Context, fn RetryableFuncWithContext, cfg RetryConfig) error {
	var lastErr error
	attempt := 0

	for attempt < cfg.MaxAttempts {
		attempt++

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err())
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		if attempt >= cfg.MaxAttempts {
			break
		}

		if !IsRetryable(lastErr, cfg) {
			return lastErr
		}

		interval := CalculateBackoff(attempt, cfg)

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, lastErr, interval)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
		case <-time.After(interval):
		}
	}

	return fmt.Errorf("retry exhausted after %d attempts: %w", attempt, lastErr)
}

func CalculateBackoff(attempt int, cfg RetryConfig) time.Duration {
	interval := float64(cfg.InitialInterval) * pow(cfg.Multiplier, float64(attempt-1))

	if interval > float64(cfg.MaxInterval) {
		interval = float64(cfg.MaxInterval)
	}

	if cfg.Jitter > 0 {
		jitterRange := interval * cfg.Jitter
		jitter := randomFloat64(-jitterRange, jitterRange)
		interval += jitter
	}

	if interval < 0 {
		interval = 0
	}

	return time.Duration(interval)
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	frac := exp - float64(int(exp))
	if frac > 0 {
		result *= (1 + frac*(base-1))
	}
	return result
}

func randomFloat64(min, max float64) float64 {
	return min + float64(int64(float64(time.Now().UnixNano())%int64(max-min))) + float64(time.Now().UnixNano()%1000)/1000.0
}

type RetryMetrics struct {
	TotalAttempts  int   `json:"total_attempts"`
	SuccessCount   int   `json:"success_count"`
	FailureCount   int   `json:"failure_count"`
	TotalRetries   int   `json:"total_retries"`
	TotalLatencyMs int64 `json:"total_latency_ms"`
}

type RetryStats struct {
	attempts      int
	successes     int
	failures      int
	retries       int
	totalLatencyNs int64
}

func (s *RetryStats) RecordAttempt() {
	s.attempts++
}

func (s *RetryStats) RecordSuccess(latency time.Duration) {
	s.successes++
	s.totalLatencyNs += latency.Nanoseconds()
}

func (s *RetryStats) RecordFailure(latency time.Duration) {
	s.failures++
	s.totalLatencyMs += latency.Nanoseconds() / 1e6
}

func (s *RetryStats) RecordRetry() {
	s.retries++
}

func (s *RetryStats) GetMetrics() RetryMetrics {
	return RetryMetrics{
		TotalAttempts:  s.attempts,
		SuccessCount:   s.successes,
		FailureCount:   s.failures,
		TotalRetries:   s.retries,
		TotalLatencyMs: s.totalLatencyNs / 1e6,
	}
}

func (s *RetryStats) Reset() {
	s.attempts = 0
	s.successes = 0
	s.failures = 0
	s.retries = 0
	s.totalLatencyNs = 0
}

type RetryExecutor struct {
	config RetryConfig
	stats *RetryStats
}

func NewRetryExecutor(config RetryConfig) *RetryExecutor {
	return &RetryExecutor{
		config: config,
		stats: &RetryStats{},
	}
}

func (e *RetryExecutor) Execute(fn RetryableFunc) error {
	e.stats.Reset()
	start := time.Now()

	var lastErr error
	attempt := 0

	for attempt < e.config.MaxAttempts {
		attempt++
		e.stats.RecordAttempt()

		lastErr = fn()
		if lastErr == nil {
			e.stats.RecordSuccess(time.Since(start))
			return nil
		}

		if attempt >= e.config.MaxAttempts {
			break
		}

		if !IsRetryable(lastErr, e.config) {
			e.stats.RecordFailure(time.Since(start))
			return lastErr
		}

		e.stats.RecordRetry()
		interval := CalculateBackoff(attempt, e.config)

		if e.config.OnRetry != nil {
			e.config.OnRetry(attempt, lastErr, interval)
		}

		time.Sleep(interval)
	}

	e.stats.RecordFailure(time.Since(start))
	return fmt.Errorf("retry exhausted after %d attempts: %w", attempt, lastErr)
}

func (e *RetryExecutor) ExecuteContext(ctx context.Context, fn RetryableFuncWithContext) error {
	e.stats.Reset()
	start := time.Now()

	var lastErr error
	attempt := 0

	for attempt < e.config.MaxAttempts {
		attempt++
		e.stats.RecordAttempt()

		select {
		case <-ctx.Done():
			e.stats.RecordFailure(time.Since(start))
			return fmt.Errorf("context cancelled after %d attempts: %w", attempt, ctx.Err())
		default:
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			e.stats.RecordSuccess(time.Since(start))
			return nil
		}

		if attempt >= e.config.MaxAttempts {
			break
		}

		if !IsRetryable(lastErr, e.config) {
			e.stats.RecordFailure(time.Since(start))
			return lastErr
		}

		e.stats.RecordRetry()
		interval := CalculateBackoff(attempt, e.config)

		if e.config.OnRetry != nil {
			e.config.OnRetry(attempt, lastErr, interval)
		}

		select {
		case <-ctx.Done():
			e.stats.RecordFailure(time.Since(start))
			return fmt.Errorf("context cancelled during retry backoff: %w", ctx.Err())
		case <-time.After(interval):
		}
	}

	e.stats.RecordFailure(time.Since(start))
	return fmt.Errorf("retry exhausted after %d attempts: %w", attempt, lastErr)
}

func (e *RetryExecutor) GetMetrics() RetryMetrics {
	return e.stats.GetMetrics()
}

func (e *RetryExecutor) GetStats() *RetryStats {
	return e.stats
}
