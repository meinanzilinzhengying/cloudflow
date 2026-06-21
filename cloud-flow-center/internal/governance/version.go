//go:build linux

package governance

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// 一、服务版本管理
// ============================================================================

// ServiceVersion 服务版本信息
type ServiceVersion struct {
	ServiceName string            `json:"service_name"`
	Version     string            `json:"version"`        // 语义化版本如 v1.2.3
	InstanceID  string            `json:"instance_id"`
	Host        string            `json:"host"`
	Port        int               `json:"port"`
	Status      VersionStatus     `json:"status"`
	Weight      int               `json:"weight"`        // 权重（0-100）
	Labels      map[string]string `json:"labels,omitempty"`
	RegisteredAt time.Time        `json:"registered_at"`
	HealthAt    time.Time         `json:"health_at"`
}

// VersionStatus 版本状态
type VersionStatus string

const (
	VersionActive    VersionStatus = "active"     // 活跃
	VersionCanary    VersionStatus = "canary"     // 灰度
	VersionDeprecated VersionStatus = "deprecated" // 废弃
	VersionRetired   VersionStatus = "retired"    // 退役
)

// VersionRegistry 版本注册表
type VersionRegistry struct {
	mu       sync.RWMutex
	versions map[string]map[string]*ServiceVersion // service -> instanceID -> version
}

// NewVersionRegistry 创建版本注册表
func NewVersionRegistry() *VersionRegistry {
	return &VersionRegistry{
		versions: make(map[string]map[string]*ServiceVersion),
	}
}

// Register 注册版本
func (vr *VersionRegistry) Register(v *ServiceVersion) error {
	if v.ServiceName == "" || v.Version == "" || v.InstanceID == "" {
		return fmt.Errorf("service_name, version, instance_id required")
	}
	if v.Status == "" {
		v.Status = VersionActive
	}
	if v.Weight == 0 && v.Status == VersionCanary {
		v.Weight = 5 // 默认灰度 5%
	}
	if v.RegisteredAt.IsZero() {
		v.RegisteredAt = time.Now()
	}

	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.versions[v.ServiceName] == nil {
		vr.versions[v.ServiceName] = make(map[string]*ServiceVersion)
	}
	vr.versions[v.ServiceName][v.InstanceID] = v
	return nil
}

// Deregister 注销版本
func (vr *VersionRegistry) Deregister(serviceName, instanceID string) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if instances, ok := vr.versions[serviceName]; ok {
		delete(instances, instanceID)
		if len(instances) == 0 {
			delete(vr.versions, serviceName)
		}
	}
}

// GetVersions 获取服务所有版本
func (vr *VersionRegistry) GetVersions(serviceName string) []*ServiceVersion {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	instances := vr.versions[serviceName]
	if len(instances) == 0 {
		return nil
	}
	result := make([]*ServiceVersion, 0, len(instances))
	for _, v := range instances {
		result = append(result, v)
	}
	return result
}

// GetActiveVersions 获取活跃版本
func (vr *VersionRegistry) GetActiveVersions(serviceName string) []*ServiceVersion {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	instances := vr.versions[serviceName]
	var result []*ServiceVersion
	for _, v := range instances {
		if v.Status == VersionActive || v.Status == VersionCanary {
			result = append(result, v)
		}
	}
	return result
}

// GetVersionDistribution 获取版本分布
func (vr *VersionRegistry) GetVersionDistribution(serviceName string) map[string]int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	distribution := make(map[string]int)
	for _, v := range vr.versions[serviceName] {
		distribution[v.Version]++
	}
	return distribution
}

// UpdateStatus 更新版本状态
func (vr *VersionRegistry) UpdateStatus(serviceName, instanceID string, status VersionStatus) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	instances, ok := vr.versions[serviceName]
	if !ok {
		return fmt.Errorf("service not found: %s", serviceName)
	}
	v, ok := instances[instanceID]
	if !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}
	v.Status = status
	return nil
}

// UpdateWeight 更新版本权重
func (vr *VersionRegistry) UpdateWeight(serviceName, instanceID string, weight int) error {
	if weight < 0 || weight > 100 {
		return fmt.Errorf("weight must be 0-100")
	}
	vr.mu.Lock()
	defer vr.mu.Unlock()

	instances, ok := vr.versions[serviceName]
	if !ok {
		return fmt.Errorf("service not found: %s", serviceName)
	}
	v, ok := instances[instanceID]
	if !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}
	v.Weight = weight
	return nil
}

