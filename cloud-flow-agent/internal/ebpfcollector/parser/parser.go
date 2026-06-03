// Package parser 提供协议深度解析功能
package parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	edge "cloudflow/proto"
)

// Parser 协议解析器接口
type Parser interface {
	Parse(data []byte) (map[string]string, error)
}

// NewParser 创建协议解析器
func NewParser(protocol string) Parser {
	switch protocol {
	case "http":
		return &HTTPParser{}
	case "https":
		return &HTTPSSParser{}
	case "tcp":
		return &TCPParser{}
	case "udp":
		return &UDPParser{}
	case "dns":
		return &DNSParser{}
	case "icmp":
		return &ICMPParser{}
	case "ssh":
		return &SSHParser{}
	case "ftp":
		return &FTPParser{}
	default:
		return &GenericParser{}
	}
}

// ParseNetworkData 解析网络流量数据
func ParseNetworkData(srcIP, dstIP string, srcPort, dstPort uint16, protocol string, data []byte) *edge.MetricData {
	// 创建协议解析器
	parser := NewParser(protocol)

	// 解析数据
	tags, err := parser.Parse(data)
	if err != nil {
		// 解析失败，使用默认标签
		tags = map[string]string{
			"src_port": fmt.Sprintf("%d", srcPort),
			"dst_port": fmt.Sprintf("%d", dstPort),
			"type":     "network_flow",
			"error":    err.Error(),
		}
	} else {
		// 添加基本标签
		tags["src_port"] = fmt.Sprintf("%d", srcPort)
		tags["dst_port"] = fmt.Sprintf("%d", dstPort)
		tags["type"] = "network_flow"
	}

	return &edge.MetricData{
		Timestamp: 0, // 会在采集器中设置
		SrcIp:     srcIP,
		DstIp:     dstIP,
		Protocol:  protocol,
		Bytes:     0, // 不在此处设置，由 eBPF 采集器层提供准确的字节数
		Packets:   0, // 不在此处设置，由 eBPF 采集器层提供准确的数据包数
		Tags:      tags,
	}
}

// HTTPParser HTTP协议解析器
type HTTPParser struct{}

// Parse 解析HTTP协议数据
func (p *HTTPParser) Parse(data []byte) (map[string]string, error) {
	// 查找HTTP请求行
	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("无效的HTTP数据")
	}

	// 解析请求行
	requestLine := lines[0]
	parts := strings.Split(requestLine, " ")
	if len(parts) < 3 {
		return nil, fmt.Errorf("无效的HTTP请求行")
	}

	method := parts[0]
	url := parts[1]
	version := parts[2]

	// 解析头部
	headers := make(map[string]string)
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			break
		}
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			headers[key] = value
		}
	}

	// 构建标签
	tags := map[string]string{
		"method":  method,
		"url":     url,
		"version": version,
	}

	// 添加关键头部
	if userAgent, ok := headers["User-Agent"]; ok {
		tags["user_agent"] = userAgent
	}
	if host, ok := headers["Host"]; ok {
		tags["host"] = host
	}
	if contentType, ok := headers["Content-Type"]; ok {
		tags["content_type"] = contentType
	}

	return tags, nil
}

// HTTPSSParser HTTPS协议解析器
type HTTPSSParser struct{}

// Parse 解析HTTPS协议数据
func (p *HTTPSSParser) Parse(data []byte) (map[string]string, error) {
	// HTTPS是加密的，只能解析TLS握手信息
	if len(data) < 5 {
		return nil, fmt.Errorf("无效的HTTPS数据")
	}

	// 检查是否为TLS握手
	if data[0] != 0x16 { // TLS握手消息类型
		return nil, fmt.Errorf("不是TLS握手数据")
	}

	// 解析TLS版本
	version := binary.BigEndian.Uint16(data[1:3])

	// 构建标签
	tags := map[string]string{
		"tls_version": fmt.Sprintf("%d.%d", version>>8, version&0xff),
		"encrypted":   "true",
	}

	return tags, nil
}

// TCPParser TCP协议解析器
type TCPParser struct{}

// Parse 解析TCP协议数据
func (p *TCPParser) Parse(data []byte) (map[string]string, error) {
	// TCP头部最小长度为20字节
	if len(data) < 20 {
		return nil, fmt.Errorf("无效的TCP数据")
	}

	// 解析TCP头部
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	seqNum := binary.BigEndian.Uint32(data[4:8])
	ackNum := binary.BigEndian.Uint32(data[8:12])
	offset := (data[12] >> 4) * 4
	flags := data[13]

	// 构建标签
	tags := map[string]string{
		"src_port": fmt.Sprintf("%d", srcPort),
		"dst_port": fmt.Sprintf("%d", dstPort),
		"seq_num":  fmt.Sprintf("%d", seqNum),
		"ack_num":  fmt.Sprintf("%d", ackNum),
		"offset":   fmt.Sprintf("%d", offset),
		"flags":    fmt.Sprintf("%08b", flags),
	}

	// 解析标志位
	if flags&0x01 != 0 {
		tags["fin"] = "true"
	}
	if flags&0x02 != 0 {
		tags["syn"] = "true"
	}
	if flags&0x04 != 0 {
		tags["rst"] = "true"
	}
	if flags&0x08 != 0 {
		tags["psh"] = "true"
	}
	if flags&0x10 != 0 {
		tags["ack"] = "true"
	}
	if flags&0x20 != 0 {
		tags["urg"] = "true"
	}

	return tags, nil
}

