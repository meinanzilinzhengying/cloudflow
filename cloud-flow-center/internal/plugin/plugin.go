//go:build linux

package plugin

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// P12 插件化与扩展机制
// 解决：缺少插件机制、缺少 SDK 和 API 生态、缺少自定义仪表板报表、缺少二次开发文档
// ============================================================================

// ============================================================================
// 1. 插件类型与接口定义
// ============================================================================

// PluginType 插件类型
type PluginType string

const (
	PluginProtocolParser PluginType = "protocol_parser"   // 协议解析插件
	PluginAlertChannel   PluginType = "alert_channel"       // 告警通道插件
	PluginDataProcessor  PluginType = "data_processor"      // 数据处理插件
	PluginDashboard      PluginType = "dashboard"           // 仪表板插件
	PluginExporter       PluginType = "exporter"            // 导出插件
	PluginAuthProvider   PluginType = "auth_provider"       // 认证插件
)

// Plugin 插件接口
type Plugin interface {
	ID() string
	Name() string
	Version() string
	Type() PluginType
	Init(config map[string]interface{}) error
	Start() error
	Stop() error
	Health() (bool, string)
}

// BasePlugin 插件基础实现
type BasePlugin struct {
	id      string
	name    string
	version string
	pType   PluginType
	config  map[string]interface{}
	
	mu      sync.RWMutex
	running bool
}

// NewBasePlugin 创建基础插件
func NewBasePlugin(id, name, version string, pType PluginType) *BasePlugin {
	return &BasePlugin{
		id:      id,
		name:    name,
		version: version,
		pType:   pType,
		config:  make(map[string]interface{}),
	}
}

func (bp *BasePlugin) ID() string                       { return bp.id }
func (bp *BasePlugin) Name() string                   { return bp.name }
func (bp *BasePlugin) Version() string                { return bp.version }
func (bp *BasePlugin) Type() PluginType                { return bp.pType }
func (bp *BasePlugin) Init(config map[string]interface{}) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.config = config
	return nil
}
func (bp *BasePlugin) Start() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.running = true
	return nil
}
func (bp *BasePlugin) Stop() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.running = false
	return nil
}
func (bp *BasePlugin) Health() (bool, string) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	if bp.running {
		return true, "running"
	}
	return false, "stopped"
}

// ============================================================================
// 2. 插件管理器
// ============================================================================

// PluginManager 插件管理器
type PluginManager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	byType  map[PluginType][]string
	
	registryDir string
	
	// 扩展点：类型 -> 扩展列表
	extensions map[string][]Extension
}

// NewPluginManager 创建插件管理器
func NewPluginManager(registryDir string) *PluginManager {
	return &PluginManager{
		plugins:     make(map[string]Plugin),
		byType:      make(map[PluginType][]string),
		registryDir: registryDir,
		extensions:  make(map[string][]Extension),
	}
}

// Register 注册插件
func (pm *PluginManager) Register(p Plugin) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	if _, ok := pm.plugins[p.ID()]; ok {
		return fmt.Errorf("plugin %s already registered", p.ID())
	}
	
	pm.plugins[p.ID()] = p
	pm.byType[p.Type()] = append(pm.byType[p.Type()], p.ID())
	return nil
}

// Unregister 注销插件
func (pm *PluginManager) Unregister(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	p, ok := pm.plugins[id]
	if !ok {
		return fmt.Errorf("plugin %s not found", id)
	}
	
	p.Stop()
	delete(pm.plugins, id)
	
	// 从类型列表中移除
	list := pm.byType[p.Type()]
	newList := make([]string, 0, len(list)-1)
	for _, pid := range list {
		if pid != id {
			newList = append(newList, pid)
		}
	}
	pm.byType[p.Type()] = newList
	return nil
}

// GetPlugin 获取插件
func (pm *PluginManager) GetPlugin(id string) Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.plugins[id]
}

// GetPluginsByType 按类型获取插件
func (pm *PluginManager) GetPluginsByType(pType PluginType) []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	ids, ok := pm.byType[pType]
	if !ok {
		return nil
	}
	
	result := make([]Plugin, 0, len(ids))
	for _, id := range ids {
		if p, ok := pm.plugins[id]; ok {
			result = append(result, p)
		}
	}
	return result
}

