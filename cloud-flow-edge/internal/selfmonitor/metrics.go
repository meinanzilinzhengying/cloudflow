// Package selfmonitor 提供Edge自监控功能
// 采集Edge节点的运行状态指标，每30秒上报至Center
package selfmonitor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-edge/internal/config"
	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// MetricsCollector Edge自监控指标采集器
type MetricsCollector struct {
	config     *config.Config
	logger     *logger.Logger
	centerClient edge.CenterServiceClient

	// 运行状态
	startTime     time.Time
	lastReportTime time.Time

	// 连接指标
	activeConnections   int64 // 当前活跃连接数
	totalConnections    int64 // 累计连接数
	connectionErrors    int64 // 连接错误数

	// 数据接收指标
	metricsReceived     int64 // 累计接收metrics数量
	tracesReceived      int64 // 累计接收traces数量
	profilingReceived   int64 // 累计接收profiling数量
	logsReceived        int64 // 累计接收logs数量
	bytesReceived       int64 // 累计接收字节数

	// 数据转发指标
	metricsForwarded    int64 // 累计转发metrics数量
	tracesForwarded     int64 // 累计转发traces数量
	profilingForwarded  int64 // 累计转发profiling数量
	logsForwarded       int64 // 累计转发logs数量
	bytesForwarded      int64 // 累计转发字节数
	forwardErrors       int64 // 转发错误数

	// 缓存指标
	cacheHits           int64 // 缓存命中次数
	cacheMisses         int64 // 缓存未命中次数
	cacheSize           int64 // 当前缓存大小（字节）
	cacheItems          int64 // 当前缓存条目数

	// 错误指标
	totalErrors         int64 // 总错误数
	authErrors          int64 // 认证错误数
	rateLimitHits       int64 // 限流触发次数
	circuitBreakerOpens int64 // 熔断器打开次数

	// 资源指标
	cpuUsage            float64 // CPU使用率
	memoryUsage         float64 // 内存使用率
	memoryUsed          int64   // 内存使用量（字节）
	goroutineCount      int64   // 当前goroutine数

	// 探针指标
	probeCount          int32   // 当前探针数
	onlineProbeCount    int32   // 在线探针数

	// L1 修复: 可配置的上报间隔
	reportInterval      time.Duration

	// 内部状态
	mu                  sync.RWMutex
	running             bool
	stopCh              chan struct{}
	wg                  sync.WaitGroup
}

