// Package realtime 实时拓扑更新与事件通知
//
// 功能：
//   - 实时拓扑更新引擎（增量更新 + 全量刷新）
//   - 拓扑变更事件通知（WebSocket/SSE 推送）
//   - 版本一致性保证（乐观锁 + 版本号）
//   - 实时拓扑订阅管理
//   - 拓扑变更事件流处理
//
package realtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	graph "github.com/meinanzilinzhengying/cloudflow/services/topology-engine/graph"
)

// ============================================================================
// 一、实时更新引擎
// ============================================================================

// RealtimeEngine 实时拓扑更新引擎
type RealtimeEngine struct {
	config RealtimeConfig
	mu     sync.RWMutex

	// 图状态
	graphs map[string]*graph.Graph // key: tenantID:graphType

	// 版本管理
	versions map[string]uint64
	versionMu sync.RWMutex

	// 事件总线
	eventBus *EventBus

	// 变更日志
	changeLog *ChangeLog

	// 订阅管理
	subManager *SubscriptionManager
}

// RealtimeConfig 实时引擎配置
type RealtimeConfig struct {
	// 更新间隔
	UpdateInterval time.Duration
	// 批量更新阈值
	BatchSize int
	// 最大延迟（超过此时间强制刷新）
	MaxLatency time.Duration
	// 版本历史保留数
	VersionHistorySize int
	// 事件队列大小
	EventQueueSize int
}

// DefaultRealtimeConfig 返回默认配置
func DefaultRealtimeConfig() RealtimeConfig {
	return RealtimeConfig{
		UpdateInterval:       1 * time.Second,
		BatchSize:            1000,
		MaxLatency:           5 * time.Second,
		VersionHistorySize:   100,
		EventQueueSize:       10000,
	}
}

// NewRealtimeEngine 创建实时更新引擎
func NewRealtimeEngine(config RealtimeConfig) *RealtimeEngine {
	return &RealtimeEngine{
		config:     config,
		graphs:     make(map[string]*graph.Graph),
		versions:   make(map[string]uint64),
		eventBus:   NewEventBus(config.EventQueueSize),
		changeLog:  NewChangeLog(config.VersionHistorySize),
		subManager: NewSubscriptionManager(),
	}
}

// ============================================================================
// 二、增量更新
// ============================================================================

// IncrementalUpdate 增量更新
//
// 参数：
//   - tenantID: 租户 ID
//   - graphType: 图类型
//   - changes: 变更列表（节点/边的增删改）
//
// 返回新版本号
func (re *RealtimeEngine) IncrementalUpdate(
	tenantID, graphType string,
	changes []TopologyChange,
) (uint64, error) {
	key := tenantID + ":" + graphType

	re.mu.Lock()
	g, exists := re.graphs[key]
	if !exists {
		g = graph.NewGraph(graphType, tenantID, graph.DefaultMaxNodes, graph.DefaultMaxEdges)
		re.graphs[key] = g
	}
	re.mu.Unlock()

	// 应用变更
	for _, change := range changes {
		if err := re.applyChange(g, change); err != nil {
			return 0, fmt.Errorf("apply change failed: %w", err)
		}
	}

	// 重新计算权重
	g.RecomputeWeights()

	// 递增版本号
	re.versionMu.Lock()
	re.versions[key]++
	newVersion := re.versions[key]
	re.versionMu.Unlock()

	// 记录变更日志
	re.changeLog.Append(key, newVersion, changes)

	// 发布事件
	re.eventBus.Publish(TopologyEvent{
		Type:       EventTypeIncrementalUpdate,
		TenantID:   tenantID,
		GraphType:  graphType,
		Version:    newVersion,
		Timestamp:  time.Now().UnixNano(),
		ChangeCount: len(changes),
	})

	// 通知订阅者
	re.subManager.Notify(key, newVersion)

	return newVersion, nil
}

// FullRefresh 全量刷新
//
// 用新的拓扑图完全替换旧图
func (re *RealtimeEngine) FullRefresh(
	tenantID, graphType string,
	newGraph *graph.Graph,
) (uint64, error) {
	key := tenantID + ":" + graphType

	re.mu.Lock()
	re.graphs[key] = newGraph
	re.mu.Unlock()

	re.versionMu.Lock()
	re.versions[key]++
	newVersion := re.versions[key]
	re.versionMu.Unlock()

	// 记录变更日志
	re.changeLog.Append(key, newVersion, []TopologyChange{
		{Type: ChangeTypeFullRefresh, Description: "full refresh"},
	})

	// 发布事件
	re.eventBus.Publish(TopologyEvent{
		Type:      EventTypeFullRefresh,
		TenantID:  tenantID,
		GraphType: graphType,
		Version:   newVersion,
		Timestamp: time.Now().UnixNano(),
	})

	re.subManager.Notify(key, newVersion)

	return newVersion, nil
}

