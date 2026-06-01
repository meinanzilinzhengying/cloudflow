// Package circuitbreaker 提供基于资源使用率的过载熔断功能
//
// 三级状态机：
//
//	Normal（正常采集）
//	  ├─ CPU连续30秒>80% 或 内存>90% ──→ Degraded（降级采集：仅核心指标）
//	  └─ CPU>95% 或 内存>95% ───────────→ Silent（完全静默）
//
//	Degraded（降级采集）
//	  ├─ CPU>95% 或 内存>95% ───────────→ Silent（完全静默）
//	  └─ CPU≤80% 且 内存≤85% ──────────→ Normal（恢复全量采集）
//
//	Silent（完全静默）
//	  └─ CPU≤70% 且 内存≤80% ──────────→ Normal（恢复全量采集）
//
// 采集分级：
//   - 核心指标（Degraded仍保留）：基础流量(TC) + TCP深度指标
//   - 非核心指标（Degraded时停止）：HTTP/DNS/MySQL全字段解析 + SQL聚合 + 传统采集(CPU/Memory/Disk)
package circuitbreaker

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// State 熔断器状态
type State int

const (
	// StateNormal 正常状态：全量采集
	StateNormal State = iota
	// StateDegraded 降级状态：仅核心指标采集
	StateDegraded
	// StateSilent 静默状态：完全停止采集
	StateSilent
)

func (s State) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StateDegraded:
		return "degraded"
	case StateSilent:
		return "silent"
	default:
		return "unknown"
	}
}

// Config 过载熔断器配置
type Config struct {
	// 检查间隔（默认3秒）
	CheckInterval time.Duration

	// CPU降级阈值：连续超过此百分比触发降级（默认80）
	CPUDegradedThreshold float64

	// CPU静默阈值：超过此百分比触发完全静默（默认95）
	CPUSilentThreshold float64

	// 内存降级阈值：超过此百分比触发降级（默认90）
	MemDegradedThreshold float64

	// 内存静默阈值：超过此百分比触发完全静默（默认95）
	MemSilentThreshold float64

	// CPU持续超限秒数才触发降级（默认30秒）
	CPUDegradedDuration time.Duration

	// 内存恢复阈值（低于此值才从Degraded恢复，默认85，避免频繁切换）
	MemRecoverThreshold float64

	// CPU恢复阈值（低于此值才从Degraded恢复，默认80）
	CPURecoverThreshold float64

	// 静默恢复CPU阈值（默认70，需更低才从Silent恢复）
	SilentCPURecoverThreshold float64

	// 静默恢复内存阈值（默认80）
	SilentMemRecoverThreshold float64

	// 内存限制MB（用于计算百分比，默认1024）
	MaxMemoryMB float64

	// CPU核心数（用于计算百分比，默认1.0）
	MaxCPUCores float64
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		CheckInterval:             3 * time.Second,
		CPUDegradedThreshold:      80.0,
		CPUSilentThreshold:        95.0,
		MemDegradedThreshold:      90.0,
		MemSilentThreshold:        95.0,
		CPUDegradedDuration:       30 * time.Second,
		MemRecoverThreshold:       85.0,
		CPURecoverThreshold:       80.0,
		SilentCPURecoverThreshold: 70.0,
		SilentMemRecoverThreshold: 80.0,
		MaxMemoryMB:               1024.0,
		MaxCPUCores:               1.0,
	}
}

// ResourceSnapshot 资源使用快照
type ResourceSnapshot struct {
	Timestamp  time.Time
	CPUPercent float64 // CPU使用率百分比 (0-100)
	MemPercent float64 // 内存使用率百分比 (0-100)
	MemUsedMB  float64 // 内存使用量MB
	Goroutines int     // 协程数
}

// StateChangeCallback 状态变更回调函数
// 参数: fromState, toState, snapshot
type StateChangeCallback func(from, to State, snapshot ResourceSnapshot)

// Breaker 过载熔断器
type Breaker struct {
	cfg Config
	mu  sync.RWMutex

	// 当前状态
	state State

	// CPU持续超限追踪
	cpuHighSince time.Time // CPU首次超过降级阈值的时间
	cpuHighCount int       // 连续超限检查次数

	// 上一次CPU统计（用于计算使用率）
	prevCPUIdle    uint64
	prevCPUTotal   uint64
	cpuInitialized bool

	// 生命周期
	stopCh chan struct{}

	// 回调
	onStateChange StateChangeCallback

	// 最近一次资源快照（线程安全读取）
	latestSnapshot ResourceSnapshot
}

