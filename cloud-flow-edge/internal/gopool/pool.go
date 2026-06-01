// Package gopool provides a fixed-size goroutine pool for bounding concurrency
// and preventing OOM under high load (e.g. 5000+ concurrent Agent gRPC streams).
package gopool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Task is a unit of work submitted to the pool.
type Task func()

// Metrics holds pool statistics.
type Metrics struct {
	ActiveWorkers int64 // number of workers currently executing a task
	QueuedTasks   int64 // number of tasks waiting in the queue
	CompletedTasks int64 // total tasks completed since pool creation
	RejectedTasks int64 // total tasks rejected because the queue was full
}

// Pool is a fixed-size goroutine pool with a bounded task queue.
type Pool struct {
	workers    int
	queueCap   int
	taskCh     chan Task
	wg         sync.WaitGroup // tracks running workers
	once       sync.Once      // ensures Stop is idempotent
	stopCh     chan struct{}  // signals workers to exit
	stopped    int32          // atomic flag: 1 = pool has been stopped

	activeWorkers  int64 // atomic
	queuedTasks    int64 // atomic
	completedTasks int64 // atomic
	rejectedTasks  int64 // atomic
}

// Option configures a Pool.
type Option func(*Pool)

// WithWorkers sets the number of worker goroutines (default 500).
func WithWorkers(n int) Option {
	return func(p *Pool) {
		if n > 0 {
			p.workers = n
		}
	}
}

// WithQueueCap sets the bounded task queue capacity (default 10000).
func WithQueueCap(n int) Option {
	return func(p *Pool) {
		if n > 0 {
			p.queueCap = n
		}
	}
}

// New creates a new goroutine pool and starts all workers immediately.
func New(opts ...Option) *Pool {
	p := &Pool{
		workers:  500,
		queueCap: 10000,
	}
	for _, o := range opts {
		o(p)
	}
	p.taskCh = make(chan Task, p.queueCap)
	p.stopCh = make(chan struct{})

	p.wg.Add(p.workers)
	for i := 0; i < p.workers; i++ {
		go p.worker(i)
	}
	return p
}

// worker is a long-running goroutine that reads tasks from the task channel.
func (p *Pool) worker(id int) {
	defer p.wg.Done()
	for {
		select {
		case task, ok := <-p.taskCh:
			if !ok {
				// channel closed — pool is shutting down
				return
			}
			atomic.AddInt64(&p.activeWorkers, 1)
			atomic.AddInt64(&p.queuedTasks, -1)
			task()
			atomic.AddInt64(&p.activeWorkers, -1)
			atomic.AddInt64(&p.completedTasks, 1)
		case <-p.stopCh:
			// Drain remaining tasks in the queue before exiting.
			p.drain()
			return
		}
	}
}

// drain processes all remaining tasks in the queue after the stop signal.
func (p *Pool) drain() {
	for {
		select {
		case task, ok := <-p.taskCh:
			if !ok {
				return
			}
			atomic.AddInt64(&p.activeWorkers, 1)
			atomic.AddInt64(&p.queuedTasks, -1)
			task()
			atomic.AddInt64(&p.activeWorkers, -1)
			atomic.AddInt64(&p.completedTasks, 1)
		default:
			return
		}
	}
}

// Submit enqueues a task. Returns an error non-blockingly if the pool is full.
// Once the pool is stopped, Submit always returns ErrPoolStopped.
func (p *Pool) Submit(task Task) error {
	if atomic.LoadInt32(&p.stopped) == 1 {
		return ErrPoolStopped
	}
	select {
	case p.taskCh <- task:
		atomic.AddInt64(&p.queuedTasks, 1)
		return nil
	default:
		atomic.AddInt64(&p.rejectedTasks, 1)
		return ErrPoolFull
	}
}

// SubmitWait blocks until the task is accepted into the queue or the pool is stopped.
// If the pool is stopped before the task is accepted, it returns ErrPoolStopped.
func (p *Pool) SubmitWait(ctx context.Context, task Task) error {
	if atomic.LoadInt32(&p.stopped) == 1 {
		return ErrPoolStopped
	}
	select {
	case p.taskCh <- task:
		atomic.AddInt64(&p.queuedTasks, 1)
		return nil
	case <-ctx.Done():
		atomic.AddInt64(&p.rejectedTasks, 1)
		return fmt.Errorf("%w: %v", ErrPoolFull, ctx.Err())
	case <-p.stopCh:
		atomic.AddInt64(&p.rejectedTasks, 1)
		return ErrPoolStopped
	}
}

// Stop gracefully shuts down the pool. It signals all workers to stop and
// waits for them to finish draining queued tasks. Stop is idempotent.
func (p *Pool) Stop() {
	p.once.Do(func() {
		atomic.StoreInt32(&p.stopped, 1)
		close(p.stopCh)
		p.wg.Wait()
	})
}

// Stopped returns true if the pool has been stopped.
func (p *Pool) Stopped() bool {
	return atomic.LoadInt32(&p.stopped) == 1
}

// GetMetrics returns a snapshot of the current pool metrics.
func (p *Pool) GetMetrics() Metrics {
	return Metrics{
		ActiveWorkers:  atomic.LoadInt64(&p.activeWorkers),
		QueuedTasks:    atomic.LoadInt64(&p.queuedTasks),
		CompletedTasks: atomic.LoadInt64(&p.completedTasks),
		RejectedTasks:  atomic.LoadInt64(&p.rejectedTasks),
	}
}

// Workers returns the configured number of workers.
func (p *Pool) Workers() int {
	return p.workers
}

// QueueCap returns the configured queue capacity.
func (p *Pool) QueueCap() int {
	return p.queueCap
}

// WaitUntilIdle blocks until no tasks are queued and no workers are active,
// or the given context is cancelled, or the pool is stopped.
func (p *Pool) WaitUntilIdle(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if atomic.LoadInt64(&p.queuedTasks) == 0 && atomic.LoadInt64(&p.activeWorkers) == 0 {
			return nil
		}
		if atomic.LoadInt32(&p.stopped) == 1 {
			return ErrPoolStopped
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
