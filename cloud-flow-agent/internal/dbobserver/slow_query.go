// Package dbobserver 提供数据库观测功能
// 本文件实现慢查询自动识别与排行
package dbobserver

import (
	"container/heap"
	"sort"
	"strings"
	"sync"
	"time"
)

// ==================== 慢查询检测配置 ====================

// SlowQueryDetectorConfig 慢查询检测器配置
type SlowQueryDetectorConfig struct {
	// 默认慢查询阈值（微秒）
	DefaultThreshold int64
	// 按 SQL 类型设置不同阈值
	ThresholdsByType map[SQLType]int64
	// 按数据库类型设置不同阈值
	ThresholdsByDB map[DatabaseType]int64
	// 最大慢查询记录数
	MaxRecords int
	// 慢查询评分权重
	ScoreWeights SlowQueryScoreWeights
	// 是否自动分析原因
	AutoAnalyze bool
}

// SlowQueryScoreWeights 慢查询评分权重
type SlowQueryScoreWeights struct {
	DurationWeight    float64 // 时延权重
	FrequencyWeight   float64 // 频率权重
	DataSizeWeight    float64 // 数据量权重
	ResourceWeight    float64 // 资源占用权重
}

// DefaultSlowQueryDetectorConfig 返回默认配置
func DefaultSlowQueryDetectorConfig() *SlowQueryDetectorConfig {
	return &SlowQueryDetectorConfig{
		DefaultThreshold: 1000000, // 1秒
		ThresholdsByType: map[SQLType]int64{
			SQLTypeSelect: 1000000,  // 1秒
			SQLTypeInsert: 500000,   // 500毫秒
			SQLTypeUpdate: 500000,   // 500毫秒
			SQLTypeDelete: 500000,   // 500毫秒
			SQLTypeDDL:    2000000,  // 2秒
		},
		ThresholdsByDB: map[DatabaseType]int64{
			DatabaseTypeMySQL:      1000000,
			DatabaseTypeOracle:     1000000,
			DatabaseTypePostgreSQL: 1000000,
			DatabaseTypeDaMeng:     1500000,
			DatabaseTypeGaussDB:    1000000,
		},
		MaxRecords:   1000,
		AutoAnalyze:  true,
		ScoreWeights: SlowQueryScoreWeights{
			DurationWeight:  0.4,
			FrequencyWeight: 0.3,
			DataSizeWeight:  0.2,
			ResourceWeight:  0.1,
		},
	}
}

// ==================== 慢查询原因分析 ====================

// SlowQueryReason 慢查询原因
type SlowQueryReason string

const (
	SlowReasonFullTableScan    SlowQueryReason = "full_table_scan"     // 全表扫描
	SlowReasonMissingIndex     SlowQueryReason = "missing_index"       // 缺少索引
	SlowReasonLargeResult      SlowQueryReason = "large_result_set"    // 结果集过大
	SlowReasonHighCPU          SlowQueryReason = "high_cpu_usage"      // CPU 使用率高
	SlowReasonHighIO           SlowQueryReason = "high_io_usage"       // IO 使用率高
	SlowReasonLockWait         SlowQueryReason = "lock_wait"           // 锁等待
	SlowReasonNetworkLatency   SlowQueryReason = "network_latency"     // 网络延迟
	SlowReasonComplexQuery     SlowQueryReason = "complex_query"       // 复杂查询
	SlowReasonSubquery         SlowQueryReason = "subquery"            // 子查询
	SlowReasonJoin             SlowQueryReason = "join_operation"      // JOIN 操作
	SlowReasonOrderBy          SlowQueryReason = "order_by"            // 排序操作
	SlowReasonGroupBy          SlowQueryReason = "group_by"            // 分组操作
	SlowReasonDistinct         SlowQueryReason = "distinct"            // 去重操作
	SlowReasonUnknown          SlowQueryReason = "unknown"             // 未知原因
)

// SlowQueryAnalyzer 慢查询分析器
type SlowQueryAnalyzer struct {
	// 全表扫描特征
	fullTableScanPatterns []string
	// 子查询特征
	subqueryPatterns []string
	// 复杂 JOIN 特征
	joinPatterns []string
	// 排序特征
	orderByPatterns []string
	// 分组特征
	groupByPatterns []string
}

