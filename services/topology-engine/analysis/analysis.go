// Package analysis 拓扑依赖分析与影响分析
//
// 功能：
//   - 上游依赖分析：查找某个服务的所有上游调用者
//   - 下游影响分析：查找某个服务的所有下游被调用者
//   - 关键路径分析：找出影响端到端延迟的关键链路
//   - 故障传播分析：模拟故障在拓扑中的传播路径
//   - 依赖环检测：检测循环依赖
//
package analysis

import (
	"fmt"
	"math"
	"sort"
	"sync"

	graph "github.com/meinanzilinzhengying/cloudflow/services/topology-engine/graph"
)

// ============================================================================
// 一、依赖分析器
// ============================================================================

// DependencyAnalyzer 拓扑依赖分析器
type DependencyAnalyzer struct {
	mu sync.RWMutex
}

// NewDependencyAnalyzer 创建依赖分析器
func NewDependencyAnalyzer() *DependencyAnalyzer {
	return &DependencyAnalyzer{}
}

// ============================================================================
// 上游依赖分析（谁调用我）
// ============================================================================

// UpstreamDependency 上游依赖信息
type UpstreamDependency struct {
	NodeID          graph.NodeID
	NodeName        string
	NodeType        string
	DirectCallCount int       // 直接调用次数
	IndirectPaths   int       // 间接路径数
	TotalBytes      uint64    // 总流量
	AvgLatencyMs    float64   // 平均延迟 (ms)
	ErrorRate       float64   // 错误率
	Depth           int       // 依赖深度（距离）
	Path            []string  // 完整调用路径
}

// UpstreamResult 上游分析结果
type UpstreamResult struct {
	TargetNode   graph.NodeID
	DirectDeps   []*UpstreamDependency // 直接上游（距离 1）
	IndirectDeps []*UpstreamDependency // 间接上游（距离 > 1）
	TotalCount   int
	MaxDepth     int
}

// AnalyzeUpstream 分析某个节点的所有上游依赖
func (da *DependencyAnalyzer) AnalyzeUpstream(g *graph.Graph, targetID graph.NodeID, maxDepth int) *UpstreamResult {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	result := &UpstreamResult{TargetNode: targetID}
	visited := make(map[graph.NodeID]bool)
	queue := []struct {
		id    graph.NodeID
		depth int
		path  []string
	}{{targetID, 0, []string{string(targetID)}}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth > maxDepth {
			continue
		}
		if visited[curr.id] {
			continue
		}
		visited[curr.id] = true

		// 查找所有指向 curr.id 的边（上游）
		edges := g.Edges()
		for _, e := range edges {
			if e.Target == curr.id && e.Active {
				srcNode, exists := g.GetNode(e.Source)
				if !exists {
					continue
				}

				dep := &UpstreamDependency{
					NodeID:   e.Source,
					NodeName: srcNode.Name,
					NodeType: srcNode.Type,
					Depth:    curr.depth + 1,
					Path:     append([]string{}, curr.path...),
				}
				dep.Path = append(dep.Path, string(e.Source))
				dep.DirectCallCount = int(e.RequestCount)
				dep.TotalBytes = e.Bytes
				if e.LatencyCount > 0 {
					dep.AvgLatencyMs = float64(e.Latency) / 1e6
				}
				dep.ErrorRate = e.ErrorRate()

				if curr.depth == 0 {
					result.DirectDeps = append(result.DirectDeps, dep)
				} else {
					result.IndirectDeps = append(result.IndirectDeps, dep)
				}
				result.TotalCount++
				if dep.Depth > result.MaxDepth {
					result.MaxDepth = dep.Depth
				}

				// 继续向上游搜索
				if curr.depth+1 < maxDepth {
					queue = append(queue, struct {
						id    graph.NodeID
						depth int
						path  []string
					}{e.Source, curr.depth + 1, dep.Path})
				}
			}
		}
	}

	// 按深度排序
	sort.Slice(result.DirectDeps, func(i, j int) bool {
		return result.DirectDeps[i].TotalBytes > result.DirectDeps[j].TotalBytes
	})
	sort.Slice(result.IndirectDeps, func(i, j int) bool {
		if result.IndirectDeps[i].Depth != result.IndirectDeps[j].Depth {
			return result.IndirectDeps[i].Depth < result.IndirectDeps[j].Depth
		}
		return result.IndirectDeps[i].TotalBytes > result.IndirectDeps[j].TotalBytes
	})

	return result
}

