//go:build linux

package sysconfig

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、审计日志条目
// ============================================================================

// AuditAction 审计操作类型
type AuditAction string

const (
	AuditActionCreate  AuditAction = "create"
	AuditActionUpdate  AuditAction = "update"
	AuditActionDelete  AuditAction = "delete"
	AuditActionRollback AuditAction = "rollback"
	AuditActionImport  AuditAction = "import"
	AuditActionExport  AuditAction = "export"
	AuditActionReload  AuditAction = "reload"
	AuditActionView    AuditAction = "view"
)

// AuditLog 审计日志条目
type AuditLog struct {
	ID        string      `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Action    AuditAction `json:"action"`
	UserID    string      `json:"user_id"`
	UserName  string      `json:"user_name,omitempty"`
	IP        string      `json:"ip,omitempty"`
	// 变更详情
	ConfigKey    string      `json:"config_key,omitempty"`    // 单个配置项变更
	OldValue     interface{} `json:"old_value,omitempty"`
	NewValue     interface{} `json:"new_value,omitempty"`
	Version      string      `json:"version,omitempty"`       // 关联版本
	Description  string      `json:"description,omitempty"`
	// 结果
	Success   bool   `json:"success"`
	ErrorMsg  string `json:"error_msg,omitempty"`
	// 来源
	Source    string `json:"source"`  // api/ui/file
}

// Summary 返回审计摘要
func (al *AuditLog) Summary() string {
	return fmt.Sprintf("[%s] %s by %s: %s %s", al.Timestamp.Format("2006-01-02 15:04:05"), al.Action, al.UserID, al.ConfigKey, al.Description)
}

// ============================================================================
// 二、审计管理器
// ============================================================================

// AuditManager 审计管理器
type AuditManager struct {
	mu        sync.RWMutex
	logs      []*AuditLog
	maxLogs   int
	listeners []AuditListener
}

// AuditListener 审计监听器
type AuditListener func(log *AuditLog)

// NewAuditManager 创建审计管理器
func NewAuditManager() *AuditManager {
	return &AuditManager{
		logs:    make([]*AuditLog, 0),
		maxLogs: 1000, // 默认保留1000条
	}
}

// SetMaxLogs 设置最大日志数
func (am *AuditManager) SetMaxLogs(n int) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.maxLogs = n
}

// Record 记录审计日志
func (am *AuditManager) Record(log *AuditLog) {
	if log == nil {
		return
	}
	if log.ID == "" {
		log.ID = fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	if log.Timestamp.IsZero() {
		log.Timestamp = time.Now()
	}

	am.mu.Lock()
	am.logs = append(am.logs, log)
	am.cleanup()
	listeners := make([]AuditListener, len(am.listeners))
	copy(listeners, am.listeners)
	am.mu.Unlock()

	// 通知监听器
	for _, listener := range listeners {
		go func(l AuditListener) {
			defer func() { recover() }()
			l(log)
		}(listener)
	}
}

// RecordChange 快捷记录变更
func (am *AuditManager) RecordChange(action AuditAction, userID, key string, oldVal, newVal interface{}, success bool, description string) {
	am.Record(&AuditLog{
		Action:      action,
		UserID:      userID,
		ConfigKey:   key,
		OldValue:    oldVal,
		NewValue:    newVal,
		Success:     success,
		Description: description,
	})
}

// RecordVersion 记录版本相关操作
func (am *AuditManager) RecordVersion(action AuditAction, userID, version, description string, success bool) {
	am.Record(&AuditLog{
		Action:      action,
		UserID:      userID,
		Version:     version,
		Success:     success,
		Description: description,
	})
}

// GetLogs 获取所有日志（按时间倒序）
func (am *AuditManager) GetLogs() []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	result := make([]*AuditLog, len(am.logs))
	for i, log := range am.logs {
		result[len(result)-1-i] = log
	}
	return result
}

// GetLogsByUser 按用户获取日志
func (am *AuditManager) GetLogsByUser(userID string) []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*AuditLog
	for i := len(am.logs) - 1; i >= 0; i-- {
		if am.logs[i].UserID == userID {
			result = append(result, am.logs[i])
		}
	}
	return result
}

// GetLogsByAction 按操作类型获取日志
func (am *AuditManager) GetLogsByAction(action AuditAction) []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*AuditLog
	for i := len(am.logs) - 1; i >= 0; i-- {
		if am.logs[i].Action == action {
			result = append(result, am.logs[i])
		}
	}
	return result
}

// GetLogsByKey 按配置键获取日志
func (am *AuditManager) GetLogsByKey(key string) []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*AuditLog
	for i := len(am.logs) - 1; i >= 0; i-- {
		if am.logs[i].ConfigKey == key {
			result = append(result, am.logs[i])
		}
	}
	return result
}

// GetLogsByTimeRange 按时间范围获取日志
func (am *AuditManager) GetLogsByTimeRange(from, to time.Time) []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*AuditLog
	for i := len(am.logs) - 1; i >= 0; i-- {
		t := am.logs[i].Timestamp
		if (t.Equal(from) || t.After(from)) && (t.Equal(to) || t.Before(to)) {
			result = append(result, am.logs[i])
		}
	}
	return result
}

// Query 综合查询日志
func (am *AuditManager) Query(opts AuditQueryOptions) []*AuditLog {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var result []*AuditLog
	for i := len(am.logs) - 1; i >= 0; i-- {
		log := am.logs[i]
		if opts.UserID != "" && log.UserID != opts.UserID {
			continue
		}
		if opts.Action != "" && log.Action != opts.Action {
			continue
		}
		if opts.Key != "" && log.ConfigKey != opts.Key {
			continue
		}
		if opts.Version != "" && log.Version != opts.Version {
			continue
		}
		if opts.SuccessOnly && !log.Success {
			continue
		}
		if !opts.From.IsZero() && log.Timestamp.Before(opts.From) {
			continue
		}
		if !opts.To.IsZero() && log.Timestamp.After(opts.To) {
			continue
		}
		result = append(result, log)
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
	}
	return result
}

// AuditQueryOptions 审计查询选项
type AuditQueryOptions struct {
	UserID      string
	Action      AuditAction
	Key         string
	Version     string
	SuccessOnly bool
	From        time.Time
	To          time.Time
	Limit       int
}

// GetStats 获取审计统计
func (am *AuditManager) GetStats() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	actionCounts := make(map[string]int)
	userCounts := make(map[string]int)
	for _, log := range am.logs {
		actionCounts[string(log.Action)]++
		userCounts[log.UserID]++
	}

	return map[string]interface{}{
		"total_logs":    len(am.logs),
		"max_logs":      am.maxLogs,
		"action_counts": actionCounts,
		"user_counts":   userCounts,
	}
}

// AddListener 添加审计监听器
func (am *AuditManager) AddListener(listener AuditListener) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.listeners = append(am.listeners, listener)
}

// Clear 清空所有日志
func (am *AuditManager) Clear() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.logs = make([]*AuditLog, 0)
}

// cleanup 清理旧日志
func (am *AuditManager) cleanup() {
	if am.maxLogs <= 0 {
		return
	}
	if len(am.logs) <= am.maxLogs {
		return
	}
	am.logs = am.logs[len(am.logs)-am.maxLogs:]
}

// ============================================================================
// 三、配置变更审计（自动生成差异审计）
// ============================================================================

// AuditDiff 对配置差异生成审计日志
func (am *AuditManager) AuditDiff(diff *ConfigDiff, userID, source string) {
	if diff == nil || diff.IsEmpty() {
		return
	}

	for _, key := range diff.Changed {
		am.Record(&AuditLog{
			Action:      AuditActionUpdate,
			UserID:      userID,
			ConfigKey:   key,
			Version:     diff.ToVersion,
			Description: fmt.Sprintf("Changed in version %s", diff.ToVersion),
			Success:     true,
			Source:      source,
		})
	}

	for _, key := range diff.Added {
		am.Record(&AuditLog{
			Action:      AuditActionCreate,
			UserID:      userID,
			ConfigKey:   key,
			Version:     diff.ToVersion,
			Description: fmt.Sprintf("Added in version %s", diff.ToVersion),
			Success:     true,
			Source:      source,
		})
	}

	for _, key := range diff.Removed {
		am.Record(&AuditLog{
			Action:      AuditActionDelete,
			UserID:      userID,
			ConfigKey:   key,
			Version:     diff.ToVersion,
			Description: fmt.Sprintf("Removed in version %s", diff.ToVersion),
			Success:     true,
			Source:      source,
		})
	}
}