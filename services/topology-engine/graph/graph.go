// Package graph 高性能拓扑图数据结构
//
// 设计目标:
//   - 百万级 edge 支持
//   - 增量更新 (O(1) node/edge lookup)
//   - 无锁读 (RWMutex)
//   - 内存紧凑 (node/edge 使用 slice + index)
//
// Graph 类型:
//   - ServiceGraph:   service → service 依赖关系
//   - ProcessGraph:   process → process 通信关系
//   - PodGraph:       pod → pod 网络拓扑
//   - NamespaceGraph: namespace → namespace 流量关系
package graph

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// 基础类型
// ---------------------------------------------------------------------------

// NodeID 节点唯一标识
type NodeID = string

// EdgeKey 边的唯一标识，用于 map 键
type EdgeKey struct {
	Source NodeID
	Target NodeID
}

// String 返回 "source→target" 格式的字符串表示
func (k EdgeKey) String() string {
	return k.Source + "\u2192" + k.Target
}

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

// Node 表示拓扑图中的一个节点
type Node struct {
	ID   NodeID
	Name string
	Type string // service / process / pod / namespace

	Namespace string
	Metadata  map[string]string

	// 流量指标
	BytesIn      uint64
	BytesOut     uint64
	Errors       uint64
	RequestCount uint64

	// 延迟统计
	AvgLatencyNs  uint64 // 计算值
	TotalLatencyNs uint64 // 累加器
	LatencyCount  uint64 // 采样计数

	// 增量更新标记
	Active bool
}

// ---------------------------------------------------------------------------
// Edge
// ---------------------------------------------------------------------------

// Edge 表示拓扑图中的一条边
type Edge struct {
	Source, Target NodeID
	Protocol       string
	Port           uint16

	// 流量指标
	Bytes        uint64
	Packets      uint64
	Latency      uint64 // 平均延迟 (ns)
	TotalLatency uint64 // 延迟累加器
	LatencyCount uint64 // 延迟采样计数
	Errors       uint64
	RequestCount uint64

	// 增量更新标记
	Active bool

	// 计算权重 (用于可视化)
	// 公式: 0.3*normalizedBytes + 0.3*normalizedLatency + 0.2*normalizedErrors + 0.2*normalizedRequestCount
	Weight float64
}

// ---------------------------------------------------------------------------
// Graph
// ---------------------------------------------------------------------------

// Graph 高性能拓扑图，支持百万级边、增量更新和无锁读
type Graph struct {
	mu sync.RWMutex

	nodes map[NodeID]*Node
	edges map[EdgeKey]*Edge

	// 插入顺序，保证确定性输出
	nodeOrder []NodeID
	edgeOrder []EdgeKey

	version   uint64
	timestamp int64

	graphType string
	tenantID  string

	maxNodes int
	maxEdges int
}

// NewGraph 创建一个新的拓扑图实例
func NewGraph(graphType, tenantID string, maxNodes, maxEdges int) *Graph {
	if maxNodes <= 0 {
		maxNodes = 1_000_000
	}
	if maxEdges <= 0 {
		maxEdges = 10_000_000
	}
	return &Graph{
		nodes:     make(map[NodeID]*Node, maxNodes),
		edges:     make(map[EdgeKey]*Edge, maxEdges),
		nodeOrder: make([]NodeID, 0, maxNodes),
		edgeOrder: make([]EdgeKey, 0, maxEdges),
		graphType: graphType,
		tenantID:  tenantID,
		maxNodes:  maxNodes,
		maxEdges:  maxEdges,
		timestamp: time.Now().UnixNano(),
	}
}

// ---------------------------------------------------------------------------
// 写操作 (需要写锁)
// ---------------------------------------------------------------------------

