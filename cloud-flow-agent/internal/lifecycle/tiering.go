// P7: 冷热数据分层存储引擎
package lifecycle

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// DataTier 数据分层类型
type DataTier string

const (
	TierHot  DataTier = "hot"   // 热数据: 最近7天, 内存/SSD, 高频查询
	TierWarm DataTier = "warm"  // 温数据: 7-30天, SSD, 中频查询
	TierCold DataTier = "cold"  // 冷数据: 30-90天, 归档存储, 低频查询
	TierFrozen DataTier = "frozen" // 冻结数据: >90天, 对象存储/磁带, 仅归档
)

// TieringPolicy 分层策略
type TieringPolicy struct {
	Category      DataCategory `json:"category"`
	Enabled       bool         `json:"enabled"`
	
	// 各层保留天数
	HotDays       int          `json:"hot_days"`   // 热数据保留天数(默认7)
	WarmDays      int          `json:"warm_days"`  // 温数据保留天数(默认30)
	ColdDays      int          `json:"cold_days"`  // 冷数据保留天数(默认90)
	FrozenDays    int          `json:"frozen_days"` // 冻结数据保留天数(默认365)
	
	// 各层存储介质
	HotStorage    string       `json:"hot_storage"`   // 热存储介质
	WarmStorage   string       `json:"warm_storage"`  // 温存储介质
	ColdStorage   string       `json:"cold_storage"`  // 冷存储介质
	FrozenStorage string       `json:"frozen_storage"` // 冻结存储介质
	
	// 迁移规则
	AutoMigrate   bool         `json:"auto_migrate"`  // 自动迁移
	MigrateWindow string       `json:"migrate_window"` // 迁移窗口(e.g. "02:00-06:00")
	MaxMigrateRate int64       `json:"max_migrate_rate"` // 每秒最大迁移条数
}

// DefaultTieringPolicy 返回默认分层策略
func DefaultTieringPolicy(category DataCategory) *TieringPolicy {
	return &TieringPolicy{
		Category:       category,
		Enabled:        true,
		HotDays:        7,
		WarmDays:       30,
		ColdDays:       90,
		FrozenDays:     365,
		HotStorage:     "memory",
		WarmStorage:    "ssd",
		ColdStorage:    "hdd",
		FrozenStorage:  "object_storage",
		AutoMigrate:    true,
		MigrateWindow:  "02:00-06:00",
		MaxMigrateRate: 50000,
	}
}

// Validate 验证策略
func (p *TieringPolicy) Validate() error {
	if p.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	if p.HotDays < 1 {
		p.HotDays = 7
	}
	if p.WarmDays <= p.HotDays {
		p.WarmDays = p.HotDays + 7
	}
	if p.ColdDays <= p.WarmDays {
		p.ColdDays = p.WarmDays + 30
	}
	if p.FrozenDays <= p.ColdDays {
		p.FrozenDays = p.ColdDays + 90
	}
	return nil
}

// GetTierForTime 根据时间判断数据所属层级
func (p *TieringPolicy) GetTierForTime(dataTime time.Time) DataTier {
	now := time.Now()
	age := now.Sub(dataTime)
	
	switch {
	case age <= time.Duration(p.HotDays)*24*time.Hour:
		return TierHot
	case age <= time.Duration(p.WarmDays)*24*time.Hour:
		return TierWarm
	case age <= time.Duration(p.ColdDays)*24*time.Hour:
		return TierCold
	default:
		return TierFrozen
	}
}

// GetCutoffTimeForTier 获取某层级的截止时间
func (p *TieringPolicy) GetCutoffTimeForTier(tier DataTier) time.Time {
	now := time.Now()
	switch tier {
	case TierHot:
		return now.AddDate(0, 0, -p.HotDays)
	case TierWarm:
		return now.AddDate(0, 0, -p.WarmDays)
	case TierCold:
		return now.AddDate(0, 0, -p.ColdDays)
	case TierFrozen:
		return now.AddDate(0, 0, -p.FrozenDays)
	default:
		return now.AddDate(0, 0, -p.HotDays)
	}
}

// GetStorageForTier 获取层级对应的存储介质
func (p *TieringPolicy) GetStorageForTier(tier DataTier) string {
	switch tier {
	case TierHot:
		return p.HotStorage
	case TierWarm:
		return p.WarmStorage
	case TierCold:
		return p.ColdStorage
	case TierFrozen:
		return p.FrozenStorage
	default:
		return p.HotStorage
	}
}

// ============================================================================
// 分层管理器
// ============================================================================

// TieringManager 分层管理器
type TieringManager struct {
	mu       sync.RWMutex
	policies map[DataCategory]*TieringPolicy
	
	// 迁移统计
	migrateStats map[DataCategory]*TierMigrateStats
	
	// 分层存储接口
	hotStore  func(category DataCategory, batch DataBatch) error
	warmStore func(category DataCategory, batch DataBatch) error
	coldStore func(category DataCategory, batch DataBatch) error
	frozenStore func(category DataCategory, batch DataBatch) error
	
	// 读取接口
	readFromTier func(tier DataTier, category DataCategory, start, end time.Time) (DataBatch, error)
}

