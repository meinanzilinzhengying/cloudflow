// Package dbobserver 提供数据库观测功能
// 本文件实现 Agent 集成服务
package dbobserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ==================== 数据库观测服务 ====================

// DBObserverService 数据库观测服务
type DBObserverService struct {
	observer   *DBObserver
	capture    *PacketCapture
	correlator *MetricsCorrelator
	discoverer *DBProcessDiscoverer

	// 配置
	config *ObserverConfig

	// 状态
	running bool
	mu      sync.RWMutex
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// 回调
	onSQLStats      func([]*SQLStats)
	onSlowQuery     func(*SlowQuery)
	onSlowQueryRank func([]*SlowQueryRank)
	onAlert         func(*SlowQueryAlert)
}

// NewDBObserverService 创建数据库观测服务
func NewDBObserverService(cfg *ObserverConfig) *DBObserverService {
	if cfg == nil {
		cfg = DefaultObserverConfig()
	}

	observer := NewDBObserver(cfg)
	capture := NewPacketCapture()
	correlator := NewMetricsCorrelator()
	discoverer := NewDBProcessDiscoverer()

	service := &DBObserverService{
		observer:   observer,
		capture:    capture,
		correlator: correlator,
		discoverer: discoverer,
		config:     cfg,
		stopCh:     make(chan struct{}),
	}

	// 设置事件处理器
	capture.SetEventHandler(service.handleSQLEvent)

	// 设置慢查询检测器的依赖
	observer.detector.SetStatsGetter(func(fingerprint string) *SQLStats {
		return observer.aggregator.GetStats()[fingerprint]
	})
	observer.detector.SetMetricsGetter(func(pid uint32) *ProcessMetrics {
		return observer.collector.GetMetrics(pid)
	})

	return service
}

// Start 启动服务
func (s *DBObserverService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("服务已在运行")
	}
	s.running = true
	s.mu.Unlock()

	// 发现数据库进程
	s.discoverProcesses()

	// 启动观测器
	if err := s.observer.Start(ctx); err != nil {
		return err
	}

	// 启动进程发现循环
	s.wg.Add(1)
	go s.discoveryLoop(ctx)

	// 启动指标上报循环
	s.wg.Add(1)
	go s.reportLoop(ctx)

	return nil
}

// Stop 停止服务
func (s *DBObserverService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.observer.Stop()
	s.correlator.Close()
	s.wg.Wait()
}

// discoverProcesses 发现数据库进程
func (s *DBObserverService) discoverProcesses() {
	processes := s.discoverer.Discover()
	for _, proc := range processes {
		s.observer.collector.RegisterDBProcess(proc)
	}
}

// discoveryLoop 进程发现循环
func (s *DBObserverService) discoveryLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.discoverProcesses()
		}
	}
}

// reportLoop 指标上报循环
func (s *DBObserverService) reportLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.AggregationWindow)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.reportStats()
		}
	}
}

// reportStats 上报统计
func (s *DBObserverService) reportStats() {
	// 获取 SQL 统计
	stats := s.observer.GetStats()
	if s.onSQLStats != nil && len(stats) > 0 {
		statsList := make([]*SQLStats, 0, len(stats))
		for _, s := range stats {
			statsList = append(statsList, s)
		}
		s.onSQLStats(statsList)
	}

	// 获取慢查询排行
	ranking := s.observer.GetSlowQueryRanking(10)
	if s.onSlowQueryRank != nil && len(ranking) > 0 {
		s.onSlowQueryRank(ranking)
	}
}

// handleSQLEvent 处理 SQL 事件
func (s *DBObserverService) handleSQLEvent(event *SQLEvent) {
	// 关联进程指标
	correlated := s.correlator.Correlate(event)
	if correlated.ProcessMetrics != nil {
		event.PID = correlated.ProcessMetrics.PID
		event.ProcessName = correlated.ProcessMetrics.ProcessName
	}

	// 记录事件
	s.observer.RecordEvent(event)

	// 检查是否是慢查询
	if event.Duration > s.config.SlowQueryThreshold {
		slowQueries := s.observer.GetSlowQueries(1)
		if len(slowQueries) > 0 && s.onSlowQuery != nil {
			s.onSlowQuery(slowQueries[0])
		}
	}
}

// ==================== 回调设置 ====================

// SetSQLStatsCallback 设置 SQL 统计回调
func (s *DBObserverService) SetSQLStatsCallback(callback func([]*SQLStats)) {
	s.onSQLStats = callback
}

// SetSlowQueryCallback 设置慢查询回调
func (s *DBObserverService) SetSlowQueryCallback(callback func(*SlowQuery)) {
	s.onSlowQuery = callback
}

