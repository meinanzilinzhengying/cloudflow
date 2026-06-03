package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/cgroup"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/circuitbreaker"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/collector"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/ebpfcollector"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/grpcclient"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/network"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/reliable"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/selfmonitor"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/sqlaggregator"
	"github.com/meinanzilinzhengying/cloudflow/agent/internal/storage"
	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/metrics"
)

// Dependencies 包含 Agent 的所有依赖，便于测试时注入 mock
type Dependencies struct {
	Config          *config.Config
	Logger          *logger.Logger
	GRPCClient      *grpcclient.Client
	NetMonitor      *network.Monitor
	TSStore        *storage.TimeSeriesStore
	Breaker        *circuitbreaker.Breaker
	SelfMonitor    *selfmonitor.Collector
	Reporter       *reliable.Reporter
	LegacyCollector *collector.Collector
	EBPFCollector  *ebpfcollector.Collector
	SQLAggregator  *sqlaggregator.SQLAggregator
	MetricCollector *metrics.Metrics
	CgroupManager  *cgroup.Manager
}

// Provider 负责创建和提供 Dependencies
type Provider struct {
	cfg *config.Config
}

// NewProvider 创建 Provider
func NewProvider(cfg *config.Config) *Provider {
	return &Provider{cfg: cfg}
}

// Provide 返回完整的依赖图
func (p *Provider) Provide() (*Dependencies, error) {
	log := logger.New(logger.Config{
		Level:  p.cfg.Log.Level,
		Format: p.cfg.Log.Format,
	})

	deps := &Dependencies{
		Config: p.cfg,
		Logger: log,
	}

	p.setupRuntime(deps)
	deps.CgroupManager = p.setupCgroup(deps)

	deps.Breaker = p.setupCircuitBreaker(deps)

	var err error
	deps.SelfMonitor, err = p.setupSelfMonitor(deps)
	if err != nil {
		log.Warnf("[自监控] 初始化失败: %v", err)
	}

	deps.MetricCollector = p.setupMetrics(deps)
	log.Infof("探针启动中... 配置: %s", p.cfg.Summary())

	deps.NetMonitor, err = p.setupNetwork(deps)
	if err != nil {
		log.Errorf("管理IP配置无效: %v", err)
		return nil, err
	}

	mgmtIP := deps.NetMonitor.GetMgmtIP()
	if mgmtIP != "" {
		log.Infof("使用管理IP: %s", mgmtIP)
	}

	deps.TSStore, err = p.setupStorage(deps)
	if err != nil {
		log.Warnf("时序存储初始化失败: %v", err)
		deps.TSStore = nil
	}

	deps.GRPCClient, err = p.connectToEdge(deps, mgmtIP)
	if err != nil {
		log.Errorf("连接边缘节点失败: %v", err)
		return nil, err
	}

	deps.Reporter, err = p.setupReliableReporter(deps)
	if err != nil {
		log.Warnf("[可靠上报] 初始化失败: %v", err)
		deps.Reporter = nil
	}

	deps.LegacyCollector, deps.EBPFCollector, deps.SQLAggregator, err = p.setupCollectors(deps)
	if err != nil {
		log.Warnf("采集器初始化失败: %v", err)
	}

	return deps, nil
}

func (p *Provider) setupRuntime(deps *Dependencies) {
	if p.cfg.EBPF.ResourceLimit.Enabled && p.cfg.EBPF.ResourceLimit.MaxCPUCore > 0 {
		procs := int(p.cfg.EBPF.ResourceLimit.MaxCPUCore)
		if procs < 1 {
			procs = 1
		}
		deps.Logger.Infof("[Runtime] GOMAXPROCS 设置为 %d", procs)
	}
}

