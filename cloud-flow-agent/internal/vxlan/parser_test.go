package vxlan

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVXLANHeader_Parse(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected VXLANHeader
		wantErr  bool
	}{
		{
			name: "valid VXLAN header with VNI 100",
			input: []byte{
				0x08, 0x00, 0x00, 0x00, // Flags + Reserved
				0x00, 0x00, 0x64, 0x00, // VNI=100 + Reserved2
			},
			expected: VXLANHeader{
				Flags:     0x08,
				Reserved1: [3]uint8{0, 0, 0},
				VNI:       [3]uint8{0, 0, 0x64},
				Reserved2: 0,
			},
			wantErr: false,
		},
		{
			name: "valid VXLAN header with VNI 1000",
			input: []byte{
				0x08, 0x00, 0x00, 0x00,
				0x00, 0x03, 0xE8, 0x00, // VNI=1000
			},
			expected: VXLANHeader{
				Flags:     0x08,
				Reserved1: [3]uint8{0, 0, 0},
				VNI:       [3]uint8{0, 0x03, 0xE8},
				Reserved2: 0,
			},
			wantErr: false,
		},
		{
			name:    "too short packet",
			input:   []byte{0x08, 0x00, 0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			hdr, err := parser.ParseVXLANHeader(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.Flags, hdr.Flags)
			assert.Equal(t, tt.expected.VNI, hdr.VNI)
		})
	}
}

func TestVXLANHeader_GetVNI(t *testing.T) {
	hdr := VXLANHeader{
		VNI: [3]uint8{0x00, 0x00, 0x64},
	}
	assert.Equal(t, uint32(100), hdr.GetVNI())

	hdr.VNI = [3]uint8{0x00, 0x03, 0xE8}
	assert.Equal(t, uint32(1000), hdr.GetVNI())

	hdr.VNI = [3]uint8{0x01, 0x00, 0x00}
	assert.Equal(t, uint32(65536), hdr.GetVNI())
}

func TestEthernetHeader_Parse(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected EthernetHeader
		wantErr  bool
	}{
		{
			name: "valid ethernet header IPv4",
			input: []byte{
				0x00, 0x11, 0x22, 0x33, 0x44, 0x55, // DstMAC
				0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, // SrcMAC
				0x08, 0x00, // EtherType IPv4
			},
			expected: EthernetHeader{
				DstMAC:    net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
				SrcMAC:    net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				EtherType: 0x0800,
			},
			wantErr: false,
		},
		{
			name: "valid ethernet header IPv6",
			input: []byte{
				0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
				0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
				0x86, 0xDD, // EtherType IPv6
			},
			expected: EthernetHeader{
				DstMAC:    net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
				SrcMAC:    net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				EtherType: 0x86DD,
			},
			wantErr: false,
		},
		{
			name:    "too short packet",
			input:   []byte{0x00, 0x11, 0x22},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			hdr, err := parser.ParseEthernetHeader(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.DstMAC, hdr.DstMAC)
			assert.Equal(t, tt.expected.SrcMAC, hdr.SrcMAC)
			assert.Equal(t, tt.expected.EtherType, hdr.EtherType)
		})
	}
}

func TestIPv4Header_Parse(t *testing.T) {
	// 构建最小IPv4头（20字节）
	input := make([]byte, 20)
	input[0] = 0x45 // Version 4, IHL 5
	input[9] = 6     // TCP protocol
	binary.BigEndian.PutUint16(input[12:14], 0xC0A80101>>16) // 192.168.1.1
	input[12] = 192
	input[13] = 168
	input[14] = 1
	input[15] = 1
	input[16] = 10
	input[17] = 0
	input[18] = 0
	input[19] = 1

	parser := NewParser()
	hdr, err := parser.ParseIPv4Header(input)
	require.NoError(t, err)
	assert.Equal(t, uint8(4), hdr.Version)
	assert.Equal(t, uint8(6), hdr.Protocol)
	assert.Equal(t, net.IPv4(192, 168, 1, 1), hdr.SrcIP)
	assert.Equal(t, net.IPv4(10, 0, 0, 1), hdr.DstIP)
}

