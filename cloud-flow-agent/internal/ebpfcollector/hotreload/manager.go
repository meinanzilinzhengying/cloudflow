// Package hotreload eBPF 程序热更新管理器
package hotreload

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// SubsystemType 子系统类型枚举
type SubsystemType string

const (
	SubsystemTC       SubsystemType = "tc"
	SubsystemTCP      SubsystemType = "tcp"
	SubsystemHTTP     SubsystemType = "http"
	SubsystemHTTPFull SubsystemType = "http_full"
	SubsystemDNS      SubsystemType = "dns"
	SubsystemMySQL    SubsystemType = "mysql"
)

// ProgramState 程序状态枚举
type ProgramState int

const (
	StateLoading  ProgramState = 0
	StateRunning  ProgramState = 1
	StateReloading ProgramState = 2
	StateRollback ProgramState = 3
	StateFailed   ProgramState = 4
)

func (s ProgramState) String() string {
	switch s {
	case StateLoading:
		return "loading"
	case StateRunning:
		return "running"
	case StateReloading:
		return "reloading"
	case StateRollback:
		return "rollback"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SubsystemConfig 子系统配置
type SubsystemConfig struct {
	Type         SubsystemType
	Enabled      bool
	MapNames     []string // maps that need pinning
	ProgramNames []string // programs that need attaching
	LoadFunc     func(bytecode []byte) (interface{}, error)                     // function to load the subsystem
	AttachFunc   func(obj interface{}) ([]interface{}, error)                   // function to attach programs, returns links
	DetachFunc   func(links []interface{}) error                                // function to detach programs
	CloseFunc    func(obj interface{}) error                                    // function to close the object
}

// ReloadResult 热更新结果
type ReloadResult struct {
	Success         bool
	Version         string
	Duration        time.Duration
	MigratedEntries int
	Error           string
}

// HealthStatus 健康检查状态
type HealthStatus struct {
	Healthy       bool
	Subsystem     string
	EventsPerSec  float64
	LastEventTime int64
	ErrorCount    int
	CheckedAt     int64
}

// ReloadRequest 异步热更新请求
type ReloadRequest struct {
	Subsystem SubsystemType
	Bytecode  []byte
	Version   string
	Force     bool
	ResultCh  chan *ReloadResult
}

// ProgramManager eBPF 程序热更新管理器（主协调器）
type ProgramManager struct {
	bpffs          *BpffsManager
	pinning        *MapPinning
	subsystems     map[SubsystemType]*SubsystemConfig
	loadedObjects  map[SubsystemType]interface{}   // currently loaded eBPF objects
	activeLinks    map[SubsystemType][]interface{}  // currently attached links
	currentVersion string
	previousVersion string
	state          ProgramState
	stateMu        sync.RWMutex
	healthCheckInterval time.Duration
	reloadCh       chan ReloadRequest
	stopCh         chan struct{}
	healthResults  map[SubsystemType]*HealthStatus
	healthMu       sync.RWMutex
}

// NewProgramManager 创建新的 ProgramManager 实例
func NewProgramManager(bpffsMountPoint string) *ProgramManager {
	return &ProgramManager{
		bpffs:              NewBpffsManager(bpffsMountPoint),
		pinning:            NewMapPinning(bpffsMountPoint),
		subsystems:         make(map[SubsystemType]*SubsystemConfig),
		loadedObjects:      make(map[SubsystemType]interface{}),
		activeLinks:        make(map[SubsystemType][]interface{}),
		state:              StateLoading,
		healthCheckInterval: 10 * time.Second,
		reloadCh:           make(chan ReloadRequest, 16),
		stopCh:             make(chan struct{}),
		healthResults:      make(map[SubsystemType]*HealthStatus),
	}
}

// RegisterSubsystem 注册子系统用于热更新
func (pm *ProgramManager) RegisterSubsystem(config SubsystemConfig) {
	pm.subsystems[config.Type] = &config
	pm.healthResults[config.Type] = &HealthStatus{
		Healthy:   false,
		Subsystem: string(config.Type),
		CheckedAt: time.Now().Unix(),
	}
}

// Start 启动健康检查循环和热更新处理器
func (pm *ProgramManager) Start(ctx context.Context) {
	pm.setState(StateRunning)

	// 启动健康检查循环
	go pm.healthCheckLoop(ctx)

	// 启动热更新请求处理器
	go pm.reloadProcessor(ctx)
}

// Stop 停止所有 goroutine
func (pm *ProgramManager) Stop() {
	pm.setState(StateLoading)
	close(pm.stopCh)
}

// reloadProcessor 处理异步热更新请求
func (pm *ProgramManager) reloadProcessor(ctx context.Context) {
	for {
		select {
		case <-pm.stopCh:
			return
		case req := <-pm.reloadCh:
			result := pm.ReloadSubsystem(ctx, req.Subsystem, req.Bytecode, req.Version)
			if req.ResultCh != nil {
				req.ResultCh <- result
			}
		}
	}
}

// ReloadSubsystem 主热更新方法
// 步骤:
//  1. 设置状态为 Reloading
//  2. 从字节码加载新的 eBPF 对象
//  3. 将新 map pin 到 bpffs（先带版本后缀，再原子重命名）
//  4. 从旧 map 迁移数据到新 map（状态保留）
//  5. 挂载新程序
//  6. 运行健康检查（等待事件，超时 5s）
//  7. 如果健康: 卸载旧程序，关闭旧对象，更新版本
//  8. 如果不健康: 卸载新程序，关闭新对象，保留旧版本（回滚）
//  9. 设置状态为 Running 或 Failed
// 10. 返回 ReloadResult
func (pm *ProgramManager) ReloadSubsystem(ctx context.Context, subsystem SubsystemType, bytecode []byte, version string) *ReloadResult {
	startTime := time.Now()
	result := &ReloadResult{
		Version: version,
	}

	config, ok := pm.subsystems[subsystem]
	if !ok {
		result.Error = fmt.Sprintf("subsystem %s not registered", subsystem)
		result.Duration = time.Since(startTime)
		return result
	}

	if !config.Enabled {
		result.Error = fmt.Sprintf("subsystem %s is not enabled", subsystem)
		result.Duration = time.Since(startTime)
		return result
	}

	pm.setState(StateReloading)

	// 保存旧对象和链接的引用，用于回滚
	oldObj := pm.loadedObjects[subsystem]
	oldLinks := pm.activeLinks[subsystem]

	// 步骤 2: 加载新的 eBPF 对象
	newObj, err := config.LoadFunc(bytecode)
	if err != nil {
		result.Error = fmt.Sprintf("failed to load new eBPF object: %v", err)
		pm.setState(StateFailed)
		result.Duration = time.Since(startTime)
		return result
	}

	// 步骤 3 & 4: Pin 新 map 并迁移数据
	migratedEntries, err := pm.pinAndMigrateMaps(subsystem, oldObj, newObj, version)
	if err != nil {
		// 加载失败，关闭新对象
		_ = config.CloseFunc(newObj)
		result.Error = fmt.Sprintf("failed to pin and migrate maps: %v", err)
		pm.setState(StateFailed)
		result.Duration = time.Since(startTime)
		return result
	}
	result.MigratedEntries = migratedEntries

	// 步骤 5: 挂载新程序
	newLinks, err := config.AttachFunc(newObj)
	if err != nil {
		// 挂载失败，关闭新对象
		_ = config.CloseFunc(newObj)
		result.Error = fmt.Sprintf("failed to attach new programs: %v", err)
		pm.setState(StateFailed)
		result.Duration = time.Since(startTime)
		return result
	}

	// 步骤 6: 运行健康检查（等待事件，超时 5s）
	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	healthStatus := pm.runHealthCheckWithTimeout(healthCtx, subsystem)
	healthCancel()

	// 步骤 7 & 8: 根据健康检查结果决定提交或回滚
	if healthStatus.Healthy {
		// 健康: 卸载旧程序，关闭旧对象
		if oldLinks != nil {
			_ = config.DetachFunc(oldLinks)
		}
		if oldObj != nil {
			_ = config.CloseFunc(oldObj)
		}

		// 更新状态
		pm.previousVersion = pm.currentVersion
		pm.currentVersion = version
		pm.loadedObjects[subsystem] = newObj
		pm.activeLinks[subsystem] = newLinks

		pm.setState(StateRunning)
		result.Success = true
	} else {
		// 不健康: 回滚 - 卸载新程序，关闭新对象，保留旧版本
		_ = config.DetachFunc(newLinks)
		_ = config.CloseFunc(newObj)

		// 重新挂载旧程序（如果旧链接已被意外关闭）
		if len(pm.activeLinks[subsystem]) == 0 && oldObj != nil {
			reattachedLinks, reattachErr := config.AttachFunc(oldObj)
			if reattachErr != nil {
				result.Error = fmt.Sprintf("health check failed and rollback reattach failed: %v (original error: %s)",
					reattachErr, healthStatus.Subsystem)
			} else {
				pm.activeLinks[subsystem] = reattachedLinks
			}
		} else {
			pm.activeLinks[subsystem] = oldLinks
		}

		pm.setState(StateFailed)
		result.Error = fmt.Sprintf("health check failed after reload: subsystem not producing events")
	}

	result.Duration = time.Since(startTime)
	return result
}

// pinAndMigrateMaps 将新 map pin 到 bpffs 并从旧 map 迁移数据
// 原子替换逻辑:
//   - map 通过 bpffs pin，在程序替换后仍然存活
//   - 步骤:
//     a. 加载新对象（创建新 map + 程序）
//     b. 对于新对象中的每个 map:
//        - 检查 bpffs 中是否存在同名的旧 map
//        - 如果存在: 从旧 pinned map 迁移数据到新 map
//        - 将新 map pin 到 bpffs（覆盖旧 pin）
//     c. 挂载新程序（它们将使用新 pinned 的 map）
//     d. 健康检查新程序
//     e. 如果 OK: 卸载旧链接，关闭旧对象
//     f. 如果 NOT OK: 卸载新链接，关闭新对象，重新挂载旧链接
func (pm *ProgramManager) pinAndMigrateMaps(subsystem SubsystemType, oldObj, newObj interface{}, version string) (int, error) {
	config := pm.subsystems[subsystem]
	totalMigrated := 0

	for _, mapName := range config.MapNames {
		// 尝试从旧对象获取 map 并迁移数据
		if oldObj != nil {
			migrated, err := pm.pinning.MigrateMapData(oldObj, newObj, mapName)
			if err != nil {
				// 迁移失败不致命，记录警告继续
				continue
			}
			totalMigrated += migrated
		}

		// Pin 新 map 到 bpffs（覆盖旧 pin）
		err := pm.pinning.PinMap(newObj, mapName)
		if err != nil {
			return totalMigrated, fmt.Errorf("failed to pin map %s: %w", mapName, err)
		}
	}

	return totalMigrated, nil
}

// Rollback 回滚到上一个版本
func (pm *ProgramManager) Rollback(ctx context.Context, subsystem SubsystemType) *ReloadResult {
	config, ok := pm.subsystems[subsystem]
	if !ok {
		return &ReloadResult{
			Error: fmt.Sprintf("subsystem %s not registered", subsystem),
		}
	}

	if pm.previousVersion == "" {
		return &ReloadResult{
			Error: "no previous version to rollback to",
		}
	}

	pm.setState(StateRollback)

	// 保存当前对象和链接用于回滚失败时恢复
	currentObj := pm.loadedObjects[subsystem]
	currentLinks := pm.activeLinks[subsystem]

	// 关闭当前程序
	if currentLinks != nil {
		_ = config.DetachFunc(currentLinks)
	}
	if currentObj != nil {
		_ = config.CloseFunc(currentObj)
	}

	// 清理当前状态
	delete(pm.loadedObjects, subsystem)
	delete(pm.activeLinks, subsystem)

	// 注意: 回滚到上一个版本需要重新加载之前的字节码。
	// 这里将 previousVersion 设为 currentVersion，表示回滚完成。
	// 实际的重新加载由调用者通过 ReloadSubsystem 完成。
	pm.currentVersion = pm.previousVersion
	pm.previousVersion = ""

	pm.setState(StateRunning)

	return &ReloadResult{
		Success: true,
		Version: pm.currentVersion,
	}
}

// RollbackAll 回滚所有子系统
func (pm *ProgramManager) RollbackAll(ctx context.Context) []*ReloadResult {
	var results []*ReloadResult

	for subsystemType := range pm.subsystems {
		result := pm.Rollback(ctx, subsystemType)
		results = append(results, result)
	}

	return results
}

// GetState 获取当前程序状态
func (pm *ProgramManager) GetState() ProgramState {
	pm.stateMu.RLock()
	defer pm.stateMu.RUnlock()
	return pm.state
}

// GetVersion 获取当前版本
func (pm *ProgramManager) GetVersion() string {
	pm.stateMu.RLock()
	defer pm.stateMu.RUnlock()
	return pm.currentVersion
}

// GetHealthStatus 获取子系统健康状态
func (pm *ProgramManager) GetHealthStatus(subsystem SubsystemType) *HealthStatus {
	pm.healthMu.RLock()
	defer pm.healthMu.RUnlock()

	status, ok := pm.healthResults[subsystem]
	if !ok {
		return &HealthStatus{
			Healthy:   false,
			Subsystem: string(subsystem),
			CheckedAt: time.Now().Unix(),
		}
	}

	// 返回副本以避免外部修改
	copy := *status
	return &copy
}

// IsHealthy 检查所有子系统是否健康
func (pm *ProgramManager) IsHealthy() bool {
	pm.healthMu.RLock()
	defer pm.healthMu.RUnlock()

	for _, status := range pm.healthResults {
		if !status.Healthy {
			return false
		}
	}
	return true
}

// AsyncReload 非阻塞热更新
func (pm *ProgramManager) AsyncReload(subsystem SubsystemType, bytecode []byte, version string) <-chan *ReloadResult {
	resultCh := make(chan *ReloadResult, 1)

	req := ReloadRequest{
		Subsystem: subsystem,
		Bytecode:  bytecode,
		Version:   version,
		ResultCh:  resultCh,
	}

	select {
	case pm.reloadCh <- req:
		// 请求已入队
	default:
		// 通道已满，返回错误
		resultCh <- &ReloadResult{
			Error: "reload queue is full, try again later",
		}
	}

	return resultCh
}

// setState 线程安全地设置程序状态
func (pm *ProgramManager) setState(state ProgramState) {
	pm.stateMu.Lock()
	defer pm.stateMu.Unlock()
	pm.state = state
}

// runHealthCheck 检查子系统是否正在产生事件
// 健康检查策略: 读取 map 计数器，如果正在增长则程序健康
func (pm *ProgramManager) runHealthCheck(subsystem SubsystemType) *HealthStatus {
	config, ok := pm.subsystems[subsystem]
	if !ok || !config.Enabled {
		return &HealthStatus{
			Healthy:   false,
			Subsystem: string(subsystem),
			CheckedAt: time.Now().Unix(),
		}
	}

	status := &HealthStatus{
		Subsystem: string(subsystem),
		CheckedAt: time.Now().Unix(),
	}

	obj := pm.loadedObjects[subsystem]
	if obj == nil {
		status.Healthy = false
		return status
	}

	// 读取事件计数 map 来判断健康状态
	// 策略: 读取一个计数器 map，如果值在增长则程序健康
	counterMapName := config.MapNames[0] // 使用第一个 map 作为事件计数器
	if counterMapName == "" {
		// 没有可用的 map 用于健康检查
		status.Healthy = true // 无法检查时默认健康
		return status
	}

	// 尝试从 eBPF 对象中获取 map 并读取计数器
	// 这里使用反射方式通过 ebpf 包的 Map 来读取
	counterValue, err := pm.readEventCounter(obj, counterMapName)
	if err != nil {
		status.Healthy = false
		status.ErrorCount++
		return status
	}

	pm.healthMu.RLock()
	prevStatus := pm.healthResults[subsystem]
	pm.healthMu.RUnlock()

	if prevStatus != nil && prevStatus.LastEventTime > 0 {
		// 如果计数器在增长，认为程序健康
		if counterValue > prevStatus.LastEventTime {
			status.Healthy = true
			status.EventsPerSec = float64(counterValue-prevStatus.LastEventTime) /
				float64(time.Now().Unix()-prevStatus.CheckedAt+1)
		} else {
			status.Healthy = false
		}
	} else {
		// 第一次检查，只要有计数就认为健康
		status.Healthy = counterValue > 0
	}

	status.LastEventTime = counterValue

	// 更新存储的健康状态
	pm.healthMu.Lock()
	pm.healthResults[subsystem] = status
	pm.healthMu.Unlock()

	return status
}

// runHealthCheckWithTimeout 带超时的健康检查
func (pm *ProgramManager) runHealthCheckWithTimeout(ctx context.Context, subsystem SubsystemType) *HealthStatus {
	type healthResult struct {
		status *HealthStatus
	}

	resultCh := make(chan healthResult, 1)

	go func() {
		// 在超时范围内多次检查，等待事件产生
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				resultCh <- healthResult{
					status: &HealthStatus{
						Healthy:   false,
						Subsystem: string(subsystem),
						Error:     "health check timeout",
						CheckedAt: time.Now().Unix(),
					},
				}
				return
			case <-ticker.C:
				status := pm.runHealthCheck(subsystem)
				if status.Healthy {
					resultCh <- healthResult{status: status}
					return
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return &HealthStatus{
			Healthy:   false,
			Subsystem: string(subsystem),
			Error:     "health check timed out",
			CheckedAt: time.Now().Unix(),
		}
	case result := <-resultCh:
		return result.status
	}
}

// healthCheckLoop 定期对所有子系统进行健康检查
func (pm *ProgramManager) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(pm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			for subsystemType := range pm.subsystems {
				_ = pm.runHealthCheck(subsystemType)
			}
		}
	}
}

// readEventCounter 从 eBPF map 中读取事件计数器
func (pm *ProgramManager) readEventCounter(obj interface{}, mapName string) (int64, error) {
	// 通过类型断言获取 ebpf.Collection 或 ebpf.Map
	// 支持多种 eBPF 对象类型
	switch v := obj.(type) {
	case interface{ Map(string) (*ebpf.Map, error) }:
		m, err := v.Map(mapName)
		if err != nil {
			return 0, fmt.Errorf("failed to get map %s: %w", mapName, err)
		}
		return pm.readCounterFromMap(m)
	default:
		return 0, fmt.Errorf("unsupported eBPF object type for map reading")
	}
}

// readCounterFromMap 从单个 eBPF map 读取事件计数
func (pm *ProgramManager) readCounterFromMap(m *ebpf.Map) (int64, error) {
	if m == nil {
		return 0, fmt.Errorf("map is nil")
	}

	var counter uint64
	// 使用 key=0 读取计数器值（常见的事件计数 map 模式）
	key := uint32(0)
	err := m.Lookup(&key, &counter)
	if err != nil {
		// 尝试使用 uint64 key
		key64 := uint64(0)
		err = m.Lookup(&key64, &counter)
		if err != nil {
			return 0, fmt.Errorf("failed to lookup counter in map: %w", err)
		}
	}

	return int64(counter), nil
}