// ============================================================================
// 下游影响分析（我调用谁）
// ============================================================================

// DownstreamImpact 下游影响信息
type DownstreamImpact struct {
	NodeID          graph.NodeID
	NodeName        string
	NodeType        string
	DirectCallCount int
	IndirectPaths   int
	TotalBytes      uint64
	AvgLatencyMs    float64
	ErrorRate       float64
	Depth           int
	Path            []string
	// 影响评分：综合考虑流量、延迟、错误率
	ImpactScore float64
}

// DownstreamResult 下游分析结果
type DownstreamResult struct {
	SourceNode   graph.NodeID
	DirectDeps   []*DownstreamImpact
	IndirectDeps []*DownstreamImpact
	TotalCount   int
	MaxDepth     int
	// 关键下游节点（高影响评分）
	CriticalDeps []*DownstreamImpact
}

// AnalyzeDownstream 分析某个节点的所有下游影响
func (da *DependencyAnalyzer) AnalyzeDownstream(g *graph.Graph, sourceID graph.NodeID, maxDepth int) *DownstreamResult {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	result := &DownstreamResult{SourceNode: sourceID}
	visited := make(map[graph.NodeID]bool)
	queue := []struct {
		id    graph.NodeID
		depth int
		path  []string
	}{{sourceID, 0, []string{string(sourceID)}}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth > maxDepth {
			continue
		}
		if visited[curr.id] {
			continue
		}
		visited[curr.id] = true

		edges := g.Edges()
		for _, e := range edges {
			if e.Source == curr.id && e.Active {
				tgtNode, exists := g.GetNode(e.Target)
				if !exists {
					continue
				}

				impact := &DownstreamImpact{
					NodeID:   e.Target,
					NodeName: tgtNode.Name,
					NodeType: tgtNode.Type,
					Depth:    curr.depth + 1,
					Path:     append([]string{}, curr.path...),
				}
				impact.Path = append(impact.Path, string(e.Target))
				impact.DirectCallCount = int(e.RequestCount)
				impact.TotalBytes = e.Bytes
				if e.LatencyCount > 0 {
					impact.AvgLatencyMs = float64(e.Latency) / 1e6
				}
				impact.ErrorRate = e.ErrorRate()
				// 影响评分 = 0.4*归一化流量 + 0.3*归一化延迟 + 0.3*错误率
				impact.ImpactScore = computeImpactScore(impact.TotalBytes, impact.AvgLatencyMs, impact.ErrorRate)

				if curr.depth == 0 {
					result.DirectDeps = append(result.DirectDeps, impact)
				} else {
					result.IndirectDeps = append(result.IndirectDeps, impact)
				}
				result.TotalCount++
				if impact.Depth > result.MaxDepth {
					result.MaxDepth = impact.Depth
				}

				if curr.depth+1 < maxDepth {
					queue = append(queue, struct {
						id    graph.NodeID
						depth int
						path  []string
					}{e.Target, curr.depth + 1, impact.Path})
				}
			}
		}
	}

	// 按影响评分排序，提取关键下游
	allDeps := append(result.DirectDeps, result.IndirectDeps...)
	sort.Slice(allDeps, func(i, j int) bool {
		return allDeps[i].ImpactScore > allDeps[j].ImpactScore
	})
	if len(allDeps) > 10 {
		result.CriticalDeps = allDeps[:10]
	} else {
		result.CriticalDeps = allDeps
	}

	// 直接下游按流量排序
	sort.Slice(result.DirectDeps, func(i, j int) bool {
		return result.DirectDeps[i].TotalBytes > result.DirectDeps[j].TotalBytes
	})

	return result
}