// AddOrUpdateNode 添加或获取已有节点。若节点已存在则返回现有指针。
func (g *Graph) AddOrUpdateNode(id NodeID, name, nodeType, namespace string, metadata map[string]string) *Node {
	g.mu.Lock()
	defer g.mu.Unlock()

	if n, ok := g.nodes[id]; ok {
		// 已存在：更新活跃标记和元数据
		n.Active = true
		if name != "" {
			n.Name = name
		}
		if nodeType != "" {
			n.Type = nodeType
		}
		if namespace != "" {
			n.Namespace = namespace
		}
		if len(metadata) > 0 {
			if n.Metadata == nil {
				n.Metadata = make(map[string]string, len(metadata))
			}
			for k, v := range metadata {
				n.Metadata[k] = v
			}
		}
		return n
	}

	// 容量检查
	if len(g.nodes) >= g.maxNodes {
		return nil
	}

	n := &Node{
		ID:        id,
		Name:      name,
		Type:      nodeType,
		Namespace: namespace,
		Metadata:  metadata,
		Active:    true,
	}
	g.nodes[id] = n
	g.nodeOrder = append(g.nodeOrder, id)
	g.version++
	g.timestamp = time.Now().UnixNano()
	return n
}

// AddOrUpdateEdge 添加或获取已有边。若边已存在则返回现有指针。
func (g *Graph) AddOrUpdateEdge(source, target NodeID, protocol string, port uint16) *Edge {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := EdgeKey{Source: source, Target: target}

	if e, ok := g.edges[key]; ok {
		e.Active = true
		if protocol != "" {
			e.Protocol = protocol
		}
		if port != 0 {
			e.Port = port
		}
		return e
	}

	// 容量检查
	if len(g.edges) >= g.maxEdges {
		return nil
	}

	e := &Edge{
		Source:   source,
		Target:   target,
		Protocol: protocol,
		Port:     port,
		Active:   true,
	}
	g.edges[key] = e
	g.edgeOrder = append(g.edgeOrder, key)
	g.version++
	g.timestamp = time.Now().UnixNano()
	return e
}

// AccumulateEdge 累加流量指标到边和对应节点（热路径，已优化）。
// 该方法是线程安全的，同时更新边指标和源/目标节点的计数器。
func (g *Graph) AccumulateEdge(source, target NodeID, bytes, packets, latencyNs, errors uint64) {
	g.mu.Lock()

	key := EdgeKey{Source: source, Target: target}
	e, ok := g.edges[key]
	if !ok {
		// 容量检查
		if len(g.edges) >= g.maxEdges {
			g.mu.Unlock()
			return
		}
		e = &Edge{
			Source: source,
			Target: target,
			Active: true,
		}
		g.edges[key] = e
		g.edgeOrder = append(g.edgeOrder, key)
	}

	// 累加边指标
	e.Bytes += bytes
	e.Packets += packets
	e.RequestCount++
	e.TotalLatency += latencyNs
	e.LatencyCount++
	e.Latency = e.TotalLatency / e.LatencyCount
	e.Errors += errors
	e.Active = true

	// 更新源节点
	if sn, ok := g.nodes[source]; ok {
		sn.BytesOut += bytes
		sn.RequestCount++
		sn.TotalLatencyNs += latencyNs
		sn.LatencyCount++
		sn.AvgLatencyNs = sn.TotalLatencyNs / sn.LatencyCount
		sn.Errors += errors
		sn.Active = true
	}

	// 更新目标节点
	if tn, ok := g.nodes[target]; ok {
		tn.BytesIn += bytes
		tn.RequestCount++
		tn.TotalLatencyNs += latencyNs
		tn.LatencyCount++
		tn.AvgLatencyNs = tn.TotalLatencyNs / tn.LatencyCount
		tn.Errors += errors
		tn.Active = true
	}

	g.version++
	g.timestamp = time.Now().UnixNano()
	g.mu.Unlock()
}

// MarkAllInactive 将所有节点和边标记为不活跃（增量更新前调用）。
func (g *Graph) MarkAllInactive() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, n := range g.nodes {
		n.Active = false
	}
	for _, e := range g.edges {
		e.Active = false
	}
	g.version++
	g.timestamp = time.Now().UnixNano()
}

