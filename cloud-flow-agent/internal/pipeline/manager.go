//go:build linux

package pipeline

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 一、数据管道管理器（统一数据流入口）
// ============================================================================

// PipelineManager 管道管理器
type PipelineManager struct {
	mu sync.RWMutex

	// 配置
	config *PipelineConfig

	// 预处理管道
	preprocess *PreprocessPipeline

	// 输入通道
	inputCh   chan *DataRecord

	// 输出通道（批量）
	outputCh  chan *DataBatch

	// 工作线程
	workers   []*pipelineWorker
	wg        sync.WaitGroup

	// 状态
	running   atomic.Bool
	stopCh    chan struct{}

	// 统计
	stats     *PipelineStats
}

// PipelineStats 管道统计
type PipelineStats struct {
	RecordsReceived int64 `json:"records_received"`
	RecordsDropped  int64 `json:"records_dropped"`
	RecordsPassed   int64 `json:"records_passed"`
	BatchesCreated  int64 `json:"batches_created"`
	BatchesSent     int64 `json:"batches_sent"`
	Errors          int64 `json:"errors"`
}

// pipelineWorker 工作线程
type pipelineWorker struct {
	id        int
	manager   *PipelineManager
	batch     *DataBatch
	ticker    *time.Ticker
}

// NewPipelineManager 创建管道管理器
func NewPipelineManager(config *PipelineConfig) (*PipelineManager, error) {
	if config == nil {
		config = DefaultPipelineConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}

	pm := &PipelineManager{
		config:     config,
		preprocess: NewPreprocessPipeline(),
		inputCh:    make(chan *DataRecord, config.BufferSize),
		outputCh:   make(chan *DataBatch, config.OutputBuffer),
		stopCh:     make(chan struct{}),
		stats:      &PipelineStats{},
	}

	// 初始化默认预处理器
	if config.EnableFilter {
		pm.preprocess.Add(NewFilterPreprocessor([]*FilterRule{}))
	}
	if config.EnableSample && config.SampleRate < 1.0 {
		pm.preprocess.Add(NewSamplePreprocessor(config.SampleRate, SampleStrategyRandom))
	}
	if config.EnableEnrich {
		enrich := NewEnrichPreprocessor()
		enrich.AddEnricher(AddTimeEnricher())
		pm.preprocess.Add(enrich)
	}

	return pm, nil
}

// Start 启动管道
func (pm *PipelineManager) Start() error {
	if pm.running.Load() {
		return fmt.Errorf("pipeline already running")
	}
	pm.running.Store(true)

	// 启动工作线程
	for i := 0; i < pm.config.WorkerCount; i++ {
		worker := &pipelineWorker{
			id:     i,
			manager: pm,
			batch:  NewDataBatch(DataTypeFlow),
			ticker: time.NewTicker(pm.config.BatchTimeout),
		}
		pm.workers = append(pm.workers, worker)
		pm.wg.Add(1)
		go worker.run()
	}

	return nil
}

// Stop 停止管道
func (pm *PipelineManager) Stop() {
	if !pm.running.Load() {
		return
	}
	pm.running.Store(false)

	close(pm.stopCh)
	close(pm.inputCh)

	pm.wg.Wait()

	// 关闭输出通道
	close(pm.outputCh)
}

// Submit 提交单条记录
func (pm *PipelineManager) Submit(record *DataRecord) error {
	if !pm.running.Load() {
		return fmt.Errorf("pipeline not running")
	}

	select {
	case pm.inputCh <- record:
		atomic.AddInt64(&pm.stats.RecordsReceived, 1)
		return nil
	case <-pm.stopCh:
		return fmt.Errorf("pipeline stopped")
	default:
		atomic.AddInt64(&pm.stats.Errors, 1)
		return fmt.Errorf("input buffer full")
	}
}

// SubmitBatch 提交批次
func (pm *PipelineManager) SubmitBatch(batch *DataBatch) error {
	if !pm.running.Load() {
		return fmt.Errorf("pipeline not running")
	}

	for _, record := range batch.Records {
		if err := pm.Submit(record); err != nil {
			return err
		}
	}
	return nil
}

