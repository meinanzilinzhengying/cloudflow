package safety

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// Logger 是一个简单的日志接口，用于记录错误和 panic
type Logger interface {
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
}

// PanicRecovery 用于捕获和恢复 panic
func PanicRecovery(logger Logger, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			logger.Errorf("捕获到 panic: %v\n堆栈: %s", r, string(stack))
		}
	}()
	fn()
}

// Go 是安全的 goroutine 启动函数，自动捕获 panic
func Go(logger Logger, fn func()) {
	go func() {
		PanicRecovery(logger, fn)
	}()
}

// SafeGo 带有名称的安全 goroutine 启动函数，用于更好的日志记录
func SafeGo(logger Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Errorf("goroutine '%s' 捕获到 panic: %v\n堆栈: %s", name, r, string(stack))
			}
		}()
		fn()
	}()
}

// SafeGoWithWaitGroup 创建带 WaitGroup 的安全 goroutine
func SafeGoWithWaitGroup(logger Logger, name string, wg *sync.WaitGroup, fn func()) {
	wg.Add(1)
	go func() {
		defer func() {
			wg.Done()
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Errorf("goroutine '%s' 捕获到 panic: %v\n堆栈: %s", name, r, string(stack))
			}
		}()
		fn()
	}()
}

// Must 是一个辅助函数，用于包装必须成功的操作
// 如果 err 不为 nil，则 panic（适用于初始化阶段）
func Must(err error) {
	if err != nil {
		panic(fmt.Sprintf("必须成功的操作失败: %v", err))
	}
}

// MustVal 是必须成功并返回值的辅助函数
func MustVal[T any](val T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("必须成功的操作失败: %v", err))
	}
	return val
}

// CheckAndLog 检查并记录错误，不处理
func CheckAndLog(logger Logger, err error, msg string) {
	if err != nil {
		logger.Errorf("%s: %v", msg, err)
	}
}

// CheckAndWarn 检查并以警告级别记录错误
func CheckAndWarn(logger Logger, err error, msg string) {
	if err != nil {
		logger.Warnf("%s: %v", msg, err)
	}
}
