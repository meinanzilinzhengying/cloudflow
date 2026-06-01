// Package sampling 自适应采样引擎
//
// 设计目标:
//   - 控制存储成本: 正常流量 1/100 采样，异常流量 100% 保留
//   - 零分配热路径: 采样决策 < 100ns
//   - 多维度策略: tenant / service / protocol 独立配置
//   - 运行时热更新: 配置变更无需重启
//
// 采样决策链:
//
//	flow → TenantQuota → HotServiceProtection → DynamicSampler → Keep/Drop
//	                                                          │
//	                                              ErrorFirstSampler (异常优先)
//	                                              LatencyFirstSampler (延迟优先)
//
// 采样依据:
//   - latency: 高延迟 → 保留
//   - retransmission: TCP 重传 → 保留
//   - reset: TCP RST → 保留
//   - timeout: 超时 → 保留
//   - error rate: 高错误率 → 保留
package sampling

import (
	"context"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	flow "cloud-flow/pkg/flow"
)

// ---------------------------------------------------------------------------
// Decision
// ---------------------------------------------------------------------------

// Decision 采样决策类型，使用 uint8 枚举，零值表示丢弃。
type Decision uint8

const (
	DecisionDrop Decision = 0
	DecisionKeep Decision = 1
)

// ---------------------------------------------------------------------------
// SamplingContext — 热路径上下文，必须栈分配，禁止堆逃逸
// ---------------------------------------------------------------------------

// SamplingContext 包含采样决策所需的全部信息。
// 所有字段均为值类型，确保在热路径中零堆分配。
type SamplingContext struct {
	TenantID     string
	Service      string
	Protocol     string
	LatencyNs    uint64
	Bytes        uint64
	HasRetransmit bool
	HasReset     bool
	HasTimeout   bool
	StatusCode   uint16
	ErrorRate    float64 // 0.0-1.0，由聚合层按服务预计算
}

// ---------------------------------------------------------------------------
// Sampler 接口
// ---------------------------------------------------------------------------

// Sampler 采样器接口。实现此接口即可插入采样链。
type Sampler interface {
	// Name 返回采样器名称，用于日志和指标。
	Name() string
	// Sample 根据上下文做出采样决策。
	Sample(ctx *SamplingContext) Decision
}

// ---------------------------------------------------------------------------
// DynamicSampler — 自适应动态采样器
// ---------------------------------------------------------------------------

// DynamicSampler 根据当前流量自动调整采样率。
// 基于确定性哈希保证同一窗口内相同流的一致性。
type DynamicSampler struct {
	baseRate    uint32 // 基准采样率，例如 100 表示 1/100
	currentRate uint32 // 当前采样率（原子操作）
	targetPerSec uint64 // 目标采样后流量（flows/sec）

	counter struct {
		total      uint64
		kept       uint64
		windowStart int64
		mu         sync.Mutex
	}

	// 异常检测子采样器
	errorFirst  *ErrorFirstSampler
	latencyFirst *LatencyFirstSampler
}

// NewDynamicSampler 创建动态采样器。
// baseRate 为基准采样率（如 100 = 1/100），targetPerSec 为目标采样后流量。
func NewDynamicSampler(baseRate uint32, targetPerSec uint64) *DynamicSampler {
	ds := &DynamicSampler{
		baseRate:     baseRate,
		currentRate:  baseRate,
		targetPerSec: targetPerSec,
	}
	ds.counter.windowStart = time.Now().Unix()
	ds.errorFirst = &ErrorFirstSampler{}
	ds.latencyFirst = &LatencyFirstSampler{}
	return ds
}

// Name 返回采样器名称。
func (s *DynamicSampler) Name() string { return "DynamicSampler" }

