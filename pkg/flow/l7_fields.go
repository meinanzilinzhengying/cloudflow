// Package flow L7 扩展字段定义
//
// 本文件扩展 UnifiedFlow 以支持 L7 协议解析引擎所需的字段

package flow

// ============================================================================
// L7 扩展字段 ID (用于 presence bitmap)
// ============================================================================

const (
	// 基础 L7 字段 (已在 flow.go 中定义)
	// FieldL7Protocol  FieldID = 7
	// FieldMethod      FieldID = 8
	// FieldPath        FieldID = 9
	// FieldStatusCode  FieldID = 10
	// FieldReqSize     FieldID = 11
	// FieldRespSize    FieldID = 12

	// 新增 L7 字段 (从 36 开始)
	FieldRequest     FieldID = 36 // 请求体 (截断)
	FieldResponse    FieldID = 37 // 响应体 (截断)
	FieldException   FieldID = 38 // 异常信息
	FieldGRPCService FieldID = 39 // gRPC 服务名
	FieldGRPCMethod  FieldID = 40 // gRPC 方法名
	FieldLatencyMs   FieldID = 41 // 延迟 (毫秒)

	// 预留字段 (用于未来扩展)
	FieldReserved42 FieldID = 42
	FieldReserved43 FieldID = 43
	FieldReserved44 FieldID = 44
	FieldReserved45 FieldID = 45
	FieldReserved46 FieldID = 46
	FieldReserved47 FieldID = 47

	// 更新总字段数
	FieldCountV2 = 48
)

// ============================================================================
// 定长字符串扩展 (用于 L7 内容存储)
// ============================================================================

// MaxRequestLen  请求体最大长度 (1KB)
const MaxRequestLen = 1024

// MaxResponseLen 响应体最大长度 (1KB)
const MaxResponseLen = 1024

// MaxExceptionLen 异常信息最大长度 (256 bytes)
const MaxExceptionLen = 256

// MaxGRPCServiceLen gRPC 服务名最大长度 (128 bytes)
const MaxGRPCServiceLen = 128

// MaxGRPCMethodLen gRPC 方法名最大长度 (128 bytes)
const MaxGRPCMethodLen = 128

// FixedString1024 定长字符串 (1024 bytes)
type FixedString1024 [MaxRequestLen]byte

func (s FixedString1024) String() string {
	n := 0
	for n < MaxRequestLen && s[n] != 0 {
		n++
	}
	return string(s[:n])
}

func (s FixedString1024) IsZero() bool {
	return s[0] == 0
}

func (s *FixedString1024) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxRequestLen {
		s[len(v)] = 0
	}
}

// FixedString128 定长字符串 (128 bytes)
type FixedString128 [MaxGRPCServiceLen]byte

func (s FixedString128) String() string {
	n := 0
	for n < MaxGRPCServiceLen && s[n] != 0 {
		n++
	}
	return string(s[:n])
}

func (s FixedString128) IsZero() bool {
	return s[0] == 0
}

func (s *FixedString128) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxGRPCServiceLen {
		s[len(v)] = 0
	}
}

// FixedString256 扩展方法 (用于异常信息)
type FixedString256Ext [MaxExceptionLen]byte

func (s FixedString256Ext) String() string {
	n := 0
	for n < MaxExceptionLen && s[n] != 0 {
		n++
	}
	return string(s[:n])
}

func (s FixedString256Ext) IsZero() bool {
	return s[0] == 0
}

func (s *FixedString256Ext) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxExceptionLen {
		s[len(v)] = 0
	}
}

// ============================================================================
// L7Flow L7 层扩展结构
// ============================================================================

// L7Flow L7 层完整信息
type L7Flow struct {
	// 协议信息
	Protocol Protocol // L7 协议类型

	// 请求/响应信息
	Method     uint8          // HTTP 方法
	Path       FixedString256 // URL 路径
	StatusCode uint16         // HTTP/gRPC 状态码

	// 内容 (截断存储)
	Request  FixedString1024 // 请求体
	Response FixedString1024 // 响应体

	// 大小
	ReqSize  uint64 // 请求大小 (bytes)
	RespSize uint64 // 响应大小 (bytes)

	// 异常信息
	Exception FixedString256Ext // 异常描述

	// gRPC 特有
	GRPCService FixedString128 // gRPC 服务名
	GRPCMethod  FixedString128 // gRPC 方法名

	// 性能指标
	LatencyNs uint64 // 延迟 (纳秒)
}