// NewSlowQueryAnalyzer 创建慢查询分析器
func NewSlowQueryAnalyzer() *SlowQueryAnalyzer {
	return &SlowQueryAnalyzer{
		fullTableScanPatterns: []string{
			"WHERE 1=1",
			"WHERE TRUE",
			"WHERE ''",
		},
		subqueryPatterns: []string{
			"SELECT * FROM (SELECT",
			"IN (SELECT",
			"EXISTS (SELECT",
			"NOT EXISTS (SELECT",
		},
		joinPatterns: []string{
			"LEFT JOIN",
			"RIGHT JOIN",
			"INNER JOIN",
			"OUTER JOIN",
			"CROSS JOIN",
		},
		orderByPatterns: []string{
			"ORDER BY",
		},
		groupByPatterns: []string{
			"GROUP BY",
			"HAVING",
		},
	}
}

// Analyze 分析慢查询原因
func (a *SlowQueryAnalyzer) Analyze(query *SlowQuery, processMetrics *ProcessMetrics) []SlowQueryReason {
	reasons := make([]SlowQueryReason, 0)
	sqlUpper := strings.ToUpper(query.SQLText)

	// 检查全表扫描
	if a.checkFullTableScan(sqlUpper) {
		reasons = append(reasons, SlowReasonFullTableScan)
	}

	// 检查子查询
	if a.checkSubquery(sqlUpper) {
		reasons = append(reasons, SlowReasonSubquery)
	}

	// 检查 JOIN
	if a.checkJoin(sqlUpper) {
		reasons = append(reasons, SlowReasonJoin)
	}

	// 检查排序
	if a.checkOrderBy(sqlUpper) {
		reasons = append(reasons, SlowReasonOrderBy)
	}

	// 检查分组
	if a.checkGroupBy(sqlUpper) {
		reasons = append(reasons, SlowReasonGroupBy)
	}

	// 检查结果集大小
	if query.RowsReturned > 10000 {
		reasons = append(reasons, SlowReasonLargeResult)
	}

	// 检查进程资源使用
	if processMetrics != nil {
		if processMetrics.CPUUsage > 80 {
			reasons = append(reasons, SlowReasonHighCPU)
		}
		if processMetrics.IOReadBytes > 100*1024*1024 || processMetrics.IOWriteBytes > 100*1024*1024 {
			reasons = append(reasons, SlowReasonHighIO)
		}
	}

	// 如果没有找到原因
	if len(reasons) == 0 {
		reasons = append(reasons, SlowReasonUnknown)
	}

	return reasons
}

func (a *SlowQueryAnalyzer) checkFullTableScan(sql string) bool {
	// 检查是否有 WHERE 条件
	if !strings.Contains(sql, "WHERE") {
		return true
	}
	// 检查是否有无效 WHERE 条件
	for _, pattern := range a.fullTableScanPatterns {
		if strings.Contains(sql, pattern) {
			return true
		}
	}
	return false
}

func (a *SlowQueryAnalyzer) checkSubquery(sql string) bool {
	for _, pattern := range a.subqueryPatterns {
		if strings.Contains(sql, pattern) {
			return true
		}
	}
	return false
}

func (a *SlowQueryAnalyzer) checkJoin(sql string) bool {
	joinCount := 0
	for _, pattern := range a.joinPatterns {
		joinCount += strings.Count(sql, pattern)
	}
	return joinCount > 2
}

func (a *SlowQueryAnalyzer) checkOrderBy(sql string) bool {
	return strings.Contains(sql, "ORDER BY")
}

func (a *SlowQueryAnalyzer) checkGroupBy(sql string) bool {
	return strings.Contains(sql, "GROUP BY") || strings.Contains(sql, "HAVING")
}

// ==================== 慢查询评分 ====================

// SlowQueryScorer 慢查询评分器
type SlowQueryScorer struct {
	weights SlowQueryScoreWeights
}

// NewSlowQueryScorer 创建慢查询评分器
func NewSlowQueryScorer(weights SlowQueryScoreWeights) *SlowQueryScorer {
	return &SlowQueryScorer{
		weights: weights,
	}
}

