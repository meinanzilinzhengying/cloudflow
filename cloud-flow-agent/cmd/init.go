package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
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

// setupRuntime 配置运行时参数
func setupRuntime(cfg *config.Config, log *logger.Logger) {
	if cfg.EBPF.ResourceLimit.Enabled && cfg.EBPF.ResourceLimit.MaxCPUCore > 0 {
		procs := int(cfg.EBPF.ResourceLimit.MaxCPUCore)
		if procs < 1 {
			procs = 1
		}
		runtime.GOMAXPROCS(procs)
		log.Infof("[Runtime] GOMAXPROCS 设置为 %d", procs)
	}

	if cfg.EBPF.ResourceLimit.Enabled && cfg.EBPF.ResourceLimit.MaxMemoryMB > 0 {
		debug.SetMemoryLimit(int64(cfg.EBPF.ResourceLimit.MaxMemoryMB) * 1024 * 1024)
		debug.SetGCPercent(100)
		log.Infof("[Runtime] 内存限制设置为 %.0f MB", cfg.EBPF.ResourceLimit.MaxMemoryMB)
	}
}

// setupCgroup 配置 cgroup 管理器
func setupCgroup(cfg *config.Config, log *logger.Logger) *cgroup.Manager {
	if !cfg.EBPF.ResourceLimit.Enabled || !cfg.EBPF.ResourceLimit.UseCgroup {
		return nil
	}

	cgroupCfg := &cgroup.Config{
		MaxCPUCores: cfg.EBPF.ResourceLimit.MaxCPUCore,
		MaxMemoryMB: int64(cfg.EBPF.ResourceLimit.MaxMemoryMB),
	}

	cgroupMgr, err := cgroup.NewManager(cgroupCfg)
	if err != nil {
		log.Warnf("[Cgroup] 初始化失败: %v，继续使用应用层限制", err)
		return nil
	}

	if err := cgroupMgr.ApplyToCurrentProcess(); err != nil {
		log.Warnf("[Cgroup] 应用限制失败: %v", err)
	} else {
		log.Infof("[Cgroup] 已应用限制: CPU≤%.1f核, 内存≤%.0fMB",
			cfg.EBPF.ResourceLimit.MaxCPUCore, cfg.EBPF.ResourceLimit.MaxMemoryMB)
	}

	return cgroupMgr
}

// setupCircuitBreaker 配置过载熔断器
func setupCircuitBreaker(cfg *config.Config, log *logger.Logger) *circuitbreaker.Breaker {
	if !cfg.EBPF.CircuitBreaker.Enabled {
		return nil
	}

	obCfg := circuitbreaker.Config{
		CheckInterval:             cfg.EBPF.CircuitBreaker.CheckInterval,
		CPUDegradedThreshold:      cfg.EBPF.CircuitBreaker.CPUDegradedThreshold,
		CPUSilentThreshold:        cfg.EBPF.CircuitBreaker.CPUSilentThreshold,
		MemDegradedThreshold:      cfg.EBPF.CircuitBreaker.MemDegradedThreshold,
		MemSilentThreshold:        cfg.EBPF.CircuitBreaker.MemSilentThreshold,
		CPUDegradedDuration:       cfg.EBPF.CircuitBreaker.CPUDegradedDuration,
		CPURecoverThreshold:       cfg.EBPF.CircuitBreaker.CPURecoverThreshold,
		MemRecoverThreshold:       cfg.EBPF.CircuitBreaker.MemRecoverThreshold,
		SilentCPURecoverThreshold: cfg.EBPF.CircuitBreaker.SilentCPURecoverThreshold,
		SilentMemRecoverThreshold: cfg.EBPF.CircuitBreaker.SilentMemRecoverThreshold,
		MaxMemoryMB:               cfg.EBPF.ResourceLimit.MaxMemoryMB,
		MaxCPUCores:               cfg.EBPF.ResourceLimit.MaxCPUCore,
	}

	breaker := circuitbreaker.NewBreaker(obCfg)
	breaker.OnStateChange(func(from, to circuitbreaker.State, snapshot circuitbreaker.ResourceSnapshot) {
		switch to {
		case circuitbreaker.StateDegraded:
			log.Warnf("[过载熔断] 进入降级模式: CPU=%.1f%%, 内存=%.1f%%",
				snapshot.CPUPercent, snapshot.MemPercent)
		case circuitbreaker.StateSilent:
			log.Errorf("[过载熔断] 进入完全静默模式: CPU=%.1f%%, 内存=%.1f%%",
				snapshot.CPUPercent, snapshot.MemPercent)
		case circuitbreaker.StateNormal:
			log.Infof("[过载熔断] 恢复正常采集: CPU=%.1f%%, 内存=%.1f%%", snapshot.CPUPercent, snapshot.MemPercent)
		}
	})

	breaker.Start()
	log.Infof("[过载熔断] 已启动: CPU降级≥%.0f%%(持续%.0fs)/静默≥%.0f%%, 内存降级≥%.0f%%/静默≥%.0f%%",
		obCfg.CPUDegradedThreshold, obCfg.CPUDegradedDuration.Seconds(),
		obCfg.CPUSilentThreshold, obCfg.MemDegradedThreshold, obCfg.MemSilentThreshold)

	return breaker
}

