// Package parsers HTTP/1.x 协议解析器
//
// 实现要点:
//   - 支持 HTTP/1.0 和 HTTP/1.1
//   - 支持 chunked encoding
//   - 支持 keep-alive 连接
//   - 流式解析，处理部分包

package parsers

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidHTTP     = errors.New("invalid http message")
	ErrIncompleteHTTP  = errors.New("incomplete http message")
)

// HTTPMethod HTTP 方法
type HTTPMethod uint8

const (
	MethodUnknown HTTPMethod = iota
	MethodGET
	MethodPOST
	MethodPUT
	MethodDELETE
	MethodHEAD
	MethodOPTIONS
	MethodPATCH
	MethodCONNECT
	MethodTRACE
)

func (m HTTPMethod) String() string {
	names := []string{
		"UNKNOWN", "GET", "POST", "PUT", "DELETE",
		"HEAD", "OPTIONS", "PATCH", "CONNECT", "TRACE",
	}
	if int(m) < len(names) {
		return names[m]
	}
	return "UNKNOWN"
}

// ParseHTTPMethod 解析 HTTP 方法
func ParseHTTPMethod(method string) HTTPMethod {
	switch method {
	case "GET":
		return MethodGET
	case "POST":
		return MethodPOST
	case "PUT":
		return MethodPUT
	case "DELETE":
		return MethodDELETE
	case "HEAD":
		return MethodHEAD
	case "OPTIONS":
		return MethodOPTIONS
	case "PATCH":
		return MethodPATCH
	case "CONNECT":
		return MethodCONNECT
	case "TRACE":
		return MethodTRACE
	default:
		return MethodUnknown
	}
}

// HTTPParser HTTP/1.x 解析器
type HTTPParser struct {
	state *httpParseState
}

// httpParseState 解析状态
type httpParseState struct {
	buffer []byte

	// 解析阶段
	phase httpParsePhase

	// 当前解析的消息
	currentMsg *HTTPMessage

	// body 读取状态
	bodyRemaining int64
	chunkedState  chunkedParseState
}

type httpParsePhase uint8

const (
	httpPhaseStartLine httpParsePhase = iota
	httpPhaseHeaders
	httpPhaseBody
	httpPhaseChunked
	httpPhaseComplete
)

type chunkedParseState uint8

const (
	chunkedStateSize chunkedParseState = iota
	chunkedStateData
	chunkedStateTrailer
	chunkedStateComplete
)

// HTTPMessage HTTP 消息
type HTTPMessage struct {
	// 请求行 / 状态行
	IsRequest bool
	Method    string
	Path      string
	Version   string
	StatusCode int
	StatusText string

	// Headers
	Headers map[string]string

	// Body
	Body []byte

	// 解析状态
	HeaderComplete bool
	BodyComplete   bool
	ContentLength  int64
	IsChunked      bool
}

// NewHTTPParser 创建 HTTP 解析器
func NewHTTPParser() *HTTPParser {
	return &HTTPParser{
		state: &httpParseState{
			buffer:  make([]byte, 0, 65536),
			phase:   httpPhaseStartLine,
			currentMsg: &HTTPMessage{
				Headers: make(map[string]string),
			},
		},
	}
}

// Type 返回解析器类型
func (p *HTTPParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeHTTP1
}

// Name 返回解析器名称
func (p *HTTPParser) Name() string {
	return "http1"
}

// Priority 返回解析优先级
func (p *HTTPParser) Priority() int {
	return 90
}

// Detect 协议检测
func (p *HTTPParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 4 {
		return false, 0
	}

	// 检查 HTTP 方法
	methods := []string{"GET ", "POST", "PUT ", "DELE", "HEAD", "OPTI", "PATC", "CONN", "TRAC"}
	for _, method := range methods {
		if strings.HasPrefix(string(data), method) {
			if dstPort == 80 || dstPort == 8080 || dstPort == 8000 {
				return true, 0.95
			}
			return true, 0.85
		}
	}

	// 检查 HTTP 响应
	if strings.HasPrefix(string(data), "HTTP") {
		if dstPort == 80 || dstPort == 8080 || dstPort == 8000 {
			return true, 0.95
		}
		return true, 0.85
	}

	return false, 0
}