// RemoveInactiveNodes 移除所有 Active=false 的节点及其关联边，返回移除数量。
func (g *Graph) RemoveInactiveNodes() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	removed := 0

	// 收集不活跃的节点 ID
	inactiveNodes := make(map[NodeID]struct{})
	for id, n := range g.nodes {
		if !n.Active {
			inactiveNodes[id] = struct{}{}
			removed++
		}
	}

	// 移除关联边
	for key, e := range g.edges {
		if _, ok := inactiveNodes[key.Source]; ok || _, ok := inactiveNodes[key.Target]; ok {
			delete(g.edges, key)
			e.Active = false
		}
	}

	// 移除节点
	for id := range inactiveNodes {
		delete(g.nodes, id)
	}

	// 重建顺序切片
	g.rebuildOrders()

	if removed > 0 {
		g.version++
		g.timestamp = time.Now().UnixNano()
	}
	return removed
}

// PruneEdges 移除权重低于 threshold 的边，返回移除数量。
func (g *Graph) PruneEdges(threshold float64) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	removed := 0
	for key, e := range g.edges {
		if e.Weight < threshold {
			delete(g.edges, key)
			e.Active = false
			removed++
		}
	}

	if removed > 0 {
		g.rebuildOrders()
		g.version++
		g.timestamp = time.Now().UnixNano()
	}
	return removed
}

// RecomputeWeights 重新计算所有活跃边的权重。
// 权重公式: 0.3*normalizedBytes + 0.3*normalizedLatency + 0.2*normalizedErrors + 0.2*normalizedRequestCount
func (g *Graph) RecomputeWeights() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.edges) == 0 {
		return
	}

	// 第一遍：找最大值用于归一化
	var maxBytes, maxLatency, maxErrors, maxRequests uint64
	for _, e := range g.edges {
		if !e.Active {
			continue
		}
		if e.Bytes > maxBytes {
			maxBytes = e.Bytes
		}
		if e.Latency > maxLatency {
			maxLatency = e.Latency
		}
		if e.Errors > maxErrors {
			maxErrors = e.Errors
		}
		if e.RequestCount > maxRequests {
			maxRequests = e.RequestCount
		}
	}

	// 避免除零
	invMaxBytes := float64(1)
	invMaxLatency := float64(1)
	invMaxErrors := float64(1)
	invMaxRequests := float64(1)
	if maxBytes > 0 {
		invMaxBytes = 1.0 / float64(maxBytes)
	}
	if maxLatency > 0 {
		invMaxLatency = 1.0 / float64(maxLatency)
	}
	if maxErrors > 0 {
		invMaxErrors = 1.0 / float64(maxErrors)
	}
	if maxRequests > 0 {
		invMaxRequests = 1.0 / float64(maxRequests)
	}

	// 第二遍：计算权重
	for _, e := range g.edges {
		if !e.Active {
			e.Weight = 0
			continue
		}
		nb := float64(e.Bytes) * invMaxBytes
		nl := float64(e.Latency) * invMaxLatency
		ne := float64(e.Errors) * invMaxErrors
		nr := float64(e.RequestCount) * invMaxRequests
		e.Weight = 0.3*nb + 0.3*nl + 0.2*ne + 0.2*nr
	}

	g.version++
	g.timestamp = time.Now().UnixNano()
}

// Clear 清空所有节点和边。
func (g *Graph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes = make(map[NodeID]*Node, g.maxNodes)
	g.edges = make(map[EdgeKey]*Edge, g.maxEdges)
	g.nodeOrder = g.nodeOrder[:0]
	g.edgeOrder = g.edgeOrder[:0]
	g.version++
	g.timestamp = time.Now().UnixNano()
}

// ---------------------------------------------------------------------------
// 读操作 (需要读锁)
// ---------------------------------------------------------------------------

// GetNode 获取节点，返回节点指针和是否存在。
func (g *Graph) GetNode(id NodeID) (*Node, bool) {
	g.mu.RLock()
	n, ok := g.nodes[id]
	g.mu.RUnlock()
	return n, ok
}

