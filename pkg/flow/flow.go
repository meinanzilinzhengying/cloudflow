// Package flow 定义 CloudFlow 统一流量数据模型
//
// 设计目标:
//   - 统一抽象: metrics/logs/traces 全部关联到 UnifiedFlow
//   - 内存对齐: 热路径字段紧凑排列，减少 cache miss
//   - Nullable 优化: 使用存在位图 (presence bitmap) 替代 nil 指针
//   - Versioning: SchemaVersion 字段支持前向/后向兼容
//   - 零值安全: 所有字段零值语义明确
//
// 数据层次:
//
//	┌─────────────────────────────────────────────────┐
//	│                 UnifiedFlow                      │
//	│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐  │
//	│  │ L3   │ │ L4   │ │ L7   │ │Proc  │ │ K8s  │  │
//	│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘  │
//	│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐  │
//	│  │Trace │ │Host  │ │Tenant│ │Metric│ │Custom│  │
//	│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘  │
//	└─────────────────────────────────────────────────┘
package flow

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"unsafe"
)

// ============================================================================
// Schema Versioning
// ============================================================================

const (
	// SchemaV1 初始版本: L3/L4/L7/Process/K8s/Trace/Host/Tenant/Metrics
	SchemaV1 uint32 = 1

	// CurrentSchema 当前 schema 版本
	CurrentSchema = SchemaV1

	// MaxIPBytes IP 地址最大字节数 (IPv6 = 16 bytes)
	MaxIPBytes = 16

	// MaxNameLen 名称字段最大长度
	MaxNameLen = 64

	// MaxPathLen 路径字段最大长度
	MaxPathLen = 256

	// MaxTraceIDLen TraceID 最大长度 (32 hex chars for W3C)
	MaxTraceIDLen = 32

	// MaxTagsCount 标签最大数量
	MaxTagsCount = 16

	// MaxTagKeyLen 标签 key 最大长度
	MaxTagKeyLen = 32

	// MaxTagValueLen 标签 value 最大长度
	MaxTagValueLen = 128
)

// ============================================================================
// Nullable 优化: 存在位图 (Presence Bitmap)
// ============================================================================

// FieldID 字段标识位 (用于 presence bitmap)
type FieldID uint32

const (
	// L3 层字段
	FieldSrcIP       FieldID = 0
	FieldDstIP       FieldID = 1
	FieldIPVersion   FieldID = 2

	// L4 层字段
	FieldSrcPort     FieldID = 3
	FieldDstPort     FieldID = 4
	FieldProtocol    FieldID = 5
	FieldTCPFlags    FieldID = 6

	// L7 层字段
	FieldL7Protocol  FieldID = 7
	FieldMethod      FieldID = 8
	FieldPath        FieldID = 9
	FieldStatusCode  FieldID = 10
	FieldReqSize     FieldID = 11
	FieldRespSize    FieldID = 12

	// Process 字段
	FieldPID         FieldID = 13
	FieldProcessName FieldID = 14
	FieldComm        FieldID = 15

	// Container 字段
	FieldContainerID FieldID = 16
	FieldContainerName FieldID = 17
	FieldImage       FieldID = 18

	// Kubernetes 字段
	FieldPod         FieldID = 19
	FieldNamespace   FieldID = 20
	FieldDeployment  FieldID = 21
	FieldService     FieldID = 22
	FieldNode        FieldID = 23

	// Trace 字段
	FieldTraceID     FieldID = 24
	FieldSpanID      FieldID = 25
	FieldParentID    FieldID = 26

	// Host 字段
	FieldHostID      FieldID = 27
	FieldHostname    FieldID = 28

	// Tenant 字段
	FieldTenantID    FieldID = 29

	// Metrics 字段
	FieldBytes       FieldID = 30
	FieldPackets     FieldID = 31
	FieldLatencyNs   FieldID = 32
	FieldDirection   FieldID = 33

	// Custom 字段
	FieldTags        FieldID = 34
	FieldCustom      FieldID = 35

	// 总字段数
	FieldCount       = 36
)

// Presence 存在位图 (36 字段 = 2 x uint64)
// 使用 uint64 数组，每个 bit 表示一个字段是否设置
type Presence [2]uint64

// Set 设置字段存在
func (p *Presence) Set(f FieldID) {
	p[f/64] |= 1 << (f % 64)
}

// IsSet 检查字段是否存在
func (p *Presence) IsSet(f FieldID) bool {
	return p[f/64]&(1<<(f%64)) != 0
}

// Clear 清除所有字段
func (p *Presence) Clear() {
	p[0] = 0
	p[1] = 0
}

// Or 合并两个 presence bitmap
func (p *Presence) Or(other Presence) {
	p[0] |= other[0]
	p[1] |= other[1]
}

// ============================================================================
// IP 地址 (统一 IPv4/IPv6)
// ============================================================================

