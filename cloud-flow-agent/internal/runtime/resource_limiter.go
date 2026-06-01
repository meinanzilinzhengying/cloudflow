// Package runtime 提供资源限制和过载保护功能
package runtime

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"cloud-flow-agent/pkg/logger"
)

// ResourceLimit 资源限制配置
type ResourceLimit struct {
	MaxCPUCore    float64 // 最大CPU核心数 (如: 1.0表示1核)
	MaxMemoryMB   float64 // 最大内存使用 (MB)
	MaxGoroutines int     // 最大协程数
}

// ResourceLimiter 资源限制器
type ResourceLimiter struct {
	log          *logger.Logger
	limit        ResourceLimit
	mu           sync.RWMutex
	stopCh       chan struct{}
	interval     time.Duration
	
	// 状态
	isLimited    bool      // 是否处于限制状态
	isSilent     bool      // 是否处于静默状态
	silentStart  time.Time // 静默开始时间
	silentDuration time.Duration // 静默持续时间
	
	// 回调
	onLimit      func()    // 触发限制时的回调
	onSilent     func()    // 进入静默时的回调
	onRecover    func()    // 恢复时的回调
	
	// 采集控制
	collectionPaused bool
	pauseCh          chan struct{}
	resumeCh         chan struct{}
}

// ResourceLimiterConfig 资源限制器配置
type ResourceLimiterConfig struct {
	Limit          ResourceLimit
	Interval       time.Duration // 检查间隔
	SilentDuration time.Duration // 静默持续时间
	OnLimit        func()
	OnSilent       func()
	OnRecover      func()
}

// DefaultResourceLimiterConfig 默认配置
func DefaultResourceLimiterConfig() ResourceLimiterConfig {
	return ResourceLimiterConfig{
		Limit: ResourceLimit{
			MaxCPUCore:    1.0,   // 默认1核
			MaxMemoryMB:   1024,  // 默认1GB
			MaxGoroutines: 10000,
		},
		Interval:       5 * time.Second,  // 5秒检查一次
		SilentDuration: 60 * time.Second, // 静默60秒
	}
}

// NewResourceLimiter 创建资源限制器
func NewResourceLimiter(log *logger.Logger, cfg ResourceLimiterConfig) *ResourceLimiter {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.SilentDuration <= 0 {
		cfg.SilentDuration = 60 * time.Second
	}
	
	return &ResourceLimiter{
		log:            log,
		limit:          cfg.Limit,
		interval:       cfg.Interval,
		silentDuration: cfg.SilentDuration,
		onLimit:        cfg.OnLimit,
		onSilent:       cfg.OnSilent,
		onRecover:      cfg.OnRecover,
		stopCh:         make(chan struct{}),
		pauseCh:        make(chan struct{}),
		resumeCh:       make(chan struct{}),
	}
}

// Start 启动资源限制器
func (rl *ResourceLimiter) Start() {
	go rl.monitorLoop()
	rl.log.Infof("资源限制器已启动: CPU≤%.1f核, 内存≤%.0fMB, 协程≤%d",
		rl.limit.MaxCPUCore, rl.limit.MaxMemoryMB, rl.limit.MaxGoroutines)
}

// Stop 停止资源限制器
func (rl *ResourceLimiter) Stop() {
	close(rl.stopCh)
	rl.log.Info("资源限制器已停止")
}

// IsLimited 检查是否处于限制状态
func (rl *ResourceLimiter) IsLimited() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.isLimited
}

// IsSilent 检查是否处于静默状态
func (rl *ResourceLimiter) IsSilent() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.isSilent
}

// IsCollectionPaused 检查采集是否暂停
func (rl *ResourceLimiter) IsCollectionPaused() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.collectionPaused
}

// WaitForResume 等待恢复信号
func (rl *ResourceLimiter) WaitForResume(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-rl.resumeCh:
		return nil
	}
}

// monitorLoop 监控循环
func (rl *ResourceLimiter) monitorLoop() {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.checkAndLimit()
		case <-rl.stopCh:
			return
		}
	}
}

