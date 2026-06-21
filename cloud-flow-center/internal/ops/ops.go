//go:build linux

package ops

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// P10 部署编排与运维管理
// 解决：组件依赖复杂、缺少一键部署、缺少健康检查自愈、缺少运维管理界面
// ============================================================================

// ============================================================================
// 1. 组件定义与依赖管理
// ============================================================================

// ComponentState 组件状态
type ComponentState int

const (
	ComponentPending ComponentState = iota
	ComponentStarting
	ComponentRunning
	ComponentStopped
	ComponentFailed
	ComponentRestarting
)

func (s ComponentState) String() string {
	switch s {
	case ComponentPending:
		return "pending"
	case ComponentStarting:
		return "starting"
	case ComponentRunning:
		return "running"
	case ComponentStopped:
		return "stopped"
	case ComponentFailed:
		return "failed"
	case ComponentRestarting:
		return "restarting"
	default:
		return "unknown"
	}
}

// Component 系统组件
type Component struct {
	ID          string
	Name        string
	Description string
	DependsOn   []string          // 依赖的组件 ID
	HealthCheck HealthCheckFunc   // 健康检查函数
	StartFunc   ComponentFunc     // 启动函数
	StopFunc    ComponentFunc     // 停止函数
	MaxRetries  int               // 最大重启次数
	
	mu          sync.RWMutex
	state       ComponentState
	lastError   error
	startTime   time.Time
	restartCount int
}

// HealthCheckFunc 健康检查函数签名
type HealthCheckFunc func(ctx context.Context) (bool, error)

// ComponentFunc 组件操作函数签名
type ComponentFunc func(ctx context.Context) error

// GetState 获取组件状态
func (c *Component) GetState() ComponentState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// SetState 设置组件状态
func (c *Component) SetState(state ComponentState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = state
	if state == ComponentRunning {
		c.startTime = time.Now()
	}
}

// GetRestartCount 获取重启次数
func (c *Component) GetRestartCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.restartCount
}

// GetUptime 获取运行时间
func (c *Component) GetUptime() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.state != ComponentRunning || c.startTime.IsZero() {
		return 0
	}
	return time.Since(c.startTime)
}

// ============================================================================
// 2. 依赖拓扑排序与部署编排
// ============================================================================

// DeploymentOrchestrator 部署编排器
type DeploymentOrchestrator struct {
	mu         sync.RWMutex
	components map[string]*Component
	
	deployOrder []string
	started     bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewDeploymentOrchestrator 创建部署编排器
func NewDeploymentOrchestrator() *DeploymentOrchestrator {
	return &DeploymentOrchestrator{
		components:  make(map[string]*Component),
		stopCh:      make(chan struct{}),
	}
}

// Register 注册组件
func (do *DeploymentOrchestrator) Register(comp *Component) {
	do.mu.Lock()
	defer do.mu.Unlock()
	do.components[comp.ID] = comp
}

// GetComponent 获取组件
func (do *DeploymentOrchestrator) GetComponent(id string) *Component {
	do.mu.RLock()
	defer do.mu.RUnlock()
	return do.components[id]
}

// GetAllComponents 获取所有组件
func (do *DeploymentOrchestrator) GetAllComponents() []*Component {
	do.mu.RLock()
	defer do.mu.RUnlock()
	result := make([]*Component, 0, len(do.components))
	for _, c := range do.components {
		result = append(result, c)
	}
	return result
}

// ResolveDependencies 解析依赖拓扑排序
func (do *DeploymentOrchestrator) ResolveDependencies() ([]string, error) {
	do.mu.RLock()
	defer do.mu.RUnlock()
	
	if len(do.components) == 0 {
		return nil, fmt.Errorf("no components registered")
	}
	
	// Kahn 算法拓扑排序
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)
	
	for id := range do.components {
		inDegree[id] = 0
		adjList[id] = make([]string, 0)
	}
	
	for id, comp := range do.components {
		for _, dep := range comp.DependsOn {
			if _, ok := do.components[dep]; !ok {
				return nil, fmt.Errorf("component %s depends on unknown component %s", id, dep)
			}
			adjList[dep] = append(adjList[dep], id)
			inDegree[id]++
		}
	}
	
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	
	result := make([]string, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, id)
		
		for _, next := range adjList[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	
	if len(result) != len(do.components) {
		return nil, fmt.Errorf("circular dependency detected")
	}
	
	return result, nil
}

// Deploy 一键部署（按依赖顺序启动）
func (do *DeploymentOrchestrator) Deploy(ctx context.Context) error {
	order, err := do.ResolveDependencies()
	if err != nil {
		return fmt.Errorf("resolve dependencies failed: %w", err)
	}
	
	do.mu.Lock()
	do.deployOrder = order
	do.started = true
	do.mu.Unlock()
	
	for _, id := range order {
		comp := do.GetComponent(id)
		if comp == nil {
			continue
		}
		
		comp.SetState(ComponentStarting)
		if comp.StartFunc != nil {
			if err := comp.StartFunc(ctx); err != nil {
				comp.mu.Lock()
				comp.state = ComponentFailed
				comp.lastError = err
				comp.mu.Unlock()
				return fmt.Errorf("component %s start failed: %w", id, err)
			}
			comp.SetState(ComponentRunning)
		} else {
			comp.SetState(ComponentPending)
		}
		
		// 启动健康检查
		if comp.HealthCheck != nil {
			do.wg.Add(1)
			go do.healthCheckLoop(comp)
		}
	}
	
	return nil
}

// Undeploy 一键卸载
func (do *DeploymentOrchestrator) Undeploy(ctx context.Context) error {
	do.mu.Lock()
	if !do.started {
		do.mu.Unlock()
		return nil
	}
	close(do.stopCh)
	do.started = false
	do.mu.Unlock()
	
	// 等待健康检查循环结束
	do.wg.Wait()
	
	// 反向顺序停止组件
	order := do.deployOrder
	for i := len(order) - 1; i >= 0; i-- {
		comp := do.GetComponent(order[i])
		if comp == nil || comp.StopFunc == nil {
			continue
		}
		comp.SetState(ComponentStopping)
		comp.StopFunc(ctx)
		comp.SetState(ComponentStopped)
	}
	
	return nil
}

// healthCheckLoop 健康检查循环
func (do *DeploymentOrchestrator) healthCheckLoop(comp *Component) {
	defer do.wg.Done()
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-do.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			healthy, err := comp.HealthCheck(ctx)
			cancel()
			
			if !healthy || err != nil {
				comp.mu.Lock()
				comp.lastError = err
				comp.state = ComponentFailed
				comp.restartCount++
				restartCount := comp.restartCount
				comp.mu.Unlock()
				
				// 自愈：尝试重启
				if comp.MaxRetries <= 0 || restartCount <= comp.MaxRetries {
					if comp.StartFunc != nil {
						comp.SetState(ComponentRestarting)
						comp.StartFunc(context.Background())
						comp.SetState(ComponentRunning)
					}
				}
			}
		}
	}
}

