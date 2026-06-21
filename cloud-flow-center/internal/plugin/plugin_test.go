//go:build linux

package plugin

import (
	"context"
	"errors"
	"testing"
)

func TestBasePlugin(t *testing.T) {
	bp := NewBasePlugin("p1", "Test Plugin", "1.0.0", PluginProtocolParser)
	
	if bp.ID() != "p1" {
		t.Errorf("expected ID p1, got %s", bp.ID())
	}
	if bp.Name() != "Test Plugin" {
		t.Errorf("expected name 'Test Plugin', got %s", bp.Name())
	}
	if bp.Version() != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", bp.Version())
	}
	if bp.Type() != PluginProtocolParser {
		t.Errorf("expected type protocol_parser, got %v", bp.Type())
	}
	
	// 健康检查
	healthy, msg := bp.Health()
	if healthy {
		t.Error("expected not healthy before start")
	}
	
	// 启动
	if err := bp.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	healthy, msg = bp.Health()
	if !healthy || msg != "running" {
		t.Errorf("expected healthy running, got %v %s", healthy, msg)
	}
	
	// 停止
	if err := bp.Stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	healthy, _ = bp.Health()
	if healthy {
		t.Error("expected not healthy after stop")
	}
	
	// 初始化配置
	if err := bp.Init(map[string]interface{}{"key": "value"}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
}

func TestPluginManagerRegister(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	bp := NewBasePlugin("p1", "Parser", "1.0", PluginProtocolParser)
	
	err := pm.Register(bp)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	
	if pm.GetPluginCount() != 1 {
		t.Errorf("expected 1 plugin, got %d", pm.GetPluginCount())
	}
	
	// 重复注册应失败
	err = pm.Register(bp)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestPluginManagerGetByType(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	pm.Register(NewBasePlugin("p1", "Parser1", "1.0", PluginProtocolParser))
	pm.Register(NewBasePlugin("p2", "Parser2", "1.0", PluginProtocolParser))
	pm.Register(NewBasePlugin("a1", "Alert1", "1.0", PluginAlertChannel))
	
	parsers := pm.GetPluginsByType(PluginProtocolParser)
	if len(parsers) != 2 {
		t.Errorf("expected 2 parsers, got %d", len(parsers))
	}
	
	alerts := pm.GetPluginsByType(PluginAlertChannel)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert plugin, got %d", len(alerts))
	}
	
	empty := pm.GetPluginsByType(PluginDashboard)
	if len(empty) != 0 {
		t.Errorf("expected 0 dashboard plugins, got %d", len(empty))
	}
}

func TestPluginManagerUnregister(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	bp := NewBasePlugin("p1", "Parser", "1.0", PluginProtocolParser)
	pm.Register(bp)
	pm.StartPlugin("p1")
	
	err := pm.Unregister("p1")
	if err != nil {
		t.Fatalf("unregister failed: %v", err)
	}
	
	if pm.GetPluginCount() != 0 {
		t.Errorf("expected 0 plugins, got %d", pm.GetPluginCount())
	}
	if pm.GetPluginCountByType(PluginProtocolParser) != 0 {
		t.Errorf("expected 0 parsers, got %d", pm.GetPluginCountByType(PluginProtocolParser))
	}
	
	// 注销不存在的插件
	err = pm.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for unregistering nonexistent plugin")
	}
}

func TestPluginManagerStartStop(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	bp := NewBasePlugin("p1", "Parser", "1.0", PluginProtocolParser)
	pm.Register(bp)
	
	if err := pm.StartPlugin("p1"); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	// 测试不存在的插件
	err := pm.StartPlugin("nonexistent")
	if err == nil {
		t.Error("expected error for starting nonexistent plugin")
	}
}

func TestExtensionPoint(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	
	// 注册扩展
	pm.RegisterExtension("pre_process", &mockExtension{
		name: "ext1",
		executeFn: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			val, _ := args["value"].(int)
			return val * 2, nil
		},
	})
	pm.RegisterExtension("pre_process", &mockExtension{
		name: "ext2",
		executeFn: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			val, _ := args["value"].(int)
			return val + 10, nil
		},
	})
	
	results, err := pm.ExecuteExtensions(context.Background(), "pre_process", map[string]interface{}{"value": 5})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	
	// 第一个扩展：5 * 2 = 10
	if results[0] != 10 {
		t.Errorf("expected result 10, got %v", results[0])
	}
	// 第二个扩展：5 + 10 = 15
	if results[1] != 15 {
		t.Errorf("expected result 15, got %v", results[1])
	}
	
	// 获取扩展点名称
	names := pm.GetExtensionPointNames()
	if len(names) != 1 || names[0] != "pre_process" {
		t.Errorf("unexpected extension points: %v", names)
	}
	
	// 执行不存在的扩展点
	empty, err := pm.ExecuteExtensions(context.Background(), "nonexistent", nil)
	if err != nil {
		t.Errorf("expected no error for nonexistent extension point, got %v", err)
	}
	if empty != nil {
		t.Errorf("expected nil results, got %v", empty)
	}
}

