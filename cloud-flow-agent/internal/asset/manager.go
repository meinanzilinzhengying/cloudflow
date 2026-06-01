//go:build linux

// Package asset 提供资产管理功能
// - 资产CRUD
// - 租户归属管理
// - 单资产下钻网络/应用指标
package asset

import (
	"fmt"
	"sync"
	"time"

	"cloud-flow-agent/internal/topology"
	"cloud-flow-agent/pkg/logger"
)

// AssetType 资产类型
type AssetType string

const (
	AssetTypeServer    AssetType = "server"    // 服务器
	AssetTypeVM        AssetType = "vm"        // 虚拟机
	AssetTypeContainer AssetType = "container" // 容器
	AssetTypeNetwork   AssetType = "network"   // 网络设备
	AssetTypeDatabase  AssetType = "database"  // 数据库
	AssetTypeMiddleware AssetType = "middleware" // 中间件
	AssetTypeApplication AssetType = "application" // 应用
	AssetTypeService   AssetType = "service"   // 服务
)

// AssetStatus 资产状态
type AssetStatus string

const (
	AssetStatusRunning   AssetStatus = "running"   // 运行中
	AssetStatusStopped   AssetStatus = "stopped"   // 已停止
	AssetStatusError     AssetStatus = "error"     // 异常
	AssetStatusMaintenance AssetStatus = "maintenance" // 维护中
	AssetStatusUnknown   AssetStatus = "unknown"   // 未知
)

// Asset 资产
type Asset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        AssetType         `json:"type"`
	Status      AssetStatus       `json:"status"`
	
	// 归属
	TenantID    string            `json:"tenant_id"`
	OwnerID     string            `json:"owner_id,omitempty"`
	
	// 网络信息
	IP          string            `json:"ip,omitempty"`
	Hostname    string            `json:"hostname,omitempty"`
	MAC         string            `json:"mac,omitempty"`
	
	// 位置信息
	Region      string            `json:"region,omitempty"`
	Zone        string            `json:"zone,omitempty"`
	Rack        string            `json:"rack,omitempty"`
	
	// 规格
	Specs       AssetSpecs        `json:"specs,omitempty"`
	
	// 标签
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	
	// 关联
	ParentID    string            `json:"parent_id,omitempty"`  // 父资产
	ChildrenIDs []string          `json:"children_ids,omitempty"` // 子资产
	
	// 告警统计
	AlertCount  int               `json:"alert_count"`
	AlertLevel  string            `json:"alert_level,omitempty"`
	
	// 时间戳
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	LastSeen    time.Time         `json:"last_seen"`
}

// AssetSpecs 资产规格
type AssetSpecs struct {
	CPU       int    `json:"cpu,omitempty"`       // CPU核数
	MemoryGB  int    `json:"memory_gb,omitempty"` // 内存GB
	DiskGB    int    `json:"disk_gb,omitempty"`   // 磁盘GB
	OS        string `json:"os,omitempty"`        // 操作系统
	Version   string `json:"version,omitempty"`   // 版本
}

// AssetMetrics 资产指标
type AssetMetrics struct {
	AssetID     string    `json:"asset_id"`
	Timestamp   time.Time `json:"timestamp"`
	
	// 网络指标
	NetworkMetrics NetworkMetrics `json:"network,omitempty"`
	
	// 应用指标
	ApplicationMetrics ApplicationMetrics `json:"application,omitempty"`
	
	// 系统指标
	SystemMetrics SystemMetrics `json:"system,omitempty"`
}

// NetworkMetrics 网络指标
type NetworkMetrics struct {
	LatencyMs       float64 `json:"latency_ms"`
	PacketLossRate  float64 `json:"packet_loss_rate"`
	RetransmitRate  float64 `json:"retransmit_rate"`
	ThroughputIn    float64 `json:"throughput_in_bps"`
	ThroughputOut   float64 `json:"throughput_out_bps"`
	ConnectionCount int     `json:"connection_count"`
	TCPConnections  int     `json:"tcp_connections"`
	UDPConnections  int     `json:"udp_connections"`
}

// ApplicationMetrics 应用指标
type ApplicationMetrics struct {
	RequestCount    int64   `json:"request_count"`
	ErrorCount      int64   `json:"error_count"`
	ErrorRate       float64 `json:"error_rate"`
	AvgResponseTime float64 `json:"avg_response_time_ms"`
	P99ResponseTime float64 `json:"p99_response_time_ms"`
	QPS             float64 `json:"qps"`
	ActiveUsers     int     `json:"active_users"`
}

