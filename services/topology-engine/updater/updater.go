// Package updater 增量拓扑更新引擎
//
// 核心职责:
//   - 接收实时 UnifiedFlow 批次
//   - 增量更新拓扑图 (非全量重建)
//   - 管理拓扑版本
//   - 触发缓存刷新
//   - 触发 heatmap 更新
//
// 更新策略:
//  1. Mark all nodes/edges inactive
//  2. Process incoming flows -> reactivate/add nodes and edges
//  3. Remove inactive nodes/edges (stale)
//  4. Recompute edge weights
//  5. Snapshot -> update cache
//  6. Update heatmap
//  7. Increment version
package updater

import (
	"fmt"
	"sync"
	"time"

	flow "cloud-flow/pkg/flow"
	graph "cloud-flow/services/topology-engine/graph"
	"cloud-flow/services/topology-engine/cache"
	"cloud-flow/services/topology-engine/heatmap"
)

// ---------------------------------------------------------------------------
// 常量
// ---------------------------------------------------------------------------

const (
	// DefaultUpdateInterval 默认更新间隔 (5s)
	DefaultUpdateInterval = 5 * time.Second

	// DefaultStaleThreshold 默认过期阈值 (60s)
	DefaultStaleThreshold = 60 * time.Second

	// DefaultMaxNodesPerTenant 每租户默认最大节点数
	DefaultMaxNodesPerTenant = 50000

	// DefaultMaxEdgesPerTenant 每租户默认最大边数
	DefaultMaxEdgesPerTenant = 1000000

	// FlowBufferFlushThreshold 流量缓冲区刷新阈值
	FlowBufferFlushThreshold = 10000

	// StaleCleanupMultiplier 过期清理周期 = UpdateInterval * 此系数
	StaleCleanupMultiplier = 5

	// MaxVersionHistory 最大版本历史记录数 (用于 Diff 计算)
	MaxVersionHistory = 10

	// supportedGraphTypes 支持的图类型列表
	supportedGraphTypes = []string{
		graph.GraphTypeService,
		graph.GraphTypeProcess,
		graph.GraphTypePod,
		graph.GraphTypeNamespace,
	}
)

// ---------------------------------------------------------------------------
// BuilderConfig (占位，传递给 graph builder 的配置)
// ---------------------------------------------------------------------------

// BuilderConfig 图构建器配置
type BuilderConfig struct {
	// 预留扩展字段
}

// ---------------------------------------------------------------------------
// UpdaterConfig
// ---------------------------------------------------------------------------

// UpdaterConfig 增量更新引擎配置
type UpdaterConfig struct {
	// UpdateInterval mark-inactive-sweep 的执行间隔 (默认 5s)
	UpdateInterval time.Duration

	// StaleThreshold 节点/边超过此时间未被观测到则视为过期并移除 (默认 60s)
	StaleThreshold time.Duration

	// MaxNodesPerTenant 每租户最大节点数 (默认 50000)
	MaxNodesPerTenant int

	// MaxEdgesPerTenant 每租户最大边数 (默认 1000000)
	MaxEdgesPerTenant int

	// BuilderConfig 传递给图构建器的配置
	BuilderConfig BuilderConfig
}

// DefaultUpdaterConfig 返回默认配置
func DefaultUpdaterConfig() UpdaterConfig {
	return UpdaterConfig{
		UpdateInterval:    DefaultUpdateInterval,
		StaleThreshold:    DefaultStaleThreshold,
		MaxNodesPerTenant: DefaultMaxNodesPerTenant,
		MaxEdgesPerTenant: DefaultMaxEdgesPerTenant,
	}
}

// normalize 填充零值字段为默认值
func (c *UpdaterConfig) normalize() {
	if c.UpdateInterval <= 0 {
		c.UpdateInterval = DefaultUpdateInterval
	}
	if c.StaleThreshold <= 0 {
		c.StaleThreshold = DefaultStaleThreshold
	}
	if c.MaxNodesPerTenant <= 0 {
		c.MaxNodesPerTenant = DefaultMaxNodesPerTenant
	}
	if c.MaxEdgesPerTenant <= 0 {
		c.MaxEdgesPerTenant = DefaultMaxEdgesPerTenant
	}
}

// ---------------------------------------------------------------------------
// versionHistoryEntry 版本历史条目
// ---------------------------------------------------------------------------

