// Package builder 拓扑图构建器
//
// 将 UnifiedFlow 转换为不同粒度的拓扑图:
//
//	UnifiedFlow ──→ ServiceGraphBuilder   ──→ Service Graph (service → service)
//	            ──→ ProcessGraphBuilder   ──→ Process Graph (process → process)
//	            ──→ PodGraphBuilder       ──→ Pod Graph (pod → pod)
//	            ──→ NamespaceGraphBuilder ──→ Namespace Graph (ns → ns)
//
// 构建策略:
//   - Service:  以 K8s Service 名称分组，无 Service 时 fallback 到 Deployment
//   - Process:  以 hostname:processName:pid 分组
//   - Pod:      以 K8s Pod 名称分组，无 Pod 时 fallback 到 IP
//   - Namespace: 以 K8s Namespace 分组，无 Namespace 时 fallback 到 "default"
package builder

import (
	"fmt"
	"strconv"

	flow "cloud-flow/pkg/flow"
	graph "cloud-flow/services/topology-engine/graph"
)

// ============================================================================
// BuilderConfig 构建器配置
// ============================================================================

// BuilderConfig 图构建器通用配置
type BuilderConfig struct {
	// MaxNodes 图中最大节点数
	MaxNodes int
	// MaxEdges 图中最大边数
	MaxEdges int
	// PruneThreshold 边权重剪枝阈值，低于此值的边将被移除
	PruneThreshold float64
}

// DefaultConfig 返回默认构建器配置
func DefaultConfig() BuilderConfig {
	return BuilderConfig{
		MaxNodes:       50000,
		MaxEdges:       1000000,
		PruneThreshold: 0.01,
	}
}

// applyDefaults 对零值字段填充默认值
func (c BuilderConfig) applyDefaults() BuilderConfig {
	if c.MaxNodes <= 0 {
		c.MaxNodes = 50000
	}
	if c.MaxEdges <= 0 {
		c.MaxEdges = 1000000
	}
	if c.PruneThreshold <= 0 {
		c.PruneThreshold = 0.01
	}
	return c
}

// ============================================================================
// GraphBuilder 接口
// ============================================================================

// GraphBuilder 拓扑图构建器接口
type GraphBuilder interface {
	// Build 从流量数据构建拓扑图
	Build(flows []*flow.UnifiedFlow) *graph.Graph
	// GraphType 返回构建器产生的图类型标识
	GraphType() string
}

// ============================================================================
// ServiceGraphBuilder 服务拓扑图构建器
// ============================================================================

// ServiceGraphBuilder 以 K8s Service 维度构建拓扑图
//
// 节点标识策略:
//   - 优先使用 K8s Service: "namespace/service-name"
//   - 无 Service 时 fallback 到 Deployment: "namespace/deployment-name"
//   - 均无时 fallback 到源/目标 IP
type ServiceGraphBuilder struct {
	config BuilderConfig
}

// NewServiceGraphBuilder 创建服务拓扑图构建器
func NewServiceGraphBuilder(config BuilderConfig) *ServiceGraphBuilder {
	return &ServiceGraphBuilder{
		config: config.applyDefaults(),
	}
}

// GraphType 返回 "service"
func (b *ServiceGraphBuilder) GraphType() string {
	return graph.GraphTypeService
}

// Build 从流量数据构建服务拓扑图
func (b *ServiceGraphBuilder) Build(flows []*flow.UnifiedFlow) *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeService, "", b.config.MaxNodes, b.config.MaxEdges)

	for _, f := range flows {
		ns := resolveNamespace(f)
		protocol := resolveProtocol(f)
		port := f.DstPort
		errorCount := resolveErrorCount(f)

		srcID := resolveServiceNodeID(f, true)
		dstID := resolveServiceNodeID(f, false)

		g.AddOrUpdateNode(srcID, srcID, graph.GraphTypeService, ns, nil)
		g.AddOrUpdateNode(dstID, dstID, graph.GraphTypeService, ns, nil)
		g.AccumulateEdge(srcID, dstID, f.Bytes, f.Packets, f.LatencyNs, errorCount)
	}

	g.RecomputeWeights()
	g.PruneEdges(b.config.PruneThreshold)

	return g
}

// resolveServiceNodeID 根据流量解析服务节点 ID
//
//	优先级: Service > Deployment > IP
//
// isSource 为 true 时使用源端字段，false 时使用目的端字段
func resolveServiceNodeID(f *flow.UnifiedFlow, isSource bool) string {
	ns := resolveNamespace(f)

	if isSource {
		if !f.Service.IsZero() {
			return ns + "/" + f.Service.String()
		}
		if !f.Deployment.IsZero() {
			return ns + "/" + f.Deployment.String()
		}
		return f.SrcIP.String()
	}

	if !f.Service.IsZero() {
		return ns + "/" + f.Service.String()
	}
	if !f.Deployment.IsZero() {
		return ns + "/" + f.Deployment.String()
	}
	return f.DstIP.String()
}

// ============================================================================
// ProcessGraphBuilder 进程拓扑图构建器
// ============================================================================

