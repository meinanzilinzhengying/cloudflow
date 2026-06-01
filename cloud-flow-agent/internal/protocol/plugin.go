// Package protocol 提供插件化协议解析框架
//
// 设计原则：
//   - 插件与主程序通过 gRPC 进程间通信，完全解耦
//   - 插件独立编译为二进制，可独立升级
//   - 统一的 Plugin 接口，所有协议解析器实现同一契约
//   - 支持运行时动态加载/卸载/热更新
//
// 架构：
//
//	┌──────────────────────────────────────────┐
//	│            Agent 主进程                    │
//	│  ┌────────────────────────────────────┐  │
//	│  │        PluginManager               │  │
//	│  │  ┌──────┐ ┌──────┐ ┌──────┐       │  │
//	│  │  │Oracle│ │  PG  │ │Redis │ ...   │  │
//	│  │  └──┬───┘ └──┬───┘ └──┬───┘       │  │
//	│  └─────┼────────┼────────┼──────────┘  │
//	│        │   gRPC (Unix Socket)  │        │
//	└────────┼────────┼────────┼──────────┘
//	         │        │        │
//	┌────────▼──┐ ┌──▼─────┐ ┌▼──────────┐
//	│oracle_plugin│ │pg_plugin│ │redis_plugin│
//	│  (binary)  │ │(binary) │ │  (binary)  │
//	└────────────┘ └────────┘ └────────────┘
package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"

	edge "cloud-flow/proto"
)

// ============================================================================
// 插件接口定义
// ============================================================================

// Plugin 协议解析插件接口
// 所有协议解析插件必须实现此接口
type Plugin interface {
	// Info 返回插件元信息
	Info() PluginInfo

	// Init 初始化插件
	Init(ctx context.Context, cfg PluginConfig) error

	// Parse 解析原始数据包，返回协议解析结果
	// data: 原始网络包数据
	// metadata: 包元数据（五元组、方向等）
	Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error)

	// ParseBatch 批量解析（可选优化）
	ParseBatch(ctx context.Context, packets []*PacketInput) ([]*ParseResult, error)

	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// Shutdown 优雅关闭
	Shutdown(ctx context.Context) error
}

// PacketMetadata 数据包元数据
type PacketMetadata struct {
	SrcIP     string `json:"src_ip"`
	DstIP     string `json:"dst_ip"`
	SrcPort   int32  `json:"src_port"`
	DstPort   int32  `json:"dst_port"`
	Protocol  string `json:"protocol"` // tcp/udp
	Timestamp int64  `json:"timestamp"`
	VNI       uint32 `json:"vni,omitempty"` // VXLAN VNI
	Direction  string `json:"direction"`  // ingress/egress
}

// PacketInput 批量解析输入
type PacketInput struct {
	Data     []byte          `json:"data"`
	Metadata *PacketMetadata `json:"metadata"`
}

// ParseResult 协议解析结果
type ParseResult struct {
	Protocol    string            `json:"protocol"`     // 协议名称 (oracle/postgresql/redis/kafka/dubbo)
	IsMatch     bool              `json:"is_match"`     // 是否匹配该协议
	IsPartial   bool              `json:"is_partial"`   // 是否为部分解析（跨包）
	IsRequest   bool              `json:"is_request"`   // 是否为请求（vs 响应）
	Query       string            `json:"query"`        // 查询语句/命令
	Database    string            `json:"database"`     // 数据库名
	User        string            `json:"user"`         // 用户名
	Status      string            `json:"status"`       // 状态码/错误码
	Duration    float64           `json:"duration_ms"`  // 执行时长(毫秒)
	AffectedRows int64            `json:"affected_rows"` // 影响行数
	Fields      map[string]string `json:"fields"`       // 协议特有字段
	Tags        map[string]string `json:"tags"`         // 附加标签
	Error       string            `json:"error"`        // 错误信息
	Latency     float64           `json:"latency_ms"`   // 端到端延迟
}

// PluginInfo 插件元信息
type PluginInfo struct {
	Name         string   `json:"name"`           // 插件名称
	Version      string   `json:"version"`        // 插件版本
	Protocol     string   `json:"protocol"`       // 解析的协议
	Description  string   `json:"description"`    // 描述
	Author       string   `json:"author"`         // 作者
	MinAgentVer  string   `json:"min_agent_ver"`  // 最低兼容Agent版本
	MaxAgentVer  string   `json:"max_agent_ver"`  // 最高兼容Agent版本
	SupportedOps []string `json:"supported_ops"`  // 支持的操作类型
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status    string `json:"status"`     // healthy/degraded/unhealthy
	Message   string `json:"message"`    // 状态描述
	Uptime    int64  `json:"uptime"`     // 运行时长(秒)
	ParseCount uint64 `json:"parse_count"` // 已解析包数
	ErrorCount uint64 `json:"error_count"` // 错误数
}

