package ebpfcollector

import (
	"encoding/binary"
	"net"
	"testing"
)

// buildValidKey 构造有效的 BPF key（12字节）
func buildValidKey(srcIP, dstIP string, srcPort, dstPort uint16) []byte {
	key := make([]byte, bpfKeySize)
	copy(key[0:4], net.ParseIP(srcIP).To4())
	copy(key[4:8], net.ParseIP(dstIP).To4())
	binary.BigEndian.PutUint16(key[8:10], srcPort)
	binary.BigEndian.PutUint16(key[10:12], dstPort)
	return key
}

// buildValidValue 构造有效的 BPF value（31字节）
func buildValidValue(dstIP string, dstPort uint16, protocol byte, bytes, packets, timestamp int64) []byte {
	value := make([]byte, bpfValueSize)
	copy(value[0:4], net.ParseIP(dstIP).To4())
	binary.BigEndian.PutUint16(value[4:6], dstPort)
	value[6] = protocol
	binary.BigEndian.PutUint64(value[7:15], uint64(bytes))
	binary.BigEndian.PutUint64(value[15:23], uint64(packets))
	binary.BigEndian.PutUint64(value[23:31], uint64(timestamp))
	return value
}

func TestParseNetworkData_ValidTCP(t *testing.T) {
	key := buildValidKey("10.0.0.1", "10.0.0.2", 12345, 80)
	value := buildValidValue("10.0.0.2", 80, 6, 1024, 10, 1700000000)

	flow := parseNetworkData(key, value)
	if flow == nil {
		t.Fatal("parseNetworkData should not return nil for valid input")
	}
	if flow.SrcIP != "10.0.0.1" {
		t.Errorf("SrcIP = %q, want %q", flow.SrcIP, "10.0.0.1")
	}
	if flow.DstIP != "10.0.0.2" {
		t.Errorf("DstIP = %q, want %q", flow.DstIP, "10.0.0.2")
	}
	if flow.SrcPort != 12345 {
		t.Errorf("SrcPort = %d, want %d", flow.SrcPort, 12345)
	}
	if flow.DstPort != 80 {
		t.Errorf("DstPort = %d, want %d", flow.DstPort, 80)
	}
	if flow.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want %q", flow.Protocol, "tcp")
	}
	if flow.Bytes != 1024 {
		t.Errorf("Bytes = %d, want %d", flow.Bytes, 1024)
	}
	if flow.Packets != 10 {
		t.Errorf("Packets = %d, want %d", flow.Packets, 10)
	}
}

func TestParseNetworkData_Protocols(t *testing.T) {
	tests := []struct {
		name     string
		protocol byte
		want     string
	}{
		{"tcp", 6, "tcp"},
		{"udp", 17, "udp"},
		{"icmp", 1, "icmp"},
		{"unknown", 99, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := buildValidKey("10.0.0.1", "10.0.0.2", 1000, 80)
			value := buildValidValue("10.0.0.2", 80, tt.protocol, 100, 1, 0)
			flow := parseNetworkData(key, value)
			if flow == nil {
				t.Fatal("parseNetworkData returned nil")
			}
			if flow.Protocol != tt.want {
				t.Errorf("Protocol = %q, want %q", flow.Protocol, tt.want)
			}
		})
	}
}

func TestParseNetworkData_InvalidKeyLength(t *testing.T) {
	value := buildValidValue("10.0.0.2", 80, 6, 100, 1, 0)
	// key 太短
	flow := parseNetworkData([]byte{1, 2, 3}, value)
	if flow != nil {
		t.Error("should return nil for invalid key length")
	}
	// key 太长
	longKey := make([]byte, bpfKeySize+10)
	flow = parseNetworkData(longKey, value)
	if flow != nil {
		t.Error("should return nil for invalid key length")
	}
}

func TestParseNetworkData_InvalidValueLength(t *testing.T) {
	key := buildValidKey("10.0.0.1", "10.0.0.2", 1000, 80)
	// value 太短
	flow := parseNetworkData(key, []byte{1, 2, 3})
	if flow != nil {
		t.Error("should return nil for invalid value length")
	}
	// value 太长
	longValue := make([]byte, bpfValueSize+10)
	flow = parseNetworkData(key, longValue)
	if flow != nil {
		t.Error("should return nil for invalid value length")
	}
}

func TestParseNetworkData_LargeByteValues(t *testing.T) {
	key := buildValidKey("192.168.1.1", "8.8.8.8", 443, 443)
	value := buildValidValue("8.8.8.8", 443, 6, 1<<40, 1<<30, 1700000000)
	flow := parseNetworkData(key, value)
	if flow == nil {
		t.Fatal("parseNetworkData returned nil")
	}
	if flow.Bytes != (1 << 40) {
		t.Errorf("Bytes = %d, want %d", flow.Bytes, int64(1<<40))
	}
	if flow.Packets != (1 << 30) {
		t.Errorf("Packets = %d, want %d", flow.Packets, int64(1<<30))
	}
}
