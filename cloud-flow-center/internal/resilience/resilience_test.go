//go:build linux

package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryPolicyExponentialBackoff(t *testing.T) {
	policy := DefaultRetryPolicy()
	
	// 测试计算退避时间
	d1 := calculateBackoff(policy, 0)
	d2 := calculateBackoff(policy, 1)
	d3 := calculateBackoff(policy, 2)
	
	if d1 >= d2 {
		t.Errorf("expected backoff to increase: d1=%v d2=%v", d1, d2)
	}
	if d2 >= d3 {
		t.Errorf("expected backoff to increase: d2=%v d3=%v", d2, d3)
	}
	
	// 不超过最大值
	max := calculateBackoff(policy, 10)
	if max > policy.MaxInterval {
		t.Errorf("backoff %v exceeds max %v", max, policy.MaxInterval)
	}
}

func TestDoRetrySuccess(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:   3,
		BaseInterval: 10 * time.Millisecond,
		MaxInterval:  1 * time.Second,
		Multiplier:   2.0,
	}
	
	attempts := 0
	fn := func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}
	
	err := DoRetry(context.Background(), policy, fn)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoRetryExhausted(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:   2,
		BaseInterval: 10 * time.Millisecond,
		MaxInterval:  1 * time.Second,
		Multiplier:   2.0,
	}
	
	fn := func() error {
		return errors.New("persistent error")
	}
	
	err := DoRetry(context.Background(), policy, fn)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
}

func TestCircuitBreakerStateTransition(t *testing.T) {
	cb := NewCircuitBreaker(nil)
	
	// 初始状态为关闭
	if cb.GetState() != StateClosed {
		t.Errorf("expected initial state closed, got %v", cb.GetState())
	}
	
	// 记录5次失败 -> 打开
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	if cb.GetState() != StateOpen {
		t.Errorf("expected state open after 5 failures, got %v", cb.GetState())
	}
	
	// 打开状态下不允许请求
	if cb.Allow() {
		t.Error("expected Allow to be false when open")
	}
	
	// 模拟超时后进入半开
	cb.lastFailTime = time.Now().Add(-31 * time.Second)
	if !cb.Allow() {
		t.Error("expected Allow to be true after timeout (half-open)")
	}
	if cb.GetState() != StateHalfOpen {
		t.Errorf("expected state half-open after timeout, got %v", cb.GetState())
	}
	
	// 半开状态记录成功 -> 关闭
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordSuccess()
	if cb.GetState() != StateClosed {
		t.Errorf("expected state closed after 3 successes in half-open, got %v", cb.GetState())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(&CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		TimeoutDuration:  30 * time.Second,
	})
	
	// 触发熔断
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	if cb.GetState() != StateOpen {
		t.Fatal("expected open state")
	}
	
	// 进入半开
	cb.lastFailTime = time.Now().Add(-31 * time.Second)
	cb.Allow() // 触发状态转换
	
	// 半开状态再次失败 -> 重新打开
	cb.RecordFailure()
	if cb.GetState() != StateOpen {
		t.Errorf("expected state open after failure in half-open, got %v", cb.GetState())
	}
}

func TestDBConnectionManagerReconnect(t *testing.T) {
	attempts := 0
	dbm := NewDBConnectionManager(10*time.Millisecond, 3)
	dbm.SetConnectFunc(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	})
	
	err := dbm.Connect()
	if err != nil {
		t.Fatalf("expected connection after retries, got: %v", err)
	}
	if dbm.GetState() != DBConnected {
		t.Errorf("expected state connected, got %v", dbm.GetState())
	}
	if attempts < 3 {
		t.Errorf("expected at least 3 attempts, got %d", attempts)
	}
}

func TestDBConnectionManagerDegraded(t *testing.T) {
	dbm := NewDBConnectionManager(10*time.Millisecond, 2)
	dbm.SetConnectFunc(func() error {
		return errors.New("connection refused")
	})
	
	err := dbm.Connect()
	if err == nil {
		t.Fatal("expected connection failure")
	}
	if dbm.GetState() != DBDisconnected {
		t.Errorf("expected state disconnected, got %v", dbm.GetState())
	}
	if dbm.IsHealthy() {
		t.Error("expected not healthy")
	}
}

