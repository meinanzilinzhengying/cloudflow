//go:build linux

package resilience

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// P8 故障自动恢复机制
// 解决：服务崩溃后缺少自动重试和熔断恢复、DB 连接断开后缺少重连和降级、
//       MQ 积压时缺少流控和背压、节点故障时缺少流量自动切换
// ============================================================================

// ============================================================================
// 1. 重试策略（指数退避）
// ============================================================================

// RetryPolicy 重试策略配置
type RetryPolicy struct {
	MaxRetries      int
	BaseInterval    time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	JitterFactor    float64
	RetryableErrors []error // 可重试的错误列表（nil 表示所有错误都重试）
}

// DefaultRetryPolicy 返回默认重试策略
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:   3,
		BaseInterval: 100 * time.Millisecond,
		MaxInterval:  10 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.1,
	}
}

// RetryableFunc 可重试的函数签名
type RetryableFunc func() error

// DoRetry 执行带重试的操作
func DoRetry(ctx context.Context, policy *RetryPolicy, fn RetryableFunc) error {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}
	
	var lastErr error
	for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			if attempt == policy.MaxRetries {
				break
			}
		}
		
		// 计算退避时间
		delay := calculateBackoff(policy, attempt)
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", policy.MaxRetries, lastErr)
}

func calculateBackoff(policy *RetryPolicy, attempt int) time.Duration {
	backoff := float64(policy.BaseInterval) * math.Pow(policy.Multiplier, float64(attempt))
	if backoff > float64(policy.MaxInterval) {
		backoff = float64(policy.MaxInterval)
	}
	
	// 添加 jitter（在 0 到 jitterFactor * backoff 之间随机）
	if policy.JitterFactor > 0 && policy.JitterFactor < 1 {
		jitter := backoff * policy.JitterFactor * (float64(time.Now().UnixNano()%1000) / 1000.0)
		backoff += jitter
	}
	
	// 最终确保不超过 MaxInterval
	if backoff > float64(policy.MaxInterval) {
		backoff = float64(policy.MaxInterval)
	}
	
	return time.Duration(backoff)
}

// ============================================================================
// 2. 熔断器自动恢复（半开状态探测）
// ============================================================================

// CircuitBreakerState 熔断器状态
type CircuitBreakerState int32

const (
	StateClosed CircuitBreakerState = iota   // 关闭（正常通过）
	StateOpen                                  // 打开（拒绝请求）
	StateHalfOpen                              // 半开（试探恢复）
)

func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	FailureThreshold    int           // 触发熔断的失败次数
	SuccessThreshold    int           // 半开状态恢复的成功次数
	TimeoutDuration     time.Duration // 熔断后等待多久进入半开
	RecoveryInterval    time.Duration // 半开状态探测间隔
}

// DefaultCircuitBreakerConfig 返回默认熔断器配置
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		TimeoutDuration:  30 * time.Second,
		RecoveryInterval: 5 * time.Second,
	}
}

// CircuitBreaker 自动恢复熔断器
type CircuitBreaker struct {
	config *CircuitBreakerConfig
	
	mu           sync.RWMutex
	state        int32 // CircuitBreakerState
	failures     int32
	successes    int32
	lastFailTime time.Time
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreaker{
		config:    config,
		state:     int32(StateClosed),
		failures:  0,
		successes: 0,
	}
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return CircuitBreakerState(atomic.LoadInt32(&cb.state))
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		atomic.StoreInt32(&cb.failures, 0)
	case StateHalfOpen:
		count := atomic.AddInt32(&cb.successes, 1)
		if int(count) >= cb.config.SuccessThreshold {
			cb.transitionTo(StateClosed)
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		count := atomic.AddInt32(&cb.failures, 1)
		if int(count) >= cb.config.FailureThreshold {
			cb.transitionTo(StateOpen)
		}
	case StateHalfOpen:
		cb.transitionTo(StateOpen)
	}
}

