// P8: 探针管理引擎
package probe

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ============================================================================
// 一、探针版本管理
// ============================================================================

// Version 探针版本
type Version struct {
	Major     int       `json:"major"`
	Minor     int       `json:"minor"`
	Patch     int       `json:"patch"`
	Build     string    `json:"build,omitempty"`
	GitCommit string    `json:"git_commit,omitempty"`
	ReleaseAt time.Time `json:"release_at"`
}

// String 返回版本字符串
func (v Version) String() string {
	if v.Build != "" {
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Build)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare 比较版本，返回 >0 表示 v > other, <0 表示 v < other, 0 表示相等
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return v.Major - other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor - other.Minor
	}
	return v.Patch - other.Patch
}

// IsNewerThan 检查是否比另一个版本新
func (v Version) IsNewerThan(other Version) bool {
	return v.Compare(other) > 0
}

// ProbeInfo 探针信息
type ProbeInfo struct {
	ID            string    `json:"id"`
	HostName      string    `json:"hostname"`
	IP            string    `json:"ip"`
	Version       Version   `json:"version"`
	Status        ProbeStatus `json:"status"`
	RegisteredAt  time.Time `json:"registered_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Tags          map[string]string `json:"tags"`
	
	// 配置
	CurrentConfig ConfigVersion `json:"current_config"`
	
	// 性能指标
	Performance ProbePerformance `json:"performance"`
}

// ProbeStatus 探针状态
type ProbeStatus string

const (
	ProbeStatusOnline    ProbeStatus = "online"
	ProbeStatusOffline   ProbeStatus = "offline"
	ProbeStatusUpgrading ProbeStatus = "upgrading"
	ProbeStatusError     ProbeStatus = "error"
	ProbeStatusPaused    ProbeStatus = "paused"
)

// ConfigVersion 配置版本
type ConfigVersion struct {
	Version   string    `json:"version"`
	AppliedAt time.Time `json:"applied_at"`
}

// ============================================================================
// 二、版本管理器
// ============================================================================

// VersionManager 版本管理器
type VersionManager struct {
	mu sync.RWMutex
	
	// 当前版本
	currentVersion Version
	
	// 可用版本列表
	availableVersions []Version
	
	// 版本发布说明
	releaseNotes map[string]string
	
	// 强制升级版本
	mandatoryVersion *Version
}

// NewVersionManager 创建版本管理器
func NewVersionManager(current Version) *VersionManager {
	return &VersionManager{
		currentVersion:    current,
		availableVersions: []Version{},
		releaseNotes:      make(map[string]string),
	}
}

// GetCurrentVersion 获取当前版本
func (vm *VersionManager) GetCurrentVersion() Version {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.currentVersion
}

// RegisterAvailableVersion 注册可用版本
func (vm *VersionManager) RegisterAvailableVersion(v Version, notes string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	
	// 去重并排序
	exists := false
	for i, av := range vm.availableVersions {
		if av.Compare(v) == 0 {
			vm.availableVersions[i] = v
			exists = true
			break
		}
	}
	if !exists {
		vm.availableVersions = append(vm.availableVersions, v)
	}
	
	vm.releaseNotes[v.String()] = notes
	
	// 按版本排序（最新的在前）
	sort.Slice(vm.availableVersions, func(i, j int) bool {
		return vm.availableVersions[i].Compare(vm.availableVersions[j]) > 0
	})
}

// GetLatestVersion 获取最新版本
func (vm *VersionManager) GetLatestVersion() *Version {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	if len(vm.availableVersions) == 0 {
		return nil
	}
	v := vm.availableVersions[0]
	return &v
}

// CheckUpgrade 检查是否需要升级
func (vm *VersionManager) CheckUpgrade() (*UpgradeInfo, error) {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	
	latest := vm.GetLatestVersion()
	if latest == nil {
		return nil, fmt.Errorf("no available version")
	}
	
	if !latest.IsNewerThan(vm.currentVersion) {
		return nil, nil // 无需升级
	}
	
	info := &UpgradeInfo{
		CurrentVersion: vm.currentVersion,
		TargetVersion:  *latest,
		ReleaseNotes:   vm.releaseNotes[latest.String()],
		Mandatory:      vm.mandatoryVersion != nil && latest.Compare(*vm.mandatoryVersion) >= 0,
	}
	
	return info, nil
}

// SetMandatoryVersion 设置强制升级版本
func (vm *VersionManager) SetMandatoryVersion(v Version) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.mandatoryVersion = &v
}

// UpgradeInfo 升级信息
type UpgradeInfo struct {
	CurrentVersion Version  `json:"current_version"`
	TargetVersion  Version  `json:"target_version"`
	ReleaseNotes   string   `json:"release_notes"`
	Mandatory      bool     `json:"mandatory"`
}

// ============================================================================
// 三、自动升级引擎
// ============================================================================

// UpgradeEngine 自动升级引擎
type UpgradeEngine struct {
	mu sync.RWMutex
	
	vm            *VersionManager
	probes        map[string]*ProbeInfo
	
	// 升级策略
	strategy      UpgradeStrategy
	
	// 升级状态
	upgrading     map[string]*UpgradeTask
	upgradeHistory []*UpgradeTask
	
	// 下载函数
	downloadFunc  func(version Version, progress chan float64) (string, error)
	
	// 安装函数
	installFunc   func(packagePath string) error
}

// UpgradeStrategy 升级策略
type UpgradeStrategy struct {
	AutoUpgrade       bool          `json:"auto_upgrade"`       // 是否自动升级
	UpgradeWindow     string        `json:"upgrade_window"`     // 升级窗口(如"02:00-04:00")
	MaxConcurrent     int           `json:"max_concurrent"`       // 最大并发升级数
	RollbackOnFail    bool          `json:"rollback_on_fail"`     // 失败回滚
	HealthCheckAfter  time.Duration `json:"health_check_after"`   // 升级后健康检查时间
}

// DefaultUpgradeStrategy 返回默认升级策略
func DefaultUpgradeStrategy() UpgradeStrategy {
	return UpgradeStrategy{
		AutoUpgrade:      false, // 默认关闭自动升级
		UpgradeWindow:    "02:00-04:00",
		MaxConcurrent:    3,
		RollbackOnFail:   true,
		HealthCheckAfter: 5 * time.Minute,
	}
}

// UpgradeTask 升级任务
type UpgradeTask struct {
	ID          string        `json:"id"`
	ProbeID     string        `json:"probe_id"`
	FromVersion Version       `json:"from_version"`
	ToVersion   Version       `json:"to_version"`
	Status      UpgradeTaskStatus `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at,omitempty"`
	Error       string        `json:"error,omitempty"`
	Progress    float64       `json:"progress"`
}

