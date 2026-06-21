// P5: 流量分析深度增强 — 协议深度解析与异常检测
package l7parser

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 一、协议深度解析增强器
// ============================================================================

// DeepProtocolAnalyzer 协议深度解析增强器
type DeepProtocolAnalyzer struct {
	mu sync.RWMutex

	// 按协议/表/键聚合的统计
	mysqlStats map[string]*MySQLStats // key: db/table
	redisStats map[string]*RedisStats // key: db/key
	kafkaStats map[string]*KafkaStats // key: topic
	httpStats  map[string]*HTTPStats  // key: host/path
}

// NewDeepProtocolAnalyzer 创建深度解析分析器
func NewDeepProtocolAnalyzer() *DeepProtocolAnalyzer {
	return &DeepProtocolAnalyzer{
		mysqlStats: make(map[string]*MySQLStats),
		redisStats: make(map[string]*RedisStats),
		kafkaStats: make(map[string]*KafkaStats),
		httpStats:  make(map[string]*HTTPStats),
	}
}

// ============================================================================
// MySQL 深度解析
// ============================================================================

// MySQLStats MySQL 深度统计
type MySQLStats struct {
	DBName       string
	TableName    string
	Operation    string // SELECT/INSERT/UPDATE/DELETE

	QueryCount   uint64
	ErrorCount   uint64
	SlowCount    uint64 // > 100ms

	AvgLatencyMs float64
	MaxLatencyMs float64
	P99LatencyMs float64

	Latencies    []float64 // ms, 滑动窗口
	MaxHistory   int

	AffectedRows uint64
	ReturnedRows uint64
}

// AnalyzeMySQL 深度分析 MySQL 请求
func (dpa *DeepProtocolAnalyzer) AnalyzeMySQL(
	db, table, operation string,
	latencyMs float64,
	affectedRows, returnedRows uint64,
	hasError bool,
) *MySQLStats {
	key := fmt.Sprintf("%s/%s/%s", db, table, operation)

	dpa.mu.Lock()
	defer dpa.mu.Unlock()

	stats, exists := dpa.mysqlStats[key]
	if !exists {
		stats = &MySQLStats{
			DBName:     db,
			TableName:  table,
			Operation:  operation,
			MaxHistory: 100,
		}
		dpa.mysqlStats[key] = stats
	}

	stats.QueryCount++
	stats.Latencies = append(stats.Latencies, latencyMs)
	if len(stats.Latencies) > stats.MaxHistory {
		stats.Latencies = stats.Latencies[len(stats.Latencies)-stats.MaxHistory:]
	}
	stats.AvgLatencyMs = avgFloat64(stats.Latencies)
	stats.P99LatencyMs = p99Float64(stats.Latencies)
	if latencyMs > stats.MaxLatencyMs {
		stats.MaxLatencyMs = latencyMs
	}
	if latencyMs > 100 {
		stats.SlowCount++
	}
	if hasError {
		stats.ErrorCount++
	}
	stats.AffectedRows += affectedRows
	stats.ReturnedRows += returnedRows

	return stats
}

