// Package worker 提供高性能 worker pool 实现
//
// 特性:
//   - CPU Pinning: 每个 worker 绑定到特定 CPU 核心
//   - 批量处理: 支持批量消费，减少调度开销
//   - 无锁通信: 通过 ringbuffer 与 dispatcher 通信
//   - 优雅关闭: 支持 graceful shutdown，处理完当前任务后退出
//
// 性能目标:
//   - 单 worker 100k+ event/s
//   - 延迟 < 10μs (P99)
package worker

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
	"cloud-flow-agent/internal/ebpfconsumer/ringbuffer"
)

// Handler 是 worker 处理函数类型
type Handler func(events []*pool.RawEvent, flows []*pool.ParsedFlow, count int)

// Config worker pool 配置
type Config struct {
	WorkerCount    int           // worker 数量 (默认: CPU 核心数)
	BatchSize      int           // 批量大小 (默认: 32)
	PollInterval   time.Duration // 轮询间隔 (默认: 1μs)
	EnableCPUBind  bool          // 启用 CPU 绑定 (默认: true)
	QueueCapacity  uint64        // 每个 worker 队列容量 (默认: 4096)
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		WorkerCount:   runtime.NumCPU(),
		BatchSize:     32,
		PollInterval:  time.Microsecond,
		EnableCPUBind: true,
		QueueCapacity: 4096,
	}
}

// Worker 单个 worker
type Worker struct {
	id         int
	cpuID      int
	queue      *ringbuffer.RingBuffer
	handler    Handler
	pool       *pool.Pool
	
	// 控制
	stopCh     chan struct{}
	stopped    int32
	wg         sync.WaitGroup
	
	// 统计
	processedCount uint64
	batchCount     uint64
}

// NewWorker 创建新的 worker
func NewWorker(id, cpuID int, queue *ringbuffer.RingBuffer, handler Handler, p *pool.Pool) *Worker {
	return &Worker{
		id:      id,
		cpuID:   cpuID,
		queue:   queue,
		handler: handler,
		pool:    p,
		stopCh:  make(chan struct{}),
	}
}

// Start 启动 worker
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
}

// Stop 停止 worker
func (w *Worker) Stop() {
	atomic.StoreInt32(&w.stopped, 1)
	close(w.stopCh)
	w.wg.Wait()
}

// run worker 主循环
func (w *Worker) run() {
	defer w.wg.Done()
	
	// CPU 绑定
	if w.cpuID >= 0 {
		w.bindCPU(w.cpuID)
	}
	
	// 预分配批量缓冲区
	events := make([]*pool.RawEvent, 32)
	flows := make([]*pool.ParsedFlow, 32)
	
	for atomic.LoadInt32(&w.stopped) == 0 {
		// 批量读取
		n := w.queue.TryPopBatch(events)
		if n == 0 {
			// 无数据，短暂休眠避免忙等
			runtime.Gosched()
			continue
		}
		
		// 处理事件
		for i := 0; i < n; i++ {
			// 从对象池获取 ParsedFlow
			flows[i] = w.pool.GetParsedFlow()
			// 解析事件 (zero-copy)
			w.parseEvent(events[i], flows[i])
		}
		
		// 调用 handler
		w.handler(events[:n], flows[:n], n)
		
		// 归还对象到 pool
		for i := 0; i < n; i++ {
			w.pool.PutRawEvent(events[i])
			// ParsedFlow 由 handler 负责归还，或在这里统一归还
			// w.pool.PutParsedFlow(flows[i])
		}
		
		// 更新统计
		atomic.AddUint64(&w.processedCount, uint64(n))
		atomic.AddUint64(&w.batchCount, 1)
	}
}

// bindCPU 绑定到指定 CPU
func (w *Worker) bindCPU(cpuID int) {
	var mask syscall.CPUSet
	mask.Zero()
	mask.Set(cpuID)
	syscall.SchedSetaffinity(0, &mask)
}

