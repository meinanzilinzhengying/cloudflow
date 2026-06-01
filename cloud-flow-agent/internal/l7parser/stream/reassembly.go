// Package stream 提供流式解析和包重组功能
//
// 设计原则:
//   - 部分重组: 只重组必要的数据，避免全包缓存
//   - 滑动窗口: 使用滑动窗口机制管理乱序包
//   - 超时清理: 自动清理过期流
//   - 内存限制: 单流和总内存都有上限

package stream

import (
	"container/list"
	"errors"
	"sync"
	"time"
)

var (
	ErrBufferFull  = errors.New("reassembly buffer full")
	ErrInvalidSeq  = errors.New("invalid sequence number")
	ErrGapTooLarge = errors.New("sequence gap too large")
)

// Segment 数据片段
type Segment struct {
	Seq    uint32 // 起始序列号
	Data   []byte // 数据 (引用原始 buffer)
	Len    uint32 // 数据长度
	IsLast bool   // 是否是最后一个片段 (FIN)
	ts     int64  // 添加时间戳
}

// overlaps 检查是否与另一个片段重叠
func (s *Segment) overlaps(other *Segment) bool {
	sEnd := s.Seq + s.Len
	oEnd := other.Seq + other.Len
	return s.Seq < oEnd && sEnd > other.Seq
}

// ReassemblyBuffer 重组缓冲区实现
type ReassemblyBuffer struct {
	// 流标识
	flowID uint64

	// 期望的下一个序列号
	nextSeq uint32

	// 片段列表 (按 Seq 排序)
	segments *list.List

	// 内存限制
	maxSize int
	curSize int

	// 最大缺口大小 (超过则放弃重组)
	maxGap uint32

	// 超时
	timeout time.Duration
	lastAct int64

	// 互斥锁
	mu sync.Mutex
}

// NewReassemblyBuffer 创建重组缓冲区
func NewReassemblyBuffer(flowID uint64, maxSize int, maxGap uint32, timeout time.Duration) *ReassemblyBuffer {
	return &ReassemblyBuffer{
		flowID:    flowID,
		segments:  list.New(),
		maxSize:   maxSize,
		maxGap:    maxGap,
		timeout:   timeout,
		lastAct:   time.Now().UnixNano(),
	}
}

