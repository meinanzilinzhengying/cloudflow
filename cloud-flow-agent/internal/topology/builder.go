//go:build linux

// Package topology 提供拓扑构建功能
package topology

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// TopologyBuilder 拓扑构建器
type TopologyBuilder struct {
	mu       sync.RWMutex
	graph    *TopologyGraph
	log      *logger.Logger
	
	// 配置
	config   BuilderConfig
	
	// 数据源
	sources  map[string]DataSource
	
	// 控制
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// BuilderConfig 构建器配置
type BuilderConfig struct {
	Enabled          bool          `yaml:"enabled" json:"enabled"`
	RefreshInterval  time.Duration `yaml:"refresh_interval" json:"refresh_interval"`
	AutoDiscovery    bool          `yaml:"auto_discovery" json:"auto_discovery"`
	IncludePods      bool          `yaml:"include_pods" json:"include_pods"`
	IncludeVMs       bool          `yaml:"include_vms" json:"include_vms"`
	IncludePhysical  bool          `yaml:"include_physical" json:"include_physical"`
}

// DefaultBuilderConfig 默认配置
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		Enabled:         true,
		RefreshInterval: 5 * time.Minute,
		AutoDiscovery:   true,
		IncludePods:     true,
		IncludeVMs:      true,
		IncludePhysical: true,
	}
}

// DataSource 数据源接口
type DataSource interface {
	Name() string
	Discover(ctx context.Context) ([]*TopologyNode, []*TopologyEdge, error)
	IsAvailable() bool
}

// NewTopologyBuilder 创建拓扑构建器
func NewTopologyBuilder(config BuilderConfig, log *logger.Logger) *TopologyBuilder {
	return &TopologyBuilder{
		config:  config,
		log:     log,
		graph:   NewTopologyGraph("fullstack", "网络全栈拓扑", "fullstack"),
		sources: make(map[string]DataSource),
		stopCh:  make(chan struct{}),
	}
}

// RegisterSource 注册数据源
func (b *TopologyBuilder) RegisterSource(source DataSource) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sources[source.Name()] = source
}

// Build 构建拓扑
func (b *TopologyBuilder) Build(ctx context.Context) error {
	if !b.config.Enabled {
		return nil
	}
	
	b.log.Info("开始构建网络全栈拓扑...")
	
	// 清空现有拓扑
	b.graph = NewTopologyGraph("fullstack", "网络全栈拓扑", "fullstack")
	
	// 构建层级结构
	if b.config.IncludePhysical {
		if err := b.buildPhysicalLayer(ctx); err != nil {
			b.log.Warnf("构建物理层失败: %v", err)
		}
	}
	
	if b.config.IncludeVMs {
		if err := b.buildVMLayer(ctx); err != nil {
			b.log.Warnf("构建VM层失败: %v", err)
		}
	}
	
	if err := b.buildNodeLayer(ctx); err != nil {
		b.log.Warnf("构建Node层失败: %v", err)
	}
	
	if b.config.IncludePods {
		if err := b.buildPodLayer(ctx); err != nil {
			b.log.Warnf("构建Pod层失败: %v", err)
		}
	}
	
	// 构建连接关系
	b.buildConnections()
	
	// 构建业务分组
	b.buildGroups()
	
	b.log.Infof("拓扑构建完成: 节点=%d, 边=%d, 分组=%d",
		b.graph.Stats.NodeCount, b.graph.Stats.EdgeCount, b.graph.Stats.GroupCount)
	
	return nil
}

