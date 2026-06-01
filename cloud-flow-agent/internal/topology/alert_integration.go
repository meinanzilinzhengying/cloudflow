//go:build linux

// Package topology 提供告警集成功能
package topology

import (
	"sync"
	"time"

	"cloud-flow-agent/internal/alert"
	"cloud-flow-agent/pkg/logger"
)

// AlertIntegrator 告警集成器
type AlertIntegrator struct {
	mu       sync.RWMutex
	builder  *TopologyBuilder
	manager  *alert.AlertManager
	log      *logger.Logger
	
	// 告警映射: 告警ID -> 节点ID
	alertNodeMap map[string]string
	
	// 节点告警统计
	nodeAlertStats map[string]*NodeAlertStats
}

// NodeAlertStats 节点告警统计
type NodeAlertStats struct {
	NodeID      string    `json:"node_id"`
	TotalAlerts int       `json:"total_alerts"`
	CriticalCount int     `json:"critical_count"`
	WarningCount  int     `json:"warning_count"`
	InfoCount     int     `json:"info_count"`
	LastAlertAt   time.Time `json:"last_alert_at"`
	ActiveAlerts  []string  `json:"active_alerts"`
}

// NewAlertIntegrator 创建告警集成器
func NewAlertIntegrator(builder *TopologyBuilder, manager *alert.AlertManager, log *logger.Logger) *AlertIntegrator {
	return &AlertIntegrator{
		builder:        builder,
		manager:        manager,
		log:            log,
		alertNodeMap:   make(map[string]string),
		nodeAlertStats: make(map[string]*NodeAlertStats),
	}
}

// Start 启动集成
func (i *AlertIntegrator) Start() {
	if i.builder == nil || i.manager == nil {
		return
	}
	
	// 启动同步循环
	go i.syncLoop()
	
	i.log.Info("告警集成器已启动")
}

// Stop 停止集成
func (i *AlertIntegrator) Stop() {
	// 清理资源
}

// syncLoop 同步循环
func (i *AlertIntegrator) syncLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		i.syncAlerts()
	}
}

// syncAlerts 同步告警到拓扑
func (i *AlertIntegrator) syncAlerts() {
	if i.manager == nil {
		return
	}
	
	// 获取活跃告警
	activeAlerts := i.manager.GetActiveAlerts()
	
	i.mu.Lock()
	defer i.mu.Unlock()
	
	// 重置统计
	for _, stats := range i.nodeAlertStats {
		stats.TotalAlerts = 0
		stats.CriticalCount = 0
		stats.WarningCount = 0
		stats.InfoCount = 0
		stats.ActiveAlerts = nil
	}
	
	// 处理每个告警
	for _, alertEvent := range activeAlerts {
		nodeID := i.findNodeForAlert(alertEvent)
		if nodeID == "" {
			continue
		}
		
		// 更新映射
		i.alertNodeMap[alertEvent.ID] = nodeID
		
		// 更新节点统计
		stats, exists := i.nodeAlertStats[nodeID]
		if !exists {
			stats = &NodeAlertStats{NodeID: nodeID}
			i.nodeAlertStats[nodeID] = stats
		}
		
		stats.TotalAlerts++
		stats.ActiveAlerts = append(stats.ActiveAlerts, alertEvent.ID)
		stats.LastAlertAt = alertEvent.FiredAt
		
		switch alertEvent.Level {
		case alert.AlertLevelCritical, alert.AlertLevelEmergency:
			stats.CriticalCount++
		case alert.AlertLevelWarning:
			stats.WarningCount++
		case alert.AlertLevelInfo:
			stats.InfoCount++
		}
		
		// 更新拓扑节点
		if i.builder != nil {
			alertLevel := alertEvent.Level.String()
			i.builder.UpdateNodeAlert(nodeID, stats.TotalAlerts, alertLevel)
		}
	}
	
	// 更新无告警节点的状态
	graph := i.builder.GetGraph()
	if graph != nil {
		for nodeID, node := range graph.Nodes {
			if _, hasAlert := i.nodeAlertStats[nodeID]; !hasAlert {
				if node.AlertCount > 0 {
					i.builder.UpdateNodeAlert(nodeID, 0, "")
				}
			}
		}
	}
}

// findNodeForAlert 查找告警对应的节点
func (i *AlertIntegrator) findNodeForAlert(event *alert.AlertEvent) string {
	graph := i.builder.GetGraph()
	if graph == nil {
		return ""
	}
	
	// 根据告警标签匹配节点
	for nodeID, node := range graph.Nodes {
		// 匹配节点名称
		if node.Name == event.RuleName || node.Name == event.Labels["instance"] {
			return nodeID
		}
		
		// 匹配IP地址
		if ip, ok := node.Metadata["ip"]; ok {
			if ip == event.Labels["ip"] || ip == event.Labels["instance"] {
				return nodeID
			}
		}
		
		// 匹配Pod名称
		if event.Labels["pod"] == node.Name {
			return nodeID
		}
		
		// 匹配Node名称
		if event.Labels["node"] == node.Name {
			return nodeID
		}
		
		// 匹配服务名称
		if event.Labels["service"] == node.Labels["app"] {
			return nodeID
		}
	}
	
	return ""
}