// UpgradeTaskStatus 升级任务状态
type UpgradeTaskStatus string

const (
	UpgradeStatusPending    UpgradeTaskStatus = "pending"
	UpgradeStatusDownloading UpgradeTaskStatus = "downloading"
	UpgradeStatusInstalling  UpgradeTaskStatus = "installing"
	UpgradeStatusVerifying UpgradeTaskStatus = "verifying"
	UpgradeStatusCompleted  UpgradeTaskStatus = "completed"
	UpgradeStatusFailed     UpgradeTaskStatus = "failed"
	UpgradeStatusRolledBack UpgradeTaskStatus = "rolled_back"
)

// NewUpgradeEngine 创建升级引擎
func NewUpgradeEngine(vm *VersionManager, strategy UpgradeStrategy) *UpgradeEngine {
	return &UpgradeEngine{
		vm:             vm,
		probes:         make(map[string]*ProbeInfo),
		strategy:       strategy,
		upgrading:      make(map[string]*UpgradeTask),
		upgradeHistory: []*UpgradeTask{},
	}
}

// RegisterProbe 注册探针
func (ue *UpgradeEngine) RegisterProbe(probe *ProbeInfo) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.probes[probe.ID] = probe
}

// GetProbe 获取探针信息
func (ue *UpgradeEngine) GetProbe(id string) *ProbeInfo {
	ue.mu.RLock()
	defer ue.mu.RUnlock()
	return ue.probes[id]
}

// GetAllProbes 获取所有探针
func (ue *UpgradeEngine) GetAllProbes() []*ProbeInfo {
	ue.mu.RLock()
	defer ue.mu.RUnlock()
	
	probes := make([]*ProbeInfo, 0, len(ue.probes))
	for _, p := range ue.probes {
		probes = append(probes, p)
	}
	
	sort.Slice(probes, func(i, j int) bool {
		return probes[i].ID < probes[j].ID
	})
	
	return probes
}

// SetDownloadFunc 设置下载函数
func (ue *UpgradeEngine) SetDownloadFunc(fn func(version Version, progress chan float64) (string, error)) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.downloadFunc = fn
}

// SetInstallFunc 设置安装函数
func (ue *UpgradeEngine) SetInstallFunc(fn func(packagePath string) error) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.installFunc = fn
}

