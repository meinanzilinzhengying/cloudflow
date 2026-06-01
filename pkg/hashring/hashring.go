// Package hashring 实现一致性哈希环，用于 Agent 到 Edge 节点的分配
// 支持 10000+ Agent 的稳定连接分配，扩缩容时最小化迁移
package hashring

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
)

// HashFunc 定义哈希函数签名
type HashFunc func([]byte) uint32

// DefaultHash 使用 FNV-1a 32 位哈希
func DefaultHash(data []byte) uint32 {
	h := fnv.New32a()
	h.Write(data)
	return h.Sum32()
}

// Ring 一致性哈希环
type Ring struct {
	mu        sync.RWMutex
	nodes     []*Node        // 排序后的虚拟节点列表
	nodeMap   map[string]int // 真实节点名 -> 虚拟节点数量
	replicas  int            // 每个真实节点的虚拟节点数
	hashFunc  HashFunc       // 哈希函数
}

// Node 哈希环上的虚拟节点
type Node struct {
	Key     string // 虚拟节点键（如 "edge-01#160"）
	NodeID  string // 真实节点 ID（如 "edge-01" }
	HashKey uint32 // 哈希值，用于排序
}

// NewRing 创建一致性哈希环
// replicas: 每个真实节点的虚拟节点数（默认 150，支持 ~10000 agent 时迁移率 < 1%）
func NewRing(replicas int) *Ring {
	if replicas <= 0 {
		replicas = 150
	}
	return &Ring{
		nodes:    make([]*Node, 0),
		nodeMap:  make(map[string]int),
		replicas: replicas,
		hashFunc: DefaultHash,
	}
}

// AddNode 添加真实节点到哈希环
func (r *Ring) AddNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodeMap[nodeID]; exists {
		return // 已存在
	}

	for i := 0; i < r.replicas; i++ {
		vKey := fmt.Sprintf("%s#%d", nodeID, i)
		hashKey := r.hashFunc([]byte(vKey))
		r.nodes = append(r.nodes, &Node{
			Key:     vKey,
			NodeID:  nodeID,
			HashKey: hashKey,
		})
	}
	r.nodeMap[nodeID] = r.replicas
	r.sortNodes()
}

// RemoveNode 从哈希环移除真实节点
func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count, exists := r.nodeMap[nodeID]
	if !exists {
		return
	}

	// 过滤掉该节点的所有虚拟节点
	filtered := make([]*Node, 0, len(r.nodes)-count)
	for _, n := range r.nodes {
		if n.NodeID != nodeID {
			filtered = append(filtered, n)
		}
	}
	r.nodes = filtered
	delete(r.nodeMap, nodeID)
}

// GetNode 获取 key 应该分配到的节点
// 使用二分查找，时间复杂度 O(log N)
func (r *Ring) GetNode(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.nodes) == 0 {
		return "", false
	}

	hashKey := r.hashFunc([]byte(key))
	idx := r.search(hashKey)
	return r.nodes[idx].NodeID, true
}

// GetNodes 获取 key 应该分配到的 N 个不同节点（用于副本/故障转移）
func (r *Ring) GetNodes(key string, n int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.nodes) == 0 {
		return nil
	}

	if n <= 0 {
		n = 1
	}

	hashKey := r.hashFunc([]byte(key))
	startIdx := r.search(hashKey)

	result := make([]string, 0, n)
	seen := make(map[string]bool)
	totalNodes := len(r.nodes)

	for i := 0; i < totalNodes && len(result) < n; i++ {
		idx := (startIdx + i) % totalNodes
		nodeID := r.nodes[idx].NodeID
		if !seen[nodeID] {
			seen[nodeID] = true
			result = append(result, nodeID)
		}
	}

	return result
}

// GetNodeCount 获取真实节点数量
func (r *Ring) GetNodeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodeMap)
}

// GetNodesList 获取所有真实节点 ID
func (r *Ring) GetNodesList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	nodes := make([]string, 0, len(r.nodeMap))
	for id := range r.nodeMap {
		nodes = append(nodes, id)
	}
	return nodes
}

// search 二分查找第一个 hashKey >= 目标哈希的虚拟节点
func (r *Ring) search(hashKey uint32) int {
	idx := sort.Search(len(r.nodes), func(i int) bool {
		return r.nodes[i].HashKey >= hashKey
	})
	// 环形：如果超出范围，回到第一个节点
	if idx >= len(r.nodes) {
		idx = 0
	}
	return idx
}

func (r *Ring) sortNodes() {
	sort.Slice(r.nodes, func(i, j int) bool {
		return r.nodes[i].HashKey < r.nodes[j].HashKey
	})
}

// MigrationRate 计算添加/移除节点后的迁移率
// 返回需要迁移的 key 数量占总数的比例
func (r *Ring) MigrationRate(keys []string, oldRing *Ring) float64 {
	if len(keys) == 0 {
		return 0
	}

	migrated := 0
	for _, key := range keys {
		oldNode, _ := oldRing.GetNode(key)
		newNode, _ := r.GetNode(key)
		if oldNode != newNode {
			migrated++
		}
	}

	return float64(migrated) / float64(len(keys))
}

// ParseNodeID 从虚拟节点键中解析真实节点 ID
// 例如 "edge-01#160" -> "edge-01"
func ParseNodeID(vKey string) string {
	for i := len(vKey) - 1; i >= 0; i-- {
		if vKey[i] == '#' {
			return vKey[:i]
		}
	}
	return vKey
}

// GenerateProbeID 生成稳定的 Probe ID（基于主机名和序列号）
func GenerateProbeID(hostname string, index int) string {
	return fmt.Sprintf("probe-%s-%d", hostname, index)
}

// HashString 计算字符串的哈希值（用于调试和测试）
func HashString(s string) uint32 {
	return DefaultHash([]byte(s))
}

// HashInt 计算整数的哈希值
func HashInt(i int) uint32 {
	return DefaultHash([]byte(strconv.Itoa(i)))
}
