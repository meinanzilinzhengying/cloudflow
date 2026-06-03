package circuitbreaker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBreakerInitialState(t *testing.T) {
	config := Config{
		CheckInterval:        1 * time.Second,
		CPUDegradedThreshold: 80.0,
		CPUSilentThreshold:   95.0,
		MemDegradedThreshold: 80.0,
		MemSilentThreshold:   95.0,
	}

	breaker := NewBreaker(config)
	assert.NotNil(t, breaker, "熔断器应成功创建")
	assert.Equal(t, StateNormal, breaker.State(), "初始状态应为 Normal")
}

func TestBreakerStateTransition(t *testing.T) {
	config := Config{
		CheckInterval:             100 * time.Millisecond,
		CPUDegradedThreshold:      80.0,
		CPUSilentThreshold:        95.0,
		MemDegradedThreshold:      80.0,
		MemSilentThreshold:        95.0,
		CPUDegradedDuration:       200 * time.Millisecond,
		CPURecoverThreshold:       70.0,
		MemRecoverThreshold:       70.0,
		SilentCPURecoverThreshold: 85.0,
		SilentMemRecoverThreshold: 85.0,
	}

	breaker := NewBreaker(config)
	
	// 模拟资源快照 - CPU 超过降级阈值
	snapshot := ResourceSnapshot{
		CPUUsagePercent: 85.0,
		MemoryUsageMB:   100,
		MaxMemoryMB:     1000,
		MaxCPUCores:     4,
	}

	// 检查是否应该降级
	shouldDegrade := breaker.shouldDegrade(snapshot)
	assert.True(t, shouldDegrade, "CPU 85% 应触发降级")

	// 检查是否应该静默
	shouldSilent := breaker.shouldSilent(snapshot)
	assert.False(t, shouldSilent, "CPU 85% 不应触发静默")
}

func TestBreakerSilentMode(t *testing.T) {
	config := Config{
		CheckInterval:        100 * time.Millisecond,
		CPUDegradedThreshold: 80.0,
		CPUSilentThreshold:   95.0,
		MemDegradedThreshold: 80.0,
		MemSilentThreshold:   95.0,
	}

	breaker := NewBreaker(config)

	// 模拟资源快照 - CPU 超过静默阈值
	snapshot := ResourceSnapshot{
		CPUUsagePercent: 96.0,
		MemoryUsageMB:   100,
		MaxMemoryMB:     1000,
		MaxCPUCores:     4,
	}

	shouldSilent := breaker.shouldSilent(snapshot)
	assert.True(t, shouldSilent, "CPU 96% 应触发静默")
}

func TestBreakerRecovery(t *testing.T) {
	config := Config{
		CheckInterval:             100 * time.Millisecond,
		CPUDegradedThreshold:      80.0,
		CPUSilentThreshold:        95.0,
		MemDegradedThreshold:      80.0,
		MemSilentThreshold:        95.0,
		CPURecoverThreshold:       70.0,
		MemRecoverThreshold:       70.0,
		SilentCPURecoverThreshold: 85.0,
		SilentMemRecoverThreshold: 85.0,
	}

	breaker := NewBreaker(config)

	// 从降级状态恢复的快照
	snapshot := ResourceSnapshot{
		CPUUsagePercent: 65.0, // 低于恢复阈值 70%
		MemoryUsageMB:   100,
		MaxMemoryMB:     1000,
		MaxCPUCores:     4,
	}

	shouldRecover := breaker.shouldRecoverFromDegraded(snapshot)
	assert.True(t, shouldRecover, "CPU 65% 应从降级状态恢复")
}

func TestBreakerConfigDefaults(t *testing.T) {
	config := Config{
		CheckInterval: 1 * time.Second,
		// 其他字段使用默认值
	}

	breaker := NewBreaker(config)
	assert.NotNil(t, breaker, "使用部分配置应成功创建熔断器")
}

func TestResourceSnapshotCalculation(t *testing.T) {
	snapshot := ResourceSnapshot{
		CPUUsagePercent: 75.5,
		MemoryUsageMB:   512,
		MaxMemoryMB:     1024,
		MaxCPUCores:     4,
	}

	assert.InDelta(t, 75.5, snapshot.CPUUsagePercent, 0.01, "CPU 使用率应正确")
	assert.Equal(t, 512, snapshot.MemoryUsageMB, "内存使用应正确")
	assert.Equal(t, 1024, snapshot.MaxMemoryMB, "最大内存应正确")
	
	// 计算内存使用百分比
	memPercent := float64(snapshot.MemoryUsageMB) / float64(snapshot.MaxMemoryMB) * 100
	assert.InDelta(t, 50.0, memPercent, 0.01, "内存使用百分比应为 50%")
}

func TestBreakerStartStop(t *testing.T) {
	config := Config{
		CheckInterval:        100 * time.Millisecond,
		CPUDegradedThreshold: 80.0,
		CPUSilentThreshold:   95.0,
		MemDegradedThreshold: 80.0,
		MemSilentThreshold:   95.0,
	}

	breaker := NewBreaker(config)
	
	// 启动熔断器
	breaker.Start()
	
	// 等待一小段时间让 goroutine 运行
	time.Sleep(150 * time.Millisecond)
	
	// 停止熔断器
	breaker.Stop()
	
	// 验证可以安全停止多次
	breaker.Stop() // 第二次调用不应 panic
}

func TestOnStateChangeCallback(t *testing.T) {
	config := Config{
		CheckInterval:        1 * time.Second,
		CPUDegradedThreshold: 80.0,
		CPUSilentThreshold:   95.0,
		MemDegradedThreshold: 80.0,
		MemSilentThreshold:   95.0,
	}

	breaker := NewBreaker(config)
	
	callbackCalled := false
	breaker.OnStateChange(func(from, to State, snapshot ResourceSnapshot) {
		callbackCalled = true
	})

	// 触发状态变化（这里只是测试回调注册）
	assert.NotNil(t, breaker.stateChangeCallback, "状态变化回调应已注册")
}
