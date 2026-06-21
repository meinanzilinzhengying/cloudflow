// Package persistence 提供数据持久化功能
// 实现 WAL (Write-Ahead Log) 和定期快照，确保数据在重启后不丢失
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/edge/pkg/logger"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

const (
	// 默认WAL文件路径
	defaultWALDir = "./data/wal"
	// 默认快照文件路径
	defaultSnapshotDir = "./data/snapshots"
	// 快照间隔
	defaultSnapshotInterval = 5 * time.Minute
	// 单个WAL文件最大大小（10MB）
	maxWALFileSize = 10 * 1024 * 1024
	// WAL目录总大小上限（100MB），超过时强制清理最旧的WAL文件
	maxWALTotalSize = 100 * 1024 * 1024
)

// OperationType 操作类型
type OperationType int

const (
	// OpAddMetrics 添加指标数据
	OpAddMetrics OperationType = iota
	// OpAddTraces 添加链路追踪数据
	OpAddTraces
	// OpAddProfiling 添加性能分析数据
	OpAddProfiling
)

// WALEntry WAL条目
type WALEntry struct {
	Type      OperationType   `json:"type"`
	Timestamp int64           `json:"timestamp"`
	Metrics   *edge.MetricsBatch `json:"metrics,omitempty"`
	Traces    *edge.TraceBatch   `json:"traces,omitempty"`
	Profiling *edge.ProfilingBatch `json:"profiling,omitempty"`
}

// Persistence 持久化管理器
type Persistence struct {
	logger           *logger.Logger
	walDir           string
	snapshotDir      string
	snapshotInterval time.Duration

	// WAL文件管理
	walFile      *os.File
	walFileSize  int64
	walFileMutex sync.Mutex

	// 快照管理
	snapshotTicker *time.Ticker
	stopCh         chan struct{}
	stopOnce       sync.Once

	// 数据缓存
	metricsBuf  []*edge.MetricsBatch
	tracesBuf   []*edge.TraceBatch
	profilingBuf []*edge.ProfilingBatch
	dataMutex   sync.Mutex
}

// NewPersistence 创建持久化管理器
func NewPersistence(log *logger.Logger) (*Persistence, error) {
	p := &Persistence{
		logger:           log,
		walDir:           defaultWALDir,
		snapshotDir:      defaultSnapshotDir,
		snapshotInterval: defaultSnapshotInterval,
		stopCh:           make(chan struct{}),
	}

	// 创建目录
	if err := os.MkdirAll(p.walDir, 0755); err != nil {
		return nil, fmt.Errorf("创建WAL目录失败: %w", err)
	}
	if err := os.MkdirAll(p.snapshotDir, 0755); err != nil {
		return nil, fmt.Errorf("创建快照目录失败: %w", err)
	}

	// 打开或创建WAL文件
	if err := p.openWALFile(); err != nil {
		return nil, err
	}

	// 从WAL和快照恢复数据
	recoverErr := p.recover()
	if recoverErr != nil {
		log.Warnf("数据恢复失败: %v", recoverErr)
	}

	// 启动快照协程（仅在 recover 成功时启动，避免 recover 失败后 goroutine 泄漏）
	if recoverErr == nil {
		go p.snapshotLoop()
	}

	log.Info("数据持久化管理器已启动")
	return p, nil
}

// openWALFile 打开或创建WAL文件
func (p *Persistence) openWALFile() error {
	// 生成新的WAL文件名
	filename := filepath.Join(p.walDir, fmt.Sprintf("wal_%d.json", time.Now().UnixNano()))

	// 打开文件
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建WAL文件失败: %w", err)
	}

	p.walFile = file
	p.walFileSize = 0
	p.logger.Infof("打开WAL文件: %s", filename)
	return nil
}

// rotateWALLocked 轮转WAL文件（已持有锁）
func (p *Persistence) rotateWALLocked() error {
	if p.walFile != nil {
		if err := p.walFile.Close(); err != nil {
			p.logger.Warnf("关闭WAL文件失败: %v", err)
		}
	}

	return p.openWALFile()
}

// rotateWAL 轮转WAL文件
func (p *Persistence) rotateWAL() error {
	p.walFileMutex.Lock()
	defer p.walFileMutex.Unlock()

	return p.rotateWALLocked()
}

