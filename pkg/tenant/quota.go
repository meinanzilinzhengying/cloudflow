// P25: 租户配额管理器 — 流量限制、存储限制、告警规则数限制
//
// 解决：租户配额管理缺失
// 提供：
//   - 配额检查（实时/延迟两种模式）
//   - 配额更新（内存缓存 + 数据库持久化）
//   - 超限处理策略（拒绝/降级/告警）
//   - 配额告警（Prometheus 指标）
//
package tenant

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ============================================================================
// 配额指标
// ============================================================================

var (
	// QuotaExceededTotal 配额超限次数
	QuotaExceededTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cloudflow_tenant_quota_exceeded_total",
			Help: "Total number of quota exceedance events",
		},
		[]string{"tenant_id", "quota_type"},
	)

	// QuotaUsage 配额使用率（Gauge，0.0~1.0）
	QuotaUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cloudflow_tenant_quota_usage",
			Help: "Current quota usage ratio (0.0~1.0)",
		},
		[]string{"tenant_id", "quota_type"},
	)

	// QuotaCheckDuration 配额检查耗时
	QuotaCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cloudflow_tenant_quota_check_duration_seconds",
			Help:    "Quota check duration in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"quota_type"},
	)
)

// ============================================================================
// 一、配额类型定义
// ============================================================================

// QuotaType 配额类型
type QuotaType string

const (
	QuotaTypeFlowsPerMin    QuotaType = "flows_per_min"
	QuotaTypeFlowsPerDay    QuotaType = "flows_per_day"
	QuotaTypeMetricsPerMin  QuotaType = "metrics_per_min"
	QuotaTypeStorageBytes   QuotaType = "storage_bytes"
	QuotaTypeAgentCount     QuotaType = "agent_count"
	QuotaTypeAlertRules     QuotaType = "alert_rules"
	QuotaTypeRetentionDays  QuotaType = "retention_days"
	QuotaTypeAPIRateLimit   QuotaType = "api_rate_limit"
	QuotaTypeQueryRateLimit QuotaType = "query_rate_limit"
	QuotaTypeUserCount      QuotaType = "user_count"
	QuotaTypeProjectCount   QuotaType = "project_count"
)

// String 返回配额类型的可读名称
func (qt QuotaType) String() string {
	return string(qt)
}

// ============================================================================
// 二、配额配置（QuotaConfig）
// ============================================================================

// QuotaConfig 定义租户配额配置
type QuotaConfig struct {
	TenantID string

	// 流量配额
	MaxFlowsPerMin   int64 // 每分钟最大流量数
	MaxFlowsPerDay   int64 // 每日最大流量数
	MaxMetricsPerMin int64 // 每分钟最大指标数

	// 存储配额
	MaxStorageBytes int64 // 最大存储字节数
	MaxStorageGB    int   // 最大存储 GB（便捷字段，内部转为 bytes）

	// 资源配额
	MaxAgentCount int // 最大 Agent 数量
	MaxAlertRules int // 最大告警规则数
	MaxUserCount  int // 最大用户数量
	MaxProjectCount int // 最大项目数量

	// 查询配额
	MaxAPIRateLimit   int // 每分钟 API 调用次数
	MaxQueryRateLimit int // 每分钟查询次数

	// 保留策略
	RetentionDays int // 数据保留天数

	// 更新时间和版本
	UpdatedAt int64
	Version   int64 // 乐观锁版本
}

// DefaultQuotaConfig 返回默认配额配置
func DefaultQuotaConfig() *QuotaConfig {
	return &QuotaConfig{
		MaxFlowsPerMin:    10000,
		MaxFlowsPerDay:    10_000_000,
		MaxMetricsPerMin:  50000,
		MaxStorageBytes:   100 * 1024 * 1024 * 1024, // 100GB
		MaxStorageGB:      100,
		MaxAgentCount:     100,
		MaxAlertRules:     100,
		MaxUserCount:      50,
		MaxProjectCount:   10,
		MaxAPIRateLimit:   1000,
		MaxQueryRateLimit: 500,
		RetentionDays:     30,
	}
}

// PlanQuotaConfig 定义不同套餐的默认配额
type PlanQuotaConfig struct {
	PlanName string
	Quota    *QuotaConfig
}

