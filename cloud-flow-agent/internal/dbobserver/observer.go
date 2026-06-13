// Package dbobserver 提供数据库观测功能
// 本文件实现数据库观测核心模块，支持 SQL 语句级别的性能监控
// 支持 MySQL、Oracle、PostgreSQL、达梦、GaussDB
package dbobserver

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ==================== 数据库类型定义 ====================

// DatabaseType 数据库类型
type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"       // MySQL
	DatabaseTypeOracle     DatabaseType = "oracle"      // Oracle
	DatabaseTypePostgreSQL DatabaseType = "postgresql"  // PostgreSQL
	DatabaseTypeDaMeng     DatabaseType = "dameng"      // 达梦
	DatabaseTypeGaussDB    DatabaseType = "gaussdb"     // GaussDB
	DatabaseTypeUnknown    DatabaseType = "unknown"     // 未知
)

// SQLType SQL 操作类型
type SQLType string

const (
	SQLTypeSelect  SQLType = "SELECT"  // 查询
	SQLTypeInsert  SQLType = "INSERT"  // 插入
	SQLTypeUpdate  SQLType = "UPDATE"  // 更新
	SQLTypeDelete  SQLType = "DELETE"  // 删除
	SQLTypeDDL     SQLType = "DDL"     // 数据定义语言
	SQLTypeDCL     SQLType = "DCL"     // 数据控制语言
	SQLTypeOther   SQLType = "OTHER"   // 其他
)

// ==================== SQL 事件定义 ====================

// SQLEvent SQL 执行事件
type SQLEvent struct {
	Timestamp     int64        `json:"timestamp"`      // 事件时间戳(纳秒)
	DatabaseType  DatabaseType `json:"database_type"`  // 数据库类型
	DatabaseName  string       `json:"database_name"`  // 数据库名
	ServerAddr    string       `json:"server_addr"`    // 数据库服务器地址
	ServerPort    int          `json:"server_port"`    // 数据库服务器端口
	ClientAddr    string       `json:"client_addr"`    // 客户端地址
	ClientPort    int          `json:"client_port"`    // 客户端端口

	// SQL 信息
	SQLText       string       `json:"sql_text"`       // SQL 文本
	SQLTemplate   string       `json:"sql_template"`   // SQL 模板（参数化后的）
	SQLType       SQLType      `json:"sql_type"`       // SQL 类型
	SQLFingerprint string      `json:"sql_fingerprint"` // SQL 指纹（用于聚合）

	// 执行信息
	Duration      int64        `json:"duration"`       // 执行时长(微秒)
	RowsAffected  int64        `json:"rows_affected"`  // 影响行数
	RowsReturned  int64        `json:"rows_returned"`  // 返回行数
	BytesSent     int64        `json:"bytes_sent"`     // 发送字节数
	BytesReceived int64        `json:"bytes_received"` // 接收字节数

	// 结果信息
	Success       bool         `json:"success"`        // 是否成功
	ErrorCode     string       `json:"error_code"`     // 错误码
	ErrorMessage  string       `json:"error_message"`  // 错误消息

	// 关联信息
	PID           uint32       `json:"pid"`            // 数据库进程 PID
	ProcessName   string       `json:"process_name"`   // 进程名
	SessionID     string       `json:"session_id"`     // 会话 ID
	TransactionID string       `json:"transaction_id"` // 事务 ID

	// 扩展信息
	User          string       `json:"user"`           // 用户名
	Schema        string       `json:"schema"`         // 模式名
	Table         string       `json:"table"`          // 主表名
	Index         string       `json:"index"`          // 索引名
}

// ==================== SQL 聚合统计 ====================

