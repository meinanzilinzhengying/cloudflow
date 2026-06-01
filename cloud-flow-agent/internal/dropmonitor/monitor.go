// Package dropmonitor 提供采集器丢包监控功能
//
// 功能：
// 1. 内核态丢包统计（通过eBPF ring buffer接收丢包事件）
// 2. 用户态丢包统计（采集器内部各阶段的丢包）
// 3. 丢包率计算和告警
// 4. 按原因、按流的丢包分析
package dropmonitor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// ============================================================================
// 常量定义
// ============================================================================

// DropReason 丢包原因枚举
const (
	DropReasonUnknown = iota
	DropReasonNoSocket
	DropReasonSocketFilter
	DropReasonTCPCSum
	DropReasonUDPCSum
	DropReasonIPCSum
	DropReasonIPHeader
	DropReasonTCPHeader
	DropReasonUDPHeader
	DropReasonNoRoute
	DropReasonCongestion
	DropReasonRateLimit
	DropReasonRPFilter
	DropReasonNetFilter
	DropReasonTC
	DropReasonXDP
	DropReasonMax
)

// DropReasonNames 丢包原因名称映射
var DropReasonNames = map[int]string{
	DropReasonUnknown:      "unknown",
	DropReasonNoSocket:     "no_socket",
	DropReasonSocketFilter: "socket_filter",
	DropReasonTCPCSum:      "tcp_csum",
	DropReasonUDPCSum:      "udp_csum",
	DropReasonIPCSum:       "ip_csum",
	DropReasonIPHeader:     "ip_header",
	DropReasonTCPHeader:    "tcp_header",
	DropReasonUDPHeader:    "udp_header",
	DropReasonNoRoute:      "no_route",
	DropReasonCongestion:   "congestion",
	DropReasonRateLimit:    "rate_limit",
	DropReasonRPFilter:     "rp_filter",
	DropReasonNetFilter:    "netfilter",
	DropReasonTC:           "tc",
	DropReasonXDP:          "xdp",
}

// UserDropStage 用户态丢包阶段枚举
const (
	UserDropStageEBPFSubmit = iota  // eBPF提交失败
	UserDropStageRingBufFull        // Ring Buffer满
	UserDropStageParseFail          // 解析失败
	UserDropStageBatchDrop          // 批处理丢弃
	UserDropStageSendFail           // 发送失败
	UserDropStageCacheFail          // 缓存失败
	UserDropStageFilterDrop         // 过滤器丢弃
	UserDropStageMax
)

// UserDropStageNames 用户态丢包阶段名称
var UserDropStageNames = map[int]string{
	UserDropStageEBPFSubmit: "ebpf_submit_fail",
	UserDropStageRingBufFull: "ringbuf_full",
	UserDropStageParseFail:   "parse_fail",
	UserDropStageBatchDrop:   "batch_drop",
	UserDropStageSendFail:    "send_fail",
	UserDropStageCacheFail:   "cache_fail",
	UserDropStageFilterDrop:  "filter_drop",
}

// ============================================================================
// 数据结构
// ============================================================================

// DropEvent 丢包事件（从eBPF接收）
type DropEvent struct {
	TimestampNs uint64 `json:"timestamp_ns"`
	PID         uint32 `json:"pid"`
	Reason      uint32 `json:"reason"`
	SrcIP       uint32 `json:"saddr"`
	DstIP       uint32 `json:"daddr"`
	SrcPort     uint16 `json:"sport"`
	DstPort     uint16 `json:"dport"`
	Protocol    uint8  `json:"protocol"`
	SKBLen      uint32 `json:"skb_len"`
	DropCount   uint32 `json:"drop_count"`
}

