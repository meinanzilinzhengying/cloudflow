// Package profiler 提供 OFF-CPU 性能剖析功能
// 本文件提供 Agent 集成接口，将 OFF-CPU 剖析功能暴露给外部调用
package profiler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ==================== OFF-CPU 剖析服务 ====================

// OffCPUProfileRequest OFF-CPU 剖析请求
type OffCPUProfileRequest struct {
	// 目标选择
	PID         uint32   `json:"pid,omitempty"`          // 目标进程 ID
	PIDs        []uint32 `json:"pids,omitempty"`         // 多个目标进程
	ProcessName string   `json:"process_name,omitempty"` // 进程名匹配

	// 采集配置
	Duration      int  `json:"duration,omitempty"`        // 剖析时长(秒)
	MinDuration   int64 `json:"min_duration,omitempty"`   // 最小采集时长(微秒)
	MaxDuration   int64 `json:"max_duration,omitempty"`   // 最大采集时长(微秒)
	MaxStackDepth int  `json:"max_stack_depth,omitempty"` // 最大栈深度

	// 事件筛选
	CollectIOWait      bool `json:"collect_io_wait,omitempty"`      // 采集 IO 等待
	CollectLockContention bool `json:"collect_lock_contention,omitempty"` // 采集锁竞争
	CollectScheduler   bool `json:"collect_scheduler,omitempty"`   // 采集调度延迟
	CollectNetwork     bool `json:"collect_network,omitempty"`     // 采集网络等待
	CollectDisk        bool `json:"collect_disk,omitempty"`        // 采集磁盘等待
	CollectFutex       bool `json:"collect_futex,omitempty"`       // 采集 futex 等待
	CollectSleep       bool `json:"collect_sleep,omitempty"`       // 采集主动睡眠

	// 输出配置
	IncludeKernelStack bool `json:"include_kernel_stack,omitempty"` // 包含内核栈
	IncludeUserStack   bool `json:"include_user_stack,omitempty"`   // 包含用户栈
	GenerateFlameGraph bool `json:"generate_flame_graph,omitempty"` // 生成火焰图
	GenerateComparison bool `json:"generate_comparison,omitempty"`  // 生成 ON/OFF-CPU 对比
}

// OffCPUProfileResponse OFF-CPU 剖析响应
type OffCPUProfileResponse struct {
	SessionID     string           `json:"session_id,omitempty"`     // 会话 ID
	Status        string           `json:"status,omitempty"`         // 状态
	Duration      int              `json:"duration,omitempty"`       // 实际剖析时长(秒)
	TotalEvents   uint64           `json:"total_events,omitempty"`   // 总事件数
	TotalDuration int64            `json:"total_duration,omitempty"` // 总阻塞时长(微秒)

	// 阻塞原因统计
	ReasonStats map[OffCPUReason]*ReasonStat `json:"reason_stats,omitempty"`

	// 火焰图
	FlameGraphSVG []byte `json:"flame_graph_svg,omitempty"` // 火焰图 SVG

	// 热点阻塞点
	HotSpots []OffCPUHotSpot `json:"hot_spots,omitempty"`

	// ON/OFF-CPU 对比
	Comparison *CPUComparison `json:"comparison,omitempty"`

	// 原始数据
	RawData []byte `json:"raw_data,omitempty"` // 原始事件数据(JSON)

	Error string `json:"error,omitempty"` // 错误信息
}

// ReasonStat 阻塞原因统计
type ReasonStat struct {
	Count         uint64  `json:"count"`          // 事件数
	TotalDuration int64   `json:"total_duration"` // 总时长(微秒)
	AvgDuration   float64 `json:"avg_duration"`   // 平均时长(微秒)
	Percentage    float64 `json:"percentage"`     // 占比
}

// ==================== OFF-CPU 剖析服务 ====================

