//go:build linux

// Package dashboard 提供仪表盘API功能
// - 管理员/租户视图
// - 汇总资产数、告警数
// - 单资产下钻网络/应用指标
package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"cloud-flow-agent/internal/asset"
	"cloud-flow-agent/internal/tenant"
	"cloud-flow-agent/pkg/logger"
)

// DashboardAPI 仪表盘API
type DashboardAPI struct {
	tenantManager *tenant.TenantManager
	assetManager  *asset.AssetManager
	log           *logger.Logger
}

// NewDashboardAPI 创建仪表盘API
func NewDashboardAPI(tenantManager *tenant.TenantManager, assetManager *asset.AssetManager, log *logger.Logger) *DashboardAPI {
	return &DashboardAPI{
		tenantManager: tenantManager,
		assetManager:  assetManager,
		log:           log,
	}
}

// RegisterRoutes 注册路由
func (api *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	// 管理员视图
	mux.HandleFunc("/api/v1/admin/dashboard", api.handleAdminDashboard)
	mux.HandleFunc("/api/v1/admin/tenants", api.handleListTenants)
	mux.HandleFunc("/api/v1/admin/assets", api.handleListAllAssets)
	
	// 租户视图
	mux.HandleFunc("/api/v1/tenant/dashboard", api.handleTenantDashboard)
	mux.HandleFunc("/api/v1/tenant/assets", api.handleListTenantAssets)
	
	// 通用接口
	mux.HandleFunc("/api/v1/assets/", api.handleAssetDetail)
	mux.HandleFunc("/api/v1/assets/drilldown", api.handleAssetDrillDown)
	mux.HandleFunc("/api/v1/metrics/network", api.handleNetworkMetrics)
	mux.HandleFunc("/api/v1/metrics/application", api.handleApplicationMetrics)
}

// AdminDashboardResponse 管理员仪表盘响应
type AdminDashboardResponse struct {
	Summary AdminSummary `json:"summary"`
	Tenants []TenantInfo `json:"tenants"`
	Alerts  AlertSummary `json:"alerts"`
}

// AdminSummary 管理员汇总
type AdminSummary struct {
	TotalTenants   int `json:"total_tenants"`
	TotalAssets    int `json:"total_assets"`
	TotalUsers     int `json:"total_users"`
	TotalAlerts    int `json:"total_alerts"`
	CriticalAlerts int `json:"critical_alerts"`
}

// TenantInfo 租户信息
type TenantInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AssetCount  int    `json:"asset_count"`
	UserCount   int    `json:"user_count"`
	AlertCount  int    `json:"alert_count"`
	Status      string `json:"status"`
}

// AlertSummary 告警汇总
type AlertSummary struct {
	Total      int            `json:"total"`
	Critical   int            `json:"critical"`
	Warning    int            `json:"warning"`
	Info       int            `json:"info"`
	ByTenant   map[string]int `json:"by_tenant"`
	ByAsset    map[string]int `json:"by_asset"`
}

// handleAdminDashboard 处理管理员仪表盘请求
func (api *DashboardAPI) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 检查权限
	if !tenant.IsAdmin(r.Context()) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	// 获取所有租户
	tenants := api.tenantManager.ListTenants()
	
	// 获取所有资产
	allAssets := api.assetManager.ListAssets(asset.AssetFilter{})
	
	// 获取所有用户
	allUsers := api.tenantManager.ListUsers("")
	
	// 计算告警统计
	alertSummary := api.calculateAlertSummary("")
	
	// 构建租户信息
	var tenantInfos []TenantInfo
	for _, t := range tenants {
		stats := api.tenantManager.GetTenantStats(t.ID)
		
		tenantInfos = append(tenantInfos, TenantInfo{
			ID:         t.ID,
			Name:       t.Name,
			AssetCount: api.assetManager.CountAssets(t.ID),
			UserCount:  stats["user_count"].(int),
			AlertCount: 0, // 需要从告警模块获取
			Status:     string(t.Status),
		})
	}
	
	response := AdminDashboardResponse{
		Summary: AdminSummary{
			TotalTenants:   len(tenants),
			TotalAssets:    len(allAssets),
			TotalUsers:     len(allUsers),
			TotalAlerts:    alertSummary.Total,
			CriticalAlerts: alertSummary.Critical,
		},
		Tenants: tenantInfos,
		Alerts:  alertSummary,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// TenantDashboardResponse 租户仪表盘响应
type TenantDashboardResponse struct {
	Summary     TenantSummary          `json:"summary"`
	Assets      []AssetInfo            `json:"assets"`
	AssetTypes  map[string]int         `json:"asset_types"`
	AssetStatus map[string]int         `json:"asset_status"`
	Alerts      AlertSummary           `json:"alerts"`
	Trends      MetricTrends           `json:"trends"`
}

// TenantSummary 租户汇总
type TenantSummary struct {
	TenantID    string `json:"tenant_id"`
	TenantName  string `json:"tenant_name"`
	AssetCount  int    `json:"asset_count"`
	AlertCount  int    `json:"alert_count"`
	OnlineCount int    `json:"online_count"`
	ErrorCount  int    `json:"error_count"`
}

// AssetInfo 资产信息
type AssetInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	IP         string `json:"ip"`
	AlertCount int    `json:"alert_count"`
	AlertLevel string `json:"alert_level,omitempty"`
	LastSeen   string `json:"last_seen"`
}