// SQLStats SQL 聚合统计
type SQLStats struct {
	// SQL 标识
	SQLFingerprint string       `json:"sql_fingerprint"` // SQL 指纹
	SQLTemplate    string       `json:"sql_template"`    // SQL 模板
	SQLType        SQLType      `json:"sql_type"`        // SQL 类型
	DatabaseType   DatabaseType `json:"database_type"`   // 数据库类型
	DatabaseName   string       `json:"database_name"`   // 数据库名

	// 请求统计
	RequestCount   int64        `json:"request_count"`   // 请求次数
	SuccessCount   int64        `json:"success_count"`   // 成功次数
	ErrorCount     int64        `json:"error_count"`     // 错误次数
	SuccessRate    float64      `json:"success_rate"`    // 成功率

	// 时延统计
	TotalDuration  int64        `json:"total_duration"`  // 总时延(微秒)
	MinDuration    int64        `json:"min_duration"`    // 最小时延
	MaxDuration    int64        `json:"max_duration"`    // 最大时延
	AvgDuration    float64      `json:"avg_duration"`    // 平均时延
	P50Duration    int64        `json:"p50_duration"`    // P50 时延
	P90Duration    int64        `json:"p90_duration"`    // P90 时延
	P99Duration    int64        `json:"p99_duration"`    // P99 时延

	// 数据量统计
	TotalRowsAffected  int64    `json:"total_rows_affected"`  // 总影响行数
	TotalRowsReturned  int64    `json:"total_rows_returned"`  // 总返回行数
	TotalBytesSent     int64    `json:"total_bytes_sent"`     // 总发送字节
	TotalBytesReceived int64    `json:"total_bytes_received"` // 总接收字节

	// 时间窗口
	FirstSeen      time.Time    `json:"first_seen"`      // 首次出现时间
	LastSeen       time.Time    `json:"last_seen"`       // 最后出现时间
	LastUpdateTime time.Time    `json:"last_update_time"` // 最后更新时间

	// 错误统计
	ErrorCodes     map[string]int64 `json:"error_codes"` // 错误码分布

	// 关联进程
	ProcessPID     uint32       `json:"process_pid"`    // 关联进程 PID
	ProcessName    string       `json:"process_name"`   // 进程名
}

// ==================== 慢查询定义 ====================

// SlowQuery 慢查询信息
type SlowQuery struct {
	// 基本信息
	Timestamp     int64        `json:"timestamp"`      // 时间戳
	SQLText       string       `json:"sql_text"`       // SQL 文本
	SQLTemplate   string       `json:"sql_template"`   // SQL 模板
	SQLType       SQLType      `json:"sql_type"`       // SQL 类型
	DatabaseType  DatabaseType `json:"database_type"`  // 数据库类型
	DatabaseName  string       `json:"database_name"`  // 数据库名

	// 执行信息
	Duration      int64        `json:"duration"`       // 执行时长(微秒)
	RowsAffected  int64        `json:"rows_affected"`  // 影响行数
	RowsReturned  int64        `json:"rows_returned"`  // 返回行数

	// 慢查询分析
	IsSlowQuery   bool         `json:"is_slow_query"`  // 是否慢查询
	SlowThreshold int64        `json:"slow_threshold"` // 慢查询阈值(微秒)
	SlowReason    string       `json:"slow_reason"`    // 慢查询原因
	SlowScore     float64      `json:"slow_score"`     // 慢查询评分

	// 执行计划（可选）
	ExplainPlan   string       `json:"explain_plan"`   // 执行计划

	// 关联信息
	ClientAddr    string       `json:"client_addr"`    // 客户端地址
	ServerAddr    string       `json:"server_addr"`    // 服务器地址
	PID           uint32       `json:"pid"`            // 进程 PID
	User          string       `json:"user"`           // 用户名
	Schema        string       `json:"schema"`         // 模式名
	Table         string       `json:"table"`          // 主表名

	// 进程指标
	ProcessCPU    float64      `json:"process_cpu"`    // 进程 CPU 使用率
	ProcessMemory int64        `json:"process_memory"` // 进程内存使用(字节)
	ProcessIORead int64        `json:"process_io_read"` // 进程 IO 读(字节)
	ProcessIOWrite int64       `json:"process_io_write"` // 进程 IO 写(字节)
}

