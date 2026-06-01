package errors

import (
	"fmt"
	"strings"
)

// Error 是项目的自定义错误类型，支持上下文包装和堆栈信息
type Error struct {
	Code    string
	Message string
	Err     error
}

// New 创建新的错误
func New(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Wrap 包装原始错误，添加上下文信息
func Wrap(err error, code, message string) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Wrapf 包装原始错误，添加上下文信息（格式化字符串）
func Wrapf(err error, code, format string, args ...interface{}) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Err:     err,
	}
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
}

// Unwrap 实现 unwrap 接口，用于 errors.Is 和 errors.As
func (e *Error) Unwrap() error {
	return e.Err
}

// Is 判断错误链中是否包含指定的错误代码
func (e *Error) IsCode(code string) bool {
	return e != nil && e.Code == code
}

// Cause 获取最原始的错误
func (e *Error) Cause() error {
	current := e
	for current.Err != nil {
		if wrapped, ok := current.Err.(*Error); ok {
			current = wrapped
		} else {
			return current.Err
		}
	}
	return current
}

// CodeList 获取错误链中的所有代码
func (e *Error) CodeList() []string {
	var codes []string
	current := e
	for current != nil {
		codes = append(codes, current.Code)
		if wrapped, ok := current.Err.(*Error); ok {
			current = wrapped
		} else {
			break
		}
	}
	return codes
}

// Format 格式化完整的错误链
func (e *Error) Format() string {
	var sb strings.Builder
	current := e
	level := 0
	for current != nil {
		if level > 0 {
			sb.WriteString("\n")
			sb.WriteString(strings.Repeat("  ", level))
		}
		sb.WriteString(fmt.Sprintf("[%s] %s", current.Code, current.Message))
		if wrapped, ok := current.Err.(*Error); ok {
			current = wrapped
			level++
		} else if current.Err != nil {
			sb.WriteString(fmt.Sprintf(": %v", current.Err))
			break
		} else {
			break
		}
	}
	return sb.String()
}

// 常用错误代码
const (
	CodeInvalidParam       = "invalid_param"
	CodeNotFound           = "not_found"
	CodeInternalError      = "internal_error"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeTimeout            = "timeout"
	CodeRateLimit          = "rate_limit"
	CodeDatabaseError      = "database_error"
	CodeNetworkError       = "network_error"
	CodeAuthenticationFail = "authentication_fail"
	CodeValidationError    = "validation_error"
)

// InvalidParam 创建参数错误
func InvalidParam(message string) *Error {
	return New(CodeInvalidParam, message)
}

// NotFound 创建未找到错误
func NotFound(message string) *Error {
	return New(CodeNotFound, message)
}

// InternalError 创建内部错误
func InternalError(message string) *Error {
	return New(CodeInternalError, message)
}

// Unauthorized 创建未授权错误
func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, message)
}

// Forbidden 创建禁止访问错误
func Forbidden(message string) *Error {
	return New(CodeForbidden, message)
}

// Timeout 创建超时错误
func Timeout(message string) *Error {
	return New(CodeTimeout, message)
}

// RateLimit 创建限流错误
func RateLimit(message string) *Error {
	return New(CodeRateLimit, message)
}

// DatabaseError 创建数据库错误
func DatabaseError(err error, message string) *Error {
	return Wrap(err, CodeDatabaseError, message)
}

// NetworkError 创建网络错误
func NetworkError(err error, message string) *Error {
	return Wrap(err, CodeNetworkError, message)
}

// AuthenticationFail 创建认证失败错误
func AuthenticationFail(message string) *Error {
	return New(CodeAuthenticationFail, message)
}

// ValidationError 创建验证错误
func ValidationError(message string) *Error {
	return New(CodeValidationError, message)
}
