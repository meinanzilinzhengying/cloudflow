package controlplane

import (
	"fmt"
	"time"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/resilience"
)

type ControlPlaneResilienceClient struct {
	breaker *resilience.CircuitBreaker
	limiter *resilience.RateLimiter
}

func NewControlPlaneResilienceClient() *ControlPlaneResilienceClient {
	cfg := resilience.DefaultCircuitBreakerConfig("control-plane")
	cfg.FailureThreshold = 0.5
	cfg.MinRequests = 10
	cfg.OpenTimeout = 30 * time.Second
	cfg.OnStateChange = func(from, to resilience.CircuitState) {
		fmt.Printf("Control-plane circuit: %s -> %s\n", from, to)
	}
	limiterCfg := resilience.DefaultRateLimiterConfig("control-plane")
	limiterCfg.RequestsPerSecond = 500
	limiterCfg.BurstSize = 1000
	return &ControlPlaneResilienceClient{
		breaker: resilience.NewCircuitBreaker(cfg),
		limiter: resilience.NewRateLimiter(limiterCfg),
	}
}

func (c *ControlPlaneResilienceClient) Allow() bool {
	if !c.limiter.Allow() { return false }
	if !c.breaker.Allow() { return false }
	return true
}
func (c *ControlPlaneResilienceClient) RecordSuccess() { c.breaker.RecordSuccess() }
func (c *ControlPlaneResilienceClient) RecordFailure(err error) { c.breaker.RecordFailure() }