// Add 添加数据片段
// seq: TCP 序列号
// data: 数据 (会被复制)
// isLast: 是否是最后一个片段
func (rb *ReassemblyBuffer) Add(seq uint32, data []byte, isLast bool) error {
	if len(data) == 0 && !isLast {
		return nil
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	// 检查内存限制
	if rb.curSize+len(data) > rb.maxSize {
		// 尝试清理旧数据
		rb.evictOld()
		if rb.curSize+len(data) > rb.maxSize {
			return ErrBufferFull
		}
	}

	// 创建新片段
	newSeg := &Segment{
		Seq:    seq,
		Data:   make([]byte, len(data)),
		Len:    uint32(len(data)),
		IsLast: isLast,
		ts:     time.Now().UnixNano(),
	}
	copy(newSeg.Data, data)

	// 检查缺口大小
	if rb.nextSeq != 0 && seq > rb.nextSeq {
		gap := seq - rb.nextSeq
		if gap > rb.maxGap {
			// 缺口太大，放弃之前的等待，从当前开始
			rb.segments.Init()
			rb.curSize = 0
			rb.nextSeq = seq
		}
	}

	// 插入到正确位置 (保持有序)
	rb.insertSegment(newSeg)
	rb.curSize += len(data)
	rb.lastAct = time.Now().UnixNano()

	// 合并重叠片段
	rb.mergeOverlaps()

	return nil
}

// insertSegment 插入片段到有序位置
func (rb *ReassemblyBuffer) insertSegment(seg *Segment) {
	for e := rb.segments.Front(); e != nil; e = e.Next() {
		s := e.Value.(*Segment)
		if seg.Seq < s.Seq {
			rb.segments.InsertBefore(seg, e)
			return
		}
		if seg.Seq == s.Seq {
			// 相同序列号，保留更长的数据
			if seg.Len > s.Len {
				e.Value = seg
			}
			return
		}
	}
	// 插入到末尾
	rb.segments.PushBack(seg)
}

// mergeOverlaps 合并重叠的片段
func (rb *ReassemblyBuffer) mergeOverlaps() {
	if rb.segments.Len() < 2 {
		return
	}

	for e := rb.segments.Front(); e != nil && e.Next() != nil; {
		curr := e.Value.(*Segment)
		next := e.Next().Value.(*Segment)

		currEnd := curr.Seq + curr.Len
		nextEnd := next.Seq + next.Len

		// 检查是否重叠或相邻
		if currEnd >= next.Seq {
			// 需要合并
			if nextEnd > currEnd {
				// next 延伸到 curr 之外，扩展 curr
				extendLen := nextEnd - currEnd
				newData := make([]byte, curr.Len+extendLen)
				copy(newData, curr.Data)
				copy(newData[curr.Len:], next.Data[currEnd-next.Seq:])
				curr.Data = newData
				curr.Len += extendLen
			}
			// 合并 IsLast 标志
			curr.IsLast = curr.IsLast || next.IsLast

			// 删除 next
			nextElem := e.Next()
			rb.segments.Remove(nextElem)
			rb.curSize -= len(next.Data)
		} else {
			e = e.Next()
		}
	}
}

// GetNext 获取下一个连续的数据块
// 返回: 数据，是否是最后一块
func (rb *ReassemblyBuffer) GetNext() ([]byte, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.segments.Len() == 0 {
		return nil, false
	}

	first := rb.segments.Front().Value.(*Segment)

	// 检查是否是我们期望的下一个序列号
	if rb.nextSeq != 0 && first.Seq != rb.nextSeq {
		return nil, false
	}

	// 返回数据
	data := first.Data
	isLast := first.IsLast

	// 更新状态
	rb.nextSeq = first.Seq + first.Len
	rb.segments.Remove(rb.segments.Front())
	rb.curSize -= len(data)
	rb.lastAct = time.Now().UnixNano()

	return data, isLast
}

// PeekNext 查看下一个数据块但不移除
func (rb *ReassemblyBuffer) PeekNext() ([]byte, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.segments.Len() == 0 {
		return nil, false
	}

	first := rb.segments.Front().Value.(*Segment)

	if rb.nextSeq != 0 && first.Seq != rb.nextSeq {
		return nil, false
	}

	return first.Data, first.IsLast
}

// HasGap 检查是否有缺口
func (rb *ReassemblyBuffer) HasGap() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.segments.Len() == 0 {
		return false
	}

	first := rb.segments.Front().Value.(*Segment)
	return rb.nextSeq != 0 && first.Seq > rb.nextSeq
}

// GapSize 返回缺口大小
func (rb *ReassemblyBuffer) GapSize() uint32 {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.segments.Len() == 0 {
		return 0
	}

	first := rb.segments.Front().Value.(*Segment)
	if rb.nextSeq != 0 && first.Seq > rb.nextSeq {
		return first.Seq - rb.nextSeq
	}
	return 0
}

// Clear 清空缓冲区
func (rb *ReassemblyBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.segments.Init()
	rb.curSize = 0
	rb.nextSeq = 0
}

// Size 当前缓冲区大小
func (rb *ReassemblyBuffer) Size() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.curSize
}

// IsExpired 检查是否过期
func (rb *ReassemblyBuffer) IsExpired() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return time.Now().UnixNano()-rb.lastAct > int64(rb.timeout)
}

// SetNextSeq 设置期望的下一个序列号
func (rb *ReassemblyBuffer) SetNextSeq(seq uint32) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.nextSeq = seq
}

// evictOld 清理旧数据
func (rb *ReassemblyBuffer) evictOld() {
	now := time.Now().UnixNano()
	expireTime := now - int64(rb.timeout)

	for e := rb.segments.Front(); e != nil; {
		seg := e.Value.(*Segment)
		next := e.Next()

		if seg.ts < expireTime {
			rb.curSize -= len(seg.Data)
			rb.segments.Remove(e)
		}
		e = next
	}
}

