package resilience

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

type CircuitBreakerConfig struct {
	Name string

	FailureThreshold float64
	SuccessThreshold float64

	MinRequests int64

	OpenTimeout     time.Duration
	HalfOpenTimeout time.Duration

	MaxHalfOpenRequests int64

	OnStateChange func(from, to CircuitState)
	OnError       func(err error, duration time.Duration)
	OnSuccess     func(duration time.Duration)
}

func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:                  name,
		FailureThreshold:      0.5,
		SuccessThreshold:      3,
		MinRequests:           10,
		OpenTimeout:           30 * time.Second,
		HalfOpenTimeout:       60 * time.Second,
		MaxHalfOpenRequests:   5,
		OnStateChange:         nil,
		OnError:               nil,
		OnSuccess:             nil,
	}
}

type CircuitBreakerMetrics struct {
	Name                  string  `json:"name"`
	State                 string  `json:"state"`
	TotalRequests         int64   `json:"total_requests"`
	SuccessCount          int64   `json:"success_count"`
	FailureCount          int64   `json:"failure_count"`
	RejectionCount        int64   `json:"rejection_count"`
	SuccessRate           float64 `json:"success_rate"`
	ErrorRate             float64 `json:"error_rate"`
	LastSuccessTime       string  `json:"last_success_time,omitempty"`
	LastFailureTime       string  `json:"last_failure_time,omitempty"`
	LastStateChangeTime   string  `json:"last_state_change_time"`
	OpenTime              string  `json:"open_time,omitempty"`
	AverageLatencyMs      float64 `json:"average_latency_ms"`
	TotalLatencyMs        int64   `json:"total_latency_ms"`
}

type CircuitBreaker struct {
	config CircuitBreakerConfig

	state            int32
	totalRequests    int64
	successCount     int64
	failureCount     int64
	rejectionCount   int64
	halfOpenCount    int64
	lastSuccessTime  int64
	lastFailureTime  int64
	lastStateChange  int64
	totalLatencyNs   int64
	successLatencyNs int64

	consecutiveSuccesses int64
	consecutiveFailures int64

	mu sync.RWMutex
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.Name == "" {
		config.Name = "default"
	}
	if config.OpenTimeout <= 0 {
		config.OpenTimeout = 30 * time.Second
	}
	if config.HalfOpenTimeout <= 0 {
		config.HalfOpenTimeout = 60 * time.Second
	}
	if config.MinRequests <= 0 {
		config.MinRequests = 10
	}

	cb := &CircuitBreaker{
		config:        config,
		state:         int32(StateClosed),
		lastStateChange: time.Now().UnixNano(),
	}

	atomic.StoreInt64(&cb.totalRequests, 0)
	atomic.StoreInt64(&cb.successCount, 0)
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.rejectionCount, 0)
	atomic.StoreInt64(&cb.halfOpenCount, 0)

	return cb
}

func (cb *CircuitBreaker) getState() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

func (cb *CircuitBreaker) setState(state CircuitState) {
	oldState := CircuitState(atomic.LoadInt32(&cb.state))
	if oldState != state {
		atomic.StoreInt32(&cb.state, int32(state))
		atomic.StoreInt64(&cb.lastStateChange, time.Now().UnixNano())
		atomic.StoreInt64(&cb.halfOpenCount, 0)

		if cb.config.OnStateChange != nil {
			go cb.config.OnStateChange(oldState, state)
		}
	}
}

func (cb *CircuitBreaker) Allow() bool {
	return cb.AllowWithContext(nil)
}

func (cb *CircuitBreaker) AllowWithContext(ctx interface{}) bool {
	state := cb.getState()
	now := time.Now()

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		lastChange := atomic.LoadInt64(&cb.lastStateChange)
		elapsed := now.Sub(time.Unix(0, lastChange))

		if elapsed >= cb.config.OpenTimeout {
			cb.mu.Lock()
			if cb.getState() == StateOpen {
				atomic.StoreInt64(&cb.halfOpenCount, 0)
				cb.setState(StateHalfOpen)
			}
			cb.mu.Unlock()
			return cb.AllowWithContext(ctx)
		}

		atomic.AddInt64(&cb.rejectionCount, 1)
		return false

	case StateHalfOpen:
		currentCount := atomic.LoadInt64(&cb.halfOpenCount)
		if currentCount >= cb.config.MaxHalfOpenRequests {
			atomic.AddInt64(&cb.rejectionCount, 1)
			return false
		}
		atomic.AddInt64(&cb.halfOpenCount, 1)
		return true

	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.RecordSuccessWithLatency(0)
}

