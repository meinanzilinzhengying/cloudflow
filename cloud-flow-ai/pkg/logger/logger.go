package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	Level      string
	Format     string
	Output     string
	LogDir     string
	MaxSize    int
	MaxBackups int
	MaxAge     int
}

type Logger struct {
	zap *zap.Logger
	sugar *zap.SugaredLogger
}

func New(cfg Config) *Logger {
	level := zapcore.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	zapCfg := zap.NewProductionConfig()
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	if cfg.Format == "json" {
		zapCfg.Encoding = "json"
	}

	logger, _ := zapCfg.Build()
	return &Logger{
		zap: logger,
		sugar: logger.Sugar(),
	}
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.sugar.Debugw(msg, args...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.sugar.Debugf(format, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.sugar.Infow(msg, args...)
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.sugar.Infof(format, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.sugar.Warnw(msg, args...)
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.sugar.Warnf(format, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.sugar.Errorw(msg, args...)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.sugar.Errorf(format, args...)
}

func (l *Logger) With(args ...interface{}) *Logger {
	return &Logger{
		zap: l.zap.With(toZapFields(args)...),
		sugar: l.sugar.With(args...),
	}
}

func toZapFields(args []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(args))
	for i := 0; i < len(args)-1; i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		value := args[i+1]
		fields = append(fields, zap.Any(key, value))
	}
	return fields
}

func (l *Logger) Sync() {
	_ = l.zap.Sugar().Sync()
}
