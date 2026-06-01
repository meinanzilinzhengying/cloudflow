// Package flow 提供 Protobuf Schema 定义和 FlowRouter
package flow

import (
	"fmt"
	"strings"
)

// ============================================================================
// Protobuf Schema (.proto 文本格式)
// ============================================================================
//
// syntax = "proto3";
// package cloudflow.flow;
//
// option go_package = "cloud-flow/pkg/flow";
// option java_package = "com.cloudflow.flow";
//
// import "google/protobuf/timestamp.proto";
// import "google/protobuf/duration.proto";
//
// // ====================================================================
// // Versioning: 所有消息包含 schema_version 字段
// // 规则:
// //   - 新增字段使用 reserved field number
// //   - 废弃字段标记 [deprecated = true]
// //   - 删除字段保留 reserved 声明
// //   - uint32 schema_version 用于前向/后向兼容
// // ====================================================================
//
// // UnifiedFlow 统一流量数据模型
// message UnifiedFlow {
//   // --- Header ---
//   uint32 schema_version = 1;    // Schema 版本 (当前=1)
//   uint64 timestamp_ns  = 2;     // 纳秒时间戳
//   uint32 flow_id       = 3;     // 流唯一 ID (5-tuple hash)
//
//   // --- L3: 网络层 ---
//   bytes  src_ip        = 10;    // 源 IP (4 or 16 bytes)
//   bytes  dst_ip        = 11;    // 目的 IP (4 or 16 bytes)
//   uint32 ip_version    = 12;    // IP 版本: 4 or 6
//
//   // --- L4: 传输层 ---
//   uint32 src_port      = 20;    // 源端口
//   uint32 dst_port      = 21;    // 目的端口
//   uint32 protocol      = 22;    // L4 协议 (IP protocol number)
//   uint32 tcp_flags     = 23;    // TCP 标志位
//
//   // --- L7: 应用层 ---
//   uint32 l7_protocol   = 30;    // L7 协议枚举
//   uint32 method        = 31;    // HTTP 方法
//   string path          = 32;    // URL 路径
//   uint32 status_code   = 33;    // HTTP 状态码
//   uint64 req_size      = 34;    // 请求大小
//   uint64 resp_size     = 35;    // 响应大小
//
//   // --- Process ---
//   uint32 pid           = 40;    // 进程 ID
//   string process_name  = 41;    // 进程名
//   string comm          = 42;    // 进程 comm
//
//   // --- Container ---
//   string container_id   = 50;   // 容器 ID
//   string container_name = 51;   // 容器名
//   string image           = 52;   // 镜像名
//
//   // --- Kubernetes ---
//   string pod           = 60;    // Pod 名称
//   string namespace     = 61;    // K8s 命名空间
//   string deployment    = 62;    // Deployment 名称
//   string service       = 63;    // K8s Service
//   string node          = 64;    // K8s Node
//
//   // --- Trace ---
//   string trace_id      = 70;    // Trace ID (W3C: 32 hex)
//   string span_id       = 71;    // Span ID
//   string parent_id     = 72;    // Parent Span ID
//
//   // --- Host ---
//   string host_id       = 80;    // 主机 ID
//   string hostname      = 81;    // 主机名
//
//   // --- Tenant ---
//   string tenant_id     = 90;    // 租户 ID
//
//   // --- Metrics ---
//   uint64 bytes         = 100;   // 字节数
//   uint64 packets       = 101;   // 包数
//   uint64 latency_ns    = 102;   // 延迟 (纳秒)
//   uint32 direction     = 103;   // 方向: 0=unknown, 1=ingress, 2=egress
//
//   // --- Tags ---
//   repeated Tag tags   = 110;    // 自定义标签
//
//   // --- Reserved (未来扩展) ---
//   reserved 200 to 299;
// }
//
// message Tag {
//   string key   = 1;
//   string value = 2;
// }
//
// // FlowBatch 流数据批量
// message FlowBatch {
//   uint32 schema_version = 1;
//   string probe_id        = 2;
//   string asset_id        = 3;
//   uint32 count           = 4;
//   uint64 seq_id          = 5;
//   uint64 timestamp_ns    = 6;
//   string checksum        = 7;
//   FlowDataType type      = 8;
//   repeated UnifiedFlow flows = 10;
// }
//
// enum FlowDataType {
//   FLOW_DATA_RAW       = 0;
//   FLOW_DATA_L4        = 1;
//   FLOW_DATA_L7        = 2;
//   FLOW_DATA_METRIC    = 3;
//   FLOW_DATA_TRACE     = 4;
//   FLOW_DATA_LOG       = 5;
//   FLOW_DATA_PROFILING = 6;
// }
//
// // ====================================================================
// // Versioning 策略
// // ====================================================================
// //
// // 1. 字段编号分段:
// //    1-9:    Header (核心元数据，永不删除)
// //    10-19:  L3 (网络层)
// //    20-29:  L4 (传输层)
// //    30-39:  L7 (应用层)
// //    40-49:  Process
// //    50-59:  Container
// //    60-69:  Kubernetes
// //    70-79:  Trace
// //    80-89:  Host
// //    90-99:  Tenant
// //    100-109: Metrics
// //    110-119: Tags
// //    200-299: Reserved (未来扩展)
// //
// // 2. 兼容性规则:
// //    - 新增字段: 使用 reserved 范围内的编号
// //    - 废弃字段: 添加 [deprecated = true] 注释
// //    - 删除字段: 保留 reserved 声明，至少保留 2 个版本
// //    - 类型变更: 新增字段使用新编号，旧字段标记 deprecated
// //
// // 3. schema_version 语义:
// //    - V1: 初始版本 (当前)
// //    - V2: 预留 (新增字段时递增)
// //    - 消费者根据 version 决定如何解析
// //    - 未知 version 的消息应记录告警但不丢弃
// //