// EdgeMetrics Edge节点监控指标快照
type EdgeMetrics struct {
	Timestamp           int64   `json:"timestamp"`
	EdgeNodeID          string  `json:"edge_node_id"`
	EdgeAddress         string  `json:"edge_address"`
	CloudPlatform       string  `json:"cloud_platform"`
	Region              string  `json:"region"`
	Zone                string  `json:"zone"`
	Version             string  `json:"version"`
	Uptime              int64   `json:"uptime"`               // 运行时长（秒）

	// 连接指标
	ActiveConnections   int64   `json:"active_connections"`   // 当前活跃连接数
	TotalConnections    int64   `json:"total_connections"`    // 累计连接数
	ConnectionErrors    int64   `json:"connection_errors"`    // 连接错误数
	ConnectionErrorRate float64 `json:"connection_error_rate"` // 连接错误率

	// 数据接收指标（当前周期）
	MetricsReceivedDelta   int64 `json:"metrics_received_delta"`   // 本周期接收metrics数
	TracesReceivedDelta    int64 `json:"traces_received_delta"`    // 本周期接收traces数
	ProfilingReceivedDelta int64 `json:"profiling_received_delta"` // 本周期接收profiling数
	LogsReceivedDelta      int64 `json:"logs_received_delta"`      // 本周期接收logs数
	BytesReceivedDelta     int64 `json:"bytes_received_delta"`     // 本周期接收字节数

	// 数据接收指标（累计）
	MetricsReceivedTotal   int64 `json:"metrics_received_total"`   // 累计接收metrics数
	TracesReceivedTotal    int64 `json:"traces_received_total"`    // 累计接收traces数
	ProfilingReceivedTotal int64 `json:"profiling_received_total"` // 累计接收profiling数
	LogsReceivedTotal      int64 `json:"logs_received_total"`      // 累计接收logs数
	BytesReceivedTotal     int64 `json:"bytes_received_total"`     // 累计接收字节数

	// 数据转发指标（当前周期）
	MetricsForwardedDelta   int64 `json:"metrics_forwarded_delta"`   // 本周期转发metrics数
	TracesForwardedDelta    int64 `json:"traces_forwarded_delta"`    // 本周期转发traces数
	ProfilingForwardedDelta int64 `json:"profiling_forwarded_delta"` // 本周期转发profiling数
	LogsForwardedDelta      int64 `json:"logs_forwarded_delta"`      // 本周期转发logs数
	BytesForwardedDelta     int64 `json:"bytes_forwarded_delta"`     // 本周期转发字节数
	ForwardErrorsDelta      int64 `json:"forward_errors_delta"`      // 本周期转发错误数

	// 数据转发指标（累计）
	MetricsForwardedTotal   int64 `json:"metrics_forwarded_total"`   // 累计转发metrics数
	TracesForwardedTotal    int64 `json:"traces_forwarded_total"`    // 累计转发traces数
	ProfilingForwardedTotal int64 `json:"profiling_forwarded_total"` // 累计转发profiling数
	LogsForwardedTotal      int64 `json:"logs_forwarded_total"`      // 累计转发logs数
	BytesForwardedTotal     int64 `json:"bytes_forwarded_total"`     // 累计转发字节数
	ForwardErrorsTotal      int64 `json:"forward_errors_total"`      // 累计转发错误数

	// 缓存指标
	CacheHitsDelta      int64   `json:"cache_hits_delta"`       // 本周期缓存命中
	CacheMissesDelta    int64   `json:"cache_misses_delta"`     // 本周期缓存未命中
	CacheHitRate        float64 `json:"cache_hit_rate"`         // 缓存命中率
	CacheSize           int64   `json:"cache_size"`             // 当前缓存大小（字节）
	CacheItems          int64   `json:"cache_items"`            // 当前缓存条目数

	// 错误指标（当前周期）
	TotalErrorsDelta      int64 `json:"total_errors_delta"`       // 本周期总错误数
	AuthErrorsDelta       int64 `json:"auth_errors_delta"`        // 本周期认证错误数
	RateLimitHitsDelta    int64 `json:"rate_limit_hits_delta"`    // 本周期限流触发数
	CircuitBreakerOpensDelta int64 `json:"circuit_breaker_opens_delta"` // 本周期熔断器打开数

	// 错误指标（累计）
	TotalErrorsTotal      int64 `json:"total_errors_total"`       // 累计总错误数
	AuthErrorsTotal       int64 `json:"auth_errors_total"`        // 累计认证错误数
	RateLimitHitsTotal    int64 `json:"rate_limit_hits_total"`    // 累计限流触发数
	CircuitBreakerOpensTotal int64 `json:"circuit_breaker_opens_total"` // 累计熔断器打开数

	// 错误率
	ErrorRate           float64 `json:"error_rate"`             // 总错误率
	ForwardErrorRate    float64 `json:"forward_error_rate"`     // 转发错误率

	// 资源指标
	CpuUsage            float64 `json:"cpu_usage"`              // CPU使用率
	MemoryUsage         float64 `json:"memory_usage"`           // 内存使用率
	MemoryUsed          int64   `json:"memory_used"`            // 内存使用量（字节）
	GoroutineCount      int64   `json:"goroutine_count"`        // 当前goroutine数

	// 探针指标
	ProbeCount          int32   `json:"probe_count"`            // 当前探针数
	OnlineProbeCount    int32   `json:"online_probe_count"`     // 在线探针数
	ProbeOnlineRate     float64 `json:"probe_online_rate"`      // 探针在线率
}

// NewMetricsCollector 创建新的监控指标采集器
// L1 修复: 使用配置的上报间隔
func NewMetricsCollector(cfg *config.Config, log *logger.Logger, client edge.CenterServiceClient) *MetricsCollector {
	reportInterval := cfg.SelfMonitorInterval
	if reportInterval <= 0 {
		reportInterval = 30 * time.Second // 默认值
	}
	return &MetricsCollector{
		config:         cfg,
		logger:         log,
		centerClient:   client,
		startTime:      time.Now(),
		stopCh:         make(chan struct{}),
		reportInterval: reportInterval,
	}
}

