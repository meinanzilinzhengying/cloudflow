//go:build linux

// Package topology 提供网络全栈拓扑功能
// - Pod/Node/VM/物理设备层级
// - 告警高亮、下钻指标
// - 横向/纵向布局、业务分组
package topology

import (
	"fmt"
	"sync"
	"time"
)

// NodeType 节点类型
type NodeType string

const (
	NodeTypePod       NodeType = "pod"       // Kubernetes Pod
	NodeTypeNode      NodeType = "node"      // Kubernetes Node
	NodeTypeVM        NodeType = "vm"        // 虚拟机
	NodeTypePhysical  NodeType = "physical"  // 物理设备
	NodeTypeSwitch    NodeType = "switch"    // 交换机
	NodeTypeRouter    NodeType = "router"    // 路由器
	NodeTypeFirewall  NodeType = "firewall"  // 防火墙
	NodeTypeLoadBalancer NodeType = "lb"     // 负载均衡器
	NodeTypeService   NodeType = "service"   // 服务
	NodeTypeNamespace NodeType = "namespace" // 命名空间
	NodeTypeCluster   NodeType = "cluster"   // 集群
	NodeTypeApp       NodeType = "app"       // 应用
)

// String 返回节点类型名称
func (t NodeType) String() string {
	return string(t)
}

// NodeStatus 节点状态
type NodeStatus string

const (
	NodeStatusHealthy   NodeStatus = "healthy"   // 健康
	NodeStatusWarning   NodeStatus = "warning"   // 警告
	NodeStatusCritical  NodeStatus = "critical"  // 严重
	NodeStatusUnknown   NodeStatus = "unknown"   // 未知
	NodeStatusOffline   NodeStatus = "offline"   // 离线
)

// LayoutType 布局类型
type LayoutType string

const (
	LayoutHorizontal LayoutType = "horizontal" // 横向布局
	LayoutVertical   LayoutType = "vertical"   // 纵向布局
	LayoutRadial     LayoutType = "radial"     // 径向布局
	LayoutForce      LayoutType = "force"      // 力导向布局
)

// TopologyNode 拓扑节点
type TopologyNode struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        NodeType               `json:"type"`
	Status      NodeStatus             `json:"status"`
	Level       int                    `json:"level"`       // 层级: 0=物理设备, 1=VM, 2=Node, 3=Pod
	ParentID    string                 `json:"parent_id,omitempty"`
	GroupID     string                 `json:"group_id,omitempty"`   // 业务分组ID
	
	// 位置信息 (用于可视化)
	X           float64                `json:"x,omitempty"`
	Y           float64                `json:"y,omitempty"`
	Width       float64                `json:"width,omitempty"`
	Height      float64                `json:"height,omitempty"`
	
	// 元数据
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
	
	// 指标数据
	Metrics     NodeMetrics            `json:"metrics,omitempty"`
	
	// 告警状态
	AlertCount  int                    `json:"alert_count"`
	AlertLevel  string                 `json:"alert_level,omitempty"`
	
	// 时间戳
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	LastSeen    time.Time              `json:"last_seen"`
}

// NodeMetrics 节点指标
type NodeMetrics struct {
	// 网络指标
	LatencyMs      float64 `json:"latency_ms,omitempty"`
	PacketLossRate float64 `json:"packet_loss_rate,omitempty"`
	RetransmitRate float64 `json:"retransmit_rate,omitempty"`
	ThroughputBps  float64 `json:"throughput_bps,omitempty"`
	
	// 资源指标
	CPUPercent     float64 `json:"cpu_percent,omitempty"`
	MemoryPercent  float64 `json:"memory_percent,omitempty"`
	DiskPercent    float64 `json:"disk_percent,omitempty"`
	
	// 连接指标
	ConnectionCount int    `json:"connection_count,omitempty"`
	RequestCount    int64  `json:"request_count,omitempty"`
	ErrorCount      int64  `json:"error_count,omitempty"`
}

// TopologyEdge 拓扑边（连接关系）
type TopologyEdge struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Target     string            `json:"target"`
	Type       EdgeType          `json:"type"`
	Status     EdgeStatus        `json:"status"`
	
	// 带宽和流量
	Bandwidth  float64           `json:"bandwidth,omitempty"`  // Mbps
	TrafficIn  float64           `json:"traffic_in,omitempty"`  // Bps
	TrafficOut float64           `json:"traffic_out,omitempty"` // Bps
	
	// 延迟和丢包
	LatencyMs  float64           `json:"latency_ms,omitempty"`
	LossRate   float64           `json:"loss_rate,omitempty"`
	
	// 元数据
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// EdgeType 边类型
type EdgeType string