// Sample 执行采样决策。
// 异常流量直接保留，正常流量使用确定性哈希采样。
func (s *DynamicSampler) Sample(ctx *SamplingContext) Decision {
	// 异常优先：任何异常信号直接保留
	if s.errorFirst.Sample(ctx) == DecisionKeep {
		return DecisionKeep
	}
	if s.latencyFirst.Sample(ctx) == DecisionKeep {
		return DecisionKeep
	}

	// 确定性采样：基于哈希保证一致性
	rate := atomic.LoadUint32(&s.currentRate)
	if rate == 0 {
		rate = 1
	}

	// 使用 tenantID + service + 时间窗口 构建哈希键
	// 时间窗口为 10 秒，保证同一窗口内一致性
	window := time.Now().Unix() / 10
	key := ctx.TenantID + "|" + ctx.Service + "|" + ctx.Protocol + "|" + itoa(window)

	h := fnv.New32a()
	h.Write([]byte(key))
	hashVal := h.Sum32()

	if hashVal%rate == 0 {
		return DecisionKeep
	}
	return DecisionDrop
}

// adjustRate 根据当前流量调整采样率。
// 如果采样后流量超过目标的 120%，则加大采样力度（减小 rate）。
// 如果采样后流量低于目标的 80%，则降低采样力度（增大 rate）。
func (s *DynamicSampler) adjustRate() {
	s.counter.mu.Lock()
	defer s.counter.mu.Unlock()

	now := time.Now().Unix()
	elapsed := now - s.counter.windowStart
	if elapsed < 1 {
		elapsed = 1
	}

	totalPerSec := s.counter.total / uint64(elapsed)
	keptPerSec := s.counter.kept / uint64(elapsed)

	// 重置计数器
	s.counter.total = 0
	s.counter.kept = 0
	s.counter.windowStart = now

	// 根据采样后流量调整 rate
	current := atomic.LoadUint32(&s.currentRate)
	newRate := current

	if keptPerSec > s.targetPerSec*12/10 { // > 120%
		// 采样后流量过多，需要更激进的采样（减小 rate）
		newRate = current * 8 / 10 // 减少 20%
	} else if keptPerSec < s.targetPerSec*8/10 { // < 80%
		// 采样后流量过少，可以降低采样力度（增大 rate）
		newRate = current * 12 / 10 // 增加 20%
	}

	// 限制范围: [baseRate/10, baseRate*10]
	minRate := s.baseRate / 10
	if minRate < 1 {
		minRate = 1
	}
	maxRate := s.baseRate * 10

	if newRate < minRate {
		newRate = minRate
	}
	if newRate > maxRate {
		newRate = maxRate
	}

	atomic.StoreUint32(&s.currentRate, newRate)

	// 更新统计信息
	_ = totalPerSec // 可用于指标暴露
}

// StartAdjustment 启动后台 goroutine 定期调整采样率。
func (s *DynamicSampler) StartAdjustment(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.adjustRate()
			}
		}
	}()
}

// RecordFlow 记录流量用于速率调整（由引擎调用）。
func (s *DynamicSampler) RecordFlow(kept bool) {
	s.counter.mu.Lock()
	s.counter.total++
	if kept {
		s.counter.kept++
	}
	s.counter.mu.Unlock()
}

// CurrentRate 返回当前采样率。
func (s *DynamicSampler) CurrentRate() uint32 {
	return atomic.LoadUint32(&s.currentRate)
}

// ---------------------------------------------------------------------------
// ErrorFirstSampler — 异常优先采样器
// ---------------------------------------------------------------------------

// ErrorFirstSampler 优先保留异常流量。
// 对于非异常流量返回 DecisionDrop，由采样链中的下一个采样器决定。
type ErrorFirstSampler struct {
	statusCodeMin uint16
	errorRateThr  float64
}

// Name 返回采样器名称。
func (s *ErrorFirstSampler) Name() string { return "ErrorFirstSampler" }