// versionHistoryEntry 保存某个版本的图快照，用于 Diff 计算
type versionHistoryEntry struct {
	version   uint64
	snapshot  *graph.GraphSnapshot
	timestamp int64
}

// ---------------------------------------------------------------------------
// TopologyUpdater
// ---------------------------------------------------------------------------

// TopologyUpdater 增量拓扑更新引擎
//
// 负责接收实时 UnifiedFlow 批次，增量更新多类型拓扑图，
// 管理版本号，刷新缓存，并触发 heatmap 更新。
type TopologyUpdater struct {
	config UpdaterConfig

	// graphs 存储活跃拓扑图，key 格式: "tenantID:graphType"
	graphs map[string]*graph.Graph
	mu     sync.RWMutex

	// cache 拓扑图缓存 (LRU + TTL)
	cache *cache.Cache

	// heatmapEngine 热力图引擎 (可为 nil)
	heatmapEngine *heatmap.HeatmapEngine

	// versionMap 当前版本号，key 格式: "tenantID:graphType"
	versionMap map[string]uint64

	// lastUpdateMap 最后更新时间戳 (UnixNano)，key 格式: "tenantID:graphType"
	lastUpdateMap map[string]int64

	// versionHistory 版本历史记录，key 格式: "tenantID:graphType"
	// 保留最近 MaxVersionHistory 个版本用于 Diff 计算
	versionHistory map[string][]*versionHistoryEntry

	// flowBuffer 按租户缓冲的待处理流量，key: tenantID
	flowBuffer map[string][]*flow.UnifiedFlow
	bufferMu   sync.Mutex

	// stopCh 用于通知后台 goroutine 停止
	stopCh chan struct{}

	// started 标记是否已启动
	started bool
	startMu sync.Mutex
}

// NewTopologyUpdater 创建增量拓扑更新引擎
//
// 参数:
//   - config: 更新配置 (零值时使用默认值)
//   - cache: 拓扑图缓存实例 (不可为 nil)
//   - heatmapEngine: 热力图引擎 (可为 nil，传入 nil 则跳过 heatmap 更新)
func NewTopologyUpdater(config UpdaterConfig, cache *cache.Cache, heatmapEngine *heatmap.HeatmapEngine) *TopologyUpdater {
	config.normalize()

	return &TopologyUpdater{
		config:         config,
		graphs:         make(map[string]*graph.Graph),
		cache:          cache,
		heatmapEngine:  heatmapEngine,
		versionMap:     make(map[string]uint64),
		lastUpdateMap:  make(map[string]int64),
		versionHistory: make(map[string][]*versionHistoryEntry),
		flowBuffer:     make(map[string][]*flow.UnifiedFlow),
		stopCh:         make(chan struct{}),
	}
}

// ---------------------------------------------------------------------------
// 生命周期管理
// ---------------------------------------------------------------------------

// Start 启动后台更新循环
//
// 启动后会定期执行:
//   - 每 UpdateInterval: 刷新所有租户的缓冲流量
//   - 每 StaleCleanupMultiplier * UpdateInterval: 清理过期条目
func (u *TopologyUpdater) Start() {
	u.startMu.Lock()
	defer u.startMu.Unlock()

	if u.started {
		return
	}
	u.started = true

	go u.updateLoop()
}

// Stop 停止后台更新循环
//
// 调用后:
//   - 后台 goroutine 退出
//   - 所有缓冲的流量被立即刷新
func (u *TopologyUpdater) Stop() {
	u.startMu.Lock()
	defer u.startMu.Unlock()

	if !u.started {
		return
	}
	u.started = false

	close(u.stopCh)

	// 最后一次刷新所有租户的缓冲流量
	u.flushAllTenants()
}

// ---------------------------------------------------------------------------
// 流量摄入
// ---------------------------------------------------------------------------

// IngestFlows 接收一批流量并缓冲
//
// 参数:
//   - tenantID: 租户 ID
//   - flows: 流量批次
//
// 返回:
//   - int: 接受的流量数量
//   - error: 错误信息
//
// 当缓冲区大小超过 FlowBufferFlushThreshold 时，会立即触发该租户的刷新。
func (u *TopologyUpdater) IngestFlows(tenantID string, flows []*flow.UnifiedFlow) (int, error) {
	if len(flows) == 0 {
		return 0, nil
	}

	u.bufferMu.Lock()
	u.flowBuffer[tenantID] = append(u.flowBuffer[tenantID], flows...)
	count := len(u.flowBuffer[tenantID])
	needFlush := count >= FlowBufferFlushThreshold
	u.bufferMu.Unlock()

	if needFlush {
		// 异步刷新，避免阻塞摄入路径
		go u.flushTenant(tenantID)
	}

	return len(flows), nil
}

