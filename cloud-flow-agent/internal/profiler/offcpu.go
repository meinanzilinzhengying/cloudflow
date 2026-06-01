// Package profiler 提供 OFF-CPU 性能剖析功能
// 本文件实现 OFF-CPU 剖析器，采集线程阻塞、等待事件
// 支持定位 IO 等待、锁竞争、调度延迟等瓶颈
package profiler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ==================== OFF-CPU 事件类型 ====================

// OffCPUReason 阻塞原因类型
type OffCPUReason string

const (
	OffCPUReasonIOWait      OffCPUReason = "io_wait"       // IO 等待
	OffCPUReasonLockContention OffCPUReason = "lock_contention" // 锁竞争
	OffCPUReasonScheduler   OffCPUReason = "scheduler"     // 调度延迟
	OffCPUReasonNetwork     OffCPUReason = "network"       // 网络等待
	OffCPUReasonDisk        OffCPUReason = "disk"          // 磁盘等待
	OffCPUReasonFutex       OffCPUReason = "futex"         // futex 等待
	OffCPUReasonPipe        OffCPUReason = "pipe"          // 管道等待
	OffCPUReasonPoll        OffCPUReason = "poll"          // poll/select 等待
	OffCPUReasonSleep       OffCPUReason = "sleep"         // 主动睡眠
	OffCPUReasonUnknown     OffCPUReason = "unknown"       // 未知原因
)

// OffCPUEvent OFF-CPU 事件
// 记录线程从 CPU 上离开到重新进入 CPU 的完整信息
type OffCPUEvent struct {
	Timestamp     int64        `json:"timestamp"`      // 事件时间戳
	PID           uint32       `json:"pid"`            // 进程 ID
	TID           uint32       `json:"tid"`            // 线程 ID
	ProcessName   string       `json:"process_name"`   // 进程名
	ThreadName    string       `json:"thread_name"`    // 线程名

	// 时间信息
	OffCPUTime    int64        `json:"off_cpu_time"`   // 离开 CPU 时间
	OnCPUTime     int64        `json:"on_cpu_time"`    // 重新进入 CPU 时间
	Duration      int64        `json:"duration"`       // 阻塞时长 (微秒)

	// 阻塞原因
	Reason        OffCPUReason `json:"reason"`         // 阻塞原因分类
	Syscall       string       `json:"syscall"`        // 系统调用名
	SyscallArgs   []string     `json:"syscall_args"`   // 系统调用参数

	// 调用栈
	StackTrace    []string     `json:"stack_trace"`    // 用户态调用栈
	KernelStack   []string     `json:"kernel_stack"`   // 内核态调用栈

	// 额外信息
	FilePath      string       `json:"file_path"`      // 相关文件路径 (IO 操作)
	FD            int          `json:"fd"`             // 文件描述符
	LockAddr      uint64       `json:"lock_addr"`      // 锁地址
	WaitChannel   string       `json:"wait_channel"`   // 等待通道
}

// OffCPUStats OFF-CPU 统计信息
type OffCPUStats struct {
	TotalEvents     uint64            // 总事件数
	TotalDuration   int64             // 总阻塞时长 (微秒)
	AvgDuration     float64           // 平均阻塞时长
	MaxDuration     int64             // 最大阻塞时长
	MinDuration     int64             // 最小阻塞时长

	// 按原因分类统计
	ReasonCounts    map[OffCPUReason]uint64  // 各原因事件数
	ReasonDurations map[OffCPUReason]int64   // 各原因总时长

	// 按进程统计
	ProcessStats    map[uint32]*ProcessOffCPUStats // 进程级统计
}

// ProcessOffCPUStats 进程级 OFF-CPU 统计
type ProcessOffCPUStats struct {
	PID           uint32
	ProcessName   string
	EventCount    uint64
	TotalDuration int64
	ReasonCounts  map[OffCPUReason]uint64
}

// ==================== OFF-CPU 剖析器配置 ====================

// OffCPUConfig OFF-CPU 剖析器配置
type OffCPUConfig struct {
	// 目标选择
	PID         uint32   // 目标进程 ID，0 表示所有进程
	PIDs        []uint32 // 多个目标进程
	ProcessName string   // 进程名匹配

	// 采集配置
	MinDuration   int64  // 最小采集时长 (微秒)，低于此值的阻塞不采集
	MaxDuration   int64  // 最大采集时长 (微秒)，高于此值的阻塞截断
	MaxStackDepth int    // 最大栈深度

	// 事件筛选
	CollectIOWait      bool // 采集 IO 等待
	CollectLockContention bool // 采集锁竞争
	CollectScheduler   bool // 采集调度延迟
	CollectNetwork     bool // 采集网络等待
	CollectDisk        bool // 采集磁盘等待
	CollectFutex       bool // 采集 futex 等待
	CollectSleep       bool // 采集主动睡眠

	// 输出配置
	IncludeKernelStack bool // 是否包含内核栈
	IncludeUserStack   bool // 是否包含用户栈
}

