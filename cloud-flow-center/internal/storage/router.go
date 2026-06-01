// Package storage 多存储后端路由层
//
// 设计目标:
//   - Flow → ClickHouse (海量数据，高性能查询)
//   - Metrics → VictoriaMetrics (时序数据，高效压缩)
//   - Logs → Loki (日志数据，全文检索)
//   - Metadata → TiDB (元数据，事务支持)
//
// 架构:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                     Storage Router                           │
//	│  ┌──────────────────────────────────────────────────────┐   │
//	│  │                    Route Decision                     │   │
//	│  │   - DataType: flow/metrics/logs/metadata             │   │
//	│  │   - TenantID: 多租户隔离                              │   │
//	│  │   - QueryPattern: 时间范围/聚合/拓扑                  │   │
//	│  └──────────────────────────────────────────────────────┘   │
//	│                                                             │
//	│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌────────┐│
//	│  │ ClickHouse  │ │ Victoria    │ │    Loki     │ │  TiDB  ││
//	│  │  (Flow)     │ │ Metrics     │ │   (Logs)    │ │(Meta)  ││
//	│  └─────────────┘ └─────────────┘ └─────────────┘ └────────┘│
//	└─────────────────────────────────────────────────────────────┘
package storage

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	ErrUnknownDataType   = errors.New("unknown data type")
	ErrStorageNotReady   = errors.New("storage backend not ready")
	ErrRouteNotFound     = errors.New("route not found")
	ErrWriteFailed       = errors.New("write failed")
	ErrQueryFailed       = errors.New("query failed")
	ErrTenantNotFound    = errors.New("tenant not found")
)

// ============================================================================
// 数据类型定义
// ============================================================================

// DataType 数据类型
type DataType uint8

const (
	DataTypeUnknown DataType = iota
	DataTypeFlow             // 网络流量数据 → ClickHouse
	DataTypeMetrics          // 指标数据 → VictoriaMetrics
	DataTypeLogs             // 日志数据 → Loki
	DataTypeMetadata         // 元数据 → TiDB
	DataTypeTrace            // 链路追踪 → ClickHouse
	DataTypeEvent            // 事件数据 → ClickHouse
)

func (t DataType) String() string {
	switch t {
	case DataTypeFlow:
		return "flow"
	case DataTypeMetrics:
		return "metrics"
	case DataTypeLogs:
		return "logs"
	case DataTypeMetadata:
		return "metadata"
	case DataTypeTrace:
		return "trace"
	case DataTypeEvent:
		return "event"
	default:
		return "unknown"
	}
}

// ============================================================================
// 存储后端接口
// ============================================================================

// StorageBackend 存储后端接口
type StorageBackend interface {
	// Name 返回后端名称
	Name() string

	// Type 返回后端类型
	Type() DataType

	// Ready 检查是否就绪
	Ready() bool

	// Write 写入数据
	Write(ctx context.Context, data interface{}) error

	// WriteBatch 批量写入
	WriteBatch(ctx context.Context, batch []interface{}) error

	// Query 查询数据
	Query(ctx context.Context, query *QueryRequest) (*QueryResult, error)

	// Close 关闭连接
	Close() error
}

// QueryRequest 查询请求
type QueryRequest struct {
	// 基础过滤
	DataType   DataType
	TenantID   string
	StartTime  time.Time
	EndTime    time.Time

	// Flow 查询条件
	SrcIP      string
	DstIP      string
	SrcPort    uint16
	DstPort    uint16
	Protocol   string
	Namespace  string
	Service    string
	Pod        string

	// 查询选项
	Limit      int
	Offset     int
	OrderBy    string
	OrderDesc  bool

	// 聚合查询
	Aggregation *AggregationConfig

	// 拓扑查询
	Topology    *TopologyConfig
}

// AggregationConfig 聚合配置
type AggregationConfig struct {
	Type       AggregationType
	GroupBy    []string
	TimeBucket time.Duration // 时间桶大小
	Field      string        // 聚合字段
}

// AggregationType 聚合类型
type AggregationType uint8

const (
	AggCount AggregationType = iota
	AggSum
	AggAvg
	AggMin
	AggMax
	AggP95
	AggP99
)

// TopologyConfig 拓扑查询配置
type TopologyConfig struct {
	// 拓扑类型
	Type       TopologyType
	// 分组维度
	GroupBy    []string // e.g., ["namespace", "service", "pod"]
	// 过滤条件
	Filters    map[string]string
	// 深度限制
	MaxDepth   int
}

// TopologyType 拓扑类型
type TopologyType uint8

const (
	TopologyService TopologyType = iota   // 服务拓扑
	TopologyPod                            // Pod 拓扑
	TopologyIP                             // IP 拓扑
	TopologyProcess                        // 进程拓扑
)

// QueryResult 查询结果
type QueryResult struct {
	// 原始数据
	Records []map[string]interface{}

	// 聚合结果
	Aggregations map[string]interface{}

	// 拓扑结果
	Topology *TopologyResult

	// 元信息
	Total       int64
	TookMs      int64
	FromCache   bool
}

// TopologyResult 拓扑查询结果
type TopologyResult struct {
	// 节点列表
	Nodes []*TopologyNode

	// 边列表 (连接关系)
	Edges []*TopologyEdge

	// 统计信息
	Stats TopologyStats
}

// TopologyNode 拓扑节点
type TopologyNode struct {
	ID         string
	Name       string
	Type       string // service/pod/ip/process
	Namespace  string
	Metadata   map[string]string

	// 统计
	BytesIn    uint64
	BytesOut   uint64
	ConnCount  uint64
	ErrorCount uint64
}