// IP 统一 IP 地址 (16 bytes, 支持 IPv4-mapped IPv6)
// 内存布局: 前 12 bytes 为 IPv4-mapped 前缀 (IPv4 时), 后 4 bytes 为 IPv4 地址
type IP [MaxIPBytes]byte

// IPv4 返回 IPv4 地址 (仅当 IP 为 IPv4-mapped 时有效)
func (ip IP) IPv4() net.IP {
	if ip[10] == 0xff && ip[11] == 0xff {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15])
	}
	return nil
}

// IPv6 返回 IPv6 地址
func (ip IP) IPv6() net.IP {
	return net.IP(ip[:])
}

// String 返回 IP 字符串
func (ip IP) String() string {
	v4 := ip.IPv4()
	if v4 != nil {
		return v4.String()
	}
	return ip.IPv6().String()
}

// IsIPv4 是否为 IPv4 地址
func (ip IP) IsIPv4() bool {
	return ip[10] == 0xff && ip[11] == 0xff &&
		ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] == 0 &&
		ip[4] == 0 && ip[5] == 0 && ip[6] == 0 && ip[7] == 0 &&
		ip[8] == 0 && ip[9] == 0
}

// IsZero 是否为零值
func (ip IP) IsZero() bool {
	for _, b := range ip {
		if b != 0 {
			return false
		}
	}
	return true
}

// ParseIP 从字符串解析 IP
func ParseIP(s string) IP {
	ip := IP{}
	parsed := net.ParseIP(s)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		// IPv4-mapped IPv6
		ip[10] = 0xff
		ip[11] = 0xff
		copy(ip[12:16], v4)
	} else {
		copy(ip[:], parsed.To16())
	}
	return ip
}

// ParseIPv4 从 4 字节数组解析 (eBPF 层零拷贝)
func ParseIPv4(b [4]byte) IP {
	ip := IP{}
	ip[10] = 0xff
	ip[11] = 0xff
	copy(ip[12:16], b[:])
	return ip
}

// ============================================================================
// FixedString 定长字符串 (避免动态分配)
// ============================================================================

// FixedString64 定长字符串 (64 bytes)
type FixedString64 [MaxNameLen]byte

func (s FixedString64) String() string {
	n := strings.IndexByte(string(s[:]), 0)
	if n < 0 {
		n = MaxNameLen
	}
	return string(s[:n])
}

func (s FixedString64) IsZero() bool {
	return s[0] == 0
}

// Set 设置字符串
func (s *FixedString64) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxNameLen {
		s[len(v)] = 0
	}
}

// FixedString256 定长字符串 (256 bytes)
type FixedString256 [MaxPathLen]byte

func (s FixedString256) String() string {
	n := strings.IndexByte(string(s[:]), 0)
	if n < 0 {
		n = MaxPathLen
	}
	return string(s[:n])
}

func (s FixedString256) IsZero() bool {
	return s[0] == 0
}

func (s *FixedString256) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxPathLen {
		s[len(v)] = 0
	}
}

// FixedString32 定长字符串 (32 bytes, 用于 TraceID)
type FixedString32 [MaxTraceIDLen]byte

func (s FixedString32) String() string {
	n := strings.IndexByte(string(s[:]), 0)
	if n < 0 {
		n = MaxTraceIDLen
	}
	return string(s[:n])
}

func (s FixedString32) IsZero() bool {
	return s[0] == 0
}

func (s *FixedString32) Set(v string) {
	copy(s[:], []byte(v))
	if len(v) < MaxTraceIDLen {
		s[len(v)] = 0
	}
}

// ============================================================================
// Tag 定长键值对 (避免 map[string]string)
// ============================================================================

// Tag 定长标签
type Tag struct {
	Key   [MaxTagKeyLen]byte
	Value [MaxTagValueLen]byte
}

func (t Tag) GetKey() string {
	n := strings.IndexByte(string(t.Key[:]), 0)
	if n < 0 {
		n = MaxTagKeyLen
	}
	return string(t.Key[:n])
}

func (t Tag) GetValue() string {
	n := strings.IndexByte(string(t.Value[:]), 0)
	if n < 0 {
		n = MaxTagValueLen
	}
	return string(t.Value[:n])
}

func (t *Tag) Set(key, value string) {
	copy(t.Key[:], []byte(key))
	if len(key) < MaxTagKeyLen {
		t.Key[len(key)] = 0
	}
	copy(t.Value[:], []byte(value))
	if len(value) < MaxTagValueLen {
		t.Value[len(value)] = 0
	}
}

// Tags 固定大小标签数组
type Tags [MaxTagsCount]Tag

func (t *Tags) Set(key, value string) {
	for i := range t {
		if t[i].GetKey() == "" || t[i].GetKey() == key {
			t[i].Set(key, value)
			return
		}
	}
}

