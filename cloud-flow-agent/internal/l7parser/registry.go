// Package l7parser 解析器注册中心
//
// 设计原则:
//   - 插件化: 每个解析器独立注册，可动态增删
//   - 优先级: 高优先级解析器优先进行协议检测
//   - 无锁读: 使用 atomic.Value 实现无锁读取
//   - 懒加载: 解析器实例按需创建

package l7parser

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

// Registry 解析器注册中心
type Registry struct {
	// 注册表 (name -> factory)
	factories map[string]ParserFactory

	// 优先级排序的解析器列表 (缓存)
	parsers atomic.Value // []Parser

	// 互斥锁 (仅用于写操作)
	mu sync.RWMutex
}

// ParserFactory 解析器工厂函数
type ParserFactory func() Parser

// registryEntry 内部条目
type registryEntry struct {
	name     string
	factory  ParserFactory
	priority int
	parser   Parser // 缓存的实例
}

// globalRegistry 全局注册中心实例
var globalRegistry = &Registry{
	factories: make(map[string]ParserFactory),
}

// GetRegistry 获取全局注册中心
func GetRegistry() *Registry {
	return globalRegistry
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ParserFactory),
	}
}

// Register 注册解析器
func (r *Registry) Register(name string, factory ParserFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("parser %s already registered", name)
	}

	r.factories[name] = factory

	// 更新缓存
	r.refreshCache()

	return nil
}

// Unregister 注销解析器
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.factories, name)

	// 更新缓存
	r.refreshCache()
}

// Get 获取解析器实例
func (r *Registry) Get(name string) (Parser, bool) {
	r.mu.RLock()
	factory, exists := r.factories[name]
	r.mu.RUnlock()

	if !exists {
		return nil, false
	}

	return factory(), true
}

// GetByType 根据类型获取解析器
func (r *Registry) GetByType(parserType ParserType) (Parser, bool) {
	parsers := r.All()
	for _, p := range parsers {
		if p.Type() == parserType {
			return p, true
		}
	}
	return nil, false
}

// All 获取所有解析器 (按优先级排序)
func (r *Registry) All() []Parser {
	// 原子读取缓存
	if cached := r.parsers.Load(); cached != nil {
		return cached.([]Parser)
	}
	return nil
}

// Detect 协议自动检测
// 遍历所有解析器，返回置信度最高的匹配结果
func (r *Registry) Detect(data []byte, dstPort uint16) (ParserType, float64) {
	parsers := r.All()
	if len(parsers) == 0 {
		return ParserTypeUnknown, 0
	}

	var bestType ParserType
	var bestScore float64

	for _, parser := range parsers {
		matched, score := parser.Detect(data, dstPort)
		if matched && score > bestScore {
			bestType = parser.Type()
			bestScore = score
		}
	}

	return bestType, bestScore
}

// refreshCache 刷新解析器缓存
// 必须在持有写锁的情况下调用
func (r *Registry) refreshCache() {
	entries := make([]*registryEntry, 0, len(r.factories))

	for name, factory := range r.factories {
		// 创建临时实例获取优先级
		parser := factory()
		entries = append(entries, &registryEntry{
			name:     name,
			factory:  factory,
			priority: parser.Priority(),
			parser:   parser,
		})
	}

	// 按优先级降序排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	// 提取解析器列表
	parsers := make([]Parser, len(entries))
	for i, e := range entries {
		parsers[i] = e.parser
	}

	// 原子更新缓存
	r.parsers.Store(parsers)
}

// List 列出所有已注册解析器
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// ============================================================================
// 便捷注册函数
// ============================================================================

// Register 注册解析器到全局注册中心
func Register(name string, factory ParserFactory) error {
	return globalRegistry.Register(name, factory)
}

// MustRegister 必须成功注册，否则 panic
func MustRegister(name string, factory ParserFactory) {
	if err := globalRegistry.Register(name, factory); err != nil {
		panic(err)
	}
}

// GetParser 获取解析器
func GetParser(name string) (Parser, bool) {
	return globalRegistry.Get(name)
}

// DetectProtocol 自动检测协议
func DetectProtocol(data []byte, dstPort uint16) (ParserType, float64) {
	return globalRegistry.Detect(data, dstPort)
}

// ============================================================================
// 内置解析器注册
// ============================================================================

// Init 初始化注册中心，注册所有内置解析器
func Init() {
	// 注意: 实际解析器在各自文件中注册
	// 这里只是确保注册中心已初始化
	if globalRegistry.factories == nil {
		globalRegistry.factories = make(map[string]ParserFactory)
	}
}

// InitWithParsers 使用指定解析器初始化
func InitWithParsers(parsers []ParserType) {
	Init()

	// 根据类型注册对应的解析器
	for _, pt := range parsers {
		switch pt {
		case ParserTypeHTTP1:
			RegisterHTTP1Parser()
		case ParserTypeHTTP2:
			RegisterHTTP2Parser()
		case ParserTypeGRPC:
			RegisterGRPCParser()
		case ParserTypeMySQL:
			RegisterMySQLParser()
		case ParserTypeRedis:
			RegisterRedisParser()
		case ParserTypeKafka:
			RegisterKafkaParser()
		case ParserTypeDNS:
			RegisterDNSParser()
		}
	}
}

// 前向声明 (在各自文件中实现)
func RegisterHTTP1Parser()
func RegisterHTTP2Parser()
func RegisterGRPCParser()
func RegisterMySQLParser()
func RegisterRedisParser()
func RegisterKafkaParser()
func RegisterDNSParser()
