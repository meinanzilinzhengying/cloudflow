package vxlan

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
)

// VXLANHeader VXLAN协议头部 (8字节)
type VXLANHeader struct {
	Flags     uint8    // 标志位，I位=1表示VNI有效 (0x08)
	Reserved1 [3]uint8 // 保留字段
	VNI       [3]uint8 // 24位VNI网络标识符
	Reserved2 uint8    // 保留字段
}

// EthernetHeader 以太网帧头部 (14字节)
type EthernetHeader struct {
	DstMAC    [6]byte // 目的MAC地址
	SrcMAC    [6]byte // 源MAC地址
	EtherType uint16  // 以太类型
}

// IPv4Header IPv4头部 (20字节 + 选项)
type IPv4Header struct {
	VersionIHL     uint8  // 版本(4位) + IHL(4位)
	TOS            uint8  // 服务类型
	TotalLength    uint16 // 总长度
	ID             uint16 // 标识
	FlagsFragOff   uint16 // 标志(3位) + 片偏移(13位)
	TTL            uint8  // 生存时间
	Protocol       uint8  // 协议号
	Checksum       uint16 // 校验和
	SrcIP          [4]byte // 源IP
	DstIP          [4]byte // 目的IP
}

// IPv6Header IPv6头部 (40字节)
type IPv6Header struct {
	VersionClassFlow [4]byte // 版本(4位) + 流量类(8位) + 流标签(20位)
	PayloadLength    uint16  // 有效载荷长度
	NextHeader       uint8   // 下一个头
	HopLimit         uint8   // 跳数限制
	SrcIP            [16]byte // 源IP
	DstIP            [16]byte // 目的IP
}

// TCPHeader TCP头部 (20字节 + 选项)
type TCPHeader struct {
	SrcPort    uint16 // 源端口
	DstPort    uint16 // 目的端口
	SeqNum     uint32 // 序列号
	AckNum     uint32 // 确认号
	DataOffRes uint8  // 数据偏移(4位) + 保留(6位)
	Flags      uint8  // 标志位
	WindowSize uint16 // 窗口大小
	Checksum   uint16 // 校验和
	UrgentPtr  uint16 // 紧急指针
}

// UDPHeader UDP头部 (8字节)
type UDPHeader struct {
	SrcPort  uint16 // 源端口
	DstPort  uint16 // 目的端口
	Length   uint16 // 长度
	Checksum uint16 // 校验和
}

// VXLANFlow VXLAN流五元组信息
type VXLANFlow struct {
	VNI        uint32           `json:"vni"`
	SrcIP      net.IP           `json:"src_ip"`
	DstIP      net.IP           `json:"dst_ip"`
	SrcPort    uint16           `json:"src_port"`
	DstPort    uint16           `json:"dst_port"`
	Protocol   uint8            `json:"protocol"`
	SrcMAC     net.HardwareAddr `json:"src_mac"`
	DstMAC     net.HardwareAddr `json:"dst_mac"`
	Bytes      uint64           `json:"bytes"`
	Packets    uint64           `json:"packets"`
	EtherType  uint16           `json:"ether_type"`
}

// Parser VXLAN协议解析器
type Parser struct {
	stats ParserStats
}

// ParserStats 解析统计
type ParserStats struct {
	TotalPackets     uint64 `json:"total_packets"`
	ParsedSuccess    uint64 `json:"parsed_success"`
	ParsedFailed     uint64 `json:"parsed_failed"`
	VXLANPackets     uint64 `json:"vxlan_packets"`
	NonVXLANPackets  uint64 `json:"non_vxlan_packets"`
	IPv4Packets      uint64 `json:"ipv4_packets"`
	IPv6Packets      uint64 `json:"ipv6_packets"`
	TCPPackets       uint64 `json:"tcp_packets"`
	UDPPackets       uint64 `json:"udp_packets"`
	OtherPackets     uint64 `json:"other_packets"`
}

// 协议常量
const (
	// VXLAN常量
	VXLAN_PORT        = 8472  // VXLAN标准UDP端口
	VXLAN_FLAG_I      = 0x08  // VNI有效标志位
	VXLAN_HEADER_LEN  = 8     // VXLAN头部长度

	// 以太类型
	ETH_TYPE_IPv4     = 0x0800
	ETH_TYPE_IPv6     = 0x86DD
	ETH_TYPE_ARP      = 0x0806
	ETH_TYPE_VLAN     = 0x8100

	// IP协议号
	IP_PROTO_TCP      = 6
	IP_PROTO_UDP      = 17
	IP_PROTO_ICMP     = 1
	IP_PROTO_ICMPv6   = 58

	// TCP标志位
	TCP_FLAG_FIN      = 0x01
	TCP_FLAG_SYN      = 0x02
	TCP_FLAG_RST      = 0x04
	TCP_FLAG_PSH      = 0x08
	TCP_FLAG_ACK      = 0x10
	TCP_FLAG_URG      = 0x20
	TCP_FLAG_ECE      = 0x40
	TCP_FLAG_CWR      = 0x80

	// 最小长度检查
	MIN_VXLAN_PACKET_LEN = 8 + 14 + 20 + 8 // VXLAN + ETH + IPv4 + UDP
)

