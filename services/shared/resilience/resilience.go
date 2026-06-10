package resilience

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ResilientClient struct {
	circuitBreaker *CircuitBreaker
	rateLimiter    *RateLimiter
	retry          *RetryExecutor
	bulkhead       *Bulkhead
	fallback       *FallbackHandler
	degradation    *DegradationHandler

	mu sync.RWMutex
}

type ResilientClientConfig struct {
	CircuitBreaker CircuitBreakerConfig
	RateLimiter    RateLimiterConfig
	Retry          RetryConfig
	Bulkhead       BulkheadConfig
	Fallback       FallbackConfig

	EnableCircuitBreaker bool
	EnableRateLimiter    bool
	EnableRetry          bool
	EnableBulkhead       bool
	EnableFallback       bool
}

func DefaultResilientClientConfig(name string) ResilientClientConfig {
	return ResilientClientConfig{
		CircuitBreaker: DefaultCircuitBreakerConfig(name),
		RateLimiter:    DefaultRateLimiterConfig(name),
		Retry:          DefaultRetryConfig(),
		Bulkhead:       DefaultBulkheadConfig(),
		Fallback:       DefaultFallbackConfig(),

		EnableCircuitBreaker: true,
		EnableRateLimiter:    true,
		EnableRetry:          true,
		EnableBulkhead:       true,
		EnableFallback:       true,
	}
}

func NewResilientClient(config ResilientClientConfig) *ResilientClient {
	rc := &ResilientClient{}

	if config.EnableCircuitBreaker {
		rc.circuitBreaker = NewCircuitBreaker(config.CircuitBreaker)
	}

	if config.EnableRateLimiter {
		config.RateLimiter.OnLimitExceeded = func(limitType string) {
			fmt.Printf("Rate limit exceeded: %s\n", limitType)
		}
		rc.rateLimiter = NewRateLimiter(config.RateLimiter)
	}

	if config.EnableRetry {
		config.Retry.OnRetry = func(attempt int, err error, nextInterval time.Duration) {
			fmt.Printf("Retry attempt %d failed: %v, next retry in %v\n", attempt, err, nextInterval)
		}
		rc.retry = NewRetryExecutor(config.Retry)
	}

	if config.EnableBulkhead {
		rc.bulkhead = NewBulkhead(config.Bulkhead)
	}

	if config.EnableFallback {
		rc.fallback = NewFallbackHandler(config.Fallback)
	}

	rc.degradation = NewDegradationHandler()

	return rc
}

func (rc *ResilientClient) Execute(fn func() error) error {
	return rc.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		return fn()
	})
}

func (rc *ResilientClient) ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	start := time.Now()

	if rc.rateLimiter != nil && !rc.rateLimiter.Allow() {
		if rc.fallback != nil {
			return rc.executeFallback("rate_limit", fmt.Errorf("rate limit exceeded"))
		}
		return fmt.Errorf("rate limit exceeded")
	}

	if rc.circuitBreaker != nil && !rc.circuitBreaker.Allow() {
		if rc.fallback != nil {
			return rc.executeFallback("circuit_open", fmt.Errorf("circuit breaker open"))
		}
		return fmt.Errorf("circuit breaker open")
	}

	if rc.bulkhead != nil && !rc.bulkhead.IsAvailable() {
		if rc.fallback != nil {
			return rc.executeFallback("bulkhead_full", fmt.Errorf("bulkhead at capacity"))
		}
		return fmt.Errorf("bulkhead at capacity")
	}

	var lastErr error
	executeFn := func(ctx context.Context) error {
		if rc.bulkhead != nil {
			err := rc.bulkhead.ExecuteWithContext(ctx, func(ctx context.Context) error {
				return fn(ctx)
			})
			if err != nil {
				lastErr = err
				return err
			}
			return nil
		}
		return fn(ctx)
	}

	if rc.retry != nil {
		err := rc.retry.ExecuteContext(ctx, executeFn)
		if err != nil {
			if rc.circuitBreaker != nil {
				rc.circuitBreaker.RecordFailure()
			}
			if rc.fallback != nil {
				return rc.executeFallback("retry_exhausted", err)
			}
			return err
		}
	} else {
		err := executeFn(ctx)
		if err != nil {
			lastErr = err
			if rc.circuitBreaker != nil {
				rc.circuitBreaker.RecordFailure()
			}
			if rc.fallback != nil {
				return rc.executeFallback("execution_failed", err)
			}
			return err
		}
	}

	if rc.circuitBreaker != nil {
		rc.circuitBreaker.RecordSuccessWithLatency(time.Since(start))
	}

	return nil
}

func (rc *ResilientClient) executeFallback(name string, err error) error {
	result, fErr := rc.fallback.Execute(name, err)
	if fErr != nil {
		return err
	}

	if result == nil {
		return err
	}

	if retErr, ok := result.(error); ok {
		return retErr
	}

	return nil
}

func (rc *ResilientClient) RegisterFallback(name string, fn FallbackFunc) {
	if rc.fallback != nil {
		rc.fallback.Register(name, fn)
	}
}

func (rc *ResilientClient) GetCircuitBreaker() *CircuitBreaker {
	return rc.circuitBreaker
}

