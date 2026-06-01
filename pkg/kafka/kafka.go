// Package kafka 定义 Kafka 相关常量、Topic 和配置
package kafka

// Topic 常量定义
const (
	TopicFlowRaw      = "flow.raw"      // 原始网络流数据
	TopicFlowL4       = "flow.l4"       // L4 层网络流（TCP/UDP）
	TopicFlowL7       = "flow.l7"       // L7 层网络流（HTTP/gRPC）
	TopicMetrics      = "metrics"       // 系统指标（CPU/内存/磁盘/网络）
	TopicLogs         = "logs"          // 日志数据
	TopicAlerts       = "alerts"        // 告警事件
	TopicTopology     = "topology"      // 拓扑数据
	TopicProfiling    = "profiling"     // 性能分析数据
	TopicTraces       = "traces"        // 链路追踪数据
	TopicDLQPrefix    = "dlq."          // 死信队列前缀
)

// DLQTopic 生成死信队列 Topic 名
func DLQTopic(sourceTopic string) string {
	return TopicDLQPrefix + sourceTopic
}

// AllTopics 返回所有业务 Topic
func AllTopics() []string {
	return []string{
		TopicFlowRaw,
		TopicFlowL4,
		TopicFlowL7,
		TopicMetrics,
		TopicLogs,
		TopicAlerts,
		TopicTopology,
		TopicProfiling,
		TopicTraces,
	}
}

// Config Kafka 客户端配置
type Config struct {
	Brokers           []string `mapstructure:"brokers"`             // Kafka broker 地址列表
	ProducerID        string   `mapstructure:"producer_id"`         // Producer ID（用于日志和指标）
	ClientID          string   `mapstructure:"client_id"`           // Kafka 客户端 ID
	Compression       string   `mapstructure:"compression"`        // 压缩算法: snappy/gzip/lz4/zstd/none
	MaxMessageBytes   int      `mapstructure:"max_message_bytes"`   // 单条消息最大字节数
	Acks              string   `mapstructure:"acks"`                // acks: 0/1/all
	Retries           int      `mapstructure:"retries"`             // 发送重试次数
	RetryBackoffMs    int      `mapstructure:"retry_backoff_ms"`    // 重试基础延迟（毫秒）
	RetryMaxBackoffMs int      `mapstructure:"retry_max_backoff_ms"`// 重试最大延迟（毫秒）
	BatchSize         int      `mapstructure:"batch_size"`          // 批量发送大小（字节数）
	LingerMs          int      `mapstructure:"linger_ms"`           // 批量等待时间（毫秒）
	BufferMemory      int      `mapstructure:"buffer_memory"`       // Producer 缓冲区大小（字节）
	ChannelBufferSize int      `mapstructure:"channel_buffer_size"` // 内部通道缓冲区大小
	EnableIdempotent  bool     `mapstructure:"enable_idempotent"`   // 幂等 Producer
	SecurityProtocol  string   `mapstructure:"security_protocol"`   // 安全协议: PLAINTEXT/SASL_SSL/SASL_PLAINTEXT
	SASLMechanism     string   `mapstructure:"sasl_mechanism"`      // SASL 机制: PLAIN/SCRAM-SHA-256/SCRAM-SHA-512
	SASLUsername      string   `mapstructure:"sasl_username"`       // SASL 用户名
	SASLPassword      string   `mapstructure:"sasl_password"`       // SASL 密码
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		ProducerID:        "cloud-flow-edge",
		ClientID:          "cloud-flow-edge-producer",
		Compression:       "snappy",
		MaxMessageBytes:   4 * 1024 * 1024, // 4MB
		Acks:              "1",
		Retries:           3,
		RetryBackoffMs:    100,
		RetryMaxBackoffMs: 1000,
		BatchSize:         65536,       // 64KB
		LingerMs:          5,           // 5ms
		BufferMemory:      64 * 1024 * 1024, // 64MB
		ChannelBufferSize: 8192,
		EnableIdempotent:  true,
		SecurityProtocol:  "PLAINTEXT",
	}
}

// ConsumerConfig Kafka Consumer 配置
type ConsumerConfig struct {
	Brokers           []string `mapstructure:"brokers"`
	GroupID           string   `mapstructure:"group_id"`
	ClientID          string   `mapstructure:"client_id"`
	Topics            []string `mapstructure:"topics"`
	AutoOffsetReset   string   `mapstructure:"auto_offset_reset"`  // earliest/latest
	SessionTimeoutMs  int      `mapstructure:"session_timeout_ms"`
	HeartbeatIntervalMs int    `mapstructure:"heartbeat_interval_ms"`
	MaxPollRecords    int      `mapstructure:"max_poll_records"`
	MaxPollIntervalMs int      `mapstructure:"max_poll_interval_ms"`
	EnableAutoCommit  bool     `mapstructure:"enable_auto_commit"`
	AutoCommitIntervalMs int   `mapstructure:"auto_commit_interval_ms"`
	SecurityProtocol  string   `mapstructure:"security_protocol"`
	SASLMechanism     string   `mapstructure:"sasl_mechanism"`
	SASLUsername      string   `mapstructure:"sasl_username"`
	SASLPassword      string   `mapstructure:"sasl_password"`
}

// DefaultConsumerConfig 返回默认 Consumer 配置
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		GroupID:            "cloud-flow-consumer",
		ClientID:           "cloud-flow-consumer",
		AutoOffsetReset:    "earliest",
		SessionTimeoutMs:   10000,
		HeartbeatIntervalMs: 3000,
		MaxPollRecords:     500,
		MaxPollIntervalMs:  300000,
		EnableAutoCommit:   true,
		AutoCommitIntervalMs: 1000,
		SecurityProtocol:   "PLAINTEXT",
	}
}
