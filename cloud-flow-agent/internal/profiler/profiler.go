// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件实现 ON-CPU 剖析器主模块，整合 perf_event 采样、符号解析和火焰图生成
package profiler

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ==================== 配置与结果结构体 ====================

// ProfilerConfig 剖析器配置参数
// 用于控制采样行为和目标选择
type ProfilerConfig struct {
	SampleFreq    int  // 采样频率 (Hz)，默认 99，范围 1-100000
	TargetPID     uint32 // 目标进程 ID，0 表示监控所有进程
	MaxStackDepth int  // 最大栈深度，默认 127
}

// ProfileResult 剖析结果
// 包含火焰图 SVG 数据、热点函数列表和统计信息
type ProfileResult struct {
	FlameGraphSVG []byte        // 火焰图 SVG 字节数据，可直接写入 .svg 文件
	HotFunctions  []HotFunction // 热点函数列表，按采样次数降序排列
	Stats         ProfilerStats // 采样统计信息
}

// ProfilerStats 采样统计信息
// 记录剖析过程中的关键指标
type ProfilerStats struct {
	TotalSamples uint64        // 总采样数 (成功采集的样本)
	LostSamples  uint64        // 丢失采样数 (ring buffer 溢出等原因)
	Duration     time.Duration // 采样持续时间
	SampleFreq   int           // 实际采样频率 (Hz)
}

// ==================== Profiler 主结构体 ====================

// Profiler ON-CPU 剖析器
// 基于 Linux perf_event 子系统实现 CPU 采样
// 支持多进程监控、动态频率调整和实时符号解析
//
// 使用流程:
//  1. New(cfg) 创建剖析器
//  2. Start() 启动采样
//  3. Collect(duration) 或手动读取数据
//  4. GenerateFlameGraph() / GetHotFunctions() 获取结果
//  5. Stop() 停止采样
type Profiler struct {
	symbolizer  *Symbolizer  // 多语言符号解析器
	flameGraph  *FlameGraph  // 火焰图生成器
	perfEventFD int          // perf_event 文件描述符
	ringBuffer  *perfRingBuffer // perf ring buffer (用于读取采样数据)
	enabled     bool         // 是否正在采样
	sampleFreq  int          // 当前采样频率 (Hz)
	targetPID   uint32       // 目标进程 ID
	stackCounts map[string]uint64 // 栈合并计数 map (key=栈帧链, value=采样次数)
	mu          sync.Mutex   // 互斥锁，保护共享状态
	stopCh      chan struct{} // 停止信号通道
	doneCh      chan struct{} // 采样 goroutine 完成信号
	maxStackDepth int         // 最大栈深度
	stats       ProfilerStats // 采样统计信息
}

// ==================== 构造函数 ====================

// New 创建一个新的 ON-CPU 剖析器
// 参数:
//   - cfg: 剖析器配置
//
// 初始化流程:
//  1. 验证配置参数
//  2. 创建符号解析器和火焰图生成器
//  3. 使用 perf_event_open 系统调用创建采样事件
//  4. 使用 mmap 映射 perf_event ring buffer
//  5. 初始化采样 goroutine (但不启动)
//
// 返回: 剖析器实例或错误
func New(cfg ProfilerConfig) (*Profiler, error) {
	// 验证并设置默认配置
	if cfg.SampleFreq <= 0 {
		cfg.SampleFreq = 99 // 默认 99Hz，与 perf 默认值一致
	}
	if cfg.SampleFreq > 100000 {
		return nil, fmt.Errorf("采样频率 %d 超出范围 (1-100000)", cfg.SampleFreq)
	}
	if cfg.MaxStackDepth <= 0 {
		cfg.MaxStackDepth = 127 // 默认最大栈深度
	}

	// 创建符号解析器
	symbolizer := NewSymbolizer()

	// 创建火焰图生成器
	flameGraph := NewFlameGraph()

	// 确定 perf_event_open 的 pid 参数
	// pid = -1: 监控所有进程
	// pid = 0: 监控当前进程
	// pid > 0: 监控指定进程
	perfPID := -1
	if cfg.TargetPID > 0 {
		perfPID = int(cfg.TargetPID)
	}

	// 打开 perf 事件
	// cpu = -1: 监控所有 CPU
	// 使用 PERF_TYPE_SOFTWARE + PERF_COUNT_SW_CPU_CLOCK 以获得最佳兼容性
	fd, err := OpenPerfEvent(-1, uint64(cfg.SampleFreq), perfPID)
	if err != nil {
		return nil, fmt.Errorf("打开 perf event 失败: %w", err)
	}

	// 创建 ring buffer 用于读取采样数据
	ringBuf, err := NewPerfRingBuffer(fd)
	if err != nil {
		// ring buffer 创建失败，关闭 perf event
		ClosePerfEvent(fd)
		return nil, fmt.Errorf("创建 ring buffer 失败: %w", err)
	}

	p := &Profiler{
		symbolizer:    symbolizer,
		flameGraph:    flameGraph,
		perfEventFD:   fd,
		ringBuffer:    ringBuf,
		enabled:       false,
		sampleFreq:    cfg.SampleFreq,
		targetPID:     cfg.TargetPID,
		stackCounts:   make(map[string]uint64),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		maxStackDepth: cfg.MaxStackDepth,
		stats: ProfilerStats{
			SampleFreq: cfg.SampleFreq,
		},
	}

	return p, nil
}

