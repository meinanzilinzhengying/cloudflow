// Package pool Parser Worker Pool 实现
//
// 设计要点:
//   - 独立线程池，不阻塞 ingest pipeline
//   - 支持 backpressure
//   - 动态扩缩容
//   - 任务优先级

package pool

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow/cloud-flow-agent/internal/l7parser"
)

// WorkerPool Parser Worker Pool
type WorkerPool struct {
	// 配置
	config Config

	// Worker 列表
	workers []*Worker

	// 任务队列
	taskQueue chan *l7parser.ParseTask

	// 优先级队列 (高优先级任务)
	priorityQueue chan *l7parser.ParseTask

	// 运行状态
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// 统计
	stats Stats

	// 动态扩容控制
	scaleMu      sync.Mutex
	lastScaleUp  time.Time
	lastScaleDown time.Time

	// 信号量 (用于 backpressure)
	semaphore chan struct{}
}

// Config Worker Pool 配置
type Config struct {
	MinWorkers      int           // 最小 worker 数
	MaxWorkers      int           // 最大 worker 数
	QueueSize       int           // 任务队列大小
	PriorityQueueSize int         // 优先级队列大小
	BatchSize       int           // 批量处理大小
	BatchTimeout    time.Duration // 批量超时
	ScaleUpThreshold float64      // 扩容阈值 (队列使用率)
	ScaleDownThreshold float64    // 缩容阈值
	ScaleCooldown   time.Duration // 扩缩容冷却时间
	EnableBackpressure bool       // 启用背压
	MaxPendingTasks int           // 最大待处理任务数
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	cpuCount := runtime.NumCPU()
	return Config{
		MinWorkers:         cpuCount,
		MaxWorkers:         cpuCount * 4,
		QueueSize:          1024,
		PriorityQueueSize:  128,
		BatchSize:          32,
		BatchTimeout:       10 * time.Millisecond,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		ScaleCooldown:      30 * time.Second,
		EnableBackpressure: true,
		MaxPendingTasks:    10000,
	}
}

// Stats Worker Pool 统计
type Stats struct {
	ActiveWorkers   int32
	TotalWorkers    int32
	TasksSubmitted  uint64
	TasksProcessed  uint64
	TasksDropped    uint64
	TasksPriority   uint64
	QueueUtilization float64
	ScaleUpCount    uint64
	ScaleDownCount  uint64
}

// Worker 解析 Worker
type Worker struct {
	id       int
	pool     *WorkerPool
	parser   l7parser.Parser
	ctx      context.Context
	cancel   context.CancelFunc
	busy     atomic.Bool
	taskCount uint64
}

// NewWorkerPool 创建 Worker Pool
func NewWorkerPool(config Config) (*WorkerPool, error) {
	if config.MinWorkers <= 0 {
		config.MinWorkers = runtime.NumCPU()
	}
	if config.MaxWorkers < config.MinWorkers {
		config.MaxWorkers = config.MinWorkers
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		config:        config,
		taskQueue:     make(chan *l7parser.ParseTask, config.QueueSize),
		priorityQueue: make(chan *l7parser.ParseTask, config.PriorityQueueSize),
		ctx:           ctx,
		cancel:        cancel,
		semaphore:     make(chan struct{}, config.MaxPendingTasks),
	}

	// 创建初始 workers
	for i := 0; i < config.MinWorkers; i++ {
		w := &Worker{
			id:   i,
			pool: pool,
		}
		pool.workers = append(pool.workers, w)
	}

	return pool, nil
}

// Start 启动 Worker Pool
func (p *WorkerPool) Start() error {
	if p.running.Load() {
		return fmt.Errorf("worker pool already running")
	}

	p.running.Store(true)

	// 启动所有 worker
	for _, w := range p.workers {
		w.ctx, w.cancel = context.WithCancel(p.ctx)
		p.wg.Add(1)
		go w.run()
	}

	// 启动监控协程
	p.wg.Add(1)
	go p.monitor()

	return nil
}

// Stop 停止 Worker Pool
func (p *WorkerPool) Stop() error {
	if !p.running.Load() {
		return nil
	}

	p.running.Store(false)
	p.cancel()
	p.wg.Wait()

	// 关闭队列
	close(p.taskQueue)
	close(p.priorityQueue)

	return nil
}

// Submit 提交任务
func (p *WorkerPool) Submit(task *l7parser.ParseTask) error {
	return p.submit(task, false)
}

// SubmitPriority 提交高优先级任务
func (p *WorkerPool) SubmitPriority(task *l7parser.ParseTask) error {
	return p.submit(task, true)
}

func (p *WorkerPool) submit(task *l7parser.ParseTask, priority bool) error {
	if !p.running.Load() {
		return fmt.Errorf("worker pool not running")
	}

	p.stats.TasksSubmitted++

	// Backpressure 检查
	if p.config.EnableBackpressure {
		select {
		case p.semaphore <- struct{}{}:
			// 获取信号量，继续处理
		default:
			// 信号量已满，拒绝任务
			p.stats.TasksDropped++
			return l7parser.ErrQueueFull
		}
	}

	// 选择队列
	var queue chan *l7parser.ParseTask
	if priority {
		queue = p.priorityQueue
		p.stats.TasksPriority++
	} else {
		queue = p.taskQueue
	}

	// 提交任务
	select {
	case queue <- task:
		return nil
	default:
		// 队列满
		if p.config.EnableBackpressure {
			<-p.semaphore // 释放信号量
		}
		p.stats.TasksDropped++
		return l7parser.ErrQueueFull
	}
}

