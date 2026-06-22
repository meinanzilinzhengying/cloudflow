// Package drilldown 拓扑层级钻取与下钻
//
// 功能：
//   - 从粗粒度到细粒度的拓扑层级钻取
//   - namespace → service → pod → process 层级下钻
//   - 聚合视图到具体实例的钻取
//   - 钻取路径追踪和面包屑导航
//
package drilldown

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	graph "github.com/meinanzilinzhengying/cloudflow/services/topology-engine/graph"
)

// ============================================================================
// 一、层级定义
// ============================================================================

// HierarchyLevel 拓扑层级类型
type HierarchyLevel string

const (
	LevelNamespace HierarchyLevel = "namespace" // 最粗：命名空间
	LevelService   HierarchyLevel = "service"   // 服务
	LevelPod       HierarchyLevel = "pod"       // Pod
	LevelProcess   HierarchyLevel = "process"   // 最细：进程
	LevelInstance  HierarchyLevel = "instance"  // 实例（IP:Port）
)

// HierarchyOrder 层级顺序（从粗到细）
var HierarchyOrder = []HierarchyLevel{
	LevelNamespace,
	LevelService,
	LevelPod,
	LevelProcess,
	LevelInstance,
}

// LevelDepth 返回层级深度（0 = 最粗）
func LevelDepth(level HierarchyLevel) int {
	for i, l := range HierarchyOrder {
		if l == level {
			return i
		}
	}
	return -1
}

// NextFinerLevel 返回下一级更细的层级
func NextFinerLevel(level HierarchyLevel) HierarchyLevel {
	depth := LevelDepth(level)
	if depth >= 0 && depth < len(HierarchyOrder)-1 {
		return HierarchyOrder[depth+1]
	}
	return ""
}

// NextCoarserLevel 返回上一级更粗的层级
func NextCoarserLevel(level HierarchyLevel) HierarchyLevel {
	depth := LevelDepth(level)
	if depth > 0 && depth < len(HierarchyOrder) {
		return HierarchyOrder[depth-1]
	}
	return ""
}

// ============================================================================
// 二、钻取分析器
// ============================================================================

// DrillDownAnalyzer 拓扑钻取分析器
type DrillDownAnalyzer struct {
	mu sync.RWMutex
}

// NewDrillDownAnalyzer 创建钻取分析器
func NewDrillDownAnalyzer() *DrillDownAnalyzer {
	return &DrillDownAnalyzer{}
}

// DrillDownContext 钻取上下文
type DrillDownContext struct {
	CurrentLevel   HierarchyLevel
	TargetNodeID   graph.NodeID
	TargetNodeName string
	// 面包屑导航路径
	Breadcrumb []DrillDownStep
	// 可用的子层级
	AvailableSubLevels []HierarchyLevel
}

// DrillDownStep 钻取步骤
type DrillDownStep struct {
	Level    HierarchyLevel
	NodeID   graph.NodeID
	NodeName string
}

// DrillDownResult 钻取结果
type DrillDownResult struct {
	Context   *DrillDownContext
	SubNodes  []*SubNodeInfo
	SubEdges  []*SubEdgeInfo
	Summary   *DrillDownSummary
}

// SubNodeInfo 子节点信息
type SubNodeInfo struct {
	NodeID       graph.NodeID
	NodeName     string
	NodeType     string
	ParentID     graph.NodeID
	TrafficIn    uint64
	TrafficOut   uint64
	ErrorRate    float64
	LatencyMs    float64
	RequestCount uint64
	// 健康状态
	HealthStatus string // healthy / warning / critical
	// 是否可以继续下钻
	Drillable bool
}

// SubEdgeInfo 子边信息
type SubEdgeInfo struct {
	Source       graph.NodeID
	Target       graph.NodeID
	SourceName   string
	TargetName   string
	Bytes        uint64
	LatencyMs    float64
	ErrorRate    float64
	RequestCount uint64
	HealthStatus string
}