func (p *Provider) setupCgroup(deps *Dependencies) *cgroup.Manager {
	if !p.cfg.EBPF.ResourceLimit.Enabled || !p.cfg.EBPF.ResourceLimit.UseCgroup {
		return nil
	}

	cgroupCfg := &cgroup.Config{
		MaxCPUCores: p.cfg.EBPF.ResourceLimit.MaxCPUCore,
		MaxMemoryMB: int64(p.cfg.EBPF.ResourceLimit.MaxMemoryMB),
	}

	cgroupMgr, err := cgroup.NewManager(cgroupCfg)
	if err != nil {
		deps.Logger.Warnf("[Cgroup] 初始化失败: %v", err)
		return nil
	}

	if err := cgroupMgr.ApplyToCurrentProcess(); err != nil {
		deps.Logger.Warnf("[Cgroup] 应用限制失败: %v", err)
	} else {
		deps.Logger.Infof("[Cgroup] 已应用限制: CPU≤%.1f核, 内存≤%.0fMB",
			p.cfg.EBPF.ResourceLimit.MaxCPUCore, p.cfg.EBPF.ResourceLimit.MaxMemoryMB)
	}

	return cgroupMgr
}

func (p *Provider) setupCircuitBreaker(deps *Dependencies) *circuitbreaker.Breaker {
	if !p.cfg.EBPF.CircuitBreaker.Enabled {
		return nil
	}

	obCfg := circuitbreaker.Config{
		CheckInterval:             p.cfg.EBPF.CircuitBreaker.CheckInterval,
		CPUDegradedThreshold:      p.cfg.EBPF.CircuitBreaker.CPUDegradedThreshold,
		CPUSilentThreshold:        p.cfg.EBPF.CircuitBreaker.CPUSilentThreshold,
		MemDegradedThreshold:      p.cfg.EBPF.CircuitBreaker.MemDegradedThreshold,
		MemSilentThreshold:        p.cfg.EBPF.CircuitBreaker.MemSilentThreshold,
		CPUDegradedDuration:       p.cfg.EBPF.CircuitBreaker.CPUDegradedDuration,
		CPURecoverThreshold:       p.cfg.EBPF.CircuitBreaker.CPURecoverThreshold,
		MemRecoverThreshold:       p.cfg.EBPF.CircuitBreaker.MemRecoverThreshold,
		SilentCPURecoverThreshold: p.cfg.EBPF.CircuitBreaker.SilentCPURecoverThreshold,
		SilentMemRecoverThreshold: p.cfg.EBPF.CircuitBreaker.SilentMemRecoverThreshold,
		MaxMemoryMB:              p.cfg.EBPF.ResourceLimit.MaxMemoryMB,
		MaxCPUCores:              p.cfg.EBPF.ResourceLimit.MaxCPUCore,
	}

	breaker := circuitbreaker.NewBreaker(obCfg)
	breaker.OnStateChange(func(from, to circuitbreaker.State, snapshot circuitbreaker.ResourceSnapshot) {
		switch to {
		case circuitbreaker.StateDegraded:
			deps.Logger.Warnf("[过载熔断] 进入降级模式")
		case circuitbreaker.StateSilent:
			deps.Logger.Errorf("[过载熔断] 进入完全静默模式")
		case circuitbreaker.StateNormal:
			deps.Logger.Infof("[过载熔断] 恢复正常采集")
		}
	})

	breaker.Start()
	deps.Logger.Infof("[过载熔断] 已启动")

	return breaker
}

func (p *Provider) setupSelfMonitor(deps *Dependencies) (*selfmonitor.Collector, error) {
	if !p.cfg.EBPF.SelfMonitor.Enabled {
		return nil, nil
	}

	smCfg := selfmonitor.Config{
		Enabled:          p.cfg.EBPF.SelfMonitor.Enabled,
		CollectInterval:  p.cfg.EBPF.SelfMonitor.CollectInterval,
		ReportInterval:   p.cfg.EBPF.SelfMonitor.ReportInterval,
		HeartbeatTimeout: p.cfg.EBPF.SelfMonitor.HeartbeatTimeout,
		MaxMemoryMB:     p.cfg.EBPF.ResourceLimit.MaxMemoryMB,
	}

	collector := selfmonitor.NewCollector(smCfg, deps.Logger)
	collector.Start()
	deps.Logger.Infof("[自监控] 采集器已启动")

	return collector, nil
}

func (p *Provider) setupMetrics(deps *Dependencies) *metrics.Metrics {
	collector := metrics.New()
	metricsAddr := p.cfg.MetricsPort
	_, errCh := collector.StartServer(metricsAddr)
	go func() {
		if err := <-errCh; err != nil {
			deps.Logger.Warnf("启动 Prometheus 指标服务器失败: %v", err)
		}
	}()
	return collector
}

