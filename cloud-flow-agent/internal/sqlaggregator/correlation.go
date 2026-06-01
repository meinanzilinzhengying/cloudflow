//go:build linux

// Package sqlaggregator 提供数据库进程性能关联分析功能
// 将SQL执行性能与数据库进程指标关联
package sqlaggregator

import (
	"fmt"
	"log"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// CorrelationLevel 关联级别
type CorrelationLevel int

const (
	CorrelationNone CorrelationLevel = iota
	CorrelationLow
	CorrelationMedium
	CorrelationHigh
)

// String 返回关联级别名称
func (l CorrelationLevel) String() string {
	switch l {
	case CorrelationNone:
		return "无关联"
	case CorrelationLow:
		return "低关联"
	case CorrelationMedium:
		return "中等关联"
	case CorrelationHigh:
		return "高关联"
	default:
		return "未知"
	}
}

// ProcessMetrics 进程性能指标
type ProcessMetrics struct {
	PID           uint32
	Command       string
	CPUPercent    float64
	MemoryMB      float64
	MemoryPercent float64
	Threads       int
	FDCount       int
	IOReadBPS     float64
	IOWriteBPS    float64
	NetConnCount  int
	StartTime     time.Time
	LastUpdate    time.Time
}

// SQLMetrics SQL执行指标
type SQLMetrics struct {
	RequestCount  uint64
	SuccessCount  uint64
	ErrorCount    uint64
	AvgLatencyMs  float64
	MaxLatencyMs  float64
	MinLatencyMs  float64
	SlowCount     uint64
	TimeoutCount  uint64
	BytesSent     uint64
	BytesReceived uint64
}

// CorrelationResult 关联分析结果
type CorrelationResult struct {
	PID            uint32
	SQLType        SQLType
	Database       string
	Table          string
	ProcessMetrics ProcessMetrics
	SQLMetrics     SQLMetrics
	CorrelationScore float64  // 关联度得分 (0-100)
	CorrelationLevel CorrelationLevel
	Recommendations []string   // 优化建议
	Timestamp      time.Time
}

// PerformanceSnapshot 性能快照
type PerformanceSnapshot struct {
	PID            uint32
	Timestamp      time.Time
	CPUPercent     float64
	MemoryMB       float64
	SQLLatencyMs   float64
	QueryCount     uint64
	ConnectionCount uint64
}

// TrendDirection 趋势方向
type TrendDirection int

const (
	TrendStable TrendDirection = iota
	TrendImproving
	TrendDegrading
)

// PerformanceTrend 性能趋势
type PerformanceTrend struct {
	PID         uint32
	Metric      string
	Direction   TrendDirection
	ChangeRate  float64 // 每分钟变化率
	Samples     []PerformanceSnapshot
}

// AlertInfo 告警信息
type AlertInfo struct {
	AlertType    string
	Severity     string // info, warning, critical
	PID          uint32
	Message      string
	Threshold    float64
	ActualValue  float64
	Timestamp    time.Time
}

// CorrelationAnalyzer 关联分析器
type CorrelationAnalyzer struct {
	mu              sync.RWMutex
	snapshots       map[uint32][]PerformanceSnapshot
	trends          map[uint32]*PerformanceTrend
	alerts          []AlertInfo
	maxSnapshots    int
	analyzeInterval time.Duration

	// 阈值配置
	thresholds struct {
		CPUMax      float64
		MemoryMaxMB float64
		LatencyMaxMs float64
		SlowQueryMax uint64
		ConnMax     uint64
	}

	stats struct {
		analyzeCount    atomic.Uint64
		alertCount      atomic.Uint64
		lastAnalyzeTime atomic.Int64
	}
}

// CorrelationAnalyzerOptions 分析器配置
type CorrelationAnalyzerOptions struct {
	MaxSnapshots     int
	AnalyzeInterval  time.Duration
	CPUMaxThreshold  float64
	MemoryMaxMB      float64
	LatencyMaxMs     float64
	SlowQueryMax     uint64
	ConnMax          uint64
}

// NewCorrelationAnalyzer 创建关联分析器
func NewCorrelationAnalyzer(opts *CorrelationAnalyzerOptions) *CorrelationAnalyzer {
	if opts == nil {
		opts = &CorrelationAnalyzerOptions{
			MaxSnapshots:     60,           // 保存60个快照(1分钟一个)
			AnalyzeInterval:  time.Minute,
			CPUMaxThreshold:  80.0,         // 80% CPU
			MemoryMaxMB:      1024.0,       // 1GB内存
			LatencyMaxMs:     1000.0,       // 1秒延迟
			SlowQueryMax:     10,           // 10个慢查询
			ConnMax:          100,          // 100连接
		}
	}

	analyzer := &CorrelationAnalyzer{
		snapshots:       make(map[uint32][]PerformanceSnapshot),
		trends:          make(map[uint32]*PerformanceTrend),
		maxSnapshots:    opts.MaxSnapshots,
		analyzeInterval: opts.AnalyzeInterval,
	}

	analyzer.thresholds.CPUMax = opts.CPUMaxThreshold
	analyzer.thresholds.MemoryMaxMB = opts.MemoryMaxMB
	analyzer.thresholds.LatencyMaxMs = opts.LatencyMaxMs
	analyzer.thresholds.SlowQueryMax = opts.SlowQueryMax
	analyzer.thresholds.ConnMax = opts.ConnMax

	return analyzer
}

// AddSnapshot 添加性能快照
func (a *CorrelationAnalyzer) AddSnapshot(snapshot PerformanceSnapshot) {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshots := a.snapshots[snapshot.PID]
	snapshots = append(snapshots, snapshot)

	// 保持最大快照数
	if len(snapshots) > a.maxSnapshots {
		snapshots = snapshots[len(snapshots)-a.maxSnapshots:]
	}

	a.snapshots[snapshot.PID] = snapshots
}

// Analyze 分析关联性
func (a *CorrelationAnalyzer) Analyze(processStats map[uint32]DBProcessStats, sqlAgg map[SQLAggKey]SQLAggValue) []CorrelationResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	var results []CorrelationResult
	now := time.Now()

	// 按PID和SQL类型分组分析
	typeAgg := make(map[string]*struct {
		PID       uint32
		SQLType   SQLType
		Database  string
		ProcStats *DBProcessStats
		SQLStats  SQLAggValue
		Count     int
	})

	// 聚合进程统计
	for pid, stats := range processStats {
		key := fmt.Sprintf("%d", pid)
		if _, ok := typeAgg[key]; !ok {
			typeAgg[key] = &struct {
				PID       uint32
				SQLType   SQLType
				Database  string
				ProcStats *DBProcessStats
				SQLStats  SQLAggValue
				Count     int
			}{
				PID:       pid,
				ProcStats: &stats,
			}
		}
	}

	// 关联SQL统计
	for key, stats := range sqlAgg {
		pidKey := fmt.Sprintf("%d", key.PID)
		if agg, ok := typeAgg[pidKey]; ok {
			agg.SQLType = key.SQLType
			agg.Database = key.Database
			agg.SQLStats.RequestCount += stats.RequestCount
			agg.SQLStats.SuccessCount += stats.SuccessCount
			agg.SQLStats.ErrorCount += stats.ErrorCount
			agg.SQLStats.TotalLatencyNs += stats.TotalLatencyNs
			if stats.MaxLatencyNs > agg.SQLStats.MaxLatencyNs {
				agg.SQLStats.MaxLatencyNs = stats.MaxLatencyNs
			}
			agg.Count++
		}
	}

	// 生成关联结果
	for _, agg := range typeAgg {
		if agg.Count == 0 {
			continue
		}

		result := CorrelationResult{
			PID:          agg.PID,
			SQLType:      agg.SQLType,
			Database:     agg.Database,
			Timestamp:    now,
		}

		// 填充进程指标
		if agg.ProcStats != nil {
			result.ProcessMetrics = ProcessMetrics{
				PID:      agg.PID,
				CPUPercent: agg.ProcStats.CPUUsagePercent(),
				MemoryMB: agg.ProcStats.MemoryMB(),
			}
		}

		// 填充SQL指标
		if agg.SQLStats.RequestCount > 0 {
			result.SQLMetrics = SQLMetrics{
				RequestCount: agg.SQLStats.RequestCount,
				SuccessCount: agg.SQLStats.SuccessCount,
				ErrorCount:   agg.SQLStats.ErrorCount,
				AvgLatencyMs: float64(agg.SQLStats.TotalLatencyNs) / float64(agg.SQLStats.RequestCount) / 1_000_000,
				MaxLatencyMs: float64(agg.SQLStats.MaxLatencyNs) / 1_000_000,
				MinLatencyMs: float64(agg.SQLStats.MinLatencyNs) / 1_000_000,
				SlowCount:    agg.SQLStats.TimeoutCount,
			}
		}

		// 计算关联度
		result.CorrelationScore = a.calculateCorrelationScore(result)
		result.CorrelationLevel = a.getCorrelationLevel(result.CorrelationScore)

		// 生成建议
		result.Recommendations = a.generateRecommendations(result)

		results = append(results, result)
	}

	// 排序: 按关联度降序
	sort.Slice(results, func(i, j int) bool {
		return results[i].CorrelationScore > results[j].CorrelationScore
	})

	atomic.AddUint64(&a.stats.analyzeCount, 1)
	atomic.StoreInt64(&a.stats.lastAnalyzeTime, now.Unix())

	return results
}