// SetSlowQueryRankCallback 设置慢查询排行回调
func (s *DBObserverService) SetSlowQueryRankCallback(callback func([]*SlowQueryRank)) {
	s.onSlowQueryRank = callback
}

// SetAlertCallback 设置告警回调
func (s *DBObserverService) SetAlertCallback(callback func(*SlowQueryAlert)) {
	s.onAlert = callback
}

// ==================== 查询接口 ====================

// GetSQLStats 获取 SQL 统计
func (s *DBObserverService) GetSQLStats() map[string]*SQLStats {
	return s.observer.GetStats()
}

// GetTopSQLByRequestCount 按请求次数获取 Top SQL
func (s *DBObserverService) GetTopSQLByRequestCount(n int) []*SQLStats {
	return s.observer.aggregator.GetTopByRequestCount(n)
}

// GetTopSQLByAvgDuration 按平均时延获取 Top SQL
func (s *DBObserverService) GetTopSQLByAvgDuration(n int) []*SQLStats {
	return s.observer.aggregator.GetTopByAvgDuration(n)
}

// GetTopSQLByErrorCount 按错误数获取 Top SQL
func (s *DBObserverService) GetTopSQLByErrorCount(n int) []*SQLStats {
	return s.observer.aggregator.GetTopByErrorCount(n)
}

// GetSlowQueries 获取慢查询列表
func (s *DBObserverService) GetSlowQueries(limit int) []*SlowQuery {
	return s.observer.GetSlowQueries(limit)
}

// GetSlowQueryRanking 获取慢查询排行
func (s *DBObserverService) GetSlowQueryRanking(limit int) []*SlowQueryRank {
	return s.observer.GetSlowQueryRanking(limit)
}

// GetProcessMetrics 获取进程指标
func (s *DBObserverService) GetProcessMetrics(pid uint32) *ProcessMetrics {
	return s.observer.GetProcessMetrics(pid)
}

// GetAllProcessMetrics 获取所有进程指标
func (s *DBObserverService) GetAllProcessMetrics() map[uint32]*ProcessMetrics {
	return s.observer.GetAllProcessMetrics()
}

// GetAggregatedMetrics 获取聚合指标
func (s *DBObserverService) GetAggregatedMetrics(dbType DatabaseType) *AggregatedMetrics {
	return s.correlator.aggregator.GetAggregated(dbType)
}

// ==================== 手动操作接口 ====================

// RecordSQLEvent 手动记录 SQL 事件
func (s *DBObserverService) RecordSQLEvent(event *SQLEvent) {
	s.handleSQLEvent(event)
}

// ProcessPacket 处理网络数据包
func (s *DBObserverService) ProcessPacket(data []byte, direction PacketDirection, port int) error {
	return s.capture.ProcessPacket(data, direction, port)
}

// ==================== 导出接口 ====================

// ExportSQLStats 导出 SQL 统计
func (s *DBObserverService) ExportSQLStats(format string) ([]byte, error) {
	stats := s.observer.GetStats()

	switch format {
	case "json":
		return json.Marshal(stats)
	case "prometheus":
		return s.exportPrometheus(stats), nil
	default:
		return json.Marshal(stats)
	}
}

