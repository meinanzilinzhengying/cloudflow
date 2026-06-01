package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type FallbackFunc func(err error) interface{}
type FallbackFuncWithContext func(ctx context.Context, err error) interface{}

type FallbackHandler struct {
	fallbacks map[string]FallbackFunc
	mu        sync.RWMutex

	timeout       time.Duration
	fallbackCache map[string]*cachedFallback
	cacheMu       sync.RWMutex
}

type cachedFallback struct {
	value      interface{}
	expiry     time.Time
	isStale    bool
}

type FallbackConfig struct {
	Timeout time.Duration
}

func DefaultFallbackConfig() FallbackConfig {
	return FallbackConfig{
		Timeout: 5 * time.Second,
	}
}

func NewFallbackHandler(config FallbackConfig) *FallbackHandler {
	return &FallbackHandler{
		fallbacks:    make(map[string]FallbackFunc),
		timeout:      config.Timeout,
		fallbackCache: make(map[string]*cachedFallback),
	}
}

func (h *FallbackHandler) Register(name string, fn FallbackFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fallbacks[name] = fn
}

func (h *FallbackHandler) RegisterWithContext(name string, fn FallbackFuncWithContext) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fallbacks[name] = func(err error) interface{} {
		ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
		defer cancel()
		return fn(ctx, err)
	}
}

func (h *FallbackHandler) Execute(name string, err error) (interface{}, error) {
	h.mu.RLock()
	fn, exists := h.fallbacks[name]
	h.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("fallback %s not registered", name)
	}

	result := fn(err)
	return result, nil
}

func (h *FallbackHandler) ExecuteWithCache(name string, err error, ttl time.Duration) (interface{}, error) {
	if cached := h.getCachedFallback(name); cached != nil {
		return cached.value, nil
	}

	result, err := h.Execute(name, err)
	if err != nil {
		return nil, err
	}

	h.setCachedFallback(name, result, ttl)
	return result, nil
}

func (h *FallbackHandler) getCachedFallback(name string) *cachedFallback {
	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()

	if cached, exists := h.cacheCache[name]; exists {
		if time.Now().Before(cached.expiry) {
			return cached
		}
	}

	return nil
}

func (h *FallbackHandler) setCachedFallback(name string, value interface{}, ttl time.Duration) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	h.cacheCache[name] = &cachedFallback{
		value:  value,
		expiry: time.Now().Add(ttl),
	}
}

func (h *FallbackHandler) InvalidateCache(name string) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	delete(h.cacheCache, name)
}

func (h *FallbackHandler) ClearCache() {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	h.cacheCache = make(map[string]*cachedFallback)
}

type DegradationLevel int

const (
	DegradationNone DegradationLevel = iota
	DegradationGraceful
	DegradationMinimal
	DegradationFallback
)

func (d DegradationLevel) String() string {
	switch d {
	case DegradationNone:
		return "none"
	case DegradationGraceful:
		return "graceful"
	case DegradationMinimal:
		return "minimal"
	case DegradationFallback:
		return "fallback"
	default:
		return "unknown"
	}
}

type DegradationHandler struct {
	level       DegradationLevel
	mu          sync.RWMutex

	thresholds struct {
		ErrorRate     float64
		LatencyMs     int64
		TimeoutRatio  float64
	}

	onLevelChange func(from, to DegradationLevel)
}

func NewDegradationHandler() *DegradationHandler {
	return &DegradationHandler{
		level: DegradationNone,
	}
}

func (h *DegradationHandler) SetThresholds(errorRate, latencyMs int64, timeoutRatio float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.thresholds.ErrorRate = float64(errorRate) / 100
	h.thresholds.LatencyMs = latencyMs
	h.thresholds.TimeoutRatio = timeoutRatio
}

func (h *DegradationHandler) OnLevelChange(fn func(from, to DegradationLevel)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.onLevelChange = fn
}

func (h *DegradationHandler) GetLevel() DegradationLevel {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.level
}

func (h *DegradationHandler) SetLevel(level DegradationLevel) {
	h.mu.Lock()
	defer h.mu.Unlock()

	oldLevel := h.level
	h.level = level

	if oldLevel != level && h.onLevelChange != nil {
		go h.onLevelChange(oldLevel, level)
	}
}

func (h *DegradationHandler) Evaluate(errorRate float64, avgLatencyMs int64, timeoutRatio float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var newLevel DegradationLevel

	switch {
	case errorRate >= 0.5 || timeoutRatio >= 0.3:
		newLevel = DegradationFallback
	case errorRate >= 0.2 || avgLatencyMs >= h.thresholds.LatencyMs*2:
		newLevel = DegradationMinimal
	case errorRate >= 0.1 || avgLatencyMs >= h.thresholds.LatencyMs:
		newLevel = DegradationGraceful
	default:
		newLevel = DegradationNone
	}

	if newLevel != h.level {
		h.level = newLevel
		if h.onLevelChange != nil {
			go h.onLevelChange(DegradationNone, newLevel)
		}
	}
}

type Bulkhead struct {
	maxConcurrent int
	semaphore     chan struct{}

	activeCount  int64
	maxWaitTime  time.Duration

	mu sync.RWMutex
}

type BulkheadConfig struct {
	MaxConcurrent int
	MaxWaitTime   time.Duration
}

func DefaultBulkheadConfig() BulkheadConfig {
	return BulkheadConfig{
		MaxConcurrent: 100,
		MaxWaitTime:   5 * time.Second,
	}
}

func NewBulkhead(config BulkheadConfig) *Bulkhead {
	return &Bulkhead{
		maxConcurrent: config.MaxConcurrent,
		semaphore:     make(chan struct{}, config.MaxConcurrent),
		maxWaitTime:  config.MaxWaitTime,
	}
}

func (b *Bulkhead) Execute(fn func() error) error {
	return b.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		return fn()
	})
}

func (b *Bulkhead) ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	atomic.AddInt64(&b.activeCount, 1)
	defer atomic.AddInt64(&b.activeCount, -1)

	select {
	case b.semaphore <- struct{}{}:
		defer func() { <-b.semaphore }()
		return fn(ctx)
	case <-ctx.Done():
		return fmt.Errorf("bulkhead: context cancelled while waiting for semaphore: %w", ctx.Err())
	case <-time.After(b.maxWaitTime):
		return fmt.Errorf("bulkhead: timeout while waiting for semaphore after %v", b.maxWaitTime)
	}
}

func (b *Bulkhead) GetActiveCount() int64 {
	return atomic.LoadInt64(&b.activeCount)
}

func (b *Bulkhead) GetMaxConcurrent() int {
	return b.maxConcurrent
}

func (b *Bulkhead) IsAvailable() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	active := atomic.LoadInt64(&b.activeCount)
	return int(active) < b.maxConcurrent
}

func (b *Bulkhead) GetUtilization() float64 {
	active := atomic.LoadInt64(&b.activeCount)
	return float64(active) / float64(b.maxConcurrent) * 100
}

type BulkheadMetrics struct {
	MaxConcurrent   int     `json:"max_concurrent"`
	ActiveCount     int64   `json:"active_count"`
	Utilization     float64 `json:"utilization_percent"`
	IsAvailable     bool    `json:"is_available"`
	MaxWaitTimeMs   int64   `json:"max_wait_time_ms"`
}