// buildPhysicalLayer 构建物理层
func (b *TopologyBuilder) buildPhysicalLayer(ctx context.Context) error {
	// 获取物理网络设备信息
	// 实际生产环境应从CMDB或网络管理系统获取
	
	// 模拟物理设备
	devices := []*TopologyNode{
		{
			ID:       "switch-core-01",
			Name:     "核心交换机-01",
			Type:     NodeTypeSwitch,
			Status:   NodeStatusHealthy,
			Level:    0,
			Metadata: map[string]interface{}{
				"vendor":   "Cisco",
				"model":    "Nexus 9000",
				"location": "机房A",
			},
			Labels: map[string]string{
				"tier": "core",
				"env":  "production",
			},
		},
		{
			ID:       "switch-core-02",
			Name:     "核心交换机-02",
			Type:     NodeTypeSwitch,
			Status:   NodeStatusHealthy,
			Level:    0,
			Metadata: map[string]interface{}{
				"vendor":   "Cisco",
				"model":    "Nexus 9000",
				"location": "机房B",
			},
			Labels: map[string]string{
				"tier": "core",
				"env":  "production",
			},
		},
		{
			ID:       "router-edge-01",
			Name:     "边缘路由器-01",
			Type:     NodeTypeRouter,
			Status:   NodeStatusHealthy,
			Level:    0,
			Metadata: map[string]interface{}{
				"vendor": "Juniper",
				"model":  "MX480",
			},
			Labels: map[string]string{
				"tier": "edge",
				"env":  "production",
			},
		},
	}
	
	for _, device := range devices {
		if err := b.graph.AddNode(device); err != nil {
			b.log.Warnf("添加物理设备失败: %v", err)
		}
	}
	
	// 添加物理设备间的连接
	edges := []*TopologyEdge{
		{
			Source:   "switch-core-01",
			Target:   "switch-core-02",
			Type:     EdgeTypeNetwork,
			Status:   EdgeStatusNormal,
			Bandwidth: 10000, // 10Gbps
		},
		{
			Source:   "switch-core-01",
			Target:   "router-edge-01",
			Type:     EdgeTypeNetwork,
			Status:   EdgeStatusNormal,
			Bandwidth: 10000,
		},
	}
	
	for _, edge := range edges {
		if err := b.graph.AddEdge(edge); err != nil {
			b.log.Warnf("添加边失败: %v", err)
		}
	}
	
	return nil
}

// buildVMLayer 构建VM层
func (b *TopologyBuilder) buildVMLayer(ctx context.Context) error {
	// 从虚拟化平台获取VM信息
	// 实际生产环境应从vCenter/OpenStack获取
	
	vms := []*TopologyNode{
		{
			ID:       "vm-k8s-master-01",
			Name:     "k8s-master-01",
			Type:     NodeTypeVM,
			Status:   NodeStatusHealthy,
			Level:    1,
			ParentID: "switch-core-01",
			Metadata: map[string]interface{}{
				"ip":       "10.0.1.10",
				"vcpu":     8,
				"memory":   "32GB",
				"hypervisor": "VMware",
			},
			Labels: map[string]string{
				"role": "master",
				"env":  "production",
			},
		},
		{
			ID:       "vm-k8s-worker-01",
			Name:     "k8s-worker-01",
			Type:     NodeTypeVM,
			Status:   NodeStatusHealthy,
			Level:    1,
			ParentID: "switch-core-01",
			Metadata: map[string]interface{}{
				"ip":       "10.0.1.11",
				"vcpu":     16,
				"memory":   "64GB",
				"hypervisor": "VMware",
			},
			Labels: map[string]string{
				"role": "worker",
				"env":  "production",
			},
		},
		{
			ID:       "vm-k8s-worker-02",
			Name:     "k8s-worker-02",
			Type:     NodeTypeVM,
			Status:   NodeStatusHealthy,
			Level:    1,
			ParentID: "switch-core-02",
			Metadata: map[string]interface{}{
				"ip":       "10.0.1.12",
				"vcpu":     16,
				"memory":   "64GB",
				"hypervisor": "VMware",
			},
			Labels: map[string]string{
				"role": "worker",
				"env":  "production",
			},
		},
	}
	
	for _, vm := range vms {
		if err := b.graph.AddNode(vm); err != nil {
			b.log.Warnf("添加VM失败: %v", err)
		}
	}
	
	return nil
}

// buildNodeLayer 构建Node层
func (b *TopologyBuilder) buildNodeLayer(ctx context.Context) error {
	// 从Kubernetes获取Node信息
	hostname, _ := os.Hostname()
	
	nodes := []*TopologyNode{
		{
			ID:       "node-" + hostname,
			Name:     hostname,
			Type:     NodeTypeNode,
			Status:   NodeStatusHealthy,
			Level:    2,
			ParentID: "vm-k8s-worker-01",
			Metadata: map[string]interface{}{
				"ip":         getLocalIP(),
				"os":         "Linux",
				"kernel":     "5.15.0",
				"container_runtime": "containerd",
			},
			Labels: map[string]string{
				"kubernetes.io/os":   "linux",
				"node-role.kubernetes.io/worker": "true",
			},
		},
	}
	
	// 添加更多节点（如果有）
	for i := 2; i <= 3; i++ {
		nodeName := fmt.Sprintf("k8s-worker-0%d", i)
		nodes = append(nodes, &TopologyNode{
			ID:       "node-" + nodeName,
			Name:     nodeName,
			Type:     NodeTypeNode,
			Status:   NodeStatusHealthy,
			Level:    2,
			ParentID: fmt.Sprintf("vm-k8s-worker-0%d", i),
			Metadata: map[string]interface{}{
				"ip": fmt.Sprintf("10.0.1.%d", 10+i),
			},
			Labels: map[string]string{
				"kubernetes.io/os": "linux",
			},
		})
	}
	
	for _, node := range nodes {
		if err := b.graph.AddNode(node); err != nil {
			b.log.Warnf("添加Node失败: %v", err)
		}
	}
	
	return nil
}