// Calculate 计算慢查询评分
func (s *SlowQueryScorer) Calculate(query *SlowQuery, stats *SQLStats) float64 {
	score := 0.0

	// 时延评分（相对于阈值的倍数）
	durationScore := float64(query.Duration) / float64(query.SlowThreshold)
	score += durationScore * s.weights.DurationWeight

	// 频率评分
	if stats != nil && stats.RequestCount > 0 {
		frequencyScore := float64(stats.RequestCount) / 100.0 // 归一化
		if frequencyScore > 1.0 {
			frequencyScore = 1.0
		}
		score += frequencyScore * s.weights.FrequencyWeight
	}

	// 数据量评分
	dataSizeScore := float64(query.RowsReturned+query.RowsAffected) / 10000.0
	if dataSizeScore > 1.0 {
		dataSizeScore = 1.0
	}
	score += dataSizeScore * s.weights.DataSizeWeight

	// 资源占用评分
	resourceScore := 0.0
	if query.ProcessCPU > 0 {
		resourceScore = query.ProcessCPU / 100.0
	}
	score += resourceScore * s.weights.ResourceWeight

	return score
}

// ==================== 慢查询记录 ====================

// SlowQueryRecord 慢查询记录（内部使用）
type SlowQueryRecord struct {
	*SlowQuery
	Score       float64
	Reasons     []SlowQueryReason
	RecordTime  time.Time
}

// ==================== 慢查询排行堆 ====================

// SlowQueryHeap 慢查询堆
type SlowQueryHeap []*SlowQueryRecord

func (h SlowQueryHeap) Len() int           { return len(h) }
func (h SlowQueryHeap) Less(i, j int) bool { return h[i].Score < h[j].Score } // 最小堆
func (h SlowQueryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *SlowQueryHeap) Push(x interface{}) {
	*h = append(*h, x.(*SlowQueryRecord))
}

func (h *SlowQueryHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// ==================== 慢查询排行 ====================

// SlowQueryRanking 慢查询排行管理
type SlowQueryRanking struct {
	records    map[string]*SlowQueryRank // fingerprint -> rank
	heap       SlowQueryHeap
	maxSize    int
	mu         sync.RWMutex
}

// NewSlowQueryRanking 创建慢查询排行
func NewSlowQueryRanking(maxSize int) *SlowQueryRanking {
	return &SlowQueryRanking{
		records: make(map[string]*SlowQueryRank),
		heap:    make(SlowQueryHeap, 0),
		maxSize: maxSize,
	}
}

// Update 更新排行
func (r *SlowQueryRanking) Update(fingerprint string, duration int64, slowCount int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rank, exists := r.records[fingerprint]
	if exists {
		rank.SlowCount++
		rank.TotalDuration += duration
		if duration > rank.MaxDuration {
			rank.MaxDuration = duration
		}
		rank.AvgDuration = float64(rank.TotalDuration) / float64(rank.SlowCount)
		rank.LastSeen = time.Now()
	} else {
		rank = &SlowQueryRank{
			SQLFingerprint: fingerprint,
			SlowCount:      1,
			TotalDuration:  duration,
			MaxDuration:    duration,
			AvgDuration:    float64(duration),
			LastSeen:       time.Now(),
		}
		r.records[fingerprint] = rank
	}
}

// GetTopN 获取 Top N 排行
func (r *SlowQueryRanking) GetTopN(n int) []*SlowQueryRank {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 收集所有排行
	ranks := make([]*SlowQueryRank, 0, len(r.records))
	for _, rank := range r.records {
		ranks = append(ranks, rank)
	}

	// 按慢查询次数排序
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i].SlowCount > ranks[j].SlowCount
	})

	// 取 Top N
	if n > len(ranks) {
		n = len(ranks)
	}

	result := make([]*SlowQueryRank, n)
	for i := 0; i < n; i++ {
		// 复制并设置排名
		rank := *ranks[i]
		rank.Rank = i + 1
		result[i] = &rank
	}

	return result
}

// ==================== 慢查询检测器 ====================