func (t *Tags) Get(key string) string {
	for i := range t {
		if t[i].GetKey() == key {
			return t[i].GetValue()
		}
	}
	return ""
}

func (t *Tags) Count() int {
	count := 0
	for i := range t {
		if t[i].GetKey() != "" {
			count++
		}
	}
	return count
}

// ============================================================================
// Protocol 协议枚举 (uint8, 零值安全)
// ============================================================================

// Protocol 网络协议 (L4 + L7)
type Protocol uint8

const (
	ProtoUnknown Protocol = iota
	ProtoTCP
	ProtoUDP
	ProtoICMP
	ProtoHTTP
	ProtoHTTP2
	ProtoGRPC
	ProtoDNS
	ProtoMySQL
	ProtoRedis
	ProtoKafka
	ProtoMax
)

func (p Protocol) String() string {
	switch p {
	case ProtoTCP:
		return "tcp"
	case ProtoUDP:
		return "udp"
	case ProtoICMP:
		return "icmp"
	case ProtoHTTP:
		return "http"
	case ProtoHTTP2:
		return "http2"
	case ProtoGRPC:
		return "grpc"
	case ProtoDNS:
		return "dns"
	case ProtoMySQL:
		return "mysql"
	case ProtoRedis:
		return "redis"
	case ProtoKafka:
		return "kafka"
	default:
		return "unknown"
	}
}

// FromIPProtocol 从 IP 协议号转换 (6=TCP, 17=UDP, 1=ICMP)
func FromIPProtocol(n uint8) Protocol {
	switch n {
	case 6:
		return ProtoTCP
	case 17:
		return ProtoUDP
	case 1:
		return ProtoICMP
	default:
		return ProtoUnknown
	}
}

// Direction 流方向
type Direction uint8

const (
	DirUnknown  Direction = iota
	DirIngress          // 入站
	DirEgress           // 出站
	DirInternal         // 内部
)

// ============================================================================
// UnifiedFlow 统一流量数据模型
// ============================================================================

// UnifiedFlow 统一流量数据模型
//
// 内存布局 (按 cache line 64 bytes 对齐):
//
//	Offset  Size    Field
//	0       8       Timestamp (int64)
//	8       4       SchemaVersion (uint32)
//	12      4       FlowID (uint32)
//	16      2       Presence bitmap [0] (uint16, 前 16 个字段)
//	18      2       Presence bitmap [1] (uint16, 后 20 个字段)
//	20      2       Presence bitmap [2] (uint16, 扩展)
//	22      2       _padding
//	--- L3 (24 bytes) ---
//	24      16      SrcIP (IP, 16 bytes)
//	40      16      DstIP (IP, 16 bytes)
//	56      1       IPVersion (uint8: 4 or 6)
//	57      7       _padding
//	--- L4 (16 bytes) ---
//	64      2       SrcPort (uint16)
//	66      2       DstPort (uint16)
//	68      1       Protocol (uint8)
//	69      1       TCPFlags (uint8)
//	70      2       _padding
//	--- L7 (280 bytes) ---
//	72      1       L7Protocol (uint8)
//	73      1       HTTPMethod (uint8)
//	74      2       StatusCode (uint16)
//	76      8       ReqSize (uint64)
//	84      8       RespSize (uint64)
//	92      256     Path (FixedString256)
//	--- Process (140 bytes) ---
//	348     4       PID (uint32)
//	352     64      ProcessName (FixedString64)
//	416     64      Comm (FixedString64)
//	--- Container (192 bytes) ---
//	480     64      ContainerID (FixedString64)
//	544     64      ContainerName (FixedString64)
//	608     64      Image (FixedString64)
//	--- Kubernetes (320 bytes) ---
//	672     64      Pod (FixedString64)
//	736     64      Namespace (FixedString64)
//	800     64      Deployment (FixedString64)
//	864     64      Service (FixedString64)
//	928     64      Node (FixedString64)
//	--- Trace (96 bytes) ---
//	992     32      TraceID (FixedString32)
//	1024    32      SpanID (FixedString32)
//	1056    32      ParentID (FixedString32)
//	--- Host (128 bytes) ---
//	1088    64      HostID (FixedString64)
//	1152    64      Hostname (FixedString64)
//	--- Tenant (64 bytes) ---
//	1216    64      TenantID (FixedString64)
//	--- Metrics (32 bytes) ---
//	1280    8       Bytes (uint64)
//	1288    8       Packets (uint64)
//	1296    8       LatencyNs (uint64)
//	1304    1       Direction (uint8)
//	1305    7       _padding
//	--- Tags (3232 bytes) ---
//	1312    3232    Tags [16]Tag (16 * 202 bytes)
//	---
//	Total: ~4544 bytes
type UnifiedFlow struct {
	// ---- Header ----
	Timestamp     int64    // 纳秒时间戳
	SchemaVersion uint32   // Schema 版本 (用于 versioning)
	FlowID        uint32   // 流唯一 ID (hash)
	Presence      [3]uint16 // 存在位图 (最多 48 个字段)

	// ---- L3: 网络层 ----
	SrcIP     IP     // 源 IP (16 bytes, IPv4-mapped IPv6)
	DstIP     IP     // 目的 IP
	IPVersion uint8  // IP 版本: 4 or 6

	// ---- L4: 传输层 ----
	SrcPort   uint16   // 源端口
	DstPort   uint16   // 目的端口
	Protocol  Protocol // L4 协议
	TCPFlags  uint8    // TCP 标志位 (SYN/ACK/FIN/RST/PSH/URG)

	// ---- L7: 应用层 ----
	L7Protocol Protocol // L7 协议 (HTTP/gRPC/DNS/MySQL/...)
	Method     uint8    // HTTP 方法 (GET/POST/...)
	Path       FixedString256 // URL 路径
	StatusCode uint16   // HTTP/gRPC 状态码
	ReqSize    uint64   // 请求大小 (bytes)
	RespSize   uint64   // 响应大小 (bytes)

	// ---- Process ----
	PID         uint32         // 进程 ID
	ProcessName FixedString64  // 进程名
	Comm        FixedString64  // 进程 comm

	// ---- Container ----
	ContainerID   FixedString64 // 容器 ID
	ContainerName FixedString64 // 容器名
	Image         FixedString64 // 镜像名

	// ---- Kubernetes ----
	Pod        FixedString64 // Pod 名称
	Namespace  FixedString64 // K8s 命名空间
	Deployment FixedString64 // Deployment 名称
	Service    FixedString64 // K8s Service 名称
	Node       FixedString64 // K8s Node 名称

	// ---- Trace ----
	TraceID  FixedString32 // Trace ID (W3C: 32 hex chars)
	SpanID   FixedString32 // Span ID
	ParentID FixedString32 // Parent Span ID

	// ---- Host ----
	HostID   FixedString64 // 主机 ID
	Hostname FixedString64 // 主机名

	// ---- Tenant ----
	TenantID FixedString64 // 租户 ID

	// ---- Metrics ----
	Bytes     uint64    // 字节数
	Packets   uint64    // 包数
	LatencyNs uint64    // 延迟 (纳秒)
	Direction Direction // 方向

	// ---- Tags ----
	Tags Tags // 自定义标签 (最多 16 个)
}

