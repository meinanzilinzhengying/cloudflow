// Package selfmonitor 提供Agent自监控功能
//
// 采集以下自监控指标：
//   - 心跳状态：最后一次心跳是否成功、心跳延迟
//   - CPU使用率：进程CPU使用率
//   - 内存使用率：进程内存使用率
//   - 采集丢包率：EBPF采集丢包 / 总应采集包数
//   - 上报成功率：成功发送次数 / 总发送次数
//
// 采集周期：10秒
// 上报目标：Center
package selfmonitor

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricType 自监控指标类型
type MetricType string

const (
	MetricHeartbeatStatus   MetricType = "heartbeat_status"   // 心跳状态 (1=正常, 0=异常)
	MetricHeartbeatLatency  MetricType = "heartbeat_latency_ms" // 心跳延迟(毫秒)
	MetricCPUPercent        MetricType = "cpu_percent"        // CPU使用率(%)
	MetricMemoryPercent     MetricType = "memory_percent"     // 内存使用率(%)
	MetricPacketDropRate    MetricType = "packet_drop_rate"   // 采集丢包率(%)
	MetricReportSuccessRate MetricType = "report_success_rate" // 上报成功率(%)
)

// MetricsSnapshot 自监控指标快照
type MetricsSnapshot struct {
	Timestamp       time.Time
	HeartbeatStatus int     // 1=正常, 0=异常
	HeartbeatLatencyMs float64 // 心跳延迟(毫秒)
	CPUPercent      float64 // CPU使用率(%)
	MemoryPercent   float64 // 内存使用率(%)
	PacketDropRate  float64 // 采集丢包率(%)
	ReportSuccessRate float64 // 上报成功率(%)
	
	// 原始计数（用于计算差值）
	CollectTotal    uint64 // 应采集总数
	CollectDropped  uint64 // 丢包数
	SendTotal       uint64 // 发送总数
	SendSuccess     uint64 // 发送成功数
}

// Config 自监控配置
type Config struct {
	Enabled           bool          // 启用自监控
	CollectInterval   time.Duration // 采集间隔（默认10秒）
	ReportInterval    time.Duration // 上报间隔（默认10秒）
	HeartbeatTimeout  time.Duration // 心跳超时判定（默认5秒）
	MaxMemoryMB       float64       // 内存限制，用于计算使用率
	
	// 告警阈值
	AlertThresholds AlertThresholds
}

// AlertThresholds 告警阈值配置
type AlertThresholds struct {
	HeartbeatFailCount    int     // 连续心跳失败次数触发告警
	CPUPercentThreshold   float64 // CPU使用率告警阈值(%)
	MemoryPercentThreshold float64 // 内存使用率告警阈值(%)
	PacketDropRateThreshold float64 // 丢包率告警阈值(%)
	ReportSuccessRateMin  float64 // 上报成功率最低阈值(%)
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		CollectInterval:  10 * time.Second,
		ReportInterval:   10 * time.Second,
		HeartbeatTimeout: 5 * time.Second,
		MaxMemoryMB:      1024,
		AlertThresholds: AlertThresholds{
			HeartbeatFailCount:      3,
			CPUPercentThreshold:     80.0,
			MemoryPercentThreshold:  90.0,
			PacketDropRateThreshold: 5.0,
			ReportSuccessRateMin:    95.0,
		},
	}
}

// Collector 自监控采集器
type Collector struct {
	cfg    Config
	log    *logger.Logger
	mu     sync.RWMutex
	
	// 状态
	lastSnapshot    MetricsSnapshot
	heartbeatStatus int       // 当前心跳状态
	heartbeatLatency time.Duration // 最后一次心跳延迟
	lastHeartbeatAt time.Time // 最后一次心跳时间
	
	// CPU使用率计算
	prevCPUIdle     uint64
	prevCPUTotal    uint64
	cpuInitialized  bool
	
	// 计数器引用（从metrics包获取）
	collectTotal    prometheus.Counter
	collectDropped  prometheus.Counter
	sendTotal       prometheus.Counter
	sendSuccess     prometheus.Counter
	
	// 上一次计数（用于计算差值）
	prevCollectTotal uint64
	prevCollectDropped uint64
	prevSendTotal    uint64
	prevSendSuccess  uint64
	
	// 生命周期
	stopCh chan struct{}
	
	// 告警回调
	onAlert func(alertType string, value float64, message string)
}

// NewCollector 创建自监控采集器
func NewCollector(cfg Config, log *logger.Logger) *Collector {
	if cfg.CollectInterval <= 0 {
		cfg.CollectInterval = 10 * time.Second
	}
	if cfg.ReportInterval <= 0 {
		cfg.ReportInterval = 10 * time.Second
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = 5 * time.Second
	}
	
	return &Collector{
		cfg:             cfg,
		log:             log,
		heartbeatStatus: 1, // 初始状态为正常
		stopCh:          make(chan struct{}),
	}
}

// SetCounters 设置Prometheus计数器引用
func (c *Collector) SetCounters(collectTotal, collectDropped, sendTotal, sendSuccess prometheus.Counter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.collectTotal = collectTotal
	c.collectDropped = collectDropped
	c.sendTotal = sendTotal
	c.sendSuccess = sendSuccess
}

