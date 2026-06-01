// Package parsers Kafka 协议解析器
//
// Kafka 协议特点:
//   - 请求/响应协议
//   - 基于 TCP
//   - 消息格式: [length(4)] [api_key(2)] [api_version(2)] [correlation_id(4)] [payload]
//   - 支持多个 API versions
//
// 实现要点:
//   - 支持常见 API (Produce, Fetch, Metadata, etc.)
//   - 版本协商处理
//   - 流式解析支持

package parsers

import (
	"context"
	"encoding/binary"
	"errors"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidKafka     = errors.New("invalid kafka message")
	ErrIncompleteKafka  = errors.New("incomplete kafka message")
)

// Kafka API Keys
const (
	APIProduce         int16 = 0
	APIFetch           int16 = 1
	APIListOffsets     int16 = 2
	APIMetadata        int16 = 3
	APILeaderAndISR    int16 = 4
	APIStopReplica     int16 = 5
	APIUpdateMetadata  int16 = 6
	APIControlledShutdown int16 = 7
	APIOffsetCommit    int16 = 8
	APIOffsetFetch     int16 = 9
	APIFindCoordinator int16 = 10
	APIJoinGroup       int16 = 11
	APIHeartbeat       int16 = 12
	APILeaveGroup      int16 = 13
	APISyncGroup       int16 = 14
	APIDescribeGroups  int16 = 15
	APIListGroups      int16 = 16
	APISaslHandshake   int16 = 17
	APIApiVersions     int16 = 18
	APICreateTopics    int16 = 19
	APIDeleteTopics    int16 = 20
	APIDeleteRecords   int16 = 21
	APIInitProducerId  int16 = 22
	APIOffsetForLeaderEpoch int16 = 23
	APIAddPartitionsToTxn int16 = 24
	APIAddOffsetsToTxn int16 = 25
	APIEndTxn          int16 = 26
	APIWriteTxnMarkers int16 = 27
	APITxnOffsetCommit int16 = 28
	APIDescribeAcls    int16 = 29
	APICreateAcls      int16 = 30
	APIDeleteAcls      int16 = 31
	APIDescribeConfigs int16 = 32
	APIAlterConfigs    int16 = 33
	APIAlterReplicaLogDirs int16 = 34
	APIDescribeLogDirs int16 = 35
	APISaslAuthenticate int16 = 36
	APICreatePartitions int16 = 37
	APICreateDelegationToken int16 = 38
	APIRenewDelegationToken int16 = 39
	APIExpireDelegationToken int16 = 40
	APIDescribeDelegationToken int16 = 41
	APIDeleteGroups    int16 = 42
	APIElectLeaders    int16 = 43
	APIIncrementalAlterConfigs int16 = 44
	APIAlterPartitionReassignments int16 = 45
	APIListPartitionReassignments int16 = 46
	APIOffsetDelete    int16 = 47
	APIDescribeClientQuotas int16 = 48
	APIAlterClientQuotas int16 = 49
	APIDescribeUserScramCredentials int16 = 50
	APIAlterUserScramCredentials int16 = 51
	APIDescribeQuorum  int16 = 55
	APIAlterPartition  int16 = 56
	APIUpdateFeatures  int16 = 57
	APIDescribeCluster int16 = 60
	APIDescribeProducers int16 = 61
	APIUnregisterBroker int16 = 64
	APIDescribeTransactions int16 = 65
	APIListTransactions int16 = 66
	APIAllocateProducerIds int16 = 67
)

