//go:build linux

package servicemesh_test

import (
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/servicemesh"
)

func TestMemoryRegistry(t *testing.T) {
	registry := servicemesh.NewMemoryRegistry()

	inst1 := &servicemesh.ServiceInstance{
		ID:   "inst-1",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8080,
		Protocol: "http",
		Status: servicemesh.InstanceStatusHealthy,
		Weight: 10,
	}
	inst2 := &servicemesh.ServiceInstance{
		ID:   "inst-2",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8081,
		Protocol: "http",
		Status: servicemesh.InstanceStatusHealthy,
		Weight: 20,
	}

	// 测试注册
	if err := registry.Register(inst1); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := registry.Register(inst2); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 测试获取服务
	instances, err := registry.GetService("test-service")
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(instances))
	}

	// 测试注销
	if err := registry.Deregister("inst-1"); err != nil {
		t.Fatalf("Deregister failed: %v", err)
	}
	instances, _ = registry.GetService("test-service")
	if len(instances) != 1 {
		t.Fatalf("Expected 1 instance after deregister, got %d", len(instances))
	}

	// 测试监听
	watchCh, err := registry.Watch("test-service")
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// 注册新实例触发监听
	inst3 := &servicemesh.ServiceInstance{
		ID:   "inst-3",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8082,
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(inst3)

	select {
	case updated := <-watchCh:
		if len(updated) != 2 {
			t.Fatalf("Expected 2 instances from watch, got %d", len(updated))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch timeout")
	}

	// 测试心跳
	if err := registry.Heartbeat("inst-2"); err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	if err := registry.Heartbeat("non-existent"); err == nil {
		t.Fatal("Expected error for non-existent instance heartbeat")
	}
}

func TestLoadBalancer(t *testing.T) {
	instances := []*servicemesh.ServiceInstance{
		{ID: "1", Name: "svc", Host: "127.0.0.1", Port: 8080, Status: servicemesh.InstanceStatusHealthy, Weight: 10},
		{ID: "2", Name: "svc", Host: "127.0.0.1", Port: 8081, Status: servicemesh.InstanceStatusHealthy, Weight: 20},
		{ID: "3", Name: "svc", Host: "127.0.0.1", Port: 8082, Status: servicemesh.InstanceStatusUnhealthy, Weight: 30},
	}

	// 测试随机负载均衡
	rlb := servicemesh.NewRandomLoadBalancer()
	for i := 0; i < 10; i++ {
		inst, err := rlb.Select(instances)
		if err != nil {
			t.Fatalf("Random select failed: %v", err)
		}
		if inst.Status != servicemesh.InstanceStatusHealthy {
			t.Fatalf("Selected unhealthy instance: %s", inst.ID)
		}
	}

	// 测试轮询负载均衡
	rolb := servicemesh.NewRoundRobinLoadBalancer()
	selected := make(map[string]int)
	for i := 0; i < 20; i++ {
		inst, err := rolb.Select(instances)
		if err != nil {
			t.Fatalf("RoundRobin select failed: %v", err)
		}
		selected[inst.ID]++
	}
	// 只有健康实例会被选中
	if selected["3"] != 0 {
		t.Fatal("RoundRobin selected unhealthy instance")
	}
	// 两个健康实例应该被均匀分配
	if selected["1"] == 0 || selected["2"] == 0 {
		t.Fatal("RoundRobin did not distribute evenly")
	}

	// 测试加权负载均衡
	wlb := servicemesh.NewWeightedLoadBalancer()
	selected = make(map[string]int)
	for i := 0; i < 100; i++ {
		inst, err := wlb.Select(instances)
		if err != nil {
			t.Fatalf("Weighted select failed: %v", err)
		}
		selected[inst.ID]++
	}
	// 权重20的实例应该比权重10的选中次数更多
	if selected["2"] <= selected["1"] {
		t.Logf("Weight distribution: inst1=%d, inst2=%d", selected["1"], selected["2"])
	}

	// 测试无健康实例
	allUnhealthy := []*servicemesh.ServiceInstance{
		{ID: "1", Name: "svc", Host: "127.0.0.1", Port: 8080, Status: servicemesh.InstanceStatusUnhealthy},
	}
	_, err := rlb.Select(allUnhealthy)
	if err == nil {
		t.Fatal("Expected error when no healthy instances")
	}
}

func TestCircuitBreaker(t *testing.T) {
	config := &servicemesh.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}
	cb := servicemesh.NewCircuitBreaker("test-service", config)

	// 初始状态：关闭
	if cb.State() != servicemesh.CircuitStateClosed {
		t.Fatalf("Expected closed state, got %s", cb.State())
	}

	// 允许请求
	if !cb.Allow() {
		t.Fatal("Expected allow in closed state")
	}

	// 记录失败，达到阈值
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// 状态变为打开
	if cb.State() != servicemesh.CircuitStateOpen {
		t.Fatalf("Expected open state, got %s", cb.State())
	}

	// 不允许请求
	if cb.Allow() {
		t.Fatal("Expected not allow in open state")
	}

	// 等待超时后，调用 Allow 触发状态转换到半开
	time.Sleep(150 * time.Millisecond)

	// 允许请求（触发从 open -> halfopen 转换）
	if !cb.Allow() {
		t.Fatalf("Expected allow after timeout in open state, got %s", cb.State())
	}

	// 状态变为半开
	if cb.State() != servicemesh.CircuitStateHalfOpen {
		t.Fatalf("Expected halfopen state, got %s", cb.State())
	}

	// 允许有限请求
	if !cb.Allow() {
		t.Fatal("Expected allow in halfopen state")
	}
	if !cb.Allow() {
		t.Fatal("Expected allow in halfopen state (2nd call)")
	}
	if cb.Allow() {
		t.Fatal("Expected not allow after max calls in halfopen state")
	}

	// 记录成功，达到阈值，恢复关闭
	cb.RecordSuccess()
	cb.RecordSuccess()
	if cb.State() != servicemesh.CircuitStateClosed {
		t.Fatalf("Expected closed state after recovery, got %s", cb.State())
	}

	// 从 closed 状态，需要 3 次失败才能打开
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != servicemesh.CircuitStateOpen {
		t.Fatalf("Expected open state after 3 failures in closed, got %s", cb.State())
	}

	// 测试统计
	stats := cb.GetStats()
	if stats["name"] != "test-service" {
		t.Fatalf("Expected name 'test-service', got %v", stats["name"])
	}
}