func computeImpactScore(bytes uint64, latencyMs, errorRate float64) float64 {
	// 归一化（假设阈值）
	var normBytes float64
	if bytes > 0 {
		normBytes = math.Min(1.0, float64(bytes)/1e9) // 1GB = 1.0
	}
	normLatency := math.Min(1.0, latencyMs/1000.0) // 1000ms = 1.0
	normError := math.Min(1.0, errorRate*10)        // 10% error = 1.0
	return 0.4*normBytes + 0.3*normLatency + 0.3*normError
}

// ============================================================================
// 二、关键路径分析
// ============================================================================

// CriticalPathAnalyzer 关键路径分析器
type CriticalPathAnalyzer struct{}

// NewCriticalPathAnalyzer 创建关键路径分析器
func NewCriticalPathAnalyzer() *CriticalPathAnalyzer {
	return &CriticalPathAnalyzer{}
}

// PathNode 路径上的节点
type PathNode struct {
	NodeID       graph.NodeID
	NodeName     string
	LatencyMs    float64
	ErrorRate    float64
	RequestCount uint64
	// 在关键路径中的贡献度
	LatencyContribution float64
}

// CriticalPath 关键路径
type CriticalPath struct {
	Nodes        []*PathNode
	TotalLatency float64
	TotalHops    int
	// 瓶颈节点（贡献度最高）
	Bottleneck *PathNode
}

// FindCriticalPath 查找从 source 到 target 的关键路径（延迟最大路径）
func (cpa *CriticalPathAnalyzer) FindCriticalPath(g *graph.Graph, sourceID, targetID graph.NodeID) *CriticalPath {
	// 使用 Dijkstra 变体：找延迟最大路径
	type pathState struct {
		nodeID  graph.NodeID
		latency float64
		path    []graph.NodeID
	}

	bestPaths := make(map[graph.NodeID]*pathState)
	bestPaths[sourceID] = &pathState{nodeID: sourceID, latency: 0, path: []graph.NodeID{sourceID}}

	visited := make(map[graph.NodeID]bool)
	queue := []*pathState{bestPaths[sourceID]}

	for len(queue) > 0 {
		// 找当前延迟最大的路径
		maxIdx := 0
		for i := 1; i < len(queue); i++ {
			if queue[i].latency > queue[maxIdx].latency {
				maxIdx = i
			}
		}
		curr := queue[maxIdx]
		queue = append(queue[:maxIdx], queue[maxIdx+1:]...)

		if visited[curr.nodeID] {
			continue
		}
		visited[curr.nodeID] = true

		if curr.nodeID == targetID {
			break // 找到目标
		}

		// 遍历所有出边
		edges := g.Edges()
		for _, e := range edges {
			if e.Source == curr.nodeID && e.Active {
				edgeLatency := float64(e.Latency) / 1e6 // ns -> ms
				newLatency := curr.latency + edgeLatency

				if best, exists := bestPaths[e.Target]; !exists || newLatency > best.latency {
					newPath := append([]graph.NodeID{}, curr.path...)
					newPath = append(newPath, e.Target)
					bestPaths[e.Target] = &pathState{
						nodeID:  e.Target,
						latency: newLatency,
						path:    newPath,
					}
					queue = append(queue, bestPaths[e.Target])
				}
			}
		}
	}

	// 构建结果
	if targetState, exists := bestPaths[targetID]; exists {
		path := &CriticalPath{
			TotalLatency: targetState.latency,
			TotalHops:    len(targetState.path) - 1,
		}

		var maxContribution float64
		for i, nodeID := range targetState.path {
			node, ok := g.GetNode(nodeID)
			if !ok {
				continue
			}
			pn := &PathNode{
				NodeID:    nodeID,
				NodeName:  node.Name,
				LatencyMs: float64(node.AvgLatencyNs) / 1e6,
				ErrorRate: node.ErrorRate(),
			}
			if i < len(targetState.path)-1 {
				// 查找到下一跳的边延迟
				nextID := targetState.path[i+1]
				if e, ok := g.GetEdge(nodeID, nextID); ok {
					edgeLatency := float64(e.Latency) / 1e6
					pn.LatencyMs = edgeLatency
					if path.TotalLatency > 0 {
						pn.LatencyContribution = edgeLatency / path.TotalLatency
					}
				}
			}
			path.Nodes = append(path.Nodes, pn)
			if pn.LatencyContribution > maxContribution {
				maxContribution = pn.LatencyContribution
				path.Bottleneck = pn
			}
		}
		return path
	}

	return nil
}