// GetMySQLHotTables 获取热点表（按查询次数排序）
func (dpa *DeepProtocolAnalyzer) GetMySQLHotTables(limit int) []*MySQLStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*MySQLStats
	for _, stats := range dpa.mysqlStats {
		all = append(all, stats)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].QueryCount > all[j].QueryCount
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// GetMySQLSlowQueries 获取慢查询统计
func (dpa *DeepProtocolAnalyzer) GetMySQLSlowQueries(limit int) []*MySQLStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*MySQLStats
	for _, stats := range dpa.mysqlStats {
		if stats.SlowCount > 0 {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].AvgLatencyMs > all[j].AvgLatencyMs
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ============================================================================
// Redis 深度解析
// ============================================================================

// RedisStats Redis 深度统计
type RedisStats struct {
	DBName       string
	KeyPattern   string
	Command      string // GET/SET/DEL/HGET/HSET/LPUSH/RPOP...

	CmdCount     uint64
	ErrorCount   uint64
	SlowCount    uint64

	AvgLatencyMs float64
	MaxLatencyMs float64
	P99LatencyMs float64
	Latencies    []float64
	MaxHistory   int

	ValueSize    uint64 // 平均 value 大小
	ValueSamples int

	// 热点key检测
	IsHotKey bool
}

// AnalyzeRedis 深度分析 Redis 请求
func (dpa *DeepProtocolAnalyzer) AnalyzeRedis(
	db, keyPattern, command string,
	latencyMs float64,
	valueSize uint64,
	hasError bool,
) *RedisStats {
	key := fmt.Sprintf("%s/%s/%s", db, keyPattern, command)

	dpa.mu.Lock()
	defer dpa.mu.Unlock()

	stats, exists := dpa.redisStats[key]
	if !exists {
		stats = &RedisStats{
			DBName:     db,
			KeyPattern: keyPattern,
			Command:    command,
			MaxHistory: 100,
		}
		dpa.redisStats[key] = stats
	}

	stats.CmdCount++
	stats.Latencies = append(stats.Latencies, latencyMs)
	if len(stats.Latencies) > stats.MaxHistory {
		stats.Latencies = stats.Latencies[len(stats.Latencies)-stats.MaxHistory:]
	}
	stats.AvgLatencyMs = avgFloat64(stats.Latencies)
	stats.P99LatencyMs = p99Float64(stats.Latencies)
	if latencyMs > stats.MaxLatencyMs {
		stats.MaxLatencyMs = latencyMs
	}
	if latencyMs > 10 {
		stats.SlowCount++
	}
	if hasError {
		stats.ErrorCount++
	}
	if valueSize > 0 {
		stats.ValueSize = (stats.ValueSize*uint64(stats.ValueSamples) + valueSize) / uint64(stats.ValueSamples+1)
		stats.ValueSamples++
	}
	// 热点key: 查询次数 > 1000
	if stats.CmdCount > 1000 {
		stats.IsHotKey = true
	}

	return stats
}

// GetRedisHotKeys 获取热点 Key
func (dpa *DeepProtocolAnalyzer) GetRedisHotKeys(limit int) []*RedisStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*RedisStats
	for _, stats := range dpa.redisStats {
		if stats.IsHotKey {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].CmdCount > all[j].CmdCount
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// GetRedisSlowCommands 获取慢命令
func (dpa *DeepProtocolAnalyzer) GetRedisSlowCommands(limit int) []*RedisStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*RedisStats
	for _, stats := range dpa.redisStats {
		if stats.SlowCount > 0 {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].AvgLatencyMs > all[j].AvgLatencyMs
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ============================================================================
// Kafka 深度解析
// ============================================================================

// KafkaStats Kafka 深度统计
type KafkaStats struct {
	Topic          string
	Operation      string // Produce/Consume
	Partition      int32

	MsgCount       uint64
	ErrorCount     uint64
	SlowCount      uint64

	AvgLatencyMs   float64
	MaxLatencyMs   float64
	P99LatencyMs   float64
	Latencies      []float64
	MaxHistory     int

	AvgMsgSize     uint64
	MsgSizeSamples int

	// 积压检测
	LagEstimate    uint64
}

// AnalyzeKafka 深度分析 Kafka 请求
func (dpa *DeepProtocolAnalyzer) AnalyzeKafka(
	topic, operation string,
	partition int32,
	latencyMs float64,
	msgSize uint64,
	lagEstimate uint64,
	hasError bool,
) *KafkaStats {
	key := fmt.Sprintf("%s/%s/%d", topic, operation, partition)

	dpa.mu.Lock()
	defer dpa.mu.Unlock()

	stats, exists := dpa.kafkaStats[key]
	if !exists {
		stats = &KafkaStats{
			Topic:      topic,
			Operation:  operation,
			Partition:  partition,
			MaxHistory: 100,
		}
		dpa.kafkaStats[key] = stats
	}

	stats.MsgCount++
	stats.Latencies = append(stats.Latencies, latencyMs)
	if len(stats.Latencies) > stats.MaxHistory {
		stats.Latencies = stats.Latencies[len(stats.Latencies)-stats.MaxHistory:]
	}
	stats.AvgLatencyMs = avgFloat64(stats.Latencies)
	stats.P99LatencyMs = p99Float64(stats.Latencies)
	if latencyMs > stats.MaxLatencyMs {
		stats.MaxLatencyMs = latencyMs
	}
	if latencyMs > 50 {
		stats.SlowCount++
	}
	if hasError {
		stats.ErrorCount++
	}
	if msgSize > 0 {
		stats.AvgMsgSize = (stats.AvgMsgSize*uint64(stats.MsgSizeSamples) + msgSize) / uint64(stats.MsgSizeSamples+1)
		stats.MsgSizeSamples++
	}
	if lagEstimate > stats.LagEstimate {
		stats.LagEstimate = lagEstimate
	}

	return stats
}

// GetKafkaHighLagTopics 获取高延迟积压 Topic
func (dpa *DeepProtocolAnalyzer) GetKafkaHighLagTopics(limit int) []*KafkaStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*KafkaStats
	for _, stats := range dpa.kafkaStats {
		if stats.LagEstimate > 1000 {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].LagEstimate > all[j].LagEstimate
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ============================================================================
// HTTP 深度解析
// ============================================================================

// HTTPStats HTTP 深度统计
type HTTPStats struct {
	Host         string
	Path         string
	Method       string

	ReqCount     uint64
	ErrorCount   uint64
	SlowCount    uint64

	AvgLatencyMs float64
	MaxLatencyMs float64
	P99LatencyMs float64
	Latencies    []float64
	MaxHistory   int

	StatusDist   map[int]uint64 // 状态码分布
	AvgBodySize  uint64
}

// AnalyzeHTTP 深度分析 HTTP 请求
func (dpa *DeepProtocolAnalyzer) AnalyzeHTTP(
	host, path, method string,
	latencyMs float64,
	statusCode int,
	bodySize uint64,
) *HTTPStats {
	key := fmt.Sprintf("%s/%s/%s", host, path, method)

	dpa.mu.Lock()
	defer dpa.mu.Unlock()

	stats, exists := dpa.httpStats[key]
	if !exists {
		stats = &HTTPStats{
			Host:       host,
			Path:       path,
			Method:     method,
			MaxHistory: 100,
			StatusDist: make(map[int]uint64),
		}
		dpa.httpStats[key] = stats
	}

	stats.ReqCount++
	stats.Latencies = append(stats.Latencies, latencyMs)
	if len(stats.Latencies) > stats.MaxHistory {
		stats.Latencies = stats.Latencies[len(stats.Latencies)-stats.MaxHistory:]
	}
	stats.AvgLatencyMs = avgFloat64(stats.Latencies)
	stats.P99LatencyMs = p99Float64(stats.Latencies)
	if latencyMs > stats.MaxLatencyMs {
		stats.MaxLatencyMs = latencyMs
	}
	if latencyMs > 500 {
		stats.SlowCount++
	}
	if statusCode >= 400 {
		stats.ErrorCount++
	}
	stats.StatusDist[statusCode]++
	if bodySize > 0 {
		stats.AvgBodySize = (stats.AvgBodySize*(stats.ReqCount-1) + bodySize) / stats.ReqCount
	}

	return stats
}

// GetHTTPErrorEndpoints 获取错误率高的 HTTP 端点
func (dpa *DeepProtocolAnalyzer) GetHTTPErrorEndpoints(limit int) []*HTTPStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*HTTPStats
	for _, stats := range dpa.httpStats {
		if stats.ReqCount > 0 && float64(stats.ErrorCount)/float64(stats.ReqCount) > 0.05 {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		ri := float64(all[i].ErrorCount) / float64(all[i].ReqCount)
		rj := float64(all[j].ErrorCount) / float64(all[j].ReqCount)
		return ri > rj
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// GetHTTPSlowEndpoints 获取慢 HTTP 端点
func (dpa *DeepProtocolAnalyzer) GetHTTPSlowEndpoints(limit int) []*HTTPStats {
	dpa.mu.RLock()
	defer dpa.mu.RUnlock()

	var all []*HTTPStats
	for _, stats := range dpa.httpStats {
		if stats.SlowCount > 0 {
			all = append(all, stats)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].AvgLatencyMs > all[j].AvgLatencyMs
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// ============================================================================
// 辅助函数
// ============================================================================

func avgFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func p99Float64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := int(math.Ceil(0.99 * float64(len(sorted)-1)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// ============================================================================
// 二、流量异常检测
// ============================================================================

// TrafficAnomalyDetector 流量异常检测器
type TrafficAnomalyDetector struct {
	config TrafficAnomalyConfig
	mu     sync.RWMutex

	// 历史窗口
	history  map[string]*TrafficHistory
	baseline map[string]*TrafficBaseline
}

// TrafficAnomalyConfig 流量异常检测配置
type TrafficAnomalyConfig struct {
	// 突增检测: 当前流量超过基线倍数
	SpikeFactor float64
	// 突降检测: 当前流量低于基线比例
	DropThreshold float64
	// 异常端口检测阈值
	AbnormalPortThreshold int
	// 可疑连接: 连接数/流量比阈值
	SuspiciousConnRatio float64
	// 历史窗口大小
	HistoryWindowSize int
}

// DefaultTrafficAnomalyConfig 返回默认配置
func DefaultTrafficAnomalyConfig() TrafficAnomalyConfig {
	return TrafficAnomalyConfig{
		SpikeFactor:           5.0,
		DropThreshold:         0.2,
		AbnormalPortThreshold: 1000, // 端口连接数 > 1000
		SuspiciousConnRatio:   0.01, // 1% 连接异常
		HistoryWindowSize:     60,
	}
}

// NewTrafficAnomalyDetector 创建流量异常检测器
func NewTrafficAnomalyDetector(config TrafficAnomalyConfig) *TrafficAnomalyDetector {
	return &TrafficAnomalyDetector{
		config:   config,
		history:  make(map[string]*TrafficHistory),
		baseline: make(map[string]*TrafficBaseline),
	}
}

// TrafficHistory 流量历史
type TrafficHistory struct {
	Key     string
	Bytes   []uint64
	Packets []uint64
	Conns   []uint64
	MaxSize int
}

// Add 添加流量记录
func (th *TrafficHistory) Add(bytes, packets, conns uint64) {
	th.Bytes = append(th.Bytes, bytes)
	th.Packets = append(th.Packets, packets)
	th.Conns = append(th.Conns, conns)
	if th.MaxSize > 0 {
		if len(th.Bytes) > th.MaxSize {
			th.Bytes = th.Bytes[len(th.Bytes)-th.MaxSize:]
			th.Packets = th.Packets[len(th.Packets)-th.MaxSize:]
			th.Conns = th.Conns[len(th.Conns)-th.MaxSize:]
		}
	}
}

// AvgBytes 平均字节数
func (th *TrafficHistory) AvgBytes() uint64 {
	if len(th.Bytes) == 0 {
		return 0
	}
	var sum uint64
	for _, v := range th.Bytes {
		sum += v
	}
	return sum / uint64(len(th.Bytes))
}

// AvgConns 平均连接数
func (th *TrafficHistory) AvgConns() uint64 {
	if len(th.Conns) == 0 {
		return 0
	}
	var sum uint64
	for _, v := range th.Conns {
		sum += v
	}
	return sum / uint64(len(th.Conns))
}

// TrafficBaseline 流量基线
type TrafficBaseline struct {
	Key         string
	AvgBytes    uint64
	AvgPackets  uint64
	AvgConns    uint64
	StdDevBytes float64
	UpdatedAt   time.Time
}

// TrafficAnomaly 流量异常
type TrafficAnomaly struct {
	Type        AnomalyType
	Severity    AnomalySeverity
	Key         string
	Message     string
	CurrentVal  float64
	BaselineVal float64
	Ratio       float64
	DetectedAt  time.Time
}

// AnomalyType 异常类型
type AnomalyType string

const (
	AnomalySpike          AnomalyType = "traffic_spike"     // 流量突增
	AnomalyDrop           AnomalyType = "traffic_drop"      // 流量突降
	AnomalyAbnormalPort   AnomalyType = "abnormal_port"     // 异常端口
	AnomalySuspiciousConn AnomalyType = "suspicious_conn"   // 可疑连接
	AnomalyPortScan       AnomalyType = "port_scan"         // 端口扫描
	AnomalyDDoS           AnomalyType = "ddos"              // DDoS
)

// AnomalySeverity 异常严重级别
type AnomalySeverity string

const (
	AnomalySeverityInfo     AnomalySeverity = "info"
	AnomalySeverityWarning  AnomalySeverity = "warning"
	AnomalySeverityCritical AnomalySeverity = "critical"
)

// Detect 检测流量异常
func (tad *TrafficAnomalyDetector) Detect(key string, bytes, packets, conns uint64) []*TrafficAnomaly {
	var anomalies []*TrafficAnomaly

	tad.mu.Lock()
	defer tad.mu.Unlock()

	history, exists := tad.history[key]
	if !exists {
		history = &TrafficHistory{Key: key, MaxSize: tad.config.HistoryWindowSize}
		tad.history[key] = history
	}
	history.Add(bytes, packets, conns)

	// 需要足够历史数据才检测
	if len(history.Bytes) < 5 {
		return anomalies
	}

	avgBytes := history.AvgBytes()
	avgConns := history.AvgConns()

	// 1. 突增检测
	if avgBytes > 0 {
		ratio := float64(bytes) / float64(avgBytes)
		if ratio > tad.config.SpikeFactor {
			anomalies = append(anomalies, &TrafficAnomaly{
				Type:        AnomalySpike,
				Severity:    AnomalySeverityCritical,
				Key:         key,
				Message:     fmt.Sprintf("流量突增: 当前 %.0f 倍于基线", ratio),
				CurrentVal:  float64(bytes),
				BaselineVal: float64(avgBytes),
				Ratio:       ratio,
				DetectedAt:  time.Now(),
			})
		}
	}

	// 2. 突降检测
	if avgBytes > 0 {
		dropRate := 1.0 - float64(bytes)/float64(avgBytes)
		if dropRate > tad.config.DropThreshold {
			anomalies = append(anomalies, &TrafficAnomaly{
				Type:        AnomalyDrop,
				Severity:    AnomalySeverityWarning,
				Key:         key,
				Message:     fmt.Sprintf("流量突降: 下降 %.0f%%", dropRate*100),
				CurrentVal:  float64(bytes),
				BaselineVal: float64(avgBytes),
				Ratio:       dropRate,
				DetectedAt:  time.Now(),
			})
		}
	}

	// 3. 异常连接检测
	if avgConns > 0 {
		connRatio := float64(conns) / float64(avgConns)
		if connRatio > 10 {
			anomalies = append(anomalies, &TrafficAnomaly{
				Type:        AnomalySuspiciousConn,
				Severity:    AnomalySeverityWarning,
				Key:         key,
				Message:     fmt.Sprintf("可疑连接: 连接数 %.0f 倍于基线", connRatio),
				CurrentVal:  float64(conns),
				BaselineVal: float64(avgConns),
				Ratio:       connRatio,
				DetectedAt:  time.Now(),
			})
		}
	}

	return anomalies
}

// DetectPortScan 检测端口扫描
func (tad *TrafficAnomalyDetector) DetectPortScan(srcIP string, dstPorts []uint16) *TrafficAnomaly {
	if len(dstPorts) < 10 {
		return nil
	}
	return &TrafficAnomaly{
		Type:       AnomalyPortScan,
		Severity:   AnomalySeverityCritical,
		Key:        srcIP,
		Message:    fmt.Sprintf("端口扫描: 扫描 %d 个端口", len(dstPorts)),
		CurrentVal: float64(len(dstPorts)),
		DetectedAt: time.Now(),
	}
}

// DetectDDoS 检测 DDoS
func (tad *TrafficAnomalyDetector) DetectDDoS(dstIP string, srcIPs []string, connCount uint64) *TrafficAnomaly {
	if len(srcIPs) < 100 && connCount < 10000 {
		return nil
	}
	severity := AnomalySeverityWarning
	if len(srcIPs) > 1000 || connCount > 100000 {
		severity = AnomalySeverityCritical
	}
	return &TrafficAnomaly{
		Type:       AnomalyDDoS,
		Severity:   severity,
		Key:        dstIP,
		Message:    fmt.Sprintf("DDoS: %d 个源IP, %d 连接", len(srcIPs), connCount),
		CurrentVal: float64(connCount),
		DetectedAt: time.Now(),
	}
}

// UpdateBaseline 更新流量基线
func (tad *TrafficAnomalyDetector) UpdateBaseline(key string) {
	tad.mu.Lock()
	defer tad.mu.Unlock()

	history, ok := tad.history[key]
	if !ok || len(history.Bytes) < 5 {
		return
	}

	avgBytes := history.AvgBytes()
	avgPackets := uint64(0)
	if len(history.Packets) > 0 {
		var sum uint64
		for _, v := range history.Packets {
			sum += v
		}
		avgPackets = sum / uint64(len(history.Packets))
	}
	avgConns := history.AvgConns()

	var variance float64
	avgF := float64(avgBytes)
	for _, v := range history.Bytes {
		diff := float64(v) - avgF
		variance += diff * diff
	}
	variance /= float64(len(history.Bytes))
	stdDev := math.Sqrt(variance)

	tad.baseline[key] = &TrafficBaseline{
		Key:         key,
		AvgBytes:    avgBytes,
		AvgPackets:  avgPackets,
		AvgConns:    avgConns,
		StdDevBytes: stdDev,
		UpdatedAt:   time.Now(),
	}
}

// GetBaseline 获取基线
func (tad *TrafficAnomalyDetector) GetBaseline(key string) *TrafficBaseline {
	tad.mu.RLock()
	defer tad.mu.RUnlock()
	return tad.baseline[key]
}

// ============================================================================
// 三、流量基线智能分析
// ============================================================================

// SmartBaselineAnalyzer 智能基线分析器
type SmartBaselineAnalyzer struct {
	mu sync.RWMutex

	// 按小时/星期几的周期性基线
	hourlyBaseline  map[string]map[int]*TrafficBaseline // key -> hour -> baseline
	weeklyBaseline  map[string]map[int]*TrafficBaseline // key -> weekday -> baseline

	learningRate float64
}

// NewSmartBaselineAnalyzer 创建智能基线分析器
func NewSmartBaselineAnalyzer() *SmartBaselineAnalyzer {
	return &SmartBaselineAnalyzer{
		hourlyBaseline: make(map[string]map[int]*TrafficBaseline),
		weeklyBaseline: make(map[string]map[int]*TrafficBaseline),
		learningRate:   0.1,
	}
}

// Learn 学习周期性基线
func (sba *SmartBaselineAnalyzer) Learn(key string, timestamp time.Time, bytes, conns uint64) {
	hour := timestamp.Hour()
	weekday := int(timestamp.Weekday())

	sba.mu.Lock()
	defer sba.mu.Unlock()

	// 更新小时基线
	if sba.hourlyBaseline[key] == nil {
		sba.hourlyBaseline[key] = make(map[int]*TrafficBaseline)
	}
	if sba.hourlyBaseline[key][hour] == nil {
		sba.hourlyBaseline[key][hour] = &TrafficBaseline{Key: key}
	}
	sba.updateBaseline(sba.hourlyBaseline[key][hour], bytes, conns)

	// 更新星期基线
	if sba.weeklyBaseline[key] == nil {
		sba.weeklyBaseline[key] = make(map[int]*TrafficBaseline)
	}
	if sba.weeklyBaseline[key][weekday] == nil {
		sba.weeklyBaseline[key][weekday] = &TrafficBaseline{Key: key}
	}
	sba.updateBaseline(sba.weeklyBaseline[key][weekday], bytes, conns)
}

func (sba *SmartBaselineAnalyzer) updateBaseline(baseline *TrafficBaseline, bytes, conns uint64) {
	if baseline.AvgBytes == 0 {
		baseline.AvgBytes = bytes
		baseline.AvgConns = conns
	} else {
		baseline.AvgBytes = uint64(float64(baseline.AvgBytes)*(1-sba.learningRate) + float64(bytes)*sba.learningRate)
		baseline.AvgConns = uint64(float64(baseline.AvgConns)*(1-sba.learningRate) + float64(conns)*sba.learningRate)
	}
	baseline.UpdatedAt = time.Now()
}

// GetExpectedTraffic 获取预期流量（基于当前时间）
func (sba *SmartBaselineAnalyzer) GetExpectedTraffic(key string, now time.Time) (bytes, conns uint64) {
	sba.mu.RLock()
	defer sba.mu.RUnlock()

	hour := now.Hour()
	weekday := int(now.Weekday())

	var hourBytes, hourConns uint64
	if hb, ok := sba.hourlyBaseline[key][hour]; ok {
		hourBytes = hb.AvgBytes
		hourConns = hb.AvgConns
	}

	var weekBytes, weekConns uint64
	if wb, ok := sba.weeklyBaseline[key][weekday]; ok {
		weekBytes = wb.AvgBytes
		weekConns = wb.AvgConns
	}

	// 加权组合
	return (hourBytes*3 + weekBytes) / 4, (hourConns*3 + weekConns) / 4
}

// CompareWithBaseline 与基线比较，返回偏差率
func (sba *SmartBaselineAnalyzer) CompareWithBaseline(key string, now time.Time, actualBytes, actualConns uint64) (bytesRatio, connsRatio float64) {
	expectedBytes, expectedConns := sba.GetExpectedTraffic(key, now)
	if expectedBytes > 0 {
		bytesRatio = float64(actualBytes) / float64(expectedBytes)
	}
	if expectedConns > 0 {
		connsRatio = float64(actualConns) / float64(expectedConns)
	}
	return
}

// ============================================================================
// 四、流量回放与模拟
// ============================================================================

// TrafficReplayer 流量回放器
type TrafficReplayer struct {
	mu sync.RWMutex

	// 录制的流量记录
	records []*TrafficRecord

	// 回放状态
	isPlaying bool
	position  int
	speed     float64 // 倍速
}

// TrafficRecord 流量记录
type TrafficRecord struct {
	Timestamp  time.Time
	SrcIP      string
	DstIP      string
	SrcPort    uint16
	DstPort    uint16
	Protocol   string
	Bytes      uint64
	Packets    uint64

	// L7 信息
	L7Protocol string
	Method     string
	Path       string
	Host       string
	StatusCode int
	LatencyMs  float64

	// 标签
	Tags       map[string]string
}

// NewTrafficReplayer 创建流量回放器
func NewTrafficReplayer() *TrafficReplayer {
	return &TrafficReplayer{
		records: []*TrafficRecord{},
		speed:   1.0,
	}
}

// Record 录制流量
func (tr *TrafficReplayer) Record(record *TrafficRecord) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.records = append(tr.records, record)
}

// LoadRecords 加载记录
func (tr *TrafficReplayer) LoadRecords(records []*TrafficRecord) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.records = records
	tr.position = 0
}

// Play 开始回放
func (tr *TrafficReplayer) Play(callback func(*TrafficRecord)) {
	tr.mu.Lock()
	tr.isPlaying = true
	tr.position = 0
	tr.mu.Unlock()

	go tr.playLoop(callback)
}

// Stop 停止回放
func (tr *TrafficReplayer) Stop() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.isPlaying = false
}

// SetSpeed 设置回放倍速
func (tr *TrafficReplayer) SetSpeed(speed float64) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.speed = speed
}

func (tr *TrafficReplayer) playLoop(callback func(*TrafficRecord)) {
	for {
		tr.mu.RLock()
		if !tr.isPlaying || tr.position >= len(tr.records) {
			tr.mu.RUnlock()
			break
		}
		record := tr.records[tr.position]
		speed := tr.speed
		tr.mu.RUnlock()

		callback(record)

		tr.mu.Lock()
		tr.position++
		// 计算下一条记录的间隔
		var waitTime time.Duration
		if tr.position < len(tr.records) {
			next := tr.records[tr.position]
			waitTime = next.Timestamp.Sub(record.Timestamp)
			if speed != 1.0 && speed > 0 {
				waitTime = time.Duration(float64(waitTime) / speed)
			}
		}
		tr.mu.Unlock()

		if waitTime > 0 {
			time.Sleep(waitTime)
		}
	}

	tr.mu.Lock()
	tr.isPlaying = false
	tr.mu.Unlock()
}

// GetStats 回放统计
func (tr *TrafficReplayer) GetStats() ReplayerStats {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	return ReplayerStats{
		TotalRecords: len(tr.records),
		CurrentPos:   tr.position,
		IsPlaying:    tr.isPlaying,
		Speed:        tr.speed,
	}
}

// ReplayerStats 回放统计
type ReplayerStats struct {
	TotalRecords int
	CurrentPos   int
	IsPlaying    bool
	Speed        float64
}

// ============================================================================
// 五、流量模拟器
// ============================================================================

// TrafficSimulator 流量模拟器
type TrafficSimulator struct {
	config SimulatorConfig
}

// SimulatorConfig 模拟器配置
type SimulatorConfig struct {
	// 基础流量率 (bytes/sec)
	BaseRate float64
	// 峰值倍数
	PeakFactor float64
	// 波动率
	Volatility float64
	// 异常注入概率
	AnomalyProb float64
}

// DefaultSimulatorConfig 返回默认配置
func DefaultSimulatorConfig() SimulatorConfig {
	return SimulatorConfig{
		BaseRate:    10000,
		PeakFactor:  3.0,
		Volatility:  0.2,
		AnomalyProb: 0.01,
	}
}

// NewTrafficSimulator 创建流量模拟器
func NewTrafficSimulator(config SimulatorConfig) *TrafficSimulator {
	return &TrafficSimulator{config: config}
}

// GenerateTraffic 生成模拟流量
func (ts *TrafficSimulator) GenerateTraffic(duration time.Duration, interval time.Duration) []*TrafficRecord {
	var records []*TrafficRecord
	start := time.Now()

	for elapsed := time.Duration(0); elapsed < duration; elapsed += interval {
		record := ts.generateRecord(start.Add(elapsed))
		records = append(records, record)
	}

	return records
}

func (ts *TrafficSimulator) generateRecord(timestamp time.Time) *TrafficRecord {
	// 基础流量 + 正弦波动 + 随机噪声
	cycle := math.Sin(float64(timestamp.Unix()) * 0.001) // 约 1000s 周期
	base := ts.config.BaseRate * (1 + cycle*ts.config.Volatility)

	// 随机噪声
	noise := (mathrand() - 0.5) * 2 * ts.config.Volatility
	base *= (1 + noise)

	// 异常注入
	isAnomaly := mathrand() < ts.config.AnomalyProb
	if isAnomaly {
		base *= ts.config.PeakFactor
	}

	bytes := uint64(base)
	if bytes == 0 {
		bytes = 1
	}

	return &TrafficRecord{
		Timestamp:  timestamp,
		SrcIP:      "10.0.0.1",
		DstIP:      "10.0.0.2",
		SrcPort:    12345,
		DstPort:    80,
		Protocol:   "TCP",
		Bytes:      bytes,
		Packets:    bytes/1000 + 1,
		L7Protocol: "HTTP",
		Method:     "GET",
		Path:       "/api/test",
		Host:       "example.com",
		Tags: map[string]string{
			"simulated": "true",
			"anomaly":   fmt.Sprintf("%v", isAnomaly),
		},
	}
}

// mathrand 返回 0-1 的伪随机数 (简单实现，避免导入 math/rand)
func mathrand() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// ExportRecords 导出记录为 JSON 格式字符串
func (ts *TrafficSimulator) ExportRecords(records []*TrafficRecord) string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, r := range records {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"ts":"%s","bytes":%d}`, r.Timestamp.Format(time.RFC3339), r.Bytes)
	}
	sb.WriteString("]")
	return sb.String()
}

// ============================================================================
// 六、综合流量报告
// ============================================================================

// TrafficReport 综合流量报告
type TrafficReport struct {
	GeneratedAt       time.Time

	// MySQL
	MySQLHotTables    []*MySQLStats
	MySQLSlowQueries  []*MySQLStats

	// Redis
	RedisHotKeys      []*RedisStats
	RedisSlowCommands []*RedisStats

	// Kafka
	KafkaHighLagTopics []*KafkaStats

	// HTTP
	HTTPErrorEndpoints []*HTTPStats
	HTTPSlowEndpoints  []*HTTPStats

	// 异常
	Anomalies         []*TrafficAnomaly

	// 基线
	Baselines         map[string]*TrafficBaseline

	// 模拟
	SimulatedRecords  int
}

// GenerateTrafficReport 生成综合流量报告
func GenerateTrafficReport(
	dpa *DeepProtocolAnalyzer,
	tad *TrafficAnomalyDetector,
	limit int,
) *TrafficReport {
	return &TrafficReport{
		GeneratedAt:        time.Now(),
		MySQLHotTables:      dpa.GetMySQLHotTables(limit),
		MySQLSlowQueries:    dpa.GetMySQLSlowQueries(limit),
		RedisHotKeys:        dpa.GetRedisHotKeys(limit),
		RedisSlowCommands:   dpa.GetRedisSlowCommands(limit),
		KafkaHighLagTopics:  dpa.GetKafkaHighLagTopics(limit),
		HTTPErrorEndpoints:  dpa.GetHTTPErrorEndpoints(limit),
		HTTPSlowEndpoints:   dpa.GetHTTPSlowEndpoints(limit),
	}
}
