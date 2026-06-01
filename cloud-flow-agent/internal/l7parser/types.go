// Package l7parser L7 协议解析引擎
//
// 架构设计:
//   ┌─────────────────────────────────────────────────────────────┐
//   │                    L7Parser Engine                           │
//   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
//   │  │   HTTP1     │  │   HTTP2     │  │      gRPC           │  │
//   │  │  Parser     │  │  Parser     │  │     Parser          │  │
//   │  └─────────────┘  └─────────────┘  └─────────────────────┘  │
//   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
//   │  │   MySQL     │  │   Redis     │  │      Kafka          │  │
//   │  │  Parser     │  │  Parser     │  │     Parser          │  │
//   │  └─────────────┘  └─────────────┘  └─────────────────────┘  │
//   │                                                             │
//   │  ┌─────────────────────────────────────────────────────────┐│
//   │  │              Protocol Auto Detector                      ││
//   │  │         (Feature-based, no port dependency)              ││
//   │  └─────────────────────────────────────────────────────────┘│
//   │                                                             │
//   │  ┌─────────────────────────────────────────────────────────┐│
//   │  │              Streaming Parser Core                       ││
//   │  │    - Partial packet reassembly                          ││
//   │  │    - Zero-copy parsing                                  ││
//   │  │    - Backpressure handling                              ││
//   │  └─────────────────────────────────────────────────────────┘│
//   └─────────────────────────────────────────────────────────────┘
//
// 性能特性:
//   - 无锁队列: 使用 lock-free MPMC queue
//   - 独立线程池: parser worker 独立调度
//   - Backpressure: 队列满时优雅降级
//   - 零拷贝: 避免不必要的内存分配
//   - Streaming: 支持部分包解析，无需全包重组
//
// 禁止事项:
//   - 禁止使用 regex 解析
//   - 禁止全包重组 (full packet reassembly)
//   - 禁止巨型 switch-case
//
package l7parser

import (
	"context"
	"errors"
	"time"

	"cloud-flow/pkg/flow"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	// ErrIncompletePacket 数据包不完整，需要更多数据
	ErrIncompletePacket = errors.New("incomplete packet, need more data")

	// ErrUnknownProtocol 无法识别的协议
	ErrUnknownProtocol = errors.New("unknown protocol")

	// ErrParserNotFound 未找到对应协议的解析器
	ErrParserNotFound = errors.New("parser not found")

	// ErrParseFailed 解析失败
	ErrParseFailed = errors.New("parse failed")

	// ErrQueueFull 解析队列已满 (backpressure)
	ErrQueueFull = errors.New("parser queue full")
)

// ============================================================================
// 核心类型定义
// ============================================================================

// ParserType 解析器类型
type ParserType uint8

const (
	ParserTypeUnknown ParserType = iota
	ParserTypeHTTP1
	ParserTypeHTTP2
	ParserTypeGRPC
	ParserTypeMySQL
	ParserTypeRedis
	ParserTypeKafka
	ParserTypeDNS
)

func (t ParserType) String() string {
	switch t {
	case ParserTypeHTTP1:
		return "http1"
	case ParserTypeHTTP2:
		return "http2"
	case ParserTypeGRPC:
		return "grpc"
	case ParserTypeMySQL:
		return "mysql"
	case ParserTypeRedis:
		return "redis"
	case ParserTypeKafka:
		return "kafka"
	case ParserTypeDNS:
		return "dns"
	default:
		return "unknown"
	}
}

// ToFlowProtocol 转换为 flow.Protocol
func (t ParserType) ToFlowProtocol() flow.Protocol {
	switch t {
	case ParserTypeHTTP1:
		return flow.ProtoHTTP
	case ParserTypeHTTP2:
		return flow.ProtoHTTP2
	case ParserTypeGRPC:
		return flow.ProtoGRPC
	case ParserTypeMySQL:
		return flow.ProtoMySQL
	case ParserTypeRedis:
		return flow.ProtoRedis
	case ParserTypeKafka:
		return flow.ProtoKafka
	case ParserTypeDNS:
		return flow.ProtoDNS
	default:
		return flow.ProtoUnknown
	}
}

// PacketDirection 数据包方向
type PacketDirection uint8

const (
	DirUnknown PacketDirection = iota
	DirRequest              // 请求方向 (client -> server)
	DirResponse             // 响应方向 (server -> client)
)

// PacketMetadata 数据包元数据
type PacketMetadata struct {
	// 五元组
	SrcIP   [16]byte // IPv4-mapped IPv6
	DstIP   [16]byte
	SrcPort uint16
	DstPort uint16

	// 协议信息
	L4Proto     uint8 // TCP=6, UDP=17
	L7ProtoHint ParserType // 协议提示 (可能为 Unknown)

	// 时间戳
	TimestampNs int64

	// TCP 序列号 (用于重组)
	SeqNum uint32
	AckNum uint32

	// 方向
	Direction PacketDirection

	// 流标识
	FlowID uint64

	// 原始包序号 (用于排序)
	PacketSeq uint32
}