// FindAllPaths 查找从 source 到 target 的所有路径（限制最大路径数）
func (cpa *CriticalPathAnalyzer) FindAllPaths(g *graph.Graph, sourceID, targetID graph.NodeID, maxPaths int) []*CriticalPath {
	if maxPaths <= 0 {
		maxPaths = 10
	}

	var paths []*CriticalPath
	visited := make(map[graph.NodeID]bool)
	var currentPath []graph.NodeID

	var dfs func(nodeID graph.NodeID)
	dfs = func(nodeID graph.NodeID) {
		if len(paths) >= maxPaths {
			return
		}
		if visited[nodeID] {
			return // 避免环
		}
		if nodeID == targetID {
			// 找到一条路径
			path := buildPathFromNodes(g, currentPath)
			if path != nil {
				paths = append(paths, path)
			}
			return
		}

		visited[nodeID] = true
		currentPath = append(currentPath, nodeID)

		edges := g.Edges()
		for _, e := range edges {
			if e.Source == nodeID && e.Active {
				dfs(e.Target)
			}
		}

		currentPath = currentPath[:len(currentPath)-1]
		visited[nodeID] = false
	}

	dfs(sourceID)
	return paths
}

func buildPathFromNodes(g *graph.Graph, nodeIDs []graph.NodeID) *CriticalPath {
	if len(nodeIDs) < 2 {
		return nil
	}

	path := &CriticalPath{}
	var totalLatency float64
	var maxContribution float64

	for i, nodeID := range nodeIDs {
		node, ok := g.GetNode(nodeID)
		if !ok {
			continue
		}
		pn := &PathNode{
			NodeID:    nodeID,
			NodeName:  node.Name,
			ErrorRate: node.ErrorRate(),
		}
		if i < len(nodeIDs)-1 {
			nextID := nodeIDs[i+1]
			if e, ok := g.GetEdge(nodeID, nextID); ok {
				edgeLatency := float64(e.Latency) / 1e6
				pn.LatencyMs = edgeLatency
				totalLatency += edgeLatency
			}
		}
		path.Nodes = append(path.Nodes, pn)
	}

	path.TotalLatency = totalLatency
	path.TotalHops = len(nodeIDs) - 1

	// 计算贡献度
	for _, pn := range path.Nodes {
		if path.TotalLatency > 0 {
			pn.LatencyContribution = pn.LatencyMs / path.TotalLatency
			if pn.LatencyContribution > maxContribution {
				maxContribution = pn.LatencyContribution
				path.Bottleneck = pn
			}
		}
	}

	return path
}

// ============================================================================
// 三、故障传播分析
// ============================================================================

// FailurePropagationAnalyzer 故障传播分析器
type FailurePropagationAnalyzer struct{}

// NewFailurePropagationAnalyzer 创建故障传播分析器
func NewFailurePropagationAnalyzer() *FailurePropagationAnalyzer {
	return &FailurePropagationAnalyzer{}
}

// FailureImpact 故障影响范围
type FailureImpact struct {
	NodeID       graph.NodeID
	NodeName     string
	NodeType     string
	ImpactLevel  string // critical / high / medium / low
	AffectedPaths int   // 受影响的路径数
	TotalRequests uint64 // 总受影响请求数
	Reason       string
}

// PropagationResult 故障传播结果
type PropagationResult struct {
	SourceNode      graph.NodeID
	AffectedNodes   []*FailureImpact
	TotalAffected   int
	CriticalCount   int
	HighCount       int
	PropagationTree *PropagationNode
}

