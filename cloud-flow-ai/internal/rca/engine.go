// P6: 根因分析（RCA）自动化引擎
package rca

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// RCAEngine 根因分析引擎
type RCAEngine struct {
	mu sync.RWMutex

	// 因果关系图
	causeGraph *CauseGraph

	// 历史异常库
	patternDB map[string]*AnomalyPattern

	// 拓扑依赖信息
	topologyDeps map[string][]string // service -> upstream services
}

// CauseGraph 因果关系图
type CauseGraph struct {
	Nodes map[string]*CauseNode
	Edges map[string]*CauseEdge
}

// CauseNode 因果节点
type CauseNode struct {
	ID          string
	Service     string
	Metric      string
	Value       float64
	Severity    string
	Timestamp   time.Time
	Confidence  float64 // 0-1
}

// CauseEdge 因果关系边
type CauseEdge struct {
	From      string // 原因节点ID
	To        string // 结果节点ID
	Type      EdgeType
	Strength  float64 // 0-1
	Evidence  []string
}

// EdgeType 因果边类型
type EdgeType string

const (
	EdgeTypeDependency  EdgeType = "dependency"   // 服务依赖
	EdgeTypeCorrelation EdgeType = "correlation"    // 指标相关性
	EdgeTypeCascade     EdgeType = "cascade"        // 级联故障
	EdgeTypeResource    EdgeType = "resource"       // 资源竞争
)

// AnomalyPattern 异常模式
type AnomalyPattern struct {
	ID          string
	Name        string
	Symptoms    []Symptom
	RootCauses  []string
	Confidence  float64
	Occurrences int
}

// Symptom 症状
type Symptom struct {
	Service  string
	Metric   string
	Operator string // > < ==
	Threshold float64
}

// RCAResult RCA 分析结果
type RCAResult struct {
	RootCauses  []*RootCause
	CauseChain  []string
	Confidence  float64
	PatternMatch string
	Recommendations []string
	AnalyzedAt  time.Time
}

// RootCause 根因
type RootCause struct {
	Service     string
	Metric      string
	Description string
	Confidence  float64
	Evidence    []string
}

// NewRCAEngine 创建 RCA 引擎
func NewRCAEngine() *RCAEngine {
	return &RCAEngine{
		causeGraph:   NewCauseGraph(),
		patternDB:    make(map[string]*AnomalyPattern),
		topologyDeps: make(map[string][]string),
	}
}

// NewCauseGraph 创建因果关系图
func NewCauseGraph() *CauseGraph {
	return &CauseGraph{
		Nodes: make(map[string]*CauseNode),
		Edges: make(map[string]*CauseEdge),
	}
}

// SetTopologyDeps 设置拓扑依赖
func (e *RCAEngine) SetTopologyDeps(deps map[string][]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.topologyDeps = deps
}

// AddAnomalyPattern 添加异常模式
func (e *RCAEngine) AddAnomalyPattern(pattern *AnomalyPattern) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.patternDB[pattern.ID] = pattern
}