// ============================================================================
// ProtoFieldNumber 字段编号常量 (与 proto schema 对应)
// ============================================================================

const (
	// Header
	FieldNumSchemaVersion = 1
	FieldNumTimestamp     = 2
	FieldNumFlowID        = 3

	// L3
	FieldNumSrcIP     = 10
	FieldNumDstIP     = 11
	FieldNumIPVersion = 12

	// L4
	FieldNumSrcPort  = 20
	FieldNumDstPort  = 21
	FieldNumProtocol = 22
	FieldNumTCPFlags = 23

	// L7
	FieldNumL7Protocol = 30
	FieldNumMethod     = 31
	FieldNumPath       = 32
	FieldNumStatusCode = 33
	FieldNumReqSize    = 34
	FieldNumRespSize   = 35

	// Process
	FieldNumPID         = 40
	FieldNumProcessName = 41
	FieldNumComm        = 42

	// Container
	FieldNumContainerID   = 50
	FieldNumContainerName = 51
	FieldNumImage         = 52

	// Kubernetes
	FieldNumPod        = 60
	FieldNumNamespace  = 61
	FieldNumDeployment = 62
	FieldNumService    = 63
	FieldNumNode       = 64

	// Trace
	FieldNumTraceID  = 70
	FieldNumSpanID   = 71
	FieldNumParentID = 72

	// Host
	FieldNumHostID   = 80
	FieldNumHostname = 81

	// Tenant
	FieldNumTenantID = 90

	// Metrics
	FieldNumBytes     = 100
	FieldNumPackets   = 101
	FieldNumLatencyNs = 102
	FieldNumDirection = 103

	// Tags
	FieldNumTags = 110
)

// ============================================================================
// FlowRouter 统一关联路由器
// ============================================================================

// FlowRoute 流路由目标
type FlowRoute uint8

const (
	RouteFlowL4    FlowRoute = iota // 路由到 flow.l4 topic
	RouteFlowL7                      // 路由到 flow.l7 topic
	RouteMetrics                     // 路由到 metrics topic
	RouteTraces                      // 路由到 traces topic
	RouteLogs                        // 路由到 logs topic
	RouteProfiling                   // 路由到 profiling topic
	RouteTopology                    // 路由到 topology topic
	RouteAlerts                      // 路由到 alerts topic
)

