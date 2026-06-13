// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件提供Agent集成接口，将剖析功能暴露给外部调用
package profiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ==================== Agent集成接口 ====================

// ProfilerService 剖析服务接口
// 提供给Agent主模块调用的接口
type ProfilerService struct {
	manager   *ProfilerManager
	config    *ServiceConfig
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	DefaultSampleFreq    int           // 默认采样频率
	DefaultDuration      time.Duration // 默认剖析时长
	MaxConcurrentSessions int          // 最大并发会话数
	EnableAutoDiscovery  bool          // 是否启用进程自动发现
}

// ProfileRequest 剖析请求
type ProfileRequest struct {
	// 目标选择
	PID         uint32   `json:"pid,omitempty"`          // 目标进程ID
	PIDs        []uint32 `json:"pids,omitempty"`         // 多个目标进程
	ProcessName string   `json:"process_name,omitempty"` // 进程名匹配

	// 剖析配置
	Duration       int    `json:"duration,omitempty"`        // 剖析时长(秒)
	SampleFreq     int    `json:"sample_freq,omitempty"`     // 采样频率(Hz)
	MaxStackDepth  int    `json:"max_stack_depth,omitempty"` // 最大栈深度
	IncludeKernel  bool   `json:"include_kernel,omitempty"`  // 包含内核栈

	// 输出配置
	OutputFormat string `json:"output_format,omitempty"` // 输出格式: svg/json/text
}

// ProfileResponse 剖析响应
type ProfileResponse struct {
	SessionID     string        `json:"session_id,omitempty"`     // 会话ID
	Status        string        `json:"status,omitempty"`         // 状态
	Duration      int           `json:"duration,omitempty"`       // 实际剖析时长(秒)
	TotalSamples  uint64        `json:"total_samples,omitempty"`  // 总采样数
	SampleFreq    int           `json:"sample_freq,omitempty"`    // 实际采样频率
	FlameGraphSVG []byte        `json:"flame_graph_svg,omitempty"` // 火焰图SVG
	HotFunctions  []HotFunction `json:"hot_functions,omitempty"`  // 热点函数
	RawData       []byte        `json:"raw_data,omitempty"`       // 原始数据(JSON格式)
	Error         string        `json:"error,omitempty"`          // 错误信息
}

// ==================== 构造函数 ====================

// NewProfilerService 创建剖析服务
func NewProfilerService(cfg *ServiceConfig) *ProfilerService {
	if cfg == nil {
		cfg = &ServiceConfig{
			DefaultSampleFreq:     99,
			DefaultDuration:       30 * time.Second,
			MaxConcurrentSessions: 10,
			EnableAutoDiscovery:   true,
		}
	}

	return &ProfilerService{
		manager: NewProfilerManager(),
		config:  cfg,
		stopCh:  make(chan struct{}),
	}
}

// ==================== 服务生命周期 ====================

// Start 启动剖析服务
func (s *ProfilerService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("服务已在运行")
	}

	s.running = true
	s.stopCh = make(chan struct{})

	return nil
}

// Stop 停止剖析服务
func (s *ProfilerService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopCh)
	s.running = false

	// 关闭管理器
	return s.manager.Close()
}

// IsRunning 检查服务是否运行中
func (s *ProfilerService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// ==================== 剖析操作 ====================

// Profile 执行一次剖析
// 这是主要的入口函数，用于执行ON-CPU剖析
func (s *ProfilerService) Profile(ctx context.Context, req ProfileRequest) (*ProfileResponse, error) {
	// 设置默认值
	if req.Duration <= 0 {
		req.Duration = int(s.config.DefaultDuration.Seconds())
	}
	if req.SampleFreq <= 0 {
		req.SampleFreq = s.config.DefaultSampleFreq
	}

	// 构建筛选条件
	filter := Filter{
		PID:           req.PID,
		PIDs:          req.PIDs,
		ProcessName:   req.ProcessName,
		SampleFreq:    req.SampleFreq,
		MaxStackDepth: req.MaxStackDepth,
		IncludeKernel: req.IncludeKernel,
		IncludeUser:   true,
	}

	// 创建剖析会话
	duration := time.Duration(req.Duration) * time.Second
	sessionID, err := s.manager.StartSession(ctx, filter, duration)
	if err != nil {
		return nil, fmt.Errorf("创建剖析会话失败: %w", err)
	}

	// 等待剖析完成
	response := &ProfileResponse{
		SessionID: sessionID,
	}

	// 轮询检查会话状态
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.manager.StopSession(sessionID)
			response.Status = "cancelled"
			response.Error = "剖析被取消"
			return response, nil

		case <-ticker.C:
			session, err := s.manager.GetSession(sessionID)
			if err != nil {
				response.Status = "failed"
				response.Error = err.Error()
				return response, nil
			}

			if session.Status != "running" {
				response.Status = session.Status
				if session.Error != nil {
					response.Error = session.Error.Error()
				}
				if session.Result != nil {
					response.FlameGraphSVG = session.Result.FlameGraphSVG
					response.HotFunctions = session.Result.HotFunctions
					response.TotalSamples = session.Result.Stats.TotalSamples
					response.SampleFreq = session.Result.Stats.SampleFreq
					response.Duration = int(session.Result.Stats.Duration.Seconds())
				}
				return response, nil
			}
		}
	}
}

