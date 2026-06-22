//go:build linux

package sysconfig

import (
	"fmt"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
)

// ============================================================================
// 一、系统配置管理器
// ============================================================================

// Manager 系统配置管理器
type Manager struct {
	mu sync.RWMutex

	// 核心组件
	versionManager *VersionManager
	auditManager   *AuditManager
	changeHandler  *ConfigChangeHandler

	// 配置源
	sources   map[string]ConfigSource
	sourceMu  sync.RWMutex

	// 当前配置
	current   *ConfigSnapshot

	// 日志
	log       *logger.Logger
}

// NewManager 创建系统配置管理器
func NewManager(log *logger.Logger) *Manager {
	m := &Manager{
		versionManager: NewVersionManager(),
		auditManager:   NewAuditManager(),
		changeHandler:  NewConfigChangeHandler(),
		sources:       make(map[string]ConfigSource),
		current:       &ConfigSnapshot{Version: "v-init", Items: make(map[string]*ConfigItem)},
		log:           log,
	}
	
	if m.log == nil {
		m.log = logger.New(logger.Config{})
	}
	
	return m
}

// ============================================================================
// 二、配置源管理
// ============================================================================

// AddSource 添加配置源
func (m *Manager) AddSource(name string, source ConfigSource) error {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	
	if _, exists := m.sources[name]; exists {
		return fmt.Errorf("source already exists: %s", name)
	}
	
	m.sources[name] = source
	
	// 设置变更回调
	switch s := source.(type) {
	case *FileSource:
		s.SetOnChange(func(snapshot *ConfigSnapshot) {
			m.handleSourceChange(name, snapshot)
		})
	case *MemorySource:
		s.SetOnChange(func(snapshot *ConfigSnapshot) {
			m.handleSourceChange(name, snapshot)
		})
	}
	
	// 启动监视
	if err := source.Watch(); err != nil {
		m.log.Warnf("[sysconfig] 无法启动源监视: %s, error=%v", name, err)
	}
	
	m.log.Infof("[sysconfig] 配置源已添加: %s (type=%s)", name, source.SourceType())
	return nil
}

// RemoveSource 移除配置源
func (m *Manager) RemoveSource(name string) error {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	
	source, ok := m.sources[name]
	if !ok {
		return fmt.Errorf("source not found: %s", name)
	}
	
	source.Stop()
	delete(m.sources, name)
	m.log.Infof("[sysconfig] 配置源已移除: %s", name)
	return nil
}

// LoadFromSource 从指定源加载配置
func (m *Manager) LoadFromSource(name string, userID string) (*ConfigSnapshot, error) {
	m.sourceMu.RLock()
	source, ok := m.sources[name]
	m.sourceMu.RUnlock()
	
	if !ok {
		return nil, fmt.Errorf("source not found: %s", name)
	}
	
	snapshot, err := source.Load()
	if err != nil {
		m.auditManager.RecordVersion(AuditActionImport, userID, "", fmt.Sprintf("Failed to load from source %s", name), false)
		return nil, err
	}
	
	// 验证并保存
	if err := m.SaveSnapshot(snapshot, userID); err != nil {
		return nil, err
	}
	
	m.auditManager.RecordVersion(AuditActionImport, userID, snapshot.Version, fmt.Sprintf("Loaded from source %s", name), true)
	return snapshot, nil
}

// handleSourceChange 处理源配置变更
func (m *Manager) handleSourceChange(sourceName string, snapshot *ConfigSnapshot) {
	m.log.Infof("[sysconfig] 配置源变更: %s, version=%s", sourceName, snapshot.Version)
	
	// 标记来源
	snapshot.Source = sourceName
	
	// 保存快照
	if err := m.SaveSnapshot(snapshot, "system"); err != nil {
		m.log.Errorf("[sysconfig] 保存配置失败: %v", err)
	}
}

// ============================================================================
// 三、配置项操作
// ============================================================================

// GetItem 获取配置项
func (m *Manager) GetItem(key string) *ConfigItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil || m.current.Items == nil {
		return nil
	}
	return m.current.Items[key]
}

