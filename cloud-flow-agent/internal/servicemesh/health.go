//go:build linux

package servicemesh

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// 一、健康检查器
// ============================================================================

// HealthChecker 健康检查器
type HealthChecker struct {
	mu         sync.RWMutex
	checks     map[string]*HealthCheck
	registry   Registry
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// HealthCheck 单个健康检查
type HealthCheck struct {
	Instance   *ServiceInstance
	Interval   time.Duration
	Timeout    time.Duration
	Endpoint   string
	CheckFunc  func(*ServiceInstance) (bool, error)
	LastResult bool
	LastError  error
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(registry Registry) *HealthChecker {
	return &HealthChecker{
		checks:   make(map[string]*HealthCheck),
		registry: registry,
		stopCh:   make(chan struct{}),
	}
}

// AddCheck 添加健康检查，返回 *HealthCheck 以便外部自定义 CheckFunc
func (hc *HealthChecker) AddCheck(instance *ServiceInstance, interval, timeout time.Duration) *HealthCheck {
	id := instance.ID
	check := &HealthCheck{
		Instance: instance,
		Interval: interval,
		Timeout:  timeout,
		Endpoint: "/health",
	}
	
	// 默认 HTTP 健康检查
	check.CheckFunc = func(inst *ServiceInstance) (bool, error) {
		client := &http.Client{Timeout: timeout}
		url := fmt.Sprintf("http://%s%s", inst.Address(), check.Endpoint)
		resp, err := client.Get(url)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK, nil
	}
	
	hc.mu.Lock()
	hc.checks[id] = check
	hc.mu.Unlock()
	
	// 启动检查 goroutine
	hc.wg.Add(1)
	go hc.runCheck(check)
	
	return check
}

// RemoveCheck 移除健康检查
func (hc *HealthChecker) RemoveCheck(instanceID string) {
	hc.mu.Lock()
	delete(hc.checks, instanceID)
	hc.mu.Unlock()
}

// runCheck 运行检查循环
func (hc *HealthChecker) runCheck(check *HealthCheck) {
	defer hc.wg.Done()
	
	ticker := time.NewTicker(check.Interval)
	defer ticker.Stop()
	
	// 立即检查一次
	hc.doCheck(check)
	
	for {
		select {
		case <-hc.stopCh:
			return
		case <-ticker.C:
			hc.doCheck(check)
		}
	}
}

// doCheck 执行一次检查
func (hc *HealthChecker) doCheck(check *HealthCheck) {
	ok, err := check.CheckFunc(check.Instance)
	
	check.LastResult = ok
	check.LastError = err
	
	if ok {
		if check.Instance.Status != InstanceStatusHealthy {
			check.Instance.Status = InstanceStatusHealthy
			// 更新注册中心
			if hc.registry != nil {
				hc.registry.Register(check.Instance)
			}
		}
	} else {
		if check.Instance.Status != InstanceStatusUnhealthy {
			check.Instance.Status = InstanceStatusUnhealthy
			if hc.registry != nil {
				hc.registry.Register(check.Instance)
			}
		}
	}
}

// GetCheckResult 获取检查结果
func (hc *HealthChecker) GetCheckResult(instanceID string) (*HealthCheck, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	check, ok := hc.checks[instanceID]
	return check, ok
}

// GetAllResults 获取所有检查结果
func (hc *HealthChecker) GetAllResults() map[string]*HealthCheck {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	
	result := make(map[string]*HealthCheck, len(hc.checks))
	for id, check := range hc.checks {
		result[id] = check
	}
	return result
}

// Stop 停止所有检查
func (hc *HealthChecker) Stop() {
	hc.stopOnce.Do(func() {
		close(hc.stopCh)
	})
	hc.wg.Wait()
}

// ============================================================================
// 二、服务依赖管理
// ============================================================================

// DependencyGraph 服务依赖图
type DependencyGraph struct {
	mu         sync.RWMutex
	dependencies map[string][]string // service -> []dependencies
	consumers    map[string][]string // service -> []consumers
}

// NewDependencyGraph 创建依赖图
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		dependencies: make(map[string][]string),
		consumers:    make(map[string][]string),
	}
}

// AddDependency 添加依赖
func (dg *DependencyGraph) AddDependency(service, dependency string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	// 添加依赖
	if !contains(dg.dependencies[service], dependency) {
		dg.dependencies[service] = append(dg.dependencies[service], dependency)
	}
	
	// 添加消费者
	if !contains(dg.consumers[dependency], service) {
		dg.consumers[dependency] = append(dg.consumers[dependency], service)
	}
}

// RemoveDependency 移除依赖
func (dg *DependencyGraph) RemoveDependency(service, dependency string) {
	dg.mu.Lock()
	defer dg.mu.Unlock()
	
	dg.dependencies[service] = removeFromSlice(dg.dependencies[service], dependency)
	dg.consumers[dependency] = removeFromSlice(dg.consumers[dependency], service)
}