// APIKeyNames API 名称映射
var APIKeyNames = map[int16]string{
	APIProduce:         "Produce",
	APIFetch:           "Fetch",
	APIListOffsets:     "ListOffsets",
	APIMetadata:        "Metadata",
	APILeaderAndISR:    "LeaderAndISR",
	APIStopReplica:     "StopReplica",
	APIUpdateMetadata:  "UpdateMetadata",
	APIControlledShutdown: "ControlledShutdown",
	APIOffsetCommit:    "OffsetCommit",
	APIOffsetFetch:     "OffsetFetch",
	APIFindCoordinator: "FindCoordinator",
	APIJoinGroup:       "JoinGroup",
	APIHeartbeat:       "Heartbeat",
	APILeaveGroup:      "LeaveGroup",
	APISyncGroup:       "SyncGroup",
	APIDescribeGroups:  "DescribeGroups",
	APIListGroups:      "ListGroups",
	APISaslHandshake:   "SaslHandshake",
	APIApiVersions:     "ApiVersions",
	APICreateTopics:    "CreateTopics",
	APIDeleteTopics:    "DeleteTopics",
	APIDeleteRecords:   "DeleteRecords",
	APIInitProducerId:  "InitProducerId",
	APIOffsetForLeaderEpoch: "OffsetForLeaderEpoch",
	APIAddPartitionsToTxn: "AddPartitionsToTxn",
	APIAddOffsetsToTxn: "AddOffsetsToTxn",
	APIEndTxn:          "EndTxn",
	APIWriteTxnMarkers: "WriteTxnMarkers",
	APITxnOffsetCommit: "TxnOffsetCommit",
	APIDescribeAcls:    "DescribeAcls",
	APICreateAcls:      "CreateAcls",
	APIDeleteAcls:      "DeleteAcls",
	APIDescribeConfigs: "DescribeConfigs",
	APIAlterConfigs:    "AlterConfigs",
	APIAlterReplicaLogDirs: "AlterReplicaLogDirs",
	APIDescribeLogDirs: "DescribeLogDirs",
	APISaslAuthenticate: "SaslAuthenticate",
	APICreatePartitions: "CreatePartitions",
	APICreateDelegationToken: "CreateDelegationToken",
	APIRenewDelegationToken: "RenewDelegationToken",
	APIExpireDelegationToken: "ExpireDelegationToken",
	APIDescribeDelegationToken: "DescribeDelegationToken",
	APIDeleteGroups:    "DeleteGroups",
	APIElectLeaders:    "ElectLeaders",
	APIIncrementalAlterConfigs: "IncrementalAlterConfigs",
	APIAlterPartitionReassignments: "AlterPartitionReassignments",
	APIListPartitionReassignments: "ListPartitionReassignments",
	APIOffsetDelete:    "OffsetDelete",
	APIDescribeClientQuotas: "DescribeClientQuotas",
	APIAlterClientQuotas: "AlterClientQuotas",
	APIDescribeUserScramCredentials: "DescribeUserScramCredentials",
	APIAlterUserScramCredentials: "AlterUserScramCredentials",
	APIDescribeQuorum:  "DescribeQuorum",
	APIAlterPartition:  "AlterPartition",
	APIUpdateFeatures:  "UpdateFeatures",
	APIDescribeCluster: "DescribeCluster",
	APIDescribeProducers: "DescribeProducers",
	APIUnregisterBroker: "UnregisterBroker",
	APIDescribeTransactions: "DescribeTransactions",
	APIListTransactions: "ListTransactions",
	APIAllocateProducerIds: "AllocateProducerIds",
}

// GetAPIKeyName 获取 API 名称
func GetAPIKeyName(key int16) string {
	if name, ok := APIKeyNames[key]; ok {
		return name
	}
	return "Unknown"
}

// KafkaRequestHeader Kafka 请求头
type KafkaRequestHeader struct {
	Length        int32
	APIKey        int16
	APIVersion    int16
	CorrelationID int32
	ClientID      string
}

// KafkaResponseHeader Kafka 响应头
type KafkaResponseHeader struct {
	Length        int32
	CorrelationID int32
}

// KafkaParser Kafka 协议解析器
type KafkaParser struct {
	// 解析状态
	state *kafkaParseState
}

// kafkaParseState 解析状态
type kafkaParseState struct {
	buffer []byte
	offset int
}

// NewKafkaParser 创建 Kafka 解析器
func NewKafkaParser() *KafkaParser {
	return &KafkaParser{
		state: &kafkaParseState{
			buffer: make([]byte, 0, 65536),
		},
	}
}

// Type 返回解析器类型
func (p *KafkaParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeKafka
}

// Name 返回解析器名称
func (p *KafkaParser) Name() string {
	return "kafka"
}

// Priority 返回解析优先级
func (p *KafkaParser) Priority() int {
	return 60
}

// Detect 协议检测
func (p *KafkaParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 8 {
		return false, 0
	}

	// 检查长度字段 (前 4 字节)
	length := binary.BigEndian.Uint32(data[0:4])
	if length < 4 || length > 100*1024*1024 { // 最大 100MB
		return false, 0
	}

	// 检查 API key (接下来 2 字节)
	apiKey := int16(binary.BigEndian.Uint16(data[4:6]))
	if apiKey < 0 || apiKey > 100 {
		return false, 0
	}

	// 检查 API version (接下来 2 字节)
	apiVersion := int16(binary.BigEndian.Uint16(data[6:8]))
	if apiVersion < 0 || apiVersion > 15 {
		return false, 0
	}

	// 检查端口
	if dstPort == 9092 || dstPort == 9093 || dstPort == 9094 {
		return true, 0.95
	}

	return true, 0.80
}

