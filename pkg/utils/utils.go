package utils

import (
	"fmt"
	"strconv"
)

// MaskSecret 对敏感字符串进行脱敏处理
// 保留前4后4位，中间用 *** 替代
func MaskSecret(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}

// ParseFloat 安全解析浮点数字符串
func ParseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// SafeFloat64 安全获取 float64 指针
func SafeFloat64(v float64) *float64 {
	return &v
}

// SafeInt64 安全获取 int64 指针
func SafeInt64(v int64) *int64 {
	return &v
}

// SafeString 安全获取 string 指针
func SafeString(v string) *string {
	return &v
}

// FormatBytes 格式化字节数为可读字符串
func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
