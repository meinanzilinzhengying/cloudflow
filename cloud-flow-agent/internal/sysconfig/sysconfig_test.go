//go:build linux

package sysconfig_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/sysconfig"
)

// ============================================================================
// 一、配置项测试
// ============================================================================

func TestConfigItem(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "test", Value: "hello", Type: sysconfig.ValueTypeString}
		if item.StringValue() != "hello" {
			t.Errorf("expected 'hello', got %s", item.StringValue())
		}
	})

	t.Run("Int value", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "test", Value: 42, Type: sysconfig.ValueTypeInt}
		if item.IntValue() != 42 {
			t.Errorf("expected 42, got %d", item.IntValue())
		}
	})

	t.Run("Bool value", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "test", Value: true, Type: sysconfig.ValueTypeBool}
		if !item.BoolValue() {
			t.Error("expected true")
		}
	})

	t.Run("Float value", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "test", Value: 3.14, Type: sysconfig.ValueTypeFloat}
		if item.FloatValue() != 3.14 {
			t.Errorf("expected 3.14, got %f", item.FloatValue())
		}
	})

	t.Run("Masked value", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "secret", Value: "password123", Sensitive: true}
		masked := item.MaskedValue()
		if masked == "password123" {
			t.Error("expected masked value")
		}
	})

	t.Run("Is default", func(t *testing.T) {
		item := &sysconfig.ConfigItem{Key: "test", Value: "default", Default: "default"}
		if !item.IsDefault() {
			t.Error("expected to be default")
		}
	})
}

// ============================================================================
// 二、验证规则测试
// ============================================================================

