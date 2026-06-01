// Package parsers HTTP/2 协议解析器
//
// 实现要点:
//   - HTTP/2 Frame 解析 (HEADERS, DATA, SETTINGS, etc.)
//   - HPACK 解码 (header 压缩)
//   - Stream 多路复用处理
//   - 支持 gRPC-over-HTTP/2

package parsers

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidFrame     = errors.New("invalid http2 frame")
	ErrFrameTooLarge    = errors.New("http2 frame too large")
	ErrUnknownFrameType = errors.New("unknown http2 frame type")
)

// HTTP2 Frame 类型常量
const (
	FrameData         uint8 = 0x0
	FrameHeaders      uint8 = 0x1
	FramePriority     uint8 = 0x2
	FrameRSTStream    uint8 = 0x3
	FrameSettings     uint8 = 0x4
	FramePushPromise  uint8 = 0x5
	FramePing         uint8 = 0x6
	FrameGoAway       uint8 = 0x7
	FrameWindowUpdate uint8 = 0x8
	FrameContinuation uint8 = 0x9
)

// HTTP2 Frame 标志位
const (
	FlagEndStream  uint8 = 0x1
	FlagAck        uint8 = 0x1 // Settings/Ping
	FlagEndHeaders uint8 = 0x4
	FlagPadded     uint8 = 0x8
	FlagPriority   uint8 = 0x20
)

// HTTP2 魔术字 (连接序言)
var HTTP2Magic = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

// HTTP2 默认设置
const (
	DefaultHeaderTableSize   uint32 = 4096
	DefaultMaxConcurrentStreams uint32 = 100
	DefaultInitialWindowSize uint32 = 65535
	DefaultMaxFrameSize      uint32 = 16384
	DefaultMaxHeaderListSize uint32 = 0 // 无限制
)

// Frame HTTP/2 Frame 结构
type Frame struct {
	Type     uint8
	Flags    uint8
	StreamID uint32
	Payload  []byte
}

// IsEndStream 检查 END_STREAM 标志
func (f *Frame) IsEndStream() bool {
	return f.Flags&FlagEndStream != 0
}

// IsEndHeaders 检查 END_HEADERS 标志
func (f *Frame) IsEndHeaders() bool {
	return f.Flags&FlagEndHeaders != 0
}

// FrameHeaderSize Frame 头大小 (9 bytes)
const FrameHeaderSize = 9

// ParseFrameHeader 解析 Frame Header
// 返回: 长度, 类型, 标志, StreamID, 错误
func ParseFrameHeader(data []byte) (length uint32, frameType, flags uint8, streamID uint32, err error) {
	if len(data) < FrameHeaderSize {
		return 0, 0, 0, 0, l7parser.ErrIncompletePacket
	}

	// Length: 3 bytes (big-endian)
	length = uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])

	// Type: 1 byte
	frameType = data[3]

	// Flags: 1 byte
	flags = data[4]

	// StreamID: 4 bytes (big-endian, 最高位保留为 0)
	streamID = binary.BigEndian.Uint32(data[5:9]) & 0x7FFFFFFF

	return length, frameType, flags, streamID, nil
}

// ParseFrame 解析完整 Frame
func ParseFrame(data []byte) (*Frame, int, error) {
	if len(data) < FrameHeaderSize {
		return nil, 0, l7parser.ErrIncompletePacket
	}

	length, frameType, flags, streamID, err := ParseFrameHeader(data)
	if err != nil {
		return nil, 0, err
	}

	// 检查 payload 是否完整
	if uint32(len(data)) < FrameHeaderSize+length {
		return nil, 0, l7parser.ErrIncompletePacket
	}

	// 最大 frame 大小检查 (默认 16KB，但可协商到 16MB)
	if length > 16*1024*1024 {
		return nil, 0, ErrFrameTooLarge
	}

	frame := &Frame{
		Type:     frameType,
		Flags:    flags,
		StreamID: streamID,
		Payload:  data[FrameHeaderSize : FrameHeaderSize+length],
	}

	return frame, int(FrameHeaderSize + length), nil
}

// ============================================================================
// HPACK 解码器 (简化实现)
// ============================================================================

// HPACKDecoder HPACK 解码器
type HPACKDecoder struct {
	dynamicTable       []HeaderField
	dynamicTableSize   uint32
	maxDynamicTableSize uint32
}

// HeaderField HTTP Header 字段
type HeaderField struct {
	Name  string
	Value string
}