// SlowQueryRank 慢查询排行
type SlowQueryRank struct {
	Rank          int          `json:"rank"`           // 排名
	SQLFingerprint string       `json:"sql_fingerprint"` // SQL 指纹
	SQLTemplate   string       `json:"sql_template"`   // SQL 模板
	SlowCount     int64        `json:"slow_count"`     // 慢查询次数
	TotalDuration int64        `json:"total_duration"` // 总时延(微秒)
	MaxDuration   int64        `json:"max_duration"`   // 最大时延
	AvgDuration   float64      `json:"avg_duration"`   // 平均时延
	DatabaseName  string       `json:"database_name"`  // 数据库名
	LastSeen      time.Time    `json:"last_seen"`      // 最后出现时间
}

// ==================== 进程指标关联 ====================

// ProcessMetrics 进程指标
type ProcessMetrics struct {
	PID           uint32    `json:"pid"`             // 进程 PID
	ProcessName   string    `json:"process_name"`    // 进程名
	DatabaseType  DatabaseType `json:"database_type"` // 数据库类型

	// CPU 指标
	CPUUsage      float64   `json:"cpu_usage"`       // CPU 使用率(%)
	CPUTime       int64     `json:"cpu_time"`        // CPU 时间(毫秒)
	UserCPUTime   int64     `json:"user_cpu_time"`   // 用户态 CPU 时间
	SysCPUTime    int64     `json:"sys_cpu_time"`    // 内核态 CPU 时间

	// 内存指标
	MemoryUsage   int64     `json:"memory_usage"`    // 内存使用(字节)
	MemoryRSS     int64     `json:"memory_rss"`      // RSS 内存(字节)
	MemoryVMS     int64     `json:"memory_vms"`      // 虚拟内存(字节)

	// IO 指标
	IOReadBytes   int64     `json:"io_read_bytes"`   // IO 读字节数
	IOWriteBytes  int64     `json:"io_write_bytes"`  // IO 写字节数
	IOReadOps     int64     `json:"io_read_ops"`     // IO 读次数
	IOWriteOps    int64     `json:"io_write_ops"`    // IO 写次数

	// 网络指标
	NetRxBytes    int64     `json:"net_rx_bytes"`    // 网络接收字节
	NetTxBytes    int64     `json:"net_tx_bytes"`    // 网络发送字节
	NetRxPackets  int64     `json:"net_rx_packets"`  // 网络接收包数
	NetTxPackets  int64     `json:"net_tx_packets"`  // 网络发送包数

	// 连接指标
	ConnectionCount int     `json:"connection_count"` // 连接数
	ThreadCount    int      `json:"thread_count"`    // 线程数

	// 时间戳
	Timestamp     time.Time `json:"timestamp"`       // 采集时间
}

// ==================== 观测器配置 ====================

// ObserverConfig 观测器配置
type ObserverConfig struct {
	// 目标数据库
	DatabaseTypes []DatabaseType `json:"database_types"` // 监控的数据库类型

	// 采集配置
	CollectInterval   time.Duration `json:"collect_interval"`   // 采集间隔
	MaxSQLTextLength  int           `json:"max_sql_text_length"` // SQL 文本最大长度

	// 聚合配置
	AggregationWindow time.Duration `json:"aggregation_window"` // 聚合时间窗口
	MaxAggregatedSQLs int           `json:"max_aggregated_sqls"` // 最大聚合 SQL 数

	// 慢查询配置
	SlowQueryThreshold int64        `json:"slow_query_threshold"` // 慢查询阈值(微秒)
	MaxSlowQueries      int         `json:"max_slow_queries"`     // 最大慢查询记录数
	AutoExplain         bool        `json:"auto_explain"`         // 是否自动获取执行计划

	// 进程关联配置
	EnableProcessMetrics bool       `json:"enable_process_metrics"` // 是否采集进程指标
	ProcessMetricsInterval time.Duration `json:"process_metrics_interval"` // 进程指标采集间隔

	// 输出配置
	OutputFormat string `json:"output_format"` // 输出格式: json/prometheus
}

// DefaultObserverConfig 返回默认配置
func DefaultObserverConfig() *ObserverConfig {
	return &ObserverConfig{
		DatabaseTypes: []DatabaseType{
			DatabaseTypeMySQL,
			DatabaseTypeOracle,
			DatabaseTypePostgreSQL,
			DatabaseTypeDaMeng,
			DatabaseTypeGaussDB,
		},
		CollectInterval:     1 * time.Second,
		MaxSQLTextLength:    4096,
		AggregationWindow:   1 * time.Minute,
		MaxAggregatedSQLs:   10000,
		SlowQueryThreshold:  1000000, // 1秒
		MaxSlowQueries:      1000,
		AutoExplain:         false,
		EnableProcessMetrics: true,
		ProcessMetricsInterval: 5 * time.Second,
		OutputFormat:       "json",
	}
}

