// Package slo 提供服务等级目标（SLO）监控和错误预算计算功能
// P22: 定义 SLI/SLO 指标，支持错误预算管理
package slo

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/meinanzilinzhengying/cloudflow/edge/pkg/logger"
)

// SLO 类型常量
const (
	TypeAvailability = "availability" // 可用性
	TypeLatency      = "latency"      // 延迟
	TypeErrorRate    = "error_rate"   // 错误率
	TypeThroughput   = "throughput"   // 吞吐量
)

// SLO 定义
type SLO struct {
	ID          string        // SLO 唯一标识
	Name        string        // SLO 名称
	Type        string        // SLO 类型
	Target      float64       // 目标值 (0.0-1.0)
	Window      time.Duration // 测量窗口
	Description string        // 描述
}

// SLI 测量值
type SLIMeasurement struct {
	SLOID     string    // 对应 SLO ID
	Value     float64   // 当前测量值
	Timestamp time.Time // 测量时间
	Valid     bool      // 是否有效
}

// ErrorBudget 错误预算
type ErrorBudget struct {
	SLOID          string        // 对应 SLO ID
	Target         float64       // SLO 目标
	TotalBudget    float64       // 总错误预算（比率，如 0.001 = 0.1%）
	Consumed       float64       // 已消耗的错误预算
	Remaining      float64       // 剩余错误预算
	WindowStart    time.Time     // 测量窗口开始时间
	WindowEnd      time.Time     // 测量窗口结束时间
	ConsumptionRate float64      // 消耗速率（每小时）
	Status         BudgetStatus  // 预算状态
}

// BudgetStatus 错误预算状态
type BudgetStatus int

const (
	BudgetHealthy  BudgetStatus = iota // 🟢 健康
	BudgetCaution                      // 🟡 注意
	BudgetWarning                      // 🟠 警告
	BudgetCritical                     // 🔴 危险
	BudgetExceeded                     // 🔴 超标
)

func (s BudgetStatus) String() string {
	switch s {
	case BudgetHealthy:
		return "healthy"
	case BudgetCaution:
		return "caution"
	case BudgetWarning:
		return "warning"
	case BudgetCritical:
		return "critical"
	case BudgetExceeded:
		return "exceeded"
	default:
		return "unknown"
	}
}

// Manager SLO 管理器
type Manager struct {
	slos      map[string]*SLO
	budgets   map[string]*ErrorBudget
	measurements []SLIMeasurement
	mu        sync.RWMutex
	logger    *logger.Logger

	// Prometheus 指标
	sloComplianceGauge   *prometheus.GaugeVec   // SLO 合规率
	errorBudgetGauge     *prometheus.GaugeVec   // 错误预算剩余
	sloBurnRateGauge     *prometheus.GaugeVec   // 错误预算消耗速率
}

// NewManager 创建 SLO 管理器
func NewManager(log *logger.Logger) *Manager {
	m := &Manager{
		slos:         make(map[string]*SLO),
		budgets:      make(map[string]*ErrorBudget),
		measurements: make([]SLIMeasurement, 0),
		logger:       log,
	}

	// 注册 Prometheus 指标
	m.sloComplianceGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cloud_flow_slo_compliance_ratio",
		Help: "Current SLO compliance ratio (0-1)",
	}, []string{"slo_id", "slo_name"})

	m.errorBudgetGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cloud_flow_slo_error_budget_remaining_ratio",
		Help: "Remaining error budget ratio (0-1)",
	}, []string{"slo_id", "slo_name"})

	m.sloBurnRateGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cloud_flow_slo_burn_rate",
		Help: "Error budget burn rate (consumption per hour)",
	}, []string{"slo_id", "slo_name"})

	// 注册默认 SLO
	m.registerDefaultSLOs()

	return m
}