// CanUpgradeNow 检查当前是否可以升级
func (ue *UpgradeEngine) CanUpgradeNow() bool {
	if ue.strategy.UpgradeWindow == "" {
		return true
	}
	
	now := time.Now()
	parts := splitWindow(ue.strategy.UpgradeWindow)
	if len(parts) != 2 {
		return true
	}
	
	start, err1 := parseTime(parts[0])
	end, err2 := parseTime(parts[1])
	if err1 != nil || err2 != nil {
		return true
	}
	
	nowTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), 0, 0, time.UTC)
	return nowTime.After(start) && nowTime.Before(end)
}

func splitWindow(window string) []string {
	for _, sep := range []string{"-", "~", " to "} {
		if parts := splitBy(window, sep); len(parts) == 2 {
			return parts
		}
	}
	return nil
}

func splitBy(s, sep string) []string {
	var result []string
	idx := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[idx:i])
			idx = i + len(sep)
		}
	}
	result = append(result, s[idx:])
	return result
}

func parseTime(t string) (time.Time, error) {
	return time.Parse("15:04", t)
}

// StartUpgrade 开始升级指定探针
func (ue *UpgradeEngine) StartUpgrade(probeID string) (*UpgradeTask, error) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	
	probe, ok := ue.probes[probeID]
	if !ok {
		return nil, fmt.Errorf("probe not found: %s", probeID)
	}
	
	info, err := ue.vm.CheckUpgrade()
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, fmt.Errorf("no upgrade available")
	}
	
	// 检查是否已在升级中
	if _, upgrading := ue.upgrading[probeID]; upgrading {
		return nil, fmt.Errorf("probe already upgrading")
	}
	
	// 检查并发数
	if len(ue.upgrading) >= ue.strategy.MaxConcurrent {
		return nil, fmt.Errorf("max concurrent upgrades reached")
	}
	
	task := &UpgradeTask{
		ID:          fmt.Sprintf("upgrade-%d", time.Now().UnixNano()),
		ProbeID:     probeID,
		FromVersion: probe.Version,
		ToVersion:   info.TargetVersion,
		Status:      UpgradeStatusPending,
		StartedAt:   time.Now(),
	}
	
	ue.upgrading[probeID] = task
	ue.upgradeHistory = append(ue.upgradeHistory, task)
	
	// 更新探针状态
	probe.Status = ProbeStatusUpgrading
	
	go ue.executeUpgrade(task)
	
	return task, nil
}

func (ue *UpgradeEngine) executeUpgrade(task *UpgradeTask) {
	// 模拟升级流程
	task.Status = UpgradeStatusDownloading
	task.Progress = 0.1
	
	// 下载
	if ue.downloadFunc != nil {
		progressCh := make(chan float64, 10)
		go func() {
			for p := range progressCh {
				task.Progress = 0.1 + p*0.4
			}
		}()
		_, err := ue.downloadFunc(task.ToVersion, progressCh)
		// 安全关闭channel（可能已经被downloadFunc关闭）
		func() {
			defer func() { recover() }()
			close(progressCh)
		}()
		if err != nil {
			task.Status = UpgradeStatusFailed
			task.Error = fmt.Sprintf("download failed: %v", err)
			ue.finishUpgrade(task)
			return
		}
	}
	
	task.Status = UpgradeStatusInstalling
	task.Progress = 0.5
	
	// 安装
	if ue.installFunc != nil {
		if err := ue.installFunc(""); err != nil {
			task.Status = UpgradeStatusFailed
			task.Error = fmt.Sprintf("install failed: %v", err)
			ue.finishUpgrade(task)
			return
		}
	}
	
	task.Status = UpgradeStatusVerifying
	task.Progress = 0.8
	
	// 验证
	time.Sleep(100 * time.Millisecond) // 模拟验证
	
	task.Status = UpgradeStatusCompleted
	task.Progress = 1.0
	task.CompletedAt = time.Now()
	
	// 更新探针版本
	ue.mu.Lock()
	if probe, ok := ue.probes[task.ProbeID]; ok {
		probe.Version = task.ToVersion
		probe.Status = ProbeStatusOnline
	}
	ue.mu.Unlock()
	
	ue.finishUpgrade(task)
}

func (ue *UpgradeEngine) finishUpgrade(task *UpgradeTask) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	delete(ue.upgrading, task.ProbeID)
}

// GetUpgradeTask 获取升级任务
func (ue *UpgradeEngine) GetUpgradeTask(probeID string) *UpgradeTask {
	ue.mu.RLock()
	defer ue.mu.RUnlock()
	return ue.upgrading[probeID]
}

