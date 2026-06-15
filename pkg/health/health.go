// Package health 通用健康检查框架
//
// 提供组件级健康检查、/health HTTP端点、依赖状态监控
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// 健康状态定义
// ============================================================================

// HealthStatus 健康状态
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"   // 健康
	StatusDegraded  HealthStatus = "degraded"  // 降级（部分功能不可用）
	StatusUnhealthy HealthStatus = "unhealthy" // 不健康（核心功能不可用）
)

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Name      string       `json:"name"`
	Status    HealthStatus `json:"status"`
	Error     string       `json:"error,omitempty"`
	LatencyMs int64        `json:"latency_ms"`
	Timestamp time.Time    `json:"timestamp"`
}

// HealthResponse 完整健康检查响应
type HealthResponse struct {
	Status     HealthStatus      `json:"status"`
	Components []ComponentHealth `json:"components"`
	Timestamp  time.Time         `json:"timestamp"`
	Uptime     string            `json:"uptime"`
	Version    string            `json:"version"`
}

// ============================================================================
// 健康检查接口
// ============================================================================

// Checker 健康检查器接口
type Checker interface {
	// Name 返回组件名称
	Name() string

	// Check 执行健康检查
	Check(ctx context.Context) ComponentHealth
}

// ============================================================================
// 健康检查管理器
// ============================================================================

// Manager 健康检查管理器
type Manager struct {
	mu         sync.RWMutex
	checkers   map[string]Checker
	startTime  time.Time
	version    string
	cache      map[string]ComponentHealth
	cacheTTL   time.Duration
}

// NewManager 创建健康检查管理器
func NewManager(version string) *Manager {
	return &Manager{
		checkers:  make(map[string]Checker),
		startTime: time.Now(),
		version:   version,
		cache:     make(map[string]ComponentHealth),
		cacheTTL:  5 * time.Second,
	}
}

// RegisterChecker 注册健康检查器
func (m *Manager) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers[checker.Name()] = checker
}

// CheckAll 执行所有健康检查
func (m *Manager) CheckAll(ctx context.Context) HealthResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	components := make([]ComponentHealth, 0, len(m.checkers))
	overallStatus := StatusHealthy

	for _, checker := range m.checkers {
		component := checker.Check(ctx)
		components = append(components, component)

		// 更新整体状态
		switch component.Status {
		case StatusUnhealthy:
			overallStatus = StatusUnhealthy
		case StatusDegraded:
			if overallStatus == StatusHealthy {
				overallStatus = StatusDegraded
			}
		}
	}

	return HealthResponse{
		Status:     overallStatus,
		Components: components,
		Timestamp:  time.Now(),
		Uptime:     time.Since(m.startTime).String(),
		Version:    m.version,
	}
}

// HTTPHandler 返回HTTP处理函数
func (m *Manager) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		response := m.CheckAll(ctx)

		// 设置HTTP状态码
		switch response.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // 降级仍返回200
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable) // 不健康返回503
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// ============================================================================
// 内置检查器实现
// ============================================================================

// DatabaseChecker 数据库健康检查器
type DatabaseChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewDatabaseChecker(name string, checkFn func(ctx context.Context) error) *DatabaseChecker {
	return &DatabaseChecker{name: name, checkFn: checkFn}
}

func (d *DatabaseChecker) Name() string { return d.name }

func (d *DatabaseChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := d.checkFn(ctx)
	latency := time.Since(start).Milliseconds()

	status := StatusHealthy
	errMsg := ""
	if err != nil {
		status = StatusUnhealthy
		errMsg = err.Error()
	}

	return ComponentHealth{
		Name:      d.name,
		Status:    status,
		Error:     errMsg,
		LatencyMs: latency,
		Timestamp: time.Now(),
	}
}

// RedisChecker Redis健康检查器
type RedisChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewRedisChecker(name string, checkFn func(ctx context.Context) error) *RedisChecker {
	return &RedisChecker{name: name, checkFn: checkFn}
}

func (r *RedisChecker) Name() string { return r.name }

func (r *RedisChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := r.checkFn(ctx)
	latency := time.Since(start).Milliseconds()

	status := StatusHealthy
	errMsg := ""
	if err != nil {
		status = StatusUnhealthy
		errMsg = err.Error()
	}

	return ComponentHealth{
		Name:      r.name,
		Status:    status,
		Error:     errMsg,
		LatencyMs: latency,
		Timestamp: time.Now(),
	}
}

// KafkaChecker Kafka健康检查器
type KafkaChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewKafkaChecker(name string, checkFn func(ctx context.Context) error) *KafkaChecker {
	return &KafkaChecker{name: name, checkFn: checkFn}
}

func (k *KafkaChecker) Name() string { return k.name }

func (k *KafkaChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := k.checkFn(ctx)
	latency := time.Since(start).Milliseconds()

	status := StatusHealthy
	errMsg := ""
	if err != nil {
		status = StatusDegraded // Kafka降级不影响核心功能
		errMsg = err.Error()
	}

	return ComponentHealth{
		Name:      k.name,
		Status:    status,
		Error:     errMsg,
		LatencyMs: latency,
		Timestamp: time.Now(),
	}
}

// EtcdChecker etcd健康检查器
type EtcdChecker struct {
	name    string
	checkFn func(ctx context.Context) error
}

func NewEtcdChecker(name string, checkFn func(ctx context.Context) error) *EtcdChecker {
	return &EtcdChecker{name: name, checkFn: checkFn}
}

func (e *EtcdChecker) Name() string { return e.name }

func (e *EtcdChecker) Check(ctx context.Context) ComponentHealth {
	start := time.Now()
	err := e.checkFn(ctx)
	latency := time.Since(start).Milliseconds()

	status := StatusHealthy
	errMsg := ""
	if err != nil {
		status = StatusUnhealthy
		errMsg = err.Error()
	}

	return ComponentHealth{
		Name:      e.name,
		Status:    status,
		Error:     errMsg,
		LatencyMs: latency,
		Timestamp: time.Now(),
	}
}

// DiskChecker 磁盘空间检查器
type DiskChecker struct {
	name      string
	path      string
	warnPct   float64
	critPct   float64
}

func NewDiskChecker(name, path string, warnPct, critPct float64) *DiskChecker {
	return &DiskChecker{
		name:    name,
		path:    path,
		warnPct: warnPct,
		critPct: critPct,
	}
}

func (d *DiskChecker) Name() string { return d.name }

func (d *DiskChecker) Check(ctx context.Context) ComponentHealth {
	// 简化实现：实际使用时需要调用syscall.Statfs
	status := StatusHealthy
	errMsg := ""

	return ComponentHealth{
		Name:      d.name,
		Status:    status,
		Error:     errMsg,
		LatencyMs: 1,
		Timestamp: time.Now(),
	}
}

// ============================================================================
// 便捷函数
// ============================================================================

// SimpleHealthHandler 简单健康检查处理器（仅返回状态）
func SimpleHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now(),
		})
	}
}

// LivenessHandler 存活检查处理器
func LivenessHandler() http.HandlerFunc {
	return SimpleHealthHandler()
}

// ReadinessHandler 就绪检查处理器
func ReadinessHandler() http.HandlerFunc {
	return SimpleHealthHandler()
}
