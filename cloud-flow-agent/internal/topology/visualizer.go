//go:build linux

// Package topology 提供可视化API功能
package topology

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// Visualizer 可视化器
type Visualizer struct {
	mu      sync.RWMutex
	builder *TopologyBuilder
	log     *logger.Logger
	
	// 缓存
	cache   map[string]*VisualizationCache
	
	// 配置
	config  VisualizerConfig
}

// VisualizerConfig 可视化配置
type VisualizerConfig struct {
	Enabled         bool          `yaml:"enabled" json:"enabled"`
	CacheDuration   time.Duration `yaml:"cache_duration" json:"cache_duration"`
	MaxNodes        int           `yaml:"max_nodes" json:"max_nodes"`
	DefaultLayout   LayoutType    `yaml:"default_layout" json:"default_layout"`
}

// DefaultVisualizerConfig 默认配置
func DefaultVisualizerConfig() VisualizerConfig {
	return VisualizerConfig{
		Enabled:       true,
		CacheDuration: 30 * time.Second,
		MaxNodes:      1000,
		DefaultLayout: LayoutVertical,
	}
}

// VisualizationCache 可视化缓存
type VisualizationCache struct {
	Data      interface{}
	Timestamp time.Time
}

// NewVisualizer 创建可视化器
func NewVisualizer(builder *TopologyBuilder, config VisualizerConfig, log *logger.Logger) *Visualizer {
	return &Visualizer{
		builder: builder,
		log:     log,
		cache:   make(map[string]*VisualizationCache),
		config:  config,
	}
}

// TopologyResponse 拓扑响应
type TopologyResponse struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Type    string                 `json:"type"`
	Nodes   []NodeView             `json:"nodes"`
	Edges   []EdgeView             `json:"edges"`
	Groups  []GroupView            `json:"groups"`
	Stats   TopologyStats          `json:"stats"`
	Layout  LayoutConfig           `json:"layout"`
}

// NodeView 节点视图
type NodeView struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Level       int                    `json:"level"`
	X           float64                `json:"x"`
	Y           float64                `json:"y"`
	Width       float64                `json:"width"`
	Height      float64                `json:"height"`
	ParentID    string                 `json:"parent_id,omitempty"`
	GroupID     string                 `json:"group_id,omitempty"`
	AlertCount  int                    `json:"alert_count"`
	AlertLevel  string                 `json:"alert_level,omitempty"`
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
	Labels      map[string]string      `json:"labels,omitempty"`
}