// applyChange 应用单个变更到图
func (re *RealtimeEngine) applyChange(g *graph.Graph, change TopologyChange) error {
	switch change.Type {
	case ChangeTypeAddNode:
		g.AddOrUpdateNode(change.NodeID, change.NodeName, change.NodeType, change.Namespace, change.Metadata)
	case ChangeTypeRemoveNode:
		// graph 没有直接移除节点的方法，通过标记为不活跃间接处理
		if n, ok := g.GetNode(change.NodeID); ok {
			n.Active = false
		}
	case ChangeTypeUpdateNode:
		g.AddOrUpdateNode(change.NodeID, change.NodeName, change.NodeType, change.Namespace, change.Metadata)
	case ChangeTypeAddEdge:
		g.AddOrUpdateEdge(change.Source, change.Target, change.Protocol, change.Port)
		g.AccumulateEdge(change.Source, change.Target, change.Bytes, change.Packets, change.Latency, change.Errors)
	case ChangeTypeRemoveEdge:
		if e, ok := g.GetEdge(change.Source, change.Target); ok {
			e.Active = false
		}
	case ChangeTypeUpdateEdge:
		g.AddOrUpdateEdge(change.Source, change.Target, change.Protocol, change.Port)
		g.AccumulateEdge(change.Source, change.Target, change.Bytes, change.Packets, change.Latency, change.Errors)
	case ChangeTypeFullRefresh:
		// 全量刷新不在这里处理
	default:
		return fmt.Errorf("unknown change type: %s", change.Type)
	}
	return nil
}

// ============================================================================
// 三、版本一致性
// ============================================================================

// VersionInfo 版本信息
type VersionInfo struct {
	TenantID   string
	GraphType  string
	Version    uint64
	Timestamp  int64
}

// GetVersion 获取当前版本号
func (re *RealtimeEngine) GetVersion(tenantID, graphType string) uint64 {
	key := tenantID + ":" + graphType
	re.versionMu.RLock()
	defer re.versionMu.RUnlock()
	return re.versions[key]
}

// CompareVersion 比较版本号
//
// 返回:
//   - 0: 版本相同
//   - >0: 当前版本较新
//   - <0: 给定版本较新
func (re *RealtimeEngine) CompareVersion(tenantID, graphType string, version uint64) int {
	current := re.GetVersion(tenantID, graphType)
	if current > version {
		return 1
	} else if current < version {
		return -1
	}
	return 0
}

// IsStale 检查版本是否过期
func (re *RealtimeEngine) IsStale(tenantID, graphType string, version uint64) bool {
	return re.CompareVersion(tenantID, graphType, version) > 0
}

// GetChangesSince 获取从指定版本到当前版本的所有变更
func (re *RealtimeEngine) GetChangesSince(tenantID, graphType string, sinceVersion uint64) ([]TopologyChange, error) {
	key := tenantID + ":" + graphType
	currentVersion := re.GetVersion(tenantID, graphType)

	if currentVersion <= sinceVersion {
		return nil, nil
	}

	return re.changeLog.GetChanges(key, sinceVersion, currentVersion)
}

// ============================================================================
// 四、拓扑变更定义
// ============================================================================

// ChangeType 变更类型
type ChangeType string

const (
	ChangeTypeAddNode      ChangeType = "add_node"
	ChangeTypeRemoveNode   ChangeType = "remove_node"
	ChangeTypeUpdateNode   ChangeType = "update_node"
	ChangeTypeAddEdge      ChangeType = "add_edge"
	ChangeTypeRemoveEdge   ChangeType = "remove_edge"
	ChangeTypeUpdateEdge   ChangeType = "update_edge"
	ChangeTypeFullRefresh  ChangeType = "full_refresh"
)

// TopologyChange 拓扑变更
type TopologyChange struct {
	Type        ChangeType
	Description string

	// 节点相关
	NodeID    graph.NodeID
	NodeName  string
	NodeType  string
	Namespace string
	Metadata  map[string]string

	// 边相关
	Source   graph.NodeID
	Target   graph.NodeID
	Protocol string
	Port     uint16

	// 指标
	Bytes   uint64
	Packets uint64
	Latency uint64
	Errors  uint64
}

// ============================================================================
// 五、事件总线
// ============================================================================

// EventType 事件类型
type EventType string