// ProcessGraphBuilder 以进程维度构建拓扑图
//
// 节点标识策略:
//   - 格式: "hostname:processName:pid"
//   - ProcessName 为空时 fallback 到 Comm
//   - PID 为 0 时使用字符串 "0"
//   - Hostname 为空时 fallback 到源/目标 IP
type ProcessGraphBuilder struct {
	config BuilderConfig
}

// NewProcessGraphBuilder 创建进程拓扑图构建器
func NewProcessGraphBuilder(config BuilderConfig) *ProcessGraphBuilder {
	return &ProcessGraphBuilder{
		config: config.applyDefaults(),
	}
}

// GraphType 返回 "process"
func (b *ProcessGraphBuilder) GraphType() string {
	return graph.GraphTypeProcess
}

// Build 从流量数据构建进程拓扑图
func (b *ProcessGraphBuilder) Build(flows []*flow.UnifiedFlow) *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeProcess, "", b.config.MaxNodes, b.config.MaxEdges)

	for _, f := range flows {
		ns := resolveNamespace(f)
		protocol := resolveProtocol(f)
		_ = protocol
		port := f.DstPort
		_ = port
		errorCount := resolveErrorCount(f)

		srcID := resolveProcessNodeID(f, true)
		dstID := resolveProcessNodeID(f, false)

		g.AddOrUpdateNode(srcID, srcID, graph.GraphTypeProcess, ns, nil)
		g.AddOrUpdateNode(dstID, dstID, graph.GraphTypeProcess, ns, nil)
		g.AccumulateEdge(srcID, dstID, f.Bytes, f.Packets, f.LatencyNs, errorCount)
	}

	g.RecomputeWeights()
	g.PruneEdges(b.config.PruneThreshold)

	return g
}

// resolveProcessNodeID 根据流量解析进程节点 ID
//
//	格式: "hostname:processName:pid"
//	ProcessName 为空时使用 Comm，PID 为 0 时使用 "0"
//	Hostname 为空时使用 IP
//
// isSource 为 true 时使用源端字段，false 时使用目的端字段
func resolveProcessNodeID(f *flow.UnifiedFlow, isSource bool) string {
	var hostname, processName, pidStr string

	if isSource {
		if !f.Hostname.IsZero() {
			hostname = f.Hostname.String()
		} else {
			hostname = f.SrcIP.String()
		}
		if !f.ProcessName.IsZero() {
			processName = f.ProcessName.String()
		} else if !f.Comm.IsZero() {
			processName = f.Comm.String()
		} else {
			processName = "unknown"
		}
		pidStr = strconv.FormatUint(uint64(f.PID), 10)
	} else {
		if !f.Hostname.IsZero() {
			hostname = f.Hostname.String()
		} else {
			hostname = f.DstIP.String()
		}
		if !f.ProcessName.IsZero() {
			processName = f.ProcessName.String()
		} else if !f.Comm.IsZero() {
			processName = f.Comm.String()
		} else {
			processName = "unknown"
		}
		pidStr = strconv.FormatUint(uint64(f.PID), 10)
	}

	return hostname + ":" + processName + ":" + pidStr
}

// ============================================================================
// PodGraphBuilder Pod 拓扑图构建器
// ============================================================================

// PodGraphBuilder 以 K8s Pod 维度构建拓扑图
//
// 节点标识策略:
//   - 优先使用 K8s Pod: "namespace/pod-name"
//   - 无 Pod 时 fallback 到源/目标 IP
type PodGraphBuilder struct {
	config BuilderConfig
}

// NewPodGraphBuilder 创建 Pod 拓扑图构建器
func NewPodGraphBuilder(config BuilderConfig) *PodGraphBuilder {
	return &PodGraphBuilder{
		config: config.applyDefaults(),
	}
}

// GraphType 返回 "pod"
func (b *PodGraphBuilder) GraphType() string {
	return graph.GraphTypePod
}

// Build 从流量数据构建 Pod 拓扑图
func (b *PodGraphBuilder) Build(flows []*flow.UnifiedFlow) *graph.Graph {
	g := graph.NewGraph(graph.GraphTypePod, "", b.config.MaxNodes, b.config.MaxEdges)

	for _, f := range flows {
		ns := resolveNamespace(f)
		errorCount := resolveErrorCount(f)

		srcID := resolvePodNodeID(f, true)
		dstID := resolvePodNodeID(f, false)

		g.AddOrUpdateNode(srcID, srcID, graph.GraphTypePod, ns, nil)
		g.AddOrUpdateNode(dstID, dstID, graph.GraphTypePod, ns, nil)
		g.AccumulateEdge(srcID, dstID, f.Bytes, f.Packets, f.LatencyNs, errorCount)
	}

	g.RecomputeWeights()
	g.PruneEdges(b.config.PruneThreshold)

	return g
}

// resolvePodNodeID 根据流量解析 Pod 节点 ID
//
//	优先级: Pod > IP
//
// isSource 为 true 时使用源端字段，false 时使用目的端字段
func resolvePodNodeID(f *flow.UnifiedFlow, isSource bool) string {
	ns := resolveNamespace(f)

	if isSource {
		if !f.Pod.IsZero() {
			return ns + "/" + f.Pod.String()
		}
		return f.SrcIP.String()
	}

	if !f.Pod.IsZero() {
		return ns + "/" + f.Pod.String()
	}
	return f.DstIP.String()
}