// ---------------------------------------------------------------------------
// 缓冲区刷新
// ---------------------------------------------------------------------------

// flushTenant 刷新指定租户的缓冲流量
//
// 处理流程:
//  1. 取出并清空缓冲区
//  2. 对每种图类型 (service/process/pod/namespace):
//     a. 获取或创建图
//     b. 标记所有节点/边为不活跃
//     c. 逐条处理流量 (AddOrUpdateNode + AccumulateEdge)
//     d. 移除不活跃的节点/边
//     e. 重新计算边权重
//     f. 创建快照
//     g. 更新缓存
//     h. 更新 heatmap
//     i. 递增版本号
func (u *TopologyUpdater) flushTenant(tenantID string) {
	// 取出缓冲流量
	u.bufferMu.Lock()
	flows := u.flowBuffer[tenantID]
	if len(flows) == 0 {
		u.bufferMu.Unlock()
		return
	}
	u.flowBuffer[tenantID] = nil
	u.bufferMu.Unlock()

	// 对每种图类型执行增量更新
	for _, graphType := range supportedGraphTypes {
		u.flushTenantForGraphType(tenantID, graphType, flows)
	}
}

// flushTenantForGraphType 对指定租户和图类型执行增量更新
func (u *TopologyUpdater) flushTenantForGraphType(tenantID, graphType string, flows []*flow.UnifiedFlow) {
	graphKey := tenantID + ":" + graphType

	// 获取或创建图
	g := u.getOrCreateGraph(tenantID, graphType)

	// 步骤 1: 标记所有节点/边为不活跃
	g.MarkAllInactive()

	// 步骤 2: 逐条处理流量，增量更新图
	for i := range flows {
		processFlowForGraph(g, flows[i], graphType)
	}

	// 步骤 3: 移除不活跃的节点/边
	g.RemoveInactiveNodes()

	// 步骤 4: 重新计算边权重
	g.RecomputeWeights()

	// 步骤 5: 创建快照
	snapshot := g.Snapshot()

	// 步骤 6: 更新缓存
	u.cache.Put(tenantID, graphType, snapshot)

	// 步骤 7: 更新 heatmap
	if u.heatmapEngine != nil {
		u.heatmapEngine.UpdateFromGraph(snapshot)
	}

	// 步骤 8: 递增版本号并记录历史
	u.mu.Lock()
	u.versionMap[graphKey] = snapshot.Version
	u.lastUpdateMap[graphKey] = time.Now().UnixNano()

	// 保存版本历史 (保留最近 MaxVersionHistory 个)
	history := u.versionHistory[graphKey]
	history = append(history, &versionHistoryEntry{
		version:   snapshot.Version,
		snapshot:  snapshot,
		timestamp: snapshot.Timestamp,
	})
	if len(history) > MaxVersionHistory {
		// 丢弃最旧的条目
		history = history[len(history)-MaxVersionHistory:]
	}
	u.versionHistory[graphKey] = history
	u.mu.Unlock()
}

// flushAllTenants 刷新所有租户的缓冲流量
func (u *TopologyUpdater) flushAllTenants() {
	u.bufferMu.Lock()
	tenantIDs := make([]string, 0, len(u.flowBuffer))
	for tenantID := range u.flowBuffer {
		tenantIDs = append(tenantIDs, tenantID)
	}
	u.bufferMu.Unlock()

	for _, tenantID := range tenantIDs {
		u.flushTenant(tenantID)
	}
}

// ---------------------------------------------------------------------------
// 图管理
// ---------------------------------------------------------------------------