const (
	EdgeTypeNetwork   EdgeType = "network"   // 网络连接
	EdgeTypeParent    EdgeType = "parent"    // 父子关系
	EdgeTypeService   EdgeType = "service"   // 服务依赖
	EdgeTypeTraffic   EdgeType = "traffic"   // 流量关系
)

// EdgeStatus 边状态
type EdgeStatus string

const (
	EdgeStatusNormal   EdgeStatus = "normal"
	EdgeStatusWarning  EdgeStatus = "warning"
	EdgeStatusError    EdgeStatus = "error"
	EdgeStatusOffline  EdgeStatus = "offline"
)

// TopologyGroup 拓扑分组（业务分组）
type TopologyGroup struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        GroupType         `json:"type"`
	ParentID    string            `json:"parent_id,omitempty"`
	
	// 分组内的节点
	NodeIDs     []string          `json:"node_ids,omitempty"`
	
	// 样式配置
	Color       string            `json:"color,omitempty"`
	Icon        string            `json:"icon,omitempty"`
	
	// 元数据
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
	
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// GroupType 分组类型
type GroupType string

const (
	GroupTypeBusiness GroupType = "business" // 业务分组
	GroupTypeTeam     GroupType = "team"     // 团队分组
	GroupTypeEnv      GroupType = "env"      // 环境分组
	GroupTypeApp      GroupType = "app"      // 应用分组
	GroupTypeService  GroupType = "service"  // 服务分组
)

// TopologyGraph 拓扑图
type TopologyGraph struct {
	mu     sync.RWMutex
	
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type"` // fullstack, service, network
	
	Nodes   map[string]*TopologyNode `json:"nodes"`
	Edges   map[string]*TopologyEdge `json:"edges"`
	Groups  map[string]*TopologyGroup `json:"groups"`
	
	// 层级结构
	Levels  map[int][]string `json:"levels"` // level -> node IDs
	
	// 布局配置
	Layout  LayoutConfig      `json:"layout"`
	
	// 统计
	Stats   TopologyStats     `json:"stats"`
	
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// LayoutConfig 布局配置
type LayoutConfig struct {
	Type        LayoutType `json:"type"`
	Direction   string     `json:"direction,omitempty"` // TB, BT, LR, RL
	NodeSpacing float64    `json:"node_spacing"`
	LevelSpacing float64   `json:"level_spacing"`
	
	// 力导向布局参数
	Repulsion   float64    `json:"repulsion,omitempty"`
	Attraction  float64    `json:"attraction,omitempty"`
	Gravity     float64    `json:"gravity,omitempty"`
}

// TopologyStats 拓扑统计
type TopologyStats struct {
	NodeCount      int            `json:"node_count"`
	EdgeCount      int            `json:"edge_count"`
	GroupCount     int            `json:"group_count"`
	
	// 按类型统计
	NodeTypeCount  map[NodeType]int `json:"node_type_count"`
	
	// 健康状态统计
	HealthyCount   int            `json:"healthy_count"`
	WarningCount   int            `json:"warning_count"`
	CriticalCount  int            `json:"critical_count"`
	OfflineCount   int            `json:"offline_count"`
	
	// 告警统计
	TotalAlerts    int            `json:"total_alerts"`
}

// NewTopologyGraph 创建拓扑图
func NewTopologyGraph(id, name, graphType string) *TopologyGraph {
	return &TopologyGraph{
		ID:        id,
		Name:      name,
		Type:      graphType,
		Nodes:     make(map[string]*TopologyNode),
		Edges:     make(map[string]*TopologyEdge),
		Groups:    make(map[string]*TopologyGroup),
		Levels:    make(map[int][]string),
		Layout: LayoutConfig{
			Type:         LayoutVertical,
			Direction:    "TB",
			NodeSpacing:  50,
			LevelSpacing: 100,
		},
		NodeTypeCount: make(map[NodeType]int),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// AddNode 添加节点
func (g *TopologyGraph) AddNode(node *TopologyNode) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if node.ID == "" {
		return fmt.Errorf("节点ID不能为空")
	}
	
	if _, exists := g.Nodes[node.ID]; exists {
		return fmt.Errorf("节点已存在: %s", node.ID)
	}
	
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}
	node.UpdatedAt = time.Now()
	node.LastSeen = time.Now()
	
	g.Nodes[node.ID] = node
	g.Levels[node.Level] = append(g.Levels[node.Level], node.ID)
	g.NodeTypeCount[node.Type]++
	g.Stats.NodeCount++
	
	g.updateStats()
	return nil
}

// RemoveNode 移除节点
func (g *TopologyGraph) RemoveNode(nodeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}
	
	// 移除相关边
	for edgeID, edge := range g.Edges {
		if edge.Source == nodeID || edge.Target == nodeID {
			delete(g.Edges, edgeID)
			g.Stats.EdgeCount--
		}
	}
	
	// 从层级中移除
	levelNodes := g.Levels[node.Level]
	for i, id := range levelNodes {
		if id == nodeID {
			g.Levels[node.Level] = append(levelNodes[:i], levelNodes[i+1:]...)
			break
		}
	}
	
	g.NodeTypeCount[node.Type]--
	delete(g.Nodes, nodeID)
	g.Stats.NodeCount--
	
	g.updateStats()
}