// OnAlert 设置告警回调
func (c *Collector) OnAlert(cb func(alertType string, value float64, message string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAlert = cb
}

// Start 启动采集器
func (c *Collector) Start() {
	go c.collectLoop()
	c.log.Info("[自监控] 采集器已启动，采集间隔: %v", c.cfg.CollectInterval)
}

// Stop 停止采集器
func (c *Collector) Stop() {
	close(c.stopCh)
	c.log.Info("[自监控] 采集器已停止")
}

// RecordHeartbeat 记录心跳结果（由外部调用）
func (c *Collector) RecordHeartbeat(success bool, latency time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.lastHeartbeatAt = time.Now()
	c.heartbeatLatency = latency
	if success {
		c.heartbeatStatus = 1
	} else {
		c.heartbeatStatus = 0
	}
}

// GetSnapshot 获取最新指标快照
func (c *Collector) GetSnapshot() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSnapshot
}

// collectLoop 采集循环
func (c *Collector) collectLoop() {
	ticker := time.NewTicker(c.cfg.CollectInterval)
	defer ticker.Stop()
	
	// 立即执行一次采集
	c.collect()
	
	for {
		select {
		case <-ticker.C:
			c.collect()
		case <-c.stopCh:
			return
		}
	}
}

// collect 执行一次指标采集
func (c *Collector) collect() {
	snapshot := c.doCollect()
	
	c.mu.Lock()
	c.lastSnapshot = snapshot
	onAlert := c.onAlert
	c.mu.Unlock()
	
	// 检查告警
	if onAlert != nil {
		c.checkAlerts(snapshot, onAlert)
	}
}

// doCollect 采集所有指标
func (c *Collector) doCollect() MetricsSnapshot {
	now := time.Now()
	
	// 1. 心跳状态
	heartbeatStatus, heartbeatLatency := c.getHeartbeatStatus()
	
	// 2. CPU使用率
	cpuPercent := c.getCPUPercent()
	
	// 3. 内存使用率
	memPercent, memUsedMB := c.getMemoryPercent()
	
	// 4. 采集丢包率
	packetDropRate, collectTotal, collectDropped := c.getPacketDropRate()
	
	// 5. 上报成功率
	successRate, sendTotal, sendSuccess := c.getReportSuccessRate()
	
	return MetricsSnapshot{
		Timestamp:          now,
		HeartbeatStatus:    heartbeatStatus,
		HeartbeatLatencyMs: heartbeatLatency,
		CPUPercent:         cpuPercent,
		MemoryPercent:      memPercent,
		PacketDropRate:     packetDropRate,
		ReportSuccessRate:  successRate,
		CollectTotal:       collectTotal,
		CollectDropped:     collectDropped,
		SendTotal:          sendTotal,
		SendSuccess:        sendSuccess,
	}
}

// getHeartbeatStatus 获取心跳状态
func (c *Collector) getHeartbeatStatus() (status int, latencyMs float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// 检查心跳是否超时
	if time.Since(c.lastHeartbeatAt) > c.cfg.HeartbeatTimeout {
		return 0, float64(c.cfg.HeartbeatTimeout.Milliseconds())
	}
	
	return c.heartbeatStatus, float64(c.heartbeatLatency.Milliseconds())
}

// getCPUPercent 获取CPU使用率
func (c *Collector) getCPUPercent() float64 {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	
	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	n, _ := fmt.Sscanf(string(data),
		"cpu %d %d %d %d %d %d %d %d",
		&user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal)
	if n < 4 {
		return 0
	}
	
	totalIdle := idle + iowait
	total := user + nice + system + idle + iowait + irq + softirq + steal
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if !c.cpuInitialized || c.prevCPUTotal == 0 {
		c.prevCPUIdle = totalIdle
		c.prevCPUTotal = total
		c.cpuInitialized = true
		return 0
	}
	
	deltaIdle := totalIdle - c.prevCPUIdle
	deltaTotal := total - c.prevCPUTotal
	
	c.prevCPUIdle = totalIdle
	c.prevCPUTotal = total
	
	if deltaTotal == 0 {
		return 0
	}
	
	usage := (1.0 - float64(deltaIdle)/float64(deltaTotal)) * 100
	if usage > 100 {
		usage = 100
	}
	if usage < 0 {
		usage = 0
	}
	
	return usage
}

// getMemoryPercent 获取内存使用率
func (c *Collector) getMemoryPercent() (percent float64, usedMB float64) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	usedMB = float64(memStats.Alloc) / 1024 / 1024
	percent = 0
	if c.cfg.MaxMemoryMB > 0 {
		percent = (usedMB / c.cfg.MaxMemoryMB) * 100
		if percent > 100 {
			percent = 100
		}
	}
	
	return percent, usedMB
}

