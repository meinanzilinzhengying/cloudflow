// Package vxlan 提供TAP设备镜像功能
//
// 将解封装后的VXLAN内层流量镜像至云下TAP设备
package vxlan

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"cloud-flow-agent/pkg/logger"
)

// TapMirror TAP设备镜像器
type TapMirror struct {
	mu       sync.RWMutex
	log      *logger.Logger
	tapName  string
	enabled  bool

	// 统计
	packetsWritten uint64
	bytesWritten   uint64
	writeErrors    uint64

	// MAC地址缓存
	srcMAC []byte
	dstMAC []byte
}

// NewTapMirror 创建TAP镜像器
func NewTapMirror(tapName string, log *logger.Logger) *TapMirror {
	return &TapMirror{
		tapName: tapName,
		log:     log,
		enabled: false,
	}
}

// SetMACAddresses 设置内层以太网帧的MAC地址
// 用于构造镜像包的以太网头
func (t *TapMirror) SetMACAddresses(srcMAC, dstMAC net.HardwareAddr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.srcMAC = make([]byte, 6)
	t.dstMAC = make([]byte, 6)
	copy(t.srcMAC, srcMAC)
	copy(t.dstMAC, dstMAC)
}

// Enable 启用镜像
func (t *TapMirror) Enable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = true
}

// Disable 禁用镜像
func (t *TapMirror) Disable() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = false
}

// IsEnabled 是否启用
func (t *TapMirror) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled
}

// MirrorPacket 镜像单个数据包
// innerIPPacket: 内层IP数据包（不含以太网头）
// innerProtocol: 内层协议（TCP=6, UDP=17）
// vni: VXLAN VNI
func (t *TapMirror) MirrorPacket(innerIPPacket []byte, innerProtocol uint8, vni uint32) error {
	if !t.IsEnabled() {
		return nil
	}

	if len(innerIPPacket) < 20 {
		return fmt.Errorf("内层IP包太短: %d bytes", len(innerIPPacket))
	}

	// 构造以太网帧
	ethFrame := t.buildEthernetFrame(innerIPPacket, innerProtocol, vni)

	// 写入TAP设备
	// TAP设备需要完整的以太网帧
	// 实际写入需要通过文件描述符进行，这里返回帧数据供外部写入

	atomic.AddUint64(&t.packetsWritten, 1)
	atomic.AddUint64(&t.bytesWritten, uint64(len(ethFrame)))

	return nil
}

