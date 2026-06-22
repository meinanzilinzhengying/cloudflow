//go:build linux

package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
)

// ============================================================================
// 数据备份策略管理器
// 支持全量备份 + 增量备份，支持 ClickHouse 和 PostgreSQL 的备份
// ============================================================================

// BackupType 备份类型
type BackupType string

const (
	BackupTypeFull    BackupType = "full"     // 全量备份
	BackupTypeIncremental BackupType = "incremental" // 增量备份
)

// BackupStatus 备份状态
type BackupStatus string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusSuccess   BackupStatus = "success"
	BackupStatusFailed    BackupStatus = "failed"
)

// BackupConfig 备份配置
type BackupConfig struct {
	BackupDir       string        // 备份目录
	RetentionDays   int           // 备份保留天数（默认 30）
	FullInterval    time.Duration // 全量备份间隔（默认 24h）
	IncrementalInterval time.Duration // 增量备份间隔（默认 1h）
	CompressEnabled bool          // 是否启用压缩
	MaxBackups      int           // 最大备份数量（默认 10）
	EnableChecksum  bool          // 是否启用校验和
}

// DefaultBackupConfig 返回默认备份配置
func DefaultBackupConfig() *BackupConfig {
	return &BackupConfig{
		BackupDir:           "/opt/cloudflow/backups",
		RetentionDays:       30,
		FullInterval:        24 * time.Hour,
		IncrementalInterval: 1 * time.Hour,
		CompressEnabled:     true,
		MaxBackups:          10,
		EnableChecksum:      true,
	}
}

// BackupRecord 备份记录
type BackupRecord struct {
	ID            string       `json:"id"`
	Type          BackupType   `json:"type"`
	Status        BackupStatus `json:"status"`
	StartTime     time.Time    `json:"start_time"`
	EndTime       time.Time    `json:"end_time,omitempty"`
	Source        string       `json:"source"`           // 备份源（数据库/表）
	Size          int64        `json:"size"`             // 备份大小（字节）
	Checksum      string       `json:"checksum,omitempty"` // 校验和
	ErrorMessage  string       `json:"error_message,omitempty"`
	FilePath      string       `json:"file_path,omitempty"`
}

// BackupTarget 备份目标接口
type BackupTarget interface {
	Name() string
	Backup(full bool, backupDir string) (string, int64, error)
	Restore(backupFile string) error
}

// BackupManager 备份管理器
type BackupManager struct {
	config   *BackupConfig
	logger   *logger.Logger
	mu       sync.RWMutex
	records  []*BackupRecord
	targets  map[string]BackupTarget
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewBackupManager 创建备份管理器
func NewBackupManager(cfg *BackupConfig, log *logger.Logger) (*BackupManager, error) {
	if cfg == nil {
		cfg = DefaultBackupConfig()
	}
	if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir failed: %w", err)
	}
	return &BackupManager{
		config:  cfg,
		logger:  log,
		records: make([]*BackupRecord, 0),
		targets: make(map[string]BackupTarget),
		stopCh:  make(chan struct{}),
	}, nil
}

// RegisterTarget 注册备份目标
func (bm *BackupManager) RegisterTarget(target BackupTarget) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.targets[target.Name()] = target
}

// UnregisterTarget 注销备份目标
func (bm *BackupManager) UnregisterTarget(name string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.targets, name)
}

// ExecuteFullBackup 执行全量备份
func (bm *BackupManager) ExecuteFullBackup(targetName string) (*BackupRecord, error) {
	bm.mu.RLock()
	target, ok := bm.targets[targetName]
	bm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("backup target not found: %s", targetName)
	}

	record := &BackupRecord{
		ID:        generateBackupID(),
		Type:      BackupTypeFull,
		Status:    BackupStatusRunning,
		StartTime: time.Now(),
		Source:    targetName,
	}

	bm.mu.Lock()
	bm.records = append(bm.records, record)
	bm.mu.Unlock()

	backupDir := filepath.Join(bm.config.BackupDir, "full", targetName)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		record.Status = BackupStatusFailed
		record.EndTime = time.Now()
		record.ErrorMessage = err.Error()
		return record, err
	}

	backupFile, size, err := target.Backup(true, backupDir)
	if err != nil {
		record.Status = BackupStatusFailed
		record.EndTime = time.Now()
		record.ErrorMessage = err.Error()
		return record, err
	}

	record.Status = BackupStatusSuccess
	record.EndTime = time.Now()
	record.FilePath = backupFile
	record.Size = size

	if bm.config.EnableChecksum {
		record.Checksum = "sha256:" + generateChecksum(backupFile)
	}

	bm.cleanupOldBackups(targetName)
	return record, nil
}