// transitionTo 状态转换
func (cb *CircuitBreaker) transitionTo(state CircuitBreakerState) {
	atomic.StoreInt32(&cb.state, int32(state))
	atomic.StoreInt32(&cb.failures, 0)
	atomic.StoreInt32(&cb.successes, 0)
	if state == StateOpen {
		cb.lastFailTime = time.Now()
	}
}

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() bool {
	state := CircuitBreakerState(atomic.LoadInt32(&cb.state))
	
	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// 检查是否超时进入半开
		cb.mu.Lock()
		if time.Since(cb.lastFailTime) > cb.config.TimeoutDuration {
			cb.transitionTo(StateHalfOpen)
			cb.mu.Unlock()
			return true
		}
		cb.mu.Unlock()
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// ============================================================================
// 3. DB 连接恢复与降级
// ============================================================================

// DBConnectionState 数据库连接状态
type DBConnectionState int

const (
	DBConnected DBConnectionState = iota
	DBDisconnected
	DBDegraded
)

// DBConnectionManager 数据库连接管理器
type DBConnectionManager struct {
	reconnectInterval time.Duration
	maxReconnects       int
	degradedTimeout     time.Duration
	
	mu          sync.RWMutex
	state       DBConnectionState
	reconnects  int
	lastConnect time.Time
	
	connectFn    func() error
	disconnectFn func() error
}

// NewDBConnectionManager 创建 DB 连接管理器
func NewDBConnectionManager(reconnectInterval time.Duration, maxReconnects int) *DBConnectionManager {
	return &DBConnectionManager{
		reconnectInterval: reconnectInterval,
		maxReconnects:     maxReconnects,
		degradedTimeout:   10 * time.Second,
		state:             DBDisconnected,
	}
}

// SetConnectFunc 设置连接函数
func (dbm *DBConnectionManager) SetConnectFunc(fn func() error) {
	dbm.mu.Lock()
	defer dbm.mu.Unlock()
	dbm.connectFn = fn
}

// SetDisconnectFunc 设置断开函数
func (dbm *DBConnectionManager) SetDisconnectFunc(fn func() error) {
	dbm.mu.Lock()
	defer dbm.mu.Unlock()
	dbm.disconnectFn = fn
}

// GetState 获取当前状态
func (dbm *DBConnectionManager) GetState() DBConnectionState {
	dbm.mu.RLock()
	defer dbm.mu.RUnlock()
	return dbm.state
}

// Connect 尝试连接
func (dbm *DBConnectionManager) Connect() error {
	dbm.mu.Lock()
	defer dbm.mu.Unlock()
	
	if dbm.state == DBConnected {
		return nil
	}
	
	if dbm.connectFn == nil {
		return fmt.Errorf("connect function not set")
	}
	
	for i := 0; i <= dbm.maxReconnects; i++ {
		if err := dbm.connectFn(); err == nil {
			dbm.state = DBConnected
			dbm.reconnects = 0
			dbm.lastConnect = time.Now()
			return nil
		} else if i < dbm.maxReconnects {
			time.Sleep(dbm.reconnectInterval)
		}
	}
	
	dbm.state = DBDisconnected
	dbm.reconnects = dbm.maxReconnects
	return fmt.Errorf("failed to connect after %d retries", dbm.maxReconnects)
}

// HandleDisconnect 处理连接断开
func (dbm *DBConnectionManager) HandleDisconnect() {
	dbm.mu.Lock()
	defer dbm.mu.Unlock()
	
	if dbm.state == DBConnected {
		dbm.state = DBDegraded
	}
	
	// 尝试重连
	go dbm.autoReconnect()
}

// autoReconnect 自动重连
func (dbm *DBConnectionManager) autoReconnect() {
	for i := 0; i < dbm.maxReconnects; i++ {
		time.Sleep(dbm.reconnectInterval)
		
		dbm.mu.Lock()
		if dbm.state == DBConnected {
			dbm.mu.Unlock()
			return
		}
		if dbm.connectFn == nil {
			dbm.mu.Unlock()
			return
		}
		fn := dbm.connectFn
		dbm.mu.Unlock()
		
		if err := fn(); err == nil {
			dbm.mu.Lock()
			dbm.state = DBConnected
			dbm.reconnects = 0
			dbm.lastConnect = time.Now()
			dbm.mu.Unlock()
			return
		}
		
		dbm.mu.Lock()
		dbm.reconnects++
		dbm.mu.Unlock()
	}
	
	dbm.mu.Lock()
	dbm.state = DBDisconnected
	dbm.mu.Unlock()
}

// IsHealthy 检查是否健康
func (dbm *DBConnectionManager) IsHealthy() bool {
	return dbm.GetState() == DBConnected
}

// IsDegraded 检查是否降级
func (dbm *DBConnectionManager) IsDegraded() bool {
	return dbm.GetState() == DBDegraded
}

// ============================================================================
// 4. 消息队列背压与流控
// ============================================================================

// BackpressureConfig 背压配置
type BackpressureConfig struct {
	LowWatermark      int64         // 低水位线（恢复阈值）
	HighWatermark     int64         // 高水位线（触发阈值）
	CriticalWatermark int64         // 临界水位线（最大阈值）
	CheckInterval     time.Duration // 检查间隔
	ReduceRate        float64       // 超过高水位后的限流比例（0-1）
}

// DefaultBackpressureConfig 返回默认背压配置
func DefaultBackpressureConfig() *BackpressureConfig {
	return &BackpressureConfig{
		LowWatermark:      100,
		HighWatermark:     500,
		CriticalWatermark: 1000,
		CheckInterval:     1 * time.Second,
		ReduceRate:        0.5,
	}
}

// BackpressureLevel 背压级别
type BackpressureLevel int

const (
	LevelNormal BackpressureLevel = iota
	LevelWarning
	LevelHigh
	LevelCritical
)

// BackpressureManager 背压管理器
type BackpressureManager struct {
	config *BackpressureConfig
	
	mu          sync.RWMutex
	queueDepth  int64
	level       BackpressureLevel
	reduceRate  float64
	paused      bool
	
	listeners []func(BackpressureLevel)
}

// NewBackpressureManager 创建背压管理器
func NewBackpressureManager(config *BackpressureConfig) *BackpressureManager {
	if config == nil {
		config = DefaultBackpressureConfig()
	}
	return &BackpressureManager{
		config:     config,
		level:      LevelNormal,
		reduceRate: 1.0,
		listeners:  make([]func(BackpressureLevel), 0),
	}
}

// UpdateQueueDepth 更新队列深度
func (bm *BackpressureManager) UpdateQueueDepth(depth int64) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	
	bm.queueDepth = depth
	oldLevel := bm.level
	
	// 根据水位线判断背压级别
	switch {
	case depth >= bm.config.CriticalWatermark:
		bm.level = LevelCritical
		bm.reduceRate = 0.1
		bm.paused = true
	case depth >= bm.config.HighWatermark:
		bm.level = LevelHigh
		bm.reduceRate = bm.config.ReduceRate
		bm.paused = false
	case depth >= bm.config.LowWatermark:
		bm.level = LevelWarning
		bm.reduceRate = 0.8
		bm.paused = false
	default:
		bm.level = LevelNormal
		bm.reduceRate = 1.0
		bm.paused = false
	}
	
	// 通知监听器
	if oldLevel != bm.level {
		for _, fn := range bm.listeners {
			go fn(bm.level)
		}
	}
}