// calculateCorrelationScore 计算关联度得分
func (a *CorrelationAnalyzer) calculateCorrelationScore(r CorrelationResult) float64 {
	var score float64

	// CPU与延迟关联
	if r.ProcessMetrics.CPUPercent > 50 {
		cpuWeight := r.ProcessMetrics.CPUPercent / 100
		latencyWeight := math.Min(r.SQLMetrics.AvgLatencyMs/a.thresholds.LatencyMaxMs, 1)
		score += (cpuWeight + latencyWeight) / 2 * 40
	}

	// 内存与查询量关联
	if r.ProcessMetrics.MemoryMB > a.thresholds.MemoryMaxMB*0.7 {
		memWeight := r.ProcessMetrics.MemoryMB / a.thresholds.MemoryMaxMB
		queryWeight := math.Min(float64(r.SQLMetrics.RequestCount)/1000, 1)
		score += (memWeight + queryWeight) / 2 * 30
	}

	// 错误率关联
	if r.SQLMetrics.ErrorCount > 0 && r.SQLMetrics.RequestCount > 0 {
		errorRate := float64(r.SQLMetrics.ErrorCount) / float64(r.SQLMetrics.RequestCount)
		if errorRate > 0.01 { // >1%错误率
			score += errorRate * 30
		}
	}

	return math.Min(score, 100)
}