// buildPodLayer 构建Pod层
func (b *TopologyBuilder) buildPodLayer(ctx context.Context) error {
	// 从Kubernetes获取Pod信息
	// 实际生产环境应使用k8s client-go
	
	pods := []*TopologyNode{
		{
			ID:       "pod-nginx-001",
			Name:     "nginx-001",
			Type:     NodeTypePod,
			Status:   NodeStatusHealthy,
			Level:    3,
			ParentID: "node-" + getHostname(),
			GroupID:  "group-web",
			Metadata: map[string]interface{}{
				"namespace": "default",
				"image":     "nginx:latest",
				"restarts":  0,
			},
			Labels: map[string]string{
				"app": "nginx",
				"tier": "frontend",
			},
		},
		{
			ID:       "pod-app-001",
			Name:     "app-001",
			Type:     NodeTypePod,
			Status:   NodeStatusHealthy,
			Level:    3,
			ParentID: "node-" + getHostname(),
			GroupID:  "group-app",
			Metadata: map[string]interface{}{
				"namespace": "default",
				"image":     "myapp:v1.0",
				"restarts":  0,
			},
			Labels: map[string]string{
				"app": "myapp",
				"tier": "backend",
			},
		},
		{
			ID:       "pod-db-001",
			Name:     "db-001",
			Type:     NodeTypePod,
			Status:   NodeStatusHealthy,
			Level:    3,
			ParentID: "node-" + getHostname(),
			GroupID:  "group-db",
			Metadata: map[string]interface{}{
				"namespace": "default",
				"image":     "mysql:8.0",
				"restarts":  0,
			},
			Labels: map[string]string{
				"app": "mysql",
				"tier": "database",
			},
		},
	}
	
	for _, pod := range pods {
		if err := b.graph.AddNode(pod); err != nil {
			b.log.Warnf("添加Pod失败: %v", err)
		}
	}
	
	// 添加Pod间的服务依赖关系
	edges := []*TopologyEdge{
		{
			Source: "pod-nginx-001",
			Target: "pod-app-001",
			Type:   EdgeTypeService,
			Status: EdgeStatusNormal,
			Metadata: map[string]interface{}{
				"protocol": "HTTP",
				"port":     8080,
			},
		},
		{
			Source: "pod-app-001",
			Target: "pod-db-001",
			Type:   EdgeTypeService,
			Status: EdgeStatusNormal,
			Metadata: map[string]interface{}{
				"protocol": "TCP",
				"port":     3306,
			},
		},
	}
	
	for _, edge := range edges {
		if err := b.graph.AddEdge(edge); err != nil {
			b.log.Warnf("添加边失败: %v", err)
		}
	}
	
	return nil
}

// buildConnections 构建连接关系
func (b *TopologyBuilder) buildConnections() {
	// 构建层级间的连接
	for level := 0; level < 3; level++ {
		parents := b.graph.GetNodesByLevel(level)
		children := b.graph.GetNodesByLevel(level + 1)
		
		for _, child := range children {
			for _, parent := range parents {
				// 简单的匹配逻辑：检查ParentID或网络连通性
				if child.ParentID == parent.ID {
					edge := &TopologyEdge{
						Source: child.ID,
						Target: parent.ID,
						Type:   EdgeTypeParent,
						Status: EdgeStatusNormal,
					}
					if err := b.graph.AddEdge(edge); err != nil {
						b.log.Warnf("添加父子边失败: %v", err)
					}
				}
			}
		}
	}
}

