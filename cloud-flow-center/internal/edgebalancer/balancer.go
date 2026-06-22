//go:build linux

package edgebalancer

import (
	"fmt"
	"hash/crc32"
	"math"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 一、一致性哈希负载均衡器
// 解决 Edge 有状态问题：Agent 通过一致性哈希选择 Edge 节点
// 支持虚拟节点，扩缩容时最小化数据迁移
// ============================================================================

// EdgeNode Edge 节点信息
type EdgeNode struct {
	ID       string            `json:"id"`
	Address  string            `json:"address"`   // host:port
	Weight   int               `json:"weight"`    // 权重
	Capacity int64             `json:"capacity"`  // 最大承载连接数
	Labels   map[string]string `json:"labels,omitempty"`
	Healthy  bool              `json:"healthy"`
	AddedAt  time.Time         `json:"added_at"`
}

// VirtualNode 虚拟节点
type VirtualNode struct {
	Hash     uint32
	NodeID   string
	RealNode *EdgeNode
}

// ConsistentHashBalancer 一致性哈希负载均衡器
type ConsistentHashBalancer struct {
	mu            sync.RWMutex
	replicas      int                 // 每个真实节点的虚拟节点数
	nodes         map[string]*EdgeNode
	virtualNodes  []VirtualNode
	nodeMap       map[string][]uint32   // nodeID -> virtual node hashes
}

// NewConsistentHashBalancer 创建一致性哈希负载均衡器
func NewConsistentHashBalancer(replicas int) *ConsistentHashBalancer {
	if replicas <= 0 {
		replicas = 150 // 默认每个节点 150 个虚拟节点
	}
	return &ConsistentHashBalancer{
		replicas:     replicas,
		nodes:        make(map[string]*EdgeNode),
		virtualNodes: make([]VirtualNode, 0),
		nodeMap:      make(map[string][]uint32),
	}
}

// AddNode 添加节点
func (chb *ConsistentHashBalancer) AddNode(node *EdgeNode) error {
	if node == nil || node.ID == "" {
		return fmt.Errorf("node and node.ID required")
	}
	if node.Weight <= 0 {
		node.Weight = 1
	}
	if node.Capacity <= 0 {
		node.Capacity = math.MaxInt64
	}
	if node.AddedAt.IsZero() {
		node.AddedAt = time.Now()
	}

	chb.mu.Lock()
	defer chb.mu.Unlock()

	if _, exists := chb.nodes[node.ID]; exists {
		return fmt.Errorf("node %s already exists", node.ID)
	}

	chb.nodes[node.ID] = node

	// 根据权重计算虚拟节点数
	virtualCount := chb.replicas * node.Weight
	hashes := make([]uint32, 0, virtualCount)

	for i := 0; i < virtualCount; i++ {
		hash := chb.hashKey(fmt.Sprintf("%s#%d", node.ID, i))
		hashes = append(hashes, hash)
		chb.virtualNodes = append(chb.virtualNodes, VirtualNode{
			Hash:     hash,
			NodeID:   node.ID,
			RealNode: node,
		})
	}

	chb.nodeMap[node.ID] = hashes
	chb.sortVirtualNodes()
	return nil
}

// RemoveNode 移除节点
func (chb *ConsistentHashBalancer) RemoveNode(nodeID string) {
	chb.mu.Lock()
	defer chb.mu.Unlock()

	delete(chb.nodes, nodeID)
	delete(chb.nodeMap, nodeID)

	// 移除虚拟节点
	newNodes := make([]VirtualNode, 0, len(chb.virtualNodes))
	for _, vn := range chb.virtualNodes {
		if vn.NodeID != nodeID {
			newNodes = append(newNodes, vn)
		}
	}
	chb.virtualNodes = newNodes
	chb.sortVirtualNodes()
}

// UpdateNodeHealth 更新节点健康状态
func (chb *ConsistentHashBalancer) UpdateNodeHealth(nodeID string, healthy bool) {
	chb.mu.Lock()
	defer chb.mu.Unlock()

	if node, ok := chb.nodes[nodeID]; ok {
		node.Healthy = healthy
	}
}

// GetNode 根据 key 获取节点（用于 Agent 选择 Edge）
func (chb *ConsistentHashBalancer) GetNode(key string) (*EdgeNode, error) {
	chb.mu.RLock()
	defer chb.mu.RUnlock()

	if len(chb.virtualNodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}

	hash := chb.hashKey(key)

	// 二分查找最近的虚拟节点
	idx := chb.searchNearest(hash)

	// 找到健康节点（跳过不健康节点）
	for i := 0; i < len(chb.virtualNodes); i++ {
		vn := chb.virtualNodes[(idx+i)%len(chb.virtualNodes)]
		if vn.RealNode.Healthy {
			return vn.RealNode, nil
		}
	}
	return nil, fmt.Errorf("no healthy nodes available")
}

// GetNodesForKey 获取 key 的 N 个节点（用于数据分片副本）
func (chb *ConsistentHashBalancer) GetNodesForKey(key string, n int) ([]*EdgeNode, error) {
	chb.mu.RLock()
	defer chb.mu.RUnlock()

	if len(chb.virtualNodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}
	if n <= 0 {
		n = 1
	}

	hash := chb.hashKey(key)
	idx := chb.searchNearest(hash)

	result := make([]*EdgeNode, 0, n)
	seen := make(map[string]bool)

	for i := 0; i < len(chb.virtualNodes) && len(result) < n; i++ {
		vn := chb.virtualNodes[(idx+i)%len(chb.virtualNodes)]
		if !seen[vn.NodeID] && vn.RealNode.Healthy {
			result = append(result, vn.RealNode)
			seen[vn.NodeID] = true
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no healthy nodes available")
	}
	return result, nil
}

// GetAllNodes 获取所有节点
func (chb *ConsistentHashBalancer) GetAllNodes() []*EdgeNode {
	chb.mu.RLock()
	defer chb.mu.RUnlock()

	result := make([]*EdgeNode, 0, len(chb.nodes))
	for _, node := range chb.nodes {
		result = append(result, node)
	}
	return result
}

// GetHealthyNodes 获取健康节点
func (chb *ConsistentHashBalancer) GetHealthyNodes() []*EdgeNode {
	chb.mu.RLock()
	defer chb.mu.RUnlock()

	result := make([]*EdgeNode, 0)
	for _, node := range chb.nodes {
		if node.Healthy {
			result = append(result, node)
		}
	}
	return result
}

// NodeCount 节点数量
func (chb *ConsistentHashBalancer) NodeCount() int {
	chb.mu.RLock()
	defer chb.mu.RUnlock()
	return len(chb.nodes)
}

// VirtualNodeCount 虚拟节点数量
func (chb *ConsistentHashBalancer) VirtualNodeCount() int {
	chb.mu.RLock()
	defer chb.mu.RUnlock()
	return len(chb.virtualNodes)
}

// MigrationRate 计算节点移除时的数据迁移率
func (chb *ConsistentHashBalancer) MigrationRate(removedNodeID string) float64 {
	chb.mu.RLock()
	defer chb.mu.RUnlock()

	removedHashes := chb.nodeMap[removedNodeID]
	if len(removedHashes) == 0 {
		return 0
	}

	migrated := 0
	for _, hash := range removedHashes {
		idx := chb.searchNearest(hash)
		// 如果最近节点不是被移除的节点，说明数据需要迁移
		if chb.virtualNodes[idx].NodeID != removedNodeID {
			migrated++
		}
	}
	return float64(migrated) / float64(len(removedHashes))
}

// hashKey 计算 key 的哈希值
func (chb *ConsistentHashBalancer) hashKey(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

// sortVirtualNodes 排序虚拟节点
func (chb *ConsistentHashBalancer) sortVirtualNodes() {
	sort.Slice(chb.virtualNodes, func(i, j int) bool {
		return chb.virtualNodes[i].Hash < chb.virtualNodes[j].Hash
	})
}

// searchNearest 二分查找最近的虚拟节点
func (chb *ConsistentHashBalancer) searchNearest(hash uint32) int {
	idx := sort.Search(len(chb.virtualNodes), func(i int) bool {
		return chb.virtualNodes[i].Hash >= hash
	})
	if idx >= len(chb.virtualNodes) {
		idx = 0
	}
	return idx
}

// ============================================================================
// 二、数据分片管理器
// ============================================================================

// ShardStrategy 分片策略
type ShardStrategy string

const (
	ShardByProbeID   ShardStrategy = "probe_id"   // 按探针ID分片
	ShardByTenantID  ShardStrategy = "tenant_id"  // 按租户ID分片
	ShardByHash      ShardStrategy = "hash"       // 按内容哈希分片
)

// ShardManager 分片管理器
type ShardManager struct {
	mu       sync.RWMutex
	balancer *ConsistentHashBalancer
	strategy ShardStrategy
	shards   map[int][]string // shardIndex -> key列表（用于统计）
}

// NewShardManager 创建分片管理器
func NewShardManager(strategy ShardStrategy, replicas int) *ShardManager {
	return &ShardManager{
		balancer: NewConsistentHashBalancer(replicas),
		strategy: strategy,
		shards:   make(map[int][]string),
	}
}

// AddShardNode 添加分片节点
func (sm *ShardManager) AddShardNode(node *EdgeNode) error {
	return sm.balancer.AddNode(node)
}

// RemoveShardNode 移除分片节点
func (sm *ShardManager) RemoveShardNode(nodeID string) {
	sm.balancer.RemoveNode(nodeID)
}

// GetShardForKey 获取 key 对应的分片节点
func (sm *ShardManager) GetShardForKey(key string) (*EdgeNode, error) {
	return sm.balancer.GetNode(key)
}

// GetShardKeys 获取分片 key（根据策略生成）
func (sm *ShardManager) GetShardKey(probeID, tenantID string, data []byte) string {
	switch sm.strategy {
	case ShardByProbeID:
		return probeID
	case ShardByTenantID:
		return tenantID
	case ShardByHash:
		return fmt.Sprintf("%d", crc32.ChecksumIEEE(data))
	default:
		return probeID
	}
}

// GetShardDistribution 获取分片分布统计
func (sm *ShardManager) GetShardDistribution(sampleKeys []string) map[string]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	distribution := make(map[string]int)
	for _, key := range sampleKeys {
		node, err := sm.balancer.GetNode(key)
		if err != nil {
			continue
		}
		distribution[node.ID]++
	}
	return distribution
}

// Rebalance 重新平衡（检查是否需要迁移）
func (sm *ShardManager) Rebalance() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 找到负载不均的节点
	nodes := sm.balancer.GetAllNodes()
	if len(nodes) <= 1 {
		return nil
	}

	var totalCapacity int64
	for _, node := range nodes {
		totalCapacity += node.Capacity
	}
	avgCapacity := totalCapacity / int64(len(nodes))

	var overloaded []string
	for _, node := range nodes {
		if node.Capacity > avgCapacity*2 {
			overloaded = append(overloaded, node.ID)
		}
	}
	return overloaded
}

// Stats 获取分片统计
func (sm *ShardManager) Stats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	nodes := sm.balancer.GetAllNodes()
	healthy := sm.balancer.GetHealthyNodes()

	return map[string]interface{}{
		"strategy":        string(sm.strategy),
		"total_nodes":     len(nodes),
		"healthy_nodes":   len(healthy),
		"virtual_nodes":   sm.balancer.VirtualNodeCount(),
		"rebalance_needed": len(sm.Rebalance()) > 0,
	}
}