// OutputChannel 获取输出通道
func (pm *PipelineManager) OutputChannel() <-chan *DataBatch {
	return pm.outputCh
}

// SetPreprocessPipeline 设置自定义预处理管道
func (pm *PipelineManager) SetPreprocessPipeline(p *PreprocessPipeline) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.preprocess = p
}

// AddPreprocessor 添加预处理器
func (pm *PipelineManager) AddPreprocessor(p Preprocessor) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.preprocess.Add(p)
}

// GetStats 获取统计信息
func (pm *PipelineManager) GetStats() *PipelineStats {
	return &PipelineStats{
		RecordsReceived: atomic.LoadInt64(&pm.stats.RecordsReceived),
		RecordsDropped:  atomic.LoadInt64(&pm.stats.RecordsDropped),
		RecordsPassed:   atomic.LoadInt64(&pm.stats.RecordsPassed),
		BatchesCreated:  atomic.LoadInt64(&pm.stats.BatchesCreated),
		BatchesSent:     atomic.LoadInt64(&pm.stats.BatchesSent),
		Errors:          atomic.LoadInt64(&pm.stats.Errors),
	}
}

// ResetStats 重置统计
func (pm *PipelineManager) ResetStats() {
	atomic.StoreInt64(&pm.stats.RecordsReceived, 0)
	atomic.StoreInt64(&pm.stats.RecordsDropped, 0)
	atomic.StoreInt64(&pm.stats.RecordsPassed, 0)
	atomic.StoreInt64(&pm.stats.BatchesCreated, 0)
	atomic.StoreInt64(&pm.stats.BatchesSent, 0)
	atomic.StoreInt64(&pm.stats.Errors, 0)
}

// ============================================================================
// 二、工作线程实现
// ============================================================================

func (w *pipelineWorker) run() {
	defer w.manager.wg.Done()
	defer w.ticker.Stop()

	for {
		select {
		case <-w.manager.stopCh:
			w.flush()
			return

		case record, ok := <-w.manager.inputCh:
			if !ok {
				w.flush()
				return
			}
			w.processRecord(record)

		case <-w.ticker.C:
			w.flush()
		}
	}
}

func (w *pipelineWorker) processRecord(record *DataRecord) {
	// 1. 预处理
	if err := w.manager.preprocess.Process(record); err != nil {
		atomic.AddInt64(&w.manager.stats.Errors, 1)
		record.Dropped = true
		record.DropReason = err.Error()
	}

	if record.Dropped {
		atomic.AddInt64(&w.manager.stats.RecordsDropped, 1)
		return
	}

	atomic.AddInt64(&w.manager.stats.RecordsPassed, 1)

	// 2. 加入批次
	w.batch.Add(record)

	// 3. 检查是否需要发送
	if w.batch.RecordCount >= w.manager.config.BatchSize ||
		w.batch.TotalSize >= w.manager.config.MaxBatchSize {
		w.flush()
	}
}

func (w *pipelineWorker) flush() {
	if w.batch.IsEmpty() {
		return
	}

	// 发送批次
	select {
	case w.manager.outputCh <- w.batch:
		atomic.AddInt64(&w.manager.stats.BatchesSent, 1)
	case <-w.manager.stopCh:
		// 管道已停止，丢弃
		return
	default:
		atomic.AddInt64(&w.manager.stats.Errors, 1)
		return
	}

	atomic.AddInt64(&w.manager.stats.BatchesCreated, 1)

	// 创建新批次
	w.batch = NewDataBatch(DataTypeFlow)
}

// ============================================================================
// 三、统一数据流写入器（消除重复路径）
// ============================================================================