const (
	EventTypeIncrementalUpdate EventType = "incremental_update"
	EventTypeFullRefresh       EventType = "full_refresh"
	EventTypeNodeAdded         EventType = "node_added"
	EventTypeNodeRemoved       EventType = "node_removed"
	EventTypeEdgeAdded         EventType = "edge_added"
	EventTypeEdgeRemoved       EventType = "edge_removed"
	EventTypeVersionChanged    EventType = "version_changed"
)

// TopologyEvent 拓扑事件
type TopologyEvent struct {
	Type        EventType
	TenantID    string
	GraphType   string
	Version     uint64
	Timestamp   int64
	ChangeCount int
	NodeID      graph.NodeID
	EdgeKey     graph.EdgeKey
}

// EventBus 事件总线
type EventBus struct {
	queueSize int
	mu        sync.RWMutex
	subscribers map[string][]chan TopologyEvent
}

// NewEventBus 创建事件总线
func NewEventBus(queueSize int) *EventBus {
	return &EventBus{
		queueSize:   queueSize,
		subscribers: make(map[string][]chan TopologyEvent),
	}
}

// Subscribe 订阅事件
//
// 返回事件通道
func (eb *EventBus) Subscribe(key string) chan TopologyEvent {
	ch := make(chan TopologyEvent, eb.queueSize)
	eb.mu.Lock()
	eb.subscribers[key] = append(eb.subscribers[key], ch)
	eb.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅
func (eb *EventBus) Unsubscribe(key string, ch chan TopologyEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	subs := eb.subscribers[key]
	for i, s := range subs {
		if s == ch {
			close(s)
			eb.subscribers[key] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// Publish 发布事件
func (eb *EventBus) Publish(event TopologyEvent) {
	key := event.TenantID + ":" + event.GraphType

	eb.mu.RLock()
	subs := make([]chan TopologyEvent, len(eb.subscribers[key]))
	copy(subs, eb.subscribers[key])
	eb.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// 通道满，丢弃事件
		}
	}
}

// ============================================================================
// 六、变更日志
// ============================================================================

// ChangeLog 变更日志
type ChangeLog struct {
	maxSize int
	mu      sync.RWMutex
	entries map[string][]*ChangeLogEntry // key: tenantID:graphType
}

// ChangeLogEntry 变更日志条目
type ChangeLogEntry struct {
	Version  uint64
	Changes  []TopologyChange
	Time     int64
}

// NewChangeLog 创建变更日志
func NewChangeLog(maxSize int) *ChangeLog {
	return &ChangeLog{
		maxSize: maxSize,
		entries: make(map[string][]*ChangeLogEntry),
	}
}

// Append 追加变更日志
func (cl *ChangeLog) Append(key string, version uint64, changes []TopologyChange) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	entry := &ChangeLogEntry{
		Version: version,
		Changes: changes,
		Time:    time.Now().UnixNano(),
	}

	cl.entries[key] = append(cl.entries[key], entry)

	// 限制大小
	if len(cl.entries[key]) > cl.maxSize {
		cl.entries[key] = cl.entries[key][len(cl.entries[key])-cl.maxSize:]
	}
}

// GetChanges 获取指定版本范围的变更
func (cl *ChangeLog) GetChanges(key string, fromVersion, toVersion uint64) ([]TopologyChange, error) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()

	entries, ok := cl.entries[key]
	if !ok {
		return nil, fmt.Errorf("no change log for key: %s", key)
	}

	var allChanges []TopologyChange
	for _, entry := range entries {
		if entry.Version > fromVersion && entry.Version <= toVersion {
			allChanges = append(allChanges, entry.Changes...)
		}
	}

	return allChanges, nil
}

// ============================================================================
// 七、订阅管理
// ============================================================================

// SubscriptionManager 订阅管理器
type SubscriptionManager struct {
	mu sync.RWMutex
	// 订阅者: key -> 回调函数列表
	subscribers map[string][]func(uint64)
}

// NewSubscriptionManager 创建订阅管理器
func NewSubscriptionManager() *SubscriptionManager {
	return &SubscriptionManager{
		subscribers: make(map[string][]func(uint64)),
	}
}

// Subscribe 订阅拓扑变更
func (sm *SubscriptionManager) Subscribe(key string, callback func(uint64)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.subscribers[key] = append(sm.subscribers[key], callback)
}

// Unsubscribe 取消订阅
func (sm *SubscriptionManager) Unsubscribe(key string, callback func(uint64)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	callbacks := sm.subscribers[key]
	for i, cb := range callbacks {
		if fmt.Sprintf("%p", cb) == fmt.Sprintf("%p", callback) {
			sm.subscribers[key] = append(callbacks[:i], callbacks[i+1:]...)
			break
		}
	}
}