func (r FlowRoute) String() string {
	switch r {
	case RouteFlowL4:
		return "flow.l4"
	case RouteFlowL7:
		return "flow.l7"
	case RouteMetrics:
		return "metrics"
	case RouteTraces:
		return "traces"
	case RouteLogs:
		return "logs"
	case RouteProfiling:
		return "profiling"
	case RouteTopology:
		return "topology"
	case RouteAlerts:
		return "alerts"
	default:
		return "flow.raw"
	}
}

// RouteDecision 路由决策
type RouteDecision struct {
	Primary   FlowRoute // 主路由
	Secondary FlowRoute // 次要路由 (可选，用于关联查询)
	Reason    string    // 路由原因
}

// Router 流路由器
type Router struct{}

// NewRouter 创建路由器
func NewRouter() *Router {
	return &Router{}
}

// Route 根据流内容决定路由目标
func (r *Router) Route(f *UnifiedFlow) RouteDecision {
	// 1. 有 TraceID -> 同时路由到 traces + 原始 topic
	if !f.TraceID.IsZero() {
		return RouteDecision{
			Primary:   RouteTraces,
			Secondary: r.routeByProtocol(f),
			Reason:    "has_trace_id",
		}
	}

	// 2. 有 L7 协议 -> 路由到 flow.l7
	if f.L7Protocol != ProtoUnknown {
		return RouteDecision{
			Primary:   RouteFlowL7,
			Secondary: RouteMetrics,
			Reason:    fmt.Sprintf("l7_protocol=%s", f.L7Protocol),
		}
	}

	// 3. 有 L4 协议 -> 路由到 flow.l4
	if f.Protocol == ProtoTCP || f.Protocol == ProtoUDP {
		return RouteDecision{
			Primary:   RouteFlowL4,
			Secondary: RouteMetrics,
			Reason:    fmt.Sprintf("l4_protocol=%s", f.Protocol),
		}
	}

	// 4. 默认 -> 路由到 flow.raw
	return RouteDecision{
		Primary: RouteFlowL4,
		Reason:  "default",
	}
}

// routeByProtocol 根据协议路由
func (r *Router) routeByProtocol(f *UnifiedFlow) FlowRoute {
	switch f.L7Protocol {
	case ProtoHTTP, ProtoHTTP2, ProtoGRPC:
		return RouteFlowL7
	case ProtoDNS:
		return RouteFlowL7
	case ProtoMySQL, ProtoRedis, ProtoKafka:
		return RouteFlowL7
	default:
		return RouteFlowL4
	}
}

// RouteBatch 批量路由
func (r *Router) RouteBatch(batch *FlowBatch) map[FlowRoute][]*UnifiedFlow {
	routes := make(map[FlowRoute][]*UnifiedFlow)

	for _, f := range batch.Flows {
		decision := r.Route(f)
		routes[decision.Primary] = append(routes[decision.Primary], f)
		if decision.Secondary != 0 {
			routes[decision.Secondary] = append(routes[decision.Secondary], f)
		}
	}

	return routes
}

// ============================================================================
// FlowCorrelator 流关联器
// ============================================================================

// CorrelationKey 关联键
type CorrelationKey struct {
	TraceID string
	FlowID  uint32
}

// Correlator 流关联器
type Correlator struct{}

// NewCorrelator 创建关联器
func NewCorrelator() *Correlator {
	return &Correlator{}
}

// CorrelateMetrics 将指标关联到流
func (c *Correlator) CorrelateMetrics(f *UnifiedFlow) CorrelationKey {
	return CorrelationKey{
		TraceID: f.TraceID.String(),
		FlowID:  f.FlowID,
	}
}

// CorrelateLogs 将日志关联到流
func (c *Correlator) CorrelateLogs(f *UnifiedFlow) CorrelationKey {
	return CorrelationKey{
		TraceID: f.TraceID.String(),
		FlowID:  f.FlowID,
	}
}

// CorrelateTraces 将 trace 关联到流
func (c *Correlator) CorrelateTraces(f *UnifiedFlow) CorrelationKey {
	return CorrelationKey{
		TraceID: f.TraceID.String(),
		FlowID:  f.FlowID,
	}
}