// SetItem 设置单个配置项
func (m *Manager) SetItem(key string, value interface{}, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	oldItem := m.current.Items[key]
	oldVal := interface{}(nil)
	if oldItem != nil {
		oldVal = oldItem.Value
	}
	
	newItem := &ConfigItem{
		Key:       key,
		Value:     value,
		UpdatedAt: time.Now(),
		UpdatedBy: userID,
	}
	if oldItem != nil {
		newItem.Type = oldItem.Type
		newItem.Description = oldItem.Description
		newItem.Category = oldItem.Category
		newItem.Default = oldItem.Default
		newItem.Editable = oldItem.Editable
		newItem.Sensitive = oldItem.Sensitive
		newItem.Validation = oldItem.Validation
		newItem.Source = oldItem.Source
		newItem.CreatedAt = oldItem.CreatedAt
	}
	
	// 验证
	if newItem.Validation != nil {
		if err := newItem.Validation.Validate(value); err != nil {
			m.auditManager.RecordChange(AuditActionUpdate, userID, key, oldVal, value, false, err.Error())
			return err
		}
	}
	
	m.current.Items[key] = newItem
	m.current.UpdatedAt = time.Now()
	
	// 通知变更处理器
	m.changeHandler.Handle(key, oldVal, value)
	
	// 记录审计
	m.auditManager.RecordChange(AuditActionUpdate, userID, key, oldVal, value, true, "Item updated")
	
	m.log.Infof("[sysconfig] 配置项已更新: %s by %s", key, userID)
	return nil
}

// DeleteItem 删除配置项
func (m *Manager) DeleteItem(key, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	oldItem, ok := m.current.Items[key]
	if !ok {
		return fmt.Errorf("config item not found: %s", key)
	}
	
	delete(m.current.Items, key)
	m.current.UpdatedAt = time.Now()
	
	m.auditManager.RecordChange(AuditActionDelete, userID, key, oldItem.Value, nil, true, "Item deleted")
	m.log.Infof("[sysconfig] 配置项已删除: %s by %s", key, userID)
	return nil
}

// ListItems 列出所有配置项
func (m *Manager) ListItems(category string) []*ConfigItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var result []*ConfigItem
	for _, item := range m.current.Items {
		if category == "" || item.Category == category {
			result = append(result, item)
		}
	}
	return result
}

// GetAllItems 获取所有配置项
func (m *Manager) GetAllItems() map[string]*ConfigItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*ConfigItem, len(m.current.Items))
	for k, v := range m.current.Items {
		result[k] = v
	}
	return result
}

// ResetToDefault 恢复默认值
func (m *Manager) ResetToDefault(key, userID string) error {
	item := m.GetItem(key)
	if item == nil {
		return fmt.Errorf("config item not found: %s", key)
	}
	
	if item.Default == nil {
		return fmt.Errorf("no default value for: %s", key)
	}
	
	return m.SetItem(key, item.Default, userID)
}

// ============================================================================
// 四、快照管理
// ============================================================================

// SaveSnapshot 保存配置快照
func (m *Manager) SaveSnapshot(snapshot *ConfigSnapshot, userID string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot cannot be nil")
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 计算差异
	var diff *ConfigDiff
	if m.current != nil && m.current.Items != nil {
		diff = m.current.Compare(snapshot)
	}
	
	// 保存版本
	if err := m.versionManager.Save(snapshot); err != nil {
		return err
	}
	
	m.current = snapshot.DeepCopy()
	
	// 审计
	if diff != nil && !diff.IsEmpty() {
		m.auditManager.AuditDiff(diff, userID, snapshot.Source)
	}
	
	m.auditManager.RecordVersion(AuditActionCreate, userID, snapshot.Version, snapshot.Description, true)
	
	m.log.Infof("[sysconfig] 配置快照已保存: version=%s, items=%d", snapshot.Version, len(snapshot.Items))
	return nil
}

// GetCurrentSnapshot 获取当前快照
func (m *Manager) GetCurrentSnapshot() *ConfigSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == nil {
		return nil
	}
	return m.current.DeepCopy()
}

// Rollback 回滚到指定版本
func (m *Manager) Rollback(version, userID string) (*ConfigSnapshot, error) {
	rolledBack, err := m.versionManager.Rollback(version, userID)
	if err != nil {
		m.auditManager.RecordVersion(AuditActionRollback, userID, version, err.Error(), false)
		return nil, err
	}
	
	m.mu.Lock()
	m.current = rolledBack.DeepCopy()
	m.mu.Unlock()
	
	m.auditManager.RecordVersion(AuditActionRollback, userID, rolledBack.Version, fmt.Sprintf("Rolled back to %s", version), true)
	m.log.Infof("[sysconfig] 配置已回滚: %s -> %s", version, rolledBack.Version)
	return rolledBack, nil
}

