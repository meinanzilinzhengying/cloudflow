//go:build linux

// Package servicemesh 提供服务网格能力
// - 服务注册发现
// - 负载均衡
// - 熔断降级
// - 健康检查
// - 服务间通信治理
package servicemesh

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、服务实例定义
// ============================================================================

// InstanceStatus 服务实例状态
type InstanceStatus string

const (
	InstanceStatusHealthy   InstanceStatus = "healthy"
	InstanceStatusUnhealthy InstanceStatus = "unhealthy"
	InstanceStatusStarting  InstanceStatus = "starting"
	InstanceStatusStopping  InstanceStatus = "stopping"
)

// ServiceInstance 服务实例
type ServiceInstance struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Protocol string            `json:"protocol"` // http/grpc/tcp
	Metadata map[string]string `json:"metadata,omitempty"`
	Status   InstanceStatus    `json:"status"`
	Version  string            `json:"version"`
	Weight   int               `json:"weight"` // 负载均衡权重
	
	// 时间戳
	RegisterAt  time.Time `json:"register_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// Address 返回服务地址
func (si *ServiceInstance) Address() string {
	return fmt.Sprintf("%s:%d", si.Host, si.Port)
}

// FullAddress 返回完整地址（含协议）
func (si *ServiceInstance) FullAddress() string {
	return fmt.Sprintf("%s://%s", si.Protocol, si.Address())
}

// IsHealthy 是否健康
func (si *ServiceInstance) IsHealthy() bool {
	return si.Status == InstanceStatusHealthy
}

// IsExpired 是否过期（超过心跳间隔）
func (si *ServiceInstance) IsExpired(timeout time.Duration) bool {
	return time.Since(si.LastHeartbeat) > timeout
}

// ============================================================================
// 二、服务注册发现接口
// ============================================================================

// Registry 服务注册发现接口
type Registry interface {
	Register(instance *ServiceInstance) error
	Deregister(instanceID string) error
	GetService(name string) ([]*ServiceInstance, error)
	GetInstance(name, instanceID string) (*ServiceInstance, error)
	Watch(name string) (chan []*ServiceInstance, error)
	Close() error
}

// ============================================================================
// 三、内存注册中心（开发环境/测试用）
// ============================================================================

// MemoryRegistry 内存注册中心
type MemoryRegistry struct {
	mu        sync.RWMutex
	services  map[string]map[string]*ServiceInstance // serviceName -> instanceID -> instance
	heartbeat time.Duration
	stopCh    chan struct{}
	watchers  map[string][]chan []*ServiceInstance
	watchMu   sync.RWMutex
}

// NewMemoryRegistry 创建内存注册中心
func NewMemoryRegistry() *MemoryRegistry {
	r := &MemoryRegistry{
		services:  make(map[string]map[string]*ServiceInstance),
		heartbeat: 30 * time.Second,
		stopCh:    make(chan struct{}),
		watchers:  make(map[string][]chan []*ServiceInstance),
	}
	go r.cleanupLoop()
	return r
}

// Register 注册服务
func (r *MemoryRegistry) Register(instance *ServiceInstance) error {
	if instance.ID == "" {
		instance.ID = fmt.Sprintf("%s-%s-%d", instance.Name, instance.Host, instance.Port)
	}
	if instance.Status == "" {
		instance.Status = InstanceStatusHealthy
	}
	instance.RegisterAt = time.Now()
	instance.LastHeartbeat = time.Now()
	if instance.Weight <= 0 {
		instance.Weight = 1
	}
	
	r.mu.Lock()
	if r.services[instance.Name] == nil {
		r.services[instance.Name] = make(map[string]*ServiceInstance)
	}
	old := r.services[instance.Name][instance.ID]
	r.services[instance.Name][instance.ID] = instance
	r.mu.Unlock()
	
	// 通知 watcher
	if old == nil || old.Status != instance.Status {
		r.notifyWatchers(instance.Name)
	}
	
	return nil
}

// Deregister 注销服务
func (r *MemoryRegistry) Deregister(instanceID string) error {
	r.mu.Lock()
	for name, instances := range r.services {
		if _, ok := instances[instanceID]; ok {
			delete(instances, instanceID)
			if len(instances) == 0 {
				delete(r.services, name)
			}
			r.mu.Unlock()
			r.notifyWatchers(name)
			return nil
		}
	}
	r.mu.Unlock()
	return fmt.Errorf("instance not found: %s", instanceID)
}

// GetService 获取服务所有实例
func (r *MemoryRegistry) GetService(name string) ([]*ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	instances := r.services[name]
	if len(instances) == 0 {
		return []*ServiceInstance{}, nil
	}
	
	result := make([]*ServiceInstance, 0, len(instances))
	for _, inst := range instances {
		result = append(result, inst)
	}
	return result, nil
}

// GetInstance 获取单个实例
func (r *MemoryRegistry) GetInstance(name, instanceID string) (*ServiceInstance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	instances := r.services[name]
	if instances == nil {
		return nil, fmt.Errorf("service not found: %s", name)
	}
	inst := instances[instanceID]
	if inst == nil {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}
	return inst, nil
}

// Watch 监听服务变化
func (r *MemoryRegistry) Watch(name string) (chan []*ServiceInstance, error) {
	ch := make(chan []*ServiceInstance, 10)
	
	r.watchMu.Lock()
	r.watchers[name] = append(r.watchers[name], ch)
	r.watchMu.Unlock()
	
	return ch, nil
}

// Heartbeat 心跳更新
func (r *MemoryRegistry) Heartbeat(instanceID string) error {
	r.mu.Lock()
	for _, instances := range r.services {
		if inst, ok := instances[instanceID]; ok {
			inst.LastHeartbeat = time.Now()
			if inst.Status == InstanceStatusUnhealthy {
				inst.Status = InstanceStatusHealthy
			}
			r.mu.Unlock()
			return nil
		}
	}
	r.mu.Unlock()
	return fmt.Errorf("instance not found: %s", instanceID)
}

// Close 关闭注册中心
func (r *MemoryRegistry) Close() error {
	close(r.stopCh)
	
	r.watchMu.Lock()
	for _, chs := range r.watchers {
		for _, ch := range chs {
			close(ch)
		}
	}
	r.watchers = make(map[string][]chan []*ServiceInstance)
	r.watchMu.Unlock()
	
	return nil
}

// notifyWatchers 通知监听器
func (r *MemoryRegistry) notifyWatchers(name string) {
	instances, _ := r.GetService(name)
	
	r.watchMu.RLock()
	chs := r.watchers[name]
	r.watchMu.RUnlock()
	
	for _, ch := range chs {
		select {
		case ch <- instances:
		default:
		}
	}
}

// cleanupLoop 清理过期实例
func (r *MemoryRegistry) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

func (r *MemoryRegistry) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	timeout := r.heartbeat * 2
	for name, instances := range r.services {
		changed := false
		for id, inst := range instances {
			if inst.IsExpired(timeout) {
				delete(instances, id)
				changed = true
			}
		}
		if len(instances) == 0 {
			delete(r.services, name)
		} else if changed {
			r.mu.Unlock()
			r.notifyWatchers(name)
			r.mu.Lock()
		}
	}
}

// GetServiceCount 获取服务数量
func (r *MemoryRegistry) GetServiceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.services)
}

// GetInstanceCount 获取实例总数
func (r *MemoryRegistry) GetInstanceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, instances := range r.services {
		count += len(instances)
	}
	return count
}

// ListServices 列出所有服务名
func (r *MemoryRegistry) ListServices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	result := make([]string, 0, len(r.services))
	for name := range r.services {
		result = append(result, name)
	}
	return result
}