// AddEdge 添加边
func (g *TopologyGraph) AddEdge(edge *TopologyEdge) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if edge.ID == "" {
		edge.ID = fmt.Sprintf("%s-%s", edge.Source, edge.Target)
	}
	
	// 检查节点是否存在
	if _, exists := g.Nodes[edge.Source]; !exists {
		return fmt.Errorf("源节点不存在: %s", edge.Source)
	}
	if _, exists := g.Nodes[edge.Target]; !exists {
		return fmt.Errorf("目标节点不存在: %s", edge.Target)
	}
	
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now()
	}
	edge.UpdatedAt = time.Now()
	
	g.Edges[edge.ID] = edge
	g.Stats.EdgeCount++
	
	return nil
}

// RemoveEdge 移除边
func (g *TopologyGraph) RemoveEdge(edgeID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if _, exists := g.Edges[edgeID]; exists {
		delete(g.Edges, edgeID)
		g.Stats.EdgeCount--
	}
}

// AddGroup 添加分组
func (g *TopologyGraph) AddGroup(group *TopologyGroup) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if group.ID == "" {
		return fmt.Errorf("分组ID不能为空")
	}
	
	if group.CreatedAt.IsZero() {
		group.CreatedAt = time.Now()
	}
	group.UpdatedAt = time.Now()
	
	g.Groups[group.ID] = group
	g.Stats.GroupCount++
	
	return nil
}

// GetNode 获取节点
func (g *TopologyGraph) GetNode(nodeID string) *TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Nodes[nodeID]
}

// GetNodesByType 按类型获取节点
func (g *TopologyGraph) GetNodesByType(nodeType NodeType) []*TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var result []*TopologyNode
	for _, node := range g.Nodes {
		if node.Type == nodeType {
			result = append(result, node)
		}
	}
	return result
}

// GetNodesByLevel 按层级获取节点
func (g *TopologyGraph) GetNodesByLevel(level int) []*TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var result []*TopologyNode
	for _, nodeID := range g.Levels[level] {
		if node, exists := g.Nodes[nodeID]; exists {
			result = append(result, node)
		}
	}
	return result
}

// GetChildren 获取子节点
func (g *TopologyGraph) GetChildren(parentID string) []*TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var result []*TopologyNode
	for _, node := range g.Nodes {
		if node.ParentID == parentID {
			result = append(result, node)
		}
	}
	return result
}

// GetNeighbors 获取邻居节点
func (g *TopologyGraph) GetNeighbors(nodeID string) []*TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	neighborIDs := make(map[string]struct{})
	for _, edge := range g.Edges {
		if edge.Source == nodeID {
			neighborIDs[edge.Target] = struct{}{}
		}
		if edge.Target == nodeID {
			neighborIDs[edge.Source] = struct{}{}
		}
	}
	
	var result []*TopologyNode
	for id := range neighborIDs {
		if node, exists := g.Nodes[id]; exists {
			result = append(result, node)
		}
	}
	return result
}

// UpdateNodeStatus 更新节点状态
func (g *TopologyGraph) UpdateNodeStatus(nodeID string, status NodeStatus) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if node, exists := g.Nodes[nodeID]; exists {
		node.Status = status
		node.UpdatedAt = time.Now()
		node.LastSeen = time.Now()
		g.updateStats()
	}
}

// UpdateNodeMetrics 更新节点指标
func (g *TopologyGraph) UpdateNodeMetrics(nodeID string, metrics NodeMetrics) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if node, exists := g.Nodes[nodeID]; exists {
		node.Metrics = metrics
		node.UpdatedAt = time.Now()
		node.LastSeen = time.Now()
	}
}

