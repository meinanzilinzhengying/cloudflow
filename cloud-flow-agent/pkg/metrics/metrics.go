// Package metrics 提供 Prometheus 指标收集和暴露功能
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 指标收集器
type Metrics struct {
	// 采集指标
	collectCount    prometheus.Counter
	collectErrors   prometheus.Counter
	collectDuration prometheus.Histogram

	// 发送指标
	sendCount    prometheus.Counter
	sendErrors   prometheus.Counter
	sendDuration prometheus.Histogram
	sendBytes    prometheus.Counter

	// 心跳指标
	heartbeatCount  prometheus.Counter
	heartbeatErrors prometheus.Counter

	// EBPF 指标
	ebpfCollectCount  prometheus.Counter
	ebpfCollectErrors prometheus.Counter

	// 数据丢弃指标 (C3 修复)
	dataDroppedCount prometheus.Counter
	bufOverflowCount prometheus.Counter

	// 注册中心
	registry *prometheus.Registry
}

// New 创建指标收集器
func New() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		// 采集指标
		collectCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_collect_total",
			Help: "Total number of metric collections",
		}),
		collectErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_collect_errors_total",
			Help: "Total number of collection errors",
		}),
		collectDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "cloud_flow_agent_collect_duration_seconds",
			Help:    "Duration of metric collection in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}),

		// 发送指标
		sendCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_send_total",
			Help: "Total number of metric sends",
		}),
		sendErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_send_errors_total",
			Help: "Total number of send errors",
		}),
		sendDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "cloud_flow_agent_send_duration_seconds",
			Help:    "Duration of metric send in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		}),
		sendBytes: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_send_bytes_total",
			Help: "Total bytes sent",
		}),

		// 心跳指标
		heartbeatCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_heartbeat_total",
			Help: "Total number of heartbeats",
		}),
		heartbeatErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_heartbeat_errors_total",
			Help: "Total number of heartbeat errors",
		}),

		// EBPF 指标
		ebpfCollectCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_ebpf_collect_total",
			Help: "Total number of EBPF collections",
		}),
		ebpfCollectErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_ebpf_collect_errors_total",
			Help: "Total number of EBPF collection errors",
		}),

		// 数据丢弃指标 (C3 修复)
		dataDroppedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_data_dropped_total",
			Help: "Total number of data points dropped due to buffer overflow or send failure",
		}),
		bufOverflowCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cloud_flow_agent_buf_overflow_total",
			Help: "Total number of buffer overflow events",
		}),

		registry: registry,
	}

	// 注册指标
	registry.MustRegister(
		m.collectCount,
		m.collectErrors,
		m.collectDuration,
		m.sendCount,
		m.sendErrors,
		m.sendDuration,
		m.sendBytes,
		m.heartbeatCount,
		m.heartbeatErrors,
		m.ebpfCollectCount,
		m.ebpfCollectErrors,
		m.dataDroppedCount,
		m.bufOverflowCount,
	)

	// 注册标准指标
	registry.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	return m
}

// CollectStarted 采集开始
func (m *Metrics) CollectStarted() time.Time {
	return time.Now()
}

// CollectFinished 采集完成
func (m *Metrics) CollectFinished(start time.Time, err error) {
	m.collectDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		m.collectErrors.Inc()
	} else {
		m.collectCount.Inc()
	}
}

// SendStarted 发送开始
func (m *Metrics) SendStarted() time.Time {
	return time.Now()
}

// SendFinished 发送完成
func (m *Metrics) SendFinished(start time.Time, bytes int, err error) {
	m.sendDuration.Observe(time.Since(start).Seconds())
	m.sendBytes.Add(float64(bytes))
	if err != nil {
		m.sendErrors.Inc()
	} else {
		m.sendCount.Inc()
	}
}

// HeartbeatSent 心跳发送
func (m *Metrics) HeartbeatSent(err error) {
	if err != nil {
		m.heartbeatErrors.Inc()
	} else {
		m.heartbeatCount.Inc()
	}
}

// EBPFCollect 记录 EBPF 采集
func (m *Metrics) EBPFCollect(err error) {
	if err != nil {
		m.ebpfCollectErrors.Inc()
	} else {
		m.ebpfCollectCount.Inc()
	}
}

// GetCollectCount 获取采集总数计数器（用于自监控）
func (m *Metrics) GetCollectCount() prometheus.Counter {
	return m.collectCount
}

// GetCollectErrors 获取采集错误计数器（用于自监控）
func (m *Metrics) GetCollectErrors() prometheus.Counter {
	return m.collectErrors
}

// GetSendCount 获取发送总数计数器（用于自监控）
func (m *Metrics) GetSendCount() prometheus.Counter {
	return m.sendCount
}

// GetSendErrors 获取发送错误计数器（用于自监控）
func (m *Metrics) GetSendErrors() prometheus.Counter {
	return m.sendErrors
}

// RecordDataDropped 记录数据丢弃 (C3 修复)
// count: 丢弃的数据点数量
// reason: 丢弃原因 (buffer_overflow, cache_failed, network_unavailable)
func (m *Metrics) RecordDataDropped(count int, reason string) {
	m.dataDroppedCount.Add(float64(count))
}

// RecordBufOverflow 记录缓冲区溢出事件 (C3 修复)
func (m *Metrics) RecordBufOverflow() {
	m.bufOverflowCount.Inc()
}

// StartServer 启动 Prometheus 指标服务器
// 返回 *http.Server 用于优雅关闭，error channel 用于传递服务器运行中的错误
func (m *Metrics) StartServer(addr string) (*http.Server, <-chan error) {
	// 创建独立的 ServeMux 实例，避免与业务路由冲突
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	errCh := make(chan error, 1)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	return server, errCh
}