// ============================================================================
// Schema Registry (版本管理)
// ============================================================================

// SchemaInfo Schema 信息
type SchemaInfo struct {
	Version     uint32
	Description string
	Fields      []FieldInfo
}

// FieldInfo 字段信息
type FieldInfo struct {
	Number    uint32
	Name      string
	Type      string
	Deprecated bool
}

// SchemaRegistry Schema 注册表
type SchemaRegistry struct {
	schemas map[uint32]SchemaInfo
}

// NewSchemaRegistry 创建 Schema 注册表
func NewSchemaRegistry() *SchemaRegistry {
	r := &SchemaRegistry{
		schemas: make(map[uint32]SchemaInfo),
	}
	r.registerV1()
	return r
}

// Register 注册 Schema
func (r *SchemaRegistry) Register(info SchemaInfo) {
	r.schemas[info.Version] = info
}

// Get 获取 Schema
func (r *SchemaRegistry) Get(version uint32) (SchemaInfo, bool) {
	info, ok := r.schemas[version]
	return info, ok
}

// Current 获取当前 Schema
func (r *SchemaRegistry) Current() SchemaInfo {
	return r.schemas[CurrentSchema]
}

// IsCompatible 检查版本兼容性
func (r *SchemaRegistry) IsCompatible(version uint32) bool {
	// 当前只支持 V1
	_, ok := r.schemas[version]
	return ok
}

// registerV1 注册 V1 Schema
func (r *SchemaRegistry) registerV1() {
	r.Register(SchemaInfo{
		Version:     SchemaV1,
		Description: "Initial unified flow model",
		Fields: []FieldInfo{
			{1, "schema_version", "uint32", false},
			{2, "timestamp_ns", "int64", false},
			{3, "flow_id", "uint32", false},
			{10, "src_ip", "bytes", false},
			{11, "dst_ip", "bytes", false},
			{12, "ip_version", "uint32", false},
			{20, "src_port", "uint32", false},
			{21, "dst_port", "uint32", false},
			{22, "protocol", "uint32", false},
			{23, "tcp_flags", "uint32", false},
			{30, "l7_protocol", "uint32", false},
			{31, "method", "uint32", false},
			{32, "path", "string", false},
			{33, "status_code", "uint32", false},
			{34, "req_size", "uint64", false},
			{35, "resp_size", "uint64", false},
			{40, "pid", "uint32", false},
			{41, "process_name", "string", false},
			{42, "comm", "string", false},
			{50, "container_id", "string", false},
			{51, "container_name", "string", false},
			{52, "image", "string", false},
			{60, "pod", "string", false},
			{61, "namespace", "string", false},
			{62, "deployment", "string", false},
			{63, "service", "string", false},
			{64, "node", "string", false},
			{70, "trace_id", "string", false},
			{71, "span_id", "string", false},
			{72, "parent_id", "string", false},
			{80, "host_id", "string", false},
			{81, "hostname", "string", false},
			{90, "tenant_id", "string", false},
			{100, "bytes", "uint64", false},
			{101, "packets", "uint64", false},
			{102, "latency_ns", "uint64", false},
			{103, "direction", "uint32", false},
			{110, "tags", "repeated Tag", false},
		},
	})
}

// ============================================================================
// Proto 文本输出 (用于生成 .proto 文件)
// ============================================================================