// GetVersionHistory 获取版本历史
func (m *Manager) GetVersionHistory() []VersionHistory {
	return m.versionManager.GetHistory()
}

// GetVersion 获取指定版本
func (m *Manager) GetVersion(version string) *ConfigSnapshot {
	return m.versionManager.GetVersion(version)
}

// DiffVersions 比较两个版本
func (m *Manager) DiffVersions(from, to string) (*ConfigDiff, error) {
	return m.versionManager.Diff(from, to)
}

// ExportCurrent 导出当前配置
func (m *Manager) ExportCurrent() (string, error) {
	snapshot := m.GetCurrentSnapshot()
	if snapshot == nil {
		return "", fmt.Errorf("no current config")
	}
	return snapshot.ToJSON()
}

// ImportSnapshot 导入配置快照
func (m *Manager) ImportSnapshot(data []byte, userID string) (*ConfigSnapshot, error) {
	snapshot, err := m.versionManager.ImportSnapshot(data, userID)
	if err != nil {
		m.auditManager.RecordVersion(AuditActionImport, userID, "", err.Error(), false)
		return nil, err
	}
	
	m.mu.Lock()
	m.current = snapshot.DeepCopy()
	m.mu.Unlock()
	
	m.auditManager.RecordVersion(AuditActionImport, userID, snapshot.Version, "Snapshot imported", true)
	m.log.Infof("[sysconfig] 配置已导入: version=%s", snapshot.Version)
	return snapshot, nil
}

// ============================================================================
// 五、审计相关
// ============================================================================

// GetAuditLogs 获取审计日志
func (m *Manager) GetAuditLogs() []*AuditLog {
	return m.auditManager.GetLogs()
}

// QueryAuditLogs 查询审计日志
func (m *Manager) QueryAuditLogs(opts AuditQueryOptions) []*AuditLog {
	return m.auditManager.Query(opts)
}

// GetAuditStats 获取审计统计
func (m *Manager) GetAuditStats() map[string]interface{} {
	return m.auditManager.GetStats()
}

// ============================================================================
// 六、变更监听
// ============================================================================

// RegisterChangeHandler 注册配置变更处理器
func (m *Manager) RegisterChangeHandler(key string, handler func(oldVal, newVal interface{})) {
	m.changeHandler.Register(key, handler)
}

// ============================================================================
// 七、统计和状态
// ============================================================================

// GetStats 获取统计信息
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	m.sourceMu.RLock()
	sourceCount := len(m.sources)
	m.sourceMu.RUnlock()
	
	currentVersion := ""
	if m.current != nil {
		currentVersion = m.current.Version
	}
	
	return map[string]interface{}{
		"item_count":      len(m.current.Items),
		"version_stats":   m.versionManager.GetStats(),
		"audit_stats":     m.auditManager.GetStats(),
		"source_count":    sourceCount,
		"current_version": currentVersion,
	}
}
func (m *Manager) GetSummary() *ConfigSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	summary := &ConfigSummary{
		Version:       m.current.Version,
		ItemCount:     len(m.current.Items),
		CategoryCount: make(map[string]int),
		UpdatedAt:     m.current.UpdatedAt,
		UpdatedBy:     m.current.CreatedBy,
	}
	
	for _, item := range m.current.Items {
		summary.CategoryCount[item.Category]++
		if !item.IsDefault() {
			summary.CustomCount++
		}
	}
	
	return summary
}

// ConfigSummary 配置摘要
type ConfigSummary struct {
	Version       string            `json:"version"`
	ItemCount     int               `json:"item_count"`
	CustomCount   int               `json:"custom_count"`
	CategoryCount map[string]int    `json:"category_count"`
	UpdatedAt     time.Time         `json:"updated_at"`
	UpdatedBy     string            `json:"updated_by"`
}

// ============================================================================
// 八、清理和关闭
// ============================================================================

// Stop 停止所有配置源
func (m *Manager) Stop() {
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	
	for name, source := range m.sources {
		if err := source.Stop(); err != nil {
			m.log.Warnf("[sysconfig] 停止源失败: %s, error=%v", name, err)
		}
	}
	m.sources = make(map[string]ConfigSource)
	m.log.Info("[sysconfig] 所有配置源已停止")
}

// ClearAuditLogs 清空审计日志
func (m *Manager) ClearAuditLogs() {
	m.auditManager.Clear()
	m.log.Info("[sysconfig] 审计日志已清空")
}