// DefaultOffCPUConfig 返回默认配置
func DefaultOffCPUConfig() *OffCPUConfig {
	return &OffCPUConfig{
		MinDuration:        1000,      // 默认只采集超过 1ms 的阻塞
		MaxDuration:        10000000,  // 最大 10s
		MaxStackDepth:      127,
		CollectIOWait:      true,
		CollectLockContention: true,
		CollectScheduler:   true,
		CollectNetwork:     true,
		CollectDisk:        true,
		CollectFutex:       true,
		CollectSleep:       false,     // 默认不采集主动睡眠
		IncludeKernelStack: true,
		IncludeUserStack:   true,
	}
}

// ==================== OFF-CPU 剖析器 ====================

// OffCPUProfiler OFF-CPU 剖析器
// 基于 Linux tracepoint/sched:sched_switch 事件实现
// 通过追踪上下文切换来统计线程在 CPU 外的时间
type OffCPUProfiler struct {
	config      *OffCPUConfig
	symbolizer  *Symbolizer
	events      []*OffCPUEvent
	eventChan   chan *OffCPUEvent
	mu          sync.RWMutex
	running     bool
	stopCh      chan struct{}
	doneCh      chan struct{}
	stats       OffCPUStats

	// 线程状态追踪 (用于关联 sched_switch 事件)
	threadStates map[threadKey]*threadState
	stateMu      sync.RWMutex
}

// threadKey 线程标识
type threadKey struct {
	PID uint32
	TID uint32
}

// threadState 线程状态
type threadState struct {
	PID         uint32
	TID         uint32
	ProcessName string
	ThreadName  string
	OffCPUTime  int64
	StackTrace  []string
	KernelStack []string
}

// NewOffCPUProfiler 创建 OFF-CPU 剖析器
func NewOffCPUProfiler(cfg *OffCPUConfig) (*OffCPUProfiler, error) {
	if cfg == nil {
		cfg = DefaultOffCPUConfig()
	}

	return &OffCPUProfiler{
		config:       cfg,
		symbolizer:   NewSymbolizer(),
		events:       make([]*OffCPUEvent, 0),
		eventChan:    make(chan *OffCPUEvent, 10000),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		threadStates: make(map[threadKey]*threadState),
		stats: OffCPUStats{
			ReasonCounts:    make(map[OffCPUReason]uint64),
			ReasonDurations: make(map[OffCPUReason]int64),
			ProcessStats:    make(map[uint32]*ProcessOffCPUStats),
		},
	}, nil
}

// Start 启动 OFF-CPU 采集
func (p *OffCPUProfiler) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return fmt.Errorf("OFF-CPU 剖析器已在运行")
	}

	p.running = true

	// 启动事件处理 goroutine
	go p.eventLoop()

	// 启动采集 goroutine
	go p.collectLoop()

	return nil
}

// Stop 停止 OFF-CPU 采集
func (p *OffCPUProfiler) Stop() error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return fmt.Errorf("OFF-CPU 剖析器未在运行")
	}
	p.running = false
	p.mu.Unlock()

	close(p.stopCh)
	<-p.doneCh

	return nil
}

// collectLoop 采集循环
// 通过读取 /sys/kernel/debug/tracing/trace_pipe 或 perf_event 获取 sched_switch 事件
func (p *OffCPUProfiler) collectLoop() {
	defer close(p.doneCh)

	// 实际实现中，这里应该：
	// 1. 设置 tracepoint/sched:sched_switch 事件
	// 2. 读取 trace_pipe 或使用 perf_event_open
	// 3. 解析事件并发送到 eventChan

	// 简化实现：模拟采集
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			// 模拟读取事件
			// 实际实现中这里会读取 trace_pipe
		}
	}
}

// eventLoop 事件处理循环
func (p *OffCPUProfiler) eventLoop() {
	for {
		select {
		case <-p.stopCh:
			return
		case event := <-p.eventChan:
			p.processEvent(event)
		}
	}
}

// processEvent 处理单个 OFF-CPU 事件
func (p *OffCPUProfiler) processEvent(event *OffCPUEvent) {
	// 过滤事件
	if !p.shouldCollect(event) {
		return
	}

	// 分类阻塞原因
	if event.Reason == OffCPUReasonUnknown {
		event.Reason = p.classifyEvent(event)
	}

	// 保存事件
	p.mu.Lock()
	p.events = append(p.events, event)
	p.mu.Unlock()

	// 更新统计
	p.updateStats(event)
}