// OffCPUService OFF-CPU 剖析服务
type OffCPUService struct {
	mu        sync.RWMutex
	profilers map[string]*OffCPUProfiler // 会话 ID -> 剖析器
	sessions  map[string]*OffCPUSession  // 会话 ID -> 会话
	symbolizer *Symbolizer
	onCPUData map[string]map[string]uint64 // 会话 ID -> ON-CPU 栈计数
}

// OffCPUSession OFF-CPU 剖析会话
type OffCPUSession struct {
	ID           string
	Request      OffCPUProfileRequest
	StartTime    time.Time
	EndTime      time.Time
	Status       string // running/completed/failed
	Profiler     *OffCPUProfiler
	Error        error
	Result       *OffCPUProfileResponse
}

// NewOffCPUService 创建 OFF-CPU 剖析服务
func NewOffCPUService() *OffCPUService {
	return &OffCPUService{
		profilers:  make(map[string]*OffCPUProfiler),
		sessions:   make(map[string]*OffCPUSession),
		symbolizer: NewSymbolizer(),
		onCPUData:  make(map[string]map[string]uint64),
	}
}

// Profile 执行 OFF-CPU 剖析
func (s *OffCPUService) Profile(ctx context.Context, req OffCPUProfileRequest) (*OffCPUProfileResponse, error) {
	// 设置默认值
	if req.Duration <= 0 {
		req.Duration = 30 // 默认 30 秒
	}

	// 创建会话
	sessionID := generateSessionID()
	session := &OffCPUSession{
		ID:        sessionID,
		Request:   req,
		StartTime: time.Now(),
		Status:    "running",
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	// 构建配置
	cfg := &OffCPUConfig{
		PID:           req.PID,
		PIDs:          req.PIDs,
		ProcessName:   req.ProcessName,
		MinDuration:   req.MinDuration,
		MaxDuration:   req.MaxDuration,
		MaxStackDepth: req.MaxStackDepth,
		CollectIOWait:      req.CollectIOWait,
		CollectLockContention: req.CollectLockContention,
		CollectScheduler:   req.CollectScheduler,
		CollectNetwork:     req.CollectNetwork,
		CollectDisk:        req.CollectDisk,
		CollectFutex:       req.CollectFutex,
		CollectSleep:       req.CollectSleep,
		IncludeKernelStack: req.IncludeKernelStack,
		IncludeUserStack:   req.IncludeUserStack,
	}

	// 设置默认值
	if cfg.MinDuration == 0 {
		cfg.MinDuration = 1000 // 默认 1ms
	}
	if cfg.MaxStackDepth == 0 {
		cfg.MaxStackDepth = 127
	}

	// 创建剖析器
	profiler, err := NewOffCPUProfiler(cfg)
	if err != nil {
		session.Status = "failed"
		session.Error = err
		return nil, err
	}

	session.Profiler = profiler

	s.mu.Lock()
	s.profilers[sessionID] = profiler
	s.mu.Unlock()

	// 启动剖析
	if err := profiler.Start(); err != nil {
		session.Status = "failed"
		session.Error = err
		profiler.Close()
		return nil, err
	}

	// 等待剖析完成
	duration := time.Duration(req.Duration) * time.Second
	timer := time.NewTimer(duration)

	select {
	case <-ctx.Done():
		timer.Stop()
		profiler.Stop()
		session.Status = "cancelled"
	case <-timer.C:
		profiler.Stop()
		session.Status = "completed"
	}

	session.EndTime = time.Now()

	// 生成结果
	result := s.generateResult(session, req)
	session.Result = result

	return result, nil
}

// ProfileAsync 异步执行 OFF-CPU 剖析
func (s *OffCPUService) ProfileAsync(ctx context.Context, req OffCPUProfileRequest) (string, error) {
	// 设置默认值
	if req.Duration <= 0 {
		req.Duration = 30
	}

	// 创建会话
	sessionID := generateSessionID()
	session := &OffCPUSession{
		ID:        sessionID,
		Request:   req,
		StartTime: time.Now(),
		Status:    "running",
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	// 构建配置并创建剖析器
	cfg := DefaultOffCPUConfig()
	cfg.PID = req.PID
	cfg.PIDs = req.PIDs
	cfg.ProcessName = req.ProcessName
	cfg.MinDuration = req.MinDuration
	cfg.MaxDuration = req.MaxDuration
	cfg.MaxStackDepth = req.MaxStackDepth

	profiler, err := NewOffCPUProfiler(cfg)
	if err != nil {
		session.Status = "failed"
		session.Error = err
		return "", err
	}

	session.Profiler = profiler

	s.mu.Lock()
	s.profilers[sessionID] = profiler
	s.mu.Unlock()

	// 启动剖析
	if err := profiler.Start(); err != nil {
		session.Status = "failed"
		session.Error = err
		profiler.Close()
		return "", err
	}

	// 在后台运行
	go func() {
		duration := time.Duration(req.Duration) * time.Second
		timer := time.NewTimer(duration)

		select {
		case <-ctx.Done():
			timer.Stop()
			profiler.Stop()
			session.Status = "cancelled"
		case <-timer.C:
			profiler.Stop()
			session.Status = "completed"
		}

		session.EndTime = time.Now()

		// 生成结果
		result := s.generateResult(session, req)
		session.Result = result
	}()

	return sessionID, nil
}

// generateResult 生成剖析结果
func (s *OffCPUService) generateResult(session *OffCPUSession, req OffCPUProfileRequest) *OffCPUProfileResponse {
	profiler := session.Profiler
	if profiler == nil {
		return &OffCPUProfileResponse{
			SessionID: session.ID,
			Status:    "failed",
			Error:     "剖析器未初始化",
		}
	}

	events := profiler.GetEvents()
	stats := profiler.GetStats()

	result := &OffCPUProfileResponse{
		SessionID:     session.ID,
		Status:        session.Status,
		Duration:      int(session.EndTime.Sub(session.StartTime).Seconds()),
		TotalEvents:   stats.TotalEvents,
		TotalDuration: stats.TotalDuration,
		ReasonStats:   make(map[OffCPUReason]*ReasonStat),
	}

	// 填充阻塞原因统计
	for reason, count := range stats.ReasonCounts {
		duration := stats.ReasonDurations[reason]
		percentage := float64(0)
		if stats.TotalDuration > 0 {
			percentage = float64(duration) * 100.0 / float64(stats.TotalDuration)
		}

		avgDuration := float64(0)
		if count > 0 {
			avgDuration = float64(duration) / float64(count)
		}

		result.ReasonStats[reason] = &ReasonStat{
			Count:         count,
			TotalDuration: duration,
			AvgDuration:   avgDuration,
			Percentage:    percentage,
		}
	}

	// 生成火焰图
	if req.GenerateFlameGraph && len(events) > 0 {
		fg := NewOffCPUFlameGraph()
		var buf []byte
		writer := &byteBuffer{buf: &buf}
		if err := fg.Generate(events, writer); err == nil {
			result.FlameGraphSVG = buf
		}

		// 生成热点阻塞点
		result.HotSpots = fg.GenerateHotSpots(events, 20)
	}

	// 生成 ON/OFF-CPU 对比
	if req.GenerateComparison {
		// 获取 ON-CPU 数据（如果有）
		s.mu.RLock()
		onCPUStacks := s.onCPUData[session.ID]
		s.mu.RUnlock()

		if onCPUStacks != nil {
			comparator := NewCPUComparator(nil)
			comparison, err := comparator.Compare(onCPUStacks, events, session.EndTime.Sub(session.StartTime))
			if err == nil {
				result.Comparison = comparison
			}
		}
	}

	// 导出原始数据
	if len(events) > 0 {
		if rawData, err := json.Marshal(events); err == nil {
			result.RawData = rawData
		}
	}

	return result
}

// byteBuffer 字节缓冲区
type byteBuffer struct {
	buf *[]byte
}

func (b *byteBuffer) Write(p []byte) (n int, err error) {
	*b.buf = append(*b.buf, p...)
	return len(p), nil
}

// GetResult 获取剖析结果
func (s *OffCPUService) GetResult(sessionID string) (*OffCPUProfileResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	return session.Result, nil
}

// GetSessionStatus 获取会话状态
func (s *OffCPUService) GetSessionStatus(sessionID string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	status := map[string]interface{}{
		"session_id": session.ID,
		"status":     session.Status,
		"start_time": session.StartTime,
	}

	if !session.EndTime.IsZero() {
		status["end_time"] = session.EndTime
		status["duration"] = session.EndTime.Sub(session.StartTime).Seconds()
	}

	if session.Error != nil {
		status["error"] = session.Error.Error()
	}

	if session.Result != nil {
		status["total_events"] = session.Result.TotalEvents
		status["total_duration_ms"] = session.Result.TotalDuration / 1000
	}

	return status, nil
}

// StopSession 停止剖析会话
func (s *OffCPUService) StopSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Status == "running" && session.Profiler != nil {
		session.Profiler.Stop()
		session.Status = "stopped"
		session.EndTime = time.Now()
	}

	return nil
}

// ListSessions 列出所有会话
func (s *OffCPUService) ListSessions() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []map[string]interface{}
	for _, session := range s.sessions {
		status := map[string]interface{}{
			"session_id": session.ID,
			"status":     session.Status,
			"start_time": session.StartTime,
		}
		if session.Result != nil {
			status["total_events"] = session.Result.TotalEvents
		}
		result = append(result, status)
	}
	return result
}

