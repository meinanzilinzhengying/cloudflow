// Package kafka Kafka 高可用消息队列集成
// 
// 特性:
//   - 多 Broker 集群支持
//   - 消息持久化和副本复制
//   - 消费者组重平衡优化
//   - 失败消息重试机制
//   - Dead Letter Queue (DLQ) 支持
//   - 指数退避重试策略
//   - 批量消息处理

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

// Config Kafka 配置
type Config struct {
	Brokers           []string
	Topic             string
	ConsumerGroup     string
	MinBytes          int
	MaxBytes          int
	MaxWait           time.Duration
	CommitInterval    time.Duration
	RetryAttempts     int
	RetryBackoff      time.Duration
	EnableDeadLetter  bool
	DeadLetterTopic   string
	MaxRetryCount     int
	BalancerType      string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Brokers:          []string{"localhost:9092"},
		Topic:            "cloudflow-flows",
		ConsumerGroup:    "cloudflow-consumer-group",
		MinBytes:         1,
		MaxBytes:         10e6,
		MaxWait:          time.Second,
		CommitInterval:   time.Second,
		RetryAttempts:    3,
		RetryBackoff:     time.Second,
		EnableDeadLetter: true,
		DeadLetterTopic:  "cloudflow-flows-dlq",
		MaxRetryCount:    3,
		BalancerType:     "leastbytes",
	}
}

// MessageHandler 消息处理函数类型
type MessageHandler func(ctx context.Context, key, value []byte) error

// Consumer Kafka 消费者
type Consumer struct {
	config     *Config
	reader    *kafka.Reader
	dlqWriter *kafka.Writer
	handlers  []MessageHandler
	running   bool
	mu        sync.RWMutex
	wg        sync.WaitGroup
	stopCh    chan struct{}
}

// NewConsumer 创建新的 Kafka 消费者
func NewConsumer(cfg *Config) (*Consumer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.ConsumerGroup,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		MaxWait:        cfg.MaxWait,
		CommitInterval: cfg.CommitInterval,
		StartOffset:    kafka.FirstOffset,
	})

	var dlqWriter *kafka.Writer
	if cfg.EnableDeadLetter {
		dlqWriter = &kafka.Writer{
			Addr:         kafka.TCP(cfg.Brokers...),
			Topic:        cfg.DeadLetterTopic,
			Balancer:     &kafka.LeastBytes{},
			BatchTimeout: 10 * time.Millisecond,
			RequiredAcks: kafka.RequireAll,
		}
	}

	return &Consumer{
		config:     cfg,
		reader:     reader,
		dlqWriter:  dlqWriter,
		handlers:   make([]MessageHandler, 0),
		stopCh:     make(chan struct{}),
	}, nil
}

// RegisterHandler 注册消息处理器
func (c *Consumer) RegisterHandler(handler MessageHandler) {
	c.handlers = append(c.handlers, handler)
}

// Start 启动消费者
func (c *Consumer) Start(ctx context.Context, workerCount int) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer already running")
	}
	c.running = true
	c.mu.Unlock()

	for i := 0; i < workerCount; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i)
	}

	return nil
}

// Stop 停止消费者
func (c *Consumer) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	close(c.stopCh)
	c.wg.Wait()

	if c.dlqWriter != nil {
		c.dlqWriter.Close()
	}

	return c.reader.Close()
}

// worker 消费者 worker
func (c *Consumer) worker(ctx context.Context, id int) {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Consumer worker %d: failed to fetch message: %v", id, err)
				time.Sleep(c.config.RetryBackoff)
				continue
			}

			if err := c.processMessageWithRetry(ctx, msg); err != nil {
				log.Printf("Consumer worker %d: failed to process message after retries: %v", id, err)
				if c.config.EnableDeadLetter {
					c.sendToDeadLetter(ctx, msg, err)
				}
			}

			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("Consumer worker %d: failed to commit message: %v", id, err)
			}
		}
	}
}