// DrillDownSummary 钻取摘要
type DrillDownSummary struct {
	TotalSubNodes    int
	TotalSubEdges    int
	HealthyCount     int
	WarningCount     int
	CriticalCount    int
	TotalTrafficIn   uint64
	TotalTrafficOut  uint64
	AvgLatencyMs     float64
	AvgErrorRate     float64
	MaxLatencyMs     float64
	MaxErrorRate     float64
}

// DrillDown 从当前节点下钻到下一层级
//
// 参数:
//   - g: 当前层级的拓扑图
//   - parentNodeID: 要下钻的父节点 ID
//   - parentLevel: 父节点的层级
//   - subLevel: 目标子层级（必须比 parentLevel 更细）
//
// 返回子层级的拓扑图和节点列表
func (da *DrillDownAnalyzer) DrillDown(
	g *graph.Graph,
	parentNodeID graph.NodeID,
	parentLevel HierarchyLevel,
	subLevel HierarchyLevel,
) *DrillDownResult {
	result := &DrillDownResult{
		Context: &DrillDownContext{
			CurrentLevel:   subLevel,
			TargetNodeID:   parentNodeID,
			TargetNodeName: "",
			Breadcrumb:     []DrillDownStep{},
		},
		SubNodes: []*SubNodeInfo{},
		SubEdges: []*SubEdgeInfo{},
		Summary:  &DrillDownSummary{},
	}

	// 获取父节点信息
	parentNode, exists := g.GetNode(parentNodeID)
	if exists {
		result.Context.TargetNodeName = parentNode.Name
		result.Context.Breadcrumb = append(result.Context.Breadcrumb, DrillDownStep{
			Level:    parentLevel,
			NodeID:   parentNodeID,
			NodeName: parentNode.Name,
		})
	}

	// 确定可用的子层级
	parentDepth := LevelDepth(parentLevel)
	subDepth := LevelDepth(subLevel)
	if subDepth > parentDepth {
		for i := parentDepth + 1; i <= subDepth && i < len(HierarchyOrder); i++ {
			result.Context.AvailableSubLevels = append(result.Context.AvailableSubLevels, HierarchyOrder[i])
		}
	}

	// 收集子节点：找出所有 parentNodeID 的下游节点，过滤符合 subLevel 类型的节点
	subNodes := da.findSubNodes(g, parentNodeID, subLevel)
	result.SubNodes = subNodes

	// 收集子边：子节点之间的边
	subEdges := da.findSubEdges(g, subNodes)
	result.SubEdges = subEdges

	// 计算摘要
	result.Summary = da.computeSummary(subNodes, subEdges)

	return result
}

