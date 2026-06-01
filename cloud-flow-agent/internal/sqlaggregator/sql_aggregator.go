//go:build linux

// Package sqlaggregator 提供MySQL SQL聚合统计分析功能
// 按SQL语句聚合: 请求数、成功率、平均时延、异常统计
// 关联数据库进程性能数据
package sqlaggregator

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"

	"cloud-flow-agent/internal/config"
	edge "cloud-flow/proto"
)

// SQLType SQL语句类型
type SQLType uint8

const (
	SQLTypeOther  SQLType = 0
	SQLTypeSelect SQLType = 1
	SQLTypeInsert SQLType = 2
	SQLTypeUpdate SQLType = 3
	SQLTypeDelete SQLType = 4
	SQLTypeDDL    SQLType = 5 // CREATE, ALTER, DROP
	SQLTypeDCL    SQLType = 6 // GRANT, REVOKE
)

// SQLTypeNames SQL类型名称映射
var SQLTypeNames = map[SQLType]string{
	SQLTypeOther:  "OTHER",
	SQLTypeSelect: "SELECT",
	SQLTypeInsert: "INSERT",
	SQLTypeUpdate: "UPDATE",
	SQLTypeDelete: "DELETE",
	SQLTypeDDL:    "DDL",
	SQLTypeDCL:    "DCL",
}

