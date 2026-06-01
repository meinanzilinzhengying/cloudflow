package authservice

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

	MaxConcurrentAuths int
	AuthTimeout         time.Duration
}

func DefaultResilienceConfig() ResilienceConfig {
	cfg := ResilienceConfig{
		EnableCircuitBreaker: true,
		EnableRateLimiter:     true,
		EnableRetry:           true,
		EnableBulkhead:       true,
		EnableFallback:       true,

		CircuitBreaker: resilience.DefaultCircuitBreakerConfig("auth-service"),
		RateLimiter:    resilience.DefaultRateLimiterConfig("auth-service"),
		Retry:          resilience.DefaultRetryConfig(),
		Bulkhead:       resilience.DefaultBulkheadConfig(),

		MaxConcurrentAuths: 5000,
		AuthTimeout:        5 * time.Second,
	}

	cfg.RateLimiter.RequestsPerSecond = 1000
	cfg.RateLimiter.BurstSize = 2000

	cfg.CircuitBreaker.FailureThreshold = 0.5
	cfg.CircuitBreaker.SuccessThreshold = 3
	cfg.CircuitBreaker.MinRequests = 100
	cfg.CircuitBreaker.OpenTimeout = 30 * time.Second

	cfg.Bulkhead.MaxConcurrent = 5000
	cfg.Bulkhead.MaxWaitTime = 3 * time.Second

	return cfg
}

type AuthResilienceClient struct {
	tiDBBreaker *resilience.CircuitBreaker
	limiter     *resilience.RateLimiter
	bulkhead    *resilience.Bulkhead
}

func NewAuthResilienceClient(cfg ResilienceConfig) *AuthResilienceClient {
	tiDBCfg := cfg.ToClientConfig("auth-tidb")
	tiDBCfg.CircuitBreaker.Name = "auth-tidb"
	tiDBCfg.CircuitBreaker.FailureThreshold = 0.3
	tiDBCfg.CircuitBreaker.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Auth TiDB circuit: %s -> %s\n", from, to)
	}

	return &AuthResilienceClient{
		tiDBBreaker: resilience.NewCircuitBreaker(tiDBCfg.CircuitBreaker),
		limiter:     resilience.NewRateLimiter(cfg.RateLimiter),
		bulkhead:    resilience.NewBulkhead(cfg.Bulkhead),
	}
}

func (c *AuthResilienceClient) AllowAuth() bool {
	if !c.limiter.Allow() {
		return false
	}
	if !c.bulkhead.IsAvailable() {
		return false
	}
	if !c.tiDBBreaker.Allow() {
		return false
	}
	return true
}

func (c *AuthResilienceClient) RecordSuccess() {
	c.tiDBBreaker.RecordSuccess()
}

func (c *AuthResilienceClient) RecordFailure(err error) {
	c.tiDBBreaker.RecordFailure()
}

func (c *AuthResilienceClient) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"tiadb_breaker": c.tiDBBreaker.GetMetrics(),
		"rate_limiter":  c.limiter.GetMetrics(),
		"bulkhead":      resilience.BulkheadMetrics{
			MaxConcurrent: c.bulkhead.GetMaxConcurrent(),
			ActiveCount:   c.bulkhead.GetActiveCount(),
			Utilization:   c.bulkhead.GetUtilization(),
			IsAvailable:   c.bulkhead.IsAvailable(),
		},
	}
}

func (c *AuthResilienceClient) IsAvailable() bool {
	return c.tiDBBreaker.IsAvailable() && c.bulkhead.IsAvailable()
}

func (c *AuthResilienceClient) GetHealthStatus() string {
	if !c.tiDBBreaker.IsAvailable() {
		return "degraded"
	}
	if !c.bulkhead.IsAvailable() {
		return "limited"
	}
	if c.bulkhead.GetUtilization() > 80 {
		return "high_load"
	}
	return "healthy"
}

type GracefulDegradationConfig struct {
	EnableCachedTokens bool
	EnableOfflineMode  bool
	MaxCachedTokens    int
	TokenCacheTTL      time.Duration
}

func DefaultGracefulDegradationConfig() GracefulDegradationConfig {
	return GracefulDegradationConfig{
		EnableCachedTokens: true,
		EnableOfflineMode:  false,
		MaxCachedTokens:    10000,
		TokenCacheTTL:      1 * time.Hour,
	}
}

type GracefulDegradation struct {
	config GracefulDegradationConfig
	tokens map[string]*cachedToken
	mu     sync.RWMutex
}

type cachedToken struct {
	token     string
	userID    string
	tenantID  string
	expiresAt time.Time
}

func NewGracefulDegradation(cfg GracefulDegradationConfig) *GracefulDegradation {
	return &GracefulDegradation{
		config: cfg,
		tokens: make(map[string]*cachedToken),
	}
}

func (g *GracefulDegradation) GetCachedToken(token string) (string, string, bool) {
	if !g.config.EnableCachedTokens {
		return "", "", false
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	cached, exists := g.tokens[token]
	if !exists || time.Now().After(cached.expiresAt) {
		return "", "", false
	}

	return cached.userID, cached.tenantID, true
}

func (g *GracefulDegradation) CacheToken(token, userID, tenantID string, ttl time.Duration) {
	if !g.config.EnableCachedTokens {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.tokens) >= g.config.MaxCachedTokens {
		g.evictOldest()
	}

	g.tokens[token] = &cachedToken{
		token:     token,
		userID:    userID,
		tenantID:  tenantID,
		expiresAt: time.Now().Add(ttl),
	}
}

func (g *GracefulDegradation) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, t := range g.tokens {
		if oldestTime.IsZero() || t.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = t.expiresAt
		}
	}

	delete(g.tokens, oldestKey)
}

func (g *GracefulDegradation) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokens = make(map[string]*cachedToken)
}

func (g *GracefulDegradation) Size() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.tokens)
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