// ============================================================================
// 三、扩容/缩容管理器（保证数据完整性）
// ============================================================================

// ScalingManager 扩缩容管理器
type ScalingManager struct {
	mu        sync.RWMutex
	balancer  *ConsistentHashBalancer
	draining  map[string]bool  // 正在下线的节点
	warmingUp map[string]bool  // 正在预热的节点
}

// NewScalingManager 创建扩缩容管理器
func NewScalingManager(balancer *ConsistentHashBalancer) *ScalingManager {
	return &ScalingManager{
		balancer:  balancer,
		draining:  make(map[string]bool),
		warmingUp: make(map[string]bool),
	}
}

// ScaleUp 扩容（添加新节点）
func (sm *ScalingManager) ScaleUp(node *EdgeNode) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.warmingUp[node.ID] {
		return fmt.Errorf("node %s is already warming up", node.ID)
	}

	node.Healthy = true
	if err := sm.balancer.AddNode(node); err != nil {
		return err
	}

	sm.warmingUp[node.ID] = true
	// 预热完成后标记（实际应由外部健康检查完成）
	go func() {
		time.Sleep(30 * time.Second)
		sm.mu.Lock()
		delete(sm.warmingUp, node.ID)
		sm.mu.Unlock()
	}()

	return nil
}

// ScaleDown 缩容（优雅下线）
func (sm *ScalingManager) ScaleDown(nodeID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.draining[nodeID] {
		return fmt.Errorf("node %s is already draining", nodeID)
	}

	// 1. 标记节点为不健康（不再接收新连接）
	sm.balancer.UpdateNodeHealth(nodeID, false)
	sm.draining[nodeID] = true

	// 2. 等待数据排空（实际应由外部监控完成）
	go func() {
		time.Sleep(60 * time.Second) // 默认 60s 排空时间
		sm.mu.Lock()
		sm.balancer.RemoveNode(nodeID)
		delete(sm.draining, nodeID)
		sm.mu.Unlock()
	}()

	return nil
}

// GetDrainStatus 获取下线状态
func (sm *ScalingManager) GetDrainStatus(nodeID string) (bool, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.draining[nodeID], sm.warmingUp[nodeID]
}

// GetMigrationPlan 生成扩容/缩容时的数据迁移计划
func (sm *ScalingManager) GetMigrationPlan(addedNodes, removedNodes []string) map[string][]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	plan := make(map[string][]string) // sourceNode -> []targetNodeKeys

	// 计算移除节点影响的数据分布
	for _, removedID := range removedNodes {
		removedHashes := sm.balancer.nodeMap[removedID]
		for _, hash := range removedHashes {
			idx := sm.balancer.searchNearest(hash)
			if idx < len(sm.balancer.virtualNodes) {
				targetNode := sm.balancer.virtualNodes[idx].NodeID
				if targetNode != removedID {
					plan[removedID] = append(plan[removedID], targetNode)
				}
			}
		}
	}
	return plan
}

// Stats 获取扩缩容统计
func (sm *ScalingManager) Stats() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"draining_nodes":  len(sm.draining),
		"warming_up_nodes": len(sm.warmingUp),
		"total_nodes":      sm.balancer.NodeCount(),
	}
}