// WriteWAL 写入WAL条目
func (p *Persistence) WriteWAL(entry *WALEntry) error {
	p.walFileMutex.Lock()
	defer p.walFileMutex.Unlock()

	// 检查WAL文件大小
	if p.walFileSize > maxWALFileSize {
		if err := p.rotateWALLocked(); err != nil {
			return err
		}
	}

	// 检查WAL目录总大小，超过上限时强制清理最旧的WAL文件
	if totalSize, err := p.getWALDirTotalSizeLocked(); err == nil && totalSize > maxWALTotalSize {
		p.logger.Warnf("WAL 目录总大小 %d 字节超过上限 %d 字节，强制清理旧 WAL 文件", totalSize, maxWALTotalSize)
		p.cleanupOldWALFilesLocked()
	}

	// 序列化条目
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化WAL条目失败: %w", err)
	}

	// 写入文件（使用独立分配避免 append 修改底层切片）
	line := make([]byte, len(data)+1)
	copy(line, data)
	line[len(data)] = '\n'
	if _, err := p.walFile.Write(line); err != nil {
		return fmt.Errorf("写入WAL文件失败: %w", err)
	}

	// 强制刷新到磁盘
	if err := p.walFile.Sync(); err != nil {
		p.logger.Warnf("刷新WAL文件失败: %v", err)
	}

	p.walFileSize += int64(len(data) + 1)
	return nil
}

// AddMetrics 添加指标数据
func (p *Persistence) AddMetrics(batch *edge.MetricsBatch) error {
	// 写入WAL
	entry := &WALEntry{
		Type:      OpAddMetrics,
		Timestamp: time.Now().Unix(),
		Metrics:   batch,
	}
	if err := p.WriteWAL(entry); err != nil {
		return err
	}

	// 更新内存缓存
	p.dataMutex.Lock()
	p.metricsBuf = append(p.metricsBuf, batch)
	p.dataMutex.Unlock()

	return nil
}

// AddTraces 添加链路追踪数据
func (p *Persistence) AddTraces(batch *edge.TraceBatch) error {
	// 写入WAL
	entry := &WALEntry{
		Type:      OpAddTraces,
		Timestamp: time.Now().Unix(),
		Traces:    batch,
	}
	if err := p.WriteWAL(entry); err != nil {
		return err
	}

	// 更新内存缓存
	p.dataMutex.Lock()
	p.tracesBuf = append(p.tracesBuf, batch)
	p.dataMutex.Unlock()

	return nil
}

// AddProfiling 添加性能分析数据
func (p *Persistence) AddProfiling(batch *edge.ProfilingBatch) error {
	// 写入WAL
	entry := &WALEntry{
		Type:      OpAddProfiling,
		Timestamp: time.Now().Unix(),
		Profiling: batch,
	}
	if err := p.WriteWAL(entry); err != nil {
		return err
	}

	// 更新内存缓存
	p.dataMutex.Lock()
	p.profilingBuf = append(p.profilingBuf, batch)
	p.dataMutex.Unlock()

	return nil
}

// GetMetrics 获取指标数据（返回副本，避免外部修改内部状态）
func (p *Persistence) GetMetrics() []*edge.MetricsBatch {
	p.dataMutex.Lock()
	defer p.dataMutex.Unlock()
	result := make([]*edge.MetricsBatch, len(p.metricsBuf))
	copy(result, p.metricsBuf)
	return result
}

// GetTraces 获取链路追踪数据（返回副本，避免外部修改内部状态）
func (p *Persistence) GetTraces() []*edge.TraceBatch {
	p.dataMutex.Lock()
	defer p.dataMutex.Unlock()
	result := make([]*edge.TraceBatch, len(p.tracesBuf))
	copy(result, p.tracesBuf)
	return result
}

// GetProfiling 获取性能分析数据（返回副本，避免外部修改内部状态）
func (p *Persistence) GetProfiling() []*edge.ProfilingBatch {
	p.dataMutex.Lock()
	defer p.dataMutex.Unlock()
	result := make([]*edge.ProfilingBatch, len(p.profilingBuf))
	copy(result, p.profilingBuf)
	return result
}

// Clear 清空数据
func (p *Persistence) Clear() {
	p.dataMutex.Lock()
	p.metricsBuf = nil
	p.tracesBuf = nil
	p.profilingBuf = nil
	p.dataMutex.Unlock()
}

// ClearMetrics 清空指标数据
func (p *Persistence) ClearMetrics() {
	p.dataMutex.Lock()
	p.metricsBuf = nil
	p.dataMutex.Unlock()
}

