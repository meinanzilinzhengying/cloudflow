//go:build linux

package autoscaler

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// ============================================================================
// P7 弹性伸缩机制
// 解决：没有 HPA 配置、没有基于指标的自动扩缩容策略、没有扩容预热、没有缩容保护
// ============================================================================

// MetricType 指标类型
type MetricType string

const (
	MetricCPU    MetricType = "cpu"
	MetricMemory MetricType = "memory"
	MetricQPS    MetricType = "qps"
	MetricConn   MetricType = "connections"
)

// MetricValue 指标值
type MetricValue struct {
	Type      MetricType
	Value     float64
	Timestamp time.Time
}

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	Collect(ctx context.Context) ([]MetricValue, error)
}

// StaticMetricsCollector 静态指标收集器（用于测试）
type StaticMetricsCollector struct {
	metrics []MetricValue
}

func NewStaticMetricsCollector(metrics []MetricValue) *StaticMetricsCollector {
	return &StaticMetricsCollector{metrics: metrics}
}

func (s *StaticMetricsCollector) Collect(ctx context.Context) ([]MetricValue, error) {
	return s.metrics, nil
}

// HPAConfig HPA 配置
type HPAConfig struct {
	MinReplicas           int           // 最小副本数
	MaxReplicas           int           // 最大副本数
	TargetCPUUtilization  float64       // 目标 CPU 利用率 (%)
	TargetMemoryUtilization float64     // 目标内存利用率 (%)
	TargetQPSPerReplica   float64       // 每个副本目标 QPS
	ScaleUpThreshold      float64       // 扩容阈值（超出目标多少比例触发）
	ScaleDownThreshold    float64       // 缩容阈值（低于目标多少比例触发）
	ScaleUpStabilization  time.Duration // 扩容稳定窗口（冷却时间）
	ScaleDownStabilization time.Duration // 缩容稳定窗口（冷却时间）
	ScaleUpStep           int           // 每次扩容增加副本数
	ScaleDownStep         int           // 每次缩容减少副本数
}

// DefaultHPAConfig 返回默认 HPA 配置
func DefaultHPAConfig() *HPAConfig {
	return &HPAConfig{
		MinReplicas:            2,
		MaxReplicas:            20,
		TargetCPUUtilization:   70.0,
		TargetMemoryUtilization: 80.0,
		TargetQPSPerReplica:    1000,
		ScaleUpThreshold:       1.1,  // 超出目标 10%
		ScaleDownThreshold:     0.7,  // 低于目标 30%
		ScaleUpStabilization:   60 * time.Second,
		ScaleDownStabilization: 5 * time.Minute,
		ScaleUpStep:            2,
		ScaleDownStep:          1,
	}
}

// Validate 验证配置
func (c *HPAConfig) Validate() error {
	if c.MinReplicas < 1 {
		return fmt.Errorf("minReplicas must be >= 1")
	}
	if c.MaxReplicas < c.MinReplicas {
		return fmt.Errorf("maxReplicas must be >= minReplicas")
	}
	if c.TargetCPUUtilization <= 0 {
		return fmt.Errorf("targetCPUUtilization must be > 0")
	}
	if c.ScaleUpStep < 1 {
		return fmt.Errorf("scaleUpStep must be >= 1")
	}
	if c.ScaleDownStep < 1 {
		return fmt.Errorf("scaleDownStep must be >= 1")
	}
	return nil
}

// ScaleDecision 扩缩容决策
type ScaleDecision int

const (
	ScaleNone ScaleDecision = iota
	ScaleUp
	ScaleDown
)

// ScaleRecommendation 扩缩容建议
type ScaleRecommendation struct {
	Decision      ScaleDecision
	CurrentReplicas int
	TargetReplicas  int
	Reason        string
	Timestamp     time.Time
}

// AutoScaler 自动扩缩容引擎
type AutoScaler struct {
	config     *HPAConfig
	collector  MetricsCollector
	
	mu         sync.RWMutex
	currentReplicas int
	lastScaleUpTime time.Time
	lastScaleDownTime time.Time
	lastDecision    ScaleDecision
	
	recommendations []ScaleRecommendation
}