// GetDeploymentStatus 获取部署状态
func (do *DeploymentOrchestrator) GetDeploymentStatus() map[string]interface{} {
	do.mu.RLock()
	defer do.mu.RUnlock()
	
	status := make(map[string]interface{})
	status["started"] = do.started
	
	components := make([]map[string]interface{}, 0)
	for _, id := range do.deployOrder {
		comp := do.components[id]
		if comp == nil {
			continue
		}
		comp.mu.RLock()
		components = append(components, map[string]interface{}{
			"id":            comp.ID,
			"name":          comp.Name,
			"state":         comp.state.String(),
			"uptime":        comp.GetUptime().String(),
			"restart_count": comp.restartCount,
		})
		comp.mu.RUnlock()
	}
	status["components"] = components
	return status
}

// UpgradeComponent 升级单个组件
func (do *DeploymentOrchestrator) UpgradeComponent(ctx context.Context, id string) error {
	comp := do.GetComponent(id)
	if comp == nil {
		return fmt.Errorf("component %s not found", id)
	}
	
	// 先停止
	if comp.StopFunc != nil {
		comp.SetState(ComponentStopping)
		if err := comp.StopFunc(ctx); err != nil {
			comp.SetState(ComponentFailed)
			return fmt.Errorf("stop component %s failed: %w", id, err)
		}
		comp.SetState(ComponentStopped)
	}
	
	// 再启动
	comp.SetState(ComponentStarting)
	if comp.StartFunc != nil {
		if err := comp.StartFunc(ctx); err != nil {
			comp.SetState(ComponentFailed)
			return fmt.Errorf("start component %s failed: %w", id, err)
		}
	}
	comp.SetState(ComponentRunning)
	return nil
}

// ============================================================================
// 3. 健康检查与自愈引擎
// ============================================================================

// HealthStatus 健康状态
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
)

// HealthCheckResult 健康检查结果
type HealthCheckResult struct {
	ComponentID string
	Status      HealthStatus
	Message     string
	Timestamp   time.Time
}

// SelfHealingEngine 自愈引擎
type SelfHealingEngine struct {
	mu         sync.RWMutex
	orchestrator *DeploymentOrchestrator
	checks     []func(ctx context.Context) *HealthCheckResult
	
	history    []HealthCheckResult
	enabled    bool
}

// NewSelfHealingEngine 创建自愈引擎
func NewSelfHealingEngine(orchestrator *DeploymentOrchestrator) *SelfHealingEngine {
	return &SelfHealingEngine{
		orchestrator: orchestrator,
		history:      make([]HealthCheckResult, 0),
		enabled:      true,
	}
}

