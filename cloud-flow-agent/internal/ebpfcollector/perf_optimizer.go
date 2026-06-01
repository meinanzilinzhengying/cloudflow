// Package ebpfcollector 提供eBPF采集性能优化功能
package ebpfcollector

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// PerfConfig 性能优化配置
type PerfConfig struct {
	// 采样率配置
	SampleRate float64 // 采样率 (0.0-1.0, 1.0表示全量采集)
	
	// 批处理配置
	BatchSize     int           // 批处理大小
	BatchInterval time.Duration // 批处理间隔
	
	// 限流配置
	MaxEventsPerSec int // 每秒最大事件数
	
	// 资源自适应
	EnableAdaptive bool  // 启用自适应采样
	CPUTarget      float64 // CPU使用率目标
	MemoryTarget   float64 // 内存使用率目标
	
	// 高负载场景配置 (700Mbps, RPS1400)
	HighLoadMode bool // 高负载模式
}

// DefaultPerfConfig 默认性能配置
func DefaultPerfConfig() PerfConfig {
	return PerfConfig{
		SampleRate:      1.0,           // 默认全量采集
		BatchSize:       100,           // 每批100条
		BatchInterval:   1 * time.Second, // 每秒一批
		MaxEventsPerSec: 10000,         // 每秒最多1万事件
		EnableAdaptive:  true,
		CPUTarget:       80.0,          // CPU目标80%
		MemoryTarget:    80.0,          // 内存目标80%
		HighLoadMode:    false,
	}
}

// HighLoadPerfConfig 高负载场景配置 (700Mbps, RPS1400)
func HighLoadPerfConfig() PerfConfig {
	return PerfConfig{
		SampleRate:      0.5,            // 50%采样
		BatchSize:       500,            // 每批500条
		BatchInterval:   2 * time.Second, // 每2秒一批
		MaxEventsPerSec: 5000,           // 每秒最多5千事件
		EnableAdaptive:  true,
		CPUTarget:       70.0,           // CPU目标70%
		MemoryTarget:    70.0,           // 内存目标70%
		HighLoadMode:    true,
	}
}

