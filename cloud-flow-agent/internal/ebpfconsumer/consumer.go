// Package ebpfconsumer 提供高性能 eBPF userspace consumer
//
// 架构: kernel ringbuffer -> dispatcher -> worker pool -> flow parser
//
// 核心特性:
//   - 无锁队列: MPMC lock-free ringbuffer
//   - 对象池: sync.Pool 复用 RawEvent/ParsedFlow
//   - CPU 亲和: dispatcher 和 workers 绑定到独立 CPU
//   - 批量处理: 批量读取/解析/输出
//   - Zero-copy: unsafe 减少内存拷贝
//
// 性能目标:
//   - 单节点 500k event/s
//   - perf lost < 0.01%
//   - gc pause < 10ms
//
//go:build linux
package ebpfconsumer

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf/perf"

	"cloud-flow-agent/internal/ebpfconsumer/dispatcher"
	"cloud-flow-agent/internal/ebpfconsumer/parser"
	"cloud-flow-agent/internal/ebpfconsumer/pool"
	"cloud-flow-agent/internal/ebpfconsumer/ringbuffer"
	"cloud-flow-agent/internal/ebpfconsumer/worker"
	edge "cloud-flow/proto"
)

// Config consumer 配置
type Config struct {
	// RingBuffer 配置
	RingBufferSize  int  // perf ringbuffer 大小 (页数，必须是 2 的幂)
	
	// Dispatcher 配置
	DispatcherConfig dispatcher.Config
	
	// Worker Pool 配置
	WorkerConfig worker.Config
	
	// 输出配置
	OutputChanSize int           // 输出通道大小
	FlushInterval  time.Duration // 刷新间隔
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		RingBufferSize:  8192, // 8192 页 ≈ 32MB
		DispatcherConfig: dispatcher.DefaultConfig(),
		WorkerConfig:     worker.DefaultConfig(),
		OutputChanSize:   1000,
		FlushInterval:    100 * time.Millisecond,
	}
}

// Consumer eBPF 消费者
type Consumer struct {
	config Config
	
	// 组件
	pool       *pool.Pool
	dispatcher *dispatcher.Dispatcher
	workerPool *worker.Pool
	parser     *parser.Parser
	
	// 队列
	dispatchQueue *ringbuffer.RingBuffer // dispatcher -> worker pool
	
	// 输出
	outputCh   chan []*edge.MetricData
	
	// 控制
	stopCh     chan struct{}
	stopped    int32
	wg         sync.WaitGroup
	
	// 统计
	stats      Stats
}

// Stats consumer 统计
type Stats struct {
	EventsConsumed uint64
	FlowsParsed    uint64
	BatchesOutput  uint64
	Errors         uint64
}

// New 创建新的 consumer
func New(perfReader *perf.Reader, config Config) *Consumer {
	// 创建对象池
	p := pool.New()
	
	// 创建 dispatch queue (dispatcher -> worker pool)
	dispatchQueue := ringbuffer.New(65536) // 64k 容量
	
	// 创建 consumer 实例
	c := &Consumer{
		config:        config,
		pool:          p,
		dispatchQueue: dispatchQueue,
		outputCh:      make(chan []*edge.MetricData, config.OutputChanSize),
		stopCh:        make(chan struct{}),
		parser:        parser.New(),
	}
	
	// 创建 worker pool
	workerHandler := func(events []*pool.RawEvent, flows []*pool.ParsedFlow, count int) {
		c.handleWorkerOutput(events, flows, count)
	}
	c.workerPool = worker.NewPool(config.WorkerConfig, workerHandler, p)
	
	// 创建 dispatcher
	perfAdapter := dispatcher.NewPerfBufferAdapter(perfReader)
	c.dispatcher = dispatcher.New(config.DispatcherConfig, perfAdapter, dispatchQueue, p)
	
	return c
}

// Start 启动 consumer
func (c *Consumer) Start() {
	// 启动 worker pool
	c.workerPool.Start()
	
	// 启动 dispatcher
	c.dispatcher.Start()
	
	// 启动输出协程
	c.wg.Add(1)
	go c.outputLoop()
	
	// 启动统计协程
	c.wg.Add(1)
	go c.statsLoop()
}

// Stop 停止 consumer
func (c *Consumer) Stop() {
	atomic.StoreInt32(&c.stopped, 1)
	close(c.stopCh)
	
	// 停止 dispatcher
	c.dispatcher.Stop()
	
	// 停止 worker pool
	c.workerPool.Stop()
	
	// 等待输出协程
	c.wg.Wait()
	
	// 关闭输出通道
	close(c.outputCh)
}

// Output 获取输出通道
func (c *Consumer) Output() <-chan []*edge.MetricData {
	return c.outputCh
}