// registerDefaultSLOs 注册 CloudFlow 默认 SLO
func (m *Manager) registerDefaultSLOs() {
	defaults := []SLO{
		{
			ID:          "SLO-EDGE-001",
			Name:        "edge-availability",
			Type:        TypeAvailability,
			Target:      0.999, // 99.9%
			Window:      30 * 24 * time.Hour,
			Description: "Edge 服务可用性 ≥ 99.9%",
		},
		{
			ID:          "SLO-EDGE-002",
			Name:        "edge-latency-p99",
			Type:        TypeLatency,
			Target:      0.99, // 99% 的请求在 500ms 内
			Window:      time.Hour,
			Description: "Edge P99 转发延迟 ≤ 500ms",
		},
		{
			ID:          "SLO-EDGE-003",
			Name:        "edge-buffer-drop",
			Type:        TypeErrorRate,
			Target:      0.999, // 丢弃率 ≤ 0.1%
			Window:      30 * 24 * time.Hour,
			Description: "Edge 缓冲区丢弃率 ≤ 0.1%",
		},
		{
			ID:          "SLO-EDGE-004",
			Name:        "edge-memory-usage",
			Type:        TypeErrorRate,
			Target:      0.80, // 内存使用率 ≤ 80%
			Window:      time.Hour,
			Description: "Edge 内存使用率 ≤ 80%",
		},
		{
			ID:          "SLO-AGENT-001",
			Name:        "agent-availability",
			Type:        TypeAvailability,
			Target:      0.995, // 99.5%
			Window:      30 * 24 * time.Hour,
			Description: "Agent 可用性 ≥ 99.5%",
		},
		{
			ID:          "SLO-AGENT-002",
			Name:        "agent-heartbeat-success",
			Type:        TypeAvailability,
			Target:      0.995, // 99.5%
			Window:      30 * 24 * time.Hour,
			Description: "Agent 心跳成功率 ≥ 99.5%",
		},
		{
			ID:          "SLO-AGENT-003",
			Name:        "agent-data-loss",
			Type:        TypeErrorRate,
			Target:      0.9999, // 数据丢失率 ≤ 0.01%
			Window:      30 * 24 * time.Hour,
			Description: "Agent 数据丢失率 ≤ 0.01%",
		},
	}

	for i := range defaults {
		m.RegisterSLO(&defaults[i])
	}

	m.logger.Info("[P22][SLO] 已注册默认 SLO 定义")
}

// RegisterSLO 注册 SLO
func (m *Manager) RegisterSLO(slo *SLO) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.slos[slo.ID] = slo

	// 初始化错误预算
	budget := &ErrorBudget{
		SLOID:       slo.ID,
		Target:      slo.Target,
		TotalBudget: 1.0 - slo.Target,
		Consumed:    0,
		Remaining:   1.0 - slo.Target,
		WindowStart: time.Now(),
		WindowEnd:   time.Now().Add(slo.Window),
		Status:      BudgetHealthy,
	}
	m.budgets[slo.ID] = budget

	m.logger.Infof("[P22][SLO] 注册 SLO: %s (目标: %.2f%%)", slo.ID, slo.Target*100)
}

// RecordMeasurement 记录 SLI 测量值
func (m *Manager) RecordMeasurement(sloID string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	measurement := SLIMeasurement{
		SLOID:     sloID,
		Value:     value,
		Timestamp: time.Now(),
		Valid:     true,
	}
	m.measurements = append(m.measurements, measurement)

	// 更新错误预算
	m.updateBudget(sloID, value)
}

