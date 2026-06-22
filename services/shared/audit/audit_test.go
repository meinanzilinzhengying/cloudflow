package audit

import (
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Event 测试
// ============================================================================

func TestNewEvent(t *testing.T) {
	e := NewEvent(ActionLogin, ActorUser, "user123")

	if e.EventID == "" {
		t.Error("expected EventID to be set")
	}
	if e.Action != ActionLogin {
		t.Errorf("expected Action=login, got %s", e.Action)
	}
	if e.ActorType != ActorUser {
		t.Errorf("expected ActorType=user, got %s", e.ActorType)
	}
	if e.ActorID != "user123" {
		t.Errorf("expected ActorID=user123, got %s", e.ActorID)
	}
	if e.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestEventWithResource(t *testing.T) {
	e := NewEvent(ActionUserCreate, ActorUser, "admin").
		WithResource(ResourceUser, "newuser")

	if e.ResourceType != ResourceUser {
		t.Errorf("expected ResourceType=user, got %s", e.ResourceType)
	}
	if e.ResourceID != "newuser" {
		t.Errorf("expected ResourceID=newuser, got %s", e.ResourceID)
	}
}

func TestEventWithTenant(t *testing.T) {
	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithTenant("tenant-abc")

	if e.TenantID != "tenant-abc" {
		t.Errorf("expected TenantID=tenant-abc, got %s", e.TenantID)
	}
}

func TestEventWithStatus(t *testing.T) {
	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithStatus(StatusFailure, "invalid credentials")

	if e.Status != StatusFailure {
		t.Errorf("expected Status=failure, got %s", e.Status)
	}
	if e.ErrorMessage != "invalid credentials" {
		t.Errorf("expected ErrorMessage=invalid credentials, got %s", e.ErrorMessage)
	}
}

func TestEventWithClient(t *testing.T) {
	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithClient("192.168.1.1", "Mozilla/5.0", "req-123")

	if e.ClientIP != "192.168.1.1" {
		t.Errorf("expected ClientIP=192.168.1.1, got %s", e.ClientIP)
	}
	if e.UserAgent != "Mozilla/5.0" {
		t.Errorf("expected UserAgent=Mozilla/5.0, got %s", e.UserAgent)
	}
	if e.RequestID != "req-123" {
		t.Errorf("expected RequestID=req-123, got %s", e.RequestID)
	}
}

func TestEventWithDetails(t *testing.T) {
	details := map[string]interface{}{
		"login_method": "password",
		"mfa_used":     true,
	}
	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithDetails(details)

	if e.Details == nil {
		t.Fatal("expected Details to be set")
	}
}

func TestEventToJSON(t *testing.T) {
	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithResource(ResourceToken, "token-123").
		WithTenant("tenant-1").
		WithStatus(StatusSuccess, "").
		WithClient("10.0.0.1", "TestAgent", "req-1")

	jsonStr, err := e.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	if !strings.Contains(jsonStr, `"action":"login"`) {
		t.Errorf("JSON missing action: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"actor_id":"user1"`) {
		t.Errorf("JSON missing actor_id: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"resource_type":"token"`) {
		t.Errorf("JSON missing resource_type: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"success"`) {
		t.Errorf("JSON missing status: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"client_ip":"10.0.0.1"`) {
		t.Errorf("JSON missing client_ip: %s", jsonStr)
	}
}

// ============================================================================
// Auditor 测试
// ============================================================================

func TestAuditor_Log(t *testing.T) {
	w := &testWriter{}
	a := NewAuditor(Config{
		Writer:      w,
		BufferSize:  100,
		LogToStdout: false,
	})
	defer a.Close()

	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithStatus(StatusSuccess, "")
	a.Log(e)

	// 等待异步写入
	time.Sleep(100 * time.Millisecond)

	if len(w.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(w.events))
	}
	if w.events[0].Action != ActionLogin {
		t.Errorf("expected Action=login, got %s", w.events[0].Action)
	}
}

func TestAuditor_LogSync(t *testing.T) {
	w := &testWriter{}
	a := NewAuditor(Config{
		Writer:      w,
		BufferSize:  100,
		LogToStdout: false,
	})
	defer a.Close()

	e := NewEvent(ActionUserDelete, ActorUser, "admin").
		WithResource(ResourceUser, "deleted-user").
		WithStatus(StatusSuccess, "")
	err := a.LogSync(e)
	if err != nil {
		t.Fatalf("LogSync failed: %v", err)
	}

	if len(w.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(w.events))
	}
	if w.events[0].ResourceID != "deleted-user" {
		t.Errorf("expected ResourceID=deleted-user, got %s", w.events[0].ResourceID)
	}
}

func TestAuditor_BufferOverflow(t *testing.T) {
	w := &testWriter{}
	a := NewAuditor(Config{
		Writer:      w,
		BufferSize:  2,
		LogToStdout: false,
	})
	defer a.Close()

	// 写入超过缓冲区大小的事件
	for i := 0; i < 10; i++ {
		a.Log(NewEvent(ActionLogin, ActorUser, "user"))
	}

	// 等待所有事件处理
	time.Sleep(200 * time.Millisecond)

	// 应该所有事件都被写入
	if len(w.events) != 10 {
		t.Errorf("expected 10 events, got %d", len(w.events))
	}
}

func TestAuditor_Close(t *testing.T) {
	w := &testWriter{}
	a := NewAuditor(Config{
		Writer:      w,
		BufferSize:  100,
		LogToStdout: false,
	})

	// 写入事件到缓冲
	for i := 0; i < 5; i++ {
		a.Log(NewEvent(ActionLogin, ActorUser, "user"))
	}

	// 关闭审计器，应该排空所有缓冲
	err := a.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if len(w.events) != 5 {
		t.Errorf("expected 5 events after close, got %d", len(w.events))
	}

	// 重复关闭应该安全
	err = a.Close()
	if err != nil {
		t.Fatalf("Second close failed: %v", err)
	}
}

// ============================================================================
// Writer 测试
// ============================================================================

func TestStdoutWriter(t *testing.T) {
	w := NewStdoutWriter()
	e := NewEvent(ActionLogin, ActorUser, "user1")
	err := w.Write(e)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestFileWriter(t *testing.T) {
	path := "/tmp/test_audit.log"
	_ = os.Remove(path)

	w, err := NewFileWriter(path)
	if err != nil {
		t.Fatalf("NewFileWriter failed: %v", err)
	}

	e := NewEvent(ActionLogin, ActorUser, "user1").
		WithStatus(StatusSuccess, "")
	err = w.Write(e)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	err = w.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read file failed: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected file to have content")
	}
	if !strings.Contains(string(data), `"action":"login"`) {
		t.Errorf("file content missing action: %s", string(data))
	}

	_ = os.Remove(path)
}

// ============================================================================
// 辅助函数测试
// ============================================================================

func TestExtractRequestID(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "req-abc-123")

	id := ExtractRequestID(req)
	if id != "req-abc-123" {
		t.Errorf("expected RequestID=req-abc-123, got %s", id)
	}
}

func TestExtractClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	ip := ExtractClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected IP=10.0.0.1, got %s", ip)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	ip2 := ExtractClientIP(req2)
	if ip2 != "1.2.3.4" {
		t.Errorf("expected IP=1.2.3.4, got %s", ip2)
	}

	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("X-Real-Ip", "2.3.4.5")
	ip3 := ExtractClientIP(req3)
	if ip3 != "2.3.4.5" {
		t.Errorf("expected IP=2.3.4.5, got %s", ip3)
	}
}

// ============================================================================
// 测试辅助类型
// ============================================================================

type testWriter struct {
	mu     sync.Mutex
	events []Event
}

func (w *testWriter) Write(event Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	return nil
}

func (w *testWriter) Close() error { return nil }