// setupSelfMonitor 配置自监控采集器
func setupSelfMonitor(cfg *config.Config, log *logger.Logger) (*selfmonitor.Collector, error) {
	if !cfg.EBPF.SelfMonitor.Enabled {
		return nil, nil
	}

	smCfg := selfmonitor.Config{
		Enabled:          cfg.EBPF.SelfMonitor.Enabled,
		CollectInterval:  cfg.EBPF.SelfMonitor.CollectInterval,
		ReportInterval:   cfg.EBPF.SelfMonitor.ReportInterval,
		HeartbeatTimeout: cfg.EBPF.SelfMonitor.HeartbeatTimeout,
		MaxMemoryMB:      cfg.EBPF.ResourceLimit.MaxMemoryMB,
		AlertThresholds: selfmonitor.AlertThresholds{
			HeartbeatFailCount:      cfg.EBPF.SelfMonitor.AlertHeartbeatFailCount,
			CPUPercentThreshold:     cfg.EBPF.SelfMonitor.AlertCPUPercent,
			MemoryPercentThreshold:  cfg.EBPF.SelfMonitor.AlertMemoryPercent,
			PacketDropRateThreshold: cfg.EBPF.SelfMonitor.AlertPacketDropRate,
			ReportSuccessRateMin:    cfg.EBPF.SelfMonitor.AlertReportSuccessRateMin,
		},
	}

	collector := selfmonitor.NewCollector(smCfg, log)
	collector.OnAlert(func(alertType string, value float64, message string) {
		switch alertType {
		case "heartbeat_failure":
			log.Errorf("[自监控告警] 心跳异常: %s", message)
		case "cpu_high":
			log.Warnf("[自监控告警] CPU使用率过高: %s", message)
		case "memory_high":
			log.Warnf("[自监控告警] 内存使用率过高: %s", message)
		case "packet_drop_high":
			log.Warnf("[自监控告警] 采集丢包率过高: %s", message)
		case "report_success_low":
			log.Warnf("[自监控告警] 上报成功率过低: %s", message)
		}
	})
	collector.Start()

	log.Infof("[自监控] 采集器已启动: 采集间隔=%v, 上报间隔=%v",
		smCfg.CollectInterval, smCfg.ReportInterval)

	return collector, nil
}

// setupMetrics 配置指标收集器
func setupMetrics(cfg *config.Config, log *logger.Logger, selfMonitorCollector *selfmonitor.Collector) (*metrics.Metrics, chan error) {
	collector := metrics.New()

	if selfMonitorCollector != nil {
		selfMonitorCollector.SetCounters(
			collector.GetCollectCount(),
			collector.GetCollectErrors(),
			collector.GetSendCount(),
			collector.GetSendCount(),
		)
	}

	metricsAddr := cfg.MetricsPort
	server, errCh := collector.StartServer(metricsAddr)
	go func() {
		if err := <-errCh; err != nil {
			log.Warnf("启动 Prometheus 指标服务器失败: %v", err)
		}
	}()

	return collector, errCh
}

// setupNetwork 配置网络监控
func setupNetwork(cfg *config.Config, log *logger.Logger) (*network.Monitor, error) {
	if err := network.ValidateMgmtIP(cfg.Network.MgmtIP); err != nil {
		return nil, err
	}

	netMonitor := network.NewMonitor(cfg.Network.MgmtIP, cfg.EdgeAddr, log)
	netMonitor.Start()

	mgmtIP := netMonitor.GetMgmtIP()
	if mgmtIP != "" {
		log.Infof("使用管理IP: %s", mgmtIP)
	}

	return netMonitor, nil
}

// setupStorage 配置时序存储
func setupStorage(cfg *config.Config, log *logger.Logger) (*storage.TimeSeriesStore, error) {
	if !cfg.Storage.Enabled {
		return nil, nil
	}

	opts := &storage.StorageOptions{
		BaseDir: cfg.Storage.BaseDir,
		Retention: storage.RetentionConfig{
			Enabled:     true,
			DefaultDays: cfg.Storage.RetentionDays,
			CustomPeriod: map[storage.DataType]int{
				storage.DataTypeMetric: cfg.Storage.MetricRetentionDays,
				storage.DataTypeLog:    cfg.Storage.LogRetentionDays,
				storage.DataTypeTrace:  cfg.Storage.TraceRetentionDays,
				storage.DataTypeEvent:  cfg.Storage.EventRetentionDays,
			},
		},
		ChunkSize:          cfg.Storage.ChunkSize,
		WriteBufferSize:    cfg.Storage.WriteBufferSize,
		CompressionType:    storage.CompressionZSTD,
		IndexEnabled:       cfg.Storage.EnableIndex,
		RetentionInterval:  time.Duration(cfg.Storage.RetentionIntervalMin) * time.Minute,
	}

	tsStore, err := storage.NewTimeSeriesStore(opts, log)
	if err != nil {
		return nil, err
	}

	log.Infof("时序存储已启用: 目录=%s", cfg.Storage.BaseDir)
	return tsStore, nil
}

