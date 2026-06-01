// Package protocol 提供插件管理功能
//
// 管理插件的完整生命周期：发现 → 加载 → 运行 → 健康检查 → 卸载 → 热更新
package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"cloud-flow-agent/pkg/logger"
	edge "cloud-flow/proto"
)

// ============================================================================
// 插件管理器
// ============================================================================

// Manager 插件管理器
type Manager struct {
	mu     sync.RWMutex
	log    *logger.Logger
	config ManagerConfig

	// 运行中的插件实例
	instances map[string]*PluginInstance

	// 协议匹配器
	matcher *ProtocolMatcher

	// 停止信号
	stopCh chan struct{}
	wg     sync.WaitGroup

	// 指标回调
	onMetrics func(metrics []*edge.MetricData)
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	PluginDir      string        // 插件目录
	AutoDiscovery  bool          // 自动发现插件
	CheckInterval  time.Duration // 健康检查间隔
	MaxMemoryMB    int           // 单插件内存限制
	GRPCTimeout    time.Duration // gRPC 通信超时
}

// DefaultManagerConfig 默认管理器配置
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		PluginDir:     "/opt/cloud-flow-agent/plugins",
		AutoDiscovery: true,
		CheckInterval: 30 * time.Second,
		MaxMemoryMB:   256,
		GRPCTimeout:   5 * time.Second,
	}
}

// PluginInstance 运行中的插件实例
type PluginInstance struct {
	Name     string
	Info     PluginInfo
	Config   PluginConfig
	Plugin   Plugin
	Cmd      *exec.Cmd
	Socket   string
	LoadedAt time.Time
	Status   string // running/stopped/error
	Error    error
}

// NewManager 创建插件管理器
func NewManager(cfg ManagerConfig, log *logger.Logger) *Manager {
	if cfg.PluginDir == "" {
		cfg.PluginDir = DefaultManagerConfig().PluginDir
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = DefaultManagerConfig().CheckInterval
	}

	m := &Manager{
		log:       log,
		config:    cfg,
		instances: make(map[string]*PluginInstance),
		matcher:   NewProtocolMatcher(),
		stopCh:    make(chan struct{}),
	}

	// 注册内置匹配规则
	RegisterBuiltinMatchers(m.matcher)

	return m
}

// Start 启动管理器
func (m *Manager) Start() error {
	// 确保插件目录存在
	if err := os.MkdirAll(m.config.PluginDir, 0755); err != nil {
		return fmt.Errorf("创建插件目录失败: %w", err)
	}

	// 自动发现并加载插件
	if m.config.AutoDiscovery {
		if err := m.discoverPlugins(); err != nil {
			m.log.Warnf("[插件管理器] 自动发现插件失败: %v", err)
		}
	}

	// 启动健康检查
	m.wg.Add(1)
	go m.healthCheckLoop()

	m.log.Info("[插件管理器] 已启动")
	return nil
}

// Stop 停止管理器
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()

	// 停止所有插件
	m.mu.Lock()
	for name, inst := range m.instances {
		if inst.Plugin != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			inst.Plugin.Shutdown(ctx)
			cancel()
		}
		if inst.Cmd != nil && inst.Cmd.Process != nil {
			inst.Cmd.Process.Kill()
		}
		inst.Status = "stopped"
	}
	m.mu.Unlock()

	m.log.Info("[插件管理器] 已停止")
}

// OnMetrics 设置指标回调
func (m *Manager) OnMetrics(fn func(metrics []*edge.MetricData)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMetrics = fn
}

// discoverPlugins 发现插件目录中的所有插件
func (m *Manager) discoverPlugins() error {
	entries, err := os.ReadDir(m.config.PluginDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 查找插件清单文件
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			if err := m.loadPluginManifest(filepath.Join(m.config.PluginDir, name)); err != nil {
				m.log.Warnf("[插件管理器] 加载插件清单失败 %s: %v", name, err)
			}
		}
	}

	return nil
}

// PluginManifest 插件清单文件
type PluginManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Binary      string            `json:"binary"`
	Protocol    string            `json:"protocol"`
	Enabled     bool              `json:"enabled"`
	Config      map[string]string `json:"config"`
	Description string            `json:"description"`
}

