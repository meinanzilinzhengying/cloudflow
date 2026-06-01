// Package parsers DNS 协议解析器
//
// DNS 协议特点:
//   - 基于 UDP (通常) 或 TCP
//   - 请求/响应协议
//   - 二进制格式
//
// 实现要点:
//   - 解析 DNS header (12 bytes)
//   - 提取查询域名和类型
//   - 支持 TCP DNS (2-byte length prefix)

package parsers

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

var (
	ErrInvalidDNS    = errors.New("invalid dns packet")
	ErrIncompleteDNS = errors.New("incomplete dns packet")
)

// DNS Header 标志位
const (
	DNSFlagQR     uint16 = 0x8000 // Query/Response
	DNSFlagOpcode uint16 = 0x7800 // Operation Code
	DNSFlagAA     uint16 = 0x0400 // Authoritative Answer
	DNSFlagTC     uint16 = 0x0200 // Truncated
	DNSFlagRD     uint16 = 0x0100 // Recursion Desired
	DNSFlagRA     uint16 = 0x0080 // Recursion Available
	DNSFlagZ      uint16 = 0x0070 // Reserved
	DNSFlagRCODE  uint16 = 0x000F // Response Code
)

// DNS Operation Codes
const (
	DNSOpcodeQuery  uint8 = 0
	DNSOpcodeIQuery uint8 = 1 // Inverse Query (obsolete)
	DNSOpcodeStatus uint8 = 2
	DNSOpcodeNotify uint8 = 4
	DNSOpcodeUpdate uint8 = 5
)

// DNS Response Codes
const (
	DNSRcodeNoError        uint8 = 0
	DNSRcodeFormErr        uint8 = 1
	DNSRcodeServFail       uint8 = 2
	DNSRcodeNXDomain       uint8 = 3
	DNSRcodeNotImp         uint8 = 4
	DNSRcodeRefused        uint8 = 5
	DNSRcodeYXDomain       uint8 = 6
	DNSRcodeYXRRSet        uint8 = 7
	DNSRcodeNXRRSet        uint8 = 8
	DNSRcodeNotAuth        uint8 = 9
	DNSRcodeNotZone        uint8 = 10
	DNSRcodeDSOTYPENI      uint8 = 11
	DNSRcodeBadSig         uint8 = 16
	DNSRcodeBadKey         uint8 = 17
	DNSRcodeBadTime        uint8 = 18
	DNSRcodeBadMode        uint8 = 19
	DNSRcodeBadName        uint8 = 20
	DNSRcodeBadAlg         uint8 = 21
	DNSRcodeBadTrunc       uint8 = 22
	DNSRcodeBadCookie      uint8 = 23
)