// Parse 解析数据包
func (p *HTTPParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建解析状态
	parseState, ok := state.(*httpParseState)
	if !ok {
		parseState = &httpParseState{
			buffer: make([]byte, 0, 65536),
			currentMsg: &HTTPMessage{
				Headers: make(map[string]string),
			},
		}
	}

	// 添加数据到缓冲区
	parseState.buffer = append(parseState.buffer, data...)

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeHTTP1,
		Headers:    make(map[string]string),
	}

	// 解析消息
	msg, err := p.parseMessage(parseState)
	if err != nil {
		if err == ErrIncompleteHTTP {
			result.IsPartial = true
			result.NeedMore = true
			return result, parseState, nil
		}
		return nil, parseState, err
	}

	if msg != nil {
		// 填充结果
		if msg.IsRequest {
			result.Direction = l7parser.DirRequest
			result.Method = uint8(ParseHTTPMethod(msg.Method))
			result.Path = msg.Path
		} else {
			result.Direction = l7parser.DirResponse
			result.StatusCode = uint16(msg.StatusCode)
		}

		result.Headers = msg.Headers
		result.ContentType = msg.Headers["content-type"]

		if len(msg.Body) > 0 {
			if msg.IsRequest {
				result.Request = msg.Body
			} else {
				result.Response = msg.Body
			}
		}

		result.IsComplete = msg.BodyComplete
	}

	return result, parseState, nil
}

// parseMessage 解析 HTTP 消息
func (p *HTTPParser) parseMessage(state *httpParseState) (*HTTPMessage, error) {
	msg := state.currentMsg

	for state.phase != httpPhaseComplete {
		switch state.phase {
		case httpPhaseStartLine:
			if err := p.parseStartLine(state, msg); err != nil {
				return nil, err
			}

		case httpPhaseHeaders:
			if err := p.parseHeaders(state, msg); err != nil {
				return nil, err
			}

		case httpPhaseBody:
			if err := p.parseBody(state, msg); err != nil {
				return nil, err
			}

		case httpPhaseChunked:
			if err := p.parseChunked(state, msg); err != nil {
				return nil, err
			}
		}
	}

	// 重置状态准备下一条消息
	completeMsg := msg
	state.phase = httpPhaseStartLine
	state.currentMsg = &HTTPMessage{
		Headers: make(map[string]string),
	}
	state.bodyRemaining = 0
	state.chunkedState = chunkedStateSize

	return completeMsg, nil
}

// parseStartLine 解析请求行/状态行
func (p *HTTPParser) parseStartLine(state *httpParseState, msg *HTTPMessage) error {
	// 查找 \r\n
	endIdx := bytes.Index(state.buffer, []byte("\r\n"))
	if endIdx < 0 {
		return ErrIncompleteHTTP
	}

	line := string(state.buffer[:endIdx])
	state.buffer = state.buffer[endIdx+2:]

	// 判断是请求还是响应
	if strings.HasPrefix(line, "HTTP") {
		// 状态行: HTTP/1.1 200 OK
		msg.IsRequest = false
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 2 {
			return ErrInvalidHTTP
		}
		msg.Version = parts[0]
		statusCode, _ := strconv.Atoi(parts[1])
		msg.StatusCode = statusCode
		if len(parts) > 2 {
			msg.StatusText = parts[2]
		}
	} else {
		// 请求行: GET /path HTTP/1.1
		msg.IsRequest = true
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			return ErrInvalidHTTP
		}
		msg.Method = parts[0]
		msg.Path = parts[1]
		msg.Version = parts[2]
	}

	state.phase = httpPhaseHeaders
	return nil
}

