// Package queue 提供有界内存队列实现，支持流量控制与背压机制
package queue

import (
	"context"
	"sync/atomic"

	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
)

// DropPolicy 队列满时的丢弃策略
type DropPolicy int

const (
	// DropOldest 丢弃最旧的条目
	DropOldest DropPolicy = iota
	// DropNewest 丢弃最新的条目
	DropNewest
)

// QueueStats 队列统计信息
type QueueStats struct {
	Enqueued    uint64 `json:"enqueued"`
	Dropped     uint64 `json:"dropped"`
	CurrentSize int    `json:"current_size"`
	MaxSize     int    `json:"max_size"`
}

// BoundedQueue 有界内存队列，支持流量控制
type BoundedQueue struct {
	ch       chan interface{}
	maxLen   int
	policy   DropPolicy
	enqueued uint64
	dropped  uint64
	logger   *logger.Logger
}

// NewBoundedQueue 创建有界队列
func NewBoundedQueue(maxLen int, policy DropPolicy, log *logger.Logger) *BoundedQueue {
	return &BoundedQueue{
		ch:     make(chan interface{}, maxLen),
		maxLen: maxLen,
		policy: policy,
		logger: log,
	}
}

// Enqueue 入队，队列满时按策略丢弃
func (q *BoundedQueue) Enqueue(item interface{}) bool {
	select {
	case q.ch <- item:
		atomic.AddUint64(&q.enqueued, 1)
		return true
	default:
		// 队列满，按策略丢弃
		if q.policy == DropOldest {
			select {
			case <-q.ch:
				q.ch <- item
				atomic.AddUint64(&q.dropped, 1)
				atomic.AddUint64(&q.enqueued, 1)
				if q.logger != nil {
					q.logger.Warnf("queue full, dropped oldest item, total dropped: %d", atomic.LoadUint64(&q.dropped))
				}
				return true
			default:
			}
		}
		atomic.AddUint64(&q.dropped, 1)
		if q.logger != nil {
			q.logger.Warnf("queue full, dropped newest item, total dropped: %d", atomic.LoadUint64(&q.dropped))
		}
		return false
	}
}

// Dequeue 出队，阻塞直到有数据或上下文取消
func (q *BoundedQueue) Dequeue(ctx context.Context) (interface{}, bool) {
	select {
	case item := <-q.ch:
		return item, true
	case <-ctx.Done():
		return nil, false
	}
}

// TryDequeue 非阻塞出队
func (q *BoundedQueue) TryDequeue() (interface{}, bool) {
	select {
	case item := <-q.ch:
		return item, true
	default:
		return nil, false
	}
}

// Len 当前队列长度
func (q *BoundedQueue) Len() int {
	return len(q.ch)
}

// Cap 队列容量
func (q *BoundedQueue) Cap() int {
	return cap(q.ch)
}

// Stats 获取队列统计
func (q *BoundedQueue) Stats() QueueStats {
	return QueueStats{
		Enqueued:    atomic.LoadUint64(&q.enqueued),
		Dropped:     atomic.LoadUint64(&q.dropped),
		CurrentSize: len(q.ch),
		MaxSize:     q.maxLen,
	}
}

// Close 关闭队列
func (q *BoundedQueue) Close() {
	close(q.ch)
}
