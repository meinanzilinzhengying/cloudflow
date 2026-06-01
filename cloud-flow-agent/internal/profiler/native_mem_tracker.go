// Package profiler 提供 Java 堆外内存检测功能
// 本文件实现堆外内存分配追踪器，基于 eBPF hook malloc/free/Unsafe.allocateMemory
// 统计 DirectByteBuffer、JNI 调用分配的堆外内存
package profiler

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ==================== 堆外内存分配类型 ====================

// AllocSource 堆外内存分配来源
type AllocSource string

const (
	AllocSourceDirectByteBuffer AllocSource = "direct_byte_buffer" // ByteBuffer.allocateDirect()
	AllocSourceUnsafe           AllocSource = "unsafe"             // Unsafe.allocateMemory()
	AllocSourceJNI              AllocSource = "jni"               // JNI NewDirectByteBuffer / GetDirectBufferAddress
	AllocSourceMappedByteBuffer AllocSource = "mapped_byte_buffer" // MappedByteBuffer (mmap)
	AllocSourceNativeIO         AllocSource = "native_io"         // Native IO (FileChannel)
	AllocSourceNetty            AllocSource = "netty"             // Netty Pooled/UnpooledByteBufAllocator
	AllocSourceUnknown          AllocSource = "unknown"           // 未知来源
)

// AllocEvent 堆外内存分配事件
type AllocEvent struct {
	Timestamp   int64       `json:"timestamp"`    // 事件时间戳(纳秒)
	PID         uint32      `json:"pid"`          // 进程 ID
	TID         uint32      `json:"tid"`          // 线程 ID
	AllocSize   int64       `json:"alloc_size"`   // 分配大小(字节)
	Address     uint64      `json:"address"`      // 分配地址
	Source      AllocSource `json:"source"`       // 分配来源
	IsAlloc     bool        `json:"is_alloc"`     // true=分配, false=释放
	JNIClass    string      `json:"jni_class"`    // JNI 调用的 Java 类名
	JNIMethod   string      `json:"jni_method"`   // JNI 调用的 Java 方法名
	NativeStack []string    `json:"native_stack"` // 原生调用栈
	JavaStack   []string    `json:"java_stack"`   // 翻译后的 Java 调用栈
}

// MemoryBlock 堆外内存块
// 表示一次 malloc 分配对应的内存区域
type MemoryBlock struct {
	Address     uint64      `json:"address"`      // 内存地址
	Size        int64       `json:"size"`         // 大小(字节)
	AllocTime   time.Time   `json:"alloc_time"`   // 分配时间
	AllocTID    uint32      `json:"alloc_tid"`    // 分配线程 ID
	Source      AllocSource `json:"source"`       // 分配来源
	JavaStack   []string    `json:"java_stack"`   // Java 调用栈
	NativeStack []string    `json:"native_stack"` // 原生调用栈
	JNIClass    string      `json:"jni_class"`    // JNI 类名
	JNIMethod   string      `json:"jni_method"`   // JNI 方法名
	Released    bool        `json:"released"`     // 是否已释放
	ReleaseTime time.Time   `json:"release_time"` // 释放时间
	ReleaseTID  uint32      `json:"release_tid"`  // 释放线程 ID
	Lifetime    int64       `json:"lifetime"`     // 生命周期(纳秒)
}

// ==================== 堆外内存追踪器配置 ====================

// NativeMemConfig 堆外内存追踪器配置
type NativeMemConfig struct {
	PID            uint32        // 目标 Java 进程 ID
	Duration       time.Duration // 追踪时长
	MinBlockSize   int64         // 最小追踪块大小(字节)，默认 256
	MaxBlocks      int           // 最大追踪块数量，默认 100000
	TrackStack     bool          // 是否采集调用栈
	MaxStackDepth  int           // 最大栈深度，默认 64
	SampleRate     float64       // 采样率 (0.0-1.0)，默认 1.0 (全量)
}

// DefaultNativeMemConfig 返回默认配置
func DefaultNativeMemConfig() *NativeMemConfig {
	return &NativeMemConfig{
		MinBlockSize:  256,
		MaxBlocks:     100000,
		TrackStack:    true,
		MaxStackDepth: 64,
		SampleRate:    1.0,
	}
}

// ==================== 堆外内存追踪器 ====================

