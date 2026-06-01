// Package runtime 提供运行时管理功能
// 包括资源监控（CPU/内存/协程数）、熔断器、速率限制器和优雅关闭
package runtime

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// ============================================================================
// 资源监控
// ============================================================================

// ResourceStats 资源使用统计
type ResourceStats struct {
	Timestamp    time.Time // 采集时间
	GoroutineNum int       // 当前协程数
	MemoryMB     float64   // 内存使用量（MB）
	CPUUsage     float64   // CPU 使用率（百分比）
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	log         *logger.Logger
	stats       ResourceStats
	mu          sync.RWMutex
	stopCh      chan struct{}
	interval    time.Duration
	onExceed    func(stats ResourceStats) // 资源超限回调
	maxGoroutine int                      // 最大协程数阈值
	maxMemoryMB  float64                  // 最大内存阈值（MB）
}

// ResourceMonitorConfig 资源监控配置
type ResourceMonitorConfig struct {
	Interval     time.Duration       // 采集间隔
	MaxGoroutine int                 // 最大协程数阈值（0 表示不限制）
	MaxMemoryMB  float64             // 最大内存阈值 MB（0 表示不限制）
	OnExceed     func(stats ResourceStats) // 资源超限回调
}

// DefaultResourceMonitorConfig 默认资源监控配置
func DefaultResourceMonitorConfig() ResourceMonitorConfig {
	return ResourceMonitorConfig{
		Interval:     10 * time.Second,
		MaxGoroutine: 10000,
		MaxMemoryMB:  512,
	}
}

// NewResourceMonitor 创建资源监控器
func NewResourceMonitor(log *logger.Logger, cfg ResourceMonitorConfig) *ResourceMonitor {
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	return &ResourceMonitor{
		log:          log,
		stopCh:       make(chan struct{}),
		interval:     cfg.Interval,
		onExceed:     cfg.OnExceed,
		maxGoroutine: cfg.MaxGoroutine,
		maxMemoryMB:  cfg.MaxMemoryMB,
	}
}

// Start 启动资源监控
func (m *ResourceMonitor) Start() {
	go m.monitorLoop()
	m.log.Infof("资源监控已启动，间隔: %s", m.interval)
}

// Stop 停止资源监控
func (m *ResourceMonitor) Stop() {
	close(m.stopCh)
	m.log.Info("资源监控已停止")
}

// GetStats 获取最新资源统计
func (m *ResourceMonitor) GetStats() ResourceStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// monitorLoop 监控循环
func (m *ResourceMonitor) monitorLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.collect()
		case <-m.stopCh:
			return
		}
	}
}

// collect 采集资源使用情况
func (m *ResourceMonitor) collect() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	stats := ResourceStats{
		Timestamp:    time.Now(),
		GoroutineNum: runtime.NumGoroutine(),
		MemoryMB:     float64(memStats.Alloc) / 1024 / 1024,
	}

	m.mu.Lock()
	m.stats = stats
	m.mu.Unlock()

	// 检查资源是否超限
	exceeded := false

	if m.maxGoroutine > 0 && stats.GoroutineNum > m.maxGoroutine {
		m.log.Warnf("协程数超限: 当前 %d，阈值 %d", stats.GoroutineNum, m.maxGoroutine)
		exceeded = true
	}

	if m.maxMemoryMB > 0 && stats.MemoryMB > m.maxMemoryMB {
		m.log.Warnf("内存使用超限: 当前 %.2f MB，阈值 %.2f MB", stats.MemoryMB, m.maxMemoryMB)
		exceeded = true
	}

	if exceeded && m.onExceed != nil {
		m.onExceed(stats)
	}
}

// ============================================================================
// 熔断器 (Circuit Breaker)
// ============================================================================

// CircuitState 熔断器状态
type CircuitState int