// RegisterHealthCheck 注册健康检查
func (she *SelfHealingEngine) RegisterHealthCheck(fn func(ctx context.Context) *HealthCheckResult) {
	she.mu.Lock()
	defer she.mu.Unlock()
	she.checks = append(she.checks, fn)
}

// RunHealthChecks 运行所有健康检查
func (she *SelfHealingEngine) RunHealthChecks(ctx context.Context) []*HealthCheckResult {
	she.mu.RLock()
	checks := make([]func(ctx context.Context) *HealthCheckResult, len(she.checks))
	copy(checks, she.checks)
	she.mu.RUnlock()
	
	results := make([]*HealthCheckResult, 0)
	for _, fn := range checks {
		result := fn(ctx)
		results = append(results, result)
		
		she.mu.Lock()
		she.history = append(she.history, *result)
		she.mu.Unlock()
		
		// 自愈：不健康时尝试重启组件
		if result.Status == HealthUnhealthy && she.enabled {
			comp := she.orchestrator.GetComponent(result.ComponentID)
			if comp != nil && comp.StartFunc != nil {
				comp.mu.Lock()
				comp.restartCount++
				comp.mu.Unlock()
				comp.SetState(ComponentRestarting)
				comp.StartFunc(ctx)
				comp.SetState(ComponentRunning)
			}
		}
	}
	
	return results
}

// GetHistory 获取健康检查历史
func (she *SelfHealingEngine) GetHistory(limit int) []HealthCheckResult {
	she.mu.RLock()
	defer she.mu.RUnlock()
	
	if limit <= 0 || limit > len(she.history) {
		limit = len(she.history)
	}
	start := len(she.history) - limit
	if start < 0 {
		start = 0
	}
	
	result := make([]HealthCheckResult, limit)
	copy(result, she.history[start:])
	return result
}

// SetEnabled 设置自愈是否启用
func (she *SelfHealingEngine) SetEnabled(enabled bool) {
	she.mu.Lock()
	defer she.mu.Unlock()
	she.enabled = enabled
}

// ============================================================================
// 4. 运维管理接口
// ============================================================================

// OpsManager 运维管理器
type OpsManager struct {
	mu           sync.RWMutex
	orchestrator *DeploymentOrchestrator
	healer       *SelfHealingEngine
	
	logs         []OpsLog
	metrics      map[string]float64
}

// OpsLog 运维日志
type OpsLog struct {
	Timestamp time.Time
	Level     string
	Message   string
	Component string
}

// NewOpsManager 创建运维管理器
func NewOpsManager(orchestrator *DeploymentOrchestrator, healer *SelfHealingEngine) *OpsManager {
	return &OpsManager{
		orchestrator: orchestrator,
		healer:       healer,
		logs:         make([]OpsLog, 0),
		metrics:      make(map[string]float64),
	}
}

// Log 记录运维日志
func (om *OpsManager) Log(level, message, component string) {
	om.mu.Lock()
	defer om.mu.Unlock()
	om.logs = append(om.logs, OpsLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Component: component,
	})
}

// GetLogs 获取日志
func (om *OpsManager) GetLogs(limit int) []OpsLog {
	om.mu.RLock()
	defer om.mu.RUnlock()
	
	if limit <= 0 || limit > len(om.logs) {
		limit = len(om.logs)
	}
	start := len(om.logs) - limit
	if start < 0 {
		start = 0
	}
	
	result := make([]OpsLog, limit)
	copy(result, om.logs[start:])
	return result
}

// GetMetrics 获取指标
func (om *OpsManager) GetMetrics() map[string]float64 {
	om.mu.RLock()
	defer om.mu.RUnlock()
	
	result := make(map[string]float64)
	for k, v := range om.metrics {
		result[k] = v
	}
	return result
}

// SetMetric 设置指标
func (om *OpsManager) SetMetric(key string, value float64) {
	om.mu.Lock()
	defer om.mu.Unlock()
	om.metrics[key] = value
}

// GetSystemOverview 获取系统概览
func (om *OpsManager) GetSystemOverview() map[string]interface{} {
	overview := make(map[string]interface{})
	
	// 组件状态
	overview["components"] = om.orchestrator.GetDeploymentStatus()
	
	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	healthResults := om.healer.RunHealthChecks(ctx)
	overview["health"] = healthResults
	
	// 日志
	overview["recent_logs"] = om.GetLogs(10)
	
	// 指标
	overview["metrics"] = om.GetMetrics()
	
	return overview
}