// GetAllPlugins 获取所有插件
func (pm *PluginManager) GetAllPlugins() []Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	result := make([]Plugin, 0, len(pm.plugins))
	for _, p := range pm.plugins {
		result = append(result, p)
	}
	return result
}

// StartPlugin 启动插件
func (pm *PluginManager) StartPlugin(id string) error {
	pm.mu.RLock()
	p, ok := pm.plugins[id]
	pm.mu.RUnlock()
	
	if !ok {
		return fmt.Errorf("plugin %s not found", id)
	}
	return p.Start()
}

// StopPlugin 停止插件
func (pm *PluginManager) StopPlugin(id string) error {
	pm.mu.RLock()
	p, ok := pm.plugins[id]
	pm.mu.RUnlock()
	
	if !ok {
		return fmt.Errorf("plugin %s not found", id)
	}
	return p.Stop()
}

// GetPluginCount 获取插件数量
func (pm *PluginManager) GetPluginCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.plugins)
}

// GetPluginCountByType 按类型统计插件数量
func (pm *PluginManager) GetPluginCountByType(pType PluginType) int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.byType[pType])
}

// ============================================================================
// 3. 扩展点机制（Extension Point）
// ============================================================================

// Extension 扩展接口
type Extension interface {
	Name() string
	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

// ExtensionPoint 扩展点
type ExtensionPoint struct {
	Name        string
	Description string
	Required    bool
}

// RegisterExtension 注册扩展
func (pm *PluginManager) RegisterExtension(pointName string, ext Extension) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.extensions[pointName] = append(pm.extensions[pointName], ext)
}

// ExecuteExtensions 执行扩展点
func (pm *PluginManager) ExecuteExtensions(ctx context.Context, pointName string, args map[string]interface{}) ([]interface{}, error) {
	pm.mu.RLock()
	exts := pm.extensions[pointName]
	pm.mu.RUnlock()
	
	if len(exts) == 0 {
		return nil, nil
	}
	
	results := make([]interface{}, 0, len(exts))
	for _, ext := range exts {
		result, err := ext.Execute(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("extension %s execution failed: %w", ext.Name(), err)
		}
		results = append(results, result)
	}
	return results, nil
}

// GetExtensionPointNames 获取扩展点名称列表
func (pm *PluginManager) GetExtensionPointNames() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	result := make([]string, 0, len(pm.extensions))
	for name := range pm.extensions {
		result = append(result, name)
	}
	return result
}

// ============================================================================
// 4. SDK 扩展机制
// ============================================================================

// SDKExtension SDK 扩展接口
type SDKExtension interface {
	Name() string
	Version() string
	APIs() map[string]APIHandler
}

// APIHandler API 处理函数
type APIHandler func(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error)

// SDKManager SDK 管理器
type SDKManager struct {
	mu        sync.RWMutex
	sdks      map[string]SDKExtension
	apiRoutes map[string]APIHandler
}

// NewSDKManager 创建 SDK 管理器
func NewSDKManager() *SDKManager {
	return &SDKManager{
		sdks:      make(map[string]SDKExtension),
		apiRoutes: make(map[string]APIHandler),
	}
}

// RegisterSDK 注册 SDK
func (sm *SDKManager) RegisterSDK(sdk SDKExtension) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if _, ok := sm.sdks[sdk.Name()]; ok {
		return fmt.Errorf("SDK %s already registered", sdk.Name())
	}
	
	sm.sdks[sdk.Name()] = sdk
	
	// 注册 API 路由
	for path, handler := range sdk.APIs() {
		routeKey := sdk.Name() + "/" + path
		if _, ok := sm.apiRoutes[routeKey]; ok {
			return fmt.Errorf("API route %s already registered", routeKey)
		}
		sm.apiRoutes[routeKey] = handler
	}
	
	return nil
}

// UnregisterSDK 注销 SDK
func (sm *SDKManager) UnregisterSDK(name string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	sdk, ok := sm.sdks[name]
	if !ok {
		return
	}
	
	// 清理 API 路由
	for path := range sdk.APIs() {
		delete(sm.apiRoutes, name+"/"+path)
	}
	delete(sm.sdks, name)
}

