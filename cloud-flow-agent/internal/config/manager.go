// Package config 提供远程配置管理功能
package config

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

type ConfigListener interface {
	OnConfigUpdate(oldCfg, newCfg *CollectionConfig)
}

type ConfigListenerFunc func(oldCfg, newCfg *CollectionConfig)

func (f ConfigListenerFunc) OnConfigUpdate(oldCfg, newCfg *CollectionConfig) {
	f(oldCfg, newCfg)
}

type Manager struct {
	mu            sync.RWMutex
	log           *logger.Logger
	currentConfig atomic.Value
	localConfig   *Config
	listeners     []ConfigListener
	version       int64
	groupID       string
	probeID       string
	updateCh      chan *CollectionConfig
	stopCh        chan struct{}
	client        ConfigClient
}

type ConfigClient interface {
	GetConfig(ctx context.Context, req *edge.GetConfigRequest) (*edge.GetConfigResponse, error)
}

type CollectionConfig struct {
	Version               int64
	GroupId               string
	UpdatedAt             int64
	UpdatedBy             string
	SampleRate            float64
	TCPSampleRate         float64
	HTTPSampleRate        float64
	EnableTCPMetrics      bool
	EnableHTTPMetrics     bool
	EnableHTTPFull        bool
	EnableDNSFull         bool
	EnableMySQLFull       bool
	EnableSQLAggregator   bool
	EnableCPUProfiler     bool
	MaxCPUCore            float64
	MaxMemoryMB           float64
	MaxGoroutines         int
	CollectInterval       int
	BatchSize             int
	CircuitBreakerEnabled bool
}

func NewConfigManager(localCfg *Config, probeID, groupID string, log *logger.Logger) *Manager {
	m := &Manager{
		localConfig: localCfg,
		probeID:     probeID,
		groupID:     groupID,
		log:         log,
		listeners:   make([]ConfigListener, 0),
		updateCh:    make(chan *CollectionConfig, 10),
		stopCh:      make(chan struct{}),
	}
	defaultCfg := m.buildDefaultConfig()
	m.currentConfig.Store(defaultCfg)
	m.version = defaultCfg.Version
	return m
}

func (m *Manager) SetClient(client ConfigClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = client
}

func (m *Manager) Start() {
	go m.updateLoop()
	m.log.Info("[配置管理器] 已启动")
}

func (m *Manager) Stop() {
	close(m.stopCh)
	m.log.Info("[配置管理器] 已停止")
}

func (m *Manager) GetConfig() *CollectionConfig {
	cfg := m.currentConfig.Load()
	if cfg == nil {
		return m.buildDefaultConfig()
	}
	return cfg.(*CollectionConfig)
}

func (m *Manager) GetVersion() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

func (m *Manager) AddListener(listener ConfigListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}

func (m *Manager) RemoveListener(listener ConfigListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, l := range m.listeners {
		if l == listener {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			break
		}
	}
}

func (m *Manager) UpdateConfig(newCfg *CollectionConfig) error {
	if newCfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	current := m.GetConfig()
	if newCfg.Version <= current.Version && !m.isForceUpdate(newCfg) {
		m.log.Debugf("[配置管理器] 忽略过期配置: current=%d, new=%d", current.Version, newCfg.Version)
		return nil
	}
	select {
	case m.updateCh <- newCfg:
		return nil
	case <-m.stopCh:
		return fmt.Errorf("配置管理器已停止")
	default:
		return fmt.Errorf("更新通道已满")
	}
}

func (m *Manager) FetchConfig(ctx context.Context) (*CollectionConfig, error) {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		return nil, fmt.Errorf("gRPC客户端未初始化")
	}
	req := &edge.GetConfigRequest{
		ProbeId: m.probeID,
		GroupId: m.groupID,
		Version: m.GetVersion(),
	}
	resp, err := client.GetConfig(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("拉取配置失败: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("拉取配置被拒绝: %s", resp.Message)
	}
	if !resp.HasUpdate || resp.Config == nil {
		return nil, nil
	}
	return m.protoToLocalConfig(resp.Config), nil
}

