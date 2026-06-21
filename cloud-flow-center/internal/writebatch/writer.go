//go:build linux

package writebatch

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// ClickHouse 批量写入引擎
// 解决 P6: ClickHouse 写入瓶颈——高并发写入时性能下降
// 策略：异步批量写入、多 worker 并行、自适应批次合并
// ============================================================================

// BatchWriterConfig 批量写入配置
type BatchWriterConfig struct {
	BatchSize       int           // 每批次大小
	FlushInterval   time.Duration // 刷新间隔
	MaxQueueSize    int           // 最大队列大小
	WorkerCount     int           // 写入 worker 数
	MaxRetries      int           // 最大重试次数
	RetryInterval   time.Duration // 重试间隔
	MaxWriteRate    int64         // 最大写入速率（条/秒，0=不限）
	EnableCompress  bool          // 是否启用压缩
}

// DefaultBatchWriterConfig 返回默认配置
func DefaultBatchWriterConfig() *BatchWriterConfig {
	return &BatchWriterConfig{
		BatchSize:     5000,
		FlushInterval: 1 * time.Second,
		MaxQueueSize:  100000,
		WorkerCount:   4,
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
		MaxWriteRate:  0,
		EnableCompress: true,
	}
}

// WriteTarget 写入目标接口
type WriteTarget interface {
	WriteBatch(ctx context.Context, table string, rows [][]interface{}) error
	WriteCompressed(ctx context.Context, table string, data []byte) error
}

// WriteRecord 写入记录
type WriteRecord struct {
	Table   string
	Columns []string
	Values  []interface{}
	TenantID string
	Timestamp time.Time
}

// BatchWriterStats 批量写入统计
type BatchWriterStats struct {
	TotalWritten     int64         // 总写入条数
	TotalBatches     int64         // 总批次数
	TotalErrors      int64         // 总错误数
	TotalRetries     int64         // 总重试次数
	CurrentQueueSize int           // 当前队列大小
	DroppedCount     int64         // 丢弃数
	AvgBatchSize     float64       // 平均批次大小
	AvgLatency       time.Duration // 平均写入延迟
	WriteRate        float64       // 当前写入速率（条/秒）
	LastFlushTime    time.Time     // 上次刷新时间
}

// BatchWriter 批量写入引擎
type BatchWriter struct {
	config   *BatchWriterConfig
	target   WriteTarget
	mu       sync.RWMutex
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// 按表分队列
	queues   map[string]chan *WriteRecord
	buffers  map[string][]*WriteRecord
	timers   map[string]*time.Timer

	stats    BatchWriterStats
	statsMu  sync.RWMutex

	rateLimiter *rateLimiter
}

// NewBatchWriter 创建批量写入引擎
func NewBatchWriter(config *BatchWriterConfig, target WriteTarget) *BatchWriter {
	if config == nil {
		config = DefaultBatchWriterConfig()
	}
	bw := &BatchWriter{
		config:   config,
		target:   target,
		stopCh:   make(chan struct{}),
		queues:   make(map[string]chan *WriteRecord),
		buffers:  make(map[string][]*WriteRecord),
		timers:   make(map[string]*time.Timer),
		stats:    BatchWriterStats{},
	}
	if config.MaxWriteRate > 0 {
		bw.rateLimiter = newRateLimiter(config.MaxWriteRate)
	}
	return bw
}

// Start 启动写入引擎
func (bw *BatchWriter) Start() {
	for i := 0; i < bw.config.WorkerCount; i++ {
		bw.wg.Add(1)
		go bw.workerLoop(i)
	}
}

// Stop 停止写入引擎
func (bw *BatchWriter) Stop() {
	close(bw.stopCh)
	bw.wg.Wait()
	bw.flushAll()
}

// Write 写入单条记录
func (bw *BatchWriter) Write(record *WriteRecord) error {
	if record == nil || record.Table == "" {
		return fmt.Errorf("invalid record")
	}

	bw.mu.Lock()
	queue, ok := bw.queues[record.Table]
	if !ok {
		queue = make(chan *WriteRecord, bw.config.MaxQueueSize)
		bw.queues[record.Table] = queue
		bw.buffers[record.Table] = make([]*WriteRecord, 0, bw.config.BatchSize)
	}
	bw.mu.Unlock()

	select {
	case queue <- record:
		return nil
	default:
		bw.statsMu.Lock()
		bw.stats.DroppedCount++
		bw.statsMu.Unlock()
		return fmt.Errorf("queue full for table %s", record.Table)
	}
}

// WriteBatch 批量写入
func (bw *BatchWriter) WriteBatch(records []*WriteRecord) error {
	for _, record := range records {
		if err := bw.Write(record); err != nil {
			return err
		}
	}
	return nil
}