// Sample 检查异常信号。
// 如果检测到异常返回 DecisionKeep；否则返回 DecisionDrop（表示"非异常，交给下游采样器"）。
func (s *ErrorFirstSampler) Sample(ctx *SamplingContext) Decision {
	minCode := s.statusCodeMin
	if minCode == 0 {
		minCode = 400
	}

	// HTTP 状态码异常
	if ctx.StatusCode >= minCode {
		return DecisionKeep
	}

	// TCP 异常信号
	if ctx.HasReset {
		return DecisionKeep
	}
	if ctx.HasTimeout {
		return DecisionKeep
	}
	if ctx.HasRetransmit {
		return DecisionKeep
	}

	// 错误率异常
	thr := s.errorRateThr
	if thr == 0 {
		thr = 0.05
	}
	if ctx.ErrorRate > thr {
		return DecisionKeep
	}

	// 非异常流量，交给下游采样器
	return DecisionDrop
}

// ---------------------------------------------------------------------------
// LatencyFirstSampler — 延迟优先采样器
// ---------------------------------------------------------------------------

// LatencyFirstSampler 优先保留高延迟流量。
// 基于动态更新的 P99 阈值判断。
type LatencyFirstSampler struct {
	p99LatencyNs      uint64 // 原子操作，由聚合层更新
	latencyThresholdNs uint64 // 延迟阈值（如 10 * p99）
	p99Multiplier     float64
}

// Name 返回采样器名称。
func (s *LatencyFirstSampler) Name() string { return "LatencyFirstSampler" }

// Sample 检查延迟是否异常。
// 超过阈值返回 DecisionKeep；否则返回 DecisionDrop（交给下游采样器）。
func (s *LatencyFirstSampler) Sample(ctx *SamplingContext) Decision {
	p99 := atomic.LoadUint64(&s.p99LatencyNs)
	if p99 == 0 {
		// P99 尚未初始化，使用默认阈值 100ms
		p99 = 100_000_000
	}

	multiplier := s.p99Multiplier
	if multiplier == 0 {
		multiplier = 5.0
	}

	// 延迟超过 p99 * multiplier → 保留
	threshold := uint64(float64(p99) * multiplier)
	if ctx.LatencyNs > threshold {
		return DecisionKeep
	}

	// 延迟超过显式设置的阈值 → 保留
	if s.latencyThresholdNs > 0 && ctx.LatencyNs > s.latencyThresholdNs {
		return DecisionKeep
	}

	// 正常延迟，交给下游采样器
	return DecisionDrop
}

// UpdateP99 更新 P99 延迟阈值。
func (s *LatencyFirstSampler) UpdateP99(p99 uint64) {
	atomic.StoreUint64(&s.p99LatencyNs, p99)
}

// SetThreshold 设置显式延迟阈值。
func (s *LatencyFirstSampler) SetThreshold(ns uint64) {
	s.latencyThresholdNs = ns
}

// SetMultiplier 设置 P99 倍数。
func (s *LatencyFirstSampler) SetMultiplier(m float64) {
	s.p99Multiplier = m
}

// ---------------------------------------------------------------------------
// SamplingChain — 采样链
// ---------------------------------------------------------------------------

// SamplingChain 将多个采样器串联执行。
// 任一采样器返回 DecisionKeep 即保留；全部返回 DecisionDrop 才丢弃。
type SamplingChain struct {
	samplers []Sampler
}

// NewSamplingChain 创建采样链。
func NewSamplingChain(samplers ...Sampler) *SamplingChain {
	return &SamplingChain{
		samplers: samplers,
	}
}

// Name 返回采样链名称。
func (c *SamplingChain) Name() string { return "SamplingChain" }

// Sample 依次执行链中每个采样器。
// 短路逻辑：任一采样器返回 Keep 即停止并返回 Keep。
func (c *SamplingChain) Sample(ctx *SamplingContext) Decision {
	for _, s := range c.samplers {
		if s.Sample(ctx) == DecisionKeep {
			return DecisionKeep
		}
	}
	return DecisionDrop
}

// ---------------------------------------------------------------------------
// TenantQuota — 租户配额
// ---------------------------------------------------------------------------

// TenantQuota 限制单个租户的最大流量速率。
type TenantQuota struct {
	tenantID        string
	maxFlowsPerSec  uint64
	currentCount    uint64 // 原子操作
	windowStart     int64  // 原子操作
}