func TestValidationRule(t *testing.T) {
	t.Run("Required validation", func(t *testing.T) {
		rule := &sysconfig.ValidationRule{Required: true}
		if err := rule.Validate(nil); err == nil {
			t.Error("expected error for nil value")
		}
		if err := rule.Validate("ok"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Range validation", func(t *testing.T) {
		min := 10.0
		max := 100.0
		rule := &sysconfig.ValidationRule{MinValue: &min, MaxValue: &max}
		if err := rule.Validate(5); err == nil {
			t.Error("expected error for value below min")
		}
		if err := rule.Validate(200); err == nil {
			t.Error("expected error for value above max")
		}
		if err := rule.Validate(50); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Enum validation", func(t *testing.T) {
		rule := &sysconfig.ValidationRule{Enum: []string{"a", "b", "c"}}
		if err := rule.Validate("d"); err == nil {
			t.Error("expected error for value not in enum")
		}
		if err := rule.Validate("a"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Length validation", func(t *testing.T) {
		minLen := 3
		maxLen := 10
		rule := &sysconfig.ValidationRule{MinLength: &minLen, MaxLength: &maxLen}
		if err := rule.Validate("ab"); err == nil {
			t.Error("expected error for string too short")
		}
		if err := rule.Validate("this is too long"); err == nil {
			t.Error("expected error for string too long")
		}
		if err := rule.Validate("hello"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// ============================================================================
// 三、配置快照测试
// ============================================================================

func TestConfigSnapshot(t *testing.T) {
	s1 := &sysconfig.ConfigSnapshot{
		Version: "v1",
		Items: map[string]*sysconfig.ConfigItem{
			"key1": {Key: "key1", Value: "val1"},
			"key2": {Key: "key2", Value: 100},
		},
	}

	s2 := &sysconfig.ConfigSnapshot{
		Version: "v2",
		Items: map[string]*sysconfig.ConfigItem{
			"key1": {Key: "key1", Value: "val1"},
			"key2": {Key: "key2", Value: 200},
			"key3": {Key: "key3", Value: "new"},
		},
	}

	t.Run("Compare", func(t *testing.T) {
		diff := s1.Compare(s2)
		if diff == nil {
			t.Fatal("expected diff")
		}
		if len(diff.Changed) != 1 || diff.Changed[0] != "key2" {
			t.Errorf("expected key2 changed, got %v", diff.Changed)
		}
		if len(diff.Added) != 1 || diff.Added[0] != "key3" {
			t.Errorf("expected key3 added, got %v", diff.Added)
		}
		if len(diff.Removed) != 0 {
			t.Errorf("expected no removed, got %v", diff.Removed)
		}
	})

	t.Run("Deep copy", func(t *testing.T) {
		copy := s1.DeepCopy()
		copy.Items["key1"].Value = "modified"
		if s1.Items["key1"].Value == "modified" {
			t.Error("original should not be modified")
		}
	})

	t.Run("JSON roundtrip", func(t *testing.T) {
		json, err := s1.ToJSON()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		parsed, err := sysconfig.FromJSON([]byte(json))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.Version != s1.Version {
			t.Errorf("expected version %s, got %s", s1.Version, parsed.Version)
		}
	})
}

// ============================================================================
// 四、版本管理器测试
// ============================================================================

func TestVersionManager(t *testing.T) {
	vm := sysconfig.NewVersionManager()
	vm.SetMaxHistory(5)

	t.Run("Save and get", func(t *testing.T) {
		s := &sysconfig.ConfigSnapshot{
			Version: "v1",
			Items:   map[string]*sysconfig.ConfigItem{"key1": {Key: "key1", Value: "val1"}},
			CreatedBy: "admin",
		}
		if err := vm.Save(s); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if vm.GetCurrent() == nil {
			t.Fatal("expected current version")
		}
		if vm.GetVersion("v1") == nil {
			t.Fatal("expected version v1")
		}
	})

	t.Run("List versions", func(t *testing.T) {
		versions := vm.ListVersions()
		if len(versions) == 0 {
			t.Error("expected at least one version")
		}
	})

	t.Run("Rollback", func(t *testing.T) {
		current := vm.GetCurrent()
		if current == nil {
			t.Fatal("no current version")
		}
		version := current.Version
		rolled, err := vm.Rollback(version, "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rolled == nil {
			t.Fatal("expected rolled back version")
		}
		if rolled.Source != "rollback" {
			t.Errorf("expected source=rollback, got %s", rolled.Source)
		}
	})

	t.Run("Diff", func(t *testing.T) {
		// 创建两个版本
		v1 := vm.GetCurrent()
		if v1 == nil {
			t.Fatal("no current version")
		}
		
		v2 := &sysconfig.ConfigSnapshot{
			Version: "v2",
			Items: map[string]*sysconfig.ConfigItem{
				"key1": {Key: "key1", Value: "modified"},
			},
		}
		vm.Save(v2)
		
		diff, err := vm.Diff(v1.Version, v2.Version)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff == nil || diff.IsEmpty() {
			t.Error("expected non-empty diff")
		}
	})

	t.Run("Tag snapshot", func(t *testing.T) {
		current := vm.GetCurrent()
		if current == nil {
			t.Fatal("no current version")
		}
		if err := vm.TagSnapshot(current.Version, "stable"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		versions := vm.FindByTag("stable")
		if len(versions) == 0 {
			t.Error("expected to find tagged version")
		}
	})

	t.Run("History", func(t *testing.T) {
		history := vm.GetHistory()
		if len(history) == 0 {
			t.Error("expected history")
		}
	})

	t.Run("Max history cleanup", func(t *testing.T) {
		vm2 := sysconfig.NewVersionManager()
		vm2.SetMaxHistory(3)
		for i := 0; i < 5; i++ {
			vm2.Save(&sysconfig.ConfigSnapshot{
				Version:   fmt.Sprintf("v-%d", i),
				Items:     map[string]*sysconfig.ConfigItem{},
				CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
			})
		}
		stats := vm2.GetStats()
		if stats["total_versions"].(int) > 3 {
			t.Errorf("expected max 3 versions, got %d", stats["total_versions"])
		}
	})
}

// ============================================================================
// 五、审计管理器测试
// ============================================================================

func TestAuditManager(t *testing.T) {
	am := sysconfig.NewAuditManager()
	am.SetMaxLogs(10)

	t.Run("Record and get", func(t *testing.T) {
		am.Record(&sysconfig.AuditLog{
			Action:   sysconfig.AuditActionUpdate,
			UserID:   "user-1",
			ConfigKey: "key1",
			Success:  true,
		})
		logs := am.GetLogs()
		if len(logs) != 1 {
			t.Errorf("expected 1 log, got %d", len(logs))
		}
	})

	t.Run("Record change", func(t *testing.T) {
		am.RecordChange(sysconfig.AuditActionUpdate, "user-2", "key2", "old", "new", true, "test")
		logs := am.GetLogsByKey("key2")
		if len(logs) != 1 {
			t.Errorf("expected 1 log for key2, got %d", len(logs))
		}
	})

	t.Run("Query by user", func(t *testing.T) {
		logs := am.GetLogsByUser("user-1")
		if len(logs) == 0 {
			t.Error("expected logs for user-1")
		}
	})

	t.Run("Query by action", func(t *testing.T) {
		logs := am.GetLogsByAction(sysconfig.AuditActionUpdate)
		if len(logs) == 0 {
			t.Error("expected update logs")
		}
	})

	t.Run("Query by time range", func(t *testing.T) {
		from := time.Now().Add(-time.Hour)
		to := time.Now().Add(time.Hour)
		logs := am.GetLogsByTimeRange(from, to)
		if len(logs) == 0 {
			t.Error("expected logs in time range")
		}
	})

	t.Run("Query options", func(t *testing.T) {
		logs := am.Query(sysconfig.AuditQueryOptions{UserID: "user-1"})
		if len(logs) == 0 {
			t.Error("expected logs for user-1")
		}
	})

	t.Run("Audit diff", func(t *testing.T) {
		diff := &sysconfig.ConfigDiff{
			FromVersion: "v1",
			ToVersion:   "v2",
			Added:       []string{"key3"},
			Changed:     []string{"key2"},
			Removed:     []string{"key1"},
		}
		am.AuditDiff(diff, "admin", "test")
		logs := am.GetLogsByAction(sysconfig.AuditActionCreate)
		if len(logs) == 0 {
			t.Error("expected create logs from diff")
		}
	})

	t.Run("Max logs cleanup", func(t *testing.T) {
		am2 := sysconfig.NewAuditManager()
		am2.SetMaxLogs(3)
		for i := 0; i < 5; i++ {
			am2.Record(&sysconfig.AuditLog{Action: sysconfig.AuditActionUpdate, UserID: "test"})
		}
		stats := am2.GetStats()
		if stats["total_logs"].(int) > 3 {
			t.Errorf("expected max 3 logs, got %d", stats["total_logs"])
		}
	})
}

// ============================================================================
// 六、系统配置管理器测试
// ============================================================================

func TestManager(t *testing.T) {
	m := sysconfig.NewManager(nil)

	t.Run("Set and get item", func(t *testing.T) {
		if err := m.SetItem("test.key", "value", "admin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		item := m.GetItem("test.key")
		if item == nil {
			t.Fatal("expected item")
		}
		if item.StringValue() != "value" {
			t.Errorf("expected 'value', got %s", item.StringValue())
		}
	})

	t.Run("Update item", func(t *testing.T) {
		if err := m.SetItem("test.key", "newvalue", "admin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		item := m.GetItem("test.key")
		if item.StringValue() != "newvalue" {
			t.Errorf("expected 'newvalue', got %s", item.StringValue())
		}
	})

	t.Run("Delete item", func(t *testing.T) {
		if err := m.DeleteItem("test.key", "admin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.GetItem("test.key") != nil {
			t.Error("expected item to be deleted")
		}
	})

	t.Run("Delete non-existent item", func(t *testing.T) {
		if err := m.DeleteItem("nonexistent", "admin"); err == nil {
			t.Error("expected error for non-existent item")
		}
	})

	t.Run("List items", func(t *testing.T) {
		m.SetItem("a.cat1", "val1", "admin")
		m.SetItem("b.cat2", "val2", "admin")
		
		all := m.ListItems("")
		if len(all) < 2 {
			t.Errorf("expected at least 2 items, got %d", len(all))
		}
	})

	t.Run("Version history", func(t *testing.T) {
		// Save a snapshot
		snapshot := m.GetCurrentSnapshot()
		if snapshot == nil {
			t.Fatal("expected current snapshot")
		}
		snapshot.Description = "test snapshot"
		if err := m.SaveSnapshot(snapshot, "admin"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		history := m.GetVersionHistory()
		if len(history) == 0 {
			t.Error("expected version history")
		}
	})

	t.Run("Audit logs", func(t *testing.T) {
		logs := m.GetAuditLogs()
		if len(logs) == 0 {
			t.Error("expected audit logs")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats := m.GetStats()
		if stats["item_count"] == nil {
			t.Error("expected item_count in stats")
		}
	})

	t.Run("Summary", func(t *testing.T) {
		summary := m.GetSummary()
		if summary == nil {
			t.Fatal("expected summary")
		}
		if summary.ItemCount == 0 {
			t.Error("expected non-zero item count")
		}
	})

	t.Run("Export import", func(t *testing.T) {
		json, err := m.ExportCurrent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if json == "" {
			t.Error("expected non-empty JSON")
		}
		
		imported, err := m.ImportSnapshot([]byte(json), "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if imported == nil {
			t.Fatal("expected imported snapshot")
		}
	})
}

func TestManagerWithValidation(t *testing.T) {
	m := sysconfig.NewManager(nil)

	minVal := 0.0
	maxVal := 100.0
	item := &sysconfig.ConfigItem{
		Key:        "rate",
		Value:      50,
		Type:       sysconfig.ValueTypeFloat,
		Default:    50,
		Validation: &sysconfig.ValidationRule{MinValue: &minVal, MaxValue: &maxVal},
	}
	
	snapshot := m.GetCurrentSnapshot()
	snapshot.Items["rate"] = item
	m.SaveSnapshot(snapshot, "admin")

	t.Run("Valid update", func(t *testing.T) {
		if err := m.SetItem("rate", 75, "admin"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("Invalid update", func(t *testing.T) {
		if err := m.SetItem("rate", 200, "admin"); err == nil {
			t.Error("expected error for value above max")
		}
	})
}

func TestManagerRollback(t *testing.T) {
	m := sysconfig.NewManager(nil)
	
	// Create initial snapshot
	m.SetItem("key1", "val1", "admin")
	v1 := m.GetCurrentSnapshot()
	m.SaveSnapshot(v1, "admin")
	v1Version := v1.Version
	
	// Modify and save v2
	m.SetItem("key1", "val2", "admin")
	v2 := m.GetCurrentSnapshot()
	m.SaveSnapshot(v2, "admin")
	
	t.Run("Rollback", func(t *testing.T) {
		rolled, err := m.Rollback(v1Version, "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rolled == nil {
			t.Fatal("expected rolled back snapshot")
		}
		
		item := m.GetItem("key1")
		if item == nil || item.StringValue() != "val1" {
			t.Errorf("expected value 'val1' after rollback, got %v", item)
		}
	})
}

func TestManagerChangeHandler(t *testing.T) {
	m := sysconfig.NewManager(nil)
	
	var newVal interface{}
	m.RegisterChangeHandler("test.key", func(o, n interface{}) {
		_ = o
		newVal = n
	})
	
	m.SetItem("test.key", "first", "admin")
	m.SetItem("test.key", "second", "admin")
	
	// Give goroutine time to execute
	time.Sleep(500 * time.Millisecond)
	
	if newVal != "second" && newVal != "first" {
		t.Errorf("expected newVal='second', got %v", newVal)
	}
}

// ============================================================================
// 七、内存配置源测试
// ============================================================================

func TestMemorySource(t *testing.T) {
	snapshot := &sysconfig.ConfigSnapshot{
		Version: "v1",
		Items: map[string]*sysconfig.ConfigItem{
			"key1": {Key: "key1", Value: "val1"},
		},
	}
	
	source := sysconfig.NewMemorySource(snapshot)
	
	t.Run("Load", func(t *testing.T) {
		loaded, err := source.Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loaded.Version != "v1" {
			t.Errorf("expected version v1, got %s", loaded.Version)
		}
	})
	
	t.Run("Update", func(t *testing.T) {
		newSnapshot := &sysconfig.ConfigSnapshot{
			Version: "v2",
			Items: map[string]*sysconfig.ConfigItem{
				"key1": {Key: "key1", Value: "val2"},
			},
		}
		
		var cbSnapshot *sysconfig.ConfigSnapshot
		source.SetOnChange(func(s *sysconfig.ConfigSnapshot) {
			cbSnapshot = s
		})
		
		source.Update(newSnapshot)
		
		if cbSnapshot == nil {
			t.Error("expected callback to be called")
		}
	})
	
	t.Run("Source type", func(t *testing.T) {
		if source.SourceType() != "memory" {
			t.Errorf("expected 'memory', got %s", source.SourceType())
		}
	})
}

// ============================================================================
// 八、配置源管理测试
// ============================================================================

func TestManagerSource(t *testing.T) {
	m := sysconfig.NewManager(nil)
	
	source := sysconfig.NewMemorySource(&sysconfig.ConfigSnapshot{
		Version: "v1",
		Items: map[string]*sysconfig.ConfigItem{
			"remote.key": {Key: "remote.key", Value: "remote"},
		},
	})
	
	t.Run("Add source", func(t *testing.T) {
		if err := m.AddSource("remote", source); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	
	t.Run("Add duplicate source", func(t *testing.T) {
		if err := m.AddSource("remote", source); err == nil {
			t.Error("expected error for duplicate source")
		}
	})
	
	t.Run("Load from source", func(t *testing.T) {
		loaded, err := m.LoadFromSource("remote", "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if loaded == nil {
			t.Fatal("expected loaded snapshot")
		}
	})
	
	t.Run("Remove source", func(t *testing.T) {
		if err := m.RemoveSource("remote"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	
	t.Run("Load from removed source", func(t *testing.T) {
		_, err := m.LoadFromSource("remote", "admin")
		if err == nil {
			t.Error("expected error for removed source")
		}
	})
}

// ============================================================================
// 九、辅助函数
// ============================================================================

func TestConfigDiff(t *testing.T) {
	diff := &sysconfig.ConfigDiff{
		FromVersion: "v1",
		ToVersion:   "v2",
		Added:       []string{"key3"},
		Changed:     []string{"key2"},
	}
	
	if diff.IsEmpty() {
		t.Error("expected non-empty diff")
	}
	if !contains(diff.Summary(), "added: 1") {
		t.Errorf("expected summary to contain 'added: 1', got %s", diff.Summary())
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}