// EdgeView 边视图
type EdgeView struct {
	ID         string                 `json:"id"`
	Source     string                 `json:"source"`
	Target     string                 `json:"target"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	Bandwidth  float64                `json:"bandwidth,omitempty"`
	LatencyMs  float64                `json:"latency_ms,omitempty"`
	LossRate   float64                `json:"loss_rate,omitempty"`
}

// GroupView 分组视图
type GroupView struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Type    string            `json:"type"`
	Color   string            `json:"color"`
	NodeIDs []string          `json:"node_ids"`
}

// GetTopology 获取拓扑数据
func (v *Visualizer) GetTopology(layoutType LayoutType, groupBy string) (*TopologyResponse, error) {
	if !v.config.Enabled {
		return nil, fmt.Errorf("可视化未启用")
	}
	
	graph := v.builder.GetGraph()
	if graph == nil {
		return nil, fmt.Errorf("拓扑图未初始化")
	}
	
	// 应用布局
	if layoutType == "" {
		layoutType = v.config.DefaultLayout
	}
	
	// 计算布局位置
	v.calculateLayout(graph, layoutType)
	
	// 构建响应
	response := &TopologyResponse{
		ID:     graph.ID,
		Name:   graph.Name,
		Type:   graph.Type,
		Stats:  graph.GetStats(),
		Layout: graph.Layout,
	}
	
	// 转换节点
	for _, node := range graph.Nodes {
		if len(response.Nodes) >= v.config.MaxNodes {
			break
		}
		
		nodeView := NodeView{
			ID:         node.ID,
			Name:       node.Name,
			Type:       string(node.Type),
			Status:     string(node.Status),
			Level:      node.Level,
			X:          node.X,
			Y:          node.Y,
			Width:      node.Width,
			Height:     node.Height,
			ParentID:   node.ParentID,
			GroupID:    node.GroupID,
			AlertCount: node.AlertCount,
			AlertLevel: node.AlertLevel,
			Labels:     node.Labels,
		}
		
		// 转换指标
		if node.Metrics.LatencyMs > 0 {
			nodeView.Metrics = map[string]interface{}{
				"latency_ms":       node.Metrics.LatencyMs,
				"packet_loss_rate": node.Metrics.PacketLossRate,
				"cpu_percent":      node.Metrics.CPUPercent,
				"memory_percent":   node.Metrics.MemoryPercent,
			}
		}
		
		response.Nodes = append(response.Nodes, nodeView)
	}
	
	// 转换边
	for _, edge := range graph.Edges {
		edgeView := EdgeView{
			ID:        edge.ID,
			Source:    edge.Source,
			Target:    edge.Target,
			Type:      string(edge.Type),
			Status:    string(edge.Status),
			Bandwidth: edge.Bandwidth,
			LatencyMs: edge.LatencyMs,
			LossRate:  edge.LossRate,
		}
		response.Edges = append(response.Edges, edgeView)
	}
	
	// 转换分组
	for _, group := range graph.Groups {
		groupView := GroupView{
			ID:      group.ID,
			Name:    group.Name,
			Type:    string(group.Type),
			Color:   group.Color,
			NodeIDs: group.NodeIDs,
		}
		response.Groups = append(response.Groups, groupView)
	}
	
	return response, nil
}

// GetNodeDetail 获取节点详情
func (v *Visualizer) GetNodeDetail(nodeID string) (*NodeDetailResponse, error) {
	graph := v.builder.GetGraph()
	if graph == nil {
		return nil, fmt.Errorf("拓扑图未初始化")
	}
	
	node := graph.GetNode(nodeID)
	if node == nil {
		return nil, fmt.Errorf("节点不存在: %s", nodeID)
	}
	
	// 获取子节点
	children := graph.GetChildren(nodeID)
	
	// 获取邻居节点
	neighbors := graph.GetNeighbors(nodeID)
	
	// 获取路径
	paths := v.findPaths(graph, nodeID)
	
	response := &NodeDetailResponse{
		Node: NodeView{
			ID:         node.ID,
			Name:       node.Name,
			Type:       string(node.Type),
			Status:     string(node.Status),
			Level:      node.Level,
			ParentID:   node.ParentID,
			GroupID:    node.GroupID,
			AlertCount: node.AlertCount,
			AlertLevel: node.AlertLevel,
			Labels:     node.Labels,
			Metrics: map[string]interface{}{
				"latency_ms":       node.Metrics.LatencyMs,
				"packet_loss_rate": node.Metrics.PacketLossRate,
				"retransmit_rate":  node.Metrics.RetransmitRate,
				"throughput_bps":   node.Metrics.ThroughputBps,
				"cpu_percent":      node.Metrics.CPUPercent,
				"memory_percent":   node.Metrics.MemoryPercent,
				"connection_count": node.Metrics.ConnectionCount,
			},
		},
		Children:  make([]NodeView, 0, len(children)),
		Neighbors: make([]NodeView, 0, len(neighbors)),
		Paths:     paths,
	}
	
	// 转换子节点
	for _, child := range children {
		response.Children = append(response.Children, NodeView{
			ID:     child.ID,
			Name:   child.Name,
			Type:   string(child.Type),
			Status: string(child.Status),
		})
	}
	
	// 转换邻居节点
	for _, neighbor := range neighbors {
		response.Neighbors = append(response.Neighbors, NodeView{
			ID:     neighbor.ID,
			Name:   neighbor.Name,
			Type:   string(neighbor.Type),
			Status: string(neighbor.Status),
		})
	}
	
	return response, nil
}

// NodeDetailResponse 节点详情响应
type NodeDetailResponse struct {
	Node      NodeView   `json:"node"`
	Children  []NodeView `json:"children"`
	Neighbors []NodeView `json:"neighbors"`
	Paths     []PathInfo `json:"paths"`
}

// PathInfo 路径信息
type PathInfo struct {
	TargetID   string   `json:"target_id"`
	TargetName string   `json:"target_name"`
	Hops       int      `json:"hops"`
	Nodes      []string `json:"nodes"`
	LatencyMs  float64  `json:"latency_ms"`
}

// findPaths 查找路径
func (v *Visualizer) findPaths(graph *TopologyGraph, sourceID string) []PathInfo {
	var paths []PathInfo
	
	// 简单的BFS查找到各层的路径
	for level := 0; level <= 3; level++ {
		nodes := graph.GetNodesByLevel(level)
		for _, node := range nodes {
			if node.ID == sourceID {
				continue
			}
			
			// 简化：直接连接或父子关系
			hops := abs(node.Level - graph.GetNode(sourceID).Level)
			
			paths = append(paths, PathInfo{
				TargetID:   node.ID,
				TargetName: node.Name,
				Hops:       hops,
				Nodes:      []string{sourceID, node.ID},
				LatencyMs:  float64(hops) * 0.5, // 估算延迟
			})
		}
	}
	
	return paths
}

// DrillDown 下钻查询
func (v *Visualizer) DrillDown(req DrillDownRequest) (*DrillDownResponse, error) {
	graph := v.builder.GetGraph()
	if graph == nil {
		return nil, fmt.Errorf("拓扑图未初始化")
	}
	
	node := graph.GetNode(req.NodeID)
	if node == nil {
		return nil, fmt.Errorf("节点不存在: %s", req.NodeID)
	}
	
	// 解析时间范围
	duration := parseDuration(req.TimeRange)
	endTime := time.Now()
	startTime := endTime.Add(-duration)
	
	// 生成模拟数据（实际应从时序数据库查询）
	data := v.generateMetricData(node, req.MetricType, startTime, endTime)
	
	// 计算统计
	stats := calculateStatistics(data)
	
	response := &DrillDownResponse{
		NodeID:     req.NodeID,
		MetricType: req.MetricType,
		Data:       data,
		Statistics: stats,
	}
	
	return response, nil
}

// generateMetricData 生成指标数据
func (v *Visualizer) generateMetricData(node *TopologyNode, metricType string, startTime, endTime time.Time) []MetricDataPoint {
	var data []MetricDataPoint
	
	// 根据指标类型获取基准值
	var baseValue float64
	switch metricType {
	case "latency":
		baseValue = node.Metrics.LatencyMs
	case "throughput":
		baseValue = node.Metrics.ThroughputBps
	case "cpu":
		baseValue = node.Metrics.CPUPercent
	case "memory":
		baseValue = node.Metrics.MemoryPercent
	case "packet_loss":
		baseValue = node.Metrics.PacketLossRate
	default:
		baseValue = 50
	}
	
	// 生成时间序列数据
	interval := time.Minute
	for t := startTime; t.Before(endTime); t = t.Add(interval) {
		// 添加随机波动
		value := baseValue + (float64(t.Unix()%10) - 5)
		if value < 0 {
			value = 0
		}
		
		data = append(data, MetricDataPoint{
			Timestamp: t.Unix(),
			Value:     value,
		})
	}
	
	return data
}

// calculateStatistics 计算统计
func calculateStatistics(data []MetricDataPoint) MetricStatistics {
	if len(data) == 0 {
		return MetricStatistics{}
	}
	
	var min, max, sum float64
	min = data[0].Value
	max = data[0].Value
	
	for _, d := range data {
		if d.Value < min {
			min = d.Value
		}
		if d.Value > max {
			max = d.Value
		}
		sum += d.Value
	}
	
	avg := sum / float64(len(data))
	
	// 计算P95和P99
	sort.Slice(data, func(i, j int) bool {
		return data[i].Value < data[j].Value
	})
	
	p95Index := int(float64(len(data)) * 0.95)
	p99Index := int(float64(len(data)) * 0.99)
	
	if p95Index >= len(data) {
		p95Index = len(data) - 1
	}
	if p99Index >= len(data) {
		p99Index = len(data) - 1
	}
	
	return MetricStatistics{
		Min:   min,
		Max:   max,
		Avg:   avg,
		P95:   data[p95Index].Value,
		P99:   data[p99Index].Value,
		Count: len(data),
	}
}

// calculateLayout 计算布局
func (v *Visualizer) calculateLayout(graph *TopologyGraph, layoutType LayoutType) {
	graph.mu.Lock()
	defer graph.mu.Unlock()
	
	switch layoutType {
	case LayoutVertical:
		v.calculateVerticalLayout(graph)
	case LayoutHorizontal:
		v.calculateHorizontalLayout(graph)
	case LayoutRadial:
		v.calculateRadialLayout(graph)
	default:
		v.calculateVerticalLayout(graph)
	}
}

// calculateVerticalLayout 纵向布局
func (v *Visualizer) calculateVerticalLayout(graph *TopologyGraph) {
	graph.Layout.Type = LayoutVertical
	graph.Layout.Direction = "TB"
	
	levelHeight := graph.Layout.LevelSpacing
	nodeWidth := 120.0
	nodeHeight := 60.0
	
	for level := 0; level <= 3; level++ {
		nodes := graph.Levels[level]
		if len(nodes) == 0 {
			continue
		}
		
		// 计算该层的起始X位置（居中）
		totalWidth := float64(len(nodes)) * nodeWidth + float64(len(nodes)-1) * graph.Layout.NodeSpacing
		startX := -totalWidth / 2
		
		for i, nodeID := range nodes {
			if node, exists := graph.Nodes[nodeID]; exists {
				node.X = startX + float64(i) * (nodeWidth + graph.Layout.NodeSpacing)
				node.Y = float64(level) * levelHeight
				node.Width = nodeWidth
				node.Height = nodeHeight
			}
		}
	}
}

// calculateHorizontalLayout 横向布局
func (v *Visualizer) calculateHorizontalLayout(graph *TopologyGraph) {
	graph.Layout.Type = LayoutHorizontal
	graph.Layout.Direction = "LR"
	
	levelWidth := graph.Layout.LevelSpacing
	nodeWidth := 120.0
	nodeHeight := 60.0
	
	for level := 0; level <= 3; level++ {
		nodes := graph.Levels[level]
		if len(nodes) == 0 {
			continue
		}
		
		// 计算该层的起始Y位置（居中）
		totalHeight := float64(len(nodes)) * nodeHeight + float64(len(nodes)-1) * graph.Layout.NodeSpacing
		startY := -totalHeight / 2
		
		for i, nodeID := range nodes {
			if node, exists := graph.Nodes[nodeID]; exists {
				node.X = float64(level) * levelWidth
				node.Y = startY + float64(i) * (nodeHeight + graph.Layout.NodeSpacing)
				node.Width = nodeWidth
				node.Height = nodeHeight
			}
		}
	}
}

// calculateRadialLayout 径向布局
func (v *Visualizer) calculateRadialLayout(graph *TopologyGraph) {
	graph.Layout.Type = LayoutRadial
	
	centerX, centerY := 0.0, 0.0
	maxRadius := 400.0
	
	maxLevel := 3
	for level := 0; level <= maxLevel; level++ {
		nodes := graph.Levels[level]
		if len(nodes) == 0 {
			continue
		}
		
		radius := maxRadius * float64(level) / float64(maxLevel)
		if radius == 0 {
			radius = 50
		}
		
		angleStep := 2 * 3.14159 / float64(len(nodes))
		
		for i, nodeID := range nodes {
			if node, exists := graph.Nodes[nodeID]; exists {
				angle := float64(i) * angleStep
				node.X = centerX + radius * cos(angle)
				node.Y = centerY + radius * sin(angle)
				node.Width = 100
				node.Height = 50
			}
		}
	}
}

// HTTPHandler HTTP处理器
type HTTPHandler struct {
	visualizer *Visualizer
}

// NewHTTPHandler 创建HTTP处理器
func NewHTTPHandler(visualizer *Visualizer) *HTTPHandler {
	return &HTTPHandler{visualizer: visualizer}
}

// RegisterRoutes 注册路由
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/topology", h.handleTopology)
	mux.HandleFunc("/api/v1/topology/node/", h.handleNodeDetail)
	mux.HandleFunc("/api/v1/topology/drilldown", h.handleDrillDown)
	mux.HandleFunc("/api/v1/topology/layout", h.handleSetLayout)
}

// handleTopology 处理拓扑请求
func (h *HTTPHandler) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 获取布局类型
	layoutType := LayoutType(r.URL.Query().Get("layout"))
	if layoutType == "" {
		layoutType = LayoutVertical
	}
	
	response, err := h.visualizer.GetTopology(layoutType, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNodeDetail 处理节点详情请求
func (h *HTTPHandler) handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 提取节点ID
	nodeID := r.URL.Path[len("/api/v1/topology/node/"):]
	if nodeID == "" {
		http.Error(w, "Node ID required", http.StatusBadRequest)
		return
	}
	
	response, err := h.visualizer.GetNodeDetail(nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDrillDown 处理下钻请求
func (h *HTTPHandler) handleDrillDown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	req := DrillDownRequest{
		NodeID:      r.URL.Query().Get("node_id"),
		MetricType:  r.URL.Query().Get("metric_type"),
		TimeRange:   r.URL.Query().Get("time_range"),
		Aggregation: r.URL.Query().Get("aggregation"),
	}
	
	response, err := h.visualizer.DrillDown(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSetLayout 处理设置布局请求
func (h *HTTPHandler) handleSetLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	var req struct {
		LayoutType string `json:"layout_type"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	// 重新计算布局
	graph := h.visualizer.builder.GetGraph()
	if graph != nil {
		h.visualizer.calculateLayout(graph, LayoutType(req.LayoutType))
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// 辅助函数

func parseDuration(s string) time.Duration {
	switch s {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "24h", "1d":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	default:
		return time.Hour
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func cos(x float64) float64 {
	// 简化实现
	return 1 - x*x/2 + x*x*x*x/24
}

func sin(x float64) float64 {
	// 简化实现
	return x - x*x*x/6 + x*x*x*x*x/120
}
