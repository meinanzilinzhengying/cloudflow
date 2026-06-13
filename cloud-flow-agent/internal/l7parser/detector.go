// Package l7parser 协议自动检测
//
// 基于特征匹配的协议检测，不依赖端口号
// 支持多种启发式规则提高检测准确率

package l7parser

import (
	"bytes"
	"encoding/binary"
)

// ProtocolDetector 协议检测器
type ProtocolDetector struct {
	// 检测规则列表 (按优先级排序)
	rules []DetectionRule
}

// DetectionRule 检测规则
type DetectionRule struct {
	Name        string
	ParserType  ParserType
	Priority    int
	MatchFunc   func(data []byte, dstPort uint16) (bool, float64)
}

// NewProtocolDetector 创建协议检测器
func NewProtocolDetector() *ProtocolDetector {
	d := &ProtocolDetector{}
	d.registerBuiltinRules()
	return d
}

// Detect 检测协议
func (d *ProtocolDetector) Detect(data []byte, dstPort uint16) (ParserType, float64) {
	var bestType ParserType
	var bestScore float64

	for _, rule := range d.rules {
		matched, score := rule.MatchFunc(data, dstPort)
		if matched && score > bestScore {
			bestType = rule.ParserType
			bestScore = score
		}
	}

	return bestType, bestScore
}

// AddRule 添加检测规则
func (d *ProtocolDetector) AddRule(rule DetectionRule) {
	// 按优先级插入
	insertIdx := len(d.rules)
	for i, r := range d.rules {
		if rule.Priority > r.Priority {
			insertIdx = i
			break
		}
	}

	// 插入到正确位置
	d.rules = append(d.rules, DetectionRule{})
	copy(d.rules[insertIdx+1:], d.rules[insertIdx:])
	d.rules[insertIdx] = rule
}

// registerBuiltinRules 注册内置检测规则
func (d *ProtocolDetector) registerBuiltinRules() {
	// HTTP/1.x 检测 (高优先级)
	d.AddRule(DetectionRule{
		Name:       "http1",
		ParserType: ParserTypeHTTP1,
		Priority:   100,
		MatchFunc:  detectHTTP1,
	})

	// HTTP/2 检测 (高优先级)
	d.AddRule(DetectionRule{
		Name:       "http2",
		ParserType: ParserTypeHTTP2,
		Priority:   110,
		MatchFunc:  detectHTTP2,
	})

	// gRPC 检测 (最高优先级，基于 HTTP/2)
	d.AddRule(DetectionRule{
		Name:       "grpc",
		ParserType: ParserTypeGRPC,
		Priority:   120,
		MatchFunc:  detectGRPC,
	})

	// MySQL 检测
	d.AddRule(DetectionRule{
		Name:       "mysql",
		ParserType: ParserTypeMySQL,
		Priority:   80,
		MatchFunc:  detectMySQL,
	})

	// Redis 检测
	d.AddRule(DetectionRule{
		Name:       "redis",
		ParserType: ParserTypeRedis,
		Priority:   70,
		MatchFunc:  detectRedis,
	})

	// Kafka 检测
	d.AddRule(DetectionRule{
		Name:       "kafka",
		ParserType: ParserTypeKafka,
		Priority:   60,
		MatchFunc:  detectKafka,
	})

	// DNS 检测
	d.AddRule(DetectionRule{
		Name:       "dns",
		ParserType: ParserTypeDNS,
		Priority:   50,
		MatchFunc:  detectDNS,
	})
}

// ============================================================================
// 具体检测函数
// ============================================================================

// detectHTTP1 检测 HTTP/1.x
func detectHTTP1(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 4 {
		return false, 0
	}

	// HTTP 方法
	methods := [][]byte{
		[]byte("GET "),
		[]byte("POST"),
		[]byte("PUT "),
		[]byte("DELE"),
		[]byte("HEAD"),
		[]byte("OPTI"),
		[]byte("PATC"),
		[]byte("CONN"),
		[]byte("TRAC"),
	}

	for _, method := range methods {
		if bytes.HasPrefix(data, method) {
			if dstPort == 80 || dstPort == 8080 || dstPort == 8000 {
				return true, 0.95
			}
			return true, 0.85
		}
	}

	// HTTP 响应
	if bytes.HasPrefix(data, []byte("HTTP")) {
		if dstPort == 80 || dstPort == 8080 || dstPort == 8000 {
			return true, 0.95
		}
		return true, 0.85
	}

	return false, 0
}