// parseHeaders 解析 headers
func (p *HTTPParser) parseHeaders(state *httpParseState, msg *HTTPMessage) error {
	for {
		// 查找 \r\n
		endIdx := bytes.Index(state.buffer, []byte("\r\n"))
		if endIdx < 0 {
			return ErrIncompleteHTTP
		}

		// 空行表示 headers 结束
		if endIdx == 0 {
			state.buffer = state.buffer[2:]
			msg.HeaderComplete = true

			// 确定 body 解析方式
			if msg.Headers["transfer-encoding"] == "chunked" {
				msg.IsChunked = true
				state.phase = httpPhaseChunked
			} else if cl := msg.Headers["content-length"]; cl != "" {
				length, _ := strconv.ParseInt(cl, 10, 64)
				msg.ContentLength = length
				state.bodyRemaining = length
				if length == 0 {
					msg.BodyComplete = true
					state.phase = httpPhaseComplete
				} else {
					state.phase = httpPhaseBody
				}
			} else {
				// 没有 body
				msg.BodyComplete = true
				state.phase = httpPhaseComplete
			}
			return nil
		}

		// 解析 header 行
		line := string(state.buffer[:endIdx])
		state.buffer = state.buffer[endIdx+2:]

		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue // 跳过无效行
		}

		key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
		value := strings.TrimSpace(line[colonIdx+1:])
		msg.Headers[key] = value
	}
}

// parseBody 解析固定长度的 body
func (p *HTTPParser) parseBody(state *httpParseState, msg *HTTPMessage) error {
	if state.bodyRemaining <= 0 {
		msg.BodyComplete = true
		state.phase = httpPhaseComplete
		return nil
	}

	toRead := int(state.bodyRemaining)
	if toRead > len(state.buffer) {
		toRead = len(state.buffer)
	}

	msg.Body = append(msg.Body, state.buffer[:toRead]...)
	state.buffer = state.buffer[toRead:]
	state.bodyRemaining -= int64(toRead)

	if state.bodyRemaining <= 0 {
		msg.BodyComplete = true
		state.phase = httpPhaseComplete
	}

	return nil
}

// parseChunked 解析 chunked encoding
func (p *HTTPParser) parseChunked(state *httpParseState, msg *HTTPMessage) error {
	for state.chunkedState != chunkedStateComplete {
		switch state.chunkedState {
		case chunkedStateSize:
			// 解析 chunk size
			endIdx := bytes.Index(state.buffer, []byte("\r\n"))
			if endIdx < 0 {
				return ErrIncompleteHTTP
			}

			sizeLine := string(state.buffer[:endIdx])
			state.buffer = state.buffer[endIdx+2:]

			// 解析十六进制 size
			semicolonIdx := strings.Index(sizeLine, ";")
			if semicolonIdx >= 0 {
				sizeLine = sizeLine[:semicolonIdx]
			}

			size, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 16, 64)
			if err != nil {
				return ErrInvalidHTTP
			}

			state.bodyRemaining = size
			if size == 0 {
				state.chunkedState = chunkedStateTrailer
			} else {
				state.chunkedState = chunkedStateData
			}

		case chunkedStateData:
			// 读取 chunk data
			if int64(len(state.buffer)) < state.bodyRemaining+2 {
				return ErrIncompleteHTTP
			}

			msg.Body = append(msg.Body, state.buffer[:state.bodyRemaining]...)
			state.buffer = state.buffer[state.bodyRemaining+2:] // +2 for \r\n
			state.chunkedState = chunkedStateSize

		case chunkedStateTrailer:
			// 跳过 trailer headers
			for {
				endIdx := bytes.Index(state.buffer, []byte("\r\n"))
				if endIdx < 0 {
					return ErrIncompleteHTTP
				}

				if endIdx == 0 {
					state.buffer = state.buffer[2:]
					state.chunkedState = chunkedStateComplete
					break
				}

				state.buffer = state.buffer[endIdx+2:]
			}
		}
	}

	if state.chunkedState == chunkedStateComplete {
		msg.BodyComplete = true
		state.phase = httpPhaseComplete
	}

	return nil
}

// ParseStreaming 流式解析
func (p *HTTPParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *HTTPParser) Reset() {
	p.state = &httpParseState{
		buffer: make([]byte, 0, 65536),
		currentMsg: &HTTPMessage{
			Headers: make(map[string]string),
		},
	}
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("http1", func() l7parser.Parser {
		return NewHTTPParser()
	})
}
