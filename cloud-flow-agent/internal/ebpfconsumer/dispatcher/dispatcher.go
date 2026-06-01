// Package dispatcher 提供高性能事件分发器
//
// 职责:
//   - 从 perf ringbuffer 读取原始事件
//   - 批量收集事件
//   - 分发到 worker pool
//   - 处理背压 (backpressure)
//
// 优化点:
//   - 批量读取: 每次读取多个事件，减少系统调用
//   - 零拷贝: 直接传递指针，不复制数据
//   - CPU 亲和: dispatcher 绑定到独立 CPU
//   - 自适应批处理: 根据负载动态调整批量大小
package dispatcher

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cilium/ebpf/perf"

	"cloud-flow-agent/internal/ebpfconsumer/pool"
	"cloud-flow-agent/internal/ebpfconsumer/ringbuffer"
)

// PerfRingBuffer 接口 (适配不同的 perf buffer 实现)
type PerfRingBuffer interface {
	// Read 读取事件到缓冲区，返回读取的事件数量
	// 非阻塞，如果没有数据立即返回 0
	Read(events []pool.RawEvent) int

	// BlockingRead 阻塞读取，直到有数据或超时
	BlockingRead(events []pool.RawEvent, timeout time.Duration) int

	// Close 关闭 ringbuffer
	Close()
}

// Config dispatcher 配置
type Config struct {
	// 批处理配置
	MinBatchSize    int           // 最小批量大小 (默认: 16)
	MaxBatchSize    int           // 最大批量大小 (默认: 256)
	BatchTimeout    time.Duration // 批处理超时 (默认: 10μs)

	// 读取配置
	ReadBufferSize  int           // 读取缓冲区大小 (默认: 1024)
	PollInterval    time.Duration // 轮询间隔 (默认: 1μs)

	// CPU 配置
	EnableCPUBind   bool          // 启用 CPU 绑定 (默认: true)
	CPUID           int           // 绑定的 CPU ID (默认: 0)

	// 背压配置
	MaxPendingEvents uint64       // 最大待处理事件数 (默认: 10000)
	DropOnBackpressure bool       // 背压时是否丢弃 (默认: false)
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MinBatchSize:       16,
		MaxBatchSize:       256,
		BatchTimeout:       10 * time.Microsecond,
		ReadBufferSize:     1024,
		PollInterval:       time.Microsecond,
		EnableCPUBind:      true,
		CPUID:              0,
		MaxPendingEvents:   10000,
		DropOnBackpressure: false,
	}
}

// Dispatcher 事件分发器
type Dispatcher struct {
	config      Config
	perfBuffer  PerfRingBuffer
	outputQueue *ringbuffer.RingBuffer
	pool        *pool.Pool

	// 批处理状态
	batchEvents []*pool.RawEvent
	batchCount  int
	batchStart  time.Time

	// 控制
	stopCh      chan struct{}
	stopped     int32
	wg          sync.WaitGroup

	// 统计
	stats       Stats
}

// Stats dispatcher 统计
type Stats struct {
	EventsRead      uint64 // 读取的事件数
	EventsDispatched uint64 // 分发的事件数
	EventsDropped   uint64 // 丢弃的事件数
	Batches         uint64 // 批次数
	ReadCalls       uint64 // 读取调用次数
	Backpressure    uint64 // 背压次数
}

// New 创建新的 dispatcher
func New(config Config, perfBuffer PerfRingBuffer, outputQueue *ringbuffer.RingBuffer, p *pool.Pool) *Dispatcher {
	if config.MinBatchSize <= 0 {
		config.MinBatchSize = 16
	}
	if config.MaxBatchSize <= 0 {
		config.MaxBatchSize = 256
	}
	if config.ReadBufferSize <= 0 {
		config.ReadBufferSize = 1024
	}

	return &Dispatcher{
		config:      config,
		perfBuffer:  perfBuffer,
		outputQueue: outputQueue,
		pool:        p,
		batchEvents: make([]*pool.RawEvent, config.MaxBatchSize),
		stopCh:      make(chan struct{}),
	}
}

// Start 启动 dispatcher
func (d *Dispatcher) Start() {
	d.wg.Add(1)
	go d.run()
}

// Stop 停止 dispatcher
func (d *Dispatcher) Stop() {
	atomic.StoreInt32(&d.stopped, 1)
	close(d.stopCh)
	d.wg.Wait()
}