// GetUpgradeHistory 获取升级历史
func (ue *UpgradeEngine) GetUpgradeHistory(probeID string) []*UpgradeTask {
	ue.mu.RLock()
	defer ue.mu.RUnlock()
	
	var history []*UpgradeTask
	for _, task := range ue.upgradeHistory {
		if task.ProbeID == probeID || probeID == "" {
			history = append(history, task)
		}
	}
	
	// 按时间倒序
	sort.Slice(history, func(i, j int) bool {
		return history[i].StartedAt.After(history[j].StartedAt)
	})
	
	return history
}

// ============================================================================
// 四、配置远程下发
// ============================================================================

// ConfigManager 配置管理器
type ConfigManager struct {
	mu sync.RWMutex
	
	// 配置模板
	templates map[string]*ConfigTemplate
	
	// 探针配置
	probeConfigs map[string]*ProbeConfig
	
	// 配置历史
	configHistory map[string][]*ConfigChangeRecord
}

// ConfigTemplate 配置模板
type ConfigTemplate struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Content     map[string]interface{} `json:"content"`
	Version     string            `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ProbeConfig 探针配置
type ProbeConfig struct {
	ProbeID      string            `json:"probe_id"`
	TemplateID   string            `json:"template_id"`
	Content      map[string]interface{} `json:"content"`
	Version      string            `json:"version"`
	AppliedAt    time.Time         `json:"applied_at"`
	Status       ConfigStatus      `json:"status"`
}

// ConfigStatus 配置状态
type ConfigStatus string

const (
	ConfigStatusPending  ConfigStatus = "pending"
	ConfigStatusApplied  ConfigStatus = "applied"
	ConfigStatusFailed   ConfigStatus = "failed"
	ConfigStatusReverted ConfigStatus = "reverted"
)

// ConfigChangeRecord 配置变更记录
type ConfigChangeRecord struct {
	Version   string    `json:"version"`
	Content   map[string]interface{} `json:"content"`
	AppliedAt time.Time `json:"applied_at"`
	Status    ConfigStatus `json:"status"`
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		templates:     make(map[string]*ConfigTemplate),
		probeConfigs:  make(map[string]*ProbeConfig),
		configHistory: make(map[string][]*ConfigChangeRecord),
	}
}

// RegisterTemplate 注册配置模板
func (cm *ConfigManager) RegisterTemplate(template *ConfigTemplate) error {
	if template.ID == "" {
		return fmt.Errorf("template ID cannot be empty")
	}
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	template.UpdatedAt = time.Now()
	if template.CreatedAt.IsZero() {
		template.CreatedAt = template.UpdatedAt
	}
	
	cm.templates[template.ID] = template
	return nil
}

// GetTemplate 获取配置模板
func (cm *ConfigManager) GetTemplate(id string) *ConfigTemplate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.templates[id]
}

// GetAllTemplates 获取所有模板
func (cm *ConfigManager) GetAllTemplates() []*ConfigTemplate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var templates []*ConfigTemplate
	for _, t := range cm.templates {
		templates = append(templates, t)
	}
	
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].UpdatedAt.After(templates[j].UpdatedAt)
	})
	
	return templates
}

// DeployConfig 下发配置到探针
func (cm *ConfigManager) DeployConfig(probeID, templateID string, content map[string]interface{}) (*ProbeConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	template, ok := cm.templates[templateID]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}
	
	// 合并模板和自定义内容
	merged := make(map[string]interface{})
	for k, v := range template.Content {
		merged[k] = v
	}
	for k, v := range content {
		merged[k] = v
	}
	
	version := fmt.Sprintf("v%d", time.Now().Unix())
	
	config := &ProbeConfig{
		ProbeID:    probeID,
		TemplateID: templateID,
		Content:    merged,
		Version:    version,
		AppliedAt:  time.Now(),
		Status:     ConfigStatusPending,
	}
	
	cm.probeConfigs[probeID] = config
	
	// 记录变更
	record := &ConfigChangeRecord{
		Version:   version,
		Content:   merged,
		AppliedAt: config.AppliedAt,
		Status:    ConfigStatusPending,
	}
	cm.configHistory[probeID] = append(cm.configHistory[probeID], record)
	
	return config, nil
}

// GetProbeConfig 获取探针配置
func (cm *ConfigManager) GetProbeConfig(probeID string) *ProbeConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.probeConfigs[probeID]
}

// ConfirmConfigApplied 确认配置已应用
func (cm *ConfigManager) ConfirmConfigApplied(probeID string, success bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	if config, ok := cm.probeConfigs[probeID]; ok {
		if success {
			config.Status = ConfigStatusApplied
		} else {
			config.Status = ConfigStatusFailed
		}
	}
	
	if history, ok := cm.configHistory[probeID]; ok && len(history) > 0 {
		last := history[len(history)-1]
		if success {
			last.Status = ConfigStatusApplied
		} else {
			last.Status = ConfigStatusFailed
		}
	}
}

// GetConfigHistory 获取配置历史
func (cm *ConfigManager) GetConfigHistory(probeID string) []*ConfigChangeRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.configHistory[probeID]
}

// RollbackConfig 回滚配置
func (cm *ConfigManager) RollbackConfig(probeID string) (*ProbeConfig, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	history := cm.configHistory[probeID]
	if len(history) < 2 {
		return nil, fmt.Errorf("no previous config to rollback")
	}
	
	// 找到上一个成功应用的配置
	var previous *ConfigChangeRecord
	for i := len(history) - 2; i >= 0; i-- {
		if history[i].Status == ConfigStatusApplied {
			previous = history[i]
			break
		}
	}
	
	if previous == nil {
		return nil, fmt.Errorf("no successful config to rollback")
	}
	
	config := cm.probeConfigs[probeID]
	if config != nil {
		config.Content = previous.Content
		config.Version = previous.Version + "-rollback"
		config.AppliedAt = time.Now()
		config.Status = ConfigStatusPending
	}
	
	return config, nil
}

// ============================================================================
// 五、灰度发布
// ============================================================================

// CanaryRelease 灰度发布
type CanaryRelease struct {
	ID          string            `json:"id"`
	Version     Version           `json:"version"`
	Strategy    CanaryStrategy    `json:"strategy"`
	Status      CanaryStatus      `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   time.Time         `json:"started_at,omitempty"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
}

// CanaryStrategy 灰度策略
type CanaryStrategy struct {
	// 按百分比
	Percentage float64 `json:"percentage"` // 0-100
	
	// 按数量
	Count      int     `json:"count"`      // 指定探针数量
	
	// 按标签
	TagSelector map[string]string `json:"tag_selector"` // 标签选择器
	
	// 按探针ID列表
	ProbeIDs   []string `json:"probe_ids"` // 指定探针列表
	
	// 健康检查
	HealthCheckDuration time.Duration `json:"health_check_duration"` // 健康检查持续时间
	SuccessRateThreshold float64     `json:"success_rate_threshold"` // 成功率阈值
	
	// 自动推进
	AutoProgress bool `json:"auto_progress"` // 是否自动推进到下一阶段
}

// CanaryStatus 灰度状态
type CanaryStatus string

const (
	CanaryStatusPending    CanaryStatus = "pending"
	CanaryStatusRunning    CanaryStatus = "running"
	CanaryStatusPaused     CanaryStatus = "paused"
	CanaryStatusPromoted   CanaryStatus = "promoted"   // 全量发布
	CanaryStatusRolledBack CanaryStatus = "rolled_back" // 回滚
)

// CanaryStage 灰度阶段
type CanaryStage struct {
	Percentage float64   `json:"percentage"`
	ProbeIDs   []string  `json:"probe_ids"`
	StartedAt  time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// CanaryManager 灰度管理器
type CanaryManager struct {
	mu sync.RWMutex
	
	releases   map[string]*CanaryRelease
	probes     map[string]*ProbeInfo
	
	// 当前阶段
	currentStages map[string]*CanaryStage
}

// NewCanaryManager 创建灰度管理器
func NewCanaryManager() *CanaryManager {
	return &CanaryManager{
		releases:      make(map[string]*CanaryRelease),
		probes:        make(map[string]*ProbeInfo),
		currentStages: make(map[string]*CanaryStage),
	}
}

// RegisterProbe 注册探针
func (cm *CanaryManager) RegisterProbe(probe *ProbeInfo) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.probes[probe.ID] = probe
}

// CreateRelease 创建灰度发布
func (cm *CanaryManager) CreateRelease(version Version, strategy CanaryStrategy) (*CanaryRelease, error) {
	if strategy.Percentage <= 0 && strategy.Count <= 0 && len(strategy.ProbeIDs) == 0 && len(strategy.TagSelector) == 0 {
		return nil, fmt.Errorf("must specify at least one selection criteria")
	}
	
	release := &CanaryRelease{
		ID:        fmt.Sprintf("canary-%d", time.Now().UnixNano()),
		Version:   version,
		Strategy:  strategy,
		Status:    CanaryStatusPending,
		CreatedAt: time.Now(),
	}
	
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.releases[release.ID] = release
	
	return release, nil
}

// StartRelease 开始灰度发布
func (cm *CanaryManager) StartRelease(releaseID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	release, ok := cm.releases[releaseID]
	if !ok {
		return fmt.Errorf("release not found: %s", releaseID)
	}
	
	release.Status = CanaryStatusRunning
	release.StartedAt = time.Now()
	
	// 选择第一批探针
	selected := cm.selectProbes(release.Strategy)
	stage := &CanaryStage{
		Percentage: release.Strategy.Percentage,
		ProbeIDs:   selected,
		StartedAt:  time.Now(),
	}
	cm.currentStages[releaseID] = stage
	
	return nil
}

func (cm *CanaryManager) selectProbes(strategy CanaryStrategy) []string {
	var selected []string
	
	// 按探针ID精确选择
	if len(strategy.ProbeIDs) > 0 {
		return strategy.ProbeIDs
	}
	
	// 按标签选择
	if len(strategy.TagSelector) > 0 {
		for id, probe := range cm.probes {
			match := true
			for k, v := range strategy.TagSelector {
				if probe.Tags[k] != v {
					match = false
					break
				}
			}
			if match {
				selected = append(selected, id)
			}
		}
		return selected
	}
	
	// 按百分比或数量选择
	allProbes := make([]string, 0, len(cm.probes))
	for id := range cm.probes {
		allProbes = append(allProbes, id)
	}
	
	if strategy.Count > 0 {
		count := strategy.Count
		if count > len(allProbes) {
			count = len(allProbes)
		}
		selected = allProbes[:count]
	} else if strategy.Percentage > 0 {
		count := int(math.Ceil(float64(len(allProbes)) * strategy.Percentage / 100))
		if count > len(allProbes) {
			count = len(allProbes)
		}
		selected = allProbes[:count]
	}
	
	return selected
}

// PromoteRelease 全量发布
func (cm *CanaryManager) PromoteRelease(releaseID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	release, ok := cm.releases[releaseID]
	if !ok {
		return fmt.Errorf("release not found: %s", releaseID)
	}
	
	release.Status = CanaryStatusPromoted
	release.CompletedAt = time.Now()
	
	if stage, ok := cm.currentStages[releaseID]; ok {
		stage.CompletedAt = time.Now()
	}
	
	return nil
}

// RollbackRelease 回滚发布
func (cm *CanaryManager) RollbackRelease(releaseID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	
	release, ok := cm.releases[releaseID]
	if !ok {
		return fmt.Errorf("release not found: %s", releaseID)
	}
	
	release.Status = CanaryStatusRolledBack
	release.CompletedAt = time.Now()
	
	if stage, ok := cm.currentStages[releaseID]; ok {
		stage.CompletedAt = time.Now()
	}
	
	return nil
}

// GetRelease 获取发布
func (cm *CanaryManager) GetRelease(releaseID string) *CanaryRelease {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.releases[releaseID]
}

// GetAllReleases 获取所有发布
func (cm *CanaryManager) GetAllReleases() []*CanaryRelease {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	var releases []*CanaryRelease
	for _, r := range cm.releases {
		releases = append(releases, r)
	}
	
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].CreatedAt.After(releases[j].CreatedAt)
	})
	
	return releases
}

// GetSelectedProbes 获取当前阶段选中的探针
func (cm *CanaryManager) GetSelectedProbes(releaseID string) []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	
	if stage, ok := cm.currentStages[releaseID]; ok {
		return stage.ProbeIDs
	}
	return nil
}

// ============================================================================
// 六、性能影响评估
// ============================================================================

// ProbePerformance 探针性能指标
type ProbePerformance struct {
	CPUPercent     float64 `json:"cpu_percent"`     // CPU使用率
	MemoryMB       float64 `json:"memory_mb"`       // 内存使用MB
	MemoryPercent  float64 `json:"memory_percent"`  // 内存使用率
	DropRate       float64 `json:"drop_rate"`       // 丢包率
	LatencyMs      float64 `json:"latency_ms"`      // 处理延迟
	Throughput     float64 `json:"throughput"`      // 吞吐量(条/秒)
	
	// 基准值
	BaselineCPUPercent    float64 `json:"baseline_cpu_percent"`
	BaselineMemoryMB      float64 `json:"baseline_memory_mb"`
	BaselineLatencyMs     float64 `json:"baseline_latency_ms"`
}

// PerformanceImpact 性能影响评估
type PerformanceImpact struct {
	CPUDelta       float64 `json:"cpu_delta"`       // CPU变化百分点
	MemoryDelta    float64 `json:"memory_delta"`    // 内存变化MB
	LatencyDelta   float64 `json:"latency_delta"`   // 延迟变化ms
	DropRateDelta  float64 `json:"drop_rate_delta"` // 丢包率变化
	
	ImpactLevel    ImpactLevel `json:"impact_level"`  // 影响级别
	Score          float64     `json:"score"`         // 影响评分(0-100)
}

// ImpactLevel 影响级别
type ImpactLevel string

const (
	ImpactLevelNone     ImpactLevel = "none"     // 无影响
	ImpactLevelLow      ImpactLevel = "low"      // 轻微影响
	ImpactLevelMedium   ImpactLevel = "medium"   // 中等影响
	ImpactLevelHigh     ImpactLevel = "high"     // 严重影响
	ImpactLevelCritical ImpactLevel = "critical" // 灾难影响
)

// PerformanceEvaluator 性能评估器
type PerformanceEvaluator struct {
	mu sync.RWMutex
	
	// 基准性能
	baselines map[string]*ProbePerformance
	
	// 告警阈值
	thresholds PerformanceThresholds
}

// PerformanceThresholds 性能阈值
type PerformanceThresholds struct {
	MaxCPUPercent      float64 `json:"max_cpu_percent"`      // 最大CPU使用率
	MaxMemoryPercent   float64 `json:"max_memory_percent"`   // 最大内存使用率
	MaxDropRate        float64 `json:"max_drop_rate"`        // 最大丢包率
	MaxLatencyMs       float64 `json:"max_latency_ms"`        // 最大延迟
	
	CPUDeltaThreshold    float64 `json:"cpu_delta_threshold"`    // CPU变化阈值
	MemoryDeltaThreshold float64 `json:"memory_delta_threshold"` // 内存变化阈值
	LatencyDeltaThreshold float64 `json:"latency_delta_threshold"` // 延迟变化阈值
}

// DefaultPerformanceThresholds 返回默认性能阈值
func DefaultPerformanceThresholds() PerformanceThresholds {
	return PerformanceThresholds{
		MaxCPUPercent:         80.0,
		MaxMemoryPercent:      90.0,
		MaxDropRate:           5.0,
		MaxLatencyMs:          100.0,
		CPUDeltaThreshold:     10.0,
		MemoryDeltaThreshold:  100.0, // MB
		LatencyDeltaThreshold: 20.0,  // ms
	}
}

// NewPerformanceEvaluator 创建性能评估器
func NewPerformanceEvaluator(thresholds PerformanceThresholds) *PerformanceEvaluator {
	return &PerformanceEvaluator{
		baselines:  make(map[string]*ProbePerformance),
		thresholds: thresholds,
	}
}

// SetBaseline 设置基准性能
func (pe *PerformanceEvaluator) SetBaseline(probeID string, baseline *ProbePerformance) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.baselines[probeID] = baseline
}

// Evaluate 评估性能影响
func (pe *PerformanceEvaluator) Evaluate(probeID string, current *ProbePerformance) *PerformanceImpact {
	pe.mu.RLock()
	baseline, ok := pe.baselines[probeID]
	pe.mu.RUnlock()
	
	impact := &PerformanceImpact{}
	
	if !ok || baseline == nil {
		// 没有基准，使用当前值作为评估依据
		impact.Score = pe.calculateScore(current, nil)
		impact.ImpactLevel = pe.scoreToLevel(impact.Score)
		return impact
	}
	
	// 计算变化
	impact.CPUDelta = current.CPUPercent - baseline.BaselineCPUPercent
	impact.MemoryDelta = current.MemoryMB - baseline.BaselineMemoryMB
	impact.LatencyDelta = current.LatencyMs - baseline.BaselineLatencyMs
	impact.DropRateDelta = current.DropRate
	
	// 计算评分
	impact.Score = pe.calculateScore(current, baseline)
	impact.ImpactLevel = pe.scoreToLevel(impact.Score)
	
	return impact
}

func (pe *PerformanceEvaluator) calculateScore(current, baseline *ProbePerformance) float64 {
	score := 0.0
	
	// CPU评分
	if current.CPUPercent > pe.thresholds.MaxCPUPercent {
		score += 30
	} else if current.CPUPercent > pe.thresholds.MaxCPUPercent*0.8 {
		score += 15
	}
	
	// 内存评分
	if current.MemoryPercent > pe.thresholds.MaxMemoryPercent {
		score += 30
	} else if current.MemoryPercent > pe.thresholds.MaxMemoryPercent*0.8 {
		score += 15
	}
	
	// 丢包率评分
	if current.DropRate > pe.thresholds.MaxDropRate {
		score += 25
	} else if current.DropRate > pe.thresholds.MaxDropRate*0.5 {
		score += 10
	}
	
	// 延迟评分
	if current.LatencyMs > pe.thresholds.MaxLatencyMs {
		score += 15
	} else if current.LatencyMs > pe.thresholds.MaxLatencyMs*0.8 {
		score += 5
	}
	
	if baseline != nil {
		// CPU变化
		if math.Abs(impactCPUDelta(current, baseline)) > pe.thresholds.CPUDeltaThreshold {
			score += 10
		}
		
		// 内存变化
		if math.Abs(impactMemoryDelta(current, baseline)) > pe.thresholds.MemoryDeltaThreshold {
			score += 10
		}
	}
	
	if score > 100 {
		score = 100
	}
	return score
}

func impactCPUDelta(current, baseline *ProbePerformance) float64 {
	return current.CPUPercent - baseline.CPUPercent
}

func impactMemoryDelta(current, baseline *ProbePerformance) float64 {
	return current.MemoryMB - baseline.MemoryMB
}

func (pe *PerformanceEvaluator) scoreToLevel(score float64) ImpactLevel {
	switch {
	case score >= 80:
		return ImpactLevelCritical
	case score >= 60:
		return ImpactLevelHigh
	case score >= 40:
		return ImpactLevelMedium
	case score >= 20:
		return ImpactLevelLow
	default:
		return ImpactLevelNone
	}
}

// CheckAlert 检查是否需要告警
func (pe *PerformanceEvaluator) CheckAlert(probeID string, current *ProbePerformance) *PerformanceAlert {
	impact := pe.Evaluate(probeID, current)
	
	if impact.ImpactLevel == ImpactLevelNone {
		return nil
	}
	
	alert := &PerformanceAlert{
		ID:          fmt.Sprintf("perf-%s-%d", probeID, time.Now().Unix()),
		ProbeID:     probeID,
		Level:       impact.ImpactLevel,
		Score:       impact.Score,
		Timestamp:   time.Now(),
		Impact:      *impact,
	}
	
	alert.Recommendations = pe.generateRecommendations(impact)
	
	return alert
}

func (pe *PerformanceEvaluator) generateRecommendations(impact *PerformanceImpact) []string {
	var recs []string
	
	switch impact.ImpactLevel {
	case ImpactLevelCritical:
		recs = append(recs, "【紧急】探针性能严重影响系统，建议立即停止探针")
		recs = append(recs, "检查探针配置是否过于激进")
		recs = append(recs, "考虑减少采集频率或采样率")
	case ImpactLevelHigh:
		recs = append(recs, "探针性能影响较大，建议调整采集策略")
		recs = append(recs, "考虑降低采集频率或启用过滤")
	case ImpactLevelMedium:
		recs = append(recs, "探针性能有轻微影响，建议优化配置")
	case ImpactLevelLow:
		recs = append(recs, "探针性能轻微波动，建议持续监控")
	}
	
	if impact.CPUDelta > 10 {
		recs = append(recs, fmt.Sprintf("CPU使用率增加 %.1f%%，建议优化采集逻辑", impact.CPUDelta))
	}
	if impact.MemoryDelta > 100 {
		recs = append(recs, fmt.Sprintf("内存使用增加 %.1fMB，建议检查内存泄漏", impact.MemoryDelta))
	}
	if impact.LatencyDelta > 20 {
		recs = append(recs, fmt.Sprintf("处理延迟增加 %.1fms，建议优化处理流程", impact.LatencyDelta))
	}
	if impact.DropRateDelta > 1 {
		recs = append(recs, fmt.Sprintf("丢包率增加 %.1f%%，建议增加缓冲区或优化采集频率", impact.DropRateDelta))
	}
	
	return recs
}

// PerformanceAlert 性能告警
type PerformanceAlert struct {
	ID              string          `json:"id"`
	ProbeID         string          `json:"probe_id"`
	Level           ImpactLevel     `json:"level"`
	Score           float64         `json:"score"`
	Timestamp       time.Time       `json:"timestamp"`
	Impact          PerformanceImpact `json:"impact"`
	Recommendations []string        `json:"recommendations"`
	Resolved        bool            `json:"resolved"`
}

// GetBaseline 获取基准性能
func (pe *PerformanceEvaluator) GetBaseline(probeID string) *ProbePerformance {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.baselines[probeID]
}
