// Package l7parser L7 协议解析引擎测试

package l7parser

import (
	"context"
	"testing"
	"time"

	"cloud-flow/cloud-flow-agent/internal/l7parser/parsers"
)

// ============================================================================
// HTTP/1.x 解析测试
// ============================================================================

func TestHTTP1Parser(t *testing.T) {
	parser := parsers.NewHTTPParser()

	tests := []struct {
		name     string
		data     []byte
		wantType ParserType
		wantDir  PacketDirection
		wantPath string
	}{
		{
			name:     "GET request",
			data:     []byte("GET /api/users HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			wantType: ParserTypeHTTP1,
			wantDir:  DirRequest,
			wantPath: "/api/users",
		},
		{
			name:     "POST request",
			data:     []byte("POST /api/users HTTP/1.1\r\nHost: example.com\r\nContent-Length: 13\r\n\r\n{\"name\":\"test\"}"),
			wantType: ParserTypeHTTP1,
			wantDir:  DirRequest,
			wantPath: "/api/users",
		},
		{
			name:     "HTTP response",
			data:     []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}"),
			wantType: ParserTypeHTTP1,
			wantDir:  DirResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &ParserInput{
				Packet: RawPacket{Data: tt.data},
			}
			result, _, err := parser.Parse(context.Background(), input, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			if result == nil {
				t.Fatal("Expected result, got nil")
			}
			if result.ParserType != tt.wantType {
				t.Errorf("ParserType = %v, want %v", result.ParserType, tt.wantType)
			}
			if result.Direction != tt.wantDir {
				t.Errorf("Direction = %v, want %v", result.Direction, tt.wantDir)
			}
			if tt.wantPath != "" && result.Path != tt.wantPath {
				t.Errorf("Path = %v, want %v", result.Path, tt.wantPath)
			}
		})
	}
}

func TestHTTP1Detect(t *testing.T) {
	parser := parsers.NewHTTPParser()

	tests := []struct {
		name     string
		data     []byte
		dstPort  uint16
		wantMatch bool
	}{
		{
			name:      "GET on port 80",
			data:      []byte("GET / HTTP/1.1\r\n"),
			dstPort:   80,
			wantMatch: true,
		},
		{
			name:      "POST on port 8080",
			data:      []byte("POST /api HTTP/1.1\r\n"),
			dstPort:   8080,
			wantMatch: true,
		},
		{
			name:      "Response on port 80",
			data:      []byte("HTTP/1.1 200 OK\r\n"),
			dstPort:   80,
			wantMatch: true,
		},
		{
			name:      "Invalid data",
			data:      []byte("INVALID DATA"),
			dstPort:   80,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, score := parser.Detect(tt.data, tt.dstPort)
			if matched != tt.wantMatch {
				t.Errorf("Detect() matched = %v, want %v, score = %f", matched, tt.wantMatch, score)
			}
		})
	}
}

// ============================================================================
// HTTP/2 解析测试
// ============================================================================

func TestHTTP2Parser(t *testing.T) {
	parser := parsers.NewHTTP2Parser()

	// 构建 HTTP/2 连接序言
	magic := []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

	t.Run("Magic detection", func(t *testing.T) {
		matched, score := parser.Detect(magic, 443)
		if !matched {
			t.Error("Expected magic to match")
		}
		if score < 0.99 {
			t.Errorf("Expected score ~1.0, got %f", score)
		}
	})

	// 构建 HTTP/2 HEADERS frame
	headersFrame := buildHTTP2HeadersFrame()

	t.Run("Headers frame", func(t *testing.T) {
		input := &ParserInput{
			Packet: RawPacket{Data: headersFrame},
		}
		result, _, err := parser.Parse(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		if result.ParserType != ParserTypeHTTP2 {
			t.Errorf("ParserType = %v, want %v", result.ParserType, ParserTypeHTTP2)
		}
	})
}

func buildHTTP2HeadersFrame() []byte {
	// 简化的 HEADERS frame
	// 实际应该使用 HPACK 编码的 headers
	length := uint32(10)
	frameType := uint8(0x1) // HEADERS
	flags := uint8(0x4)     // END_HEADERS
	streamID := uint32(1)

	frame := make([]byte, 9+length)
	frame[0] = byte(length >> 16)
	frame[1] = byte(length >> 8)
	frame[2] = byte(length)
	frame[3] = frameType
	frame[4] = flags
	frame[5] = byte(streamID >> 24)
	frame[6] = byte(streamID >> 16)
	frame[7] = byte(streamID >> 8)
	frame[8] = byte(streamID)

	return frame
}

// ============================================================================
// gRPC 解析测试
// ============================================================================

func TestGRPCParser(t *testing.T) {
	parser := parsers.NewGRPCParser()

	t.Run("Service method extraction", func(t *testing.T) {
		service, method := parsers.ExtractServiceMethod("/package.service/method")
		if service != "package.service" {
			t.Errorf("Service = %v, want package.service", service)
		}
		if method != "method" {
			t.Errorf("Method = %v, want method", method)
		}
	})

	t.Run("Timeout parsing", func(t *testing.T) {
		value, unit, ok := parsers.ParseGRPCTimeout("10S")
		if !ok {
			t.Error("Expected timeout to parse successfully")
		}
		if value != 10 {
			t.Errorf("Value = %v, want 10", value)
		}
		if unit != 'S' {
			t.Errorf("Unit = %c, want S", unit)
		}
	})
}

// ============================================================================
// Redis 解析测试
// ============================================================================

func TestRedisParser(t *testing.T) {
	parser := parsers.NewRedisParser()

	tests := []struct {
		name      string
		data      []byte
		wantCmd   string
		wantMatch bool
	}{
		{
			name:      "GET command",
			data:      []byte("*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n"),
			wantCmd:   "GET",
			wantMatch: true,
		},
		{
			name:      "SET command",
			data:      []byte("*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$5\r\nvalue\r\n"),
			wantCmd:   "SET",
			wantMatch: true,
		},
		{
			name:      "Inline command",
			data:      []byte("GET key1\r\n"),
			wantCmd:   "GET",
			wantMatch: true,
		},
		{
			name:      "Simple string response",
			data:      []byte("+OK\r\n"),
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 测试检测
			matched, _ := parser.Detect(tt.data, 6379)
			if matched != tt.wantMatch {
				t.Errorf("Detect() matched = %v, want %v", matched, tt.wantMatch)
			}

			if tt.wantMatch {
				// 测试解析
				input := &ParserInput{
					Packet: RawPacket{Data: tt.data},
				}
				result, _, err := parser.Parse(context.Background(), input, nil)
				if err != nil {
					t.Fatalf("Parse failed: %v", err)
				}
				if result != nil && tt.wantCmd != "" {
					if result.Headers["command"] != tt.wantCmd {
						t.Errorf("Command = %v, want %v", result.Headers["command"], tt.wantCmd)
					}
				}
			}
		})
	}
}

