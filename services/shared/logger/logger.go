// Package logger 提供结构化日志、全链路 traceId、错误堆栈和审计日志功能
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ==============================
// 上下文 key 定义
// ==============================

type ctxKey string

const (
	// TraceIDKey 全链路追踪 ID
	TraceIDKey ctxKey = "trace_id"
	// ServiceNameKey 服务名称
	ServiceNameKey ctxKey = "service_name"
	// UserIDKey 用户 ID
	UserIDKey ctxKey = "user_id"
	// TenantIDKey 租户 ID
	TenantIDKey ctxKey = "tenant_id"
)

// ==============================
// 日志级别定义
// ==============================

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
	LevelFatal = "fatal"
)

// ==============================
// 审计日志事件类型
// ==============================

const (
	AuditTypeLogin      = "login"
	AuditTypeLogout     = "logout"
	AuditTypeCreate     = "create"
	AuditTypeUpdate     = "update"
	AuditTypeDelete     = "delete"
	AuditTypeRead       = "read"
	AuditTypeAccess     = "access"
	AuditTypePermission = "permission"
	AuditTypeSystem     = "system"
)

// ==============================
// 配置结构
// ==============================

type Config struct {
	Level            string `mapstructure:"level"`
	Format           string `mapstructure:"format"`      // json, console
	Output           string `mapstructure:"output"`      // stdout, file, both
	LogDir           string `mapstructure:"log_dir"`
	MaxSize          int    `mapstructure:"max_size"`     // MB
	MaxBackups       int    `mapstructure:"max_backups"`
	MaxAge           int    `mapstructure:"max_age"`     // days
	ServiceName      string `mapstructure:"service_name"`
	EnableAudit      bool   `mapstructure:"enable_audit"`
	AuditLogDir      string `mapstructure:"audit_log_dir"`
	EnableStackTrace bool   `mapstructure:"enable_stack_trace"`
}

// ==============================
// 结构化日志记录
// ==============================

// LogEntry 结构化日志条目
type LogEntry struct {
	Timestamp    time.Time              `json:"timestamp"`
	Level        string                 `json:"level"`
	ServiceName  string                 `json:"service_name"`
	TraceID      string                 `json:"trace_id"`
	UserID       string                 `json:"user_id,omitempty"`
	TenantID     string                 `json:"tenant_id,omitempty"`
	Message      string                 `json:"message"`
	Fields       map[string]interface{} `json:"fields,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Stack        string                 `json:"stack_trace,omitempty"`
	Caller       string                 `json:"caller"`
}

// AuditEntry 审计日志条目
type AuditEntry struct {
	Timestamp   time.Time              `json:"timestamp"`
	TraceID     string                 `json:"trace_id"`
	EventType   string                 `json:"event_type"`
	ServiceName string                 `json:"service_name"`
	UserID      string                 `json:"user_id,omitempty"`
	TenantID    string                 `json:"tenant_id,omitempty"`
	Resource    string                 `json:"resource"`
	Action      string                 `json:"action"`
	Result      string                 `json:"result"` // success, failed
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Duration    int64                  `json:"duration_ms,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
}

// ==============================
// Logger 结构体
// ==============================

type Logger struct {
	zapLogger *zap.SugaredLogger
	config    Config
	ctx       context.Context
	fields    []Field
}

// Field 日志字段
type Field struct {
	Key   string
	Value interface{}
}

// ==============================
// 全局实例
// ==============================

var (
	globalLogger *Logger
	globalMu     sync.RWMutex
)

// ==============================
// 构造函数
// ==============================

func New(cfg Config) *Logger {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "unknown-service"
	}
	if cfg.EnableStackTrace == false {
		cfg.EnableStackTrace = true // 默认启用
	}

	level := parseLevel(cfg.Level)
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stack_trace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

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
		if err := os.MkdirAll(cfg.LogDir, 0755); err == nil {
			fileWriter := NewFileWriter(cfg.LogDir, cfg.ServiceName, cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge)
			writers = append(writers, zapcore.AddSync(fileWriter))
		}
	}

	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	core := zapcore.NewCore(encoder, zapcore.NewMultiWriteSyncer(writers...), level)
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))

	return &Logger{
		zapLogger: zapLogger.Sugar(),
		config:    cfg,
		ctx:       context.Background(),
		fields:    []Field{},
	}
}

