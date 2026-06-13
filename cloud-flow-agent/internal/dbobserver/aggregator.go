// Package dbobserver 提供数据库观测功能
// 本文件实现 SQL 聚合统计引擎
package dbobserver

import (
	"container/heap"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ==================== SQL 指纹生成 ====================

// SQLFingerprintGenerator SQL 指纹生成器
type SQLFingerprintGenerator struct {
	// 数字常量替换正则
	numberRegex *regexp.Regexp
	// 单引号字符串替换正则
	singleQuoteRegex *regexp.Regexp
	// 双引号字符串替换正则
	doubleQuoteRegex *regexp.Regexp
	// IN 列表替换正则
	inListRegex *regexp.Regexp
	// 多空白替换正则
	multiSpaceRegex *regexp.Regexp
	// 注释替换正则
	commentRegex *regexp.Regexp
}

// NewSQLFingerprintGenerator 创建 SQL 指纹生成器
func NewSQLFingerprintGenerator() *SQLFingerprintGenerator {
	return &SQLFingerprintGenerator{
		numberRegex:      regexp.MustCompile(`\b\d+\.?\d*\b`),
		singleQuoteRegex: regexp.MustCompile(`'[^']*'`),
		doubleQuoteRegex: regexp.MustCompile(`"[^"]*"`),
		inListRegex:      regexp.MustCompile(`\bIN\s*\([^)]+\)`),
		multiSpaceRegex:  regexp.MustCompile(`\s+`),
		commentRegex:     regexp.MustCompile(`--.*?$|/\*.*?\*/`),
	}
}

// Generate 生成 SQL 指纹
func (g *SQLFingerprintGenerator) Generate(sql string) string {
	// 转大写统一
	result := strings.ToUpper(sql)

	// 移除注释
	result = g.commentRegex.ReplaceAllString(result, "")

	// 替换字符串常量
	result = g.singleQuoteRegex.ReplaceAllString(result, "'?'")
	result = g.doubleQuoteRegex.ReplaceAllString(result, "\"?\"")

	// 替换 IN 列表
	result = g.inListRegex.ReplaceAllString(result, "IN (?)")

	// 替换数字常量
	result = g.numberRegex.ReplaceAllString(result, "?")

	// 合并多个空白
	result = g.multiSpaceRegex.ReplaceAllString(result, " ")

	// 去除首尾空白
	result = strings.TrimSpace(result)

	return result
}

// GenerateTemplate 生成 SQL 模板（保留参数位置）
func (g *SQLFingerprintGenerator) GenerateTemplate(sql string) string {
	// 与指纹类似，但保留更多结构信息
	return g.Generate(sql)
}

// ==================== 时延百分位计算 ====================

// DurationHeap 时延堆（用于计算百分位）
type DurationHeap []int64

func (h DurationHeap) Len() int           { return len(h) }
func (h DurationHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h DurationHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *DurationHeap) Push(x interface{}) {
	*h = append(*h, x.(int64))
}

func (h *DurationHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// PercentileCalculator 百分位计算器
type PercentileCalculator struct {
	durations []int64
	sorted    bool
	mu        sync.RWMutex
}

// NewPercentileCalculator 创建百分位计算器
func NewPercentileCalculator() *PercentileCalculator {
	return &PercentileCalculator{
		durations: make([]int64, 0),
		sorted:    false,
	}
}

// Add 添加时延值
func (p *PercentileCalculator) Add(duration int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.durations = append(p.durations, duration)
	p.sorted = false
}

// AddBatch 批量添加时延值
func (p *PercentileCalculator) AddBatch(durations []int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.durations = append(p.durations, durations...)
	p.sorted = false
}

// Calculate 计算百分位值
func (p *PercentileCalculator) Calculate(percentile float64) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.durations) == 0 {
		return 0
	}

	// 排序
	if !p.sorted {
		sort.Slice(p.durations, func(i, j int) bool {
			return p.durations[i] < p.durations[j]
		})
		p.sorted = true
	}

	// 计算百分位位置
	n := len(p.durations)
	index := int(float64(n-1) * percentile / 100.0)
	if index >= n {
		index = n - 1
	}

	return p.durations[index]
}

// CalculateP50 计算 P50
func (p *PercentileCalculator) CalculateP50() int64 {
	return p.Calculate(50)
}