// ============================================================================
// NamespaceGraphBuilder 命名空间拓扑图构建器
// ============================================================================

// NamespaceGraphBuilder 以 K8s Namespace 维度构建拓扑图
//
// 节点标识策略:
//   - 使用 K8s Namespace，无 Namespace 时 fallback 到 "default"
//   - 产生较粗粒度的命名空间间流量关系图
type NamespaceGraphBuilder struct {
	config BuilderConfig
}

// NewNamespaceGraphBuilder 创建命名空间拓扑图构建器
func NewNamespaceGraphBuilder(config BuilderConfig) *NamespaceGraphBuilder {
	return &NamespaceGraphBuilder{
		config: config.applyDefaults(),
	}
}

// GraphType 返回 "namespace"
func (b *NamespaceGraphBuilder) GraphType() string {
	return graph.GraphTypeNamespace
}

// Build 从流量数据构建命名空间拓扑图
func (b *NamespaceGraphBuilder) Build(flows []*flow.UnifiedFlow) *graph.Graph {
	g := graph.NewGraph(graph.GraphTypeNamespace, "", b.config.MaxNodes, b.config.MaxEdges)

	for _, f := range flows {
		errorCount := resolveErrorCount(f)

		srcID := resolveNamespaceNodeID(f)
		dstID := srcID // 命名空间级别，源和目标使用同一 namespace 字段

		g.AddOrUpdateNode(srcID, srcID, graph.GraphTypeNamespace, srcID, nil)
		g.AddOrUpdateNode(dstID, dstID, graph.GraphTypeNamespace, dstID, nil)
		g.AccumulateEdge(srcID, dstID, f.Bytes, f.Packets, f.LatencyNs, errorCount)
	}

	g.RecomputeWeights()
	g.PruneEdges(b.config.PruneThreshold)

	return g
}

// resolveNamespaceNodeID 根据流量解析命名空间节点 ID
func resolveNamespaceNodeID(f *flow.UnifiedFlow) string {
	if !f.Namespace.IsZero() {
		return f.Namespace.String()
	}
	return "default"
}

// ============================================================================
// BuilderRegistry 构建器注册表
// ============================================================================

// BuilderRegistry 图构建器注册表，支持按名称查找和构建
type BuilderRegistry struct {
	builders map[string]GraphBuilder
}

// NewRegistry 创建空的构建器注册表
func NewRegistry(config BuilderConfig) *BuilderRegistry {
	return &BuilderRegistry{
		builders: make(map[string]GraphBuilder),
	}
}

// Register 注册一个构建器
func (r *BuilderRegistry) Register(name string, builder GraphBuilder) {
	r.builders[name] = builder
}

// Get 获取指定名称的构建器
func (r *BuilderRegistry) Get(name string) (GraphBuilder, bool) {
	b, ok := r.builders[name]
	return b, ok
}

// Build 使用指定名称的构建器从流量数据构建拓扑图
func (r *BuilderRegistry) Build(name string, flows []*flow.UnifiedFlow) (*graph.Graph, error) {
	b, ok := r.builders[name]
	if !ok {
		return nil, fmt.Errorf("builder %q not registered", name)
	}
	return b.Build(flows), nil
}

// DefaultRegistry 创建并预注册所有内置构建器的注册表
//
// 注册的构建器:
//   - "service":    ServiceGraphBuilder
//   - "process":    ProcessGraphBuilder
//   - "pod":        PodGraphBuilder
//   - "namespace":  NamespaceGraphBuilder
func DefaultRegistry(config BuilderConfig) *BuilderRegistry {
	cfg := config.applyDefaults()

	r := NewRegistry(cfg)
	r.Register("service", NewServiceGraphBuilder(cfg))
	r.Register("process", NewProcessGraphBuilder(cfg))
	r.Register("pod", NewPodGraphBuilder(cfg))
	r.Register("namespace", NewNamespaceGraphBuilder(cfg))

	return r
}

// ============================================================================
// 内部辅助函数
// ============================================================================

// resolveNamespace 解析流量的命名空间，为空时返回 "default"
func resolveNamespace(f *flow.UnifiedFlow) string {
	if !f.Namespace.IsZero() {
		return f.Namespace.String()
	}
	return "default"
}

// resolveProtocol 解析流量的协议标识
//
//	优先使用 L7 协议，未设置时 fallback 到 L4 协议
func resolveProtocol(f *flow.UnifiedFlow) string {
	if f.L7Protocol != flow.ProtoUnknown {
		return f.L7Protocol.String()
	}
	return f.Protocol.String()
}

// resolveErrorCount 解析流量的错误计数
//
//	StatusCode >= 400 视为错误，返回 1；否则返回 0
func resolveErrorCount(f *flow.UnifiedFlow) uint64 {
	if f.StatusCode >= 400 {
		return 1
	}
	return 0
}
