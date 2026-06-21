// P7: 数据归档策略引擎
package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// 核心类型定义
// ============================================================================

// ArchivePolicy 归档策略
type ArchivePolicy struct {
	Category       DataCategory `json:"category"`
	Enabled        bool         `json:"enabled"`
	
	// 归档触发条件
	ArchiveAfterDays   int      `json:"archive_after_days"`   // 数据超过多少天后归档(默认90)
	ArchiveWhenSizeGB  float64  `json:"archive_when_size_gb"` // 数据超过多少GB后归档
	ArchiveWhenCount   int64    `json:"archive_when_count"`   // 数据超过多少条后归档
	
	// 归档目标
	ArchivePath        string   `json:"archive_path"`         // 归档路径
	CompressionType    string   `json:"compression_type"`      // 压缩类型(gzip/zstd/none)
	CompressLevel      int      `json:"compress_level"`        // 压缩级别(1-9)
	
	// 归档命名
	NamingPattern      string   `json:"naming_pattern"`        // 命名模式(如"{category}_{date}")
	
	// 清理策略
	DeleteAfterArchive bool     `json:"delete_after_archive"`  // 归档后是否删除原数据
	KeepArchiveDays    int      `json:"keep_archive_days"`     // 归档文件保留天数(默认365)
	
	// 执行窗口
	ArchiveWindow      string   `json:"archive_window"`        // 归档窗口(如"02:00-06:00")
	MaxArchiveRate     int64    `json:"max_archive_rate"`      // 每秒最大归档条数
}

// DefaultArchivePolicy 返回默认归档策略
func DefaultArchivePolicy(category DataCategory) *ArchivePolicy {
	return &ArchivePolicy{
		Category:           category,
		Enabled:            true,
		ArchiveAfterDays:   90,
		ArchiveWhenSizeGB:  10,
		ArchivePath:        "/var/lib/cloudflow/archives",
		CompressionType:    "gzip",
		CompressLevel:      6,
		NamingPattern:      "{category}_{date}_{seq}",
		DeleteAfterArchive: false,
		KeepArchiveDays:    365,
		ArchiveWindow:      "02:00-06:00",
		MaxArchiveRate:     50000,
	}
}

// Validate 验证归档策略
func (p *ArchivePolicy) Validate() error {
	if p.Category == "" {
		return fmt.Errorf("category cannot be empty")
	}
	if p.ArchiveAfterDays < 1 {
		p.ArchiveAfterDays = 90
	}
	if p.ArchivePath == "" {
		p.ArchivePath = "/var/lib/cloudflow/archives"
	}
	if p.CompressionType == "" {
		p.CompressionType = "gzip"
	}
	if p.CompressLevel < 1 || p.CompressLevel > 9 {
		p.CompressLevel = 6
	}
	if p.KeepArchiveDays < 1 {
		p.KeepArchiveDays = 365
	}
	return nil
}

// GetArchiveFileName 生成归档文件名
func (p *ArchivePolicy) GetArchiveFileName(seq int) string {
	date := time.Now().Format("20060102")
	name := p.NamingPattern
	name = strings.ReplaceAll(name, "{category}", string(p.Category))
	name = strings.ReplaceAll(name, "{date}", date)
	name = strings.ReplaceAll(name, "{seq}", fmt.Sprintf("%04d", seq))
	return name + ".tar.gz"
}

// IsInArchiveWindow 检查当前是否在归档窗口内
func (p *ArchivePolicy) IsInArchiveWindow() bool {
	if p.ArchiveWindow == "" {
		return true
	}
	
	parts := strings.Split(p.ArchiveWindow, "-")
	if len(parts) != 2 {
		return true
	}
	
	now := time.Now()
	start, err1 := time.Parse("15:04", parts[0])
	end, err2 := time.Parse("15:04", parts[1])
	if err1 != nil || err2 != nil {
		return true
	}
	
	nowTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
	startTime := time.Date(0, 1, 1, start.Hour(), start.Minute(), 0, 0, time.UTC)
	endTime := time.Date(0, 1, 1, end.Hour(), end.Minute(), 0, 0, time.UTC)
	
	return nowTime.After(startTime) && nowTime.Before(endTime)
}

// ============================================================================
// 归档管理器
// ============================================================================