func TestExtensionPointError(t *testing.T) {
	pm := NewPluginManager("/tmp/plugins")
	pm.RegisterExtension("fail_point", &mockExtension{
		name: "fail_ext",
		executeFn: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return nil, errors.New("extension failed")
		},
	})
	
	_, err := pm.ExecuteExtensions(context.Background(), "fail_point", nil)
	if err == nil {
		t.Error("expected error from failing extension")
	}
}

func TestSDKManager(t *testing.T) {
	sm := NewSDKManager()
	
	sdk := &mockSDK{
		name:    "test-sdk",
		version: "1.0",
		apis: map[string]APIHandler{
			"hello": func(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error) {
				name, _ := req["name"].(string)
				return map[string]interface{}{"message": "hello " + name}, nil
			},
		},
	}
	
	err := sm.RegisterSDK(sdk)
	if err != nil {
		t.Fatalf("register SDK failed: %v", err)
	}
	
	// 重复注册
	err = sm.RegisterSDK(sdk)
	if err == nil {
		t.Error("expected error for duplicate SDK registration")
	}
	
	// 调用 API
	result, err := sm.CallAPI(context.Background(), "test-sdk", "hello", map[string]interface{}{"name": "world"})
	if err != nil {
		t.Fatalf("call API failed: %v", err)
	}
	if result["message"] != "hello world" {
		t.Errorf("unexpected result: %v", result)
	}
	
	// 调用不存在的 API
	_, err = sm.CallAPI(context.Background(), "test-sdk", "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent API")
	}
	
	// 获取已注册 SDK
	sdks := sm.GetRegisteredSDKs()
	if len(sdks) != 1 || sdks[0] != "test-sdk" {
		t.Errorf("unexpected SDKs: %v", sdks)
	}
	
	// 获取路由
	routes := sm.GetAPIRoutes()
	if len(routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(routes))
	}
	
	// 注销 SDK
	sm.UnregisterSDK("test-sdk")
	if len(sm.GetRegisteredSDKs()) != 0 {
		t.Error("expected 0 SDKs after unregister")
	}
}

func TestDashboardManager(t *testing.T) {
	dm := NewDashboardManager()
	
	// 创建仪表板
	dash := dm.CreateDashboard("dash-1", "Main Dashboard", "Main system dashboard")
	if dash == nil {
		t.Fatal("expected dashboard")
	}
	if dash.Name != "Main Dashboard" {
		t.Errorf("unexpected name: %s", dash.Name)
	}
	
	// 添加组件
	dash.AddWidget(&Widget{
		ID:    "w1",
		Title: "CPU Usage",
		Type:  WidgetGauge,
		Position: Position{X: 0, Y: 0},
		Size:     Size{Width: 2, Height: 2},
	})
	dash.AddWidget(&Widget{
		ID:    "w2",
		Title: "Memory",
		Type:  WidgetChart,
		Position: Position{X: 2, Y: 0},
		Size:     Size{Width: 3, Height: 2},
	})
	
	if dash.GetWidgetCount() != 2 {
		t.Errorf("expected 2 widgets, got %d", dash.GetWidgetCount())
	}
	
	// 获取组件
	w1 := dash.GetWidget("w1")
	if w1 == nil || w1.Title != "CPU Usage" {
		t.Error("expected widget w1")
	}
	
	// 移除组件
	dash.RemoveWidget("w1")
	if dash.GetWidgetCount() != 1 {
		t.Errorf("expected 1 widget after removal, got %d", dash.GetWidgetCount())
	}
	if dash.GetWidget("w1") != nil {
		t.Error("expected w1 to be removed")
	}
	
	// 获取仪表板
	retrieved := dm.GetDashboard("dash-1")
	if retrieved == nil {
		t.Fatal("expected to retrieve dashboard")
	}
	
	// 获取所有仪表板
	all := dm.GetAllDashboards()
	if len(all) != 1 {
		t.Errorf("expected 1 dashboard, got %d", len(all))
	}
	
	// 删除仪表板
	dm.DeleteDashboard("dash-1")
	if dm.GetDashboard("dash-1") != nil {
		t.Error("expected dashboard to be deleted")
	}
}

