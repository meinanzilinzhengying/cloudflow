//go:build linux

package ops

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestComponentStateString(t *testing.T) {
	if ComponentRunning.String() != "running" {
		t.Errorf("expected 'running', got %s", ComponentRunning.String())
	}
	if ComponentFailed.String() != "failed" {
		t.Errorf("expected 'failed', got %s", ComponentFailed.String())
	}
	if ComponentState(99).String() != "unknown" {
		t.Errorf("expected 'unknown', got %s", ComponentState(99).String())
	}
}

func TestComponentUptime(t *testing.T) {
	comp := &Component{
		ID:   "test-1",
		Name: "test",
	}
	comp.SetState(ComponentRunning)
	
	time.Sleep(50 * time.Millisecond)
	uptime := comp.GetUptime()
	if uptime < 50*time.Millisecond {
		t.Errorf("expected uptime >= 50ms, got %v", uptime)
	}
	
	// 未运行时应为0
	comp2 := &Component{ID: "test-2"}
	if comp2.GetUptime() != 0 {
		t.Error("expected uptime 0 for non-running component")
	}
}

func TestResolveDependencies(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	// comp3 -> comp2 -> comp1
	orchestrator.Register(&Component{
		ID:        "comp1",
		Name:      "Component 1",
		DependsOn: []string{},
	})
	orchestrator.Register(&Component{
		ID:        "comp2",
		Name:      "Component 2",
		DependsOn: []string{"comp1"},
	})
	orchestrator.Register(&Component{
		ID:        "comp3",
		Name:      "Component 3",
		DependsOn: []string{"comp2"},
	})
	
	order, err := orchestrator.ResolveDependencies()
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	
	if len(order) != 3 {
		t.Fatalf("expected 3 components, got %d", len(order))
	}
	
	// comp1 必须在 comp2 之前
	idx1 := -1
	idx2 := -1
	idx3 := -1
	for i, id := range order {
		switch id {
		case "comp1":
			idx1 = i
		case "comp2":
			idx2 = i
		case "comp3":
			idx3 = i
		}
	}
	
	if idx1 >= idx2 {
		t.Error("comp1 should be before comp2")
	}
	if idx2 >= idx3 {
		t.Error("comp2 should be before comp3")
	}
}

func TestResolveDependenciesCircular(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	orchestrator.Register(&Component{
		ID:        "a",
		DependsOn: []string{"b"},
	})
	orchestrator.Register(&Component{
		ID:        "b",
		DependsOn: []string{"a"},
	})
	
	_, err := orchestrator.ResolveDependencies()
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
	if err.Error() != "circular dependency detected" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveDependenciesUnknownDependency(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:        "a",
		DependsOn: []string{"nonexistent"},
	})
	
	_, err := orchestrator.ResolveDependencies()
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

func TestDeploy(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	startOrder := []string{}
	orchestrator.Register(&Component{
		ID:        "db",
		Name:      "Database",
		DependsOn: []string{},
		StartFunc: func(ctx context.Context) error {
			startOrder = append(startOrder, "db")
			return nil
		},
	})
	orchestrator.Register(&Component{
		ID:        "api",
		Name:      "API Server",
		DependsOn: []string{"db"},
		StartFunc: func(ctx context.Context) error {
			startOrder = append(startOrder, "api")
			return nil
		},
	})
	orchestrator.Register(&Component{
		ID:        "web",
		Name:      "Web UI",
		DependsOn: []string{"api"},
		StartFunc: func(ctx context.Context) error {
			startOrder = append(startOrder, "web")
			return nil
		},
	})
	
	err := orchestrator.Deploy(context.Background())
	if err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	
	if len(startOrder) != 3 {
		t.Fatalf("expected 3 starts, got %d: %v", len(startOrder), startOrder)
	}
	
	if startOrder[0] != "db" || startOrder[1] != "api" || startOrder[2] != "web" {
		t.Errorf("unexpected start order: %v", startOrder)
	}
	
	// 检查组件状态
	for _, id := range []string{"db", "api", "web"} {
		comp := orchestrator.GetComponent(id)
		if comp.GetState() != ComponentRunning {
			t.Errorf("expected %s to be running, got %v", id, comp.GetState())
		}
	}
}