// StaticTable HTTP/2 静态表 (部分常用条目)
// 完整静态表有 61 个条目，这里只包含最常用的
var StaticTable = []HeaderField{
	{":authority", ""},
	{":method", "GET"},
	{":method", "POST"},
	{":path", "/"},
	{":path", "/index.html"},
	{":scheme", "http"},
	{":scheme", "https"},
	{":status", "200"},
	{":status", "204"},
	{":status", "206"},
	{":status", "304"},
	{":status", "400"},
	{":status", "404"},
	{":status", "500"},
	{"accept-charset", ""},
	{"accept-encoding", "gzip, deflate"},
	{"accept-language", ""},
	{"accept-ranges", ""},
	{"accept", ""},
	{"access-control-allow-origin", ""},
	{"age", ""},
	{"allow", ""},
	{"authorization", ""},
	{"cache-control", ""},
	{"content-disposition", ""},
	{"content-encoding", ""},
	{"content-language", ""},
	{"content-length", ""},
	{"content-location", ""},
	{"content-range", ""},
	{"content-type", ""},
	{"cookie", ""},
	{"date", ""},
	{"etag", ""},
	{"expect", ""},
	{"expires", ""},
	{"from", ""},
	{"host", ""},
	{"if-match", ""},
	{"if-modified-since", ""},
	{"if-none-match", ""},
	{"if-range", ""},
	{"if-unmodified-since", ""},
	{"last-modified", ""},
	{"link", ""},
	{"location", ""},
	{"max-forwards", ""},
	{"proxy-authenticate", ""},
	{"proxy-authorization", ""},
	{"range", ""},
	{"referer", ""},
	{"refresh", ""},
	{"retry-after", ""},
	{"server", ""},
	{"set-cookie", ""},
	{"strict-transport-security", ""},
	{"transfer-encoding", ""},
	{"user-agent", ""},
	{"vary", ""},
	{"via", ""},
	{"www-authenticate", ""},
}

// NewHPACKDecoder 创建 HPACK 解码器
func NewHPACKDecoder() *HPACKDecoder {
	return &HPACKDecoder{
		dynamicTable:        make([]HeaderField, 0),
		maxDynamicTableSize: DefaultHeaderTableSize,
	}
}

// Decode 解码 HPACK 编码的 headers
func (d *HPACKDecoder) Decode(data []byte) ([]HeaderField, error) {
	var headers []HeaderField
	offset := 0

	for offset < len(data) {
		b := data[offset]

		if b&0x80 != 0 {
			// Indexed Header Field (1xxxxxxx)
			index, n := decodeInteger(data[offset:], 7)
			offset += n
			header := d.lookupTable(index)
			headers = append(headers, header)

		} else if b&0xC0 == 0x40 {
			// Literal Header Field with Incremental Indexing (01xxxxxx)
			index, n1 := decodeInteger(data[offset:], 6)
			offset += n1
			name := d.lookupName(index)
			value, n2 := decodeString(data[offset:])
			offset += n2
			headers = append(headers, HeaderField{Name: name, Value: value})
			dd.addToDynamicTable(name, value)

		} else if b&0xF0 == 0x00 {
			// Literal Header Field without Indexing (0000xxxx)
			index, n1 := decodeInteger(data[offset:], 4)
			offset += n1
			name := d.lookupName(index)
			value, n2 := decodeString(data[offset:])
			offset += n2
			headers = append(headers, HeaderField{Name: name, Value: value})

		} else if b&0xF0 == 0x10 {
			// Literal Header Field Never Indexed (0001xxxx)
			index, n1 := decodeInteger(data[offset:], 4)
			offset += n1
			name := d.lookupName(index)
			value, n2 := decodeString(data[offset:])
			offset += n2
			headers = append(headers, HeaderField{Name: name, Value: value})

		} else if b&0xE0 == 0x20 {
			// Dynamic Table Size Update (001xxxxx)
			size, n := decodeInteger(data[offset:], 5)
			offset += n
			d.setDynamicTableSize(uint32(size))

		} else {
			return nil, fmt.Errorf("invalid HPACK encoding at offset %d: 0x%02x", offset, b)
		}
	}

	return headers, nil
}

// lookupTable 查找表项
func (d *HPACKDecoder) lookupTable(index int) HeaderField {
	if index == 0 {
		return HeaderField{}
	}

	// 静态表: 1-61
	if index <= len(StaticTable) {
		return StaticTable[index-1]
	}

	// 动态表: 62+
	dynIndex := index - len(StaticTable) - 1
	if dynIndex < len(d.dynamicTable) {
		return d.dynamicTable[dynIndex]
	}

	return HeaderField{}
}