// findSubNodes 查找父节点下的所有子节点
func (da *DrillDownAnalyzer) findSubNodes(g *graph.Graph, parentID graph.NodeID, subLevel HierarchyLevel) []*SubNodeInfo {
	var subNodes []*SubNodeInfo
	seen := make(map[graph.NodeID]bool)

	// 遍历父节点的所有出边
	edges := g.Edges()
	for _, e := range edges {
		if e.Source == parentID && e.Active {
			targetNode, exists := g.GetNode(e.Target)
			if !exists || seen[targetNode.ID] {
				continue
			}

			// 检查节点类型是否匹配子层级
			if !matchesLevel(targetNode.Type, subLevel) {
				continue
			}

			seen[targetNode.ID] = true
			info := &SubNodeInfo{
				NodeID:       targetNode.ID,
				NodeName:     targetNode.Name,
				NodeType:     targetNode.Type,
				ParentID:     parentID,
				TrafficIn:    targetNode.BytesIn,
				TrafficOut:   targetNode.BytesOut,
				ErrorRate:    targetNode.ErrorRate(),
				RequestCount: targetNode.RequestCount,
				Drillable:    canDrillDown(subLevel),
			}
			if targetNode.LatencyCount > 0 {
				info.LatencyMs = float64(targetNode.AvgLatencyNs) / 1e6
			}
			info.HealthStatus = classifyHealth(info.ErrorRate, info.LatencyMs)

			subNodes = append(subNodes, info)
		}
	}

	// 也检查入边（上游调用者）
	for _, e := range edges {
		if e.Target == parentID && e.Active {
			sourceNode, exists := g.GetNode(e.Source)
			if !exists || seen[sourceNode.ID] {
				continue
			}
			if !matchesLevel(sourceNode.Type, subLevel) {
				continue
			}

			seen[sourceNode.ID] = true
			info := &SubNodeInfo{
				NodeID:       sourceNode.ID,
				NodeName:     sourceNode.Name,
				NodeType:     sourceNode.Type,
				ParentID:     parentID,
				TrafficIn:    sourceNode.BytesIn,
				TrafficOut:   sourceNode.BytesOut,
				ErrorRate:    sourceNode.ErrorRate(),
				RequestCount: sourceNode.RequestCount,
				Drillable:    canDrillDown(subLevel),
			}
			if sourceNode.LatencyCount > 0 {
				info.LatencyMs = float64(sourceNode.AvgLatencyNs) / 1e6
			}
			info.HealthStatus = classifyHealth(info.ErrorRate, info.LatencyMs)

			subNodes = append(subNodes, info)
		}
	}

	// 按流量排序
	sort.Slice(subNodes, func(i, j int) bool {
		return subNodes[i].TrafficOut > subNodes[j].TrafficOut
	})

	return subNodes
}

// findSubEdges 查找子节点之间的边
func (da *DrillDownAnalyzer) findSubEdges(g *graph.Graph, subNodes []*SubNodeInfo) []*SubEdgeInfo {
	nodeSet := make(map[graph.NodeID]bool)
	for _, n := range subNodes {
		nodeSet[n.NodeID] = true
	}

	var subEdges []*SubEdgeInfo
	edges := g.Edges()
	for _, e := range edges {
		if !e.Active {
			continue
		}
		if nodeSet[e.Source] && nodeSet[e.Target] {
			srcNode, srcOK := g.GetNode(e.Source)
			tgtNode, tgtOK := g.GetNode(e.Target)
			if !srcOK || !tgtOK {
				continue
			}

			info := &SubEdgeInfo{
				Source:     e.Source,
				Target:     e.Target,
				SourceName: srcNode.Name,
				TargetName: tgtNode.Name,
				Bytes:      e.Bytes,
				RequestCount: e.RequestCount,
			}
			if e.LatencyCount > 0 {
				info.LatencyMs = float64(e.Latency) / 1e6
			}
			info.ErrorRate = e.ErrorRate()
			info.HealthStatus = classifyHealth(info.ErrorRate, info.LatencyMs)

			subEdges = append(subEdges, info)
		}
	}

	// 按流量排序
	sort.Slice(subEdges, func(i, j int) bool {
		return subEdges[i].Bytes > subEdges[j].Bytes
	})

	return subEdges
}

func (da *DrillDownAnalyzer) computeSummary(nodes []*SubNodeInfo, edges []*SubEdgeInfo) *DrillDownSummary {
	summary := &DrillDownSummary{
		TotalSubNodes: len(nodes),
		TotalSubEdges: len(edges),
	}

	var totalLatency float64
	var totalErrorRate float64
	var latencyCount int

	for _, n := range nodes {
		summary.TotalTrafficIn += n.TrafficIn
		summary.TotalTrafficOut += n.TrafficOut

		switch n.HealthStatus {
		case "healthy":
			summary.HealthyCount++
		case "warning":
			summary.WarningCount++
		case "critical":
			summary.CriticalCount++
		}

		if n.LatencyMs > 0 {
			totalLatency += n.LatencyMs
			latencyCount++
			if n.LatencyMs > summary.MaxLatencyMs {
				summary.MaxLatencyMs = n.LatencyMs
			}
		}
		totalErrorRate += n.ErrorRate
		if n.ErrorRate > summary.MaxErrorRate {
			summary.MaxErrorRate = n.ErrorRate
		}
	}

	if latencyCount > 0 {
		summary.AvgLatencyMs = totalLatency / float64(latencyCount)
	}
	if len(nodes) > 0 {
		summary.AvgErrorRate = totalErrorRate / float64(len(nodes))
	}

	return summary
}