// String 返回SQL类型名称
func (t SQLType) String() string {
	if name, ok := SQLTypeNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

// SQL聚合键 (对应eBPF中的sql_agg_key)
type SQLAggKey struct {
	PID      uint32
	NetNS    uint32
	SQLType  SQLType
	Database string
	Table    string
	CmdType  uint8
}

// SQL聚合统计值 (对应eBPF中的sql_agg_value)
type SQLAggValue struct {
	RequestCount    uint64 // 请求数
	SuccessCount    uint64 // 成功数
	ErrorCount      uint64 // 异常数
	TotalLatencyNs  uint64 // 总时延(纳秒)
	AvgLatencyNs    uint64 // 平均时延(纳秒)
	MaxLatencyNs    uint64 // 最大时延
	MinLatencyNs    uint64 // 最小时延
	LastTimestamp   uint64 // 最后请求时间
	TimeoutCount    uint64 // 超时次数
	RetryCount      uint64 // 重试次数
}

// SuccessRate 计算成功率
func (v *SQLAggValue) SuccessRate() float64 {
	if v.RequestCount == 0 {
		return 0
	}
	return float64(v.SuccessCount) / float64(v.RequestCount) * 100
}

// ErrorRate 计算异常率
func (v *SQLAggValue) ErrorRate() float64 {
	if v.RequestCount == 0 {
		return 0
	}
	return float64(v.ErrorCount) / float64(v.RequestCount) * 100
}

// AvgLatencyMs 返回平均时延(毫秒)
func (v *SQLAggValue) AvgLatencyMs() float64 {
	return float64(v.AvgLatencyNs) / 1_000_000
}

// 数据库进程性能统计 (对应eBPF中的db_process_stats)
type DBProcessStats struct {
	PID             uint32
	CPUtimeNs       uint64 // CPU时间(纳秒)
	MemoryRSS       uint64 // 内存RSS(字节)
	IOReadBytes     uint64 // I/O读字节
	IOWriteBytes    uint64 // I/O写字节
	Connections     uint64 // 当前连接数
	QueriesPerSec   uint64 // 每秒查询数
	Transactions     uint64 // 事务数
	LockWaits       uint64 // 锁等待次数
	SlowQueries     uint64 // 慢查询数
	LastUpdate      uint64 // 最后更新时间
}

// CPUUsagePercent 计算CPU使用率百分比
func (s *DBProcessStats) CPUUsagePercent() float64 {
	// 假设采样周期为1秒
	return float64(s.CPUtimeNs) / 10_000_000 // 归一化到百分比
}

// MemoryMB 返回内存使用(MB)
func (s *DBProcessStats) MemoryMB() float64 {
	return float64(s.MemoryRSS) / (1024 * 1024)
}

// 全局SQL聚合统计
type GlobalSQLStats struct {
	TotalRequests uint64 // 总请求数
	TotalSuccess  uint64 // 总成功数
	TotalErrors   uint64 // 总错误数
	AvgLatencyNs  uint64 // 全局平均时延
	SlowQueries   uint64 // 慢查询总数
	Queries1s     uint64 // 1秒内查询数
	Queries10s    uint64 // 10秒内查询数
	Queries60s    uint64 // 60秒内查询数
	LastFlush     uint64 // 最后刷新时间
}

// SuccessRate 计算成功率
func (g *GlobalSQLStats) SuccessRate() float64 {
	if g.TotalRequests == 0 {
		return 0
	}
	return float64(g.TotalSuccess) / float64(g.TotalRequests) * 100
}

// ErrorRate 计算异常率
func (g *GlobalSQLStats) ErrorRate() float64 {
	if g.TotalRequests == 0 {
		return 0
	}
	return float64(g.TotalErrors) / float64(g.TotalRequests) * 100
}

// AvgLatencyMs 返回平均时延(毫秒)
func (g *GlobalSQLStats) AvgLatencyMs() float64 {
	return float64(g.AvgLatencyNs) / 1_000_000
}

// SQL聚合记录
type SQLAggRecord struct {
	Key   SQLAggKey
	Value SQLAggValue
}

// SQLAggregator SQL聚合器
type SQLAggregator struct {
	objs           *MySQLSQLAggObjects
	links          []link.Link
	stopCh         chan struct{}
	collectCh      chan []*edge.MetricData
	mu             sync.RWMutex
	enabled        atomic.Bool
	slowThreshold  atomic.Uint64 // 慢SQL阈值(纳秒),默认1秒

	// 内存缓存的聚合数据
	sqlAggregation map[SQLAggKey]SQLAggValue
	dbProcessStats map[uint32]DBProcessStats
	globalStats    GlobalSQLStats

	// 统计计数
	statsCollCount  atomic.Uint64
	statsErrorCount atomic.Uint64
}

// MySQLSQLAggObjects eBPF对象(需要先编译)
type MySQLSQLAggObjects struct {
	*bpf.SQLEvents
	*bpf.SQLAggregation
	*bpf.SQLGlobalStats
	*bpf.DBProcessStats
	*bpf.SlowQueryThreshold
}

// Close 关闭资源
func (o *MySQLSQLAggObjects) Close() error {
	if o.SQLEvents != nil {
		o.SQLEvents.Close()
	}
	if o.SQLAggregation != nil {
		o.SQLAggregation.Close()
	}
	if o.SQLGlobalStats != nil {
		o.SQLGlobalStats.Close()
	}
	if o.DBProcessStats != nil {
		o.DBProcessStats.Close()
	}
	if o.SlowQueryThreshold != nil {
		o.SlowQueryThreshold.Close()
	}
	return nil
}

// SQLAggregatorOptions 聚合器配置选项
type SQLAggregatorOptions struct {
	EnableMySQLSQLAgg bool  // 启用MySQL SQL聚合
	SlowQueryThresholdMs uint64 // 慢SQL阈值(毫秒)
}

// New 创建SQL聚合器
func New() (*SQLAggregator, error) {
	return NewWithOptions(&SQLAggregatorOptions{
		EnableMySQLSQLAgg: true,
		SlowQueryThresholdMs: 1000, // 默认1秒
	})
}

// NewWithOptions 使用选项创建SQL聚合器
func NewWithOptions(opts *SQLAggregatorOptions) (*SQLAggregator, error) {
	if opts == nil {
		opts = &SQLAggregatorOptions{
			EnableMySQLSQLAgg: true,
			SlowQueryThresholdMs: 1000,
		}
	}

	// 移除内存限制
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("移除内存限制失败: %w", err)
	}

	// 查找eBPF对象文件
	bpfDir := config.GetBPFObjectDir()
	bpfPath := filepath.Join(bpfDir, "mysql_sql_agg_bpf.o")

	if _, err := os.Stat(bpfPath); os.IsNotExist(err) {
		log.Printf("[SQL聚合器] eBPF对象文件不存在: %s, 将跳过初始化", bpfPath)
		return &SQLAggregator{
			enabled: atomic.Bool{},
		}, nil
	}

	// 加载eBPF规范
	spec, err := ebpf.LoadLoadSpecFromFile(bpfPath)
	if err != nil {
		return nil, fmt.Errorf("加载eBPF规范失败: %w", err)
	}

	// 创建eBPF集合
	var objs MySQLSQLAggObjects
	if err := spec.LoadAndAssign(&objs, &ebpf.CollectionOptions{
		MapReplacements: map[string]*ebpf.Map{},
	}); err != nil {
		return nil, fmt.Errorf("创建eBPF对象失败: %w", err)
	}

	// 创建聚合器
	agg := &SQLAggregator{
		objs:            &objs,
		stopCh:          make(chan struct{}),
		collectCh:       make(chan []*edge.MetricData, 100),
		sqlAggregation:  make(map[SQLAggKey]SQLAggValue),
		dbProcessStats:  make(map[uint32]DBProcessStats),
		slowThreshold:   atomic.Uint64{},
	}

	// 设置慢SQL阈值
	agg.slowThreshold.Store(opts.SlowQueryThresholdMs * 1_000_000)

	// 初始化全局统计
	agg.globalStats = GlobalSQLStats{
		LastFlush: uint64(time.Now().UnixNano()),
	}

	agg.enabled.Store(true)

	log.Printf("[SQL聚合器] 初始化完成, 慢SQL阈值: %dms", opts.SlowQueryThresholdMs)
	return agg, nil
}