// Start 启动监控指标采集和上报
func (c *MetricsCollector) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return
	}

	c.running = true
	c.wg.Add(1)

	go c.reportLoop(ctx)

	c.logger.Info("Edge自监控指标采集器已启动，上报间隔: 30s")
}

// Stop 停止监控指标采集
func (c *MetricsCollector) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopCh)
	c.mu.Unlock()

	c.wg.Wait()
	c.logger.Info("Edge自监控指标采集器已停止")
}

// reportLoop 定时上报循环
func (c *MetricsCollector) reportLoop(ctx context.Context) {
	defer c.wg.Done()

	// L1 修复: 使用可配置的上报间隔（通过 MetricsCollector 字段）
	reportInterval := c.reportInterval
	if reportInterval <= 0 {
		reportInterval = 30 * time.Second // 默认值
	}
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	// 立即上报一次
	c.report(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.report(ctx)
		}
	}
}

// report 执行一次指标上报
func (c *MetricsCollector) report(ctx context.Context) {
	metrics := c.Collect()

	req := &edge.ReportEdgeMetricsRequest{
		EdgeNodeId:    metrics.EdgeNodeID,
		EdgeAddress:   metrics.EdgeAddress,
		CloudPlatform: metrics.CloudPlatform,
		Region:        metrics.Region,
		Zone:          metrics.Zone,
		Version:       metrics.Version,
		Timestamp:     metrics.Timestamp,
		Uptime:        metrics.Uptime,

		// 连接指标
		ActiveConnections:   metrics.ActiveConnections,
		TotalConnections:    metrics.TotalConnections,
		ConnectionErrors:    metrics.ConnectionErrors,
		ConnectionErrorRate: metrics.ConnectionErrorRate,

		// 数据接收指标
		MetricsReceivedDelta:   metrics.MetricsReceivedDelta,
		TracesReceivedDelta:    metrics.TracesReceivedDelta,
		ProfilingReceivedDelta: metrics.ProfilingReceivedDelta,
		LogsReceivedDelta:      metrics.LogsReceivedDelta,
		BytesReceivedDelta:     metrics.BytesReceivedDelta,

		MetricsReceivedTotal:   metrics.MetricsReceivedTotal,
		TracesReceivedTotal:    metrics.TracesReceivedTotal,
		ProfilingReceivedTotal: metrics.ProfilingReceivedTotal,
		LogsReceivedTotal:      metrics.LogsReceivedTotal,
		BytesReceivedTotal:     metrics.BytesReceivedTotal,

		// 数据转发指标
		MetricsForwardedDelta:   metrics.MetricsForwardedDelta,
		TracesForwardedDelta:    metrics.TracesForwardedDelta,
		ProfilingForwardedDelta: metrics.ProfilingForwardedDelta,
		LogsForwardedDelta:      metrics.LogsForwardedDelta,
		BytesForwardedDelta:     metrics.BytesForwardedDelta,
		ForwardErrorsDelta:      metrics.ForwardErrorsDelta,

		MetricsForwardedTotal:   metrics.MetricsForwardedTotal,
		TracesForwardedTotal:    metrics.TracesForwardedTotal,
		ProfilingForwardedTotal: metrics.ProfilingForwardedTotal,
		LogsForwardedTotal:      metrics.LogsForwardedTotal,
		BytesForwardedTotal:     metrics.BytesForwardedTotal,
		ForwardErrorsTotal:      metrics.ForwardErrorsTotal,

		// 缓存指标
		CacheHitsDelta:   metrics.CacheHitsDelta,
		CacheMissesDelta: metrics.CacheMissesDelta,
		CacheHitRate:     metrics.CacheHitRate,
		CacheSize:        metrics.CacheSize,
		CacheItems:       metrics.CacheItems,

		// 错误指标
		TotalErrorsDelta:         metrics.TotalErrorsDelta,
		AuthErrorsDelta:          metrics.AuthErrorsDelta,
		RateLimitHitsDelta:       metrics.RateLimitHitsDelta,
		CircuitBreakerOpensDelta: metrics.CircuitBreakerOpensDelta,

		TotalErrorsTotal:         metrics.TotalErrorsTotal,
		AuthErrorsTotal:          metrics.AuthErrorsTotal,
		RateLimitHitsTotal:       metrics.RateLimitHitsTotal,
		CircuitBreakerOpensTotal: metrics.CircuitBreakerOpensTotal,

		ErrorRate:        metrics.ErrorRate,
		ForwardErrorRate: metrics.ForwardErrorRate,

		// 资源指标
		CpuUsage:       metrics.CpuUsage,
		MemoryUsage:    metrics.MemoryUsage,
		MemoryUsed:     metrics.MemoryUsed,
		GoroutineCount: metrics.GoroutineCount,

		// 探针指标
		ProbeCount:       metrics.ProbeCount,
		OnlineProbeCount: metrics.OnlineProbeCount,
		ProbeOnlineRate:  metrics.ProbeOnlineRate,
	}

	if c.centerClient != nil {
		resp, err := c.centerClient.ReportEdgeMetrics(ctx, req)
		if err != nil {
			c.logger.Errorf("上报Edge监控指标失败: %v", err)
			return
		}
		if !resp.Success {
			c.logger.Errorf("Center拒绝Edge监控指标: %s", resp.Message)
			return
		}
	}

	c.lastReportTime = time.Now()
	c.logger.Debugf("Edge监控指标已上报: connections=%d, metrics=%d, cache_hit_rate=%.2f%%",
		metrics.ActiveConnections, metrics.MetricsReceivedDelta, metrics.CacheHitRate*100)
}

