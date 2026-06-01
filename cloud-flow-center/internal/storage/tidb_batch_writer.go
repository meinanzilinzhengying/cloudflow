// Package storage 提供基于 TiDB 的数据持久化
// 本文件实现高性能批量写入引擎
// 目标：每批次1000条，写入速率≥5万条/秒
// 策略：异步批量通道 + 多协程并行写入 + 自适应批次合并 + 写入限速
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cloud-flow-center/pkg/logger"
	edge "cloud-flow/proto"
)

// ==================== 批量写入配置 ====================

// BatchWriterConfig 批量写入配置
type BatchWriterConfig struct {
	BatchSize       int           // 每批次大小（默认1000条）
	FlushInterval   time.Duration // 刷新间隔（默认1秒）
	QueueSize       int           // 写入队列大小（默认100000）
	WorkerCount     int           // 并行写入协程数（默认4）
	MaxRetries      int           // 最大重试次数（默认3）
	RetryInterval   time.Duration // 重试间隔（默认100ms）
	MaxWriteRate    int64         // 最大写入速率（条/秒，0=不限）
}

// DefaultBatchWriterConfig 返回默认配置
func DefaultBatchWriterConfig() *BatchWriterConfig {
	return &BatchWriterConfig{
		BatchSize:     1000,
		FlushInterval: 1 * time.Second,
		QueueSize:     100000,
		WorkerCount:   4,
		MaxRetries:    3,
		RetryInterval: 100 * time.Millisecond,
		MaxWriteRate:  0,
	}
}

// ==================== 批量写入统计 ====================

// BatchWriterStats 批量写入统计
type BatchWriterStats struct {
	TotalWritten   int64         // 总写入条数
	TotalBatches   int64         // 总批次数
	TotalErrors    int64         // 总错误数
	TotalRetries   int64         // 总重试次数
	WriteRate      float64       // 当前写入速率（条/秒）
	AvgBatchSize   float64       // 平均批次大小
	AvgBatchLatency time.Duration // 平均批次延迟
	MaxBatchLatency time.Duration // 最大批次延迟
	QueueLength    int           // 当前队列长度
	DroppedCount   int64         // 丢弃条数（队列满时）
}

// ==================== 写入任务 ====================

// WriteTask 写入任务
type WriteTask struct {
	TaskType string      // 任务类型：metrics/traces/profiling
	ProbeID  string      // 探针 ID
	Data     interface{} // 数据（[]*edge.MetricData / []*edge.TraceSpanData 等）
}

// ==================== 批量写入引擎 ====================

// BatchWriteEngine 批量写入引擎
type BatchWriteEngine struct {
	config *BatchWriterConfig
	db     *sql.DB
	logger *logger.Logger

	// 写入队列（按类型分通道）
	metricsQueue  chan *WriteTask
	tracesQueue   chan *WriteTask
	profilingQueue chan *WriteTask

	// 运行状态
	running bool
	mu      sync.RWMutex
	stopCh  chan struct{}
	wg      sync.WaitGroup

	// 统计
	stats      BatchWriterStats
	statsMu    sync.RWMutex
	lastCount  int64
	lastTime   time.Time
	rateTicker *time.Ticker

	// 限速器
	rateLimiter *WriteRateLimiter
}

// NewBatchWriteEngine 创建批量写入引擎
func NewBatchWriteEngine(db *sql.DB, config *BatchWriterConfig, log *logger.Logger) *BatchWriteEngine {
	if config == nil {
		config = DefaultBatchWriterConfig()
	}

	engine := &BatchWriteEngine{
		config:        config,
		db:            db,
		logger:        log,
		metricsQueue:  make(chan *WriteTask, config.QueueSize),
		tracesQueue:   make(chan *WriteTask, config.QueueSize),
		profilingQueue: make(chan *WriteTask, config.QueueSize),
		stopCh:        make(chan struct{}),
		lastTime:      time.Now(),
	}

	if config.MaxWriteRate > 0 {
		engine.rateLimiter = NewWriteRateLimiter(config.MaxWriteRate)
	}

	return engine
}