// CallAPI 调用 API
func (sm *SDKManager) CallAPI(ctx context.Context, sdkName, path string, req map[string]interface{}) (map[string]interface{}, error) {
	sm.mu.RLock()
	handler, ok := sm.apiRoutes[sdkName+"/"+path]
	sm.mu.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("API %s/%s not found", sdkName, path)
	}
	
	return handler(ctx, req)
}

// GetRegisteredSDKs 获取已注册的 SDK 列表
func (sm *SDKManager) GetRegisteredSDKs() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	result := make([]string, 0, len(sm.sdks))
	for name := range sm.sdks {
		result = append(result, name)
	}
	return result
}

// GetAPIRoutes 获取所有 API 路由
func (sm *SDKManager) GetAPIRoutes() map[string]APIHandler {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	result := make(map[string]APIHandler)
	for k, v := range sm.apiRoutes {
		result[k] = v
	}
	return result
}

// ============================================================================
// 5. 自定义仪表板和报表
// ============================================================================

// WidgetType 组件类型
type WidgetType string

const (
	WidgetChart   WidgetType = "chart"
	WidgetTable   WidgetType = "table"
	WidgetMetric  WidgetType = "metric"
	WidgetText    WidgetType = "text"
	WidgetGauge   WidgetType = "gauge"
)

// Widget 仪表板组件
type Widget struct {
	ID       string
	Title    string
	Type     WidgetType
	Position Position
	Size     Size
	Config   map[string]interface{}
	Data     interface{}
}

// Position 位置
type Position struct {
	X int
	Y int
}

// Size 大小
type Size struct {
	Width  int
	Height int
}

// Dashboard 仪表板
type Dashboard struct {
	ID          string
	Name        string
	Description string
	Widgets     []*Widget
	CreatedAt   time.Time
	UpdatedAt   time.Time
	
	mu          sync.RWMutex
}

// NewDashboard 创建仪表板
func NewDashboard(id, name, description string) *Dashboard {
	now := time.Now()
	return &Dashboard{
		ID:          id,
		Name:        name,
		Description: description,
		Widgets:     make([]*Widget, 0),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// AddWidget 添加组件
func (d *Dashboard) AddWidget(widget *Widget) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Widgets = append(d.Widgets, widget)
	d.UpdatedAt = time.Now()
}

// RemoveWidget 移除组件
func (d *Dashboard) RemoveWidget(widgetID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	newWidgets := make([]*Widget, 0, len(d.Widgets)-1)
	for _, w := range d.Widgets {
		if w.ID != widgetID {
			newWidgets = append(newWidgets, w)
		}
	}
	d.Widgets = newWidgets
	d.UpdatedAt = time.Now()
}

// GetWidget 获取组件
func (d *Dashboard) GetWidget(id string) *Widget {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, w := range d.Widgets {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// GetWidgetCount 获取组件数量
func (d *Dashboard) GetWidgetCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.Widgets)
}

// DashboardManager 仪表板管理器
type DashboardManager struct {
	mu          sync.RWMutex
	dashboards  map[string]*Dashboard
	
	// 报表模板
	templates   map[string]*ReportTemplate
}

// ReportTemplate 报表模板
type ReportTemplate struct {
	ID          string
	Name        string
	Description string
	DataSources []string
	Filters     map[string]interface{}
	Columns     []string
	Format      string
}

// NewDashboardManager 创建仪表板管理器
func NewDashboardManager() *DashboardManager {
	return &DashboardManager{
		dashboards: make(map[string]*Dashboard),
		templates:  make(map[string]*ReportTemplate),
	}
}

// CreateDashboard 创建仪表板
func (dm *DashboardManager) CreateDashboard(id, name, description string) *Dashboard {
	d := NewDashboard(id, name, description)
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.dashboards[id] = d
	return d
}

// GetDashboard 获取仪表板
func (dm *DashboardManager) GetDashboard(id string) *Dashboard {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.dashboards[id]
}

// DeleteDashboard 删除仪表板
func (dm *DashboardManager) DeleteDashboard(id string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.dashboards, id)
}

// GetAllDashboards 获取所有仪表板
func (dm *DashboardManager) GetAllDashboards() []*Dashboard {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	result := make([]*Dashboard, 0, len(dm.dashboards))
	for _, d := range dm.dashboards {
		result = append(result, d)
	}
	return result
}