// NewAutoScaler 创建自动扩缩容引擎
func NewAutoScaler(config *HPAConfig, collector MetricsCollector) (*AutoScaler, error) {
	if config == nil {
		config = DefaultHPAConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &AutoScaler{
		config:          config,
		collector:       collector,
		currentReplicas: config.MinReplicas,
		recommendations: make([]ScaleRecommendation, 0),
	}, nil
}

// SetCurrentReplicas 设置当前副本数
func (as *AutoScaler) SetCurrentReplicas(n int) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.currentReplicas = n
}

// GetCurrentReplicas 获取当前副本数
func (as *AutoScaler) GetCurrentReplicas() int {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.currentReplicas
}

// Evaluate 评估是否需要扩缩容
func (as *AutoScaler) Evaluate(ctx context.Context) (*ScaleRecommendation, error) {
	metrics, err := as.collector.Collect(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect metrics failed: %w", err)
	}
	
	as.mu.Lock()
	defer as.mu.Unlock()
	
	current := as.currentReplicas
	if current < as.config.MinReplicas {
		current = as.config.MinReplicas
	}
	
	// 计算 CPU 建议副本数
	var cpuDesired, memDesired, qpsDesired float64
	for _, m := range metrics {
		switch m.Type {
		case MetricCPU:
			if as.config.TargetCPUUtilization > 0 {
				cpuDesired = float64(current) * m.Value / as.config.TargetCPUUtilization
			}
		case MetricMemory:
			if as.config.TargetMemoryUtilization > 0 {
				memDesired = float64(current) * m.Value / as.config.TargetMemoryUtilization
			}
		case MetricQPS:
			if as.config.TargetQPSPerReplica > 0 {
				qpsDesired = m.Value / as.config.TargetQPSPerReplica
			}
		}
	}
	
	// 取最大需求
	desired := math.Max(cpuDesired, math.Max(memDesired, qpsDesired))
	desiredReplicas := int(math.Ceil(desired))
	if desiredReplicas < as.config.MinReplicas {
		desiredReplicas = as.config.MinReplicas
	}
	if desiredReplicas > as.config.MaxReplicas {
		desiredReplicas = as.config.MaxReplicas
	}
	
	now := time.Now()
	var decision ScaleDecision = ScaleNone
	var targetReplicas = current
	var reason string
	
	if desiredReplicas > current {
		// 检查扩容冷却
		if now.Sub(as.lastScaleUpTime) < as.config.ScaleUpStabilization {
			decision = ScaleNone
			reason = fmt.Sprintf("cpu=%.1f%% desired=%d but in scale-up stabilization", 
				getMetricValue(metrics, MetricCPU), desiredReplicas)
		} else {
			decision = ScaleUp
			step := as.config.ScaleUpStep
			if step < 1 {
				step = 1
			}
			targetReplicas = current + step
			if targetReplicas > desiredReplicas {
				targetReplicas = desiredReplicas
			}
			if targetReplicas > as.config.MaxReplicas {
				targetReplicas = as.config.MaxReplicas
			}
			reason = fmt.Sprintf("cpu=%.1f%% scale up from %d to %d", 
				getMetricValue(metrics, MetricCPU), current, targetReplicas)
		}
	} else if desiredReplicas < current {
		// 检查缩容冷却
		if now.Sub(as.lastScaleDownTime) < as.config.ScaleDownStabilization {
			decision = ScaleNone
			reason = fmt.Sprintf("cpu=%.1f%% desired=%d but in scale-down stabilization", 
				getMetricValue(metrics, MetricCPU), desiredReplicas)
		} else {
			decision = ScaleDown
			step := as.config.ScaleDownStep
			if step < 1 {
				step = 1
			}
			targetReplicas = current - step
			if targetReplicas < desiredReplicas {
				targetReplicas = desiredReplicas
			}
			if targetReplicas < as.config.MinReplicas {
				targetReplicas = as.config.MinReplicas
			}
			reason = fmt.Sprintf("cpu=%.1f%% scale down from %d to %d", 
				getMetricValue(metrics, MetricCPU), current, targetReplicas)
		}
	} else {
		reason = fmt.Sprintf("cpu=%.1f%% current=%d desired=%d no change", 
			getMetricValue(metrics, MetricCPU), current, desiredReplicas)
	}
	
	rec := &ScaleRecommendation{
		Decision:        decision,
		CurrentReplicas: current,
		TargetReplicas:  targetReplicas,
		Reason:          reason,
		Timestamp:       now,
	}
	
	if decision != ScaleNone {
		as.recommendations = append(as.recommendations, *rec)
		if decision == ScaleUp {
			as.lastScaleUpTime = now
		} else {
			as.lastScaleDownTime = now
		}
		as.currentReplicas = targetReplicas
	}
	
	as.lastDecision = decision
	return rec, nil
}