// GetStats 获取统计信息
func (p *WorkerPool) GetStats() Stats {
	p.stats.ActiveWorkers = int32(p.getActiveWorkerCount())
	p.stats.TotalWorkers = int32(len(p.workers))

	// 计算队列使用率
	queueLen := len(p.taskQueue)
	p.stats.QueueUtilization = float64(queueLen) / float64(p.config.QueueSize)

	return p.stats
}

// ScaleUp 扩容
func (p *WorkerPool) ScaleUp(count int) error {
	p.scaleMu.Lock()
	defer p.scaleMu.Unlock()

	// 检查冷却时间
	if time.Since(p.lastScaleUp) < p.config.ScaleCooldown {
		return nil
	}

	// 检查最大限制
	current := len(p.workers)
	if current >= p.config.MaxWorkers {
		return nil
	}

	// 计算实际扩容数量
	if current+count > p.config.MaxWorkers {
		count = p.config.MaxWorkers - current
	}

	// 创建新 workers
	for i := 0; i < count; i++ {
		w := &Worker{
			id:   current + i,
			pool: p,
		}
		w.ctx, w.cancel = context.WithCancel(p.ctx)
		p.workers = append(p.workers, w)
		p.wg.Add(1)
		go w.run()
	}

	p.lastScaleUp = time.Now()
	p.stats.ScaleUpCount++

	return nil
}

// ScaleDown 缩容
func (p *WorkerPool) ScaleDown(count int) error {
	p.scaleMu.Lock()
	defer p.scaleMu.Unlock()

	// 检查冷却时间
	if time.Since(p.lastScaleDown) < p.config.ScaleCooldown {
		return nil
	}

	// 检查最小限制
	current := len(p.workers)
	if current <= p.config.MinWorkers {
		return nil
	}

	// 计算实际缩容数量
	if current-count < p.config.MinWorkers {
		count = current - p.config.MinWorkers
	}

	// 停止多余的 workers
	stopped := 0
	for i := len(p.workers) - 1; i >= 0 && stopped < count; i-- {
		w := p.workers[i]
		if !w.busy.Load() {
			w.cancel()
			p.workers = p.workers[:i]
			stopped++
		}
	}

	if stopped > 0 {
		p.lastScaleDown = time.Now()
		p.stats.ScaleDownCount++
	}

	return nil
}

// getActiveWorkerCount 获取活跃 worker 数量
func (p *WorkerPool) getActiveWorkerCount() int {
	count := 0
	for _, w := range p.workers {
		if w.busy.Load() {
			count++
		}
	}
	return count
}

// monitor 监控协程
func (p *WorkerPool) monitor() {
	defer p.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.autoScale()
		}
	}
}

// autoScale 自动扩缩容
func (p *WorkerPool) autoScale() {
	queueUtil := float64(len(p.taskQueue)) / float64(p.config.QueueSize)
	activeWorkers := p.getActiveWorkerCount()
	totalWorkers := len(p.workers)

	// 扩容检查
	if queueUtil > p.config.ScaleUpThreshold && activeWorkers >= totalWorkers {
		// 队列使用率高且所有 worker 都忙，需要扩容
		p.ScaleUp(2)
	}

	// 缩容检查
	if queueUtil < p.config.ScaleDownThreshold && totalWorkers > p.config.MinWorkers {
		// 队列使用率低，可以缩容
		p.ScaleDown(1)
	}
}

// ============================================================================
// Worker 实现
// ============================================================================

func (w *Worker) run() {
	defer w.pool.wg.Done()

	batch := make([]*l7parser.ParseTask, 0, w.pool.config.BatchSize)
	ticker := time.NewTicker(w.pool.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			w.processBatch(batch)
			return

		case task := <-w.pool.priorityQueue:
			// 优先处理高优先级任务
			w.busy.Store(true)
			w.processTask(task)
			w.busy.Store(false)

		case task := <-w.pool.taskQueue:
			batch = append(batch, task)
			if len(batch) >= w.pool.config.BatchSize {
				w.busy.Store(true)
				w.processBatch(batch)
				w.busy.Store(false)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.busy.Store(true)
				w.processBatch(batch)
				w.busy.Store(false)
				batch = batch[:0]
			}
		}
	}
}

func (w *Worker) processBatch(tasks []*l7parser.ParseTask) {
	for _, task := range tasks {
		w.processTask(task)
	}
}

func (w *Worker) processTask(task *l7parser.ParseTask) {
	// 释放信号量
	if w.pool.config.EnableBackpressure {
		select {
		case <-w.pool.semaphore:
		default:
		}
	}

	// 执行任务
	// 注意: 实际的解析逻辑由调用方提供
	// 这里只是更新统计
	w.taskCount++
	w.pool.stats.TasksProcessed++
}

// SetParser 设置解析器
func (w *Worker) SetParser(parser l7parser.Parser) {
	w.parser = parser
}

// GetTaskCount 获取任务计数
func (w *Worker) GetTaskCount() uint64 {
	return w.taskCount
}

// IsBusy 检查是否忙碌
func (w *Worker) IsBusy() bool {
	return w.busy.Load()
}