// GetEdge 获取边，返回边指针和是否存在。
func (g *Graph) GetEdge(source, target NodeID) (*Edge, bool) {
	g.mu.RLock()
	e, ok := g.edges[EdgeKey{Source: source, Target: target}]
	g.mu.RUnlock()
	return e, ok
}

// Nodes 返回所有活跃节点的副本。
func (g *Graph) Nodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Node, 0, len(g.nodes))
	for _, id := range g.nodeOrder {
		if n, ok := g.nodes[id]; ok && n.Active {
			result = append(result, n)
		}
	}
	return result
}

// Edges 返回所有活跃边的副本。
func (g *Graph) Edges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Edge, 0, len(g.edges))
	for _, key := range g.edgeOrder {
		if e, ok := g.edges[key]; ok && e.Active {
			result = append(result, e)
		}
	}
	return result
}

// NodeCount 返回活跃节点数量。
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, n := range g.nodes {
		if n.Active {
			count++
		}
	}
	return count
}

// EdgeCount 返回活跃边数量。
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	count := 0
	for _, e := range g.edges {
		if e.Active {
			count++
		}
	}
	return count
}

// Version 返回当前图版本号。
func (g *Graph) Version() uint64 {
	g.mu.RLock()
	v := g.version
	g.mu.RUnlock()
	return v
}

// Timestamp 返回最后更新时间戳。
func (g *Graph) Timestamp() int64 {
	g.mu.RLock()
	ts := g.timestamp
	g.mu.RUnlock()
	return ts
}

// ---------------------------------------------------------------------------
// 快照与差异
// ---------------------------------------------------------------------------

// GraphSnapshot 图的不可变快照
type GraphSnapshot struct {
	Version   uint64
	Timestamp int64
	Nodes     []*Node
	Edges     []*Edge
	NodeCount int
	EdgeCount int
}

// Snapshot 创建当前图的不可变快照。
func (g *Graph) Snapshot() *GraphSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]*Node, 0, len(g.nodes))
	for _, id := range g.nodeOrder {
		if n, ok := g.nodes[id]; ok && n.Active {
			nodes = append(nodes, n)
		}
	}

	edges := make([]*Edge, 0, len(g.edges))
	for _, key := range g.edgeOrder {
		if e, ok := g.edges[key]; ok && e.Active {
			edges = append(edges, e)
		}
	}

	return &GraphSnapshot{
		Version:   g.version,
		Timestamp: g.timestamp,
		Nodes:     nodes,
		Edges:     edges,
		NodeCount: len(nodes),
		EdgeCount: len(edges),
	}
}

// TopologyDiff 两个图拓扑之间的差异
type TopologyDiff struct {
	AddedNodes   []*Node
	RemovedNodes []*Node
	AddedEdges   []*Edge
	RemovedEdges []*Edge
	ChangedEdges []*EdgeChange

	BaseVersion    uint64
	CompareVersion uint64
}

// EdgeChange 边的指标变化
type EdgeChange struct {
	Source, Target NodeID
	OldBytes, NewBytes   uint64
	OldLatency, NewLatency uint64
	OldErrors, NewErrors   uint64
	BytesDelta   int64
	LatencyDelta int64
	ErrorsDelta  int64
}