// NewParser 创建VXLAN解析器
func NewParser() *Parser {
	return &Parser{}
}

// ParseVXLAN 解析VXLAN数据包
func (p *Parser) ParseVXLAN(data []byte) (*VXLANFlow, error) {
	p.stats.TotalPackets++

	// 最小长度检查
	if len(data) < MIN_VXLAN_PACKET_LEN {
		p.stats.ParsedFailed++
		return nil, fmt.Errorf("packet too short: %d bytes, min required: %d", len(data), MIN_VXLAN_PACKET_LEN)
	}

	// 1. 解析VXLAN头部
	vxlanHeader, offset, err := p.parseVXLANHeader(data)
	if err != nil {
		p.stats.ParsedFailed++
		p.stats.NonVXLANPackets++
		return nil, fmt.Errorf("parse VXLAN header failed: %w", err)
	}

	p.stats.VXLANPackets++
	flow := &VXLANFlow{
		VNI: vxlanHeader.GetVNI(),
	}

	// 2. 解析内层以太网帧
	ethHeader, offset, err := p.parseEthernetHeader(data[offset:])
	if err != nil {
		p.stats.ParsedFailed++
		logrus.Debugf("parse ethernet header failed: %v", err)
		return flow, nil
	}

	flow.SrcMAC = net.HardwareAddr(ethHeader.SrcMAC[:])
	flow.DstMAC = net.HardwareAddr(ethHeader.DstMAC[:])
	flow.EtherType = ethHeader.EtherType

	// 3. 解析内层IP协议
	switch ethHeader.EtherType {
	case ETH_TYPE_IPv4:
		p.stats.IPv4Packets++
		ipHeader, ipOffset, err := p.parseIPv4Header(data[offset:])
		if err != nil {
			p.stats.ParsedFailed++
			logrus.Debugf("parse IPv4 header failed: %v", err)
			return flow, nil
		}
		flow.SrcIP = net.IP(ipHeader.SrcIP[:])
		flow.DstIP = net.IP(ipHeader.DstIP[:])
		flow.Protocol = ipHeader.Protocol
		offset += ipOffset

		// 4. 解析传输层协议
		err = p.parseTransportLayer(data[offset:], ipHeader.Protocol, flow)
		if err != nil {
			logrus.Debugf("parse transport layer failed: %v", err)
		}

	case ETH_TYPE_IPv6:
		p.stats.IPv6Packets++
		ipHeader, ipOffset, err := p.parseIPv6Header(data[offset:])
		if err != nil {
			p.stats.ParsedFailed++
			logrus.Debugf("parse IPv6 header failed: %v", err)
			return flow, nil
		}
		flow.SrcIP = net.IP(ipHeader.SrcIP[:])
		flow.DstIP = net.IP(ipHeader.DstIP[:])
		flow.Protocol = ipHeader.NextHeader
		offset += ipOffset

		// 4. 解析传输层协议
		err = p.parseTransportLayer(data[offset:], ipHeader.NextHeader, flow)
		if err != nil {
			logrus.Debugf("parse transport layer failed: %v", err)
		}

	default:
		p.stats.OtherPackets++
		logrus.Debugf("unsupported ether type: 0x%04x", ethHeader.EtherType)
	}

	flow.Packets = 1
	flow.Bytes = uint64(len(data))
	p.stats.ParsedSuccess++

	return flow, nil
}

// parseVXLANHeader 解析VXLAN头部
func (p *Parser) parseVXLANHeader(data []byte) (*VXLANHeader, int, error) {
	if len(data) < VXLAN_HEADER_LEN {
		return nil, 0, fmt.Errorf("VXLAN header too short: %d", len(data))
	}

	header := &VXLANHeader{}
	header.Flags = data[0]
	copy(header.Reserved1[:], data[1:4])
	copy(header.VNI[:], data[4:7])
	header.Reserved2 = data[7]

	// 验证I标志位
	if (header.Flags & VXLAN_FLAG_I) == 0 {
		logrus.Debugf("VXLAN I flag not set: 0x%02x", header.Flags)
	}

	return header, VXLAN_HEADER_LEN, nil
}

// parseEthernetHeader 解析以太网头部
func (p *Parser) parseEthernetHeader(data []byte) (*EthernetHeader, int, error) {
	if len(data) < 14 {
		return nil, 0, fmt.Errorf("ethernet header too short: %d", len(data))
	}

	header := &EthernetHeader{}
	copy(header.DstMAC[:], data[0:6])
	copy(header.SrcMAC[:], data[6:12])
	header.EtherType = binary.BigEndian.Uint16(data[12:14])

	// 处理VLAN标签 (802.1Q)
	if header.EtherType == ETH_TYPE_VLAN {
		if len(data) < 18 {
			return nil, 0, fmt.Errorf("VLAN tagged frame too short: %d", len(data))
		}
		// 跳过TCI(2字节)，读取真正的EtherType
		header.EtherType = binary.BigEndian.Uint16(data[16:18])
		return header, 18, nil
	}

	return header, 14, nil
}