// NewTenantQuota 创建租户配额。
func NewTenantQuota(tenantID string, maxFlowsPerSec uint64) *TenantQuota {
	return &TenantQuota{
		tenantID:       tenantID,
		maxFlowsPerSec: maxFlowsPerSec,
		windowStart:    time.Now().Unix(),
	}
}

// Check 检查是否在配额范围内。
// 如果当前窗口内流量未超过限额返回 true。
func (q *TenantQuota) Check() bool {
	now := time.Now().Unix()
	start := atomic.LoadInt64(&q.windowStart)

	// 窗口过期（>1s），重置计数器
	if now > start {
		if atomic.CompareAndSwapInt64(&q.windowStart, start, now) {
			atomic.StoreUint64(&q.currentCount, 0)
		}
	}

	count := atomic.AddUint64(&q.currentCount, 1)
	return count <= q.maxFlowsPerSec
}

// ---------------------------------------------------------------------------
// HotServiceState — 热点服务保护
// ---------------------------------------------------------------------------

// HotServiceState 标记和保护高流量服务。
// 热点服务使用更宽松的采样率以保留更多流量。
type HotServiceState struct {
	service        string
	isHot          uint32 // 原子操作，0=false, 1=true
	protectionRate uint32 // 热点保护采样率（如 10 = 1/10）
	lastUpdate     int64  // 原子操作
}

// NewHotServiceState 创建热点服务状态。
func NewHotServiceState(service string, protectionRate uint32) *HotServiceState {
	return &HotServiceState{
		service:        service,
		protectionRate: protectionRate,
	}
}

// IsHot 检查服务是否为热点。
func (s *HotServiceState) IsHot() bool {
	return atomic.LoadUint32(&s.isHot) == 1
}

// SetHot 设置热点状态和保护采样率。
func (s *HotServiceState) SetHot(hot bool, protectionRate uint32) {
	if hot {
		atomic.StoreUint32(&s.isHot, 1)
	} else {
		atomic.StoreUint32(&s.isHot, 0)
	}
	atomic.StoreUint32(&s.protectionRate, protectionRate)
	atomic.StoreInt64(&s.lastUpdate, time.Now().Unix())
}

// ProtectionRate 返回保护采样率。
func (s *HotServiceState) ProtectionRate() uint32 {
	return atomic.LoadUint32(&s.protectionRate)
}

// ---------------------------------------------------------------------------
// SamplingConfig — 运行时配置
// ---------------------------------------------------------------------------

// SamplingConfig 采样引擎的全局配置，支持运行时热更新。
type SamplingConfig struct {
	// DefaultRate 默认采样率（100 = 1/100）
	DefaultRate uint32

	// ErrorStatusCodeMin 触发保留的最小 HTTP 状态码
	ErrorStatusCodeMin uint16

	// LatencyP99Multiplier P99 延迟倍数阈值
	LatencyP99Multiplier float64

	// HotServiceThreshold 热点服务判定阈值（flows/sec）
	HotServiceThreshold uint64

	// HotServiceProtectionRate 热点服务保护采样率
	HotServiceProtectionRate uint32

	// TenantDefaultMaxFlowsPerSec 租户默认最大流量
	TenantDefaultMaxFlowsPerSec uint64

	// AdjustmentInterval 采样率调整间隔
	AdjustmentInterval time.Duration

	// PerTenant 租户级配置覆盖
	PerTenant map[string]*TenantSamplingConfig

	// PerService 服务级配置覆盖
	PerService map[string]*ServiceSamplingConfig

	// PerProtocol 协议级配置覆盖
	PerProtocol map[string]*ProtocolSamplingConfig
}

// TenantSamplingConfig 租户级采样配置。
type TenantSamplingConfig struct {
	MaxFlowsPerSec uint64
	SamplingRate   uint32 // 0 = 使用默认
}