// PlanQuotas 预定义套餐配额
var PlanQuotas = map[string]*QuotaConfig{
	"free": {
		MaxFlowsPerMin:    1000,
		MaxFlowsPerDay:     1_000_000,
		MaxMetricsPerMin:  5000,
		MaxStorageBytes:   10 * 1024 * 1024 * 1024, // 10GB
		MaxAgentCount:     10,
		MaxAlertRules:     20,
		MaxUserCount:      5,
		MaxProjectCount:   2,
		MaxAPIRateLimit:   100,
		MaxQueryRateLimit: 50,
		RetentionDays:     7,
	},
	"pro": {
		MaxFlowsPerMin:    10000,
		MaxFlowsPerDay:    10_000_000,
		MaxMetricsPerMin:  50000,
		MaxStorageBytes:   100 * 1024 * 1024 * 1024, // 100GB
		MaxAgentCount:     100,
		MaxAlertRules:     100,
		MaxUserCount:      50,
		MaxProjectCount:   10,
		MaxAPIRateLimit:   1000,
		MaxQueryRateLimit: 500,
		RetentionDays:     30,
	},
	"enterprise": {
		MaxFlowsPerMin:    100000,
		MaxFlowsPerDay:    100_000_000,
		MaxMetricsPerMin:  500000,
		MaxStorageBytes:   1024 * 1024 * 1024 * 1024, // 1TB
		MaxAgentCount:     1000,
		MaxAlertRules:     1000,
		MaxUserCount:      500,
		MaxProjectCount:   100,
		MaxAPIRateLimit:   10000,
		MaxQueryRateLimit: 5000,
		RetentionDays:     90,
	},
}

// ============================================================================
// 三、配额管理器（QuotaManager）
// ============================================================================

// QuotaManager 租户配额管理器
type QuotaManager struct {
	// 配额配置存储（内存缓存 + 数据库持久化）
	configs   map[string]*QuotaConfig
	configsMu sync.RWMutex

	// 实时使用量（滑动窗口统计）
	usage map[string]*TenantUsage
	usageMu sync.RWMutex

	// 滑动窗口大小
	windowSize time.Duration

	// 超限处理策略
	overflowPolicy OverflowPolicy

	// 数据库持久化回调
	persistFunc PersistQuotaFunc

	// 配额检查模式
	checkMode QuotaCheckMode
}

// OverflowPolicy 超限处理策略
type OverflowPolicy int

const (
	// OverflowReject 拒绝请求（返回配额超限错误）
	OverflowReject OverflowPolicy = iota
	// OverflowThrottle 限流（降低速率，但不拒绝）
	OverflowThrottle
	// OverflowWarn 仅告警（记录日志，不拒绝）
	OverflowWarn
	// OverflowAutoUpgrade 自动升级（触发套餐升级提示）
	OverflowAutoUpgrade
)

// String 返回超限策略名称
func (p OverflowPolicy) String() string {
	switch p {
	case OverflowReject:
		return "reject"
	case OverflowThrottle:
		return "throttle"
	case OverflowWarn:
		return "warn"
	case OverflowAutoUpgrade:
		return "auto_upgrade"
	default:
		return "unknown"
	}
}

// QuotaCheckMode 配额检查模式
type QuotaCheckMode int

const (
	// CheckModeSync 同步检查（实时校验，性能开销较大）
	CheckModeSync QuotaCheckMode = iota
	// CheckModeAsync 异步检查（内存缓存，定期同步）
	CheckModeAsync
	// CheckModeLazy 延迟检查（仅在超限阈值附近校验）
	CheckModeLazy
)

// PersistQuotaFunc 配额持久化回调函数签名
type PersistQuotaFunc func(ctx context.Context, tenantID string, config *QuotaConfig) error

// QuotaResult 配额检查结果
type QuotaResult struct {
	Allowed    bool
	QuotaType  QuotaType
	Limit      int64
	Current    int64
	Remaining  int64
	UsageRatio float64
	Exceeded   bool
	Message    string
}

// NewQuotaManager 创建配额管理器
// windowSize: 滑动窗口大小（如 1 分钟）
// policy: 超限处理策略
// checkMode: 检查模式
func NewQuotaManager(windowSize time.Duration, policy OverflowPolicy, checkMode QuotaCheckMode) *QuotaManager {
	return &QuotaManager{
		configs:        make(map[string]*QuotaConfig),
		usage:          make(map[string]*TenantUsage),
		windowSize:     windowSize,
		overflowPolicy: policy,
		checkMode:      checkMode,
	}
}

// SetPersistFunc 设置配额持久化回调
func (m *QuotaManager) SetPersistFunc(fn PersistQuotaFunc) {
	m.persistFunc = fn
}

// RegisterTenant 为租户注册配额配置
func (m *QuotaManager) RegisterTenant(tenantID string, config *QuotaConfig) {
	if config == nil {
		config = DefaultQuotaConfig()
	}
	config.TenantID = tenantID

	m.configsMu.Lock()
	m.configs[tenantID] = config
	m.configsMu.Unlock()

	m.usageMu.Lock()
	if _, ok := m.usage[tenantID]; !ok {
		m.usage[tenantID] = NewTenantUsage(windowSize)
	}
	m.usageMu.Unlock()
}