// DNS Record Types
const (
	DNSTypeA     uint16 = 1
	DNSTypeNS    uint16 = 2
	DNSTypeMD    uint16 = 3
	DNSTypeMF    uint16 = 4
	DNSTypeCNAME uint16 = 5
	DNSTypeSOA   uint16 = 6
	DNSTypeMB    uint16 = 7
	DNSTypeMG    uint16 = 8
	DNSTypeMR    uint16 = 9
	DNSTypeNULL  uint16 = 10
	DNSTypeWKS   uint16 = 11
	DNSTypePTR   uint16 = 12
	DNSTypeHINFO uint16 = 13
	DNSTypeMINFO uint16 = 14
	DNSTypeMX    uint16 = 15
	DNSTypeTXT   uint16 = 16
	DNSTypeRP    uint16 = 17
	DNSTypeAFSDB uint16 = 18
	DNSTypeX25   uint16 = 19
	DNSTypeISDN  uint16 = 20
	DNSTypeRT    uint16 = 21
	DNSTypeNSAP  uint16 = 22
	DNSTypePX    uint16 = 26
	DNSTypeGPOS  uint16 = 27
	DNSTypeAAAA  uint16 = 28
	DNSTypeLOC   uint16 = 29
	DNSTypeNXT   uint16 = 30
	DNSTypeEID   uint16 = 31
	DNSTypeNIMLOC uint16 = 32
	DNSTypeSRV   uint16 = 33
	DNSTypeATMA  uint16 = 34
	DNSTypeNAPTR uint16 = 35
	DNSTypeKX    uint16 = 36
	DNSTypeCERT  uint16 = 37
	DNSTypeDNAME uint16 = 39
	DNSTypeOPT   uint16 = 41
	DNSTypeAPL   uint16 = 42
	DNSTypeDS    uint16 = 43
	DNSTypeSSHFP uint16 = 44
	DNSTypeIPSECKEY uint16 = 45
	DNSTypeRRSIG uint16 = 46
	DNSTypeNSEC  uint16 = 47
	DNSTypeDNSKEY uint16 = 48
	DNSTypeDHCID uint16 = 49
	DNSTypeNSEC3 uint16 = 50
	DNSTypeNSEC3PARAM uint16 = 51
	DNSTypeTLSA  uint16 = 52
	DNSTypeSMIMEA uint16 = 53
	DNSTypeHIP   uint16 = 55
	DNSTypeNINFO uint16 = 56
	DNSTypeRKEY  uint16 = 57
	DNSTypeTALINK uint16 = 58
	DNSTypeCDS   uint16 = 59
	DNSTypeCDNSKEY uint16 = 60
	DNSTypeOPENPGPKEY uint16 = 61
	DNSTypeCSYNC uint16 = 62
	DNSTypeZONEMD uint16 = 63
	DNSTypeSVCB  uint16 = 64
	DNSTypeHTTPS uint16 = 65
	DNSTypeSPF   uint16 = 99
	DNSTypeUINFO uint16 = 100
	DNSTypeUID   uint16 = 101
	DNSTypeGID   uint16 = 102
	DNSTypeUNSPEC uint16 = 103
	DNSTypeNID   uint16 = 104
	DNSTypeL32   uint16 = 105
	DNSTypeL64   uint16 = 106
	DNSTypeLP    uint16 = 107
	DNSTypeEUI48 uint16 = 108
	DNSTypeEUI64 uint16 = 109
	DNSTypeTKEY  uint16 = 249
	DNSTypeTSIG  uint16 = 250
	DNSTypeIXFR  uint16 = 251
	DNSTypeAXFR  uint16 = 252
	DNSTypeMAILB uint16 = 253
	DNSTypeMAILA uint16 = 254
	DNSTypeANY   uint16 = 255
	DNSTypeURI   uint16 = 256
	DNSTypeCAA   uint16 = 257
	DNSTypeAVC   uint16 = 258
	DNSTypeDOA   uint16 = 259
	DNSTypeAMTRELAY uint16 = 260
	DNSTypeTA    uint16 = 32768
	DNSTypeDLV   uint16 = 32769
)

// DNSTypeNames DNS 类型名称映射
var DNSTypeNames = map[uint16]string{
	DNSTypeA:     "A",
	DNSTypeNS:    "NS",
	DNSTypeCNAME: "CNAME",
	DNSTypeSOA:   "SOA",
	DNSTypePTR:   "PTR",
	DNSTypeMX:    "MX",
	DNSTypeTXT:   "TXT",
	DNSTypeAAAA:  "AAAA",
	DNSTypeSRV:   "SRV",
	DNSTypeNAPTR: "NAPTR",
	DNSTypeDS:    "DS",
	DNSTypeDNSKEY: "DNSKEY",
	DNSTypeRRSIG: "RRSIG",
	DNSTypeNSEC:  "NSEC",
	DNSTypeCAA:   "CAA",
	DNSTypeANY:   "ANY",
	DNSTypeAXFR:  "AXFR",
}