// NativeMemTracker 堆外内存追踪器
// 通过 hook malloc/free 和 JNI 调用来追踪 Java 堆外内存
type NativeMemTracker struct {
	config     *NativeMemConfig
	javaStack  *JavaStackTranslator // Java 栈翻译器

	mu         sync.RWMutex
	blocks     map[uint64]*MemoryBlock // address -> MemoryBlock
	events     []*AllocEvent           // 所有分配/释放事件

	// 统计计数器
	totalAllocs    int64 // 总分配次数
	totalFrees     int64 // 总释放次数
	totalAllocSize int64 // 总分配大小
	currentSize    int64 // 当前活跃大小
	peakSize       int64 // 峰值大小

	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewNativeMemTracker 创建堆外内存追踪器
func NewNativeMemTracker(cfg *NativeMemConfig) (*NativeMemTracker, error) {
	if cfg == nil {
		cfg = DefaultNativeMemConfig()
	}

	return &NativeMemTracker{
		config:    cfg,
		javaStack: NewJavaStackTranslator(cfg.PID),
		blocks:    make(map[uint64]*MemoryBlock),
		events:    make([]*AllocEvent, 0),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}, nil
}

// Start 启动追踪
func (t *NativeMemTracker) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("追踪器已在运行")
	}
	t.running = true
	t.mu.Unlock()

	go t.collectLoop(ctx)
	go t.eventLoop()

	return nil
}

// Stop 停止追踪
func (t *NativeMemTracker) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	close(t.stopCh)
	t.mu.Unlock()

	<-t.doneCh
}

// collectLoop 采集循环
// 实际实现中通过 eBPF uprobe hook 以下函数:
//   - malloc/calloc/realloc/free
//   - JNI_NewDirectByteBuffer
//   - Java_java_nio_Bits_reserveMemory
//   - Java_sun_misc_Unsafe_allocateMemory
//   - mmap/munmap (for MappedByteBuffer)
func (t *NativeMemTracker) collectLoop(ctx context.Context) {
	defer close(t.doneCh)

	// 实际实现中这里会加载 eBPF 程序并读取 perf event
	// 简化实现：模拟采集循环
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			// 实际实现中从 eBPF ring buffer 读取事件
		}
	}
}

// eventLoop 事件处理循环
func (t *NativeMemTracker) eventLoop() {
	for {
		select {
		case <-t.stopCh:
			return
		default:
		}
	}
}

// ==================== 事件处理 ====================

// recordAlloc 记录内存分配
func (t *NativeMemTracker) recordAlloc(event *AllocEvent) {
	if int64(len(t.blocks)) >= int64(t.config.MaxBlocks) {
		return
	}
	if event.AllocSize < t.config.MinBlockSize {
		return
	}

	// 采样过滤
	if t.config.SampleRate < 1.0 {
		// 简化采样逻辑
	}

	// 翻译 Java 栈
	if t.config.TrackStack && len(event.NativeStack) > 0 {
		event.JavaStack = t.javaStack.Translate(event.NativeStack)
	}

	block := &MemoryBlock{
		Address:     event.Address,
		Size:        event.AllocSize,
		AllocTime:   time.Unix(0, event.Timestamp),
		AllocTID:    event.TID,
		Source:      event.Source,
		JavaStack:   event.JavaStack,
		NativeStack: event.NativeStack,
		JNIClass:    event.JNIClass,
		JNIMethod:   event.JNIMethod,
	}

	t.mu.Lock()
	t.blocks[event.Address] = block
	t.events = append(t.events, event)
	t.mu.Unlock()

	// 更新统计
	atomic.AddInt64(&t.totalAllocs, 1)
	newSize := atomic.AddInt64(&t.totalAllocSize, event.AllocSize)
	current := atomic.AddInt64(&t.currentSize, event.AllocSize)

	// 更新峰值
	for {
		peak := atomic.LoadInt64(&t.peakSize)
		if current <= peak || atomic.CompareAndSwapInt64(&t.peakSize, peak, current) {
			break
		}
	}
	_ = newSize
}

// recordFree 记录内存释放
func (t *NativeMemTracker) recordFree(event *AllocEvent) {
	t.mu.Lock()
	block, ok := t.blocks[event.Address]
	if !ok {
		t.mu.Unlock()
		return
	}

	block.Released = true
	block.ReleaseTime = time.Unix(0, event.Timestamp)
	block.ReleaseTID = event.TID
	block.Lifetime = block.ReleaseTime.Sub(block.AllocTime).Nanoseconds()

	delete(t.blocks, event.Address)
	t.mu.Unlock()

	t.mu.Lock()
	t.events = append(t.events, event)
	t.mu.Unlock()

	atomic.AddInt64(&t.totalFrees, 1)
	atomic.AddInt64(&t.currentSize, -block.Size)
}

// ==================== 查询接口 ====================