// FlowKey 流标识
type FlowKey struct {
	SrcIP    uint32
	DstIP    uint32
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

// String 返回流的可读表示
func (f FlowKey) String() string {
	return fmt.Sprintf("%s:%d->%s:%d/%d",
		ipToString(f.SrcIP), f.SrcPort,
		ipToString(f.DstIP), f.DstPort, f.Protocol)
}

// FlowDropStat 流级丢包统计
type FlowDropStat struct {
	FlowKey
	TotalDrops    uint64    `json:"total_drops"`
	TotalBytes    uint64    `json:"total_bytes"`
	LastDropTime  time.Time `json:"last_drop_time"`
	FirstDropTime time.Time `json:"first_drop_time"`
}

// KernelDropStat 内核态按原因丢包统计
type KernelDropStat struct {
	Reason       int       `json:"reason"`
	ReasonName   string    `json:"reason_name"`
	Count        uint64    `json:"count"`
	Bytes        uint64    `json:"bytes"`
	LastDropTime time.Time `json:"last_drop_time"`
}

// UserDropStat 用户态丢包统计
type UserDropStat struct {
	Stage        int       `json:"stage"`
	StageName    string    `json:"stage_name"`
	Count        uint64    `json:"count"`
	LastDropTime time.Time `json:"last_drop_time"`
}

// Snapshot 丢包监控快照
type Snapshot struct {
	Timestamp        time.Time         `json:"timestamp"`
	KernelTotalDrops uint64            `json:"kernel_total_drops"`
	KernelTotalBytes uint64            `json:"kernel_total_bytes"`
	UserTotalDrops   uint64            `json:"user_total_drops"`
	RingBufFailures  uint64            `json:"ringbuf_failures"`
	DropRate         float64           `json:"drop_rate"`           // 丢包率(0-100)
	KernelByReason   []KernelDropStat  `json:"kernel_by_reason"`
	UserByStage      []UserDropStat    `json:"user_by_stage"`
	TopFlows         []FlowDropStat    `json:"top_flows"`
}

// Config 丢包监控配置
type Config struct {
	Enabled           bool          // 启用丢包监控
	RingBufSize       int           // Ring Buffer大小
	SampleRate        float64       // 采样率(0-1)
	TopFlowCount      int           // 记录TOP N流
	SnapshotInterval  time.Duration // 快照间隔
	AlertThreshold    float64       // 丢包率告警阈值(%)
	EnableKernelDrop  bool          // 启用内核态丢包监控
	EnableUserDrop    bool          // 启用用户态丢包监控
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		RingBufSize:      256 * 1024,
		SampleRate:       1.0,
		TopFlowCount:     10,
		SnapshotInterval: 10 * time.Second,
		AlertThreshold:   1.0,  // 1%丢包率触发告警
		EnableKernelDrop: true,
		EnableUserDrop:   true,
	}
}

// ============================================================================
// Monitor 丢包监控器
// ============================================================================

// Monitor 丢包监控器
type Monitor struct {
	config Config
	log    *logger.Logger

	// 内核态统计
	kernelTotalDrops uint64
	kernelTotalBytes uint64
	ringBufFailures  uint64
	kernelByReason   map[int]*KernelDropStat

	// 用户态统计
	userTotalDrops uint64
	userByStage    map[int]*UserDropStat

	// 流级统计
	flowStats map[FlowKey]*FlowDropStat
	flowMu    sync.RWMutex

	// 总接收包数（用于计算丢包率）
	totalReceived uint64
	totalDropped  uint64

	// 事件通道
	eventCh chan *DropEvent
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// 回调函数
	onAlert func(dropRate float64, message string)
}

// NewMonitor 创建丢包监控器
func NewMonitor(cfg Config, log *logger.Logger) *Monitor {
	if cfg.RingBufSize == 0 {
		cfg = DefaultConfig()
	}

	return &Monitor{
		config:         cfg,
		log:            log,
		kernelByReason: make(map[int]*KernelDropStat),
		userByStage:    make(map[int]*UserDropStat),
		flowStats:      make(map[FlowKey]*FlowDropStat),
		eventCh:        make(chan *DropEvent, 1000),
		stopCh:         make(chan struct{}),
	}
}

// OnAlert 设置告警回调
func (m *Monitor) OnAlert(fn func(dropRate float64, message string)) {
	m.onAlert = fn
}

// Start 启动监控器
func (m *Monitor) Start() error {
	if !m.config.Enabled {
		return nil
	}

	m.wg.Add(2)
	go m.eventProcessor()
	go m.snapshotLoop()

	m.log.Info("[丢包监控] 已启动")
	return nil
}

// Stop 停止监控器
func (m *Monitor) Stop() {
	if !m.config.Enabled {
		return
	}

	close(m.stopCh)
	m.wg.Wait()
	m.log.Info("[丢包监控] 已停止")
}