func (rc *ResilientClient) GetRateLimiter() *RateLimiter {
	return rc.rateLimiter
}

func (rc *ResilientClient) GetBulkhead() *Bulkhead {
	return rc.bulkhead
}

func (rc *ResilientClient) GetDegradation() *DegradationHandler {
	return rc.degradation
}

func (rc *ResilientClient) GetMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})

	if rc.circuitBreaker != nil {
		metrics["circuit_breaker"] = rc.circuitBreaker.GetMetrics()
	}

	if rc.rateLimiter != nil {
		metrics["rate_limiter"] = rc.rateLimiter.GetMetrics()
	}

	if rc.retry != nil {
		metrics["retry"] = rc.retry.GetMetrics()
	}

	if rc.bulkhead != nil {
		metrics["bulkhead"] = BulkheadMetrics{
			MaxConcurrent: rc.bulkhead.GetMaxConcurrent(),
			ActiveCount:   rc.bulkhead.GetActiveCount(),
			Utilization:   rc.bulkhead.GetUtilization(),
			IsAvailable:   rc.bulkhead.IsAvailable(),
			MaxWaitTimeMs: int64(rc.bulkhead.maxWaitTime / time.Millisecond),
		}
	}

	if rc.degradation != nil {
		metrics["degradation"] = map[string]interface{}{
			"level": rc.degradation.GetLevel().String(),
		}
	}

	return metrics
}

type ResilientHandler struct {
	clients    map[string]*ResilientClient
	defaultClient *ResilientClient
	mu         sync.RWMutex
}

func NewResilientHandler() *ResilientHandler {
	return &ResilientHandler{
		clients:      make(map[string]*ResilientClient),
		defaultClient: NewResilientClient(DefaultResilientClientConfig("default")),
	}
}

func (h *ResilientHandler) RegisterClient(name string, config ResilientClientConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[name] = NewResilientClient(config)
}

func (h *ResilientHandler) GetClient(name string) *ResilientClient {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if client, exists := h.clients[name]; exists {
		return client
	}

	return h.defaultClient
}

func (h *ResilientHandler) Execute(serviceName string, fn func() error) error {
	client := h.GetClient(serviceName)
	return client.Execute(fn)
}

func (h *ResilientHandler) ExecuteWithContext(serviceName string, ctx context.Context, fn func(ctx context.Context) error) error {
	client := h.GetClient(serviceName)
	return client.ExecuteWithContext(ctx, fn)
}

func (h *ResilientHandler) RegisterFallback(serviceName, fallbackName string, fn FallbackFunc) {
	client := h.GetClient(serviceName)
	client.RegisterFallback(fallbackName, fn)
}

func (h *ResilientHandler) GetAllMetrics() map[string]map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	allMetrics := make(map[string]map[string]interface{})

	for name, client := range h.clients {
		allMetrics[name] = client.GetMetrics()
	}

	allMetrics["default"] = h.defaultClient.GetMetrics()

	return allMetrics
}

func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter != nil && !limiter.Allow() {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CircuitBreakerMiddleware(breaker *CircuitBreaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if breaker != nil && !breaker.Allow() {
				http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)

			if rw.statusCode >= 500 {
				breaker.RecordFailure()
			} else {
				breaker.RecordSuccess()
			}
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

type HTTPMetrics struct {
	TotalRequests   int64   `json:"total_requests"`
	SuccessCount   int64   `json:"success_count"`
	ErrorCount     int64   `json:"error_count"`
	TimeoutCount   int64   `json:"timeout_count"`
	RateLimitedCount int64 `json:"rate_limited_count"`
	AverageLatency float64 `json:"average_latency_ms"`
}

type HTTPMonitoring struct {
	metrics  HTTPMetrics
	mu       sync.RWMutex
}

func NewHTTPMonitoring() *HTTPMonitoring {
	return &HTTPMonitoring{
		metrics: HTTPMetrics{},
	}
}

func (m *HTTPMonitoring) RecordSuccess(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.TotalRequests++
	m.metrics.SuccessCount++
}

func (m *HTTPMonitoring) RecordError(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.TotalRequests++
	m.metrics.ErrorCount++
}

func (m *HTTPMonitoring) RecordRateLimited() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics.TotalRequests++
	m.metrics.RateLimitedCount++
}

func (m *HTTPMonitoring) GetMetrics() HTTPMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := m.metrics
	if metrics.TotalRequests > 0 {
		metrics.AverageLatency = float64(metrics.TotalRequests) / float64(metrics.TotalRequests)
	}

	return metrics
}

func MonitoringMiddleware(monitor *HTTPMonitoring) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			latency := time.Since(start)

			switch {
			case rw.statusCode == http.StatusTooManyRequests:
				monitor.RecordRateLimited()
			case rw.statusCode >= 500:
				monitor.RecordError(latency)
			case rw.statusCode >= 400:
				monitor.RecordError(latency)
			default:
				monitor.RecordSuccess(latency)
			}

			r.Header.Set("X-Response-Time", latency.String())
		})
	}
}

func ExtractServiceName(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "api" {
		return parts[1]
	}
	return "default"
}