// detectHTTP2 检测 HTTP/2
func detectHTTP2(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 9 {
		return false, 0
	}

	// HTTP/2 魔术字
	http2Magic := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")
	if len(data) >= len(http2Magic) && bytes.HasPrefix(data, http2Magic) {
		return true, 1.0
	}

	// HTTP/2 Frame 格式检查
	length := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	frameType := data[3]

	if length < 1 || length > 16384 {
		return false, 0
	}

	if frameType > 0x09 { // 最大 frame type
		return false, 0
	}

	if dstPort == 443 || dstPort == 80 {
		return true, 0.90
	}

	return true, 0.75
}

// detectGRPC 检测 gRPC
func detectGRPC(data []byte, dstPort uint16) (bool, float64) {
	// gRPC 基于 HTTP/2，先检测 HTTP/2
	if matched, score := detectHTTP2(data, dstPort); !matched {
		return false, 0
	} else {
		// 如果是 HTTP/2，检查 content-type 特征
		// 简化: 基于端口和 HTTP/2 检测结果
		if dstPort == 443 || dstPort == 50051 || dstPort == 50052 {
			return true, score * 0.95
		}
		return true, score * 0.80
	}
}

// detectMySQL 检测 MySQL
func detectMySQL(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 5 {
		return false, 0
	}

	// MySQL Packet 格式: length(3) + sequence(1) + payload
	length := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16
	if length < 1 || length > 0xFFFFFF {
		return false, 0
	}

	// 检查命令类型
	command := data[4]
	if command > 30 {
		return false, 0
	}

	if dstPort == 3306 || dstPort == 3307 {
		return true, 0.95
	}

	return true, 0.80
}

// detectRedis 检测 Redis (RESP)
func detectRedis(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 1 {
		return false, 0
	}

	// RESP 类型标识
	respTypes := []byte{'+', '-', ':', '$', '*', '_', '#', ',', '(', '!'}
	first := data[0]

	for _, t := range respTypes {
		if first == t {
			if dstPort == 6379 || dstPort == 6380 {
				return true, 0.95
			}
			return true, 0.85
		}
	}

	// 内联命令检测
	if isInlineRedisCommand(data) {
		if dstPort == 6379 || dstPort == 6380 {
			return true, 0.90
		}
		return true, 0.75
	}

	return false, 0
}

// isInlineRedisCommand 检测内联 Redis 命令
func isInlineRedisCommand(data []byte) bool {
	if len(data) < 3 {
		return false
	}

	// 检查是否以 RESP 类型字符开头
	if bytes.IndexByte([]byte("+-:$*_#!,(">"), data[0]) >= 0 {
		return false
	}

	// 查找 \r\n
	if !bytes.Contains(data, []byte("\r\n")) {
		return false
	}

	// 检查是否包含空格
	return bytes.Contains(data, []byte(" "))
}

// detectKafka 检测 Kafka
func detectKafka(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 8 {
		return false, 0
	}

	// Kafka 请求格式: length(4) + api_key(2) + api_version(2) + ...
	length := binary.BigEndian.Uint32(data[0:4])
	if length < 4 || length > 100*1024*1024 {
		return false, 0
	}

	apiKey := int16(binary.BigEndian.Uint16(data[4:6]))
	if apiKey < 0 || apiKey > 100 {
		return false, 0
	}

	apiVersion := int16(binary.BigEndian.Uint16(data[6:8]))
	if apiVersion < 0 || apiVersion > 15 {
		return false, 0
	}

	if dstPort == 9092 || dstPort == 9093 {
		return true, 0.95
	}

	return true, 0.80
}

// detectDNS 检测 DNS
func detectDNS(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 12 {
		return false, 0
	}

	// DNS Header 检查
	flags := binary.BigEndian.Uint16(data[2:4])
	opcode := (flags & 0x7800) >> 11
	rcode := flags & 0x000F

	if opcode > 6 {
		return false, 0
	}

	if rcode > 23 {
		return false, 0
	}

	// QDCOUNT 检查
	qdcount := binary.BigEndian.Uint16(data[4:6])
	if qdcount > 100 {
		return false, 0
	}

	if dstPort == 53 {
		return true, 0.95
	}

	return true, 0.75
}

// ============================================================================
// 全局检测器
// ============================================================================

var globalDetector = NewProtocolDetector()

// DetectProtocol 全局协议检测函数
func DetectProtocol(data []byte, dstPort uint16) (ParserType, float64) {
	return globalDetector.Detect(data, dstPort)
}

// RegisterDetectionRule 注册自定义检测规则
func RegisterDetectionRule(rule DetectionRule) {
	globalDetector.AddRule(rule)
}
