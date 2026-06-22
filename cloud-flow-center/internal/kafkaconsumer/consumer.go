// Package kafkaconsumer 提供 Kafka 消费者功能
// P0: Flow Ingest Pipeline - Center 消费 Kafka 数据写入 TiDB
// P1: 消息幂等性 - Redis SETNX 去重 + 手动提交 offset
package kafkaconsumer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"

	"github.com/meinanzilinzhengying/cloudflow/center/internal/storage"
	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

// Consumer Kafka 消费者
type Consumer struct {
	brokers       []string
	groupID       string
	topics        []string
	storage       storage.StorageEngine
	logger        *logger.Logger
	dedup         DedupChecker // P2-03: 消息去重器
	consumerGroup sarama.ConsumerGroup
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	stopped       bool
	stopMu        sync.Mutex

	// 消息幂等性：Redis 去重
	redisClient  *redis.Client
	dedupEnabled bool
	dedupTTL     time.Duration
	dedupSkipped atomic.Int64  // 去重跳过的消息数
	dedupErrors  atomic.Int64  // 去重错误数
}

// ConsumerGroupHandler 消费者组处理器
type ConsumerGroupHandler struct {
	storage storage.StorageEngine
	logger  *logger.Logger
	dedup   DedupChecker // P2-03: 消息去重器
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
		dedup:   NewMemoryDedup(24 * time.Hour), // P0-9 修复: 默认启用内存去重
	}, nil
}

// NewWithDedup 创建带消息去重的 Kafka 消费者
func NewWithDedup(brokers []string, groupID string, topics []string, store storage.StorageEngine, log *logger.Logger, dedup DedupChecker) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers 不能为空")
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("Kafka topics 不能为空")
	}
	if dedup == nil {
		dedup = &NoOpDedup{}
	}

	return &Consumer{
		brokers: brokers,
		groupID: groupID,
		topics:  topics,
		storage: store,
		logger:  log,
		dedup:   dedup,
	}, nil
}

// WithDedup 启用 Redis 消息去重
func (c *Consumer) WithDedup(redisAddr string, ttl time.Duration) *Consumer {
	if c == nil {
		return nil
	}
	c.redisClient = redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	c.dedupEnabled = true
	c.dedupTTL = ttl
	c.logger.Infof("[kafkaconsumer] 消息幂等性已启用: Redis=%s, TTL=%v", redisAddr, ttl)
	return c
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
	// P1修复: 改为手动提交，保证 at-least-once + 去重 = effectively-once
	config.Consumer.Offsets.AutoCommit.Enable = false

	consumerGroup, err := sarama.NewConsumerGroup(c.brokers, c.groupID, config)
	if err != nil {
		return fmt.Errorf("创建 Kafka 消费者组失败: %w", err)
	}

	c.consumerGroup = consumerGroup
	c.ctx, c.cancel = context.WithCancel(context.Background())

	handler := &ConsumerGroupHandler{
		storage: c.storage,
		logger:  c.logger,
		dedup:   c.dedup, // P2-03: 传递去重器
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
	c.logger.Infof("Kafka 消费者已启动 (brokers=%v, topics=%v, group=%s, dedup=%v)",
		c.brokers, c.topics, c.groupID, c.dedupEnabled)
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

	// P2-03: 关闭去重器
	if c.dedup != nil {
		if err := c.dedup.Close(); err != nil {
			c.logger.Warnf("关闭去重器失败: %v", err)
		}
	}

	c.logger.Info("Kafka 消费者已停止")
}

// DedupStats 返回幂等性统计
func (c *Consumer) DedupStats() (skipped, errors int64) {
	return c.dedupSkipped.Load(), c.dedupErrors.Load()
}

// Setup 消费者组设置
func (h *ConsumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	return nil
}

// Cleanup 消费者组清理
func (h *ConsumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	// 清理前提交 offset
	session.Commit()
	return nil
}

// ConsumeClaim 消费消息（幂等性保证）
func (h *ConsumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// 定时提交 offset（手动提交模式下防止大量未提交 offset 堆积）
	commitTicker := time.NewTicker(10 * time.Second)
	defer commitTicker.Stop()

	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// P2-03: 消息去重检查
			isDup, err := h.dedup.IsDuplicate(message.Topic, message.Partition, message.Offset)
			if err != nil {
				h.logger.Warnf("去重检查失败 (topic=%s, partition=%d, offset=%d): %v, 继续处理",
					message.Topic, message.Partition, message.Offset, err)
			} else if isDup {
				h.logger.Infof("跳过重复消息 (topic=%s, partition=%d, offset=%d)",
					message.Topic, message.Partition, message.Offset)
				session.MarkMessage(message, "")
				continue
			}

			if err := h.processMessage(message); err != nil {
				h.logger.Errorf("处理 Kafka 消息失败 (topic=%s, partition=%d, offset=%d): %v",
					message.Topic, message.Partition, message.Offset, err)
				// P1修复: 处理失败不标记消息, 下次 rebalance 后重新消费
				// 同时记录去重 key, 防止无限重试
				if h.dedupEnabled && h.redisClient != nil {
					h.markDedup(h.buildDedupKey(message))
				}
				continue
			}

			// P1修复: 只有成功处理才标记消息
			session.MarkMessage(message, "")

		case <-commitTicker.C:
			// 定期提交已标记的 offset
			session.Commit()

		case <-session.Context().Done():
			session.Commit()
			return nil
		}
	}
}

// buildDedupKey 构建消息去重键
// 使用 topic + partition + offset 组合生成唯一标识
func (h *ConsumerGroupHandler) buildDedupKey(msg *sarama.ConsumerMessage) string {
	// 如果有业务 key, 优先使用
	if len(msg.Key) > 0 {
		return fmt.Sprintf("kafka:dedup:%s:%s", msg.Topic, string(msg.Key))
	}
	// 否则使用 topic:partition:offset:timestamp hash
	raw := fmt.Sprintf("%s:%d:%d:%d:%s", msg.Topic, msg.Partition, msg.Offset, msg.Timestamp.UnixNano(), string(msg.Value[:min(len(msg.Value), 128)]))
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("kafka:dedup:%s:%x", msg.Topic, hash[:8])
}

// checkDedup 检查消息是否已处理（Redis SETNX）
// 返回 (isDuplicate, error)
func (h *ConsumerGroupHandler) checkDedup(key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// SETNX: 如果 key 不存在则设置并返回 true（新消息）, 否则返回 false（重复消息）
	result, err := h.redisClient.SetNX(ctx, key, "1", h.dedupTTL).Result()
	if err != nil {
		return false, err
	}
	// SetNX 返回 true 表示 key 不存在, 是新消息 => 不重复
	// SetNX 返回 false 表示 key 已存在, 是重复消息 => 重复
	return !result, nil
}

// markDedup 标记消息为已处理（用于失败消息防止无限重试）
func (h *ConsumerGroupHandler) markDedup(key string) {
	if h.redisClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.redisClient.Set(ctx, key, "failed", h.dedupTTL)
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