// loadPluginManifest 加载插件清单
func (m *Manager) loadPluginManifest(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}

	if !manifest.Enabled {
		m.log.Infof("[插件管理器] 插件 %s 已禁用，跳过", manifest.Name)
		return nil
	}

	// 加载插件
	cfg := PluginConfig{
		Enabled:    true,
		BinaryPath: filepath.Join(m.config.PluginDir, manifest.Binary),
		SocketPath: filepath.Join(os.TempDir(), fmt.Sprintf("cloud-flow-%s.sock", manifest.Name)),
		Timeout:    m.config.GRPCTimeout,
		MaxMemoryMB: m.config.MaxMemoryMB,
	}

	if err := m.LoadPlugin(manifest.Name, cfg); err != nil {
		return fmt.Errorf("加载插件 %s 失败: %w", manifest.Name, err)
	}

	m.log.Infof("[插件管理器] 插件已加载: %s v%s", manifest.Name, manifest.Version)
	return nil
}

// LoadPlugin 加载插件
func (m *Manager) LoadPlugin(name string, cfg PluginConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已加载
	if _, exists := m.instances[name]; exists {
		return fmt.Errorf("插件 %s 已加载", name)
	}

	// 从注册中心获取工厂
	registry := GetRegistry()
	factory, exists := registry.plugins[name]
	if !exists {
		// 尝试启动外部插件进程
		plugin, cmd, err := m.startExternalPlugin(name, cfg)
		if err != nil {
			return err
		}

		m.instances[name] = &PluginInstance{
			Name:     name,
			Plugin:   plugin,
			Config:   cfg,
			Cmd:      cmd,
			Socket:   cfg.SocketPath,
			LoadedAt: time.Now(),
			Status:   "running",
		}
		return nil
	}

	// 使用内置插件
	plugin := factory()
	if err := plugin.Init(context.Background(), cfg); err != nil {
		return fmt.Errorf("初始化插件 %s 失败: %w", name, err)
	}

	info := plugin.Info()
	m.instances[name] = &PluginInstance{
		Name:     name,
		Info:     info,
		Plugin:   plugin,
		Config:   cfg,
		LoadedAt: time.Now(),
		Status:   "running",
	}

	return nil
}

// startExternalPlugin 启动外部插件进程
func (m *Manager) startExternalPlugin(name string, cfg PluginConfig) (Plugin, *exec.Cmd, error) {
	if _, err := os.Stat(cfg.BinaryPath); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("插件二进制不存在: %s", cfg.BinaryPath)
	}

	// 启动插件进程
	cmd := exec.Command(cfg.BinaryPath,
		"--socket", cfg.SocketPath,
		"--name", name,
	)
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("启动插件进程失败: %w", err)
	}

	// 等待插件就绪（检查 socket 文件）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(cfg.SocketPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 创建 gRPC 客户端连接
	// 实际实现中使用 hashicorp/go-plugin
	// 这里简化为创建占位实例
	plugin := &externalPluginClient{
		name:      name,
		socket:    cfg.SocketPath,
	}

	return plugin, cmd, nil
}

// UnloadPlugin 卸载插件
func (m *Manager) UnloadPlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, exists := m.instances[name]
	if !exists {
		return fmt.Errorf("插件 %s 未加载", name)
	}

	// 优雅关闭
	if inst.Plugin != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		inst.Plugin.Shutdown(ctx)
		cancel()
	}

	// 终止进程
	if inst.Cmd != nil && inst.Cmd.Process != nil {
		inst.Cmd.Process.Kill()
	}

	// 清理 socket
	if inst.Socket != "" {
		os.Remove(inst.Socket)
	}

	inst.Status = "stopped"
	delete(m.instances, name)

	m.log.Infof("[插件管理器] 插件已卸载: %s", name)
	return nil
}