// ==============================
// 上下文操作
// ==============================

// WithTraceID 创建带有 traceId 的上下文
func WithTraceID(ctx context.Context) context.Context {
	traceID := uuid.New().String()
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// WithTraceIDValue 创建带有指定 traceId 的上下文
func WithTraceIDValue(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// TraceIDFromContext 从上下文获取 traceId
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// WithUserID 添加上下文用户 ID
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// UserIDFromContext 获取用户 ID
func UserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// WithTenantID 添加租户 ID
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// TenantIDFromContext 获取租户 ID
func TenantIDFromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
}

// WithServiceName 添加服务名称
func WithServiceName(ctx context.Context, serviceName string) context.Context {
	return context.WithValue(ctx, ServiceNameKey, serviceName)
}

// ServiceNameFromContext 获取服务名称
func ServiceNameFromContext(ctx context.Context) string {
	if serviceName, ok := ctx.Value(ServiceNameKey).(string); ok {
		return serviceName
	}
	return ""
}

// ==============================
// 方法
// ==============================

// WithContext 注入上下文到 logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	newLogger := *l
	newLogger.ctx = ctx
	return &newLogger
}

// With 添加字段
func (l *Logger) With(key string, value interface{}) *Logger {
	newLogger := *l
	newLogger.fields = append(newLogger.fields, Field{Key: key, Value: value})
	return &newLogger
}

// WithFields 添加多个字段
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := *l
	for k, v := range fields {
		newLogger.fields = append(newLogger.fields, Field{Key: k, Value: v})
	}
	return &newLogger
}

// ==============================
// 结构化日志方法
// ==============================

func (l *Logger) log(level string, msg string, keysAndValues ...interface{}) {
	fields := l.buildFields()

	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := fmt.Sprintf("%v", keysAndValues[i])
			fields[key] = keysAndValues[i+1]
		}
	}

	switch level {
	case LevelDebug:
		l.zapLogger.Debugw(msg, toZapFields(fields)...)
	case LevelInfo:
		l.zapLogger.Infow(msg, toZapFields(fields)...)
	case LevelWarn:
		l.zapLogger.Warnw(msg, toZapFields(fields)...)
	case LevelError:
		if l.config.EnableStackTrace {
			l.zapLogger.Errorw(msg, toZapFields(fields)...)
		} else {
			l.zapLogger.Errorw(msg, toZapFields(fields)...)
		}
	case LevelFatal:
		l.zapLogger.Fatalw(msg, toZapFields(fields)...)
	}
}

func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.log(LevelDebug, msg, keysAndValues...)
}

func (l *Logger) Info(msg string, keysAndValues ...interface{}) {
	l.log(LevelInfo, msg, keysAndValues...)
}

func (l *Logger) Warn(msg string, keysAndValues ...interface{}) {
	l.log(LevelWarn, msg, keysAndValues...)
}

func (l *Logger) Error(msg string, keysAndValues ...interface{}) {
	l.log(LevelError, msg, keysAndValues...)
}

func (l *Logger) Fatal(msg string, keysAndValues ...interface{}) {
	l.log(LevelFatal, msg, keysAndValues...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, fmt.Sprintf(format, args...))
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, fmt.Sprintf(format, args...))
}

func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...))
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(LevelFatal, fmt.Sprintf(format, args...))
}

// ==============================
// 带错误和堆栈的日志
// ==============================

func (l *Logger) ErrorWithStack(err error, msg string, keysAndValues ...interface{}) {
	stackTrace := getStackTrace()
	allFields := append(keysAndValues, "error", err.Error(), "stack_trace", stackTrace)
	l.log(LevelError, msg, allFields...)
}