// Analyze 执行根因分析
func (e *RCAEngine) Analyze(anomalies []*AnomalyEvent) *RCAResult {
	result := &RCAResult{
		AnalyzedAt:      time.Now(),
		RootCauses:      []*RootCause{},
		Recommendations: []string{},
	}

	if len(anomalies) == 0 {
		return result
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. 构建因果图
	graph := e.buildCauseGraph(anomalies)

	// 2. 模式匹配
	matchedPattern := e.matchPattern(anomalies)
	if matchedPattern != nil {
		result.PatternMatch = matchedPattern.Name
		result.Confidence = matchedPattern.Confidence
		for _, rc := range matchedPattern.RootCauses {
			result.RootCauses = append(result.RootCauses, &RootCause{
				Service:     "",
				Description: rc,
				Confidence:  matchedPattern.Confidence,
			})
		}
	}

	// 3. 拓扑传播分析
	rootNodes := e.topologicalAnalysis(graph, anomalies)

	// 4. 如果没有模式匹配，使用拓扑分析结果
	if len(result.RootCauses) == 0 && len(rootNodes) > 0 {
		for _, node := range rootNodes {
			result.RootCauses = append(result.RootCauses, &RootCause{
				Service:     node.Service,
				Metric:      node.Metric,
				Description: fmt.Sprintf("%s 指标 %s 异常 (%.2f)", node.Service, node.Metric, node.Value),
				Confidence:  node.Confidence,
				Evidence:    []string{fmt.Sprintf("异常值: %.2f", node.Value)},
			})
		}
		result.Confidence = 0.7
	}

	// 5. 生成修复建议
	result.Recommendations = e.generateRecommendations(result.RootCauses)
	result.CauseChain = e.buildCauseChain(graph, rootNodes)

	return result
}

// AnomalyEvent 异常事件
type AnomalyEvent struct {
	ID        string
	Service   string
	Metric    string
	Value     float64
	Severity  string
	Timestamp time.Time
}

// buildCauseGraph 构建因果关系图
func (e *RCAEngine) buildCauseGraph(anomalies []*AnomalyEvent) *CauseGraph {
	graph := NewCauseGraph()

	// 添加所有异常节点
	for _, a := range anomalies {
		nodeID := fmt.Sprintf("%s:%s", a.Service, a.Metric)
		graph.Nodes[nodeID] = &CauseNode{
			ID:         nodeID,
			Service:    a.Service,
			Metric:     a.Metric,
			Value:      a.Value,
			Severity:   a.Severity,
			Timestamp:  a.Timestamp,
			Confidence: 0.8,
		}
	}

	// 根据拓扑依赖添加边
	for nodeID, node := range graph.Nodes {
		// 查找上游依赖
		upstreams, ok := e.topologyDeps[node.Service]
		if !ok {
			continue
		}
		for _, upstream := range upstreams {
			upstreamID := fmt.Sprintf("%s:%s", upstream, node.Metric)
			if _, exists := graph.Nodes[upstreamID]; exists {
				edgeID := fmt.Sprintf("%s->%s", upstreamID, nodeID)
				graph.Edges[edgeID] = &CauseEdge{
					From:     upstreamID,
					To:       nodeID,
					Type:     EdgeTypeDependency,
					Strength: 0.75,
					Evidence: []string{fmt.Sprintf("%s 依赖 %s", node.Service, upstream)},
				}
			}
		}
	}

	// 根据时间相关性添加边（时间早的异常可能是原因）
	nodeList := make([]*CauseNode, 0, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodeList = append(nodeList, n)
	}
	for i, n1 := range nodeList {
		for j, n2 := range nodeList {
			if i != j && n1.Timestamp.Before(n2.Timestamp) {
				diff := n2.Timestamp.Sub(n1.Timestamp)
				if diff < 5*time.Minute {
					edgeID := fmt.Sprintf("%s->%s", n1.ID, n2.ID)
					if _, exists := graph.Edges[edgeID]; !exists {
						graph.Edges[edgeID] = &CauseEdge{
							From:     n1.ID,
							To:       n2.ID,
							Type:     EdgeTypeCorrelation,
							Strength: 0.6 - float64(diff)/float64(5*time.Minute)*0.3,
							Evidence: []string{fmt.Sprintf("时间相关性: %v", diff)},
						}
					}
				}
			}
		}
	}

	return graph
}

// topologicalAnalysis 拓扑传播分析，找根节点
func (e *RCAEngine) topologicalAnalysis(graph *CauseGraph, anomalies []*AnomalyEvent) []*CauseNode {
	// 计算每个节点的入度（被多少节点指向）
	inDegree := make(map[string]int)
	for _, edge := range graph.Edges {
		inDegree[edge.To]++
	}

	// 根节点 = 入度为 0 的异常节点（或入度最小的）
	var rootNodes []*CauseNode
	for nodeID, node := range graph.Nodes {
		if inDegree[nodeID] == 0 {
			rootNodes = append(rootNodes, node)
		}
	}

	// 如果所有节点都有入度，选择入度最小的
	if len(rootNodes) == 0 {
		minInDegree := len(anomalies)
		for _, node := range graph.Nodes {
			if inDegree[node.ID] < minInDegree {
				minInDegree = inDegree[node.ID]
			}
		}
		for _, node := range graph.Nodes {
			if inDegree[node.ID] == minInDegree {
				rootNodes = append(rootNodes, node)
			}
		}
	}

	// 按置信度排序
	sort.Slice(rootNodes, func(i, j int) bool {
		return rootNodes[i].Confidence > rootNodes[j].Confidence
	})

	return rootNodes
}

// matchPattern 匹配异常模式
func (e *RCAEngine) matchPattern(anomalies []*AnomalyEvent) *AnomalyPattern {
	var bestMatch *AnomalyPattern
	bestScore := 0.0

	for _, pattern := range e.patternDB {
		score := e.scorePattern(pattern, anomalies)
		if score > bestScore && score > 0.5 {
			bestScore = score
			bestMatch = pattern
		}
	}

	if bestMatch != nil {
		bestMatch.Confidence = bestScore
	}
	return bestMatch
}

// scorePattern 计算模式匹配分数
func (e *RCAEngine) scorePattern(pattern *AnomalyPattern, anomalies []*AnomalyEvent) float64 {
	if len(pattern.Symptoms) == 0 || len(anomalies) == 0 {
		return 0
	}

	matched := 0
	for _, symptom := range pattern.Symptoms {
		for _, anomaly := range anomalies {
			if symptom.Service == anomaly.Service && symptom.Metric == anomaly.Metric {
				if matchOperator(anomaly.Value, symptom.Operator, symptom.Threshold) {
					matched++
					break
				}
			}
		}
	}

	return float64(matched) / float64(len(pattern.Symptoms))
}

func matchOperator(value float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	case ">=":
		return value >= threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
	}
}