// Start 启动聚合器
func (a *SQLAggregator) Start() error {
	if !a.enabled.Load() {
		return nil
	}

	if a.objs == nil {
		return nil
	}

	// 启动收集循环
	go a.collectLoop()

	// 启动定时刷新循环
	go a.flushLoop()

	log.Printf("[SQL聚合器] 启动成功")
	return nil
}

// Stop 停止聚合器
func (a *SQLAggregator) Stop() {
	if !a.enabled.Load() {
		return
	}

	a.enabled.Store(false)
	close(a.stopCh)

	// 关闭eBPF链接
	for _, l := range a.links {
		l.Close()
	}

	// 关闭eBPF对象
	if a.objs != nil {
		a.objs.Close()
	}

	log.Printf("[SQL聚合器] 已停止")
}

// collectLoop 收集循环
func (a *SQLAggregator) collectLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.collect()
		}
	}
}

// flushLoop 定时刷新循环
func (a *SQLAggregator) flushLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.flush()
		}
	}
}

// collect 收集BPF Map数据
func (a *SQLAggregator) collect() {
	if a.objs == nil || a.objs.SQLAggregation == nil {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	var key SQLAggKey
	var value sqlAggValue

	// 遍历SQL聚合Map
	iter := a.objs.SQLAggregation.Iterate()
	for iter.Next(&key, &value) {
		aggKey := convertKey(key)
		a.sqlAggregation[aggKey] = convertValue(value)
	}

	// 遍历进程统计Map
	var pidKey uint32
	var procValue dbProcessValue
	iter = a.objs.DBProcessStats.Iterate()
	for iter.Next(&pidKey, &procValue) {
		a.dbProcessStats[pidKey] = convertProcessStats(procValue)
	}

	// 读取全局统计
	if a.objs.SQLGlobalStats != nil {
		var gkey uint32
		var gstats globalStatsValue
		if err := a.objs.SQLGlobalStats.Lookup(&gkey, &gstats); err == nil {
			a.globalStats = convertGlobalStats(gstats)
		}
	}

	// 更新慢SQL阈值
	var threshKey uint32
	var threshold uint64
	if a.objs.SlowQueryThreshold != nil {
		if err := a.objs.SlowQueryThreshold.Lookup(&threshKey, &threshold); err == nil {
			a.slowThreshold.Store(threshold)
		}
	}

	atomic.AddUint64(&a.statsCollCount, 1)
}

// flush 刷新统计数据
func (a *SQLAggregator) flush() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().UnixNano()

	// 重置时间窗口统计
	a.globalStats.Queries1s = 0
	a.globalStats.Queries10s = 0
	a.globalStats.Queries60s = 0
	a.globalStats.LastFlush = uint64(now)

	log.Printf("[SQL聚合器] 刷新统计: 总请求=%d, 成功=%d, 错误=%d, 慢查询=%d",
		a.globalStats.TotalRequests,
		a.globalStats.TotalSuccess,
		a.globalStats.TotalErrors,
		a.globalStats.SlowQueries)
}

// GetSQLAggregation 获取SQL聚合数据
func (a *SQLAggregator) GetSQLAggregation() []SQLAggRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	records := make([]SQLAggRecord, 0, len(a.sqlAggregation))
	for k, v := range a.sqlAggregation {
		records = append(records, SQLAggRecord{Key: k, Value: v})
	}
	return records
}

