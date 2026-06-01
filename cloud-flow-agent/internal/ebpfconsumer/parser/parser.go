// Package parser 提供高性能流解析器
//
// 特性:
//   - Zero-copy: 直接解析原始字节，不创建中间对象
//   - 无反射: 所有解析逻辑内联，无 interface{}
//   - 预分配: 使用对象池复用解析结果
//   - Unsafe: 使用 unsafe 包减少内存拷贝
//
// 禁止:
//   - 禁止 json decode
//   - 禁止 map[string]interface{}
//   - 禁止 interface{}
package parser

import (
	"encoding/binary"
	"unsafe"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
)

// Protocol 协议类型
type Protocol uint8

const (
	ProtocolUnknown Protocol = iota
	ProtocolTCP
	ProtocolUDP
	ProtocolICMP
	ProtocolHTTP
	ProtocolDNS
	ProtocolMySQL
)

// Parser 流解析器
type Parser struct {
	// 协议特定解析器
	tcpParser  *TCPParser
	httpParser *HTTPParser
	dnsParser  *DNSParser
	mysqlParser *MySQLParser
}

// New 创建新的解析器
func New() *Parser {
	return &Parser{
		tcpParser:   NewTCPParser(),
		httpParser:  NewHTTPParser(),
		dnsParser:   NewDNSParser(),
		mysqlParser: NewMySQLParser(),
	}
}

// Parse 解析原始事件到 ParsedFlow
// 使用 zero-copy 技术，直接解析原始字节
func (p *Parser) Parse(event *pool.RawEvent, flow *pool.ParsedFlow) {
	if event == nil || flow == nil {
		return
	}

	data := event.Data[:event.Len]
	if len(data) < 13 {
		return
	}

	// 基础解析 (L3/L4)
	p.parseBase(data, flow)

	// 协议特定解析 (L7)
	switch flow.Protocol {
	case 6: // TCP
		p.tcpParser.Parse(data, flow)
		// 尝试解析应用层协议
		if flow.DstPort == 80 || flow.SrcPort == 80 {
			p.httpParser.Parse(data, flow)
		} else if flow.DstPort == 3306 || flow.SrcPort == 3306 {
			p.mysqlParser.Parse(data, flow)
		}
	case 17: // UDP
		if flow.DstPort == 53 || flow.SrcPort == 53 {
			p.dnsParser.Parse(data, flow)
		}
	}

	// 设置元数据
	flow.Type = pool.EventType(event.Type)
	flow.CPU = event.CPU
	flow.Seq = event.Seq
}

// parseBase 解析基础网络层信息
func (p *Parser) parseBase(data []byte, flow *pool.ParsedFlow) {
	// 数据格式: src_ip[4] + dst_ip[4] + src_port[2] + dst_port[2] + protocol[1] + ...
	
	// 使用 unsafe 直接读取 (小端序)
	*(*uint32)(unsafe.Pointer(&flow.SrcIP[0])) = *(*uint32)(unsafe.Pointer(&data[0]))
	*(*uint32)(unsafe.Pointer(&flow.DstIP[0])) = *(*uint32)(unsafe.Pointer(&data[4]))
	
	// 端口 (大端序网络字节序)
	flow.SrcPort = binary.BigEndian.Uint16(data[8:10])
	flow.DstPort = binary.BigEndian.Uint16(data[10:12])
	
	// 协议
	flow.Protocol = data[12]
	
	// 统计数据 (offset 13)
	if len(data) >= 29 {
		flow.Bytes = binary.BigEndian.Uint64(data[13:21])
		flow.Packets = binary.BigEndian.Uint64(data[21:29])
	}
	
	// 时间戳 (offset 29)
	if len(data) >= 37 {
		flow.Timestamp = binary.BigEndian.Uint64(data[29:37])
	}
	
	// 连接 ID (offset 37)
	if len(data) >= 45 {
		flow.ConnID = binary.BigEndian.Uint64(data[37:45])
	}
	
	// 方向和标志 (offset 45)
	if len(data) >= 47 {
		flow.Direction = data[45]
		flow.Flags = binary.BigEndian.Uint16(data[46:48])
	}
}

// TCPParser TCP 解析器
type TCPParser struct{}

// NewTCPParser 创建 TCP 解析器
func NewTCPParser() *TCPParser {
	return &TCPParser{}
}

// Parse 解析 TCP 特定字段
func (p *TCPParser) Parse(data []byte, flow *pool.ParsedFlow) {
	if len(data) < 48 {
		return
	}
	
	// TCP flags (offset 48)
	flow.TCPFlags = data[48]
	
	// TCP 选项解析 (如果需要)
	// ...
}

// HTTPParser HTTP 解析器
type HTTPParser struct {
	// 预分配的请求方法查找表
	methodTable [8]string
}

// NewHTTPParser 创建 HTTP 解析器
func NewHTTPParser() *HTTPParser {
	return &HTTPParser{
		methodTable: [8]string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT"},
	}
}

