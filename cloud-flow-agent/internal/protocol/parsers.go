// Package protocol 内置协议解析插件
package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"
)

// ============================================================================
// Oracle 协议解析插件
// ============================================================================

// OracleParser Oracle TNS 协议解析器
type OracleParser struct {
	BaseParser
}

// NewOracleParser 创建 Oracle 解析器
func NewOracleParser() *OracleParser {
	return &OracleParser{
		BaseParser: *NewBaseParser(PluginInfo{
			Name:        "oracle",
			Version:     "1.0.0",
			Protocol:    "oracle",
			Description: "Oracle TNS 协议解析",
			MinAgentVer: "1.0.0",
			SupportedOps: []string{"query", "connect", "commit", "rollback"},
		}),
	}
}

// Parse 解析 Oracle TNS 数据包
func (p *OracleParser) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	defer p.recordParse(nil)

	if len(data) < 10 {
		return nil, fmt.Errorf("数据包太短")
	}

	result := &ParseResult{
		Protocol:  "oracle",
		IsMatch:   true,
		Fields:    make(map[string]string),
		Tags:      make(map[string]string),
	}

	// TNS 包类型判断
	if len(data) >= 2 {
		pktType := data[0]
		switch pktType {
		case 0x01: // Connect
			result.IsRequest = true
			result.Fields["tns_type"] = "connect"
			if len(data) > 10 {
				result.User = extractOracleString(data[10:])
			}
		case 0x02: // Accept
			result.IsRequest = false
			result.Fields["tns_type"] = "accept"
		case 0x03: // Acknowledge
			result.Fields["tns_type"] = "ack"
		case 0x04: // Refuse
			result.IsRequest = false
			result.Fields["tns_type"] = "refuse"
			if len(data) > 8 {
				result.Error = fmt.Sprintf("refused: code=%d", data[8])
			}
		case 0x06: // Redirect
			result.Fields["tns_type"] = "redirect"
		case 0x11: // Data
			result.IsRequest = true
			result.Fields["tns_type"] = "data"
			result.Query = extractOracleQuery(data)
		case 0x12: // Cursor
			result.Fields["tns_type"] = "cursor"
		default:
			result.IsMatch = false
		}
	}

	return result, nil
}

func extractOracleString(data []byte) string {
	end := len(data)
	if end > 255 {
		end = 255
	}
	return strings.TrimSpace(string(data[:end]))
}

func extractOracleQuery(data []byte) string {
	// 简化：提取可打印字符
	var buf strings.Builder
	for _, b := range data {
		if b >= 32 && b <= 126 {
			buf.WriteByte(b)
		} else if buf.Len() > 0 && b == 0 {
			break
		}
	}
	return buf.String()
}

// ============================================================================
// PostgreSQL 协议解析插件
// ============================================================================

// PostgreSQLParser PostgreSQL 协议解析器
type PostgreSQLParser struct {
	BaseParser
}

// NewPostgreSQLParser 创建 PostgreSQL 解析器
func NewPostgreSQLParser() *PostgreSQLParser {
	return &PostgreSQLParser{
		BaseParser: *NewBaseParser(PluginInfo{
			Name:        "postgresql",
			Version:     "1.0.0",
			Protocol:    "postgresql",
			Description: "PostgreSQL 协议解析",
			MinAgentVer: "1.0.0",
			SupportedOps: []string{"query", "prepare", "bind", "execute", "parse"},
		}),
	}
}

