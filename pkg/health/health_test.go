package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{StatusHealthy, "healthy"},
		{StatusDegraded, "degraded"},
		{StatusUnhealthy, "unhealthy"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.String())
		})
	}
}

func TestComponentHealth_WithError(t *testing.T) {
	ch := ComponentHealth{
		Name:      "test",
		Status:    StatusHealthy,
		LatencyMs: 100,
	}

	err := assert.AnError
	result := ch.WithError(err)

	assert.Equal(t, StatusUnhealthy, result.Status)
	assert.Equal(t, err.Error(), result.Error)
}

func TestComponentHealth_WithLatency(t *testing.T) {
	ch := ComponentHealth{
		Name:   "test",
		Status: StatusHealthy,
	}

	latency := 150 * time.Millisecond
	result := ch.WithLatency(latency)

	assert.Equal(t, int64(150), result.LatencyMs)
}

func TestHealthCheckerRegistry(t *testing.T) {
	registry := NewRegistry()

	checker := &MockChecker{name: "test-checker"}
	registry.Register(checker)

	assert.Len(t, registry.checkers, 1)
}

func TestHealthCheckerRegistry_CheckAll(t *testing.T) {
	registry := NewRegistry()

	healthyChecker := &MockChecker{
		name:   "healthy",
		status: StatusHealthy,
	}
	unhealthyChecker := &MockChecker{
		name:   "unhealthy",
		status: StatusUnhealthy,
		err:    assert.AnError,
	}

	registry.Register(healthyChecker)
	registry.Register(unhealthyChecker)

	ctx := context.Background()
	response := registry.CheckAll(ctx)

	assert.Equal(t, StatusUnhealthy, response.Status)
	assert.Len(t, response.Components, 2)
	assert.WithinDuration(t, time.Now(), response.Timestamp, time.Second)
}

func TestHealthHandler(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&MockChecker{name: "test", status: StatusHealthy})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler := registry.Handler()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthy")
}

// MockChecker 模拟健康检查器
type MockChecker struct {
	name   string
	status HealthStatus
	err    error
}

func (m *MockChecker) Name() string {
	return m.name
}

func (m *MockChecker) Check(ctx context.Context) ComponentHealth {
	return ComponentHealth{
		Name:   m.name,
		Status: m.status,
		Error:  errorString(m.err),
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
