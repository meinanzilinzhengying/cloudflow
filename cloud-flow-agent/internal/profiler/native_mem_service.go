// Package profiler 提供 Java 堆外内存检测功能
// 本文件提供 Agent 集成服务接口
package profiler

import (
	"context"
	"fmt"
	"time"
)

// ==================== Agent 集成服务 ====================

// NativeMemService Java 堆外内存检测服务
type NativeMemService struct {
	tracker *NativeMemTracker
	detector *LeakDetector
}

// NativeMemRequest 堆外内存检测请求
type NativeMemRequest struct {
	PID            uint32  `json:"pid"`              // Java 进程 ID
	Duration       int     `json:"duration"`         // 检测时长(秒)
	MinBlockSize   int64   `json:"min_block_size"`   // 最小追踪块大小
	MaxBlocks      int     `json:"max_blocks"`       // 最大追踪块数
	MinLeakAgeSec  int     `json:"min_leak_age_sec"` // 最小泄漏年龄(秒)
	GenerateReport bool    `json:"generate_report"`  // 是否生成报告
}

// NativeMemResponse 堆外内存检测响应
type NativeMemResponse struct {
	Status       string            `json:"status"`        // 状态
	Stats        NativeMemStats    `json:"stats"`         // 堆外内存统计
	LeakSummary  *LeakSummary      `json:"leak_summary"`  // 泄漏摘要
	Report       string            `json:"report"`        // 文本报告
	Snapshot     *NativeMemSnapshot `json:"snapshot"`     // 内存快照
	Error        string            `json:"error"`         // 错误信息
}

// NewNativeMemService 创建堆外内存检测服务
func NewNativeMemService() *NativeMemService {
	return &NativeMemService{
		detector: NewLeakDetector(nil),
	}
}

// Detect 执行堆外内存检测
func (s *NativeMemService) Detect(ctx context.Context, req NativeMemRequest) (*NativeMemResponse, error) {
	if req.PID == 0 {
		return nil, fmt.Errorf("PID 不能为 0")
	}
	if req.Duration <= 0 {
		req.Duration = 60 // 默认 60 秒
	}

	// 创建追踪器
	cfg := &NativeMemConfig{
		PID:          req.PID,
		Duration:     time.Duration(req.Duration) * time.Second,
		MinBlockSize: req.MinBlockSize,
		MaxBlocks:    req.MaxBlocks,
		TrackStack:   true,
	}

	if cfg.MinBlockSize == 0 {
		cfg.MinBlockSize = 256
	}
	if cfg.MaxBlocks == 0 {
		cfg.MaxBlocks = 100000
	}

	tracker, err := NewNativeMemTracker(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建追踪器失败: %w", err)
	}
	s.tracker = tracker

	// 启动追踪
	if err := tracker.Start(ctx); err != nil {
		return nil, fmt.Errorf("启动追踪失败: %w", err)
	}

	// 等待追踪完成
	select {
	case <-ctx.Done():
		tracker.Stop()
		return &NativeMemResponse{
			Status: "cancelled",
			Error:  "检测被取消",
		}, nil
	case <-time.After(cfg.Duration):
		tracker.Stop()
	}

	// 获取结果
	stats := tracker.GetStats()
	activeBlocks := tracker.GetActiveBlocks()

	// 配置泄漏检测
	detectorCfg := &LeakDetectorConfig{
		MinBlockSize:   1024,
		MaxReportCount: 100,
		GroupByStack:   true,
		SortBy:         "size",
	}
	if req.MinLeakAgeSec > 0 {
		detectorCfg.MinLeakAge = time.Duration(req.MinLeakAgeSec) * time.Second
	}
	s.detector = NewLeakDetector(detectorCfg)

	// 执行泄漏检测
	leakSummary := s.detector.Detect(activeBlocks, time.Now())

	// 获取 Java 进程信息
	javaStack := NewJavaStackTranslator(req.PID)
	javaInfo := javaStack.GetProcessJavaInfo()
	javaStack.Close()

	// 生成报告
	response := &NativeMemResponse{
		Status:      "completed",
		Stats:       stats,
		LeakSummary: leakSummary,
	}

	if req.GenerateReport {
		reportCfg := DefaultReportConfig()
		response.Report = GenerateReport(leakSummary, stats, javaInfo, reportCfg)
	}

	// 获取快照
	response.Snapshot = tracker.TakeSnapshot()

	tracker.Close()
	s.tracker = nil

	return response, nil
}

// TakeSnapshot 获取当前堆外内存快照
func (s *NativeMemService) TakeSnapshot(pid uint32) (*NativeMemSnapshot, error) {
	if s.tracker == nil {
		return nil, fmt.Errorf("追踪器未运行")
	}
	return s.tracker.TakeSnapshot(), nil
}

// GetStats 获取当前统计信息
func (s *NativeMemService) GetStats() (NativeMemStats, error) {
	if s.tracker == nil {
		return NativeMemStats{}, fmt.Errorf("追踪器未运行")
	}
	return s.tracker.GetStats(), nil
}

// GetActiveBlocks 获取所有未释放的内存块
func (s *NativeMemService) GetActiveBlocks() ([]*MemoryBlock, error) {
	if s.tracker == nil {
		return nil, fmt.Errorf("追踪器未运行")
	}
	return s.tracker.GetActiveBlocks(), nil
}

// GetStatsBySource 按来源获取统计
func (s *NativeMemService) GetStatsBySource() (map[AllocSource]*SourceStat, error) {
	if s.tracker == nil {
		return nil, fmt.Errorf("追踪器未运行")
	}
	return s.tracker.GetStatsBySource(), nil
}

// Close 关闭服务
func (s *NativeMemService) Close() {
	if s.tracker != nil {
		s.tracker.Close()
		s.tracker = nil
	}
}