// New 创建新的 UnifiedFlow
func New() *UnifiedFlow {
	f := &UnifiedFlow{
		SchemaVersion: CurrentSchema,
	}
	return f
}

// SetPresent 标记字段存在
func (f *UnifiedFlow) SetPresent(field FieldID) {
	word := field / 16
	bit := field % 16
	f.Presence[word] |= 1 << bit
}

// IsPresent 检查字段是否存在
func (f *UnifiedFlow) IsPresent(field FieldID) bool {
	word := field / 16
	bit := field % 16
	return f.Presence[word]&(1<<bit) != 0
}

// ============================================================================
// Fluent Setter 方法 (链式调用)
// ============================================================================

// SetTimestamp 设置时间戳
func (f *UnifiedFlow) SetTimestamp(ns int64) *UnifiedFlow {
	f.Timestamp = ns
	return f
}

// SetL3 设置 L3 层字段
func (f *UnifiedFlow) SetL3(srcIP, dstIP string) *UnifiedFlow {
	f.SrcIP = ParseIP(srcIP)
	f.DstIP = ParseIP(dstIP)
	if f.SrcIP.IsIPv4() {
		f.IPVersion = 4
	} else {
		f.IPVersion = 6
	}
	f.SetPresent(FieldSrcIP)
	f.SetPresent(FieldDstIP)
	f.SetPresent(FieldIPVersion)
	return f
}

// SetL3IPv4 从 4 字节数组设置 L3 层 (eBPF 零拷贝)
func (f *UnifiedFlow) SetL3IPv4(src, dst [4]byte) *UnifiedFlow {
	f.SrcIP = ParseIPv4(src)
	f.DstIP = ParseIPv4(dst)
	f.IPVersion = 4
	f.SetPresent(FieldSrcIP)
	f.SetPresent(FieldDstIP)
	f.SetPresent(FieldIPVersion)
	return f
}

// SetL4 设置 L4 层字段
func (f *UnifiedFlow) SetL4(srcPort, dstPort uint16, proto Protocol, tcpFlags uint8) *UnifiedFlow {
	f.SrcPort = srcPort
	f.DstPort = dstPort
	f.Protocol = proto
	f.TCPFlags = tcpFlags
	f.SetPresent(FieldSrcPort)
	f.SetPresent(FieldDstPort)
	f.SetPresent(FieldProtocol)
	if tcpFlags != 0 {
		f.SetPresent(FieldTCPFlags)
	}
	return f
}