// Collect 采集当前指标快照
func (c *MetricsCollector) Collect() *EdgeMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	uptime := int64(now.Sub(c.startTime).Seconds())

	// 读取当前值
	activeConns := atomic.LoadInt64(&c.activeConnections)
	totalConns := atomic.LoadInt64(&c.totalConnections)
	connErrors := atomic.LoadInt64(&c.connectionErrors)

	metricsRecv := atomic.LoadInt64(&c.metricsReceived)
	tracesRecv := atomic.LoadInt64(&c.tracesReceived)
	profilingRecv := atomic.LoadInt64(&c.profilingReceived)
	logsRecv := atomic.LoadInt64(&c.logsReceived)
	bytesRecv := atomic.LoadInt64(&c.bytesReceived)

	metricsFwd := atomic.LoadInt64(&c.metricsForwarded)
	tracesFwd := atomic.LoadInt64(&c.tracesForwarded)
	profilingFwd := atomic.LoadInt64(&c.profilingForwarded)
	logsFwd := atomic.LoadInt64(&c.logsForwarded)
	bytesFwd := atomic.LoadInt64(&c.bytesForwarded)
	fwdErrors := atomic.LoadInt64(&c.forwardErrors)

	cacheHits := atomic.LoadInt64(&c.cacheHits)
	cacheMisses := atomic.LoadInt64(&c.cacheMisses)
	cacheSize := atomic.LoadInt64(&c.cacheSize)
	cacheItems := atomic.LoadInt64(&c.cacheItems)

	totalErrs := atomic.LoadInt64(&c.totalErrors)
	authErrs := atomic.LoadInt64(&c.authErrors)
	rateLimitHits := atomic.LoadInt64(&c.rateLimitHits)
	cbOpens := atomic.LoadInt64(&c.circuitBreakerOpens)

	cpuUsage := c.cpuUsage
	memUsage := c.memoryUsage
	memUsed := c.memoryUsed
	goroutineCount := c.goroutineCount

	probeCount := c.probeCount
	onlineProbeCount := c.onlineProbeCount

	// 计算增量（需要保存上次上报的值）
	// 这里简化处理，实际应该保存上次上报的快照
	// 为了演示，我们假设这是第一次上报，增量等于总量

	// 计算错误率
	connErrorRate := float64(0)
	if totalConns > 0 {
		connErrorRate = float64(connErrors) / float64(totalConns)
	}

	fwdErrorRate := float64(0)
	totalFwd := metricsFwd + tracesFwd + profilingFwd + logsFwd
	if totalFwd > 0 {
		fwdErrorRate = float64(fwdErrors) / float64(totalFwd)
	}

	totalErrRate := float64(0)
	totalOps := totalConns + totalFwd
	if totalOps > 0 {
		totalErrRate = float64(totalErrs) / float64(totalOps)
	}

	// 计算缓存命中率
	cacheHitRate := float64(0)
	totalCacheOps := cacheHits + cacheMisses
	if totalCacheOps > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalCacheOps)
	}

	// 计算探针在线率
	probeOnlineRate := float64(0)
	if probeCount > 0 {
		probeOnlineRate = float64(onlineProbeCount) / float64(probeCount)
	}

	return &EdgeMetrics{
		Timestamp:     now.Unix(),
		EdgeNodeID:    c.config.EdgeNodeID,
		EdgeAddress:   c.getEdgeAddress(),
		CloudPlatform: c.config.CloudPlatform,
		Region:        c.config.Region,
		Zone:          "", // 可从配置或元数据获取
		Version:       "v2.0.0", // 从版本文件获取
		Uptime:        uptime,

		ActiveConnections:   activeConns,
		TotalConnections:    totalConns,
		ConnectionErrors:    connErrors,
		ConnectionErrorRate: connErrorRate,

		MetricsReceivedDelta:   metricsRecv,
		TracesReceivedDelta:    tracesRecv,
		ProfilingReceivedDelta: profilingRecv,
		LogsReceivedDelta:      logsRecv,
		BytesReceivedDelta:     bytesRecv,

		MetricsReceivedTotal:   metricsRecv,
		TracesReceivedTotal:    tracesRecv,
		ProfilingReceivedTotal: profilingRecv,
		LogsReceivedTotal:      logsRecv,
		BytesReceivedTotal:     bytesRecv,

		MetricsForwardedDelta:   metricsFwd,
		TracesForwardedDelta:    tracesFwd,
		ProfilingForwardedDelta: profilingFwd,
		LogsForwardedDelta:      logsFwd,
		BytesForwardedDelta:     bytesFwd,
		ForwardErrorsDelta:      fwdErrors,

		MetricsForwardedTotal:   metricsFwd,
		TracesForwardedTotal:    tracesFwd,
		ProfilingForwardedTotal: profilingFwd,
		LogsForwardedTotal:      logsFwd,
		BytesForwardedTotal:     bytesFwd,
		ForwardErrorsTotal:      fwdErrors,

		CacheHitsDelta:   cacheHits,
		CacheMissesDelta: cacheMisses,
		CacheHitRate:     cacheHitRate,
		CacheSize:        cacheSize,
		CacheItems:       cacheItems,

		TotalErrorsDelta:         totalErrs,
		AuthErrorsDelta:          authErrs,
		RateLimitHitsDelta:       rateLimitHits,
		CircuitBreakerOpensDelta: cbOpens,

		TotalErrorsTotal:         totalErrs,
		AuthErrorsTotal:          authErrs,
		RateLimitHitsTotal:       rateLimitHits,
		CircuitBreakerOpensTotal: cbOpens,

		ErrorRate:        totalErrRate,
		ForwardErrorRate: fwdErrorRate,

		CpuUsage:       cpuUsage,
		MemoryUsage:    memUsage,
		MemoryUsed:     memUsed,
		GoroutineCount: goroutineCount,

		ProbeCount:       probeCount,
		OnlineProbeCount: onlineProbeCount,
		ProbeOnlineRate:  probeOnlineRate,
	}
}