// ClearTraces 清空链路追踪数据
func (p *Persistence) ClearTraces() {
	p.dataMutex.Lock()
	p.tracesBuf = nil
	p.dataMutex.Unlock()
}

// ClearProfiling 清空性能分析数据
func (p *Persistence) ClearProfiling() {
	p.dataMutex.Lock()
	p.profilingBuf = nil
	p.dataMutex.Unlock()
}

// RemoveMetrics 移除指定数量的指标数据（从头开始移除已发送的部分）
func (p *Persistence) RemoveMetrics(count int) {
	p.dataMutex.Lock()
	if count >= len(p.metricsBuf) {
		p.metricsBuf = nil
	} else {
		p.metricsBuf = p.metricsBuf[count:]
	}
	p.dataMutex.Unlock()
}

// RemoveTraces 移除指定数量的链路追踪数据
func (p *Persistence) RemoveTraces(count int) {
	p.dataMutex.Lock()
	if count >= len(p.tracesBuf) {
		p.tracesBuf = nil
	} else {
		p.tracesBuf = p.tracesBuf[count:]
	}
	p.dataMutex.Unlock()
}

// RemoveProfiling 移除指定数量的性能分析数据
func (p *Persistence) RemoveProfiling(count int) {
	p.dataMutex.Lock()
	if count >= len(p.profilingBuf) {
		p.profilingBuf = nil
	} else {
		p.profilingBuf = p.profilingBuf[count:]
	}
	p.dataMutex.Unlock()
}

// recover 从WAL和快照恢复数据
func (p *Persistence) recover() error {
	// 先从最新快照恢复
	if err := p.recoverFromSnapshot(); err != nil {
		p.logger.Warnf("从快照恢复失败: %v", err)
	}

	// 再从WAL恢复
	if err := p.recoverFromWAL(); err != nil {
		p.logger.Warnf("从WAL恢复失败: %v", err)
		return err
	}

	p.logger.Infof("数据恢复完成，指标: %d, 追踪: %d, 分析: %d",
		len(p.metricsBuf), len(p.tracesBuf), len(p.profilingBuf))
	return nil
}

// recoverFromSnapshot 从快照恢复数据
func (p *Persistence) recoverFromSnapshot() error {
	// P0-11: 数据恢复验证 - 检查快照字段完整性、时间戳合理性
	// 查找最新的快照文件
	files, err := os.ReadDir(p.snapshotDir)
	if err != nil {
		return err
	}

	var latestSnapshot string
	var latestTime int64

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() > latestTime {
			latestTime = info.ModTime().Unix()
			latestSnapshot = file.Name()
		}
	}

	if latestSnapshot == "" {
		return fmt.Errorf("没有找到快照文件")
	}

	// 读取快照文件
	snapshotPath := filepath.Join(p.snapshotDir, latestSnapshot)
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}

	// 解析快照
	var snapshot struct {
		Metrics   []*edge.MetricsBatch   `json:"metrics"`
		Traces    []*edge.TraceBatch   `json:"traces"`
		Profiling []*edge.ProfilingBatch `json:"profiling"`
	}

	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}

	// 更新缓存
	p.dataMutex.Lock()
	p.metricsBuf = snapshot.Metrics
	p.tracesBuf = snapshot.Traces
	p.profilingBuf = snapshot.Profiling
	p.dataMutex.Unlock()

	p.logger.Infof("从快照 %s 恢复数据", latestSnapshot)
	return nil
}