func TestDeployFailure(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	orchestrator.Register(&Component{
		ID:        "db",
		StartFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Register(&Component{
		ID:        "api",
		DependsOn: []string{"db"},
		StartFunc: func(ctx context.Context) error {
			return errors.New("api start failed")
		},
	})
	orchestrator.Register(&Component{
		ID:        "web",
		DependsOn: []string{"api"},
		StartFunc: func(ctx context.Context) error { return nil },
	})
	
	err := orchestrator.Deploy(context.Background())
	if err == nil {
		t.Fatal("expected deploy to fail")
	}
	
	// api 和 web 不应该启动
	api := orchestrator.GetComponent("api")
	if api.GetState() != ComponentFailed {
		t.Errorf("expected api failed, got %v", api.GetState())
	}
	
	web := orchestrator.GetComponent("web")
	if web.GetState() != ComponentPending {
		t.Errorf("expected web pending, got %v", web.GetState())
	}
}

func TestUndeploy(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	stopOrder := []string{}
	orchestrator.Register(&Component{
		ID:        "db",
		StartFunc: func(ctx context.Context) error { return nil },
		StopFunc: func(ctx context.Context) error {
			stopOrder = append(stopOrder, "db")
			return nil
		},
	})
	orchestrator.Register(&Component{
		ID:        "api",
		DependsOn: []string{"db"},
		StartFunc: func(ctx context.Context) error { return nil },
		StopFunc: func(ctx context.Context) error {
			stopOrder = append(stopOrder, "api")
			return nil
		},
	})
	
	orchestrator.Deploy(context.Background())
	orchestrator.Undeploy(context.Background())
	
	if len(stopOrder) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(stopOrder))
	}
	// 反向停止：api 先停，然后 db
	if stopOrder[0] != "api" || stopOrder[1] != "db" {
		t.Errorf("expected reverse stop order [api, db], got %v", stopOrder)
	}
}

func TestUpgradeComponent(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	
	restartCount := 0
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error {
			restartCount++
			return nil
		},
		StopFunc: func(ctx context.Context) error { return nil },
	})
	
	orchestrator.Deploy(context.Background())
	if restartCount != 1 {
		t.Fatalf("expected 1 start, got %d", restartCount)
	}
	
	err := orchestrator.UpgradeComponent(context.Background(), "svc")
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	
	if restartCount != 2 {
		t.Errorf("expected 2 starts (deploy + upgrade), got %d", restartCount)
	}
	
	comp := orchestrator.GetComponent("svc")
	if comp.GetState() != ComponentRunning {
		t.Errorf("expected running after upgrade, got %v", comp.GetState())
	}
}

func TestGetDeploymentStatus(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "test",
		Name: "Test",
		StartFunc: func(ctx context.Context) error { return nil },
	})
	
	orchestrator.Deploy(context.Background())
	status := orchestrator.GetDeploymentStatus()
	
	if !status["started"].(bool) {
		t.Error("expected started to be true")
	}
	
	comps := status["components"].([]map[string]interface{})
	if len(comps) != 1 {
		t.Fatalf("expected 1 component, got %d", len(comps))
	}
	if comps[0]["id"] != "test" {
		t.Errorf("expected component id 'test', got %v", comps[0]["id"])
	}
}

func TestSelfHealingEngine(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Deploy(context.Background())
	
	healer := NewSelfHealingEngine(orchestrator)
	
	// 注册健康检查
	restartTriggered := false
	orchestrator.GetComponent("svc").SetState(ComponentFailed)
	
	healer.RegisterHealthCheck(func(ctx context.Context) *HealthCheckResult {
		comp := orchestrator.GetComponent("svc")
		if comp.GetState() == ComponentFailed {
			restartTriggered = true
			return &HealthCheckResult{
				ComponentID: "svc",
				Status:      HealthUnhealthy,
				Message:     "service failed",
				Timestamp:   time.Now(),
			}
		}
		return &HealthCheckResult{
			ComponentID: "svc",
			Status:      HealthHealthy,
			Message:     "ok",
			Timestamp:   time.Now(),
		}
	})
	
	results := healer.RunHealthChecks(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != HealthUnhealthy {
		t.Errorf("expected unhealthy, got %v", results[0].Status)
	}
	
	// 检查自愈是否触发
	if !restartTriggered {
		t.Error("expected restart to be triggered by self-healing")
	}
	
	// 检查历史记录
	history := healer.GetHistory(10)
	if len(history) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(history))
	}
}

