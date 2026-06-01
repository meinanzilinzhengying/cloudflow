// Package servicediscovery 提供服务发现功能
// 支持 Edge 集群服务发现和负载均衡
package servicediscovery

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"
)

const (
	// 默认健康检查间隔
	defaultHealthCheckInterval = 10 * time.Second
	// 默认实例超时时间
	defaultInstanceTimeout = 30 * time.Second
	// 健康检查失败阈值
	maxHealthFailures = 3
)

// EdgeInstance 表示 Edge 集群中的一个实例
type EdgeInstance struct {
	ID              string            // 实例唯一标识
	Address         string            // 实例地址（IP或主机名）
	Port            int               // 实例端口
	Weight          int               // 权重（用于加权负载均衡）
	Healthy         bool              // 健康状态
	LastHeartbeat   time.Time         // 最后一次心跳时间
	Tags            map[string]string // 标签（用于服务分组）
	ConnectionCount int               // 当前连接数（用于最少连接负载均衡）
	healthFailures  int               // 连续健康检查失败次数（内部使用）
}

// FullAddress 返回完整的地址（host:port）
func (e *EdgeInstance) FullAddress() string {
	return fmt.Sprintf("%s:%d", e.Address, e.Port)
}

// ClusterDiscovery 集群服务发现接口
type ClusterDiscovery interface {
	// GetInstances 获取所有实例列表
	GetInstances() []EdgeInstance
	// Watch 监听实例变化
	Watch(callback func(instances []EdgeInstance))
	// Register 注册实例
	Register(instance EdgeInstance) error
	// Deregister 注销实例
	Deregister(instanceID string) error
	// UpdateHealth 更新实例健康状态
	UpdateHealth(instanceID string, healthy bool)
}

// ClusterManager 管理 Edge 集群实例和负载均衡
type ClusterManager struct {
	mu         sync.RWMutex
	instances  map[string]*EdgeInstance // 实例映射
	sortedKeys []string                 // 用于一致性哈希的排序键
	roundRobin uint64                   // 轮询计数器

	discovery       ClusterDiscovery    // 服务发现组件
	healthCheckIntv time.Duration       // 健康检查间隔
	instanceTimeout time.Duration       // 实例超时时间
	stopCh          chan struct{}       // 停止信号
	stopped         sync.Once           // 确保只停止一次
	watchCallbacks  []func([]EdgeInstance) // 监听回调列表

	logger interface { // 简化日志接口
		Infof(format string, args ...interface{})
		Warnf(format string, args ...interface{})
		Errorf(format string, args ...interface{})
	}
}

// ClusterManagerOption 配置选项函数
type ClusterManagerOption func(*ClusterManager)

// WithHealthCheckInterval 设置健康检查间隔
func WithHealthCheckInterval(interval time.Duration) ClusterManagerOption {
	return func(cm *ClusterManager) {
		cm.healthCheckIntv = interval
	}
}

// WithInstanceTimeout 设置实例超时时间
func WithInstanceTimeout(timeout time.Duration) ClusterManagerOption {
	return func(cm *ClusterManager) {
		cm.instanceTimeout = timeout
	}
}

// WithLogger 设置日志器
func WithLogger(logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}) ClusterManagerOption {
	return func(cm *ClusterManager) {
		cm.logger = logger
	}
}

// NewClusterManager 创建集群管理器
func NewClusterManager(discovery ClusterDiscovery, opts ...ClusterManagerOption) *ClusterManager {
	cm := &ClusterManager{
		instances:       make(map[string]*EdgeInstance),
		discovery:       discovery,
		healthCheckIntv: defaultHealthCheckInterval,
		instanceTimeout: defaultInstanceTimeout,
		stopCh:          make(chan struct{}),
		watchCallbacks:  make([]func([]EdgeInstance), 0),
		logger:          &noopLogger{},
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(cm)
	}

	// 启动健康检查循环
	go cm.healthCheckLoop()

	// 启动实例监听
	if discovery != nil {
		go cm.watchInstances()
	}

	return cm
}

// noopLogger 空日志实现
type noopLogger struct{}

