package queryservice

import (
	"fmt"
	"net/http"
	"time"

	"cloud-flow/services/shared/resilience"
)

type ResilienceConfig struct {
	EnableCircuitBreaker bool
	EnableRateLimiter    bool
	EnableRetry          bool
	EnableBulkhead       bool
	EnableFallback       bool

	CircuitBreaker resilience.CircuitBreakerConfig
	RateLimiter    resilience.RateLimiterConfig
	Retry          resilience.RetryConfig
	Bulkhead       resilience.BulkheadConfig

	HTTPTimeout time.Duration

	MaxConcurrentQueries int
	QueryTimeout         time.Duration
}

func DefaultResilienceConfig() ResilienceConfig {
	cfg := ResilienceConfig{
		EnableCircuitBreaker: true,
		EnableRateLimiter:     true,
		EnableRetry:           true,
		EnableBulkhead:       true,
		EnableFallback:       true,

		CircuitBreaker: resilience.DefaultCircuitBreakerConfig("query-service"),
		RateLimiter:    resilience.DefaultRateLimiterConfig("query-service"),
		Retry:          resilience.DefaultRetryConfig(),
		Bulkhead:       resilience.DefaultBulkheadConfig(),

		HTTPTimeout:         30 * time.Second,
		MaxConcurrentQueries: 1000,
		QueryTimeout:         30 * time.Second,
	}

	cfg.RateLimiter.RequestsPerSecond = 100
	cfg.RateLimiter.BurstSize = 200

	cfg.Bulkhead.MaxConcurrent = 1000
	cfg.Bulkhead.MaxWaitTime = 5 * time.Second

	return cfg
}

func (c *ResilienceConfig) ToClientConfig(serviceName string) resilience.ResilientClientConfig {
	cfg := resilience.DefaultResilientClientConfig(serviceName)
	cfg.EnableCircuitBreaker = c.EnableCircuitBreaker
	cfg.EnableRateLimiter = c.EnableRateLimiter
	cfg.EnableRetry = c.EnableRetry
	cfg.EnableBulkhead = c.EnableBulkhead
	cfg.EnableFallback = c.EnableFallback

	cfg.CircuitBreaker = c.CircuitBreaker
	cfg.RateLimiter = c.RateLimiter
	cfg.Retry = c.Retry
	cfg.Bulkhead = c.Bulkhead

	return cfg
}

type QueryResilienceClient struct {
	clickHouse *resilience.ResilientClient
	vm         *resilience.ResilientClient
	loki       *resilience.ResilientClient
}

func NewQueryResilienceClient(cfg ResilienceConfig) *QueryResilienceClient {
	chCfg := cfg.ToClientConfig("query-clickhouse")
	chCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("ClickHouse circuit: %s -> %s\n", from, to)
	}
	chCfg.CircuitBreaker.FailureThreshold = 0.3

	vmCfg := cfg.ToClientConfig("query-victoriametrics")
	vmCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("VictoriaMetrics circuit: %s -> %s\n", from, to)
	}
	vmCfg.CircuitBreaker.FailureThreshold = 0.3

	lokiCfg := cfg.ToClientConfig("query-loki")
	lokiCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Loki circuit: %s -> %s\n", from, to)
	}
	lokiCfg.CircuitBreaker.FailureThreshold = 0.3

	chClient := resilience.NewResilientClient(chCfg)
	chClient.RegisterFallback("clickhouse", func(err error) interface{} {
		return fmt.Errorf("ClickHouse degraded: returning cached/stale data")
	})

	vmClient := resilience.NewResilientClient(vmCfg)
	vmClient.RegisterFallback("victoriametrics", func(err error) interface{} {
		return fmt.Errorf("VictoriaMetrics degraded: metrics may be stale")
	})

	lokiClient := resilience.NewResilientClient(lokiCfg)
	lokiClient.RegisterFallback("loki", func(err error) interface{} {
		return fmt.Errorf("Loki degraded: logs may be incomplete")
	})

	return &QueryResilienceClient{
		clickHouse: chClient,
		vm:         vmClient,
		loki:       lokiClient,
	}
}

type ResilienceMiddleware struct {
	limiter         *resilience.RateLimiter
	bulkhead        *resilience.Bulkhead
	queryBreaker    *resilience.CircuitBreaker
	storageBreaker  *resilience.CircuitBreaker
	monitor         *resilience.HTTPMonitoring
}

func NewResilienceMiddleware(cfg ResilienceConfig) *ResilienceMiddleware {
	return &ResilienceMiddleware{
		limiter:         resilience.NewRateLimiter(cfg.RateLimiter),
		bulkhead:        resilience.NewBulkhead(cfg.Bulkhead),
		queryBreaker:    resilience.NewCircuitBreaker(cfg.CircuitBreaker),
		storageBreaker:  resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
			Name:                  "query-storage",
			FailureThreshold:      0.3,
			SuccessThreshold:       5,
			MinRequests:           10,
			OpenTimeout:           60 * time.Second,
			HalfOpenTimeout:       120 * time.Second,
			MaxHalfOpenRequests:   5,
		}),
		monitor:         resilience.NewHTTPMonitoring(),
	}
}

func (m *ResilienceMiddleware) RateLimit(next http.Handler) http.Handler {
	return resilience.RateLimitMiddleware(m.limiter)(next)
}

func (m *ResilienceMiddleware) CircuitBreaker(next http.Handler) http.Handler {
	return resilience.CircuitBreakerMiddleware(m.queryBreaker)(next)
}

func (m *ResilienceMiddleware) Monitoring(next http.Handler) http.Handler {
	return resilience.MonitoringMiddleware(m.monitor)(next)
}

func (m *ResilienceMiddleware) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"rate_limiter":      m.limiter.GetMetrics(),
		"query_breaker":    m.queryBreaker.GetMetrics(),
		"storage_breaker":   m.storageBreaker.GetMetrics(),
		"http":             m.monitor.GetMetrics(),
		"bulkhead":         resilience.BulkheadMetrics{
			MaxConcurrent: m.bulkhead.GetMaxConcurrent(),
			ActiveCount:   m.bulkhead.GetActiveCount(),
			Utilization:   m.bulkhead.GetUtilization(),
			IsAvailable:   m.bulkhead.IsAvailable(),
		},
	}
}

func (m *ResilienceMiddleware) IsAvailable() bool {
	return m.queryBreaker.IsAvailable() && m.limiter.IsAvailable() && m.bulkhead.IsAvailable()
}

func (m *ResilienceMiddleware) AllowQuery() bool {
	if !m.limiter.Allow() {
		return false
	}
	if !m.bulkhead.IsAvailable() {
		return false
	}
	return true
}