func (m *Manager) updateLoop() {
	for {
		select {
		case newCfg := <-m.updateCh:
			m.applyConfig(newCfg)
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) applyConfig(newCfg *CollectionConfig) {
	oldCfg := m.GetConfig()
	if err := m.validateConfig(newCfg); err != nil {
		m.log.Errorf("[配置管理器] 配置验证失败: %v", err)
		return
	}
	m.currentConfig.Store(newCfg)
	m.mu.Lock()
	m.version = newCfg.Version
	m.mu.Unlock()
	m.log.Infof("[配置管理器] 配置已更新: version=%d, group=%s", newCfg.Version, newCfg.GroupId)
	m.notifyListeners(oldCfg, newCfg)
}

func (m *Manager) notifyListeners(oldCfg, newCfg *CollectionConfig) {
	m.mu.RLock()
	listeners := make([]ConfigListener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()
	for _, listener := range listeners {
		go func(l ConfigListener) {
			defer func() {
				if r := recover(); r != nil {
					m.log.Errorf("[配置管理器] 监听器panic: %v", r)
				}
			}()
			l.OnConfigUpdate(oldCfg, newCfg)
		}(listener)
	}
}

func (m *Manager) validateConfig(cfg *CollectionConfig) error {
	if cfg.SampleRate < 0 || cfg.SampleRate > 1 {
		return fmt.Errorf("采样率必须在0-1之间: %f", cfg.SampleRate)
	}
	if cfg.CollectInterval < 1 {
		return fmt.Errorf("采集间隔至少为1秒: %d", cfg.CollectInterval)
	}
	if cfg.BatchSize < 1 {
		return fmt.Errorf("批处理大小至少为1: %d", cfg.BatchSize)
	}
	return nil
}

func (m *Manager) isForceUpdate(cfg *CollectionConfig) bool {
	return cfg.Version < 0
}

func (m *Manager) buildDefaultConfig() *CollectionConfig {
	local := m.localConfig
	if local == nil {
		return &CollectionConfig{Version: 1, SampleRate: 1.0, MaxCPUCore: 1.0, MaxMemoryMB: 1024}
	}
	return &CollectionConfig{
		Version:               1,
		SampleRate:            local.EBPF.PerfOptimizer.SampleRate,
		TCPSampleRate:         local.EBPF.PerfOptimizer.SampleRate,
		HTTPSampleRate:        local.EBPF.PerfOptimizer.SampleRate,
		EnableTCPMetrics:      local.EBPF.TCPMetrics.Enabled,
		EnableHTTPMetrics:     local.EBPF.HTTPMetrics.Enabled,
		EnableHTTPFull:        local.EBPF.ProtocolParsing.HTTPFull,
		EnableDNSFull:         local.EBPF.ProtocolParsing.DNSFull,
		EnableMySQLFull:       local.EBPF.ProtocolParsing.MySQLFull,
		EnableSQLAggregator:   local.EBPF.SQLAggregator.Enabled,
		EnableCPUProfiler:     local.EBPF.CPUProfiler.Enabled,
		MaxCPUCore:            local.EBPF.ResourceLimit.MaxCPUCore,
		MaxMemoryMB:           local.EBPF.ResourceLimit.MaxMemoryMB,
		MaxGoroutines:         local.EBPF.ResourceLimit.MaxGoroutines,
		CollectInterval:       local.CollectInterval,
		BatchSize:             local.BatchSize,
		CircuitBreakerEnabled: local.EBPF.CircuitBreaker.Enabled,
	}
}

func (m *Manager) protoToLocalConfig(protoCfg *edge.CollectionConfig) *CollectionConfig {
	return &CollectionConfig{
		Version:               protoCfg.Version,
		GroupId:               protoCfg.GroupId,
		UpdatedAt:             protoCfg.UpdatedAt,
		UpdatedBy:             protoCfg.UpdatedBy,
		SampleRate:            protoCfg.SampleRate,
		TCPSampleRate:         protoCfg.TCPSampleRate,
		HTTPSampleRate:        protoCfg.HTTPSampleRate,
		EnableTCPMetrics:      protoCfg.EnableTCPMetrics,
		EnableHTTPMetrics:     protoCfg.EnableHTTPMetrics,
		EnableHTTPFull:        protoCfg.EnableHTTPFull,
		EnableDNSFull:         protoCfg.EnableDNSFull,
		EnableMySQLFull:       protoCfg.EnableMySQLFull,
		EnableSQLAggregator:   protoCfg.EnableSQLAggregator,
		EnableCPUProfiler:     protoCfg.EnableCPUProfiler,
		MaxCPUCore:            protoCfg.MaxCPUCore,
		MaxMemoryMB:           protoCfg.MaxMemoryMB,
		MaxGoroutines:         protoCfg.MaxGoroutines,
		CollectInterval:       protoCfg.CollectInterval,
		BatchSize:             protoCfg.BatchSize,
		CircuitBreakerEnabled: protoCfg.CircuitBreakerEnabled,
	}
}

func (m *Manager) StartPeriodicFetch(interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				cfg, err := m.FetchConfig(ctx)
				cancel()
				if err != nil {
					m.log.Warnf("[配置管理器] 定期拉取配置失败: %v", err)
					continue
				}
				if cfg != nil {
					if err := m.UpdateConfig(cfg); err != nil {
						m.log.Warnf("[配置管理器] 更新配置失败: %v", err)
					}
				}
			case <-m.stopCh:
				return
			}
		}
	}()
	m.log.Infof("[配置管理器] 定期拉取已启动: 间隔=%v", interval)
}