func (l *Logger) ErrorfWithStack(format string, err error, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	stackTrace := getStackTrace()
	l.log(LevelError, msg, "error", err.Error(), "stack_trace", stackTrace)
}

// ==============================
// 审计日志
// ==============================

func (l *Logger) Audit(ctx context.Context, eventType, resource, action, result string, details map[string]interface{}) {
	if !l.config.EnableAudit {
		return
	}

	traceID := TraceIDFromContext(ctx)
	userID := UserIDFromContext(ctx)
	tenantID := TenantIDFromContext(ctx)

	entry := AuditEntry{
		Timestamp:   time.Now(),
		TraceID:     traceID,
		EventType:   eventType,
		ServiceName: l.config.ServiceName,
		UserID:      userID,
		TenantID:    tenantID,
		Resource:    resource,
		Action:      action,
		Result:      result,
		Details:     details,
	}

	l.writeAuditEntry(entry)
}

// AuditWithDuration 带执行时长的审计
func (l *Logger) AuditWithDuration(ctx context.Context, eventType, resource, action, result string, duration time.Duration, details map[string]interface{}) {
	if !l.config.EnableAudit {
		return
	}

	traceID := TraceIDFromContext(ctx)
	userID := UserIDFromContext(ctx)
	tenantID := TenantIDFromContext(ctx)

	entry := AuditEntry{
		Timestamp:   time.Now(),
		TraceID:     traceID,
		EventType:   eventType,
		ServiceName: l.config.ServiceName,
		UserID:      userID,
		TenantID:    tenantID,
		Resource:    resource,
		Action:      action,
		Result:      result,
		Duration:    duration.Milliseconds(),
		Details:     details,
	}

	l.writeAuditEntry(entry)
}

// ==============================
// 辅助方法
// ==============================

func (l *Logger) buildFields() map[string]interface{} {
	fields := make(map[string]interface{})

	for _, f := range l.fields {
		fields[f.Key] = f.Value
	}

	if traceID := TraceIDFromContext(l.ctx); traceID != "" {
		fields["trace_id"] = traceID
	}
	if userID := UserIDFromContext(l.ctx); userID != "" {
		fields["user_id"] = userID
	}
	if tenantID := TenantIDFromContext(l.ctx); tenantID != "" {
		fields["tenant_id"] = tenantID
	}
	if l.config.ServiceName != "" {
		fields["service_name"] = l.config.ServiceName
	}

	return fields
}

func toZapFields(fields map[string]interface{}) []interface{} {
	zapFields := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		zapFields = append(zapFields, k, v)
	}
	return zapFields
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
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