// processMessageWithRetry 带重试的消息处理
func (c *Consumer) processMessageWithRetry(ctx context.Context, msg kafka.Message) error {
	var lastErr error
	retryCount := 0

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		for _, handler := range c.handlers {
			if err := handler(ctx, msg.Key, msg.Value); err != nil {
				lastErr = err
				retryCount = attempt
				log.Printf("Consumer: handler failed (attempt %d/%d): %v", attempt+1, c.config.RetryAttempts, err)
				break
			}
		}

		if lastErr == nil {
			return nil
		}
	}

	return fmt.Errorf("after %d attempts: %w", retryCount+1, lastErr)
}

// calculateBackoff 计算指数退避时间
func (c *Consumer) calculateBackoff(attempt int) time.Duration {
	base := c.config.RetryBackoff
	factor := time.Duration(1<<uint(attempt-1)) * base
	maxBackoff := 30 * time.Second
	if factor > maxBackoff {
		factor = maxBackoff
	}
	return factor
}

// sendToDeadLetter 发送失败消息到 DLQ
func (c *Consumer) sendToDeadLetter(ctx context.Context, msg kafka.Message, originalErr error) {
	if c.dlqWriter == nil {
		return
	}

	headers := []kafka.Header{
		{Key: "original-topic", Value: []byte(msg.Topic)},
		{Key: "original-partition", Value: []byte(fmt.Sprintf("%d", msg.Partition))},
		{Key: "original-offset", Value: []byte(fmt.Sprintf("%d", msg.Offset))},
		{Key: "error-message", Value: []byte(originalErr.Error())},
		{Key: "retry-count", Value: []byte(fmt.Sprintf("%d", c.config.RetryAttempts))},
		{Key: "timestamp", Value: []byte(time.Now().Format(time.RFC3339))},
	}

	dlqMsg := kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	}

	if err := c.dlqWriter.WriteMessages(ctx, dlqMsg); err != nil {
		log.Printf("Failed to send message to DLQ: %v", err)
	}
}

// Producer Kafka 生产者
type Producer struct {
	writer *kafka.Writer
	config *Config
	mu     sync.RWMutex
}

// NewProducer 创建新的 Kafka 生产者
func NewProducer(cfg *Config) (*Producer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}

	return &Producer{
		writer: writer,
		config: cfg,
	}, nil
}

// SendMessage 发送单条消息
func (p *Producer) SendMessage(ctx context.Context, key, value []byte) error {
	return p.SendMessageWithRetry(ctx, key, value, p.config.RetryAttempts)
}

// SendMessageWithRetry 带重试的消息发送
func (p *Producer) SendMessageWithRetry(ctx context.Context, key, value []byte, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * p.config.RetryBackoff
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := p.writer.WriteMessages(ctx, kafka.Message{
			Key:   key,
			Value: value,
		})

		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("Producer: failed to send message (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
	}

	return fmt.Errorf("after %d attempts: %w", maxRetries+1, lastErr)
}

// SendBatch 批量发送消息
func (p *Producer) SendBatch(ctx context.Context, messages []kafka.Message) error {
	var lastErr error

	for attempt := 0; attempt <= p.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * p.config.RetryBackoff
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := p.writer.WriteMessages(ctx, messages...)
		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("Producer: failed to send batch (attempt %d/%d): %v", attempt+1, p.config.RetryAttempts+1, err)
	}

	return fmt.Errorf("after %d attempts: %w", p.config.RetryAttempts+1, lastErr)
}

// Close 关闭生产者
func (p *Producer) Close() error {
	return p.writer.Close()
}

// EnsureTopicExists 确保 Topic 存在
func EnsureTopicExists(brokers []string, topic string, partitions int, replicationFactor int) error {
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerConn, err := kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topic,
			NumPartitions:     partitions,
			ReplicationFactor: replicationFactor,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		return fmt.Errorf("failed to create topic %s: %w", topic, err)
	}

	return nil
}

// RetryMessage 重试消息结构
type RetryMessage struct {
	Key       string                 `json:"key"`
	Value     interface{}            `json:"value"`
	Headers   map[string]string      `json:"headers"`
	Retries   int                   `json:"retries"`
	NextRetry time.Time             `json:"next_retry"`
	Error     string                `json:"error,omitempty"`
}

