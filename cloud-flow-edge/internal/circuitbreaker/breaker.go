// Package circuitbreaker provides a gRPC service-level circuit breaker for the Edge server.
// It implements the standard three-state pattern: Closed -> Open -> HalfOpen.
package circuitbreaker

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the state of the circuit breaker.
type State int32

const (
	// StateClosed indicates the circuit breaker is closed and requests flow normally.
	StateClosed State = iota
	// StateOpen indicates the circuit breaker is open and requests are rejected.
	StateOpen
	// StateHalfOpen indicates the circuit breaker is testing recovery with limited requests.
	StateHalfOpen
)

func (s State) String() string {
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

// Config holds configuration for the circuit breaker.
type Config struct {
	// FailureThreshold is the error rate (0.0-1.0) that triggers the Open state (default: 0.5)
	FailureThreshold float64
	// MinRequests is the minimum number of requests before the circuit can open (default: 100)
	MinRequests int64
	// RecoveryTimeout is how long the breaker stays in Open state before transitioning
	// to HalfOpen (default: 30s)
	RecoveryTimeout time.Duration
	// HalfOpenMaxRequests is the maximum number of test requests allowed in HalfOpen
	// state (default: 10)
	HalfOpenMaxRequests int64
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		FailureThreshold:   0.5,
		MinRequests:        100,
		RecoveryTimeout:    30 * time.Second,
		HalfOpenMaxRequests: 10,
	}
}

