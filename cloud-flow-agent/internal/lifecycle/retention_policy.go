// Package lifecycle 提供监控数据生命周期管理
// 本文件实现数据保留策略管理
// 支持：默认保留周期、按分类自定义保留周期、动态策略更新
package lifecycle

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ==================== 保留策略定义 ====================

// RetentionPolicy 保留策略
type RetentionPolicy struct {
	// 基本信息
	Category      DataCategory `json:"category"`        // 数据分类
	Enabled       bool         `json:"enabled"`         // 是否启用
	RetentionDays int          `json:"retention_days"`  // 保留天数

	// 高级配置
	GracePeriod   time.Duration `json:"grace_period"`   // 宽限期（避免误删最近数据）
	CleanupOrder  int           `json:"cleanup_order"`  // 清理优先级（数字越小越先清理）
	MaxCleanupPct float64       `json:"max_cleanup_pct"` // 单次最大清理比例(0-1)，防止一次性删除过多

	// 数据源过滤（可选）
	SourceFilter  string        `json:"source_filter"`  // 数据源过滤（空表示所有数据源）
	TagFilter     map[string]string `json:"tag_filter"` // 标签过滤

	// 时间约束
	ActiveAfter   time.Time     `json:"active_after"`   // 策略生效时间
	ActiveBefore  time.Time     `json:"active_before"`  // 策略失效时间
}

// Validate 验证策略有效性
func (p *RetentionPolicy) Validate() error {
	if p.Category == "" {
		return fmt.Errorf("数据分类不能为空")
	}
	if p.RetentionDays < 1 {
		return fmt.Errorf("保留天数不能小于1天")
	}
	if p.MaxCleanupPct > 0 && (p.MaxCleanupPct < 0.01 || p.MaxCleanupPct > 1.0) {
		return fmt.Errorf("单次最大清理比例必须在 0.01-1.0 之间")
	}
	return nil
}

// IsExpired 检查给定时间的数据是否过期
func (p *RetentionPolicy) IsExpired(dataTime time.Time) bool {
	if !p.Enabled {
		return false
	}
	cutoff := time.Now().AddDate(0, 0, -p.RetentionDays)
	// 考虑宽限期：在宽限期内的数据不删除
	if p.GracePeriod > 0 {
		cutoff = cutoff.Add(p.GracePeriod)
	}
	return dataTime.Before(cutoff)
}

// GetCutoffTime 获取截止时间
func (p *RetentionPolicy) GetCutoffTime() time.Time {
	cutoff := time.Now().AddDate(0, 0, -p.RetentionDays)
	if p.GracePeriod > 0 {
		cutoff = cutoff.Add(p.GracePeriod)
	}
	return cutoff
}

// IsActive 检查策略是否在有效期内
func (p *RetentionPolicy) IsActive() bool {
	now := time.Now()
	if !p.ActiveAfter.IsZero() && now.Before(p.ActiveAfter) {
		return false
	}
	if !p.ActiveBefore.IsZero() && now.After(p.ActiveBefore) {
		return false
	}
	return true
}

// ==================== 保留策略管理器 ====================

// RetentionPolicyManager 保留策略管理器
type RetentionPolicyManager struct {
	policies map[DataCategory]*RetentionPolicy
	mu       sync.RWMutex
	config   *LifecycleConfig
}

// NewRetentionPolicyManager 创建保留策略管理器
func NewRetentionPolicyManager(cfg *LifecycleConfig) *RetentionPolicyManager {
	mgr := &RetentionPolicyManager{
		policies: make(map[DataCategory]*RetentionPolicy),
		config:   cfg,
	}

	// 加载默认策略
	mgr.loadDefaultPolicies()

	// 加载自定义策略
	for _, policy := range cfg.CustomPolicies {
		mgr.policies[policy.Category] = policy
	}

	return mgr
}