// ProtoSchema 返回完整的 .proto 文本
func ProtoSchema() string {
	var b strings.Builder

	b.WriteString(`syntax = "proto3";
package cloudflow.flow;

option go_package = "cloud-flow/pkg/flow";
option java_package = "com.cloudflow.flow";
option csharp_namespace = "CloudFlow.Flow";

import "google/protobuf/timestamp.proto";

// UnifiedFlow 统一流量数据模型
// Schema Version: `)
	b.WriteString(fmt.Sprintf("%d", CurrentSchema))
	b.WriteString(`
message UnifiedFlow {
  // --- Header ---
  uint32 schema_version = 1;    // Schema 版本
  int64  timestamp_ns  = 2;     // 纳秒时间戳
  uint32 flow_id       = 3;     // 流唯一 ID (5-tuple hash)

  // --- L3: 网络层 ---
  bytes  src_ip        = 10;    // 源 IP (4 or 16 bytes)
  bytes  dst_ip        = 11;    // 目的 IP (4 or 16 bytes)
  uint32 ip_version    = 12;    // IP 版本: 4 or 6

  // --- L4: 传输层 ---
  uint32 src_port      = 20;    // 源端口
  uint32 dst_port      = 21;    // 目的端口
  uint32 protocol      = 22;    // L4 协议 (IP protocol number)
  uint32 tcp_flags     = 23;    // TCP 标志位

  // --- L7: 应用层 ---
  uint32 l7_protocol   = 30;    // L7 协议枚举
  uint32 method        = 31;    // HTTP 方法
  string path          = 32;    // URL 路径
  uint32 status_code   = 33;    // HTTP 状态码
  uint64 req_size      = 34;    // 请求大小
  uint64 resp_size     = 35;    // 响应大小

  // --- Process ---
  uint32 pid           = 40;    // 进程 ID
  string process_name  = 41;    // 进程名
  string comm          = 42;    // 进程 comm

  // --- Container ---
  string container_id   = 50;   // 容器 ID
  string container_name = 51;   // 容器名
  string image           = 52;   // 镜像名

  // --- Kubernetes ---
  string pod           = 60;    // Pod 名称
  string namespace     = 61;    // K8s 命名空间
  string deployment    = 62;    // Deployment 名称
  string service       = 63;    // K8s Service
  string node          = 64;    // K8s Node

  // --- Trace ---
  string trace_id      = 70;    // Trace ID (W3C: 32 hex)
  string span_id       = 71;    // Span ID
  string parent_id     = 72;    // Parent Span ID

  // --- Host ---
  string host_id       = 80;    // 主机 ID
  string hostname      = 81;    // 主机名

  // --- Tenant ---
  string tenant_id     = 90;    // 租户 ID

  // --- Metrics ---
  uint64 bytes         = 100;   // 字节数
  uint64 packets       = 101;   // 包数
  uint64 latency_ns    = 102;   // 延迟 (纳秒)
  uint32 direction     = 103;   // 方向: 0=unknown, 1=ingress, 2=egress

  // --- Tags ---
  repeated Tag tags   = 110;    // 自定义标签

  // --- Reserved (未来扩展) ---
  reserved 200 to 299;
}

// Tag 键值对
message Tag {
  string key   = 1;
  string value = 2;
}

// FlowBatch 流数据批量
message FlowBatch {
  uint32 schema_version = 1;
  string probe_id        = 2;
  string asset_id        = 3;
  uint32 count           = 4;
  uint64 seq_id          = 5;
  int64  timestamp_ns    = 6;
  string checksum        = 7;
  FlowDataType type      = 8;
  repeated UnifiedFlow flows = 10;
}

// FlowDataType 流数据类型
enum FlowDataType {
  FLOW_DATA_RAW       = 0;
  FLOW_DATA_L4        = 1;
  FLOW_DATA_L7        = 2;
  FLOW_DATA_METRIC    = 3;
  FLOW_DATA_TRACE     = 4;
  FLOW_DATA_LOG       = 5;
  FLOW_DATA_PROFILING = 6;
}

// Protocol 协议枚举
enum Protocol {
  PROTOCOL_UNKNOWN = 0;
  PROTOCOL_TCP     = 1;
  PROTOCOL_UDP     = 2;
  PROTOCOL_ICMP    = 3;
  PROTOCOL_HTTP    = 4;
  PROTOCOL_HTTP2   = 5;
  PROTOCOL_GRPC    = 6;
  PROTOCOL_DNS     = 7;
  PROTOCOL_MYSQL   = 8;
  PROTOCOL_REDIS   = 9;
  PROTOCOL_KAFKA   = 10;
}

// Direction 流方向
enum Direction {
  DIRECTION_UNKNOWN  = 0;
  DIRECTION_INGRESS  = 1;
  DIRECTION_EGRESS   = 2;
  DIRECTION_INTERNAL = 3;
}
`)

	return b.String()
}