// UnifiedWriter 统一数据流写入器
type UnifiedWriter struct {
	mu        sync.RWMutex
	pm        *PipelineManager
	writers   []BatchWriter
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// BatchWriter 批次写入器接口
type BatchWriter interface {
	Write(batch *DataBatch) error
	Close() error
}

// NewUnifiedWriter 创建统一写入器
func NewUnifiedWriter(pm *PipelineManager) *UnifiedWriter {
	return &UnifiedWriter{
		pm:      pm,
		writers: make([]BatchWriter, 0),
		stopCh:  make(chan struct{}),
	}
}

// AddWriter 添加写入器
func (uw *UnifiedWriter) AddWriter(w BatchWriter) {
	uw.mu.Lock()
	defer uw.mu.Unlock()
	uw.writers = append(uw.writers, w)
}

// Start 启动写入器
func (uw *UnifiedWriter) Start() {
	uw.wg.Add(1)
	go uw.writeLoop()
}

// Stop 停止写入器
func (uw *UnifiedWriter) Stop() {
	close(uw.stopCh)
	uw.wg.Wait()

	uw.mu.Lock()
	defer uw.mu.Unlock()
	for _, w := range uw.writers {
		w.Close()
	}
}

func (uw *UnifiedWriter) writeLoop() {
	defer uw.wg.Done()

	outputCh := uw.pm.OutputChannel()
	for {
		select {
		case <-uw.stopCh:
			return
		case batch, ok := <-outputCh:
			if !ok {
				return
			}
			uw.writeBatch(batch)
		}
	}
}

func (uw *UnifiedWriter) writeBatch(batch *DataBatch) {
	uw.mu.RLock()
	writers := make([]BatchWriter, len(uw.writers))
	copy(writers, uw.writers)
	uw.mu.RUnlock()

	for _, w := range writers {
		if err := w.Write(batch); err != nil {
			// 记录错误但不阻塞其他写入器
			_ = err
		}
	}
}

// ============================================================================
// 四、模拟写入器（用于测试）
// ============================================================================

// MockWriter 模拟写入器
type MockWriter struct {
	mu       sync.RWMutex
	batches  []*DataBatch
	closed   bool
}

// NewMockWriter 创建模拟写入器
func NewMockWriter() *MockWriter {
	return &MockWriter{
		batches: make([]*DataBatch, 0),
	}
}

// Write 写入批次
func (mw *MockWriter) Write(batch *DataBatch) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	if mw.closed {
		return fmt.Errorf("writer closed")
	}
	mw.batches = append(mw.batches, batch)
	return nil
}

// Close 关闭写入器
func (mw *MockWriter) Close() error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.closed = true
	return nil
}

// GetBatches 获取所有批次
func (mw *MockWriter) GetBatches() []*DataBatch {
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	result := make([]*DataBatch, len(mw.batches))
	copy(result, mw.batches)
	return result
}

// GetRecordCount 获取记录总数
func (mw *MockWriter) GetRecordCount() int {
	mw.mu.RLock()
	defer mw.mu.RUnlock()
	count := 0
	for _, b := range mw.batches {
		count += b.RecordCount
	}
	return count
}

// ============================================================================
// 五、架构职责说明（文档）
// ============================================================================

/*
统一数据流架构（P11 修复）:

【问题】
1. 数据路径不唯一：Agent → Edge → ClickHouse 和 Agent → Kafka → Center → Storage 两条路径
2. Edge 和 Center 职责重叠：Edge 直接写 ClickHouse，Center 也写存储
3. 多次序列化：JSON 序列化/反序列化在 Edge、Kafka、Center 各发生一次

【修复方案】
┌─────────────┐     ┌─────────────────────────────────────────┐     ┌──────────────┐
│   Agent     │────▶│  Edge (数据接收 + 预处理管道)            │────▶│  统一存储     │
│             │     │  - 接收 Agent 数据                       │     │  ClickHouse  │
│  protobuf   │     │  - 过滤 → 采样 → 富化 → 聚合             │     │  Kafka       │
│  二进制传输  │     │  - 统一批次写入                          │     │  TiDB        │
└─────────────┘     └─────────────────────────────────────────┘     └──────────────┘

【职责边界】
- Edge: 数据接收、预处理、统一写入（不再直接写 ClickHouse）
- Center: 数据查询、告警、可视化（不再处理数据写入）
- Kafka: 可选的数据持久化/回放（不是主要路径）

【序列化优化】
- 传输层：protobuf 二进制（替代 JSON）
- 预处理：内存中处理，避免序列化
- 写入：批量写入，减少 I/O 次数

【数据路径】
Agent → protobuf → Edge → 预处理管道 → 统一写入 → Storage
  ↑                                          ↓
  └──────────── 唯一路径 ────────────────────┘
*/
