// Package testutil 提供测试共享工具
package testutil

import "cloud-flow-edge/pkg/logger"

// NewTestLogger 创建测试用日志器
func NewTestLogger() *logger.Logger {
	return logger.New(logger.Config{Level: "error", Format: "console"})
}