// SetL7 设置 L7 层字段
func (f *UnifiedFlow) SetL7(proto Protocol, method uint8, path string, statusCode uint16) *UnifiedFlow {
	f.L7Protocol = proto
	f.Method = method
	f.Path.Set(path)
	f.StatusCode = statusCode
	f.SetPresent(FieldL7Protocol)
	f.SetPresent(FieldMethod)
	f.SetPresent(FieldPath)
	f.SetPresent(FieldStatusCode)
	return f
}

// SetL7Sizes 设置 L7 请求/响应大小
func (f *UnifiedFlow) SetL7Sizes(reqSize, respSize uint64) *UnifiedFlow {
	f.ReqSize = reqSize
	f.RespSize = respSize
	f.SetPresent(FieldReqSize)
	f.SetPresent(FieldRespSize)
	return f
}

// SetProcess 设置进程信息
func (f *UnifiedFlow) SetProcess(pid uint32, name, comm string) *UnifiedFlow {
	f.PID = pid
	f.ProcessName.Set(name)
	f.Comm.Set(comm)
	f.SetPresent(FieldPID)
	f.SetPresent(FieldProcessName)
	f.SetPresent(FieldComm)
	return f
}

// SetContainer 设置容器信息
func (f *UnifiedFlow) SetContainer(id, name, image string) *UnifiedFlow {
	f.ContainerID.Set(id)
	f.ContainerName.Set(name)
	f.Image.Set(image)
	f.SetPresent(FieldContainerID)
	f.SetPresent(FieldContainerName)
	f.SetPresent(FieldImage)
	return f
}

// SetKubernetes 设置 Kubernetes 信息
func (f *UnifiedFlow) SetK8s(pod, ns, deploy, svc, node string) *UnifiedFlow {
	f.Pod.Set(pod)
	f.Namespace.Set(ns)
	f.Deployment.Set(deploy)
	f.Service.Set(svc)
	f.Node.Set(node)
	f.SetPresent(FieldPod)
	f.SetPresent(FieldNamespace)
	f.SetPresent(FieldDeployment)
	f.SetPresent(FieldService)
	f.SetPresent(FieldNode)
	return f
}

// SetTrace 设置 Trace 信息
func (f *UnifiedFlow) SetTrace(traceID, spanID, parentID string) *UnifiedFlow {
	f.TraceID.Set(traceID)
	f.SpanID.Set(spanID)
	f.ParentID.Set(parentID)
	f.SetPresent(FieldTraceID)
	f.SetPresent(FieldSpanID)
	if parentID != "" {
		f.SetPresent(FieldParentID)
	}
	return f
}

// SetHost 设置主机信息
func (f *UnifiedFlow) SetHost(hostID, hostname string) *UnifiedFlow {
	f.HostID.Set(hostID)
	f.Hostname.Set(hostname)
	f.SetPresent(FieldHostID)
	f.SetPresent(FieldHostname)
	return f
}

// SetTenant 设置租户信息
func (f *UnifiedFlow) SetTenant(tenantID string) *UnifiedFlow {
	f.TenantID.Set(tenantID)
	f.SetPresent(FieldTenantID)
	return f
}

// SetMetrics 设置指标
func (f *UnifiedFlow) SetMetrics(bytes, packets, latencyNs uint64, dir Direction) *UnifiedFlow {
	f.Bytes = bytes
	f.Packets = packets
	f.LatencyNs = latencyNs
	f.Direction = dir
	f.SetPresent(FieldBytes)
	f.SetPresent(FieldPackets)
	f.SetPresent(FieldLatencyNs)
	f.SetPresent(FieldDirection)
	return f
}

// SetTag 设置自定义标签
func (f *UnifiedFlow) SetTag(key, value string) *UnifiedFlow {
	f.Tags.Set(key, value)
	f.SetPresent(FieldTags)
	return f
}

// ============================================================================
// 序列化/反序列化 (二进制格式，非 JSON)
// ============================================================================

// Serialize 序列化为二进制格式
// 格式: [schema_version:4][presence:6][body:variable]
func (f *UnifiedFlow) Serialize() []byte {
	// 计算实际需要的 body 大小
	bodySize := f.serializedBodySize()
	buf := make([]byte, 10+bodySize) // 4 version + 6 presence + body

	binary.BigEndian.PutUint32(buf[0:4], f.SchemaVersion)
	binary.BigEndian.PutUint16(buf[4:6], f.Presence[0])
	binary.BigEndian.PutUint16(buf[6:8], f.Presence[1])
	binary.BigEndian.PutUint16(buf[8:10], f.Presence[2])

	offset := 10
	offset = f.serializeBody(buf[offset:])

	return buf[:offset]
}