// GetDependencies 获取服务依赖
func (dg *DependencyGraph) GetDependencies(service string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	result := make([]string, len(dg.dependencies[service]))
	copy(result, dg.dependencies[service])
	return result
}

// GetConsumers 获取服务消费者
func (dg *DependencyGraph) GetConsumers(service string) []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	result := make([]string, len(dg.consumers[service]))
	copy(result, dg.consumers[service])
	return result
}

// GetAllServices 获取所有服务
func (dg *DependencyGraph) GetAllServices() []string {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	
	seen := make(map[string]bool)
	for svc := range dg.dependencies {
		seen[svc] = true
	}
	for svc := range dg.consumers {
		seen[svc] = true
	}
	
	result := make([]string, 0, len(seen))
	for svc := range seen {
		result = append(result, svc)
	}
	return result
}

// CheckCircular 检查循环依赖
func (dg *DependencyGraph) CheckCircular() ([]string, bool) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	
	for svc := range dg.dependencies {
		if path := dg.findCycle(svc, make(map[string]bool), make([]string, 0)); path != nil {
			return path, true
		}
	}
	return nil, false
}

func (dg *DependencyGraph) findCycle(svc string, visited map[string]bool, path []string) []string {
	if visited[svc] {
		// 找到循环，返回路径
		for i, s := range path {
			if s == svc {
				return append(path[i:], svc)
			}
		}
		return nil
	}
	
	visited[svc] = true
	path = append(path, svc)
	
	for _, dep := range dg.dependencies[svc] {
		if cycle := dg.findCycle(dep, visited, path); cycle != nil {
			return cycle
		}
	}
	
	visited[svc] = false
	return nil
}

// TopologicalSort 拓扑排序（启动顺序）
func (dg *DependencyGraph) TopologicalSort() ([]string, error) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	
	inDegree := make(map[string]int)
	// 初始化所有服务
	for svc := range dg.dependencies {
		inDegree[svc] = len(dg.dependencies[svc])
	}
	for svc := range dg.consumers {
		if _, ok := inDegree[svc]; !ok {
			inDegree[svc] = 0
		}
	}
	
	var queue []string
	for svc, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, svc)
		}
	}
	
	var result []string
	for len(queue) > 0 {
		svc := queue[0]
		queue = queue[1:]
		result = append(result, svc)
		
		// 减少消费者（依赖 svc 的服务）的入度
		for _, consumer := range dg.consumers[svc] {
			inDegree[consumer]--
			if inDegree[consumer] == 0 {
				queue = append(queue, consumer)
			}
		}
	}
	
	if len(result) != len(inDegree) {
		return nil, fmt.Errorf("circular dependency detected")
	}
	
	return result, nil
}

// ============================================================================
// 三、服务启动管理器
// ============================================================================

// ServiceStarter 服务启动管理器
type ServiceStarter struct {
	mu         sync.RWMutex
	services   map[string]Service
	graph      *DependencyGraph
	registry   Registry
}

// Service 服务接口
type Service interface {
	Name() string
	Start() error
	Stop() error
	Health() bool
}

// NewServiceStarter 创建启动管理器
func NewServiceStarter(registry Registry) *ServiceStarter {
	return &ServiceStarter{
		services: make(map[string]Service),
		graph:    NewDependencyGraph(),
		registry: registry,
	}
}

// Register 注册服务
func (ss *ServiceStarter) Register(svc Service, dependencies ...string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	
	name := svc.Name()
	ss.services[name] = svc
	for _, dep := range dependencies {
		ss.graph.AddDependency(name, dep)
	}
}

// StartAll 按依赖顺序启动所有服务
func (ss *ServiceStarter) StartAll() error {
	order, err := ss.graph.TopologicalSort()
	if err != nil {
		return err
	}
	
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	
	for _, name := range order {
		if svc, ok := ss.services[name]; ok {
			if err := svc.Start(); err != nil {
				return fmt.Errorf("failed to start %s: %w", name, err)
			}
		}
	}
	
	return nil
}

// StopAll 按依赖反序停止所有服务
func (ss *ServiceStarter) StopAll() error {
	order, err := ss.graph.TopologicalSort()
	if err != nil {
		return err
	}
	
	// 反序停止
	for i := len(order) - 1; i >= 0; i-- {
		name := order[i]
		ss.mu.RLock()
		svc, ok := ss.services[name]
		ss.mu.RUnlock()
		if ok {
			svc.Stop()
		}
	}
	
	return nil
}

// GetService 获取服务
func (ss *ServiceStarter) GetService(name string) (Service, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	svc, ok := ss.services[name]
	return svc, ok
}

// ============================================================================
// 辅助函数
// ============================================================================

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func removeFromSlice(slice []string, item string) []string {
	var result []string
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