// Parse 解析 PostgreSQL 数据包
func (p *PostgreSQLParser) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	defer p.recordParse(nil)

	if len(data) < 8 {
		return nil, fmt.Errorf("数据包太短")
	}

	result := &ParseResult{
		Protocol: "postgresql",
		IsMatch:  true,
		Fields:   make(map[string]string),
		Tags:     make(map[string]string),
	}

	// PostgreSQL 消息类型
	msgType := data[0]
	switch msgType {
	case 'Q': // SimpleQuery
		result.IsRequest = true
		result.Query = extractPGString(data[5:])
		result.Fields["msg_type"] = "SimpleQuery"
	case 'P': // Parse
		result.IsRequest = true
		if len(data) > 5 {
			result.Query = extractPGString(data[5:])
		}
		result.Fields["msg_type"] = "Parse"
	case 'B': // Bind
		result.IsRequest = true
		result.Fields["msg_type"] = "Bind"
	case 'E': // Execute
		result.IsRequest = true
		result.Fields["msg_type"] = "Execute"
	case 'C': // Close
		result.IsRequest = true
		result.Fields["msg_type"] = "Close"
	case 'T': // RowDescription
		result.IsRequest = false
		result.Fields["msg_type"] = "RowDescription"
	case 'D': // DataRow
		result.IsRequest = false
		result.Fields["msg_type"] = "DataRow"
	case 'Z': // ReadyForQuery
		result.IsRequest = false
		if len(data) > 5 {
			status := string(data[5])
			result.Fields["tx_status"] = status // I=idle, T=in-transaction, E=in-error
		}
		result.Fields["msg_type"] = "ReadyForQuery"
	case '1': // ParseComplete
		result.IsRequest = false
		result.Fields["msg_type"] = "ParseComplete"
	case '2': // BindComplete
		result.IsRequest = false
		result.Fields["msg_type"] = "BindComplete"
	case '3': // CloseComplete
		result.IsRequest = false
		result.Fields["msg_type"] = "CloseComplete"
	case 'n': // NoticeResponse
		result.IsRequest = false
		result.Fields["msg_type"] = "NoticeResponse"
	case 'E': // ErrorResponse
		result.IsRequest = false
		result.Fields["msg_type"] = "ErrorResponse"
		if len(data) > 5 {
			result.Error = extractPGString(data[5:])
		}
	case 'S': // ParameterStatus
		result.IsRequest = false
		result.Fields["msg_type"] = "ParameterStatus"
		if len(data) > 5 {
			parts := strings.SplitN(extractPGString(data[5:]), "\x00", 2)
			if len(parts) == 2 {
				if parts[0] == "application_name" {
					result.User = parts[1]
				}
				if parts[0] == "database" {
					result.Database = parts[1]
				}
			}
		}
	case 'K': // BackendKeyData
		result.IsRequest = false
		result.Fields["msg_type"] = "BackendKeyData"
	default:
		// Startup 消息（无类型前缀）
		if len(data) >= 8 && data[4] == 0x00 && data[5] == 0x03 {
			result.IsRequest = true
			result.Fields["msg_type"] = "Startup"
			// 提取参数
			params := extractPGStartupParams(data[8:])
			if db, ok := params["database"]; ok {
				result.Database = db
			}
			if user, ok := params["user"]; ok {
				result.User = user
			}
		} else {
			result.IsMatch = false
		}
	}

	return result, nil
}

func extractPGString(data []byte) string {
	end := len(data)
	if end > 10000 {
		end = 10000
	}
	return strings.TrimSpace(strings.ReplaceAll(string(data[:end]), "\x00", " "))
}

func extractPGStartupParams(data []byte) map[string]string {
	params := make(map[string]string)
	parts := strings.Split(string(data), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		params[parts[i]] = parts[i+1]
	}
	return params
}

// ============================================================================
// Redis 协议解析插件
// ============================================================================

// RedisParser Redis RESP 协议解析器
type RedisParser struct {
	BaseParser
}

// NewRedisParser 创建 Redis 解析器
func NewRedisParser() *RedisParser {
	return &RedisParser{
		BaseParser: *NewBaseParser(PluginInfo{
			Name:        "redis",
			Version:     "1.0.0",
			Protocol:    "redis",
			Description: "Redis RESP 协议解析",
			MinAgentVer: "1.0.0",
			SupportedOps: []string{"get", "set", "del", "hget", "hset", "lpush", "rpush", "publish", "subscribe"},
		}),
	}
}

