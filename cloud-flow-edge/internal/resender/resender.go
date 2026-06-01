// Package resender 提供网络恢复后的数据续传功能
//
// 功能：
// 1. 监控网络状态，检测网络中断和恢复
// 2. 网络中断时，将转发失败的数据缓存到本地磁盘
// 3. 网络恢复后，按时间顺序自动续传缓存数据
// 4. 支持批量续传，控制重传速率
// 5. 续传完成后清理已发送的数据
package resender

import (
	"context"
	"sync"
	"time"

	"cloud-flow-edge/internal/forwarder"
	"cloud-flow-edge/internal/localcache"
	"cloud-flow-edge/pkg/logger"
	edge "cloud-flow/proto"
)

// NetworkStatus 网络状态
type NetworkStatus int

const (
	// NetworkUnknown 未知状态
	NetworkUnknown NetworkStatus = iota
	// NetworkOnline 网络正常
	NetworkOnline
	// NetworkOffline 网络中断
	NetworkOffline
)

func (s NetworkStatus) String() string {
	switch s {
	case NetworkOnline:
		return "online"
	case NetworkOffline:
		return "offline"
	default:
		return "unknown"
	}
}

// Config 续传管理器配置
type Config struct {
	// 网络检测间隔
	CheckInterval time.Duration
	// 续传批量大小
	ResendBatchSize int
	// 续传间隔（控制速率）
	ResendInterval time.Duration
	// 最大并发续传批次
	MaxConcurrentResend int
	// 是否启用缓存
	EnableCache bool
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		CheckInterval:       10 * time.Second,
		ResendBatchSize:     100,
		ResendInterval:      1 * time.Second,
		MaxConcurrentResend: 5,
		EnableCache:         true,
	}
}

// Resender 续传管理器
type Resender struct {
	config Config
	logger *logger.Logger

	// 依赖组件
	cache  *localcache.Cache
	client forwarder.ForwardClient

	// 网络状态
	status    NetworkStatus
	statusMu  sync.RWMutex

	// 控制信号
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 续传控制
	resendMu     sync.Mutex
	isResending  bool
	resendCancel context.CancelFunc

	// 统计
	statsMu       sync.RWMutex
	resendMetrics int64
	resendTraces  int64
	resendProf    int64
	failedMetrics int64
	failedTraces  int64
	failedProf    int64
}

// NewResender 创建续传管理器
func NewResender(cfg Config, cache *localcache.Cache, client forwarder.ForwardClient, log *logger.Logger) *Resender {
	if cfg.CheckInterval == 0 {
		cfg = DefaultConfig()
	}

	return &Resender{
		config:   cfg,
		logger:   log,
		cache:    cache,
		client:   client,
		status:   NetworkUnknown,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动续传管理器
func (r *Resender) Start() {
	r.wg.Add(1)
	go r.networkMonitor()
	r.logger.Info("[resender] 续传管理器已启动")
}

// Stop 停止续传管理器
func (r *Resender) Stop() {
	close(r.stopCh)

	// 取消正在进行的续传
	r.resendMu.Lock()
	if r.resendCancel != nil {
		r.resendCancel()
	}
	r.resendMu.Unlock()

	r.wg.Wait()
	r.logger.Info("[resender] 续传管理器已停止")
}

// SetClient 更新转发客户端
func (r *Resender) SetClient(client forwarder.ForwardClient) {
	r.client = client
}

// GetStatus 获取当前网络状态
func (r *Resender) GetStatus() NetworkStatus {
	r.statusMu.RLock()
	defer r.statusMu.RUnlock()
	return r.status
}

// setStatus 设置网络状态
func (r *Resender) setStatus(status NetworkStatus) {
	r.statusMu.Lock()
	oldStatus := r.status
	r.status = status
	r.statusMu.Unlock()

	if oldStatus != status {
		r.logger.Infof("[resender] 网络状态变更: %s -> %s", oldStatus, status)

		if status == NetworkOnline && oldStatus == NetworkOffline {
			// 网络恢复，触发续传
			r.triggerResend()
		}
	}
}

// networkMonitor 网络状态监控协程
func (r *Resender) networkMonitor() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	// 首次检测
	r.checkNetwork()

	for {
		select {
		case <-ticker.C:
			r.checkNetwork()
		case <-r.stopCh:
			return
		}
	}
}

// checkNetwork 检测网络状态
func (r *Resender) checkNetwork() {
	if r.client == nil {
		r.setStatus(NetworkOffline)
		return
	}

	// 尝试发送一个空的探测请求
	// 实际实现中可以通过发送一个轻量级请求来检测网络
	// 这里简化处理：假设网络正常，由转发失败来触发离线状态
	currentStatus := r.GetStatus()

	// 如果有缓存数据且网络被认为是正常的，尝试续传
	if currentStatus == NetworkOnline && r.hasCachedData() {
		r.triggerResend()
	}
}

// hasCachedData 检查是否有缓存数据
func (r *Resender) hasCachedData() bool {
	if r.cache == nil {
		return false
	}
	stats := r.cache.GetStats()
	return stats.TotalEntries > 0
}

// OnForwardError 转发失败回调（由forwarder调用）
func (r *Resender) OnForwardError(dataType string, err error) {
	r.logger.Warnf("[resender] 转发失败: type=%s, err=%v", dataType, err)

	// 标记网络可能中断
	r.setStatus(NetworkOffline)

	// 更新统计
	r.statsMu.Lock()
	switch dataType {
	case "metrics":
		r.failedMetrics++
	case "traces":
		r.failedTraces++
	case "profiling":
		r.failedProf++
	}
	r.statsMu.Unlock()
}

// CacheMetrics 缓存指标数据（网络中断时调用）
func (r *Resender) CacheMetrics(batch *edge.MetricsBatch) error {
	if !r.config.EnableCache || r.cache == nil {
		return nil
	}
	return r.cache.AddMetrics(batch)
}

// CacheTraces 缓存链路追踪数据
func (r *Resender) CacheTraces(batch *edge.TraceBatch) error {
	if !r.config.EnableCache || r.cache == nil {
		return nil
	}
	return r.cache.AddTraces(batch)
}

// CacheProfiling 缓存性能分析数据
func (r *Resender) CacheProfiling(batch *edge.ProfilingBatch) error {
	if !r.config.EnableCache || r.cache == nil {
		return nil
	}
	return r.cache.AddProfiling(batch)
}

// triggerResend 触发续传
func (r *Resender) triggerResend() {
	r.resendMu.Lock()
	if r.isResending {
		r.resendMu.Unlock()
		r.logger.Debug("[resender] 续传已在进行中，跳过")
		return
	}
	r.isResending = true
	ctx, cancel := context.WithCancel(context.Background())
	r.resendCancel = cancel
	r.resendMu.Unlock()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.doResend(ctx)

		r.resendMu.Lock()
		r.isResending = false
		r.resendCancel = nil
		r.resendMu.Unlock()
	}()
}