// GetNodeAlertStats 获取节点告警统计
func (i *AlertIntegrator) GetNodeAlertStats(nodeID string) *NodeAlertStats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	if stats, exists := i.nodeAlertStats[nodeID]; exists {
		return stats
	}
	return nil
}

// GetAllAlertStats 获取所有告警统计
func (i *AlertIntegrator) GetAllAlertStats() map[string]*NodeAlertStats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	result := make(map[string]*NodeAlertStats)
	for k, v := range i.nodeAlertStats {
		result[k] = v
	}
	return result
}

// HighlightAlertNodes 高亮告警节点
func (i *AlertIntegrator) HighlightAlertNodes() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	var highlighted []string
	for nodeID, stats := range i.nodeAlertStats {
		if stats.TotalAlerts > 0 {
			highlighted = append(highlighted, nodeID)
		}
	}
	return highlighted
}

// GetAlertPath 获取告警路径
func (i *AlertIntegrator) GetAlertPath(nodeID string) []string {
	graph := i.builder.GetGraph()
	if graph == nil {
		return nil
	}
	
	// 从节点向上追溯路径
	var path []string
	currentID := nodeID
	
	for currentID != "" {
		path = append([]string{currentID}, path...)
		
		node := graph.GetNode(currentID)
		if node == nil {
			break
		}
		currentID = node.ParentID
	}
	
	return path
}

// PropagateAlertStatus 传播告警状态
func (i *AlertIntegrator) PropagateAlertStatus() {
	graph := i.builder.GetGraph()
	if graph == nil {
		return
	}
	
	// 从下向上传播告警状态
	for level := 3; level >= 0; level-- {
		nodes := graph.GetNodesByLevel(level)
		
		for _, node := range nodes {
			if node.AlertCount == 0 {
				continue
			}
			
			// 向父节点传播
			if node.ParentID != "" {
				parent := graph.GetNode(node.ParentID)
				if parent != nil && parent.Status != NodeStatusCritical {
					if node.Status == NodeStatusCritical {
						i.builder.UpdateNodeStatus(node.ParentID, NodeStatusWarning)
					}
				}
			}
		}
	}
}

// AlertTopologyView 告警拓扑视图
type AlertTopologyView struct {
	Nodes []AlertNodeView `json:"nodes"`
	Edges []AlertEdgeView `json:"edges"`
}

// AlertNodeView 告警节点视图
type AlertNodeView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	AlertCount int    `json:"alert_count"`
	AlertLevel string `json:"alert_level"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}

// AlertEdgeView 告警边视图
type AlertEdgeView struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Status string `json:"status"`
}

// GetAlertTopologyView 获取告警拓扑视图
func (i *AlertIntegrator) GetAlertTopologyView() *AlertTopologyView {
	graph := i.builder.GetGraph()
	if graph == nil {
		return nil
	}
	
	view := &AlertTopologyView{}
	
	// 只包含有告警的节点及其路径
	highlighted := i.HighlightAlertNodes()
	included := make(map[string]bool)
	
	for _, nodeID := range highlighted {
		path := i.GetAlertPath(nodeID)
		for _, id := range path {
			if included[id] {
				continue
			}
			included[id] = true
			
			node := graph.GetNode(id)
			if node == nil {
				continue
			}
			
			stats := i.GetNodeAlertStats(id)
			alertCount := 0
			alertLevel := ""
			if stats != nil {
				alertCount = stats.TotalAlerts
				if stats.CriticalCount > 0 {
					alertLevel = "critical"
				} else if stats.WarningCount > 0 {
					alertLevel = "warning"
				}
			}
			
			view.Nodes = append(view.Nodes, AlertNodeView{
				ID:         node.ID,
				Name:       node.Name,
				Type:       string(node.Type),
				Status:     string(node.Status),
				AlertCount: alertCount,
				AlertLevel: alertLevel,
				X:          node.X,
				Y:          node.Y,
			})
		}
	}
	
	// 添加边
	for _, edge := range graph.Edges {
		if included[edge.Source] && included[edge.Target] {
			view.Edges = append(view.Edges, AlertEdgeView{
				Source: edge.Source,
				Target: edge.Target,
				Status: string(edge.Status),
			})
		}
	}
	
	return view
}

// GetAlertSummary 获取告警摘要
func (i *AlertIntegrator) GetAlertSummary() map[string]interface{} {
	i.mu.RLock()
	defer i.mu.RUnlock()
	
	summary := map[string]interface{}{
		"total_nodes_with_alerts": len(i.nodeAlertStats),
		"total_active_alerts":     0,
		"critical_nodes":          0,
		"warning_nodes":           0,
	}
	
	for _, stats := range i.nodeAlertStats {
		summary["total_active_alerts"] = summary["total_active_alerts"].(int) + stats.TotalAlerts
		
		if stats.CriticalCount > 0 {
			summary["critical_nodes"] = summary["critical_nodes"].(int) + 1
		} else if stats.WarningCount > 0 {
			summary["warning_nodes"] = summary["warning_nodes"].(int) + 1
		}
	}
	
	return summary
}