func TestDashboardManagerReportTemplate(t *testing.T) {
	dm := NewDashboardManager()
	
	template := &ReportTemplate{
		ID:          "tpl-1",
		Name:        "Daily Report",
		Description: "Daily system report",
		DataSources: []string{"cpu", "memory"},
		Columns:     []string{"timestamp", "value"},
		Format:      "json",
	}
	
	dm.RegisterReportTemplate(template)
	
	retrieved := dm.GetReportTemplate("tpl-1")
	if retrieved == nil {
		t.Fatal("expected template")
	}
	if retrieved.Name != "Daily Report" {
		t.Errorf("unexpected name: %s", retrieved.Name)
	}
	
	all := dm.GetAllReportTemplates()
	if len(all) != 1 {
		t.Errorf("expected 1 template, got %d", len(all))
	}
	
	// 生成报表
	report, err := dm.GenerateReport(context.Background(), "tpl-1", map[string]interface{}{"date": "2024-01-01"})
	if err != nil {
		t.Fatalf("generate report failed: %v", err)
	}
	if report.TemplateID != "tpl-1" {
		t.Errorf("unexpected template ID: %s", report.TemplateID)
	}
	if len(report.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(report.Columns))
	}
	
	// 生成不存在的模板
	_, err = dm.GenerateReport(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestLifecycleManager(t *testing.T) {
	lm := NewLifecycleManager()
	
	bp := NewBasePlugin("p1", "Test", "1.0", PluginProtocolParser)
	
	// 注册钩子
	initCalled := false
	lm.RegisterHook(HookAfterInit, func(p Plugin) error {
		initCalled = true
		return nil
	})
	
	startCalled := false
	lm.RegisterHook(HookBeforeStart, func(p Plugin) error {
		startCalled = true
		return nil
	})
	
	// 执行钩子
	if err := lm.ExecuteHooks(HookAfterInit, bp); err != nil {
		t.Fatalf("execute hook failed: %v", err)
	}
	if !initCalled {
		t.Error("expected init hook to be called")
	}
	
	if err := lm.ExecuteHooks(HookBeforeStart, bp); err != nil {
		t.Fatalf("execute hook failed: %v", err)
	}
	if !startCalled {
		t.Error("expected start hook to be called")
	}
}

func TestLifecycleManagerError(t *testing.T) {
	lm := NewLifecycleManager()
	
	bp := NewBasePlugin("p1", "Test", "1.0", PluginProtocolParser)
	
	lm.RegisterHook(HookAfterInit, func(p Plugin) error {
		return errors.New("hook error")
	})
	
	err := lm.ExecuteHooks(HookAfterInit, bp)
	if err == nil {
		t.Error("expected error from failing hook")
	}
}

func TestDocManager(t *testing.T) {
	dm := NewDocManager()
	
	doc := &PluginDoc{
		Name:        "parser-guide",
		Type:        PluginProtocolParser,
		Version:     "1.0",
		Description: "How to write protocol parsers",
		Author:      "CloudFlow Team",
		Examples: []PluginExample{
			{Title: "Basic Parser", Code: "func Parse(data []byte) {}", Output: "parsed"},
		},
	}
	
	dm.RegisterDoc(doc)
	
	if dm.GetDocCount() != 1 {
		t.Errorf("expected 1 doc, got %d", dm.GetDocCount())
	}
	
	retrieved := dm.GetDoc("parser-guide")
	if retrieved == nil {
		t.Fatal("expected doc")
	}
	if retrieved.Description != "How to write protocol parsers" {
		t.Errorf("unexpected description: %s", retrieved.Description)
	}
	if len(retrieved.Examples) != 1 {
		t.Errorf("expected 1 example, got %d", len(retrieved.Examples))
	}
	
	all := dm.GetAllDocs()
	if len(all) != 1 {
		t.Errorf("expected 1 doc, got %d", len(all))
	}
	
	if dm.GetDocCountByType(PluginProtocolParser) != 1 {
		t.Errorf("expected 1 parser doc, got %d", dm.GetDocCountByType(PluginProtocolParser))
	}
	if dm.GetDocCountByType(PluginAlertChannel) != 0 {
		t.Errorf("expected 0 alert docs, got %d", dm.GetDocCountByType(PluginAlertChannel))
	}
}

// ============================================================================
// Mock 实现
// ============================================================================

type mockExtension struct {
	name      string
	executeFn func(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

func (m *mockExtension) Name() string { return m.name }
func (m *mockExtension) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	return m.executeFn(ctx, args)
}

type mockSDK struct {
	name    string
	version string
	apis    map[string]APIHandler
}

func (m *mockSDK) Name() string                { return m.name }
func (m *mockSDK) Version() string             { return m.version }
func (m *mockSDK) APIs() map[string]APIHandler { return m.apis }