func TestRedisCommandTypes(t *testing.T) {
	tests := []struct {
		cmd      string
		wantType string
	}{
		{"GET", "read"},
		{"SET", "write"},
		{"MGET", "read"},
		{"DEL", "write"},
		{"PING", "read"},
		{"INFO", "read"},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			cmdType := parsers.GetCommandType(tt.cmd)
			if cmdType != tt.wantType {
				t.Errorf("CommandType = %v, want %v", cmdType, tt.wantType)
			}
		})
	}
}

// ============================================================================
// Kafka 解析测试
// ============================================================================

func TestKafkaParser(t *testing.T) {
	parser := parsers.NewKafkaParser()

	// 构建 Kafka Metadata 请求
	metadataRequest := buildKafkaRequest(3, 0, 1) // Metadata API, version 0

	t.Run("Metadata request", func(t *testing.T) {
		matched, score := parser.Detect(metadataRequest, 9092)
		if !matched {
			t.Error("Expected to detect Kafka")
		}
		if score < 0.9 {
			t.Errorf("Expected high score, got %f", score)
		}

		input := &ParserInput{
			Packet: RawPacket{Data: metadataRequest},
		}
		result, _, err := parser.Parse(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		if result.Headers["api_key"] != "Metadata" {
			t.Errorf("API Key = %v, want Metadata", result.Headers["api_key"])
		}
	})
}

func buildKafkaRequest(apiKey, apiVersion, correlationID int16) []byte {
	// Kafka 请求格式: length(4) + api_key(2) + api_version(2) + correlation_id(4)
	length := 8
	request := make([]byte, 4+length)

	// Length
	request[0] = byte(length >> 24)
	request[1] = byte(length >> 16)
	request[2] = byte(length >> 8)
	request[3] = byte(length)

	// API Key
	request[4] = byte(apiKey >> 8)
	request[5] = byte(apiKey)

	// API Version
	request[6] = byte(apiVersion >> 8)
	request[7] = byte(apiVersion)

	// Correlation ID
	request[8] = byte(correlationID >> 24)
	request[9] = byte(correlationID >> 16)
	request[10] = byte(correlationID >> 8)
	request[11] = byte(correlationID)

	return request
}

// ============================================================================
// DNS 解析测试
// ============================================================================

func TestDNSParser(t *testing.T) {
	parser := parsers.NewDNSParser()

	// 构建 DNS 查询
	dnsQuery := buildDNSQuery()

	t.Run("DNS query", func(t *testing.T) {
		matched, score := parser.Detect(dnsQuery, 53)
		if !matched {
			t.Error("Expected to detect DNS")
		}
		if score < 0.9 {
			t.Errorf("Expected high score, got %f", score)
		}

		input := &ParserInput{
			Packet: RawPacket{Data: dnsQuery},
		}
		result, _, err := parser.Parse(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		if result.Direction != DirRequest {
			t.Errorf("Direction = %v, want DirRequest", result.Direction)
		}
	})
}

func buildDNSQuery() []byte {
	// 简化的 DNS 查询
	// DNS Header: 12 bytes
	query := make([]byte, 12+5+4) // header + question + type/class

	// Transaction ID
	query[0] = 0x12
	query[1] = 0x34

	// Flags: Standard query
	query[2] = 0x01
	query[3] = 0x00

	// QDCOUNT: 1 question
	query[4] = 0x00
	query[5] = 0x01

	// ANCOUNT: 0
	query[6] = 0x00
	query[7] = 0x00

	// NSCOUNT: 0
	query[8] = 0x00
	query[9] = 0x00

	// ARCOUNT: 0
	query[10] = 0x00
	query[11] = 0x00

	// Question: example.com
	query[12] = 0x07 // length of "example"
	copy(query[13:20], []byte("example"))
	query[20] = 0x03 // length of "com"
	copy(query[21:24], []byte("com"))
	query[24] = 0x00 // end of name

	// QTYPE: A (1)
	query[25] = 0x00
	query[26] = 0x01

	// QCLASS: IN (1)
	query[27] = 0x00
	query[28] = 0x01

	return query
}

// ============================================================================
// 协议检测测试
// ============================================================================

func TestProtocolDetection(t *testing.T) {
	detector := NewProtocolDetector()

	tests := []struct {
		name      string
		data      []byte
		dstPort   uint16
		wantType  ParserType
		minScore  float64
	}{
		{
			name:      "HTTP/1.1 GET",
			data:      []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			dstPort:   80,
			wantType:  ParserTypeHTTP1,
			minScore:  0.9,
		},
		{
			name:      "HTTP/2 Magic",
			data:      []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"),
			dstPort:   443,
			wantType:  ParserTypeHTTP2,
			minScore:  0.99,
		},
		{
			name:      "Redis GET",
			data:      []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"),
			dstPort:   6379,
			wantType:  ParserTypeRedis,
			minScore:  0.9,
		},
		{
			name:      "MySQL Query",
			data:      []byte{0x14, 0x00, 0x00, 0x00, 0x03, 0x53, 0x45, 0x4c, 0x45, 0x43, 0x54, 0x20, 0x31},
			dstPort:   3306,
			wantType:  ParserTypeMySQL,
			minScore:  0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parserType, score := detector.Detect(tt.data, tt.dstPort)
			if parserType != tt.wantType {
				t.Errorf("Detect() type = %v, want %v", parserType, tt.wantType)
			}
			if score < tt.minScore {
				t.Errorf("Detect() score = %f, want >= %f", score, tt.minScore)
			}
		})
	}
}

// ============================================================================
// Engine 测试
// ============================================================================

func TestEngine(t *testing.T) {
	// 初始化解析器
	InitWithParsers([]ParserType{
		ParserTypeHTTP1,
		ParserTypeHTTP2,
		ParserTypeGRPC,
		ParserTypeRedis,
		ParserTypeMySQL,
		ParserTypeKafka,
		ParserTypeDNS,
	})

	config := DefaultConfig()
	config.WorkerNum = 2
	config.QueueSize = 100

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	if err := engine.Start(); err != nil {
		t.Fatalf("Failed to start engine: %v", err)
	}
	defer engine.Stop()

	t.Run("Submit task", func(t *testing.T) {
		packet := RawPacket{
			Metadata: PacketMetadata{
				DstPort: 80,
				FlowID:  1,
			},
			Data: []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		}

		done := make(chan bool)
		callback := func(result *ParseResult, err error) {
			if err != nil {
				t.Errorf("Parse error: %v", err)
			}
			if result != nil {
				t.Logf("Parsed: type=%v, path=%s", result.ParserType, result.Path)
			}
			done <- true
		}

		if err := engine.SubmitWithCallback(packet, nil, callback); err != nil {
			t.Fatalf("Submit failed: %v", err)
		}

		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Error("Timeout waiting for parse result")
		}
	})
}