// Metrics holds metrics about the circuit breaker.
type Metrics struct {
	State              string  `json:"state"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessCount       int64   `json:"success_count"`
	FailureCount       int64   `json:"failure_count"`
	ErrorRate          float64 `json:"error_rate"`
	LastFailureTime    string  `json:"last_failure_time,omitempty"`
	LastStateChange    string  `json:"last_state_change"`
	HalfOpenProcessed  int64   `json:"half_open_processed,omitempty"`
	ConsecutiveSuccess int64   `json:"consecutive_success,omitempty"`
}

// Breaker implements a circuit breaker with three states.
type Breaker struct {
	config Config
	name   string

	state          int32 // atomic; stores State value
	totalRequests  int64 // atomic
	successCount   int64 // atomic
	failureCount   int64 // atomic
	lastFailure    int64 // atomic; Unix nanoseconds
	lastStateChange int64 // atomic; Unix nanoseconds

	// halfOpenCount tracks the number of requests processed in HalfOpen state.
	// Protected by mu for atomicity with state transitions.
	halfOpenCount int64

	mu sync.RWMutex
}

// New creates a new circuit breaker with the given name and configuration.
// If name is empty, "default" is used. If config values are zero, defaults are applied.
func New(name string, config Config) *Breaker {
	if name == "" {
		name = "default"
	}
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 0.5
	}
	if config.FailureThreshold > 1.0 {
		config.FailureThreshold = 1.0
	}
	if config.MinRequests <= 0 {
		config.MinRequests = 100
	}
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 10
	}

	now := time.Now().UnixNano()
	return &Breaker{
		config:         config,
		name:           name,
		state:          int32(StateClosed),
		lastStateChange: now,
	}
}

// getState returns the current state of the circuit breaker.
func (b *Breaker) getState() State {
	return State(atomic.LoadInt32(&b.state))
}

// setState atomically sets the circuit breaker state and records the transition time.
func (b *Breaker) setState(s State) {
	atomic.StoreInt32(&b.state, int32(s))
	atomic.StoreInt64(&b.lastStateChange, time.Now().UnixNano())
}

// resetCounters resets the success and failure counters (used on state transitions).
func (b *Breaker) resetCounters() {
	atomic.StoreInt64(&b.totalRequests, 0)
	atomic.StoreInt64(&b.successCount, 0)
	atomic.StoreInt64(&b.failureCount, 0)
	atomic.StoreInt64(&b.halfOpenCount, 0)
}

// Allow checks whether a request should be processed based on the current state.
// Returns true if the request should proceed, false if it should be rejected.
func (b *Breaker) Allow() bool {
	state := b.getState()

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if recovery timeout has elapsed; if so, transition to HalfOpen
		lastChange := atomic.LoadInt64(&b.lastStateChange)
		elapsed := time.Since(time.Unix(0, lastChange))
		if elapsed >= b.config.RecoveryTimeout {
			b.mu.Lock()
			// Double-check after acquiring lock to avoid race
			if b.getState() == StateOpen {
				b.resetCounters()
				b.setState(StateHalfOpen)
			}
			b.mu.Unlock()
			return b.Allow() // Re-evaluate after state transition
		}
		return false

	case StateHalfOpen:
		b.mu.Lock()
		defer b.mu.Unlock()
		// Re-check state in case it changed while waiting for lock
		if b.getState() != StateHalfOpen {
			return b.Allow()
		}
		currentCount := atomic.LoadInt64(&b.halfOpenCount)
		if currentCount >= b.config.HalfOpenMaxRequests {
			return false
		}
		atomic.AddInt64(&b.halfOpenCount, 1)
		return true
	}

	return false
}

// RecordSuccess records a successful request and may trigger state transitions.
func (b *Breaker) RecordSuccess() {
	atomic.AddInt64(&b.totalRequests, 1)
	atomic.AddInt64(&b.successCount, 1)

	state := b.getState()

	switch state {
	case StateClosed:
		// In Closed state, check if the error rate has improved (no action needed;
		// the error rate is evaluated on failures)
		_ = state // no-op, success in closed state is the normal path

	case StateHalfOpen:
		b.mu.Lock()
		defer b.mu.Unlock()
		// If all half-open requests succeed, close the circuit
		total := atomic.LoadInt64(&b.totalRequests)
		failures := atomic.LoadInt64(&b.failureCount)
		if failures == 0 && total >= b.config.HalfOpenMaxRequests {
			b.resetCounters()
			b.setState(StateClosed)
		}
		// Also close if we've had enough consecutive successes
		if failures == 0 {
			successes := atomic.LoadInt64(&b.successCount)
			if successes >= b.config.HalfOpenMaxRequests {
				b.resetCounters()
				b.setState(StateClosed)
			}
		}
	}
}

// RecordFailure records a failed request and may trigger state transitions.
func (b *Breaker) RecordFailure() {
	atomic.AddInt64(&b.totalRequests, 1)
	atomic.AddInt64(&b.failureCount, 1)
	atomic.StoreInt64(&b.lastFailure, time.Now().UnixNano())

	state := b.getState()

	switch state {
	case StateClosed:
		b.evaluateClosedState()

	case StateHalfOpen:
		b.mu.Lock()
		defer b.mu.Unlock()
		// Any failure in HalfOpen immediately reopens the circuit
		if b.getState() == StateHalfOpen {
			b.resetCounters()
			b.setState(StateOpen)
		}
	}
}

// evaluateClosedState checks if the error rate exceeds the threshold and opens the circuit.
func (b *Breaker) evaluateClosedState() {
	total := atomic.LoadInt64(&b.totalRequests)
	if total < b.config.MinRequests {
		return
	}

	failures := atomic.LoadInt64(&b.failureCount)
	errorRate := float64(failures) / float64(total)

	if errorRate >= b.config.FailureThreshold {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Double-check state after acquiring lock
		if b.getState() == StateClosed {
			b.setState(StateOpen)
		}
	}
}

// GetState returns the current state and metrics of the circuit breaker.
func (b *Breaker) GetState() Metrics {
	state := b.getState()
	total := atomic.LoadInt64(&b.totalRequests)
	successes := atomic.LoadInt64(&b.successCount)
	failures := atomic.LoadInt64(&b.failureCount)

	var errorRate float64
	if total > 0 {
		errorRate = math.Round(float64(failures)/float64(total)*10000) / 10000 // 4 decimal places
	}

	lastFail := atomic.LoadInt64(&b.lastFailure)
	lastChange := atomic.LoadInt64(&b.lastStateChange)
	halfOpenCount := atomic.LoadInt64(&b.halfOpenCount)

	metrics := Metrics{
		State:             state.String(),
		TotalRequests:     total,
		SuccessCount:      successes,
		FailureCount:      failures,
		ErrorRate:         errorRate,
		LastStateChange:   time.Unix(0, lastChange).Format(time.RFC3339),
		HalfOpenProcessed: halfOpenCount,
	}

	if lastFail > 0 {
		metrics.LastFailureTime = time.Unix(0, lastFail).Format(time.RFC3339)
	}

	return metrics
}

// Name returns the name of this circuit breaker instance.
func (b *Breaker) Name() string {
	return b.name
}

// Reset manually resets the circuit breaker to Closed state and clears all counters.
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetCounters()
	b.setState(StateClosed)
}

// ForceOpen forcibly opens the circuit breaker, rejecting all requests until
// RecoveryTimeout elapses.
func (b *Breaker) ForceOpen() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetCounters()
	b.setState(StateOpen)
}

// Manager manages multiple circuit breakers by service name.
type Manager struct {
	breakers sync.Map // map[string]*Breaker
	config   Config
}

// NewManager creates a new circuit breaker manager with the given default configuration.
func NewManager(config Config) *Manager {
	if config.FailureThreshold <= 0 {
		config = DefaultConfig()
	}
	return &Manager{
		config: config,
	}
}

// GetBreaker returns the circuit breaker for the given service name, creating one
// if it does not exist. If serviceName is empty, returns a global default breaker.
func (m *Manager) GetBreaker(serviceName string) *Breaker {
	if serviceName == "" {
		serviceName = "global"
	}

	if val, ok := m.breakers.Load(serviceName); ok {
		return val.(*Breaker)
	}

	breaker := New(serviceName, m.config)
	actual, loaded := m.breakers.LoadOrStore(serviceName, breaker)
	if loaded {
		return actual.(*Breaker)
	}
	return breaker
}

// GetAllBreakers returns metrics for all managed circuit breakers.
func (m *Manager) GetAllBreakers() map[string]Metrics {
	result := make(map[string]Metrics)
	m.breakers.Range(func(key, value interface{}) bool {
		breaker := value.(*Breaker)
		result[breaker.Name()] = breaker.GetState()
		return true
	})
	return result
}

// ResetAll resets all circuit breakers to Closed state.
func (m *Manager) ResetAll() {
	m.breakers.Range(func(key, value interface{}) bool {
		breaker := value.(*Breaker)
		breaker.Reset()
		return true
	})
}