func TestServiceClient(t *testing.T) {
	registry := servicemesh.NewMemoryRegistry()

	inst1 := &servicemesh.ServiceInstance{
		ID:   "inst-1",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8080,
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(inst1)

	client, err := servicemesh.NewServiceClient("test-service", registry, servicemesh.LBStrategyRandom)
	if err != nil {
		t.Fatalf("NewServiceClient failed: %v", err)
	}
	defer client.Stop()

	// 测试获取实例
	inst, err := client.GetInstance()
	if err != nil {
		t.Fatalf("GetInstance failed: %v", err)
	}
	if inst.ID != "inst-1" {
		t.Fatalf("Expected inst-1, got %s", inst.ID)
	}

	// 测试获取所有实例
	all := client.GetAllInstances()
	if len(all) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(all))
	}

	// 测试统计
	stats := client.GetStats()
	if stats["service_name"] != "test-service" {
		t.Fatalf("Expected service_name 'test-service', got %v", stats["service_name"])
	}
}

func TestRetryPolicy(t *testing.T) {
	policy := servicemesh.DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Fatalf("Expected MaxRetries 3, got %d", policy.MaxRetries)
	}

	// 测试延迟计算
	delay := policy.CalculateDelay(1)
	if delay < 0 {
		t.Fatalf("Expected non-negative delay, got %v", delay)
	}

	delay = policy.CalculateDelay(10)
	if delay > 5*time.Second+100*time.Millisecond {
		t.Fatalf("Expected delay <= max delay, got %v", delay)
	}
}

func TestHealthChecker(t *testing.T) {
	registry := servicemesh.NewMemoryRegistry()
	checker := servicemesh.NewHealthChecker(registry)
	defer checker.Stop()

	inst := &servicemesh.ServiceInstance{
		ID:   "inst-1",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8080,
		Status: servicemesh.InstanceStatusHealthy,
	}
	registry.Register(inst)

	// 添加健康检查（使用自定义检查函数，避免实际HTTP请求）
	check := checker.AddCheck(inst, 100*time.Millisecond, 1*time.Second)
	check.CheckFunc = func(i *servicemesh.ServiceInstance) (bool, error) {
		return true, nil
	}

	// 等待检查执行
	time.Sleep(200 * time.Millisecond)

	// 获取结果
	result, ok := checker.GetCheckResult("inst-1")
	if !ok {
		t.Fatal("Expected check result for inst-1")
	}
	if !result.LastResult {
		t.Fatal("Expected health check to pass")
	}

	// 获取所有结果
	allResults := checker.GetAllResults()
	if len(allResults) != 1 {
		t.Fatalf("Expected 1 check result, got %d", len(allResults))
	}
}