// SetRequest 设置请求体 (自动截断)
func (l *L7Flow) SetRequest(data []byte) {
	if len(data) > MaxRequestLen {
		data = data[:MaxRequestLen]
	}
	copy(l.Request[:], data)
	if len(data) < MaxRequestLen {
		l.Request[len(data)] = 0
	}
}

// SetResponse 设置响应体 (自动截断)
func (l *L7Flow) SetResponse(data []byte) {
	if len(data) > MaxResponseLen {
		data = data[:MaxResponseLen]
	}
	copy(l.Response[:], data)
	if len(data) < MaxResponseLen {
		l.Response[len(data)] = 0
	}
}

// SetException 设置异常信息
func (l *L7Flow) SetException(msg string) {
	l.Exception.Set(msg)
}

// SetGRPCInfo 设置 gRPC 信息
func (l *L7Flow) SetGRPCInfo(service, method string) {
	l.GRPCService.Set(service)
	l.GRPCMethod.Set(method)
}

// ============================================================================
// UnifiedFlow L7 扩展方法
// ============================================================================

// SetL7Request 设置 L7 请求
func (f *UnifiedFlow) SetL7Request(data []byte) *UnifiedFlow {
	// 存储到 Tags 中 (因为 UnifiedFlow 结构体大小固定)
	// 实际使用时应该扩展 UnifiedFlow 结构体
	// 这里为了兼容性使用 Tags
	if len(data) > 0 {
		reqStr := string(data)
		if len(reqStr) > MaxTagValueLen {
			reqStr = reqStr[:MaxTagValueLen]
		}
		f.Tags.Set("l7_request", reqStr)
		f.SetPresent(FieldTags)
	}
	return f
}

// SetL7Response 设置 L7 响应
func (f *UnifiedFlow) SetL7Response(data []byte) *UnifiedFlow {
	if len(data) > 0 {
		respStr := string(data)
		if len(respStr) > MaxTagValueLen {
			respStr = respStr[:MaxTagValueLen]
		}
		f.Tags.Set("l7_response", respStr)
		f.SetPresent(FieldTags)
	}
	return f
}

// SetL7Exception 设置 L7 异常
func (f *UnifiedFlow) SetL7Exception(msg string) *UnifiedFlow {
	if msg != "" {
		if len(msg) > MaxTagValueLen {
			msg = msg[:MaxTagValueLen]
		}
		f.Tags.Set("l7_exception", msg)
		f.SetPresent(FieldTags)
	}
	return f
}

// SetGRPCInfo 设置 gRPC 信息
func (f *UnifiedFlow) SetGRPCInfo(service, method string) *UnifiedFlow {
	if service != "" {
		f.Tags.Set("grpc_service", service)
	}
	if method != "" {
		f.Tags.Set("grpc_method", method)
	}
	if service != "" || method != "" {
		f.SetPresent(FieldTags)
	}
	return f
}

// SetL7Latency 设置 L7 延迟
func (f *UnifiedFlow) SetL7Latency(latencyNs uint64) *UnifiedFlow {
	f.LatencyNs = latencyNs
	f.SetPresent(FieldLatencyNs)
	return f
}

// GetL7Request 获取 L7 请求
func (f *UnifiedFlow) GetL7Request() string {
	return f.Tags.Get("l7_request")
}

// GetL7Response 获取 L7 响应
func (f *UnifiedFlow) GetL7Response() string {
	return f.Tags.Get("l7_response")
}

// GetL7Exception 获取 L7 异常
func (f *UnifiedFlow) GetL7Exception() string {
	return f.Tags.Get("l7_exception")
}

// GetGRPCService 获取 gRPC 服务名
func (f *UnifiedFlow) GetGRPCService() string {
	return f.Tags.Get("grpc_service")
}

// GetGRPCMethod 获取 gRPC 方法名
func (f *UnifiedFlow) GetGRPCMethod() string {
	return f.Tags.Get("grpc_method")
}

// ============================================================================
// L7 协议辅助函数
// ============================================================================

// IsL7Protocol 检查是否为 L7 协议
func IsL7Protocol(p Protocol) bool {
	switch p {
	case ProtoHTTP, ProtoHTTP2, ProtoGRPC, ProtoDNS, ProtoMySQL, ProtoRedis, ProtoKafka:
		return true
	default:
		return false
	}
}

// GetL7ProtocolName 获取 L7 协议名称
func GetL7ProtocolName(p Protocol) string {
	return p.String()
}

// ParseL7Protocol 解析 L7 协议字符串
func ParseL7Protocol(s string) Protocol {
	return ParseProtocol(s)
}