// GetDBProcessStats 获取数据库进程性能数据
func (a *SQLAggregator) GetDBProcessStats() map[uint32]DBProcessStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[uint32]DBProcessStats, len(a.dbProcessStats))
	for k, v := range a.dbProcessStats {
		result[k] = v
	}
	return result
}

// GetGlobalStats 获取全局SQL统计
func (a *SQLAggregator) GetGlobalStats() GlobalSQLStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.globalStats
}

// GetMetrics 获取指标数据
func (a *SQLAggregator) GetMetrics() []*edge.MetricData {
	if !a.enabled.Load() {
		return nil
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var metrics []*edge.MetricData

	// 全局SQL统计指标
	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_total_requests",
		Value:     float64(a.globalStats.TotalRequests),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "counter"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_success_count",
		Value:     float64(a.globalStats.TotalSuccess),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "counter"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_error_count",
		Value:     float64(a.globalStats.TotalErrors),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "counter"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_success_rate",
		Value:     a.globalStats.SuccessRate(),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "gauge", "unit": "percent"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_avg_latency_ms",
		Value:     a.globalStats.AvgLatencyMs(),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "gauge", "unit": "ms"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_slow_queries",
		Value:     float64(a.globalStats.SlowQueries),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "counter"},
	})

	metrics = append(metrics, &edge.MetricData{
		Name:      "sql_queries_per_sec",
		Value:     float64(a.globalStats.Queries1s),
		Timestamp: time.Now().Unix(),
		Labels:    map[string]string{"type": "gauge", "unit": "qps"},
	})

	// 按SQL类型聚合
	typeAgg := make(map[SQLType]SQLAggValue)
	for _, rec := range a.sqlAggregation {
		typeAgg[rec.Key.SQLType] = SQLAggValue{
			RequestCount:  typeAgg[rec.Key.SQLType].RequestCount + rec.Value.RequestCount,
			SuccessCount:  typeAgg[rec.Key.SQLType].SuccessCount + rec.Value.SuccessCount,
			ErrorCount:    typeAgg[rec.Key.SQLType].ErrorCount + rec.Value.ErrorCount,
			TotalLatencyNs: typeAgg[rec.Key.SQLType].TotalLatencyNs + rec.Value.TotalLatencyNs,
		}
	}

	for sqlType, agg := range typeAgg {
		if agg.RequestCount > 0 {
			avgLat := float64(agg.TotalLatencyNs) / float64(agg.RequestCount)
			metrics = append(metrics, &edge.MetricData{
				Name:      fmt.Sprintf("sql_type_%s_requests", sqlType.String()),
				Value:     float64(agg.RequestCount),
				Timestamp: time.Now().Unix(),
				Labels:    map[string]string{"type": "counter", "sql_type": sqlType.String()},
			})

			metrics = append(metrics, &edge.MetricData{
				Name:      fmt.Sprintf("sql_type_%s_latency_ms", sqlType.String()),
				Value:     avgLat / 1_000_000,
				Timestamp: time.Now().Unix(),
				Labels:    map[string]string{"type": "gauge", "unit": "ms", "sql_type": sqlType.String()},
			})
		}
	}

	// 关联进程性能指标
	for pid, stats := range a.dbProcessStats {
		metrics = append(metrics, &edge.MetricData{
			Name:      "db_process_queries",
			Value:     float64(stats.QueriesPerSec),
			Timestamp: time.Now().Unix(),
			Labels:    map[string]string{"type": "gauge", "pid": fmt.Sprintf("%d", pid)},
		})

		metrics = append(metrics, &edge.MetricData{
			Name:      "db_process_connections",
			Value:     float64(stats.Connections),
			Timestamp: time.Now().Unix(),
			Labels:    map[string]string{"type": "gauge", "pid": fmt.Sprintf("%d", pid)},
		})

		metrics = append(metrics, &edge.MetricData{
			Name:      "db_process_slow_queries",
			Value:     float64(stats.SlowQueries),
			Timestamp: time.Now().Unix(),
			Labels:    map[string]string{"type": "counter", "pid": fmt.Sprintf("%d", pid)},
		})
	}

	return metrics
}

// GetSlowQueries 获取慢SQL列表
func (a *SQLAggregator) GetSlowQueries(thresholdMs uint64) []SQLAggRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var slowQueries []SQLAggRecord
	thresholdNs := thresholdMs * 1_000_000

	for _, rec := range a.sqlAggregation {
		if rec.Value.AvgLatencyNs > thresholdNs || rec.Value.MaxLatencyNs > thresholdNs {
			slowQueries = append(slowQueries, rec)
		}
	}

	return slowQueries
}