// CalculateP90 计算 P90
func (p *PercentileCalculator) CalculateP90() int64 {
	return p.Calculate(90)
}

// CalculateP99 计算 P99
func (p *PercentileCalculator) CalculateP99() int64 {
	return p.Calculate(99)
}

// Reset 重置
func (p *PercentileCalculator) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.durations = make([]int64, 0)
	p.sorted = false
}

// Count 返回样本数量
func (p *PercentileCalculator) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.durations)
}

// ==================== SQL 统计条目 ====================

// SQLStatEntry SQL 统计条目（内部使用）
type SQLStatEntry struct {
	// SQL 标识
	SQLFingerprint string
	SQLTemplate    string
	SQLType        SQLType
	DatabaseType   DatabaseType
	DatabaseName   string

	// 请求统计
	RequestCount  int64
	SuccessCount  int64
	ErrorCount    int64

	// 时延统计
	TotalDuration int64
	MinDuration   int64
	MaxDuration   int64
	Durations     *PercentileCalculator

	// 数据量统计
	TotalRowsAffected  int64
	TotalRowsReturned  int64
	TotalBytesSent     int64
	TotalBytesReceived int64

	// 时间窗口
	FirstSeen      time.Time
	LastSeen       time.Time
	LastUpdateTime time.Time

	// 错误统计
	ErrorCodes map[string]int64

	// 关联进程
	ProcessPID  uint32
	ProcessName string

	// 并发控制
	mu sync.RWMutex
}

// NewSQLStatEntry 创建 SQL 统计条目
func NewSQLStatEntry(fingerprint, template string) *SQLStatEntry {
	return &SQLStatEntry{
		SQLFingerprint: fingerprint,
		SQLTemplate:    template,
		MinDuration:    int64(^uint64(0) >> 1), // 最大值
		Durations:      NewPercentileCalculator(),
		ErrorCodes:     make(map[string]int64),
		FirstSeen:      time.Now(),
		LastSeen:       time.Now(),
		LastUpdateTime: time.Now(),
	}
}

// Update 更新统计
func (e *SQLStatEntry) Update(event *SQLEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 更新请求统计
	e.RequestCount++
	if event.Success {
		e.SuccessCount++
	} else {
		e.ErrorCount++
		if event.ErrorCode != "" {
			e.ErrorCodes[event.ErrorCode]++
		}
	}

	// 更新时延统计
	e.TotalDuration += event.Duration
	if event.Duration < e.MinDuration {
		e.MinDuration = event.Duration
	}
	if event.Duration > e.MaxDuration {
		e.MaxDuration = event.Duration
	}
	e.Durations.Add(event.Duration)

	// 更新数据量统计
	e.TotalRowsAffected += event.RowsAffected
	e.TotalRowsReturned += event.RowsReturned
	e.TotalBytesSent += event.BytesSent
	e.TotalBytesReceived += event.BytesReceived

	// 更新时间
	e.LastSeen = time.Now()
	e.LastUpdateTime = time.Now()

	// 更新关联信息
	if event.PID > 0 {
		e.ProcessPID = event.PID
		e.ProcessName = event.ProcessName
	}
}

// ToSQLStats 转换为 SQLStats
func (e *SQLStatEntry) ToSQLStats() *SQLStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := &SQLStats{
		SQLFingerprint:     e.SQLFingerprint,
		SQLTemplate:        e.SQLTemplate,
		SQLType:            e.SQLType,
		DatabaseType:       e.DatabaseType,
		DatabaseName:       e.DatabaseName,
		RequestCount:       e.RequestCount,
		SuccessCount:       e.SuccessCount,
		ErrorCount:         e.ErrorCount,
		TotalDuration:      e.TotalDuration,
		MinDuration:        e.MinDuration,
		MaxDuration:        e.MaxDuration,
		TotalRowsAffected:  e.TotalRowsAffected,
		TotalRowsReturned:  e.TotalRowsReturned,
		TotalBytesSent:     e.TotalBytesSent,
		TotalBytesReceived: e.TotalBytesReceived,
		FirstSeen:          e.FirstSeen,
		LastSeen:           e.LastSeen,
		LastUpdateTime:     e.LastUpdateTime,
		ErrorCodes:         make(map[string]int64),
		ProcessPID:         e.ProcessPID,
		ProcessName:        e.ProcessName,
	}

	// 计算成功率
	if stats.RequestCount > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.RequestCount)
		stats.AvgDuration = float64(stats.TotalDuration) / float64(stats.RequestCount)
	}

	// 计算百分位时延
	stats.P50Duration = e.Durations.CalculateP50()
	stats.P90Duration = e.Durations.CalculateP90()
	stats.P99Duration = e.Durations.CalculateP99()

	// 复制错误码
	for k, v := range e.ErrorCodes {
		stats.ErrorCodes[k] = v
	}

	return stats
}

