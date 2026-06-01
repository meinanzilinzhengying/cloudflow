// Package flow 提供 FlowBatch 批量结构和 FlowConverter 转换器
package flow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"sync/atomic"
	"time"
	"unsafe"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
	edge "cloud-flow/proto"
)

// ============================================================================
// BatchHeader 统一批次头 (所有数据类型复用)
// ============================================================================

// BatchHeader 批次头 (8 bytes, cache line 对齐)
type BatchHeader struct {
	ProbeID   FixedString64 // 探针 ID
	AssetID   FixedString64 // 资产 ID
	Count     uint32        // 数据条数
	SeqID     uint64        // 序列号 (单调递增)
	Timestamp int64         // 批次时间戳 (纳秒)
	Checksum  [32]byte      // SHA256 校验和
}

// ============================================================================
// FlowBatch 流数据批量结构
// ============================================================================

// FlowDataType 流数据类型
type FlowDataType uint8

const (
	FlowDataRaw   FlowDataType = iota // 原始网络流
	FlowDataL4                        // L4 聚合流
	FlowDataL7                        // L7 应用流
	FlowDataMetric                    // 指标数据
	FlowDataTrace                     // 链路追踪
	FlowDataLog                       // 日志数据
	FlowDataProfiling                 // 性能分析
)

// FlowBatch 流数据批量
type FlowBatch struct {
	Header BatchHeader
	Type   FlowDataType
	Flows  []*UnifiedFlow
}

// NewFlowBatch 创建流数据批量
func NewFlowBatch(flowType FlowDataType, probeID string, capacity int) *FlowBatch {
	b := &FlowBatch{
		Type:  flowType,
		Flows: make([]*UnifiedFlow, 0, capacity),
	}
	b.Header.ProbeID.Set(probeID)
	b.Header.Timestamp = time.Now().UnixNano()
	return b
}

// Add 添加流数据
func (b *FlowBatch) Add(f *UnifiedFlow) {
	b.Flows = append(b.Flows, f)
	b.Header.Count = uint32(len(b.Flows))
}

// Finalize 完成批次 (计算校验和)
func (b *FlowBatch) Finalize() {
	b.Header.Count = uint32(len(b.Flows))
	b.Header.Checksum = b.computeChecksum()
}