func (n *noopLogger) Infof(format string, args ...interface{})  {}
func (n *noopLogger) Warnf(format string, args ...interface{})  {}
func (n *noopLogger) Errorf(format string, args ...interface{}) {}

// GetInstances 获取所有健康实例
func (cm *ClusterManager) GetInstances() []EdgeInstance {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]EdgeInstance, 0, len(cm.instances))
	for _, inst := range cm.instances {
		if inst.Healthy {
			result = append(result, *inst)
		}
	}
	return result
}

// GetAllInstances 获取所有实例（包括不健康）
func (cm *ClusterManager) GetAllInstances() []EdgeInstance {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]EdgeInstance, 0, len(cm.instances))
	for _, inst := range cm.instances {
		result = append(result, *inst)
	}
	return result
}

// GetInstanceByLoad 使用最少连接数策略选择实例
func (cm *ClusterManager) GetInstanceByLoad() (*EdgeInstance, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var selected *EdgeInstance
	minConnections := -1

	for _, inst := range cm.instances {
		if !inst.Healthy {
			continue
		}
		if minConnections == -1 || inst.ConnectionCount < minConnections {
			minConnections = inst.ConnectionCount
			selected = inst
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("没有可用的健康实例")
	}

	return selected, nil
}

// GetInstanceByHash 使用一致性哈希选择实例
func (cm *ClusterManager) GetInstanceByHash(key string) (*EdgeInstance, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.sortedKeys) == 0 {
		return nil, fmt.Errorf("没有可用的实例")
	}

	// 计算 key 的哈希值
	h := fnv.New32a()
	h.Write([]byte(key))
	hash := h.Sum32()

	// 找到第一个大于等于 hash 的节点
	idx := sort.Search(len(cm.sortedKeys), func(i int) bool {
		return hashString(cm.sortedKeys[i]) >= hash
	})

	// 如果超出范围，回到第一个节点
	if idx >= len(cm.sortedKeys) {
		idx = 0
	}

	instanceID := cm.sortedKeys[idx]
	inst, ok := cm.instances[instanceID]
	if !ok || !inst.Healthy {
		// 如果选中的实例不健康，尝试找下一个健康的实例
		for i := 1; i <= len(cm.sortedKeys); i++ {
			nextIdx := (idx + i) % len(cm.sortedKeys)
			nextID := cm.sortedKeys[nextIdx]
			if next, ok := cm.instances[nextID]; ok && next.Healthy {
				return next, nil
			}
		}
		return nil, fmt.Errorf("没有可用的健康实例")
	}

	return inst, nil
}

// GetInstanceByRoundRobin 使用轮询策略选择实例
func (cm *ClusterManager) GetInstanceByRoundRobin() (*EdgeInstance, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	healthyInstances := make([]*EdgeInstance, 0, len(cm.instances))
	for _, inst := range cm.instances {
		if inst.Healthy {
			healthyInstances = append(healthyInstances, inst)
		}
	}

	if len(healthyInstances) == 0 {
		return nil, fmt.Errorf("没有可用的健康实例")
	}

	// 原子递增轮询计数器
	cm.roundRobin++
	idx := (cm.roundRobin - 1) % uint64(len(healthyInstances))
	return healthyInstances[idx], nil
}

// UpdateInstanceStats 更新实例统计信息
func (cm *ClusterManager) UpdateInstanceStats(instanceID string, connections int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	inst, ok := cm.instances[instanceID]
	if !ok {
		return fmt.Errorf("实例 %s 不存在", instanceID)
	}

	inst.ConnectionCount = connections
	inst.LastHeartbeat = time.Now()
	return nil
}

// AddInstance 添加实例
func (cm *ClusterManager) AddInstance(instance EdgeInstance) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.instances[instance.ID] = &instance
	cm.rebuildSortedKeys()

	cm.logger.Infof("添加实例: %s (%s)", instance.ID, instance.FullAddress())
}

// RemoveInstance 移除实例
func (cm *ClusterManager) RemoveInstance(instanceID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, ok := cm.instances[instanceID]; ok {
		delete(cm.instances, instanceID)
		cm.rebuildSortedKeys()
		cm.logger.Infof("移除实例: %s", instanceID)
	}
}