// NewTieringManager 创建分层管理器
func NewTieringManager() *TieringManager {
	return &TieringManager{
		policies:     make(map[DataCategory]*TieringPolicy),
		migrateStats: make(map[DataCategory]*TierMigrateStats),
	}
}

// RegisterPolicy 注册分层策略
func (tm *TieringManager) RegisterPolicy(policy *TieringPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.policies[policy.Category] = policy
	return nil
}

// GetPolicy 获取策略
func (tm *TieringManager) GetPolicy(category DataCategory) *TieringPolicy {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.policies[category]
}

// GetTier 获取数据应属层级
func (tm *TieringManager) GetTier(category DataCategory, dataTime time.Time) DataTier {
	policy := tm.GetPolicy(category)
	if policy == nil {
		return TierHot // 默认热数据
	}
	return policy.GetTierForTime(dataTime)
}

// SetTierStore 设置各层存储接口
func (tm *TieringManager) SetTierStore(tier DataTier, storeFn func(category DataCategory, batch DataBatch) error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	switch tier {
	case TierHot:
		tm.hotStore = storeFn
	case TierWarm:
		tm.warmStore = storeFn
	case TierCold:
		tm.coldStore = storeFn
	case TierFrozen:
		tm.frozenStore = storeFn
	}
}

// SetReadFromTier 设置从层级读取接口
func (tm *TieringManager) SetReadFromTier(fn func(tier DataTier, category DataCategory, start, end time.Time) (DataBatch, error)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.readFromTier = fn
}

// MigrateData 执行数据迁移
func (tm *TieringManager) MigrateData(category DataCategory, fromTier, toTier DataTier, batch DataBatch) error {
	tm.mu.Lock()
	storeFn := tm.hotStore
	switch toTier {
	case TierHot:
		storeFn = tm.hotStore
	case TierWarm:
		storeFn = tm.warmStore
	case TierCold:
		storeFn = tm.coldStore
	case TierFrozen:
		storeFn = tm.frozenStore
	}
	tm.mu.Unlock()
	
	if storeFn == nil {
		return fmt.Errorf("storage not configured for tier %s", toTier)
	}
	
	if err := storeFn(category, batch); err != nil {
		return fmt.Errorf("migrate to %s failed: %w", toTier, err)
	}
	
	// 更新统计
	tm.updateMigrateStats(category, fromTier, toTier, batch.Count, batch.Size)
	
	return nil
}

// AutoMigrate 自动迁移过期数据
func (tm *TieringManager) AutoMigrate(category DataCategory, scanner DataScanner) (*MigrateResult, error) {
	result := &MigrateResult{Category: category}
	
	policy := tm.GetPolicy(category)
	if policy == nil || !policy.Enabled || !policy.AutoMigrate {
		return result, nil
	}
	
	// 热->温
	warmCutoff := policy.GetCutoffTimeForTier(TierWarm)
	if err := tm.migrateTier(scanner, category, TierHot, TierWarm, warmCutoff, result); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("hot->warm: %v", err))
	}
	
	// 温->冷
	coldCutoff := policy.GetCutoffTimeForTier(TierCold)
	if err := tm.migrateTier(scanner, category, TierWarm, TierCold, coldCutoff, result); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("warm->cold: %v", err))
	}
	
	// 冷->冻结
	frozenCutoff := policy.GetCutoffTimeForTier(TierFrozen)
	if err := tm.migrateTier(scanner, category, TierCold, TierFrozen, frozenCutoff, result); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("cold->frozen: %v", err))
	}
	
	return result, nil
}

func (tm *TieringManager) migrateTier(scanner DataScanner, category DataCategory, fromTier, toTier DataTier, cutoff time.Time, result *MigrateResult) error {
	if scanner == nil {
		return fmt.Errorf("scanner not set")
	}
	
	var totalCount, totalSize int64
	err := scanner.ScanExpired(nil, cutoff, category, func(batch DataBatch) bool {
		if err := tm.MigrateData(category, fromTier, toTier, batch); err != nil {
			return false
		}
		totalCount += batch.Count
		totalSize += batch.Size
		return true
	})
	
	if totalCount > 0 {
		result.Migrations = append(result.Migrations, TierMigration{
			FromTier:  fromTier,
			ToTier:    toTier,
			Count:     totalCount,
			Size:      totalSize,
			Timestamp: time.Now(),
		})
		result.TotalCount += totalCount
		result.TotalSize += totalSize
	}
	
	return err
}

func (tm *TieringManager) updateMigrateStats(category DataCategory, fromTier, toTier DataTier, count, size int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	stats, ok := tm.migrateStats[category]
	if !ok {
		stats = &TierMigrateStats{Category: category, MigrationsByTier: make(map[string]int64)}
		tm.migrateStats[category] = stats
	}
	
	stats.LastMigrate = time.Now()
	stats.TotalMigrated += count
	stats.TotalSize += size
	stats.MigrationsByTier[string(fromTier)+"->"+string(toTier)] += count
}

// GetMigrateStats 获取迁移统计
func (tm *TieringManager) GetMigrateStats(category DataCategory) *TierMigrateStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.migrateStats[category]
}

