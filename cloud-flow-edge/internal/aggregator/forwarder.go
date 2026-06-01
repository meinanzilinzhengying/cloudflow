// Package aggregator 提供聚合后的转发功能
//
// 在原有forwarder基础上增加数据聚合能力，减少上报数据量
package aggregator

import (
	"context"
	"sync"
	"time"

	"cloud-flow-edge/internal/forwarder"
	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// AggregatedForwarder 聚合转发器
// 包装原有forwarder，在数据转发前进行聚合
type AggregatedForwarder struct {
	// 底层转发器
	inner *forwarder.Forwarder

	// 聚合器
	aggregator *Aggregator

	// 配置
	config Config
	logger *logger.Logger

	// 控制
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 上报控制
	reportInterval time.Duration
}

// NewAggregatedForwarder 创建聚合转发器
func NewAggregatedForwarder(inner *forwarder.Forwarder, cfg Config, log *logger.Logger) *AggregatedForwarder {
	if cfg.WindowSize == 0 {
		cfg = DefaultConfig()
	}

	agg := NewAggregator(cfg, log)

	af := &AggregatedForwarder{
		inner:          inner,
		aggregator:     agg,
		config:         cfg,
		logger:         log,
		stopCh:         make(chan struct{}),
		reportInterval: cfg.WindowSize, // 按窗口大小上报
	}

	return af
}

// Start 启动聚合转发器
func (af *AggregatedForwarder) Start() {
	af.aggregator.Start()

	// 启动上报协程
	af.wg.Add(1)
	go af.reportLoop()

	af.logger.Infof("[aggregated_forwarder] 聚合转发器已启动: 上报间隔=%v", af.reportInterval)
}

// Stop 停止聚合转发器
func (af *AggregatedForwarder) Stop() {
	close(af.stopCh)

	// 最后一次上报
	af.flushAll()

	af.wg.Wait()
	af.aggregator.Stop()

	af.logger.Info("[aggregated_forwarder] 聚合转发器已停止")
}

// AddMetrics 添加指标数据（聚合后转发）
func (af *AggregatedForwarder) AddMetrics(batch *edge.MetricsBatch) {
	if batch == nil || len(batch.GetMetrics()) == 0 {
		return
	}

	// 添加到聚合器
	af.aggregator.AddMetrics(batch.GetMetrics())

	af.logger.Debugf("[aggregated_forwarder] 添加 %d 条指标数据到聚合器", len(batch.GetMetrics()))
}

// AddTraces 添加链路追踪数据（直接转发，不聚合）
func (af *AggregatedForwarder) AddTraces(batch *edge.TraceBatch) {
	// 链路追踪数据通常需要保留原始细节，直接透传
	af.inner.AddTraces(batch)
}

// AddProfiling 添加性能分析数据（直接转发，不聚合）
func (af *AggregatedForwarder) AddProfiling(batch *edge.ProfilingBatch) {
	// 性能分析数据直接透传
	af.inner.AddProfiling(batch)
}

// reportLoop 定期上报聚合后的数据
func (af *AggregatedForwarder) reportLoop() {
	defer af.wg.Done()

	ticker := time.NewTicker(af.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			af.flushAll()
		case <-af.stopCh:
			return
		}
	}
}

// flushAll 上报所有已完成的窗口数据
func (af *AggregatedForwarder) flushAll() {
	// 获取已完成的窗口
	completedWindows := af.aggregator.GetCompletedWindows()
	if len(completedWindows) == 0 {
		return
	}

	af.logger.Infof("[aggregated_forwarder] 开始上报 %d 个窗口的聚合数据", len(completedWindows))

	totalBefore := 0
	totalAfter := 0

	for _, windowStart := range completedWindows {
		// 获取聚合结果
		aggregated := af.aggregator.GetAggregatedMetrics(windowStart)
		if len(aggregated) == 0 {
			af.aggregator.RemoveWindow(windowStart)
			continue
		}

		// 统计压缩率
		for _, agg := range aggregated {
			totalBefore += int(agg.Count)
			totalAfter++
		}

		// 转换为MetricsBatch
		// 使用第一个聚合键的ProbeID作为批次ID
		probeID := ""
		if len(aggregated) > 0 {
			probeID = aggregated[0].Key.ProbeID
		}

		batch := af.aggregator.ConvertToMetricsBatch(aggregated, probeID)

		// 上报到inner forwarder
		af.inner.AddMetrics(batch)

		// 移除已上报的窗口
		af.aggregator.RemoveWindow(windowStart)

		af.logger.Debugf("[aggregated_forwarder] 上报窗口 %d: %d 条聚合数据", windowStart, len(aggregated))
	}

	// 输出统计
	if totalBefore > 0 {
		compression := float64(totalBefore-totalAfter) / float64(totalBefore) * 100
		af.logger.Infof("[aggregated_forwarder] 上报完成: 原始 %d 条 -> 聚合后 %d 条, 压缩率 %.1f%%",
			totalBefore, totalAfter, compression)
	}
}

// GetStats 获取聚合统计
func (af *AggregatedForwarder) GetStats() Stats {
	return af.aggregator.GetStats()
}

// SetClient 设置转发客户端
func (af *AggregatedForwarder) SetClient(client forwarder.ForwardClient) {
	af.inner.SetClient(client)
}

// UpdateConfig 更新配置
func (af *AggregatedForwarder) UpdateConfig(batchSize, flushIntervalSec int) {
	af.inner.UpdateConfig(batchSize, flushIntervalSec)
}

// SetMetricsSink 设置指标上报接口
func (af *AggregatedForwarder) SetMetricsSink(sink forwarder.MetricsSink) {
	af.inner.SetMetricsSink(sink)
}
