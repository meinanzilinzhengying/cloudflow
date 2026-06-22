//go:build linux

package sysconfig

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、配置版本管理器
// ============================================================================

// VersionManager 配置版本管理器
type VersionManager struct {
	mu        sync.RWMutex
	snapshots map[string]*ConfigSnapshot // version -> snapshot
	current   *ConfigSnapshot
	maxHistory int
}

// NewVersionManager 创建版本管理器
func NewVersionManager() *VersionManager {
	return &VersionManager{
		snapshots:  make(map[string]*ConfigSnapshot),
		maxHistory: 50, // 默认保留50个版本
	}
}

// SetMaxHistory 设置最大历史版本数
func (vm *VersionManager) SetMaxHistory(n int) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.maxHistory = n
}

// Save 保存配置快照
func (vm *VersionManager) Save(snapshot *ConfigSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot cannot be nil")
	}
	if snapshot.Version == "" || vm.snapshots[snapshot.Version] != nil {
		snapshot.Version = vm.generateVersion()
	}
	
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// 保存到历史
	vm.snapshots[snapshot.Version] = snapshot.DeepCopy()
	vm.current = snapshot.DeepCopy()
	
	// 清理旧版本
	vm.cleanupOldVersions()
	
	return nil
}

// GetCurrent 获取当前版本
func (vm *VersionManager) GetCurrent() *ConfigSnapshot {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if vm.current == nil {
		return nil
	}
	return vm.current.DeepCopy()
}

// GetVersion 获取指定版本
func (vm *VersionManager) GetVersion(version string) *ConfigSnapshot {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if s, ok := vm.snapshots[version]; ok {
		return s.DeepCopy()
	}
	return nil
}

// ListVersions 列出所有版本（按时间倒序）
func (vm *VersionManager) ListVersions() []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	type versionInfo struct {
		version string
		at      time.Time
	}
	
	var infos []versionInfo
	for v, s := range vm.snapshots {
		infos = append(infos, versionInfo{version: v, at: s.CreatedAt})
	}
	
	// 按时间倒序排序
	for i := 0; i < len(infos)-1; i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[j].at.After(infos[i].at) {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	
	result := make([]string, len(infos))
	for i, info := range infos {
		result[i] = info.version
	}
	return result
}

// Rollback 回滚到指定版本
func (vm *VersionManager) Rollback(version string, operator string) (*ConfigSnapshot, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	target, ok := vm.snapshots[version]
	if !ok {
		return nil, fmt.Errorf("version not found: %s", version)
	}
	
	rolledBack := target.DeepCopy()
	rolledBack.Version = vm.generateVersion()
	rolledBack.Description = fmt.Sprintf("Rollback to version %s", version)
	rolledBack.CreatedAt = time.Now()
	rolledBack.CreatedBy = operator
	rolledBack.Source = "rollback"
	
	vm.snapshots[rolledBack.Version] = rolledBack.DeepCopy()
	vm.current = rolledBack.DeepCopy()
	vm.cleanupOldVersions()
	
	return rolledBack, nil
}

// Diff 比较两个版本
func (vm *VersionManager) Diff(fromVersion, toVersion string) (*ConfigDiff, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	from := vm.snapshots[fromVersion]
	to := vm.snapshots[toVersion]
	
	if from == nil {
		return nil, fmt.Errorf("version not found: %s", fromVersion)
	}
	if to == nil {
		return nil, fmt.Errorf("version not found: %s", toVersion)
	}
	
	return from.Compare(to), nil
}

// DiffWithCurrent 与当前版本比较
func (vm *VersionManager) DiffWithCurrent(version string) (*ConfigDiff, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	if vm.current == nil {
		return nil, fmt.Errorf("no current version")
	}
	
	target := vm.snapshots[version]
	if target == nil {
		return nil, fmt.Errorf("version not found: %s", version)
	}
	
	return target.Compare(vm.current), nil
}