// getCorrelationLevel 获取关联级别
func (a *CorrelationAnalyzer) getCorrelationLevel(score float64) CorrelationLevel {
	switch {
	case score >= 70:
		return CorrelationHigh
	case score >= 40:
		return CorrelationMedium
	case score >= 20:
		return CorrelationLow
	default:
		return CorrelationNone
	}
}

// generateRecommendations 生成优化建议
func (a *CorrelationAnalyzer) generateRecommendations(r CorrelationResult) []string {
	var recommendations []string

	// CPU建议
	if r.ProcessMetrics.CPUPercent > a.thresholds.CPUMax {
		recommendations = append(recommendations,
			fmt.Sprintf("CPU使用率过高(%.1f%%), 建议优化查询或增加资源", r.ProcessMetrics.CPUPercent))
	}

	// 内存建议
	if r.ProcessMetrics.MemoryMB > a.thresholds.MemoryMaxMB {
		recommendations = append(recommendations,
			fmt.Sprintf("内存使用过高(%.1fMB), 建议检查缓存配置或增加内存", r.ProcessMetrics.MemoryMB))
	}

	// 延迟建议
	if r.SQLMetrics.AvgLatencyMs > a.thresholds.LatencyMaxMs {
		recommendations = append(recommendations,
			fmt.Sprintf("平均延迟过高(%.1fms), 建议添加索引或优化SQL", r.SQLMetrics.AvgLatencyMs))
	}

	// 慢查询建议
	if r.SQLMetrics.SlowCount > int(a.thresholds.SlowQueryMax) {
		recommendations = append(recommendations,
			fmt.Sprintf("慢查询过多(%d个), 建议分析执行计划", r.SQLMetrics.SlowCount))
	}

	// 错误率建议
	if r.SQLMetrics.RequestCount > 0 {
		errorRate := float64(r.SQLMetrics.ErrorCount) / float64(r.SQLMetrics.RequestCount) * 100
		if errorRate > 5 {
			recommendations = append(recommendations,
				fmt.Sprintf("错误率较高(%.2f%%), 建议检查SQL语法或业务逻辑", errorRate))
		}
	}

	// SQL类型特定建议
	switch r.SQLType {
	case SQLTypeSelect:
		if r.SQLMetrics.AvgLatencyMs > 100 {
			recommendations = append(recommendations, "SELECT查询较慢, 建议检查是否需要添加索引")
		}
	case SQLTypeInsert:
		if r.SQLMetrics.AvgLatencyMs > 50 {
			recommendations = append(recommendations, "INSERT性能较差, 建议检查批量插入或索引数量")
		}
	case SQLTypeUpdate:
		if r.SQLMetrics.AvgLatencyMs > 100 {
			recommendations = append(recommendations, "UPDATE操作较慢, 建议优化WHERE条件")
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "性能指标正常, 继续监控")
	}

	return recommendations
}