// generateRecommendations 生成修复建议
func (e *RCAEngine) generateRecommendations(rootCauses []*RootCause) []string {
	var recommendations []string
	seen := make(map[string]bool)

	for _, rc := range rootCauses {
		recs := e.getRecommendationsFor(rc.Service, rc.Metric)
		for _, rec := range recs {
			if !seen[rec] {
				recommendations = append(recommendations, rec)
				seen[rec] = true
			}
		}
	}

	return recommendations
}

func (e *RCAEngine) getRecommendationsFor(service, metric string) []string {
	// 基于 metric 类型的建议
	switch metric {
	case "cpu", "cpu_usage":
		return []string{
			fmt.Sprintf("检查 %s 的 CPU 密集型进程", service),
			fmt.Sprintf("考虑扩容 %s 的实例数量", service),
			"检查是否存在死循环或低效算法",
		}
	case "memory", "memory_usage":
		return []string{
			fmt.Sprintf("检查 %s 的内存泄漏", service),
			fmt.Sprintf("考虑增加 %s 的内存限制", service),
			"检查缓存策略是否合理",
		}
	case "latency", "response_time":
		return []string{
			fmt.Sprintf("检查 %s 的数据库慢查询", service),
			fmt.Sprintf("检查 %s 的外部依赖延迟", service),
			"考虑启用缓存或优化查询",
		}
	case "error_rate":
		return []string{
			fmt.Sprintf("检查 %s 的最近部署变更", service),
			fmt.Sprintf("查看 %s 的错误日志获取堆栈信息", service),
			"检查依赖服务是否可用",
		}
	case "disk", "disk_usage":
		return []string{
			fmt.Sprintf("清理 %s 的日志文件", service),
			"检查磁盘 IO 瓶颈",
			"考虑扩容存储",
		}
	default:
		return []string{
			fmt.Sprintf("检查 %s 的 %s 指标趋势", service, metric),
			fmt.Sprintf("查看 %s 的相关日志和变更历史", service),
		}
	}
}

// buildCauseChain 构建因果链
func (e *RCAEngine) buildCauseChain(graph *CauseGraph, rootNodes []*CauseNode) []string {
	if len(rootNodes) == 0 {
		return []string{}
	}

	var chain []string
	visited := make(map[string]bool)

	// BFS 从根节点遍历因果链
	queue := make([]string, 0, len(rootNodes))
	for _, root := range rootNodes {
		queue = append(queue, root.ID)
		chain = append(chain, fmt.Sprintf("[根因] %s/%s", root.Service, root.Metric))
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		for _, edge := range graph.Edges {
			if edge.From == current && !visited[edge.To] {
				queue = append(queue, edge.To)
				node := graph.Nodes[edge.To]
				if node != nil {
					chain = append(chain, fmt.Sprintf("  -> %s/%s (%.0f%%)", node.Service, node.Metric, edge.Strength*100))
				}
			}
		}
	}

	return chain
}

// LoadBuiltinPatterns 加载内置异常模式
func (e *RCAEngine) LoadBuiltinPatterns() {
	patterns := []*AnomalyPattern{
		{
			ID:   "cascade-db-timeout",
			Name: "数据库级联超时",
			Symptoms: []Symptom{
				{Service: "db", Metric: "latency", Operator: ">", Threshold: 1000},
				{Service: "api", Metric: "latency", Operator: ">", Threshold: 2000},
				{Service: "api", Metric: "error_rate", Operator: ">", Threshold: 0.05},
			},
			RootCauses: []string{"数据库连接池耗尽", "数据库慢查询", "数据库锁竞争"},
		},
		{
			ID:   "memory-oom-cascade",
			Name: "内存溢出级联故障",
			Symptoms: []Symptom{
				{Service: "worker", Metric: "memory", Operator: ">", Threshold: 90},
				{Service: "worker", Metric: "error_rate", Operator: ">", Threshold: 0.1},
				{Service: "queue", Metric: "depth", Operator: ">", Threshold: 10000},
			},
			RootCauses: []string{"内存泄漏", "大对象未释放", "并发量突增"},
		},
		{
			ID:   "cpu-throttling",
			Name: "CPU 节流导致延迟",
			Symptoms: []Symptom{
				{Service: "app", Metric: "cpu", Operator: ">", Threshold: 85},
				{Service: "app", Metric: "latency", Operator: ">", Threshold: 500},
			},
			RootCauses: []string{"CPU 限制过低", "计算密集型任务", "垃圾回收频繁"},
		},
	}

	for _, p := range patterns {
		e.AddAnomalyPattern(p)
	}
}