// parseIPv4Header 解析IPv4头部
func (p *Parser) parseIPv4Header(data []byte) (*IPv4Header, int, error) {
	if len(data) < 20 {
		return nil, 0, fmt.Errorf("IPv4 header too short: %d", len(data))
	}

	header := &IPv4Header{}
	header.VersionIHL = data[0]
	header.TOS = data[1]
	header.TotalLength = binary.BigEndian.Uint16(data[2:4])
	header.ID = binary.BigEndian.Uint16(data[4:6])
	header.FlagsFragOff = binary.BigEndian.Uint16(data[6:8])
	header.TTL = data[8]
	header.Protocol = data[9]
	header.Checksum = binary.BigEndian.Uint16(data[10:12])
	copy(header.SrcIP[:], data[12:16])
	copy(header.DstIP[:], data[16:20])

	// IHL表示32位字的数量，乘以4得到字节数
	ihl := int((header.VersionIHL & 0x0F) * 4)
	if ihl < 20 {
		ihl = 20
	}

	return header, ihl, nil
}

// parseIPv6Header 解析IPv6头部
func (p *Parser) parseIPv6Header(data []byte) (*IPv6Header, int, error) {
	if len(data) < 40 {
		return nil, 0, fmt.Errorf("IPv6 header too short: %d", len(data))
	}

	header := &IPv6Header{}
	copy(header.VersionClassFlow[:], data[0:4])
	header.PayloadLength = binary.BigEndian.Uint16(data[4:6])
	header.NextHeader = data[6]
	header.HopLimit = data[7]
	copy(header.SrcIP[:], data[8:24])
	copy(header.DstIP[:], data[24:40])

	return header, 40, nil
}

// parseTransportLayer 解析传输层协议
func (p *Parser) parseTransportLayer(data []byte, protocol uint8, flow *VXLANFlow) error {
	switch protocol {
	case IP_PROTO_TCP:
		p.stats.TCPPackets++
		if len(data) < 20 {
			return fmt.Errorf("TCP header too short: %d", len(data))
		}
		tcpHeader := &TCPHeader{}
		tcpHeader.SrcPort = binary.BigEndian.Uint16(data[0:2])
		tcpHeader.DstPort = binary.BigEndian.Uint16(data[2:4])
		flow.SrcPort = tcpHeader.SrcPort
		flow.DstPort = tcpHeader.DstPort

	case IP_PROTO_UDP:
		p.stats.UDPPackets++
		if len(data) < 8 {
			return fmt.Errorf("UDP header too short: %d", len(data))
		}
		udpHeader := &UDPHeader{}
		udpHeader.SrcPort = binary.BigEndian.Uint16(data[0:2])
		udpHeader.DstPort = binary.BigEndian.Uint16(data[2:4])
		flow.SrcPort = udpHeader.SrcPort
		flow.DstPort = udpHeader.DstPort

	default:
		p.stats.OtherPackets++
		return fmt.Errorf("unsupported protocol: %d", protocol)
	}

	return nil
}

// GetVNI 获取24位VNI值
func (h *VXLANHeader) GetVNI() uint32 {
	return uint32(h.VNI[0])<<16 | uint32(h.VNI[1])<<8 | uint32(h.VNI[2])
}

// GetStats 获取解析统计
func (p *Parser) GetStats() ParserStats {
	return p.stats
}

// ResetStats 重置统计
func (p *Parser) ResetStats() {
	p.stats = ParserStats{}
}

// IsVXLANPacket 检查是否是VXLAN数据包
func IsVXLANPacket(data []byte, dstPort uint16) bool {
	// 检查UDP目标端口
	if dstPort != VXLAN_PORT && dstPort != 4789 {
		return false
	}

	// 检查最小长度
	if len(data) < VXLAN_HEADER_LEN {
		return false
	}

	// 检查I标志位
	return (data[0] & VXLAN_FLAG_I) != 0
}

// ProtocolToString 协议号转字符串
func ProtocolToString(protocol uint8) string {
	switch protocol {
	case IP_PROTO_TCP:
		return "TCP"
	case IP_PROTO_UDP:
		return "UDP"
	case IP_PROTO_ICMP:
		return "ICMP"
	case IP_PROTO_ICMPv6:
		return "ICMPv6"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", protocol)
	}
}

// EtherTypeToString 以太类型转字符串
func EtherTypeToString(etherType uint16) string {
	switch etherType {
	case ETH_TYPE_IPv4:
		return "IPv4"
	case ETH_TYPE_IPv6:
		return "IPv6"
	case ETH_TYPE_ARP:
		return "ARP"
	case ETH_TYPE_VLAN:
		return "VLAN"
	default:
		return fmt.Sprintf("0x%04x", etherType)
	}
}