// DetectAlerts 检测告警
func (a *CorrelationAnalyzer) DetectAlerts(processStats map[uint32]DBProcessStats, sqlAgg map[SQLAggKey]SQLAggValue) []AlertInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	var alerts []AlertInfo
	now := time.Now()

	// 检测进程告警
	for pid, stats := range processStats {
		// CPU告警
		cpuPercent := stats.CPUUsagePercent()
		if cpuPercent > a.thresholds.CPUMax {
			alerts = append(alerts, AlertInfo{
				AlertType:   "cpu_high",
				Severity:    "warning",
				PID:         pid,
				Message:     fmt.Sprintf("进程 %d CPU使用率过高: %.1f%%", pid, cpuPercent),
				Threshold:   a.thresholds.CPUMax,
				ActualValue: cpuPercent,
				Timestamp:   now,
			})
		}

		// 内存告警
		memMB := stats.MemoryMB()
		if memMB > a.thresholds.MemoryMaxMB {
			alerts = append(alerts, AlertInfo{
				AlertType:   "memory_high",
				Severity:    "warning",
				PID:         pid,
				Message:     fmt.Sprintf("进程 %d 内存使用过高: %.1fMB", pid, memMB),
				Threshold:   a.thresholds.MemoryMaxMB,
				ActualValue: memMB,
				Timestamp:   now,
			})
		}

		// 连接数告警
		if stats.Connections > a.thresholds.ConnMax {
			alerts = append(alerts, AlertInfo{
				AlertType:   "connections_high",
				Severity:    "warning",
				PID:         pid,
				Message:     fmt.Sprintf("进程 %d 连接数过多: %d", pid, stats.Connections),
				Threshold:   float64(a.thresholds.ConnMax),
				ActualValue: float64(stats.Connections),
				Timestamp:   now,
			})
		}

		// 慢查询告警
		if stats.SlowQueries > a.thresholds.SlowQueryMax {
			alerts = append(alerts, AlertInfo{
				AlertType:   "slow_queries",
				Severity:    "info",
				PID:         pid,
				Message:     fmt.Sprintf("进程 %d 慢查询数过多: %d", pid, stats.SlowQueries),
				Threshold:   float64(a.thresholds.SlowQueryMax),
				ActualValue: float64(stats.SlowQueries),
				Timestamp:   now,
			})
		}
	}

	// 按SQL聚合检测告警
	for key, stats := range sqlAgg {
		if stats.RequestCount == 0 {
			continue
		}

		// 延迟告警
		avgLatMs := float64(stats.TotalLatencyNs) / float64(stats.RequestCount) / 1_000_000
		if avgLatMs > a.thresholds.LatencyMaxMs {
			alerts = append(alerts, AlertInfo{
				AlertType:   "latency_high",
				Severity:    "warning",
				PID:         key.PID,
				Message:     fmt.Sprintf("SQL类型 %s 延迟过高: %.1fms", key.SQLType, avgLatMs),
				Threshold:   a.thresholds.LatencyMaxMs,
				ActualValue: avgLatMs,
				Timestamp:   now,
			})
		}

		// 错误率告警
		if stats.ErrorCount > 0 {
			errorRate := float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
			if errorRate > 10 {
				alerts = append(alerts, AlertInfo{
					AlertType:   "error_rate_high",
					Severity:    "critical",
					PID:         key.PID,
					Message:     fmt.Sprintf("SQL错误率过高: %.2f%%", errorRate),
					Threshold:   10,
					ActualValue: errorRate,
					Timestamp:   now,
				})
			}
		}
	}

	a.alerts = append(a.alerts, alerts...)
	atomic.AddUint64(&a.stats.alertCount, uint64(len(alerts)))

	return alerts
}