// MetricTrends 指标趋势
type MetricTrends struct {
	Network   []TrendPoint `json:"network"`
	Application []TrendPoint `json:"application"`
	System    []TrendPoint `json:"system"`
}

// TrendPoint 趋势点
type TrendPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// handleTenantDashboard 处理租户仪表盘请求
func (api *DashboardAPI) handleTenantDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 获取租户ID
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if tenantID == "" {
		http.Error(w, "Tenant ID required", http.StatusBadRequest)
		return
	}
	
	// 获取租户信息
	t := api.tenantManager.GetTenant(tenantID)
	if t == nil {
		http.Error(w, "Tenant not found", http.StatusNotFound)
		return
	}
	
	// 获取租户资产
	assets := api.assetManager.ListAssets(asset.AssetFilter{TenantID: tenantID})
	
	// 计算状态统计
	statusCounts := api.assetManager.CountAssetsByStatus(tenantID)
	typeCounts := api.assetManager.CountAssetsByType(tenantID)
	
	// 计算告警统计
	alertSummary := api.calculateAlertSummary(tenantID)
	
	// 构建资产信息
	var assetInfos []AssetInfo
	for _, a := range assets {
		assetInfos = append(assetInfos, AssetInfo{
			ID:         a.ID,
			Name:       a.Name,
			Type:       string(a.Type),
			Status:     string(a.Status),
			IP:         a.IP,
			AlertCount: a.AlertCount,
			AlertLevel: a.AlertLevel,
			LastSeen:   a.LastSeen.Format(time.RFC3339),
		})
	}
	
	// 获取趋势数据
	trends := api.getMetricTrends(tenantID)
	
	response := TenantDashboardResponse{
		Summary: TenantSummary{
			TenantID:    tenantID,
			TenantName:  t.Name,
			AssetCount:  len(assets),
			AlertCount:  alertSummary.Total,
			OnlineCount: statusCounts[asset.AssetStatusRunning],
			ErrorCount:  statusCounts[asset.AssetStatusError],
		},
		Assets:      assetInfos,
		AssetTypes:  convertTypeCounts(typeCounts),
		AssetStatus: convertStatusCounts(statusCounts),
		Alerts:      alertSummary,
		Trends:      trends,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListTenants 处理列出租户请求
func (api *DashboardAPI) handleListTenants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 检查权限
	if !tenant.IsAdmin(r.Context()) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	tenants := api.tenantManager.ListTenants()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenants)
}

// handleListAllAssets 处理列出所有资产请求
func (api *DashboardAPI) handleListAllAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 检查权限
	if !tenant.IsAdmin(r.Context()) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	assets := api.assetManager.ListAssets(asset.AssetFilter{})
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assets)
}

// handleListTenantAssets 处理列出租户资产请求
func (api *DashboardAPI) handleListTenantAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 获取租户ID
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if tenantID == "" {
		http.Error(w, "Tenant ID required", http.StatusBadRequest)
		return
	}
	
	// 解析查询参数
	filter := asset.AssetFilter{
		TenantID: tenantID,
	}
	
	if types := r.URL.Query().Get("type"); types != "" {
		filter.Types = []asset.AssetType{asset.AssetType(types)}
	}
	
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Statuses = []asset.AssetStatus{asset.AssetStatus(status)}
	}
	
	if search := r.URL.Query().Get("search"); search != "" {
		filter.Search = search
	}
	
	assets := api.assetManager.ListAssets(filter)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assets)
}