// ============================================================================
// 性能测试
// ============================================================================

func BenchmarkHTTP1Parser(b *testing.B) {
	parser := parsers.NewHTTPParser()
	data := []byte("GET /api/users/123 HTTP/1.1\r\nHost: example.com\r\nUser-Agent: Test\r\nAccept: application/json\r\n\r\n")
	input := &ParserInput{
		Packet: RawPacket{Data: data},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(context.Background(), input, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRedisParser(b *testing.B) {
	parser := parsers.NewRedisParser()
	data := []byte("*3\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$6\r\nvalue1\r\n")
	input := &ParserInput{
		Packet: RawPacket{Data: data},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, err := parser.Parse(context.Background(), input, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtocolDetection(b *testing.B) {
	detector := NewProtocolDetector()
	testData := [][]byte{
		[]byte("GET / HTTP/1.1\r\n"),
		[]byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"),
		[]byte{0x14, 0x00, 0x00, 0x00, 0x03},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data := testData[i%len(testData)]
		detector.Detect(data, 80)
	}
}

func BenchmarkEngineSubmit(b *testing.B) {
	InitWithParsers([]ParserType{ParserTypeHTTP1})

	config := DefaultConfig()
	config.WorkerNum = 4
	config.QueueSize = 10000
	config.EnableBackpressure = false

	engine, _ := NewEngine(config)
	engine.Start()
	defer engine.Stop()

	packet := RawPacket{
		Metadata: PacketMetadata{
			DstPort: 80,
			FlowID:  1,
		},
		Data: []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.Submit(packet, nil)
	}
}
