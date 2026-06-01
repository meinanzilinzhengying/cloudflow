// Package parsers gRPC 协议解析器
//
// gRPC 基于 HTTP/2，额外支持:
//   - gRPC 消息帧解析 (Length-Prefixed Message)
//   - gRPC 状态码解析 (trailers)
//   - gRPC 元数据提取 (timeout, encoding, etc.)
//   - 服务名/方法名解析

package parsers

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"sync"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidGRPCMessage = errors.New("invalid gRPC message")
)

// gRPC 内容类型
const (
	GRPCContentType             = "application/grpc"
	GRPCContentTypeProto        = "application/grpc+proto"
	GRPCContentTypeJSON         = "application/grpc+json"
	GRPCWebContentType          = "application/grpc-web"
	GRPCWebTextContentType      = "application/grpc-web-text"
)

// gRPC 头部常量
const (
	GRPCStatusHeader        = "grpc-status"
	GRPCMessageHeader       = "grpc-message"
	GRPCStatusDetailsHeader = "grpc-status-details-bin"
	GRPCTimeoutHeader       = "grpc-timeout"
	GRPCMethodHeader        = ":path"
	GRPCAuthorityHeader     = ":authority"
	GRPCContentEncoding     = "grpc-encoding"
	GRPCAcceptEncoding      = "grpc-accept-encoding"
	GRPCMessageType         = "grpc-message-type"
)

// GRPCMessage gRPC 消息结构
type GRPCMessage struct {
	Compressed bool   // 是否压缩
	Length     uint32 // 消息长度
	Data       []byte // 消息数据 (protobuf)
}

// ParseGRPCMessage 解析 gRPC Length-Prefixed Message
func ParseGRPCMessage(data []byte) (*GRPCMessage, int, error) {
	if len(data) < 5 {
		return nil, 0, l7parser.ErrIncompletePacket
	}

	msg := &GRPCMessage{
		Compressed: data[0] != 0,
		Length:     binary.BigEndian.Uint32(data[1:5]),
	}

	if msg.Length > 4*1024*1024 { // 4MB 限制
		return nil, 0, ErrInvalidGRPCMessage
	}

	if uint32(len(data)) < 5+msg.Length {
		return nil, 0, l7parser.ErrIncompletePacket
	}

	msg.Data = data[5 : 5+msg.Length]
	return msg, int(5 + msg.Length), nil
}

// GRPCStatus gRPC 状态码
type GRPCStatus uint32

const (
	GRPCStatusOK GRPCStatus = iota
	GRPCStatusCanceled
	GRPCStatusUnknown
	GRPCStatusInvalidArgument
	GRPCStatusDeadlineExceeded
	GRPCStatusNotFound
	GRPCStatusAlreadyExists
	GRPCStatusPermissionDenied
	GRPCStatusResourceExhausted
	GRPCStatusFailedPrecondition
	GRPCStatusAborted
	GRPCStatusOutOfRange
	GRPCStatusUnimplemented
	GRPCStatusInternal
	GRPCStatusUnavailable
	GRPCStatusDataLoss
	GRPCStatusUnauthenticated
)