// connectToEdge 连接到边缘节点
func connectToEdge(cfg *config.Config, log *logger.Logger, mgmtIP string) (*grpcclient.Client, error) {
	connectDelay := cfg.ReconnectBaseDelay
	if connectDelay <= 0 {
		connectDelay = 2 * time.Second
	}
	maxRetries := cfg.MaxRetries

	for attempt := 1; ; attempt++ {
		if maxRetries > 0 && attempt > maxRetries {
			return nil, fmt.Errorf("连接边缘节点失败: 已达到最大重试次数 %d", maxRetries)
		}

		client, err := grpcclient.NewClient(cfg.EdgeAddr, cfg.APIKey, mgmtIP, grpcclient.TLSConfig{
			Enabled:    cfg.TLS.Enabled,
			ServerName: cfg.TLS.ServerName,
			CACert:     cfg.TLS.CACert,
			ClientCert: cfg.TLS.ClientCert,
			ClientKey:  cfg.TLS.ClientKey,
		}, log)
		if err == nil {
			return client, nil
		}

		log.Warnf("连接边缘节点失败 (第 %d 次): %v，%s 后重试...", attempt, err, connectDelay)
		time.Sleep(connectDelay)

		maxDelay := cfg.ReconnectMaxDelay
		if maxDelay <= 0 {
			maxDelay = 30 * time.Second
		}
		if connectDelay < maxDelay {
			connectDelay *= 2
		}
	}
}

// setupReliableReporter 配置可靠上报器
func setupReliableReporter(cfg *config.Config, log *logger.Logger, client *grpcclient.Client, netMonitor *network.Monitor) (*reliable.Reporter, error) {
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
		client,
		netMonitor,
		log,
	)
	if err != nil {
		return nil, err
	}

	log.Info("[可靠上报] 已启动: 校验和=SHA256, 缓存时长=1h, 缓存上限=100MB")
	return reporter, nil
}

// setupCollectors 配置采集器
func setupCollectors(cfg *config.Config, log *logger.Logger) (*collector.Collector, *ebpfcollector.Collector, *sqlaggregator.SQLAggregator, error) {
	c := collector.New(collector.CollectConfig{
		CPU:     cfg.Collect.CPU,
		Memory:  cfg.Collect.Memory,
		Network: cfg.Collect.Network,
		Disk:    cfg.Collect.Disk,
	})

	var ebpfCollector *ebpfcollector.Collector
	var sqlAggregator *sqlaggregator.SQLAggregator

	if cfg.EBPF.Enabled {
		ebpfOpts := &ebpfcollector.CollectorOptions{
			EnableTCPMetrics:  cfg.EBPF.TCPMetrics.Enabled,
			EnableHTTPMetrics: cfg.EBPF.HTTPMetrics.Enabled,
			EnableHTTPFull:    cfg.EBPF.ProtocolParsing.Enabled && cfg.EBPF.ProtocolParsing.HTTPFull,
			EnableDNSFull:     cfg.EBPF.ProtocolParsing.Enabled && cfg.EBPF.ProtocolParsing.DNSFull,
			EnableMySQLFull:   cfg.EBPF.ProtocolParsing.Enabled && cfg.EBPF.ProtocolParsing.MySQLFull,
			MgmtIface:         cfg.Network.MgmtIface,
		}

		ebpfCollector, err = ebpfcollector.NewWithOptions(ebpfOpts)
		if err != nil {
			log.Warnf("EBPF 采集器初始化失败: %v，将只使用传统采集器", err)
		} else {
			log.Info("EBPF 采集器初始化成功，开始采集网络流量")
			if ebpfCollector.IsTCPMetricsAvailable() {
				log.Info("TCP深度指标采集已启用: 建连时延、重传率、零窗口、队列溢出、连接失败")
			}
			if ebpfCollector.IsHTTPMetricsAvailable() {
				log.Info("HTTP应用层指标采集已启用: 请求成功率、响应时延、异常比例、请求数、响应数")
			}
			ebpfCollector.Start()
		}
	} else {
		log.Info("EBPF 采集已禁用，使用传统采集器")
	}

	if cfg.EBPF.SQLAggregator.Enabled {
		sqlAggOpts := &sqlaggregator.SQLAggregatorOptions{
			EnableMySQLSQLAgg:     true,
			SlowQueryThresholdMs: cfg.EBPF.SQLAggregator.SlowQueryThresholdMs,
		}
		sqlAggregator, err = sqlaggregator.NewWithOptions(sqlAggOpts)
		if err != nil {
			log.Warnf("SQL聚合分析器初始化失败: %v", err)
		}
	}

	return c, ebpfCollector, sqlAggregator, nil
}