// Diff 计算当前图与另一张图的拓扑差异。
// 当前图为 base，other 为 compare。
func (g *Graph) Diff(other *Graph) *TopologyDiff {
	// 对两个图分别加读锁，按 NodeID 排序以避免死锁
	g.mu.RLock()
	other.mu.RLock()

	diff := &TopologyDiff{
		BaseVersion:    g.version,
		CompareVersion: other.version,
	}

	// ---- 节点差异 ----
	otherNodeSet := make(map[NodeID]struct{}, len(other.nodes))
	for _, n := range other.nodes {
		if !n.Active {
			continue
		}
		otherNodeSet[n.ID] = struct{}{}
		if _, exists := g.nodes[n.ID]; !exists || !g.nodes[n.ID].Active {
			diff.AddedNodes = append(diff.AddedNodes, n)
		}
	}
	for _, n := range g.nodes {
		if !n.Active {
			continue
		}
		if _, exists := otherNodeSet[n.ID]; !exists {
			diff.RemovedNodes = append(diff.RemovedNodes, n)
		}
	}

	// ---- 边差异 ----
	otherEdgeSet := make(map[EdgeKey]struct{}, len(other.edges))
	for key, e := range other.edges {
		if !e.Active {
			continue
		}
		otherEdgeSet[key] = struct{}{}
		if ge, exists := g.edges[key]; !exists || !ge.Active {
			diff.AddedEdges = append(diff.AddedEdges, e)
		}
	}
	for key, e := range g.edges {
		if !e.Active {
			continue
		}
		if _, exists := otherEdgeSet[key]; !exists {
			diff.RemovedEdges = append(diff.RemovedEdges, e)
		}
	}

	// ---- 边指标变化 ----
	for key, oe := range other.edges {
		if !oe.Active {
			continue
		}
		ge, exists := g.edges[key]
		if !exists || !ge.Active {
			continue
		}
		// 两边都存在，检查指标变化
		if ge.Bytes != oe.Bytes || ge.Latency != oe.Latency || ge.Errors != oe.Errors {
			ec := &EdgeChange{
				Source:     key.Source,
				Target:     key.Target,
				OldBytes:   ge.Bytes,
				NewBytes:   oe.Bytes,
				OldLatency: ge.Latency,
				NewLatency: oe.Latency,
				OldErrors:  ge.Errors,
				NewErrors:  oe.Errors,
			}
			ec.BytesDelta = int64(oe.Bytes) - int64(ge.Bytes)
			ec.LatencyDelta = int64(oe.Latency) - int64(ge.Latency)
			ec.ErrorsDelta = int64(oe.Errors) - int64(ge.Errors)
			diff.ChangedEdges = append(diff.ChangedEdges, ec)
		}
	}

	other.mu.RUnlock()
	g.mu.RUnlock()

	return diff
}

// ---------------------------------------------------------------------------
// 内部辅助方法
// ---------------------------------------------------------------------------

// rebuildOrders 根据当前 nodes/edges map 重建顺序切片（调用时需持有写锁）。
func (g *Graph) rebuildOrders() {
	g.nodeOrder = g.nodeOrder[:0]
	g.edgeOrder = g.edgeOrder[:0]

	for id := range g.nodes {
		g.nodeOrder = append(g.nodeOrder, id)
	}
	for key := range g.edges {
		g.edgeOrder = append(g.edgeOrder, key)
	}

	// 保持确定性：按插入顺序排序（NodeID / EdgeKey 的自然序）
	sort.Slice(g.nodeOrder, func(i, j int) bool {
		return g.nodeOrder[i] < g.nodeOrder[j]
	})
	sort.Slice(g.edgeOrder, func(i, j int) bool {
		si, sj := g.edgeOrder[i].Source, g.edgeOrder[j].Source
		if si != sj {
			return si < sj
		}
		return g.edgeOrder[i].Target < g.edgeOrder[j].Target
	})
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// safeSubInt64 安全的 uint64 差值转 int64，避免溢出
func safeSubInt64(a, b uint64) int64 {
	if a >= b {
		return int64(a - b)
	}
	return -int64(b - a)
}

// safeDivUint64 安全除法，避免除零
func safeDivUint64(a, b uint64) uint64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// maxUint64 返回两个 uint64 中的较大值
func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// GraphType 返回图类型
func (g *Graph) GraphType() string {
	g.mu.RLock()
	t := g.graphType
	g.mu.RUnlock()
	return t
}

// TenantID 返回租户 ID
func (g *Graph) TenantID() string {
	g.mu.RLock()
	t := g.tenantID
	g.mu.RUnlock()
	return t
}

// ActiveNodeCount 返回活跃节点总数（包含不活跃的）
func (g *Graph) TotalNodeCount() int {
	g.mu.RLock()
	c := len(g.nodes)
	g.mu.RUnlock()
	return c
}

// ActiveEdgeCount 返回所有边总数（包含不活跃的）
func (g *Graph) TotalEdgeCount() int {
	g.mu.RLock()
	c := len(g.edges)
	g.mu.RUnlock()
	return c
}

// Stats 返回图的基本统计信息
func (g *Graph) Stats() (version uint64, timestamp int64, totalNodes, activeNodes, totalEdges, activeEdges int) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	version = g.version
	timestamp = g.timestamp
	totalNodes = len(g.nodes)
	totalEdges = len(g.edges)
	for _, n := range g.nodes {
		if n.Active {
			activeNodes++
		}
	}
	for _, e := range g.edges {
		if e.Active {
			activeEdges++
		}
	}
	return
}

