// Package l7parser L7 协议解析引擎
//
// Engine 是 L7 协议解析的核心组件，负责:
//   - 管理解析器注册表
//   - 调度解析任务到 worker pool
//   - 处理 backpressure
//   - 维护流状态

package l7parser

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/l7parser/stream"
)

// Engine L7 协议解析引擎
type Engine struct {
	// 配置
	config *EngineConfig

	// 解析器注册表
	registry *Registry

	// Worker Pool
	workers []*parserWorker
	
	// 任务队列 (每个 worker 一个队列，避免竞争)
	queues []chan *ParseTask

	// 流管理器
	streamManager *stream.StreamManager

	// 统计
	stats   Stats
	statsMu sync.Mutex

	// 运行状态
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// 结果回调
	resultHandler func(*ParseResult)

	// 对象池 (复用 ParseTask)
	taskPool sync.Pool
}

// ParseTask 解析任务
type ParseTask struct {
	Packet   RawPacket
	Flow     interface{} // *flow.UnifiedFlow
	Callback func(*ParseResult, error)

	// 内部使用
	submitTime int64
	retryCount int
}

// parserWorker 解析 worker
type parserWorker struct {
	id        int
	engine    *Engine
	queue     chan *ParseTask
	parser    Parser
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewEngine 创建 L7 解析引擎
func NewEngine(config *EngineConfig) (*Engine, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 设置默认 worker 数量
	if config.WorkerNum <= 0 {
		config.WorkerNum = runtime.NumCPU()
	}

	ctx, cancel := context.WithCancel(context.Background())

	engine := &Engine{
		config:        config,
		registry:      GetRegistry(),
		queues:        make([]chan *ParseTask, config.WorkerNum),
		streamManager: stream.NewStreamManager(config.MaxStreams, config.MaxReasmSize, 65536, config.StreamTimeout),
		ctx:           ctx,
		cancel:        cancel,
		taskPool: sync.Pool{
			New: func() interface{} {
				return &ParseTask{}
			},
		},
	}

	// 初始化 worker 队列
	for i := 0; i < config.WorkerNum; i++ {
		engine.queues[i] = make(chan *ParseTask, config.QueueSize)
	}

	return engine, nil
}

// Start 启动引擎
func (e *Engine) Start() error {
	if e.running.Load() {
		return fmt.Errorf("engine already running")
	}

	// 初始化所有启用的解析器
	for _, pt := range e.config.EnabledParsers {
		parser, ok := e.registry.GetByType(pt)
		if !ok {
			return fmt.Errorf("parser not found for type: %v", pt)
		}
		e.workers = append(e.workers, &parserWorker{
			id:     len(e.workers),
			engine: e,
			queue:  e.queues[len(e.workers)%e.config.WorkerNum],
			parser: parser,
		})
	}

	// 启动 workers
	for _, w := range e.workers {
		w.ctx, w.cancel = context.WithCancel(e.ctx)
		e.wg.Add(1)
		go w.run()
	}

	e.running.Store(true)
	return nil
}

// Stop 停止引擎
func (e *Engine) Stop() error {
	if !e.running.Load() {
		return nil
	}

	e.cancel()
	e.wg.Wait()
	e.running.Store(false)
	return nil
}

// Submit 提交解析任务
// 如果队列满且启用 backpressure，返回 ErrQueueFull
func (e *Engine) Submit(packet RawPacket, flow interface{}) error {
	return e.SubmitWithCallback(packet, flow, nil)
}

// SubmitWithCallback 提交解析任务并设置回调
func (e *Engine) SubmitWithCallback(packet RawPacket, flow interface{}, callback func(*ParseResult, error)) error {
	if !e.running.Load() {
		return fmt.Errorf("engine not running")
	}

	// 从对象池获取任务
	task := e.taskPool.Get().(*ParseTask)
	task.Packet = packet
	task.Flow = flow
	task.Callback = callback
	task.submitTime = time.Now().UnixNano()
	task.retryCount = 0

	// 选择队列 (基于 flowID 的哈希，保证同一流的包进入同一队列)
	queueIdx := packet.Metadata.FlowID % uint64(e.config.WorkerNum)
	queue := e.queues[queueIdx]

	// 尝试提交
	select {
	case queue <- task:
		return nil
	default:
		// 队列满
		e.statsMu.Lock()
		e.stats.ErrorsQueueFull++
		e.statsMu.Unlock()
		e.taskPool.Put(task)

		if e.config.EnableBackpressure {
			return ErrQueueFull
		}

		// 不启用 backpressure，丢弃任务
		e.statsMu.Lock()
		e.stats.PacketsDropped++
		e.statsMu.Unlock()
		return nil
	}
}

// SubmitBatch 批量提交
func (e *Engine) SubmitBatch(packets []RawPacket, flows []interface{}) error {
	if len(packets) != len(flows) {
		return fmt.Errorf("packets and flows length mismatch")
	}

	for i := range packets {
		if err := e.Submit(packets[i], flows[i]); err != nil {
			return err
		}
	}
	return nil
}

// SetResultHandler 设置结果处理器
func (e *Engine) SetResultHandler(handler func(*ParseResult)) {
	e.resultHandler = handler
}

// GetStats 获取统计信息（线程安全快照）
func (e *Engine) GetStats() Stats {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	return e.stats
}

// DetectProtocol 检测协议
func (e *Engine) DetectProtocol(data []byte, dstPort uint16) (ParserType, float64) {
	return e.registry.Detect(data, dstPort)
}

// GetParser 获取解析器
func (e *Engine) GetParser(parserType ParserType) (Parser, bool) {
	return e.registry.GetByType(parserType)
}

// ============================================================================
// Worker 实现
// ============================================================================

func (w *parserWorker) run() {
	defer w.engine.wg.Done()

	batch := make([]*ParseTask, 0, w.engine.config.BatchSize)
	ticker := time.NewTicker(w.engine.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// 处理剩余任务
			w.processBatch(batch)
			return

		case task := <-w.queue:
			batch = append(batch, task)
			if len(batch) >= w.engine.config.BatchSize {
				w.processBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.processBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (w *parserWorker) processBatch(tasks []*ParseTask) {
	for _, task := range tasks {
		w.processTask(task)
	}
}

func (w *parserWorker) processTask(task *ParseTask) {
	start := time.Now()

	// 获取或创建流缓冲区
	flowID := task.Packet.Metadata.FlowID
	reasmBuf := w.engine.streamManager.GetOrCreate(flowID)

	// 添加数据到重组缓冲区
	if err := reasmBuf.Add(task.Packet.Metadata.SeqNum, task.Packet.Data, false); err != nil {
		w.engine.statsMu.Lock()
		w.engine.stats.ErrorsReasmFail++
		w.engine.statsMu.Unlock()
		if task.Callback != nil {
			task.Callback(nil, err)
		}
		w.engine.taskPool.Put(task)
		return
	}

	// 尝试获取连续数据
	var results []*ParseResult
	for {
		data, isLast := reasmBuf.GetNext()
		if data == nil {
			break
		}

		// 自动检测协议
		parserType, confidence := w.engine.DetectProtocol(data, task.Packet.Metadata.DstPort)
		if confidence < 0.5 {
			w.engine.statsMu.Lock()
			w.engine.stats.ErrorsUnknownProto++
			w.engine.statsMu.Unlock()
			continue
		}

		// 获取解析器
		parser, ok := w.engine.GetParser(parserType)
		if !ok {
			w.engine.statsMu.Lock()
			w.engine.stats.ErrorsParseFail++
			w.engine.statsMu.Unlock()
			continue
		}

		// 解析
		input := &ParserInput{
			Packet: RawPacket{
				Metadata: task.Packet.Metadata,
				Data:     data,
			},
		}

		result, _, err := parser.Parse(w.ctx, input, nil)
		if err != nil {
			w.engine.statsMu.Lock()
			w.engine.stats.ErrorsParseFail++
			w.engine.statsMu.Unlock()
			continue
		}

		if result != nil {
			results = append(results, result)
		}

		if isLast {
			break
		}
	}

	// 更新统计
	duration := time.Since(start).Nanoseconds()
	w.engine.statsMu.Lock()
	w.engine.stats.PacketsParsed++
	if uint64(duration) > w.engine.stats.MaxParseTimeNs {
		w.engine.stats.MaxParseTimeNs = uint64(duration)
	}
	w.engine.stats.AvgParseTimeNs = (w.engine.stats.AvgParseTimeNs*9 + uint64(duration)) / 10
	w.engine.statsMu.Unlock()

	// 回调
	for _, result := range results {
		if w.engine.resultHandler != nil {
			w.engine.resultHandler(result)
		}
		if task.Callback != nil {
			task.Callback(result, nil)
		}
	}

	// 回收任务
	w.engine.taskPool.Put(task)
}

// ============================================================================
// 便捷函数
// ============================================================================

// QuickParse 快速解析 (同步)
func QuickParse(data []byte, dstPort uint16) (*ParseResult, error) {
	// 自动检测协议
	parserType, confidence := DetectProtocol(data, dstPort)
	if confidence < 0.5 {
		return nil, ErrUnknownProtocol
	}

	// 获取解析器
	parser, ok := GetParser(parserType.String())
	if !ok {
		return nil, ErrParserNotFound
	}

	// 解析
	input := &ParserInput{
		Packet: RawPacket{
			Metadata: PacketMetadata{
				DstPort: dstPort,
			},
			Data: data,
		},
	}

	result, _, err := parser.Parse(context.Background(), input, nil)
	return result, err
}

// ParseWithType 使用指定协议类型解析
func ParseWithType(data []byte, parserType ParserType) (*ParseResult, error) {
	parser, ok := GetParser(parserType.String())
	if !ok {
		return nil, ErrParserNotFound
	}

	input := &ParserInput{
		Packet: RawPacket{
			Data: data,
		},
	}

	result, _, err := parser.Parse(context.Background(), input, nil)
	return result, err
}