// Reset 重置统计
func (e *SQLStatEntry) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.RequestCount = 0
	e.SuccessCount = 0
	e.ErrorCount = 0
	e.TotalDuration = 0
	e.MinDuration = int64(^uint64(0) >> 1)
	e.MaxDuration = 0
	e.Durations.Reset()
	e.TotalRowsAffected = 0
	e.TotalRowsReturned = 0
	e.TotalBytesSent = 0
	e.TotalBytesReceived = 0
	e.ErrorCodes = make(map[string]int64)
	e.LastUpdateTime = time.Now()
}

// ==================== SQL 聚合器 ====================

// SQLAggregator SQL 聚合器
type SQLAggregator struct {
	config       *ObserverConfig
	fingerprint  *SQLFingerprintGenerator
	entries      map[string]*SQLStatEntry // fingerprint -> entry
	totalEntries int
	mu           sync.RWMutex

	// 回调函数
	onFlush func([]*SQLStats)
}

// NewSQLAggregator 创建 SQL 聚合器
func NewSQLAggregator(cfg *ObserverConfig) *SQLAggregator {
	return &SQLAggregator{
		config:      cfg,
		fingerprint: NewSQLFingerprintGenerator(),
		entries:     make(map[string]*SQLStatEntry),
	}
}

// Record 记录 SQL 事件
func (a *SQLAggregator) Record(event *SQLEvent) {
	// 生成 SQL 指纹
	fingerprint := a.fingerprint.Generate(event.SQLText)
	template := a.fingerprint.GenerateTemplate(event.SQLText)

	// 更新事件中的指纹
	event.SQLFingerprint = fingerprint
	event.SQLTemplate = template

	a.mu.Lock()
	defer a.mu.Unlock()

	// 检查是否超过最大条目数
	if a.totalEntries >= a.config.MaxAggregatedSQLs {
		// 执行 LRU 淘汰
		a.evictLRU()
	}

	// 获取或创建条目
	entry, exists := a.entries[fingerprint]
	if !exists {
		entry = NewSQLStatEntry(fingerprint, template)
		entry.SQLType = event.SQLType
		entry.DatabaseType = event.DatabaseType
		entry.DatabaseName = event.DatabaseName
		a.entries[fingerprint] = entry
		a.totalEntries++
	}

	// 更新统计
	entry.Update(event)
}

// evictLRU 淘汰最久未使用的条目
func (a *SQLAggregator) evictLRU() {
	// 找到最久未更新的条目
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range a.entries {
		if oldestKey == "" || entry.LastUpdateTime.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastUpdateTime
		}
	}

	// 删除最旧的条目
	if oldestKey != "" {
		delete(a.entries, oldestKey)
		a.totalEntries--
	}
}

// GetStats 获取所有统计
func (a *SQLAggregator) GetStats() map[string]*SQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]*SQLStats)
	for key, entry := range a.entries {
		result[key] = entry.ToSQLStats()
	}

	return result
}

// GetTopByRequestCount 按请求次数获取 Top N
func (a *SQLAggregator) GetTopByRequestCount(n int) []*SQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 收集所有条目
	entries := make([]*SQLStatEntry, 0, len(a.entries))
	for _, entry := range a.entries {
		entries = append(entries, entry)
	}

	// 按请求次数排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RequestCount > entries[j].RequestCount
	})

	// 取 Top N
	result := make([]*SQLStats, 0, n)
	for i := 0; i < n && i < len(entries); i++ {
		result = append(result, entries[i].ToSQLStats())
	}

	return result
}