// GetAllMigrateStats 获取所有迁移统计
func (tm *TieringManager) GetAllMigrateStats() []*TierMigrateStats {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	var stats []*TierMigrateStats
	for _, s := range tm.migrateStats {
		stats = append(stats, s)
	}
	
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Category < stats[j].Category
	})
	
	return stats
}

// ============================================================================
// 迁移结果
// ============================================================================

// MigrateResult 迁移结果
type MigrateResult struct {
	Category     DataCategory
	Migrations   []TierMigration
	TotalCount   int64
	TotalSize    int64
	Errors       []string
}

// TierMigration 单次迁移记录
type TierMigration struct {
	FromTier  DataTier
	ToTier    DataTier
	Count     int64
	Size      int64
	Timestamp time.Time
}

// TierMigrateStats 层级迁移统计
type TierMigrateStats struct {
	Category        DataCategory
	TotalMigrated   int64
	TotalSize       int64
	LastMigrate     time.Time
	MigrationsByTier map[string]int64
}

// ============================================================================
// 查询路由
// ============================================================================

// QueryRouter 查询路由，根据时间范围选择正确的层级
type QueryRouter struct {
	tm *TieringManager
}

// NewQueryRouter 创建查询路由
func NewQueryRouter(tm *TieringManager) *QueryRouter {
	return &QueryRouter{tm: tm}
}

// RouteQuery 根据查询时间范围选择应查询的层级
func (qr *QueryRouter) RouteQuery(category DataCategory, startTime, endTime time.Time) []DataTier {
	var tiers []DataTier
	now := time.Now()
	
	policy := qr.tm.GetPolicy(category)
	if policy == nil {
		return []DataTier{TierHot}
	}
	
	// 如果查询范围包含最近7天，需要查热数据
	if endTime.After(now.AddDate(0, 0, -policy.HotDays)) {
		tiers = append(tiers, TierHot)
	}
	
	// 如果查询范围包含7-30天，需要查温数据
	if startTime.Before(now.AddDate(0, 0, -policy.HotDays)) && endTime.After(now.AddDate(0, 0, -policy.WarmDays)) {
		tiers = append(tiers, TierWarm)
	}
	
	// 如果查询范围包含30-90天，需要查冷数据
	if startTime.Before(now.AddDate(0, 0, -policy.WarmDays)) && endTime.After(now.AddDate(0, 0, -policy.ColdDays)) {
		tiers = append(tiers, TierCold)
	}
	
	// 如果查询范围超过90天，需要查冻结数据
	if startTime.Before(now.AddDate(0, 0, -policy.ColdDays)) {
		tiers = append(tiers, TierFrozen)
	}
	
	return tiers
}

// GetEstimatedQueryCost 估算查询成本（从哪个层级查数据更慢）
func (qr *QueryRouter) GetEstimatedQueryCost(tier DataTier) int {
	switch tier {
	case TierHot:
		return 1  // 最快，内存
	case TierWarm:
		return 3  // SSD，快
	case TierCold:
		return 10 // HDD，较慢
	case TierFrozen:
		return 50 // 对象存储，慢
	default:
		return 1
	}
}

// ============================================================================
// 分层状态监控
// ============================================================================

// TierStatus 分层状态
type TierStatus struct {
	Category    DataCategory
	Tier        DataTier
	RecordCount int64
	DataSize    int64
	OldestTime  time.Time
	NewestTime  time.Time
	Storage     string
}

// TierMonitor 分层监控器
type TierMonitor struct {
	mu      sync.RWMutex
	status  map[string]*TierStatus // key: category:tier
}

// NewTierMonitor 创建分层监控器
func NewTierMonitor() *TierMonitor {
	return &TierMonitor{
		status: make(map[string]*TierStatus),
	}
}

// UpdateStatus 更新层级状态
func (m *TierMonitor) UpdateStatus(category DataCategory, tier DataTier, count, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	key := fmt.Sprintf("%s:%s", category, tier)
	status, ok := m.status[key]
	if !ok {
		status = &TierStatus{Category: category, Tier: tier}
		m.status[key] = status
	}
	
	status.RecordCount = count
	status.DataSize = size
	status.NewestTime = time.Now()
	if status.OldestTime.IsZero() {
		status.OldestTime = time.Now()
	}
}

// GetStatus 获取层级状态
func (m *TierMonitor) GetStatus(category DataCategory, tier DataTier) *TierStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", category, tier)
	return m.status[key]
}

// GetAllStatus 获取所有层级状态
func (m *TierMonitor) GetAllStatus() []*TierStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var statuses []*TierStatus
	for _, s := range m.status {
		statuses = append(statuses, s)
	}
	
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Category != statuses[j].Category {
			return statuses[i].Category < statuses[j].Category
		}
		return statuses[i].Tier < statuses[j].Tier
	})
	
	return statuses
}

// GetCategorySize 获取某分类总数据大小
func (m *TierMonitor) GetCategorySize(category DataCategory) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var total int64
	for key, status := range m.status {
		if strings.HasPrefix(key, string(category)+":") {
			total += status.DataSize
		}
	}
	return total
}
