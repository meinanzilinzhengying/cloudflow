// Package kafkafwd 提供 Edge 到 Kafka 的异步数据转发器
// 替代原有的同步 gRPC 直连 Center 方式
// 特性: protobuf 序列化 + snappy 压缩 + 零阻塞异步发送 + DLQ
package kafkafwd

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	kafkapkg "cloud-flow/pkg/kafka"
	"github.com/IBM/sarama"
	"google.golang.org/protobuf/proto"
)

// Serializer 序列化接口
type Serializer interface {
	Serialize(topic string, data interface{}) ([]byte, error)
	TopicName(dataType string) string
}

// KafkaForwarder Kafka 转发器
type KafkaForwarder struct {
	producer  *sarama.AsyncProducer
	serializer Serializer
	logger     Logger

	// 统计
	metricsSent     atomic.Int64
	metricsErrors   atomic.Int64
	tracesSent      atomic.Int64
	tracesErrors    atomic.Int64
	profilingSent   atomic.Int64
	profilingErrors atomic.Int64
	totalBytes      atomic.Int64
	dlqCount        atomic.Int64

	// 控制
	wg      sync.WaitGroup
	stopCh  chan struct{}
	stopped atomic.Bool
}

// Logger 日志接口
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// noopLogger 空日志
type noopLogger struct{}

func (n noopLogger) Infof(string, ...interface{})  {}
func (n noopLogger) Warnf(string, ...interface{})  {}
func (n noopLogger) Errorf(string, ...interface{}) {}

// New 创建 Kafka 转发器
func New(kafkaBrokers []string, serializer Serializer, logger Logger) (*KafkaForwarder, error) {
	if logger == nil {
		logger = noopLogger{}
	}

	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForLocal
	cfg.Producer.Retry.Max = 5
	cfg.Producer.Retry.Backoff = 100 * time.Millisecond
	cfg.Producer.Retry.BackoffMax = 2 * time.Second
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Producer.MaxMessageBytes = 4 * 1024 * 1024
	cfg.Producer.Flush.Bytes = 1 << 16       // 64KB
	cfg.Producer.Flush.Frequency = 5 * time.Millisecond
	cfg.Producer.Flush.Messages = 500
	cfg.Producer.BufferMemory = 128 * 1024 * 1024 // 128MB
	cfg.Producer.Idempotent = true
	cfg.Producer.Compression = sarama.CompressionSnappy
	cfg.ChannelBufferSize = 16384

	asyncProd, err := sarama.NewAsyncProducer(kafkaBrokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Kafka AsyncProducer 失败: %w", err)
	}

	fwd := &KafkaForwarder{
		producer:   asyncProd,
		serializer: serializer,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}

	// 启动回调处理
	go fwd.handleSuccesses()
	go fwd.handleErrors()

	return fwd, nil
}

// SendMetrics 异步发送指标数据到 Kafka
func (f *KafkaForwarder) SendMetrics(data interface{}) error {
	return f.send(kafkapkg.TopicMetrics, "metrics", data, &f.metricsSent, &f.metricsErrors)
}

// SendTraces 异步发送链路追踪数据到 Kafka
func (f *KafkaForwarder) SendTraces(data interface{}) error {
	return f.send(kafkapkg.TopicTraces, "traces", data, &f.tracesSent, &f.tracesErrors)
}

// SendProfiling 异步发送性能分析数据到 Kafka
func (f *KafkaForwarder) SendProfiling(data interface{}) error {
	return f.send(kafkapkg.TopicProfiling, "profiling", data, &f.profilingSent, &f.profilingErrors)
}

// SendToTopic 发送数据到指定 Topic
func (f *KafkaForwarder) SendToTopic(topic string, data interface{}) error {
	return f.send(topic, topic, data, &f.metricsSent, &f.metricsErrors)
}