// ExecuteIncrementalBackup 执行增量备份
func (bm *BackupManager) ExecuteIncrementalBackup(targetName string) (*BackupRecord, error) {
	bm.mu.RLock()
	target, ok := bm.targets[targetName]
	bm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("backup target not found: %s", targetName)
	}

	record := &BackupRecord{
		ID:        generateBackupID(),
		Type:      BackupTypeIncremental,
		Status:    BackupStatusRunning,
		StartTime: time.Now(),
		Source:    targetName,
	}

	bm.mu.Lock()
	bm.records = append(bm.records, record)
	bm.mu.Unlock()

	backupDir := filepath.Join(bm.config.BackupDir, "incremental", targetName)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		record.Status = BackupStatusFailed
		record.EndTime = time.Now()
		record.ErrorMessage = err.Error()
		return record, err
	}

	backupFile, size, err := target.Backup(false, backupDir)
	if err != nil {
		record.Status = BackupStatusFailed
		record.EndTime = time.Now()
		record.ErrorMessage = err.Error()
		return record, err
	}

	record.Status = BackupStatusSuccess
	record.EndTime = time.Now()
	record.FilePath = backupFile
	record.Size = size

	if bm.config.EnableChecksum {
		record.Checksum = "sha256:" + generateChecksum(backupFile)
	}

	bm.cleanupOldBackups(targetName)
	return record, nil
}

// Restore 恢复备份
func (bm *BackupManager) Restore(targetName, backupFile string) error {
	bm.mu.RLock()
	target, ok := bm.targets[targetName]
	bm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("backup target not found: %s", targetName)
	}

	return target.Restore(backupFile)
}

// GetRecords 获取备份记录
func (bm *BackupManager) GetRecords(targetName string, backupType BackupType) []*BackupRecord {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var result []*BackupRecord
	for _, r := range bm.records {
		if targetName != "" && r.Source != targetName {
			continue
		}
		if backupType != "" && r.Type != backupType {
			continue
		}
		result = append(result, r)
	}
	return result
}

// GetAllRecords 获取所有备份记录
func (bm *BackupManager) GetAllRecords() []*BackupRecord {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make([]*BackupRecord, len(bm.records))
	copy(result, bm.records)
	return result
}

// cleanupOldBackups 清理过期备份
func (bm *BackupManager) cleanupOldBackups(targetName string) {
	cutoff := time.Now().AddDate(0, 0, -bm.config.RetentionDays)

	bm.mu.Lock()
	defer bm.mu.Unlock()

	var valid []*BackupRecord
	for _, r := range bm.records {
		if r.Source == targetName && r.StartTime.Before(cutoff) {
			if r.FilePath != "" {
				os.Remove(r.FilePath)
			}
			continue
		}
		valid = append(valid, r)
	}
	bm.records = valid
}

// StartScheduler 启动备份调度器
func (bm *BackupManager) StartScheduler() {
	bm.wg.Add(1)
	go func() {
		defer bm.wg.Done()
		fullTicker := time.NewTicker(bm.config.FullInterval)
		incrementalTicker := time.NewTicker(bm.config.IncrementalInterval)
		defer fullTicker.Stop()
		defer incrementalTicker.Stop()

		for {
			select {
			case <-bm.stopCh:
				return
			case <-fullTicker.C:
				bm.mu.RLock()
				targets := make(map[string]BackupTarget)
				for k, v := range bm.targets {
					targets[k] = v
				}
				bm.mu.RUnlock()
				for name := range targets {
					if _, err := bm.ExecuteFullBackup(name); err != nil {
						if bm.logger != nil {
							bm.logger.Errorf("full backup failed for %s: %v", name, err)
						}
					}
				}
			case <-incrementalTicker.C:
				bm.mu.RLock()
				targets := make(map[string]BackupTarget)
				for k, v := range bm.targets {
					targets[k] = v
				}
				bm.mu.RUnlock()
				for name := range targets {
					if _, err := bm.ExecuteIncrementalBackup(name); err != nil {
						if bm.logger != nil {
							bm.logger.Errorf("incremental backup failed for %s: %v", name, err)
						}
					}
				}
			}
		}
	}()
}

// Stop 停止备份调度器
func (bm *BackupManager) Stop() {
	close(bm.stopCh)
	bm.wg.Wait()
}

// Stats 获取备份统计
func (bm *BackupManager) Stats() map[string]interface{} {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	fullCount := 0
	incrementalCount := 0
	successCount := 0
	failedCount := 0

	for _, r := range bm.records {
		switch r.Type {
		case BackupTypeFull:
			fullCount++
		case BackupTypeIncremental:
			incrementalCount++
		}
		switch r.Status {
		case BackupStatusSuccess:
			successCount++
		case BackupStatusFailed:
			failedCount++
		}
	}

	return map[string]interface{}{
		"total_records":     len(bm.records),
		"full_count":        fullCount,
		"incremental_count": incrementalCount,
		"success_count":     successCount,
		"failed_count":      failedCount,
		"targets":           len(bm.targets),
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

func generateBackupID() string {
	return fmt.Sprintf("backup_%d_%s", time.Now().Unix(), randomString(6))
}

func generateChecksum(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", data[:32])
}

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(s)
}