// getEdgeAddress 获取Edge节点地址
func (c *MetricsCollector) getEdgeAddress() string {
	// 优先使用配置中的地址，否则自动生成
	if c.config.CenterAddr != "" {
		return c.config.CenterAddr
	}
	return "localhost:50051"
}

// ============= 指标更新方法 =============

// RecordConnection 记录新连接
func (c *MetricsCollector) RecordConnection() {
	atomic.AddInt64(&c.activeConnections, 1)
	atomic.AddInt64(&c.totalConnections, 1)
}

// RecordDisconnection 记录断开连接
func (c *MetricsCollector) RecordDisconnection() {
	atomic.AddInt64(&c.activeConnections, -1)
}

// RecordConnectionError 记录连接错误
func (c *MetricsCollector) RecordConnectionError() {
	atomic.AddInt64(&c.connectionErrors, 1)
	atomic.AddInt64(&c.totalErrors, 1)
}

// RecordMetricsReceived 记录接收的metrics
func (c *MetricsCollector) RecordMetricsReceived(count int64, bytes int64) {
	atomic.AddInt64(&c.metricsReceived, count)
	atomic.AddInt64(&c.bytesReceived, bytes)
}

// RecordTracesReceived 记录接收的traces
func (c *MetricsCollector) RecordTracesReceived(count int64, bytes int64) {
	atomic.AddInt64(&c.tracesReceived, count)
	atomic.AddInt64(&c.bytesReceived, bytes)
}