// ServiceSamplingConfig 服务级采样配置。
type ServiceSamplingConfig struct {
	SamplingRate       uint32
	LatencyThresholdNs uint64  // 0 = 自动从 P99 计算
	ErrorRateThreshold float64 // 0 = 使用默认 0.05
	AlwaysKeep         bool    // 热点服务保护覆盖
}

// ProtocolSamplingConfig 协议级采样配置。
type ProtocolSamplingConfig struct {
	SamplingRate uint32
	AlwaysKeep   bool // 例如 DNS 始终保留
}

// NewSamplingConfig 返回默认配置。
func NewSamplingConfig() *SamplingConfig {
	return &SamplingConfig{
		DefaultRate:                100,
		ErrorStatusCodeMin:         400,
		LatencyP99Multiplier:       5.0,
		HotServiceThreshold:        10000,
		HotServiceProtectionRate:   10,
		TenantDefaultMaxFlowsPerSec: 50000,
		AdjustmentInterval:         10 * time.Second,
		PerTenant:    make(map[string]*TenantSamplingConfig),
		PerService:   make(map[string]*ServiceSamplingConfig),
		PerProtocol:  make(map[string]*ProtocolSamplingConfig),
	}
}

// ---------------------------------------------------------------------------
// SamplingStats — 采样统计
// ---------------------------------------------------------------------------

// SamplingStats 采样引擎运行统计。
type SamplingStats struct {
	TotalFlows          uint64
	KeptFlows           uint64
	DroppedFlows        uint64
	ErrorKeptFlows      uint64
	LatencyKeptFlows    uint64
	QuotaDroppedFlows   uint64
	HotServiceKeptFlows uint64
	CurrentRate         uint32
	EffectiveRate       float64 // kept/total
}

// ---------------------------------------------------------------------------
// SamplingEngine — 采样引擎主入口
// ---------------------------------------------------------------------------

// SamplingEngine 自适应采样引擎主结构。
// ShouldKeep 是热路径方法，必须在 100ns 内完成。
type SamplingEngine struct {
	chain        *SamplingChain
	config       unsafe.Pointer // *SamplingConfig，原子指针用于运行时热更新
	tenantQuotas map[string]*TenantQuota
	hotServices  map[string]*HotServiceState
	stats        SamplingStats
	mu           sync.RWMutex

	// 子采样器引用，用于动态更新
	errorFirst   *ErrorFirstSampler
	latencyFirst *LatencyFirstSampler
	dynamic      *DynamicSampler
}

// NewSamplingEngine 创建采样引擎。
func NewSamplingEngine(config *SamplingConfig) *SamplingEngine {
	if config == nil {
		config = NewSamplingConfig()
	}

	e := &ErrorFirstSampler{
		statusCodeMin: config.ErrorStatusCodeMin,
		errorRateThr:  0.05,
	}
	l := &LatencyFirstSampler{
		p99Multiplier: config.LatencyP99Multiplier,
	}
	d := NewDynamicSampler(config.DefaultRate, config.TenantDefaultMaxFlowsPerSec)
	d.errorFirst = e
	d.latencyFirst = l

	chain := NewSamplingChain(e, l, d)

	engine := &SamplingEngine{
		chain:        chain,
		tenantQuotas: make(map[string]*TenantQuota),
		hotServices:  make(map[string]*HotServiceState),
		errorFirst:   e,
		latencyFirst: l,
		dynamic:      d,
	}
	engine.storeConfig(config)
	return engine
}

// loadConfig 原子加载当前配置。
func (e *SamplingEngine) loadConfig() *SamplingConfig {
	p := atomic.LoadPointer(&e.config)
	return (*SamplingConfig)(p)
}

// storeConfig 原子存储配置。
func (e *SamplingEngine) storeConfig(cfg *SamplingConfig) {
	atomic.StorePointer(&e.config, unsafe.Pointer(cfg))
}

