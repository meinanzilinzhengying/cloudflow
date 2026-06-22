package tenantservice

import (
	"fmt"
	"time"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/resilience"
)

type TenantResilienceClient struct {
	breaker *resilience.CircuitBreaker
	limiter *resilience.RateLimiter
}

func NewTenantResilienceClient() *TenantResilienceClient {
	cfg := resilience.DefaultCircuitBreakerConfig("tenant-service")
	cfg.FailureThreshold = 0.5
	cfg.MinRequests = 10
	cfg.OpenTimeout = 30 * time.Second
	cfg.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Tenant-service circuit: %s -> %s\n", from, to)
	}
	limiterCfg := resilience.DefaultRateLimiterConfig("tenant-service")
	limiterCfg.RequestsPerSecond = 500
	limiterCfg.BurstSize = 1000
	return &TenantResilienceClient{
		breaker: resilience.NewCircuitBreaker(cfg),
		limiter: resilience.NewRateLimiter(limiterCfg),
	}
}

func (c *TenantResilienceClient) Allow() bool {
	if !c.limiter.Allow() { return false }
	if !c.breaker.Allow() { return false }
	return true
}
func (c *TenantResilienceClient) RecordSuccess() { c.breaker.RecordSuccess() }
func (c *TenantResilienceClient) RecordFailure(err error) { c.breaker.RecordFailure() }