const (
	StateClosed   CircuitState = iota // 正常状态（关闭）
	StateOpen                          // 熔断状态（打开）
	StateHalfOpen                      // 半开状态（试探）
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreaker 熔断器
// 实现经典的 Circuit Breaker 模式：
// - Closed: 正常状态，请求通过
// - Open: 熔断状态，请求被拒绝
// - HalfOpen: 半开状态，允许少量请求通过以探测恢复
type CircuitBreaker struct {
	name          string
	log           *logger.Logger
	mu            sync.RWMutex
	state         CircuitState
	failures      int            // 连续失败次数
	successes     int            // 连续成功次数（半开状态下使用）
	maxFailures   int            // 最大连续失败次数
	resetTimeout  time.Duration  // 熔断恢复超时
	halfOpenMax   int            // 半开状态最大允许请求数
	lastFailTime  time.Time      // 最后一次失败时间
	onStateChange func(from, to CircuitState) // 状态变更回调
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Name          string
	MaxFailures   int           // 最大连续失败次数（触发熔断）
	ResetTimeout  time.Duration // 熔断恢复超时
	HalfOpenMax   int           // 半开状态最大允许请求数
	OnStateChange func(from, to CircuitState)
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:         name,
		MaxFailures:  5,
		ResetTimeout: 30 * time.Second,
		HalfOpenMax:  3,
	}
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(log *logger.Logger, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 5
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}
	if cfg.HalfOpenMax <= 0 {
		cfg.HalfOpenMax = 3
	}
	return &CircuitBreaker{
		name:          cfg.Name,
		log:           log,
		state:         StateClosed,
		maxFailures:   cfg.MaxFailures,
		resetTimeout:  cfg.ResetTimeout,
		halfOpenMax:   cfg.HalfOpenMax,
		onStateChange: cfg.OnStateChange,
	}
}

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true

	case StateOpen:
		// 检查是否超过恢复超时
		if time.Since(cb.lastFailTime) >= cb.resetTimeout {
			cb.setState(StateHalfOpen)
			cb.successes = 0
			return true
		}
		return false

	case StateHalfOpen:
		// 半开状态下允许有限请求通过
		if cb.successes < cb.halfOpenMax {
			return true
		}
		return false
	}

	return false
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failures = 0

	case StateHalfOpen:
		cb.successes++
		cb.failures = 0
		if cb.successes >= cb.halfOpenMax {
			cb.setState(StateClosed)
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failures++
		cb.lastFailTime = time.Now()
		if cb.failures >= cb.maxFailures {
			cb.setState(StateOpen)
		}

	case StateHalfOpen:
		cb.failures++
		cb.lastFailTime = time.Now()
		cb.setState(StateOpen)
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Name 获取熔断器名称
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// setState 设置状态（内部方法，调用前必须持有锁）
func (cb *CircuitBreaker) setState(newState CircuitState) {
	oldState := cb.state
	if oldState == newState {
		return
	}
	cb.state = newState
	cb.log.Infof("熔断器 [%s] 状态变更: %s -> %s", cb.name, oldState, newState)

	if cb.onStateChange != nil {
		// 在锁外调用回调，避免死锁
		callback := cb.onStateChange
		cb.mu.Unlock()
		callback(oldState, newState)
		cb.mu.Lock()
	}
}

// ============================================================================
// 速率限制器 (Rate Limiter)
// ============================================================================

// RateLimiter 令牌桶速率限制器
type RateLimiter struct {
	name      string
	log       *logger.Logger
	mu        sync.Mutex
	rate      float64   // 每秒产生的令牌数
	burst     int       // 桶容量
	tokens    float64   // 当前令牌数
	lastTime  time.Time // 上次更新时间
}

// RateLimiterConfig 速率限制器配置
type RateLimiterConfig struct {
	Name  string
	Rate  float64 // 每秒产生的令牌数
	Burst int     // 桶容量（最大突发请求数）
}

// DefaultRateLimiterConfig 默认速率限制器配置
func DefaultRateLimiterConfig(name string) RateLimiterConfig {
	return RateLimiterConfig{
		Name:  name,
		Rate:  100,
		Burst: 200,
	}
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(log *logger.Logger, cfg RateLimiterConfig) *RateLimiter {
	if cfg.Rate <= 0 {
		cfg.Rate = 100
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 200
	}
	return &RateLimiter{
		name:     cfg.Name,
		log:      log,
		rate:     cfg.Rate,
		burst:    cfg.Burst,
		tokens:   float64(cfg.Burst), // 初始填满
		lastTime: time.Now(),
	}
}

// Allow 检查是否允许请求通过
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now

	// 补充令牌
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burst) {
		rl.tokens = float64(rl.burst)
	}

	// 消耗令牌
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}

	return false
}

// Wait 等待直到有可用令牌
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// 短暂等待后重试
		}
	}
}

// Name 获取速率限制器名称
func (rl *RateLimiter) Name() string {
	return rl.name
}

// ============================================================================
// 运行时管理器 (Runtime Manager)
// ============================================================================