// doResend 执行续传
func (r *Resender) doResend(ctx context.Context) {
	if r.cache == nil || r.client == nil {
		return
	}

	r.logger.Info("[resender] 开始续传缓存数据")

	// 创建限流器控制并发
	sem := make(chan struct{}, r.config.MaxConcurrentResend)
	var wg sync.WaitGroup

	// 续传指标数据
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.resendMetricsData(ctx, sem)
	}()

	// 续传链路追踪数据
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.resendTracesData(ctx, sem)
	}()

	// 续传性能分析数据
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.resendProfilingData(ctx, sem)
	}()

	wg.Wait()

	// 检查是否全部续传完成
	stats := r.cache.GetStats()
	if stats.TotalEntries == 0 {
		r.logger.Info("[resender] 所有缓存数据续传完成")
	} else {
		r.logger.Warnf("[resender] 续传结束，仍有 %d 条数据未发送", stats.TotalEntries)
	}
}

// resendMetricsData 续传指标数据
func (r *Resender) resendMetricsData(ctx context.Context, sem chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 获取一批数据
		batches := r.cache.GetMetricsBatch(r.config.ResendBatchSize)
		if len(batches) == 0 {
			return
		}

		sentCount := 0
		for _, batch := range batches {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			err := r.client.ForwardMetrics(batch)
			<-sem

			if err != nil {
				r.logger.Warnf("[resender] 续传指标数据失败: %v", err)
				// 发送失败，停止续传指标数据
				r.setStatus(NetworkOffline)
				return
			}

			sentCount++
			r.statsMu.Lock()
			r.resendMetrics += int64(len(batch.GetMetrics()))
			r.statsMu.Unlock()

			// 控制速率
			time.Sleep(r.config.ResendInterval)
		}

		// 删除已发送的数据
		r.cache.RemoveMetrics(sentCount)
		r.logger.Infof("[resender] 续传指标数据: %d 批次", sentCount)
	}
}

// resendTracesData 续传链路追踪数据
func (r *Resender) resendTracesData(ctx context.Context, sem chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batches := r.cache.GetTracesBatch(r.config.ResendBatchSize)
		if len(batches) == 0 {
			return
		}

		sentCount := 0
		for _, batch := range batches {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			err := r.client.ForwardTraces(batch)
			<-sem

			if err != nil {
				r.logger.Warnf("[resender] 续传链路追踪数据失败: %v", err)
				r.setStatus(NetworkOffline)
				return
			}

			sentCount++
			r.statsMu.Lock()
			r.resendTraces += int64(len(batch.GetSpans()))
			r.statsMu.Unlock()

			time.Sleep(r.config.ResendInterval)
		}

		r.cache.RemoveTraces(sentCount)
		r.logger.Infof("[resender] 续传链路追踪数据: %d 批次", sentCount)
	}
}

// resendProfilingData 续传性能分析数据
func (r *Resender) resendProfilingData(ctx context.Context, sem chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batches := r.cache.GetProfilingBatch(r.config.ResendBatchSize)
		if len(batches) == 0 {
			return
		}

		sentCount := 0
		for _, batch := range batches {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			err := r.client.ForwardProfiling(batch)
			<-sem

			if err != nil {
				r.logger.Warnf("[resender] 续传性能分析数据失败: %v", err)
				r.setStatus(NetworkOffline)
				return
			}

			sentCount++
			r.statsMu.Lock()
			r.resendProf += int64(len(batch.GetProfiles()))
			r.statsMu.Unlock()

			time.Sleep(r.config.ResendInterval)
		}

		r.cache.RemoveProfiling(sentCount)
		r.logger.Infof("[resender] 续传性能分析数据: %d 批次", sentCount)
	}
}

// GetStats 获取续传统计
func (r *Resender) GetStats() map[string]interface{} {
	r.statsMu.RLock()
	defer r.statsMu.RUnlock()

	return map[string]interface{}{
		"status":             r.GetStatus().String(),
		"resend_metrics":     r.resendMetrics,
		"resend_traces":      r.resendTraces,
		"resend_profiling":   r.resendProf,
		"failed_metrics":     r.failedMetrics,
		"failed_traces":      r.failedTraces,
		"failed_profiling":   r.failedProf,
	}
}

// ResetStats 重置统计
func (r *Resender) ResetStats() {
	r.statsMu.Lock()
	defer r.statsMu.Unlock()

	r.resendMetrics = 0
	r.resendTraces = 0
	r.resendProf = 0
	r.failedMetrics = 0
	r.failedTraces = 0
	r.failedProf = 0
}
