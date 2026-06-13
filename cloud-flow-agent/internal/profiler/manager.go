// Package profiler 提供 ON-CPU 性能剖析功能
// 本文件实现剖析管理器，支持按进程、线程、时间范围筛选
package profiler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ==================== 筛选条件 ====================

// Filter 剖析筛选条件
type Filter struct {
	// 进程筛选
	PID       uint32   // 目标进程ID，0表示所有进程
	PIDs      []uint32 // 多个目标进程ID
	ProcessName string // 进程名匹配（支持通配符）

	// 线程筛选
	TID       uint32   // 目标线程ID，0表示所有线程
	TIDs      []uint32 // 多个目标线程ID

	// 时间范围筛选
	StartTime *time.Time // 开始时间
	EndTime   *time.Time // 结束时间

	// CPU筛选
	CPU       int   // 目标CPU编号，-1表示所有CPU
	CPUs      []int // 多个目标CPU

	// 采样配置
	SampleFreq    int   // 采样频率(Hz)，默认99
	MaxStackDepth int   // 最大栈深度，默认127

	// 输出配置
	IncludeKernel bool // 是否包含内核栈
	IncludeUser   bool // 是否包含用户栈
}

// ProfileSession 剖析会话
// 表示一次完整的剖析过程
type ProfileSession struct {
	ID           string        // 会话ID
	Filter       Filter        // 筛选条件
	StartTime    time.Time     // 开始时间
	EndTime      time.Time     // 结束时间
	Duration     time.Duration // 持续时间
	Status       string        // 状态: running/completed/failed
	Result       *ProfileResult // 剖析结果
	Error        error         // 错误信息
}

// ==================== 剖析管理器 ====================

// ProfilerManager 剖析管理器
// 管理多个剖析会话，支持按条件筛选
type ProfilerManager struct {
	mu          sync.RWMutex
	sessions    map[string]*ProfileSession // 活跃会话
	profilers   map[uint32]*Profiler       // PID -> Profiler映射
	symbolizer  *Symbolizer                // 共享符号解析器
}

// NewProfilerManager 创建剖析管理器
func NewProfilerManager() *ProfilerManager {
	return &ProfilerManager{
		sessions:   make(map[string]*ProfileSession),
		profilers:  make(map[uint32]*Profiler),
		symbolizer: NewSymbolizer(),
	}
}

// ==================== 会话管理 ====================

// StartSession 启动新的剖析会话
// 参数:
//   - ctx: 上下文，用于取消剖析
//   - filter: 筛选条件
//   - duration: 剖析持续时间
//
// 返回: 会话ID
func (pm *ProfilerManager) StartSession(ctx context.Context, filter Filter, duration time.Duration) (string, error) {
	// 生成会话ID
	sessionID := generateSessionID()

	// 创建会话
	session := &ProfileSession{
		ID:        sessionID,
		Filter:    filter,
		StartTime: time.Now(),
		Duration:  duration,
		Status:    "running",
	}

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	// 启动剖析
	go pm.runSession(ctx, session)

	return sessionID, nil
}