// parseEvent 解析原始事件到 ParsedFlow (zero-copy)
func (w *Worker) parseEvent(raw *pool.RawEvent, flow *pool.ParsedFlow) {
	if raw == nil || flow == nil {
		return
	}
	
	// 直接解析 raw.Data 到 flow 字段
	// 使用 unsafe 避免拷贝
	data := raw.Data[:raw.Len]
	
	// 解析 IP 地址 (假设数据格式: src_ip[4] + dst_ip[4] + src_port[2] + dst_port[2] + protocol[1] + ...)
	if len(data) >= 13 {
		copy(flow.SrcIP[:], data[0:4])
		copy(flow.DstIP[:], data[4:8])
		flow.SrcPort = uint16(data[8])<<8 | uint16(data[9])
		flow.DstPort = uint16(data[10])<<8 | uint16(data[11])
		flow.Protocol = data[12]
	}
	
	// 解析统计数据 (假设在 offset 13 开始)
	if len(data) >= 29 {
		flow.Bytes = *(*uint64)(unsafe.Pointer(&data[13]))
		flow.Packets = *(*uint64)(unsafe.Pointer(&data[21]))
	}
	
	flow.Type = raw.Type
	flow.CPU = raw.CPU
	flow.Seq = raw.Seq
	flow.Timestamp = uint64(time.Now().UnixNano())
}

// Stats 返回 worker 统计
func (w *Worker) Stats() WorkerStats {
	return WorkerStats{
		ProcessedCount: atomic.LoadUint64(&w.processedCount),
		BatchCount:     atomic.LoadUint64(&w.batchCount),
		QueueSize:      w.queue.Size(),
	}
}

// WorkerStats worker 统计
type WorkerStats struct {
	ProcessedCount uint64
	BatchCount     uint64
	QueueSize      uint64
}

// Pool worker pool
type Pool struct {
	config   Config
	workers  []*Worker
	queues   []*ringbuffer.RingBuffer
	handler  Handler
	pool     *pool.Pool
	
	// 分发策略
	dispatchIdx uint64 // 轮询索引
	
	// 控制
	stopCh  chan struct{}
	stopped int32
	wg      sync.WaitGroup
}

// NewPool 创建新的 worker pool
func NewPool(config Config, handler Handler, p *pool.Pool) *Pool {
	if config.WorkerCount <= 0 {
		config.WorkerCount = runtime.NumCPU()
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 32
	}
	if config.QueueCapacity == 0 {
		config.QueueCapacity = 4096
	}
	
	wp := &Pool{
		config:  config,
		workers: make([]*Worker, config.WorkerCount),
		queues:  make([]*ringbuffer.RingBuffer, config.WorkerCount),
		handler: handler,
		pool:    p,
		stopCh:  make(chan struct{}),
	}
	
	// 创建每个 worker 的队列
	for i := 0; i < config.WorkerCount; i++ {
		wp.queues[i] = ringbuffer.New(config.QueueCapacity)
	}
	
	return wp
}

// Start 启动 worker pool
func (wp *Pool) Start() {
	for i := 0; i < wp.config.WorkerCount; i++ {
		cpuID := -1
		if wp.config.EnableCPUBind {
			cpuID = i % runtime.NumCPU()
		}
		wp.workers[i] = NewWorker(i, cpuID, wp.queues[i], wp.handler, wp.pool)
		wp.workers[i].Start()
	}
}

// Stop 停止 worker pool
func (wp *Pool) Stop() {
	atomic.StoreInt32(&wp.stopped, 1)
	close(wp.stopCh)
	
	for _, w := range wp.workers {
		w.Stop()
	}
	wp.wg.Wait()
}

// Dispatch 分发事件到 worker
func (wp *Pool) Dispatch(event *pool.RawEvent) bool {
	// 轮询选择 worker
	idx := atomic.AddUint64(&wp.dispatchIdx, 1) % uint64(wp.config.WorkerCount)
	return wp.queues[idx].TryPush(event)
}

// DispatchBatch 批量分发事件
func (wp *Pool) DispatchBatch(events []*pool.RawEvent) int {
	// 轮询选择 worker
	idx := atomic.AddUint64(&wp.dispatchIdx, 1) % uint64(wp.config.WorkerCount)
	return wp.queues[idx].TryPushBatch(events)
}

// Stats 返回 pool 统计
func (wp *Pool) Stats() PoolStats {
	stats := PoolStats{
		WorkerCount: wp.config.WorkerCount,
		WorkerStats: make([]WorkerStats, wp.config.WorkerCount),
	}
	
	for i, w := range wp.workers {
		if w != nil {
			stats.WorkerStats[i] = w.Stats()
			stats.TotalProcessed += stats.WorkerStats[i].ProcessedCount
		}
	}
	
	return stats
}

// PoolStats worker pool 统计
type PoolStats struct {
	WorkerCount    int
	TotalProcessed uint64
	WorkerStats    []WorkerStats
}