// Manager 运行时管理器
// 统一管理资源监控、熔断器和速率限制器
type Manager struct {
	log              *logger.Logger
	resourceMonitor  *ResourceMonitor
	circuitBreakers  map[string]*CircuitBreaker
	rateLimiters     map[string]*RateLimiter
	mu               sync.RWMutex
	shutdownCallbacks []func(ctx context.Context) error
}

// ManagerConfig 运行时管理器配置
type ManagerConfig struct {
	ResourceMonitor ResourceMonitorConfig
}

// DefaultManagerConfig 默认管理器配置
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		ResourceMonitor: DefaultResourceMonitorConfig(),
	}
}

// NewManager 创建运行时管理器
func NewManager(log *logger.Logger, cfg ManagerConfig) *Manager {
	m := &Manager{
		log:              log,
		circuitBreakers:  make(map[string]*CircuitBreaker),
		rateLimiters:     make(map[string]*RateLimiter),
		shutdownCallbacks: make([]func(ctx context.Context) error, 0),
	}

	// 初始化资源监控器
	m.resourceMonitor = NewResourceMonitor(log, cfg.ResourceMonitor)

	// 设置默认的资源超限回调
	cfg.ResourceMonitor.OnExceed = func(stats ResourceStats) {
		log.Warnf("资源超限: goroutines=%d, memory=%.2fMB", stats.GoroutineNum, stats.MemoryMB)
	}

	return m
}

// Start 启动运行时管理器
func (m *Manager) Start() {
	m.resourceMonitor.Start()
	m.log.Info("运行时管理器已启动")
}

// Stop 停止运行时管理器
func (m *Manager) Stop() {
	m.resourceMonitor.Stop()
	m.log.Info("运行时管理器已停止")
}

// GetResourceStats 获取资源统计
func (m *Manager) GetResourceStats() ResourceStats {
	return m.resourceMonitor.GetStats()
}

// AddCircuitBreaker 添加熔断器
func (m *Manager) AddCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	cb := NewCircuitBreaker(m.log, cfg)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.circuitBreakers[cfg.Name] = cb
	m.log.Infof("已注册熔断器: %s (maxFailures=%d, resetTimeout=%s)",
		cfg.Name, cfg.MaxFailures, cfg.ResetTimeout)
	return cb
}

// GetCircuitBreaker 获取熔断器
func (m *Manager) GetCircuitBreaker(name string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.circuitBreakers[name]
}

// AddRateLimiter 添加速率限制器
func (m *Manager) AddRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	rl := NewRateLimiter(m.log, cfg)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimiters[cfg.Name] = rl
	m.log.Infof("已注册速率限制器: %s (rate=%.0f/s, burst=%d)",
		cfg.Name, cfg.Rate, cfg.Burst)
	return rl
}

// GetRateLimiter 获取速率限制器
func (m *Manager) GetRateLimiter(name string) *RateLimiter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rateLimiters[name]
}

// RegisterShutdown 注册优雅关闭回调
func (m *Manager) RegisterShutdown(name string, callback func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCallbacks = append(m.shutdownCallbacks, callback)
	m.log.Infof("已注册关闭回调: %s", name)
}

// GracefulShutdown 优雅关闭
// 按照注册顺序依次执行关闭回调，每个回调有独立的超时控制
func (m *Manager) GracefulShutdown(ctx context.Context) error {
	m.log.Info("开始优雅关闭...")

	m.mu.RLock()
	callbacks := make([]func(ctx context.Context) error, len(m.shutdownCallbacks))
	copy(callbacks, m.shutdownCallbacks)
	m.mu.RUnlock()

	for i, callback := range callbacks {
		select {
		case <-ctx.Done():
			return fmt.Errorf("优雅关闭超时，已完成 %d/%d 个回调", i, len(callbacks))
		default:
		}

		// 每个回调最多 10 秒
		cbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := callback(cbCtx); err != nil {
			m.log.Warnf("关闭回调 %d 执行失败: %v", i, err)
		} else {
			m.log.Debugf("关闭回调 %d 执行完成", i)
		}
		cancel()
	}

	m.log.Info("优雅关闭完成")
	return nil
}

// Status 返回运行时状态摘要
func (m *Manager) Status() string {
	stats := m.resourceMonitor.GetStats()

	m.mu.RLock()
	cbCount := len(m.circuitBreakers)
	rlCount := len(m.rateLimiters)
	m.mu.RUnlock()

	return fmt.Sprintf("Goroutines=%d, Memory=%.2fMB, CircuitBreakers=%d, RateLimiters=%d",
		stats.GoroutineNum, stats.MemoryMB, cbCount, rlCount)
}