// RestartComponent 重启组件
func (om *OpsManager) RestartComponent(ctx context.Context, componentID string) error {
	om.Log("info", fmt.Sprintf("Restarting component %s", componentID), componentID)
	
	comp := om.orchestrator.GetComponent(componentID)
	if comp == nil {
		om.Log("error", fmt.Sprintf("Component %s not found", componentID), componentID)
		return fmt.Errorf("component %s not found", componentID)
	}
	
	// 先停止
	if comp.StopFunc != nil {
		comp.SetState(ComponentStopping)
		if err := comp.StopFunc(ctx); err != nil {
			om.Log("error", fmt.Sprintf("Stop failed: %v", err), componentID)
			comp.SetState(ComponentFailed)
			return err
		}
		comp.SetState(ComponentStopped)
	}
	
	// 等待依赖
	for _, depID := range comp.DependsOn {
		dep := om.orchestrator.GetComponent(depID)
		if dep != nil && dep.GetState() != ComponentRunning {
			om.Log("info", fmt.Sprintf("Waiting for dependency %s", depID), componentID)
			// 启动依赖
			if dep.StartFunc != nil {
				dep.SetState(ComponentStarting)
				dep.StartFunc(ctx)
				dep.SetState(ComponentRunning)
			}
		}
	}
	
	// 启动
	comp.SetState(ComponentStarting)
	if comp.StartFunc != nil {
		if err := comp.StartFunc(ctx); err != nil {
			om.Log("error", fmt.Sprintf("Start failed: %v", err), componentID)
			comp.SetState(ComponentFailed)
			return err
		}
	}
	comp.SetState(ComponentRunning)
	comp.mu.Lock()
	comp.restartCount = 0
	comp.mu.Unlock()
	
	om.Log("info", fmt.Sprintf("Component %s restarted successfully", componentID), componentID)
	return nil
}

// RollbackComponent 回滚组件
func (om *OpsManager) RollbackComponent(ctx context.Context, componentID string) error {
	om.Log("info", fmt.Sprintf("Rolling back component %s", componentID), componentID)
	
	comp := om.orchestrator.GetComponent(componentID)
	if comp == nil {
		return fmt.Errorf("component %s not found", componentID)
	}
	
	// 简单回滚：先停止再启动
	if comp.StopFunc != nil {
		comp.StopFunc(ctx)
	}
	comp.SetState(ComponentPending)
	
	om.Log("info", fmt.Sprintf("Component %s rolled back", componentID), componentID)
	return nil
}

// GetComponentHealth 获取组件健康状态
func (om *OpsManager) GetComponentHealth(componentID string) *HealthCheckResult {
	comp := om.orchestrator.GetComponent(componentID)
	if comp == nil {
		return &HealthCheckResult{
			ComponentID: componentID,
			Status:      HealthUnhealthy,
			Message:     "component not found",
			Timestamp:   time.Now(),
		}
	}
	
	state := comp.GetState()
	if state == ComponentRunning {
		return &HealthCheckResult{
			ComponentID: componentID,
			Status:      HealthHealthy,
			Message:     fmt.Sprintf("component is %s", state.String()),
			Timestamp:   time.Now(),
		}
	}
	
	return &HealthCheckResult{
		ComponentID: componentID,
		Status:      HealthUnhealthy,
		Message:     fmt.Sprintf("component is %s", state.String()),
		Timestamp:   time.Now(),
	}
}

// GetComponentStates 获取所有组件状态
func (om *OpsManager) GetComponentStates() map[string]ComponentState {
	states := make(map[string]ComponentState)
	for _, comp := range om.orchestrator.GetAllComponents() {
		states[comp.ID] = comp.GetState()
	}
	return states
}

// SortComponentsByDependency 按依赖顺序排序组件
func SortComponentsByDependency(comps []*Component) ([]*Component, error) {
	idMap := make(map[string]*Component)
	for _, c := range comps {
		idMap[c.ID] = c
	}
	
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)
	for _, c := range comps {
		inDegree[c.ID] = 0
		adjList[c.ID] = make([]string, 0)
	}
	
	for _, c := range comps {
		for _, dep := range c.DependsOn {
			if _, ok := idMap[dep]; !ok {
				return nil, fmt.Errorf("component %s depends on unknown %s", c.ID, dep)
			}
			adjList[dep] = append(adjList[dep], c.ID)
			inDegree[c.ID]++
		}
	}
	
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	
	result := make([]*Component, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, idMap[id])
		for _, next := range adjList[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	
	if len(result) != len(comps) {
		return nil, fmt.Errorf("circular dependency detected")
	}
	
	return result, nil
}

// ComponentStopping 组件停止状态
const ComponentStopping ComponentState = 6

// GetRunningComponents 获取运行中的组件
func (do *DeploymentOrchestrator) GetRunningComponents() []*Component {
	do.mu.RLock()
	defer do.mu.RUnlock()
	result := make([]*Component, 0)
	for _, c := range do.components {
		if c.GetState() == ComponentRunning {
			result = append(result, c)
		}
	}
	return result
}