// PluginConfig 插件配置
type PluginConfig struct {
	Enabled     bool              `json:"enabled"`
	BinaryPath  string            `json:"binary_path"`  // 插件二进制路径
	SocketPath  string            `json:"socket_path"`  // Unix Socket 路径
	Args        []string          `json:"args"`         // 启动参数
	Env         map[string]string `json:"env"`          // 环境变量
	Timeout     time.Duration     `json:"timeout"`      // 解析超时
	MaxMemoryMB int               `json:"max_memory_mb"` // 内存限制
}

// ============================================================================
// 插件注册中心
// ============================================================================

// Registry 插件注册中心（单例）
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]PluginFactory // name -> factory
	infos   map[string]PluginInfo    // name -> info
}

// PluginFactory 插件工厂函数
type PluginFactory func() Plugin

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// GetRegistry 获取全局注册中心
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = &Registry{
			plugins: make(map[string]PluginFactory),
			infos:   make(map[string]PluginInfo),
		}
	})
	return globalRegistry
}

// Register 注册插件工厂
func (r *Registry) Register(name string, factory PluginFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("插件 %s 已注册", name)
	}

	// 创建临时实例获取插件信息
	p := factory()
	info := p.Info()

	r.plugins[name] = factory
	r.infos[name] = info

	return nil
}

// Unregister 注销插件
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.plugins, name)
	delete(r.infos, name)
}

// Create 创建插件实例
func (r *Registry) Create(name string) (Plugin, error) {
	r.mu.RLock()
	factory, exists := r.plugins[name]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("插件 %s 未注册", name)
	}

	return factory(), nil
}

// List 列出所有已注册插件
func (r *Registry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(r.infos))
	for _, info := range r.infos {
		infos = append(infos, info)
	}
	return infos
}

// GetInfo 获取插件信息
func (r *Registry) GetInfo(name string) (PluginInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, exists := r.infos[name]
	return info, exists
}

// ============================================================================
// gRPC 插件协议定义
// ============================================================================

// GRPCPlugin gRPC 插件接口
// 用于跨进程通信的插件实现此接口
type GRPCPlugin interface {
	// Server 注册 gRPC 服务端
	Server(*PluginServer) error

	// Client 创建 gRPC 客户端
	Client(context.Context, *PluginClient) (Plugin, error)
}

// PluginServer gRPC 服务端接口（由主进程实现）
type PluginServer interface {
	// SendMetrics 发送解析后的指标数据
	SendMetrics(ctx context.Context, metrics []*edge.MetricData) error

	// GetConfig 获取插件配置
	GetConfig(ctx context.Context) (*PluginConfig, error)

	// ReportHealth 上报健康状态
	ReportHealth(ctx context.Context, status *HealthStatus) error

	// Log 日志记录
	Log(ctx context.Context, level, message string) error
}

// PluginClient gRPC 客户端接口（由插件实现）
type PluginClient interface {
	// Dispense 返回插件实例
	Dispense(name string) (interface{}, error)
}

// ============================================================================
// 内置协议解析器基类
// ============================================================================

// BaseParser 协议解析器基类
// 提供通用的解析框架，具体协议只需实现 ParseData 方法
type BaseParser struct {
	info     PluginInfo
	config   PluginConfig
	parseCnt uint64
	errCnt   uint64
	startAt  time.Time
}

// NewBaseParser 创建基类实例
func NewBaseParser(info PluginInfo) *BaseParser {
	return &BaseParser{
		info:    info,
		startAt: time.Now(),
	}
}

// Info 返回插件信息
func (p *BaseParser) Info() PluginInfo {
	return p.info
}

// Init 初始化
func (p *BaseParser) Init(ctx context.Context, cfg PluginConfig) error {
	p.config = cfg
	return nil
}

// HealthCheck 健康检查
func (p *BaseParser) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{
		Status:     "healthy",
		Message:    "running",
		Uptime:     int64(time.Since(p.startAt).Seconds()),
		ParseCount: p.parseCnt,
		ErrorCount: p.errCnt,
	}, nil
}

// Shutdown 关闭
func (p *BaseParser) Shutdown(ctx context.Context) error {
	return nil
}