// ToJSON 序列化为 JSON
func (m *RetryMessage) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// ParseRetryMessage 解析 JSON 为 RetryMessage
func ParseRetryMessage(data []byte) (*RetryMessage, error) {
	var msg RetryMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ConsumerStats 消费者统计
type ConsumerStats struct {
	MessagesReceived uint64
	MessagesProcessed uint64
	MessagesFailed   uint64
	MessagesDLQ     uint64
	Retries         uint64
}

// ProducerStats 生产者统计
type ProducerStats struct {
	MessagesSent     uint64
	MessagesFailed   uint64
	BatchesSent     uint64
	BatchesFailed   uint64
	AvgLatencyMs    float64
}

// FlowHandler Flow 数据处理函数
type FlowHandler func(ctx context.Context, key []byte, value []byte) error

// FlowConsumer Flow 消费者
type FlowConsumer struct {
	consumer *Consumer
	handler  FlowHandler
	config   *Config
	stats    ConsumerStats
	statsMu  sync.RWMutex
}

// NewFlowConsumer 创建 Flow 消费者
func NewFlowConsumer(cfg *Config, handler FlowHandler) (*FlowConsumer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	consumer, err := NewConsumer(cfg)
	if err != nil {
		return nil, err
	}

	fc := &FlowConsumer{
		consumer: consumer,
		handler:  handler,
		config:   cfg,
	}

	consumer.RegisterHandler(fc.messageHandler)

	return fc, nil
}

// Start 启动消费者
func (fc *FlowConsumer) Start(ctx context.Context, workerCount int) error {
	return fc.consumer.Start(ctx, workerCount)
}

// Stop 停止消费者
func (fc *FlowConsumer) Stop() error {
	return fc.consumer.Stop()
}

// messageHandler 消息处理
func (fc *FlowConsumer) messageHandler(ctx context.Context, key, value []byte) error {
	atomic.AddUint64(&fc.stats.MessagesReceived, 1)

	if err := fc.handler(ctx, key, value); err != nil {
		atomic.AddUint64(&fc.stats.MessagesFailed, 1)
		return err
	}

	atomic.AddUint64(&fc.stats.MessagesProcessed, 1)
	return nil
}

// GetStats 获取统计
func (fc *FlowConsumer) GetStats() ConsumerStats {
	fc.statsMu.RLock()
	defer fc.statsMu.RUnlock()
	return fc.stats
}

// FlowProducer Flow 生产者
type FlowProducer struct {
	producer *Producer
	config   *Config
	stats    ProducerStats
	statsMu  sync.RWMutex
}

// NewFlowProducer 创建 Flow 生产者
func NewFlowProducer(cfg *Config) (*FlowProducer, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	producer, err := NewProducer(cfg)
	if err != nil {
		return nil, err
	}

	return &FlowProducer{
		producer: producer,
		config:   cfg,
	}, nil
}

// SendFlow 发送单条 Flow 数据
func (fp *FlowProducer) SendFlow(ctx context.Context, key []byte, value []byte) error {
	if err := fp.producer.SendMessage(ctx, key, value); err != nil {
		atomic.AddUint64(&fp.stats.MessagesFailed, 1)
		return err
	}

	atomic.AddUint64(&fp.stats.MessagesSent, 1)
	return nil
}

// SendFlowBatch 批量发送 Flow 数据
func (fp *FlowProducer) SendFlowBatch(ctx context.Context, flows []kafka.Message) error {
	var lastErr error

	for attempt := 0; attempt <= fp.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * fp.config.RetryBackoff
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := fp.producer.SendBatch(ctx, flows)
		if err == nil {
			atomic.AddUint64(&fp.stats.BatchesSent, 1)
			atomic.AddUint64(&fp.stats.MessagesSent, uint64(len(flows)))
			return nil
		}

		lastErr = err
		log.Printf("FlowProducer: failed to send batch (attempt %d/%d): %v", attempt+1, fp.config.RetryAttempts+1, err)
	}

	atomic.AddUint64(&fp.stats.BatchesFailed, 1)
	atomic.AddUint64(&fp.stats.MessagesFailed, uint64(len(flows)))
	return fmt.Errorf("after %d attempts: %w", fp.config.RetryAttempts+1, lastErr)
}

