// Package lifecycle 提供监控数据生命周期管理
// 本文件实现定时调度器与查询隔离机制
// 支持：每天凌晨自动清理、并发控制、清理限速、查询优先级保障
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ==================== 清理执行器类型 ====================

// CleanupExecutor 清理执行器函数类型
type CleanupExecutor func(ctx context.Context, category DataCategory) (*CleanupTask, error)

// ==================== 定时调度器 ====================

// CleanupScheduler 清理调度器
type CleanupScheduler struct {
	config    *LifecycleConfig
	policies  *RetentionPolicyManager

	// 执行器
	executor CleanupExecutor

	// 调度状态
	running      bool
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	lastCleanup  time.Time
	nextCleanup  time.Time

	// 并发控制
	semaphore    chan struct{}
	activeTasks  int
	taskMu       sync.Mutex

	// 限速器
	throttler    *CleanupThrottler

	// 查询隔离
	queryGuard   *QueryIsolationGuard
}

// NewCleanupScheduler 创建清理调度器
func NewCleanupScheduler(cfg *LifecycleConfig) *CleanupScheduler {
	scheduler := &CleanupScheduler{
		config:    cfg,
		stopCh:    make(chan struct{}),
		semaphore: make(chan struct{}, cfg.MaxConcurrentCleanups),
		throttler: NewCleanupThrottler(cfg),
		queryGuard: NewQueryIsolationGuard(cfg),
	}

	return scheduler
}

// SetPolicies 设置策略管理器
func (s *CleanupScheduler) SetPolicies(policies *RetentionPolicyManager) {
	s.policies = policies
}

// SetExecutor 设置清理执行器
func (s *CleanupScheduler) SetExecutor(executor CleanupExecutor) {
	s.executor = executor
}

// Start 启动调度器
func (s *CleanupScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("调度器已在运行")
	}
	s.running = true
	s.mu.Unlock()

	// 计算下次清理时间
	s.calculateNextCleanup()

	// 启动调度循环
	s.wg.Add(1)
	go s.scheduleLoop(ctx)

	// 启动限速监控
	s.wg.Add(1)
	go s.throttler.Monitor(ctx)

	return nil
}

// Stop 停止调度器
func (s *CleanupScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	s.wg.Wait()
}

// scheduleLoop 调度循环
func (s *CleanupScheduler) scheduleLoop(ctx context.Context) {
	defer s.wg.Done()

	// 计算到下次清理的等待时间
	waitDuration := time.Until(s.nextCleanup)
	if waitDuration < 0 {
		waitDuration = time.Minute // 至少等待1分钟
	}

	timer := time.NewTimer(waitDuration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-timer.C:
			// 执行清理
			s.runScheduledCleanup(ctx)

			// 计算下次清理时间
			s.calculateNextCleanup()
			timer.Reset(time.Until(s.nextCleanup))
		}
	}
}

// calculateNextCleanup 计算下次清理时间
func (s *CleanupScheduler) calculateNextCleanup() {
	now := time.Now()

	// 解析清理时间（HH:MM 格式）
	hour, minute := 2, 0 // 默认凌晨2点
	if s.config.CleanupTime != "" {
		n, _ := fmt.Sscanf(s.config.CleanupTime, "%d:%d", &hour, &minute)
		if n != 2 {
			hour, minute = 2, 0
		}
	}

	// 构造下次清理时间
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

	// 如果已过今天的清理时间，安排到明天
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}

	s.mu.Lock()
	s.nextCleanup = next
	s.mu.Unlock()
}

// runScheduledCleanup 执行定时清理
func (s *CleanupScheduler) runScheduledCleanup(ctx context.Context) {
	if s.executor == nil {
		return
	}

	// 获取需要清理的分类（按优先级排序）
	policies := s.policies.GetEnabledPolicies()
	if len(policies) == 0 {
		return
	}

	// 逐个分类执行清理（受并发控制）
	for _, policy := range policies {
		// 检查是否应该停止
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		// 获取信号量（并发控制）
		select {
		case s.semaphore <- struct{}{}:
			// 获取成功
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		}

		// 在 goroutine 中执行清理
		s.wg.Add(1)
		go func(cat DataCategory) {
			defer s.wg.Done()
			defer func() { <-s.semaphore }()

			s.executeWithIsolation(ctx, cat)
		}(policy.Category)
	}

	// 等待所有清理完成
	s.wg.Wait()

	// 更新最后清理时间
	s.mu.Lock()
	s.lastCleanup = time.Now()
	s.mu.Unlock()
}