// Start 启动写入引擎
func (e *BatchWriteEngine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	// 启动速率统计
	e.wg.Add(1)
	go e.rateStatsLoop()

	// 启动 metrics 写入 worker
	for i := 0; i < e.config.WorkerCount; i++ {
		e.wg.Add(1)
		go e.metricsWorker(i)
	}

	// 启动 traces 写入 worker
	e.wg.Add(1)
	go e.tracesWorker()

	// 启动 profiling 写入 worker
	e.wg.Add(1)
	go e.profilingWorker()

	e.logger.Infof("批量写入引擎已启动: batchSize=%d, workers=%d, queueSize=%d",
		e.config.BatchSize, e.config.WorkerCount, e.config.QueueSize)
}

// Stop 停止写入引擎
func (e *BatchWriteEngine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopCh)
	e.mu.Unlock()

	e.wg.Wait()
	e.logger.Info("批量写入引擎已停止")
}

// Enqueue 投递写入任务
func (e *BatchWriteEngine) Enqueue(task *WriteTask) error {
	if !e.isRunning() {
		return fmt.Errorf("写入引擎未运行")
	}

	// 限速检查
	if e.rateLimiter != nil {
		e.rateLimiter.Wait()
	}

	switch task.TaskType {
	case "metrics":
		select {
		case e.metricsQueue <- task:
			return nil
		case <-e.stopCh:
			return fmt.Errorf("写入引擎已停止")
		default:
			// 队列满，丢弃并计数
			atomic.AddInt64(&e.stats.DroppedCount, 1)
			return fmt.Errorf("写入队列已满")
		}
	case "traces":
		select {
		case e.tracesQueue <- task:
			return nil
		case <-e.stopCh:
			return fmt.Errorf("写入引擎已停止")
		default:
			atomic.AddInt64(&e.stats.DroppedCount, 1)
			return fmt.Errorf("写入队列已满")
		}
	case "profiling":
		select {
		case e.profilingQueue <- task:
			return nil
		case <-e.stopCh:
			return fmt.Errorf("写入引擎已停止")
		default:
			atomic.AddInt64(&e.stats.DroppedCount, 1)
			return fmt.Errorf("写入队列已满")
		}
	default:
		return fmt.Errorf("未知的任务类型: %s", task.TaskType)
	}
}

// EnqueueMetrics 投递指标写入任务
func (e *BatchWriteEngine) EnqueueMetrics(probeID string, metrics []*edge.MetricData) error {
	return e.Enqueue(&WriteTask{TaskType: "metrics", ProbeID: probeID, Data: metrics})
}

// EnqueueTraces 投递追踪写入任务
func (e *BatchWriteEngine) EnqueueTraces(probeID string, spans []*edge.TraceSpanData) error {
	return e.Enqueue(&WriteTask{TaskType: "traces", ProbeID: probeID, Data: spans})
}

// EnqueueProfiling 投递剖析写入任务
func (e *BatchWriteEngine) EnqueueProfiling(probeID string, profiles []*edge.ProfilingData) error {
	return e.Enqueue(&WriteTask{TaskType: "profiling", ProbeID: probeID, Data: profiles})
}

// ==================== Worker 实现 ====================

// metricsWorker 指标写入 worker
func (e *BatchWriteEngine) metricsWorker(id int) {
	defer e.wg.Done()

	batch := make([]*edge.MetricData, 0, e.config.BatchSize)
	timer := time.NewTimer(e.config.FlushInterval)
	defer timer.Stop()

	for {
		select {
		case task, ok := <-e.metricsQueue:
			if !ok {
				// 通道关闭，刷新剩余数据
				e.flushMetricsBatch(batch)
				return
			}

			// 类型断言
			if data, ok := task.Data.([]*edge.MetricData); ok {
				batch = append(batch, data...)
			}

			// 批次满，执行写入
			if len(batch) >= e.config.BatchSize {
				e.flushMetricsBatch(batch)
				batch = batch[:0]
				// 重置定时器
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(e.config.FlushInterval)
			}

		case <-timer.C:
			// 定时刷新
			if len(batch) > 0 {
				e.flushMetricsBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(e.config.FlushInterval)

		case <-e.stopCh:
			// 停止前刷新剩余数据
			if len(batch) > 0 {
				e.flushMetricsBatch(batch)
			}
			return
		}
	}
}

// flushMetricsBatch 刷新指标批次
func (e *BatchWriteEngine) flushMetricsBatch(batch []*edge.MetricData) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	// 分批写入（每批1000条）
	const insertBatchSize = 1000
	for i := 0; i < len(batch); i += insertBatchSize {
		end := i + insertBatchSize
		if end > len(batch) {
			end = len(batch)
		}
		subBatch := batch[i:end]

		err := e.insertMetricsBatchOptimized(subBatch)
		if err != nil {
			atomic.AddInt64(&e.stats.TotalErrors, 1)
			e.logger.Warnf("批量写入指标失败: %v", err)

			// 重试
			for retry := 1; retry <= e.config.MaxRetries; retry++ {
				time.Sleep(e.config.RetryInterval)
				err = e.insertMetricsBatchOptimized(subBatch)
				if err == nil {
					break
				}
				atomic.AddInt64(&e.stats.TotalRetries, 1)
			}
			if err != nil {
				e.logger.Errorf("批量写入指标重试耗尽: %v", err)
			}
		}
	}

	duration := time.Since(start)

	// 更新统计
	atomic.AddInt64(&e.stats.TotalWritten, int64(len(batch)))
	atomic.AddInt64(&e.stats.TotalBatches, 1)
	e.updateLatencyStats(duration)
}