// GetLevel 获取当前背压级别
func (bm *BackpressureManager) GetLevel() BackpressureLevel {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.level
}

// GetReduceRate 获取当前限流比例
func (bm *BackpressureManager) GetReduceRate() float64 {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.reduceRate
}

// IsPaused 检查是否暂停
func (bm *BackpressureManager) IsPaused() bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.paused
}

// GetQueueDepth 获取队列深度
func (bm *BackpressureManager) GetQueueDepth() int64 {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.queueDepth
}

// AddListener 添加背压级别变化监听器
func (bm *BackpressureManager) AddListener(fn func(BackpressureLevel)) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.listeners = append(bm.listeners, fn)
}

// ShouldProcess 检查是否允许处理消息
func (bm *BackpressureManager) ShouldProcess() bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return !bm.paused && bm.reduceRate > 0
}

// ============================================================================
// 5. 节点故障自动切换
// ============================================================================

// NodeState 节点状态
type NodeState int

const (
	NodeHealthy NodeState = iota
	NodeUnhealthy
	NodeRecovering
)

// FailoverNode 故障切换节点
type FailoverNode struct {
	ID       string
	Address  string
	State    NodeState
	Weight   float64
	Priority int
}

// FailoverManager 故障切换管理器
type FailoverManager struct {
	healthCheckInterval time.Duration
	recoveryThreshold   int
	
	mu          sync.RWMutex
	nodes       map[string]*FailoverNode
	activeNode  string
	healthyNodes []string
	
	healthCheckFn func(node *FailoverNode) bool
}