// executeWithIsolation 在查询隔离模式下执行清理
func (s *CleanupScheduler) executeWithIsolation(ctx context.Context, category DataCategory) {
	// 检查是否启用查询隔离
	if s.config.QueryIsolationEnabled {
		// 等待查询低谷期
		s.queryGuard.WaitForLowLoad(ctx)

		// 设置清理标记（查询可感知清理状态）
		s.queryGuard.MarkCleanupStart(category)
		defer s.queryGuard.MarkCleanupEnd(category)
	}

	// 应用限速
	cleanupCtx := s.throttler.WrapContext(ctx)

	// 执行清理
	timeout := time.Duration(s.config.CleanupTimeout) * time.Minute
	taskCtx, cancel := context.WithTimeout(cleanupCtx, timeout)
	defer cancel()

	_, err := s.executor(taskCtx, category)
	if err != nil {
		// 记录错误但不中断其他清理
		_ = err
	}
}

// TriggerManualCleanup 手动触发清理
func (s *CleanupScheduler) TriggerManualCleanup(ctx context.Context, categories ...DataCategory) error {
	if s.executor == nil {
		return fmt.Errorf("清理执行器未设置")
	}

	if len(categories) == 0 {
		policies := s.policies.GetEnabledPolicies()
		for _, p := range policies {
			categories = append(categories, p.Category)
		}
	}

	for _, cat := range categories {
		select {
		case s.semaphore <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		s.wg.Add(1)
		go func(c DataCategory) {
			defer s.wg.Done()
			defer func() { <-s.semaphore }()
			s.executeWithIsolation(ctx, c)
		}(cat)
	}

	return nil
}

// GetNextCleanupTime 获取下次清理时间
func (s *CleanupScheduler) GetNextCleanupTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextCleanup
}

// GetLastCleanupTime 获取上次清理时间
func (s *CleanupScheduler) GetLastCleanupTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCleanup
}

// GetActiveTaskCount 获取当前活跃清理任务数
func (s *CleanupScheduler) GetActiveTaskCount() int {
	s.taskMu.Lock()
	defer s.taskMu.Unlock()
	return s.activeTasks
}

// ==================== 清理限速器 ====================

// CleanupThrottler 清理限速器
// 控制清理速度，避免影响正常查询性能
type CleanupThrottler struct {
	config      *LifecycleConfig
	rateLimiter *TokenBucketRateLimiter
	mu          sync.RWMutex
	paused      bool
}

// NewCleanupThrottler 创建清理限速器
func NewCleanupThrottler(cfg *LifecycleConfig) *CleanupThrottler {
	throttler := &CleanupThrottler{
		config: cfg,
	}

	// 如果配置了最大删除速率，创建令牌桶限速器
	if cfg.MaxDeleteRate > 0 {
		throttler.rateLimiter = NewTokenBucketRateLimiter(
			int64(cfg.MaxDeleteRate),
			int64(cfg.MaxDeleteRate),
		)
	}

	return throttler
}

// WrapContext 包装上下文（支持暂停和限速）
func (t *CleanupThrottler) WrapContext(ctx context.Context) context.Context {
	return &cleanupContext{
		Context:  ctx,
		throttler: t,
	}
}

// Wait 等待令牌（限速）
func (t *CleanupThrottler) Wait(ctx context.Context) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.paused {
		return false
	}

	if t.rateLimiter != nil {
		return t.rateLimiter.Wait(ctx)
	}

	return true
}

// Pause 暂停清理
func (t *CleanupThrottler) Pause() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.paused = true
}