func (s GRPCStatus) String() string {
	names := []string{
		"OK", "Canceled", "Unknown", "InvalidArgument", "DeadlineExceeded",
		"NotFound", "AlreadyExists", "PermissionDenied", "ResourceExhausted",
		"FailedPrecondition", "Aborted", "OutOfRange", "Unimplemented",
		"Internal", "Unavailable", "DataLoss", "Unauthenticated",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "Unknown"
}

// GRPCParser gRPC 解析器
type GRPCParser struct {
	http2Parser *HTTP2Parser

	// 流状态缓存 (flowID -> *grpcStreamState)
	streamStates sync.Map
}

// grpcStreamState gRPC 流状态
type grpcStreamState struct {
	// 请求信息
	Service    string
	Method     string
	Authority  string
	Timeout    string
	Encoding   string

	// 响应信息
	StatusCode uint32
	StatusMsg  string

	// 消息缓冲
	ReqMessages  [][]byte
	RespMessages [][]byte

	// 解析状态
	ReqComplete  bool
	RespComplete bool
}

// NewGRPCParser 创建 gRPC 解析器
func NewGRPCParser() *GRPCParser {
	return &GRPCParser{
		http2Parser: NewHTTP2Parser(),
	}
}

// Type 返回解析器类型
func (p *GRPCParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeGRPC
}

// Name 返回解析器名称
func (p *GRPCParser) Name() string {
	return "grpc"
}

// Priority 返回解析优先级
func (p *GRPCParser) Priority() int {
	return 110 // gRPC 优先级高于 HTTP/2
}

// Detect 协议检测
func (p *GRPCParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	// 首先检查是否为 HTTP/2
	if matched, score := p.http2Parser.Detect(data, dstPort); !matched {
		return false, 0
	} else {
		// 如果是 HTTP/2，检查 content-type 是否为 gRPC
		// 这里简化处理，实际应该解析 headers
		if dstPort == 443 || dstPort == 50051 || dstPort == 50052 {
			return true, score * 0.95
		}
		return true, score * 0.8
	}
}

// Parse 解析数据包
func (p *GRPCParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	// 首先使用 HTTP/2 解析器解析
	httpResult, _, err := p.http2Parser.Parse(ctx, input, state)
	if err != nil {
		return nil, state, err
	}
	if httpResult == nil {
		return nil, state, nil
	}

	// 转换为 gRPC 结果
	result := &l7parser.ParseResult{
		ParserType:  l7parser.ParserTypeGRPC,
		Direction:   httpResult.Direction,
		IsComplete:  httpResult.IsComplete,
		IsPartial:   httpResult.IsPartial,
		NeedMore:    httpResult.NeedMore,
		Method:      httpResult.Method,
		StatusCode:  httpResult.StatusCode,
		Path:        httpResult.Path,
		Headers:     httpResult.Headers,
		ContentType: httpResult.ContentType,
		LatencyNs:   httpResult.LatencyNs,
	}

	// 获取或创建流状态
	flowID := input.Packet.Metadata.FlowID
	streamID := uint32(0) // 从 HTTP/2 frame 中获取

	stateVal, _ := p.streamStates.LoadOrStore(flowID, &grpcStreamState{})
	streamState := stateVal.(*grpcStreamState)

	// 解析 gRPC 特有信息
	p.parseGRPCInfo(httpResult, streamState, streamID)

	// 提取 gRPC 消息
	if len(httpResult.Request) > 0 {
		p.parseGRPCMessages(httpResult.Request, streamState.ReqMessages)
	}
	if len(httpResult.Response) > 0 {
		p.parseGRPCMessages(httpResult.Response, streamState.RespMessages)
	}

	// 填充 gRPC 特有字段
	result.GRPCService = streamState.Service
	result.GRPCMethod = streamState.Method
	result.GRPCStatus = streamState.StatusCode

	if streamState.StatusMsg != "" {
		result.Exception = streamState.StatusMsg
	}

	// 检查是否完成
	if streamState.ReqComplete && streamState.RespComplete {
		result.IsComplete = true
	}

	return result, streamState, nil
}

// parseGRPCInfo 解析 gRPC 信息
func (p *GRPCParser) parseGRPCInfo(httpResult *l7parser.ParseResult, state *grpcStreamState, streamID uint32) {
	// 解析 service/method 从 :path
	if path, ok := httpResult.Headers[GRPCMethodHeader]; ok && path != "" {
		// gRPC path 格式: /package.service/method
		parts := strings.SplitN(path, "/", 3)
		if len(parts) >= 3 {
			state.Service = parts[1]
			state.Method = parts[2]
		} else if len(parts) == 2 {
			state.Service = parts[1]
		}
	}

	// 提取其他元数据
	if authority, ok := httpResult.Headers[GRPCAuthorityHeader]; ok {
		state.Authority = authority
	}
	if timeout, ok := httpResult.Headers[GRPCTimeoutHeader]; ok {
		state.Timeout = timeout
	}
	if encoding, ok := httpResult.Headers[GRPCContentEncoding]; ok {
		state.Encoding = encoding
	}

	// 检查 content-type 确认是 gRPC
	contentType := httpResult.ContentType
	if contentType == "" {
		contentType = httpResult.Headers["content-type"]
	}

	if isGRPCContentType(contentType) {
		// 确认是 gRPC 请求
		if httpResult.Direction == l7parser.DirRequest {
			state.ReqComplete = httpResult.IsComplete
		} else {
			state.RespComplete = httpResult.IsComplete
		}
	}

	// 解析 gRPC status (从 trailers)
	if statusStr, ok := httpResult.Headers[GRPCStatusHeader]; ok {
		var status uint32
		if _, err := parseUint32(statusStr, &status); err == nil {
			state.StatusCode = status
			if status != 0 {
				state.StatusMsg = httpResult.Headers[GRPCMessageHeader]
			}
		}
		state.RespComplete = true
	}
}

// parseGRPCMessages 解析 gRPC 消息
func (p *GRPCParser) parseGRPCMessages(data []byte, messages [][]byte) [][]byte {
	offset := 0
	for offset < len(data) {
		msg, consumed, err := ParseGRPCMessage(data[offset:])
		if err != nil {
			break
		}
		offset += consumed
		messages = append(messages, msg.Data)
	}
	return messages
}

// isGRPCContentType 检查是否为 gRPC content-type
func isGRPCContentType(ct string) bool {
	return strings.HasPrefix(ct, GRPCContentType) ||
		strings.HasPrefix(ct, GRPCWebContentType)
}

// parseUint32 解析 uint32
func parseUint32(s string, v *uint32) (int, error) {
	if s == "" {
		return 0, errors.New("empty string")
	}

	var n uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return i, errors.New("invalid character")
		}
		n = n*10 + uint32(c-'0')
	}
	*v = n
	return len(s), nil
}

// ParseStreaming 流式解析
func (p *GRPCParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *GRPCParser) Reset() {
	p.http2Parser.Reset()
	p.streamStates = sync.Map{}
}

// ============================================================================
// 便捷方法
// ============================================================================

// ExtractServiceMethod 从 gRPC path 提取服务名和方法名
// path 格式: /package.service/method
func ExtractServiceMethod(path string) (service, method string) {
	if !strings.HasPrefix(path, "/") {
		return "", ""
	}

	parts := strings.SplitN(path[1:], "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	} else if len(parts) == 1 {
		return parts[0], ""
	}
	return "", ""
}

// ParseGRPCTimeout 解析 gRPC timeout 头部
// 格式: TimeoutValue TimeoutUnit
// 例如: "10S" (10 seconds), "1M" (1 minute)
func ParseGRPCTimeout(timeout string) (value uint32, unit byte, ok bool) {
	if len(timeout) < 2 {
		return 0, 0, false
	}

	unit = timeout[len(timeout)-1]
	timeoutStr := timeout[:len(timeout)-1]

	var val uint32
	for i := 0; i < len(timeoutStr); i++ {
		c := timeoutStr[i]
		if c < '0' || c > '9' {
			return 0, 0, false
		}
		val = val*10 + uint32(c-'0')
	}

	return val, unit, true
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("grpc", func() l7parser.Parser {
		return NewGRPCParser()
	})
}