// lookupName 只查找 name
func (d *HPACKDecoder) lookupName(index int) string {
	if index == 0 {
		return ""
	}
	return d.lookupTable(index).Name
}

// addToDynamicTable 添加到动态表
func (d *HPACKDecoder) addToDynamicTable(name, value string) {
	entry := HeaderField{Name: name, Value: value}
	d.dynamicTable = append([]HeaderField{entry}, d.dynamicTable...)
	d.dynamicTableSize += uint32(len(name) + len(value) + 32)

	// 清理超出的条目
	for d.dynamicTableSize > d.maxDynamicTableSize && len(d.dynamicTable) > 0 {
		last := d.dynamicTable[len(d.dynamicTable)-1]
		d.dynamicTableSize -= uint32(len(last.Name) + len(last.Value) + 32)
		d.dynamicTable = d.dynamicTable[:len(d.dynamicTable)-1]
	}
}

// setDynamicTableSize 设置动态表大小
func (d *HPACKDecoder) setDynamicTableSize(size uint32) {
	d.maxDynamicTableSize = size
	// 清理超出的条目
	for d.dynamicTableSize > d.maxDynamicTableSize && len(d.dynamicTable) > 0 {
		last := d.dynamicTable[len(d.dynamicTable)-1]
		d.dynamicTableSize -= uint32(len(last.Name) + len(last.Value) + 32)
		d.dynamicTable = d.dynamicTable[:len(d.dynamicTable)-1]
	}
}

// decodeInteger 解码 HPACK 整数
func decodeInteger(data []byte, prefixBits uint8) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}

	prefixMask := uint8((1 << prefixBits) - 1)
	value := int(data[0] & prefixMask)

	if value < (1<<prefixBits)-1 {
		return value, 1
	}

	// 多字节编码
	m := 0
	offset := 1
	for offset < len(data) {
		b := data[offset]
		value += int(b&0x7F) << m
		m += 7
		offset++
		if b&0x80 == 0 {
			break
		}
	}

	return value, offset
}

// decodeString 解码 HPACK 字符串
func decodeString(data []byte) (string, int) {
	if len(data) == 0 {
		return "", 0
	}

	huffman := data[0]&0x80 != 0
	length, n := decodeInteger(data, 7)

	if n+length > len(data) {
		return "", 0
	}

	strData := data[n : n+length]

	if huffman {
		// 简化: 这里应该实现 Huffman 解码
		// 实际生产环境需要完整的 Huffman 解码表
		return string(strData), n + length
	}

	return string(strData), n + length
}

// ============================================================================
// HTTP2 Parser
// ============================================================================

// HTTP2Parser HTTP/2 解析器
type HTTP2Parser struct {
	// HPACK 解码器 (每个连接一个)
	decoders sync.Map // flowID -> *HPACKDecoder
}

// NewHTTP2Parser 创建 HTTP/2 解析器
func NewHTTP2Parser() *HTTP2Parser {
	return &HTTP2Parser{}
}

// Type 返回解析器类型
func (p *HTTP2Parser) Type() l7parser.ParserType {
	return l7parser.ParserTypeHTTP2
}

// Name 返回解析器名称
func (p *HTTP2Parser) Name() string {
	return "http2"
}

// Priority 返回解析优先级
func (p *HTTP2Parser) Priority() int {
	return 100 // HTTP/2 优先级较高
}

// Detect 协议检测
func (p *HTTP2Parser) Detect(data []byte, dstPort uint16) (bool, float64) {
	// 检查 HTTP/2 魔术字 (连接序言)
	if len(data) >= len(HTTP2Magic) {
		if string(data[:len(HTTP2Magic)]) == string(HTTP2Magic) {
			return true, 1.0
		}
	}

	// 检查是否为有效的 HTTP/2 frame
	if len(data) >= FrameHeaderSize {
		length, frameType, _, _, err := ParseFrameHeader(data)
		if err == nil {
			// 检查 frame 类型是否有效
			if frameType <= FrameContinuation {
				// 检查长度是否合理
				if length <= 16384 { // 默认 max frame size
					// 检查端口
					if dstPort == 443 || dstPort == 80 || dstPort == 8080 {
						return true, 0.9
					}
					return true, 0.7
				}
			}
		}
	}

	return false, 0
}

