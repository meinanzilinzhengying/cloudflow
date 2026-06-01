package errors

import (
	"errors"
	"fmt"
	"testing"
)

// TestNew 测试创建新错误
func TestNew(t *testing.T) {
	err := New("test_code", "test message")
	if err == nil {
		t.Fatal("New 应该返回非 nil 错误")
	}
	if err.Code != "test_code" {
		t.Errorf("Code = %q, want %q", err.Code, "test_code")
	}
	if err.Message != "test message" {
		t.Errorf("Message = %q, want %q", err.Message, "test message")
	}
	if err.Err != nil {
		t.Errorf("Err 应该为 nil")
	}
}

// TestWrap 测试错误包装
func TestWrap(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrap(original, "wrapped_code", "wrapped message")

	if wrapped == nil {
		t.Fatal("Wrap 应该返回非 nil 错误")
	}
	if wrapped.Code != "wrapped_code" {
		t.Errorf("Code = %q, want %q", wrapped.Code, "wrapped_code")
	}
	if wrapped.Message != "wrapped message" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "wrapped message")
	}
	if wrapped.Err != original {
		t.Errorf("Err = %v, want %v", wrapped.Err, original)
	}
}

// TestWrap_WithNil 测试 nil 错误包装
func TestWrap_WithNil(t *testing.T) {
	wrapped := Wrap(nil, "code", "message")
	if wrapped != nil {
		t.Error("Wrap(nil) 应该返回 nil")
	}
}

// TestWrapf 测试格式化错误包装
func TestWrapf(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrapf(original, "formatted_code", "user: %s, action: %s", "alice", "login")

	if wrapped.Code != "formatted_code" {
		t.Errorf("Code = %q, want %q", wrapped.Code, "formatted_code")
	}
	if wrapped.Message != "user: alice, action: login" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "user: alice, action: login")
	}
}

// TestError_Error 测试错误格式化
func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "无原始错误",
			err:      New("code1", "message1"),
			expected: "[code1] message1",
		},
		{
			name:     "带原始错误",
			err:      Wrap(errors.New("original"), "code2", "message2"),
			expected: "[code2] message2: original",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestError_IsCode 测试错误代码检查
func TestError_IsCode(t *testing.T) {
	err := New("test_code", "test message")

	if !err.IsCode("test_code") {
		t.Error("IsCode 应该返回 true 当代码匹配")
	}
	if err.IsCode("wrong_code") {
		t.Error("IsCode 应该返回 false 当代码不匹配")
	}
}

// TestError_Cause 测试获取根本原因
func TestError_Cause(t *testing.T) {
	original := errors.New("root cause")
	wrapped1 := Wrap(original, "level1", "level1 message")
	wrapped2 := Wrap(wrapped1, "level2", "level2 message")

	cause := wrapped2.Cause()
	if cause != original {
		t.Errorf("Cause() = %v, want %v", cause, original)
	}
}

// TestError_CodeList 测试错误链中的所有代码
func TestError_CodeList(t *testing.T) {
	original := errors.New("original")
	wrapped1 := Wrap(original, "code1", "msg1")
	wrapped2 := Wrap(wrapped1, "code2", "msg2")
	wrapped3 := Wrap(wrapped2, "code3", "msg3")

	codes := wrapped3.CodeList()
	expected := []string{"code3", "code2", "code1"}

	if len(codes) != len(expected) {
		t.Fatalf("CodeList() 返回 %d 个元素, want %d", len(codes), len(expected))
	}

	for i, code := range codes {
		if code != expected[i] {
			t.Errorf("CodeList()[%d] = %q, want %q", i, code, expected[i])
		}
	}
}

// TestError_Format 测试完整错误链格式化
func TestError_Format(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrap(original, "test_code", "wrapped message")

	formatted := wrapped.Format()
	if formatted == "" {
		t.Error("Format() 应该返回非空字符串")
	}

	// 验证格式化字符串包含关键信息
	if !contains(formatted, "[test_code]") {
		t.Error("Format() 应该包含错误代码")
	}
	if !contains(formatted, "wrapped message") {
		t.Error("Format() 应该包含错误消息")
	}
}

// TestInvalidParam 测试参数错误快捷函数
func TestInvalidParam(t *testing.T) {
	err := InvalidParam("invalid parameter")
	if !err.IsCode(CodeInvalidParam) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeInvalidParam)
	}
}