// SlowQueryDetector 慢查询检测器
type SlowQueryDetector struct {
	config    *SlowQueryDetectorConfig
	observerConfig *ObserverConfig

	// 慢查询记录
	records   []*SlowQueryRecord
	maxRecords int

	// 排行
	ranking   *SlowQueryRanking

	// 分析器和评分器
	analyzer  *SlowQueryAnalyzer
	scorer    *SlowQueryScorer

	// 统计引用（用于评分）
	statsGetter func(fingerprint string) *SQLStats

	// 进程指标引用
	metricsGetter func(pid uint32) *ProcessMetrics

	mu sync.RWMutex
}

// NewSlowQueryDetector 创建慢查询检测器
func NewSlowQueryDetector(cfg *ObserverConfig) *SlowQueryDetector {
	detectorCfg := DefaultSlowQueryDetectorConfig()

	return &SlowQueryDetector{
		config:        detectorCfg,
		observerConfig: cfg,
		records:       make([]*SlowQueryRecord, 0),
		maxRecords:    detectorCfg.MaxRecords,
		ranking:       NewSlowQueryRanking(100),
		analyzer:      NewSlowQueryAnalyzer(),
		scorer:        NewSlowQueryScorer(detectorCfg.ScoreWeights),
	}
}

// SetStatsGetter 设置统计获取器
func (d *SlowQueryDetector) SetStatsGetter(getter func(fingerprint string) *SQLStats) {
	d.statsGetter = getter
}

// SetMetricsGetter 设置指标获取器
func (d *SlowQueryDetector) SetMetricsGetter(getter func(pid uint32) *ProcessMetrics) {
	d.metricsGetter = getter
}

// GetThreshold 获取慢查询阈值
func (d *SlowQueryDetector) GetThreshold(sqlType SQLType, dbType DatabaseType) int64 {
	// 优先使用 SQL 类型阈值
	if threshold, ok := d.config.ThresholdsByType[sqlType]; ok {
		return threshold
	}

	// 其次使用数据库类型阈值
	if threshold, ok := d.config.ThresholdsByDB[dbType]; ok {
		return threshold
	}

	// 使用默认阈值
	return d.config.DefaultThreshold
}

// RecordSlowQuery 记录慢查询
func (d *SlowQueryDetector) RecordSlowQuery(event *SQLEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 获取阈值
	threshold := d.GetThreshold(event.SQLType, event.DatabaseType)

	// 创建慢查询记录
	slowQuery := &SlowQuery{
		Timestamp:     event.Timestamp,
		SQLText:       event.SQLText,
		SQLTemplate:   event.SQLTemplate,
		SQLType:       event.SQLType,
		DatabaseType:  event.DatabaseType,
		DatabaseName:  event.DatabaseName,
		Duration:      event.Duration,
		RowsAffected:  event.RowsAffected,
		RowsReturned:  event.RowsReturned,
		IsSlowQuery:   true,
		SlowThreshold: threshold,
		ClientAddr:    event.ClientAddr,
		ServerAddr:    event.ServerAddr,
		PID:           event.PID,
		User:          event.User,
		Schema:        event.Schema,
		Table:         event.Table,
	}

	// 获取进程指标
	if d.metricsGetter != nil && event.PID > 0 {
		metrics := d.metricsGetter(event.PID)
		if metrics != nil {
			slowQuery.ProcessCPU = metrics.CPUUsage
			slowQuery.ProcessMemory = metrics.MemoryUsage
			slowQuery.ProcessIORead = metrics.IOReadBytes
			slowQuery.ProcessIOWrite = metrics.IOWriteBytes
		}
	}

	// 分析慢查询原因
	var reasons []SlowQueryReason
	if d.config.AutoAnalyze {
		var processMetrics *ProcessMetrics
		if d.metricsGetter != nil && event.PID > 0 {
			processMetrics = d.metricsGetter(event.PID)
		}
		reasons = d.analyzer.Analyze(slowQuery, processMetrics)
		slowQuery.SlowReason = string(reasons[0])
	}

	// 计算评分
	var stats *SQLStats
	if d.statsGetter != nil {
		stats = d.statsGetter(event.SQLFingerprint)
	}
	score := d.scorer.Calculate(slowQuery, stats)
	slowQuery.SlowScore = score

	// 创建记录
	record := &SlowQueryRecord{
		SlowQuery:  slowQuery,
		Score:      score,
		Reasons:    reasons,
		RecordTime: time.Now(),
	}

	// 添加到记录列表
	d.records = append(d.records, record)

	// 检查是否超过最大记录数
	if len(d.records) > d.maxRecords {
		// 移除评分最低的记录
		d.removeLowestScore()
	}

	// 更新排行
	d.ranking.Update(event.SQLFingerprint, event.Duration, 1)
}