// ============================================================================
// 三、辅助函数
// ============================================================================

// matchesLevel 检查节点类型是否匹配层级
func matchesLevel(nodeType string, level HierarchyLevel) bool {
	nodeType = strings.ToLower(nodeType)
	levelStr := strings.ToLower(string(level))
	return nodeType == levelStr ||
		(nodeType == "service" && levelStr == "service") ||
		(nodeType == "pod" && levelStr == "pod") ||
		(nodeType == "process" && levelStr == "process") ||
		(nodeType == "namespace" && levelStr == "namespace") ||
		(nodeType == "instance" && levelStr == "instance")
}

// canDrillDown 判断某个层级是否还能继续下钻
func canDrillDown(level HierarchyLevel) bool {
	depth := LevelDepth(level)
	return depth >= 0 && depth < len(HierarchyOrder)-1
}

// classifyHealth 根据错误率和延迟分类健康状态
func classifyHealth(errorRate float64, latencyMs float64) string {
	switch {
	case errorRate > 0.1 || latencyMs > 1000:
		return "critical"
	case errorRate > 0.01 || latencyMs > 500:
		return "warning"
	default:
		return "healthy"
	}
}

// ============================================================================
// 四、聚合视图构建
// ============================================================================

// AggregateView 聚合视图
type AggregateView struct {
	Level       HierarchyLevel
	Nodes       []*AggregateNode
	Edges       []*AggregateEdge
	NodeCount   int
	EdgeCount   int
}

// AggregateNode 聚合节点
type AggregateNode struct {
	ID           graph.NodeID
	Name         string
	Type         string
	ChildCount   int
	TrafficIn    uint64
	TrafficOut   uint64
	ErrorRate    float64
	LatencyMs    float64
	RequestCount uint64
	HealthStatus string
	// 包含的子节点 ID
	Children []graph.NodeID
}

// AggregateEdge 聚合边
type AggregateEdge struct {
	Source       graph.NodeID
	Target       graph.NodeID
	SourceName   string
	TargetName   string
	Bytes        uint64
	ErrorRate    float64
	LatencyMs    float64
	RequestCount uint64
	EdgeCount    int // 聚合的原始边数
}