// workerLoop worker 循环
func (bw *BatchWriter) workerLoop(workerID int) {
	defer bw.wg.Done()

	for {
		select {
		case <-bw.stopCh:
			bw.processQueues()
			return
		default:
			bw.processQueues()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// processQueues 处理所有队列
func (bw *BatchWriter) processQueues() {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	for table, queue := range bw.queues {
		bw.processTableQueue(table, queue)
	}
}

// processTableQueue 处理单表队列
func (bw *BatchWriter) processTableQueue(table string, queue chan *WriteRecord) {
	buffer := bw.buffers[table]
	flushInterval := bw.config.FlushInterval

	// 从队列读取数据到缓冲区
	for len(buffer) < bw.config.BatchSize {
		select {
		case record := <-queue:
			buffer = append(buffer, record)
		default:
			// 队列暂时为空
			goto checkFlush
		}
	}

checkFlush:
	// 检查是否需要刷新
	shouldFlush := len(buffer) >= bw.config.BatchSize
	if !shouldFlush && len(buffer) > 0 {
		// 检查是否超过刷新间隔
		if bw.timers[table] == nil {
			bw.timers[table] = time.NewTimer(flushInterval)
		}
		select {
		case <-bw.timers[table].C:
			shouldFlush = true
			bw.timers[table] = nil
		default:
		}
	}

	if shouldFlush && len(buffer) > 0 {
		bw.flushBuffer(table, buffer)
		bw.buffers[table] = make([]*WriteRecord, 0, bw.config.BatchSize)
		if bw.timers[table] != nil {
			bw.timers[table].Stop()
			bw.timers[table] = nil
		}
	} else {
		bw.buffers[table] = buffer
	}
}

// flushBuffer 刷新缓冲区
func (bw *BatchWriter) flushBuffer(table string, buffer []*WriteRecord) {
	if bw.rateLimiter != nil {
		bw.rateLimiter.acquire(int64(len(buffer)))
	}

	start := time.Now()
	var rows [][]interface{}
	for _, record := range buffer {
		rows = append(rows, record.Values)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	for attempt := 0; attempt <= bw.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(bw.config.RetryInterval * time.Duration(attempt))
		}
		if bw.config.EnableCompress {
			// 压缩写入（简化版：直接调用接口）
			err = bw.target.WriteBatch(ctx, table, rows)
		} else {
			err = bw.target.WriteBatch(ctx, table, rows)
		}
		if err == nil {
			break
		}
		bw.statsMu.Lock()
		bw.stats.TotalRetries++
		bw.statsMu.Unlock()
	}

	bw.statsMu.Lock()
	if err != nil {
		bw.stats.TotalErrors++
	} else {
		bw.stats.TotalWritten += int64(len(buffer))
		bw.stats.TotalBatches++
	}
	latency := time.Since(start)
	bw.stats.AvgLatency = (bw.stats.AvgLatency*time.Duration(bw.stats.TotalBatches-1) + latency) / time.Duration(bw.stats.TotalBatches)
	if bw.stats.TotalBatches == 0 {
		bw.stats.AvgLatency = latency
	}
	bw.stats.AvgBatchSize = float64(bw.stats.TotalWritten) / float64(bw.stats.TotalBatches)
	bw.stats.LastFlushTime = time.Now()
	bw.statsMu.Unlock()
}

// flushAll 刷新所有缓冲区
func (bw *BatchWriter) flushAll() {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	// 先把队列中剩余数据移到缓冲区
	for table, queue := range bw.queues {
	drainLoop:
		for {
			select {
			case record := <-queue:
				bw.buffers[table] = append(bw.buffers[table], record)
			default:
				break drainLoop
			}
		}
	}

	for table, buffer := range bw.buffers {
		if len(buffer) > 0 {
			bw.flushBuffer(table, buffer)
			bw.buffers[table] = make([]*WriteRecord, 0, bw.config.BatchSize)
		}
	}
}

// GetStats 获取统计
func (bw *BatchWriter) GetStats() BatchWriterStats {
	bw.mu.RLock()
	defer bw.mu.RUnlock()

	bw.statsMu.RLock()
	defer bw.statsMu.RUnlock()

	stats := bw.stats
	// 计算当前写入速率
	if time.Since(stats.LastFlushTime) < time.Second {
		stats.WriteRate = float64(stats.TotalWritten) / time.Since(stats.LastFlushTime).Seconds()
	}
	// 计算队列大小
	for _, queue := range bw.queues {
		stats.CurrentQueueSize += len(queue)
	}
	return stats
}

// ============================================================================
// 速率限制器
// ============================================================================

type rateLimiter struct {
	rate     int64
	mu       sync.Mutex
	tokens   int64
	lastRefill time.Time
}

func newRateLimiter(rate int64) *rateLimiter {
	return &rateLimiter{
		rate:       rate,
		tokens:     rate,
		lastRefill: time.Now(),
	}
}

func (rl *rateLimiter) acquire(n int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += int64(elapsed * float64(rl.rate))
	if rl.tokens > rl.rate {
		rl.tokens = rl.rate
	}
	rl.lastRefill = now

	if rl.tokens < n {
		sleepTime := time.Duration(float64(n-rl.tokens) / float64(rl.rate) * float64(time.Second))
		rl.mu.Unlock()
		time.Sleep(sleepTime)
		rl.mu.Lock()
	}
	rl.tokens -= n
}