// Deserialize 从二进制格式反序列化
func (f *UnifiedFlow) Deserialize(data []byte) error {
	if len(data) < 10 {
		return ErrInvalidData
	}

	f.SchemaVersion = binary.BigEndian.Uint32(data[0:4])
	f.Presence[0] = binary.BigEndian.Uint16(data[4:6])
	f.Presence[1] = binary.BigEndian.Uint16(data[6:8])
	f.Presence[2] = binary.BigEndian.Uint16(data[8:10])

	return f.deserializeBody(data[10:])
}

// serializedBodySize 计算序列化后的 body 大小
func (f *UnifiedFlow) serializedBodySize() int {
	size := 0

	// Timestamp + FlowID
	size += 8 + 4

	// L3
	if f.IsPresent(FieldSrcIP) {
		size += MaxIPBytes
	}
	if f.IsPresent(FieldDstIP) {
		size += MaxIPBytes
	}
	if f.IsPresent(FieldIPVersion) {
		size += 1
	}

	// L4
	if f.IsPresent(FieldSrcPort) {
		size += 2
	}
	if f.IsPresent(FieldDstPort) {
		size += 2
	}
	if f.IsPresent(FieldProtocol) {
		size += 1
	}
	if f.IsPresent(FieldTCPFlags) {
		size += 1
	}

	// L7
	if f.IsPresent(FieldL7Protocol) {
		size += 1
	}
	if f.IsPresent(FieldMethod) {
		size += 1
	}
	if f.IsPresent(FieldStatusCode) {
		size += 2
	}
	if f.IsPresent(FieldReqSize) {
		size += 8
	}
	if f.IsPresent(FieldRespSize) {
		size += 8
	}
	if f.IsPresent(FieldPath) {
		size += MaxPathLen
	}

	// Process
	if f.IsPresent(FieldPID) {
		size += 4
	}
	if f.IsPresent(FieldProcessName) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldComm) {
		size += MaxNameLen
	}

	// Container
	if f.IsPresent(FieldContainerID) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldContainerName) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldImage) {
		size += MaxNameLen
	}

	// K8s
	if f.IsPresent(FieldPod) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldNamespace) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldDeployment) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldService) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldNode) {
		size += MaxNameLen
	}

	// Trace
	if f.IsPresent(FieldTraceID) {
		size += MaxTraceIDLen
	}
	if f.IsPresent(FieldSpanID) {
		size += MaxTraceIDLen
	}
	if f.IsPresent(FieldParentID) {
		size += MaxTraceIDLen
	}

	// Host
	if f.IsPresent(FieldHostID) {
		size += MaxNameLen
	}
	if f.IsPresent(FieldHostname) {
		size += MaxNameLen
	}

	// Tenant
	if f.IsPresent(FieldTenantID) {
		size += MaxNameLen
	}

	// Metrics
	if f.IsPresent(FieldBytes) {
		size += 8
	}
	if f.IsPresent(FieldPackets) {
		size += 8
	}
	if f.IsPresent(FieldLatencyNs) {
		size += 8
	}
	if f.IsPresent(FieldDirection) {
		size += 1
	}

	return size
}