// ArchiveManager 归档管理器
type ArchiveManager struct {
	mu        sync.RWMutex
	policies  map[DataCategory]*ArchivePolicy
	stats     map[DataCategory]*ArchiveStats
	
	// 归档操作
	archiveFunc func(category DataCategory, batch DataBatch, archivePath string) (*ArchiveRecord, error)
}

// NewArchiveManager 创建归档管理器
func NewArchiveManager() *ArchiveManager {
	return &ArchiveManager{
		policies: make(map[DataCategory]*ArchivePolicy),
		stats:    make(map[DataCategory]*ArchiveStats),
	}
}

// RegisterPolicy 注册归档策略
func (am *ArchiveManager) RegisterPolicy(policy *ArchivePolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	
	am.mu.Lock()
	defer am.mu.Unlock()
	am.policies[policy.Category] = policy
	return nil
}

// GetPolicy 获取策略
func (am *ArchiveManager) GetPolicy(category DataCategory) *ArchivePolicy {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.policies[category]
}

// ShouldArchive 判断是否应该归档
func (am *ArchiveManager) ShouldArchive(category DataCategory, stats *CategoryDataStats) bool {
	policy := am.GetPolicy(category)
	if policy == nil || !policy.Enabled {
		return false
	}
	
	if !policy.IsInArchiveWindow() {
		return false
	}
	
	if stats == nil {
		return false
	}
	
	// 检查年龄
	if !stats.OldestTime.IsZero() {
		age := time.Since(stats.OldestTime)
		if age >= time.Duration(policy.ArchiveAfterDays)*24*time.Hour {
			return true
		}
	}
	
	// 检查大小
	if policy.ArchiveWhenSizeGB > 0 {
		sizeGB := float64(stats.TotalSize) / (1024 * 1024 * 1024)
		if sizeGB >= policy.ArchiveWhenSizeGB {
			return true
		}
	}
	
	// 检查数量
	if policy.ArchiveWhenCount > 0 && stats.TotalCount >= policy.ArchiveWhenCount {
		return true
	}
	
	return false
}

// ArchiveData 执行数据归档
func (am *ArchiveManager) ArchiveData(category DataCategory, scanner DataScanner) (*ArchiveResult, error) {
	result := &ArchiveResult{Category: category}
	
	policy := am.GetPolicy(category)
	if policy == nil || !policy.Enabled {
		result.Status = "skipped"
		return result, nil
	}
	
	if scanner == nil {
		result.Status = "failed"
		result.Error = "scanner not set"
		return result, fmt.Errorf("scanner not set")
	}
	
	// 创建归档目录
	if err := os.MkdirAll(policy.ArchivePath, 0755); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, err
	}
	
	// 确定归档截止时间
	cutoff := time.Now().AddDate(0, 0, -policy.ArchiveAfterDays)
	
	seq := 1
	var totalCount, totalSize int64
	
	err := scanner.ScanExpired(nil, cutoff, category, func(batch DataBatch) bool {
		archivePath := filepath.Join(policy.ArchivePath, policy.GetArchiveFileName(seq))
		record, err := am.doArchive(category, batch, archivePath, policy)
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
			return false
		}
		
		result.Records = append(result.Records, record)
		totalCount += batch.Count
		totalSize += batch.Size
		seq++
		return true
	})
	
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	} else {
		result.Status = "completed"
	}
	
	result.TotalCount = totalCount
	result.TotalSize = totalSize
	result.ArchivedAt = time.Now()
	
	// 更新统计
	am.updateStats(category, result)
	
	return result, err
}

