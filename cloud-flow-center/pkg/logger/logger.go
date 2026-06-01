// Package logger 提供基于 zap 的日志组件
package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	*zap.SugaredLogger
}

type Config struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"` // stdout, file, both
	LogDir     string `mapstructure:"log_dir"`
	MaxSize    int    `mapstructure:"max_size"` // MB
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // days
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
			fileWriter := NewFileWriter(cfg.LogDir, "cloud-flow-center", cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)
			writers = append(writers, zapcore.AddSync(fileWriter))
		}
	}

	// 如果没有配置输出，默认使用标准输出
	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	core := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(writers...), level)
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

func (l *Logger) Sync() { _ = l.SugaredLogger.Sync() }

// WithContext 从 context 中提取 Trace ID 并返回一个新的 Logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return l
}

// FileWriter 文件写入器，支持日志轮转
type FileWriter struct {
	logDir      string
	fileName    string
	maxSize     int
	maxBackups  int
	maxAge      int
	currentFile *os.File
	currentSize int64
	rotateIndex int
	mu          sync.Mutex
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

// rotate 轮转日志文件
// 使用固定日志文件名（如 cloud-flow-center.log），轮转时将当前文件重命名为带序号的归档文件
func (w *FileWriter) rotate() {
	// 关闭当前文件
	if w.currentFile != nil {
		_ = w.currentFile.Close()
	}

	// 将当前活跃日志文件重命名为归档文件（如果存在且非空）
	activeFile := filepath.Join(w.logDir, w.fileName+".log")
	if info, err := os.Stat(activeFile); err == nil && info.Size() > 0 {
		w.rotateIndex++
		archiveName := filepath.Join(w.logDir, fmt.Sprintf("%s-%d.log", w.fileName, w.rotateIndex))
		// 如果归档文件已存在，继续递增序号
		for {
			if _, err := os.Stat(archiveName); os.IsNotExist(err) {
				break
			}
			w.rotateIndex++
			archiveName = filepath.Join(w.logDir, fmt.Sprintf("%s-%d.log", w.fileName, w.rotateIndex))
		}
		if err := os.Rename(activeFile, archiveName); err != nil {
			// 重命名失败，使用带时间戳的文件名作为回退
			timestamp := time.Now().Format("2006-01-02-15-04-05")
			archiveName = filepath.Join(w.logDir, fmt.Sprintf("%s-%s.log", w.fileName, timestamp))
			_ = os.Rename(activeFile, archiveName)
		}
	}

	// 创建新的活跃日志文件
	file, err := os.OpenFile(activeFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// 文件创建失败，使用标准输出
		file = os.Stdout
	}

	w.currentFile = file
	// 通过 Stat 获取实际文件大小，而非依赖内部计数器
	if info, err := file.Stat(); err == nil {
		w.currentSize = info.Size()
	} else {
		w.currentSize = 0
	}

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
	sort.Slice(logFiles, func(i, j int) bool {
		info1, _ := logFiles[i].Info()
		info2, _ := logFiles[j].Info()
		return info1.ModTime().Before(info2.ModTime())
	})

	// 保留最新的 maxBackups 个文件（跳过当前活跃文件）
	activeFileName := w.fileName + ".log"
	if len(logFiles) > w.maxBackups {
		for i := w.maxBackups; i < len(logFiles); i++ {
			if logFiles[i].Name() == activeFileName {
				continue // 跳过当前活跃文件
			}
			_ = os.Remove(filepath.Join(w.logDir, logFiles[i].Name()))
		}
	}

	// 清理超过 maxAge 天的文件（跳过当前活跃文件）
	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	for _, file := range logFiles {
		if file.Name() == activeFileName {
			continue // 跳过当前活跃文件
		}
		info, err := file.Info()
		if err != nil || info == nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(w.logDir, file.Name()))
		}
	}
}