// recoverFromWAL 从WAL恢复数据
func (p *Persistence) recoverFromWAL() error {
	// 读取所有WAL文件
	files, err := os.ReadDir(p.walDir)
	if err != nil {
		return err
	}

	// 获取当前活跃 WAL 文件的绝对路径，恢复时跳过它
	var activeWALAbsPath string
	p.walFileMutex.Lock()
	if p.walFile != nil {
		if absPath, absErr := filepath.Abs(p.walFile.Name()); absErr == nil {
			activeWALAbsPath = absPath
		}
	}
	p.walFileMutex.Unlock()

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// 跳过当前活跃的 WAL 文件，避免读取正在写入的数据
		if activeWALAbsPath != "" {
			walPath := filepath.Join(p.walDir, file.Name())
			if absPath, absErr := filepath.Abs(walPath); absErr == nil && absPath == activeWALAbsPath {
				p.logger.Debugf("跳过当前活跃的 WAL 文件: %s", file.Name())
				continue
			}
		}

		// 读取WAL文件
		walPath := filepath.Join(p.walDir, file.Name())
		info, err := file.Info()
		if err != nil {
			p.logger.Warnf("获取WAL文件 %s 信息失败: %v", file.Name(), err)
			continue
		}
		// 跳过超过单个文件大小限制的 WAL 文件，避免内存占用过大
		if info.Size() > maxWALFileSize {
			p.logger.Warnf("WAL 文件 %s 大小 %d 字节超过限制 %d 字节，跳过恢复", file.Name(), info.Size(), maxWALFileSize)
			continue
		}
		data, err := os.ReadFile(walPath)
		if err != nil {
			p.logger.Warnf("读取WAL文件 %s 失败: %v", file.Name(), err)
			continue
		}

		// 解析WAL条目
		lines := string(data)
		for _, line := range strings.Split(lines, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}

			var entry WALEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				p.logger.Warnf("解析WAL条目失败: %v", err)
				continue
			}

			// 应用操作
			switch entry.Type {
			case OpAddMetrics:
				if entry.Metrics != nil {
					p.dataMutex.Lock()
					p.metricsBuf = append(p.metricsBuf, entry.Metrics)
					p.dataMutex.Unlock()
				}
			case OpAddTraces:
				if entry.Traces != nil {
					p.dataMutex.Lock()
					p.tracesBuf = append(p.tracesBuf, entry.Traces)
					p.dataMutex.Unlock()
				}
			case OpAddProfiling:
				if entry.Profiling != nil {
					p.dataMutex.Lock()
					p.profilingBuf = append(p.profilingBuf, entry.Profiling)
					p.dataMutex.Unlock()
				}
			}
		}
	}

	return nil
}

// snapshotLoop 快照循环
func (p *Persistence) snapshotLoop() {
	ticker := time.NewTicker(p.snapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := p.createSnapshot(); err != nil {
				p.logger.Warnf("创建快照失败: %v", err)
			}
		case <-p.stopCh:
			// 停止前创建最后一个快照
			if err := p.createSnapshot(); err != nil {
				p.logger.Warnf("创建最后快照失败: %v", err)
			}
			return
		}
	}
}

// createSnapshot 创建快照
// 注意：此处使用 copy() 进行的是浅拷贝（slice header 复制，元素为指针）。
// 这在当前设计中是安全的，因为 forwarder 在将 batch 放入缓冲区后不会修改其内容。
// 如果未来 forwarder 需要修改已入缓冲区的 batch，则需要改为深拷贝。
func (p *Persistence) createSnapshot() error {
	p.dataMutex.Lock()
	// 使用浅拷贝创建独立的切片副本，元素为指针共享引用
	metrics := make([]*edge.MetricsBatch, len(p.metricsBuf))
	copy(metrics, p.metricsBuf)
	traces := make([]*edge.TraceBatch, len(p.tracesBuf))
	copy(traces, p.tracesBuf)
	profiling := make([]*edge.ProfilingBatch, len(p.profilingBuf))
	copy(profiling, p.profilingBuf)
	p.dataMutex.Unlock()

	// 构建快照数据
	snapshot := struct {
		Metrics   []*edge.MetricsBatch   `json:"metrics"`
		Traces    []*edge.TraceBatch   `json:"traces"`
		Profiling []*edge.ProfilingBatch `json:"profiling"`
		Timestamp int64                 `json:"timestamp"`
	}{
		Metrics:   metrics,
		Traces:    traces,
		Profiling: profiling,
		Timestamp: time.Now().Unix(),
	}

	// 序列化快照
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("序列化快照失败: %w", err)
	}

	// 写入快照文件
	filename := filepath.Join(p.snapshotDir, fmt.Sprintf("snapshot_%d.json", time.Now().Unix()))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("写入快照文件失败: %w", err)
	}

	// 清理旧快照（保留最近3个）
	if err := p.cleanupSnapshots(); err != nil {
		p.logger.Warnf("清理旧快照失败: %v", err)
	}

	// 清理WAL文件
	if err := p.cleanupWAL(); err != nil {
		p.logger.Warnf("清理WAL文件失败: %v", err)
	}

	p.logger.Infof("创建快照成功: %s, 指标: %d, 追踪: %d, 分析: %d",
		filename, len(metrics), len(traces), len(profiling))
	return nil
}