// eventProcessor 事件处理循环
func (m *Monitor) eventProcessor() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.eventCh:
			m.processDropEvent(event)
		case <-m.stopCh:
			return
		}
	}
}

// processDropEvent 处理丢包事件
func (m *Monitor) processDropEvent(event *DropEvent) {
	// 更新内核态总计
	atomic.AddUint64(&m.kernelTotalDrops, uint64(event.DropCount))
	atomic.AddUint64(&m.kernelTotalBytes, uint64(event.SKBLen))
	atomic.AddUint64(&m.totalDropped, uint64(event.DropCount))

	// 更新按原因统计
	reason := int(event.Reason)
	if reason >= DropReasonMax {
		reason = DropReasonUnknown
	}

	if stat, ok := m.kernelByReason[reason]; ok {
		stat.Count += uint64(event.DropCount)
		stat.Bytes += uint64(event.SKBLen)
		stat.LastDropTime = time.Now()
	} else {
		m.kernelByReason[reason] = &KernelDropStat{
			Reason:       reason,
			ReasonName:   DropReasonNames[reason],
			Count:        uint64(event.DropCount),
			Bytes:        uint64(event.SKBLen),
			LastDropTime: time.Now(),
		}
	}

	// 更新流级统计
	flowKey := FlowKey{
		SrcIP:    event.SrcIP,
		DstIP:    event.DstIP,
		SrcPort:  event.SrcPort,
		DstPort:  event.DstPort,
		Protocol: event.Protocol,
	}

	m.flowMu.Lock()
	if stat, ok := m.flowStats[flowKey]; ok {
		stat.TotalDrops += uint64(event.DropCount)
		stat.TotalBytes += uint64(event.SKBLen)
		stat.LastDropTime = time.Now()
	} else {
		m.flowStats[flowKey] = &FlowDropStat{
			FlowKey:       flowKey,
			TotalDrops:    uint64(event.DropCount),
			TotalBytes:    uint64(event.SKBLen),
			FirstDropTime: time.Now(),
			LastDropTime:  time.Now(),
		}
	}
	m.flowMu.Unlock()
}

// snapshotLoop 快照循环
func (m *Monitor) snapshotLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.SnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.takeSnapshot()
		case <-m.stopCh:
			return
		}
	}
}

// takeSnapshot 生成快照并检查告警
func (m *Monitor) takeSnapshot() {
	snapshot := m.GetSnapshot()

	// 检查丢包率告警
	if snapshot.DropRate > m.config.AlertThreshold {
		msg := fmt.Sprintf("丢包率过高: %.2f%% (阈值: %.2f%%), 内核丢包=%d, 用户丢包=%d",
			snapshot.DropRate, m.config.AlertThreshold,
			snapshot.KernelTotalDrops, snapshot.UserTotalDrops)
		m.log.Warnf("[丢包监控] %s", msg)

		if m.onAlert != nil {
			m.onAlert(snapshot.DropRate, msg)
		}
	}
}

// RecordUserDrop 记录用户态丢包
func (m *Monitor) RecordUserDrop(stage int, count int) {
	if !m.config.Enabled || !m.config.EnableUserDrop {
		return
	}

	if stage >= UserDropStageMax {
		stage = UserDropStageEBPFSubmit
	}

	atomic.AddUint64(&m.userTotalDrops, uint64(count))
	atomic.AddUint64(&m.totalDropped, uint64(count))

	if stat, ok := m.userByStage[stage]; ok {
		stat.Count += uint64(count)
		stat.LastDropTime = time.Now()
	} else {
		m.userByStage[stage] = &UserDropStat{
			Stage:        stage,
			StageName:    UserDropStageNames[stage],
			Count:        uint64(count),
			LastDropTime: time.Now(),
		}
	}
}

// RecordReceived 记录接收包数
func (m *Monitor) RecordReceived(count int) {
	atomic.AddUint64(&m.totalReceived, uint64(count))
}

// RecordRingBufFailure 记录Ring Buffer失败
func (m *Monitor) RecordRingBufFailure() {
	atomic.AddUint64(&m.ringBufFailures, 1)
}