// insertMetricsBatchOptimized 优化的批量指标插入
// 使用 INSERT ... VALUES (...), (...), ... 多值语法，单次事务写入1000条
func (e *BatchWriteEngine) insertMetricsBatchOptimized(batch []*edge.MetricData) error {
	if len(batch) == 0 {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 构建批量 VALUES
	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*13)

	for _, m := range batch {
		valueStrings = append(valueStrings, "(?,?,?,?,?,?,?,?,?,?,?,?,?)")
		ts := time.Unix(m.Timestamp, 0)
		probeID := m.ProbeId
		if probeID == "" {
			probeID = "unknown"
		}
		cpuUsage := parseTagFloat(m.Tags, "cpu_usage")
		memoryUsage := parseTagFloat(m.Tags, "memory_usage")
		diskUsage := parseTagFloat(m.Tags, "disk_usage")

		valueArgs = append(valueArgs,
			probeID, ts, m.SrcIp, m.DstIp,
			m.SrcPort, m.DstPort, m.Protocol,
			m.Bytes, m.Packets, m.Latency,
			cpuUsage, memoryUsage, diskUsage,
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO metrics (probe_id, ts, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency, cpu_usage, memory_usage, disk_usage)
		 VALUES %s`, joinStrings(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("批量写入失败(size=%d): %w", len(batch), err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// tracesWorker 追踪写入 worker
func (e *BatchWriteEngine) tracesWorker() {
	defer e.wg.Done()

	batch := make([]*edge.TraceSpanData, 0, e.config.BatchSize)
	timer := time.NewTimer(e.config.FlushInterval)
	defer timer.Stop()

	for {
		select {
		case task, ok := <-e.tracesQueue:
			if !ok {
				e.flushTracesBatch(batch)
				return
			}
			if data, ok := task.Data.([]*edge.TraceSpanData); ok {
				batch = append(batch, data...)
			}
			if len(batch) >= e.config.BatchSize {
				e.flushTracesBatch(batch)
				batch = batch[:0]
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(e.config.FlushInterval)
			}

		case <-timer.C:
			if len(batch) > 0 {
				e.flushTracesBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(e.config.FlushInterval)

		case <-e.stopCh:
			if len(batch) > 0 {
				e.flushTracesBatch(batch)
			}
			return
		}
	}
}

// flushTracesBatch 刷新追踪批次
func (e *BatchWriteEngine) flushTracesBatch(batch []*edge.TraceSpanData) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	const insertBatchSize = 1000
	for i := 0; i < len(batch); i += insertBatchSize {
		end := i + insertBatchSize
		if end > len(batch) {
			end = len(batch)
		}
		subBatch := batch[i:end]

		err := e.insertTracesBatchOptimized(subBatch)
		if err != nil {
			atomic.AddInt64(&e.stats.TotalErrors, 1)
			for retry := 1; retry <= e.config.MaxRetries; retry++ {
				time.Sleep(e.config.RetryInterval)
				if err = e.insertTracesBatchOptimized(subBatch); err == nil {
					break
				}
				atomic.AddInt64(&e.stats.TotalRetries, 1)
			}
		}
	}

	duration := time.Since(start)
	atomic.AddInt64(&e.stats.TotalWritten, int64(len(batch)))
	atomic.AddInt64(&e.stats.TotalBatches, 1)
	e.updateLatencyStats(duration)
}

// insertTracesBatchOptimized 优化的批量追踪插入
func (e *BatchWriteEngine) insertTracesBatchOptimized(batch []*edge.TraceSpanData) error {
	if len(batch) == 0 {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*4)

	for _, span := range batch {
		valueStrings = append(valueStrings, "(?,?,?,?)")
		ts := span.StartTime
		if ts == 0 {
			ts = time.Now().Unix()
		}
		probeID := span.ProbeId
		if probeID == "" {
			probeID = "unknown"
		}
		spanMap := map[string]interface{}{
			"trace_id": span.TraceId, "span_id": span.SpanId,
			"parent_id": span.ParentId, "service": span.Service,
			"operation": span.Operation, "start_time": span.StartTime,
			"end_time": span.EndTime, "duration": span.Duration,
			"status": span.Status, "tags": span.Tags,
		}
		payload, err := json.Marshal(spanMap)
		if err != nil {
			e.logger.Warnf("json marshal span failed: %v", err)
			continue
		}
		spanID := span.SpanId
		if spanID == "" {
			spanID, _ = generateUUID()
		}
		valueArgs = append(valueArgs, probeID, ts, string(payload), spanID)
	}

	query := fmt.Sprintf(
		`INSERT INTO traces (probe_id, ts, payload, span_id) VALUES %s`,
		joinStrings(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("批量写入追踪失败(size=%d): %w", len(batch), err)
	}

	return tx.Commit()
}

// profilingWorker 剖析写入 worker
func (e *BatchWriteEngine) profilingWorker() {
	defer e.wg.Done()

	batch := make([]*edge.ProfilingData, 0, e.config.BatchSize)
	timer := time.NewTimer(e.config.FlushInterval)
	defer timer.Stop()

	for {
		select {
		case task, ok := <-e.profilingQueue:
			if !ok {
				e.flushProfilingBatch(batch)
				return
			}
			if data, ok := task.Data.([]*edge.ProfilingData); ok {
				batch = append(batch, data...)
			}
			if len(batch) >= e.config.BatchSize {
				e.flushProfilingBatch(batch)
				batch = batch[:0]
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(e.config.FlushInterval)
			}

		case <-timer.C:
			if len(batch) > 0 {
				e.flushProfilingBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(e.config.FlushInterval)

		case <-e.stopCh:
			if len(batch) > 0 {
				e.flushProfilingBatch(batch)
			}
			return
		}
	}
}

// flushProfilingBatch 刷新剖析批次
func (e *BatchWriteEngine) flushProfilingBatch(batch []*edge.ProfilingData) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	const insertBatchSize = 1000
	for i := 0; i < len(batch); i += insertBatchSize {
		end := i + insertBatchSize
		if end > len(batch) {
			end = len(batch)
		}
		subBatch := batch[i:end]

		err := e.insertProfilingBatchOptimized(subBatch)
		if err != nil {
			atomic.AddInt64(&e.stats.TotalErrors, 1)
			for retry := 1; retry <= e.config.MaxRetries; retry++ {
				time.Sleep(e.config.RetryInterval)
				if err = e.insertProfilingBatchOptimized(subBatch); err == nil {
					break
				}
				atomic.AddInt64(&e.stats.TotalRetries, 1)
			}
		}
	}

	duration := time.Since(start)
	atomic.AddInt64(&e.stats.TotalWritten, int64(len(batch)))
	atomic.AddInt64(&e.stats.TotalBatches, 1)
	e.updateLatencyStats(duration)
}

// insertProfilingBatchOptimized 优化的批量剖析插入
func (e *BatchWriteEngine) insertProfilingBatchOptimized(batch []*edge.ProfilingData) error {
	if len(batch) == 0 {
		return nil
	}

	tx, err := e.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	valueStrings := make([]string, 0, len(batch))
	valueArgs := make([]interface{}, 0, len(batch)*4)

	for _, profile := range batch {
		valueStrings = append(valueStrings, "(?,?,?,?)")
		ts := time.Now().Unix()
		probeID := profile.ProbeId
		if probeID == "" {
			probeID = "unknown"
		}
		profileMap := map[string]interface{}{
			"type": profile.Type, "stack": profile.Stack,
			"count": profile.Count, "total_time": profile.TotalTime,
			"labels": profile.Labels,
		}
		payload, err := json.Marshal(profileMap)
		if err != nil {
			e.logger.Warnf("json marshal profile failed: %v", err)
			continue
		}
		valueArgs = append(valueArgs, probeID, ts, string(payload), profile.Type)
	}

	query := fmt.Sprintf(
		`INSERT INTO profiling (probe_id, ts, payload, type) VALUES %s`,
		joinStrings(valueStrings, ","))

	_, err = tx.Exec(query, valueArgs...)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("批量写入剖析失败(size=%d): %w", len(batch), err)
	}

	return tx.Commit()
}

// ==================== 速率统计 ====================

// rateStatsLoop 速率统计循环
func (e *BatchWriteEngine) rateStatsLoop() {
	defer e.wg.Done()
	e.rateTicker = time.NewTicker(1 * time.Second)
	defer e.rateTicker.Stop()

	for {
		select {
		case <-e.rateTicker.C:
			e.calculateWriteRate()
		case <-e.stopCh:
			return
		}
	}
}

// calculateWriteRate 计算写入速率
func (e *BatchWriteEngine) calculateWriteRate() {
	now := time.Now()
	current := atomic.LoadInt64(&e.stats.TotalWritten)
	prev := e.lastCount

	elapsed := now.Sub(e.lastTime).Seconds()
	if elapsed > 0 {
		rate := float64(current-prev) / elapsed
		e.statsMu.Lock()
		e.stats.WriteRate = rate
		e.statsMu.Unlock()
	}

	e.lastCount = current
	e.lastTime = now
}

// updateLatencyStats 更新延迟统计
func (e *BatchWriteEngine) updateLatencyStats(d time.Duration) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()

	total := atomic.LoadInt64(&e.stats.TotalBatches)
	if total > 0 {
		e.stats.AvgBatchLatency = time.Duration(
			(e.stats.AvgBatchLatency.Nanoseconds()*int64(total-1) + d.Nanoseconds()) / int64(total),
		)
	}
	if d > e.stats.MaxBatchLatency {
		e.stats.MaxBatchLatency = d
	}
}

// ==================== 外部接口 ====================

// isRunning 检查是否运行中
func (e *BatchWriteEngine) isRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetStats 获取写入统计
func (e *BatchWriteEngine) GetStats() BatchWriterStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()

	stats := e.stats
	stats.QueueLength = len(e.metricsQueue) + len(e.tracesQueue) + len(e.profilingQueue)
	return stats
}

// GetWriteRate 获取当前写入速率
func (e *BatchWriteEngine) GetWriteRate() float64 {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()
	return e.stats.WriteRate
}

// ==================== 写入限速器 ====================

// WriteRateLimiter 写入限速器（令牌桶）
type WriteRateLimiter struct {
	rate    int64         // 每秒令牌数
	tokens  int64         // 当前令牌
	lastRefill time.Time
	mu      sync.Mutex
}

// NewWriteRateLimiter 创建写入限速器
func NewWriteRateLimiter(rate int64) *WriteRateLimiter {
	return &WriteRateLimiter{
		rate:    rate,
		tokens:  rate,
		lastRefill: time.Now(),
	}
}

// Wait 等待令牌
func (r *WriteRateLimiter) Wait() {
	r.mu.Lock()
	r.refill()
	if r.tokens > 0 {
		r.tokens--
		r.mu.Unlock()
		return
	}
	waitTime := time.Second / time.Duration(r.rate)
	r.mu.Unlock()

	time.Sleep(waitTime)

	r.mu.Lock()
	r.tokens = 0
	r.mu.Unlock()
}

// refill 补充令牌
func (r *WriteRateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	if elapsed >= time.Second {
		r.tokens = r.rate
		r.lastRefill = now
	}
}

// ==================== 辅助函数 ====================

// joinStrings 连接字符串切片（避免 strings.Join 的额外分配）
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	totalLen := 0
	for _, p := range parts {
		totalLen += len(p)
	}
	totalLen += len(sep) * (len(parts) - 1)

	result := make([]byte, 0, totalLen)
	result = append(result, parts[0]...)
	for _, p := range parts[1:] {
		result = append(result, sep...)
		result = append(result, p...)
	}
	return string(result)
}