// Parse 解析 Redis RESP 数据包
func (p *RedisParser) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	defer p.recordParse(nil)

	if len(data) < 1 {
		return nil, fmt.Errorf("数据包太短")
	}

	result := &ParseResult{
		Protocol: "redis",
		IsMatch:  true,
		Fields:   make(map[string]string),
		Tags:     make(map[string]string),
	}

	// RESP 类型判断
	first := data[0]
	switch first {
	case '+': // Simple String
		result.IsRequest = false
		result.Fields["resp_type"] = "simple_string"
		result.Status = extractRESPLine(data[1:])
	case '-': // Error
		result.IsRequest = false
		result.Fields["resp_type"] = "error"
		result.Error = extractRESPLine(data[1:])
	case ':': // Integer
		result.IsRequest = false
		result.Fields["resp_type"] = "integer"
	case '$': // Bulk String
		result.IsRequest = metadata.Direction == "egress"
		result.Fields["resp_type"] = "bulk_string"
		result.Query = extractRESPCommand(data)
	case '*': // Array
		result.IsRequest = metadata.Direction == "egress"
		result.Fields["resp_type"] = "array"
		result.Query = extractRESPCommand(data)
	default:
		// 内联命令
		result.IsRequest = true
		result.Query = extractRESPLine(data)
		result.Fields["resp_type"] = "inline"
	}

	return result, nil
}

func extractRESPLine(data []byte) string {
	end := len(data)
	for i := 0; i < end; i++ {
		if data[i] == '\r' || data[i] == '\n' {
			end = i
			break
		}
	}
	return strings.TrimSpace(string(data[:end]))
}

func extractRESPCommand(data []byte) string {
	// 提取第一个参数作为命令
	cmd := ""
	inBulk := false
	bulkLen := 0
	bulkIdx := 0

	for i := 0; i < len(data) && i < 500; i++ {
		if data[i] == '\r' || data[i] == '\n' {
			continue
		}

		if inBulk {
			bulkIdx++
			if bulkIdx >= bulkLen {
				break
			}
			continue
		}

		if data[i] == '$' || data[i] == '*' {
			// 读取长度
			numStart := i + 1
			numEnd := numStart
			for numEnd < len(data) && data[numEnd] >= '0' && data[numEnd] <= '9' {
				numEnd++
			}
			if numEnd > numStart {
				fmt.Sscanf(string(data[numStart:numEnd]), "%d", &bulkLen)
				inBulk = true
				bulkIdx = 0
				i = numEnd
			}
			continue
		}

		if (data[i] >= 'A' && data[i] <= 'Z') || (data[i] >= 'a' && data[i] <= 'z') {
			cmd += string(data[i])
		} else if cmd != "" && data[i] == ' ' {
			break
		}
	}

	return strings.ToUpper(cmd)
}

// ============================================================================
// Kafka 协议解析插件
// ============================================================================

// KafkaParser Kafka 协议解析器
type KafkaParser struct {
	BaseParser
}

// NewKafkaParser 创建 Kafka 解析器
func NewKafkaParser() *KafkaParser {
	return &KafkaParser{
		BaseParser: *NewBaseParser(PluginInfo{
			Name:        "kafka",
			Version:     "1.0.0",
			Protocol:    "kafka",
			Description: "Kafka 协议解析",
			MinAgentVer: "1.0.0",
			SupportedOps: []string{"produce", "fetch", "metadata", "offset_commit"},
		}),
	}
}

// Parse 解析 Kafka 数据包
func (p *KafkaParser) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	defer p.recordParse(nil)

	if len(data) < 14 {
		return nil, fmt.Errorf("数据包太短")
	}

	result := &ParseResult{
		Protocol: "kafka",
		IsMatch:  true,
		Fields:   make(map[string]string),
		Tags:     make(map[string]string),
	}

	// Kafka 请求头: apiKey(2) + apiVersion(2) + correlationId(4) + clientIdLen(2) + ...
	apiKey := int(binary.BigEndian.Uint16(data[0:2]))
	apiVersion := int(binary.BigEndian.Uint16(data[2:4]))
	correlationId := binary.BigEndian.Uint32(data[4:8])

	result.Fields["api_key"] = fmt.Sprintf("%d", apiKey)
	result.Fields["api_version"] = fmt.Sprintf("%d", apiVersion)
	result.Fields["correlation_id"] = fmt.Sprintf("%d", correlationId)

	// 请求方向判断
	result.IsRequest = metadata.Direction == "egress"

	// API Key 解析
	switch apiKey {
	case 0: // Produce
		result.Fields["op"] = "produce"
		result.Query = parseKafkaProduce(data)
	case 1: // Fetch
		result.Fields["op"] = "fetch"
	case 3: // Metadata
		result.Fields["op"] = "metadata"
	case 8: // OffsetCommit
		result.Fields["op"] = "offset_commit"
	case 18: // ApiVersions
		result.Fields["op"] = "api_versions"
	default:
		result.Fields["op"] = fmt.Sprintf("unknown(%d)", apiKey)
	}

	return result, nil
}