// ProfileAsync 异步执行剖析
// 返回会话ID，可通过GetResult获取结果
func (s *ProfilerService) ProfileAsync(ctx context.Context, req ProfileRequest) (string, error) {
	// 设置默认值
	if req.Duration <= 0 {
		req.Duration = int(s.config.DefaultDuration.Seconds())
	}
	if req.SampleFreq <= 0 {
		req.SampleFreq = s.config.DefaultSampleFreq
	}

	// 构建筛选条件
	filter := Filter{
		PID:           req.PID,
		PIDs:          req.PIDs,
		ProcessName:   req.ProcessName,
		SampleFreq:    req.SampleFreq,
		MaxStackDepth: req.MaxStackDepth,
		IncludeKernel: req.IncludeKernel,
		IncludeUser:   true,
	}

	duration := time.Duration(req.Duration) * time.Second
	return s.manager.StartSession(ctx, filter, duration)
}

// GetResult 获取剖析结果
func (s *ProfilerService) GetResult(sessionID string) (*ProfileResponse, error) {
	session, err := s.manager.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	response := &ProfileResponse{
		SessionID: sessionID,
		Status:    session.Status,
	}

	if session.Error != nil {
		response.Error = session.Error.Error()
	}

	if session.Result != nil {
		response.FlameGraphSVG = session.Result.FlameGraphSVG
		response.HotFunctions = session.Result.HotFunctions
		response.TotalSamples = session.Result.Stats.TotalSamples
		response.SampleFreq = session.Result.Stats.SampleFreq
		response.Duration = int(session.Result.Stats.Duration.Seconds())
	}

	return response, nil
}

// GetStatus 获取会话状态
func (s *ProfilerService) GetStatus(sessionID string) (map[string]interface{}, error) {
	return s.manager.GetSessionStatus(sessionID)
}

// GetLiveMetrics 获取实时指标
func (s *ProfilerService) GetLiveMetrics(sessionID string) (map[string]interface{}, error) {
	return s.manager.GetLiveMetrics(sessionID)
}

// StopSession 停止剖析会话
func (s *ProfilerService) StopSession(sessionID string) error {
	return s.manager.StopSession(sessionID)
}

// ListSessions 列出所有会话
func (s *ProfilerService) ListSessions() []*ProfileSession {
	return s.manager.ListSessions()
}

// ==================== 动态调整 ====================

// SetSampleFreq 动态调整采样频率
func (s *ProfilerService) SetSampleFreq(sessionID string, freq int) error {
	return s.manager.SetSampleFreq(sessionID, freq)
}

// AddTargetPID 添加目标进程
func (s *ProfilerService) AddTargetPID(sessionID string, pid uint32) error {
	return s.manager.AddTargetPID(sessionID, pid)
}

// RemoveTargetPID 移除目标进程
func (s *ProfilerService) RemoveTargetPID(sessionID string, pid uint32) error {
	return s.manager.RemoveTargetPID(sessionID, pid)
}

// ==================== 输出格式转换 ====================

// ToJSON 将剖析结果转换为JSON格式
func (r *ProfileResponse) ToJSON() ([]byte, error) {
	return json.Marshal(r)
}

// ToText 将剖析结果转换为文本格式
func (r *ProfileResponse) ToText() string {
	var result string
	result += fmt.Sprintf("剖析会话: %s\n", r.SessionID)
	result += fmt.Sprintf("状态: %s\n", r.Status)
	result += fmt.Sprintf("持续时间: %d秒\n", r.Duration)
	result += fmt.Sprintf("采样频率: %dHz\n", r.SampleFreq)
	result += fmt.Sprintf("总采样数: %d\n", r.TotalSamples)

	if len(r.HotFunctions) > 0 {
		result += "\n热点函数 Top 10:\n"
		for i, f := range r.HotFunctions {
			if i >= 10 {
				break
			}
			result += fmt.Sprintf("  %2d. %s (%d 采样, %.2f%%)\n",
				i+1, f.Name, f.Samples, f.Percentage)
		}
	}

	if r.Error != "" {
		result += fmt.Sprintf("\n错误: %s\n", r.Error)
	}

	return result
}

// ==================== 进程发现 ====================

// DiscoverProcesses 发现符合条件的进程
func (s *ProfilerService) DiscoverProcesses(namePattern string) []uint32 {
	return s.manager.discoverProcesses(namePattern)
}

// GetProcessInfo 获取进程信息
func (s *ProfilerService) GetProcessInfo(pid uint32) (map[string]interface{}, error) {
	// 通过符号解析器获取进程信息
	lang := s.manager.symbolizer.DetectLanguage(pid)
	goVersion := s.manager.symbolizer.GetGoVersion(pid)

	info := map[string]interface{}{
		"pid":      pid,
		"language": lang,
	}

	if goVersion != "" {
		info["go_version"] = goVersion
	}

	return info, nil
}

// ==================== 资源清理 ====================

// Close 关闭服务
func (s *ProfilerService) Close() error {
	return s.Stop()
}
