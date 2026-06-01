// Package ringbuffer 提供无锁环形缓冲区实现
// 基于 Disruptor 模式，使用 CAS 操作实现 MPMC (多生产者多消费者)
//
// 性能特点:
//   - 无锁设计: 仅使用 atomic 操作，无 mutex
//   - CPU Cache 友好: 缓存行对齐，避免 false sharing
//   - 批量操作: 支持批量读写，减少 CAS 次数
//   - 预分配内存: 创建时一次性分配，运行期无 GC 压力
//
// 限制:
//   - 容量必须是 2 的幂次方
//   - 元素类型固定为 *RawEvent (避免 interface{})
package ringbuffer

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

// cacheLineSize 是 CPU 缓存行大小，用于避免 false sharing
const cacheLineSize = 64

// RawEvent 是 ringbuffer 中传输的原始事件
// 使用固定大小的数组存储原始数据，避免动态分配
type RawEvent struct {
	Data   [256]byte // 固定大小缓冲区，足够存储 BPF 数据
	Len    uint16    // 实际数据长度
	Type   uint8     // 事件类型 (tcp/udp/http/dns/mysql)
	CPU    uint8     // 来源 CPU ID (用于 CPU pinning)
	Seq    uint64    // 序列号 (用于排序和去重)
	Flags  uint32    // 标志位
}

// eventSlot 是 ringbuffer 中的单个槽位
// 使用填充确保每个槽位独占一个缓存行
type eventSlot struct {
	sequence uint64      // 序列号 (用于同步)
	_        [cacheLineSize - 8]byte // 填充到缓存行大小
	event    *RawEvent   // 事件指针
	_        [cacheLineSize - unsafe.Sizeof(uintptr(0))]byte // 填充到缓存行大小
}

// RingBuffer 是无锁环形缓冲区
type RingBuffer struct {
	capacity   uint64      // 容量 (必须是 2 的幂)
	capacityMask uint64    // 容量掩码 (用于快速取模)
	
	// 生产者位置 (缓存行对齐)
	writeSeq   uint64
	_          [cacheLineSize - 8]byte
	
	// 消费者位置 (缓存行对齐)
	readSeq    uint64
	_          [cacheLineSize - 8]byte
	
	// 缓冲区
	slots      []eventSlot
}

// New 创建新的 RingBuffer
// capacity 必须是 2 的幂次方 (如 1024, 2048, 4096, 8192, 16384, 32768, 65536)
func New(capacity uint64) *RingBuffer {
	// 确保容量是 2 的幂
	if capacity&(capacity-1) != 0 {
		panic("ringbuffer capacity must be power of 2")
	}
	
	rb := &RingBuffer{
		capacity:     capacity,
		capacityMask: capacity - 1,
		slots:        make([]eventSlot, capacity),
	}
	
	// 初始化槽位序列号
	for i := uint64(0); i < capacity; i++ {
		rb.slots[i].sequence = i
	}
	
	return rb
}

// TryPush 尝试写入单个事件
// 返回 true 表示成功，false 表示缓冲区已满
func (rb *RingBuffer) TryPush(event *RawEvent) bool {
	for {
		writeSeq := atomic.LoadUint64(&rb.writeSeq)
		slot := &rb.slots[writeSeq&rb.capacityMask]
		seq := atomic.LoadUint64(&slot.sequence)
		
		// 检查槽位是否可用 (seq == writeSeq 表示可用)
		diff := int64(seq) - int64(writeSeq)
		if diff == 0 {
			// 尝试 CAS 更新 writeSeq
			if atomic.CompareAndSwapUint64(&rb.writeSeq, writeSeq, writeSeq+1) {
				// 写入数据
				slot.event = event
				// 发布: 更新槽位序列号，表示数据已写入
				atomic.StoreUint64(&slot.sequence, writeSeq+1)
				return true
			}
			// CAS 失败，重试
			continue
		}
		
		// 槽位不可用 (缓冲区满或正在处理)
		if diff < 0 {
			// seq < writeSeq，说明缓冲区已满
			return false
		}
		
		// 其他情况，重试
		runtime.Gosched()
	}
}