// SystemMetrics 系统指标
type SystemMetrics struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	DiskPercent    float64 `json:"disk_percent"`
	LoadAvg1       float64 `json:"load_avg_1"`
	LoadAvg5       float64 `json:"load_avg_5"`
	LoadAvg15      float64 `json:"load_avg_15"`
	ProcessCount   int     `json:"process_count"`
}

// AssetFilter 资产过滤器
type AssetFilter struct {
	TenantID   string            `json:"tenant_id,omitempty"`
	Types      []AssetType       `json:"types,omitempty"`
	Statuses   []AssetStatus     `json:"statuses,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	HasAlert   bool              `json:"has_alert,omitempty"`
	Search     string            `json:"search,omitempty"`
}

// AssetManager 资产管理器
type AssetManager struct {
	mu      sync.RWMutex
	assets  map[string]*Asset
	metrics map[string][]*AssetMetrics // assetID -> metrics history
	log     *logger.Logger
}

// NewAssetManager 创建资产管理器
func NewAssetManager(log *logger.Logger) *AssetManager {
	return &AssetManager{
		assets:  make(map[string]*Asset),
		metrics: make(map[string][]*AssetMetrics),
		log:     log,
	}
}

// CreateAsset 创建资产
func (m *AssetManager) CreateAsset(asset *Asset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if asset.ID == "" {
		return fmt.Errorf("资产ID不能为空")
	}
	
	if _, exists := m.assets[asset.ID]; exists {
		return fmt.Errorf("资产已存在: %s", asset.ID)
	}
	
	asset.Status = AssetStatusRunning
	asset.CreatedAt = time.Now()
	asset.UpdatedAt = time.Now()
	asset.LastSeen = time.Now()
	
	m.assets[asset.ID] = asset
	m.log.Infof("创建资产: %s (%s), 租户: %s", asset.Name, asset.ID, asset.TenantID)
	
	return nil
}

// GetAsset 获取资产
func (m *AssetManager) GetAsset(assetID string) *Asset {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.assets[assetID]
}

// UpdateAsset 更新资产
func (m *AssetManager) UpdateAsset(asset *Asset) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.assets[asset.ID]; !exists {
		return fmt.Errorf("资产不存在: %s", asset.ID)
	}
	
	asset.UpdatedAt = time.Now()
	m.assets[asset.ID] = asset
	
	return nil
}

// DeleteAsset 删除资产
func (m *AssetManager) DeleteAsset(assetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	delete(m.assets, assetID)
	delete(m.metrics, assetID)
	m.log.Infof("删除资产: %s", assetID)
	
	return nil
}

// ListAssets 列出资产
func (m *AssetManager) ListAssets(filter AssetFilter) []*Asset {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result []*Asset
	
	for _, asset := range m.assets {
		// 租户过滤
		if filter.TenantID != "" && asset.TenantID != filter.TenantID {
			continue
		}
		
		// 类型过滤
		if len(filter.Types) > 0 {
			found := false
			for _, t := range filter.Types {
				if asset.Type == t {
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
				if asset.Status == s {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		
		// 标签过滤
		if len(filter.Labels) > 0 {
			for k, v := range filter.Labels {
				if asset.Labels[k] != v {
					continue
				}
			}
		}
		
		// 告警过滤
		if filter.HasAlert && asset.AlertCount == 0 {
			continue
		}
		
		// 搜索过滤
		if filter.Search != "" {
			if !contains(asset.Name, filter.Search) &&
			   !contains(asset.IP, filter.Search) &&
			   !contains(asset.Hostname, filter.Search) {
				continue
			}
		}
		
		result = append(result, asset)
	}
	
	return result
}

// CountAssets 统计资产数量
func (m *AssetManager) CountAssets(tenantID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if tenantID == "" {
		return len(m.assets)
	}
	
	count := 0
	for _, asset := range m.assets {
		if asset.TenantID == tenantID {
			count++
		}
	}
	return count
}

// CountAssetsByType 按类型统计
func (m *AssetManager) CountAssetsByType(tenantID string) map[AssetType]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	counts := make(map[AssetType]int)
	
	for _, asset := range m.assets {
		if tenantID != "" && asset.TenantID != tenantID {
			continue
		}
		counts[asset.Type]++
	}
	
	return counts
}

// CountAssetsByStatus 按状态统计
func (m *AssetManager) CountAssetsByStatus(tenantID string) map[AssetStatus]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	counts := make(map[AssetStatus]int)
	
	for _, asset := range m.assets {
		if tenantID != "" && asset.TenantID != tenantID {
			continue
		}
		counts[asset.Status]++
	}
	
	return counts
}

// UpdateAssetStatus 更新资产状态
func (m *AssetManager) UpdateAssetStatus(assetID string, status AssetStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if asset, exists := m.assets[assetID]; exists {
		asset.Status = status
		asset.UpdatedAt = time.Now()
		asset.LastSeen = time.Now()
	}
}

// UpdateAssetAlert 更新资产告警
func (m *AssetManager) UpdateAssetAlert(assetID string, alertCount int, alertLevel string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if asset, exists := m.assets[assetID]; exists {
		asset.AlertCount = alertCount
		asset.AlertLevel = alertLevel
		asset.UpdatedAt = time.Now()
	}
}

// StoreMetrics 存储指标
func (m *AssetManager) StoreMetrics(assetID string, metrics *AssetMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.assets[assetID]; !exists {
		return fmt.Errorf("资产不存在: %s", assetID)
	}
	
	metrics.AssetID = assetID
	metrics.Timestamp = time.Now()
	
	m.metrics[assetID] = append(m.metrics[assetID], metrics)
	
	// 限制历史数据量
	if len(m.metrics[assetID]) > 1440 { // 保留24小时（每分钟一个点）
		m.metrics[assetID] = m.metrics[assetID][len(m.metrics[assetID])-1440:]
	}
	
	return nil
}

// GetMetrics 获取指标
func (m *AssetManager) GetMetrics(assetID string, startTime, endTime time.Time) []*AssetMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result []*AssetMetrics
	
	for _, metric := range m.metrics[assetID] {
		if metric.Timestamp.After(startTime) && metric.Timestamp.Before(endTime) {
			result = append(result, metric)
		}
	}
	
	return result
}

// GetLatestMetrics 获取最新指标
func (m *AssetManager) GetLatestMetrics(assetID string) *AssetMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	metrics := m.metrics[assetID]
	if len(metrics) == 0 {
		return nil
	}
	
	return metrics[len(metrics)-1]
}

// GetAssetDrillDown 获取资产下钻数据
func (m *AssetManager) GetAssetDrillDown(assetID string) (*AssetDrillDown, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	asset, exists := m.assets[assetID]
	if !exists {
		return nil, fmt.Errorf("资产不存在: %s", assetID)
	}
	
	// 获取最新指标
	latestMetrics := m.GetLatestMetrics(assetID)
	
	// 获取历史趋势（最近1小时）
	endTime := time.Now()
	startTime := endTime.Add(-time.Hour)
	historyMetrics := m.GetMetrics(assetID, startTime, endTime)
	
	// 计算统计
	drillDown := &AssetDrillDown{
		Asset:          asset,
		LatestMetrics:  latestMetrics,
		HistoryMetrics: historyMetrics,
	}
	
	if latestMetrics != nil {
		drillDown.NetworkStats = calculateNetworkStats(historyMetrics)
		drillDown.ApplicationStats = calculateApplicationStats(historyMetrics)
		drillDown.SystemStats = calculateSystemStats(historyMetrics)
	}
	
	return drillDown, nil
}

// AssetDrillDown 资产下钻数据
type AssetDrillDown struct {
	Asset            *Asset               `json:"asset"`
	LatestMetrics    *AssetMetrics        `json:"latest_metrics,omitempty"`
	HistoryMetrics   []*AssetMetrics      `json:"history_metrics,omitempty"`
	NetworkStats     *NetworkStats        `json:"network_stats,omitempty"`
	ApplicationStats *ApplicationStats    `json:"application_stats,omitempty"`
	SystemStats      *SystemStats         `json:"system_stats,omitempty"`
}

// NetworkStats 网络统计
type NetworkStats struct {
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	MaxLatencyMs     float64 `json:"max_latency_ms"`
	AvgPacketLoss    float64 `json:"avg_packet_loss"`
	MaxPacketLoss    float64 `json:"max_packet_loss"`
	AvgThroughputIn  float64 `json:"avg_throughput_in"`
	AvgThroughputOut float64 `json:"avg_throughput_out"`
}

// ApplicationStats 应用统计
type ApplicationStats struct {
	TotalRequests    int64   `json:"total_requests"`
	TotalErrors      int64   `json:"total_errors"`
	AvgErrorRate     float64 `json:"avg_error_rate"`
	AvgResponseTime  float64 `json:"avg_response_time_ms"`
	P99ResponseTime  float64 `json:"p99_response_time_ms"`
	AvgQPS           float64 `json:"avg_qps"`
}

// SystemStats 系统统计
type SystemStats struct {
	AvgCPUPercent    float64 `json:"avg_cpu_percent"`
	MaxCPUPercent    float64 `json:"max_cpu_percent"`
	AvgMemoryPercent float64 `json:"avg_memory_percent"`
	MaxMemoryPercent float64 `json:"max_memory_percent"`
}

// 计算网络统计
func calculateNetworkStats(metrics []*AssetMetrics) *NetworkStats {
	if len(metrics) == 0 {
		return nil
	}
	
	stats := &NetworkStats{}
	var totalLatency, totalLoss, totalIn, totalOut float64
	
	for _, m := range metrics {
		nm := m.NetworkMetrics
		
		totalLatency += nm.LatencyMs
		if nm.LatencyMs > stats.MaxLatencyMs {
			stats.MaxLatencyMs = nm.LatencyMs
		}
		
		totalLoss += nm.PacketLossRate
		if nm.PacketLossRate > stats.MaxPacketLoss {
			stats.MaxPacketLoss = nm.PacketLossRate
		}
		
		totalIn += nm.ThroughputIn
		totalOut += nm.ThroughputOut
	}
	
	count := float64(len(metrics))
	stats.AvgLatencyMs = totalLatency / count
	stats.AvgPacketLoss = totalLoss / count
	stats.AvgThroughputIn = totalIn / count
	stats.AvgThroughputOut = totalOut / count
	
	return stats
}

// 计算应用统计
func calculateApplicationStats(metrics []*AssetMetrics) *ApplicationStats {
	if len(metrics) == 0 {
		return nil
	}
	
	stats := &ApplicationStats{}
	var totalErrorRate, totalResponseTime, totalQPS float64
	var responseTimes []float64
	
	for _, m := range metrics {
		am := m.ApplicationMetrics
		
		stats.TotalRequests += am.RequestCount
		stats.TotalErrors += am.ErrorCount
		totalErrorRate += am.ErrorRate
		totalResponseTime += am.AvgResponseTime
		totalQPS += am.QPS
		
		responseTimes = append(responseTimes, am.AvgResponseTime)
	}
	
	count := float64(len(metrics))
	stats.AvgErrorRate = totalErrorRate / count
	stats.AvgResponseTime = totalResponseTime / count
	stats.AvgQPS = totalQPS / count
	
	// 计算P99
	if len(responseTimes) > 0 {
		stats.P99ResponseTime = percentile(responseTimes, 0.99)
	}
	
	return stats
}

// 计算系统统计
func calculateSystemStats(metrics []*AssetMetrics) *SystemStats {
	if len(metrics) == 0 {
		return nil
	}
	
	stats := &SystemStats{}
	var totalCPU, totalMemory float64
	
	for _, m := range metrics {
		sm := m.SystemMetrics
		
		totalCPU += sm.CPUPercent
		if sm.CPUPercent > stats.MaxCPUPercent {
			stats.MaxCPUPercent = sm.CPUPercent
		}
		
		totalMemory += sm.MemoryPercent
		if sm.MemoryPercent > stats.MaxMemoryPercent {
			stats.MaxMemoryPercent = sm.MemoryPercent
		}
	}
	
	count := float64(len(metrics))
	stats.AvgCPUPercent = totalCPU / count
	stats.AvgMemoryPercent = totalMemory / count
	
	return stats
}

// percentile 计算百分位数
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	// 简单实现：排序后取对应位置
	// 实际生产环境应使用更高效的算法
	sorted := make([]float64, len(values))
	copy(sorted, values)
	
	// 冒泡排序
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(s[:len(substr)] == substr) ||
		(s[len(s)-len(substr):] == substr) ||
		containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// AssetFromTopologyNode 从拓扑节点创建资产
func AssetFromTopologyNode(node *topology.TopologyNode) *Asset {
	assetType := AssetTypeServer
	switch node.Type {
	case topology.NodeTypePod:
		assetType = AssetTypeContainer
	case topology.NodeTypeVM:
		assetType = AssetTypeVM
	case topology.NodeTypeSwitch, topology.NodeTypeRouter, topology.NodeTypeFirewall:
		assetType = AssetTypeNetwork
	}
	
	status := AssetStatusRunning
	switch node.Status {
	case topology.NodeStatusOffline:
		status = AssetStatusStopped
	case topology.NodeStatusCritical:
		status = AssetStatusError
	}
	
	return &Asset{
		ID:         node.ID,
		Name:       node.Name,
		Type:       assetType,
		Status:     status,
		IP:         getString(node.Metadata, "ip"),
		Hostname:   node.Name,
		Labels:     node.Labels,
		AlertCount: node.AlertCount,
		AlertLevel: node.AlertLevel,
		CreatedAt:  node.CreatedAt,
		UpdatedAt:  node.UpdatedAt,
		LastSeen:   node.LastSeen,
	}
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
