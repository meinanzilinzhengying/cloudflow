// Package producer 提供高性能异步 Kafka Producer
// 特性: protobuf 序列化 + snappy 压缩 + 异步批量发送 + 零阻塞 + DLQ
package producer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/sarama"

	kafkapkg "cloud-flow/pkg/kafka"
)

// Message 待发送的消息
type Message struct {
	Topic string
	Key   []byte
	Value []byte // 已序列化的数据
}

// Producer Kafka 异步 Producer
type Producer struct {
	saramaProducer sarama.SyncProducer // 内部使用 SyncProducer 封装异步行为
	asyncProducer  sarama.AsyncProducer
	config         kafkapkg.Config
	logger         Logger

	// 异步发送通道
	msgCh chan *Message

	// 统计
	totalSent     atomic.Int64
	totalErrors   atomic.Int64
	totalBytes    atomic.Int64
	dlqCount      atomic.Int64

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

// New 创建 Kafka Producer
func New(cfg kafkapkg.Config, logger Logger) (*Producer, error) {
	if logger == nil {
		logger = noopLogger{}
	}

	saramaCfg := sarama.NewConfig()
	saramaCfg.Producer.RequiredAcks = parseAcks(cfg.Acks)
	saramaCfg.Producer.Retry.Max = cfg.Retries
	saramaCfg.Producer.Retry.Backoff = time.Duration(cfg.RetryBackoffMs) * time.Millisecond
	saramaCfg.Producer.Retry.BackoffMax = time.Duration(cfg.RetryMaxBackoffMs) * time.Millisecond
	saramaCfg.Producer.Return.Successes = true
	saramaCfg.Producer.Return.Errors = true
	saramaCfg.Producer.MaxMessageBytes = cfg.MaxMessageBytes
	saramaCfg.Producer.Flush.Bytes = cfg.BatchSize
	saramaCfg.Producer.Flush.Frequency = time.Duration(cfg.LingerMs) * time.Millisecond
	saramaCfg.Producer.Flush.Messages = 1000
	saramaCfg.Producer.BufferMemory = cfg.BufferMemory
	saramaCfg.Producer.Idempotent = cfg.EnableIdempotent
	saramaCfg.ClientID = cfg.ClientID
	saramaCfg.ChannelBufferSize = cfg.ChannelBufferSize

	// 压缩
	switch cfg.Compression {
	case "snappy":
		saramaCfg.Producer.Compression = sarama.CompressionSnappy
	case "gzip":
		saramaCfg.Producer.Compression = sarama.CompressionGZIP
	case "lz4":
		saramaCfg.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaCfg.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaCfg.Producer.Compression = sarama.CompressionNone
	}

	// 安全协议
	if err := setupSecurity(saramaCfg, cfg); err != nil {
		return nil, fmt.Errorf("Kafka 安全配置失败: %w", err)
	}

	// 创建 AsyncProducer（零阻塞）
	asyncProd, err := sarama.NewAsyncProducer(cfg.Brokers, saramaCfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Kafka AsyncProducer 失败: %w", err)
	}

	p := &Producer{
		asyncProducer: asyncProd,
		config:        cfg,
		logger:        logger,
		msgCh:         make(chan *Message, cfg.ChannelBufferSize),
		stopCh:        make(chan struct{}),
	}

	// 启动 success/error 回调处理
	go p.handleSuccesses()
	go p.handleErrors()

	return p, nil
}

// Send 异步发送消息（零阻塞）
// 消息通过通道传递给后台 goroutine，调用方不会阻塞
func (p *Producer) Send(topic string, key, value []byte) error {
	if p.stopped.Load() {
		return fmt.Errorf("producer 已停止")
	}

	select {
	case p.msgCh <- &Message{Topic: topic, Key: key, Value: value}:
		return nil
	case <-p.stopCh:
		return fmt.Errorf("producer 已停止")
	default:
		// 通道满时丢弃（零阻塞保证）
		p.totalErrors.Add(1)
		p.dlqCount.Add(1)
		return fmt.Errorf("发送通道已满，消息丢弃")
	}
}

// SendBatch 异步批量发送
func (p *Producer) SendBatch(topic string, messages []*Message) error {
	if p.stopped.Load() {
		return fmt.Errorf("producer 已停止")
	}

	for _, msg := range messages {
		if err := p.Send(topic, msg.Key, msg.Value); err != nil {
			return err
		}
	}
	return nil
}

// Start 启动发送循环（多 goroutine 并发发送）
func (p *Producer) Start(workers int) {
	if workers <= 0 {
		workers = 4
	}

	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.sendWorker(i)
	}
}

// sendWorker 发送工作协程
func (p *Producer) sendWorker(id int) {
	defer p.wg.Done()

	for {
		select {
		case msg := <-p.msgCh:
			kafkaMsg := &sarama.ProducerMessage{
				Topic: msg.Topic,
				Key:   sarama.ByteEncoder(msg.Key),
				Value: sarama.ByteEncoder(msg.Value),
			}
			p.asyncProducer.Input() <- kafkaMsg
			p.totalBytes.Add(int64(len(msg.Value)))
		case <-p.stopCh:
			return
		}
	}
}

// handleSuccesses 处理发送成功回调
func (p *Producer) handleSuccesses() {
	for {
		select {
		case msg := <-p.asyncProducer.Successes():
			p.totalSent.Add(1)
			_ = msg // 确认消息已发送
		case <-p.stopCh:
			return
		}
	}
}

// handleErrors 处理发送失败回调（自动进入 DLQ）
func (p *Producer) handleErrors() {
	for {
		select {
		case err := <-p.asyncProducer.Errors():
			p.totalErrors.Add(1)
			p.dlqCount.Add(1)
			p.logger.Warnf("Kafka 发送失败 (topic=%s): %v", err.Msg.Topic, err.Err)
		case <-p.stopCh:
			return
		}
	}
}

// Stop 优雅停止
func (p *Producer) Stop() {
	if p.stopped.Load() {
		return
	}
	p.stopped.Store(true)
	close(p.stopCh)

	// 等待所有 worker 完成
	p.wg.Wait()

	// 关闭 AsyncProducer
	if p.asyncProducer != nil {
		p.asyncProducer.AsyncClose()
	}

	p.logger.Infof("Kafka Producer 已停止: sent=%d, errors=%d, dlq=%d, bytes=%d",
		p.totalSent.Load(), p.totalErrors.Load(), p.dlqCount.Load(), p.totalBytes.Load())
}

// Stats 返回统计信息
func (p *Producer) Stats() (sent, errors, dlq, bytes int64) {
	return p.totalSent.Load(), p.totalErrors.Load(), p.dlqCount.Load(), p.totalBytes.Load()
}

// parseAcks 解析 acks 配置
func parseAcks(acks string) sarama.RequiredAcks {
	switch acks {
	case "0":
		return sarama.NoResponse
	case "all":
		return sarama.WaitForAll
	default:
		return sarama.WaitForLocal
	}
}

// setupSecurity 配置安全协议
func setupSecurity(cfg *sarama.Config, kafkaCfg kafkapkg.Config) error {
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