// UDPParser UDP协议解析器
type UDPParser struct{}

// Parse 解析UDP协议数据
func (p *UDPParser) Parse(data []byte) (map[string]string, error) {
	// UDP头部长度为8字节
	if len(data) < 8 {
		return nil, fmt.Errorf("无效的UDP数据")
	}

	// 解析UDP头部
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	length := binary.BigEndian.Uint16(data[4:6])
	checksum := binary.BigEndian.Uint16(data[6:8])

	// 构建标签
	tags := map[string]string{
		"src_port":  fmt.Sprintf("%d", srcPort),
		"dst_port":  fmt.Sprintf("%d", dstPort),
		"length":    fmt.Sprintf("%d", length),
		"checksum":  fmt.Sprintf("%d", checksum),
		"payload_size": fmt.Sprintf("%d", len(data)-8),
	}

	return tags, nil
}

// GenericParser 通用协议解析器
type GenericParser struct{}

// Parse 解析通用协议数据
func (p *GenericParser) Parse(data []byte) (map[string]string, error) {
	// 通用解析器，只返回基本信息
	tags := map[string]string{
		"payload_size": fmt.Sprintf("%d", len(data)),
		"protocol":     "generic",
	}

	return tags, nil
}

// DNSParser DNS协议解析器
type DNSParser struct{}

// Parse 解析DNS协议数据
func (p *DNSParser) Parse(data []byte) (map[string]string, error) {
	// DNS头部长度为12字节
	if len(data) < 12 {
		return nil, fmt.Errorf("无效的DNS数据")
	}

	// 解析DNS头部
	id := binary.BigEndian.Uint16(data[0:2])
	flags := binary.BigEndian.Uint16(data[2:4])
	qdcount := binary.BigEndian.Uint16(data[4:6])
	ancount := binary.BigEndian.Uint16(data[6:8])
	nscount := binary.BigEndian.Uint16(data[8:10])
	arcount := binary.BigEndian.Uint16(data[10:12])

	// 构建标签
	tags := map[string]string{
		"id":       fmt.Sprintf("%d", id),
		"flags":    fmt.Sprintf("%016b", flags),
		"qdcount":  fmt.Sprintf("%d", qdcount),
		"ancount":  fmt.Sprintf("%d", ancount),
		"nscount":  fmt.Sprintf("%d", nscount),
		"arcount":  fmt.Sprintf("%d", arcount),
	}

	// 解析查询部分
	if qdcount > 0 {
		// 跳过头部
		offset := 12
		// 解析域名
		domain := ""
		maxIterations := 1000 // 防止无限循环
		iter := 0
		for iter < maxIterations {
			if offset >= len(data) {
				break // 数据不足，停止解析
			}
			length := int(data[offset])
			offset++
			if length == 0 {
				break
			}
			if offset+length > len(data) {
				break // 数据不足，停止解析
			}
			if domain != "" {
				domain += "."
			}
			domain += string(data[offset:offset+length])
			offset += length
			iter++
		}
		// 解析查询类型和类
		if offset+4 <= len(data) {
			qtype := binary.BigEndian.Uint16(data[offset:offset+2])
			qclass := binary.BigEndian.Uint16(data[offset+2:offset+4])
			tags["qname"] = domain
			tags["qtype"] = fmt.Sprintf("%d", qtype)
			tags["qclass"] = fmt.Sprintf("%d", qclass)
		}
	}

	return tags, nil
}

// ICMPParser ICMP协议解析器
type ICMPParser struct{}

// Parse 解析ICMP协议数据
func (p *ICMPParser) Parse(data []byte) (map[string]string, error) {
	// ICMP头部长度为8字节
	if len(data) < 8 {
		return nil, fmt.Errorf("无效的ICMP数据")
	}

	// 解析ICMP头部
	typeField := data[0]
	code := data[1]
	checksum := binary.BigEndian.Uint16(data[2:4])
	identifier := binary.BigEndian.Uint16(data[4:6])
	sequence := binary.BigEndian.Uint16(data[6:8])

	// 构建标签
	tags := map[string]string{
		"type":       fmt.Sprintf("%d", typeField),
		"code":       fmt.Sprintf("%d", code),
		"checksum":   fmt.Sprintf("%d", checksum),
		"identifier": fmt.Sprintf("%d", identifier),
		"sequence":   fmt.Sprintf("%d", sequence),
	}

	// 解析ICMP类型
	switch typeField {
	case 0:
		tags["type_name"] = "Echo Reply"
	case 8:
		tags["type_name"] = "Echo Request"
	case 3:
		tags["type_name"] = "Destination Unreachable"
	case 11:
		tags["type_name"] = "Time Exceeded"
	case 12:
		tags["type_name"] = "Parameter Problem"
	case 5:
		tags["type_name"] = "Redirect"
	default:
		tags["type_name"] = "Unknown"
	}

	return tags, nil
}