// PropagationNode 传播树节点
type PropagationNode struct {
	NodeID       graph.NodeID
	NodeName     string
	ImpactLevel  string
	Children     []*PropagationNode
	Depth        int
}

// AnalyzeFailurePropagation 分析某个节点故障的传播影响
func (fpa *FailurePropagationAnalyzer) AnalyzeFailurePropagation(g *graph.Graph, failedNodeID graph.NodeID, maxDepth int) *PropagationResult {
	if maxDepth <= 0 {
		maxDepth = 5
	}

	result := &PropagationResult{SourceNode: failedNodeID}
	visited := make(map[graph.NodeID]bool)
	queue := []*PropagationNode{{NodeID: failedNodeID, Depth: 0, ImpactLevel: "critical"}}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.Depth > maxDepth {
			continue
		}
		if visited[curr.NodeID] {
			continue
		}
		visited[curr.NodeID] = true

		node, ok := g.GetNode(curr.NodeID)
		if !ok {
			continue
		}

		impact := &FailureImpact{
			NodeID:   curr.NodeID,
			NodeName: node.Name,
			NodeType: node.Type,
			ImpactLevel: curr.ImpactLevel,
		}

		// 计算影响级别
		if curr.Depth == 0 {
			impact.ImpactLevel = "critical"
			impact.Reason = "故障源节点"
			result.CriticalCount++
		} else {
			impact.ImpactLevel = computeImpactLevel(curr.Depth, node.RequestCount)
			impact.Reason = fmt.Sprintf("被 %s 依赖（深度 %d）", failedNodeID, curr.Depth)
			impact.AffectedPaths = curr.Depth
			impact.TotalRequests = node.RequestCount
			if impact.ImpactLevel == "critical" {
				result.CriticalCount++
			} else if impact.ImpactLevel == "high" {
				result.HighCount++
			}
		}

		result.AffectedNodes = append(result.AffectedNodes, impact)
		result.TotalAffected++

		// 继续向下游传播
		edges := g.Edges()
		for _, e := range edges {
			if e.Source == curr.NodeID && e.Active {
				childLevel := propagateLevel(curr.ImpactLevel)
				child := &PropagationNode{
					NodeID:      e.Target,
					ImpactLevel: childLevel,
					Depth:       curr.Depth + 1,
				}
				curr.Children = append(curr.Children, child)
				queue = append(queue, child)
			}
		}
	}

	// 构建传播树
	result.PropagationTree = buildPropagationTree(result.AffectedNodes, failedNodeID)

	return result
}