// ==================== 采样控制 ====================

// Start 启动 CPU 采样
// 启用 perf 事件并开始后台 goroutine 读取采样数据
func (p *Profiler) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.enabled {
		return fmt.Errorf("剖析器已在运行中")
	}

	// 启用 perf 事件
	if err := EnablePerfEvent(p.perfEventFD); err != nil {
		return fmt.Errorf("启用 perf event 失败: %w", err)
	}

	p.enabled = true
	p.stats.Duration = 0

	// 启动后台 goroutine 读取采样数据
	go p.sampleLoop()

	return nil
}

// Stop 停止 CPU 采样
// 禁用 perf 事件并等待后台 goroutine 退出
func (p *Profiler) Stop() error {
	p.mu.Lock()

	if !p.enabled {
		p.mu.Unlock()
		return fmt.Errorf("剖析器未在运行")
	}

	// 禁用 perf 事件
	if err := DisablePerfEvent(p.perfEventFD); err != nil {
		p.mu.Unlock()
		return fmt.Errorf("禁用 perf event 失败: %w", err)
	}

	p.enabled = false
	p.mu.Unlock()

	// 发送停止信号
	close(p.stopCh)

	// 等待采样 goroutine 退出
	<-p.doneCh

	// 重新创建停止通道 (允许再次启动)
	p.stopCh = make(chan struct{})
	p.doneCh = make(chan struct{})

	return nil
}

// SetSampleFreq 动态调整采样频率
// 注意: 调整频率会先停止当前采样，重新配置后再启动
func (p *Profiler) SetSampleFreq(freq int) error {
	if freq <= 0 || freq > 100000 {
		return fmt.Errorf("采样频率 %d 超出范围 (1-100000)", freq)
	}

	p.mu.Lock()
	wasRunning := p.enabled
	p.mu.Unlock()

	// 如果正在运行，先停止
	if wasRunning {
		if err := p.Stop(); err != nil {
			return fmt.Errorf("停止剖析器失败: %w", err)
		}
	}

	// 更新采样频率
	p.mu.Lock()
	p.sampleFreq = freq
	p.stats.SampleFreq = freq
	p.mu.Unlock()

	// 如果之前在运行，重新启动
	if wasRunning {
		if err := p.Start(); err != nil {
			return fmt.Errorf("重新启动剖析器失败: %w", err)
		}
	}

	return nil
}

// SetTargetPID 设置目标进程 ID
// 参数:
//   - pid: 目标进程 ID，0 表示监控所有进程
//
// 注意: 更改目标 PID 需要重新创建 perf event，会中断当前采样
func (p *Profiler) SetTargetPID(pid uint32) error {
	p.mu.Lock()
	wasRunning := p.enabled
	p.mu.Unlock()

	// 如果正在运行，先停止
	if wasRunning {
		if err := p.Stop(); err != nil {
			return fmt.Errorf("停止剖析器失败: %w", err)
		}
	}

	// 关闭旧的 perf event
	if p.perfEventFD >= 0 {
		if p.ringBuffer != nil {
			p.ringBuffer.Close()
			p.ringBuffer = nil
		}
		ClosePerfEvent(p.perfEventFD)
		p.perfEventFD = -1
	}

	// 更新目标 PID
	p.mu.Lock()
	p.targetPID = pid
	p.mu.Unlock()

	// 重新创建 perf event
	perfPID := -1
	if pid > 0 {
		perfPID = int(pid)
	}

	fd, err := OpenPerfEvent(-1, uint64(p.sampleFreq), perfPID)
	if err != nil {
		return fmt.Errorf("重新打开 perf event 失败: %w", err)
	}

	ringBuf, err := NewPerfRingBuffer(fd)
	if err != nil {
		ClosePerfEvent(fd)
		return fmt.Errorf("重新创建 ring buffer 失败: %w", err)
	}

	p.mu.Lock()
	p.perfEventFD = fd
	p.ringBuffer = ringBuf
	p.mu.Unlock()

	// 清除符号解析器缓存 (目标进程已变更)
	p.symbolizer.ClearCache()

	// 如果之前在运行，重新启动
	if wasRunning {
		if err := p.Start(); err != nil {
			return fmt.Errorf("重新启动剖析器失败: %w", err)
		}
	}

	return nil
}

