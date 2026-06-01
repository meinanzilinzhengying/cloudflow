package parser

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestNewParser(t *testing.T) {
	tests := []struct {
		protocol string
		wantType string
	}{
		{"http", "*HTTPParser"},
		{"https", "*HTTPSSParser"},
		{"tcp", "*TCPParser"},
		{"udp", "*UDPParser"},
		{"dns", "*DNSParser"},
		{"icmp", "*ICMPParser"},
		{"ssh", "*SSHParser"},
		{"ftp", "*FTPParser"},
		{"unknown", "*GenericParser"},
		{"", "*GenericParser"},
	}
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			p := NewParser(tt.protocol)
			if p == nil {
				t.Fatal("NewParser() should not return nil")
			}
			gotType := fmt.Sprintf("%T", p)
			if gotType != tt.wantType {
				t.Errorf("NewParser(%q) type = %s, want %s", tt.protocol, gotType, tt.wantType)
			}
		})
	}
}

func TestHTTPParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantKey string
		wantVal string
	}{
		{
			name:    "valid GET request",
			input:   "GET /index.html HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test\r\n\r\n",
			wantKey: "method",
			wantVal: "GET",
		},
		{
			name:    "valid POST request",
			input:   "POST /api HTTP/1.1\r\nContent-Type: application/json\r\n\r\n",
			wantKey: "method",
			wantVal: "POST",
		},
		{
			name:    "empty data",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid request line",
			input:   "INVALID\r\n\r\n",
			wantErr: true,
		},
		{
			name:    "with content-type",
			input:   "GET / HTTP/1.1\r\nContent-Type: text/html\r\n\r\n",
			wantKey: "content_type",
			wantVal: "text/html",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &HTTPParser{}
			tags, err := p.Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tags[tt.wantKey] != tt.wantVal {
					t.Errorf("tags[%q] = %q, want %q", tt.wantKey, tags[tt.wantKey], tt.wantVal)
				}
			}
		})
	}
}

func TestHTTPSSParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid TLS handshake", []byte{0x16, 0x03, 0x01, 0x00, 0x05}, false},
		{"too short", []byte{0x16, 0x03}, true},
		{"not TLS", []byte{0x17, 0x03, 0x01, 0x00, 0x05}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &HTTPSSParser{}
			tags, err := p.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tags["encrypted"] != "true" {
				t.Errorf("expected encrypted=true, got %v", tags)
			}
		})
	}
}

func TestTCPParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid TCP header", make([]byte, 20), false},
		{"too short", make([]byte, 10), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &TCPParser{}
			tags, err := p.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if _, ok := tags["src_port"]; !ok {
					t.Error("should have src_port tag")
				}
				if _, ok := tags["flags"]; !ok {
					t.Error("should have flags tag")
				}
			}
		})
	}
}

func TestTCPParser_ParseFlags(t *testing.T) {
	data := make([]byte, 20)
	data[13] = 0x12 // SYN+ACK
	p := &TCPParser{}
	tags, err := p.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if tags["syn"] != "true" || tags["ack"] != "true" {
		t.Errorf("SYN+ACK flags not detected: %v", tags)
	}
	if tags["fin"] == "true" || tags["rst"] == "true" {
		t.Errorf("FIN/RST should not be set: %v", tags)
	}
}

func TestUDPParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid UDP header", make([]byte, 8), false},
		{"too short", make([]byte, 4), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &UDPParser{}
			tags, err := p.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tags["payload_size"] != "0" {
				t.Errorf("payload_size should be 0 for header-only, got %s", tags["payload_size"])
			}
		})
	}
}

func TestDNSParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"valid DNS header", make([]byte, 12), false},
		{"too short", make([]byte, 8), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &DNSParser{}
			tags, err := p.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDNSParser_ParseWithQuery(t *testing.T) {
	// Construct a DNS query for "example.com" A record
	data := make([]byte, 32)
	binary.BigEndian.PutUint16(data[0:2], 0x1234) // ID
	binary.BigEndian.PutUint16(data[2:4], 0x0100) // Flags: standard query
	binary.BigEndian.PutUint16(data[4:6], 1)      // QDCOUNT: 1
	// Domain: example.com
	data[12] = 7
	copy(data[13:20], []byte("example"))
	data[20] = 3
	copy(data[21:24], []byte("com"))
	data[24] = 0 // end of domain
	binary.BigEndian.PutUint16(data[25:27], 1) // QTYPE: A
	binary.BigEndian.PutUint16(data[27:29], 1) // QCLASS: IN

	p := &DNSParser{}
	tags, err := p.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if tags["qname"] != "example.com" {
		t.Errorf("qname = %q, want %q", tags["qname"], "example.com")
	}
	if tags["qtype"] != "1" {
		t.Errorf("qtype = %q, want %q", tags["qtype"], "1")
	}
}

func TestICMPParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
		wantType string
	}{
		{"echo request", []byte{8, 0, 0, 0, 0, 1, 0, 1}, false, "Echo Request"},
		{"echo reply", []byte{0, 0, 0, 0, 0, 1, 0, 1}, false, "Echo Reply"},
		{"too short", []byte{8, 0}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ICMPParser{}
			tags, err := p.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tags["type_name"] != tt.wantType {
				t.Errorf("type_name = %q, want %q", tags["type_name"], tt.wantType)
			}
		})
	}
}

func TestSSHParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantKey string
		wantVal string
	}{
		{"valid SSH-2.0", "SSH-2.0-OpenSSH_8.9\r\n", false, "protocol", "2.0"},
		{"valid SSH-1.99", "SSH-1.99-OpenSSH_7.0\r\n", false, "protocol", "1.99"},
		{"not SSH", "HTTP/1.1 200 OK\r\n", true, "", ""},
		{"too short", "SSH", true, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SSHParser{}
			tags, err := p.Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.wantKey != "" && tags[tt.wantKey] != tt.wantVal {
				t.Errorf("tags[%q] = %q, want %q", tt.wantKey, tags[tt.wantKey], tt.wantVal)
			}
		})
	}
}

func TestFTPParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantCode string
	}{
		{"220 ready", "220 Service ready for new user\r\n", false, "220"},
		{"200 OK", "200 Command okay\r\n", false, "200"},
		{"530 not logged in", "530 Not logged in\r\n", false, "530"},
		{"too short", "22\r\n", true, ""},
		{"empty", "", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &FTPParser{}
			tags, err := p.Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tags["status_code"] != tt.wantCode {
				t.Errorf("status_code = %q, want %q", tags["status_code"], tt.wantCode)
			}
		})
	}
}

func TestGenericParser_Parse(t *testing.T) {
	p := &GenericParser{}
	tags, err := p.Parse([]byte("some data"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if tags["protocol"] != "generic" {
		t.Errorf("protocol = %q, want %q", tags["protocol"], "generic")
	}
	if tags["payload_size"] != "9" {
		t.Errorf("payload_size = %q, want %q", tags["payload_size"], "9")
	}
}

func TestParseNetworkData(t *testing.T) {
	result := ParseNetworkData("10.0.0.1", "10.0.0.2", 12345, 80, "tcp", []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\n"))
	if result == nil {
		t.Fatal("ParseNetworkData() should not return nil")
	}
	if result.SrcIp != "10.0.0.1" {
		t.Errorf("SrcIp = %q, want %q", result.SrcIp, "10.0.0.1")
	}
	if result.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want %q", result.Protocol, "tcp")
	}
	if result.Tags["src_port"] != "12345" {
		t.Errorf("src_port = %q, want %q", result.Tags["src_port"], "12345")
	}
}

func TestParseNetworkData_InvalidInput(t *testing.T) {
	result := ParseNetworkData("10.0.0.1", "10.0.0.2", 12345, 80, "tcp", []byte("INVALID"))
	if result == nil {
		t.Fatal("ParseNetworkData() should not return nil even for invalid data")
	}
	if result.Tags["error"] == "" {
		t.Error("should have error tag for invalid data")
	}
}