// send 内部发送方法（零阻塞）
func (f *KafkaForwarder) send(topic, dataType string, data interface{}, sent, errors *atomic.Int64) error {
	if f.stopped.Load() {
		return fmt.Errorf("forwarder 已停止")
	}

	// 序列化
	value, err := f.serializer.Serialize(dataType, data)
	if err != nil {
		errors.Add(1)
		return fmt.Errorf("序列化失败: %w", err)
	}

	// 发送到 Kafka（异步，零阻塞）
	msg := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(value),
	}

	select {
	case f.producer.Input() <- msg:
		f.totalBytes.Add(int64(len(value)))
		return nil
	case <-f.stopCh:
		return fmt.Errorf("forwarder 已停止")
	default:
		// 通道满时丢弃（零阻塞保证）
		errors.Add(1)
		f.dlqCount.Add(1)
		return fmt.Errorf("Kafka 发送通道已满，消息丢弃")
	}
}

// handleSuccesses 处理发送成功
func (f *KafkaForwarder) handleSuccesses() {
	for {
		select {
		case <-f.producer.Successes():
			f.metricsSent.Add(1)
		case <-f.stopCh:
			return
		}
	}
}

// handleErrors 处理发送失败（自动 DLQ）
func (f *KafkaForwarder) handleErrors() {
	for {
		select {
		case err := <-f.producer.Errors():
			f.metricsErrors.Add(1)
			f.dlqCount.Add(1)
			f.logger.Warnf("Kafka 发送失败 (topic=%s): %v", err.Msg.Topic, err.Err)
		case <-f.stopCh:
			return
		}
	}
}

// Stop 优雅停止
func (f *KafkaForwarder) Stop() {
	if f.stopped.Load() {
		return
	}
	f.stopped.Store(true)
	close(f.stopCh)

	if f.producer != nil {
		f.producer.AsyncClose()
	}

	f.logger.Infof("Kafka Forwarder 已停止: metrics=%d, traces=%d, profiling=%d, errors=%d, dlq=%d, bytes=%d",
		f.metricsSent.Load(), f.tracesSent.Load(), f.profilingSent.Load(),
		f.metricsErrors.Load(), f.dlqCount.Load(), f.totalBytes.Load())
}

// Stats 返回统计信息
func (f *KafkaForwarder) Stats() map[string]int64 {
	return map[string]int64{
		"metrics_sent":     f.metricsSent.Load(),
		"metrics_errors":   f.metricsErrors.Load(),
		"traces_sent":      f.tracesSent.Load(),
		"traces_errors":    f.tracesErrors.Load(),
		"profiling_sent":   f.profilingSent.Load(),
		"profiling_errors": f.profilingErrors.Load(),
		"total_bytes":      f.totalBytes.Load(),
		"dlq_count":        f.dlqCount.Load(),
	}
}

// ProtoSerializer Protobuf 序列化器
type ProtoSerializer struct{}

// Serialize 使用 protobuf 序列化
func (s *ProtoSerializer) Serialize(dataType string, data interface{}) ([]byte, error) {
	if msg, ok := data.(proto.Message); ok {
		return proto.Marshal(msg)
	}
	return nil, fmt.Errorf("数据类型 %T 不支持 protobuf 序列化", data)
}

// TopicName 返回数据类型对应的 Kafka Topic
func (s *ProtoSerializer) TopicName(dataType string) string {
	switch dataType {
	case "metrics":
		return kafkapkg.TopicMetrics
	case "traces":
		return kafkapkg.TopicTraces
	case "profiling":
		return kafkapkg.TopicProfiling
	case "logs":
		return kafkapkg.TopicLogs
	case "alerts":
		return kafkapkg.TopicAlerts
	case "topology":
		return kafkapkg.TopicTopology
	case "flow_raw":
		return kafkapkg.TopicFlowRaw
	case "flow_l4":
		return kafkapkg.TopicFlowL4
	case "flow_l7":
		return kafkapkg.TopicFlowL7
	default:
		return kafkapkg.TopicMetrics
	}
}