// getOrCreateGraph 获取或创建指定租户+图类型的拓扑图
func (u *TopologyUpdater) getOrCreateGraph(tenantID, graphType string) *graph.Graph {
	graphKey := tenantID + ":" + graphType

	u.mu.RLock()
	g, ok := u.graphs[graphKey]
	u.mu.RUnlock()

	if ok {
		return g
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// 双重检查
	if g, ok := u.graphs[graphKey]; ok {
		return g
	}

	g = graph.NewGraph(graphType, tenantID, u.config.MaxNodesPerTenant, u.config.MaxEdgesPerTenant)
	u.graphs[graphKey] = g

	return g
}

// ---------------------------------------------------------------------------
// 查询接口
// ---------------------------------------------------------------------------

// GetGraph 获取指定租户+图类型的当前图快照 (从缓存读取)
//
// 返回:
//   - *graph.GraphSnapshot: 图快照
//   - bool: 是否命中缓存
func (u *TopologyUpdater) GetGraph(tenantID, graphType string) (*graph.GraphSnapshot, bool) {
	return u.cache.Get(tenantID, graphType)
}

// GetVersion 获取指定租户+图类型的当前版本号
func (u *TopologyUpdater) GetVersion(tenantID, graphType string) uint64 {
	graphKey := tenantID + ":" + graphType

	u.mu.RLock()
	v, ok := u.versionMap[graphKey]
	u.mu.RUnlock()

	if !ok {
		return 0
	}
	return v
}

// GetDiff 计算两个版本之间的拓扑差异
//
// 参数:
//   - tenantID: 租户 ID
//   - graphType: 图类型
//   - baseVersion: 基准版本号
//   - compareVersion: 对比版本号
//
// 返回:
//   - *graph.TopologyDiff: 拓扑差异
//   - error: 版本不存在时返回错误
func (u *TopologyUpdater) GetDiff(tenantID, graphType string, baseVersion, compareVersion uint64) (*graph.TopologyDiff, error) {
	graphKey := tenantID + ":" + graphType

	u.mu.RLock()
	history := u.versionHistory[graphKey]
	u.mu.RUnlock()

	if len(history) == 0 {
		return nil, fmt.Errorf("no version history for %s", graphKey)
	}

	// 查找 base 和 compare 版本的快照
	var baseSnap, compareSnap *graph.GraphSnapshot
	for _, entry := range history {
		if entry.version == baseVersion && baseSnap == nil {
			baseSnap = entry.snapshot
		}
		if entry.version == compareVersion && compareSnap == nil {
			compareSnap = entry.snapshot
		}
		if baseSnap != nil && compareSnap != nil {
			break
		}
	}

	if baseSnap == nil {
		return nil, fmt.Errorf("base version %d not found for %s", baseVersion, graphKey)
	}
	if compareSnap == nil {
		return nil, fmt.Errorf("compare version %d not found for %s", compareVersion, graphKey)
	}

	// 从快照重建临时 Graph 以使用 Diff 方法
	baseGraph := snapshotToGraph(baseSnap, graphType, tenantID)
	compareGraph := snapshotToGraph(compareSnap, graphType, tenantID)

	return baseGraph.Diff(compareGraph), nil
}

// ---------------------------------------------------------------------------
// 后台更新循环
// ---------------------------------------------------------------------------

// updateLoop 后台 goroutine 主循环
//
// 定时执行:
//   - 每 UpdateInterval: 刷新所有租户缓冲流量
//   - 每 StaleCleanupMultiplier * UpdateInterval: 清理过期租户条目
func (u *TopologyUpdater) updateLoop() {
	intervalTicker := time.NewTicker(u.config.UpdateInterval)
	defer intervalTicker.Stop()

	staleInterval := u.config.UpdateInterval * StaleCleanupMultiplier
	staleTicker := time.NewTicker(staleInterval)
	defer staleTicker.Stop()

	cleanupCount := 0

	for {
		select {
		case <-u.stopCh:
			return
		case <-intervalTicker.C:
			u.flushAllTenants()
		case <-staleTicker.C:
			cleanupCount++
			u.cleanupStaleEntries()
		}
	}
}

// cleanupStaleEntries 清理过期条目
//
// 移除超过 StaleThreshold 未更新的租户图，释放内存。
func (u *TopologyUpdater) cleanupStaleEntries() {
	now := time.Now().UnixNano()
	staleThresholdNs := u.config.StaleThreshold.Nanoseconds()

	u.mu.Lock()
	defer u.mu.Unlock()

	for graphKey, lastUpdate := range u.lastUpdateMap {
		if now-lastUpdate > staleThresholdNs {
			// 移除图
			if g, ok := u.graphs[graphKey]; ok {
				g.Clear()
				delete(u.graphs, graphKey)
			}
			// 移除版本信息
			delete(u.versionMap, graphKey)
			delete(u.lastUpdateMap, graphKey)
			// 移除版本历史
			delete(u.versionHistory, graphKey)
			// 使缓存失效
			// graphKey 格式: "tenantID:graphType"
			// 需要拆分
			for _, gt := range supportedGraphTypes {
				prefix := ":" + gt
				if len(graphKey) > len(prefix) {
					suffix := graphKey[len(graphKey)-len(prefix):]
					if suffix == prefix {
						tid := graphKey[:len(graphKey)-len(prefix)]
						u.cache.Invalidate(tid, gt)
						break
					}
				}
			}
		}
	}

	// 清理空的流量缓冲区
	u.bufferMu.Lock()
	for tenantID, buf := range u.flowBuffer {
		if len(buf) == 0 {
			delete(u.flowBuffer, tenantID)
		}
	}
	u.bufferMu.Unlock()
}

// ---------------------------------------------------------------------------
// 流量处理 (热路径)
// ---------------------------------------------------------------------------

// processFlowForGraph 将单条流量增量更新到图中
//
// 这是热路径方法，必须高效:
//   - 根据 graphType 提取源/目标节点 ID
//   - 调用 g.AddOrUpdateNode 添加/更新节点
//   - 调用 g.AccumulateEdge 累加边指标
//
// graphType 决定节点 ID 的提取方式:
//   - service:    源/目标 K8s Service 名称
//   - process:    源/目标 进程名 (comm)
//   - pod:        源/目标 Pod 名称
//   - namespace:  源/目标 K8s Namespace
func processFlowForGraph(g *graph.Graph, f *flow.UnifiedFlow, graphType string) {
	var srcID, srcName, tgtID, tgtName, namespace string
	var srcMeta, tgtMeta map[string]string

	switch graphType {
	case graph.GraphTypeService:
		// 服务拓扑: 节点 = K8s Service
		// 源节点: 优先 Service > Deployment > SrcIP
		srcNS := f.Namespace.String()
		if srcNS == "" {
			srcNS = "default"
		}
		if !f.Service.IsZero() {
			srcID = srcNS + "/" + f.Service.String()
			srcName = f.Service.String()
		} else if !f.Deployment.IsZero() {
			srcID = srcNS + "/" + f.Deployment.String()
			srcName = f.Deployment.String()
		} else {
			srcID = f.SrcIP.String()
			srcName = f.SrcIP.String()
		}
		// 目标节点: 优先 Service > Deployment > DstIP
		if !f.Service.IsZero() {
			tgtID = srcNS + "/" + f.Service.String()
			tgtName = f.Service.String()
		} else if !f.Deployment.IsZero() {
			tgtID = srcNS + "/" + f.Deployment.String()
			tgtName = f.Deployment.String()
		} else {
			tgtID = f.DstIP.String()
			tgtName = f.DstIP.String()
		}
		namespace = srcNS
		if srcMeta == nil {
			srcMeta = make(map[string]string, 4)
		}
		srcMeta["deployment"] = f.Deployment.String()
		srcMeta["namespace"] = namespace
		srcMeta["pod"] = f.Pod.String()
		srcMeta["node"] = f.Node.String()

	case graph.GraphTypeProcess:
		// 进程拓扑: 节点 = hostname:processName:pid
		srcHost := f.Hostname.String()
		if srcHost == "" {
			srcHost = f.SrcIP.String()
		}
		tgtHost := f.Hostname.String()
		if tgtHost == "" {
			tgtHost = f.DstIP.String()
		}
		srcComm := f.Comm.String()
		if srcComm == "" {
			srcComm = f.ProcessName.String()
		}
		tgtComm := srcComm // 同一进程通信
		srcID = fmt.Sprintf("%s:%s:%d", srcHost, srcComm, f.PID)
		tgtID = fmt.Sprintf("%s:%s:%d", tgtHost, tgtComm, f.PID)
		srcName = f.ProcessName.String()
		tgtName = f.ProcessName.String()
		namespace = f.Namespace.String()
		if namespace == "" {
			namespace = "default"
		}
		if srcMeta == nil {
			srcMeta = make(map[string]string, 4)
		}
		srcMeta["container"] = f.ContainerName.String()
		srcMeta["pod"] = f.Pod.String()
		srcMeta["namespace"] = namespace
		srcMeta["host"] = srcHost

	case graph.GraphTypePod:
		// Pod 拓扑: 节点 = K8s Pod
		podNS := f.Namespace.String()
		if podNS == "" {
			podNS = "default"
		}
		if !f.Pod.IsZero() {
			srcID = podNS + "/" + f.Pod.String()
			srcName = f.Pod.String()
		} else {
			srcID = f.SrcIP.String()
			srcName = f.SrcIP.String()
		}
		if !f.Pod.IsZero() {
			tgtID = podNS + "/" + f.Pod.String()
			tgtName = f.Pod.String()
		} else {
			tgtID = f.DstIP.String()
			tgtName = f.DstIP.String()
		}
		namespace = podNS
		if srcMeta == nil {
			srcMeta = make(map[string]string, 3)
		}
		srcMeta["namespace"] = namespace
		srcMeta["node"] = f.Node.String()
		srcMeta["service"] = f.Service.String()

	case graph.GraphTypeNamespace:
		// 命名空间拓扑: 节点 = K8s Namespace
		srcID = f.Namespace.String()
		if srcID == "" {
			srcID = "default"
		}
		tgtID = srcID // namespace 内通信
		srcName = srcID
		tgtName = srcID
		namespace = srcID

	default:
		return
	}

	// 跳过空 ID
	if srcID == "" || tgtID == "" {
		return
	}

	// 添加/更新源节点和目标节点
	g.AddOrUpdateNode(srcID, srcName, graphType, namespace, srcMeta)
	g.AddOrUpdateNode(tgtID, tgtName, graphType, namespace, tgtMeta)

	// 计算错误数: HTTP 5xx 视为错误
	var errors uint64
	if f.StatusCode >= 500 && f.StatusCode < 600 {
		errors = 1
	}

	// 累加边指标
	g.AccumulateEdge(srcID, tgtID, f.Bytes, f.Packets, f.LatencyNs, errors)
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// snapshotToGraph 从 GraphSnapshot 重建临时 Graph (用于 Diff 计算)
func snapshotToGraph(snap *graph.GraphSnapshot, graphType, tenantID string) *graph.Graph {
	g := graph.NewGraph(graphType, tenantID, len(snap.Nodes)*2, len(snap.Edges)*2)

	for _, n := range snap.Nodes {
		g.AddOrUpdateNode(n.ID, n.Name, n.Type, n.Namespace, n.Metadata)
	}

	for _, e := range snap.Edges {
		g.AddOrUpdateEdge(e.Source, e.Target, e.Protocol, e.Port)
		// 恢复边指标
		g.AccumulateEdge(e.Source, e.Target, e.Bytes, e.Packets, e.Latency, e.Errors)
	}

	return g
}

// graphKey 生成图键
func graphKey(tenantID, graphType string) string {
	return tenantID + ":" + graphType
}

// ---------------------------------------------------------------------------
// 统计与调试
// ---------------------------------------------------------------------------

// Stats 返回更新引擎的统计信息
func (u *TopologyUpdater) Stats() UpdaterStats {
	u.mu.RLock()
	defer u.mu.RUnlock()

	u.bufferMu.Lock()
	bufferedFlows := 0
	bufferedTenants := 0
	for _, buf := range u.flowBuffer {
		if len(buf) > 0 {
			bufferedFlows += len(buf)
			bufferedTenants++
		}
	}
	u.bufferMu.Unlock()

	return UpdaterStats{
		GraphCount:      len(u.graphs),
		BufferedFlows:   bufferedFlows,
		BufferedTenants: bufferedTenants,
		VersionCount:    len(u.versionMap),
	}
}

// UpdaterStats 更新引擎统计信息
type UpdaterStats struct {
	GraphCount      int // 活跃图数量
	BufferedFlows   int // 缓冲区中待处理的流量总数
	BufferedTenants int // 有缓冲流量的租户数
	VersionCount    int // 有版本记录的图数量
}

// String 返回统计信息的字符串表示
func (s UpdaterStats) String() string {
	return fmt.Sprintf(
		"UpdaterStats{graphs=%d, bufferedFlows=%d, bufferedTenants=%d, versions=%d}",
		s.GraphCount, s.BufferedFlows, s.BufferedTenants, s.VersionCount,
	)
}