func getMetricValue(metrics []MetricValue, t MetricType) float64 {
	for _, m := range metrics {
		if m.Type == t {
			return m.Value
		}
	}
	return 0
}

// GetRecommendations 获取历史建议
func (as *AutoScaler) GetRecommendations() []ScaleRecommendation {
	as.mu.RLock()
	defer as.mu.RUnlock()
	result := make([]ScaleRecommendation, len(as.recommendations))
	copy(result, as.recommendations)
	return result
}

// GetLastDecision 获取上次决策
func (as *AutoScaler) GetLastDecision() ScaleDecision {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.lastDecision
}

// ============================================================================
// 扩容预热机制
// ============================================================================

// WarmupState 预热状态
type WarmupState int

const (
	WarmupPending WarmupState = iota
	WarmupInProgress
	WarmupCompleted
)

// WarmupNode 预热节点
type WarmupNode struct {
	ID        string
	Weight    float64 // 0.0 - 1.0
	State     WarmupState
	StartTime time.Time
	EndTime   time.Time
}

// ScaleUpWarmup 扩容预热管理器
type ScaleUpWarmup struct {
	warmupDuration time.Duration
	steps          int
	
	mu     sync.RWMutex
	nodes  map[string]*WarmupNode
}

// NewScaleUpWarmup 创建预热管理器
func NewScaleUpWarmup(duration time.Duration, steps int) *ScaleUpWarmup {
	if duration <= 0 {
		duration = 2 * time.Minute
	}
	if steps <= 0 {
		steps = 10
	}
	return &ScaleUpWarmup{
		warmupDuration: duration,
		steps:          steps,
		nodes:          make(map[string]*WarmupNode),
	}
}

// RegisterNode 注册新节点（开始预热）
func (sw *ScaleUpWarmup) RegisterNode(nodeID string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.nodes[nodeID] = &WarmupNode{
		ID:        nodeID,
		Weight:    0.0,
		State:     WarmupInProgress,
		StartTime: time.Now(),
	}
}

// GetWeight 获取节点当前权重
func (sw *ScaleUpWarmup) GetWeight(nodeID string) float64 {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	
	node, ok := sw.nodes[nodeID]
	if !ok {
		return 1.0 // 未知节点默认全权重
	}
	
	if node.State == WarmupCompleted {
		return 1.0
	}
	
	elapsed := time.Since(node.StartTime)
	if elapsed >= sw.warmupDuration {
		node.State = WarmupCompleted
		node.EndTime = time.Now()
		node.Weight = 1.0
		return 1.0
	}
	
	progress := float64(elapsed) / float64(sw.warmupDuration)
	node.Weight = progress
	return progress
}

// IsWarmupCompleted 检查节点是否完成预热
func (sw *ScaleUpWarmup) IsWarmupCompleted(nodeID string) bool {
	sw.mu.RLock()
	node, ok := sw.nodes[nodeID]
	if !ok {
		return true
	}
	if node.State == WarmupCompleted {
		sw.mu.RUnlock()
		return true
	}
	elapsed := time.Since(node.StartTime)
	sw.mu.RUnlock()
	if elapsed >= sw.warmupDuration {
		// 重新加锁更新状态
		sw.mu.Lock()
		node.State = WarmupCompleted
		node.EndTime = time.Now()
		node.Weight = 1.0
		sw.mu.Unlock()
		return true
	}
	return false
}

