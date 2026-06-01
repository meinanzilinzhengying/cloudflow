package alerting

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDuration_JSONMarshal 测试 Duration 的 JSON 序列化
func TestDuration_JSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		d    Duration
		want string
	}{
		{"5 分钟", Duration{5 * time.Minute}, `"5m0s"`},
		{"1 小时", Duration{1 * time.Hour}, `"1h0m0s"`},
		{"30 秒", Duration{30 * time.Second}, `"30s"`},
		{"0", Duration{0}, `"0s"`},
		{"2 小时 30 分钟", Duration{2*time.Hour + 30*time.Minute}, `"2h30m0s"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.d)
			if err != nil {
				t.Fatalf("JSON 序列化失败: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("JSON = %s, want %s", string(data), tt.want)
			}
		})
	}
}

// TestDuration_JSONUnmarshal 测试 Duration 的 JSON 反序列化
func TestDuration_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		json string
		want time.Duration
	}{
		{"5 分钟", `"5m"`, 5 * time.Minute},
		{"1 小时", `"1h"`, 1 * time.Hour},
		{"30 秒", `"30s"`, 30 * time.Second},
		{"混合", `"1h30m"`, 1*time.Hour + 30*time.Minute},
		{"Go 格式", `"1h30m0s"`, 1*time.Hour + 30*time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			if err := json.Unmarshal([]byte(tt.json), &d); err != nil {
				t.Fatalf("JSON 反序列化失败: %v", err)
			}
			if d.Duration != tt.want {
				t.Errorf("Duration = %v, want %v", d.Duration, tt.want)
			}
		})
	}
}

// TestDuration_JSONRoundTrip 测试 JSON 序列化/反序列化往返
func TestDuration_JSONRoundTrip(t *testing.T) {
	original := Duration{2*time.Hour + 15*time.Minute + 30*time.Second}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded Duration
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if decoded.Duration != original.Duration {
		t.Errorf("往返失败: got %v, want %v", decoded.Duration, original.Duration)
	}
}

// TestRuleType_Constants 测试 RuleType 常量
func TestRuleType_Constants(t *testing.T) {
	tests := []struct {
		name string
		rt   RuleType
		want string
	}{
		{"MetricRule", RuleTypeMetric, "metric"},
		{"TraceRule", RuleTypeTrace, "trace"},
		{"ProfilingRule", RuleTypeProfiling, "profiling"},
		{"SystemRule", RuleTypeSystem, "system"},
		{"CustomRule", RuleTypeCustom, "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.rt) != tt.want {
				t.Errorf("RuleType = %q, want %q", tt.rt, tt.want)
			}
		})
	}
}

// TestConditionOperator_Constants 测试 ConditionOperator 常量
func TestConditionOperator_Constants(t *testing.T) {
	tests := []struct {
		name string
		op   ConditionOperator
		want string
	}{
		{"GreaterThan", OpGreaterThan, ">"},
		{"LessThan", OpLessThan, "<"},
		{"Equal", OpEqual, "=="},
		{"NotEqual", OpNotEqual, "!="},
		{"GreaterThanOrEqual", OpGreaterThanOrEqual, ">="},
		{"LessThanOrEqual", OpLessThanOrEqual, "<="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.op) != tt.want {
				t.Errorf("ConditionOperator = %q, want %q", tt.op, tt.want)
			}
		})
	}
}

// TestRule_JSONSerialization 测试 Rule 的 JSON 序列化
func TestRule_JSONSerialization(t *testing.T) {
	rule := Rule{
		ID:          "rule-001",
		Name:        "CPU 使用率告警",
		Type:        RuleTypeMetric,
		Enabled:     true,
		Severity:    "warning",
		Description: "CPU 使用率超过 90%",
		Conditions: []Condition{
			{
				Metric:   "cpu_usage",
				Operator: OpGreaterThan,
				Threshold: 90.0,
				Duration: Duration{5 * time.Minute},
			},
		},
		Labels: map[string]string{
			"team":    "platform",
			"service": "compute",
		},
		CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if result["id"] != "rule-001" {
		t.Errorf("id = %v, want rule-001", result["id"])
	}
	if result["name"] != "CPU 使用率告警" {
		t.Errorf("name = %v, want CPU 使用率告警", result["name"])
	}
	if result["type"] != "metric" {
		t.Errorf("type = %v, want metric", result["type"])
	}
	if result["enabled"] != true {
		t.Errorf("enabled = %v, want true", result["enabled"])
	}
}

// TestRule_JSONRoundTrip 测试 Rule 的 JSON 往返
func TestRule_JSONRoundTrip(t *testing.T) {
	original := Rule{
		ID:        "rule-002",
		Name:      "内存告警",
		Type:      RuleTypeSystem,
		Enabled:   true,
		Severity:  "critical",
		Conditions: []Condition{
			{
				Metric:   "memory_usage",
				Operator: OpGreaterThan,
				Threshold: 95.0,
				Duration: Duration{10 * time.Minute},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded Rule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if len(decoded.Conditions) != len(original.Conditions) {
		t.Errorf("Conditions 长度 = %d, want %d", len(decoded.Conditions), len(original.Conditions))
	}
	if decoded.Conditions[0].Threshold != original.Conditions[0].Threshold {
		t.Errorf("Threshold = %v, want %v", decoded.Conditions[0].Threshold, original.Conditions[0].Threshold)
	}
	if decoded.Conditions[0].Duration.Duration != original.Conditions[0].Duration.Duration {
		t.Errorf("Duration = %v, want %v", decoded.Conditions[0].Duration.Duration, original.Conditions[0].Duration.Duration)
	}
}

// TestDefaultRules 测试默认规则
func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Error("DefaultRules 不应返回空列表")
	}

	for i, rule := range rules {
		if rule.ID == "" {
			t.Errorf("规则 %d 缺少 ID", i)
		}
		if rule.Name == "" {
			t.Errorf("规则 %d 缺少名称", i)
		}
		if rule.Type == "" {
			t.Errorf("规则 %d 缺少类型", i)
		}
		if len(rule.Conditions) == 0 {
			t.Errorf("规则 %d 缺少条件", i)
		}
	}
}

// TestNewRuleManager 测试创建规则管理器
func TestNewRuleManager(t *testing.T) {
	tmpDir := t.TempDir()
	rm := NewRuleManager(tmpDir)
	if rm == nil {
		t.Fatal("NewRuleManager 返回 nil")
	}
}

// TestRuleManager_CRUD 测试规则的增删改查
func TestRuleManager_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	rm := NewRuleManager(tmpDir)

	// 1. 创建规则
	rule := Rule{
		Name:      "测试规则",
		Type:      RuleTypeMetric,
		Enabled:   true,
		Severity:  "warning",
		Conditions: []Condition{
			{
				Metric:    "cpu_usage",
				Operator:  OpGreaterThan,
				Threshold: 80.0,
				Duration:  Duration{5 * time.Minute},
			},
		},
	}

	created, err := rm.SaveRule(rule)
	if err != nil {
		t.Fatalf("SaveRule 失败: %v", err)
	}
	if created.ID == "" {
		t.Error("创建的规则缺少 ID")
	}

	// 2. 获取规则
	retrieved, err := rm.GetRuleByID(created.ID)
	if err != nil {
		t.Fatalf("GetRuleByID 失败: %v", err)
	}
	if retrieved.Name != "测试规则" {
		t.Errorf("Name = %q, want 测试规则", retrieved.Name)
	}
	if retrieved.Conditions[0].Threshold != 80.0 {
		t.Errorf("Threshold = %v, want 80.0", retrieved.Conditions[0].Threshold)
	}

	// 3. 获取所有规则
	rules, err := rm.GetRules()
	if err != nil {
		t.Fatalf("GetRules 失败: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("规则数量 = %d, want 1", len(rules))
	}

	// 4. 更新规则
	updated := retrieved
	updated.Name = "更新后的规则"
	updated.Conditions[0].Threshold = 90.0
	_, err = rm.SaveRule(updated)
	if err != nil {
		t.Fatalf("更新规则失败: %v", err)
	}

	// 验证更新
	retrieved2, _ := rm.GetRuleByID(created.ID)
	if retrieved2.Name != "更新后的规则" {
		t.Errorf("更新后 Name = %q, want 更新后的规则", retrieved2.Name)
	}
	if retrieved2.Conditions[0].Threshold != 90.0 {
		t.Errorf("更新后 Threshold = %v, want 90.0", retrieved2.Conditions[0].Threshold)
	}

	// 5. 删除规则
	err = rm.DeleteRule(created.ID)
	if err != nil {
		t.Fatalf("DeleteRule 失败: %v", err)
	}

	// 验证删除
	rules2, _ := rm.GetRules()
	if len(rules2) != 0 {
		t.Errorf("删除后规则数量 = %d, want 0", len(rules2))
	}
}

// TestRuleManager_GetRuleByID_NotFound 测试获取不存在的规则
func TestRuleManager_GetRuleByID_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	rm := NewRuleManager(tmpDir)

	_, err := rm.GetRuleByID("non-existent-id")
	if err == nil {
		t.Error("获取不存在的规则应返回错误")
	}
}

// TestRuleManager_DeleteRule_NotFound 测试删除不存在的规则
func TestRuleManager_DeleteRule_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	rm := NewRuleManager(tmpDir)

	err := rm.DeleteRule("non-existent-id")
	if err == nil {
		t.Error("删除不存在的规则应返回错误")
	}
}

// TestRuleManager_Persistence 测试规则持久化
func TestRuleManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建第一个 RuleManager 并保存规则
	rm1 := NewRuleManager(tmpDir)
	rule := Rule{
		Name:      "持久化测试",
		Type:      RuleTypeMetric,
		Enabled:   true,
		Severity:  "info",
		Conditions: []Condition{
			{Metric: "disk_usage", Operator: OpGreaterThan, Threshold: 85.0},
		},
	}
	created, err := rm1.SaveRule(rule)
	if err != nil {
		t.Fatalf("SaveRule 失败: %v", err)
	}

	// 创建第二个 RuleManager（模拟重启）
	rm2 := NewRuleManager(tmpDir)
	rules, err := rm2.GetRules()
	if err != nil {
		t.Fatalf("GetRules 失败: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("规则数量 = %d, want 1", len(rules))
	}
	if rules[0].Name != "持久化测试" {
		t.Errorf("Name = %q, want 持久化测试", rules[0].Name)
	}
	if rules[0].ID != created.ID {
		t.Errorf("ID 不一致: got %q, want %q", rules[0].ID, created.ID)
	}
}

// TestRuleManager_MultipleRules 测试多规则管理
func TestRuleManager_MultipleRules(t *testing.T) {
	tmpDir := t.TempDir()
	rm := NewRuleManager(tmpDir)

	// 创建多个规则
	for i := 0; i < 5; i++ {
		rule := Rule{
			Name:      "规则-" + string(rune('A'+i)),
			Type:      RuleTypeMetric,
			Enabled:   true,
			Severity:  "warning",
			Conditions: []Condition{
				{Metric: "metric_" + string(rune('A'+i)), Operator: OpGreaterThan, Threshold: float64(70 + i*5)},
			},
		}
		_, err := rm.SaveRule(rule)
		if err != nil {
			t.Fatalf("SaveRule %d 失败: %v", i, err)
		}
	}

	rules, err := rm.GetRules()
	if err != nil {
		t.Fatalf("GetRules 失败: %v", err)
	}
	if len(rules) != 5 {
		t.Errorf("规则数量 = %d, want 5", len(rules))
	}
}