func (p *Provider) setupNetwork(deps *Dependencies) (*network.Monitor, error) {
	if err := network.ValidateMgmtIP(p.cfg.Network.MgmtIP); err != nil {
		return nil, err
	}
	netMonitor := network.NewMonitor(p.cfg.Network.MgmtIP, p.cfg.EdgeAddr, deps.Logger)
	netMonitor.Start()
	return netMonitor, nil
}

func (p *Provider) setupStorage(deps *Dependencies) (*storage.TimeSeriesStore, error) {
	if !p.cfg.Storage.Enabled {
		return nil, nil
	}
	opts := &storage.StorageOptions{BaseDir: p.cfg.Storage.BaseDir}
	tsStore, err := storage.NewTimeSeriesStore(opts, deps.Logger)
	if err != nil {
		return nil, err
	}
	deps.Logger.Infof("时序存储已启用")
	return tsStore, nil
}

func (p *Provider) connectToEdge(deps *Dependencies, mgmtIP string) (*grpcclient.Client, error) {
	connectDelay := p.cfg.ReconnectBaseDelay
	if connectDelay <= 0 {
		connectDelay = 2
	}
	maxRetries := p.cfg.MaxRetries

	for attempt := 1; ; attempt++ {
		if maxRetries > 0 && attempt > maxRetries {
			return nil, fmt.Errorf("连接边缘节点失败: 已达到最大重试次数 %d", maxRetries)
		}

		client, err := grpcclient.NewClient(p.cfg.EdgeAddr, p.cfg.APIKey, mgmtIP, grpcclient.TLSConfig{
			Enabled:    p.cfg.TLS.Enabled,
			ServerName: p.cfg.TLS.ServerName,
			CACert:     p.cfg.TLS.CACert,
			ClientCert: p.cfg.TLS.ClientCert,
			ClientKey:  p.cfg.TLS.ClientKey,
		}, deps.Logger)
		if err == nil {
			return client, nil
		}

		deps.Logger.Warnf("连接边缘节点失败 (第 %d 次): %v", attempt, err)
		time.Sleep(time.Duration(connectDelay) * time.Second)
	}
}

func (p *Provider) setupReliableReporter(deps *Dependencies) (*reliable.Reporter, error) {
	reporter, err := reliable.NewReporter(
		reliable.Config{
			CacheDir:            filepath.Join(os.TempDir(), "cloud-flow-cache"),
			MaxCacheDuration:     1 * time.Hour,
			RetransmitBatchSize: 100,
			RetransmitInterval:  100 * time.Millisecond,
			SendTimeout:         10 * time.Second,
			EnableChecksum:      true,
			MaxCacheSizeBytes:   100 * 1024 * 1024,
		},
		deps.GRPCClient,
		deps.NetMonitor,
		deps.Logger,
	)
	if err != nil {
		return nil, err
	}
	deps.Logger.Info("[可靠上报] 已启动")
	return reporter, nil
}

func (p *Provider) setupCollectors(deps *Dependencies) (*collector.Collector, *ebpfcollector.Collector, *sqlaggregator.SQLAggregator, error) {
	c := collector.New(collector.CollectConfig{
		CPU:     p.cfg.Collect.CPU,
		Memory:  p.cfg.Collect.Memory,
		Network: p.cfg.Collect.Network,
		Disk:    p.cfg.Collect.Disk,
	})

	var ebpfCollector *ebpfcollector.Collector
	var sqlAggregator *sqlaggregator.SQLAggregator

	if p.cfg.EBPF.Enabled {
		ebpfOpts := &ebpfcollector.CollectorOptions{
			EnableTCPMetrics:  p.cfg.EBPF.TCPMetrics.Enabled,
			EnableHTTPMetrics: p.cfg.EBPF.HTTPMetrics.Enabled,
			MgmtIface:         p.cfg.Network.MgmtIface,
		}

		ebpfCollector, err = ebpfcollector.NewWithOptions(ebpfOpts)
		if err != nil {
			deps.Logger.Warnf("EBPF 采集器初始化失败: %v", err)
		} else {
			deps.Logger.Info("EBPF 采集器初始化成功")
			ebpfCollector.Start()
		}
	} else {
		deps.Logger.Info("EBPF 采集已禁用")
	}

	return c, ebpfCollector, sqlAggregator, nil
}