// ParseBatch 批量解析（默认逐条调用 Parse）
func (p *BaseParser) ParseBatch(ctx context.Context, packets []*PacketInput) ([]*ParseResult, error) {
	results := make([]*ParseResult, 0, len(packets))
	for _, pkt := range packets {
		result, err := p.Parse(ctx, pkt.Data, pkt.Metadata)
		if err != nil {
			p.errCnt++
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// recordParse 记录解析统计
func (p *BaseParser) recordParse(err error) {
	if err == nil {
		p.parseCnt++
	} else {
		p.errCnt++
	}
}

// ============================================================================
// 协议特征匹配器
// ============================================================================

// ProtocolMatcher 协议特征匹配器
// 用于快速判断数据包属于哪种协议
type ProtocolMatcher struct {
	matchers map[string]MatchFunc
}

// MatchFunc 协议匹配函数
type MatchFunc func(data []byte, dstPort int32) (bool, float64)

// NewProtocolMatcher 创建匹配器
func NewProtocolMatcher() *ProtocolMatcher {
	return &ProtocolMatcher{
		matchers: make(map[string]MatchFunc),
	}
}

// Register 注册匹配函数
func (m *ProtocolMatcher) Register(protocol string, fn MatchFunc) {
	m.matchers[protocol] = fn
}

// Match 匹配协议
// 返回: 协议名称, 置信度(0-1)
func (m *ProtocolMatcher) Match(data []byte, dstPort int32) (string, float64) {
	var bestProtocol string
	var bestScore float64

	for protocol, fn := range m.matchers {
		matched, score := fn(data, dstPort)
		if matched && score > bestScore {
			bestProtocol = protocol
			bestScore = score
		}
	}

	return bestProtocol, bestScore
}

// RegisterBuiltinMatchers 注册内置协议匹配规则
func RegisterBuiltinMatchers(m *ProtocolMatcher) {
	// Oracle: 端口1521, 特征字节
	m.Register("oracle", func(data []byte, dstPort int32) (bool, float64) {
		if dstPort != 1521 && dstPort != 1522 {
			return false, 0
		}
		if len(data) < 2 {
			return false, 0
		}
		// Oracle TNS 头: (CONNECT_DATA=...)
		if data[0] == 0x00 && data[1] == len(data)-2 {
			return true, 0.9
		}
		return false, 0
	})

	// PostgreSQL: 端口5432, Startup 消息
	m.Register("postgresql", func(data []byte, dstPort int32) (bool, float64) {
		if dstPort != 5432 {
			return false, 0
		}
		if len(data) < 8 {
			return false, 0
		}
		// PostgreSQL Startup 消息: length(4) + protocol(4) = 196608(0x00030000)
		if data[4] == 0x00 && data[5] == 0x03 && data[6] == 0x00 && data[7] == 0x00 {
			return true, 0.95
		}
		// SSL Request
		if data[4] == 0x04 && data[5] == 0xd2 && data[6] == 0x16 && data[7] == 0x2f {
			return true, 0.9
		}
		return false, 0
	})

	// Redis: 端口6379, RESP 协议
	m.Register("redis", func(data []byte, dstPort int32) (bool, float64) {
		if dstPort != 6379 {
			return false, 0
		}
		if len(data) < 1 {
			return false, 0
		}
		// RESP 命令: +OK, -ERR, *数字, $数字, :数字
		first := data[0]
		if first == '+' || first == '-' || first == ':' || first == '*' || first == '$' {
			return true, 0.85
		}
		return false, 0
	})

	// Kafka: 端口9092, 协议版本
	m.Register("kafka", func(data []byte, dstPort int32) (bool, float64) {
		if dstPort != 9092 && dstPort != 9093 {
			return false, 0
		}
		if len(data) < 4 {
			return false, 0
		}
		// Kafka 请求: apiKey(2) + apiVersion(2)
		// 常见 apiKey: 0=Produce, 1=Fetch, 3=Metadata, 18=ApiVersions
		apiKey := int(data[0])<<8 | int(data[1])
		if apiKey >= 0 && apiKey <= 50 {
			return true, 0.8
		}
		return false, 0
	})

	// Dubbo: 端口20880, Dubbo Magic Header
	m.Register("dubbo", func(data []byte, dstPort int32) (bool, float64) {
		if dstPort != 20880 {
			return false, 0
		}
		if len(data) < 16 {
			return false, 0
		}
		// Dubbo Header: magic(2) + serialization(1) + ...
		// Magic: 0xdabb
		if data[0] == 0xda && data[1] == 0xbb {
			return true, 0.95
		}
		return false, 0
	})
}
