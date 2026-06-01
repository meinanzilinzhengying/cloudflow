// Package logger 提供基于 zap 的日志组件
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger 日志封装
type Logger struct {
	*zap.SugaredLogger
}

// Config 日志配置
type Config struct {
	Level      string `mapstructure:"level"`  // debug, info, warn, error
	Format     string `mapstructure:"format"` // json, console
	Output     string `mapstructure:"output"` // stdout, file, both
	LogDir     string `mapstructure:"log_dir"`
	MaxSize    int    `mapstructure:"max_size"` // MB
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // days
}

// New 创建日志实例
func New(cfg Config) *Logger {
	// 解析日志级别
	level := parseLevel(cfg.Level)

	// 配置编码器
	var encoder zapcore.Encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if strings.ToLower(cfg.Format) == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 构建输出
	var writers []zapcore.WriteSyncer
	if cfg.Output == "stdout" || cfg.Output == "both" {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}
	if (cfg.Output == "file" || cfg.Output == "both") && cfg.LogDir != "" {
		// 确保日志目录存在
		if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
			// 目录创建失败，使用标准输出
			writers = append(writers, zapcore.AddSync(os.Stdout))
		} else {
			// 创建文件输出
			fileWriter := NewFileWriter(cfg.LogDir, "cloud-flow-edge", cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)
			writers = append(writers, zapcore.AddSync(fileWriter))
		}
	}

	// 如果没有配置输出，默认使用标准输出
	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	// 创建 core
	core := zapcore.NewCore(
		encoder,
		zapcore.NewMultiWriteSyncer(writers...),
		level,
	)

	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	return &Logger{SugaredLogger: zapLogger.Sugar()}
}

// parseLevel 将字符串日志级别转为 zapcore.Level
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

// WithFields 创建带字段的子日志器
func (l *Logger) WithFields(fields ...interface{}) *Logger {
	return &Logger{SugaredLogger: l.SugaredLogger.With(fields...)}
}

// Sync 刷新缓冲区
func (l *Logger) Sync() {
	_ = l.SugaredLogger.Sync()
}

// FileWriter 文件写入器，支持日志轮转
type FileWriter struct {
	mu            sync.Mutex
	logDir        string
	fileName      string
	maxSize       int
	maxBackups    int
	maxAge        int
	currentFile   *os.File
	currentSize   int64
}

// NewFileWriter 创建文件写入器
func NewFileWriter(logDir, fileName string, maxSize, maxBackups, maxAge int) *FileWriter {
	if maxSize <= 0 {
		maxSize = 100 // 默认 100MB
	}
	if maxBackups <= 0 {
		maxBackups = 10 // 默认 10 个备份
	}
	if maxAge <= 0 {
		maxAge = 7 // 默认 7 天
	}

	writer := &FileWriter{
		logDir:     logDir,
		fileName:   fileName,
		maxSize:    maxSize * 1024 * 1024, // 转换为字节
		maxBackups: maxBackups,
		maxAge:     maxAge,
	}

	// 初始化文件
	writer.rotate()
	return writer
}

// Write 写入数据
func (w *FileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 检查是否需要轮转
	if w.currentFile == nil || w.currentSize+int64(len(p)) > int64(w.maxSize) {
		w.rotate()
	}

	// 写入数据
	n, err = w.currentFile.Write(p)
	if err == nil {
		w.currentSize += int64(n)
	}

	// 强制刷新
	_ = w.currentFile.Sync()
	return
}

// rotate 轮转日志文件（调用者必须持有 w.mu 锁）
func (w *FileWriter) rotate() {
	// 关闭当前文件
	if w.currentFile != nil {
		_ = w.currentFile.Close()
	}

	// 生成新文件名
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	newFileName := filepath.Join(w.logDir, fmt.Sprintf("%s-%s.log", w.fileName, timestamp))

	// 创建新文件
	file, err := os.OpenFile(newFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// 文件创建失败，使用标准输出
		file = os.Stdout
	}

	w.currentFile = file
	w.currentSize = 0

	// 清理过期文件
	w.cleanup()
}

// cleanup 清理过期文件
func (w *FileWriter) cleanup() {
	files, err := os.ReadDir(w.logDir)
	if err != nil {
		return
	}

	var logFiles []os.DirEntry
	for _, file := range files {
		if !file.IsDir() && strings.Contains(file.Name(), w.fileName) && strings.HasSuffix(file.Name(), ".log") {
			logFiles = append(logFiles, file)
		}
	}

	// 按修改时间排序
	for i := 0; i < len(logFiles); i++ {
		for j := i + 1; j < len(logFiles); j++ {
			info1, _ := logFiles[i].Info()
			info2, _ := logFiles[j].Info()
			if info1.ModTime().Before(info2.ModTime()) {
				logFiles[i], logFiles[j] = logFiles[j], logFiles[i]
			}
		}
	}

	// 保留最新的 maxBackups 个文件
	if len(logFiles) > w.maxBackups {
		for i := w.maxBackups; i < len(logFiles); i++ {
			_ = os.Remove(filepath.Join(w.logDir, logFiles[i].Name()))
		}
	}

	// 清理超过 maxAge 天的文件
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	for _, file := range logFiles {
		info, _ := file.Info()
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(w.logDir, file.Name()))
		}
	}
}
