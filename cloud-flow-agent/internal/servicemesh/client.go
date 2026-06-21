//go:build linux

package servicemesh

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ============================================================================
// 一、负载均衡策略
// ============================================================================

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy string

const (
	LBStrategyRandom     LoadBalanceStrategy = "random"     // 随机
	LBStrategyRoundRobin LoadBalanceStrategy = "roundrobin" // 轮询
	LBStrategyWeight     LoadBalanceStrategy = "weight"     // 加权
	LBStrategyLeastConn  LoadBalanceStrategy = "leastconn"  // 最少连接
)

// LoadBalancer 负载均衡器
type LoadBalancer interface {
	Select(instances []*ServiceInstance) (*ServiceInstance, error)
	Strategy() LoadBalanceStrategy
}

// RandomLoadBalancer 随机负载均衡器
type RandomLoadBalancer struct{}

func NewRandomLoadBalancer() *RandomLoadBalancer {
	return &RandomLoadBalancer{}
}

func (rlb *RandomLoadBalancer) Strategy() LoadBalanceStrategy {
	return LBStrategyRandom
}

func (rlb *RandomLoadBalancer) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	healthy := filterHealthy(instances)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy instances")
	}
	return healthy[rand.Intn(len(healthy))], nil
}

// RoundRobinLoadBalancer 轮询负载均衡器
type RoundRobinLoadBalancer struct {
	mu    sync.Mutex
	index int
}

func NewRoundRobinLoadBalancer() *RoundRobinLoadBalancer {
	return &RoundRobinLoadBalancer{}
}

func (rlb *RoundRobinLoadBalancer) Strategy() LoadBalanceStrategy {
	return LBStrategyRoundRobin
}

func (rlb *RoundRobinLoadBalancer) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	healthy := filterHealthy(instances)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy instances")
	}
	
	rlb.mu.Lock()
	idx := rlb.index % len(healthy)
	rlb.index++
	rlb.mu.Unlock()
	
	return healthy[idx], nil
}

// WeightedLoadBalancer 加权负载均衡器
type WeightedLoadBalancer struct{}

func NewWeightedLoadBalancer() *WeightedLoadBalancer {
	return &WeightedLoadBalancer{}
}

func (wlb *WeightedLoadBalancer) Strategy() LoadBalanceStrategy {
	return LBStrategyWeight
}

func (wlb *WeightedLoadBalancer) Select(instances []*ServiceInstance) (*ServiceInstance, error) {
	healthy := filterHealthy(instances)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy instances")
	}
	
	totalWeight := 0
	for _, inst := range healthy {
		totalWeight += inst.Weight
	}
	if totalWeight <= 0 {
		return healthy[rand.Intn(len(healthy))], nil
	}
	
	r := rand.Intn(totalWeight)
	for _, inst := range healthy {
		r -= inst.Weight
		if r < 0 {
			return inst, nil
		}
	}
	return healthy[0], nil
}

func filterHealthy(instances []*ServiceInstance) []*ServiceInstance {
	var result []*ServiceInstance
	for _, inst := range instances {
		if inst.IsHealthy() {
			result = append(result, inst)
		}
	}
	return result
}

// LoadBalancerFactory 负载均衡器工厂
func LoadBalancerFactory(strategy LoadBalanceStrategy) LoadBalancer {
	switch strategy {
	case LBStrategyRoundRobin:
		return NewRoundRobinLoadBalancer()
	case LBStrategyWeight:
		return NewWeightedLoadBalancer()
	default:
		return NewRandomLoadBalancer()
	}
}

// ============================================================================
// 二、熔断器
// ============================================================================

// CircuitState 熔断器状态
type CircuitState string

const (
	CircuitStateClosed    CircuitState = "closed"    // 关闭：正常
	CircuitStateOpen      CircuitState = "open"      // 打开：熔断
	CircuitStateHalfOpen  CircuitState = "halfopen"  // 半开：试探
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold    int           // 失败阈值
	SuccessThreshold    int           // 成功阈值（半开状态）
	Timeout             time.Duration // 熔断超时时间
	HalfOpenMaxCalls    int           // 半开状态最大请求数
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	mu       sync.RWMutex
	name     string
	config   *CircuitBreakerConfig
	state    CircuitState
	failures int
	successes int
	lastFail time.Time
	calls    int
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(name string, config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreaker{
		name:   name,
		config: config,
		state:  CircuitStateClosed,
	}
}

// Name 返回名称
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// State 返回状态
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Allow 是否允许请求
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		if time.Since(cb.lastFail) > cb.config.Timeout {
			cb.state = CircuitStateHalfOpen
			cb.failures = 0
			cb.successes = 0
			cb.calls = 0
			return true
		}
		return false
	case CircuitStateHalfOpen:
		if cb.calls < cb.config.HalfOpenMaxCalls {
			cb.calls++
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
	case CircuitStateClosed:
		cb.failures = 0
	case CircuitStateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = CircuitStateClosed
			cb.failures = 0
			cb.successes = 0
			cb.calls = 0
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.lastFail = time.Now()
	
	switch cb.state {
	case CircuitStateClosed:
		cb.failures++
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitStateOpen
		}
	case CircuitStateHalfOpen:
		cb.state = CircuitStateOpen
		cb.failures = 0
		cb.successes = 0
		cb.calls = 0
	}
}