// TryPushBatch 尝试批量写入事件
// 返回成功写入的数量
func (rb *RingBuffer) TryPushBatch(events []*RawEvent) int {
	batchSize := uint64(len(events))
	if batchSize == 0 {
		return 0
	}
	
	for {
		writeSeq := atomic.LoadUint64(&rb.writeSeq)
		
		// 检查可用空间
		readSeq := atomic.LoadUint64(&rb.readSeq)
		available := rb.capacity - (writeSeq - readSeq)
		if available == 0 {
			return 0
		}
		
		// 限制批量大小
		if batchSize > available {
			batchSize = available
		}
		
		// 尝试 CAS 更新 writeSeq
		newWriteSeq := writeSeq + batchSize
		if !atomic.CompareAndSwapUint64(&rb.writeSeq, writeSeq, newWriteSeq) {
			continue
		}
		
		// 写入数据
		for i := uint64(0); i < batchSize; i++ {
			slot := &rb.slots[(writeSeq+i)&rb.capacityMask]
			// 等待槽位可用
			for {
				seq := atomic.LoadUint64(&slot.sequence)
				if int64(seq)-int64(writeSeq+i) == 0 {
					break
				}
				runtime.Gosched()
			}
			slot.event = events[i]
			atomic.StoreUint64(&slot.sequence, writeSeq+i+1)
		}
		
		return int(batchSize)
	}
}

// TryPop 尝试读取单个事件
// 返回事件和 true 表示成功，nil 和 false 表示缓冲区为空
func (rb *RingBuffer) TryPop() (*RawEvent, bool) {
	for {
		readSeq := atomic.LoadUint64(&rb.readSeq)
		slot := &rb.slots[readSeq&rb.capacityMask]
		seq := atomic.LoadUint64(&slot.sequence)
		
		// 检查槽位是否有数据 (seq == readSeq+1 表示有数据)
		diff := int64(seq) - int64(readSeq+1)
		if diff == 0 {
			// 尝试 CAS 更新 readSeq
			if atomic.CompareAndSwapUint64(&rb.readSeq, readSeq, readSeq+1) {
				// 读取数据
				event := slot.event
				// 重置槽位
				slot.event = nil
				// 标记槽位为可用
				atomic.StoreUint64(&slot.sequence, readSeq+rb.capacity)
				return event, true
			}
			// CAS 失败，重试
			continue
		}
		
		// 槽位无数据 (缓冲区空)
		if diff < 0 {
			return nil, false
		}
		
		// 其他情况，重试
		runtime.Gosched()
	}
}

// TryPopBatch 尝试批量读取事件
// 返回实际读取的事件数量
func (rb *RingBuffer) TryPopBatch(events []*RawEvent) int {
	batchSize := uint64(len(events))
	if batchSize == 0 {
		return 0
	}
	
	for {
		readSeq := atomic.LoadUint64(&rb.readSeq)
		writeSeq := atomic.LoadUint64(&rb.writeSeq)
		
		// 检查可用数据量
		available := writeSeq - readSeq
		if available == 0 {
			return 0
		}
		
		// 限制批量大小
		if batchSize > available {
			batchSize = available
		}
		
		// 尝试 CAS 更新 readSeq
		newReadSeq := readSeq + batchSize
		if !atomic.CompareAndSwapUint64(&rb.readSeq, readSeq, newReadSeq) {
			continue
		}
		
		// 读取数据
		for i := uint64(0); i < batchSize; i++ {
			slot := &rb.slots[(readSeq+i)&rb.capacityMask]
			// 等待槽位有数据
			for {
				seq := atomic.LoadUint64(&slot.sequence)
				if int64(seq)-int64(readSeq+i+1) == 0 {
					break
				}
				runtime.Gosched()
			}
			events[i] = slot.event
			slot.event = nil
			atomic.StoreUint64(&slot.sequence, readSeq+i+rb.capacity)
		}
		
		return int(batchSize)
	}
}

// Size 返回当前缓冲区中的事件数量
func (rb *RingBuffer) Size() uint64 {
	writeSeq := atomic.LoadUint64(&rb.writeSeq)
	readSeq := atomic.LoadUint64(&rb.readSeq)
	return writeSeq - readSeq
}

// IsEmpty 返回缓冲区是否为空
func (rb *RingBuffer) IsEmpty() bool {
	return rb.Size() == 0
}

// IsFull 返回缓冲区是否已满
func (rb *RingBuffer) IsFull() bool {
	return rb.Size() >= rb.capacity
}

// Capacity 返回缓冲区容量
func (rb *RingBuffer) Capacity() uint64 {
	return rb.capacity
}