// removeLowestScore 移除评分最低的记录
func (d *SlowQueryDetector) removeLowestScore() {
	minIdx := 0
	minScore := d.records[0].Score

	for i, record := range d.records {
		if record.Score < minScore {
			minScore = record.Score
			minIdx = i
		}
	}

	// 移除最低评分记录
	d.records = append(d.records[:minIdx], d.records[minIdx+1:]...)
}

// GetSlowQueries 获取慢查询列表
func (d *SlowQueryDetector) GetSlowQueries(limit int) []*SlowQuery {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 按评分排序
	sorted := make([]*SlowQueryRecord, len(d.records))
	copy(sorted, d.records)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	// 取指定数量
	if limit > len(sorted) {
		limit = len(sorted)
	}

	result := make([]*SlowQuery, limit)
	for i := 0; i < limit; i++ {
		result[i] = sorted[i].SlowQuery
	}

	return result
}

// GetRanking 获取慢查询排行
func (d *SlowQueryDetector) GetRanking(limit int) []*SlowQueryRank {
	return d.ranking.GetTopN(limit)
}

// UpdateRankings 更新排行
func (d *SlowQueryDetector) UpdateRankings() {
	// 排行在每次记录时已更新，这里可以做定期清理
	d.mu.Lock()
	defer d.mu.Unlock()

	// 清理过期的记录（超过 1 小时）
	now := time.Now()
	validRecords := make([]*SlowQueryRecord, 0)
	for _, record := range d.records {
		if now.Sub(record.RecordTime) < time.Hour {
			validRecords = append(validRecords, record)
		}
	}
	d.records = validRecords
}

// GetStats 获取慢查询统计
func (d *SlowQueryDetector) GetStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	totalDuration := int64(0)
	maxDuration := int64(0)
	reasons := make(map[string]int64)

	for _, record := range d.records {
		totalDuration += record.Duration
		if record.Duration > maxDuration {
			maxDuration = record.Duration
		}
		for _, reason := range record.Reasons {
			reasons[string(reason)]++
		}
	}

	avgDuration := float64(0)
	if len(d.records) > 0 {
		avgDuration = float64(totalDuration) / float64(len(d.records))
	}

	return map[string]interface{}{
		"total_count":   len(d.records),
		"total_duration": totalDuration,
		"avg_duration":  avgDuration,
		"max_duration":  maxDuration,
		"reasons":       reasons,
	}
}

// Clear 清空记录
func (d *SlowQueryDetector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.records = make([]*SlowQueryRecord, 0)
	d.ranking = NewSlowQueryRanking(100)
}

// Close 关闭检测器
func (d *SlowQueryDetector) Close() {
	d.Clear()
}

// ==================== 慢查询告警 ====================

// SlowQueryAlert 慢查询告警
type SlowQueryAlert struct {
	Timestamp     time.Time       `json:"timestamp"`
	AlertType     string          `json:"alert_type"`     // alert type
	Severity      string          `json:"severity"`       // critical/warning/info
	SQLFingerprint string          `json:"sql_fingerprint"`
	SQLTemplate   string           `json:"sql_template"`
	Duration      int64            `json:"duration"`
	Threshold     int64            `json:"threshold"`
	DatabaseName  string           `json:"database_name"`
	Reasons       []SlowQueryReason `json:"reasons"`
	Message       string           `json:"message"`
}

// SlowQueryAlerter 慢查询告警器
type SlowQueryAlerter struct {
	threshold    int64
	alertCount   int
	alertWindow  time.Duration
	alerts       []*SlowQueryAlert
	mu           sync.RWMutex
}

