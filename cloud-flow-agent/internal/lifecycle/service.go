// Package lifecycle 提供监控数据生命周期管理
// 本文件实现 Agent 集成服务，对接存储层并提供 API 接口
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ==================== 存储层适配器 ====================

// StorageAdapter 存储层适配器
// 将 lifecycle 的 DataScanner 接口适配到 storage.TimeSeriesStore
type StorageAdapter struct {
	// 存储操作接口（由外部注入）
	scanFunc    func(ctx context.Context, cutoffTime time.Time, category DataCategory, callback func(batch DataBatch) bool) error
	deleteFunc  func(ctx context.Context, batch DataBatch) (int64, int64, error)
	statsFunc   func(ctx context.Context, category DataCategory) (*CategoryDataStats, error)
}

// NewStorageAdapter 创建存储适配器
func NewStorageAdapter() *StorageAdapter {
	return &StorageAdapter{}
}

// SetScanFunc 设置扫描函数
func (a *StorageAdapter) SetScanFunc(fn func(ctx context.Context, cutoffTime time.Time, category DataCategory, callback func(batch DataBatch) bool) error) {
	a.scanFunc = fn
}

// SetDeleteFunc 设置删除函数
func (a *StorageAdapter) SetDeleteFunc(fn func(ctx context.Context, batch DataBatch) (int64, int64, error)) {
	a.deleteFunc = fn
}

// SetStatsFunc 设置统计函数
func (a *StorageAdapter) SetStatsFunc(fn func(ctx context.Context, category DataCategory) (*CategoryDataStats, error)) {
	a.statsFunc = fn
}

// ScanExpired 扫描过期数据
func (a *StorageAdapter) ScanExpired(ctx context.Context, cutoffTime time.Time, category DataCategory, callback func(batch DataBatch) bool) error {
	if a.scanFunc == nil {
		return fmt.Errorf("扫描函数未设置")
	}
	return a.scanFunc(ctx, cutoffTime, category, callback)
}

// DeleteBatch 批量删除
func (a *StorageAdapter) DeleteBatch(ctx context.Context, batch DataBatch) (int64, int64, error) {
	if a.deleteFunc == nil {
		return 0, 0, fmt.Errorf("删除函数未设置")
	}
	return a.deleteFunc(ctx, batch)
}

// GetCategoryStats 获取分类统计
func (a *StorageAdapter) GetCategoryStats(ctx context.Context, category DataCategory) (*CategoryDataStats, error) {
	if a.statsFunc == nil {
		return nil, fmt.Errorf("统计函数未设置")
	}
	return a.statsFunc(ctx, category)
}

// ==================== 生命周期服务 ====================

// LifecycleService 生命周期服务
// 对外暴露的统一接口，整合管理器、策略、调度器
type LifecycleService struct {
	manager    *LifecycleManager
	adapter    *StorageAdapter
	config     *LifecycleConfig

	// 运行状态
	running bool
	mu      sync.RWMutex
}

// NewLifecycleService 创建生命周期服务
func NewLifecycleService(cfg *LifecycleConfig) *LifecycleService {
	if cfg == nil {
		cfg = DefaultLifecycleConfig()
	}

	manager := NewLifecycleManager(cfg)
	adapter := NewStorageAdapter()

	// 连接适配器
	manager.SetScanner(adapter)

	service := &LifecycleService{
		manager: manager,
		adapter: adapter,
		config:  cfg,
	}

	return service
}

// ==================== 初始化与生命周期 ====================

// Init 初始化服务
func (s *LifecycleService) Init() error {
	if !s.config.Enabled {
		return nil
	}
	return nil
}

// Start 启动服务
func (s *LifecycleService) Start(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("服务已在运行")
	}
	s.mu.Unlock()

	return s.manager.Start(ctx)
}