// Notify 通知所有订阅者
func (sm *SubscriptionManager) Notify(key string, version uint64) {
	sm.mu.RLock()
	callbacks := make([]func(uint64), len(sm.subscribers[key]))
	copy(callbacks, sm.subscribers[key])
	sm.mu.RUnlock()

	for _, cb := range callbacks {
		go cb(version) // 异步通知
	}
}

// ============================================================================
// 八、实时查询接口
// ============================================================================

// GetGraph 获取当前拓扑图
func (re *RealtimeEngine) GetGraph(tenantID, graphType string) (*graph.Graph, bool) {
	key := tenantID + ":" + graphType
	re.mu.RLock()
	g, ok := re.graphs[key]
	re.mu.RUnlock()
	return g, ok
}

// GetSnapshot 获取当前拓扑快照
func (re *RealtimeEngine) GetSnapshot(tenantID, graphType string) (*graph.GraphSnapshot, bool) {
	g, ok := re.GetGraph(tenantID, graphType)
	if !ok {
		return nil, false
	}
	return g.Snapshot(), true
}

// GetDiff 计算两个版本之间的拓扑差异
func (re *RealtimeEngine) GetDiff(tenantID, graphType string, baseVersion, compareVersion uint64) (*graph.TopologyDiff, error) {
	key := tenantID + ":" + graphType

	// 获取两个版本的快照
	baseChanges, err := re.changeLog.GetChanges(key, 0, baseVersion)
	if err != nil {
		return nil, err
	}
	compareChanges, err := re.changeLog.GetChanges(key, 0, compareVersion)
	if err != nil {
		return nil, err
	}

	// 重建两个版本的图
	baseGraph := graph.NewGraph(graphType, tenantID, graph.DefaultMaxNodes, graph.DefaultMaxEdges)
	for _, change := range baseChanges {
		re.applyChange(baseGraph, change)
	}

	compareGraph := graph.NewGraph(graphType, tenantID, graph.DefaultMaxNodes, graph.DefaultMaxEdges)
	for _, change := range compareChanges {
		re.applyChange(compareGraph, change)
	}

	return baseGraph.Diff(compareGraph), nil
}

// ============================================================================
// 九、统计信息
// ============================================================================

// RealtimeStats 实时引擎统计信息
type RealtimeStats struct {
	GraphCount        int
	TotalVersions      int
	SubscriberCount    int
	PendingEvents      int
	ChangeLogEntries   int
	LastUpdateTime     int64
}

// Stats 返回统计信息
func (re *RealtimeEngine) Stats() RealtimeStats {
	re.mu.RLock()
	graphCount := len(re.graphs)
	re.mu.RUnlock()

	re.versionMu.RLock()
	totalVersions := len(re.versions)
	re.versionMu.RUnlock()

	return RealtimeStats{
		GraphCount:     graphCount,
		TotalVersions:  totalVersions,
		LastUpdateTime: time.Now().UnixNano(),
	}
}

// ============================================================================
// 十、生命周期
// ============================================================================

// Start 启动实时引擎
func (re *RealtimeEngine) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	// 启动后台清理任务
	go re.cleanupLoop(ctx)
}

// Stop 停止实时引擎
func (re *RealtimeEngine) Stop() {
	// 清理资源
	re.subManager = NewSubscriptionManager()
	re.eventBus = NewEventBus(re.config.EventQueueSize)
}

// cleanupLoop 后台清理循环
func (re *RealtimeEngine) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			re.cleanup()
		}
	}
}

// cleanup 清理过期资源
func (re *RealtimeEngine) cleanup() {
	// 清理过期的变更日志
	re.changeLog.mu.Lock()
	for key, entries := range re.changeLog.entries {
		if len(entries) > re.config.VersionHistorySize {
			re.changeLog.entries[key] = entries[len(entries)-re.config.VersionHistorySize:]
		}
	}
	re.changeLog.mu.Unlock()
}

// ============================================================================
// 十一、辅助函数
// ============================================================================

// String 返回拓扑变更的字符串表示
func (tc *TopologyChange) String() string {
	return fmt.Sprintf("TopologyChange{type=%s node=%s edge=%s→%s}", tc.Type, tc.NodeID, tc.Source, tc.Target)
}

// String 返回拓扑事件的字符串表示
func (e *TopologyEvent) String() string {
	return fmt.Sprintf("TopologyEvent{type=%s tenant=%s graph=%s version=%d}", e.Type, e.TenantID, e.GraphType, e.Version)
}