// GetTrends 获取性能趋势
func (a *CorrelationAnalyzer) GetTrends() map[uint32][]PerformanceTrend {
	a.mu.RLock()
	defer a.mu.RUnlock()

	trends := make(map[uint32][]PerformanceTrend)
	for pid, snapshots := range a.snapshots {
		if len(snapshots) < 2 {
			continue
		}

		// CPU趋势
		cpuTrend := a.calculateTrend(snapshots, "cpu")
		if cpuTrend != nil {
			trends[pid] = append(trends[pid], *cpuTrend)
		}

		// 内存趋势
		memTrend := a.calculateTrend(snapshots, "memory")
		if memTrend != nil {
			trends[pid] = append(trends[pid], *memTrend)
		}

		// 延迟趋势
		latTrend := a.calculateTrend(snapshots, "latency")
		if latTrend != nil {
			trends[pid] = append(trends[pid], *latTrend)
		}
	}

	return trends
}

// calculateTrend 计算趋势
func (a *CorrelationAnalyzer) calculateTrend(snapshots []PerformanceSnapshot, metric string) *PerformanceTrend {
	if len(snapshots) < 2 {
		return nil
	}

	trend := &PerformanceTrend{
		PID:     snapshots[0].PID,
		Metric:  metric,
		Samples: snapshots,
	}

	var sumX, sumY, sumXY, sumXX float64
	n := float64(len(snapshots))

	for i, s := range snapshots {
		x := float64(i)
		var y float64

		switch metric {
		case "cpu":
			y = s.CPUPercent
		case "memory":
			y = s.MemoryMB
		case "latency":
			y = s.SQLLatencyMs
		default:
			return nil
		}

		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	// 线性回归斜率
	denom := n*sumXX - sumX*sumX
	if denom == 0 {
		return nil
	}

	slope := (n*sumXY - sumX*sumY) / denom
	trend.ChangeRate = slope

	// 确定趋势方向
	avgY := sumY / n
	if avgY > 0 {
		normalizedSlope := slope / avgY * 100 // 百分比变化率
		if normalizedSlope > 5 {
			trend.Direction = TrendDegrading
		} else if normalizedSlope < -5 {
			trend.Direction = TrendImproving
		} else {
			trend.Direction = TrendStable
		}
	}

	return trend
}

// GetAlerts 获取最近告警
func (a *CorrelationAnalyzer) GetAlerts(limit int) []AlertInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alerts := a.alerts
	if len(alerts) > limit {
		return alerts[len(alerts)-limit:]
	}
	return alerts
}

// GetStats 获取统计信息
func (a *CorrelationAnalyzer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"analyze_count":     atomic.LoadUint64(&a.stats.analyzeCount),
		"alert_count":       atomic.LoadUint64(&a.stats.alertCount),
		"last_analyze_time": atomic.LoadInt64(&a.stats.lastAnalyzeTime),
		"snapshot_pids":     len(a.snapshots),
		"thresholds":        a.thresholds,
	}
}

// UpdateThresholds 更新阈值
func (a *CorrelationAnalyzer) UpdateThresholds(cpuMax, memMaxMB, latencyMaxMs float64, slowMax uint64, connMax uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.thresholds.CPUMax = cpuMax
	a.thresholds.MemoryMaxMB = memMaxMB
	a.thresholds.LatencyMaxMs = latencyMaxMs
	a.thresholds.SlowQueryMax = slowMax
	a.thresholds.ConnMax = connMax
}

// ClearAlerts 清除告警
func (a *CorrelationAnalyzer) ClearAlerts() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.alerts = nil
}
