// Package l7parser 提供 L7 协议解析引擎
//
// 架构概述:
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        L7 Parser Engine                              │
//	│                                                                       │
//	│  ┌─────────────────────────────────────────────────────────────┐   │
//	│  │                    Protocol Detector                         │   │
//	│  │   - Feature-based detection (no port dependency)             │   │
//	│  │   - Multiple heuristic rules                                 │   │
//	│  │   - Confidence scoring                                       │   │
//	│  └─────────────────────────────────────────────────────────────┘   │
//	│                                                                       │
//	│  ┌─────────────────────────────────────────────────────────────┐   │
//	│  │                    Parser Registry                           │   │
//	│  │   - Plugin-based architecture                                │   │
//	│  │   - Priority-based selection                                 │   │
//	│  │   - Lazy initialization                                      │   │
//	│  └─────────────────────────────────────────────────────────────┘   │
//	│                                                                       │
//	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
//	│  │   HTTP/1    │  │   HTTP/2    │  │    gRPC     │  │    MySQL    │ │
//	│  │   Parser    │  │   Parser    │  │   Parser    │  │   Parser    │ │
//	│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘ │
//	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                 │
//	│  │    Redis    │  │    Kafka    │  │     DNS     │                 │
//	│  │   Parser    │  │   Parser    │  │   Parser    │                 │
//	│  └─────────────┘  └─────────────┘  └─────────────┘                 │
//	│                                                                       │
//	│  ┌─────────────────────────────────────────────────────────────┐   │
//	│  │                    Worker Pool                               │   │
//	│  │   - Independent goroutine pool                               │   │
//	│  │   - Per-worker queues (lock-free)                            │   │
//	│  │   - Dynamic scaling                                          │   │
//	│  │   - Backpressure support                                     │   │
//	│  └─────────────────────────────────────────────────────────────┘   │
//	│                                                                       │
//	│  ┌─────────────────────────────────────────────────────────────┐   │
//	│  │                 Stream Manager                               │   │
//	│  │   - Partial packet reassembly                                │   │
//	│  │   - Sliding window for out-of-order packets                  │   │
//	│  │   - Timeout-based cleanup                                    │   │
//	│  │   - Memory limits per stream                                 │   │
//	│  └─────────────────────────────────────────────────────────────┘   │
//	│                                                                       │
//	└─────────────────────────────────────────────────────────────────────┘
//
// 支持的协议:
//   - P0: HTTP/1.x, HTTP/2, gRPC
//   - P1: MySQL, Redis, Kafka, DNS
//
// 性能特性:
//   - Zero-allocation parsing (对象池复用)
//   - Lock-free queues (per-worker)
//   - Streaming parser (no full reassembly)
//   - Backpressure handling
//   - CPU pinning support
//
// 使用示例:
//
//	// 初始化引擎
//	config := l7parser.DefaultConfig()
//	engine, err := l7parser.NewEngine(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	if err := engine.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer engine.Stop()
//
//	// 提交解析任务
//	packet := l7parser.RawPacket{
//	    Metadata: l7parser.PacketMetadata{
//	        DstPort: 80,
//	        FlowID:  12345,
//	    },
//	    Data: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
//	}
//
//	if err := engine.Submit(packet, flow); err != nil {
//	    log.Printf("Submit failed: %v", err)
//	}
//
// 协议检测:
//
//	// 自动检测协议类型
//	parserType, confidence := l7parser.DetectProtocol(data, dstPort)
//	if confidence > 0.8 {
//	    // 使用检测到的协议类型解析
//	    result, err := l7parser.ParseWithType(data, parserType)
//	}
//
// 扩展协议支持:
//
//	// 实现 Parser 接口
//	type MyProtocolParser struct{}
//
//	func (p *MyProtocolParser) Type() l7parser.ParserType {
//	    return l7parser.ParserTypeMyProtocol
//	}
//
//	func (p *MyProtocolParser) Detect(data []byte, dstPort uint16) (bool, float64) {
//	    // 协议特征检测
//	}
//
//	func (p *MyProtocolParser) Parse(ctx context.Context, input *l7parser.ParserInput, state interface{}) (*l7parser.ParseResult, interface{}, error) {
//	    // 解析逻辑
//	}
//
//	// 注册解析器
//	l7parser.Register("myprotocol", func() l7parser.Parser {
//	    return &MyProtocolParser{}
//	})
//
package l7parser