// buildGroups 构建业务分组
func (b *TopologyBuilder) buildGroups() {
	groups := []*TopologyGroup{
		{
			ID:      "group-web",
			Name:    "Web服务",
			Type:    GroupTypeService,
			Color:   "#4CAF50",
			NodeIDs: []string{},
			Labels: map[string]string{
				"tier": "frontend",
			},
		},
		{
			ID:      "group-app",
			Name:    "应用服务",
			Type:    GroupTypeService,
			Color:   "#2196F3",
			NodeIDs: []string{},
			Labels: map[string]string{
				"tier": "backend",
			},
		},
		{
			ID:      "group-db",
			Name:    "数据库服务",
			Type:    GroupTypeService,
			Color:   "#FF9800",
			NodeIDs: []string{},
			Labels: map[string]string{
				"tier": "database",
			},
		},
		{
			ID:      "group-infra",
			Name:    "基础设施",
			Type:    GroupTypeBusiness,
			Color:   "#9E9E9E",
			NodeIDs: []string{},
			Labels: map[string]string{
				"tier": "infrastructure",
			},
		},
	}
	
	for _, group := range groups {
		// 收集属于该分组的节点
		for _, node := range b.graph.Nodes {
			if node.GroupID == group.ID {
				group.NodeIDs = append(group.NodeIDs, node.ID)
			}
		}
		
		if err := b.graph.AddGroup(group); err != nil {
			b.log.Warnf("添加分组失败: %v", err)
		}
	}
}

// Start 启动自动刷新
func (b *TopologyBuilder) Start(ctx context.Context) error {
	if !b.config.Enabled || !b.config.AutoDiscovery {
		return nil
	}
	
	// 初始构建
	if err := b.Build(ctx); err != nil {
		return err
	}
	
	// 定期刷新
	b.wg.Add(1)
	go b.refreshLoop(ctx)
	
	return nil
}

// Stop 停止
func (b *TopologyBuilder) Stop() {
	close(b.stopCh)
	b.wg.Wait()
}

// refreshLoop 刷新循环
func (b *TopologyBuilder) refreshLoop(ctx context.Context) {
	defer b.wg.Done()
	
	ticker := time.NewTicker(b.config.RefreshInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case <-ticker.C:
			if err := b.Build(ctx); err != nil {
				b.log.Warnf("拓扑刷新失败: %v", err)
			}
		}
	}
}

// GetGraph 获取拓扑图
func (b *TopologyBuilder) GetGraph() *TopologyGraph {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.graph
}

// UpdateNodeMetrics 更新节点指标
func (b *TopologyBuilder) UpdateNodeMetrics(nodeID string, metrics NodeMetrics) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.graph != nil {
		b.graph.UpdateNodeMetrics(nodeID, metrics)
	}
}

// UpdateNodeStatus 更新节点状态
func (b *TopologyBuilder) UpdateNodeStatus(nodeID string, status NodeStatus) {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.graph != nil {
		b.graph.UpdateNodeStatus(nodeID, status)
	}
}

// 辅助函数

func getHostname() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		return "unknown"
	}
	return hostname
}

func getLocalIP() string {
	// 简化实现
	return "127.0.0.1"
}

// K8sDataSource Kubernetes数据源
type K8sDataSource struct {
	namespace string
}

// NewK8sDataSource 创建K8s数据源
func NewK8sDataSource(namespace string) *K8sDataSource {
	return &K8sDataSource{namespace: namespace}
}

// Name 返回名称
func (k *K8sDataSource) Name() string {
	return "kubernetes"
}

// Discover 发现节点
func (k *K8sDataSource) Discover(ctx context.Context) ([]*TopologyNode, []*TopologyEdge, error) {
	// 实际实现应使用client-go获取K8s资源
	return nil, nil, nil
}

// IsAvailable 检查是否可用
func (k *K8sDataSource) IsAvailable() bool {
	// 检查K8s连接
	return false
}

// VMDataSource VM数据源
type VMDataSource struct {
	endpoint string
}

// NewVMDataSource 创建VM数据源
func NewVMDataSource(endpoint string) *VMDataSource {
	return &VMDataSource{endpoint: endpoint}
}

// Name 返回名称
func (v *VMDataSource) Name() string {
	return "vsphere"
}

// Discover 发现节点
func (v *VMDataSource) Discover(ctx context.Context) ([]*TopologyNode, []*TopologyEdge, error) {
	// 实际实现应从vCenter获取VM信息
	return nil, nil, nil
}

// IsAvailable 检查是否可用
func (v *VMDataSource) IsAvailable() bool {
	return v.endpoint != ""
}