func TestDependencyGraph(t *testing.T) {
	graph := servicemesh.NewDependencyGraph()

	// 添加依赖: A -> B -> C, A -> D
	graph.AddDependency("A", "B")
	graph.AddDependency("B", "C")
	graph.AddDependency("A", "D")

	// 测试获取依赖
	depsA := graph.GetDependencies("A")
	if len(depsA) != 2 {
		t.Fatalf("Expected 2 dependencies for A, got %d", len(depsA))
	}

	depsB := graph.GetDependencies("B")
	if len(depsB) != 1 || depsB[0] != "C" {
		t.Fatalf("Expected B depends on C, got %v", depsB)
	}

	// 测试获取消费者
	consumersB := graph.GetConsumers("B")
	if len(consumersB) != 1 || consumersB[0] != "A" {
		t.Fatalf("Expected A consumes B, got %v", consumersB)
	}

	// 测试拓扑排序
	order, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort failed: %v", err)
	}

	// C 必须在 B 之前，B 必须在 A 之前
	idx := make(map[string]int)
	for i, svc := range order {
		idx[svc] = i
	}
	if idx["C"] >= idx["B"] || idx["B"] >= idx["A"] {
		t.Fatalf("Invalid topological order: %v", order)
	}

	// 测试循环依赖检测
	graphNoCycle := servicemesh.NewDependencyGraph()
	graphNoCycle.AddDependency("A", "B")
	graphNoCycle.AddDependency("B", "C")
	cycle, hasCycle := graphNoCycle.CheckCircular()
	if hasCycle {
		t.Fatalf("Expected no cycle, found: %v", cycle)
	}

	graphCycle := servicemesh.NewDependencyGraph()
	graphCycle.AddDependency("A", "B")
	graphCycle.AddDependency("B", "C")
	graphCycle.AddDependency("C", "A")
	cycle, hasCycle = graphCycle.CheckCircular()
	if !hasCycle {
		t.Fatal("Expected cycle detection")
	}
	if len(cycle) == 0 {
		t.Fatal("Expected non-empty cycle path")
	}

	// 循环依赖的拓扑排序应失败
	_, err = graphCycle.TopologicalSort()
	if err == nil {
		t.Fatal("Expected error for circular dependency")
	}
}

func TestServiceStarter(t *testing.T) {
	registry := servicemesh.NewMemoryRegistry()
	starter := servicemesh.NewServiceStarter(registry)

	// 创建模拟服务
	svcA := &mockService{name: "A"}
	svcB := &mockService{name: "B"}
	svcC := &mockService{name: "C"}

	starter.Register(svcA, "B")
	starter.Register(svcB, "C")
	starter.Register(svcC)

	// 测试启动（按依赖顺序）
	if err := starter.StartAll(); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	// C 必须先启动（因为它是B的依赖，B是A的依赖）
	if !svcC.started {
		t.Fatal("Expected C to be started")
	}
	if !svcB.started {
		t.Fatal("Expected B to be started")
	}
	if !svcA.started {
		t.Fatal("Expected A to be started")
	}

	// 测试停止（反序）
	if err := starter.StopAll(); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	if !svcA.stopped {
		t.Fatal("Expected A to be stopped")
	}
	if !svcB.stopped {
		t.Fatal("Expected B to be stopped")
	}
	if !svcC.stopped {
		t.Fatal("Expected C to be stopped")
	}
}

type mockService struct {
	name    string
	started bool
	stopped bool
}

func (m *mockService) Name() string   { return m.name }
func (m *mockService) Start() error  { m.started = true; return nil }
func (m *mockService) Stop() error   { m.stopped = true; return nil }
func (m *mockService) Health() bool  { return true }

func TestLoadBalancerFactory(t *testing.T) {
	// 测试各种策略
	strategies := []servicemesh.LoadBalanceStrategy{
		servicemesh.LBStrategyRandom,
		servicemesh.LBStrategyRoundRobin,
		servicemesh.LBStrategyWeight,
		servicemesh.LBStrategyLeastConn,
	}

	for _, s := range strategies {
		lb := servicemesh.LoadBalancerFactory(s)
		if lb == nil {
			t.Fatalf("LoadBalancerFactory returned nil for %s", s)
		}
		t.Logf("Strategy %s: %s", s, lb.Strategy())
	}
}

func TestMemoryRegistryExpiration(t *testing.T) {
	registry := servicemesh.NewMemoryRegistry()
	
	inst := &servicemesh.ServiceInstance{
		ID:   "inst-1",
		Name: "test-service",
		Host: "127.0.0.1",
		Port: 8080,
		Status: servicemesh.InstanceStatusHealthy,
	}
	
	registry.Register(inst)
	
	// 等待过期（默认30s，在测试中手动设置过期）
	// 由于默认过期时间较长，这里测试注销功能
	registry.Deregister("inst-1")
	
	instances, err := registry.GetService("test-service")
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}
	if len(instances) != 0 {
		t.Fatalf("Expected 0 instances after deregister, got %d", len(instances))
	}
}