func TestDBConnectionManagerAutoReconnect(t *testing.T) {
	dbm := NewDBConnectionManager(50*time.Millisecond, 3)
	attempts := 0
	dbm.SetConnectFunc(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("connection refused")
		}
		return nil
	})
	
	// 模拟断开
	dbm.state = DBDegraded
	go dbm.autoReconnect()
	
	time.Sleep(300 * time.Millisecond)
	if dbm.GetState() != DBConnected {
		t.Errorf("expected auto reconnect to succeed, got state %v", dbm.GetState())
	}
}

func TestBackpressureLevelTransition(t *testing.T) {
	bp := NewBackpressureManager(nil)
	
	// 正常
	bp.UpdateQueueDepth(50)
	if bp.GetLevel() != LevelNormal {
		t.Errorf("expected level normal at 50, got %v", bp.GetLevel())
	}
	if bp.GetReduceRate() != 1.0 {
		t.Errorf("expected reduce rate 1.0, got %.2f", bp.GetReduceRate())
	}
	
	// 警告
	bp.UpdateQueueDepth(150)
	if bp.GetLevel() != LevelWarning {
		t.Errorf("expected level warning at 150, got %v", bp.GetLevel())
	}
	if bp.GetReduceRate() >= 1.0 {
		t.Errorf("expected reduce rate < 1.0, got %.2f", bp.GetReduceRate())
	}
	
	// 高水位
	bp.UpdateQueueDepth(600)
	if bp.GetLevel() != LevelHigh {
		t.Errorf("expected level high at 600, got %v", bp.GetLevel())
	}
	if bp.IsPaused() {
		t.Error("expected not paused at high level")
	}
	
	// 临界
	bp.UpdateQueueDepth(1200)
	if bp.GetLevel() != LevelCritical {
		t.Errorf("expected level critical at 1200, got %v", bp.GetLevel())
	}
	if !bp.IsPaused() {
		t.Error("expected paused at critical level")
	}
	if !bp.ShouldProcess() {
		t.Log("processing paused at critical level")
	}
	
	// 恢复
	bp.UpdateQueueDepth(50)
	if bp.GetLevel() != LevelNormal {
		t.Errorf("expected level normal after recovery, got %v", bp.GetLevel())
	}
	if bp.IsPaused() {
		t.Error("expected not paused after recovery")
	}
}

func TestBackpressureListener(t *testing.T) {
	bp := NewBackpressureManager(nil)
	var levelChanged atomic.Bool
	var lastLevel BackpressureLevel
	
	bp.AddListener(func(level BackpressureLevel) {
		levelChanged.Store(true)
		lastLevel = level
	})
	
	bp.UpdateQueueDepth(600)
	time.Sleep(50 * time.Millisecond) // 等待异步通知
	
	if !levelChanged.Load() {
		t.Error("expected level change listener to be called")
	}
	if lastLevel != LevelHigh {
		t.Errorf("expected last level high, got %v", lastLevel)
	}
}

func TestFailoverNodeSwitch(t *testing.T) {
	fm := NewFailoverManager(100 * time.Millisecond)
	fm.SetHealthCheckFunc(func(node *FailoverNode) bool {
		return node.State == NodeHealthy
	})
	
	node1 := &FailoverNode{ID: "node-1", Address: "192.168.1.1", State: NodeHealthy, Weight: 1.0}
	node2 := &FailoverNode{ID: "node-2", Address: "192.168.1.2", State: NodeHealthy, Weight: 1.0}
	
	fm.RegisterNode(node1)
	fm.RegisterNode(node2)
	
	active := fm.GetActiveNode()
	if active == nil {
		t.Fatal("expected active node")
	}
	if active.ID != "node-1" {
		t.Errorf("expected node-1 as active, got %s", active.ID)
	}
	
	// 模拟 node-1 故障
	node1.State = NodeUnhealthy
	fm.PerformHealthCheck()
	
	active = fm.GetActiveNode()
	if active == nil {
		t.Fatal("expected active node after failover")
	}
	if active.ID != "node-2" {
		t.Errorf("expected node-2 as active after failover, got %s", active.ID)
	}
	
	// 恢复 node-1
	node1.State = NodeHealthy
	fm.PerformHealthCheck()
	
	// active 应该切换回 node-1（优先级高）
	active = fm.GetActiveNode()
	if active.ID != "node-1" {
		t.Logf("active node after recovery: %s", active.ID)
	}
}

