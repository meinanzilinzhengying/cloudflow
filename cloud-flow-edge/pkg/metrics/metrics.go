// Package metrics 提供 Prometheus 指标收集和暴露功能
package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cloud-flow-edge/pkg/logger"
)

// Metrics 指标收集器
type Metrics struct {
	// 接收指标
	receiveCount        prometheus.Counter
	receiveErrors       prometheus.Counter
	receiveBytes        prometheus.Counter

	// 转发指标
	forwardCount        prometheus.Counter
	forwardErrors       prometheus.Counter
	forwardDuration     prometheus.Histogram
	forwardBytes        prometheus.Counter

	// 独立的 traces 和 profiling 转发计数器
	tracesForwardCount  prometheus.Counter
	profilingForwardCount prometheus.Counter

	// 缓冲区指标
	metricsBufSize      prometheus.Gauge
	tracesBufSize       prometheus.Gauge
	profilingBufSize    prometheus.Gauge

	// 探针管理
	probeCount          prometheus.Gauge
	ProbesOnline        prometheus.Gauge
	HeartbeatsTotal     prometheus.Counter
	HeartbeatErrorsTotal prometheus.Counter

	// 注册中心
	registry            *prometheus.Registry

	// 日志记录器
	logger              *logger.Logger
}

// New 创建指标收集器
func New(log *logger.Logger) *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		logger:   log,
		// 接收指标
		receiveCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_receive_total",
			Help: "Total number of received metrics",
		}),
		receiveErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_receive_errors_total",
			Help: "Total number of receive errors",
		}),
		receiveBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_receive_bytes_total",
			Help: "Total bytes received",
		}),

		// 转发指标
		forwardCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_forward_total",
			Help: "Total number of forwarded metrics",
		}),
		forwardErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_forward_errors_total",
			Help: "Total number of forward errors",
		}),
		forwardDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "cloud_flow_edge_forward_duration_seconds",
			Help: "Duration of forward operation in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}),
		forwardBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_forward_bytes_total",
			Help: "Total bytes forwarded",
		}),

		// 独立的 traces 和 profiling 转发计数器
		tracesForwardCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_traces_forward_total",
			Help: "Total number of forwarded trace batches",
		}),
		profilingForwardCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_profiling_forward_total",
			Help: "Total number of forwarded profiling batches",
		}),

		// 缓冲区指标
		metricsBufSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cloud_flow_edge_metrics_buffer_size",
			Help: "Current size of metrics buffer",
		}),
		tracesBufSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cloud_flow_edge_traces_buffer_size",
			Help: "Current size of traces buffer",
		}),
		profilingBufSize: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cloud_flow_edge_profiling_buffer_size",
			Help: "Current size of profiling buffer",
		}),

		// 探针管理
		probeCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cloud_flow_edge_probe_count",
			Help: "Current number of registered probes",
		}),
		ProbesOnline: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "cloud_flow_edge_probes_online",
			Help: "Current number of online probes",
		}),
		HeartbeatsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_heartbeats_total",
			Help: "Total number of heartbeats sent",
		}),
		HeartbeatErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_edge_heartbeat_errors_total",
			Help: "Total number of heartbeat errors",
		}),
	}

	// 注册指标
	registry.MustRegister(
		m.receiveCount,
		m.receiveErrors,
		m.receiveBytes,
		m.forwardCount,
		m.forwardErrors,
		m.forwardDuration,
		m.forwardBytes,
		m.tracesForwardCount,
		m.profilingForwardCount,
		m.metricsBufSize,
		m.tracesBufSize,
		m.profilingBufSize,
		m.probeCount,
		m.ProbesOnline,
		m.HeartbeatsTotal,
		m.HeartbeatErrorsTotal,
	)

	// 注册标准指标
	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	return m
}

// Receive 记录接收指标
func (m *Metrics) Receive(bytes int, err error) {
	m.receiveBytes.Add(float64(bytes))
	if err != nil {
		m.receiveErrors.Inc()
	} else {
		m.receiveCount.Inc()
	}
}

// ForwardStarted 转发开始
func (m *Metrics) ForwardStarted() time.Time {
	return time.Now()
}

// ForwardFinished 转发完成
func (m *Metrics) ForwardFinished(start time.Time, bytes int, err error) {
	m.forwardDuration.Observe(time.Since(start).Seconds())
	m.forwardBytes.Add(float64(bytes))
	if err != nil {
		m.forwardErrors.Inc()
	}
	// NOTE: forwardCount 仅在 AddMetricsBatch/AddTracesBatch/AddProfilingBatch 中计数，
	// 不在此处重复 Inc()，避免双重计数。
}

// UpdateMetricsBufSize 更新指标缓冲区大小
func (m *Metrics) UpdateMetricsBufSize(size int) {
	m.metricsBufSize.Set(float64(size))
}

// UpdateTracesBufSize 更新链路追踪缓冲区大小
func (m *Metrics) UpdateTracesBufSize(size int) {
	m.tracesBufSize.Set(float64(size))
}

// UpdateProfilingBufSize 更新性能分析缓冲区大小
func (m *Metrics) UpdateProfilingBufSize(size int) {
	m.profilingBufSize.Set(float64(size))
}

// UpdateProbeCount 更新探针数量
func (m *Metrics) UpdateProbeCount(count int) {
	m.probeCount.Set(float64(count))
}

// AddForwardError 记录转发错误
func (m *Metrics) AddForwardError() {
	m.forwardErrors.Inc()
}

// AddMetricsBatch 记录指标批次
func (m *Metrics) AddMetricsBatch() {
	m.forwardCount.Inc()
}

// AddTracesBatch 记录链路追踪批次
func (m *Metrics) AddTracesBatch() {
	m.tracesForwardCount.Inc()
}

// AddProfilingBatch 记录性能分析批次
func (m *Metrics) AddProfilingBatch() {
	m.profilingForwardCount.Inc()
}

// RecordMetricsDropped 记录指标数据丢弃（M5 修复）
func (m *Metrics) RecordMetricsDropped(count int, reason string) {
	m.dataDroppedCount.Add(float64(count))
	m.logger.Warnf("[M5] 指标数据丢弃 %d 条，原因: %s", count, reason)
}

// RecordTracesDropped 记录链路追踪数据丢弃（M5 修复）
func (m *Metrics) RecordTracesDropped(count int, reason string) {
	// 复用 dataDroppedCount 或添加专门的计数器
	m.dataDroppedCount.Add(float64(count))
	m.logger.Warnf("[M5] 链路追踪数据丢弃 %d 条，原因: %s", count, reason)
}

// RecordProfilingDropped 记录性能分析数据丢弃（M5 修复）
func (m *Metrics) RecordProfilingDropped(count int, reason string) {
	m.dataDroppedCount.Add(float64(count))
	m.logger.Warnf("[M5] 性能分析数据丢弃 %d 条，原因: %s", count, reason)
}

// StartServer 启动 Prometheus 指标服务器
// 返回 *http.Server 用于优雅关闭，error channel 用于传递服务器运行中的错误
func (m *Metrics) StartServer(port int) (*http.Server, <-chan error) {
	addr := fmt.Sprintf(":%d", port)
	// 创建独立的 ServeMux 实例，避免与业务路由冲突
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	errCh := make(chan error, 1)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		m.logger.Infof("Prometheus metrics server 启动中，监听: %s/metrics", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Errorf("Prometheus metrics server 错误: %v", err)
			errCh <- err
		}
	}()

	return server, errCh
}