// loadDefaultPolicies 加载默认保留策略
func (m *RetentionPolicyManager) loadDefaultPolicies() {
	// 默认保留策略
	defaults := []*RetentionPolicy{
		{
			Category:      CategoryMetric,
			Enabled:       true,
			RetentionDays: 60,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  10,
			MaxCleanupPct: 0.5,
		},
		{
			Category:      CategoryLog,
			Enabled:       true,
			RetentionDays: 30,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  20,
			MaxCleanupPct: 0.5,
		},
		{
			Category:      CategoryTrace,
			Enabled:       true,
			RetentionDays: 7,
			GracePeriod:   30 * time.Minute,
			CleanupOrder:  30,
			MaxCleanupPct: 0.3,
		},
		{
			Category:      CategoryEvent,
			Enabled:       true,
			RetentionDays: 90,
			GracePeriod:   2 * time.Hour,
			CleanupOrder:  5,
			MaxCleanupPct: 0.5,
		},
		// 细分分类
		{
			Category:      CategorySystemMetric,
			Enabled:       true,
			RetentionDays: 60,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  11,
		},
		{
			Category:      CategoryAppMetric,
			Enabled:       true,
			RetentionDays: 60,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  12,
		},
		{
			Category:      CategoryDBMetric,
			Enabled:       true,
			RetentionDays: 30,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  15,
		},
		{
			Category:      CategorySQLAggregate,
			Enabled:       true,
			RetentionDays: 30,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  16,
		},
		{
			Category:      CategoryProfiling,
			Enabled:       true,
			RetentionDays: 14,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  25,
		},
		{
			Category:      CategoryAlert,
			Enabled:       true,
			RetentionDays: 90,
			GracePeriod:   2 * time.Hour,
			CleanupOrder:  6,
		},
		{
			Category:      CategoryTopology,
			Enabled:       true,
			RetentionDays: 30,
			GracePeriod:   1 * time.Hour,
			CleanupOrder:  18,
		},
		{
			Category:      CategorySelfMonitor,
			Enabled:       true,
			RetentionDays: 7,
			GracePeriod:   30 * time.Minute,
			CleanupOrder:  35,
		},
	}

	for _, policy := range defaults {
		m.policies[policy.Category] = policy
	}
}

// GetPolicy 获取指定分类的保留策略
func (m *RetentionPolicyManager) GetPolicy(category DataCategory) *RetentionPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 先尝试精确匹配
	if policy, ok := m.policies[category]; ok {
		return policy
	}

	// 尝试通用分类匹配
	generic := m.resolveGenericCategory(category)
	if generic != "" {
		if policy, ok := m.policies[generic]; ok {
			return policy
		}
	}

	// 使用默认策略
	return &RetentionPolicy{
		Category:      category,
		Enabled:       true,
		RetentionDays: m.config.DefaultRetentionDays,
		GracePeriod:   1 * time.Hour,
	}
}

// resolveGenericCategory 将细分分类解析为通用分类
func (m *RetentionPolicyManager) resolveGenericCategory(category DataCategory) DataCategory {
	// 细分分类 -> 通用分类的映射
	mapping := map[DataCategory]DataCategory{
		CategorySystemMetric: CategoryMetric,
		CategoryAppMetric:    CategoryMetric,
		CategoryDBMetric:     CategoryMetric,
		CategorySQLAggregate: CategoryMetric,
	}

	if generic, ok := mapping[category]; ok {
		return generic
	}
	return ""
}

// SetPolicy 设置保留策略
func (m *RetentionPolicyManager) SetPolicy(policy *RetentionPolicy) error {
	if err := policy.Validate(); err != nil {
		return fmt.Errorf("策略验证失败: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[policy.Category] = policy

	return nil
}

// DeletePolicy 删除保留策略（回退到默认）
func (m *RetentionPolicyManager) DeletePolicy(category DataCategory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.policies, category)
}

// GetAllPolicies 获取所有保留策略
func (m *RetentionPolicyManager) GetAllPolicies() map[DataCategory]*RetentionPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[DataCategory]*RetentionPolicy)
	for k, v := range m.policies {
		policy := *v // 复制
		result[k] = &policy
	}
	return result
}

// GetEnabledPolicies 获取所有已启用的保留策略（按清理优先级排序）
func (m *RetentionPolicyManager) GetEnabledPolicies() []*RetentionPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	policies := make([]*RetentionPolicy, 0)
	for _, policy := range m.policies {
		if policy.Enabled && policy.IsActive() {
			p := *policy // 复制
			policies = append(policies, &p)
		}
	}

	// 按清理优先级排序（数字越小越先清理）
	for i := 0; i < len(policies)-1; i++ {
		for j := i + 1; j < len(policies); j++ {
			if policies[i].CleanupOrder > policies[j].CleanupOrder {
				policies[i], policies[j] = policies[j], policies[i]
			}
		}
	}

	return policies
}

// GetAllCategories 获取所有已配置的数据分类
func (m *RetentionPolicyManager) GetAllCategories() []DataCategory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	categories := make([]DataCategory, 0, len(m.policies))
	for cat := range m.policies {
		categories = append(categories, cat)
	}
	return categories
}

// GetRetentionDays 获取指定分类的保留天数
func (m *RetentionPolicyManager) GetRetentionDays(category DataCategory) int {
	policy := m.GetPolicy(category)
	if policy == nil {
		return m.config.DefaultRetentionDays
	}
	return policy.RetentionDays
}