// checkAndLimit 检查资源并执行限制
func (rl *ResourceLimiter) checkAndLimit() {
	stats := rl.collectStats()
	
	// 检查是否超限
	cpuExceeded := rl.limit.MaxCPUCore > 0 && stats.CPUUsage > rl.limit.MaxCPUCore*100
	memExceeded := rl.limit.MaxMemoryMB > 0 && stats.MemoryMB > rl.limit.MaxMemoryMB
	goroutineExceeded := rl.limit.MaxGoroutines > 0 && stats.GoroutineNum > rl.limit.MaxGoroutines
	
	exceeded := cpuExceeded || memExceeded || goroutineExceeded
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	if exceeded && !rl.isLimited {
		// 触发资源限制
		rl.isLimited = true
		rl.enterSilentModeLocked()
		
		if rl.onLimit != nil {
			go rl.onLimit()
		}
		
		rl.log.Warnf("资源超限触发限制: CPU=%.1f%%(>%d%%), 内存=%.0fMB(>%dMB), 协程=%d(>%d)",
			stats.CPUUsage, int(rl.limit.MaxCPUCore*100),
			stats.MemoryMB, int(rl.limit.MaxMemoryMB),
			stats.GoroutineNum, rl.limit.MaxGoroutines)
		
	} else if !exceeded && rl.isLimited {
		// 检查是否可以恢复
		if time.Since(rl.silentStart) >= rl.silentDuration {
			// 恢复服务
			rl.isLimited = false
			rl.isSilent = false
			rl.collectionPaused = false
			
			// 发送恢复信号
			select {
			case rl.resumeCh <- struct{}{}:
			default:
			}
			
			if rl.onRecover != nil {
				go rl.onRecover()
			}
			
			rl.log.Info("资源恢复正常，退出限制模式")
		}
	}
}

// enterSilentModeLocked 进入静默模式（调用前必须持有锁）
func (rl *ResourceLimiter) enterSilentModeLocked() {
	rl.isSilent = true
	rl.silentStart = time.Now()
	rl.collectionPaused = true
	
	// 发送暂停信号
	select {
	case rl.pauseCh <- struct{}{}:
	default:
	}
	
	// 触发GC释放内存
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
		rl.log.Info("资源限制: 已触发GC和内存释放")
	}()
	
	if rl.onSilent != nil {
		go rl.onSilent()
	}
	
	rl.log.Warnf("进入静默模式，持续时间: %s", rl.silentDuration)
}

// collectStats 采集资源统计
func (rl *ResourceLimiter) collectStats() ResourceStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// 获取CPU使用率 (简化实现，实际需要更复杂的计算)
	cpuUsage := rl.getCPUUsage()
	
	return ResourceStats{
		Timestamp:    time.Now(),
		GoroutineNum: runtime.NumGoroutine(),
		MemoryMB:     float64(memStats.Alloc) / 1024 / 1024,
		CPUUsage:     cpuUsage,
	}
}

// getCPUUsage 获取CPU使用率
func (rl *ResourceLimiter) getCPUUsage() float64 {
	// 使用简单的自实现方式获取CPU使用率
	// 实际生产环境建议使用gopsutil等库
	
	var usage syscall.Rusage
	err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	if err != nil {
		return 0
	}
	
	// 计算用户态+内核态CPU时间
	totalTime := float64(usage.Utime.Sec+usage.Stime.Sec)*1000 +
		float64(usage.Utime.Usec+usage.Stime.Usec)/1000
	
	// 简化的CPU使用率估算 (实际应该基于时间差计算)
	return totalTime / 10 // 简化计算
}

// ForceGC 强制垃圾回收
func (rl *ResourceLimiter) ForceGC() {
	runtime.GC()
	debug.FreeOSMemory()
	rl.log.Info("强制GC和内存释放完成")
}

// GetStats 获取当前资源统计
func (rl *ResourceLimiter) GetStats() ResourceStats {
	return rl.collectStats()
}

// Status 返回限制器状态
func (rl *ResourceLimiter) Status() string {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	stats := rl.collectStats()
	return fmt.Sprintf("CPU=%.1f%%, Memory=%.0fMB, Goroutines=%d, Limited=%v, Silent=%v",
		stats.CPUUsage, stats.MemoryMB, stats.GoroutineNum, rl.isLimited, rl.isSilent)
}