// DeleteSession 删除会话
func (s *OffCPUService) DeleteSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if ok && session.Profiler != nil {
		session.Profiler.Close()
	}

	delete(s.sessions, sessionID)
	delete(s.profilers, sessionID)
	delete(s.onCPUData, sessionID)

	return nil
}

// SetOnCPUData 设置 ON-CPU 数据（用于对比分析）
func (s *OffCPUService) SetOnCPUData(sessionID string, stacks map[string]uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.onCPUData[sessionID] = stacks
}

// GetEventsByReason 按原因获取事件
func (s *OffCPUService) GetEventsByReason(sessionID string, reason OffCPUReason) ([]*OffCPUEvent, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Profiler == nil {
		return nil, fmt.Errorf("剖析器未初始化")
	}

	return session.Profiler.GetEventsByReason(reason), nil
}

// GetEventsByPID 按 PID 获取事件
func (s *OffCPUService) GetEventsByPID(sessionID string, pid uint32) ([]*OffCPUEvent, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Profiler == nil {
		return nil, fmt.Errorf("剖析器未初始化")
	}

	return session.Profiler.GetEventsByPID(pid), nil
}

// GenerateFlameGraphByReason 按原因生成火焰图
func (s *OffCPUService) GenerateFlameGraphByReason(sessionID string, reason OffCPUReason) ([]byte, error) {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Profiler == nil {
		return nil, fmt.Errorf("剖析器未初始化")
	}

	events := session.Profiler.GetEventsByReason(reason)
	if len(events) == 0 {
		return nil, fmt.Errorf("没有 %s 类型的阻塞事件", reason)
	}

	fg := NewOffCPUFlameGraph()
	var buf []byte
	writer := &byteBuffer{buf: &buf}
	if err := fg.GenerateByReason(events, reason, writer); err != nil {
		return nil, err
	}

	return buf, nil
}

// Close 关闭服务
func (s *OffCPUService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 停止所有剖析器
	for _, session := range s.sessions {
		if session.Profiler != nil {
			session.Profiler.Close()
		}
	}

	s.sessions = make(map[string]*OffCPUSession)
	s.profilers = make(map[string]*OffCPUProfiler)
	s.symbolizer.ClearCache()

	return nil
}