func computeImpactLevel(depth int, requestCount uint64) string {
	score := float64(depth) * 0.3
	if requestCount > 10000 {
		score += 0.4
	} else if requestCount > 1000 {
		score += 0.2
	} else if requestCount > 100 {
		score += 0.1
	}

	switch {
	case score >= 1.5:
		return "critical"
	case score >= 1.0:
		return "high"
	case score >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

func propagateLevel(parentLevel string) string {
	switch parentLevel {
	case "critical":
		return "high"
	case "high":
		return "medium"
	case "medium":
		return "low"
	default:
		return "low"
	}
}

func buildPropagationTree(impacts []*FailureImpact, rootID graph.NodeID) *PropagationNode {
	nodeMap := make(map[graph.NodeID]*PropagationNode)
	for _, impact := range impacts {
		nodeMap[impact.NodeID] = &PropagationNode{
			NodeID:      impact.NodeID,
			NodeName:    impact.NodeName,
			ImpactLevel: impact.ImpactLevel,
		}
	}

	root := nodeMap[rootID]
	if root == nil {
		return nil
	}

	// 构建父子关系（简化：基于层级深度）
	return root
}

// ============================================================================
// 四、依赖环检测
// ============================================================================

// CycleDetector 循环依赖检测器
type CycleDetector struct{}

// NewCycleDetector 创建循环依赖检测器
func NewCycleDetector() *CycleDetector {
	return &CycleDetector{}
}

// Cycle 循环依赖
type Cycle struct {
	Nodes []graph.NodeID
	Length int
}

// DetectCycles 检测拓扑图中的所有循环依赖
func (cd *CycleDetector) DetectCycles(g *graph.Graph) []*Cycle {
	var cycles []*Cycle
	nodes := g.Nodes()
	if len(nodes) == 0 {
		return cycles
	}

	// 使用 Johnson's algorithm 简化版：DFS 检测环
	visited := make(map[graph.NodeID]bool)
	recStack := make(map[graph.NodeID]bool)
	var path []graph.NodeID

	var dfs func(nodeID graph.NodeID)
	dfs = func(nodeID graph.NodeID) {
		visited[nodeID] = true
		recStack[nodeID] = true
		path = append(path, nodeID)

		edges := g.Edges()
		for _, e := range edges {
			if e.Source == nodeID && e.Active {
				if !visited[e.Target] {
					dfs(e.Target)
				} else if recStack[e.Target] {
					// 发现环
					cycle := extractCycle(path, e.Target)
					if cycle != nil {
						cycles = append(cycles, cycle)
					}
				}
			}
		}

		path = path[:len(path)-1]
		recStack[nodeID] = false
	}

	for _, n := range nodes {
		if !visited[n.ID] {
			dfs(n.ID)
		}
	}

	return cycles
}

func extractCycle(path []graph.NodeID, target graph.NodeID) *Cycle {
	// 找到 target 在 path 中的位置
	idx := -1
	for i, id := range path {
		if id == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	cycleNodes := append([]graph.NodeID{}, path[idx:]...)
	return &Cycle{
		Nodes:  cycleNodes,
		Length: len(cycleNodes),
	}
}

// String 返回循环依赖的字符串表示
func (c *Cycle) String() string {
	if len(c.Nodes) == 0 {
		return "empty cycle"
	}
	s := ""
	for i, n := range c.Nodes {
		if i > 0 {
			s += " → "
		}
		s += string(n)
	}
	s += " → " + string(c.Nodes[0])
	return s
}

// ============================================================================
// 五、聚合分析结果
// ============================================================================

// TopologyHealthReport 拓扑健康度报告
type TopologyHealthReport struct {
	TotalNodes    int
	TotalEdges    int
	HealthyNodes  int
	WarningNodes  int
	CriticalNodes int
	HealthyEdges  int
	WarningEdges  int
	CriticalEdges int
	Cycles        int
	AvgLatencyMs  float64
	AvgErrorRate  float64
}

// GenerateHealthReport 生成拓扑健康度报告
func (da *DependencyAnalyzer) GenerateHealthReport(g *graph.Graph) *TopologyHealthReport {
	report := &TopologyHealthReport{}
	nodes := g.Nodes()
	edges := g.Edges()

	report.TotalNodes = len(nodes)
	report.TotalEdges = len(edges)

	var totalLatency float64
	var totalErrorRate float64

	for _, n := range nodes {
		errRate := n.ErrorRate()
		latencyMs := float64(n.AvgLatencyNs) / 1e6

		totalErrorRate += errRate
		if n.LatencyCount > 0 {
			totalLatency += latencyMs
		}

		switch {
		case errRate > 0.1 || latencyMs > 1000:
			report.CriticalNodes++
		case errRate > 0.01 || latencyMs > 500:
			report.WarningNodes++
		default:
			report.HealthyNodes++
		}
	}

	for _, e := range edges {
		errRate := e.ErrorRate()
		latencyMs := float64(e.Latency) / 1e6

		switch {
		case errRate > 0.1 || latencyMs > 1000:
			report.CriticalEdges++
		case errRate > 0.01 || latencyMs > 500:
			report.WarningEdges++
		default:
			report.HealthyEdges++
		}
	}

	if report.TotalNodes > 0 {
		report.AvgErrorRate = totalErrorRate / float64(report.TotalNodes)
	}
	if report.TotalEdges > 0 {
		report.AvgLatencyMs = totalLatency / float64(report.TotalEdges)
	}

	cd := NewCycleDetector()
	report.Cycles = len(cd.DetectCycles(g))

	return report
}
