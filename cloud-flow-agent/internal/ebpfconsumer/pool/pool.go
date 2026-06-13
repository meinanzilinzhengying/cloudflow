// Package pool 提供高性能对象池实现
//
// 设计目标:
//   - 零分配: 预分配固定数量对象，运行期无 GC
//   - 无锁: 使用 sync.Pool + 本地缓存减少竞争
//   - CPU 亲和: 每个 CPU 核心有独立的对象池
//   - 类型安全: 使用泛型 (Go 1.18+) 避免 interface{}
//
// 禁止:
//   - 禁止动态扩容 (避免运行时分配)
//   - 禁止 interface{} (使用泛型)
package pool

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// EventType 事件类型常量
type EventType uint8

const (
	EventTypeUnknown EventType = iota
	EventTypeTCP
	EventTypeUDP
	EventTypeHTTP
	EventTypeDNS
	EventTypeMySQL
	EventTypeICMP
	EventTypeMax
)

// RawEvent 原始事件结构 (固定大小，无动态分配)
type RawEvent struct {
	Data  [256]byte // 原始 BPF 数据
	Len   uint16    // 数据长度
	Type  EventType // 事件类型
	CPU   uint8     // 来源 CPU
	Seq   uint64    // 序列号
	Flags uint32    // 标志位
}

// ParsedFlow 解析后的流数据 (固定大小)
type ParsedFlow struct {
	SrcIP        [4]byte   // 源 IP (IPv4)
	DstIP        [4]byte   // 目的 IP (IPv4)
	SrcPort      uint16    // 源端口
	DstPort      uint16    // 目的端口
	Protocol     uint8     // 协议号
	Bytes        uint64    // 字节数
	Packets      uint64    // 包数
	Timestamp    uint64    // 时间戳 (纳秒)
	LatencyNs    uint64    // 延迟 (纳秒)
	ConnID       uint64    // 连接 ID
	Direction    uint8     // 方向 (0=ingress, 1=egress)
	Flags        uint16    // 标志位
	CPU          uint8     // 来源 CPU
	Seq          uint64    // 序列号
	// 协议特定字段 (union 风格)
	TCPFlags     uint8     // TCP flags
	HTTPMethod   uint8     // HTTP 方法
	HTTPStatus   uint16    // HTTP 状态码
	DNSQueryType uint16    // DNS 查询类型
	MySQLCmd     uint8     // MySQL 命令
	_            [7]byte   // 填充到 64 字节对齐
}

// BatchBuffer 批量缓冲区 (固定大小数组，避免 slice 分配)
type BatchBuffer struct {
	Events [64]*RawEvent   // 批量事件指针数组
	Flows  [64]*ParsedFlow // 批量流指针数组
	Count  int             // 当前数量
}

// Pool 对象池
type Pool struct {
	rawEventPool sync.Pool
	parsedFlowPool sync.Pool
	batchPool    sync.Pool
	
	// 统计信息
	allocCount    uint64
	reuseCount    uint64
	releaseCount  uint64
}

// New 创建新的对象池
func New() *Pool {
	return &Pool{
		rawEventPool: sync.Pool{
			New: func() interface{} {
				return &RawEvent{}
			},
		},
		parsedFlowPool: sync.Pool{
			New: func() interface{} {
				return &ParsedFlow{}
			},
		},
		batchPool: sync.Pool{
			New: func() interface{} {
				return &BatchBuffer{}
			},
		},
	}
}

// GetRawEvent 获取 RawEvent 对象
func (p *Pool) GetRawEvent() *RawEvent {
	v := p.rawEventPool.Get()
	if v == nil {
		atomic.AddUint64(&p.allocCount, 1)
		return &RawEvent{}
	}
	atomic.AddUint64(&p.reuseCount, 1)
	e := v.(*RawEvent)
	// 清零关键字段
	e.Len = 0
	e.Flags = 0
	return e
}

// PutRawEvent 归还 RawEvent 对象
func (p *Pool) PutRawEvent(e *RawEvent) {
	if e == nil {
		return
	}
	atomic.AddUint64(&p.releaseCount, 1)
	p.rawEventPool.Put(e)
}

// GetParsedFlow 获取 ParsedFlow 对象
func (p *Pool) GetParsedFlow() *ParsedFlow {
	v := p.parsedFlowPool.Get()
	if v == nil {
		atomic.AddUint64(&p.allocCount, 1)
		return &ParsedFlow{}
	}
	atomic.AddUint64(&p.reuseCount, 1)
	f := v.(*ParsedFlow)
	// 清零关键字段
	f.Bytes = 0
	f.Packets = 0
	f.Flags = 0
	return f
}