// serializeBody 序列化 body
func (f *UnifiedFlow) serializeBody(buf []byte) int {
	offset := 0

	// Timestamp + FlowID
	binary.BigEndian.PutUint64(buf[offset:], uint64(f.Timestamp))
	offset += 8
	binary.BigEndian.PutUint32(buf[offset:], f.FlowID)
	offset += 4

	// L3
	if f.IsPresent(FieldSrcIP) {
		copy(buf[offset:], f.SrcIP[:])
		offset += MaxIPBytes
	}
	if f.IsPresent(FieldDstIP) {
		copy(buf[offset:], f.DstIP[:])
		offset += MaxIPBytes
	}
	if f.IsPresent(FieldIPVersion) {
		buf[offset] = f.IPVersion
		offset++
	}

	// L4
	if f.IsPresent(FieldSrcPort) {
		binary.BigEndian.PutUint16(buf[offset:], f.SrcPort)
		offset += 2
	}
	if f.IsPresent(FieldDstPort) {
		binary.BigEndian.PutUint16(buf[offset:], f.DstPort)
		offset += 2
	}
	if f.IsPresent(FieldProtocol) {
		buf[offset] = uint8(f.Protocol)
		offset++
	}
	if f.IsPresent(FieldTCPFlags) {
		buf[offset] = f.TCPFlags
		offset++
	}

	// L7
	if f.IsPresent(FieldL7Protocol) {
		buf[offset] = uint8(f.L7Protocol)
		offset++
	}
	if f.IsPresent(FieldMethod) {
		buf[offset] = f.Method
		offset++
	}
	if f.IsPresent(FieldStatusCode) {
		binary.BigEndian.PutUint16(buf[offset:], f.StatusCode)
		offset += 2
	}
	if f.IsPresent(FieldReqSize) {
		binary.BigEndian.PutUint64(buf[offset:], f.ReqSize)
		offset += 8
	}
	if f.IsPresent(FieldRespSize) {
		binary.BigEndian.PutUint64(buf[offset:], f.RespSize)
		offset += 8
	}
	if f.IsPresent(FieldPath) {
		copy(buf[offset:], f.Path[:])
		offset += MaxPathLen
	}

	// Process
	if f.IsPresent(FieldPID) {
		binary.BigEndian.PutUint32(buf[offset:], f.PID)
		offset += 4
	}
	if f.IsPresent(FieldProcessName) {
		copy(buf[offset:], f.ProcessName[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldComm) {
		copy(buf[offset:], f.Comm[:])
		offset += MaxNameLen
	}

	// Container
	if f.IsPresent(FieldContainerID) {
		copy(buf[offset:], f.ContainerID[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldContainerName) {
		copy(buf[offset:], f.ContainerName[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldImage) {
		copy(buf[offset:], f.Image[:])
		offset += MaxNameLen
	}

	// K8s
	if f.IsPresent(FieldPod) {
		copy(buf[offset:], f.Pod[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldNamespace) {
		copy(buf[offset:], f.Namespace[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldDeployment) {
		copy(buf[offset:], f.Deployment[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldService) {
		copy(buf[offset:], f.Service[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldNode) {
		copy(buf[offset:], f.Node[:])
		offset += MaxNameLen
	}

	// Trace
	if f.IsPresent(FieldTraceID) {
		copy(buf[offset:], f.TraceID[:])
		offset += MaxTraceIDLen
	}
	if f.IsPresent(FieldSpanID) {
		copy(buf[offset:], f.SpanID[:])
		offset += MaxTraceIDLen
	}
	if f.IsPresent(FieldParentID) {
		copy(buf[offset:], f.ParentID[:])
		offset += MaxTraceIDLen
	}

	// Host
	if f.IsPresent(FieldHostID) {
		copy(buf[offset:], f.HostID[:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldHostname) {
		copy(buf[offset:], f.Hostname[:])
		offset += MaxNameLen
	}

	// Tenant
	if f.IsPresent(FieldTenantID) {
		copy(buf[offset:], f.TenantID[:])
		offset += MaxNameLen
	}

	// Metrics
	if f.IsPresent(FieldBytes) {
		binary.BigEndian.PutUint64(buf[offset:], f.Bytes)
		offset += 8
	}
	if f.IsPresent(FieldPackets) {
		binary.BigEndian.PutUint64(buf[offset:], f.Packets)
		offset += 8
	}
	if f.IsPresent(FieldLatencyNs) {
		binary.BigEndian.PutUint64(buf[offset:], f.LatencyNs)
		offset += 8
	}
	if f.IsPresent(FieldDirection) {
		buf[offset] = uint8(f.Direction)
		offset++
	}

	return offset
}

// deserializeBody 反序列化 body
func (f *UnifiedFlow) deserializeBody(data []byte) error {
	offset := 0

	// Timestamp + FlowID
	if len(data) < 12 {
		return ErrInvalidData
	}
	f.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8
	f.FlowID = binary.BigEndian.Uint32(data[offset:])
	offset += 4

	// L3
	if f.IsPresent(FieldSrcIP) {
		if offset+MaxIPBytes > len(data) {
			return ErrInvalidData
		}
		copy(f.SrcIP[:], data[offset:])
		offset += MaxIPBytes
	}
	if f.IsPresent(FieldDstIP) {
		if offset+MaxIPBytes > len(data) {
			return ErrInvalidData
		}
		copy(f.DstIP[:], data[offset:])
		offset += MaxIPBytes
	}
	if f.IsPresent(FieldIPVersion) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.IPVersion = data[offset]
		offset++
	}

	// L4
	if f.IsPresent(FieldSrcPort) {
		if offset+2 > len(data) {
			return ErrInvalidData
		}
		f.SrcPort = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}
	if f.IsPresent(FieldDstPort) {
		if offset+2 > len(data) {
			return ErrInvalidData
		}
		f.DstPort = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}
	if f.IsPresent(FieldProtocol) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.Protocol = Protocol(data[offset])
		offset++
	}
	if f.IsPresent(FieldTCPFlags) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.TCPFlags = data[offset]
		offset++
	}

	// L7
	if f.IsPresent(FieldL7Protocol) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.L7Protocol = Protocol(data[offset])
		offset++
	}
	if f.IsPresent(FieldMethod) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.Method = data[offset]
		offset++
	}
	if f.IsPresent(FieldStatusCode) {
		if offset+2 > len(data) {
			return ErrInvalidData
		}
		f.StatusCode = binary.BigEndian.Uint16(data[offset:])
		offset += 2
	}
	if f.IsPresent(FieldReqSize) {
		if offset+8 > len(data) {
			return ErrInvalidData
		}
		f.ReqSize = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}
	if f.IsPresent(FieldRespSize) {
		if offset+8 > len(data) {
			return ErrInvalidData
		}
		f.RespSize = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}
	if f.IsPresent(FieldPath) {
		if offset+MaxPathLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Path[:], data[offset:])
		offset += MaxPathLen
	}

	// Process
	if f.IsPresent(FieldPID) {
		if offset+4 > len(data) {
			return ErrInvalidData
		}
		f.PID = binary.BigEndian.Uint32(data[offset:])
		offset += 4
	}
	if f.IsPresent(FieldProcessName) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.ProcessName[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldComm) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Comm[:], data[offset:])
		offset += MaxNameLen
	}

	// Container
	if f.IsPresent(FieldContainerID) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.ContainerID[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldContainerName) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.ContainerName[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldImage) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Image[:], data[offset:])
		offset += MaxNameLen
	}

	// K8s
	if f.IsPresent(FieldPod) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Pod[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldNamespace) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Namespace[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldDeployment) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Deployment[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldService) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Service[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldNode) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Node[:], data[offset:])
		offset += MaxNameLen
	}

	// Trace
	if f.IsPresent(FieldTraceID) {
		if offset+MaxTraceIDLen > len(data) {
			return ErrInvalidData
		}
		copy(f.TraceID[:], data[offset:])
		offset += MaxTraceIDLen
	}
	if f.IsPresent(FieldSpanID) {
		if offset+MaxTraceIDLen > len(data) {
			return ErrInvalidData
		}
		copy(f.SpanID[:], data[offset:])
		offset += MaxTraceIDLen
	}
	if f.IsPresent(FieldParentID) {
		if offset+MaxTraceIDLen > len(data) {
			return ErrInvalidData
		}
		copy(f.ParentID[:], data[offset:])
		offset += MaxTraceIDLen
	}

	// Host
	if f.IsPresent(FieldHostID) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.HostID[:], data[offset:])
		offset += MaxNameLen
	}
	if f.IsPresent(FieldHostname) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.Hostname[:], data[offset:])
		offset += MaxNameLen
	}

	// Tenant
	if f.IsPresent(FieldTenantID) {
		if offset+MaxNameLen > len(data) {
			return ErrInvalidData
		}
		copy(f.TenantID[:], data[offset:])
		offset += MaxNameLen
	}

	// Metrics
	if f.IsPresent(FieldBytes) {
		if offset+8 > len(data) {
			return ErrInvalidData
		}
		f.Bytes = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}
	if f.IsPresent(FieldPackets) {
		if offset+8 > len(data) {
			return ErrInvalidData
		}
		f.Packets = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}
	if f.IsPresent(FieldLatencyNs) {
		if offset+8 > len(data) {
			return ErrInvalidData
		}
		f.LatencyNs = binary.BigEndian.Uint64(data[offset:])
		offset += 8
	}
	if f.IsPresent(FieldDirection) {
		if offset+1 > len(data) {
			return ErrInvalidData
		}
		f.Direction = Direction(data[offset])
		offset++
	}

	return nil
}