// GetErrorSQLs 获取异常SQL列表
func (a *SQLAggregator) GetErrorSQLs() []SQLAggRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var errorSQLs []SQLAggRecord
	for _, rec := range a.sqlAggregation {
		if rec.Value.ErrorCount > 0 {
			errorSQLs = append(errorSQLs, rec)
		}
	}

	return errorSQLs
}

// GetStats 获取运行时统计
func (a *SQLAggregator) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":           a.enabled.Load(),
		"collection_count":  atomic.LoadUint64(&a.statsCollCount),
		"error_count":       atomic.LoadUint64(&a.statsErrorCount),
		"slow_threshold_ms": a.slowThreshold.Load() / 1_000_000,
		"sql_agg_count":     len(a.sqlAggregation),
		"process_count":      len(a.dbProcessStats),
	}
}

// 内部类型转换函数

type sqlAggKey struct {
	PID      uint32
	NetNS    uint32
	SQLType  uint8
	Database [32]byte
	Table    [32]byte
	CmdType  uint8
}

type sqlAggValue struct {
	RequestCount   uint64
	SuccessCount   uint64
	ErrorCount     uint64
	TotalLatencyNs uint64
	AvgLatencyNs   uint64
	MaxLatencyNs   uint64
	MinLatencyNs   uint64
	LastTimestamp  uint64
	TimeoutCount   uint64
	RetryCount     uint64
}

type dbProcessValue struct {
	PID            uint32
	CPUtimeNs      uint64
	MemoryRSS      uint64
	IOReadBytes    uint64
	IOWriteBytes   uint64
	Connections    uint64
	QueriesPerSec  uint64
	Transactions   uint64
	LockWaits      uint64
	SlowQueries    uint64
	LastUpdate     uint64
}

type globalStatsValue struct {
	TotalRequests uint64
	TotalSuccess  uint64
	TotalErrors   uint64
	AvgLatencyNs  uint64
	SlowQueries   uint64
	Queries1s     uint64
	Queries10s    uint64
	Queries60s    uint64
	LastFlush     uint64
}

func convertKey(k sqlAggKey) SQLAggKey {
	return SQLAggKey{
		PID:      k.PID,
		NetNS:    k.NetNS,
		SQLType:  SQLType(k.SQLType),
		Database: string(k.Database[:]),
		Table:    string(k.Table[:]),
		CmdType:  k.CmdType,
	}
}

func convertValue(v sqlAggValue) SQLAggValue {
	return SQLAggValue{
		RequestCount:   v.RequestCount,
		SuccessCount:   v.SuccessCount,
		ErrorCount:     v.ErrorCount,
		TotalLatencyNs: v.TotalLatencyNs,
		AvgLatencyNs:   v.AvgLatencyNs,
		MaxLatencyNs:   v.MaxLatencyNs,
		MinLatencyNs:   v.MinLatencyNs,
		LastTimestamp:  v.LastTimestamp,
		TimeoutCount:   v.TimeoutCount,
		RetryCount:     v.RetryCount,
	}
}

func convertProcessStats(v dbProcessValue) DBProcessStats {
	return DBProcessStats{
		PID:            v.PID,
		CPUtimeNs:      v.CPUtimeNs,
		MemoryRSS:      v.MemoryRSS,
		IOReadBytes:    v.IOReadBytes,
		IOWriteBytes:   v.IOWriteBytes,
		Connections:    v.Connections,
		QueriesPerSec:  v.QueriesPerSec,
		Transactions:   v.Transactions,
		LockWaits:      v.LockWaits,
		SlowQueries:    v.SlowQueries,
		LastUpdate:     v.LastUpdate,
	}
}

func convertGlobalStats(v globalStatsValue) GlobalSQLStats {
	return GlobalSQLStats{
		TotalRequests: v.TotalRequests,
		TotalSuccess:  v.TotalSuccess,
		TotalErrors:   v.TotalErrors,
		AvgLatencyNs:  v.AvgLatencyNs,
		SlowQueries:   v.SlowQueries,
		Queries1s:     v.Queries1s,
		Queries10s:    v.Queries10s,
		Queries60s:    v.Queries60s,
		LastFlush:     v.LastFlush,
	}
}
