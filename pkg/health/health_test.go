package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{Healthy, "healthy"},
		{Degraded, "degraded"},
		{Unhealthy, "unhealthy"},
		{HealthStatus(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

func TestNewManager(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)
	assert.Empty(t, manager.checkers)
}

func TestManager_RegisterChecker(t *testing.T) {
	manager := NewManager()

	checker := &MockChecker{
		name:   "test",
		status: Healthy,
	}

	manager.RegisterChecker(checker)
	assert.Len(t, manager.checkers, 1)
}

func TestManager_Check(t *testing.T) {
	manager := NewManager()

	// 全部健康
	manager.RegisterChecker(&MockChecker{name: "db", status: Healthy})
	manager.RegisterChecker(&MockChecker{name: "redis", status: Healthy})

	result := manager.Check(context.Background())
	assert.Equal(t, Healthy, result.Status)
	assert.Len(t, result.Components, 2)
}

func TestManager_Check_Degraded(t *testing.T) {
	manager := NewManager()

	manager.RegisterChecker(&MockChecker{name: "db", status: Healthy})
	manager.RegisterChecker(&MockChecker{name: "redis", status: Degraded})

	result := manager.Check(context.Background())
	assert.Equal(t, Degraded, result.Status)
}

func TestManager_Check_Unhealthy(t *testing.T) {
	manager := NewManager()

	manager.RegisterChecker(&MockChecker{name: "db", status: Unhealthy})
	manager.RegisterChecker(&MockChecker{name: "redis", status: Healthy})

	result := manager.Check(context.Background())
	assert.Equal(t, Unhealthy, result.Status)
}

func TestManager_HTTPHandler(t *testing.T) {
	manager := NewManager()
	manager.RegisterChecker(&MockChecker{name: "db", status: Healthy})

	handler := manager.HTTPHandler()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
}

func TestManager_HTTPHandler_Unhealthy(t *testing.T) {
	manager := NewManager()
	manager.RegisterChecker(&MockChecker{name: "db", status: Unhealthy})

	handler := manager.HTTPHandler()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestComponentHealth_JSON(t *testing.T) {
	comp := ComponentHealth{
		Name:      "database",
		Status:    Healthy,
		LatencyMs: 5,
	}

	assert.Equal(t, "healthy", comp.Status.String())
	assert.Equal(t, int64(5), comp.LatencyMs)
}

func TestHealthResponse_Timestamp(t *testing.T) {
	manager := NewManager()
	result := manager.Check(context.Background())

	assert.False(t, result.Timestamp.IsZero())
}

func TestMockChecker(t *testing.T) {
	checker := &MockChecker{
		name:   "test",
		status: Healthy,
		latency: 10 * time.Millisecond,
	}

	assert.Equal(t, "test", checker.Name())

	result := checker.Check(context.Background())
	assert.Equal(t, Healthy, result.Status)
	assert.Equal(t, int64(10), result.LatencyMs)
}

func TestAggregateStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []HealthStatus
		expected HealthStatus
	}{
		{"all healthy", []HealthStatus{Healthy, Healthy}, Healthy},
		{"one degraded", []HealthStatus{Healthy, Degraded}, Degraded},
		{"one unhealthy", []HealthStatus{Healthy, Unhealthy}, Unhealthy},
		{"mixed", []HealthStatus{Healthy, Degraded, Unhealthy}, Unhealthy},
		{"empty", []HealthStatus{}, Healthy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager()
			for _, s := range tt.statuses {
				manager.RegisterChecker(&MockChecker{name: "check", status: s})
			}
			result := manager.Check(context.Background())
			assert.Equal(t, tt.expected, result.Status)
		})
	}
}

func TestContextCancel(t *testing.T) {
	manager := NewManager()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := manager.Check(ctx)
	// 即使context取消也应该返回结果
	assert.NotNil(t, result)
}

// MockChecker 模拟检查器
type MockChecker struct {
	name    string
	status  HealthStatus
	latency time.Duration
	err     string
}

func (m *MockChecker) Name() string {
	return m.name
}

func (m *MockChecker) Check(ctx context.Context) ComponentHealth {
	return ComponentHealth{
		Name:      m.name,
		Status:    m.status,
		Error:     m.err,
		LatencyMs: m.latency.Milliseconds(),
	}
}