// GetStats 获取统计
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return map[string]interface{}{
		"name":     cb.name,
		"state":    string(cb.state),
		"failures": cb.failures,
		"successes": cb.successes,
		"calls":    cb.calls,
	}
}

// ============================================================================
// 三、重试策略
// ============================================================================

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries  int           // 最大重试次数
	BaseDelay   time.Duration // 基础延迟
	MaxDelay    time.Duration // 最大延迟
	Multiplier  float64       // 延迟倍数（指数退避）
	RetryableErrors []string  // 可重试错误（可选）
}

// DefaultRetryPolicy 默认重试策略
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Multiplier: 2.0,
	}
}

// CalculateDelay 计算延迟
func (rp *RetryPolicy) CalculateDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	
	delay := float64(rp.BaseDelay) * rp.Multiplier * float64(attempt-1)
	if delay > float64(rp.MaxDelay) {
		delay = float64(rp.MaxDelay)
	}
	// 添加 jitter
	jitter := time.Duration(rand.Float64() * float64(rp.BaseDelay))
	return time.Duration(delay) + jitter
}

// ============================================================================
// 四、服务客户端
// ============================================================================

// ServiceClient 服务客户端
type ServiceClient struct {
	name        string
	registry    Registry
	lb          LoadBalancer
	cb          *CircuitBreaker
	retryPolicy *RetryPolicy
	timeout     time.Duration
	mu          sync.RWMutex
	instances   []*ServiceInstance
	watchCh     chan []*ServiceInstance
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// NewServiceClient 创建服务客户端
func NewServiceClient(name string, registry Registry, strategy LoadBalanceStrategy) (*ServiceClient, error) {
	sc := &ServiceClient{
		name:        name,
		registry:    registry,
		lb:          LoadBalancerFactory(strategy),
		cb:          NewCircuitBreaker(name, nil),
		retryPolicy: DefaultRetryPolicy(),
		timeout:     10 * time.Second,
		stopCh:      make(chan struct{}),
	}
	
	// 初始获取实例
	instances, err := registry.GetService(name)
	if err != nil {
		return nil, err
	}
	sc.instances = instances
	
	// 监听变化
	watchCh, err := registry.Watch(name)
	if err != nil {
		return nil, err
	}
	sc.watchCh = watchCh
	
	go sc.watchLoop()
	
	return sc, nil
}

// SetTimeout 设置超时
func (sc *ServiceClient) SetTimeout(timeout time.Duration) {
	sc.timeout = timeout
}

// SetRetryPolicy 设置重试策略
func (sc *ServiceClient) SetRetryPolicy(policy *RetryPolicy) {
	sc.retryPolicy = policy
}

// SetCircuitBreaker 设置熔断器
func (sc *ServiceClient) SetCircuitBreaker(cb *CircuitBreaker) {
	sc.cb = cb
}

// GetInstance 获取一个实例（负载均衡）
func (sc *ServiceClient) GetInstance() (*ServiceInstance, error) {
	sc.mu.RLock()
	instances := make([]*ServiceInstance, len(sc.instances))
	copy(instances, sc.instances)
	sc.mu.RUnlock()
	
	return sc.lb.Select(instances)
}

// GetAllInstances 获取所有实例
func (sc *ServiceClient) GetAllInstances() []*ServiceInstance {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	result := make([]*ServiceInstance, len(sc.instances))
	copy(result, sc.instances)
	return result
}

// Execute 执行请求（带重试和熔断）
func (sc *ServiceClient) Execute(fn func(*ServiceInstance) error) error {
	if !sc.cb.Allow() {
		return fmt.Errorf("circuit breaker open for service: %s", sc.name)
	}
	
	var lastErr error
	for i := 0; i <= sc.retryPolicy.MaxRetries; i++ {
		inst, err := sc.GetInstance()
		if err != nil {
			return err
		}
		
		if err := fn(inst); err != nil {
			lastErr = err
			sc.cb.RecordFailure()
			
			if i < sc.retryPolicy.MaxRetries {
				time.Sleep(sc.retryPolicy.CalculateDelay(i + 1))
			}
			continue
		}
		
		sc.cb.RecordSuccess()
		return nil
	}
	
	return fmt.Errorf("all retries failed: %w", lastErr)
}

// watchLoop 监听实例变化
func (sc *ServiceClient) watchLoop() {
	for {
		select {
		case <-sc.stopCh:
			return
		case instances := <-sc.watchCh:
			sc.mu.Lock()
			sc.instances = instances
			sc.mu.Unlock()
		}
	}
}

// Stop 停止客户端
func (sc *ServiceClient) Stop() {
	sc.stopOnce.Do(func() {
		close(sc.stopCh)
	})
}

// GetStats 获取统计
func (sc *ServiceClient) GetStats() map[string]interface{} {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	return map[string]interface{}{
		"service_name":     sc.name,
		"instance_count":   len(sc.instances),
		"load_balance":     string(sc.lb.Strategy()),
		"circuit_breaker":  sc.cb.GetStats(),
		"retry_policy": map[string]interface{}{
			"max_retries": sc.retryPolicy.MaxRetries,
			"base_delay":  sc.retryPolicy.BaseDelay.String(),
		},
	}
}
