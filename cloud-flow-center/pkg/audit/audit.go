// Package audit 提供审计日志功能
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cloud-flow-center/pkg/logger"
)

// ActionType 操作类型
type ActionType string

const (
	// ActionLogin 登录操作
	ActionLogin ActionType = "login"
	// ActionLogout 注销操作
	ActionLogout ActionType = "logout"
	// ActionCreate 创建操作
	ActionCreate ActionType = "create"
	// ActionUpdate 更新操作
	ActionUpdate ActionType = "update"
	// ActionDelete 删除操作
	ActionDelete ActionType = "delete"
	// ActionQuery 查询操作
	ActionQuery ActionType = "query"
	// ActionConfig 配置变更操作
	ActionConfig ActionType = "config"
	// ActionSystem 系统操作
	ActionSystem ActionType = "system"
)

// AuditEntry 审计日志条目
type AuditEntry struct {
	Timestamp   time.Time       `json:"timestamp"`
	Action      ActionType      `json:"action"`
	User        string          `json:"user"`
	Resource    string          `json:"resource"`
	Operation   string          `json:"operation"`
	Status      string          `json:"status"`
	Message     string          `json:"message"`
	Details     json.RawMessage `json:"details,omitempty"`
	IPAddress   string          `json:"ip_address,omitempty"`
	UserAgent   string          `json:"user_agent,omitempty"`
}

// maxFileSize 审计日志文件最大大小（100MB）
const maxFileSize int64 = 100 * 1024 * 1024

// Logger 审计日志记录器
type Logger struct {
	logDir   string
	logFile  *os.File
	logger   *logger.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
}

// NewLogger 创建审计日志记录器
func NewLogger(logDir string, log *logger.Logger) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建审计日志目录失败: %w", err)
	}

	// 创建日志文件
	logFile := filepath.Join(logDir, fmt.Sprintf("audit-%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开审计日志文件失败: %w", err)
	}

	return &Logger{
		logDir:  logDir,
		logFile: file,
		logger:  log,
		stopCh:  make(chan struct{}),
	}, nil
}

// Log 记录审计日志
func (l *Logger) Log(action ActionType, user, resource, operation, status, message string, details interface{}, ipAddress, userAgent string) error {
	// 构建审计日志条目
	auditEntry := AuditEntry{
		Timestamp: time.Now(),
		Action:    action,
		User:      user,
		Resource:  resource,
		Operation: operation,
		Status:    status,
		Message:   message,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}

	// 序列化详情
	if details != nil {
		detailsJSON, err := json.Marshal(details)
		if err != nil {
			l.logger.Warnf("序列化审计日志详情失败: %v", err)
		} else {
			auditEntry.Details = detailsJSON
		}
	}

	// 序列化审计日志条目
	auditJSON, err := json.Marshal(auditEntry)
	if err != nil {
		l.logger.Warnf("序列化审计日志条目失败: %v", err)
		return err
	}

	// 获取锁以保护并发写入
	l.mu.Lock()
	defer l.mu.Unlock()

	// 检查是否已收到停止信号
	select {
	case <-l.stopCh:
		l.logger.Warnf("审计日志记录器已停止，拒绝写入")
		return fmt.Errorf("审计日志记录器已停止")
	default:
	}

	// 检查文件大小，超过限制时进行轮转
	if err := l.rotateLogFile(); err != nil {
		l.logger.Warnf("审计日志文件轮转失败: %v", err)
		// 轮转失败不阻止日志写入，继续使用当前文件
	}

	// 写入日志文件
	if _, err := l.logFile.Write(append(auditJSON, '\n')); err != nil {
		l.logger.Warnf("写入审计日志文件失败: %v", err)
		return err
	}

	// 刷新文件缓冲区
	if err := l.logFile.Sync(); err != nil {
		l.logger.Warnf("刷新审计日志文件失败: %v", err)
		return err
	}

	return nil
}

// rotateLogFile 检查当前日志文件大小，超过 maxFileSize 时进行轮转
// 将当前文件重命名为 audit-<timestamp>.log 并创建新文件
// 注意：调用者必须持有 l.mu 锁
func (l *Logger) rotateLogFile() error {
	info, err := l.logFile.Stat()
	if err != nil {
		return fmt.Errorf("获取日志文件信息失败: %w", err)
	}

	if info.Size() < maxFileSize {
		return nil
	}

	// 关闭当前文件
	if err := l.logFile.Close(); err != nil {
		return fmt.Errorf("关闭当前日志文件失败: %w", err)
	}

	// 将当前文件重命名为带时间戳的归档文件（使用纳秒避免同秒冲突）
	// 从原文件名中提取日期前缀，避免跨天轮转时日期信息丢失
	oldName := l.logFile.Name()
	baseName := filepath.Base(oldName)
	dateStr := strings.TrimPrefix(strings.TrimSuffix(baseName, ".log"), "audit-")
	if dateStr == "" || dateStr == baseName {
		dateStr = time.Now().Format("2006-01-02")
	}
	rotatedName := filepath.Join(l.logDir, fmt.Sprintf("audit-%sT%s.log", dateStr, time.Now().Format("15-04-05.999999999")))
	if err := os.Rename(oldName, rotatedName); err != nil {
		return fmt.Errorf("重命名日志文件失败: %w", err)
	}

	l.logger.Infof("审计日志文件已轮转: %s -> %s", filepath.Base(oldName), filepath.Base(rotatedName))

	// 创建新的日志文件
	newLogFile := filepath.Join(l.logDir, fmt.Sprintf("audit-%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(newLogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("创建新日志文件失败: %w", err)
	}

	l.logFile = file
	return nil
}

// Stop 停止审计日志记录器
func (l *Logger) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
		l.mu.Lock()
		defer l.mu.Unlock()
		l.logFile.Close()
		l.logger.Info("审计日志记录器已停止")
	})
}