// UpdateRetentionDays 更新保留天数
func (m *RetentionPolicyManager) UpdateRetentionDays(category DataCategory, days int) error {
	if days < 1 {
		return fmt.Errorf("保留天数不能小于1天")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	policy, exists := m.policies[category]
	if exists {
		policy.RetentionDays = days
	} else {
		m.policies[category] = &RetentionPolicy{
			Category:      category,
			Enabled:       true,
			RetentionDays: days,
			GracePeriod:   1 * time.Hour,
		}
	}

	return nil
}

// ==================== 保留策略快照 ====================

// RetentionPolicySnapshot 保留策略快照（用于审计和回滚）
type RetentionPolicySnapshot struct {
	Timestamp time.Time                       `json:"timestamp"`
	Policies  map[DataCategory]*RetentionPolicy `json:"policies"`
	Reason    string                          `json:"reason"`
}

// TakeSnapshot 创建策略快照
func (m *RetentionPolicyManager) TakeSnapshot(reason string) *RetentionPolicySnapshot {
	return &RetentionPolicySnapshot{
		Timestamp: time.Now(),
		Policies:  m.GetAllPolicies(),
		Reason:    reason,
	}
}

// RestoreFromSnapshot 从快照恢复策略
func (m *RetentionPolicyManager) RestoreFromSnapshot(snapshot *RetentionPolicySnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.policies = make(map[DataCategory]*RetentionPolicy)
	for k, v := range snapshot.Policies {
		policy := *v // 深拷贝
		m.policies[k] = &policy
	}
}

// ==================== 保留策略解析 ====================

// ParseDataCategory 从字符串解析数据分类
func ParseDataCategory(s string) DataCategory {
	switch strings.ToLower(s) {
	case "metric":
		return CategoryMetric
	case "log":
		return CategoryLog
	case "trace":
		return CategoryTrace
	case "event":
		return CategoryEvent
	case "system_metric":
		return CategorySystemMetric
	case "app_metric":
		return CategoryAppMetric
	case "db_metric":
		return CategoryDBMetric
	case "sql_aggregate":
		return CategorySQLAggregate
	case "profiling":
		return CategoryProfiling
	case "alert":
		return CategoryAlert
	case "topology":
		return CategoryTopology
	case "self_monitor":
		return CategorySelfMonitor
	case "custom":
		return CategoryCustom
	default:
		return DataCategory(s)
	}
}

// ==================== 生命周期配置 ====================

// LifecycleConfig 生命周期管理配置
type LifecycleConfig struct {
	// 基础配置
	DefaultRetentionDays int    `json:"default_retention_days"` // 默认保留天数（60天）
	Enabled              bool   `json:"enabled"`               // 是否启用生命周期管理

	// 清理调度配置
	CleanupSchedule      string `json:"cleanup_schedule"`      // 清理调度表达式（cron 格式）
	CleanupTime          string `json:"cleanup_time"`          // 清理时间（HH:MM 格式，如 "02:00"）
	CleanupBatchSize     int    `json:"cleanup_batch_size"`    // 单批清理大小
	CleanupInterval      int    `json:"cleanup_interval_min"`  // 清理间隔（分钟）

	// 并发控制
	MaxConcurrentCleanups int    `json:"max_concurrent_cleanups"` // 最大并发清理数
	CleanupTimeout        int    `json:"cleanup_timeout_min"`     // 单次清理超时（分钟）

	// 限速配置
	MaxDeleteRate         int    `json:"max_delete_rate"`          // 每秒最大删除数（0=不限）
	MaxDiskUsageForCleanup float64 `json:"max_disk_usage_for_cleanup"` // 磁盘使用率上限，超过此值触发紧急清理

	// 历史记录
	MaxHistoryRecords    int    `json:"max_history_records"`    // 最大历史记录数

	// 自定义策略
	CustomPolicies       []*RetentionPolicy `json:"custom_policies"` // 自定义保留策略

	// 查询隔离
	QueryIsolationEnabled bool   `json:"query_isolation_enabled"` // 是否启用查询隔离
	CleanupThrottleRate   float64 `json:"cleanup_throttle_rate"`   // 清理限速比例（0-1）
}

// DefaultLifecycleConfig 返回默认配置
func DefaultLifecycleConfig() *LifecycleConfig {
	return &LifecycleConfig{
		DefaultRetentionDays:  60,
		Enabled:               true,
		CleanupSchedule:       "daily",
		CleanupTime:           "02:00",
		CleanupBatchSize:      1000,
		CleanupInterval:       60,
		MaxConcurrentCleanups: 2,
		CleanupTimeout:        120,
		MaxDeleteRate:         0,
		MaxDiskUsageForCleanup: 0.85,
		MaxHistoryRecords:     1000,
		CustomPolicies:        make([]*RetentionPolicy, 0),
		QueryIsolationEnabled: true,
		CleanupThrottleRate:   0.5,
	}
}