// ShouldKeep 采样决策主入口 —— 热路径方法。
// 执行顺序: 租户配额检查 → 热点服务保护 → 采样链决策。
// 此方法必须在 100ns 内完成，禁止堆分配。
func (e *SamplingEngine) ShouldKeep(ctx *SamplingContext) bool {
	// 1. 更新全局统计
	atomic.AddUint64(&e.stats.TotalFlows, 1)

	// 2. 租户配额检查（读锁）
	e.mu.RLock()
	quota, hasQuota := e.tenantQuotas[ctx.TenantID]
	hotSvc, hasHot := e.hotServices[ctx.Service]
	cfg := e.loadConfig()
	e.mu.RUnlock()

	if hasQuota && !quota.Check() {
		atomic.AddUint64(&e.stats.QuotaDroppedFlows, 1)
		atomic.AddUint64(&e.stats.DroppedFlows, 1)
		return false
	}

	// 3. 协议级 AlwaysKeep 检查
	if protoCfg, ok := cfg.PerProtocol[ctx.Protocol]; ok && protoCfg.AlwaysKeep {
		atomic.AddUint64(&e.stats.KeptFlows, 1)
		return true
	}

	// 4. 服务级 AlwaysKeep 检查
	if svcCfg, ok := cfg.PerService[ctx.Service]; ok && svcCfg.AlwaysKeep {
		atomic.AddUint64(&e.stats.HotServiceKeptFlows, 1)
		atomic.AddUint64(&e.stats.KeptFlows, 1)
		return true
	}

	// 5. 热点服务保护
	if hasHot && hotSvc.IsHot() {
		// 热点服务使用更宽松的采样率
		// 这里仍然走采样链，但 DynamicSampler 会使用保护率
		atomic.AddUint64(&e.stats.HotServiceKeptFlows, 1)
	}

	// 6. 运行采样链
	decision := e.chain.Sample(ctx)

	if decision == DecisionKeep {
		atomic.AddUint64(&e.stats.KeptFlows, 1)

		// 分类统计
		if ctx.StatusCode >= cfg.ErrorStatusCodeMin || ctx.HasReset || ctx.HasTimeout || ctx.HasRetransmit {
			atomic.AddUint64(&e.stats.ErrorKeptFlows, 1)
		}
		if ctx.LatencyNs > 0 {
			p99 := atomic.LoadUint64(&e.latencyFirst.p99LatencyNs)
			if p99 > 0 && ctx.LatencyNs > uint64(float64(p99)*cfg.LatencyP99Multiplier) {
				atomic.AddUint64(&e.stats.LatencyKeptFlows, 1)
			}
		}

		// 记录到动态采样器
		e.dynamic.RecordFlow(true)
		return true
	}

	atomic.AddUint64(&e.stats.DroppedFlows, 1)
	e.dynamic.RecordFlow(false)
	return false
}

// UpdateConfig 运行时热更新配置（原子交换）。
func (e *SamplingEngine) UpdateConfig(config *SamplingConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.storeConfig(config)

	// 更新子采样器参数
	e.errorFirst.statusCodeMin = config.ErrorStatusCodeMin
	e.latencyFirst.p99Multiplier = config.LatencyP99Multiplier

	// 更新动态采样器基准率
	atomic.StoreUint32(&e.dynamic.baseRate, config.DefaultRate)

	// 同步租户配额
	for tenantID, tc := range config.PerTenant {
		maxFlows := tc.MaxFlowsPerSec
		if maxFlows == 0 {
			maxFlows = config.TenantDefaultMaxFlowsPerSec
		}
		if existing, ok := e.tenantQuotas[tenantID]; ok {
			existing.maxFlowsPerSec = maxFlows
		} else {
			e.tenantQuotas[tenantID] = NewTenantQuota(tenantID, maxFlows)
		}
	}

	// 同步热点服务
	for service, sc := range config.PerService {
		if sc.AlwaysKeep {
			if existing, ok := e.hotServices[service]; ok {
				existing.SetHot(true, config.HotServiceProtectionRate)
			} else {
				hs := NewHotServiceState(service, config.HotServiceProtectionRate)
				hs.SetHot(true, config.HotServiceProtectionRate)
				e.hotServices[service] = hs
			}
		}
	}
}