// GetQuotaConfig 获取租户配额配置
func (m *QuotaManager) GetQuotaConfig(tenantID string) *QuotaConfig {
	m.configsMu.RLock()
	defer m.configsMu.RUnlock()
	return m.configs[tenantID]
}

// UpdateQuota 更新租户配额
func (m *QuotaManager) UpdateQuota(ctx context.Context, tenantID string, config *QuotaConfig) error {
	config.TenantID = tenantID
	config.UpdatedAt = time.Now().Unix()

	m.configsMu.Lock()
	m.configs[tenantID] = config
	m.configsMu.Unlock()

	// 持久化到数据库
	if m.persistFunc != nil {
		if err := m.persistFunc(ctx, tenantID, config); err != nil {
			return fmt.Errorf("quota persist failed: %w", err)
		}
	}

	return nil
}

// DeleteTenant 删除租户配额配置
func (m *QuotaManager) DeleteTenant(tenantID string) {
	m.configsMu.Lock()
	delete(m.configs, tenantID)
	m.configsMu.Unlock()

	m.usageMu.Lock()
	delete(m.usage, tenantID)
	m.usageMu.Unlock()
}

// ============================================================================
// 四、配额检查接口
// ============================================================================

// CheckFlows 检查流量配额
func (m *QuotaManager) CheckFlows(ctx context.Context, tenantID string, count int64) (*QuotaResult, error) {
	start := time.Now()
	defer func() {
		QuotaCheckDuration.WithLabelValues("flows").Observe(time.Since(start).Seconds())
	}()

	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		config = DefaultQuotaConfig()
	}

	usage := m.getTenantUsage(tenantID)
	currentMin := usage.GetFlowsPerMin()
	currentDay := usage.GetFlowsPerDay()

	result := &QuotaResult{
		QuotaType: QuotaTypeFlowsPerMin,
		Limit:     config.MaxFlowsPerMin,
		Current:   currentMin,
		Remaining: config.MaxFlowsPerMin - currentMin,
	}

	if currentMin+count > config.MaxFlowsPerMin {
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(currentMin) / float64(config.MaxFlowsPerMin)
		result.Message = fmt.Sprintf("flows per minute exceeded: limit %d, current %d, requested %d",
			config.MaxFlowsPerMin, currentMin, count)
		m.handleOverflow(ctx, tenantID, QuotaTypeFlowsPerMin, result)
		return result, nil
	}

	// 日配额检查
	if currentDay+count > config.MaxFlowsPerDay {
		result.QuotaType = QuotaTypeFlowsPerDay
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(currentDay) / float64(config.MaxFlowsPerDay)
		result.Message = fmt.Sprintf("flows per day exceeded: limit %d, current %d, requested %d",
			config.MaxFlowsPerDay, currentDay, count)
		m.handleOverflow(ctx, tenantID, QuotaTypeFlowsPerDay, result)
		return result, nil
	}

	result.Allowed = true
	result.UsageRatio = float64(currentMin) / float64(config.MaxFlowsPerMin)
	return result, nil
}

// CheckStorage 检查存储配额
func (m *QuotaManager) CheckStorage(ctx context.Context, tenantID string, bytes int64) (*QuotaResult, error) {
	start := time.Now()
	defer func() {
		QuotaCheckDuration.WithLabelValues("storage").Observe(time.Since(start).Seconds())
	}()

	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		config = DefaultQuotaConfig()
	}

	usage := m.getTenantUsage(tenantID)
	current := usage.GetStorageBytes()

	result := &QuotaResult{
		QuotaType: QuotaTypeStorageBytes,
		Limit:     config.MaxStorageBytes,
		Current:   current,
		Remaining: config.MaxStorageBytes - current,
	}

	if current+bytes > config.MaxStorageBytes {
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(current) / float64(config.MaxStorageBytes)
		result.Message = fmt.Sprintf("storage exceeded: limit %d bytes, current %d, requested %d",
			config.MaxStorageBytes, current, bytes)
		m.handleOverflow(ctx, tenantID, QuotaTypeStorageBytes, result)
		return result, nil
	}

	result.Allowed = true
	result.UsageRatio = float64(current) / float64(config.MaxStorageBytes)
	return result, nil
}