// getPacketDropRate 获取采集丢包率
func (c *Collector) getPacketDropRate() (rate float64, total, dropped uint64) {
	c.mu.RLock()
	collectTotal := c.collectTotal
	collectDropped := c.collectDropped
	prevTotal := c.prevCollectTotal
	prevDropped := c.prevCollectDropped
	c.mu.RUnlock()
	
	// FIX: prometheus.Metric 是 interface，不能直接用 &prometheus.Metric{} 创建
	// 改用 prometheus.NewDesc + prometheus.Metric 的正确方式获取 Counter 值
	// 如果 counter 为 nil，直接使用上一次的累计值
	var currentTotal, currentDropped float64
	if collectTotal != nil {
		currentTotal = getCounterValue(collectTotal)
	}
	if collectDropped != nil {
		currentDropped = getCounterValue(collectDropped)
	}
	
	total = uint64(currentTotal)
	dropped = uint64(currentDropped)
	
	// 计算差值
	deltaTotal := total - prevTotal
	deltaDropped := dropped - prevDropped
	
	// 更新上一次计数
	c.mu.Lock()
	c.prevCollectTotal = total
	c.prevCollectDropped = dropped
	c.mu.Unlock()
	
	if deltaTotal == 0 {
		return 0, total, dropped
	}
	
	rate = float64(deltaDropped) / float64(deltaTotal) * 100
	return rate, total, dropped
}

// getReportSuccessRate 获取上报成功率
func (c *Collector) getReportSuccessRate() (rate float64, total, success uint64) {
	c.mu.RLock()
	sendTotal := c.sendTotal
	sendSuccess := c.sendSuccess
	prevTotal := c.prevSendTotal
	prevSuccess := c.prevSendSuccess
	c.mu.RUnlock()
	
	// FIX: 使用安全的 counter 值获取方式
	var currentTotal, currentSuccess float64
	if sendTotal != nil {
		currentTotal = getCounterValue(sendTotal)
	}
	if sendSuccess != nil {
		currentSuccess = getCounterValue(sendSuccess)
	}
	
	total = uint64(currentTotal)
	success = uint64(currentSuccess)
	
	// 计算差值
	deltaTotal := total - prevTotal
	deltaSuccess := success - prevSuccess
	
	// 更新上一次计数
	c.mu.Lock()
	c.prevSendTotal = total
	c.prevSendSuccess = success
	c.mu.Unlock()
	
	if deltaTotal == 0 {
		// 没有新数据，返回历史成功率
		if total > 0 {
			return float64(success) / float64(total) * 100, total, success
		}
		return 100, total, success // 无数据时默认为100%
	}
	
	rate = float64(deltaSuccess) / float64(deltaTotal) * 100
	return rate, total, success
}

// checkAlerts 检查告警条件
func (c *Collector) checkAlerts(s MetricsSnapshot, onAlert func(string, float64, string)) {
	cfg := c.cfg.AlertThresholds
	
	// 1. 心跳告警
	if s.HeartbeatStatus == 0 {
		onAlert("heartbeat_failure", 0, "心跳失败或超时")
	}
	
	// 2. CPU告警
	if s.CPUPercent >= cfg.CPUPercentThreshold {
		onAlert("cpu_high", s.CPUPercent, 
			fmt.Sprintf("CPU使用率%.1f%%超过阈值%.1f%%", s.CPUPercent, cfg.CPUPercentThreshold))
	}
	
	// 3. 内存告警
	if s.MemoryPercent >= cfg.MemoryPercentThreshold {
		onAlert("memory_high", s.MemoryPercent,
			fmt.Sprintf("内存使用率%.1f%%超过阈值%.1f%%", s.MemoryPercent, cfg.MemoryPercentThreshold))
	}
	
	// 4. 丢包率告警
	if s.PacketDropRate >= cfg.PacketDropRateThreshold {
		onAlert("packet_drop_high", s.PacketDropRate,
			fmt.Sprintf("采集丢包率%.1f%%超过阈值%.1f%%", s.PacketDropRate, cfg.PacketDropRateThreshold))
	}
	
	// 5. 上报成功率告警
	if s.ReportSuccessRate < cfg.ReportSuccessRateMin {
		onAlert("report_success_low", s.ReportSuccessRate,
			fmt.Sprintf("上报成功率%.1f%%低于阈值%.1f%%", s.ReportSuccessRate, cfg.ReportSuccessRateMin))
	}
}

// Status 返回采集器状态摘要
func (c *Collector) Status() string {
	c.mu.RLock()
	s := c.lastSnapshot
	c.mu.RUnlock()
	
	return fmt.Sprintf("heartbeat=%d, cpu=%.1f%%, mem=%.1f%%, drop=%.1f%%, success=%.1f%%",
		s.HeartbeatStatus, s.CPUPercent, s.MemoryPercent, s.PacketDropRate, s.ReportSuccessRate)
}

// getCounterValue 安全地从 prometheus.Counter 获取当前值。
// prometheus.Metric 是 interface，不能直接用 &prometheus.Metric{} 创建空实例。
// 改用 dto.Metric 结构体来接收 Write 的输出。
func getCounterValue(counter prometheus.Counter) float64 {
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		return 0
	}
	if metric.Counter == nil {
		return 0
	}
	return metric.Counter.GetValue()
}