func getStackTrace() string {
	stack := make([]uintptr, 32)
	frames := runtime.Callers(2, stack)
	stackStr := ""
	for i := 0; i < frames; i++ {
		pc, file, line, ok := runtime.Caller(i + 2)
		if !ok {
			break
		}
		if stackStr != "" {
			stackStr += "\n"
		}
		fn := runtime.FuncForPC(pc)
		if fn != nil {
			stackStr += fmt.Sprintf("%s (%s:%d)", fn.Name(), filepath.Base(file), line)
		} else {
			stackStr += fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}
	return stackStr
}

// ==============================
// 审计日志文件写入
// ==============================

func (l *Logger) writeAuditEntry(entry AuditEntry) {
	logDir := l.config.AuditLogDir
	if logDir == "" {
		logDir = l.config.LogDir
	}
	if logDir == "" {
		logDir = "/tmp/cloudflow-audit"
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return
	}

	fileName := fmt.Sprintf("%s-audit.log", l.config.ServiceName)
	filePath := filepath.Join(logDir, fileName)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.WriteString(string(entryJSON) + "\n"); err != nil {
		return
	}
}

// ==============================
// FileWriter
// ==============================

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

func NewFileWriter(logDir, fileName string, maxSize, maxBackups, maxAge int) *FileWriter {
	if maxSize <= 0 {
		maxSize = 100
	}
	if maxBackups <= 0 {
		maxBackups = 10
	}
	if maxAge <= 0 {
		maxAge = 7
	}
	writer := &FileWriter{
		logDir:     logDir,
		fileName:   fileName,
		maxSize:    maxSize * 1024 * 1024,
		maxBackups: maxBackups,
		maxAge:     maxAge,
	}
	writer.rotate()
	return writer
}

func (w *FileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentFile == nil || w.currentSize+int64(len(p)) > int64(w.maxSize) {
		w.rotate()
	}

	n, err = w.currentFile.Write(p)
	if err == nil {
		w.currentSize += int64(n)
	}
	_ = w.currentFile.Sync()
	return
}

func (w *FileWriter) rotate() {
	if w.currentFile != nil {
		_ = w.currentFile.Close()
	}

	activeFile := filepath.Join(w.logDir, w.fileName+".log")
	if info, err := os.Stat(activeFile); err == nil && info.Size() > 0 {
		w.rotateIndex++
		archiveName := filepath.Join(w.logDir, fmt.Sprintf("%s-%d.log", w.fileName, w.rotateIndex))
		for {
			if _, err := os.Stat(archiveName); os.IsNotExist(err) {
				break
			}
			w.rotateIndex++
			archiveName = filepath.Join(w.logDir, fmt.Sprintf("%s-%d.log", w.fileName, w.rotateIndex))
		}
		if err := os.Rename(activeFile, archiveName); err != nil {
			timestamp := time.Now().Format("2006-01-02-15-04-05")
			archiveName = filepath.Join(w.logDir, fmt.Sprintf("%s-%s.log", w.fileName, timestamp))
			_ = os.Rename(activeFile, archiveName)
		}
	}

	file, err := os.OpenFile(activeFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		file = os.Stdout
	}

	w.currentFile = file
	if info, err := file.Stat(); err == nil {
		w.currentSize = info.Size()
	} else {
		w.currentSize = 0
	}
	w.cleanup()
}

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

	sort.Slice(logFiles, func(i, j int) bool {
		info1, _ := logFiles[i].Info()
		info2, _ := logFiles[j].Info()
		return info1.ModTime().Before(info2.ModTime())
	})

	activeFileName := w.fileName + ".log"
	if len(logFiles) > w.maxBackups {
		for i := w.maxBackups; i < len(logFiles); i++ {
			if logFiles[i].Name() == activeFileName {
				continue
			}
			_ = os.Remove(filepath.Join(w.logDir, logFiles[i].Name()))
		}
	}

	cutoff := time.Now().AddDate(0, 0, -w.maxAge)
	for _, file := range logFiles {
		if file.Name() == activeFileName {
			continue
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

func (l *Logger) Sync() {
	_ = l.zapLogger.Sync()
}

// ==============================
// 全局便捷方法
// ==============================

func SetGlobalLogger(l *Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = l
}

func Global() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalLogger == nil {
		globalLogger = New(Config{
			ServiceName: "cloudflow",
			Level:       "info",
			Format:      "json",
			Output:      "stdout",
		})
	}
	return globalLogger
}

func GDebug(msg string, keysAndValues ...interface{}) {
	Global().Debug(msg, keysAndValues...)
}

func GInfo(msg string, keysAndValues ...interface{}) {
	Global().Info(msg, keysAndValues...)
}

func GWarn(msg string, keysAndValues ...interface{}) {
	Global().Warn(msg, keysAndValues...)
}

func GError(msg string, keysAndValues ...interface{}) {
	Global().Error(msg, keysAndValues...)
}

func GFatal(msg string, keysAndValues ...interface{}) {
	Global().Fatal(msg, keysAndValues...)
}

func GErrorWithStack(err error, msg string, keysAndValues ...interface{}) {
	Global().ErrorWithStack(err, msg, keysAndValues...)
}

func GAudit(ctx context.Context, eventType, resource, action, result string, details map[string]interface{}) {
	Global().Audit(ctx, eventType, resource, action, result, details)
}