// exportPrometheus 导出 Prometheus 格式
func (s *DBObserverService) exportPrometheus(stats map[string]*SQLStats) []byte {
	var builder strings.Builder

	for _, s := range stats {
		// 请求次数
		builder.WriteString(fmt.Sprintf("db_sql_request_count{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %d\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.RequestCount))

		// 成功率
		builder.WriteString(fmt.Sprintf("db_sql_success_rate{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %.4f\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.SuccessRate))

		// 平均时延
		builder.WriteString(fmt.Sprintf("db_sql_avg_duration_us{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %.2f\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.AvgDuration))

		// 最大时延
		builder.WriteString(fmt.Sprintf("db_sql_max_duration_us{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %d\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.MaxDuration))

		// P99 时延
		builder.WriteString(fmt.Sprintf("db_sql_p99_duration_us{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %d\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.P99Duration))

		// 错误数
		builder.WriteString(fmt.Sprintf("db_sql_error_count{fingerprint=\"%s\",type=\"%s\",db=\"%s\"} %d\n",
			s.SQLFingerprint, s.SQLType, s.DatabaseName, s.ErrorCount))
	}

	return []byte(builder.String())
}

// ExportSlowQueries 导出慢查询
func (s *DBObserverService) ExportSlowQueries(format string) ([]byte, error) {
	queries := s.observer.GetSlowQueries(100)

	switch format {
	case "json":
		return json.Marshal(queries)
	default:
		return json.Marshal(queries)
	}
}

// ExportProcessMetrics 导出进程指标
func (s *DBObserverService) ExportProcessMetrics(format string) ([]byte, error) {
	metrics := s.observer.GetAllProcessMetrics()

	switch format {
	case "json":
		return json.Marshal(metrics)
	default:
		return json.Marshal(metrics)
	}
}

// ==================== 状态接口 ====================

// GetStatus 获取服务状态
func (s *DBObserverService) GetStatus() *ServiceStatus {
	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()

	stats := s.observer.GetStats()
	slowQueries := s.observer.GetSlowQueries(1000)
	processMetrics := s.observer.GetAllProcessMetrics()

	return &ServiceStatus{
		Running:          running,
		TotalSQLCount:    len(stats),
		SlowQueryCount:   len(slowQueries),
		ProcessCount:     len(processMetrics),
		DatabaseTypes:    s.config.DatabaseTypes,
		CollectInterval:  s.config.CollectInterval,
		SlowThreshold:    s.config.SlowQueryThreshold,
	}
}

// ServiceStatus 服务状态
type ServiceStatus struct {
	Running         bool           `json:"running"`
	TotalSQLCount   int            `json:"total_sql_count"`
	SlowQueryCount  int            `json:"slow_query_count"`
	ProcessCount    int            `json:"process_count"`
	DatabaseTypes   []DatabaseType `json:"database_types"`
	CollectInterval time.Duration  `json:"collect_interval"`
	SlowThreshold   int64          `json:"slow_threshold"`
}

// ==================== 告警管理 ====================

// AlertManager 告警管理器
type AlertManager struct {
	alerts     []*SlowQueryAlert
	maxAlerts  int
	mu         sync.RWMutex
	handlers   []func(*SlowQueryAlert)
}

// NewAlertManager 创建告警管理器
func NewAlertManager(maxAlerts int) *AlertManager {
	return &AlertManager{
		alerts:    make([]*SlowQueryAlert, 0),
		maxAlerts: maxAlerts,
		handlers:  make([]func(*SlowQueryAlert), 0),
	}
}

// AddAlert 添加告警
func (m *AlertManager) AddAlert(alert *SlowQueryAlert) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.alerts = append(m.alerts, alert)

	// 检查是否超过最大数量
	if len(m.alerts) > m.maxAlerts {
		m.alerts = m.alerts[1:]
	}

	// 调用处理器
	for _, handler := range m.handlers {
		go handler(alert)
	}
}

// GetAlerts 获取告警列表
func (m *AlertManager) GetAlerts(limit int) []*SlowQueryAlert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit > len(m.alerts) {
		limit = len(m.alerts)
	}

	result := make([]*SlowQueryAlert, limit)
	copy(result, m.alerts[len(m.alerts)-limit:])

	return result
}

// AddHandler 添加告警处理器
func (m *AlertManager) AddHandler(handler func(*SlowQueryAlert)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// ==================== 配置管理 ====================

// ConfigManager 配置管理器
type ConfigManager struct {
	config    *ObserverConfig
	configPath string
	mu        sync.RWMutex
}

// NewConfigManager 创建配置管理器
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		config:     DefaultObserverConfig(),
		configPath: configPath,
	}
}

// Load 加载配置
func (m *ConfigManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 从文件加载配置
	// 简化实现，实际应从配置文件读取
	return nil
}

// Save 保存配置
func (m *ConfigManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 保存配置到文件
	// 简化实现
	return nil
}

// Get 获取配置
func (m *ConfigManager) Get() *ObserverConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Update 更新配置
func (m *ConfigManager) Update(config *ObserverConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// ==================== 工具函数 ====================

// FormatDuration 格式化时延
func FormatDuration(us int64) string {
	if us < 1000 {
		return fmt.Sprintf("%dμs", us)
	} else if us < 1000000 {
		return fmt.Sprintf("%.2fms", float64(us)/1000)
	} else {
		return fmt.Sprintf("%.2fs", float64(us)/1000000)
	}
}

// FormatBytes 格式化字节数
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	if bytes < KB {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < MB {
		return fmt.Sprintf("%.2fKB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.2fMB", float64(bytes)/MB)
	} else {
		return fmt.Sprintf("%.2fGB", float64(bytes)/GB)
	}
}

// CalculateSuccessRate 计算成功率
func CalculateSuccessRate(success, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(success) / float64(total) * 100
}

// CalculatePercentile 计算百分位
func CalculatePercentile(values []int64, percentile float64) int64 {
	if len(values) == 0 {
		return 0
	}

	// 排序
	sorted := make([]int64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// 计算百分位位置
	index := int(float64(len(sorted)-1) * percentile / 100.0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}