// TopologyEdge 拓扑边 (连接关系)
type TopologyEdge struct {
	Source     string
	Target     string
	Protocol   string
	Port       uint16

	// 统计
	Bytes      uint64
	Packets    uint64
	LatencyNs  uint64
	ErrorCount uint64
}

// TopologyStats 拓扑统计
type TopologyStats struct {
	NodeCount   int
	EdgeCount   int
	TotalBytes  uint64
	TotalErrors uint64
}

// ============================================================================
// 存储路由器
// ============================================================================

// Router 存储路由器
type Router struct {
	// 后端映射 (DataType -> StorageBackend)
	backends map[DataType]StorageBackend

	// 租户配置 (TenantID -> TenantConfig)
	tenants map[string]*TenantConfig

	// 读写锁
	mu sync.RWMutex

	// 配置
	config *RouterConfig
}

// TenantConfig 租户配置
type TenantConfig struct {
	TenantID       string
	EnabledTypes   []DataType // 启用的数据类型
	RetentionDays  map[DataType]int // 各类型数据保留天数
	QuotaBytes     map[DataType]int64 // 各类型存储配额
}

// RouterConfig 路由器配置
type RouterConfig struct {
	// 默认保留天数
	DefaultRetentionDays int

	// 默认存储配额 (0 = 无限制)
	DefaultQuotaBytes int64

	// 写入超时
	WriteTimeout time.Duration

	// 查询超时
	QueryTimeout time.Duration

	// 重试配置
	MaxRetries    int
	RetryInterval time.Duration
}

// DefaultRouterConfig 返回默认配置
func DefaultRouterConfig() *RouterConfig {
	return &RouterConfig{
		DefaultRetentionDays: 30,
		DefaultQuotaBytes:    0,
		WriteTimeout:         10 * time.Second,
		QueryTimeout:         30 * time.Second,
		MaxRetries:           3,
		RetryInterval:        100 * time.Millisecond,
	}
}

// NewRouter 创建存储路由器
func NewRouter(config *RouterConfig) *Router {
	if config == nil {
		config = DefaultRouterConfig()
	}

	return &Router{
		backends: make(map[DataType]StorageBackend),
		tenants:  make(map[string]*TenantConfig),
		config:   config,
	}
}

// Register 注册存储后端
func (r *Router) Register(dataType DataType, backend StorageBackend) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.backends[dataType]; exists {
		return errors.New("backend already registered for type: " + dataType.String())
	}

	r.backends[dataType] = backend
	return nil
}

// Unregister 注销存储后端
func (r *Router) Unregister(dataType DataType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.backends, dataType)
}

// GetBackend 获取存储后端
func (r *Router) GetBackend(dataType DataType) (StorageBackend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	backend, exists := r.backends[dataType]
	return backend, exists
}

// Route 路由写入请求
func (r *Router) Route(ctx context.Context, dataType DataType, data interface{}) error {
	backend, exists := r.GetBackend(dataType)
	if !exists {
		return ErrRouteNotFound
	}

	if !backend.Ready() {
		return ErrStorageNotReady
	}

	// 设置超时
	if r.config.WriteTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.WriteTimeout)
		defer cancel()
	}

	// 重试逻辑
	var lastErr error
	for i := 0; i < r.config.MaxRetries; i++ {
		if err := backend.Write(ctx, data); err != nil {
			lastErr = err
			time.Sleep(r.config.RetryInterval)
			continue
		}
		return nil
	}

	return lastErr
}

// RouteBatch 路由批量写入请求
func (r *Router) RouteBatch(ctx context.Context, dataType DataType, batch []interface{}) error {
	backend, exists := r.GetBackend(dataType)
	if !exists {
		return ErrRouteNotFound
	}

	if !backend.Ready() {
		return ErrStorageNotReady
	}

	if r.config.WriteTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.WriteTimeout)
		defer cancel()
	}

	var lastErr error
	for i := 0; i < r.config.MaxRetries; i++ {
		if err := backend.WriteBatch(ctx, batch); err != nil {
			lastErr = err
			time.Sleep(r.config.RetryInterval)
			continue
		}
		return nil
	}

	return lastErr
}

// Query 路由查询请求
func (r *Router) Query(ctx context.Context, req *QueryRequest) (*QueryResult, error) {
	backend, exists := r.GetBackend(req.DataType)
	if !exists {
		return nil, ErrRouteNotFound
	}

	if !backend.Ready() {
		return nil, ErrStorageNotReady
	}

	if r.config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.QueryTimeout)
		defer cancel()
	}

	return backend.Query(ctx, req)
}

// QueryTopology 查询拓扑
func (r *Router) QueryTopology(ctx context.Context, req *QueryRequest) (*TopologyResult, error) {
	if req.Topology == nil {
		return nil, errors.New("topology config required")
	}

	result, err := r.Query(ctx, req)
	if err != nil {
		return nil, err
	}

	return result.Topology, nil
}

// Close 关闭所有后端
func (r *Router) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for _, backend := range r.backends {
		if err := backend.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ============================================================================
// 租户管理
// ============================================================================

// AddTenant 添加租户
func (r *Router) AddTenant(config *TenantConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tenants[config.TenantID] = config
}

// RemoveTenant 移除租户
func (r *Router) RemoveTenant(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tenants, tenantID)
}

// GetTenant 获取租户配置
func (r *Router) GetTenant(tenantID string) (*TenantConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, exists := r.tenants[tenantID]
	return config, exists
}

// ListTenants 列出所有租户
func (r *Router) ListTenants() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tenants := make([]string, 0, len(r.tenants))
	for id := range r.tenants {
		tenants = append(tenants, id)
	}
	return tenants
}