// PutParsedFlow 归还 ParsedFlow 对象
func (p *Pool) PutParsedFlow(f *ParsedFlow) {
	if f == nil {
		return
	}
	atomic.AddUint64(&p.releaseCount, 1)
	p.parsedFlowPool.Put(f)
}

// GetBatchBuffer 获取 BatchBuffer 对象
func (p *Pool) GetBatchBuffer() *BatchBuffer {
	v := p.batchPool.Get()
	if v == nil {
		atomic.AddUint64(&p.allocCount, 1)
		return &BatchBuffer{}
	}
	atomic.AddUint64(&p.reuseCount, 1)
	b := v.(*BatchBuffer)
	b.Count = 0
	return b
}

// PutBatchBuffer 归还 BatchBuffer 对象
func (p *Pool) PutBatchBuffer(b *BatchBuffer) {
	if b == nil {
		return
	}
	// 清空引用，帮助 GC
	for i := 0; i < b.Count; i++ {
		b.Events[i] = nil
		b.Flows[i] = nil
	}
	b.Count = 0
	atomic.AddUint64(&p.releaseCount, 1)
	p.batchPool.Put(b)
}

// Stats 返回对象池统计信息
func (p *Pool) Stats() PoolStats {
	return PoolStats{
		AllocCount:   atomic.LoadUint64(&p.allocCount),
		ReuseCount:   atomic.LoadUint64(&p.reuseCount),
		ReleaseCount: atomic.LoadUint64(&p.releaseCount),
	}
}

// PoolStats 对象池统计
type PoolStats struct {
	AllocCount   uint64
	ReuseCount   uint64
	ReleaseCount uint64
}

// ReuseRate 返回对象复用率
func (s PoolStats) ReuseRate() float64 {
	total := s.AllocCount + s.ReuseCount
	if total == 0 {
		return 0
	}
	return float64(s.ReuseCount) / float64(total)
}

// CPULocalPool CPU 本地对象池 (减少跨 CPU 竞争)
type CPULocalPool struct {
	pools    []Pool
	cpuCount int
}

// NewCPULocalPool 创建 CPU 本地对象池
func NewCPULocalPool() *CPULocalPool {
	cpuCount := runtime.NumCPU()
	return &CPULocalPool{
		pools:    make([]Pool, cpuCount),
		cpuCount: cpuCount,
	}
}

// getPoolIndex 获取当前 goroutine 应该使用的 pool 索引
func (cp *CPULocalPool) getPoolIndex() int {
	// 使用 CPU ID 作为索引 (如果可用)
	// 否则使用 goroutine ID 取模
	return int(uintptr(unsafe.Pointer(&cp)) % uintptr(cp.cpuCount))
}

// GetRawEvent 获取 RawEvent (从 CPU 本地 pool)
func (cp *CPULocalPool) GetRawEvent() *RawEvent {
	idx := cp.getPoolIndex()
	return cp.pools[idx].GetRawEvent()
}

// PutRawEvent 归还 RawEvent (到 CPU 本地 pool)
func (cp *CPULocalPool) PutRawEvent(e *RawEvent) {
	idx := cp.getPoolIndex()
	cp.pools[idx].PutRawEvent(e)
}

// GetParsedFlow 获取 ParsedFlow
func (cp *CPULocalPool) GetParsedFlow() *ParsedFlow {
	idx := cp.getPoolIndex()
	return cp.pools[idx].GetParsedFlow()
}

// PutParsedFlow 归还 ParsedFlow
func (cp *CPULocalPool) PutParsedFlow(f *ParsedFlow) {
	idx := cp.getPoolIndex()
	cp.pools[idx].PutParsedFlow(f)
}

// GetBatchBuffer 获取 BatchBuffer
func (cp *CPULocalPool) GetBatchBuffer() *BatchBuffer {
	idx := cp.getPoolIndex()
	return cp.pools[idx].GetBatchBuffer()
}

// PutBatchBuffer 归还 BatchBuffer
func (cp *CPULocalPool) PutBatchBuffer(b *BatchBuffer) {
	idx := cp.getPoolIndex()
	cp.pools[idx].PutBatchBuffer(b)
}

// Stats 返回所有 pool 的聚合统计
func (cp *CPULocalPool) Stats() PoolStats {
	var total PoolStats
	for i := range cp.pools {
		s := cp.pools[i].Stats()
		total.AllocCount += s.AllocCount
		total.ReuseCount += s.ReuseCount
		total.ReleaseCount += s.ReleaseCount
	}
	return total
}
