// Package kafkaconsumer 提供 Kafka 消费者功能
// P0: Flow Ingest Pipeline - Center 消费 Kafka 数据写入 TiDB
package kafkaconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"cloudflow-center/internal/storage"
	"cloudflow-center/pkg/logger"
	edge "cloudflow/proto"
)

// Consumer Kafka 消费者
type Consumer struct {
	brokers       []string
	groupID       string
	topics        []string
	storage       storage.StorageEngine
	logger        *logger.Logger
	consumerGroup sarama.ConsumerGroup
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	stopped       bool
	stopMu        sync.Mutex
}

// ConsumerGroupHandler 消费者组处理器
type ConsumerGroupHandler struct {
	storage storage.StorageEngine
	logger  *logger.Logger
	ready   chan bool
}

// New 创建 Kafka 消费者
func New(brokers []string, groupID string, topics []string, store storage.StorageEngine, log *logger.Logger) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers 不能为空")
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("Kafka topics 不能为空")
	}

	return &Consumer{
		brokers: brokers,
		groupID: groupID,
		topics:  topics,
		storage: store,
		logger:  log,
	}, nil
}

// Start 启动消费者
func (c *Consumer) Start() error {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopped {
		return fmt.Errorf("消费者已停止")
	}

	config := sarama.NewConfig()
	config.Version = sarama.V2_6_0_0
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 5 * time.Second

	consumerGroup, err := sarama.NewConsumerGroup(c.brokers, c.groupID, config)
	if err != nil {
		return fmt.Errorf("创建 Kafka 消费者组失败: %w", err)
	}

	c.consumerGroup = consumerGroup
	c.ctx, c.cancel = context.WithCancel(context.Background())

	handler := &ConsumerGroupHandler{
		storage: c.storage,
		logger:  c.logger,
		ready:   make(chan bool),
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			if err := c.consumerGroup.Consume(c.ctx, c.topics, handler); err != nil {
				c.logger.Errorf("Kafka 消费错误: %v", err)
			}
			if c.ctx.Err() != nil {
				return
			}
			handler.ready = make(chan bool)
		}
	}()

	<-handler.ready
	c.logger.Infof("Kafka 消费者已启动 (brokers=%v, topics=%v, group=%s)", c.brokers, c.topics, c.groupID)
	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true

	if c.cancel != nil {
		c.cancel()
	}
	if c.consumerGroup != nil {
		c.consumerGroup.Close()
	}
	c.wg.Wait()
	c.logger.Info("Kafka 消费者已停止")
}

// Setup 消费者组设置
func (h *ConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	return nil
}

// Cleanup 消费者组清理
func (h *ConsumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim 消费消息
func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}
			if err := h.processMessage(message); err != nil {
				h.logger.Errorf("处理 Kafka 消息失败 (topic=%s, partition=%d, offset=%d): %v",
					message.Topic, message.Partition, message.Offset, err)
			}
			session.MarkMessage(message, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

// processMessage 处理单条消息
func (h *ConsumerGroupHandler) processMessage(msg *sarama.ConsumerMessage) error {
	topic := msg.Topic

	switch topic {
	case "flow.raw", "flow.l4", "flow.l7":
		return h.processFlowData(msg.Value)
	case "metrics":
		return h.processMetrics(msg.Value)
	case "traces":
		return h.processTraces(msg.Value)
	case "logs":
		return h.processLogs(msg.Value)
	case "profiling":
		return h.processProfiling(msg.Value)
	default:
		h.logger.Warnf("未知 topic: %s", topic)
		return nil
	}
}

// processFlowData 处理流数据
func (h *ConsumerGroupHandler) processFlowData(data []byte) error {
	var batch edge.FlowBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("反序列化 FlowBatch 失败: %w", err)
	}
	probeID := batch.GetProbeId()
	if probeID == "" {
		probeID = "unknown"
	}
	if err := h.storage.SaveMetrics(probeID, batch.GetFlows()); err != nil {
		return fmt.Errorf("保存流数据失败: %w", err)
	}
	return nil
}

// processMetrics 处理指标数据
func (h *ConsumerGroupHandler) processMetrics(data []byte) error {
	var batch edge.MetricsBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("反序列化 MetricsBatch 失败: %w", err)
	}
	probeID := batch.GetProbeId()
	if probeID == "" {
		probeID = "unknown"
	}
	if err := h.storage.SaveMetrics(probeID, batch.GetMetrics()); err != nil {
		return fmt.Errorf("保存指标失败: %w", err)
	}
	return nil
}

// processTraces 处理链路追踪数据
func (h *ConsumerGroupHandler) processTraces(data []byte) error {
	var batch edge.TraceBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("反序列化 TraceBatch 失败: %w", err)
	}
	probeID := batch.GetProbeId()
	if probeID == "" {
		probeID = "unknown"
	}
	if err := h.storage.SaveTraces(probeID, batch.GetSpans()); err != nil {
		return fmt.Errorf("保存链路追踪失败: %w", err)
	}
	return nil
}

// processLogs 处理日志数据
func (h *ConsumerGroupHandler) processLogs(data []byte) error {
	var batch edge.LogBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("反序列化 LogBatch 失败: %w", err)
	}
	probeID := batch.GetProbeId()
	if probeID == "" {
		probeID = "unknown"
	}
	if err := h.storage.SaveMetrics(probeID, batch.GetLogs()); err != nil {
		return fmt.Errorf("保存日志失败: %w", err)
	}
	return nil
}

// processProfiling 处理性能分析数据
func (h *ConsumerGroupHandler) processProfiling(data []byte) error {
	var batch edge.ProfilingBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return fmt.Errorf("反序列化 ProfilingBatch 失败: %w", err)
	}
	probeID := batch.GetProbeId()
	if probeID == "" {
		probeID = "unknown"
	}
	if err := h.storage.SaveProfiling(probeID, batch.GetProfiles()); err != nil {
		return fmt.Errorf("保存性能分析失败: %w", err)
	}
	return nil
}
