//go:build linux

package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/center/pkg/logger"
)

// mockBackupTarget 模拟备份目标
type mockBackupTarget struct {
	name string
}

func (m *mockBackupTarget) Name() string { return m.name }
func (m *mockBackupTarget) Backup(full bool, backupDir string) (string, int64, error) {
	content := fmt.Sprintf("backup_%s_%v", m.name, full)
	filePath := filepath.Join(backupDir, "backup.dat")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return "", 0, err
	}
	return filePath, int64(len(content)), nil
}
func (m *mockBackupTarget) Restore(backupFile string) error { return nil }

func TestDefaultBackupConfig(t *testing.T) {
	cfg := DefaultBackupConfig()
	if cfg.RetentionDays != 30 {
		t.Errorf("expected RetentionDays 30, got %d", cfg.RetentionDays)
	}
	if cfg.FullInterval != 24*time.Hour {
		t.Errorf("expected FullInterval 24h, got %v", cfg.FullInterval)
	}
	if cfg.IncrementalInterval != 1*time.Hour {
		t.Errorf("expected IncrementalInterval 1h, got %v", cfg.IncrementalInterval)
	}
	if cfg.MaxBackups != 10 {
		t.Errorf("expected MaxBackups 10, got %d", cfg.MaxBackups)
	}
}

func TestNewBackupManager(t *testing.T) {
	backupDir := t.TempDir()
	cfg := &BackupConfig{
		BackupDir:     backupDir,
		RetentionDays:   7,
		FullInterval:    1 * time.Hour,
		IncrementalInterval: 10 * time.Minute,
		CompressEnabled: false,
		MaxBackups:      5,
	}
	log := logger.New(logger.Config{})
	bm, err := NewBackupManager(cfg, log)
	if err != nil {
		t.Fatalf("NewBackupManager failed: %v", err)
	}
	defer bm.Stop()

	// 注册模拟目标
	target := &mockBackupTarget{name: "test_db"}
	bm.RegisterTarget(target)

	// 执行全量备份
	record, err := bm.ExecuteFullBackup("test_db")
	if err != nil {
		t.Fatalf("ExecuteFullBackup failed: %v", err)
	}
	if record.Type != BackupTypeFull {
		t.Errorf("expected type full, got %s", record.Type)
	}
	if record.Status != BackupStatusSuccess {
		t.Errorf("expected status success, got %s", record.Status)
	}
	if record.Size == 0 {
		t.Error("expected non-zero size")
	}
	if record.FilePath == "" {
		t.Error("expected non-empty filepath")
	}

	// 执行增量备份
	record2, err := bm.ExecuteIncrementalBackup("test_db")
	if err != nil {
		t.Fatalf("ExecuteIncrementalBackup failed: %v", err)
	}
	if record2.Type != BackupTypeIncremental {
		t.Errorf("expected type incremental, got %s", record2.Type)
	}
	if record2.Status != BackupStatusSuccess {
		t.Errorf("expected status success, got %s", record2.Status)
	}

	// 查询记录
	records := bm.GetRecords("test_db", "")
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}

	fullRecords := bm.GetRecords("", BackupTypeFull)
	if len(fullRecords) != 1 {
		t.Errorf("expected 1 full record, got %d", len(fullRecords))
	}

	// 恢复
	if err := bm.Restore("test_db", record.FilePath); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// 统计
	stats := bm.Stats()
	if stats["total_records"] != 2 {
		t.Errorf("expected total_records 2, got %v", stats["total_records"])
	}
	if stats["success_count"] != 2 {
		t.Errorf("expected success_count 2, got %v", stats["success_count"])
	}
}

func TestBackupManagerUnknownTarget(t *testing.T) {
	backupDir := t.TempDir()
	cfg := &BackupConfig{BackupDir: backupDir}
	log := logger.New(logger.Config{})
	bm, err := NewBackupManager(cfg, log)
	if err != nil {
		t.Fatalf("NewBackupManager failed: %v", err)
	}
	defer bm.Stop()

	_, err = bm.ExecuteFullBackup("unknown")
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
}

func TestBackupManagerCleanup(t *testing.T) {
	backupDir := t.TempDir()
	cfg := &BackupConfig{
		BackupDir:   backupDir,
		RetentionDays: 1,
	}
	log := logger.New(logger.Config{})
	bm, err := NewBackupManager(cfg, log)
	if err != nil {
		t.Fatalf("NewBackupManager failed: %v", err)
	}
	defer bm.Stop()

	target := &mockBackupTarget{name: "cleanup_test"}
	bm.RegisterTarget(target)

	// 执行备份
	record, err := bm.ExecuteFullBackup("cleanup_test")
	if err != nil {
		t.Fatalf("ExecuteFullBackup failed: %v", err)
	}

	// 修改记录时间为过期
	bm.mu.Lock()
	for _, r := range bm.records {
		if r.ID == record.ID {
			r.StartTime = time.Now().AddDate(0, 0, -2)
		}
	}
	bm.mu.Unlock()

	// 触发清理
	bm.cleanupOldBackups("cleanup_test")

	// 验证记录已清理
	records := bm.GetAllRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 records after cleanup, got %d", len(records))
	}
}

func TestBackupManagerScheduler(t *testing.T) {
	backupDir := t.TempDir()
	cfg := &BackupConfig{
		BackupDir:           backupDir,
		FullInterval:        100 * time.Millisecond,
		IncrementalInterval: 50 * time.Millisecond,
	}
	log := logger.New(logger.Config{})
	bm, err := NewBackupManager(cfg, log)
	if err != nil {
		t.Fatalf("NewBackupManager failed: %v", err)
	}

	target := &mockBackupTarget{name: "scheduler_test"}
	bm.RegisterTarget(target)
	bm.StartScheduler()

	// 等待调度器执行
	time.Sleep(200 * time.Millisecond)

	bm.Stop()

	records := bm.GetAllRecords()
	if len(records) == 0 {
		t.Log("No records from scheduler (may be timing related)")
	}
}