// ============================================================================
// Size / Alignment 信息
// ============================================================================

// SizeOf 返回 UnifiedFlow 结构体大小
func SizeOf() int {
	return int(unsafe.Sizeof(UnifiedFlow{}))
}

// ErrInvalidData 无效数据错误
var ErrInvalidData = &FlowError{Code: "invalid_data", Message: "invalid data format"}

// FlowError 流错误
type FlowError struct {
	Code    string
	Message string
}

func (e *FlowError) Error() string {
	return e.Code + ": " + e.Message
}

// ============================================================================
// 辅助函数
// ============================================================================

// ParsePort 解析端口号字符串
func ParsePort(s string) uint16 {
	n, _ := strconv.ParseUint(s, 10, 16)
	return uint16(n)
}

// FormatPort 格式化端口号
func FormatPort(p uint16) string {
	return strconv.FormatUint(uint64(p), 10)
}

// ParseProtocol 解析协议字符串
func ParseProtocol(s string) Protocol {
	switch strings.ToLower(s) {
	case "tcp":
		return ProtoTCP
	case "udp":
		return ProtoUDP
	case "icmp":
		return ProtoICMP
	case "http":
		return ProtoHTTP
	case "http2":
		return ProtoHTTP2
	case "grpc":
		return ProtoGRPC
	case "dns":
		return ProtoDNS
	case "mysql":
		return ProtoMySQL
	case "redis":
		return ProtoRedis
	case "kafka":
		return ProtoKafka
	default:
		return ProtoUnknown
	}
}