// GetActiveBlocks 获取所有未释放的内存块
func (t *NativeMemTracker) GetActiveBlocks() []*MemoryBlock {
	t.mu.RLock()
	defer t.mu.RUnlock()

	blocks := make([]*MemoryBlock, 0, len(t.blocks))
	for _, b := range t.blocks {
		if !b.Released {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// GetStats 获取统计信息
func (t *NativeMemTracker) GetStats() NativeMemStats {
	t.mu.RLock()
	activeCount := len(t.blocks)
	t.mu.RUnlock()

	return NativeMemStats{
		TotalAllocs:    atomic.LoadInt64(&t.totalAllocs),
		TotalFrees:     atomic.LoadInt64(&t.totalFrees),
		TotalAllocSize: atomic.LoadInt64(&t.totalAllocSize),
		CurrentSize:    atomic.LoadInt64(&t.currentSize),
		PeakSize:       atomic.LoadInt64(&t.peakSize),
		ActiveBlocks:   int64(activeCount),
	}
}

// GetStatsBySource 按来源获取统计
func (t *NativeMemTracker) GetStatsBySource() map[AllocSource]*SourceStat {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[AllocSource]*SourceStat)

	for _, block := range t.blocks {
		if block.Released {
			continue
		}
		if _, ok := stats[block.Source]; !ok {
			stats[block.Source] = &SourceStat{}
		}
		s := stats[block.Source]
		s.Count++
		s.TotalSize += block.Size
	}

	return stats
}

// GetEvents 获取所有事件
func (t *NativeMemTracker) GetEvents() []*AllocEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	events := make([]*AllocEvent, len(t.events))
	copy(events, t.events)
	return events
}

// Close 关闭追踪器
func (t *NativeMemTracker) Close() {
	if t.running {
		t.Stop()
	}
	t.javaStack.Close()
}

// ==================== 统计结构体 ====================

// NativeMemStats 堆外内存统计
type NativeMemStats struct {
	TotalAllocs    int64 `json:"total_allocs"`    // 总分配次数
	TotalFrees     int64 `json:"total_frees"`     // 总释放次数
	TotalAllocSize int64 `json:"total_alloc_size"` // 总分配大小(字节)
	CurrentSize    int64 `json:"current_size"`    // 当前活跃大小(字节)
	PeakSize       int64 `json:"peak_size"`       // 峰值大小(字节)
	ActiveBlocks   int64 `json:"active_blocks"`   // 当前活跃块数
}

// SourceStat 按来源的统计
type SourceStat struct {
	Count     int64 `json:"count"`      // 块数
	TotalSize int64 `json:"total_size"` // 总大小(字节)
}

// ==================== 辅助函数 ====================

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ==================== 堆外内存快照（用于对比分析） ====================

// NativeMemSnapshot 堆外内存快照
type NativeMemSnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Stats     NativeMemStats `json:"stats"`
	Blocks    []*MemoryBlock `json:"blocks"`
}

// TakeSnapshot 获取当前快照
func (t *NativeMemTracker) TakeSnapshot() *NativeMemSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	blocks := make([]*MemoryBlock, 0, len(t.blocks))
	for _, b := range t.blocks {
		if !b.Released {
			// 复制 block 避免外部修改
			blockCopy := *b
			blocks = append(blocks, &blockCopy)
		}
	}

	// 按大小排序
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Size > blocks[j].Size
	})

	return &NativeMemSnapshot{
		Timestamp: time.Now(),
		Stats:     t.GetStats(),
		Blocks:    blocks,
	}
}

// CompareSnapshots 对比两个快照，找出新增和释放的块
func CompareSnapshots(before, after *NativeMemSnapshot) *SnapshotDiff {
	diff := &SnapshotDiff{
		Timestamp:      after.Timestamp,
		BeforeTimestamp: before.Timestamp,
	}

	beforeMap := make(map[uint64]*MemoryBlock)
	for _, b := range before.Blocks {
		beforeMap[b.Address] = b
	}

	afterMap := make(map[uint64]*MemoryBlock)
	for _, b := range after.Blocks {
		afterMap[b.Address] = b
	}

	// 新增的块（after有，before没有）
	for addr, block := range afterMap {
		if _, ok := beforeMap[addr]; !ok {
			diff.NewBlocks = append(diff.NewBlocks, block)
			diff.NewSize += block.Size
		}
	}

	// 释放的块（before有，after没有）
	for addr, block := range beforeMap {
		if _, ok := afterMap[addr]; !ok {
			diff.FreedBlocks = append(diff.FreedBlocks, block)
			diff.FreedSize += block.Size
		}
	}

	// 仍然活跃的块（两边都有）
	for addr, afterBlock := range afterMap {
		if beforeBlock, ok := beforeMap[addr]; ok {
			diff.RemainingBlocks = append(diff.RemainingBlocks, afterBlock)
			diff.RemainingSize += afterBlock.Size
		}
	}

	return diff
}

// SnapshotDiff 快照差异
type SnapshotDiff struct {
	Timestamp      time.Time      `json:"timestamp"`
	BeforeTimestamp time.Time      `json:"before_timestamp"`
	NewBlocks      []*MemoryBlock `json:"new_blocks"`       // 新分配的块
	FreedBlocks    []*MemoryBlock `json:"freed_blocks"`      // 已释放的块
	RemainingBlocks []*MemoryBlock `json:"remaining_blocks"`  // 仍然活跃的块
	NewSize        int64          `json:"new_size"`         // 新分配大小
	FreedSize      int64          `json:"freed_size"`       // 释放大小
	RemainingSize  int64          `json:"remaining_size"`   // 仍然活跃大小
}