// ReloadPlugin 热更新插件
func (m *Manager) ReloadPlugin(name string) error {
	m.mu.RLock()
	inst, exists := m.instances[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("插件 %s 未加载", name)
	}

	// 保存配置
	cfg := inst.Config

	// 卸载旧版本
	if err := m.UnloadPlugin(name); err != nil {
		return err
	}

	// 加载新版本
	if err := m.LoadPlugin(name, cfg); err != nil {
		return err
	}

	m.log.Infof("[插件管理器] 插件已热更新: %s", name)
	return nil
}

// Parse 使用所有已加载插件解析数据包
func (m *Manager) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) ([]*ParseResult, error) {
	// 快速匹配协议
	protocol, score := m.matcher.Match(data, metadata.DstPort)
	if protocol == "" || score < 0.5 {
		return nil, nil
	}

	m.mu.RLock()
	inst, exists := m.instances[protocol]
	m.mu.RUnlock()

	if !exists || inst.Plugin == nil || inst.Status != "running" {
		return nil, nil
	}

	result, err := inst.Plugin.Parse(ctx, data, metadata)
	if err != nil {
		return nil, err
	}

	return []*ParseResult{result}, nil
}

// healthCheckLoop 健康检查循环
func (m *Manager) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAllPlugins()
		case <-m.stopCh:
			return
		}
	}
}

// checkAllPlugins 检查所有插件健康状态
func (m *Manager) checkAllPlugins() {
	m.mu.RLock()
	instances := make(map[string]*PluginInstance, len(m.instances))
	for k, v := range m.instances {
		instances[k] = v
	}
	m.mu.RUnlock()

	for name, inst := range instances {
		if inst.Plugin == nil || inst.Status != "running" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		status, err := inst.Plugin.HealthCheck(ctx)
		cancel()

		if err != nil {
			m.log.Warnf("[插件管理器] 插件 %s 健康检查失败: %v", name, err)
			inst.Status = "error"
			inst.Error = err

			// 尝试自动重启
			m.log.Infof("[插件管理器] 尝试重启插件: %s", name)
			if restartErr := m.RestartPlugin(name); restartErr != nil {
				m.log.Errorf("[插件管理器] 重启插件 %s 失败: %v", name, restartErr)
			}
		} else {
			inst.Status = status.Status
		}
	}
}

// RestartPlugin 重启插件
func (m *Manager) RestartPlugin(name string) error {
	m.mu.RLock()
	inst, exists := m.instances[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("插件 %s 未加载", name)
	}

	cfg := inst.Config

	// 停止
	if inst.Plugin != nil {
		inst.Plugin.Shutdown(context.Background())
	}
	if inst.Cmd != nil && inst.Cmd.Process != nil {
		inst.Cmd.Process.Kill()
	}

	// 重新加载
	return m.LoadPlugin(name, cfg)
}

// ListPlugins 列出所有插件状态
func (m *Manager) ListPlugins() []PluginInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PluginInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, *inst)
	}
	return result
}

// GetPlugin 获取插件实例
func (m *Manager) GetPlugin(name string) (Plugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, exists := m.instances[name]
	if !exists || inst.Plugin == nil {
		return nil, false
	}
	return inst.Plugin, true
}

// externalPluginClient 外部插件客户端（占位实现）
type externalPluginClient struct {
	name   string
	socket string
}

func (c *externalPluginClient) Info() PluginInfo {
	return PluginInfo{Name: c.name, Protocol: c.name, Version: "external"}
}

func (c *externalPluginClient) Init(ctx context.Context, cfg PluginConfig) error {
	return nil
}

func (c *externalPluginClient) Parse(ctx context.Context, data []byte, metadata *PacketMetadata) (*ParseResult, error) {
	return nil, fmt.Errorf("外部插件客户端待实现")
}

func (c *externalPluginClient) ParseBatch(ctx context.Context, packets []*PacketInput) ([]*ParseResult, error) {
	return nil, nil
}

func (c *externalPluginClient) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return &HealthStatus{Status: "healthy"}, nil
}

func (c *externalPluginClient) Shutdown(ctx context.Context) error {
	return nil
}

// 确保 externalPluginClient 实现 Plugin 接口
var _ Plugin = (*externalPluginClient)(nil)