// ==================== 数据采集 ====================

// Collect 采集指定时间的数据
// 启动采样 -> 等待指定时间 -> 停止采样 -> 返回结果
// 参数:
//   - duration: 采集持续时间
//
// 返回: 剖析结果 (火焰图 SVG + 热点函数 + 统计信息)
func (p *Profiler) Collect(duration time.Duration) (*ProfileResult, error) {
	// 清除之前的采样数据
	p.mu.Lock()
	p.stackCounts = make(map[string]uint64)
	p.stats = ProfilerStats{
		SampleFreq: p.sampleFreq,
	}
	p.mu.Unlock()

	// 启动采样
	if err := p.Start(); err != nil {
		return nil, fmt.Errorf("启动剖析器失败: %w", err)
	}

	// 等待指定时间
	time.Sleep(duration)

	// 停止采样
	if err := p.Stop(); err != nil {
		return nil, fmt.Errorf("停止剖析器失败: %w", err)
	}

	// 更新统计信息中的持续时间
	p.mu.Lock()
	p.stats.Duration = duration
	p.mu.Unlock()

	// 生成火焰图
	flameGraphSVG, err := p.GenerateFlameGraph()
	if err != nil {
		return nil, fmt.Errorf("生成火焰图失败: %w", err)
	}

	// 获取热点函数
	hotFunctions := p.GetHotFunctions(20) // 默认返回 Top 20

	// 获取统计信息
	stats := p.GetStats()

	return &ProfileResult{
		FlameGraphSVG: flameGraphSVG,
		HotFunctions:  hotFunctions,
		Stats:         stats,
	}, nil
}

// ==================== 结果生成 ====================

// GenerateFlameGraph 生成火焰图 SVG 数据
// 基于当前采集的栈采样数据生成 SVG 格式的火焰图
// 返回: SVG 字节数据，可直接写入 .svg 文件或在浏览器中显示
func (p *Profiler) GenerateFlameGraph() ([]byte, error) {
	p.mu.Lock()
	counts := p.copyStackCounts()
	p.mu.Unlock()

	if len(counts) == 0 {
		return nil, fmt.Errorf("没有采样数据，无法生成火焰图")
	}

	// 使用 bytes.Buffer 作为 SVG 输出目标
	var buf bytes.Buffer
	err := p.flameGraph.Generate(counts, &buf)
	if err != nil {
		return nil, fmt.Errorf("生成火焰图失败: %w", err)
	}

	return buf.Bytes(), nil
}

// GetHotFunctions 获取热点函数列表
// 参数:
//   - topN: 返回前 N 个热点函数，0 表示返回全部
//
// 返回: 热点函数列表，按采样次数降序排列
func (p *Profiler) GetHotFunctions(topN int) []HotFunction {
	p.mu.Lock()
	counts := p.copyStackCounts()
	p.mu.Unlock()

	return p.flameGraph.GenerateHotFunctions(counts, topN)
}

// GetStats 获取采样统计信息
// 返回: 当前统计信息的副本
func (p *Profiler) GetStats() ProfilerStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// ==================== 采样循环 ====================

// sampleLoop 采样循环 goroutine
// 持续从 perf ring buffer 中读取采样数据并进行符号解析
// 直到收到停止信号或发生错误
func (p *Profiler) sampleLoop() {
	defer close(p.doneCh)

	// 采样循环
	ticker := time.NewTicker(100 * time.Millisecond) // 每 100ms 检查一次 ring buffer
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			// 收到停止信号，退出循环
			// 最后再读取一次 ring buffer 中的剩余数据
			p.drainRingBuffer()
			return

		case <-ticker.C:
			// 定期读取 ring buffer 中的采样数据
			p.drainRingBuffer()
		}
	}
}

// drainRingBuffer 读取 ring buffer 中所有可用的采样数据
// 解析每个采样的调用链，进行符号解析，并更新栈计数
func (p *Profiler) drainRingBuffer() {
	if p.ringBuffer == nil {
		return
	}

	// 读取 ring buffer 中的所有可用事件
	err := p.ringBuffer.ReadAvailable(func(raw []byte) {
		// 解析采样事件
		cpu, _, pid, _, callchain, _ := parseSampleEvent(raw)

		// 如果设置了目标 PID，过滤非目标进程的采样
		if p.targetPID > 0 && pid != p.targetPID {
			return
		}

		// 更新统计信息
		p.mu.Lock()
		p.stats.TotalSamples++

		// 解析调用链并构建栈帧链字符串
		if len(callchain) > 0 {
			stackStr := p.resolveCallchain(callchain, pid)
			if stackStr != "" {
				p.stackCounts[stackStr]++
			}
		}
		p.mu.Unlock()

		// cpu 参数可用于按 CPU 统计 (预留)
		_ = cpu
	})

	if err != nil {
		// ring buffer 读取错误，记录但不中断采样
		p.mu.Lock()
		p.stats.LostSamples++
		p.mu.Unlock()
	}
}