// SelectByWeight 按权重选择版本（灰度用）
func (vr *VersionRegistry) SelectByWeight(serviceName string, hashKey string) (*ServiceVersion, error) {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	versions := vr.versions[serviceName]
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions for service: %s", serviceName)
	}

	// 按权重加权随机选择
	var totalWeight int
	var active []*ServiceVersion
	for _, v := range versions {
		if v.Status == VersionActive || v.Status == VersionCanary {
			active = append(active, v)
			totalWeight += v.Weight
		}
	}
	if totalWeight == 0 {
		return nil, fmt.Errorf("no active versions")
	}

	// 使用 hashKey 的一致性选择
	hash := 0
	for _, c := range hashKey {
		hash = (hash*31 + int(c)) % totalWeight
	}
	for _, v := range active {
		hash -= v.Weight
		if hash < 0 {
			return v, nil
		}
	}
	return active[0], nil
}

// ============================================================================
// 二、灰度发布管理器
// ============================================================================

// CanaryConfig 灰度发布配置
type CanaryConfig struct {
	ServiceName    string        `json:"service_name"`
	CanaryVersion  string        `json:"canary_version"`
	StableVersion  string        `json:"stable_version"`
	TrafficPercent int           `json:"traffic_percent"` // 0-100
	StepPercent    int           `json:"step_percent"`     // 每次放量步长
	StepInterval   time.Duration `json:"step_interval"`    // 放量间隔
	Criteria       CanaryCriteria `json:"criteria"`
}

// CanaryCriteria 灰度准入条件
type CanaryCriteria struct {
	MinRequestCount   int     `json:"min_request_count"`   // 最小请求数
	MaxErrorRate      float64 `json:"max_error_rate"`      // 最大错误率
	MaxLatencyP99     int64   `json:"max_latency_p99"`     // P99 延迟阈值(ms)
	MinDuration       time.Duration `json:"min_duration"`    // 最短灰度持续时间
}

// CanaryRelease 灰度发布状态
type CanaryRelease struct {
	ID            string          `json:"id"`
	Config        *CanaryConfig   `json:"config"`
	Status        CanaryStatus    `json:"status"`
	CurrentPercent int            `json:"current_percent"`
	StartTime     time.Time       `json:"start_time"`
	EndTime       *time.Time      `json:"end_time,omitempty"`
	Metrics       CanaryMetrics   `json:"metrics"`
}

// CanaryStatus 灰度状态
type CanaryStatus string

const (
	CanaryPending     CanaryStatus = "pending"
	CanaryRunning     CanaryStatus = "running"
	CanaryPromoting   CanaryStatus = "promoting"
	CanaryCompleted   CanaryStatus = "completed"
	CanaryRolledBack  CanaryStatus = "rolled_back"
	CanaryFailed      CanaryStatus = "failed"
)