// PerfOptimizer 性能优化器
type PerfOptimizer struct {
	log      *logger.Logger
	config   PerfConfig
	mu       sync.RWMutex
	
	// 采样状态
	sampleCounter uint64
	sampleDrop    uint64
	
	// 批处理
	batchBuffer   []interface{}
	batchMu       sync.Mutex
	batchTicker   *time.Ticker
	
	// 自适应控制
	adaptiveTicker *time.Ticker
	currentLoad    float64
	
	// 控制
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewPerfOptimizer 创建性能优化器
func NewPerfOptimizer(log *logger.Logger, config PerfConfig) *PerfOptimizer {
	return &PerfOptimizer{
		log:          log,
		config:       config,
		batchBuffer:  make([]interface{}, 0, config.BatchSize),
		stopCh:       make(chan struct{}),
	}
}

// Start 启动优化器
func (po *PerfOptimizer) Start() {
	po.log.Infof("性能优化器已启动: 采样率=%.2f, 批大小=%d, 高负载模式=%v",
		po.config.SampleRate, po.config.BatchSize, po.config.HighLoadMode)
	
	// 启动批处理
	po.wg.Add(1)
	go po.batchLoop()
	
	// 启动自适应控制
	if po.config.EnableAdaptive {
		po.wg.Add(1)
		go po.adaptiveLoop()
	}
}

// Stop 停止优化器
func (po *PerfOptimizer) Stop() {
	close(po.stopCh)
	po.wg.Wait()
	po.log.Info("性能优化器已停止")
}

// ShouldSample 判断是否采样
func (po *PerfOptimizer) ShouldSample() bool {
	po.mu.RLock()
	rate := po.config.SampleRate
	po.mu.RUnlock()
	
	if rate >= 1.0 {
		return true
	}
	
	// 使用计数器实现精确采样
	po.sampleCounter++
	if float64(po.sampleCounter%100) < rate*100 {
		return true
	}
	
	po.sampleDrop++
	return false
}

// AddToBatch 添加到批处理
func (po *PerfOptimizer) AddToBatch(item interface{}) bool {
	po.batchMu.Lock()
	defer po.batchMu.Unlock()
	
	po.batchBuffer = append(po.batchBuffer, item)
	
	// 达到批大小则立即处理
	if len(po.batchBuffer) >= po.config.BatchSize {
		po.flushBatch()
		return true
	}
	
	return false
}

// batchLoop 批处理循环
func (po *PerfOptimizer) batchLoop() {
	defer po.wg.Done()
	
	po.batchTicker = time.NewTicker(po.config.BatchInterval)
	defer po.batchTicker.Stop()
	
	for {
		select {
		case <-po.batchTicker.C:
			po.batchMu.Lock()
			po.flushBatch()
			po.batchMu.Unlock()
		case <-po.stopCh:
			return
		}
	}
}

// flushBatch 刷新批处理
func (po *PerfOptimizer) flushBatch() {
	if len(po.batchBuffer) == 0 {
		return
	}
	
	// 这里可以添加实际的处理逻辑
	// 例如发送到处理队列或存储
	
	po.log.Debugf("批处理刷新: %d 条记录", len(po.batchBuffer))
	po.batchBuffer = po.batchBuffer[:0] // 清空缓冲区
}

// adaptiveLoop 自适应控制循环
func (po *PerfOptimizer) adaptiveLoop() {
	defer po.wg.Done()
	
	po.adaptiveTicker = time.NewTicker(10 * time.Second)
	defer po.adaptiveTicker.Stop()
	
	for {
		select {
		case <-po.adaptiveTicker.C:
			po.adjustSampleRate()
		case <-po.stopCh:
			return
		}
	}
}

// adjustSampleRate 调整采样率
func (po *PerfOptimizer) adjustSampleRate() {
	// 获取当前资源使用
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	memUsage := float64(m.Alloc) / 1024 / 1024 // MB
	memPercent := (memUsage / 1024) * 100 // 假设限制1GB
	
	po.mu.Lock()
	defer po.mu.Unlock()
	
	oldRate := po.config.SampleRate
	
	// 根据资源使用调整采样率
	if memPercent > po.config.MemoryTarget {
		// 内存过高，降低采样率
		po.config.SampleRate *= 0.8
		if po.config.SampleRate < 0.1 {
			po.config.SampleRate = 0.1
		}
	} else if memPercent < po.config.MemoryTarget*0.5 {
		// 内存充足，提高采样率
		po.config.SampleRate *= 1.1
		if po.config.SampleRate > 1.0 {
			po.config.SampleRate = 1.0
		}
	}
	
	if oldRate != po.config.SampleRate {
		po.log.Infof("自适应采样率调整: %.2f -> %.2f (内存使用率: %.1f%%)",
			oldRate, po.config.SampleRate, memPercent)
	}
}

// GetSampleStats 获取采样统计
func (po *PerfOptimizer) GetSampleStats() (total, dropped uint64) {
	return po.sampleCounter, po.sampleDrop
}

// SetSampleRate 设置采样率
func (po *PerfOptimizer) SetSampleRate(rate float64) {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	if rate < 0.0 {
		rate = 0.0
	}
	if rate > 1.0 {
		rate = 1.0
	}
	
	po.config.SampleRate = rate
	po.log.Infof("采样率已设置为: %.2f", rate)
}

// GetConfig 获取当前配置
func (po *PerfOptimizer) GetConfig() PerfConfig {
	po.mu.RLock()
	defer po.mu.RUnlock()
	return po.config
}

// EnableHighLoadMode 启用高负载模式
func (po *PerfOptimizer) EnableHighLoadMode() {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	po.config = HighLoadPerfConfig()
	po.log.Info("已启用高负载模式 (700Mbps, RPS1400)")
}

// DisableHighLoadMode 禁用高负载模式
func (po *PerfOptimizer) DisableHighLoadMode() {
	po.mu.Lock()
	defer po.mu.Unlock()
	
	po.config = DefaultPerfConfig()
	po.log.Info("已禁用高负载模式")
}

// RateLimiter 简单令牌桶限流器
type RateLimiter struct {
	rate      float64   // 每秒令牌数
	burst     int       // 桶容量
	tokens    float64
	lastTime  time.Time
	mu        sync.Mutex
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   float64(burst),
		lastTime: time.Now(),
	}
}

// Allow 检查是否允许通过
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(rl.lastTime).Seconds()
	rl.lastTime = now
	
	// 补充令牌
	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.burst) {
		rl.tokens = float64(rl.burst)
	}
	
	// 消费令牌
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}
	
	return false
}

// Wait 等待直到有可用令牌
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
