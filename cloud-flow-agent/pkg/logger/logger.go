// Package logger 提供基于 zap 的日志组件
package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.SugaredLogger
}

type Config struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func New(cfg Config) *Logger {
	level := parseLevel(cfg.Level)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Format) == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return &Logger{SugaredLogger: zapLogger.Sugar()}
}

func parseLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Sync 覆盖 zap.SugaredLogger 的 Sync 方法。
// 除了刷新 zap 内部缓冲区外，还额外调用 os.Stdout.Sync()，
// 确保当日志输出被重定向到文件时（如 Docker 日志驱动或 nohup），
// 缓冲区中的日志能实时刷写到磁盘，避免在进程异常退出时丢失最近的日志。
func (l *Logger) Sync() {
	_ = l.SugaredLogger.Sync()
	// 强制 flush stdout，确保重定向到文件时日志实时输出
	_ = os.Stdout.Sync()
}