// GetTopByAvgDuration 按平均时延获取 Top N
func (a *SQLAggregator) GetTopByAvgDuration(n int) []*SQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 收集所有条目
	entries := make([]*SQLStatEntry, 0, len(a.entries))
	for _, entry := range a.entries {
		entries = append(entries, entry)
	}

	// 按平均时延排序
	sort.Slice(entries, func(i, j int) bool {
		avgI := float64(0)
		avgJ := float64(0)
		if entries[i].RequestCount > 0 {
			avgI = float64(entries[i].TotalDuration) / float64(entries[i].RequestCount)
		}
		if entries[j].RequestCount > 0 {
			avgJ = float64(entries[j].TotalDuration) / float64(entries[j].RequestCount)
		}
		return avgI > avgJ
	})

	// 取 Top N
	result := make([]*SQLStats, 0, n)
	for i := 0; i < n && i < len(entries); i++ {
		result = append(result, entries[i].ToSQLStats())
	}

	return result
}

// GetTopByErrorCount 按错误数获取 Top N
func (a *SQLAggregator) GetTopByErrorCount(n int) []*SQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 收集所有条目
	entries := make([]*SQLStatEntry, 0, len(a.entries))
	for _, entry := range a.entries {
		entries = append(entries, entry)
	}

	// 按错误数排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ErrorCount > entries[j].ErrorCount
	})

	// 取 Top N
	result := make([]*SQLStats, 0, n)
	for i := 0; i < n && i < len(entries); i++ {
		result = append(result, entries[i].ToSQLStats())
	}

	return result
}

// Flush 刷新统计数据
func (a *SQLAggregator) Flush() []*SQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 收集所有统计
	stats := make([]*SQLStats, 0, len(a.entries))
	for _, entry := range a.entries {
		stats = append(stats, entry.ToSQLStats())
	}

	// 调用回调
	if a.onFlush != nil {
		a.onFlush(stats)
	}

	return stats
}

// Reset 重置所有统计
func (a *SQLAggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, entry := range a.entries {
		entry.Reset()
	}
}

// Clear 清空所有条目
func (a *SQLAggregator) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.entries = make(map[string]*SQLStatEntry)
	a.totalEntries = 0
}

// SetFlushCallback 设置刷新回调
func (a *SQLAggregator) SetFlushCallback(callback func([]*SQLStats)) {
	a.onFlush = callback
}

// Close 关闭聚合器
func (a *SQLAggregator) Close() {
	a.Clear()
}

// ==================== SQL 类型识别 ====================

// SQLTypeIdentifier SQL 类型识别器
type SQLTypeIdentifier struct {
	patterns map[SQLType]*regexp.Regexp
}

// NewSQLTypeIdentifier 创建 SQL 类型识别器
func NewSQLTypeIdentifier() *SQLTypeIdentifier {
	return &SQLTypeIdentifier{
		patterns: map[SQLType]*regexp.Regexp{
			SQLTypeSelect: regexp.MustCompile(`(?i)^\s*SELECT\b`),
			SQLTypeInsert: regexp.MustCompile(`(?i)^\s*INSERT\b`),
			SQLTypeUpdate: regexp.MustCompile(`(?i)^\s*UPDATE\b`),
			SQLTypeDelete: regexp.MustCompile(`(?i)^\s*DELETE\b`),
			SQLTypeDDL: regexp.MustCompile(`(?i)^\s*(CREATE|ALTER|DROP|TRUNCATE|RENAME)\b`),
			SQLTypeDCL: regexp.MustCompile(`(?i)^\s*(GRANT|REVOKE)\b`),
		},
	}
}

// Identify 识别 SQL 类型
func (i *SQLTypeIdentifier) Identify(sql string) SQLType {
	for sqlType, pattern := range i.patterns {
		if pattern.MatchString(sql) {
			return sqlType
		}
	}
	return SQLTypeOther
}

// ==================== 滑动窗口聚合 ====================

// SlidingWindowStats 滑动窗口统计
type SlidingWindowStats struct {
	windowSize   time.Duration
	buckets      []*TimeBucket
	bucketCount  int
	currentIndex int
	mu           sync.RWMutex
}

// TimeBucket 时间桶
type TimeBucket struct {
	StartTime time.Time
	EndTime   time.Time
	Entries   map[string]*SQLStatEntry
}

