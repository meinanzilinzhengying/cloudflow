// Package forwarder 提供数据缓冲和定时转发功能
// 接收探针发送的指标、链路追踪和性能分析数据，批量转发到中心服务
//
// 优化特性：
//   - 网络中断时缓存数据到本地磁盘（LevelDB）
//   - 网络恢复后按时间顺序自动续传
//   - 缓存容量可配置，默认保留1小时数据
package forwarder

import (
	"context"
	"sync"
	"time"

	"cloud-flow-edge/internal/localcache"
	"cloud-flow-edge/internal/persistence"
	"cloud-flow-edge/internal/resender"
	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// ForwardClient 转发目标接口
// 抽象中心服务客户端，便于测试 mock
type ForwardClient interface {
	ForwardMetrics(batch *edge.MetricsBatch) error
	ForwardTraces(batch *edge.TraceBatch) error
	ForwardProfiling(batch *edge.ProfilingBatch) error
}

// MetricsSink 指标上报接口（可选依赖）
type MetricsSink interface {
	AddMetricsBatch()
	AddTracesBatch()
	AddProfilingBatch()
	AddForwardError()
	UpdateMetricsBufSize(n int)
	UpdateTracesBufSize(n int)
	UpdateProfilingBufSize(n int)
	// M5 修复: 添加缓冲区丢弃计数
	RecordMetricsDropped(count int, reason string)
	RecordTracesDropped(count int, reason string)
	RecordProfilingDropped(count int, reason string)
}

// noopMetrics 空实现，不产生依赖
type noopMetrics struct{}

func (noopMetrics) AddMetricsBatch()                   {}
func (noopMetrics) AddTracesBatch()                    {}
func (noopMetrics) AddProfilingBatch()                 {}
func (noopMetrics) AddForwardError()                   {}
func (noopMetrics) UpdateMetricsBufSize(int)           {}
func (noopMetrics) UpdateTracesBufSize(int)            {}
func (noopMetrics) UpdateProfilingBufSize(int)         {}
func (noopMetrics) RecordMetricsDropped(int, string)   {} // M5
func (noopMetrics) RecordTracesDropped(int, string)    {} // M5
func (noopMetrics) RecordProfilingDropped(int, string) {} // M5

const (
	// 默认缓冲区上限（条目数），超过时丢弃最旧数据以防止 OOM
	defaultMaxBufferLimit = 1000
	// 最大重试次数
	maxRetryAttempts = 3
)

// Forwarder 数据转发器
// 维护三类数据缓冲区，定时批量刷新到中心服务
type Forwarder struct {
	client ForwardClient
	logger *logger.Logger
	metrics MetricsSink
	metricsSinkMu sync.Mutex  // 保护 metrics 的互斥锁
	persistence *persistence.Persistence

	// 本地磁盘缓存（网络中断时使用）
	localCache *localcache.Cache
	// 续传管理器
	resender *resender.Resender

	// 指标数据缓冲
	muMetrics  sync.Mutex
	metricsBuf []*edge.MetricsBatch

	// 链路追踪数据缓冲
	muTraces  sync.Mutex
	tracesBuf []*edge.TraceBatch

	// 性能分析数据缓冲
	muProfiling  sync.Mutex
	profilingBuf []*edge.ProfilingBatch

	batchSize     int           // 批量大小阈值，达到时触发 flush
	flushInterval time.Duration // 定时刷新间隔
	maxBufLimit   int           // 缓冲区上限，超过时丢弃最旧数据
	stopCh        chan struct{}  // 停止信号
	stopped       bool           // 停止状态
	stopMu        sync.Mutex     // 停止操作的互斥锁
	clientMu      sync.Mutex     // 客户端更新的互斥锁
	configMu      sync.Mutex     // 配置更新的互斥锁

	// UpdateConfig 防抖
	configDebounceTimer *time.Timer // 防抖定时器
	configDebounceMu    sync.Mutex  // 防抖定时器的互斥锁

	// 网络状态
	networkOnline bool
	networkMu     sync.RWMutex
}

// NewForwarder 创建数据转发器
// L1 修复: 添加 maxBufferLimit 参数
func NewForwarder(client ForwardClient, batchSize, flushIntervalSec, maxBufferLimit int, log *logger.Logger) *Forwarder {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushIntervalSec <= 0 {
		flushIntervalSec = 5
	}
	// L1 修复: 使用传入的 maxBufferLimit 或默认值
	if maxBufferLimit <= 0 {
		maxBufferLimit = defaultMaxBufferLimit
	}

	// 初始化持久化管理器
	persist, err := persistence.NewPersistence(log)
	if err != nil {
		log.Warnf("[forwarder] 初始化持久化失败: %v，将不使用持久化", err)
	}

	// 初始化本地磁盘缓存
	cacheCfg := localcache.DefaultConfig()
	localCache, err := localcache.NewCache(cacheCfg, log)
	if err != nil {
		log.Warnf("[forwarder] 初始化本地缓存失败: %v，将不使用本地缓存", err)
	}

	fwd := &Forwarder{
		client:        client,
		logger:        log,
		metrics:       noopMetrics{},
		persistence:   persist,
		localCache:    localCache,
		batchSize:     batchSize,
		flushInterval: time.Duration(flushIntervalSec) * time.Second,
		maxBufLimit:   maxBufferLimit,
		stopCh:        make(chan struct{}),
		networkOnline: true,
	}

	// 初始化续传管理器
	if localCache != nil {
		resenderCfg := resender.DefaultConfig()
		fwd.resender = resender.NewResender(resenderCfg, localCache, client, log)
		fwd.resender.Start()
	}

	// 从持久化恢复数据
	if persist != nil {
		fwd.muMetrics.Lock()
		fwd.metricsBuf = persist.GetMetrics()
		fwd.muMetrics.Unlock()

		fwd.muTraces.Lock()
		fwd.tracesBuf = persist.GetTraces()
		fwd.muTraces.Unlock()

		fwd.muProfiling.Lock()
		fwd.profilingBuf = persist.GetProfiling()
		fwd.muProfiling.Unlock()

		log.Infof("[forwarder] 从持久化恢复数据，指标: %d, 追踪: %d, 分析: %d",
			len(fwd.metricsBuf), len(fwd.tracesBuf), len(fwd.profilingBuf))
	}

	return fwd
}

// SetMetrics 设置指标上报（可选）
func (f *Forwarder) SetMetrics(m MetricsSink) {
	f.metricsSinkMu.Lock()
	defer f.metricsSinkMu.Unlock()
	f.metrics = m
}

// AddMetrics 添加指标数据到缓冲区
// 如果缓冲区超过上限，丢弃最旧的数据防止 OOM
func (f *Forwarder) AddMetrics(batch *edge.MetricsBatch) {
	f.stopMu.Lock()
	stopped := f.stopped
	f.stopMu.Unlock()
	if stopped {
		f.logger.Warn("[forwarder] 转发器已停止，丢弃指标数据")
		return
	}

	f.muMetrics.Lock()
	f.metricsBuf = append(f.metricsBuf, batch)
	// M5 修复: 统一丢弃策略，与 Agent 一致（保留最新 batchSize 条）
	// 缓冲区超过上限，丢弃最旧的数据，只保留 batchSize 条
	if len(f.metricsBuf) > f.maxBufLimit {
		droppedCount := len(f.metricsBuf) - f.batchSize
		if droppedCount < 0 {
			droppedCount = len(f.metricsBuf) / 4 // 保底策略：至少丢弃 25%
		}
		f.logger.Warnf("[M5][forwarder][metrics] 缓冲区超限 (%d > %d)，丢弃最旧 %d 条，保留最新 %d 条",
			len(f.metricsBuf), f.maxBufLimit, droppedCount, f.batchSize)
		f.metrics.RecordMetricsDropped(droppedCount, "buffer_overflow")
		f.metricsBuf = f.metricsBuf[droppedCount:]
	}
	shouldFlush := len(f.metricsBuf) >= f.batchSize
	size := len(f.metricsBuf)
	f.muMetrics.Unlock()

	// 持久化数据
	if f.persistence != nil {
		if err := f.persistence.AddMetrics(batch); err != nil {
			f.logger.Warnf("[forwarder][metrics] 持久化指标数据失败: %v", err)
		}
	}

	f.metrics.UpdateMetricsBufSize(size)

	if shouldFlush {
		f.flushMetrics(false)
	}
}

// AddTraces 添加链路追踪数据到缓冲区
func (f *Forwarder) AddTraces(batch *edge.TraceBatch) {
	f.stopMu.Lock()
	stopped := f.stopped
	f.stopMu.Unlock()
	if stopped {
		f.logger.Warn("[forwarder] 转发器已停止，丢弃链路追踪数据")
		return
	}

	f.muTraces.Lock()
	f.tracesBuf = append(f.tracesBuf, batch)
	// M5 修复: 统一丢弃策略，与 Agent 一致（保留最新 batchSize 条）
	if len(f.tracesBuf) > f.maxBufLimit {
		droppedCount := len(f.tracesBuf) - f.batchSize
		if droppedCount < 0 {
			droppedCount = len(f.tracesBuf) / 4
		}
		f.logger.Warnf("[M5][forwarder][traces] 缓冲区超限 (%d > %d)，丢弃最旧 %d 条，保留最新 %d 条",
			len(f.tracesBuf), f.maxBufLimit, droppedCount, f.batchSize)
		f.metrics.RecordTracesDropped(droppedCount, "buffer_overflow")
		f.tracesBuf = f.tracesBuf[droppedCount:]
	}
	shouldFlush := len(f.tracesBuf) >= f.batchSize
	size := len(f.tracesBuf)
	f.muTraces.Unlock()

	// 持久化数据
	if f.persistence != nil {
		if err := f.persistence.AddTraces(batch); err != nil {
			f.logger.Warnf("[forwarder][traces] 持久化链路追踪数据失败: %v", err)
		}
	}

	f.metrics.UpdateTracesBufSize(size)

	if shouldFlush {
		f.flushTraces(false)
	}
}

// AddProfiling 添加性能分析数据到缓冲区
func (f *Forwarder) AddProfiling(batch *edge.ProfilingBatch) {
	f.stopMu.Lock()
	stopped := f.stopped
	f.stopMu.Unlock()
	if stopped {
		f.logger.Warn("[forwarder] 转发器已停止，丢弃性能分析数据")
		return
	}

	f.muProfiling.Lock()
	f.profilingBuf = append(f.profilingBuf, batch)
	// M5 修复: 统一丢弃策略，与 Agent 一致（保留最新 batchSize 条）
	if len(f.profilingBuf) > f.maxBufLimit {
		droppedCount := len(f.profilingBuf) - f.batchSize
		if droppedCount < 0 {
			droppedCount = len(f.profilingBuf) / 4
		}
		f.logger.Warnf("[M5][forwarder][profiling] 缓冲区超限 (%d > %d)，丢弃最旧 %d 条，保留最新 %d 条",
			len(f.profilingBuf), f.maxBufLimit, droppedCount, f.batchSize)
		f.metrics.RecordProfilingDropped(droppedCount, "buffer_overflow")
		f.profilingBuf = f.profilingBuf[droppedCount:]
	}
	shouldFlush := len(f.profilingBuf) >= f.batchSize
	size := len(f.profilingBuf)
	f.muProfiling.Unlock()

	// 持久化数据
	if f.persistence != nil {
		if err := f.persistence.AddProfiling(batch); err != nil {
			f.logger.Warnf("[forwarder][profiling] 持久化性能分析数据失败: %v", err)
		}
	}

	f.metrics.UpdateProfilingBufSize(size)

	if shouldFlush {
		f.flushProfiling(false)
	}
}

// Start 启动定时刷新协程
func (f *Forwarder) Start() {
	go func() {
		f.configMu.RLock()
		interval := f.flushInterval
		f.configMu.RUnlock()
		timer := time.NewTimer(interval)

		for {
			select {
			case <-timer.C:
				f.flushMetrics(false)
				f.flushTraces(false)
				f.flushProfiling(false)
				// 重置 timer，使用最新的 flushInterval
				f.configMu.RLock()
				interval = f.flushInterval
				f.configMu.RUnlock()
				timer.Reset(interval)
			case <-f.stopCh:
				timer.Stop()
				f.logger.Info("[forwarder] 转发器刷新协程已停止")
				return
			}
		}
	}()
	f.configMu.RLock()
	f.logger.Infof("[forwarder] 数据转发器已启动，flushInterval=%s, batchSize=%d", f.flushInterval, f.batchSize)
	f.configMu.RUnlock()
}

const flushTimeout = 30 * time.Second

// Stop 停止转发器，刷新剩余数据
func (f *Forwarder) Stop() {
	f.stopMu.Lock()
	if !f.stopped {
		close(f.stopCh)
		f.stopped = true
	}
	f.stopMu.Unlock()

	// 停止续传管理器
	if f.resender != nil {
		f.resender.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		f.flushMetrics(true)
		f.flushTraces(true)
		f.flushProfiling(true)
		close(done)
	}()

	select {
	case <-done:
		f.logger.Info("[forwarder] 剩余数据刷新完成")
	case <-ctx.Done():
		f.logger.Warnf("[forwarder] 剩余数据刷新超时 (%v)，部分数据可能未发送", flushTimeout)
	}

	// 关闭持久化管理器
	if f.persistence != nil {
		if err := f.persistence.Close(); err != nil {
			f.logger.Warnf("[forwarder] 关闭持久化管理器失败: %v", err)
		}
	}

	// 关闭本地缓存
	if f.localCache != nil {
		if err := f.localCache.Close(); err != nil {
			f.logger.Warnf("[forwarder] 关闭本地缓存失败: %v", err)
		}
	}

	f.logger.Info("[forwarder] 数据转发器已停止")
}

// UpdateConfig 更新转发器配置（带防抖，短时间内多次调用只会执行最后一次）
func (f *Forwarder) UpdateConfig(batchSize, flushIntervalSec int) {
	const debounceDelay = 500 * time.Millisecond

	f.configDebounceMu.Lock()
	if f.configDebounceTimer != nil {
		f.configDebounceTimer.Stop()
	}
	f.configDebounceTimer = time.AfterFunc(debounceDelay, func() {
		f.applyConfig(batchSize, flushIntervalSec)
	})
	f.configDebounceMu.Unlock()
}

// applyConfig 实际应用配置变更
func (f *Forwarder) applyConfig(batchSize, flushIntervalSec int) {
	// 检查是否已停止，如果已关闭则不执行
	f.stopMu.Lock()
	stopped := f.stopped
	f.stopMu.Unlock()
	if stopped {
		f.logger.Info("[forwarder] 转发器已停止，跳过配置更新")
		return
	}

	f.configMu.Lock()
	defer f.configMu.Unlock()

	if batchSize <= 0 {
		batchSize = 100
	}
	if flushIntervalSec <= 0 {
		flushIntervalSec = 5
	}

	f.batchSize = batchSize
	f.flushInterval = time.Duration(flushIntervalSec) * time.Second
	f.logger.Infof("[forwarder] 转发器配置已更新: batchSize=%d, flushInterval=%s", batchSize, f.flushInterval)
}

// SetClient 更新转发器的客户端
func (f *Forwarder) SetClient(client ForwardClient) {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	f.client = client

	// 更新续传管理器的客户端
	if f.resender != nil {
		f.resender.SetClient(client)
	}

	// 如果之前有网络中断，现在客户端更新了，尝试标记网络恢复
	f.networkMu.Lock()
	wasOffline := !f.networkOnline
	f.networkOnline = true
	f.networkMu.Unlock()

	if wasOffline {
		f.logger.Info("[forwarder] 转发客户端已更新，标记网络恢复")
	}
}

// getClient 获取线程安全的客户端
func (f *Forwarder) getClient() ForwardClient {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	return f.client
}

func (f *Forwarder) flushMetrics(force bool) {
	f.muMetrics.Lock()
	if len(f.metricsBuf) == 0 {
		f.muMetrics.Unlock()
		return
	}
	buf := f.metricsBuf
	f.metricsBuf = nil
	f.muMetrics.Unlock()
	f.metrics.UpdateMetricsBufSize(0)

	retryCount := 0
	allSent := true
	var failedBatches []*edge.MetricsBatch
	for i, batch := range buf {
		client := f.getClient()
		if err := client.ForwardMetrics(batch); err != nil {
			f.logger.Warnf("[forwarder][metrics] 转发指标数据失败 (第 %d/%d 批): %v", i+1, len(buf), err)
			f.metrics.AddForwardError()
			retryCount++
			allSent = false
			failedBatches = append(failedBatches, batch)

			// 通知续传管理器转发失败
			if f.resender != nil {
				f.resender.OnForwardError("metrics", err)
			}

			if retryCount >= maxRetryAttempts {
				// 将剩余未处理的批次也加入失败列表，避免静默丢失
				failedBatches = append(failedBatches, buf[i+1:]...)
				break
			}
			if !force {
				select {
				case <-time.After(time.Duration(retryCount) * time.Second):
				case <-f.stopCh:
					// 将已收集的失败批次缓存到本地磁盘
					if len(failedBatches) > 0 && f.localCache != nil {
						for _, failedBatch := range failedBatches {
							if err := f.localCache.AddMetrics(failedBatch); err != nil {
								f.logger.Warnf("[forwarder][metrics] 缓存失败数据到本地失败: %v", err)
							}
						}
					}
					return
				}
			} else {
				time.Sleep(time.Duration(retryCount) * time.Second)
			}
			continue
		}
		f.metrics.AddMetricsBatch()
		retryCount = 0
	}

	// 将失败批次缓存到本地磁盘（用于网络恢复后续传）
	if len(failedBatches) > 0 {
		if f.localCache != nil {
			cachedCount := 0
			for _, batch := range failedBatches {
				if err := f.localCache.AddMetrics(batch); err != nil {
					f.logger.Warnf("[forwarder][metrics] 缓存失败数据到本地失败: %v", err)
				} else {
					cachedCount++
				}
			}
			f.logger.Warnf("[forwarder][metrics] 转发失败，%d 条数据已缓存到本地磁盘", cachedCount)
		} else {
			// 如果没有本地缓存，放回内存缓冲区
			f.muMetrics.Lock()
			f.metricsBuf = append(failedBatches, f.metricsBuf...)
			f.muMetrics.Unlock()
			f.logger.Warnf("[forwarder][metrics] 转发失败，%d 条数据已放回缓冲区", len(failedBatches))
		}
	}

	// 所有数据发送成功，清空指标的持久化数据
	if allSent && f.persistence != nil {
		f.persistence.ClearMetrics()
	}
}

func (f *Forwarder) flushTraces(force bool) {
	f.muTraces.Lock()
	if len(f.tracesBuf) == 0 {
		f.muTraces.Unlock()
		return
	}
	buf := f.tracesBuf
	f.tracesBuf = nil
	f.muTraces.Unlock()
	f.metrics.UpdateTracesBufSize(0)

	retryCount := 0
	allSent := true
	var failedBatches []*edge.TraceBatch
	for i, batch := range buf {
		client := f.getClient()
		if err := client.ForwardTraces(batch); err != nil {
			f.logger.Warnf("[forwarder][traces] 转发链路追踪数据失败 (第 %d/%d 批): %v", i+1, len(buf), err)
			f.metrics.AddForwardError()
			retryCount++
			allSent = false
			failedBatches = append(failedBatches, batch)

			// 通知续传管理器转发失败
			if f.resender != nil {
				f.resender.OnForwardError("traces", err)
			}

			if retryCount >= maxRetryAttempts {
				// 将剩余未处理的批次也加入失败列表，避免静默丢失
				failedBatches = append(failedBatches, buf[i+1:]...)
				break
			}
			if !force {
				select {
				case <-time.After(time.Duration(retryCount) * time.Second):
				case <-f.stopCh:
					// 将已收集的失败批次缓存到本地磁盘
					if len(failedBatches) > 0 && f.localCache != nil {
						for _, failedBatch := range failedBatches {
							if err := f.localCache.AddTraces(failedBatch); err != nil {
								f.logger.Warnf("[forwarder][traces] 缓存失败数据到本地失败: %v", err)
							}
						}
					}
					return
				}
			} else {
				time.Sleep(time.Duration(retryCount) * time.Second)
			}
			continue
		}
		f.metrics.AddTracesBatch()
		retryCount = 0
	}

	// 将失败批次缓存到本地磁盘（用于网络恢复后续传）
	if len(failedBatches) > 0 {
		if f.localCache != nil {
			cachedCount := 0
			for _, batch := range failedBatches {
				if err := f.localCache.AddTraces(batch); err != nil {
					f.logger.Warnf("[forwarder][traces] 缓存失败数据到本地失败: %v", err)
				} else {
					cachedCount++
				}
			}
			f.logger.Warnf("[forwarder][traces] 转发失败，%d 条数据已缓存到本地磁盘", cachedCount)
		} else {
			// 如果没有本地缓存，放回内存缓冲区
			f.muTraces.Lock()
			f.tracesBuf = append(failedBatches, f.tracesBuf...)
			f.muTraces.Unlock()
			f.logger.Warnf("[forwarder][traces] 转发失败，%d 条数据已放回缓冲区", len(failedBatches))
		}
	}

	// 所有数据发送成功，清空链路追踪的持久化数据
	if allSent && f.persistence != nil {
		f.persistence.ClearTraces()
	}
}

func (f *Forwarder) flushProfiling(force bool) {
	f.muProfiling.Lock()
	if len(f.profilingBuf) == 0 {
		f.muProfiling.Unlock()
		return
	}
	buf := f.profilingBuf
	f.profilingBuf = nil
	f.muProfiling.Unlock()
	f.metrics.UpdateProfilingBufSize(0)

	retryCount := 0
	allSent := true
	var failedBatches []*edge.ProfilingBatch
	for i, batch := range buf {
		client := f.getClient()
		if err := client.ForwardProfiling(batch); err != nil {
			f.logger.Warnf("[forwarder][profiling] 转发性能分析数据失败 (第 %d/%d 批): %v", i+1, len(buf), err)
			f.metrics.AddForwardError()
			retryCount++
			allSent = false
			failedBatches = append(failedBatches, batch)

			// 通知续传管理器转发失败
			if f.resender != nil {
				f.resender.OnForwardError("profiling", err)
			}

			if retryCount >= maxRetryAttempts {
				// 将剩余未处理的批次也加入失败列表，避免静默丢失
				failedBatches = append(failedBatches, buf[i+1:]...)
				break
			}
			if !force {
				select {
				case <-time.After(time.Duration(retryCount) * time.Second):
				case <-f.stopCh:
					// 将已收集的失败批次缓存到本地磁盘
					if len(failedBatches) > 0 && f.localCache != nil {
						for _, failedBatch := range failedBatches {
							if err := f.localCache.AddProfiling(failedBatch); err != nil {
								f.logger.Warnf("[forwarder][profiling] 缓存失败数据到本地失败: %v", err)
							}
						}
					}
					return
				}
			} else {
				time.Sleep(time.Duration(retryCount) * time.Second)
			}
			continue
		}
		f.metrics.AddProfilingBatch()
		retryCount = 0
	}

	// 将失败批次缓存到本地磁盘（用于网络恢复后续传）
	if len(failedBatches) > 0 {
		if f.localCache != nil {
			cachedCount := 0
			for _, batch := range failedBatches {
				if err := f.localCache.AddProfiling(batch); err != nil {
					f.logger.Warnf("[forwarder][profiling] 缓存失败数据到本地失败: %v", err)
				} else {
					cachedCount++
				}
			}
			f.logger.Warnf("[forwarder][profiling] 转发失败，%d 条数据已缓存到本地磁盘", cachedCount)
		} else {
			// 如果没有本地缓存，放回内存缓冲区
			f.muProfiling.Lock()
			f.profilingBuf = append(failedBatches, f.profilingBuf...)
			f.muProfiling.Unlock()
			f.logger.Warnf("[forwarder][profiling] 转发失败，%d 条数据已放回缓冲区", len(failedBatches))
		}
	}

	// 所有数据发送成功，清空性能分析的持久化数据
	if allSent && f.persistence != nil {
		f.persistence.ClearProfiling()
	}
}
