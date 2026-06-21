//go:build linux

// Package pipeline 提供统一的数据流转管道
// - 数据预处理层（过滤、采样、聚合）
// - 统一序列化（protobuf 优先）
// - 单一数据流转路径
// - 职责分离：Edge 接收 → 预处理 → 存储
package pipeline

import (
	"fmt"
	"time"
)

// ============================================================================
// 一、数据记录模型（统一数据流）
// ============================================================================

// DataType 数据类型
type DataType string

const (
	DataTypeFlow      DataType = "flow"
	DataTypeMetric    DataType = "metric"
	DataTypeTrace     DataType = "trace"
	DataTypeLog       DataType = "log"
	DataTypeEvent     DataType = "event"
	DataTypeProfile   DataType = "profile"
)

// DataRecord 统一数据记录
type DataRecord struct {
	ID         string    `json:"id"`
	Type       DataType  `json:"type"`
	Timestamp  int64     `json:"timestamp"`
	ProbeID    string    `json:"probe_id"`
	TenantID   string    `json:"tenant_id"`
	
	// 原始数据（protobuf 或 JSON 编码）
	Payload    []byte    `json:"-"` // 序列化后的原始数据
	
	// 解析后的元数据（用于路由和预处理）
	Meta       *DataMeta `json:"meta,omitempty"`
	
	// 处理状态
	Processed  bool      `json:"processed"`
	Dropped    bool      `json:"dropped"`
	DropReason string    `json:"drop_reason,omitempty"`
	
	// 来源追踪
	Source     string    `json:"source"`   // agent/edge/api
	HopCount   int       `json:"hop_count"` // 经过的节点数
}

// DataMeta 数据元数据（轻量级，用于路由）
type DataMeta struct {
	SrcIP       string            `json:"src_ip,omitempty"`
	DstIP       string            `json:"dst_ip,omitempty"`
	SrcPort     uint16            `json:"src_port,omitempty"`
	DstPort     uint16            `json:"dst_port,omitempty"`
	Protocol    uint8             `json:"protocol,omitempty"`
	L7Protocol  uint8             `json:"l7_protocol,omitempty"`
	StatusCode  uint16            `json:"status_code,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Service     string            `json:"service,omitempty"`
	Pod         string            `json:"pod,omitempty"`
	Node        string            `json:"node,omitempty"`
	TraceID     string            `json:"trace_id,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

// Size 返回数据记录大小（字节）
func (dr *DataRecord) Size() int {
	return len(dr.Payload) + 256 // payload + 元数据估算
}

// String 返回摘要
func (dr *DataRecord) String() string {
	return fmt.Sprintf("DataRecord{type=%s, probe=%s, tenant=%s, size=%d}",
		dr.Type, dr.ProbeID, dr.TenantID, dr.Size())
}

// ============================================================================
// 二、数据批次（减少序列化次数）
// ============================================================================

// DataBatch 数据批次
type DataBatch struct {
	ID        string        `json:"id"`
	Type      DataType      `json:"type"`
	Records   []*DataRecord `json:"records"`
	CreatedAt time.Time     `json:"created_at"`
	
	// 聚合元数据
	RecordCount int   `json:"record_count"`
	TotalSize   int   `json:"total_size"`
	MinTime     int64 `json:"min_time"`
	MaxTime     int64 `json:"max_time"`
}

// NewDataBatch 创建数据批次
func NewDataBatch(dataType DataType) *DataBatch {
	now := time.Now()
	return &DataBatch{
		ID:        fmt.Sprintf("batch-%s-%d", dataType, now.UnixNano()),
		Type:      dataType,
		Records:   make([]*DataRecord, 0),
		CreatedAt: now,
		MinTime:   now.UnixNano(),
		MaxTime:   now.UnixNano(),
	}
}

// Add 添加记录
func (b *DataBatch) Add(record *DataRecord) {
	b.Records = append(b.Records, record)
	b.RecordCount++
	b.TotalSize += record.Size()
	if record.Timestamp < b.MinTime {
		b.MinTime = record.Timestamp
	}
	if record.Timestamp > b.MaxTime {
		b.MaxTime = record.Timestamp
	}
}

// IsEmpty 是否为空
func (b *DataBatch) IsEmpty() bool {
	return len(b.Records) == 0
}

// DroppedCount 被丢弃的记录数
func (b *DataBatch) DroppedCount() int {
	count := 0
	for _, r := range b.Records {
		if r.Dropped {
			count++
		}
	}
	return count
}

// ValidRecords 获取有效记录（未被丢弃）
func (b *DataBatch) ValidRecords() []*DataRecord {
	var result []*DataRecord
	for _, r := range b.Records {
		if !r.Dropped {
			result = append(result, r)
		}
	}
	return result
}

// ============================================================================
// 三、序列化格式枚举
// ============================================================================

// SerializationFormat 序列化格式
type SerializationFormat string

const (
	FormatProtobuf SerializationFormat = "protobuf" // 推荐：二进制，高性能
	FormatJSON     SerializationFormat = "json"     // 兼容：可读性好
	FormatMsgPack  SerializationFormat = "msgpack"  // 折中：比 JSON 小
)

// Serializer 序列化器接口
type Serializer interface {
	Serialize(record *DataRecord) ([]byte, error)
	Deserialize(data []byte, record *DataRecord) error
	Format() SerializationFormat
}

// ============================================================================
// 四、数据管道配置
// ============================================================================

// PipelineConfig 管道配置
type PipelineConfig struct {
	// 批次配置
	BatchSize     int           `json:"batch_size"`      // 批次大小
	BatchTimeout  time.Duration `json:"batch_timeout"`   // 批次超时
	MaxBatchSize  int           `json:"max_batch_size"`  // 最大批次大小（字节）
	
	// 预处理配置
	EnableFilter   bool   `json:"enable_filter"`   // 启用过滤
	EnableSample   bool   `json:"enable_sample"`   // 启用采样
	EnableEnrich   bool   `json:"enable_enrich"`   // 启用富化
	SampleRate     float64 `json:"sample_rate"`    // 采样率
	
	// 序列化配置
	Format SerializationFormat `json:"format"` // 序列化格式
	
	// 缓冲配置
	BufferSize     int `json:"buffer_size"`     // 输入缓冲区大小
	OutputBuffer   int `json:"output_buffer"`   // 输出缓冲区大小
	
	// 并发配置
	WorkerCount    int `json:"worker_count"`    // 工作线程数
}

// DefaultPipelineConfig 默认配置
func DefaultPipelineConfig() *PipelineConfig {
	return &PipelineConfig{
		BatchSize:     1000,
		BatchTimeout:  1 * time.Second,
		MaxBatchSize:  10 * 1024 * 1024, // 10MB
		EnableFilter:  true,
		EnableSample:  true,
		EnableEnrich:  true,
		SampleRate:    1.0,
		Format:        FormatProtobuf,
		BufferSize:    10000,
		OutputBuffer:  10000,
		WorkerCount:   4,
	}
}

// Validate 验证配置
func (cfg *PipelineConfig) Validate() error {
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be > 0")
	}
	if cfg.BatchTimeout <= 0 {
		return fmt.Errorf("batch_timeout must be > 0")
	}
	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return fmt.Errorf("sample_rate must be in [0, 1]")
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	return nil
}