// NewSlidingWindowStats 创建滑动窗口统计
func NewSlidingWindowStats(windowSize time.Duration, bucketCount int) *SlidingWindowStats {
	bucketDuration := windowSize / time.Duration(bucketCount)
	buckets := make([]*TimeBucket, bucketCount)
	now := time.Now()

	for i := 0; i < bucketCount; i++ {
		buckets[i] = &TimeBucket{
			StartTime: now.Add(-time.Duration(bucketCount-i-1) * bucketDuration),
			EndTime:   now.Add(-time.Duration(bucketCount-i) * bucketDuration),
			Entries:   make(map[string]*SQLStatEntry),
		}
	}

	return &SlidingWindowStats{
		windowSize:   windowSize,
		buckets:      buckets,
		bucketCount:  bucketCount,
		currentIndex: bucketCount - 1,
	}
}

// Record 记录事件
func (s *SlidingWindowStats) Record(event *SQLEvent, fingerprint string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否需要滚动窗口
	s.maybeRotate(time.Now())

	// 获取当前桶
	bucket := s.buckets[s.currentIndex]

	// 获取或创建条目
	entry, exists := bucket.Entries[fingerprint]
	if !exists {
		entry = NewSQLStatEntry(fingerprint, event.SQLTemplate)
		bucket.Entries[fingerprint] = entry
	}

	// 更新统计
	entry.Update(event)
}

// maybeRotate 检查并执行窗口滚动
func (s *SlidingWindowStats) maybeRotate(now time.Time) {
	bucketDuration := s.windowSize / time.Duration(s.bucketCount)
	currentBucket := s.buckets[s.currentIndex]

	// 检查当前桶是否过期
	if now.Sub(currentBucket.StartTime) >= bucketDuration {
		// 滚动到下一个桶
		s.currentIndex = (s.currentIndex + 1) % s.bucketCount

		// 清空新桶
		s.buckets[s.currentIndex] = &TimeBucket{
			StartTime: now,
			EndTime:   now.Add(bucketDuration),
			Entries:   make(map[string]*SQLStatEntry),
		}
	}
}

// GetStats 获取窗口内所有统计
func (s *SlidingWindowStats) GetStats() map[string]*SQLStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 合并所有桶的统计
	merged := make(map[string]*SQLStatEntry)

	for _, bucket := range s.buckets {
		for fingerprint, entry := range bucket.Entries {
			if existing, exists := merged[fingerprint]; exists {
				// 合并统计
				existing.RequestCount += entry.RequestCount
				existing.SuccessCount += entry.SuccessCount
				existing.ErrorCount += entry.ErrorCount
				existing.TotalDuration += entry.TotalDuration
				if entry.MinDuration < existing.MinDuration {
					existing.MinDuration = entry.MinDuration
				}
				if entry.MaxDuration > existing.MaxDuration {
					existing.MaxDuration = entry.MaxDuration
				}
			} else {
				// 复制条目
				newEntry := *entry
				merged[fingerprint] = &newEntry
			}
		}
	}

	// 转换为 SQLStats
	result := make(map[string]*SQLStats)
	for fingerprint, entry := range merged {
		result[fingerprint] = entry.ToSQLStats()
	}

	return result
}

// ==================== 堆排序辅助 ====================

// StatsHeap 统计堆（用于 Top N）
type StatsHeap struct {
	items    []*SQLStats
	lessFunc func(a, b *SQLStats) bool
}

func (h StatsHeap) Len() int           { return len(h.items) }
func (h StatsHeap) Less(i, j int) bool { return h.lessFunc(h.items[i], h.items[j]) }
func (h StatsHeap) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }

func (h *StatsHeap) Push(x interface{}) {
	h.items = append(h.items, x.(*SQLStats))
}

func (h *StatsHeap) Pop() interface{} {
	old := h.items
	n := len(old)
	x := old[n-1]
	h.items = old[0 : n-1]
	return x
}

// NewStatsHeap 创建统计堆
func NewStatsHeap(lessFunc func(a, b *SQLStats) bool) *StatsHeap {
	return &StatsHeap{
		items:    make([]*SQLStats, 0),
		lessFunc: lessFunc,
	}
}

// Push 推入元素
func (h *StatsHeap) PushItem(item *SQLStats) {
	heap.Push(h, item)
}

// Pop 弹出元素
func (h *StatsHeap) PopItem() *SQLStats {
	return heap.Pop(h).(*SQLStats)
}

// TopN 获取 Top N
func (h *StatsHeap) TopN(n int) []*SQLStats {
	result := make([]*SQLStats, 0, n)
	for i := 0; i < n && h.Len() > 0; i++ {
		result = append(result, h.PopItem())
	}
	return result
}