// NewBreaker 创建过载熔断器
func NewBreaker(cfg Config) *Breaker {
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 3 * time.Second
	}
	if cfg.CPUDegradedDuration <= 0 {
		cfg.CPUDegradedDuration = 30 * time.Second
	}
	return &Breaker{
		cfg:    cfg,
		state:  StateNormal,
		stopCh: make(chan struct{}),
	}
}

// OnStateChange 设置状态变更回调
func (b *Breaker) OnStateChange(cb StateChangeCallback) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onStateChange = cb
}

// Start 启动熔断器监控循环
func (b *Breaker) Start() {
	go b.monitorLoop()
}

// Stop 停止熔断器
func (b *Breaker) Stop() {
	close(b.stopCh)
}

// State 获取当前熔断状态
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// IsDegraded 是否处于降级状态
func (b *Breaker) IsDegraded() bool {
	return b.State() == StateDegraded
}

// IsSilent 是否处于静默状态
func (b *Breaker) IsSilent() bool {
	return b.State() == StateSilent
}

// IsNormal 是否处于正常状态
func (b *Breaker) IsNormal() bool {
	return b.State() == StateNormal
}

// LatestSnapshot 获取最新资源快照
func (b *Breaker) LatestSnapshot() ResourceSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.latestSnapshot
}

// CollectLevel 返回当前应执行的采集级别描述
func (b *Breaker) CollectLevel() string {
	switch b.State() {
	case StateNormal:
		return "full"
	case StateDegraded:
		return "core_only"
	case StateSilent:
		return "none"
	default:
		return "unknown"
	}
}

// ShouldCollectCore 是否应采集核心指标（基础流量+TCP）
// 在 Normal 和 Degraded 状态下返回 true
func (b *Breaker) ShouldCollectCore() bool {
	s := b.State()
	return s == StateNormal || s == StateDegraded
}

// ShouldCollectExtended 是否应采集扩展指标（HTTP/DNS/MySQL全字段+SQL聚合+传统采集）
// 仅在 Normal 状态下返回 true
func (b *Breaker) ShouldCollectExtended() bool {
	return b.State() == StateNormal
}

// monitorLoop 监控循环
func (b *Breaker) monitorLoop() {
	ticker := time.NewTicker(b.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.check()
		case <-b.stopCh:
			return
		}
	}
}

// check 执行一次资源检查和状态转换
func (b *Breaker) check() {
	snapshot := b.collectResourceSnapshot()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.latestSnapshot = snapshot
	prevState := b.state

	switch b.state {
	case StateNormal:
		b.checkFromNormal(snapshot)
	case StateDegraded:
		b.checkFromDegraded(snapshot)
	case StateSilent:
		b.checkFromSilent(snapshot)
	}

	// 状态发生变化时触发回调
	if b.state != prevState && b.onStateChange != nil {
		cb := b.onStateChange // 避免锁内回调导致死锁
		go cb(prevState, b.state, snapshot)
	}
}

// checkFromNormal 从正常状态检查
func (b *Breaker) checkFromNormal(s ResourceSnapshot) {
	// 检查是否需要进入静默（优先级最高）
	if s.CPUPercent >= b.cfg.CPUSilentThreshold || s.MemPercent >= b.cfg.MemSilentThreshold {
		b.transitionTo(StateSilent, s)
		return
	}

	// 检查CPU是否持续超限
	if s.CPUPercent >= b.cfg.CPUDegradedThreshold {
		if b.cpuHighSince.IsZero() {
			b.cpuHighSince = time.Now()
		}
		b.cpuHighCount++

		// CPU连续超限超过阈值时间
		if time.Since(b.cpuHighSince) >= b.cfg.CPUDegradedDuration {
			b.transitionTo(StateDegraded, s)
			return
		}
	} else {
		// CPU恢复正常，重置计时
		b.cpuHighSince = time.Time{}
		b.cpuHighCount = 0
	}

	// 内存超限直接触发降级（不需要持续时间）
	if s.MemPercent >= b.cfg.MemDegradedThreshold {
		b.transitionTo(StateDegraded, s)
		return
	}
}

// checkFromDegraded 从降级状态检查
func (b *Breaker) checkFromDegraded(s ResourceSnapshot) {
	// 检查是否需要进入静默
	if s.CPUPercent >= b.cfg.CPUSilentThreshold || s.MemPercent >= b.cfg.MemSilentThreshold {
		b.transitionTo(StateSilent, s)
		return
	}

	// 检查是否可以恢复到正常
	if s.CPUPercent <= b.cfg.CPURecoverThreshold && s.MemPercent <= b.cfg.MemRecoverThreshold {
		b.transitionTo(StateNormal, s)
		return
	}
}