// resolveCallchain 解析调用链中的地址为函数名
// 将地址数组转换为分号分隔的栈帧链字符串
// 例如: "main;foo;bar" (从调用者到被调用者)
func (p *Profiler) resolveCallchain(callchain []uint64, pid uint32) string {
	if len(callchain) == 0 {
		return ""
	}

	// 限制栈深度
	maxDepth := p.maxStackDepth
	if len(callchain) > maxDepth {
		callchain = callchain[:maxDepth]
	}

	// 解析每个地址为函数名
	var frames []string
	for _, addr := range callchain {
		// 跳过内核地址 (高位地址)
		if addr >= 0xFFFF800000000000 {
			continue
		}

		// 跳过零地址
		if addr == 0 {
			continue
		}

		// 使用符号解析器解析地址
		name, _, _ := p.symbolizer.Resolve(addr, pid)
		if name != "" {
			frames = append(frames, name)
		}
	}

	if len(frames) == 0 {
		return ""
	}

	// 反转栈帧顺序 (调用链是从最新帧到最旧帧，火焰图需要从最旧到最新)
	// perf 的 callchain[0] 是当前 IP (最新帧)，callchain[n] 是最旧帧
	for i, j := 0, len(frames)-1; i < j; i, j = i+1, j-1 {
		frames[i], frames[j] = frames[j], frames[i]
	}

	return joinFrames(frames)
}

// ==================== 辅助函数 ====================

// copyStackCounts 复制栈计数 map
// 返回当前栈计数的深拷贝，避免外部修改影响内部状态
func (p *Profiler) copyStackCounts() map[string]uint64 {
	counts := make(map[string]uint64, len(p.stackCounts))
	for k, v := range p.stackCounts {
		counts[k] = v
	}
	return counts
}

// joinFrames 将栈帧数组连接为分号分隔的字符串
// 例如: ["main", "foo", "bar"] -> "main;foo;bar"
func joinFrames(frames []string) string {
	if len(frames) == 0 {
		return ""
	}
	var buf strings.Builder
	for i, frame := range frames {
		if i > 0 {
			buf.WriteByte(';')
		}
		buf.WriteString(frame)
	}
	return buf.String()
}

// ==================== 资源清理 ====================

// Close 关闭剖析器并释放所有资源
// 必须在使用完毕后调用，防止资源泄漏
func (p *Profiler) Close() error {
	// 如果正在运行，先停止
	if p.enabled {
		_ = p.Stop()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	// 关闭 ring buffer
	if p.ringBuffer != nil {
		if err := p.ringBuffer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("关闭 ring buffer 失败: %w", err))
		}
		p.ringBuffer = nil
	}

	// 关闭 perf event
	if p.perfEventFD >= 0 {
		if err := ClosePerfEvent(p.perfEventFD); err != nil {
			errs = append(errs, fmt.Errorf("关闭 perf event 失败: %w", err))
		}
		p.perfEventFD = -1
	}

	// 清除符号解析器缓存
	p.symbolizer.ClearCache()

	// 清除栈计数
	p.stackCounts = nil

	if len(errs) > 0 {
		return fmt.Errorf("关闭剖析器时发生 %d 个错误: %v", len(errs), errs)
	}

	return nil
}

// IsRunning 返回剖析器是否正在运行
func (p *Profiler) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled
}

// GetTargetPID 返回当前目标进程 ID
func (p *Profiler) GetTargetPID() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.targetPID
}

// GetSampleFreq 返回当前采样频率
func (p *Profiler) GetSampleFreq() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sampleFreq
}

// Reset 清除所有采样数据，重置统计信息
// 剖析器状态 (运行/停止) 不受影响
func (p *Profiler) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stackCounts = make(map[string]uint64)
	p.stats = ProfilerStats{
		SampleFreq: p.sampleFreq,
	}

	// 清除符号解析器缓存
	p.symbolizer.ClearCache()
}

// GetStackCounts 返回当前栈计数的副本
// 用于外部分析或持久化
func (p *Profiler) GetStackCounts() map[string]uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.copyStackCounts()
}