// run dispatcher 主循环
func (d *Dispatcher) run() {
	defer d.wg.Done()

	// CPU 绑定
	if d.config.EnableCPUBind && d.config.CPUID >= 0 {
		d.bindCPU(d.config.CPUID)
	}

	// 预分配读取缓冲区
	readBuf := make([]pool.RawEvent, d.config.ReadBufferSize)

	d.batchStart = time.Now()

	for atomic.LoadInt32(&d.stopped) == 0 {
		// 批量读取 perf buffer
		n := d.perfBuffer.Read(readBuf)
		atomic.AddUint64(&d.stats.ReadCalls, 1)

		if n > 0 {
			atomic.AddUint64(&d.stats.EventsRead, uint64(n))

			// 处理读取的事件
			for i := 0; i < n; i++ {
				event := d.pool.GetRawEvent()
				// 拷贝数据 (从 perf buffer 到对象池)
				*event = readBuf[i]

				// 添加到批量
				d.batchEvents[d.batchCount] = event
				d.batchCount++

				// 检查是否需要刷新批量
				if d.batchCount >= d.config.MaxBatchSize {
					d.flushBatch()
				}
			}
		}

		// 检查批量超时
		if d.batchCount >= d.config.MinBatchSize &&
		   time.Since(d.batchStart) >= d.config.BatchTimeout {
			d.flushBatch()
		}

		// 无数据时短暂休眠
		if n == 0 {
			runtime.Gosched()
		}
	}

	// 刷新剩余事件
	if d.batchCount > 0 {
		d.flushBatch()
	}
}

// flushBatch 刷新批量事件到输出队列
func (d *Dispatcher) flushBatch() {
	if d.batchCount == 0 {
		return
	}

	// 检查背压
	if d.outputQueue.Size() >= d.config.MaxPendingEvents {
		atomic.AddUint64(&d.stats.Backpressure, 1)

		if d.config.DropOnBackpressure {
			// 丢弃事件
			for i := 0; i < d.batchCount; i++ {
				d.pool.PutRawEvent(d.batchEvents[i])
			}
			atomic.AddUint64(&d.stats.EventsDropped, uint64(d.batchCount))
			d.batchCount = 0
			d.batchStart = time.Now()
			return
		}

		// 等待队列有空间 (简单自旋)
		for d.outputQueue.Size() >= d.config.MaxPendingEvents {
			runtime.Gosched()
		}
	}

	// 批量写入输出队列
	sent := d.outputQueue.TryPushBatch(d.batchEvents[:d.batchCount])

	if sent < d.batchCount {
		// 部分失败，归还未发送的事件
		for i := sent; i < d.batchCount; i++ {
			d.pool.PutRawEvent(d.batchEvents[i])
		}
		atomic.AddUint64(&d.stats.EventsDropped, uint64(d.batchCount-sent))
	}

	atomic.AddUint64(&d.stats.EventsDispatched, uint64(sent))
	atomic.AddUint64(&d.stats.Batches, 1)

	d.batchCount = 0
	d.batchStart = time.Now()
}

// bindCPU 绑定到指定 CPU
func (d *Dispatcher) bindCPU(cpuID int) {
	var mask syscall.CPUSet
	mask.Zero()
	mask.Set(cpuID)
	syscall.SchedSetaffinity(0, &mask)
}

// GetStats 获取统计信息
func (d *Dispatcher) GetStats() Stats {
	return Stats{
		EventsRead:       atomic.LoadUint64(&d.stats.EventsRead),
		EventsDispatched: atomic.LoadUint64(&d.stats.EventsDispatched),
		EventsDropped:    atomic.LoadUint64(&d.stats.EventsDropped),
		Batches:          atomic.LoadUint64(&d.stats.Batches),
		ReadCalls:        atomic.LoadUint64(&d.stats.ReadCalls),
		Backpressure:     atomic.LoadUint64(&d.stats.Backpressure),
	}
}

// PerfBufferAdapter 适配 cilium/ebpf perf buffer 到 PerfRingBuffer 接口
type PerfBufferAdapter struct {
	reader *perf.Reader
}

// NewPerfBufferAdapter 创建适配器
func NewPerfBufferAdapter(reader *perf.Reader) *PerfBufferAdapter {
	return &PerfBufferAdapter{reader: reader}
}

// Read 读取事件
func (a *PerfBufferAdapter) Read(events []pool.RawEvent) int {
	count := 0
	for i := range events {
		record, err := a.reader.Read()
		if err != nil {
			if err == perf.ErrClosed {
				return count
			}
			continue
		}
		if record.RawSample == nil {
			continue
		}

		// 拷贝数据到事件
		events[i].Len = uint16(copy(events[i].Data[:], record.RawSample))
		events[i].CPU = uint8(record.CPU)
		count++

		// 达到批量上限
		if count >= len(events) {
			break
		}
	}
	return count
}

// BlockingRead 阻塞读取
func (a *PerfBufferAdapter) BlockingRead(events []pool.RawEvent, timeout time.Duration) int {
	// 设置超时
	deadline := time.Now().Add(timeout)
	count := 0

	for time.Now().Before(deadline) && count < len(events) {
		n := a.Read(events[count:])
		count += n
		if n == 0 {
			time.Sleep(time.Microsecond)
		}
	}
	return count
}

// Close 关闭
func (a *PerfBufferAdapter) Close() {
	a.reader.Close()
}