// TestNotFound 测试未找到错误快捷函数
func TestNotFound(t *testing.T) {
	err := NotFound("resource not found")
	if !err.IsCode(CodeNotFound) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeNotFound)
	}
}

// TestInternalError 测试内部错误快捷函数
func TestInternalError(t *testing.T) {
	err := InternalError("internal error occurred")
	if !err.IsCode(CodeInternalError) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeInternalError)
	}
}

// TestUnauthorized 测试未授权错误快捷函数
func TestUnauthorized(t *testing.T) {
	err := Unauthorized("authentication required")
	if !err.IsCode(CodeUnauthorized) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeUnauthorized)
	}
}

// TestForbidden 测试禁止访问错误快捷函数
func TestForbidden(t *testing.T) {
	err := Forbidden("access denied")
	if !err.IsCode(CodeForbidden) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeForbidden)
	}
}

// TestTimeout 测试超时错误快捷函数
func TestTimeout(t *testing.T) {
	err := Timeout("operation timed out")
	if !err.IsCode(CodeTimeout) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeTimeout)
	}
}

// TestRateLimit 测试限流错误快捷函数
func TestRateLimit(t *testing.T) {
	err := RateLimit("rate limit exceeded")
	if !err.IsCode(CodeRateLimit) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeRateLimit)
	}
}

// TestDatabaseError 测试数据库错误快捷函数
func TestDatabaseError(t *testing.T) {
	original := errors.New("connection failed")
	err := DatabaseError(original, "database connection error")

	if !err.IsCode(CodeDatabaseError) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeDatabaseError)
	}
	if err.Err != original {
		t.Errorf("Err = %v, want %v", err.Err, original)
	}
}

// TestNetworkError 测试网络错误快捷函数
func TestNetworkError(t *testing.T) {
	original := errors.New("connection refused")
	err := NetworkError(original, "network connection error")

	if !err.IsCode(CodeNetworkError) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeNetworkError)
	}
}

// TestAuthenticationFail 测试认证失败错误快捷函数
func TestAuthenticationFail(t *testing.T) {
	err := AuthenticationFail("invalid credentials")
	if !err.IsCode(CodeAuthenticationFail) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeAuthenticationFail)
	}
}

// TestValidationError 测试验证错误快捷函数
func TestValidationError(t *testing.T) {
	err := ValidationError("validation failed")
	if !err.IsCode(CodeValidationError) {
		t.Errorf("IsCode() = %q, want %q", err.Code, CodeValidationError)
	}
}

// TestError_Unwrap 测试 Unwrap 接口
func TestError_Unwrap(t *testing.T) {
	original := errors.New("original error")
	wrapped := Wrap(original, "code", "message")

	unwrapped := wrapped.Unwrap()
	if unwrapped != original {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, original)
	}
}

// TestError_MultipleWrapping 测试多层错误包装
func TestError_MultipleWrapping(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := Wrap(err1, "code2", "message 2")
	err3 := Wrap(err2, "code3", "message 3")

	// 验证错误链
	if err3.Err != err2 {
		t.Error("错误链第 3 层链接错误")
	}
	if err2.Err != err1 {
		t.Error("错误链第 2 层链接错误")
	}

	// 验证格式化输出
	formatted := err3.Format()
	if !contains(formatted, "code3") || !contains(formatted, "code2") || !contains(formatted, "code1") {
		t.Error("格式化输出应该包含所有错误代码")
	}
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkErrorCreation 测试错误创建性能
func BenchmarkErrorCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = New("code", "message")
	}
}

// BenchmarkErrorWrapping 测试错误包装性能
func BenchmarkErrorWrapping(b *testing.B) {
	original := errors.New("original")
	for i := 0; i < b.N; i++ {
		_ = Wrap(original, "code", "message")
	}
}

// BenchmarkErrorFormat 测试错误格式化性能
func BenchmarkErrorFormat(b *testing.B) {
	err := Wrap(Wrap(Wrap(errors.New("original"), "code1", "msg1"), "code2", "msg2"), "code3", "msg3")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Format()
	}
}

// BenchmarkErrorCause 测试获取根本原因性能
func BenchmarkErrorCause(b *testing.B) {
	err := Wrap(Wrap(Wrap(errors.New("original"), "code1", "msg1"), "code2", "msg2"), "code3", "msg3")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = err.Cause()
	}
}