// ==================== 数据库观测器 ====================

// DBObserver 数据库观测器
type DBObserver struct {
	config    *ObserverConfig
	aggregator *SQLAggregator
	detector   *SlowQueryDetector
	collector  *ProcessMetricsCollector
	parsers    map[DatabaseType]ProtocolParser

	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewDBObserver 创建数据库观测器
func NewDBObserver(cfg *ObserverConfig) *DBObserver {
	if cfg == nil {
		cfg = DefaultObserverConfig()
	}

	observer := &DBObserver{
		config:    cfg,
		aggregator: NewSQLAggregator(cfg),
		detector:   NewSlowQueryDetector(cfg),
		collector:  NewProcessMetricsCollector(cfg),
		parsers:    make(map[DatabaseType]ProtocolParser),
		stopCh:     make(chan struct{}),
	}

	// 初始化协议解析器
	for _, dbType := range cfg.DatabaseTypes {
		observer.parsers[dbType] = NewProtocolParser(dbType)
	}

	return observer
}

// Start 启动观测器
func (o *DBObserver) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return fmt.Errorf("观测器已在运行")
	}
	o.running = true
	o.mu.Unlock()

	// 启动聚合器
	o.wg.Add(1)
	go o.aggregationLoop(ctx)

	// 启动进程指标采集
	if o.config.EnableProcessMetrics {
		o.wg.Add(1)
		go o.processMetricsLoop(ctx)
	}

	// 启动慢查询检测
	o.wg.Add(1)
	go o.slowQueryLoop(ctx)

	return nil
}

// Stop 停止观测器
func (o *DBObserver) Stop() {
	o.mu.Lock()
	if !o.running {
		o.mu.Unlock()
		return
	}
	o.running = false
	close(o.stopCh)
	o.mu.Unlock()

	o.wg.Wait()
}

// aggregationLoop 聚合循环
func (o *DBObserver) aggregationLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(o.config.AggregationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.aggregator.Flush()
		}
	}
}

// processMetricsLoop 进程指标采集循环
func (o *DBObserver) processMetricsLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(o.config.ProcessMetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.collector.Collect()
		}
	}
}

// slowQueryLoop 慢查询检测循环
func (o *DBObserver) slowQueryLoop(ctx context.Context) {
	defer o.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.detector.UpdateRankings()
		}
	}
}

// RecordEvent 记录 SQL 事件
func (o *DBObserver) RecordEvent(event *SQLEvent) {
	// 聚合统计
	o.aggregator.Record(event)

	// 慢查询检测
	if event.Duration > o.config.SlowQueryThreshold {
		o.detector.RecordSlowQuery(event)
	}
}

// GetStats 获取 SQL 统计
func (o *DBObserver) GetStats() map[string]*SQLStats {
	return o.aggregator.GetStats()
}

// GetSlowQueries 获取慢查询列表
func (o *DBObserver) GetSlowQueries(limit int) []*SlowQuery {
	return o.detector.GetSlowQueries(limit)
}

// GetSlowQueryRanking 获取慢查询排行
func (o *DBObserver) GetSlowQueryRanking(limit int) []*SlowQueryRank {
	return o.detector.GetRanking(limit)
}

// GetProcessMetrics 获取进程指标
func (o *DBObserver) GetProcessMetrics(pid uint32) *ProcessMetrics {
	return o.collector.GetMetrics(pid)
}

// GetAllProcessMetrics 获取所有进程指标
func (o *DBObserver) GetAllProcessMetrics() map[uint32]*ProcessMetrics {
	return o.collector.GetAllMetrics()
}

// Close 关闭观测器
func (o *DBObserver) Close() {
	o.Stop()
	o.aggregator.Close()
	o.detector.Close()
	o.collector.Close()
}
