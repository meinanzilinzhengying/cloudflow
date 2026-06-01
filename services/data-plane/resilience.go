package dataplane

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
}

func DefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		EnableCircuitBreaker: true,
		EnableRateLimiter:     true,
		EnableRetry:           true,
		EnableBulkhead:       true,
		EnableFallback:       true,

		CircuitBreaker: resilience.DefaultCircuitBreakerConfig("data-plane"),
		RateLimiter:    resilience.DefaultRateLimiterConfig("data-plane"),
		Retry:          resilience.DefaultRetryConfig(),
		Bulkhead:       resilience.DefaultBulkheadConfig(),

		HTTPTimeout: 30 * time.Second,
	}
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

type ResilientHTTPClient struct {
	client     *http.Client
	resilience *resilience.ResilientClient
}

func NewResilientHTTPClient(config ResilienceConfig) *ResilientHTTPClient {
	client := &http.Client{
		Timeout: config.HTTPTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	rc := resilience.NewResilientClient(config.ToClientConfig("data-plane-http"))

	rc.RegisterFallback("clickhouse", func(err error) interface{} {
		return fmt.Errorf("ClickHouse fallback: data may be temporarily unavailable")
	})

	rc.RegisterFallback("victoriametrics", func(err error) interface{} {
		return fmt.Errorf("VictoriaMetrics fallback: metrics may be stale")
	})

	rc.RegisterFallback("loki", func(err error) interface{} {
		return fmt.Errorf("Loki fallback: logs may be delayed")
	})

	return &ResilientHTTPClient{
		client:     client,
		resilience: rc,
	}
}

func (c *ResilientHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error

	err := c.resilience.ExecuteWithContext(req.Context(), func(ctx context.Context) error {
		req = req.WithContext(ctx)
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			return err
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			return lastErr
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("client error: %d", resp.StatusCode)
			return nil
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ResilientHTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *ResilientHTTPClient) Post(url string, body interface{}) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *ResilientHTTPClient) GetMetrics() map[string]interface{} {
	return c.resilience.GetMetrics()
}

func (c *ResilientHTTPClient) GetCircuitBreaker() *resilience.CircuitBreaker {
	return c.resilience.GetCircuitBreaker()
}

func (c *ResilientHTTPClient) GetRateLimiter() *resilience.RateLimiter {
	return c.resilience.GetRateLimiter()
}

type ResilientStorageClient struct {
	clickHouse *ResilientHTTPClient
	vm        *ResilientHTTPClient
	loki      *ResilientHTTPClient
}

func NewResilientStorageClient(cfg ResilienceConfig) *ResilientStorageClient {
	return &ResilientStorageClient{
		clickHouse: NewResilientHTTPClient(cfg),
		vm:        NewResilientHTTPClient(cfg),
		loki:      NewResilientHTTPClient(cfg),
	}
}

func (c *ResilientStorageClient) GetMetrics() map[string]map[string]interface{} {
	return map[string]map[string]interface{}{
		"clickhouse": c.clickHouse.GetMetrics(),
		"victoriametrics": c.vm.GetMetrics(),
		"loki": c.loki.GetMetrics(),
	}
}

type ResilienceMiddleware struct {
	limiter    *resilience.RateLimiter
	breaker    *resilience.CircuitBreaker
	monitor    *resilience.HTTPMonitoring
}

func NewResilienceMiddleware(cfg ResilienceConfig) *ResilienceMiddleware {
	return &ResilienceMiddleware{
		limiter: resilience.NewRateLimiter(cfg.RateLimiter),
		breaker: resilience.NewCircuitBreaker(cfg.CircuitBreaker),
		monitor: resilience.NewHTTPMonitoring(),
	}
}

func (m *ResilienceMiddleware) RateLimit(next http.Handler) http.Handler {
	return resilience.RateLimitMiddleware(m.limiter)(next)
}

func (m *ResilienceMiddleware) CircuitBreaker(next http.Handler) http.Handler {
	return resilience.CircuitBreakerMiddleware(m.breaker)(next)
}

func (m *ResilienceMiddleware) Monitoring(next http.Handler) http.Handler {
	return resilience.MonitoringMiddleware(m.monitor)(next)
}

func (m *ResilienceMiddleware) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"rate_limiter":     m.limiter.GetMetrics(),
		"circuit_breaker": m.breaker.GetMetrics(),
		"http":            m.monitor.GetMetrics(),
	}
}

func (m *ResilienceMiddleware) IsAvailable() bool {
	return m.breaker.IsAvailable() && m.limiter.IsAvailable()
}