// shouldCollect 判断是否应该采集该事件
func (p *OffCPUProfiler) shouldCollect(event *OffCPUEvent) bool {
	// 时长过滤
	if event.Duration < p.config.MinDuration {
		return false
	}
	if p.config.MaxDuration > 0 && event.Duration > p.config.MaxDuration {
		return false
	}

	// PID 过滤
	if p.config.PID > 0 && event.PID != p.config.PID {
		return false
	}
	if len(p.config.PIDs) > 0 {
		found := false
		for _, pid := range p.config.PIDs {
			if pid == event.PID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 原因过滤
	switch event.Reason {
	case OffCPUReasonIOWait:
		return p.config.CollectIOWait
	case OffCPUReasonLockContention:
		return p.config.CollectLockContention
	case OffCPUReasonScheduler:
		return p.config.CollectScheduler
	case OffCPUReasonNetwork:
		return p.config.CollectNetwork
	case OffCPUReasonDisk:
		return p.config.CollectDisk
	case OffCPUReasonFutex:
		return p.config.CollectFutex
	case OffCPUReasonSleep:
		return p.config.CollectSleep
	}

	return true
}

// classifyEvent 分类阻塞原因
func (p *OffCPUProfiler) classifyEvent(event *OffCPUEvent) OffCPUReason {
	// 根据系统调用名分类
	switch event.Syscall {
	case "read", "readv", "pread64", "preadv", "preadv2":
		return OffCPUReasonIOWait
	case "write", "writev", "pwrite64", "pwritev", "pwritev2":
		return OffCPUReasonIOWait
	case "recvfrom", "recvmsg", "recvmmsg":
		return OffCPUReasonNetwork
	case "sendto", "sendmsg", "sendmmsg":
		return OffCPUReasonNetwork
	case "futex":
		return OffCPUReasonFutex
	case "epoll_wait", "epoll_pwait", "poll", "ppoll", "select", "pselect6":
		return OffCPUReasonPoll
	case "nanosleep", "clock_nanosleep":
		return OffCPUReasonSleep
	case "open", "openat", "creat":
		return OffCPUReasonDisk
	case "fsync", "fdatasync", "sync", "syncfs":
		return OffCPUReasonDisk
	}

	// 根据等待通道分类
	if event.WaitChannel != "" {
		if contains(event.WaitChannel, "wait") {
			return OffCPUReasonScheduler
		}
		if contains(event.WaitChannel, "lock") || contains(event.WaitChannel, "mutex") {
			return OffCPUReasonLockContention
		}
	}

	// 根据栈信息分类
	for _, frame := range event.StackTrace {
		if contains(frame, "mutex") || contains(frame, "lock") {
			return OffCPUReasonLockContention
		}
		if contains(frame, "io") || contains(frame, "read") || contains(frame, "write") {
			return OffCPUReasonIOWait
		}
		if contains(frame, "net") || contains(frame, "socket") {
			return OffCPUReasonNetwork
		}
	}

	return OffCPUReasonUnknown
}

// updateStats 更新统计信息
func (p *OffCPUProfiler) updateStats(event *OffCPUEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stats.TotalEvents++
	p.stats.TotalDuration += event.Duration

	if event.Duration > p.stats.MaxDuration {
		p.stats.MaxDuration = event.Duration
	}
	if p.stats.MinDuration == 0 || event.Duration < p.stats.MinDuration {
		p.stats.MinDuration = event.Duration
	}

	p.stats.AvgDuration = float64(p.stats.TotalDuration) / float64(p.stats.TotalEvents)

	// 按原因统计
	p.stats.ReasonCounts[event.Reason]++
	p.stats.ReasonDurations[event.Reason] += event.Duration

	// 按进程统计
	if _, ok := p.stats.ProcessStats[event.PID]; !ok {
		p.stats.ProcessStats[event.PID] = &ProcessOffCPUStats{
			PID:          event.PID,
			ProcessName:  event.ProcessName,
			ReasonCounts: make(map[OffCPUReason]uint64),
		}
	}
	procStats := p.stats.ProcessStats[event.PID]
	procStats.EventCount++
	procStats.TotalDuration += event.Duration
	procStats.ReasonCounts[event.Reason]++
}

// GetEvents 获取所有事件
func (p *OffCPUProfiler) GetEvents() []*OffCPUEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	events := make([]*OffCPUEvent, len(p.events))
	copy(events, p.events)
	return events
}

// GetStats 获取统计信息
func (p *OffCPUProfiler) GetStats() OffCPUStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats
}

// GetEventsByReason 按原因获取事件
func (p *OffCPUProfiler) GetEventsByReason(reason OffCPUReason) []*OffCPUEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*OffCPUEvent
	for _, event := range p.events {
		if event.Reason == reason {
			result = append(result, event)
		}
	}
	return result
}

// GetEventsByPID 按 PID 获取事件
func (p *OffCPUProfiler) GetEventsByPID(pid uint32) []*OffCPUEvent {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*OffCPUEvent
	for _, event := range p.events {
		if event.PID == pid {
			result = append(result, event)
		}
	}
	return result
}

// Clear 清除所有数据
func (p *OffCPUProfiler) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = p.events[:0]
	p.stats = OffCPUStats{
		ReasonCounts:    make(map[OffCPUReason]uint64),
		ReasonDurations: make(map[OffCPUReason]int64),
		ProcessStats:    make(map[uint32]*ProcessOffCPUStats),
	}
}

// Close 关闭剖析器
func (p *OffCPUProfiler) Close() error {
	if p.running {
		p.Stop()
	}
	p.symbolizer.ClearCache()
	return nil
}

// ==================== 辅助函数 ====================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