// String 返回图的摘要信息
func (g *Graph) String() string {
	v, ts, tn, an, te, ae := g.Stats()
	return fmt.Sprintf(
		"Graph{type=%s tenant=%s version=%d ts=%s nodes=%d/%d edges=%d/%d}",
		g.GraphType(), g.TenantID(), v,
		time.Unix(0, ts).Format(time.RFC3339Nano),
		an, tn, ae, te,
	)
}

// ---------------------------------------------------------------------------
// EdgeKey 排序支持
// ---------------------------------------------------------------------------

// EdgeKeys 对 EdgeKey 切片排序的辅助类型
type EdgeKeys []EdgeKey

func (ek EdgeKeys) Len() int      { return len(ek) }
func (ek EdgeKeys) Swap(i, j int) { ek[i], ek[j] = ek[j], ek[i] }
func (ek EdgeKeys) Less(i, j int) bool {
	if ek[i].Source != ek[j].Source {
		return ek[i].Source < ek[j].Source
	}
	return ek[i].Target < ek[j].Target
}

// ---------------------------------------------------------------------------
// NodeIDs 排序支持
// ---------------------------------------------------------------------------

// NodeIDs 对 NodeID 切片排序的辅助类型
type NodeIDs []NodeID

func (n NodeIDs) Len() int           { return len(n) }
func (n NodeIDs) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n NodeIDs) Less(i, j int) bool { return n[i] < n[j] }

// ---------------------------------------------------------------------------
// TopologyDiff 辅助方法
// ---------------------------------------------------------------------------

// Summary 返回差异的摘要字符串
func (d *TopologyDiff) String() string {
	return fmt.Sprintf(
		"TopologyDiff{baseV=%d compareV=%d +nodes=%d -nodes=%d +edges=%d -edges=%d ~edges=%d}",
		d.BaseVersion, d.CompareVersion,
		len(d.AddedNodes), len(d.RemovedNodes),
		len(d.AddedEdges), len(d.RemovedEdges),
		len(d.ChangedEdges),
	)
}

// IsEmpty 判断差异是否为空
func (d *TopologyDiff) IsEmpty() bool {
	return len(d.AddedNodes) == 0 &&
		len(d.RemovedNodes) == 0 &&
		len(d.AddedEdges) == 0 &&
		len(d.RemovedEdges) == 0 &&
		len(d.ChangedEdges) == 0
}

// ---------------------------------------------------------------------------
// GraphSnapshot 辅助方法
// ---------------------------------------------------------------------------

// String 返回快照的摘要字符串
func (s *GraphSnapshot) String() string {
	return fmt.Sprintf(
		"GraphSnapshot{version=%d ts=%s nodes=%d edges=%d}",
		s.Version,
		time.Unix(0, s.Timestamp).Format(time.RFC3339Nano),
		s.NodeCount, s.EdgeCount,
	)
}

// GetNodeByID 按 ID 查找节点（线性扫描，适合小规模快照查询）
func (s *GraphSnapshot) GetNodeByID(id NodeID) (*Node, bool) {
	for _, n := range s.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return nil, false
}