// SSHParser SSH协议解析器
type SSHParser struct{}

// Parse 解析SSH协议数据
func (p *SSHParser) Parse(data []byte) (map[string]string, error) {
	// SSH协议以"SSH-"开头
	if len(data) < 8 || !bytes.HasPrefix(data, []byte("SSH-")) {
		return nil, fmt.Errorf("无效的SSH数据")
	}

	// 解析SSH版本
	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("无效的SSH数据")
	}

	versionLine := lines[0]
	if !strings.HasPrefix(versionLine, "SSH-") {
		return nil, fmt.Errorf("无效的SSH版本")
	}

	// 构建标签
	tags := map[string]string{
		"version": versionLine,
	}

	// 提取SSH协议版本
	parts := strings.Split(versionLine, "-")
	if len(parts) >= 3 {
		tags["protocol"] = parts[1]
		tags["software"] = parts[2]
	}

	return tags, nil
}

// FTPParser FTP协议解析器
type FTPParser struct{}

// Parse 解析FTP协议数据
func (p *FTPParser) Parse(data []byte) (map[string]string, error) {
	// FTP响应以数字状态码开头
	if len(data) < 3 {
		return nil, fmt.Errorf("无效的FTP数据")
	}

	// 解析FTP响应
	lines := strings.Split(string(data), "\r\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("无效的FTP数据")
	}

	responseLine := lines[0]
	if len(responseLine) < 3 {
		return nil, fmt.Errorf("无效的FTP响应")
	}

	// 构建标签
	tags := map[string]string{
		"response": responseLine,
	}

	// 提取状态码
	statusCode := responseLine[:3]
	tags["status_code"] = statusCode

	// 解析状态码含义
	switch statusCode {
	case "120":
		tags["status_message"] = "Service ready in nnn minutes"
	case "125":
		tags["status_message"] = "Data connection already open; transfer starting"
	case "150":
		tags["status_message"] = "File status okay; about to open data connection"
	case "200":
		tags["status_message"] = "Command okay"
	case "211":
		tags["status_message"] = "System status, or system help reply"
	case "212":
		tags["status_message"] = "Directory status"
	case "213":
		tags["status_message"] = "File status"
	case "214":
		tags["status_message"] = "Help message"
	case "215":
		tags["status_message"] = "NAME system type"
	case "220":
		tags["status_message"] = "Service ready for new user"
	case "221":
		tags["status_message"] = "Service closing control connection"
	case "225":
		tags["status_message"] = "Data connection open; no transfer in progress"
	case "226":
		tags["status_message"] = "Closing data connection"
	case "227":
		tags["status_message"] = "Entering Passive Mode"
	case "230":
		tags["status_message"] = "User logged in, proceed"
	case "250":
		tags["status_message"] = "Requested file action okay, completed"
	case "257":
		tags["status_message"] = "PATHNAME created"
	case "331":
		tags["status_message"] = "User name okay, need password"
	case "332":
		tags["status_message"] = "Need account for login"
	case "350":
		tags["status_message"] = "Requested file action pending further information"
	case "421":
		tags["status_message"] = "Service not available, closing control connection"
	case "425":
		tags["status_message"] = "Can't open data connection"
	case "426":
		tags["status_message"] = "Connection closed; transfer aborted"
	case "450":
		tags["status_message"] = "Requested file action not taken"
	case "451":
		tags["status_message"] = "Requested action aborted: local error in processing"
	case "452":
		tags["status_message"] = "Requested action not taken: insufficient storage space"
	case "500":
		tags["status_message"] = "Syntax error, command unrecognized"
	case "501":
		tags["status_message"] = "Syntax error in parameters or arguments"
	case "502":
		tags["status_message"] = "Command not implemented"
	case "503":
		tags["status_message"] = "Bad sequence of commands"
	case "504":
		tags["status_message"] = "Command not implemented for that parameter"
	case "530":
		tags["status_message"] = "Not logged in"
	case "532":
		tags["status_message"] = "Need account for storing files"
	case "550":
		tags["status_message"] = "Requested action not taken: file unavailable"
	case "551":
		tags["status_message"] = "Requested action aborted: page type unknown"
	case "552":
		tags["status_message"] = "Requested file action aborted: exceeded storage allocation"
	case "553":
		tags["status_message"] = "Requested action not taken: file name not allowed"
	default:
		tags["status_message"] = "Unknown"
	}

	return tags, nil
}