// GetStats 获取统计信息
func (vm *VersionManager) GetStats() map[string]interface{} {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	return map[string]interface{}{
		"total_versions": len(vm.snapshots),
		"max_history":    vm.maxHistory,
		"current_version": func() string {
			if vm.current != nil {
				return vm.current.Version
			}
			return ""
		}(),
	}
}

// generateVersion 生成版本号
func (vm *VersionManager) generateVersion() string {
	return fmt.Sprintf("v-%d", time.Now().UnixNano())
}

// cleanupOldVersions 清理旧版本
func (vm *VersionManager) cleanupOldVersions() {
	if vm.maxHistory <= 0 {
		return
	}
	
	if len(vm.snapshots) <= vm.maxHistory {
		return
	}
	
	// 收集版本和时间
	type vInfo struct {
		version string
		at      time.Time
	}
	var infos []vInfo
	for v, s := range vm.snapshots {
		infos = append(infos, vInfo{version: v, at: s.CreatedAt})
	}
	
	// 按时间排序（最旧的在前）
	for i := 0; i < len(infos)-1; i++ {
		for j := i + 1; j < len(infos); j++ {
			if infos[j].at.Before(infos[i].at) {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}
	
	// 删除最旧的版本
	deleteCount := len(infos) - vm.maxHistory
	for i := 0; i < deleteCount; i++ {
		delete(vm.snapshots, infos[i].version)
	}
}

// ============================================================================
// 二、配置版本历史
// ============================================================================

// VersionHistory 版本历史条目
type VersionHistory struct {
	Version     string    `json:"version"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	Source      string    `json:"source"`
	ItemCount   int       `json:"item_count"`
	Tags        []string  `json:"tags,omitempty"`
	IsCurrent   bool      `json:"is_current"`
}

// GetHistory 获取版本历史列表
func (vm *VersionManager) GetHistory() []VersionHistory {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var result []VersionHistory
	for v, s := range vm.snapshots {
		vh := VersionHistory{
			Version:     v,
			Description: s.Description,
			CreatedAt:   s.CreatedAt,
			CreatedBy:   s.CreatedBy,
			Source:      s.Source,
			ItemCount:   len(s.Items),
			Tags:        append([]string{}, s.Tags...),
			IsCurrent:   vm.current != nil && vm.current.Version == v,
		}
		result = append(result, vh)
	}
	
	// 按时间倒序
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].CreatedAt.After(result[i].CreatedAt) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	
	return result
}

// ============================================================================
// 三、配置标签管理
// ============================================================================

// TagSnapshot 给快照打标签
func (vm *VersionManager) TagSnapshot(version, tag string) error {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	snapshot, ok := vm.snapshots[version]
	if !ok {
		return fmt.Errorf("version not found: %s", version)
	}
	
	for _, t := range snapshot.Tags {
		if t == tag {
			return nil // 已存在
		}
	}
	
	snapshot.Tags = append(snapshot.Tags, tag)
	return nil
}

// FindByTag 按标签查找版本
func (vm *VersionManager) FindByTag(tag string) []string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	var result []string
	for v, s := range vm.snapshots {
		for _, t := range s.Tags {
			if t == tag {
				result = append(result, v)
				break
			}
		}
	}
	return result
}

// ============================================================================
// 四、配置导出导入
// ============================================================================

// ExportVersion 导出指定版本为 JSON
func (vm *VersionManager) ExportVersion(version string) ([]byte, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	snapshot, ok := vm.snapshots[version]
	if !ok {
		return nil, fmt.Errorf("version not found: %s", version)
	}
	
	data, err := snapshot.ToJSON()
	return []byte(data), err
}

// ImportSnapshot 导入快照
func (vm *VersionManager) ImportSnapshot(data []byte, operator string) (*ConfigSnapshot, error) {
	snapshot, err := FromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	snapshot.Version = vm.generateVersion()
	snapshot.CreatedAt = time.Now()
	snapshot.CreatedBy = operator
	snapshot.Source = "import"
	
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	vm.snapshots[snapshot.Version] = snapshot.DeepCopy()
	vm.current = snapshot.DeepCopy()
	vm.cleanupOldVersions()
	
	return snapshot, nil
}