// Resume 恢复清理
func (t *CleanupThrottler) Resume() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.paused = false
}

// IsPaused 检查是否暂停
func (t *CleanupThrottler) IsPaused() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.paused
}

// SetRate 动态调整限速
func (t *CleanupThrottler) SetRate(rate int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if rate <= 0 {
		t.rateLimiter = nil
	} else {
		t.rateLimiter = NewTokenBucketRateLimiter(rate, rate)
	}
}

// Monitor 监控限速状态
func (t *CleanupThrottler) Monitor(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 可在此处添加自适应限速逻辑
			// 例如：根据系统负载动态调整清理速率
			_ = t.config.CleanupThrottleRate
		}
	}
}

// cleanupContext 清理专用上下文
type cleanupContext struct {
	context.Context
	throttler *CleanupThrottler
}

// ==================== 令牌桶限速器 ====================

// TokenBucketRateLimiter 令牌桶限速器
type TokenBucketRateLimiter struct {
	rate       int64         // 每秒产生的令牌数
	burst      int64         // 桶容量
	tokens     int64         // 当前令牌数
	lastTime   time.Time     // 上次填充时间
	mu         sync.Mutex
}

// NewTokenBucketRateLimiter 创建令牌桶限速器
func NewTokenBucketRateLimiter(rate, burst int64) *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		rate:     rate,
		burst:    burst,
		tokens:   burst,
		lastTime: time.Now(),
	}
}

// Wait 等待获取令牌
func (tb *TokenBucketRateLimiter) Wait(ctx context.Context) bool {
	for {
		tb.mu.Lock()
		tb.refill()
		if tb.tokens > 0 {
			tb.tokens--
			tb.mu.Unlock()
			return true
		}
		waitDuration := time.Second / time.Duration(tb.rate)
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return false
		case <-time.After(waitDuration):
			// 继续尝试
		}
	}
}

// refill 填充令牌
func (tb *TokenBucketRateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastTime)
	if elapsed <= 0 {
		return
	}

	// 计算新增令牌数
	newTokens := int64(elapsed.Seconds()) * tb.rate / 1e9
	if newTokens > 0 {
		tb.tokens += newTokens
		if tb.tokens > tb.burst {
			tb.tokens = tb.burst
		}
		tb.lastTime = now
	}
}

// ==================== 查询隔离守卫 ====================

// QueryIsolationGuard 查询隔离守卫
// 确保清理操作不影响正常查询性能
type QueryIsolationGuard struct {
	config         *LifecycleConfig
	activeCleanups map[DataCategory]bool
	queryCount     int64
	mu             sync.RWMutex

	// 负载感知
	highLoadThreshold int64 // 高负载阈值（并发查询数）
	lowLoadThreshold  int64 // 低负载阈值
}

// NewQueryIsolationGuard 创建查询隔离守卫
func NewQueryIsolationGuard(cfg *LifecycleConfig) *QueryIsolationGuard {
	return &QueryIsolationGuard{
		config:           cfg,
		activeCleanups:   make(map[DataCategory]bool),
		highLoadThreshold: 10, // 超过10个并发查询视为高负载
		lowLoadThreshold:  3,  // 低于3个并发查询视为低负载
	}
}

// MarkQueryStart 标记查询开始
func (g *QueryIsolationGuard) MarkQueryStart() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.queryCount++
}

// MarkQueryEnd 标记查询结束
func (g *QueryIsolationGuard) MarkQueryEnd() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.queryCount > 0 {
		g.queryCount--
	}
}

// GetQueryCount 获取当前查询数
func (g *QueryIsolationGuard) GetQueryCount() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.queryCount
}

// IsHighLoad 检查是否处于高负载
func (g *QueryIsolationGuard) IsHighLoad() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.queryCount >= g.highLoadThreshold
}

// IsLowLoad 检查是否处于低负载
func (g *QueryIsolationGuard) IsLowLoad() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.queryCount <= g.lowLoadThreshold
}

// MarkCleanupStart 标记清理开始
func (g *QueryIsolationGuard) MarkCleanupStart(category DataCategory) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.activeCleanups[category] = true
}