// Parse 解析数据包
func (p *KafkaParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建解析状态
	parseState, ok := state.(*kafkaParseState)
	if !ok {
		parseState = &kafkaParseState{
			buffer: make([]byte, 0, 65536),
		}
	}

	// 添加数据到缓冲区
	parseState.buffer = append(parseState.buffer, data...)

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeKafka,
		Headers:    make(map[string]string),
	}

	// 解析消息
	for len(parseState.buffer) >= 8 {
		header, err := p.parseRequestHeader(parseState.buffer)
		if err != nil {
			if err == ErrIncompleteKafka {
				result.IsPartial = true
				result.NeedMore = true
				return result, parseState, nil
			}
			return nil, parseState, err
		}

		if header == nil {
			break
		}

		// 填充结果
		result.Direction = l7parser.DirRequest
		result.Headers["api_key"] = GetAPIKeyName(header.APIKey)
		result.Headers["api_version"] = string(rune('0' + header.APIVersion))
		result.Headers["correlation_id"] = string(rune(header.CorrelationID))
		if header.ClientID != "" {
			result.Headers["client_id"] = header.ClientID
		}

		// 消费数据
		msgSize := int(4 + header.Length)
		if msgSize > len(parseState.buffer) {
			result.IsPartial = true
			result.NeedMore = true
			return result, parseState, nil
		}

		parseState.buffer = parseState.buffer[msgSize:]
		result.ReqSize = uint32(msgSize)
		result.IsComplete = true

		// 继续解析下一条消息
		if len(parseState.buffer) < 8 {
			break
		}
	}

	return result, parseState, nil
}

// parseRequestHeader 解析请求头
func (p *KafkaParser) parseRequestHeader(data []byte) (*KafkaRequestHeader, error) {
	if len(data) < 12 {
		return nil, ErrIncompleteKafka
	}

	length := int32(binary.BigEndian.Uint32(data[0:4]))
	if length < 4 || length > 100*1024*1024 {
		return nil, ErrInvalidKafka
	}

	// 检查数据是否完整
	if len(data) < int(4+length) {
		return nil, ErrIncompleteKafka
	}

	header := &KafkaRequestHeader{
		Length:        length,
		APIKey:        int16(binary.BigEndian.Uint16(data[4:6])),
		APIVersion:    int16(binary.BigEndian.Uint16(data[6:8])),
		CorrelationID: int32(binary.BigEndian.Uint32(data[8:12])),
	}

	// 解析 ClientID (如果存在)
	offset := 12
	if header.APIVersion >= 1 && offset+2 <= len(data) {
		clientIDLen := int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if clientIDLen > 0 && offset+int(clientIDLen) <= len(data) {
			header.ClientID = string(data[offset : offset+int(clientIDLen)])
		}
	}

	return header, nil
}

// parseResponseHeader 解析响应头
func (p *KafkaParser) parseResponseHeader(data []byte) (*KafkaResponseHeader, error) {
	if len(data) < 8 {
		return nil, ErrIncompleteKafka
	}

	length := int32(binary.BigEndian.Uint32(data[0:4]))
	if length < 4 || length > 100*1024*1024 {
		return nil, ErrInvalidKafka
	}

	// 检查数据是否完整
	if len(data) < int(4+length) {
		return nil, ErrIncompleteKafka
	}

	header := &KafkaResponseHeader{
		Length:        length,
		CorrelationID: int32(binary.BigEndian.Uint32(data[4:8])),
	}

	return header, nil
}

// ParseStreaming 流式解析
func (p *KafkaParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *KafkaParser) Reset() {
	p.state = &kafkaParseState{
		buffer: make([]byte, 0, 65536),
	}
}

// ============================================================================
// API 分类
// ============================================================================

// IsProducerAPI 检查是否为生产者 API
func IsProducerAPI(apiKey int16) bool {
	return apiKey == APIProduce
}

// IsConsumerAPI 检查是否为消费者 API
func IsConsumerAPI(apiKey int16) bool {
	return apiKey == APIFetch
}

// IsAdminAPI 检查是否为管理 API
func IsAdminAPI(apiKey int16) bool {
	adminAPIs := map[int16]bool{
		APICreateTopics:    true,
		APIDeleteTopics:    true,
		APICreatePartitions: true,
		APIDescribeConfigs: true,
		APIAlterConfigs:    true,
		APIElectLeaders:    true,
		APIAlterPartitionReassignments: true,
		APIListPartitionReassignments: true,
	}
	return adminAPIs[apiKey]
}

// IsGroupAPI 检查是否为 Consumer Group API
func IsGroupAPI(apiKey int16) bool {
	groupAPIs := map[int16]bool{
		APIFindCoordinator: true,
		APIJoinGroup:       true,
		APIHeartbeat:       true,
		APILeaveGroup:      true,
		APISyncGroup:       true,
		APIDescribeGroups:  true,
		APIListGroups:      true,
		APIDeleteGroups:    true,
	}
	return groupAPIs[apiKey]
}

// GetAPIType 获取 API 类型
func GetAPIType(apiKey int16) string {
	if IsProducerAPI(apiKey) {
		return "producer"
	}
	if IsConsumerAPI(apiKey) {
		return "consumer"
	}
	if IsAdminAPI(apiKey) {
		return "admin"
	}
	if IsGroupAPI(apiKey) {
		return "group"
	}
	return "other"
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("kafka", func() l7parser.Parser {
		return NewKafkaParser()
	})
}