// handleAssetDetail 处理资产详情请求
func (api *DashboardAPI) handleAssetDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 提取资产ID
	assetID := r.URL.Path[len("/api/v1/assets/"):]
	if assetID == "" {
		http.Error(w, "Asset ID required", http.StatusBadRequest)
		return
	}
	
	// 获取资产
	a := api.assetManager.GetAsset(assetID)
	if a == nil {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	
	// 检查权限
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if !tenant.IsAdmin(r.Context()) && a.TenantID != tenantID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

// handleAssetDrillDown 处理资产下钻请求
func (api *DashboardAPI) handleAssetDrillDown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	// 获取资产ID
	assetID := r.URL.Query().Get("asset_id")
	if assetID == "" {
		http.Error(w, "Asset ID required", http.StatusBadRequest)
		return
	}
	
	// 获取资产
	a := api.assetManager.GetAsset(assetID)
	if a == nil {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	
	// 检查权限
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if !tenant.IsAdmin(r.Context()) && a.TenantID != tenantID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	// 获取下钻数据
	drillDown, err := api.assetManager.GetAssetDrillDown(assetID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(drillDown)
}

// handleNetworkMetrics 处理网络指标请求
func (api *DashboardAPI) handleNetworkMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	assetID := r.URL.Query().Get("asset_id")
	if assetID == "" {
		http.Error(w, "Asset ID required", http.StatusBadRequest)
		return
	}
	
	// 检查权限
	a := api.assetManager.GetAsset(assetID)
	if a == nil {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if !tenant.IsAdmin(r.Context()) && a.TenantID != tenantID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	// 获取时间范围
	duration := r.URL.Query().Get("duration")
	if duration == "" {
		duration = "1h"
	}
	
	endTime := time.Now()
	startTime := endTime.Add(-parseDuration(duration))
	
	metrics := api.assetManager.GetMetrics(assetID, startTime, endTime)
	
	// 提取网络指标
	var networkMetrics []asset.NetworkMetrics
	for _, m := range metrics {
		networkMetrics = append(networkMetrics, m.NetworkMetrics)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networkMetrics)
}

// handleApplicationMetrics 处理应用指标请求
func (api *DashboardAPI) handleApplicationMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	assetID := r.URL.Query().Get("asset_id")
	if assetID == "" {
		http.Error(w, "Asset ID required", http.StatusBadRequest)
		return
	}
	
	// 检查权限
	a := api.assetManager.GetAsset(assetID)
	if a == nil {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	
	tenantID := tenant.GetTenantIDFromContext(r.Context())
	if !tenant.IsAdmin(r.Context()) && a.TenantID != tenantID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	
	// 获取时间范围
	duration := r.URL.Query().Get("duration")
	if duration == "" {
		duration = "1h"
	}
	
	endTime := time.Now()
	startTime := endTime.Add(-parseDuration(duration))
	
	metrics := api.assetManager.GetMetrics(assetID, startTime, endTime)
	
	// 提取应用指标
	var appMetrics []asset.ApplicationMetrics
	for _, m := range metrics {
		appMetrics = append(appMetrics, m.ApplicationMetrics)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appMetrics)
}

// calculateAlertSummary 计算告警汇总
func (api *DashboardAPI) calculateAlertSummary(tenantID string) AlertSummary {
	// 简化实现，实际应从告警模块获取
	summary := AlertSummary{
		ByTenant: make(map[string]int),
		ByAsset:  make(map[string]int),
	}
	
	// 遍历资产统计告警
	assets := api.assetManager.ListAssets(asset.AssetFilter{TenantID: tenantID})
	for _, a := range assets {
		if a.AlertCount > 0 {
			summary.Total += a.AlertCount
			summary.ByAsset[a.ID] = a.AlertCount
			summary.ByTenant[a.TenantID] += a.AlertCount
			
			switch a.AlertLevel {
			case "critical", "emergency":
				summary.Critical += a.AlertCount
			case "warning":
				summary.Warning += a.AlertCount
			default:
				summary.Info += a.AlertCount
			}
		}
	}
	
	return summary
}

// getMetricTrends 获取指标趋势
func (api *DashboardAPI) getMetricTrends(tenantID string) MetricTrends {
	trends := MetricTrends{
		Network:     make([]TrendPoint, 0),
		Application: make([]TrendPoint, 0),
		System:      make([]TrendPoint, 0),
	}
	
	// 获取租户资产
	assets := api.assetManager.ListAssets(asset.AssetFilter{TenantID: tenantID})
	if len(assets) == 0 {
		return trends
	}
	
	// 获取最近1小时的指标
	endTime := time.Now()
	startTime := endTime.Add(-time.Hour)
	
	// 聚合所有资产的指标
	for _, a := range assets {
		metrics := api.assetManager.GetMetrics(a.ID, startTime, endTime)
		
		for _, m := range metrics {
			ts := m.Timestamp.Unix()
			
			// 网络延迟趋势
			trends.Network = append(trends.Network, TrendPoint{
				Timestamp: ts,
				Value:     m.NetworkMetrics.LatencyMs,
			})
			
			// 应用QPS趋势
			trends.Application = append(trends.Application, TrendPoint{
				Timestamp: ts,
				Value:     m.ApplicationMetrics.QPS,
			})
			
			// 系统CPU趋势
			trends.System = append(trends.System, TrendPoint{
				Timestamp: ts,
				Value:     m.SystemMetrics.CPUPercent,
			})
		}
	}
	
	return trends
}

// 辅助函数

func convertTypeCounts(counts map[asset.AssetType]int) map[string]int {
	result := make(map[string]int)
	for t, c := range counts {
		result[string(t)] = c
	}
	return result
}

func convertStatusCounts(counts map[asset.AssetStatus]int) map[string]int {
	result := make(map[string]int)
	for s, c := range counts {
		result[string(s)] = c
	}
	return result
}

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