// RegisterReportTemplate 注册报表模板
func (dm *DashboardManager) RegisterReportTemplate(template *ReportTemplate) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.templates[template.ID] = template
}

// GetReportTemplate 获取报表模板
func (dm *DashboardManager) GetReportTemplate(id string) *ReportTemplate {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.templates[id]
}

// GetAllReportTemplates 获取所有报表模板
func (dm *DashboardManager) GetAllReportTemplates() []*ReportTemplate {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	result := make([]*ReportTemplate, 0, len(dm.templates))
	for _, t := range dm.templates {
		result = append(result, t)
	}
	return result
}

// GenerateReport 生成报表
func (dm *DashboardManager) GenerateReport(ctx context.Context, templateID string, params map[string]interface{}) (*Report, error) {
	dm.mu.RLock()
	template, ok := dm.templates[templateID]
	dm.mu.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("template %s not found", templateID)
	}
	
	report := &Report{
		ID:         fmt.Sprintf("report-%d", time.Now().Unix()),
		TemplateID: templateID,
		Name:       template.Name,
		GeneratedAt: time.Now(),
		Columns:    template.Columns,
		Data:       make([]map[string]interface{}, 0),
		Params:     params,
	}
	
	return report, nil
}

// Report 报表
type Report struct {
	ID          string
	TemplateID  string
	Name        string
	GeneratedAt time.Time
	Columns     []string
	Data        []map[string]interface{}
	Params      map[string]interface{}
}

// ============================================================================
// 6. 插件生命周期钩子
// ============================================================================

// LifecycleHook 生命周期钩子
type LifecycleHook int

const (
	HookBeforeInit LifecycleHook = iota
	HookAfterInit
	HookBeforeStart
	HookAfterStart
	HookBeforeStop
	HookAfterStop
)

// LifecycleCallback 生命周期回调
type LifecycleCallback func(plugin Plugin) error

// LifecycleManager 生命周期管理器
type LifecycleManager struct {
	mu       sync.RWMutex
	hooks    map[LifecycleHook][]LifecycleCallback
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager() *LifecycleManager {
	return &LifecycleManager{
		hooks: make(map[LifecycleHook][]LifecycleCallback),
	}
}

// RegisterHook 注册钩子
func (lm *LifecycleManager) RegisterHook(hook LifecycleHook, cb LifecycleCallback) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.hooks[hook] = append(lm.hooks[hook], cb)
}

// ExecuteHooks 执行钩子
func (lm *LifecycleManager) ExecuteHooks(hook LifecycleHook, p Plugin) error {
	lm.mu.RLock()
	callbacks := lm.hooks[hook]
	lm.mu.RUnlock()
	
	for _, cb := range callbacks {
		if err := cb(p); err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// 7. 二次开发文档与示例
// ============================================================================

// PluginDoc 插件文档
type PluginDoc struct {
	Name        string
	Type        PluginType
	Version     string
	Description string
	Author      string
	ConfigSchema map[string]interface{}
	Examples    []PluginExample
}

// PluginExample 插件示例
type PluginExample struct {
	Title   string
	Code    string
	Output  string
}

// DocManager 文档管理器
type DocManager struct {
	mu    sync.RWMutex
	docs  map[string]*PluginDoc
}

// NewDocManager 创建文档管理器
func NewDocManager() *DocManager {
	return &DocManager{
		docs: make(map[string]*PluginDoc),
	}
}

// RegisterDoc 注册文档
func (dm *DocManager) RegisterDoc(doc *PluginDoc) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.docs[doc.Name] = doc
}

// GetDoc 获取文档
func (dm *DocManager) GetDoc(name string) *PluginDoc {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.docs[name]
}

// GetAllDocs 获取所有文档
func (dm *DocManager) GetAllDocs() []*PluginDoc {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	result := make([]*PluginDoc, 0, len(dm.docs))
	for _, doc := range dm.docs {
		result = append(result, doc)
	}
	return result
}

// GetDocCount 获取文档数量
func (dm *DocManager) GetDocCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return len(dm.docs)
}

// GetDocCountByType 按类型获取文档数量
func (dm *DocManager) GetDocCountByType(pType PluginType) int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	count := 0
	for _, doc := range dm.docs {
		if doc.Type == pType {
			count++
		}
	}
	return count
}