// ============================================================================
// 流管理器
// ============================================================================

// StreamManager 流管理器
type StreamManager struct {
	// 流缓冲区映射 (flowID -> ReassemblyBuffer)
	streams sync.Map

	// 配置
	maxStreams int
	maxBufSize int
	maxGap     uint32
	timeout    time.Duration

	// 统计
	stats StreamStats
}

// StreamStats 流统计
type StreamStats struct {
	ActiveStreams uint64
	TotalStreams  uint64
	Evicted       uint64
	Expired       uint64
}

// NewStreamManager 创建流管理器
func NewStreamManager(maxStreams int, maxBufSize int, maxGap uint32, timeout time.Duration) *StreamManager {
	sm := &StreamManager{
		maxStreams: maxStreams,
		maxBufSize: maxBufSize,
		maxGap:     maxGap,
		timeout:    timeout,
	}

	// 启动清理协程
	go sm.cleanupLoop()

	return sm
}

// GetOrCreate 获取或创建流缓冲区
func (sm *StreamManager) GetOrCreate(flowID uint64) *ReassemblyBuffer {
	// 尝试获取现有流
	if v, ok := sm.streams.Load(flowID); ok {
		return v.(*ReassemblyBuffer)
	}

	// 检查流数量限制
	var streamCount int
	sm.streams.Range(func(_, _ interface{}) bool {
		streamCount++
		return streamCount < sm.maxStreams*2 // 粗略估计
	})

	if streamCount >= sm.maxStreams {
		// 清理最旧的流
		sm.evictOldest()
	}

	// 创建新流
	buf := NewReassemblyBuffer(flowID, sm.maxBufSize, sm.maxGap, sm.timeout)

	// 存储 (可能覆盖其他流，但概率低)
	actual, loaded := sm.streams.LoadOrStore(flowID, buf)
	if loaded {
		return actual.(*ReassemblyBuffer)
	}

	sm.stats.TotalStreams++
	sm.stats.ActiveStreams++

	return buf
}

// Get 获取流缓冲区
func (sm *StreamManager) Get(flowID uint64) (*ReassemblyBuffer, bool) {
	if v, ok := sm.streams.Load(flowID); ok {
		return v.(*ReassemblyBuffer), true
	}
	return nil, false
}

// Remove 移除流
func (sm *StreamManager) Remove(flowID uint64) {
	if _, ok := sm.streams.LoadAndDelete(flowID); ok {
		if sm.stats.ActiveStreams > 0 {
			sm.stats.ActiveStreams--
		}
	}
}

// cleanupLoop 清理循环
func (sm *StreamManager) cleanupLoop() {
	ticker := time.NewTicker(sm.timeout / 2)
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanup()
	}
}

// cleanup 清理过期流
func (sm *StreamManager) cleanup() {
	now := time.Now().UnixNano()
	expireTime := now - int64(sm.timeout)

	sm.streams.Range(func(key, value interface{}) bool {
		buf := value.(*ReassemblyBuffer)
		if buf.lastAct < expireTime {
			sm.streams.Delete(key)
			sm.stats.Expired++
			if sm.stats.ActiveStreams > 0 {
				sm.stats.ActiveStreams--
			}
		}
		return true
	})
}

// evictOldest 驱逐最旧的流
func (sm *StreamManager) evictOldest() {
	var oldestKey uint64
	var oldestTime int64 = time.Now().UnixNano()

	sm.streams.Range(func(key, value interface{}) bool {
		buf := value.(*ReassemblyBuffer)
		if buf.lastAct < oldestTime {
			oldestTime = buf.lastAct
			oldestKey = key.(uint64)
		}
		return true
	})

	if oldestKey != 0 {
		sm.streams.Delete(oldestKey)
		sm.stats.Evicted++
		if sm.stats.ActiveStreams > 0 {
			sm.stats.ActiveStreams--
		}
	}
}

// Stats 获取统计
func (sm *StreamManager) Stats() StreamStats {
	return sm.stats
}