// CheckAgentCount 检查 Agent 数量配额
func (m *QuotaManager) CheckAgentCount(ctx context.Context, tenantID string, addCount int) (*QuotaResult, error) {
	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		config = DefaultQuotaConfig()
	}

	usage := m.getTenantUsage(tenantID)
	current := usage.GetAgentCount()

	result := &QuotaResult{
		QuotaType: QuotaTypeAgentCount,
		Limit:     int64(config.MaxAgentCount),
		Current:   int64(current),
		Remaining: int64(config.MaxAgentCount - current),
	}

	if current+addCount > config.MaxAgentCount {
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(current) / float64(config.MaxAgentCount)
		result.Message = fmt.Sprintf("agent count exceeded: limit %d, current %d, requested %d",
			config.MaxAgentCount, current, addCount)
		m.handleOverflow(ctx, tenantID, QuotaTypeAgentCount, result)
		return result, nil
	}

	result.Allowed = true
	result.UsageRatio = float64(current) / float64(config.MaxAgentCount)
	return result, nil
}

// CheckAlertRules 检查告警规则数量配额
func (m *QuotaManager) CheckAlertRules(ctx context.Context, tenantID string, addCount int) (*QuotaResult, error) {
	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		config = DefaultQuotaConfig()
	}

	usage := m.getTenantUsage(tenantID)
	current := usage.GetAlertRuleCount()

	result := &QuotaResult{
		QuotaType: QuotaTypeAlertRules,
		Limit:     int64(config.MaxAlertRules),
		Current:   int64(current),
		Remaining: int64(config.MaxAlertRules - current),
	}

	if current+addCount > config.MaxAlertRules {
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(current) / float64(config.MaxAlertRules)
		result.Message = fmt.Sprintf("alert rules exceeded: limit %d, current %d, requested %d",
			config.MaxAlertRules, current, addCount)
		m.handleOverflow(ctx, tenantID, QuotaTypeAlertRules, result)
		return result, nil
	}

	result.Allowed = true
	result.UsageRatio = float64(current) / float64(config.MaxAlertRules)
	return result, nil
}

// CheckAPIRate 检查 API 速率限制
func (m *QuotaManager) CheckAPIRate(ctx context.Context, tenantID string) (*QuotaResult, error) {
	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		config = DefaultQuotaConfig()
	}

	usage := m.getTenantUsage(tenantID)
	current := usage.GetAPICallsPerMin()

	result := &QuotaResult{
		QuotaType: QuotaTypeAPIRateLimit,
		Limit:     int64(config.MaxAPIRateLimit),
		Current:   int64(current),
		Remaining: int64(config.MaxAPIRateLimit - current),
	}

	if current >= config.MaxAPIRateLimit {
		result.Allowed = false
		result.Exceeded = true
		result.UsageRatio = float64(current) / float64(config.MaxAPIRateLimit)
		result.Message = fmt.Sprintf("API rate limit exceeded: limit %d/min, current %d/min",
			config.MaxAPIRateLimit, current)
		m.handleOverflow(ctx, tenantID, QuotaTypeAPIRateLimit, result)
		return result, nil
	}

	result.Allowed = true
	result.UsageRatio = float64(current) / float64(config.MaxAPIRateLimit)
	return result, nil
}

// CheckAll 检查所有配额（快速模式）
func (m *QuotaManager) CheckAll(ctx context.Context, tenantID string) map[QuotaType]*QuotaResult {
	results := make(map[QuotaType]*QuotaResult)

	// 流量配额
	if r, _ := m.CheckFlows(ctx, tenantID, 0); r.Exceeded {
		results[QuotaTypeFlowsPerMin] = r
	}

	// 存储配额
	if r, _ := m.CheckStorage(ctx, tenantID, 0); r.Exceeded {
		results[QuotaTypeStorageBytes] = r
	}

	// Agent 配额
	if r, _ := m.CheckAgentCount(ctx, tenantID, 0); r.Exceeded {
		results[QuotaTypeAgentCount] = r
	}

	// 告警规则配额
	if r, _ := m.CheckAlertRules(ctx, tenantID, 0); r.Exceeded {
		results[QuotaTypeAlertRules] = r
	}

	// API 速率
	if r, _ := m.CheckAPIRate(ctx, tenantID); r.Exceeded {
		results[QuotaTypeAPIRateLimit] = r
	}

	return results
}

// ============================================================================
// 五、使用量统计接口
// ============================================================================

// RecordFlows 记录流量使用
func (m *QuotaManager) RecordFlows(tenantID string, count int64) {
	usage := m.getTenantUsage(tenantID)
	usage.AddFlows(count)
	m.updateQuotaGauge(tenantID, QuotaTypeFlowsPerMin, usage)
}