// checkFromSilent 从静默状态检查
func (b *Breaker) checkFromSilent(s ResourceSnapshot) {
	// 需要资源降到更低阈值才恢复（滞后恢复，避免震荡）
	if s.CPUPercent <= b.cfg.SilentCPURecoverThreshold && s.MemPercent <= b.cfg.SilentMemRecoverThreshold {
		b.transitionTo(StateNormal, s)
	}
}

// transitionTo 执行状态转换（调用前必须持有锁）
func (b *Breaker) transitionTo(to State, s ResourceSnapshot) {
	from := b.state
	b.state = to

	// 重置CPU持续超限追踪
	b.cpuHighSince = time.Time{}
	b.cpuHighCount = 0

	// 进入降级或静默时触发GC释放内存
	if to == StateDegraded || to == StateSilent {
		go func() {
			runtime.GC()
			debug.FreeOSMemory()
		}()
	}
}

// collectResourceSnapshot 采集当前资源使用快照
func (b *Breaker) collectResourceSnapshot() ResourceSnapshot {
	now := time.Now()

	// CPU使用率
	cpuPercent := b.getCPUUsagePercent()

	// 内存使用率
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memUsedMB := float64(memStats.Alloc) / 1024 / 1024
	memPercent := 0.0
	if b.cfg.MaxMemoryMB > 0 {
		memPercent = (memUsedMB / b.cfg.MaxMemoryMB) * 100
		if memPercent > 100 {
			memPercent = 100
		}
	}

	return ResourceSnapshot{
		Timestamp:  now,
		CPUPercent: cpuPercent,
		MemPercent: memPercent,
		MemUsedMB:  memUsedMB,
		Goroutines: runtime.NumGoroutine(),
	}
}

// getCPUUsagePercent 获取进程CPU使用率百分比
// 通过读取 /proc/stat 计算系统级CPU使用率
func (b *Breaker) getCPUUsagePercent() float64 {
	// 优先使用 /proc/stat 计算系统CPU使用率
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	var user, nice, system, idle, iowait, irq, softirq, steal uint64
	n, _ := fmt.Sscanf(string(data),
		"cpu %d %d %d %d %d %d %d %d",
		&user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal)
	if n < 4 {
		return 0
	}

	totalIdle := idle + iowait
	total := user + nice + system + idle + iowait + irq + softirq + steal

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.cpuInitialized || b.prevCPUTotal == 0 {
		b.prevCPUIdle = totalIdle
		b.prevCPUTotal = total
		b.cpuInitialized = true
		return 0 // 首次读取无法计算使用率
	}

	deltaIdle := totalIdle - b.prevCPUIdle
	deltaTotal := total - b.prevCPUTotal

	b.prevCPUIdle = totalIdle
	b.prevCPUTotal = total

	if deltaTotal == 0 {
		return 0
	}

	// CPU使用率 = (1 - idle增量/总增量) * 100
	usage := (1.0 - float64(deltaIdle)/float64(deltaTotal)) * 100

	// 如果设置了CPU核心数限制，按比例换算
	// 例如：2核系统限制1核，系统CPU 50% = 进程实际使用 100% of 1 core
	if b.cfg.MaxCPUCores > 0 {
		// 获取系统CPU核心数
		numCPU := runtime.NumCPU()
		if numCPU > 0 {
			// 将系统CPU使用率换算为相对于限制核心数的百分比
			usage = usage * float64(numCPU) / b.cfg.MaxCPUCores
		}
	}

	if usage > 100 {
		usage = 100
	}
	if usage < 0 {
		usage = 0
	}

	return usage
}

// Status 返回熔断器状态摘要
func (b *Breaker) Status() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	s := b.latestSnapshot
	cpuHigh := ""
	if !b.cpuHighSince.IsZero() && b.state == StateNormal {
		elapsed := time.Since(b.cpuHighSince)
		remaining := b.cfg.CPUDegradedDuration - elapsed
		if remaining > 0 {
			cpuHigh = fmt.Sprintf(", CPU持续超限%.0fs(%.0fs后降级)", elapsed.Seconds(), remaining.Seconds())
		}
	}

	return fmt.Sprintf("state=%s, cpu=%.1f%%, mem=%.1f%%(%dMB), goroutines=%d%s",
		b.state, s.CPUPercent, s.MemPercent, int(s.MemUsedMB), s.Goroutines, cpuHigh)
}
