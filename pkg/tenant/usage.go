// P25: 租户使用量滑动窗口统计
//
// 为配额管理器提供实时使用量统计能力
// 支持：流量/存储/Agent/告警规则/API 调用的滑动窗口统计
//
package tenant

import (
	"sync"
	"time"
)

// ============================================================================
// 一、滑动窗口统计器
// ============================================================================

// SlidingWindowCounter 滑动窗口计数器
type SlidingWindowCounter struct {
	window    time.Duration
	intervals []intervalCount
	mu        sync.RWMutex
}

type intervalCount struct {
	start time.Time
	count int64
}

// NewSlidingWindowCounter 创建滑动窗口计数器
// window: 窗口大小（如 1 分钟）
func NewSlidingWindowCounter(window time.Duration) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		window:    window,
		intervals: make([]intervalCount, 0, 64),
	}
}

// Add 增加计数
func (c *SlidingWindowCounter) Add(count int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.evict(now)
	c.intervals = append(c.intervals, intervalCount{start: now, count: count})
}

// Get 获取当前窗口内的总数量
func (c *SlidingWindowCounter) Get() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	c.evictUnsafe(now)

	var total int64
	for _, ic := range c.intervals {
		total += ic.count
	}
	return total
}

// Reset 重置计数器
func (c *SlidingWindowCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.intervals = c.intervals[:0]
}

func (c *SlidingWindowCounter) evict(now time.Time) {
	cutoff := now.Add(-c.window)
	idx := 0
	for i, ic := range c.intervals {
		if ic.start.After(cutoff) {
			idx = i
			break
		}
	}
	if idx > 0 {
		copy(c.intervals, c.intervals[idx:])
		c.intervals = c.intervals[:len(c.intervals)-idx]
	} else if len(c.intervals) > 0 {
		// 检查是否所有 interval 都已过期
		last := c.intervals[len(c.intervals)-1]
		if !last.start.After(cutoff) {
			c.intervals = c.intervals[:0]
		}
	}
}

func (c *SlidingWindowCounter) evictUnsafe(now time.Time) {
	cutoff := now.Add(-c.window)
	idx := 0
	for i, ic := range c.intervals {
		if ic.start.After(cutoff) {
			idx = i
			break
		}
	}
	if idx > 0 {
		copy(c.intervals, c.intervals[idx:])
		c.intervals = c.intervals[:len(c.intervals)-idx]
	} else if len(c.intervals) > 0 {
		last := c.intervals[len(c.intervals)-1]
		if !last.start.After(cutoff) {
			c.intervals = c.intervals[:0]
		}
	}
}

// ============================================================================
// 二、租户使用量（TenantUsage）
// ============================================================================

// TenantUsage 租户实时使用量统计
type TenantUsage struct {
	// 流量统计
	flowsPerMin  *SlidingWindowCounter // 每分钟流量
	flowsPerDay  *SlidingWindowCounter // 每日流量（24 小时窗口）
	metricsPerMin *SlidingWindowCounter // 每分钟指标

	// 存储统计
	storageBytes int64

	// 资源统计
	agentCount     int
	alertRuleCount int
	userCount      int
	projectCount   int

	// API 统计
	apiCallsPerMin *SlidingWindowCounter

	// 同步锁
	mu sync.RWMutex
}

// NewTenantUsage 创建租户使用量统计器
// windowSize: 默认滑动窗口大小（用于流量和 API 统计）
func NewTenantUsage(windowSize time.Duration) *TenantUsage {
	return &TenantUsage{
		flowsPerMin:    NewSlidingWindowCounter(windowSize),
		flowsPerDay:    NewSlidingWindowCounter(24 * time.Hour),
		metricsPerMin:  NewSlidingWindowCounter(windowSize),
		apiCallsPerMin: NewSlidingWindowCounter(windowSize),
	}
}

// AddFlows 记录流量使用
func (u *TenantUsage) AddFlows(count int64) {
	u.mu.Lock()
	u.flowsPerMin.Add(count)
	u.flowsPerDay.Add(count)
	u.mu.Unlock()
}

// GetFlowsPerMin 获取当前每分钟流量数
func (u *TenantUsage) GetFlowsPerMin() int64 {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.flowsPerMin.Get()
}

// GetFlowsPerDay 获取当前每日流量数
func (u *TenantUsage) GetFlowsPerDay() int64 {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.flowsPerDay.Get()
}