// Parse 解析数据包
func (p *HTTP2Parser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建 HPACK 解码器
	flowID := input.Packet.Metadata.FlowID
	decoder, _ := p.decoders.LoadOrStore(flowID, NewHPACKDecoder())
	hpack := decoder.(*HPACKDecoder)

	// 跳过连接序言
	offset := 0
	if len(data) >= len(HTTP2Magic) && string(data[:len(HTTP2Magic)]) == string(HTTP2Magic) {
		offset = len(HTTP2Magic)
	}

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeHTTP2,
		Headers:    make(map[string]string),
	}

	// 解析 frames
	for offset < len(data) {
		frame, consumed, err := ParseFrame(data[offset:])
		if err != nil {
			if err == l7parser.ErrIncompletePacket {
				result.IsPartial = true
				result.NeedMore = true
				return result, state, nil
			}
			return nil, state, err
		}
		offset += consumed

		// 处理不同类型的 frame
		switch frame.Type {
		case FrameHeaders:
			p.parseHeadersFrame(frame, hpack, result)

		case FrameData:
			// 数据帧
			if len(frame.Payload) > 0 {
				if result.Direction == l7parser.DirRequest {
					result.Request = append(result.Request, frame.Payload...)
				} else {
					result.Response = append(result.Response, frame.Payload...)
				}
			}
			if frame.IsEndStream() {
				result.IsComplete = true
			}

		case FrameRSTStream:
			// 流重置
			if len(frame.Payload) >= 4 {
				errorCode := binary.BigEndian.Uint32(frame.Payload[:4])
				result.Exception = fmt.Sprintf("RST_STREAM error_code=%d", errorCode)
			}

		case FrameGoAway:
			// 连接关闭
			if len(frame.Payload) >= 8 {
				lastStreamID := binary.BigEndian.Uint32(frame.Payload[:4]) & 0x7FFFFFFF
				errorCode := binary.BigEndian.Uint32(frame.Payload[4:8])
				result.Exception = fmt.Sprintf("GOAWAY last_stream_id=%d error_code=%d", lastStreamID, errorCode)
			}
		}
	}

	return result, state, nil
}

// parseHeadersFrame 解析 HEADERS frame
func (p *HTTP2Parser) parseHeadersFrame(frame *Frame, decoder *HPACKDecoder, result *l7parser.ParseResult) {
	payload := frame.Payload
	offset := 0

	// 处理 PAD 标志
	if frame.Flags&FlagPadded != 0 && len(payload) > 0 {
		padLen := int(payload[0])
		if len(payload) > padLen {
			payload = payload[1 : len(payload)-padLen]
		}
	}

	// 处理 PRIORITY 标志
	if frame.Flags&FlagPriority != 0 && len(payload) >= 5 {
		// E + StreamDependency(31) + Weight(8)
		offset = 5
	}

	// 解码 headers
	if offset < len(payload) {
		headers, err := decoder.Decode(payload[offset:])
		if err == nil {
			for _, h := range headers {
				result.Headers[h.Name] = h.Value

				// 提取关键信息
				switch h.Name {
				case ":method":
					result.Method = parseHTTPMethod(h.Value)
				case ":path":
					result.Path = h.Value
				case ":status":
					var code uint16
					fmt.Sscanf(h.Value, "%d", &code)
					result.StatusCode = code
					result.Direction = l7parser.DirResponse
				case ":authority":
					result.Headers[":authority"] = h.Value
				case "content-type":
					result.ContentType = h.Value
				}
			}

			// 判断方向
			if _, hasMethod := result.Headers[":method"]; hasMethod {
				result.Direction = l7parser.DirRequest
			}
		}
	}

	if frame.IsEndStream() {
		result.IsComplete = true
	}
}

// parseHTTPMethod 解析 HTTP 方法
func parseHTTPMethod(method string) uint8 {
	switch method {
	case "GET":
		return 1
	case "POST":
		return 2
	case "PUT":
		return 3
	case "DELETE":
		return 4
	case "HEAD":
		return 5
	case "OPTIONS":
		return 6
	case "PATCH":
		return 7
	default:
		return 0
	}
}

// ParseStreaming 流式解析
func (p *HTTP2Parser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	// HTTP/2 通常不需要跨包状态，因为 frame 边界清晰
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *HTTP2Parser) Reset() {
	p.decoders = sync.Map{}
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("http2", func() l7parser.Parser {
		return NewHTTP2Parser()
	})
}