// Close 关闭生产者
func (fp *FlowProducer) Close() error {
	return fp.producer.Close()
}

// GetStats 获取统计
func (fp *FlowProducer) GetStats() ProducerStats {
	fp.statsMu.RLock()
	defer fp.statsMu.RUnlock()
	return fp.stats
}

// KafkaBufferConfig 缓冲区配置
type KafkaBufferConfig struct {
	EnableBuffer   bool
	BufferSize     int
	FlushInterval  time.Duration
	RetryAttempts  int
	RetryBackoff   time.Duration
}

// DefaultKafkaBufferConfig 返回默认缓冲区配置
func DefaultKafkaBufferConfig() *KafkaBufferConfig {
	return &KafkaBufferConfig{
		EnableBuffer:   true,
		BufferSize:     1000,
		FlushInterval:  time.Second,
		RetryAttempts:  3,
		RetryBackoff:   time.Second,
	}
}

// BufferedFlowProducer 带缓冲的 Flow 生产者
type BufferedFlowProducer struct {
	producer *FlowProducer
	config  *KafkaBufferConfig
	buffer  []kafka.Message
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running atomic.Bool
	wg      sync.WaitGroup
}

// NewBufferedFlowProducer 创建带缓冲的生产者
func NewBufferedFlowProducer(cfg *KafkaBufferConfig) (*BufferedFlowProducer, error) {
	if cfg == nil {
		cfg = DefaultKafkaBufferConfig()
	}

	producer, err := NewFlowProducer(nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &BufferedFlowProducer{
		producer: producer,
		config:   cfg,
		buffer:   make([]kafka.Message, 0, cfg.BufferSize),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Start 启动缓冲区刷新循环
func (bp *BufferedFlowProducer) Start() {
	if !bp.running.CompareAndSwap(false, true) {
		return
	}

	bp.wg.Add(1)
	go bp.flushLoop()
}

// Stop 停止生产者
func (bp *BufferedFlowProducer) Stop() {
	if !bp.running.CompareAndSwap(true, false) {
		return
	}

	bp.cancel()
	bp.wg.Wait()

	bp.flushBuffer()
}

// Send 添加消息到缓冲区
func (bp *BufferedFlowProducer) Send(msg kafka.Message) error {
	bp.mu.Lock()
	bp.buffer = append(bp.buffer, msg)
	shouldFlush := len(bp.buffer) >= bp.config.BufferSize
	bp.mu.Unlock()

	if shouldFlush {
		return bp.flushBuffer()
	}

	return nil
}

// flushLoop 定期刷新缓冲区
func (bp *BufferedFlowProducer) flushLoop() {
	defer bp.wg.Done()

	ticker := time.NewTicker(bp.config.FlushInterval)
	defer ticker.Stop()

	for bp.running.Load() {
		select {
		case <-bp.ctx.Done():
			return
		case <-ticker.C:
			bp.flushBuffer()
		}
	}
}

// flushBuffer 刷新缓冲区
func (bp *BufferedFlowProducer) flushBuffer() error {
	bp.mu.Lock()
	if len(bp.buffer) == 0 {
		bp.mu.Unlock()
		return nil
	}

	msgs := bp.buffer
	bp.buffer = make([]kafka.Message, 0, bp.config.BufferSize)
	bp.mu.Unlock()

	return bp.producer.SendFlowBatch(bp.ctx, msgs)
}

// Close 关闭生产者
func (bp *BufferedFlowProducer) Close() error {
	bp.Stop()
	return bp.producer.Close()
}

// GetStats 获取统计
func (bp *BufferedFlowProducer) GetStats() ProducerStats {
	return bp.producer.GetStats()
}
