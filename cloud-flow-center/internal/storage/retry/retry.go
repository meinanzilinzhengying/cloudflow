// Package retry 存储重试工具
//
// P1-08 修复: 实现指数退避重试算法
//
// 提供带指数退避的重试机制，用于处理临时性故障（如网络抖动、数据库连接池耗尽等）。
package retry

import (
	"context"
	"fmt"
	"time"
)

// Config 重试配置
type Config struct {
	MaxAttempts   int           // 最大重试次数
	InitialDelay  time.Duration // 初始延迟
	MaxDelay      time.Duration // 最大延迟
	Jitter        bool          // 是否添加抖动
	BackoffFactor float64       // 退避因子（指数）
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts:   3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      10 * time.Second,
		Jitter:        true,
		BackoffFactor: 2.0,
	}
}

// Do 执行带重试的操作
//
// P1-08 修复: 使用指数退避算法重试失败的存储操作
func Do(ctx context.Context, cfg *Config, operation func() error) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		// 最后一次尝试失败，直接返回
		if attempt == cfg.MaxAttempts {
			break
		}

		// 计算延迟时间
		actualDelay := delay
		if cfg.Jitter {
			actualDelay = addJitter(delay)
		}

		// 等待
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-time.After(actualDelay):
		}

		// 更新延迟（指数退避）
		delay = time.Duration(float64(delay) * cfg.BackoffFactor)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// DoWithResult 执行带重试的操作并返回结果
func DoWithResult(ctx context.Context, cfg *Config, operation func() (interface{}, error)) (interface{}, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		default:
		}

		result, err := operation()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// 最后一次尝试失败，直接返回
		if attempt == cfg.MaxAttempts {
			break
		}

		// 计算延迟时间
		actualDelay := delay
		if cfg.Jitter {
			actualDelay = addJitter(delay)
		}

		// 等待
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		case <-time.After(actualDelay):
		}

		// 更新延迟（指数退避）
		delay = time.Duration(float64(delay) * cfg.BackoffFactor)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return nil, fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 判断是否为临时性错误
	errStr := err.Error()

	// 网络错误
	if containsAny(errStr, "connection refused", "timeout", "temporary failure", "i/o timeout", "connection reset") {
		return true
	}

	// 数据库错误
	if containsAny(errStr, "too many connections", "connection pool", "deadlock", "lock wait") {
		return true
	}

	// ClickHouse 特定错误
	if containsAny(errStr, "code: 242", "code: 164", "code: 999", "Memory limit") {
		return true
	}

	return false
}

// addJitter 添加随机抖动
func addJitter(delay time.Duration) time.Duration {
	// 添加 0-50% 的随机抖动
	jitter := time.Duration(float64(delay) * 0.5 * (float64(time.Now().UnixNano()%100) / 100.0))
	return delay + jitter
}

// containsAny 检查字符串是否包含任意子串
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) && containsSubstring(s, sub) {
			return true
		}
	}
	return false
}

// containsSubstring 检查 s 是否包含 sub
func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// RetryableOperation 可重试操作接口
type RetryableOperation func() error

// RetryWithBackoff 使用指定配置的指数退避重试
func RetryWithBackoff(ctx context.Context, cfg *Config, ops ...RetryableOperation) error {
	for _, op := range ops {
		if err := Do(ctx, cfg, op); err != nil {
			return err
		}
	}
	return nil
}

// RetryableError 包装错误为可重试错误
type RetryableError struct {
	Err error
}

// Error 实现 error 接口
func (e *RetryableError) Error() string {
	return e.Err.Error()
}

// Unwrap 解包错误
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError 创建可重试错误
func NewRetryableError(err error) error {
	return &RetryableError{Err: err}
}
