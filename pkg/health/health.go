package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthStatus 健康状态
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

func (s HealthStatus) String() string {
	return string(s)
}

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Name      string       `json:"name"`
	Status    HealthStatus `json:"status"`
	Error     string       `json:"error,omitempty"`
	LatencyMs int64        `json:"latency_ms"`
}

// WithError 设置错误
func (ch ComponentHealth) WithError(err error) ComponentHealth {
	if err != nil {
		ch.Status = StatusUnhealthy
		ch.Error = err.Error()
	}
	return ch
}

// WithLatency 设置延迟
func (ch ComponentHealth) WithLatency(dur time.Duration) ComponentHealth {
	ch.LatencyMs = dur.Milliseconds()
	return ch
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status     HealthStatus      `json:"status"`
	Components []ComponentHealth `json:"components"`
	Timestamp  time.Time         `json:"timestamp"`
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) ComponentHealth
}

// Registry 健康检查注册表
type Registry struct {
	mu       sync.RWMutex
	checkers []HealthChecker
}

// NewRegistry 创建新的注册表
func NewRegistry() *Registry {
	return &Registry{}
}

// Register 注册健康检查器
func (r *Registry) Register(checker HealthChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers = append(r.checkers, checker)
}

// CheckAll 执行所有健康检查
func (r *Registry) CheckAll(ctx context.Context) HealthResponse {
	r.mu.RLock()
	checkers := make([]HealthChecker, len(r.checkers))
	copy(checkers, r.checkers)
	r.mu.RUnlock()

	components := make([]ComponentHealth, 0, len(checkers))
	overallStatus := StatusHealthy

	for _, checker := range checkers {
		start := time.Now()
		comp := checker.Check(ctx)
		comp = comp.WithLatency(time.Since(start))
		components = append(components, comp)

		if comp.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if comp.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	return HealthResponse{
		Status:     overallStatus,
		Components: components,
		Timestamp:  time.Now(),
	}
}

// Handler HTTP处理函数
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		response := r.CheckAll(ctx)

		w.Header().Set("Content-Type", "application/json")

		if response.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(response)
	}
}