// SubmitEvent 提交丢包事件（从eBPF接收）
func (m *Monitor) SubmitEvent(event *DropEvent) {
	if !m.config.Enabled || !m.config.EnableKernelDrop {
		return
	}

	// 采样
	if m.config.SampleRate < 1.0 && randomFloat() > m.config.SampleRate {
		return
	}

	select {
	case m.eventCh <- event:
	default:
		// 通道满，丢弃事件（避免阻塞）
		m.RecordUserDrop(UserDropStageRingBufFull, 1)
	}
}

// GetSnapshot 获取当前快照
func (m *Monitor) GetSnapshot() *Snapshot {
	snapshot := &Snapshot{
		Timestamp:        time.Now(),
		KernelTotalDrops: atomic.LoadUint64(&m.kernelTotalDrops),
		KernelTotalBytes: atomic.LoadUint64(&m.kernelTotalBytes),
		UserTotalDrops:   atomic.LoadUint64(&m.userTotalDrops),
		RingBufFailures:  atomic.LoadUint64(&m.ringBufFailures),
	}

	// 计算丢包率
	received := atomic.LoadUint64(&m.totalReceived)
	dropped := atomic.LoadUint64(&m.totalDropped)
	if received+dropped > 0 {
		snapshot.DropRate = float64(dropped) * 100.0 / float64(received+dropped)
	}

	// 复制按原因统计
	snapshot.KernelByReason = make([]KernelDropStat, 0, len(m.kernelByReason))
	for _, stat := range m.kernelByReason {
		snapshot.KernelByReason = append(snapshot.KernelByReason, *stat)
	}

	// 复制用户态统计
	snapshot.UserByStage = make([]UserDropStat, 0, len(m.userByStage))
	for _, stat := range m.userByStage {
		snapshot.UserByStage = append(snapshot.UserByStage, *stat)
	}

	// 获取TOP流
	snapshot.TopFlows = m.getTopFlows(m.config.TopFlowCount)

	return snapshot
}

// getTopFlows 获取丢包最多的N个流
func (m *Monitor) getTopFlows(n int) []FlowDropStat {
	m.flowMu.RLock()
	defer m.flowMu.RUnlock()

	// 简单选择排序，取TOP N
	flows := make([]*FlowDropStat, 0, len(m.flowStats))
	for _, stat := range m.flowStats {
		flows = append(flows, stat)
	}

	// 按丢包数排序
	for i := 0; i < len(flows) && i < n; i++ {
		maxIdx := i
		for j := i + 1; j < len(flows); j++ {
			if flows[j].TotalDrops > flows[maxIdx].TotalDrops {
				maxIdx = j
			}
		}
		flows[i], flows[maxIdx] = flows[maxIdx], flows[i]
	}

	result := make([]FlowDropStat, 0, n)
	for i := 0; i < len(flows) && i < n; i++ {
		result = append(result, *flows[i])
	}

	return result
}

// Reset 重置统计
func (m *Monitor) Reset() {
	atomic.StoreUint64(&m.kernelTotalDrops, 0)
	atomic.StoreUint64(&m.kernelTotalBytes, 0)
	atomic.StoreUint64(&m.userTotalDrops, 0)
	atomic.StoreUint64(&m.ringBufFailures, 0)
	atomic.StoreUint64(&m.totalReceived, 0)
	atomic.StoreUint64(&m.totalDropped, 0)

	m.kernelByReason = make(map[int]*KernelDropStat)
	m.userByStage = make(map[int]*UserDropStat)

	m.flowMu.Lock()
	m.flowStats = make(map[FlowKey]*FlowDropStat)
	m.flowMu.Unlock()
}

// GetDropRate 获取当前丢包率
func (m *Monitor) GetDropRate() float64 {
	received := atomic.LoadUint64(&m.totalReceived)
	dropped := atomic.LoadUint64(&m.totalDropped)
	if received+dropped == 0 {
		return 0
	}
	return float64(dropped) * 100.0 / float64(received+dropped)
}

// ============================================================================
// 辅助函数
// ============================================================================

func ipToString(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		(ip>>24)&0xFF, (ip>>16)&0xFF, (ip>>8)&0xFF, ip&0xFF)
}

func randomFloat() float64 {
	// 简单伪随机数
	return float64(time.Now().UnixNano()%1000) / 1000.0
}