func (cb *CircuitBreaker) RecordSuccessWithLatency(latency time.Duration) {
	state := cb.getState()

	atomic.AddInt64(&cb.totalRequests, 1)
	atomic.AddInt64(&cb.successCount, 1)
	atomic.StoreInt64(&cb.lastSuccessTime, time.Now().UnixNano())

	if latency > 0 {
		atomic.AddInt64(&cb.totalLatencyNs, latency.Nanoseconds())
		atomic.AddInt64(&cb.successLatencyNs, latency.Nanoseconds())
	}

	atomic.AddInt64(&cb.consecutiveSuccesses, 1)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)

	if cb.config.OnSuccess != nil {
		go cb.config.OnSuccess(latency)
	}

	switch state {
	case StateClosed:
		cb.evaluateClosedState()

	case StateHalfOpen:
		consecutiveSuccesses := atomic.LoadInt64(&cb.consecutiveSuccesses)
		if consecutiveSuccesses >= cb.config.SuccessThreshold {
			cb.mu.Lock()
			if cb.getState() == StateHalfOpen {
				atomic.StoreInt64(&cb.successCount, 0)
				atomic.StoreInt64(&cb.failureCount, 0)
				atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
				atomic.StoreInt64(&cb.consecutiveFailures, 0)
				cb.setState(StateClosed)
			}
			cb.mu.Unlock()
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.RecordFailureWithLatency(0, nil)
}

func (cb *CircuitBreaker) RecordFailureWithLatency(latency time.Duration, err error) {
	state := cb.getState()

	atomic.AddInt64(&cb.totalRequests, 1)
	atomic.AddInt64(&cb.failureCount, 1)
	atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())

	if latency > 0 {
		atomic.AddInt64(&cb.totalLatencyNs, latency.Nanoseconds())
	}

	atomic.AddInt64(&cb.consecutiveFailures, 1)
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)

	if cb.config.OnError != nil {
		go cb.config.OnError(err, latency)
	}

	switch state {
	case StateClosed:
		cb.evaluateClosedState()

	case StateHalfOpen:
		cb.mu.Lock()
		if cb.getState() == StateHalfOpen {
			atomic.StoreInt64(&cb.successCount, 0)
			atomic.StoreInt64(&cb.failureCount, 0)
			atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
			atomic.StoreInt64(&cb.consecutiveFailures, 0)
			cb.setState(StateOpen)
		}
		cb.mu.Unlock()
	}
}

func (cb *CircuitBreaker) evaluateClosedState() {
	total := atomic.LoadInt64(&cb.totalRequests)
	if total < cb.config.MinRequests {
		return
	}

	failures := atomic.LoadInt64(&cb.failureCount)
	errorRate := float64(failures) / float64(total)

	if errorRate >= cb.config.FailureThreshold {
		cb.mu.Lock()
		if cb.getState() == StateClosed {
			atomic.StoreInt64(&cb.lastStateChange, time.Now().UnixNano())
			cb.setState(StateOpen)
		}
		cb.mu.Unlock()
	}
}

func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	total := atomic.LoadInt64(&cb.totalRequests)
	successes := atomic.LoadInt64(&cb.successCount)
	failures := atomic.LoadInt64(&cb.failureCount)
	rejections := atomic.LoadInt64(&cb.rejectionCount)

	var successRate, errorRate float64
	if total > 0 {
		successRate = math.Round(float64(successes)/float64(total)*10000) / 100
		errorRate = math.Round(float64(failures)/float64(total)*10000) / 100
	}

	totalLatency := atomic.LoadInt64(&cb.totalLatencyNs)
	var avgLatencyMs float64
	if successes > 0 {
		avgLatencyMs = math.Round(float64(totalLatency)/float64(successes)/10000) / 100
	}

	lastSuccess := atomic.LoadInt64(&cb.lastSuccessTime)
	lastFail := atomic.LoadInt64(&cb.lastFailureTime)
	lastChange := atomic.LoadInt64(&cb.lastStateChange)

	metrics := CircuitBreakerMetrics{
		Name:                cb.config.Name,
		State:               cb.getState().String(),
		TotalRequests:       total,
		SuccessCount:        successes,
		FailureCount:       failures,
		RejectionCount:     rejections,
		SuccessRate:        successRate,
		ErrorRate:          errorRate,
		AverageLatencyMs:   avgLatencyMs,
		TotalLatencyMs:     totalLatency / 1e6,
		LastStateChangeTime: time.Unix(0, lastChange).Format(time.RFC3339),
	}

	if lastSuccess > 0 {
		metrics.LastSuccessTime = time.Unix(0, lastSuccess).Format(time.RFC3339)
	}
	if lastFail > 0 {
		metrics.LastFailureTime = time.Unix(0, lastFail).Format(time.RFC3339)
	}
	if cb.getState() == StateOpen {
		metrics.OpenTime = cb.config.OpenTimeout.String()
	}

	return metrics
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.StoreInt64(&cb.totalRequests, 0)
	atomic.StoreInt64(&cb.successCount, 0)
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.rejectionCount, 0)
	atomic.StoreInt64(&cb.halfOpenCount, 0)
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)
	atomic.StoreInt64(&cb.totalLatencyNs, 0)
	atomic.StoreInt64(&cb.successLatencyNs, 0)

	cb.setState(StateClosed)
}

func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.StoreInt64(&cb.successCount, 0)
	atomic.StoreInt64(&cb.failureCount, 0)
	atomic.StoreInt64(&cb.consecutiveSuccesses, 0)
	atomic.StoreInt64(&cb.consecutiveFailures, 0)

	cb.setState(StateOpen)
}

func (cb *CircuitBreaker) IsAvailable() bool {
	return cb.Allow()
}

func (cb *CircuitBreaker) State() CircuitState {
	return cb.getState()
}