func TestTCPHeader_Parse(t *testing.T) {
	input := make([]byte, 20)
	binary.BigEndian.PutUint16(input[0:2], 80)    // SrcPort
	binary.BigEndian.PutUint16(input[2:4], 54321) // DstPort
	input[12] = 0x50 // Data offset 5

	parser := NewParser()
	hdr, err := parser.ParseTCPHeader(input)
	require.NoError(t, err)
	assert.Equal(t, uint16(80), hdr.SrcPort)
	assert.Equal(t, uint16(54321), hdr.DstPort)
}

func TestUDPHeader_Parse(t *testing.T) {
	input := make([]byte, 8)
	binary.BigEndian.PutUint16(input[0:2], 53)    // SrcPort DNS
	binary.BigEndian.PutUint16(input[2:4], 12345) // DstPort

	parser := NewParser()
	hdr, err := parser.ParseUDPHeader(input)
	require.NoError(t, err)
	assert.Equal(t, uint16(53), hdr.SrcPort)
	assert.Equal(t, uint16(12345), hdr.DstPort)
}

func TestIsVXLANPacket(t *testing.T) {
	tests := []struct {
		name     string
		dstPort  uint16
		expected bool
	}{
		{"VXLAN port 8472", 8472, true},
		{"VXLAN port 4789", 4789, true},
		{"other port", 80, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsVXLANPacket(tt.dstPort))
		})
	}
}

func TestEtherTypeToString(t *testing.T) {
	tests := []struct {
		etherType uint16
		expected  string
	}{
		{0x0800, "IPv4"},
		{0x86DD, "IPv6"},
		{0x0806, "ARP"},
		{0xFFFF, "UNKNOWN(0xffff)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, EtherTypeToString(tt.etherType))
		})
	}
}

func TestProtocolToString(t *testing.T) {
	tests := []struct {
		proto    uint8
		expected string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{1, "ICMP"},
		{99, "UNKNOWN(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, ProtocolToString(tt.proto))
		})
	}
}

func TestParserStats(t *testing.T) {
	parser := NewParser()

	// 初始状态
	stats := parser.GetStats()
	assert.Equal(t, uint64(0), stats.PacketsTotal)
	assert.Equal(t, uint64(0), stats.PacketsParsed)
	assert.Equal(t, uint64(0), stats.PacketsDropped)

	// 更新统计
	parser.IncrementPacketsTotal()
	parser.IncrementPacketsParsed()
	parser.IncrementPacketsDropped()

	stats = parser.GetStats()
	assert.Equal(t, uint64(1), stats.PacketsTotal)
	assert.Equal(t, uint64(1), stats.PacketsParsed)
	assert.Equal(t, uint64(1), stats.PacketsDropped)

	// 重置
	parser.ResetStats()
	stats = parser.GetStats()
	assert.Equal(t, uint64(0), stats.PacketsTotal)
}

func TestParseFullPacket(t *testing.T) {
	// 构建一个完整的VXLAN包（简化版）
	parser := NewParser()

	// 测试空包
	flow, err := parser.ParsePacket(nil)
	assert.Error(t, err)
	assert.Nil(t, flow)

	// 测试太短的包
	flow, err = parser.ParsePacket([]byte{0x00, 0x01, 0x02})
	assert.Error(t, err)
	assert.Nil(t, flow)
}

func TestVXLANFlow_String(t *testing.T) {
	flow := &VXLANFlow{
		VNI:      100,
		SrcIP:    net.IPv4(192, 168, 1, 1),
		DstIP:    net.IPv4(10, 0, 0, 1),
		SrcPort:  54321,
		DstPort:  80,
		Protocol: 6,
		Bytes:    1024,
		Packets:  10,
	}

	str := flow.String()
	assert.Contains(t, str, "VNI=100")
	assert.Contains(t, str, "192.168.1.1")
	assert.Contains(t, str, "10.0.0.1")
	assert.Contains(t, str, "TCP")
}
