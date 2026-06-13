// Package lifecycle 提供监控数据生命周期管理
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type DataCategory string

const (
	CategoryMetric       DataCategory = "metric"
	CategoryLog          DataCategory = "log"
	CategoryTrace        DataCategory = "trace"
	CategoryEvent        DataCategory = "event"
	CategorySystemMetric DataCategory = "system_metric"
	CategoryAppMetric    DataCategory = "app_metric"
	CategoryDBMetric     DataCategory = "db_metric"
	CategorySQLAggregate DataCategory = "sql_aggregate"
	CategoryProfiling    DataCategory = "profiling"
	CategoryAlert        DataCategory = "alert"
	CategoryTopology     DataCategory = "topology"
	CategorySelfMonitor  DataCategory = "self_monitor"
	CategoryCustom       DataCategory = "custom"
)

type CleanupTask struct {
	ID            string        `json:"id"`
	Category      DataCategory  `json:"category"`
	Source        string        `json:"source"`
	RetentionDays int           `json:"retention_days"`
	CutoffTime    time.Time     `json:"cutoff_time"`
	Status        TaskStatus    `json:"status"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	ScannedCount  int64         `json:"scanned_count"`
	DeletedCount  int64         `json:"deleted_count"`
	DeletedBytes  int64         `json:"deleted_bytes"`
	Error         string        `json:"error,omitempty"`
	Duration      time.Duration `json:"duration"`
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusSkipped   TaskStatus = "skipped"
)

type CleanupStats struct {
	TotalTasks      int                                   `json:"total_tasks"`
	SuccessTasks    int                                   `json:"success_tasks"`
	FailedTasks     int                                   `json:"failed_tasks"`
	SkippedTasks    int                                   `json:"skipped_tasks"`
	TotalDuration   time.Duration                         `json:"total_duration"`
	TotalScanned    int64                                 `json:"total_scanned"`
	TotalDeleted    int64                                 `json:"total_deleted"`
	TotalBytesFreed int64                                 `json:"total_bytes_freed"`
	CategoryStats   map[DataCategory]*CategoryCleanupStat `json:"category_stats"`
	LastCleanupTime time.Time                             `json:"last_cleanup_time"`
	NextCleanupTime time.Time                             `json:"next_cleanup_time"`
}

type CategoryCleanupStat struct {
	Category      DataCategory  `json:"category"`
	RetentionDays int           `json:"retention_days"`
	ScannedCount  int64         `json:"scanned_count"`
	DeletedCount  int64         `json:"deleted_count"`
	DeletedBytes  int64         `json:"deleted_bytes"`
	Duration      time.Duration `json:"duration"`
}

type DataScanner interface {
	ScanExpired(ctx context.Context, cutoffTime time.Time, category DataCategory, callback func(batch DataBatch) bool) error
	DeleteBatch(ctx context.Context, batch DataBatch) (int64, int64, error)
	GetCategoryStats(ctx context.Context, category DataCategory) (*CategoryDataStats, error)
}

type DataBatch struct {
	IDs       []string
	Category  DataCategory
	StartTime int64
	EndTime   int64
	Count     int64
	Size      int64
}

type CategoryDataStats struct {
	Category   DataCategory `json:"category"`
	TotalCount int64        `json:"total_count"`
	TotalSize  int64        `json:"total_size"`
	OldestTime time.Time    `json:"oldest_time"`
	NewestTime time.Time    `json:"newest_time"`
	ChunkCount int          `json:"chunk_count"`
}

type LifecycleManager struct {
	config         *LifecycleConfig
	policies       *RetentionPolicyManager
	scheduler      *CleanupScheduler
	scanner        DataScanner
	history        []*CleanupTask
	historyMu      sync.RWMutex
	maxHistory     int
	running        bool
	mu             sync.RWMutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	onCleanupStart func(task *CleanupTask)
	onCleanupEnd   func(task *CleanupTask)
	onStatsUpdate  func(stats *CleanupStats)
}

func NewLifecycleManager(cfg *LifecycleConfig) *LifecycleManager {
	if cfg == nil {
		cfg = DefaultLifecycleConfig()
	}
	mgr := &LifecycleManager{
		config:     cfg,
		policies:   NewRetentionPolicyManager(cfg),
		scheduler:  NewCleanupScheduler(cfg),
		history:    make([]*CleanupTask, 0),
		maxHistory: cfg.MaxHistoryRecords,
		stopCh:     make(chan struct{}),
	}
	mgr.scheduler.SetExecutor(mgr.executeCleanup)
	return mgr
}

func (m *LifecycleManager) SetScanner(scanner DataScanner) {
	m.scanner = scanner
}

func (m *LifecycleManager) SetCleanupStartCallback(callback func(task *CleanupTask)) {
	m.onCleanupStart = callback
}

func (m *LifecycleManager) SetCleanupEndCallback(callback func(task *CleanupTask)) {
	m.onCleanupEnd = callback
}

func (m *LifecycleManager) SetStatsUpdateCallback(callback func(stats *CleanupStats)) {
	m.onStatsUpdate = callback
}

func (m *LifecycleManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("生命周期管理器已在运行")
	}
	m.running = true
	m.mu.Unlock()
	if err := m.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("启动调度器失败: %w", err)
	}
	return nil
}

func (m *LifecycleManager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()
	m.scheduler.Stop()
	m.wg.Wait()
}

func (m *LifecycleManager) executeCleanup(ctx context.Context, category DataCategory) (*CleanupTask, error) {
	if m.scanner == nil {
		return nil, fmt.Errorf("数据扫描器未设置")
	}
	policy := m.policies.GetPolicy(category)
	if policy == nil || !policy.Enabled {
		return nil, nil
	}
	task := &CleanupTask{
		ID:            generateTaskID(),
		Category:      category,
		RetentionDays: policy.RetentionDays,
		CutoffTime:    time.Now().AddDate(0, 0, -policy.RetentionDays),
		Status:        TaskStatusPending,
	}
	if m.onCleanupStart != nil {
		m.onCleanupStart(task)
	}
	task.Status = TaskStatusRunning
	task.StartTime = time.Now()
	err := m.doCleanup(ctx, task)
	task.EndTime = time.Now()
	task.Duration = task.EndTime.Sub(task.StartTime)
	if err != nil {
		task.Status = TaskStatusFailed
		task.Error = err.Error()
	} else if task.DeletedCount == 0 {
		task.Status = TaskStatusSkipped
	} else {
		task.Status = TaskStatusCompleted
	}
	m.recordHistory(task)
	if m.onCleanupEnd != nil {
		m.onCleanupEnd(task)
	}
	if m.onStatsUpdate != nil {
		stats := m.GetStats()
		m.onStatsUpdate(stats)
	}
	return task, err
}

func (m *LifecycleManager) doCleanup(ctx context.Context, task *CleanupTask) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	err := m.scanner.ScanExpired(ctx, task.CutoffTime, task.Category, func(batch DataBatch) bool {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		deleted, bytesFreed, delErr := m.scanner.DeleteBatch(ctx, batch)
		if delErr != nil {
			task.Error = delErr.Error()
			return true
		}
		task.ScannedCount += batch.Count
		task.DeletedCount += deleted
		task.DeletedBytes += bytesFreed
		return true
	})
	return err
}

func (m *LifecycleManager) ManualCleanup(ctx context.Context, categories ...DataCategory) ([]*CleanupTask, error) {
	if len(categories) == 0 {
		categories = m.policies.GetAllCategories()
	}
	tasks := make([]*CleanupTask, 0, len(categories))
	for _, cat := range categories {
		task, err := m.executeCleanup(ctx, cat)
		if err != nil {
			return tasks, fmt.Errorf("清理 %s 失败: %w", cat, err)
		}
		if task != nil {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *LifecycleManager) GetStats() *CleanupStats {
	stats := &CleanupStats{
		CategoryStats: make(map[DataCategory]*CategoryCleanupStat),
	}
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()
	for _, task := range m.history {
		switch task.Status {
		case TaskStatusCompleted:
			stats.SuccessTasks++
		case TaskStatusFailed:
			stats.FailedTasks++
		case TaskStatusSkipped:
			stats.SkippedTasks++
		}
		stats.TotalTasks++
		stats.TotalDuration += task.Duration
		stats.TotalScanned += task.ScannedCount
		stats.TotalDeleted += task.DeletedCount
		stats.TotalBytesFreed += task.DeletedBytes
		catStat, exists := stats.CategoryStats[task.Category]
		if !exists {
			catStat = &CategoryCleanupStat{
				Category:      task.Category,
				RetentionDays: task.RetentionDays,
			}
			stats.CategoryStats[task.Category] = catStat
		}
		catStat.ScannedCount += task.ScannedCount
		catStat.DeletedCount += task.DeletedCount
		catStat.DeletedBytes += task.DeletedBytes
		catStat.Duration += task.Duration
	}
	if len(m.history) > 0 {
		lastTask := m.history[len(m.history)-1]
		stats.LastCleanupTime = lastTask.StartTime
	}
	stats.NextCleanupTime = m.scheduler.GetNextCleanupTime()
	return stats
}

func (m *LifecycleManager) GetHistory(limit int) []*CleanupTask {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()
	if limit <= 0 || limit > len(m.history) {
		limit = len(m.history)
	}
	result := make([]*CleanupTask, limit)
	for i := 0; i < limit; i++ {
		result[i] = m.history[len(m.history)-1-i]
	}
	return result
}

func (m *LifecycleManager) recordHistory(task *CleanupTask) {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()
	m.history = append(m.history, task)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}
}

func generateTaskID() string {
	return fmt.Sprintf("cleanup-%d", time.Now().UnixNano())
}