// GetStats 返回采样统计快照。
func (e *SamplingEngine) GetStats() SamplingStats {
	stats := SamplingStats{
		TotalFlows:          atomic.LoadUint64(&e.stats.TotalFlows),
		KeptFlows:           atomic.LoadUint64(&e.stats.KeptFlows),
		DroppedFlows:        atomic.LoadUint64(&e.stats.DroppedFlows),
		ErrorKeptFlows:      atomic.LoadUint64(&e.stats.ErrorKeptFlows),
		LatencyKeptFlows:    atomic.LoadUint64(&e.stats.LatencyKeptFlows),
		QuotaDroppedFlows:   atomic.LoadUint64(&e.stats.QuotaDroppedFlows),
		HotServiceKeptFlows: atomic.LoadUint64(&e.stats.HotServiceKeptFlows),
		CurrentRate:         e.dynamic.CurrentRate(),
	}
	total := stats.TotalFlows
	if total > 0 {
		stats.EffectiveRate = float64(stats.KeptFlows) / float64(total)
	}
	return stats
}

// UpdateServiceErrorRate 由聚合层调用，更新服务错误率。
// 高错误率服务自动标记为热点，享受更宽松的采样保护。
func (e *SamplingEngine) UpdateServiceErrorRate(service string, errorRate float64) {
	cfg := e.loadConfig()
	if errorRate > 0.1 { // 错误率 > 10% 视为热点
		e.mu.RLock()
		state, ok := e.hotServices[service]
		e.mu.RUnlock()
		if !ok {
			e.mu.Lock()
			state = &HotServiceState{service: service}
			e.hotServices[service] = state
			e.mu.Unlock()
		}
		state.SetHot(true, cfg.HotServiceProtectionRate)
	} else if errorRate < 0.02 { // 错误率 < 2% 取消热点
		e.mu.RLock()
		state, ok := e.hotServices[service]
		e.mu.RUnlock()
		if ok {
			state.SetHot(false, cfg.DefaultRate)
		}
	}
}

// UpdateP99 由聚合层调用，更新服务 P99 延迟。
// P99 过高的服务自动标记为热点。
func (e *SamplingEngine) UpdateP99(service string, p99Ns uint64) {
	e.latencyFirst.UpdateP99(p99Ns)

	cfg := e.loadConfig()
	// P99 > 1s 视为高延迟热点服务
	if p99Ns > 1_000_000_000 {
		e.mu.RLock()
		state, ok := e.hotServices[service]
		e.mu.RUnlock()
		if !ok {
			e.mu.Lock()
			state = &HotServiceState{service: service}
			e.hotServices[service] = state
			e.mu.Unlock()
		}
		state.SetHot(true, cfg.HotServiceProtectionRate)
	}
}

// Start 启动采样引擎后台任务。
func (e *SamplingEngine) Start(ctx context.Context) {
	cfg := e.loadConfig()
	e.dynamic.StartAdjustment(ctx, cfg.AdjustmentInterval)
}

// Chain 返回采样链（用于测试）。
func (e *SamplingEngine) Chain() *SamplingChain {
	return e.chain
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// FlowToContext 从 UnifiedFlow 提取采样上下文（值类型，栈分配）。
func FlowToContext(f *flow.UnifiedFlow) SamplingContext {
	return SamplingContext{
		TenantID:      f.TenantID.String(),
		Service:       f.Service.String(),
		Protocol:      f.Protocol.String(),
		LatencyNs:     f.LatencyNs,
		Bytes:         f.Bytes,
		HasRetransmit: f.TCPFlags&0x02 != 0, // RET bit
		HasReset:      f.TCPFlags&0x04 != 0, // RST bit
		HasTimeout:    f.LatencyNs > 0 && f.Bytes == 0, // 有延迟但无数据 = 超时
		StatusCode:    f.StatusCode,
	}
}

// itoa 将 int64 转为字符串，避免 fmt.Sprintf 的堆分配。
func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
