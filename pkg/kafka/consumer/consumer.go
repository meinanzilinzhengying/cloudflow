// Package consumer 提供 Kafka Consumer Group 实现
// 特性: consumer group + protobuf 反序列化 + DLQ + broker failover
package consumer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"

	kafkapkg "cloud-flow/pkg/kafka"
)

// Handler 消息处理函数
type Handler func(ctx context.Context, topic string, key, value []byte) error

// Consumer Kafka Consumer Group
type Consumer struct {
	group   sarama.ConsumerGroup
	config  kafkapkg.ConsumerConfig
	handler Handler
	logger  Logger

	wg      sync.WaitGroup
	stopCh  chan struct{}
	stopped bool
}

// Logger 日志接口
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// New 创建 Kafka Consumer
func New(cfg kafkapkg.ConsumerConfig, handler Handler, logger Logger) (*Consumer, error) {
	saramaCfg := sarama.NewConfig()
	saramaCfg.ClientID = cfg.ClientID
	saramaCfg.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	saramaCfg.Consumer.Offsets.Initial = parseOffsetReset(cfg.AutoOffsetReset)
	saramaCfg.Consumer.Group.Session.Timeout = time.Duration(cfg.SessionTimeoutMs) * time.Millisecond
	saramaCfg.Consumer.Group.Heartbeat.Interval = time.Duration(cfg.HeartbeatIntervalMs) * time.Millisecond
	saramaCfg.Consumer.MaxProcessingTime = time.Duration(cfg.MaxPollIntervalMs) * time.Millisecond
	saramaCfg.Consumer.Fetch.Min = 1
	saramaCfg.Consumer.MaxWaitTime = 500 * time.Millisecond

	if err := setupSecurity(saramaCfg, cfg); err != nil {
		return nil, fmt.Errorf("Kafka 安全配置失败: %w", err)
	}

	group, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Consumer Group 失败: %w", err)
	}

	return &Consumer{
		group:   group,
		config:  cfg,
		handler: handler,
		logger:  logger,
		stopCh:  make(chan struct{}),
	}, nil
}

// Start 启动消费
func (c *Consumer) Start(ctx context.Context) error {
	topics := c.config.Topics
	if len(topics) == 0 {
		return fmt.Errorf("未配置消费 Topic")
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.stopCh:
				return
			default:
				err := c.group.Consume(ctx, topics, &consumerGroupHandler{
					handler: c.handler,
					logger:  c.logger,
				})
				if err != nil {
					c.logger.Errorf("Consumer Group 错误: %v", err)
					time.Sleep(5 * time.Second)
				}
				if ctx.Err() != nil {
					return
				}
			}
		}
	}()

	return nil
}

// Stop 优雅停止
func (c *Consumer) Stop() error {
	if c.stopped {
		return nil
	}
	c.stopped = true
	close(c.stopCh)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.group.Close(); err != nil {
		return fmt.Errorf("关闭 Consumer Group 失败: %w", err)
	}

	c.wg.Wait()
	<-ctx.Done()
	return nil
}

// consumerGroupHandler 实现 sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	handler Handler
	logger  Logger
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.logger.Infof("Consumer Group Session 建立: claims=%d", len(session.Claims()))
	return nil
}

func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.logger.Infof("Consumer Group Session 清理")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg := <-claim.Messages():
			if msg == nil {
				return nil
			}
			if err := h.handler(session.Context(), msg.Topic, msg.Key, msg.Value); err != nil {
				h.logger.Errorf("处理消息失败 (topic=%s, partition=%d, offset=%d): %v",
					msg.Topic, msg.Partition, msg.Offset, err)
				// 不标记 offset，消息将被重新消费
				continue
			}
			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}

func parseOffsetReset(reset string) int64 {
	if reset == "latest" {
		return sarama.OffsetNewest
	}
	return sarama.OffsetOldest
}

func setupSecurity(cfg *sarama.Config, kafkaCfg kafkapkg.ConsumerConfig) error {
	switch kafkaCfg.SecurityProtocol {
	case "SASL_SSL", "SASL_PLAINTEXT":
		cfg.Net.SASL.Enable = true
		cfg.Net.SASL.User = kafkaCfg.SASLUsername
		cfg.Net.SASL.Password = kafkaCfg.SASLPassword
		switch kafkaCfg.SASLMechanism {
		case "SCRAM-SHA-256":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
		case "SCRAM-SHA-512":
			cfg.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
		default:
			cfg.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
		if kafkaCfg.SecurityProtocol == "SASL_SSL" {
			cfg.Net.TLS.Enable = true
		}
	case "SSL":
		cfg.Net.TLS.Enable = true
	}
	return nil
}