// ParseResult 解析结果
type ParseResult struct {
	// 协议类型
	ParserType ParserType

	// 解析状态
	IsComplete  bool // 是否完整解析
	IsPartial   bool // 是否为部分解析 (需要后续数据)
	NeedMore    bool // 是否需要更多数据

	// 请求/响应信息
	Direction PacketDirection

	// HTTP/HTTP2/gRPC 通用字段
	Method     uint8  // HTTP 方法 (GET/POST/...)
	StatusCode uint16 // HTTP 状态码
	Path       string // URL 路径

	// gRPC 特有字段
	GRPCService string // gRPC 服务名
	GRPCMethod  string // gRPC 方法名
	GRPCStatus  uint32 // gRPC 状态码

	// 内容
	Request  []byte // 请求体 (可能截断)
	Response []byte // 响应体 (可能截断)

	// 异常信息
	Exception string // 异常描述

	// 性能指标
	LatencyNs uint64 // 延迟 (纳秒)
	ReqSize   uint32 // 请求大小
	RespSize  uint32 // 响应大小

	// 元数据
	Headers     map[string]string // HTTP/gRPC headers
	Trailers    map[string]string // gRPC trailers
	ContentType string

	// 解析上下文 (用于流式解析状态保持)
	Context map[string]interface{}
}

// RawPacket 原始数据包
type RawPacket struct {
	Metadata PacketMetadata
	Data     []byte // 指向原始 buffer，避免拷贝
}

// ParserInput 解析器输入
type ParserInput struct {
	Packet RawPacket
	// 流状态上下文
	StreamState interface{}
}

// Parser 协议解析器接口
// 所有 L7 解析器必须实现此接口
type Parser interface {
	// Type 返回解析器类型
	Type() ParserType

	// Name 返回解析器名称
	Name() string

	// Priority 返回解析优先级 (越高越优先检测)
	Priority() int

	// Detect 协议检测
	// 返回: 是否匹配, 置信度(0-1)
	Detect(data []byte, dstPort uint16) (bool, float64)

	// Parse 解析数据包
	// 支持流式解析，返回部分结果时可继续传入后续数据
	Parse(ctx context.Context, input *ParserInput, state interface{}) (*ParseResult, interface{}, error)

	// ParseStreaming 流式解析 (用于处理分片包)
	// state: 解析状态，首次调用传入 nil
	// 返回: 解析结果，新状态，错误
	ParseStreaming(ctx context.Context, data []byte, state interface{}) (*ParseResult, interface{}, error)

	// Reset 重置解析器状态
	Reset()
}

// StreamingParser 流式解析器接口
// 用于处理 TCP 流，维护跨包状态
type StreamingParser interface {
	Parser

	// InitStream 初始化流解析状态
	InitStream(flowID uint64) interface{}

	// Feed 喂入数据
	// 返回: 解析结果列表，错误
	Feed(ctx context.Context, flowID uint64, data []byte, seq uint32) ([]*ParseResult, error)

	// CloseStream 关闭流
	CloseStream(flowID uint64)
}

// ReassemblyBuffer 重组缓冲区接口
type ReassemblyBuffer interface {
	// Add 添加数据片段
	Add(seq uint32, data []byte, isLast bool) error

	// GetNext 获取下一个连续的数据块
	GetNext() ([]byte, bool)

	// HasGap 检查是否有缺口
	HasGap() bool

	// Clear 清空缓冲区
	Clear()

	// Size 当前缓冲区大小
	Size() int
}

// ============================================================================
// 配置类型
// ============================================================================

// EngineConfig 解析引擎配置
type EngineConfig struct {
	// Worker 配置
	WorkerNum       int           // worker 数量 (默认 CPU 核心数)
	QueueSize       int           // 每个 worker 队列大小
	BatchSize       int           // 批量处理大小
	BatchTimeout    time.Duration // 批量超时

	// 性能配置
	MaxPacketSize   int  // 最大包大小
	MaxReasmSize    int  // 最大重组缓冲区大小
	EnableBackpressure bool // 启用背压

	// 协议配置
	EnabledParsers []ParserType // 启用的解析器

	// 流配置
	StreamTimeout   time.Duration // 流超时
	MaxStreams      int           // 最大并发流数
	CleanupInterval time.Duration // 清理间隔
}

// DefaultConfig 返回默认配置
func DefaultConfig() *EngineConfig {
	return &EngineConfig{
		WorkerNum:          0, // 自动设置为 NumCPU
		QueueSize:          1024,
		BatchSize:          32,
		BatchTimeout:       10 * time.Millisecond,
		MaxPacketSize:      65535,
		MaxReasmSize:       1024 * 1024, // 1MB
		EnableBackpressure: true,
		EnabledParsers: []ParserType{
			ParserTypeHTTP1,
			ParserTypeHTTP2,
			ParserTypeGRPC,
			ParserTypeMySQL,
			ParserTypeRedis,
			ParserTypeKafka,
			ParserTypeDNS,
		},
		StreamTimeout:   30 * time.Second,
		MaxStreams:      100000,
		CleanupInterval: 5 * time.Second,
	}
}

// ============================================================================
// 统计指标
// ============================================================================

// Stats 解析统计
type Stats struct {
	// 解析统计
	PacketsParsed   uint64
	PacketsDropped  uint64
	PacketsReasm    uint64
	StreamsActive   uint64
	StreamsTotal    uint64

	// 协议统计
	ProtocolStats map[ParserType]uint64

	// 错误统计
	ErrorsUnknownProto uint64
	ErrorsParseFail    uint64
	ErrorsReasmFail    uint64
	ErrorsQueueFull    uint64

	// 性能指标
	AvgParseTimeNs uint64
	MaxParseTimeNs uint64
}

// StatsCollector 统计收集器接口
type StatsCollector interface {
	// RecordParse 记录解析
	RecordParse(parserType ParserType, durationNs uint64)

	// RecordError 记录错误
	RecordError(errType string)

	// RecordDrop 记录丢弃
	RecordDrop(reason string)

	// GetStats 获取统计
	GetStats() Stats
}
