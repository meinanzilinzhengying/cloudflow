package alertengine

import (
	"fmt"
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

	MaxConcurrentAlerts int
	AlertEvalTimeout    time.Duration
}

func DefaultResilienceConfig() ResilienceConfig {
	cfg := ResilienceConfig{
		EnableCircuitBreaker: true,
		EnableRateLimiter:     true,
		EnableRetry:           true,
		EnableBulkhead:       true,
		EnableFallback:       true,

		CircuitBreaker: resilience.DefaultCircuitBreakerConfig("alert-engine"),
		RateLimiter:    resilience.DefaultRateLimiterConfig("alert-engine"),
		Retry:          resilience.DefaultRetryConfig(),
		Bulkhead:       resilience.DefaultBulkheadConfig(),

		MaxConcurrentAlerts: 1000,
		AlertEvalTimeout:    10 * time.Second,
	}

	cfg.RateLimiter.RequestsPerSecond = 50
	cfg.RateLimiter.BurstSize = 100

	cfg.CircuitBreaker.FailureThreshold = 0.5
	cfg.CircuitBreaker.SuccessThreshold = 3
	cfg.CircuitBreaker.MinRequests = 10

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

type AlertResilienceClient struct {
	metricsBreaker   *resilience.CircuitBreaker
	notificationBreaker *resilience.CircuitBreaker
	tiDBBreaker     *resilience.CircuitBreaker
	redisLimiter    *resilience.RateLimiter
}

func NewAlertResilienceClient(cfg ResilienceConfig) *AlertResilienceClient {
	metricsCfg := cfg.ToClientConfig("alert-metrics")
	metricsCfg.CircuitBreaker.Name = "alert-metrics"
	metricsCfg.CircuitBreaker.FailureThreshold = 0.3
	metricsCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Alert metrics circuit: %s -> %s\n", from, to)
	}

	notificationCfg := cfg.ToClientConfig("alert-notification")
	notificationCfg.CircuitBreaker.Name = "alert-notification"
	notificationCfg.CircuitBreaker.FailureThreshold = 0.5
	notificationCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Alert notification circuit: %s -> %s\n", from, to)
	}

	tiDBCfg := cfg.ToClientConfig("alert-tidb")
	tiDBCfg.CircuitBreaker.Name = "alert-tidb"
	tiDBCfg.CircuitBreaker.FailureThreshold = 0.3

	redisCfg := cfg.RateLimiter
	redisCfg.Name = "alert-redis"

	return &AlertResilienceClient{
		metricsBreaker:      resilience.NewCircuitBreaker(metricsCfg.CircuitBreaker),
		notificationBreaker: resilience.NewCircuitBreaker(notificationCfg.CircuitBreaker),
		tiDBBreaker:        resilience.NewCircuitBreaker(tiDBCfg.CircuitBreaker),
		redisLimiter:       resilience.NewRateLimiter(redisCfg),
	}
}

func (c *AlertResilienceClient) IsMetricsAvailable() bool {
	return c.metricsBreaker.IsAvailable()
}

func (c *AlertResilienceClient) IsNotificationAvailable() bool {
	return c.notificationBreaker.IsAvailable()
}

func (c *AlertResilienceClient) IsTiDBAvailable() bool {
	return c.tiDBBreaker.IsAvailable()
}

func (c *AlertResilienceClient) AllowNotification() bool {
	return c.notificationBreaker.Allow() && c.redisLimiter.Allow()
}

func (c *AlertResilienceClient) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"metrics_breaker":      c.metricsBreaker.GetMetrics(),
		"notification_breaker": c.notificationBreaker.GetMetrics(),
		"tidb_breaker":        c.tiDBBreaker.GetMetrics(),
		"redis_limiter":       c.redisLimiter.GetMetrics(),
	}
}

type DegradedAlertState struct {
	AlertEngine    resilience.DegradationLevel
	MetricsSource  resilience.DegradationLevel
	Notification   resilience.DegradationLevel
}

func (c *AlertResilienceClient) GetDegradedState() DegradedAlertState {
	state := DegradedAlertState{
		AlertEngine:  resilience.DegradationNone,
		MetricsSource: resilience.DegradationNone,
		Notification: resilience.DegradationNone,
	}

	if !c.tiDBBreaker.IsAvailable() {
		state.AlertEngine = resilience.DegradationFallback
	}

	if !c.metricsBreaker.IsAvailable() {
		state.MetricsSource = resilience.DegradationFallback
	}

	if !c.notificationBreaker.IsAvailable() {
		state.Notification = resilience.DegradationMinimal
	}

	return state
}

func (s DegradedAlertState) ShouldSkipEvaluation() bool {
	return s.MetricsSource == resilience.DegradationFallback
}

func (s DegradedAlertState) ShouldSkipNotifications() bool {
	return s.Notification == resilience.DegradationFallback
}

func (s DegradedAlertState) String() string {
	return fmt.Sprintf("AlertEngine=%s, Metrics=%s, Notification=%s",
		s.AlertEngine, s.MetricsSource, s.Notification)
}