// UpdateHealth 更新实例健康状态
func (cm *ClusterManager) UpdateHealth(instanceID string, healthy bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	inst, ok := cm.instances[instanceID]
	if !ok {
		return
	}

	if healthy {
		inst.healthFailures = 0
		if !inst.Healthy {
			inst.Healthy = true
			cm.logger.Infof("实例 %s 恢复健康", instanceID)
		}
	} else {
		inst.healthFailures++
		if inst.healthFailures >= maxHealthFailures && inst.Healthy {
			inst.Healthy = false
			cm.logger.Warnf("实例 %s 被标记为不健康（连续%d次健康检查失败）", instanceID, inst.healthFailures)
		}
	}
}

// rebuildSortedKeys 重建排序键（用于一致性哈希）
func (cm *ClusterManager) rebuildSortedKeys() {
	cm.sortedKeys = make([]string, 0, len(cm.instances))
	for id := range cm.instances {
		cm.sortedKeys = append(cm.sortedKeys, id)
	}
	sort.Slice(cm.sortedKeys, func(i, j int) bool {
		return hashString(cm.sortedKeys[i]) < hashString(cm.sortedKeys[j])
	})
}

// hashString 计算字符串哈希值
func hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// healthCheckLoop 健康检查循环
func (cm *ClusterManager) healthCheckLoop() {
	ticker := time.NewTicker(cm.healthCheckIntv)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.performHealthCheck()
		case <-cm.stopCh:
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (cm *ClusterManager) performHealthCheck() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for id, inst := range cm.instances {
		// 检查是否超时
		if now.Sub(inst.LastHeartbeat) > cm.instanceTimeout {
			inst.healthFailures++
			if inst.healthFailures >= maxHealthFailures && inst.Healthy {
				inst.Healthy = false
				cm.logger.Warnf("实例 %s 心跳超时，标记为不健康", id)
			}
		}
	}
}

// watchInstances 监听实例变化
func (cm *ClusterManager) watchInstances() {
	if cm.discovery == nil {
		return
	}

	cm.discovery.Watch(func(instances []EdgeInstance) {
		cm.mu.Lock()
		defer cm.mu.Unlock()

		// 更新实例列表
		newInstances := make(map[string]*EdgeInstance)
		for i := range instances {
			newInstances[instances[i].ID] = &instances[i]
		}

		// 合并现有连接数信息
		for id, newInst := range newInstances {
			if oldInst, ok := cm.instances[id]; ok {
				newInst.ConnectionCount = oldInst.ConnectionCount
			}
		}

		cm.instances = newInstances
		cm.rebuildSortedKeys()

		// 触发回调
		for _, callback := range cm.watchCallbacks {
			go callback(instances)
		}

		cm.logger.Infof("实例列表已更新，当前 %d 个实例", len(instances))
	})
}

// AddWatchCallback 添加实例变化监听回调
func (cm *ClusterManager) AddWatchCallback(callback func([]EdgeInstance)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.watchCallbacks = append(cm.watchCallbacks, callback)
}

// Stop 停止集群管理器
func (cm *ClusterManager) Stop() {
	cm.stopped.Do(func() {
		close(cm.stopCh)
		cm.logger.Info("集群管理器已停止")
	})
}

// GetInstanceCount 获取实例数量
func (cm *ClusterManager) GetInstanceCount() (total, healthy int) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	total = len(cm.instances)
	for _, inst := range cm.instances {
		if inst.Healthy {
			healthy++
		}
	}
	return
}

// SelectInstance 智能选择实例（优先最少连接，其次轮询）
func (cm *ClusterManager) SelectInstance(key string) (*EdgeInstance, error) {
	// 首先尝试最少连接策略
	inst, err := cm.GetInstanceByLoad()
	if err == nil {
		return inst, nil
	}

	// 如果失败，尝试一致性哈希
	if key != "" {
		inst, err = cm.GetInstanceByHash(key)
		if err == nil {
			return inst, nil
		}
	}

	// 最后尝试轮询
	return cm.GetInstanceByRoundRobin()
}