// GetEdgeByKey 按键查找边（线性扫描，适合小规模快照查询）
func (s *GraphSnapshot) GetEdgeByKey(source, target NodeID) (*Edge, bool) {
	for _, e := range s.Edges {
		if e.Source == source && e.Target == target {
			return e, true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Node 辅助方法
// ---------------------------------------------------------------------------

// UpdateAvgLatency 根据累加器重新计算平均延迟
func (n *Node) UpdateAvgLatency() {
	if n.LatencyCount > 0 {
		n.AvgLatencyNs = n.TotalLatencyNs / n.LatencyCount
	}
}

// TotalBytes 返回总字节数（入+出）
func (n *Node) TotalBytes() uint64 {
	return n.BytesIn + n.BytesOut
}

// ErrorRate 返回错误率 (0.0 ~ 1.0)，若无请求则返回 0
func (n *Node) ErrorRate() float64 {
	if n.RequestCount == 0 {
		return 0
	}
	return float64(n.Errors) / float64(n.RequestCount)
}

// ---------------------------------------------------------------------------
// Edge 辅助方法
// ---------------------------------------------------------------------------

// UpdateAvgLatency 根据累加器重新计算平均延迟
func (e *Edge) UpdateAvgLatency() {
	if e.LatencyCount > 0 {
		e.Latency = e.TotalLatency / e.LatencyCount
	}
}

// ErrorRate 返回错误率 (0.0 ~ 1.0)，若无请求则返回 0
func (e *Edge) ErrorRate() float64 {
	if e.RequestCount == 0 {
		return 0
	}
	return float64(e.Errors) / float64(e.RequestCount)
}

// AvgPacketSize 返回平均包大小，若无包则返回 0
func (e *Edge) AvgPacketSize() float64 {
	if e.Packets == 0 {
		return 0
	}
	return float64(e.Bytes) / float64(e.Packets)
}

// Key 返回边的 EdgeKey
func (e *Edge) Key() EdgeKey {
	return EdgeKey{Source: e.Source, Target: e.Target}
}

// ---------------------------------------------------------------------------
// EdgeChange 辅助方法
// ---------------------------------------------------------------------------

// HasChanges 判断是否有实际指标变化
func (ec *EdgeChange) HasChanges() bool {
	return ec.BytesDelta != 0 || ec.LatencyDelta != 0 || ec.ErrorsDelta != 0
}

// String 返回变化的摘要字符串
func (ec *EdgeChange) String() string {
	return fmt.Sprintf(
		"EdgeChange{%s bytes:%d->%d(\u0394%d) latency:%d->%d(\u0394%d) errors:%d->%d(\u0394%d)}",
		EdgeKey{Source: ec.Source, Target: ec.Target}.String(),
		ec.OldBytes, ec.NewBytes, ec.BytesDelta,
		ec.OldLatency, ec.NewLatency, ec.LatencyDelta,
		ec.OldErrors, ec.NewErrors, ec.ErrorsDelta,
	)
}

// ---------------------------------------------------------------------------
// 常量
// ---------------------------------------------------------------------------

const (
	// GraphTypeService 服务拓扑图
	GraphTypeService = "service"
	// GraphTypeProcess 进程拓扑图
	GraphTypeProcess = "process"
	// GraphTypePod Pod 拓扑图
	GraphTypePod = "pod"
	// GraphTypeNamespace 命名空间拓扑图
	GraphTypeNamespace = "namespace"

	// DefaultMaxNodes 默认最大节点数
	DefaultMaxNodes = 1_000_000
	// DefaultMaxEdges 默认最大边数
	DefaultMaxEdges = 10_000_000

	// WeightBytes 权重中字节的系数
	WeightBytes = 0.3
	// WeightLatency 权重中延迟的系数
	WeightLatency = 0.3
	// WeightErrors 权重中错误的系数
	WeightErrors = 0.2
	// WeightRequests 权重中请求数的系数
	WeightRequests = 0.2

	// PruneThresholdDefault 默认剪枝阈值
	PruneThresholdDefault = 0.01
)

// ---------------------------------------------------------------------------
// 数学辅助
// ---------------------------------------------------------------------------

// init 确保导入 math（用于未来扩展）
var _ = math.MaxFloat64