// GetDNSTypeName 获取 DNS 类型名称
func GetDNSTypeName(t uint16) string {
	if name, ok := DNSTypeNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

// DNSHeader DNS 包头
type DNSHeader struct {
	ID      uint16
	Flags   uint16
	QDCount uint16 // Question Count
	ANCount uint16 // Answer Count
	NSCount uint16 // Authority Count
	ARCount uint16 // Additional Count
}

// DNSQuestion DNS 查询
type DNSQuestion struct {
	Name  string
	Type  uint16
	Class uint16
}

// DNSPacket DNS 包
type DNSPacket struct {
	Header    DNSHeader
	Questions []DNSQuestion
	IsTCP     bool // 是否是 TCP DNS
}

// IsQuery 检查是否为查询
func (h *DNSHeader) IsQuery() bool {
	return h.Flags&DNSFlagQR == 0
}

// IsResponse 检查是否为响应
func (h *DNSHeader) IsResponse() bool {
	return h.Flags&DNSFlagQR != 0
}

// GetOpcode 获取操作码
func (h *DNSHeader) GetOpcode() uint8 {
	return uint8((h.Flags & DNSFlagOpcode) >> 11)
}

// GetRcode 获取响应码
func (h *DNSHeader) GetRcode() uint8 {
	return uint8(h.Flags & DNSFlagRCODE)
}

// DNSParser DNS 协议解析器
type DNSParser struct {
	state *dnsParseState
}

// dnsParseState 解析状态
type dnsParseState struct {
	buffer []byte
}

// NewDNSParser 创建 DNS 解析器
func NewDNSParser() *DNSParser {
	return &DNSParser{
		state: &dnsParseState{
			buffer: make([]byte, 0, 4096),
		},
	}
}

// Type 返回解析器类型
func (p *DNSParser) Type() l7parser.ParserType {
	return l7parser.ParserTypeDNS
}

// Name 返回解析器名称
func (p *DNSParser) Name() string {
	return "dns"
}

// Priority 返回解析优先级
func (p *DNSParser) Priority() int {
	return 50
}

// Detect 协议检测
func (p *DNSParser) Detect(data []byte, dstPort uint16) (bool, float64) {
	if len(data) < 12 {
		return false, 0
	}

	// 检查 UDP DNS (端口 53)
	if dstPort == 53 {
		// 检查 header 字段
		flags := binary.BigEndian.Uint16(data[2:4])
		opcode := (flags & DNSFlagOpcode) >> 11
		rcode := flags & DNSFlagRCODE

		// 检查 opcode 和 rcode 是否有效
		if opcode <= 6 && rcode <= 23 {
			return true, 0.95
		}
	}

	// 检查 TCP DNS (端口 53，带 2-byte length prefix)
	if dstPort == 53 && len(data) >= 14 {
		tcpLen := binary.BigEndian.Uint16(data[0:2])
		if tcpLen > 0 && tcpLen < 65535 {
			flags := binary.BigEndian.Uint16(data[4:6])
			opcode := (flags & DNSFlagOpcode) >> 11
			if opcode <= 6 {
				return true, 0.90
			}
		}
	}

	// 通用检测
	flags := binary.BigEndian.Uint16(data[2:4])
	opcode := (flags & DNSFlagOpcode) >> 11
	rcode := flags & DNSFlagRCODE

	if opcode <= 6 && rcode <= 23 {
		return true, 0.70
	}

	return false, 0
}

// Parse 解析数据包
func (p *DNSParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	data := input.Packet.Data
	if len(data) == 0 {
		return nil, state, nil
	}

	// 获取或创建解析状态
	parseState, ok := state.(*dnsParseState)
	if !ok {
		parseState = &dnsParseState{
			buffer: make([]byte, 0, 4096),
		}
	}

	// 添加数据到缓冲区
	parseState.buffer = append(parseState.buffer, data...)

	result := &l7parser.ParseResult{
		ParserType: l7parser.ParserTypeDNS,
		Headers:    make(map[string]string),
	}

	// 解析 DNS 包
	packet, err := p.parsePacket(parseState)
	if err != nil {
		if err == ErrIncompleteDNS {
			result.IsPartial = true
			result.NeedMore = true
			return result, parseState, nil
		}
		return nil, parseState, err
	}

	if packet != nil {
		// 填充结果
		if packet.Header.IsQuery() {
			result.Direction = l7parser.DirRequest
		} else {
			result.Direction = l7parser.DirResponse
		}

		result.Headers["id"] = string(rune(packet.Header.ID))
		result.Headers["opcode"] = string(rune(packet.Header.GetOpcode()))
		result.Headers["rcode"] = string(rune(packet.Header.GetRcode()))

		if len(packet.Questions) > 0 {
			q := packet.Questions[0]
			result.Headers["query"] = q.Name
			result.Headers["qtype"] = GetDNSTypeName(q.Type)
		}

		result.Headers["qdcount"] = string(rune(packet.Header.QDCount))
		result.Headers["ancount"] = string(rune(packet.Header.ANCount))

		result.IsComplete = true
	}

	return result, parseState, nil
}

// parsePacket 解析 DNS 包
func (p *DNSParser) parsePacket(state *dnsParseState) (*DNSPacket, error) {
	data := state.buffer

	// 检查是否是 TCP DNS (2-byte length prefix)
	isTCP := false
	if len(data) >= 2 {
		tcpLen := binary.BigEndian.Uint16(data[0:2])
		if tcpLen > 0 && tcpLen < 65535 && int(tcpLen)+2 <= len(data) {
			// 可能是 TCP DNS
			isTCP = true
			data = data[2 : 2+tcpLen]
		}
	}

	if len(data) < 12 {
		return nil, ErrIncompleteDNS
	}

	// 解析 header
	header := DNSHeader{
		ID:      binary.BigEndian.Uint16(data[0:2]),
		Flags:   binary.BigEndian.Uint16(data[2:4]),
		QDCount: binary.BigEndian.Uint16(data[4:6]),
		ANCount: binary.BigEndian.Uint16(data[6:8]),
		NSCount: binary.BigEndian.Uint16(data[8:10]),
		ARCount: binary.BigEndian.Uint16(data[10:12]),
	}

	// 解析 questions
	questions := make([]DNSQuestion, 0, header.QDCount)
	offset := 12

	for i := uint16(0); i < header.QDCount; i++ {
		if offset >= len(data) {
			return nil, ErrIncompleteDNS
		}

		name, newOffset, err := p.parseDomainName(data, offset)
		if err != nil {
			return nil, err
		}
		offset = newOffset

		if offset+4 > len(data) {
			return nil, ErrIncompleteDNS
		}

		q := DNSQuestion{
			Name:  name,
			Type:  binary.BigEndian.Uint16(data[offset : offset+2]),
			Class: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}
		offset += 4

		questions = append(questions, q)
	}

	// 消费数据
	if isTCP {
		tcpLen := binary.BigEndian.Uint16(state.buffer[0:2])
		state.buffer = state.buffer[2+tcpLen:]
	} else {
		state.buffer = state.buffer[len(state.buffer):]
	}

	return &DNSPacket{
		Header:    header,
		Questions: questions,
		IsTCP:     isTCP,
	}, nil
}

// parseDomainName 解析域名
func (p *DNSParser) parseDomainName(data []byte, offset int) (string, error) {
	var labels []string
	visited := make(map[int]bool) // 防止无限循环

	for {
		if offset >= len(data) {
			return "", ErrIncompleteDNS
		}

		// 检查是否访问过 (防止压缩指针循环)
		if visited[offset] {
			return "", ErrInvalidDNS
		}
		visited[offset] = true

		length := int(data[offset])

		// 检查压缩指针 (11xxxxxx)
		if length&0xC0 == 0xC0 {
			if offset+2 > len(data) {
				return "", ErrIncompleteDNS
			}
			pointer := ((length & 0x3F) << 8) | int(data[offset+1])
			// 递归解析压缩指向的域名
			compressedName, _, err := p.parseDomainName(data, pointer)
			if err != nil {
				return "", err
			}
			if len(labels) > 0 {
				return strings.Join(labels, ".") + "." + compressedName, nil
			}
			return compressedName, nil
		}

		// 结束标志
		if length == 0 {
			offset++
			break
		}

		// 检查长度
		if length > 63 {
			return "", ErrInvalidDNS
		}

		offset++
		if offset+length > len(data) {
			return "", ErrIncompleteDNS
		}

		label := string(data[offset : offset+length])
		labels = append(labels, label)
		offset += length
	}

	return strings.Join(labels, "."), offset, nil
}

// ParseStreaming 流式解析
func (p *DNSParser) ParseStreaming(ctx context.Context, data []byte, state interface{}) (*l7parser.ParseResult, interface{}, error) {
	return p.Parse(ctx, &l7parser.ParserInput{
		Packet: l7parser.RawPacket{Data: data},
	}, state)
}

// Reset 重置解析器
func (p *DNSParser) Reset() {
	p.state = &dnsParseState{
		buffer: make([]byte, 0, 4096),
	}
}

// ============================================================================
// 便捷函数
// ============================================================================

// IsDNSQueryType 检查是否为查询类型
func IsDNSQueryType(qtype uint16) bool {
	return qtype == DNSTypeA || qtype == DNSTypeAAAA || qtype == DNSTypeCNAME ||
		qtype == DNSTypeMX || qtype == DNSTypeNS || qtype == DNSTypeTXT ||
		qtype == DNSTypeSRV || qtype == DNSTypePTR || qtype == DNSTypeSOA ||
		qtype == DNSTypeANY
}

// IsDNSError 检查是否为错误响应
func IsDNSError(rcode uint8) bool {
	return rcode != DNSRcodeNoError
}

// GetDNSErrorName 获取 DNS 错误名称
func GetDNSErrorName(rcode uint8) string {
	names := map[uint8]string{
		DNSRcodeNoError:  "NOERROR",
		DNSRcodeFormErr:  "FORMERR",
		DNSRcodeServFail: "SERVFAIL",
		DNSRcodeNXDomain: "NXDOMAIN",
		DNSRcodeNotImp:   "NOTIMP",
		DNSRcodeRefused:  "REFUSED",
	}
	if name, ok := names[rcode]; ok {
		return name
	}
	return "UNKNOWN"
}

// ============================================================================
// 注册
// ============================================================================

func init() {
	l7parser.MustRegister("dns", func() l7parser.Parser {
		return NewDNSParser()
	})
}
