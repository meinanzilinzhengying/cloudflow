package safety

import (
	"errors"
	"sync"
	"testing"
)

// TestPanicRecovery 测试 panic 恢复功能
func TestPanicRecovery(t *testing.T) {
	var loggerCalled bool
	logger := &mockLogger{errFunc: func(format string, args ...interface{}) {
		loggerCalled = true
	}}

	// 测试正常执行的函数
	PanicRecovery(logger, func() {
		// 正常执行，不应该 panic
	})

	if loggerCalled {
		t.Error("正常执行不应该调用 logger")
	}
}

// TestPanicRecovery_WithPanic 测试带 panic 的函数
func TestPanicRecovery_WithPanic(t *testing.T) {
	var loggerCalled bool
	var panicValue interface{}
	logger := &mockLogger{errFunc: func(format string, args ...interface{}) {
		loggerCalled = true
		// 从日志中提取 panic 值
		if len(args) > 0 {
			panicValue = args[0]
		}
	}}

	PanicRecovery(logger, func() {
		panic("test panic")
	})

	if !loggerCalled {
		t.Error("panic 应该调用 logger")
	}
	if panicValue != "test panic" {
		t.Errorf("panic value = %v, want 'test panic'", panicValue)
	}
}

// TestGo 测试安全协程启动
func TestGo(t *testing.T) {
	var counter int
	var mu sync.Mutex
	logger := &mockLogger{}

	Go(logger, func() {
		mu.Lock()
		counter++
		mu.Unlock()
	})

	// 等待协程执行
	for i := 0; i < 100; i++ {
		mu.Lock()
		if counter > 0 {
			mu.Unlock()
			break
		}
		mu.Unlock()
	}

	mu.Lock()
	if counter != 1 {
		t.Errorf("counter = %d, want 1", counter)
	}
	mu.Unlock()
}

// TestGo_WithPanic 测试安全协程中的 panic
func TestGo_WithPanic(t *testing.T) {
	logger := &mockLogger{}

	// 这不应该导致测试失败
	Go(logger, func() {
		panic("goroutine panic")
	})

	// 等待协程执行并记录 panic
	for i := 0; i < 100; i++ {
		logger.Lock()
		if logger.errCalled {
			logger.Unlock()
			break
		}
		logger.Unlock()
	}
}

// TestSafeGo 测试带名称的安全协程
func TestSafeGo(t *testing.T) {
	var executed bool
	logger := &mockLogger{}

	SafeGo(logger, "test-goroutine", func() {
		executed = true
	})

	// 等待协程执行
	for i := 0; i < 100; i++ {
		if executed {
			break
		}
	}

	if !executed {
		t.Error("协程应该被执行")
	}
}

// TestSafeGoWithWaitGroup 测试带 WaitGroup 的安全协程
func TestSafeGoWithWaitGroup(t *testing.T) {
	var wg sync.WaitGroup
	var counter int
	var mu sync.Mutex
	logger := &mockLogger{}

	for i := 0; i < 5; i++ {
		SafeGoWithWaitGroup(logger, "test-worker", &wg, func() {
			mu.Lock()
			counter++
			mu.Unlock()
		})
	}

	// 等待所有协程完成
	wg.Wait()

	mu.Lock()
	if counter != 5 {
		t.Errorf("counter = %d, want 5", counter)
	}
	mu.Unlock()
}

// TestMust 测试 Must 函数
func TestMust(t *testing.T) {
	// 测试没有错误的情况
	Must(nil) // 不应该 panic

	// 测试有错误的情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must 应该 panic 当 error 不为 nil")
		}
	}()

	Must(errors.New("test error"))
}

// TestMustVal 测试 MustVal 函数
func TestMustVal(t *testing.T) {
	// 测试正常返回值
	result := MustVal("test", nil)
	if result != "test" {
		t.Errorf("MustVal 返回 %q, want 'test'", result)
	}

	// 测试错误情况
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustVal 应该 panic 当 error 不为 nil")
		}
	}()

	MustVal("test", errors.New("test error"))
}

// TestCheckAndLog 测试错误检查和日志记录
func TestCheckAndLog(t *testing.T) {
	logger := &mockLogger{}

	// 测试 nil 错误
	CheckAndLog(logger, nil, "test")
	if logger.errCalled {
		t.Error("nil 错误不应该调用 logger")
	}

	// 测试非 nil 错误
	CheckAndLog(logger, errors.New("test error"), "error occurred")
	if !logger.errCalled {
		t.Error("非 nil 错误应该调用 logger")
	}
}

// TestCheckAndWarn 测试警告级别的错误检查
func TestCheckAndWarn(t *testing.T) {
	logger := &mockLogger{}

	// 测试 nil 错误
	CheckAndWarn(logger, nil, "test")
	if logger.warnCalled {
		t.Error("nil 错误不应该调用 logger")
	}

	// 测试非 nil 错误
	CheckAndWarn(logger, errors.New("test error"), "warning occurred")
	if !logger.warnCalled {
		t.Error("非 nil 错误应该调用 logger")
	}
}

// mockLogger 是测试用的 Logger 实现
type mockLogger struct {
	mu         sync.Mutex
	errCalled  bool
	warnCalled bool
}

func (l *mockLogger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errCalled = true
}

func (l *mockLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnCalled = true
}