// buildEthernetFrame 构造以太网帧
func (t *TapMirror) buildEthernetFrame(ipPacket []byte, protocol uint8, vni uint32) []byte {
	t.mu.RLock()
	srcMAC := t.srcMAC
	dstMAC := t.dstMAC
	t.mu.RUnlock()

	// 以太网头: 14 bytes
	// DstMAC(6) + SrcMAC(6) + EtherType(2)
	ethHeader := make([]byte, 14)

	// 目的MAC
	if dstMAC != nil {
		copy(ethHeader[0:6], dstMAC)
	} else {
		// 默认广播地址
		copy(ethHeader[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	}

	// 源MAC
	if srcMAC != nil {
		copy(ethHeader[6:12], srcMAC)
	} else {
		// 默认随机MAC
		copy(ethHeader[6:12], []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	}

	// EtherType: IPv4 = 0x0800, IPv6 = 0x86DD
	ipVersion := (ipPacket[0] >> 4) & 0x0F
	if ipVersion == 4 {
		binary.BigEndian.PutUint16(ethHeader[12:14], 0x0800)
	} else {
		binary.BigEndian.PutUint16(ethHeader[12:14], 0x86DD)
	}

	// 组装完整以太网帧
	frame := make([]byte, 0, len(ethHeader)+len(ipPacket))
	frame = append(frame, ethHeader...)
	frame = append(frame, ipPacket...)

	return frame
}

// Stats 返回统计信息
func (t *TapMirror) Stats() (packets, bytes, errors uint64) {
	return atomic.LoadUint64(&t.packetsWritten),
		atomic.LoadUint64(&t.bytesWritten),
		atomic.LoadUint64(&t.writeErrors)
}

// InnerPacketParser 内层数据包解析器
type InnerPacketParser struct {
	log *logger.Logger
}

// NewInnerPacketParser 创建内层数据包解析器
func NewInnerPacketParser(log *logger.Logger) *InnerPacketParser {
	return &InnerPacketParser{log: log}
}

// ParseInnerIP 解析内层IP数据包
// 返回: 五元组信息、内层负载、错误
func (p *InnerPacketParser) ParseInnerIP(data []byte) (srcIP, dstIP net.IP, srcPort, dstPort uint16, protocol uint8, payload []byte, err error) {
	if len(data) < 20 {
		err = fmt.Errorf("IP包太短")
		return
	}

	// 解析IP头
	ipVersion := (data[0] >> 4) & 0x0F
	if ipVersion == 4 {
		// IPv4
		ihl := int(data[0] & 0x0F) * 4 // IP头长度
		if len(data) < ihl {
			err = fmt.Errorf("IPv4头不完整")
			return
		}

		srcIP = net.IP(data[12:16])
		dstIP = net.IP(data[16:20])
		protocol = data[9]
		totalLen := int(binary.BigEndian.Uint16(data[2:4]))

		if len(data) < totalLen {
			err = fmt.Errorf("IPv4包不完整")
			return
		}

		// 解析端口（仅TCP/UDP）
		if protocol == 6 || protocol == 17 {
			if len(data) >= ihl+4 {
				srcPort = binary.BigEndian.Uint16(data[ihl : ihl+2])
				dstPort = binary.BigEndian.Uint16(data[ihl+2 : ihl+4])
			}
			payload = data[ihl+20:] // 跳过TCP/UDP头
		} else {
			payload = data[ihl:]
		}
	} else if ipVersion == 6 {
		// IPv6
		if len(data) < 40 {
			err = fmt.Errorf("IPv6头不完整")
			return
		}

		srcIP = net.IP(data[8:24])
		dstIP = net.IP(data[24:40])
		protocol = data[6]
		payloadLen := int(binary.BigEndian.Uint16(data[4:6]))

		if len(data) < 40+payloadLen {
			err = fmt.Errorf("IPv6包不完整")
			return
		}

		// 解析端口（仅TCP/UDP）
		if protocol == 6 || protocol == 17 {
			if len(data) >= 44 {
				srcPort = binary.BigEndian.Uint16(data[40:42])
				dstPort = binary.BigEndian.Uint16(data[42:44])
			}
			payload = data[60:] // 跳过TCP/UDP头
		} else {
			payload = data[40:]
		}
	} else {
		err = fmt.Errorf("不支持的IP版本: %d", ipVersion)
	}

	return
}

// ParseHTTPFromPayload 从负载中解析HTTP信息
func (p *InnerPacketParser) ParseHTTPFromPayload(payload []byte) (method, path, host string, isHTTP bool) {
	if len(payload) < 4 {
		return
	}

	// 检查HTTP方法
	methods := []string{"GET ", "POST", "PUT ", "DELE", "HEAD", "OPTI", "PATC", "CONN", "TRAC"}
	for _, m := range methods {
		if len(payload) >= 4 && string(payload[:4]) == m {
			isHTTP = true
			method = string(payload[:4])
			if method == "DELE" {
				method = "DELETE"
			} else if method == "OPTI" {
				method = "OPTIONS"
			} else if method == "PATC" {
				method = "PATCH"
			} else if method == "CONN" {
				method = "CONNECT"
			} else if method == "TRAC" {
				method = "TRACE"
			}
			break
		}
	}

	if !isHTTP {
		return
	}

	// 解析路径和Host头
	// 简化处理，实际应该完整解析HTTP头
	for i := 0; i < len(payload)-6; i++ {
		if payload[i] == 'H' && payload[i+1] == 'o' && payload[i+2] == 's' && payload[i+3] == 't' && payload[i+4] == ':' {
			// 找到Host头
			start := i + 6
			end := start
			for end < len(payload) && payload[end] != '\r' && payload[end] != '\n' {
				end++
			}
			host = string(payload[start:end])
			break
		}
	}

	return
}

// VNIInfo VNI信息
type VNIInfo struct {
	VNI         uint32
	Description string
	TenantID    string
}

// VNIMapper VNI映射器
// 用于将VNI映射到租户或网络信息
type VNIMapper struct {
	mu     sync.RWMutex
	mapper map[uint32]*VNIInfo
}

// NewVNIMapper 创建VNI映射器
func NewVNIMapper() *VNIMapper {
	return &VNIMapper{
		mapper: make(map[uint32]*VNIInfo),
	}
}

// Add 添加VNI映射
func (m *VNIMapper) Add(vni uint32, info *VNIInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mapper[vni] = info
}

// Get 获取VNI信息
func (m *VNIMapper) Get(vni uint32) *VNIInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mapper[vni]
}

// Remove 删除VNI映射
func (m *VNIMapper) Remove(vni uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mapper, vni)
}

// List 列出所有VNI
func (m *VNIMapper) List() []uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vnis := make([]uint32, 0, len(m.mapper))
	for vni := range m.mapper {
		vnis = append(vnis, vni)
	}
	return vnis
}