// BuildAggregateView 构建指定层级的聚合视图
//
// 将细粒度图聚合到指定层级，例如：
//   - 将 process 级别的图聚合到 service 级别
//   - 将 pod 级别的图聚合到 namespace 级别
func (da *DrillDownAnalyzer) BuildAggregateView(g *graph.Graph, targetLevel HierarchyLevel) *AggregateView {
	view := &AggregateView{
		Level: targetLevel,
		Nodes: []*AggregateNode{},
		Edges: []*AggregateEdge{},
	}

	// 按 targetLevel 分组节点
	groupMap := make(map[string]*AggregateNode)

	nodes := g.Nodes()
	for _, n := range nodes {
		groupKey := extractGroupKey(n, targetLevel)
		if groupKey == "" {
			continue
		}

		agg, exists := groupMap[groupKey]
		if !exists {
			agg = &AggregateNode{
				ID:       graph.NodeID(groupKey),
				Name:     groupKey,
				Type:     string(targetLevel),
				Children: []graph.NodeID{},
			}
			groupMap[groupKey] = agg
		}

		agg.ChildCount++
		agg.Children = append(agg.Children, n.ID)
		agg.TrafficIn += n.BytesIn
		agg.TrafficOut += n.BytesOut
		agg.RequestCount += n.RequestCount
		if n.LatencyCount > 0 {
			agg.LatencyMs += float64(n.AvgLatencyNs) / 1e6
		}
		agg.ErrorRate += n.ErrorRate()
	}

	// 计算平均错误率
	for _, agg := range groupMap {
		if agg.ChildCount > 0 {
			agg.ErrorRate /= float64(agg.ChildCount)
			agg.LatencyMs /= float64(agg.ChildCount)
		}
		agg.HealthStatus = classifyHealth(agg.ErrorRate, agg.LatencyMs)
		view.Nodes = append(view.Nodes, agg)
	}

	// 按流量排序
	sort.Slice(view.Nodes, func(i, j int) bool {
		return view.Nodes[i].TrafficOut > view.Nodes[j].TrafficOut
	})

	view.NodeCount = len(view.Nodes)

	// 聚合边
	edgeGroupMap := make(map[string]*AggregateEdge)
	edges := g.Edges()
	for _, e := range edges {
		if !e.Active {
			continue
		}
		srcNode, srcOK := g.GetNode(e.Source)
		tgtNode, tgtOK := g.GetNode(e.Target)
		if !srcOK || !tgtOK {
			continue
		}

		srcKey := extractGroupKey(srcNode, targetLevel)
		tgtKey := extractGroupKey(tgtNode, targetLevel)
		if srcKey == "" || tgtKey == "" || srcKey == tgtKey {
			continue
		}

		edgeKey := srcKey + "→" + tgtKey
		aggEdge, exists := edgeGroupMap[edgeKey]
		if !exists {
			aggEdge = &AggregateEdge{
				Source:     graph.NodeID(srcKey),
				Target:     graph.NodeID(tgtKey),
				SourceName: srcKey,
				TargetName: tgtKey,
			}
			edgeGroupMap[edgeKey] = aggEdge
		}
		aggEdge.Bytes += e.Bytes
		aggEdge.RequestCount += e.RequestCount
		aggEdge.EdgeCount++
		aggEdge.ErrorRate += e.ErrorRate()
		if e.LatencyCount > 0 {
			aggEdge.LatencyMs += float64(e.Latency) / 1e6
		}
	}

	for _, aggEdge := range edgeGroupMap {
		if aggEdge.EdgeCount > 0 {
			aggEdge.ErrorRate /= float64(aggEdge.EdgeCount)
			aggEdge.LatencyMs /= float64(aggEdge.EdgeCount)
		}
		view.Edges = append(view.Edges, aggEdge)
	}

	// 按流量排序
	sort.Slice(view.Edges, func(i, j int) bool {
		return view.Edges[i].Bytes > view.Edges[j].Bytes
	})

	view.EdgeCount = len(view.Edges)
	return view
}