// Stop 停止服务
func (s *LifecycleService) Stop() {
	s.manager.Stop()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// ==================== 存储层对接 ====================

// ConnectStorage 连接存储层
// 调用方需提供 scan/delete/stats 三个函数来对接具体的存储后端
func (s *LifecycleService) ConnectStorage(
	scanFunc func(ctx context.Context, cutoffTime time.Time, category DataCategory, callback func(batch DataBatch) bool) error,
	deleteFunc func(ctx context.Context, batch DataBatch) (int64, int64, error),
	statsFunc func(ctx context.Context, category DataCategory) (*CategoryDataStats, error),
) {
	s.adapter.SetScanFunc(scanFunc)
	s.adapter.SetDeleteFunc(deleteFunc)
	s.adapter.SetStatsFunc(statsFunc)
}

// ==================== 策略管理 API ====================

// GetPolicy 获取保留策略
func (s *LifecycleService) GetPolicy(category DataCategory) *RetentionPolicy {
	return s.manager.policies.GetPolicy(category)
}

// SetPolicy 设置保留策略
func (s *LifecycleService) SetPolicy(policy *RetentionPolicy) error {
	return s.manager.policies.SetPolicy(policy)
}

// UpdateRetentionDays 更新保留天数
func (s *LifecycleService) UpdateRetentionDays(category DataCategory, days int) error {
	return s.manager.policies.UpdateRetentionDays(category, days)
}

// GetAllPolicies 获取所有保留策略
func (s *LifecycleService) GetAllPolicies() map[DataCategory]*RetentionPolicy {
	return s.manager.policies.GetAllPolicies()
}

// GetEnabledPolicies 获取所有已启用的保留策略
func (s *LifecycleService) GetEnabledPolicies() []*RetentionPolicy {
	return s.manager.policies.GetEnabledPolicies()
}

// DeletePolicy 删除保留策略
func (s *LifecycleService) DeletePolicy(category DataCategory) {
	s.manager.policies.DeletePolicy(category)
}

// TakePolicySnapshot 创建策略快照
func (s *LifecycleService) TakePolicySnapshot(reason string) *RetentionPolicySnapshot {
	return s.manager.policies.TakeSnapshot(reason)
}

// RestorePolicySnapshot 从快照恢复策略
func (s *LifecycleService) RestorePolicySnapshot(snapshot *RetentionPolicySnapshot) {
	s.manager.policies.RestoreFromSnapshot(snapshot)
}

// ==================== 清理操作 API ====================

// TriggerCleanup 手动触发清理
func (s *LifecycleService) TriggerCleanup(ctx context.Context, categories ...DataCategory) ([]*CleanupTask, error) {
	return s.manager.ManualCleanup(ctx, categories...)
}

// TriggerFullCleanup 触发全量清理（所有分类）
func (s *LifecycleService) TriggerFullCleanup(ctx context.Context) ([]*CleanupTask, error) {
	return s.manager.ManualCleanup(ctx)
}

// ==================== 查询 API ====================

// GetStats 获取清理统计
func (s *LifecycleService) GetStats() *CleanupStats {
	return s.manager.GetStats()
}

// GetHistory 获取清理历史
func (s *LifecycleService) GetHistory(limit int) []*CleanupTask {
	return s.manager.GetHistory(limit)
}

// GetNextCleanupTime 获取下次清理时间
func (s *LifecycleService) GetNextCleanupTime() time.Time {
	return s.manager.scheduler.GetNextCleanupTime()
}

// GetLastCleanupTime 获取上次清理时间
func (s *LifecycleService) GetLastCleanupTime() time.Time {
	return s.manager.scheduler.GetLastCleanupTime()
}

// GetServiceStatus 获取服务状态
func (s *LifecycleService) GetServiceStatus() *LifecycleServiceStatus {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	stats := s.manager.GetStats()
	policies := s.manager.policies.GetEnabledPolicies()

	policySummary := make([]PolicySummary, 0, len(policies))
	for _, p := range policies {
		policySummary = append(policySummary, PolicySummary{
			Category:      p.Category,
			RetentionDays: p.RetentionDays,
			Enabled:       p.Enabled,
		})
	}

	return &LifecycleServiceStatus{
		Running:        running,
		Enabled:        s.config.Enabled,
		DefaultDays:    s.config.DefaultRetentionDays,
		NextCleanup:    s.manager.scheduler.GetNextCleanupTime(),
		LastCleanup:    s.manager.scheduler.GetLastCleanupTime(),
		PolicyCount:    len(policies),
		Policies:       policySummary,
		TotalCleaned:   stats.TotalDeleted,
		TotalBytesFreed: stats.TotalBytesFreed,
	}
}

// LifecycleServiceStatus 服务状态
type LifecycleServiceStatus struct {
	Running         bool            `json:"running"`
	Enabled         bool            `json:"enabled"`
	DefaultDays     int             `json:"default_days"`
	NextCleanup     time.Time       `json:"next_cleanup"`
	LastCleanup     time.Time       `json:"last_cleanup"`
	PolicyCount     int             `json:"policy_count"`
	Policies        []PolicySummary `json:"policies"`
	TotalCleaned    int64           `json:"total_cleaned"`
	TotalBytesFreed int64           `json:"total_bytes_freed"`
}

// PolicySummary 策略摘要
type PolicySummary struct {
	Category      DataCategory `json:"category"`
	RetentionDays int          `json:"retention_days"`
	Enabled       bool         `json:"enabled"`
}

// ==================== 查询隔离 API ====================

// MarkQueryStart 标记查询开始（供查询层调用）
func (s *LifecycleService) MarkQueryStart() {
	s.manager.scheduler.queryGuard.MarkQueryStart()
}

// MarkQueryEnd 标记查询结束（供查询层调用）
func (s *LifecycleService) MarkQueryEnd() {
	s.manager.scheduler.queryGuard.MarkQueryEnd()
}

// GetQueryCount 获取当前活跃查询数
func (s *LifecycleService) GetQueryCount() int64 {
	return s.manager.scheduler.queryGuard.GetQueryCount()
}

// IsHighLoad 检查是否高负载
func (s *LifecycleService) IsHighLoad() bool {
	return s.manager.scheduler.queryGuard.IsHighLoad()
}

// GetActiveCleanups 获取活跃清理列表
func (s *LifecycleService) GetActiveCleanups() []DataCategory {
	return s.manager.scheduler.queryGuard.GetActiveCleanups()
}

// PauseCleanup 暂停清理
func (s *LifecycleService) PauseCleanup() {
	s.manager.scheduler.throttler.Pause()
}

// ResumeCleanup 恢复清理
func (s *LifecycleService) ResumeCleanup() {
	s.manager.scheduler.throttler.Resume()
}

// ==================== 数据预估 API ====================

// EstimateDataSize 预估指定分类的数据量
func (s *LifecycleService) EstimateDataSize(ctx context.Context, category DataCategory) (*DataSizeEstimate, error) {
	stats, err := s.adapter.GetCategoryStats(ctx, category)
	if err != nil {
		return nil, err
	}

	policy := s.manager.policies.GetPolicy(category)
	if policy == nil {
		return nil, fmt.Errorf("未找到 %s 的保留策略", category)
	}

	estimate := &DataSizeEstimate{
		Category:      category,
		TotalSize:     stats.TotalSize,
		TotalCount:    stats.TotalCount,
		RetentionDays: policy.RetentionDays,
	}

	// 估算过期数据量
	if !stats.OldestTime.IsZero() {
		cutoff := policy.GetCutoffTime()
		if stats.OldestTime.Before(cutoff) {
			// 简化估算：按时间比例估算过期数据
			totalDuration := stats.NewestTime.Sub(stats.OldestTime)
			expiredDuration := cutoff.Sub(stats.OldestTime)
			if totalDuration > 0 && expiredDuration > 0 {
				ratio := float64(expiredDuration) / float64(totalDuration)
				if ratio > 1.0 {
					ratio = 1.0
				}
				estimate.ExpiredSize = int64(float64(stats.TotalSize) * ratio)
				estimate.ExpiredCount = int64(float64(stats.TotalCount) * ratio)
			}
		}
	}

	return estimate, nil
}

// DataSizeEstimate 数据量预估
type DataSizeEstimate struct {
	Category      DataCategory `json:"category"`
	TotalSize     int64        `json:"total_size"`
	TotalCount    int64        `json:"total_count"`
	ExpiredSize   int64        `json:"expired_size"`
	ExpiredCount  int64        `json:"expired_count"`
	RetentionDays int          `json:"retention_days"`
}

// ==================== 回调设置 ====================

// SetCleanupStartCallback 设置清理开始回调
func (s *LifecycleService) SetCleanupStartCallback(callback func(task *CleanupTask)) {
	s.manager.SetCleanupStartCallback(callback)
}

// SetCleanupEndCallback 设置清理结束回调
func (s *LifecycleService) SetCleanupEndCallback(callback func(task *CleanupTask)) {
	s.manager.SetCleanupEndCallback(callback)
}

// SetStatsUpdateCallback 设置统计更新回调
func (s *LifecycleService) SetStatsUpdateCallback(callback func(stats *CleanupStats)) {
	s.manager.SetStatsUpdateCallback(callback)
}