// NewFailoverManager 创建故障切换管理器
func NewFailoverManager(healthCheckInterval time.Duration) *FailoverManager {
	return &FailoverManager{
		healthCheckInterval: healthCheckInterval,
		recoveryThreshold:   3,
		nodes:               make(map[string]*FailoverNode),
		healthyNodes:        make([]string, 0),
	}
}

// SetHealthCheckFunc 设置健康检查函数
func (fm *FailoverManager) SetHealthCheckFunc(fn func(node *FailoverNode) bool) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.healthCheckFn = fn
}

// RegisterNode 注册节点
func (fm *FailoverManager) RegisterNode(node *FailoverNode) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.nodes[node.ID] = node
	if node.State == NodeHealthy {
		fm.healthyNodes = append(fm.healthyNodes, node.ID)
		if fm.activeNode == "" {
			fm.activeNode = node.ID
		}
	}
}

// RemoveNode 移除节点
func (fm *FailoverManager) RemoveNode(nodeID string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	delete(fm.nodes, nodeID)
	fm.updateHealthyNodes()
}

// GetActiveNode 获取当前活跃节点
func (fm *FailoverManager) GetActiveNode() *FailoverNode {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	if fm.activeNode == "" {
		return nil
	}
	return fm.nodes[fm.activeNode]
}

// GetHealthyNodes 获取健康节点列表
func (fm *FailoverManager) GetHealthyNodes() []*FailoverNode {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	
	result := make([]*FailoverNode, 0, len(fm.healthyNodes))
	for _, id := range fm.healthyNodes {
		if node, ok := fm.nodes[id]; ok && node.State == NodeHealthy {
			result = append(result, node)
		}
	}
	return result
}

// PerformHealthCheck 执行健康检查并切换
func (fm *FailoverManager) PerformHealthCheck() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	
	if fm.healthCheckFn == nil {
		return
	}
	
	for _, node := range fm.nodes {
		isHealthy := fm.healthCheckFn(node)
		
		if isHealthy {
			if node.State == NodeUnhealthy {
				node.State = NodeRecovering
			} else if node.State == NodeRecovering {
				node.State = NodeHealthy
			}
		} else {
			node.State = NodeUnhealthy
			if fm.activeNode == node.ID {
				// 切换活跃节点
				fm.switchActiveNode()
			}
		}
	}
	
	fm.updateHealthyNodes()
}

// switchActiveNode 切换到新的活跃节点
func (fm *FailoverManager) switchActiveNode() {
	for _, node := range fm.nodes {
		if node.State == NodeHealthy && node.ID != fm.activeNode {
			fm.activeNode = node.ID
			return
		}
	}
	// 没有健康节点
	fm.activeNode = ""
}

// updateHealthyNodes 更新健康节点列表
func (fm *FailoverManager) updateHealthyNodes() {
	fm.healthyNodes = make([]string, 0)
	for id, node := range fm.nodes {
		if node.State == NodeHealthy {
			fm.healthyNodes = append(fm.healthyNodes, id)
		}
	}
}

// StartHealthCheck 启动健康检查循环
func (fm *FailoverManager) StartHealthCheck(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(fm.healthCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fm.PerformHealthCheck()
			}
		}
	}()
}

// GetNodeState 获取节点状态
func (fm *FailoverManager) GetNodeState(nodeID string) NodeState {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	if node, ok := fm.nodes[nodeID]; ok {
		return node.State
	}
	return NodeUnhealthy
}

// GetNodeCount 获取节点总数
func (fm *FailoverManager) GetNodeCount() int {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return len(fm.nodes)
}