// extractGroupKey 从节点提取聚合分组键
func extractGroupKey(n *graph.Node, level HierarchyLevel) string {
	switch level {
	case LevelNamespace:
		return n.Namespace
	case LevelService:
		// 从节点名解析服务名，格式: "namespace/service-name"
		parts := strings.Split(n.Name, "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return n.Name
	case LevelPod:
		// 从节点名解析 Pod 名，格式: "namespace/pod-name"
		parts := strings.Split(n.Name, "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return n.Name
	case LevelProcess:
		// 格式: "hostname:processName:pid"
		parts := strings.Split(n.Name, ":")
		if len(parts) >= 2 {
			return parts[0] + ":" + parts[1]
		}
		return n.Name
	case LevelInstance:
		return n.Name
	default:
		return n.Name
	}
}

// ============================================================================
// 五、面包屑导航
// ============================================================================

// BreadcrumbBuilder 面包屑导航构建器
type BreadcrumbBuilder struct{}

// NewBreadcrumbBuilder 创建面包屑构建器
func NewBreadcrumbBuilder() *BreadcrumbBuilder {
	return &BreadcrumbBuilder{}
}

// BuildBreadcrumb 构建面包屑导航路径
func (bb *BreadcrumbBuilder) BuildBreadcrumb(steps []DrillDownStep) []DrillDownStep {
	// 去重并确保顺序正确
	seen := make(map[string]bool)
	var result []DrillDownStep

	for _, step := range steps {
		key := fmt.Sprintf("%s:%s", step.Level, step.NodeID)
		if !seen[key] {
			seen[key] = true
			result = append(result, step)
		}
	}

	return result
}

// FormatBreadcrumb 格式化面包屑为字符串
func FormatBreadcrumb(steps []DrillDownStep) string {
	if len(steps) == 0 {
		return ""
	}

	parts := make([]string, len(steps))
	for i, step := range steps {
		parts[i] = fmt.Sprintf("%s: %s", step.Level, step.NodeName)
	}
	return strings.Join(parts, " > ")
}

// ============================================================================
// 六、钻取路径推荐
// ============================================================================

// DrillDownRecommendation 钻取推荐
type DrillDownRecommendation struct {
	NodeID       graph.NodeID
	NodeName     string
	CurrentLevel HierarchyLevel
	Recommended  HierarchyLevel
	Reason       string
	Priority     int // 1-10，越高越推荐
}

// RecommendDrillDown 推荐应该下钻的节点
//
// 策略：
//   - 错误率高的节点优先
//   - 延迟高的节点优先
//   - 流量大的节点优先
func (da *DrillDownAnalyzer) RecommendDrillDown(
	g *graph.Graph,
	currentLevel HierarchyLevel,
	limit int,
) []*DrillDownRecommendation {
	if limit <= 0 {
		limit = 5
	}

	var recommendations []*DrillDownRecommendation

	nodes := g.Nodes()
	for _, n := range nodes {
		if !n.Active {
			continue
		}

		errRate := n.ErrorRate()
		latencyMs := float64(n.AvgLatencyNs) / 1e6

		// 只有异常节点才推荐下钻
		if errRate < 0.01 && latencyMs < 500 {
			continue
		}

		rec := &DrillDownRecommendation{
			NodeID:       n.ID,
			NodeName:     n.Name,
			CurrentLevel: currentLevel,
			Recommended:  NextFinerLevel(currentLevel),
		}

		// 计算优先级
		rec.Priority = computeDrillPriority(errRate, latencyMs, n.RequestCount)

		if errRate > 0.1 {
			rec.Reason = fmt.Sprintf("错误率高 (%.1f%%)", errRate*100)
		} else if latencyMs > 1000 {
			rec.Reason = fmt.Sprintf("延迟高 (%.1fms)", latencyMs)
		} else if errRate > 0.01 {
			rec.Reason = fmt.Sprintf("错误率偏高 (%.1f%%)", errRate*100)
		} else {
			rec.Reason = fmt.Sprintf("延迟偏高 (%.1fms)", latencyMs)
		}

		recommendations = append(recommendations, rec)
	}

	// 按优先级排序
	sort.Slice(recommendations, func(i, j int) bool {
		return recommendations[i].Priority > recommendations[j].Priority
	})

	if len(recommendations) > limit {
		recommendations = recommendations[:limit]
	}

	return recommendations
}

func computeDrillPriority(errorRate float64, latencyMs float64, requestCount uint64) int {
	priority := 0
	if errorRate > 0.1 {
		priority += 5
	} else if errorRate > 0.05 {
		priority += 3
	} else if errorRate > 0.01 {
		priority += 1
	}
	if latencyMs > 1000 {
		priority += 4
	} else if latencyMs > 500 {
		priority += 2
	}
	if requestCount > 10000 {
		priority += 1
	}
	return min(priority, 10)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