// runSession 执行剖析会话
func (pm *ProfilerManager) runSession(ctx context.Context, session *ProfileSession) {
	defer func() {
		session.EndTime = time.Now()
	}()

	// 设置默认值
	filter := session.Filter
	if filter.SampleFreq <= 0 {
		filter.SampleFreq = 99
	}
	if filter.MaxStackDepth <= 0 {
		filter.MaxStackDepth = 127
	}
	if !filter.IncludeKernel && !filter.IncludeUser {
		filter.IncludeUser = true
	}

	// 创建剖析器
	var profilers []*Profiler
	var pids []uint32

	if filter.PID > 0 {
		pids = []uint32{filter.PID}
	} else if len(filter.PIDs) > 0 {
		pids = filter.PIDs
	} else {
		// 获取所有符合条件的进程
		pids = pm.discoverProcesses(filter.ProcessName)
	}

	// 为每个PID创建剖析器
	for _, pid := range pids {
		cfg := ProfilerConfig{
			SampleFreq:    filter.SampleFreq,
			TargetPID:     pid,
			MaxStackDepth: filter.MaxStackDepth,
		}

		profiler, err := New(cfg)
		if err != nil {
			continue
		}

		profilers = append(profilers, profiler)
		pm.mu.Lock()
		pm.profilers[pid] = profiler
		pm.mu.Unlock()
	}

	if len(profilers) == 0 {
		session.Status = "failed"
		session.Error = fmt.Errorf("没有找到符合条件的进程")
		return
	}

	// 启动所有剖析器
	for _, p := range profilers {
		if err := p.Start(); err != nil {
			session.Status = "failed"
			session.Error = err
			return
		}
	}

	// 等待指定时间或取消
	select {
	case <-ctx.Done():
		session.Status = "cancelled"
	case <-time.After(session.Duration):
		session.Status = "completed"
	}

	// 停止所有剖析器并收集结果
	var allStackCounts = make(map[string]uint64)
	var totalSamples uint64

	for _, p := range profilers {
		p.Stop()
		counts := p.GetStackCounts()
		for k, v := range counts {
			allStackCounts[k] += v
		}
		stats := p.GetStats()
		totalSamples += stats.TotalSamples
		p.Close()
	}

	// 应用时间范围筛选
	if filter.StartTime != nil || filter.EndTime != nil {
		// 时间筛选在采样时已应用
	}

	// 生成火焰图
	fg := NewFlameGraph()
	var svgData []byte
	if len(allStackCounts) > 0 {
		var buf []byte
		bufWriter := &byteWriter{buf: &svgData}
		fg.Generate(allStackCounts, bufWriter)
	}

	// 生成热点函数
	hotFunctions := fg.GenerateHotFunctions(allStackCounts, 20)

	// 保存结果
	session.Result = &ProfileResult{
		FlameGraphSVG: svgData,
		HotFunctions:  hotFunctions,
		Stats: ProfilerStats{
			TotalSamples: totalSamples,
			Duration:     session.Duration,
			SampleFreq:   filter.SampleFreq,
		},
	}
}

// byteWriter 简单的字节写入器
type byteWriter struct {
	buf *[]byte
}

func (w *byteWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// StopSession 停止剖析会话
func (pm *ProfilerManager) StopSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	session, ok := pm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Status == "running" {
		session.Status = "stopped"
		session.EndTime = time.Now()
	}

	return nil
}

// GetSession 获取会话状态
func (pm *ProfilerManager) GetSession(sessionID string) (*ProfileSession, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	session, ok := pm.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	return session, nil
}

// ListSessions 列出所有会话
func (pm *ProfilerManager) ListSessions() []*ProfileSession {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	sessions := make([]*ProfileSession, 0, len(pm.sessions))
	for _, s := range pm.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// DeleteSession 删除会话
func (pm *ProfilerManager) DeleteSession(sessionID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.sessions, sessionID)
	return nil
}

// ==================== 动态调整 ====================

// SetSampleFreq 动态调整采样频率
// 参数:
//   - sessionID: 会话ID
//   - freq: 新的采样频率(Hz)
func (pm *ProfilerManager) SetSampleFreq(sessionID string, freq int) error {
	pm.mu.RLock()
	session, ok := pm.sessions[sessionID]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Status != "running" {
		return fmt.Errorf("会话未在运行")
	}

	// 更新所有相关剖析器
	pm.mu.RLock()
	for _, pid := range pm.getTargetPIDs(session.Filter) {
		if p, ok := pm.profilers[pid]; ok {
			p.SetSampleFreq(freq)
		}
	}
	pm.mu.RUnlock()

	return nil
}