func (am *ArchiveManager) doArchive(category DataCategory, batch DataBatch, archivePath string, policy *ArchivePolicy) (*ArchiveRecord, error) {
	// 使用 gzip + tar 归档
	file, err := os.Create(archivePath)
	if err != nil {
		return nil, fmt.Errorf("create archive file: %w", err)
	}
	defer file.Close()
	
	gw, err := gzip.NewWriterLevel(file, policy.CompressLevel)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}
	defer gw.Close()
	
	tw := tar.NewWriter(gw)
	defer tw.Close()
	
	// 写入元数据
	metaContent := fmt.Sprintf("category: %s\nstart_time: %d\nend_time: %d\ncount: %d\nsize: %d\narchived_at: %s\n",
		category, batch.StartTime, batch.EndTime, batch.Count, batch.Size, time.Now().Format(time.RFC3339))
	
	header := &tar.Header{
		Name: "metadata.txt",
		Size: int64(len(metaContent)),
		Mode: 0644,
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tw.Write([]byte(metaContent)); err != nil {
		return nil, fmt.Errorf("write metadata: %w", err)
	}
	
	// 计算压缩率
	compressedSize := int64(len(metaContent)) + 100 // 估算
	compressionRatio := 1.0
	if batch.Size > 0 && compressedSize > 0 {
		compressionRatio = float64(batch.Size) / float64(compressedSize)
	}
	
	record := &ArchiveRecord{
		Category:         category,
		FilePath:         archivePath,
		OriginalCount:    batch.Count,
		OriginalSize:     batch.Size,
		CompressedSize:   compressedSize,
		CompressionRatio: compressionRatio,
		ArchivedAt:       time.Now(),
	}
	
	return record, nil
}

func (am *ArchiveManager) updateStats(category DataCategory, result *ArchiveResult) {
	am.mu.Lock()
	defer am.mu.Unlock()
	
	stats, ok := am.stats[category]
	if !ok {
		stats = &ArchiveStats{Category: category}
		am.stats[category] = stats
	}
	
	stats.TotalArchived += result.TotalCount
	stats.TotalSize += result.TotalSize
	stats.LastArchive = time.Now()
	stats.ArchiveCount += int64(len(result.Records))
}

// GetStats 获取归档统计
func (am *ArchiveManager) GetStats(category DataCategory) *ArchiveStats {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.stats[category]
}

// GetAllStats 获取所有归档统计
func (am *ArchiveManager) GetAllStats() []*ArchiveStats {
	am.mu.RLock()
	defer am.mu.RUnlock()
	
	var stats []*ArchiveStats
	for _, s := range am.stats {
		stats = append(stats, s)
	}
	
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Category < stats[j].Category
	})
	
	return stats
}

// CleanOldArchives 清理过期归档文件
func (am *ArchiveManager) CleanOldArchives(category DataCategory) (int64, error) {
	policy := am.GetPolicy(category)
	if policy == nil || policy.ArchivePath == "" {
		return 0, nil
	}
	
	cutoff := time.Now().AddDate(0, 0, -policy.KeepArchiveDays)
	var deletedCount int64
	
	err := filepath.Walk(policy.ArchivePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil {
				deletedCount++
			}
		}
		return nil
	})
	
	return deletedCount, err
}

// ============================================================================
// 归档结果类型
// ============================================================================

// ArchiveResult 归档结果
type ArchiveResult struct {
	Category     DataCategory
	Status       string
	TotalCount   int64
	TotalSize    int64
	Records      []*ArchiveRecord
	Errors       []string
	Error        string
	ArchivedAt   time.Time
}

// ArchiveRecord 归档记录
type ArchiveRecord struct {
	Category         DataCategory
	FilePath         string
	OriginalCount    int64
	OriginalSize     int64
	CompressedSize   int64
	CompressionRatio float64
	ArchivedAt       time.Time
	DeletedSource    bool
}

// ArchiveStats 归档统计
type ArchiveStats struct {
	Category      DataCategory
	TotalArchived int64
	TotalSize     int64
	ArchiveCount  int64
	LastArchive   time.Time
}

// ============================================================================
// 归档读取器
// ============================================================================

// ArchiveReader 归档读取器
type ArchiveReader struct{}

// NewArchiveReader 创建归档读取器
func NewArchiveReader() *ArchiveReader {
	return &ArchiveReader{}
}

// ReadArchive 读取归档文件
func (r *ArchiveReader) ReadArchive(archivePath string) ([]byte, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()
	
	gr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gr.Close()
	
	tr := tar.NewReader(gr)
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		
		if header.Name == "metadata.txt" {
			content := make([]byte, header.Size)
			if _, err := io.ReadFull(tr, content); err != nil {
				return nil, fmt.Errorf("read metadata: %w", err)
			}
			return content, nil
		}
	}
	
	return nil, fmt.Errorf("metadata not found in archive")
}