// Parse 解析 HTTP 特定字段
func (p *HTTPParser) Parse(data []byte, flow *pool.ParsedFlow) {
	// 简单的 HTTP 识别
	if len(data) < 100 {
		return
	}
	
	// 检查是否是 HTTP 请求/响应
	payload := data[50:] // 假设 payload 从 offset 50 开始
	if len(payload) < 20 {
		return
	}
	
	// 识别 HTTP 方法
	for i, method := range p.methodTable {
		if hasPrefix(payload, method) {
			flow.HTTPMethod = uint8(i)
			break
		}
	}
	
	// 解析状态码 (如果是响应)
	if hasPrefix(payload, "HTTP/1.") {
		// 查找状态码
		if len(payload) > 12 {
			flow.HTTPStatus = parseStatusCode(payload[9:12])
		}
	}
}

// DNSParser DNS 解析器
type DNSParser struct{}

// NewDNSParser 创建 DNS 解析器
func NewDNSParser() *DNSParser {
	return &DNSParser{}
}

// Parse 解析 DNS 特定字段
func (p *DNSParser) Parse(data []byte, flow *pool.ParsedFlow) {
	if len(data) < 60 {
		return
	}
	
	// DNS payload 假设从 offset 50 开始 (UDP header 8 bytes)
	dnsData := data[50:]
	if len(dnsData) < 12 {
		return
	}
	
	// DNS 标志
	flags := binary.BigEndian.Uint16(dnsData[2:4])
	
	// QR 位: 0=查询, 1=响应
	isResponse := (flags & 0x8000) != 0
	
	// Opcode
	opcode := (flags >> 11) & 0x0F
	
	// Rcode (响应码)
	rcode := flags & 0x000F
	
	// QDCOUNT (问题数)
	qdcount := binary.BigEndian.Uint16(dnsData[4:6])
	
	// 存储解析结果
	flow.DNSQueryType = uint16(opcode)<<8 | uint16(rcode)
	
	if isResponse {
		flow.Flags |= 0x01 // 标记为响应
	}
	
	if qdcount > 0 {
		flow.Flags |= 0x02 // 标记有查询
	}
}

// MySQLParser MySQL 解析器
type MySQLParser struct{}

// NewMySQLParser 创建 MySQL 解析器
func NewMySQLParser() *MySQLParser {
	return &MySQLParser{}
}

// Parse 解析 MySQL 特定字段
func (p *MySQLParser) Parse(data []byte, flow *pool.ParsedFlow) {
	if len(data) < 60 {
		return
	}
	
	// MySQL payload 假设从 offset 50 开始 (TCP header 20 bytes)
	mysqlData := data[50:]
	if len(mysqlData) < 5 {
		return
	}
	
	// MySQL 包长度 (3 bytes, 小端序)
	pktLen := uint32(mysqlData[0]) | uint32(mysqlData[1])<<8 | uint32(mysqlData[2])<<16
	
	// 序列号
	seq := mysqlData[3]
	
	// 命令类型
	cmd := mysqlData[4]
	
	flow.MySQLCmd = cmd
	
	// 标记为 MySQL
	flow.Flags |= 0x04
	
	_ = pktLen
	_ = seq
}

// 辅助函数

// hasPrefix 检查字节数组是否以指定前缀开头
func hasPrefix(data []byte, prefix string) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}

// parseStatusCode 解析 HTTP 状态码
func parseStatusCode(data []byte) uint16 {
	if len(data) < 3 {
		return 0
	}
	return uint16(data[0]-'0')*100 + uint16(data[1]-'0')*10 + uint16(data[2]-'0')
}

// FastPath 快速路径解析 (针对已知格式的数据)
// 如果数据格式已知，使用此函数避免完整解析开销
func (p *Parser) FastPath(event *pool.RawEvent, flow *pool.ParsedFlow) {
	if event == nil || flow == nil || event.Len < 13 {
		return
	}
	
	data := event.Data[:event.Len]
	
	// 仅解析最基础的字段
	*(*uint32)(unsafe.Pointer(&flow.SrcIP[0])) = *(*uint32)(unsafe.Pointer(&data[0]))
	*(*uint32)(unsafe.Pointer(&flow.DstIP[0])) = *(*uint32)(unsafe.Pointer(&data[4]))
	flow.SrcPort = binary.BigEndian.Uint16(data[8:10])
	flow.DstPort = binary.BigEndian.Uint16(data[10:12])
	flow.Protocol = data[12]
	
	// 统计数据
	if event.Len >= 29 {
		flow.Bytes = binary.BigEndian.Uint64(data[13:21])
		flow.Packets = binary.BigEndian.Uint64(data[21:29])
	}
	
	flow.Type = pool.EventType(event.Type)
	flow.CPU = event.CPU
	flow.Seq = event.Seq
}

// BatchParse 批量解析
// 利用 CPU cache locality，批量处理多个事件
func (p *Parser) BatchParse(events []*pool.RawEvent, flows []*pool.ParsedFlow, count int) {
	for i := 0; i < count; i++ {
		p.Parse(events[i], flows[i])
	}
}

// BatchParseFast 批量快速解析
func (p *Parser) BatchParseFast(events []*pool.RawEvent, flows []*pool.ParsedFlow, count int) {
	for i := 0; i < count; i++ {
		p.FastPath(events[i], flows[i])
	}
}