func TestFailoverHealthCheck(t *testing.T) {
	fm := NewFailoverManager(100 * time.Millisecond)
	
	checkCount := 0
	fm.SetHealthCheckFunc(func(node *FailoverNode) bool {
		checkCount++
		return node.ID != "node-2" // node-2 不健康
	})
	
	fm.RegisterNode(&FailoverNode{ID: "node-1", State: NodeHealthy})
	fm.RegisterNode(&FailoverNode{ID: "node-2", State: NodeHealthy})
	fm.RegisterNode(&FailoverNode{ID: "node-3", State: NodeHealthy})
	
	fm.PerformHealthCheck()
	
	if checkCount != 3 {
		t.Errorf("expected 3 health checks, got %d", checkCount)
	}
	
	if fm.GetNodeState("node-2") != NodeUnhealthy {
		t.Errorf("expected node-2 unhealthy, got %v", fm.GetNodeState("node-2"))
	}
	
	healthy := fm.GetHealthyNodes()
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy nodes, got %d", len(healthy))
	}
}

func TestFailoverNodeRecovery(t *testing.T) {
	fm := NewFailoverManager(100 * time.Millisecond)
	fm.SetHealthCheckFunc(func(node *FailoverNode) bool {
		return true // 所有节点都健康
	})
	
	fm.RegisterNode(&FailoverNode{ID: "node-1", State: NodeUnhealthy})
	fm.RegisterNode(&FailoverNode{ID: "node-2", State: NodeHealthy})
	
	fm.PerformHealthCheck()
	
	if fm.GetNodeState("node-1") != NodeRecovering {
		t.Errorf("expected node-1 recovering after first healthy check, got %v", fm.GetNodeState("node-1"))
	}
	
	// 第二次健康检查
	fm.PerformHealthCheck()
	
	if fm.GetNodeState("node-1") != NodeHealthy {
		t.Errorf("expected node-1 healthy after second check, got %v", fm.GetNodeState("node-1"))
	}
}

func TestFailoverContextCancel(t *testing.T) {
	fm := NewFailoverManager(50 * time.Millisecond)
	checkCount := 0
	fm.SetHealthCheckFunc(func(node *FailoverNode) bool {
		checkCount++
		return true
	})
	
	// 注册节点才能触发健康检查
	fm.RegisterNode(&FailoverNode{ID: "node-1", State: NodeHealthy})
	
	ctx, cancel := context.WithCancel(context.Background())
	fm.StartHealthCheck(ctx)
	
	// 等待几次检查
	time.Sleep(200 * time.Millisecond)
	if checkCount < 2 {
		t.Errorf("expected at least 2 health checks, got %d", checkCount)
	}
	
	cancel()
	time.Sleep(100 * time.Millisecond)
	
	// 取消后不应该继续检查
	lastCount := checkCount
	time.Sleep(200 * time.Millisecond)
	if checkCount > lastCount+1 {
		t.Errorf("health check should stop after context cancel, got %d checks after cancel", checkCount-lastCount)
	}
}

func TestRetryContextCancel(t *testing.T) {
	policy := &RetryPolicy{
		MaxRetries:   10,
		BaseInterval: 1 * time.Second,
		MaxInterval:  10 * time.Second,
		Multiplier:   2.0,
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	fn := func() error {
		return errors.New("always fail")
	}
	
	start := time.Now()
	err := DoRetry(ctx, policy, fn)
	elapsed := time.Since(start)
	
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected quick cancellation, got %v", elapsed)
	}
}
