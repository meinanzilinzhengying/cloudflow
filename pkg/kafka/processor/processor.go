// Package processor 提供 Flow 数据处理管道
// 从 Kafka 消费原始数据，处理后写入存储
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	kafkapkg "cloud-flow/pkg/kafka"
	"github.com/IBM/sarama"
)

// Storage 存储接口（解耦处理与存储）
type Storage interface {
	WriteMetrics(ctx context.Context, data []byte) error
	WriteTraces(ctx context.Context, data []byte) error
	WriteProfiling(ctx context.Context, data []byte) error
	WriteLogs(ctx context.Context, data []byte) error
	WriteAlerts(ctx context.Context, data []byte) error
	WriteTopology(ctx context.Context, data []byte) error
	WriteFlow(ctx context.Context, flowType string, data []byte) error
}

// Processor 数据处理管道
type Processor struct {
	storage Storage
	dlq     *sarama.SyncProducer // 死信队列 Producer

	// 统计
	totalProcessed atomic.Int64
	totalErrors    atomic.Int64
	totalDLQ       atomic.Int64

	// 控制
	workers int
	wg      sync.WaitGroup
	stopCh  chan struct{}
	stopped atomic.Bool
}

// New 创建 Processor
func New(storage Storage, dlqProducer *sarama.SyncProducer, workers int) *Processor {
	if workers <= 0 {
		workers = 4
	}
	return &Processor{
		storage: storage,
		dlq:     dlqProducer,
		workers: workers,
		stopCh:  make(chan struct{}),
	}
}

// Process 处理单条消息（路由到对应的存储方法）
func (p *Processor) Process(ctx context.Context, topic string, key, value []byte) error {
	if p.stopped.Load() {
		return fmt.Errorf("processor 已停止")
	}

	var err error
	switch topic {
	case kafkapkg.TopicMetrics:
		err = p.storage.WriteMetrics(ctx, value)
	case kafkapkg.TopicTraces:
		err = p.storage.WriteTraces(ctx, value)
	case kafkapkg.TopicProfiling:
		err = p.storage.WriteProfiling(ctx, value)
	case kafkapkg.TopicLogs:
		err = p.storage.WriteLogs(ctx, value)
	case kafkapkg.TopicAlerts:
		err = p.storage.WriteAlerts(ctx, value)
	case kafkapkg.TopicTopology:
		err = p.storage.WriteTopology(ctx, value)
	case kafkapkg.TopicFlowRaw:
		err = p.storage.WriteFlow(ctx, "raw", value)
	case kafkapkg.TopicFlowL4:
		err = p.storage.WriteFlow(ctx, "l4", value)
	case kafkapkg.TopicFlowL7:
		err = p.storage.WriteFlow(ctx, "l7", value)
	default:
		return fmt.Errorf("未知 topic: %s", topic)
	}

	if err != nil {
		p.totalErrors.Add(1)
		// 发送到 DLQ
		if p.dlq != nil {
			p.sendToDLQ(topic, key, value, err)
		}
		return err
	}

	p.totalProcessed.Add(1)
	return nil
}

// sendToDLQ 发送失败消息到死信队列
func (p *Processor) sendToDLQ(topic string, key, value []byte, originalErr error) {
	dlqTopic := kafkapkg.DLQTopic(topic)

	// 附加错误信息
	dlqValue := map[string]interface{}{
		"original_topic": topic,
		"error":          originalErr.Error(),
		"timestamp":      time.Now().Unix(),
		"data":           value,
	}
	data, _ := json.Marshal(dlqValue)

	msg := &sarama.ProducerMessage{
		Topic: dlqTopic,
		Key:   sarama.ByteEncoder(key),
		Value: sarama.ByteEncoder(data),
	}

	_, _, err := p.dlq.SendMessage(msg)
	if err != nil {
		p.totalDLQ.Add(1)
	}
}

// Stats 返回统计信息
func (p *Processor) Stats() (processed, errors, dlq int64) {
	return p.totalProcessed.Load(), p.totalErrors.Load(), p.totalDLQ.Load()
}

// Stop 停止 Processor
func (p *Processor) Stop() {
	if p.stopped.Load() {
		return
	}
	p.stopped.Store(true)
	close(p.stopCh)
	p.wg.Wait()
}