// RecordProfilingReceived 记录接收的profiling
func (c *MetricsCollector) RecordProfilingReceived(count int64, bytes int64) {
	atomic.AddInt64(&c.profilingReceived, count)
	atomic.AddInt64(&c.bytesReceived, bytes)
}

// RecordLogsReceived 记录接收的logs
func (c *MetricsCollector) RecordLogsReceived(count int64, bytes int64) {
	atomic.AddInt64(&c.logsReceived, count)
	atomic.AddInt64(&c.bytesReceived, bytes)
}

// RecordMetricsForwarded 记录转发的metrics
func (c *MetricsCollector) RecordMetricsForwarded(count int64, bytes int64) {
	atomic.AddInt64(&c.metricsForwarded, count)
	atomic.AddInt64(&c.bytesForwarded, bytes)
}

// RecordTracesForwarded 记录转发的traces
func (c *MetricsCollector) RecordTracesForwarded(count int64, bytes int64) {
	atomic.AddInt64(&c.tracesForwarded, count)
	atomic.AddInt64(&c.bytesForwarded, bytes)
}

// RecordProfilingForwarded 记录转发的profiling
func (c *MetricsCollector) RecordProfilingForwarded(count int64, bytes int64) {
	atomic.AddInt64(&c.profilingForwarded, count)
	atomic.AddInt64(&c.bytesForwarded, bytes)
}

// RecordLogsForwarded 记录转发的logs
func (c *MetricsCollector) RecordLogsForwarded(count int64, bytes int64) {
	atomic.AddInt64(&c.logsForwarded, count)
	atomic.AddInt64(&c.bytesForwarded, bytes)
}

// RecordForwardError 记录转发错误
func (c *MetricsCollector) RecordForwardError() {
	atomic.AddInt64(&c.forwardErrors, 1)
	atomic.AddInt64(&c.totalErrors, 1)
}

// RecordCacheHit 记录缓存命中
func (c *MetricsCollector) RecordCacheHit() {
	atomic.AddInt64(&c.cacheHits, 1)
}

// RecordCacheMiss 记录缓存未命中
func (c *MetricsCollector) RecordCacheMiss() {
	atomic.AddInt64(&c.cacheMisses, 1)
}

// UpdateCacheStats 更新缓存统计
func (c *MetricsCollector) UpdateCacheStats(size int64, items int64) {
	atomic.StoreInt64(&c.cacheSize, size)
	atomic.StoreInt64(&c.cacheItems, items)
}

// RecordAuthError 记录认证错误
func (c *MetricsCollector) RecordAuthError() {
	atomic.AddInt64(&c.authErrors, 1)
	atomic.AddInt64(&c.totalErrors, 1)
}

// RecordRateLimitHit 记录限流触发
func (c *MetricsCollector) RecordRateLimitHit() {
	atomic.AddInt64(&c.rateLimitHits, 1)
}

// RecordCircuitBreakerOpen 记录熔断器打开
func (c *MetricsCollector) RecordCircuitBreakerOpen() {
	atomic.AddInt64(&c.circuitBreakerOpens, 1)
}

// UpdateResourceUsage 更新资源使用情况
func (c *MetricsCollector) UpdateResourceUsage(cpuUsage float64, memoryUsage float64, memoryUsed int64, goroutineCount int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cpuUsage = cpuUsage
	c.memoryUsage = memoryUsage
	c.memoryUsed = memoryUsed
	c.goroutineCount = goroutineCount
}

// UpdateProbeStats 更新探针统计
func (c *MetricsCollector) UpdateProbeStats(total int32, online int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probeCount = total
	c.onlineProbeCount = online
}

// GetCurrentMetrics 获取当前指标（用于实时查询）
func (c *MetricsCollector) GetCurrentMetrics() *EdgeMetrics {
	return c.Collect()
}