// AddMetrics 记录指标使用
func (u *TenantUsage) AddMetrics(count int64) {
	u.mu.Lock()
	u.metricsPerMin.Add(count)
	u.mu.Unlock()
}

// GetMetricsPerMin 获取当前每分钟指标数
func (u *TenantUsage) GetMetricsPerMin() int64 {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.metricsPerMin.Get()
}

// SetStorageBytes 设置存储字节数
func (u *TenantUsage) SetStorageBytes(bytes int64) {
	u.mu.Lock()
	u.storageBytes = bytes
	u.mu.Unlock()
}

// GetStorageBytes 获取当前存储字节数
func (u *TenantUsage) GetStorageBytes() int64 {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.storageBytes
}

// SetAgentCount 设置 Agent 数量
func (u *TenantUsage) SetAgentCount(count int) {
	u.mu.Lock()
	u.agentCount = count
	u.mu.Unlock()
}

// GetAgentCount 获取当前 Agent 数量
func (u *TenantUsage) GetAgentCount() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.agentCount
}

// SetAlertRuleCount 设置告警规则数量
func (u *TenantUsage) SetAlertRuleCount(count int) {
	u.mu.Lock()
	u.alertRuleCount = count
	u.mu.Unlock()
}

// GetAlertRuleCount 获取当前告警规则数量
func (u *TenantUsage) GetAlertRuleCount() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.alertRuleCount
}

// SetUserCount 设置用户数量
func (u *TenantUsage) SetUserCount(count int) {
	u.mu.Lock()
	u.userCount = count
	u.mu.Unlock()
}

// GetUserCount 获取当前用户数量
func (u *TenantUsage) GetUserCount() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.userCount
}

// SetProjectCount 设置项目数量
func (u *TenantUsage) SetProjectCount(count int) {
	u.mu.Lock()
	u.projectCount = count
	u.mu.Unlock()
}

// GetProjectCount 获取当前项目数量
func (u *TenantUsage) GetProjectCount() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.projectCount
}

// AddAPICall 记录 API 调用
func (u *TenantUsage) AddAPICall() {
	u.mu.Lock()
	u.apiCallsPerMin.Add(1)
	u.mu.Unlock()
}

// GetAPICallsPerMin 获取当前每分钟 API 调用数
func (u *TenantUsage) GetAPICallsPerMin() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return int(u.apiCallsPerMin.Get())
}

// Reset 重置所有使用量（用于租户数据清理后）
func (u *TenantUsage) Reset() {
	u.mu.Lock()
	u.flowsPerMin.Reset()
	u.flowsPerDay.Reset()
	u.metricsPerMin.Reset()
	u.apiCallsPerMin.Reset()
	u.storageBytes = 0
	u.agentCount = 0
	u.alertRuleCount = 0
	u.userCount = 0
	u.projectCount = 0
	u.mu.Unlock()
}

// ============================================================================
// 三、使用量快照（TenantUsageSnapshot）
// ============================================================================

// TenantUsageSnapshot 租户使用量快照（用于外部读取）
type TenantUsageSnapshot struct {
	FlowsPerMin    int64 `json:"flows_per_min"`
	FlowsPerDay    int64 `json:"flows_per_day"`
	MetricsPerMin  int64 `json:"metrics_per_min"`
	StorageBytes   int64 `json:"storage_bytes"`
	AgentCount     int   `json:"agent_count"`
	AlertRuleCount int   `json:"alert_rule_count"`
	UserCount      int   `json:"user_count"`
	ProjectCount   int   `json:"project_count"`
	APICallsPerMin int   `json:"api_calls_per_min"`
	Timestamp      int64 `json:"timestamp"`
}

// Snapshot 生成使用量快照
func (u *TenantUsage) Snapshot() *TenantUsageSnapshot {
	u.mu.RLock()
	defer u.mu.RUnlock()

	return &TenantUsageSnapshot{
		FlowsPerMin:    u.flowsPerMin.Get(),
		FlowsPerDay:    u.flowsPerDay.Get(),
		MetricsPerMin:  u.metricsPerMin.Get(),
		StorageBytes:   u.storageBytes,
		AgentCount:     u.agentCount,
		AlertRuleCount: u.alertRuleCount,
		UserCount:      u.userCount,
		ProjectCount:   u.projectCount,
		APICallsPerMin: int(u.apiCallsPerMin.Get()),
		Timestamp:      time.Now().Unix(),
	}
}

// windowSize 包级变量，用于 NewTenantUsage 的默认值
var windowSize = time.Minute