// RecordStorage 记录存储使用
func (m *QuotaManager) RecordStorage(tenantID string, bytes int64) {
	usage := m.getTenantUsage(tenantID)
	usage.SetStorageBytes(bytes)
	m.updateQuotaGauge(tenantID, QuotaTypeStorageBytes, usage)
}

// RecordAgentCount 记录 Agent 数量
func (m *QuotaManager) RecordAgentCount(tenantID string, count int) {
	usage := m.getTenantUsage(tenantID)
	usage.SetAgentCount(count)
	m.updateQuotaGauge(tenantID, QuotaTypeAgentCount, usage)
}

// RecordAlertRuleCount 记录告警规则数量
func (m *QuotaManager) RecordAlertRuleCount(tenantID string, count int) {
	usage := m.getTenantUsage(tenantID)
	usage.SetAlertRuleCount(count)
	m.updateQuotaGauge(tenantID, QuotaTypeAlertRules, usage)
}

// RecordAPICall 记录 API 调用
func (m *QuotaManager) RecordAPICall(tenantID string) {
	usage := m.getTenantUsage(tenantID)
	usage.AddAPICall()
	m.updateQuotaGauge(tenantID, QuotaTypeAPIRateLimit, usage)
}

// GetUsage 获取租户当前使用量
func (m *QuotaManager) GetUsage(tenantID string) *TenantUsageSnapshot {
	usage := m.getTenantUsage(tenantID)
	return usage.Snapshot()
}

// ============================================================================
// 六、超限处理
// ============================================================================

func (m *QuotaManager) handleOverflow(ctx context.Context, tenantID string, quotaType QuotaType, result *QuotaResult) {
	QuotaExceededTotal.WithLabelValues(tenantID, string(quotaType)).Inc()
	m.updateQuotaGauge(tenantID, quotaType, m.getTenantUsage(tenantID))

	switch m.overflowPolicy {
	case OverflowReject:
		result.Allowed = false
	case OverflowThrottle:
		result.Allowed = true
		result.Message += " (throttled)"
	case OverflowWarn:
		result.Allowed = true
		result.Message += " (warning only)"
	case OverflowAutoUpgrade:
		result.Allowed = false
		result.Message += " (upgrade required)"
	}
}

func (m *QuotaManager) updateQuotaGauge(tenantID string, quotaType QuotaType, usage *TenantUsage) {
	config := m.GetQuotaConfig(tenantID)
	if config == nil {
		return
	}

	var ratio float64
	switch quotaType {
	case QuotaTypeFlowsPerMin:
		ratio = float64(usage.GetFlowsPerMin()) / float64(config.MaxFlowsPerMin)
	case QuotaTypeStorageBytes:
		ratio = float64(usage.GetStorageBytes()) / float64(config.MaxStorageBytes)
	case QuotaTypeAgentCount:
		ratio = float64(usage.GetAgentCount()) / float64(config.MaxAgentCount)
	case QuotaTypeAlertRules:
		ratio = float64(usage.GetAlertRuleCount()) / float64(config.MaxAlertRules)
	case QuotaTypeAPIRateLimit:
		ratio = float64(usage.GetAPICallsPerMin()) / float64(config.MaxAPIRateLimit)
	}
	QuotaUsage.WithLabelValues(tenantID, string(quotaType)).Set(ratio)
}

// ============================================================================
// 七、辅助函数
// ============================================================================

func (m *QuotaManager) getTenantUsage(tenantID string) *TenantUsage {
	m.usageMu.RLock()
	u, ok := m.usage[tenantID]
	m.usageMu.RUnlock()
	if ok {
		return u
	}

	m.usageMu.Lock()
	u, ok = m.usage[tenantID]
	if !ok {
		u = NewTenantUsage(m.windowSize)
		m.usage[tenantID] = u
	}
	m.usageMu.Unlock()
	return u
}

// GetOverflowPolicy 返回当前超限处理策略
func (m *QuotaManager) GetOverflowPolicy() OverflowPolicy {
	return m.overflowPolicy
}

// SetOverflowPolicy 设置超限处理策略
func (m *QuotaManager) SetOverflowPolicy(policy OverflowPolicy) {
	m.overflowPolicy = policy
}

// GetCheckMode 返回当前配额检查模式
func (m *QuotaManager) GetCheckMode() QuotaCheckMode {
	return m.checkMode
}

// SetCheckMode 设置配额检查模式
func (m *QuotaManager) SetCheckMode(mode QuotaCheckMode) {
	m.checkMode = mode
}

// GetTenantCount 返回已注册租户数量
func (m *QuotaManager) GetTenantCount() int {
	m.configsMu.RLock()
	defer m.configsMu.RUnlock()
	return len(m.configs)
}