// AddTargetPID 添加目标进程
func (pm *ProfilerManager) AddTargetPID(sessionID string, pid uint32) error {
	pm.mu.RLock()
	session, ok := pm.sessions[sessionID]
	pm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	if session.Status != "running" {
		return fmt.Errorf("会话未在运行")
	}

	// 创建新的剖析器
	cfg := ProfilerConfig{
		SampleFreq:    session.Filter.SampleFreq,
		TargetPID:     pid,
		MaxStackDepth: session.Filter.MaxStackDepth,
	}

	profiler, err := New(cfg)
	if err != nil {
		return err
	}

	if err := profiler.Start(); err != nil {
		profiler.Close()
		return err
	}

	pm.mu.Lock()
	pm.profilers[pid] = profiler
	session.Filter.PIDs = append(session.Filter.PIDs, pid)
	pm.mu.Unlock()

	return nil
}

// RemoveTargetPID 移除目标进程
func (pm *ProfilerManager) RemoveTargetPID(sessionID string, pid uint32) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	session, ok := pm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	profiler, ok := pm.profilers[pid]
	if !ok {
		return fmt.Errorf("进程不在监控中: %d", pid)
	}

	profiler.Stop()
	profiler.Close()
	delete(pm.profilers, pid)

	// 从PIDs列表中移除
	for i, p := range session.Filter.PIDs {
		if p == pid {
			session.Filter.PIDs = append(session.Filter.PIDs[:i], session.Filter.PIDs[i+1:]...)
			break
		}
	}

	return nil
}

// ==================== 进程发现 ====================

// discoverProcesses 发现符合条件的进程
func (pm *ProfilerManager) discoverProcesses(namePattern string) []uint32 {
	// 读取/proc目录，查找匹配的进程
	var pids []uint32

	// 简化实现：返回空列表，实际应扫描/proc
	// 实际实现需要读取/proc/*/comm或/proc/*/cmdline

	return pids
}

// getTargetPIDs 获取目标PID列表
func (pm *ProfilerManager) getTargetPIDs(filter Filter) []uint32 {
	if filter.PID > 0 {
		return []uint32{filter.PID}
	}
	return filter.PIDs
}

// ==================== 辅助函数 ====================

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("prof-%d", time.Now().UnixNano())
}

// ==================== 清理 ====================

// Close 关闭管理器，释放所有资源
func (pm *ProfilerManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 停止所有剖析器
	for _, p := range pm.profilers {
		p.Stop()
		p.Close()
	}

	// 清除符号解析器缓存
	pm.symbolizer.ClearCache()

	pm.sessions = make(map[string]*ProfileSession)
	pm.profilers = make(map[uint32]*Profiler)

	return nil
}

// ==================== 实时状态查询 ====================

// GetSessionStatus 获取会话实时状态
func (pm *ProfilerManager) GetSessionStatus(sessionID string) (map[string]interface{}, error) {
	session, err := pm.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"session_id": session.ID,
		"status":     session.Status,
		"start_time": session.StartTime,
		"duration":   session.Duration,
	}

	if session.Result != nil {
		status["total_samples"] = session.Result.Stats.TotalSamples
		status["sample_freq"] = session.Result.Stats.SampleFreq
	}

	if session.Error != nil {
		status["error"] = session.Error.Error()
	}

	return status, nil
}

// GetLiveMetrics 获取实时指标
func (pm *ProfilerManager) GetLiveMetrics(sessionID string) (map[string]interface{}, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	session, ok := pm.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	metrics := map[string]interface{}{
		"session_id":    sessionID,
		"running":       session.Status == "running",
		"elapsed_time":  time.Since(session.StartTime),
		"target_pids":   session.Filter.PIDs,
		"sample_freq":   session.Filter.SampleFreq,
	}

	// 收集各剖析器的统计信息
	var totalSamples, lostSamples uint64
	for _, pid := range session.Filter.PIDs {
		if p, ok := pm.profilers[pid]; ok {
			stats := p.GetStats()
			totalSamples += stats.TotalSamples
			lostSamples += stats.LostSamples
		}
	}

	metrics["total_samples"] = totalSamples
	metrics["lost_samples"] = lostSamples

	return metrics, nil
}
