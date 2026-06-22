// P2: 告警历史与统计分析 — MTTR、告警趋势、TopN、告警频率
//
// 功能：
//   - 告警历史记录和查询
//   - MTTR（平均恢复时间）计算
//   - 告警趋势分析（按时间/规则/租户）
//   - 告警 TopN（最频繁告警规则）
//   - 告警频率统计
//
package alerting

import (
	"context"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 一、告警历史记录
// ============================================================================

// AlertHistoryRecord 告警历史记录
type AlertHistoryRecord struct {
	AlertID     string            `json:"alert_id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	Severity    string            `json:"severity"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
	TriggeredAt time.Time         `json:"triggered_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	Duration    time.Duration     `json:"duration"` // 持续时间（resolved 时计算）
	Acknowledged bool             `json:"acknowledged"`
	Channels    []string          `json:"channels"` // 通知通道
}

// IsResolved 检查告警是否已解决
func (r *AlertHistoryRecord) IsResolved() bool {
	return r.ResolvedAt != nil
}

// GetDuration 获取告警持续时间
func (r *AlertHistoryRecord) GetDuration() time.Duration {
	if r.ResolvedAt != nil {
		return r.ResolvedAt.Sub(r.TriggeredAt)
	}
	return time.Since(r.TriggeredAt)
}

// ============================================================================
// 二、告警历史存储
// ============================================================================

// HistoryStore 告警历史存储接口
type HistoryStore interface {
	Save(record *AlertHistoryRecord) error
	Query(ctx context.Context, filter *HistoryFilter) ([]*AlertHistoryRecord, error)
	Count(ctx context.Context, filter *HistoryFilter) (int, error)
	GetStats(ctx context.Context, start, end time.Time) (*AlertStats, error)
}

// MemoryHistoryStore 内存告警历史存储（用于测试和小规模场景）
type MemoryHistoryStore struct {
	records []*AlertHistoryRecord
	mu      sync.RWMutex
	maxSize int
}

// NewMemoryHistoryStore 创建内存历史存储
func NewMemoryHistoryStore(maxSize int) *MemoryHistoryStore {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &MemoryHistoryStore{
		records: make([]*AlertHistoryRecord, 0, maxSize),
		maxSize: maxSize,
	}
}

// Save 保存告警记录
func (s *MemoryHistoryStore) Save(record *AlertHistoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果记录已存在，更新它
	for i, r := range s.records {
		if r.AlertID == record.AlertID {
			s.records[i] = record
			return nil
		}
	}

	// 新增记录
	s.records = append(s.records, record)
	if len(s.records) > s.maxSize {
		s.records = s.records[len(s.records)-s.maxSize:]
	}
	return nil
}

// Query 查询告警历史
func (s *MemoryHistoryStore) Query(ctx context.Context, filter *HistoryFilter) ([]*AlertHistoryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AlertHistoryRecord, 0)
	for _, record := range s.records {
		if filter.Match(record) {
			result = append(result, record)
		}
	}

	// 排序
	if filter.SortBy == "triggered_at" {
		if filter.SortOrder == "desc" {
			sort.Slice(result, func(i, j int) bool {
				return result[i].TriggeredAt.After(result[j].TriggeredAt)
			})
		} else {
			sort.Slice(result, func(i, j int) bool {
				return result[i].TriggeredAt.Before(result[j].TriggeredAt)
			})
		}
	}

	// 分页
	if filter.Offset > 0 {
		if filter.Offset >= len(result) {
			return []*AlertHistoryRecord{}, nil
		}
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}

// Count 统计数量
func (s *MemoryHistoryStore) Count(ctx context.Context, filter *HistoryFilter) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, record := range s.records {
		if filter.Match(record) {
			count++
		}
	}
	return count, nil
}

// GetStats 获取统计信息
func (s *MemoryHistoryStore) GetStats(ctx context.Context, start, end time.Time) (*AlertStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := NewAlertStats()
	for _, record := range s.records {
		if record.TriggeredAt.Before(start) || record.TriggeredAt.After(end) {
			continue
		}
		stats.AddRecord(record)
	}
	return stats, nil
}

// HistoryFilter 历史查询过滤器
type HistoryFilter struct {
	RuleID      string
	Severity    string
	StartTime   *time.Time
	EndTime     *time.Time
	Resolved    *bool
	Acknowledged *bool
	Offset      int
	Limit       int
	SortBy      string
	SortOrder   string
}

// Match 检查记录是否匹配过滤器
func (f *HistoryFilter) Match(record *AlertHistoryRecord) bool {
	if f.RuleID != "" && record.RuleID != f.RuleID {
		return false
	}
	if f.Severity != "" && record.Severity != f.Severity {
		return false
	}
	if f.StartTime != nil && record.TriggeredAt.Before(*f.StartTime) {
		return false
	}
	if f.EndTime != nil && record.TriggeredAt.After(*f.EndTime) {
		return false
	}
	if f.Resolved != nil {
		if *f.Resolved != record.IsResolved() {
			return false
		}
	}
	if f.Acknowledged != nil && *f.Acknowledged != record.Acknowledged {
		return false
	}
	return true
}

// ============================================================================
// 三、告警统计
// ============================================================================

// AlertStats 告警统计
type AlertStats struct {
	TotalCount       int                       `json:"total_count"`
	ResolvedCount    int                       `json:"resolved_count"`
	UnresolvedCount  int                       `json:"unresolved_count"`
	AcknowledgedCount int                      `json:"acknowledged_count"`
	AvgDuration      time.Duration             `json:"avg_duration"`
	MaxDuration      time.Duration             `json:"max_duration"`
	MinDuration      time.Duration             `json:"min_duration"`
	MTTR             time.Duration             `json:"mttr"` // Mean Time To Resolution
	ByRule           map[string]int            `json:"by_rule"`
	BySeverity       map[string]int            `json:"by_severity"`
	ByHour           map[int]int               `json:"by_hour"`       // 每小时告警数
	ByDay            map[string]int            `json:"by_day"`        // 每日告警数
	TopRules         []RuleAlertCount          `json:"top_rules"`
	Trend            []TimePointStats          `json:"trend"`         // 趋势数据
}

// RuleAlertCount 规则告警计数
type RuleAlertCount struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Count    int    `json:"count"`
}

// TimePointStats 时间点统计
type TimePointStats struct {
	Time  time.Time `json:"time"`
	Count int       `json:"count"`
}

// NewAlertStats 创建告警统计
func NewAlertStats() *AlertStats {
	return &AlertStats{
		ByRule:     make(map[string]int),
		BySeverity: make(map[string]int),
		ByHour:     make(map[int]int),
		ByDay:      make(map[string]int),
		MinDuration: time.Duration(1<<63 - 1),
	}
}

// AddRecord 添加记录到统计
func (s *AlertStats) AddRecord(record *AlertHistoryRecord) {
	s.TotalCount++
	s.ByRule[record.RuleID]++
	s.BySeverity[record.Severity]++
	s.ByHour[record.TriggeredAt.Hour()]++
	s.ByDay[record.TriggeredAt.Format("2006-01-02")]++

	if record.Acknowledged {
		s.AcknowledgedCount++
	}

	if record.IsResolved() {
		s.ResolvedCount++
		duration := record.GetDuration()
		s.AvgDuration = (s.AvgDuration*time.Duration(s.ResolvedCount-1) + duration) / time.Duration(s.ResolvedCount)
		if duration > s.MaxDuration {
			s.MaxDuration = duration
		}
		if duration < s.MinDuration {
			s.MinDuration = duration
		}
	} else {
		s.UnresolvedCount++
	}
}

// Finalize 完成统计计算
func (s *AlertStats) Finalize() {
	if s.ResolvedCount > 0 {
		s.MTTR = s.AvgDuration
	} else {
		s.MinDuration = 0
	}

	// 计算 TopRules
	type ruleCount struct {
		id    string
		name  string
		count int
	}
	var counts []ruleCount
	for id, count := range s.ByRule {
		counts = append(counts, ruleCount{id: id, count: count})
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})

	maxTop := 10
	if len(counts) < maxTop {
		maxTop = len(counts)
	}
	s.TopRules = make([]RuleAlertCount, 0, maxTop)
	for i := 0; i < maxTop; i++ {
		s.TopRules = append(s.TopRules, RuleAlertCount{
			RuleID:   counts[i].id,
			RuleName: counts[i].name,
			Count:    counts[i].count,
		})
	}
}

// ============================================================================
// 四、告警历史管理器
// ============================================================================

// HistoryManager 告警历史管理器
type HistoryManager struct {
	store   HistoryStore
	mu      sync.RWMutex
}

// NewHistoryManager 创建历史管理器
func NewHistoryManager(store HistoryStore) *HistoryManager {
	return &HistoryManager{
		store: store,
	}
}

// Record 记录告警
func (hm *HistoryManager) Record(alert *Alert, channels []string) error {
	record := &AlertHistoryRecord{
		AlertID:     alert.ID,
		RuleID:      alert.RuleID,
		RuleName:    alert.RuleName,
		Severity:    alert.Severity,
		Message:     alert.Message,
		Labels:      alert.Labels,
		Value:       alert.Value,
		Threshold:   alert.Threshold,
		TriggeredAt: alert.CreatedAt,
		Channels:    channels,
	}
	return hm.store.Save(record)
}

// RecordResolved 记录告警解决
func (hm *HistoryManager) RecordResolved(alertID string) error {
	// 查询现有记录
	records, err := hm.store.Query(context.Background(), &HistoryFilter{Offset: 0, Limit: 1000})
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.AlertID == alertID && !record.IsResolved() {
			now := time.Now()
			record.ResolvedAt = &now
			record.Duration = record.GetDuration()
			return hm.store.Save(record)
		}
	}
	return nil
}

// RecordAcknowledged 记录告警确认
func (hm *HistoryManager) RecordAcknowledged(alertID string) error {
	records, err := hm.store.Query(context.Background(), &HistoryFilter{Offset: 0, Limit: 1000})
	if err != nil {
		return err
	}

	for _, record := range records {
		if record.AlertID == alertID {
			record.Acknowledged = true
			return hm.store.Save(record)
		}
	}
	return nil
}

// Query 查询告警历史
func (hm *HistoryManager) Query(ctx context.Context, filter *HistoryFilter) ([]*AlertHistoryRecord, error) {
	return hm.store.Query(ctx, filter)
}

// GetStats 获取统计信息
func (hm *HistoryManager) GetStats(ctx context.Context, start, end time.Time) (*AlertStats, error) {
	return hm.store.GetStats(ctx, start, end)
}

// GetTrend 获取告警趋势
func (hm *HistoryManager) GetTrend(ctx context.Context, start, end time.Time, interval time.Duration) ([]TimePointStats, error) {
	records, err := hm.store.Query(ctx, &HistoryFilter{
		StartTime: &start,
		EndTime:   &end,
	})
	if err != nil {
		return nil, err
	}

	// 按时间区间聚合
	buckets := make(map[int64]int)
	for _, record := range records {
		bucket := record.TriggeredAt.Unix() / int64(interval.Seconds()) * int64(interval.Seconds())
		buckets[bucket]++
	}

	// 转换为有序结果
	var times []int64
	for t := range buckets {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	result := make([]TimePointStats, 0, len(times))
	for _, t := range times {
		result = append(result, TimePointStats{
			Time:  time.Unix(t, 0),
			Count: buckets[t],
		})
	}
	return result, nil
}

// GetTopRules 获取最频繁的告警规则
func (hm *HistoryManager) GetTopRules(ctx context.Context, start, end time.Time, limit int) ([]RuleAlertCount, error) {
	stats, err := hm.GetStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	stats.Finalize()

	if limit <= 0 || limit > len(stats.TopRules) {
		limit = len(stats.TopRules)
	}
	return stats.TopRules[:limit], nil
}

// GetMTTR 获取 MTTR
func (hm *HistoryManager) GetMTTR(ctx context.Context, start, end time.Time) (time.Duration, error) {
	stats, err := hm.GetStats(ctx, start, end)
	if err != nil {
		return 0, err
	}
	stats.Finalize()
	return stats.MTTR, nil
}

// GetHourlyDistribution 获取每小时告警分布
func (hm *HistoryManager) GetHourlyDistribution(ctx context.Context, start, end time.Time) (map[int]int, error) {
	stats, err := hm.GetStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return stats.ByHour, nil
}

// GetDailyDistribution 获取每日告警分布
func (hm *HistoryManager) GetDailyDistribution(ctx context.Context, start, end time.Time) (map[string]int, error) {
	stats, err := hm.GetStats(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return stats.ByDay, nil
}