// UpdateNodeAlert 更新节点告警
func (g *TopologyGraph) UpdateNodeAlert(nodeID string, alertCount int, alertLevel string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if node, exists := g.Nodes[nodeID]; exists {
		node.AlertCount = alertCount
		node.AlertLevel = alertLevel
		
		// 根据告警级别更新状态
		if alertCount > 0 {
			switch alertLevel {
			case "critical", "emergency":
				node.Status = NodeStatusCritical
			case "warning":
				if node.Status != NodeStatusCritical {
					node.Status = NodeStatusWarning
				}
			}
		} else if node.Status == NodeStatusWarning || node.Status == NodeStatusCritical {
			node.Status = NodeStatusHealthy
		}
		
		node.UpdatedAt = time.Now()
		g.updateStats()
	}
}

// SetLayout 设置布局
func (g *TopologyGraph) SetLayout(config LayoutConfig) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Layout = config
}

// ToJSON 转换为JSON格式
func (g *TopologyGraph) ToJSON() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	return map[string]interface{}{
		"id":         g.ID,
		"name":       g.Name,
		"type":       g.Type,
		"nodes":      g.Nodes,
		"edges":      g.Edges,
		"groups":     g.Groups,
		"levels":     g.Levels,
		"layout":     g.Layout,
		"stats":      g.Stats,
		"created_at": g.CreatedAt,
		"updated_at": g.UpdatedAt,
	}
}

// GetStats 获取统计
func (g *TopologyGraph) GetStats() TopologyStats {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.Stats
}

// updateStats 更新统计
func (g *TopologyGraph) updateStats() {
	g.Stats.HealthyCount = 0
	g.Stats.WarningCount = 0
	g.Stats.CriticalCount = 0
	g.Stats.OfflineCount = 0
	g.Stats.TotalAlerts = 0
	
	for _, node := range g.Nodes {
		switch node.Status {
		case NodeStatusHealthy:
			g.Stats.HealthyCount++
		case NodeStatusWarning:
			g.Stats.WarningCount++
		case NodeStatusCritical:
			g.Stats.CriticalCount++
		case NodeStatusOffline:
			g.Stats.OfflineCount++
		}
		g.Stats.TotalAlerts += node.AlertCount
	}
	
	g.UpdatedAt = time.Now()
}

// TopologyFilter 拓扑过滤器
type TopologyFilter struct {
	NodeTypes   []NodeType        `json:"node_types,omitempty"`
	Statuses    []NodeStatus      `json:"statuses,omitempty"`
	GroupID     string            `json:"group_id,omitempty"`
	Level       int               `json:"level,omitempty"`
	HasAlert    bool              `json:"has_alert,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// FilterNodes 过滤节点
func (g *TopologyGraph) FilterNodes(filter TopologyFilter) []*TopologyNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	var result []*TopologyNode
	
	for _, node := range g.Nodes {
		// 类型过滤
		if len(filter.NodeTypes) > 0 {
			found := false
			for _, t := range filter.NodeTypes {
				if node.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		// 状态过滤
		if len(filter.Statuses) > 0 {
			found := false
			for _, s := range filter.Statuses {
				if node.Status == s {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		// 分组过滤
		if filter.GroupID != "" && node.GroupID != filter.GroupID {
			continue
		}
		
		// 层级过滤
		if filter.Level >= 0 && node.Level != filter.Level {
			continue
		}
		
		// 告警过滤
		if filter.HasAlert && node.AlertCount == 0 {
			continue
		}
		
		// 标签过滤
		if len(filter.Labels) > 0 {
			for k, v := range filter.Labels {
				if node.Labels[k] != v {
					continue
				}
			}
		}
		
		result = append(result, node)
	}
	
	return result
}

// DrillDownRequest 下钻请求
type DrillDownRequest struct {
	NodeID      string   `json:"node_id"`
	MetricType  string   `json:"metric_type"`  // latency, throughput, error_rate, etc.
	TimeRange   string   `json:"time_range"`   // 1h, 6h, 24h, 7d
	Aggregation string   `json:"aggregation"`  // avg, max, min, p99
}

// DrillDownResponse 下钻响应
type DrillDownResponse struct {
	NodeID     string                 `json:"node_id"`
	MetricType string                 `json:"metric_type"`
	Data       []MetricDataPoint      `json:"data"`
	Statistics MetricStatistics       `json:"statistics"`
}

// MetricDataPoint 指标数据点
type MetricDataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// MetricStatistics 指标统计
type MetricStatistics struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Avg   float64 `json:"avg"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Count int     `json:"count"`
}