// computeChecksum 计算批次校验和
func (b *FlowBatch) computeChecksum() [32]byte {
	h := sha256.New()
	h.Write([]byte{byte(b.Type)})
	h.Write([]byte(b.Header.ProbeID.String()))
	h.Write([]byte(b.Header.AssetID.String()))
	for _, f := range b.Flows {
		data := f.Serialize()
		h.Write(data)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// ChecksumString 返回校验和字符串
func (b *FlowBatch) ChecksumString() string {
	return hex.EncodeToString(b.Header.Checksum[:])
}

// ============================================================================
// 全局序列号生成器
// ============================================================================

var globalSeqID uint64

// NextSeqID 生成下一个序列号
func NextSeqID() uint64 {
	return atomic.AddUint64(&globalSeqID, 1)
}

// ============================================================================
// FlowConverter 转换器
// ============================================================================

// Converter 流数据转换器
type Converter struct{}

// NewConverter 创建转换器
func NewConverter() *Converter {
	return &Converter{}
}

// ParsedFlowToUnified 将 eBPF ParsedFlow 转换为 UnifiedFlow
func (c *Converter) ParsedFlowToUnified(raw *pool.RawEvent, parsed *pool.ParsedFlow) *UnifiedFlow {
	f := New()

	// Timestamp
	f.Timestamp = int64(parsed.Timestamp)

	// L3: IPv4 (eBPF 层仅支持 IPv4)
	f.SetL3IPv4(parsed.SrcIP, parsed.DstIP)

	// L4
	f.SetL4(parsed.SrcPort, parsed.DstPort, Protocol(parsed.Protocol), parsed.TCPFlags)

	// L7: 根据端口推断应用协议
	dstPort := parsed.DstPort
	switch {
	case dstPort == 80 || dstPort == 8080 || dstPort == 8000:
		f.SetL7(ProtoHTTP, parsed.HTTPMethod, "", parsed.HTTPStatus)
	case dstPort == 443:
		f.SetL7(ProtoHTTP2, parsed.HTTPMethod, "", parsed.HTTPStatus)
	case dstPort == 3306:
		f.SetL7(ProtoMySQL, 0, "", 0)
	case dstPort == 53:
		f.SetL7(ProtoDNS, 0, "", 0)
	case dstPort == 6379:
		f.SetL7(ProtoRedis, 0, "", 0)
	case dstPort == 9092:
		f.SetL7(ProtoKafka, 0, "", 0)
	}

	// Metrics
	f.SetMetrics(parsed.Bytes, parsed.Packets, parsed.LatencyNs, Direction(parsed.Direction))

	// CPU tag
	f.SetTag("cpu", fmt.Sprintf("%d", parsed.CPU))

	return f
}

// MetricDataToUnified 将 edge.MetricData 转换为 UnifiedFlow
func (c *Converter) MetricDataToUnified(m *edge.MetricData) *UnifiedFlow {
	f := New()

	// Timestamp (毫秒 -> 纳秒)
	f.Timestamp = m.Timestamp * 1e6

	// L3
	if m.SrcIp != "" {
		f.SetL3(m.SrcIp, m.DstIp)
	}

	// L4
	f.SetL4(
		uint16(m.SrcPort),
		uint16(m.DstPort),
		ParseProtocol(string(m.Protocol)),
		0,
	)

	// Metrics
	f.SetMetrics(
		uint64(m.Bytes),
		uint64(m.Packets),
		uint64(m.Latency)*1000, // 微秒 -> 纳秒
		DirUnknown,
	)

	// Tags
	if m.Tags != nil {
		for k, v := range m.Tags {
			f.SetTag(k, v)
		}
	}

	// ProbeID / AssetID
	f.SetTag("probe_id", m.ProbeId)
	f.SetTag("asset_id", m.AssetId)
	if m.Service != "" {
		f.SetTag("service", m.Service)
	}

	return f
}

// TraceSpanToUnified 将 edge.TraceSpanData 转换为 UnifiedFlow
func (c *Converter) TraceSpanToUnified(s *edge.TraceSpanData) *UnifiedFlow {
	f := New()

	// Timestamp
	f.Timestamp = s.StartTime * 1000 // 微秒 -> 纳秒

	// L3
	if s.SrcIp != "" {
		f.SetL3(s.SrcIp, s.DstIp)
	}

	// L4
	f.SetL4(
		uint16(s.SrcPort),
		uint16(s.DstPort),
		ParseProtocol(string(s.Protocol)),
		0,
	)

	// L7
	f.SetL7(
		ParseProtocol(string(s.Protocol)),
		0,
		s.Operation,
		uint16(0),
	)
	f.SetL7Sizes(uint64(s.RequestSize), uint64(s.ResponseSize))

	// Metrics (duration as latency)
	f.SetMetrics(0, 0, uint64(s.Duration)*1000, DirUnknown) // 微秒 -> 纳秒

	// Trace
	f.SetTrace(s.TraceId, s.SpanId, s.ParentId)

	// Service
	f.SetTag("service", s.Service)

	// Status
	if s.Status == "error" {
		f.SetTag("error", s.ErrorMessage)
	}

	return f
}

// LogDataToUnified 将 edge.LogData 转换为 UnifiedFlow
func (c *Converter) LogDataToUnified(l *edge.LogData) *UnifiedFlow {
	f := New()

	// Timestamp
	f.Timestamp = l.Timestamp * 1e6 // 毫秒 -> 纳秒

	// Trace
	if l.TraceId != "" {
		f.SetTrace(l.TraceId, l.SpanId, "")
	}

	// Service
	if l.Service != "" {
		f.SetTag("service", l.Service)
	}

	// Log-specific fields
	f.SetTag("level", string(l.Level))
	f.SetTag("source", l.Source)
	f.SetTag("message", l.Message)

	// ProbeID
	f.SetTag("probe_id", l.ProbeId)

	// Additional fields
	if l.Fields != nil {
		for k, v := range l.Fields {
			f.SetTag(k, v)
		}
	}

	return f
}

// UnifiedToMetricData 将 UnifiedFlow 转换为 edge.MetricData
func (c *Converter) UnifiedToMetricData(f *UnifiedFlow) *edge.MetricData {
	m := &edge.MetricData{
		Timestamp: f.Timestamp / 1e6, // 纳秒 -> 毫秒
		SrcIp:     f.SrcIP.String(),
		DstIp:     f.DstIP.String(),
		SrcPort:   int32(f.SrcPort),
		DstPort:   int32(f.DstPort),
		Protocol:  edge.ProtocolType(f.Protocol.String()),
		Bytes:     int64(f.Bytes),
		Packets:   int64(f.Packets),
		Latency:   int64(f.LatencyNs / 1000), // 纳秒 -> 微秒
		Tags:      make(map[string]string),
	}

	// L7 字段
	if f.L7Protocol != ProtoUnknown {
		m.Tags["l7_protocol"] = f.L7Protocol.String()
	}
	if f.Method != 0 {
		methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT"}
		if int(f.Method) < len(methods) {
			m.Tags["http_method"] = methods[f.Method]
		}
	}
	if f.StatusCode != 0 {
		m.Tags["status_code"] = FormatPort(f.StatusCode)
	}
	if !f.Path.IsZero() {
		m.Endpoint = f.Path.String()
	}

	// Process
	if f.PID != 0 {
		m.Tags["pid"] = FormatPort(uint16(f.PID))
	}
	if !f.ProcessName.IsZero() {
		m.Tags["process"] = f.ProcessName.String()
	}

	// Container
	if !f.ContainerID.IsZero() {
		m.Tags["container_id"] = f.ContainerID.String()
	}

	// K8s
	if !f.Pod.IsZero() {
		m.Tags["pod"] = f.Pod.String()
	}
	if !f.Namespace.IsZero() {
		m.Tags["namespace"] = f.Namespace.String()
	}
	if !f.Service.IsZero() {
		m.Service = f.Service.String()
	}
	if !f.Node.IsZero() {
		m.Tags["node"] = f.Node.String()
	}

	// Trace
	if !f.TraceID.IsZero() {
		m.Tags["trace_id"] = f.TraceID.String()
		m.Tags["span_id"] = f.SpanID.String()
	}

	// Tenant
	if !f.TenantID.IsZero() {
		m.Tags["tenant_id"] = f.TenantID.String()
	}

	// Host
	if !f.HostID.IsZero() {
		m.Tags["host_id"] = f.HostID.String()
	}

	// Custom tags
	for i := 0; i < f.Tags.Count(); i++ {
		key := f.Tags[i].GetKey()
		val := f.Tags[i].GetValue()
		if key != "" && val != "" {
			m.Tags[key] = val
		}
	}

	return m
}

// UnifiedToTraceSpan 将 UnifiedFlow 转换为 edge.TraceSpanData
func (c *Converter) UnifiedToTraceSpan(f *UnifiedFlow) *edge.TraceSpanData {
	s := &edge.TraceSpanData{
		TraceId:  f.TraceID.String(),
		SpanId:   f.SpanID.String(),
		ParentId: f.ParentID.String(),
		StartTime: f.Timestamp / 1000, // 纳秒 -> 微秒
		EndTime:   (f.Timestamp + int64(f.LatencyNs)) / 1000,
		Duration:  int64(f.LatencyNs) / 1000,
		SrcIp:     f.SrcIP.String(),
		DstIp:     f.DstIP.String(),
		SrcPort:   int32(f.SrcPort),
		DstPort:   int32(f.DstPort),
		Protocol:  edge.ProtocolType(f.Protocol.String()),
		Tags:      make(map[string]string),
	}

	if f.L7Protocol == ProtoHTTP || f.L7Protocol == ProtoHTTP2 {
		s.Service = f.Path.String()
	}
	if f.StatusCode != 0 {
		s.Tags["status_code"] = FormatPort(f.StatusCode)
	}

	return s
}

// UnifiedToLogData 将 UnifiedFlow 转换为 edge.LogData
func (c *Converter) UnifiedToLogData(f *UnifiedFlow) *edge.LogData {
	l := &edge.LogData{
		Timestamp: f.Timestamp / 1e6, // 纳秒 -> 毫秒
		TraceId:   f.TraceID.String(),
		SpanId:    f.SpanID.String(),
		Fields:    make(map[string]string),
	}

	if !f.Service.IsZero() {
		l.Service = f.Service.String()
	}
	if !f.TenantID.IsZero() {
		l.Fields["tenant_id"] = f.TenantID.String()
	}

	return l
}

// ============================================================================
// FlowID 生成
// ============================================================================

// ComputeFlowID 计算 5-tuple 流 ID
func ComputeFlowID(srcIP, dstIP string, srcPort, dstPort uint16, proto Protocol) uint32 {
	h := fnv.New32a()
	h.Write([]byte(srcIP))
	h.Write([]byte(dstIP))
	b := make([]byte, 5)
	b[0] = byte(srcPort >> 8)
	b[1] = byte(srcPort)
	b[2] = byte(dstPort >> 8)
	b[3] = byte(dstPort)
	b[4] = byte(proto)
	h.Write(b)
	return h.Sum32()
}

// ComputeFlowIDFromParsed 从 ParsedFlow 计算流 ID
func ComputeFlowIDFromParsed(parsed *pool.ParsedFlow) uint32 {
	return ComputeFlowID(
		parsed.SrcIP.String(),
		parsed.DstIP.String(),
		parsed.SrcPort,
		parsed.DstPort,
		Protocol(parsed.Protocol),
	)
}

// ============================================================================
// Memory Alignment 验证
// ============================================================================

// AlignOf 返回结构体的对齐要求
func AlignOf() int {
	return int(unsafe.Alignof(UnifiedFlow{}))
}