// CanaryMetrics 灰度指标
type CanaryMetrics struct {
	RequestCount  int64   `json:"request_count"`
	ErrorCount    int64   `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	LatencyP99    int64   `json:"latency_p99"`
	LatencyP95    int64   `json:"latency_p95"`
	LatencyP50    int64   `json:"latency_p50"`
}

// CanaryManager 灰度发布管理器
type CanaryManager struct {
	mu         sync.RWMutex
	releases   map[string]*CanaryRelease // serviceName -> release
	registry   *VersionRegistry
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewCanaryManager 创建灰度管理器
func NewCanaryManager(registry *VersionRegistry) *CanaryManager {
	return &CanaryManager{
		releases: make(map[string]*CanaryRelease),
		registry: registry,
		stopCh:   make(chan struct{}),
	}
}

// Start 启动灰度调度
func (cm *CanaryManager) Start() {
	cm.wg.Add(1)
	go cm.promotionLoop()
}

// Stop 停止灰度调度
func (cm *CanaryManager) Stop() {
	close(cm.stopCh)
	cm.wg.Wait()
}

// promotionLoop 灰度放量循环
func (cm *CanaryManager) promotionLoop() {
	defer cm.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkPromotions()
		}
	}
}

// checkPromotions 检查并推进灰度
func (cm *CanaryManager) checkPromotions() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, release := range cm.releases {
		if release.Status != CanaryRunning && release.Status != CanaryPromoting {
			continue
		}

		cfg := release.Config
		metrics := release.Metrics

		// 检查准入条件
		if release.Status == CanaryRunning {
			if !cm.meetsCriteria(cfg.Criteria, metrics, time.Since(release.StartTime)) {
				continue
			}
			release.Status = CanaryPromoting
		}

		// 推进放量
		if release.CurrentPercent < cfg.TrafficPercent {
			release.CurrentPercent += cfg.StepPercent
			if release.CurrentPercent > cfg.TrafficPercent {
				release.CurrentPercent = cfg.TrafficPercent
			}
			// 更新 registry 权重
			cm.updateCanaryWeights(release)
		}

		// 完成
		if release.CurrentPercent >= cfg.TrafficPercent {
			release.Status = CanaryCompleted
			now := time.Now()
			release.EndTime = &now
		}
	}
}

// meetsCriteria 检查是否满足准入条件
func (cm *CanaryManager) meetsCriteria(criteria CanaryCriteria, metrics CanaryMetrics, duration time.Duration) bool {
	if metrics.RequestCount < int64(criteria.MinRequestCount) {
		return false
	}
	if metrics.ErrorRate > criteria.MaxErrorRate {
		return false
	}
	if metrics.LatencyP99 > criteria.MaxLatencyP99 {
		return false
	}
	if duration < criteria.MinDuration {
		return false
	}
	return true
}

// updateCanaryWeights 更新灰度权重
func (cm *CanaryManager) updateCanaryWeights(release *CanaryRelease) {
	if cm.registry == nil {
		return
	}
	versions := cm.registry.GetVersions(release.Config.ServiceName)
	for _, v := range versions {
		if v.Version == release.Config.CanaryVersion {
			v.Weight = release.CurrentPercent
			v.Status = VersionCanary
		} else if v.Version == release.Config.StableVersion {
			v.Weight = 100 - release.CurrentPercent
			v.Status = VersionActive
		}
	}
}

// CreateRelease 创建灰度发布
func (cm *CanaryManager) CreateRelease(config *CanaryConfig) (*CanaryRelease, error) {
	if config.ServiceName == "" || config.CanaryVersion == "" || config.StableVersion == "" {
		return nil, fmt.Errorf("service_name, canary_version, stable_version required")
	}
	if config.StepPercent <= 0 {
		config.StepPercent = 5
	}
	if config.StepInterval <= 0 {
		config.StepInterval = 5 * time.Minute
	}

	release := &CanaryRelease{
		ID:             fmt.Sprintf("canary-%s-%d", config.ServiceName, time.Now().Unix()),
		Config:         config,
		Status:         CanaryPending,
		CurrentPercent: 0,
		StartTime:      time.Now(),
	}

	cm.mu.Lock()
	cm.releases[config.ServiceName] = release
	cm.mu.Unlock()

	// 初始权重：灰度版本 0%，稳定版本 100%
	cm.updateCanaryWeights(release)
	release.Status = CanaryRunning

	return release, nil
}

// GetRelease 获取灰度发布
func (cm *CanaryManager) GetRelease(serviceName string) (*CanaryRelease, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	release, ok := cm.releases[serviceName]
	return release, ok
}

// UpdateMetrics 更新灰度指标
func (cm *CanaryManager) UpdateMetrics(serviceName string, metrics CanaryMetrics) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if release, ok := cm.releases[serviceName]; ok {
		release.Metrics = metrics
	}
}

// Rollback 回滚灰度
func (cm *CanaryManager) Rollback(serviceName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	release, ok := cm.releases[serviceName]
	if !ok {
		return fmt.Errorf("no canary release for service: %s", serviceName)
	}

	release.Status = CanaryRolledBack
	release.CurrentPercent = 0
	now := time.Now()
	release.EndTime = &now

	// 回滚权重：灰度 0%，稳定 100%
	cm.updateCanaryWeights(release)
	return nil
}

// Complete 完成灰度（将灰度版本设为稳定）
func (cm *CanaryManager) Complete(serviceName string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	release, ok := cm.releases[serviceName]
	if !ok {
		return fmt.Errorf("no canary release for service: %s", serviceName)
	}

	release.Status = CanaryCompleted
	release.CurrentPercent = 100
	now := time.Now()
	release.EndTime = &now

	// 将所有灰度实例标记为稳定
	versions := cm.registry.GetVersions(serviceName)
	for _, v := range versions {
		if v.Version == release.Config.CanaryVersion {
			v.Status = VersionActive
			v.Weight = 100
		}
	}
	return nil
}

// GetAllReleases 获取所有灰度发布
func (cm *CanaryManager) GetAllReleases() []*CanaryRelease {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]*CanaryRelease, 0, len(cm.releases))
	for _, r := range cm.releases {
		result = append(result, r)
	}
	return result
}

// Stats 获取灰度统计
func (cm *CanaryManager) Stats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	pending := 0
	running := 0
	completed := 0
	rolledBack := 0
	failed := 0

	for _, r := range cm.releases {
		switch r.Status {
		case CanaryPending:
			pending++
		case CanaryRunning, CanaryPromoting:
			running++
		case CanaryCompleted:
			completed++
		case CanaryRolledBack:
			rolledBack++
		case CanaryFailed:
			failed++
		}
	}

	return map[string]interface{}{
		"total":      len(cm.releases),
		"pending":    pending,
		"running":    running,
		"completed":  completed,
		"rolled_back": rolledBack,
		"failed":     failed,
	}
}