// NewSlowQueryAlerter 创建慢查询告警器
func NewSlowQueryAlerter(threshold int64, alertWindow time.Duration) *SlowQueryAlerter {
	return &SlowQueryAlerter{
		threshold:   threshold,
		alertWindow: alertWindow,
		alerts:      make([]*SlowQueryAlert, 0),
	}
}

// Check 检查是否需要告警
func (a *SlowQueryAlerter) Check(query *SlowQuery) *SlowQueryAlert {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 检查是否超过阈值
	if query.Duration < a.threshold {
		return nil
	}

	// 确定告警级别
	severity := "warning"
	if query.Duration > a.threshold*5 {
		severity = "critical"
	} else if query.Duration > a.threshold*2 {
		severity = "warning"
	}

	// 创建告警
	alert := &SlowQueryAlert{
		Timestamp:      time.Now(),
		AlertType:      "slow_query",
		Severity:       severity,
		SQLFingerprint: query.SQLFingerprint,
		SQLTemplate:    query.SQLTemplate,
		Duration:       query.Duration,
		Threshold:      a.threshold,
		DatabaseName:   query.DatabaseName,
		Message:        a.generateMessage(query),
	}

	a.alerts = append(a.alerts, alert)
	a.alertCount++

	return alert
}

func (a *SlowQueryAlerter) generateMessage(query *SlowQuery) string {
	return "Slow query detected: " + query.SQLTemplate + " took " +
		time.Duration(query.Duration * 1000).String()
}

// GetAlerts 获取告警列表
func (a *SlowQueryAlerter) GetAlerts(limit int) []*SlowQueryAlert {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit > len(a.alerts) {
		limit = len(a.alerts)
	}

	result := make([]*SlowQueryAlert, limit)
	copy(result, a.alerts[len(a.alerts)-limit:])

	return result
}

// ==================== 执行计划分析 ====================

// ExplainPlanAnalyzer 执行计划分析器
type ExplainPlanAnalyzer struct {
	// 全表扫描关键字
	fullScanKeywords []string
	// 索引使用关键字
	indexKeywords []string
	// 性能问题关键字
	performanceIssues []string
}

// NewExplainPlanAnalyzer 创建执行计划分析器
func NewExplainPlanAnalyzer() *ExplainPlanAnalyzer {
	return &ExplainPlanAnalyzer{
		fullScanKeywords: []string{
			"FULL TABLE SCAN",
			"SEQ SCAN",
			"TABLE ACCESS FULL",
			"ALL ROWS",
		},
		indexKeywords: []string{
			"INDEX",
			"INDEX RANGE SCAN",
			"INDEX UNIQUE SCAN",
			"BITMAP",
		},
		performanceIssues: []string{
			"CARTESIAN",
			"TEMP TABLE",
			"SORT",
			"HASH JOIN",
			"MERGE JOIN",
		},
	}
}

// Analyze 分析执行计划
func (a *ExplainPlanAnalyzer) Analyze(plan string) []string {
	issues := make([]string, 0)
	planUpper := strings.ToUpper(plan)

	// 检查全表扫描
	for _, keyword := range a.fullScanKeywords {
		if strings.Contains(planUpper, keyword) {
			issues = append(issues, "full_table_scan: "+keyword)
		}
	}

	// 检查性能问题
	for _, keyword := range a.performanceIssues {
		if strings.Contains(planUpper, keyword) {
			issues = append(issues, "performance_issue: "+keyword)
		}
	}

	return issues
}

// HasIndexUsage 检查是否使用了索引
func (a *ExplainPlanAnalyzer) HasIndexUsage(plan string) bool {
	planUpper := strings.ToUpper(plan)
	for _, keyword := range a.indexKeywords {
		if strings.Contains(planUpper, keyword) {
			return true
		}
	}
	return false
}

// HasFullScan 检查是否有全表扫描
func (a *ExplainPlanAnalyzer) HasFullScan(plan string) bool {
	planUpper := strings.ToUpper(plan)
	for _, keyword := range a.fullScanKeywords {
		if strings.Contains(planUpper, keyword) {
			return true
		}
	}
	return false
}