// handleWorkerOutput 处理 worker 输出
// 将 ParsedFlow 转换为 edge.MetricData
func (c *Consumer) handleWorkerOutput(events []*pool.RawEvent, flows []*pool.ParsedFlow, count int) {
	metrics := make([]*edge.MetricData, 0, count)
	
	for i := 0; i < count; i++ {
		flow := flows[i]
		
		// 转换为 edge.MetricData
		metric := c.flowToMetric(flow)
		if metric != nil {
			metrics = append(metrics, metric)
		}
		
		// 归还 ParsedFlow 到对象池
		c.pool.PutParsedFlow(flow)
	}
	
	if len(metrics) > 0 {
		select {
		case c.outputCh <- metrics:
			atomic.AddUint64(&c.stats.BatchesOutput, 1)
		case <-time.After(time.Second):
			// 输出通道满，丢弃
			atomic.AddUint64(&c.stats.Errors, 1)
		}
	}
	
	atomic.AddUint64(&c.stats.FlowsParsed, uint64(count))
}

// flowToMetric 将 ParsedFlow 转换为 edge.MetricData
func (c *Consumer) flowToMetric(flow *pool.ParsedFlow) *edge.MetricData {
	if flow == nil {
		return nil
	}
	
	metric := &edge.MetricData{
		Timestamp: int64(flow.Timestamp),
		SrcIp:     ipToString(flow.SrcIP),
		DstIp:     ipToString(flow.DstIP),
		SrcPort:   int32(flow.SrcPort),
		DstPort:   int32(flow.DstPort),
		Protocol:  protocolToString(flow.Protocol),
		Bytes:     int64(flow.Bytes),
		Packets:   int64(flow.Packets),
		Latency:   int64(flow.LatencyNs),
		Tags:      make(map[string]string),
	}
	
	// 添加协议特定标签
	if flow.Protocol == 6 { // TCP
		metric.Tags["tcp_flags"] = formatTCPFlags(flow.TCPFlags)
	}
	
	if flow.HTTPMethod > 0 {
		metric.Tags["http_method"] = httpMethodToString(flow.HTTPMethod)
		metric.Tags["http_status"] = formatUint16(flow.HTTPStatus)
	}
	
	if flow.DNSQueryType > 0 {
		metric.Tags["dns_query_type"] = formatUint16(flow.DNSQueryType)
	}
	
	if flow.MySQLCmd > 0 {
		metric.Tags["mysql_cmd"] = formatUint8(flow.MySQLCmd)
	}
	
	metric.Tags["cpu"] = formatUint8(flow.CPU)
	metric.Tags["seq"] = formatUint64(flow.Seq)
	
	return metric
}

// outputLoop 输出协程
func (c *Consumer) outputLoop() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(c.config.FlushInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			// 定期刷新 (如果需要)
		}
	}
}

// statsLoop 统计协程
func (c *Consumer) statsLoop() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.logStats()
		}
	}
}

// logStats 记录统计信息
func (c *Consumer) logStats() {
	stats := c.GetStats()
	dispatcherStats := c.dispatcher.GetStats()
	workerStats := c.workerPool.Stats()
	poolStats := c.pool.Stats()
	
	// 这里可以输出到日志或 metrics
	_ = stats
	_ = dispatcherStats
	_ = workerStats
	_ = poolStats
}

// GetStats 获取统计信息
func (c *Consumer) GetStats() Stats {
	return Stats{
		EventsConsumed: atomic.LoadUint64(&c.stats.EventsConsumed),
		FlowsParsed:    atomic.LoadUint64(&c.stats.FlowsParsed),
		BatchesOutput:  atomic.LoadUint64(&c.stats.BatchesOutput),
		Errors:         atomic.LoadUint64(&c.stats.Errors),
	}
}

// 辅助函数

func ipToString(ip [4]byte) string {
	return strconv.Itoa(int(ip[0])) + "." + strconv.Itoa(int(ip[1])) + "." + strconv.Itoa(int(ip[2])) + "." + strconv.Itoa(int(ip[3]))
}

func protocolToString(p uint8) string {
	switch p {
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 1:
		return "icmp"
	default:
		return "unknown"
	}
}

func formatTCPFlags(flags uint8) string {
	// 简化实现
	return strconv.Itoa(int(flags))
}

func httpMethodToString(method uint8) string {
	methods := []string{"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "CONNECT"}
	if int(method) < len(methods) {
		return methods[method]
	}
	return "UNKNOWN"
}

func formatUint8(v uint8) string {
	return strconv.FormatUint(uint64(v), 10)
}

func formatUint16(v uint16) string {
	return strconv.FormatUint(uint64(v), 10)
}

func formatUint64(v uint64) string {
	return strconv.FormatUint(v, 10)
}