// GetAllNodes 获取所有节点
func (sw *ScaleUpWarmup) GetAllNodes() map[string]*WarmupNode {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	result := make(map[string]*WarmupNode)
	for k, v := range sw.nodes {
		result[k] = v
	}
	return result
}

// ============================================================================
// 缩容保护机制
// ============================================================================

// ProtectionState 保护状态
type ProtectionState int

const (
	ProtectionIdle ProtectionState = iota
	ProtectionDraining
	ProtectionDataMigrating
	ProtectionReadyToRemove
)

// ScaleDownProtectionNode 缩容保护节点
type ScaleDownProtectionNode struct {
	ID              string
	State           ProtectionState
	StartTime       time.Time
	DataMoved       int64
	TotalData       int64
	GracefulTimeout time.Duration
}

// ScaleDownProtection 缩容保护管理器
type ScaleDownProtection struct {
	gracefulTimeout time.Duration
	
	mu     sync.Mutex
	nodes  map[string]*ScaleDownProtectionNode
}

// NewScaleDownProtection 创建缩容保护管理器
func NewScaleDownProtection(gracefulTimeout time.Duration) *ScaleDownProtection {
	if gracefulTimeout <= 0 {
		gracefulTimeout = 30 * time.Second
	}
	return &ScaleDownProtection{
		gracefulTimeout: gracefulTimeout,
		nodes:           make(map[string]*ScaleDownProtectionNode),
	}
}

// InitiateRemoval 发起缩容保护（开始数据迁移）
func (sdp *ScaleDownProtection) InitiateRemoval(nodeID string, totalData int64) {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	sdp.nodes[nodeID] = &ScaleDownProtectionNode{
		ID:              nodeID,
		State:           ProtectionDataMigrating,
		StartTime:       time.Now(),
		TotalData:       totalData,
		GracefulTimeout: sdp.gracefulTimeout,
	}
}

// UpdateMigrationProgress 更新迁移进度
func (sdp *ScaleDownProtection) UpdateMigrationProgress(nodeID string, dataMoved int64) {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	node, ok := sdp.nodes[nodeID]
	if !ok {
		return
	}
	node.DataMoved = dataMoved
	if node.TotalData > 0 && node.DataMoved >= node.TotalData {
		node.State = ProtectionReadyToRemove
	}
}

// CanRemove 检查节点是否可以安全移除
func (sdp *ScaleDownProtection) CanRemove(nodeID string) bool {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	node, ok := sdp.nodes[nodeID]
	if !ok {
		return true // 未注册默认可移除
	}
	
	if node.State == ProtectionReadyToRemove {
		return true
	}
	
	// 超时检查
	if time.Since(node.StartTime) > node.GracefulTimeout {
		node.State = ProtectionReadyToRemove
		return true
	}
	
	return false
}

// GetNodeState 获取节点保护状态
func (sdp *ScaleDownProtection) GetNodeState(nodeID string) ProtectionState {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	node, ok := sdp.nodes[nodeID]
	if !ok {
		return ProtectionIdle
	}
	return node.State
}

// GetMigrationProgress 获取迁移进度（0-1）
func (sdp *ScaleDownProtection) GetMigrationProgress(nodeID string) float64 {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	node, ok := sdp.nodes[nodeID]
	if !ok || node.TotalData == 0 {
		return 1.0
	}
	return float64(node.DataMoved) / float64(node.TotalData)
}

// ConfirmRemoval 确认移除（清理记录）
func (sdp *ScaleDownProtection) ConfirmRemoval(nodeID string) {
	sdp.mu.Lock()
	defer sdp.mu.Unlock()
	delete(sdp.nodes, nodeID)
}
