// Package forwarder 提供 Kafka 适配器
// P0: Flow Ingest Pipeline - 将 kafkafwd.KafkaForwarder 适配为 ForwardClient 接口
package forwarder

import (
	"sync"
	"time"

	"cloud-flow-edge/internal/kafkafwd"
	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// KafkaAdapter 将 KafkaForwarder 适配为 ForwardClient 接口
type KafkaAdapter struct {
	kafkaFwd *kafkafwd.KafkaForwarder
	logger   *logger.Logger
	metrics  MetricsSink
	
	// 缓冲区（用于批量发送）
	muMetrics    sync.Mutex
	metricsBuf   []*edge.MetricsBatch
	
	muTraces     sync.Mutex
	tracesBuf    []*edge.TraceBatch
	
	muProfiling  sync.Mutex
	profilingBuf []*edge.ProfilingBatch
	
	batchSize     int
	flushInterval time.Duration
	stopCh        chan struct{}
	stopped       bool
	stopMu        sync.Mutex
}

// NewKafkaAdapter 创建 Kafka 适配器，返回 *Forwarder 以保持接口兼容
func NewKafkaAdapter(kafkaFwd *kafkafwd.KafkaForwarder, log *logger.Logger) *Forwarder {
	adapter := &KafkaAdapter{
		kafkaFwd:      kafkaFwd,
		logger:        log,
		metrics:       noopMetrics{},
		metricsBuf:    make([]*edge.MetricsBatch, 0, 100),
		tracesBuf:     make([]*edge.TraceBatch, 0, 100),
		profilingBuf:  make([]*edge.ProfilingBatch, 0, 100),
		batchSize:     1,  // Kafka 模式下立即发送，不缓冲
		flushInterval: 0, // 不使用定时刷新
		stopCh:        make(chan struct{}),
		stopped:       false,
	}
	
	// 返回 Forwarder 结构，但使用 KafkaAdapter 作为 client
	return &Forwarder{
		client:        adapter,
		logger:        log,
		metrics:       noopMetrics{},
		batchSize:     1,
		flushInterval: 0,
		maxBufLimit:   10000,
		stopCh:        make(chan struct{}),
		stopped:       false,
	}
}

// ForwardMetrics 转发指标数据到 Kafka
func (a *KafkaAdapter) ForwardMetrics(batch *edge.MetricsBatch) error {
	if a.kafkaFwd == nil || batch == nil {
		return nil
	}
	return a.kafkaFwd.SendMetrics(batch)
}

// ForwardTraces 转发链路追踪数据到 Kafka
func (a *KafkaAdapter) ForwardTraces(batch *edge.TraceBatch) error {
	if a.kafkaFwd == nil || batch == nil {
		return nil
	}
	return a.kafkaFwd.SendTraces(batch)
}

// ForwardProfiling 转发性能分析数据到 Kafka
func (a *KafkaAdapter) ForwardProfiling(batch *edge.ProfilingBatch) error {
	if a.kafkaFwd == nil || batch == nil {
		return nil
	}
	return a.kafkaFwd.SendProfiling(batch)
}

// SetMetrics 设置指标上报接口
func (a *KafkaAdapter) SetMetrics(m MetricsSink) {
	a.metrics = m
}

// Start 启动适配器（Kafka 模式下不需要定时刷新）
func (a *KafkaAdapter) Start() {
	// Kafka 模式下立即发送，不需要启动定时刷新协程
	a.logger.Info("KafkaAdapter 已启动（零阻塞模式）")
}

// Stop 停止适配器
func (a *KafkaAdapter) Stop() {
	a.stopMu.Lock()
	defer a.stopMu.Unlock()
	if !a.stopped {
		a.stopped = true
		close(a.stopCh)
		if a.kafkaFwd != nil {
			a.kafkaFwd.Stop()
		}
	}
}

// Stats 返回统计信息
func (a *KafkaAdapter) Stats() map[string]int64 {
	if a.kafkaFwd != nil {
		return a.kafkaFwd.Stats()
	}
	return map[string]int64{
		"metrics_sent": 0,
		"traces_sent":  0,
		"logs_sent":    0,
		"errors":       0,
	}
}

// UpdateClient 更新客户端（Kafka 模式下无操作）
func (a *KafkaAdapter) UpdateClient(client ForwardClient) {
	// Kafka 模式下不需要更新 client
}

// AddMetrics 添加指标（直接发送到 Kafka）
func (a *KafkaAdapter) AddMetrics(batch *edge.MetricsBatch) {
	if err := a.ForwardMetrics(batch); err != nil {
		a.logger.Errorf("Kafka 发送 Metrics 失败: %v", err)
		if a.metrics != nil {
			a.metrics.AddForwardError()
		}
	} else {
		if a.metrics != nil {
			a.metrics.AddMetricsBatch()
		}
	}
}

// AddTraces 添加链路追踪（直接发送到 Kafka）
func (a *KafkaAdapter) AddTraces(batch *edge.TraceBatch) {
	if err := a.ForwardTraces(batch); err != nil {
		a.logger.Errorf("Kafka 发送 Traces 失败: %v", err)
		if a.metrics != nil {
			a.metrics.AddForwardError()
		}
	} else {
		if a.metrics != nil {
			a.metrics.AddTracesBatch()
		}
	}
}

// AddProfiling 添加性能分析（直接发送到 Kafka）
func (a *KafkaAdapter) AddProfiling(batch *edge.ProfilingBatch) {
	if err := a.ForwardProfiling(batch); err != nil {
		a.logger.Errorf("Kafka 发送 Profiling 失败: %v", err)
		if a.metrics != nil {
			a.metrics.AddForwardError()
		}
	} else {
		if a.metrics != nil {
			a.metrics.AddProfilingBatch()
		}
	}
}