// MarkCleanupEnd 标记清理结束
func (g *QueryIsolationGuard) MarkCleanupEnd(category DataCategory) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.activeCleanups, category)
}

// IsCleanupActive 检查指定分类是否有活跃清理
func (g *QueryIsolationGuard) IsCleanupActive(category DataCategory) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.activeCleanups[category]
}

// GetActiveCleanups 获取所有活跃清理
func (g *QueryIsolationGuard) GetActiveCleanups() []DataCategory {
	g.mu.RLock()
	defer g.mu.RUnlock()

	categories := make([]DataCategory, 0)
	for cat := range g.activeCleanups {
		categories = append(categories, cat)
	}
	return categories
}

// WaitForLowLoad 等待查询低谷期
// 在低负载时执行清理，避免影响正常查询
func (g *QueryIsolationGuard) WaitForLowLoad(ctx context.Context) {
	if !g.config.QueryIsolationEnabled {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	maxWait := 30 * time.Minute // 最多等待30分钟
	deadline := time.Now().Add(maxWait)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if g.IsLowLoad() {
				return
			}
			// 超过最大等待时间，强制执行
			if time.Now().After(deadline) {
				return
			}
		}
	}
}

// ShouldThrottle 检查是否应该限速清理
// 当查询负载较高时，降低清理速度
func (g *QueryIsolationGuard) ShouldThrottle() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.config.QueryIsolationEnabled {
		return false
	}

	// 高负载时限速
	return g.queryCount >= g.highLoadThreshold
}

// GetThrottleFactor 获取限速因子
// 返回 0-1 之间的值，0 表示完全暂停，1 表示全速
func (g *QueryIsolationGuard) GetThrottleFactor() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.config.QueryIsolationEnabled {
		return 1.0
	}

	// 根据查询负载计算限速因子
	if g.queryCount == 0 {
		return 1.0
	}

	// 线性递减：查询越多，清理越慢
	factor := 1.0 - float64(g.queryCount)/float64(g.highLoadThreshold*2)
	if factor < g.config.CleanupThrottleRate {
		factor = g.config.CleanupThrottleRate
	}
	if factor < 0 {
		factor = 0
	}

	return factor
}

// ==================== 紧急清理触发器 ====================

// EmergencyCleanupTrigger 紧急清理触发器
// 当磁盘使用率超过阈值时触发紧急清理
type EmergencyCleanupTrigger struct {
	config    *LifecycleConfig
	scheduler *CleanupScheduler
	mu        sync.RWMutex
	cooldown  time.Time // 冷却时间
}

// NewEmergencyCleanupTrigger 创建紧急清理触发器
func NewEmergencyCleanupTrigger(cfg *LifecycleConfig, scheduler *CleanupScheduler) *EmergencyCleanupTrigger {
	return &EmergencyCleanupTrigger{
		config:    cfg,
		scheduler: scheduler,
	}
}

// Check 检查是否需要触发紧急清理
func (t *EmergencyCleanupTrigger) Check(diskUsage float64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 检查冷却期
	if time.Now().Before(t.cooldown) {
		return false
	}

	// 检查磁盘使用率
	if diskUsage >= t.config.MaxDiskUsageForCleanup {
		// 设置冷却期（1小时内不重复触发）
		t.cooldown = time.Now().Add(1 * time.Hour)
		return true
	}

	return false
}

// TriggerEmergency 触发紧急清理
func (t *EmergencyCleanupTrigger) TriggerEmergency(ctx context.Context) error {
	// 紧急清理时降低保留天数（保留最近7天）
	// 按优先级从低到高清理
	categories := []DataCategory{
		CategoryTrace,
		CategorySelfMonitor,
		CategoryProfiling,
		CategoryLog,
		CategoryTopology,
		CategoryDBMetric,
		CategorySQLAggregate,
		CategoryAppMetric,
		CategorySystemMetric,
		CategoryEvent,
		CategoryAlert,
	}

	return t.scheduler.TriggerManualCleanup(ctx, categories...)
}
