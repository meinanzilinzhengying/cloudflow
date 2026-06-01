package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestActionType_Constants 测试 ActionType 常量
func TestActionType_Constants(t *testing.T) {
	tests := []struct {
		name string
		act  ActionType
		want string
	}{
		{"Login", ActionLogin, "login"},
		{"Logout", ActionLogout, "logout"},
		{"Create", ActionCreate, "create"},
		{"Update", ActionUpdate, "update"},
		{"Delete", ActionDelete, "delete"},
		{"Query", ActionQuery, "query"},
		{"Config", ActionConfig, "config"},
		{"System", ActionSystem, "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.act) != tt.want {
				t.Errorf("ActionType = %q, want %q", tt.act, tt.want)
			}
		})
	}
}

// TestAuditEntry_JSONSerialization 测试审计日志条目的 JSON 序列化
func TestAuditEntry_JSONSerialization(t *testing.T) {
	entry := AuditEntry{
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Action:    ActionLogin,
		User:      "admin",
		Resource:  "system",
		Operation: "login",
		Status:    "success",
		Message:   "用户登录成功",
		IPAddress: "192.168.1.100",
		UserAgent: "Mozilla/5.0",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	// 验证关键字段存在
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	if result["action"] != "login" {
		t.Errorf("action = %v, want login", result["action"])
	}
	if result["user"] != "admin" {
		t.Errorf("user = %v, want admin", result["user"])
	}
	if result["status"] != "success" {
		t.Errorf("status = %v, want success", result["status"])
	}
	if result["ip_address"] != "192.168.1.100" {
		t.Errorf("ip_address = %v, want 192.168.1.100", result["ip_address"])
	}
}

// TestAuditEntry_JSONWithDetails 测试带详情的 JSON 序列化
func TestAuditEntry_JSONWithDetails(t *testing.T) {
	entry := AuditEntry{
		Timestamp: time.Now(),
		Action:    ActionUpdate,
		User:      "admin",
		Resource:  "rule-1",
		Operation: "update",
		Status:    "success",
		Message:   "更新告警规则",
		Details:   json.RawMessage(`{"old_threshold": 80, "new_threshold": 90}`),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON 反序列化失败: %v", err)
	}

	details, ok := result["details"].(map[string]interface{})
	if !ok {
		t.Fatal("details 字段缺失或类型错误")
	}
	if details["old_threshold"].(float64) != 80 {
		t.Errorf("old_threshold = %v, want 80", details["old_threshold"])
	}
	if details["new_threshold"].(float64) != 90 {
		t.Errorf("new_threshold = %v, want 90", details["new_threshold"])
	}
}

// TestNewLogger 测试创建审计日志记录器
func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}
	defer logger.Stop()

	if logger == nil {
		t.Fatal("NewLogger 返回 nil")
	}
	if logger.logDir != tmpDir {
		t.Errorf("logDir = %q, want %q", logger.logDir, tmpDir)
	}
}

// TestNewLogger_InvalidPath 测试无效路径
func TestNewLogger_InvalidPath(t *testing.T) {
	// 使用一个不可能创建的路径（在 Linux 中 /proc/immutable 不可写）
	_, err := NewLogger("/proc/nonexistent/impossible/path", nil)
	if err == nil {
		t.Error("期望返回错误，但得到了 nil")
	}
}

// TestLogger_Log 测试日志记录
func TestLogger_Log(t *testing.T) {
	tmpDir := t.TempDir()
	auditLogger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}
	defer auditLogger.Stop()

	err = auditLogger.Log(ActionLogin, "admin", "system", "login", "success", "登录成功", nil, "10.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Log 失败: %v", err)
	}

	// 验证日志文件存在
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}

	found := false
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".log" {
			found = true
			// 读取文件内容验证 JSON 格式
			content, err := os.ReadFile(filepath.Join(tmpDir, f.Name()))
			if err != nil {
				t.Fatalf("读取日志文件失败: %v", err)
			}
			var entry AuditEntry
			if err := json.Unmarshal(content, &entry); err != nil {
				t.Fatalf("日志内容不是有效的 JSON: %v\n内容: %s", err, string(content))
			}
			if entry.Action != ActionLogin {
				t.Errorf("日志 action = %q, want %q", entry.Action, ActionLogin)
			}
			if entry.User != "admin" {
				t.Errorf("日志 user = %q, want admin", entry.User)
			}
			break
		}
	}
	if !found {
		t.Error("未找到日志文件")
	}
}

// TestLogger_LogWithDetails 测试带详情的日志记录
func TestLogger_LogWithDetails(t *testing.T) {
	tmpDir := t.TempDir()
	auditLogger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}
	defer auditLogger.Stop()

	details := map[string]interface{}{
		"cpu_usage": 95.5,
		"host":      "server-1",
	}
	err = auditLogger.Log(ActionCreate, "admin", "alert-rule", "create", "success", "创建告警规则", details, "", "")
	if err != nil {
		t.Fatalf("Log 失败: %v", err)
	}
}

// TestLogger_Stop 测试停止日志记录器
func TestLogger_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	auditLogger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	// 第一次停止应成功
	auditLogger.Stop()

	// 第二次停止应是幂等的（不 panic）
	auditLogger.Stop()
}

// TestLogger_StopPreventsWrites 测试停止后拒绝写入
func TestLogger_StopPreventsWrites(t *testing.T) {
	tmpDir := t.TempDir()
	auditLogger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}

	auditLogger.Stop()

	err = auditLogger.Log(ActionLogin, "admin", "system", "login", "success", "应该被拒绝", nil, "", "")
	if err == nil {
		t.Error("停止后写入应返回错误")
	}
}

// TestLogger_ConcurrentLog 测试并发写入安全性
func TestLogger_ConcurrentLog(t *testing.T) {
	tmpDir := t.TempDir()
	auditLogger, err := NewLogger(tmpDir, nil)
	if err != nil {
		t.Fatalf("NewLogger 失败: %v", err)
	}
	defer auditLogger.Stop()

	done := make(chan struct{})
	const goroutines = 10
	const writesPerGoroutine = 50

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < writesPerGoroutine; j++ {
				_ = auditLogger.Log(ActionLogin, "admin", "system", "login", "success", "并发测试", nil, "", "")
			}
			done <- struct{}{}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