func TestSelfHealingDisabled(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Deploy(context.Background())
	
	healer := NewSelfHealingEngine(orchestrator)
	healer.SetEnabled(false)
	
	orchestrator.GetComponent("svc").SetState(ComponentFailed)
	
	healer.RegisterHealthCheck(func(ctx context.Context) *HealthCheckResult {
		return &HealthCheckResult{
			ComponentID: "svc",
			Status:      HealthUnhealthy,
			Message:     "failed",
			Timestamp:   time.Now(),
		}
	})
	
	// 自愈被禁用，不应触发重启
	healer.RunHealthChecks(context.Background())
	if orchestrator.GetComponent("svc").GetState() != ComponentFailed {
		t.Error("expected component to remain failed when self-healing disabled")
	}
}

func TestOpsManager(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error { return nil },
		HealthCheck: func(ctx context.Context) (bool, error) { return true, nil },
	})
	orchestrator.Deploy(context.Background())
	
	healer := NewSelfHealingEngine(orchestrator)
	ops := NewOpsManager(orchestrator, healer)
	
	// 测试日志
	ops.Log("info", "test message", "svc")
	logs := ops.GetLogs(10)
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Message != "test message" {
		t.Errorf("unexpected log message: %s", logs[0].Message)
	}
	
	// 测试指标
	ops.SetMetric("cpu_usage", 75.5)
	metrics := ops.GetMetrics()
	if metrics["cpu_usage"] != 75.5 {
		t.Errorf("expected metric 75.5, got %v", metrics["cpu_usage"])
	}
	
	// 测试组件健康
	health := ops.GetComponentHealth("svc")
	if health.Status != HealthHealthy {
		t.Errorf("expected healthy, got %v", health.Status)
	}
	
	// 测试不存在的组件
	health2 := ops.GetComponentHealth("nonexistent")
	if health2.Status != HealthUnhealthy {
		t.Errorf("expected unhealthy for nonexistent, got %v", health2.Status)
	}
	
	// 测试组件状态
	states := ops.GetComponentStates()
	if states["svc"] != ComponentRunning {
		t.Errorf("expected running, got %v", states["svc"])
	}
}

func TestOpsManagerRestartComponent(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	restartCount := 0
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error {
			restartCount++
			return nil
		},
		StopFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Deploy(context.Background())
	
	healer := NewSelfHealingEngine(orchestrator)
	ops := NewOpsManager(orchestrator, healer)
	
	err := ops.RestartComponent(context.Background(), "svc")
	if err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	
	if restartCount != 2 {
		t.Errorf("expected 2 starts (deploy + restart), got %d", restartCount)
	}
}

func TestOpsManagerRollbackComponent(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "svc",
		Name: "Service",
		StartFunc: func(ctx context.Context) error { return nil },
		StopFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Deploy(context.Background())
	
	healer := NewSelfHealingEngine(orchestrator)
	ops := NewOpsManager(orchestrator, healer)
	
	err := ops.RollbackComponent(context.Background(), "svc")
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	
	comp := orchestrator.GetComponent("svc")
	if comp.GetState() != ComponentPending {
		t.Errorf("expected pending after rollback, got %v", comp.GetState())
	}
}

func TestSortComponentsByDependency(t *testing.T) {
	comps := []*Component{
		{ID: "web", DependsOn: []string{"api"}},
		{ID: "db", DependsOn: []string{}},
		{ID: "api", DependsOn: []string{"db"}},
	}
	
	result, err := SortComponentsByDependency(comps)
	if err != nil {
		t.Fatalf("sort failed: %v", err)
	}
	
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0].ID != "db" || result[1].ID != "api" || result[2].ID != "web" {
		t.Errorf("unexpected order: %v, %v, %v", result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestSortComponentsByDependencyCircular(t *testing.T) {
	comps := []*Component{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}
	
	_, err := SortComponentsByDependency(comps)
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
}

func TestGetRunningComponents(t *testing.T) {
	orchestrator := NewDeploymentOrchestrator()
	orchestrator.Register(&Component{
		ID:   "running1",
		StartFunc: func(ctx context.Context) error { return nil },
	})
	orchestrator.Register(&Component{
		ID:   "pending1",
	})
	
	orchestrator.Deploy(context.Background())
	
	running := orchestrator.GetRunningComponents()
	if len(running) != 1 {
		t.Errorf("expected 1 running component, got %d", len(running))
	}
	if running[0].ID != "running1" {
		t.Errorf("expected running1, got %s", running[0].ID)
	}
}