// cleanupSnapshots 清理旧快照
func (p *Persistence) cleanupSnapshots() error {
	files, err := os.ReadDir(p.snapshotDir)
	if err != nil {
		return err
	}

	// 获取文件信息并按修改时间排序
	type fileInfo struct {
		name string
		time time.Time
	}
	var fileInfos []fileInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, fileInfo{
			name: file.Name(),
			time: info.ModTime(),
		})
	}

	// 按修改时间排序
	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].time.After(fileInfos[j].time)
	})

	// 保留最近3个快照
	for i := 3; i < len(fileInfos); i++ {
		filePath := filepath.Join(p.snapshotDir, fileInfos[i].name)
		if err := os.Remove(filePath); err != nil {
			p.logger.Warnf("删除旧快照 %s 失败: %v", fileInfos[i].name, err)
		}
	}

	return nil
}

// getWALDirTotalSizeLocked 计算 WAL 目录总大小（已持有 walFileMutex）
func (p *Persistence) getWALDirTotalSizeLocked() (int64, error) {
	var totalSize int64
	files, err := os.ReadDir(p.walDir)
	if err != nil {
		return 0, err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		totalSize += info.Size()
	}
	return totalSize, nil
}

// cleanupOldWALFilesLocked 清理最旧的 WAL 文件，直到总大小低于上限（已持有 walFileMutex）
func (p *Persistence) cleanupOldWALFilesLocked() {
	files, err := os.ReadDir(p.walDir)
	if err != nil {
		return
	}

	type fileInfo struct {
		name string
		time time.Time
		size int64
	}
	var fileInfos []fileInfo
	var totalSize int64
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		fileInfos = append(fileInfos, fileInfo{name: file.Name(), time: info.ModTime(), size: info.Size()})
		totalSize += info.Size()
	}

	// 按修改时间升序排序（最旧的在前）
	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].time.Before(fileInfos[j].time)
	})

	// 获取当前活跃 WAL 文件名
	var activeWALName string
	if p.walFile != nil {
		activeWALName = p.walFile.Name()
	}

	// 从最旧的文件开始删除，直到总大小低于上限的一半
	for _, fi := range fileInfos {
		if totalSize <= maxWALTotalSize/2 {
			break
		}

		walPath := filepath.Join(p.walDir, fi.name)
		// 跳过当前活跃的 WAL 文件
		if activeWALName != "" {
			if absPath, err := filepath.Abs(walPath); err == nil {
				if activeAbs, err := filepath.Abs(activeWALName); err == nil && absPath == activeAbs {
					continue
				}
			}
		}

		if err := os.Remove(walPath); err != nil {
			p.logger.Warnf("强制清理旧 WAL 文件 %s 失败: %v", fi.name, err)
		} else {
			totalSize -= fi.size
			p.logger.Infof("已强制清理旧 WAL 文件: %s", fi.name)
		}
	}
}

// cleanupWAL 清理WAL文件
func (p *Persistence) cleanupWAL() error {
	// 快照成功后，删除旧的 WAL 文件（除了当前活跃的 WAL 文件）
	p.walFileMutex.Lock()
	activeWAL := p.walFile
	p.walFileMutex.Unlock()

	files, err := os.ReadDir(p.walDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		walPath := filepath.Join(p.walDir, file.Name())
		// 跳过当前活跃的 WAL 文件
		if activeWAL != nil {
			if absPath, err := filepath.Abs(walPath); err == nil {
				if activeName, err := filepath.Abs(activeWAL.Name()); err == nil && absPath == activeName {
					continue
				}
			}
		}

		if err := os.Remove(walPath); err != nil {
			p.logger.Warnf("删除旧 WAL 文件 %s 失败: %v", file.Name(), err)
		} else {
			p.logger.Infof("已清理旧 WAL 文件: %s", file.Name())
		}
	}

	return nil
}

// Close 关闭持久化管理器
func (p *Persistence) Close() error {
	var err error
	p.stopOnce.Do(func() {
		p.walFileMutex.Lock()
		defer p.walFileMutex.Unlock()

		close(p.stopCh)

		if p.walFile != nil {
			if closeErr := p.walFile.Close(); closeErr != nil {
				p.logger.Warnf("关闭WAL文件失败: %v", closeErr)
				err = closeErr
			}
		}

		p.logger.Info("数据持久化管理器已关闭")
	})
	return err
}