// updateBudget 更新错误预算
func (m *Manager) updateBudget(sloID string, value float64) {
	budget, ok := m.budgets[sloID]
	if !ok {
		return
	}

	slo, ok := m.slos[sloID]
	if !ok {
		return
	}

	// 计算错误消耗
	var errorDelta float64
	switch slo.Type {
	case TypeAvailability:
		// 可用性 = 1 - error_rate
		// error_rate = 1 - value
		if value < slo.Target {
			errorDelta = slo.Target - value
		}
	case TypeLatency:
		// 延迟：超过目标值视为错误
		if value > 0.5 { // 500ms
			errorDelta = 0.01 // 每次超延迟记为 1% 错误
		}
	case TypeErrorRate:
		// 错误率直接累加
		if value > (1.0 - slo.Target) {
			errorDelta = value - (1.0 - slo.Target)
		}
	}

	if errorDelta > 0 {
		budget.Consumed += errorDelta
		budget.Remaining = math.Max(0, budget.TotalBudget-budget.Consumed)
	}

	// 计算消耗速率（每小时）
	elapsed := time.Since(budget.WindowStart).Hours()
	if elapsed > 0 {
		budget.ConsumptionRate = budget.Consumed / elapsed
	}

	// 更新状态
	budget.Status = m.calculateBudgetStatus(budget)

	// 更新 Prometheus 指标
	m.sloComplianceGauge.WithLabelValues(sloID, slo.Name).Set(value)
	m.errorBudgetGauge.WithLabelValues(sloID, slo.Name).Set(budget.Remaining / budget.TotalBudget)
	m.sloBurnRateGauge.WithLabelValues(sloID, slo.Name).Set(budget.ConsumptionRate)
}

// calculateBudgetStatus 计算错误预算状态
func (m *Manager) calculateBudgetStatus(budget *ErrorBudget) BudgetStatus {
	if budget.Consumed >= budget.TotalBudget {
		return BudgetExceeded
	}

	consumedRatio := budget.Consumed / budget.TotalBudget
	elapsedRatio := time.Since(budget.WindowStart).Hours() / budget.WindowEnd.Sub(budget.WindowStart).Hours()

	// 基于消耗速率和时间比例判断状态
	if consumedRatio > 0.75 {
		return BudgetCritical
	}
	if consumedRatio > 0.50 {
		return BudgetWarning
	}
	if consumedRatio > 0.25 {
		return BudgetCaution
	}
	// 如果消耗比例远超时间比例，提前警告
	if elapsedRatio > 0 && consumedRatio/elapsedRatio > 2.0 {
		return BudgetCaution
	}
	return BudgetHealthy
}

// GetBudgetStatus 获取错误预算状态
func (m *Manager) GetBudgetStatus(sloID string) *ErrorBudget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.budgets[sloID]
}

// GetAllBudgets 获取所有错误预算状态
func (m *Manager) GetAllBudgets() map[string]*ErrorBudget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ErrorBudget)
	for k, v := range m.budgets {
		result[k] = v
	}
	return result
}

// GetSLO 获取 SLO 定义
func (m *Manager) GetSLO(sloID string) *SLO {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.slos[sloID]
}

// ResetBudget 重置错误预算（通常在新窗口开始时调用）
func (m *Manager) ResetBudget(sloID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	budget, ok := m.budgets[sloID]
	if !ok {
		return
	}

	budget.Consumed = 0
	budget.Remaining = budget.TotalBudget
	budget.WindowStart = time.Now()
	budget.WindowEnd = time.Now().Add(budget.WindowEnd.Sub(budget.WindowStart))
	budget.Status = BudgetHealthy

	m.logger.Infof("[P22][SLO] 重置错误预算: %s", sloID)
}

// Collectors 返回 Prometheus 指标收集器
func (m *Manager) Collectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.sloComplianceGauge,
		m.errorBudgetGauge,
		m.sloBurnRateGauge,
	}
}

// IsSLOBreached 检查 SLO 是否已达成
func (m *Manager) IsSLOBreached(sloID string) bool {
	budget := m.GetBudgetStatus(sloID)
	if budget == nil {
		return false
	}
	return budget.Status >= BudgetExceeded
}

// String 返回错误预算的字符串表示
func (b *ErrorBudget) String() string {
	return fmt.Sprintf(
		"SLO=%s target=%.4f total=%.4f consumed=%.4f remaining=%.4f rate=%.4f/h status=%s window=[%s ~ %s]",
		b.SLOID, b.Target, b.TotalBudget, b.Consumed, b.Remaining,
		b.ConsumptionRate, b.Status.String(),
		b.WindowStart.Format("2006-01-02 15:04"),
		b.WindowEnd.Format("2006-01-02 15:04"),
	)
}