func parseKafkaProduce(data []byte) string {
	// 简化：提取 Topic 名称
	if len(data) < 20 {
		return ""
	}

	// 跳过请求头
	offset := 12
	clientIdLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2 + clientIdLen

	// Required Acks
	if offset+2 > len(data) {
		return ""
	}
	offset += 2 // acks

	// Transactional ID
	if offset+4 > len(data) {
		return ""
	}
	txnIdLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2 + txnIdLen

	// Num Records
	if offset+4 > len(data) {
		return ""
	}
	numRecords := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	_ = fmt.Sprintf("%d", numRecords) // num_records 信息可用于日志
	offset += 4

	// Topic 数据
	if offset+2 > len(data) {
		return ""
	}
	topicCount := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4

	if topicCount > 0 && offset+2 <= len(data) {
		topicLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+topicLen <= len(data) {
			return string(data[offset : offset+topicLen])
		}
	}

	return ""
}

// ============================================================================
// Dubbo 协议解析插件
// ============================================================================

// DubboParser Apache Dubbo 协议解析器
type DubboParser struct {
	BaseParser
}

// NewDubboParser 创建 Dubbo 解析器
func NewDubboParser() *DubboParser {
	return &DubboParser{
		BaseParser: *NewBaseParser(PluginInfo{
			Name:        "dubbo",
			Version:     "1.0.0",
			Protocol:    "dubbo",
			Description: "Apache Dubbo 协议解析",
			MinAgentVer: "1.0.0",
			SupportedOps: []string{"invoke", "heartbeat"},
		}),
	}
}

// Parse 解析 Dubbo 数据包
func (p *DubboParser) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	defer p.recordParse(nil)

	if len(data) < 16 {
		return nil, fmt.Errorf("数据包太短")
	}

	result := &ParseResult{
		Protocol: "dubbo",
		IsMatch:  true,
		Fields:   make(map[string]string),
		Tags:     make(map[string]string),
	}

	// Dubbo Magic Header: 0xdabb
	if data[0] != 0xda || data[1] != 0xbb {
		result.IsMatch = false
		return result, nil
	}

	// 解析 Dubbo 头
	serializationID := data[2] & 0x1f
	responseFlag := (data[2] & 0x20) >> 5
	requestID := binary.BigEndian.Uint32(data[4:8])
	dataLen := binary.BigEndian.Uint32(data[12:16])

	result.Fields["serialization"] = fmt.Sprintf("%d", serializationID)
	result.Fields["request_id"] = fmt.Sprintf("%d", requestID)
	result.Fields["data_length"] = fmt.Sprintf("%d", dataLen)

	// 请求/响应判断
	if responseFlag == 0 {
		result.IsRequest = true
		result.Fields["msg_type"] = "request"
	} else {
		result.IsRequest = false
		result.Fields["msg_type"] = "response"
	}

	// 解析 RPC 信息（body 部分）
	if len(data) > 16 && int(dataLen) > 0 {
		body := data[16:]
		if len(body) > int(dataLen) {
			body = body[:dataLen]
		}

		// Dubbo 版本 + 接口名 + 方法名 + 参数类型 + 参数值
		parts := strings.Split(string(body), "\x00")
		if len(parts) >= 3 {
			result.Fields["dubbo_version"] = parts[0]
			result.Fields["service"] = parts[1]
			result.Fields["method"] = parts[2]
			result.Query = fmt.Sprintf("%s.%s", parts[1], parts[2])

			if len(parts) >= 4 {
				result.Database = parts[1] // service name as database
			}
		}
	}

	return result, nil
}

// ============================================================================
// 注册所有内置插件
// ============================================================================

// RegisterBuiltinPlugins 注册所有内置插件到注册中心
func RegisterBuiltinPlugins() {
	registry := GetRegistry()

	registry.Register("oracle", func() Plugin { return NewOracleParser() })
	registry.Register("postgresql", func() Plugin { return NewPostgreSQLParser() })
	registry.Register("redis", func() Plugin { return NewRedisParser() })
	registry.Register("kafka", func() Plugin { return NewKafkaParser() })
	registry.Register("dubbo", func() Plugin { return NewDubboParser() })
}
