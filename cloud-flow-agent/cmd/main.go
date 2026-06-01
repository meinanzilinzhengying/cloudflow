package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"cloud-flow-agent/internal/cgroup"
	"cloud-flow-agent/internal/circuitbreaker"
	"cloud-flow-agent/internal/collector"
	"cloud-flow-agent/internal/config"
	"cloud-flow-agent/internal/ebpfcollector"
	"cloud-flow-agent/internal/grpcclient"
	"cloud-flow-agent/internal/http"
	"cloud-flow-agent/internal/dropmonitor"
	"cloud-flow-agent/internal/network"
	"cloud-flow-agent/internal/ntp"
	"cloud-flow-agent/internal/protocol"
	"cloud-flow-agent/internal/reliable"
	"cloud-flow-agent/internal/selfmonitor"
	"cloud-flow-agent/internal/sqlaggregator"
	"cloud-flow-agent/internal/storage"
	"cloud-flow-agent/pkg/logger"
	"cloud-flow-agent/pkg/metrics"
	edge "cloud-flow/proto"
)

// safeClient 线程安全的客户端包装器
type safeClient struct {
	mu     sync.RWMutex
	client *grpcclient.Client
}

// Get 安全地获取客户端
// 返回可能为 nil，调用方需要检查
func (sc *safeClient) Get() *grpcclient.Client {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.client
}

// GetOrNil 安全地获取客户端，显式返回 nil 表示未设置
// 与 Get() 行为一致，但方法名更明确表达可能返回 nil
func (sc *safeClient) GetOrNil() *grpcclient.Client {
	return sc.Get()
}

// IsNil 检查客户端是否为 nil
func (sc *safeClient) IsNil() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.client == nil
}

// Set 安全地设置客户端
func (sc *safeClient) Set(c *grpcclient.Client) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.client = c
}

// Version 版本号，可在编译时通过 -ldflags "-X main.Version=x.y.z" 注入
var Version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})
	defer log.Sync()

	// 设置 GOMAXPROCS
	if cfg.EBPF.ResourceLimit.Enabled && cfg.EBPF.ResourceLimit.MaxCPUCore > 0 {
		procs := int(cfg.EBPF.ResourceLimit.MaxCPUCore)
		if procs < 1 {
			procs = 1
		}
		runtime.GOMAXPROCS(procs)
		log.Infof("[Runtime] GOMAXPROCS 设置为 %d", procs)
	}

	// 设置 GOGC 和内存限制
	if cfg.EBPF.ResourceLimit.Enabled && cfg.EBPF.ResourceLimit.MaxMemoryMB > 0 {
		// 设置软内存限制 (Go 1.19+)
		debug.SetMemoryLimit(int64(cfg.EBPF.ResourceLimit.MaxMemoryMB) * 1024 * 1024)
		// 设置 GC 目标百分比
		debug.SetGCPercent(100)
		log.Infof("[Runtime] 内存限制设置为 %.0f MB", cfg.EBPF.ResourceLimit.MaxMemoryMB)
	}

	// 初始化 cgroup 管理器
	var cgroupMgr *cgroup.Manager
	if cfg.EBPF.ResourceLimit.Enabled && cfg.EBPF.ResourceLimit.UseCgroup {
		cgroupCfg := &cgroup.Config{
			MaxCPUCores: cfg.EBPF.ResourceLimit.MaxCPUCore,
			MaxMemoryMB: int64(cfg.EBPF.ResourceLimit.MaxMemoryMB),
		}
		var err error
		cgroupMgr, err = cgroup.NewManager(cgroupCfg)
		if err != nil {
			log.Warnf("[Cgroup] 初始化失败: %v，继续使用应用层限制", err)
		} else {
			if err := cgroupMgr.ApplyToCurrentProcess(); err != nil {
				log.Warnf("[Cgroup] 应用限制失败: %v", err)
			} else {
				log.Infof("[Cgroup] 已应用限制: CPU≤%.1f核, 内存≤%.0fMB",
					cfg.EBPF.ResourceLimit.MaxCPUCore, cfg.EBPF.ResourceLimit.MaxMemoryMB)
			}
			defer cgroupMgr.Close()
		}
	}

	// 初始化过载熔断器
	var overloadBreaker *circuitbreaker.Breaker
	if cfg.EBPF.CircuitBreaker.Enabled {
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
		overloadBreaker = circuitbreaker.NewBreaker(obCfg)
		overloadBreaker.OnStateChange(func(from, to circuitbreaker.State, snapshot circuitbreaker.ResourceSnapshot) {
			switch to {
			case circuitbreaker.StateDegraded:
				log.Warnf("[过载熔断] 进入降级模式: CPU=%.1f%%, 内存=%.1f%%, 停止非核心指标采集(HTTP/DNS/MySQL全字段+SQL聚合+传统采集)",
					snapshot.CPUPercent, snapshot.MemPercent)
			case circuitbreaker.StateSilent:
				log.Errorf("[过载熔断] 进入完全静默模式: CPU=%.1f%%, 内存=%.1f%%, 停止所有采集",
					snapshot.CPUPercent, snapshot.MemPercent)
			case circuitbreaker.StateNormal:
				log.Infof("[过载熔断] 恢复正常采集: CPU=%.1f%%, 内存=%.1f%%", snapshot.CPUPercent, snapshot.MemPercent)
			}
		})
		overloadBreaker.Start()
		defer overloadBreaker.Stop()
		log.Infof("[过载熔断] 已启动: CPU降级≥%.0f%%(持续%.0fs)/静默≥%.0f%%, 内存降级≥%.0f%%/静默≥%.0f%%",
			obCfg.CPUDegradedThreshold, obCfg.CPUDegradedDuration.Seconds(),
			obCfg.CPUSilentThreshold, obCfg.MemDegradedThreshold, obCfg.MemSilentThreshold)
	}

	// 初始化自监控采集器
	var selfMonitorCollector *selfmonitor.Collector
	var selfMonitorReporter *selfmonitor.Reporter
	if cfg.EBPF.SelfMonitor.Enabled {
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
		selfMonitorCollector = selfmonitor.NewCollector(smCfg, log)

		// 设置告警回调
		selfMonitorCollector.OnAlert(func(alertType string, value float64, message string) {
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

		selfMonitorCollector.Start()
		defer selfMonitorCollector.Stop()
		log.Infof("[自监控] 采集器已启动: 采集间隔=%v, 上报间隔=%v",
			smCfg.CollectInterval, smCfg.ReportInterval)
	}

	// 初始化指标收集器
	metricCollector := metrics.New()

	// 设置自监控采集器的计数器引用
	if selfMonitorCollector != nil {
		selfMonitorCollector.SetCounters(
			metricCollector.GetCollectCount(),
			metricCollector.GetCollectErrors(),
			metricCollector.GetSendCount(),
			metricCollector.GetSendCount(), // sendCount - sendErrors = success
		)
	}

	// 启动 Prometheus 指标服务器
	metricsAddr := fmt.Sprintf(":%s", cfg.MetricsPort)
	metricsServer, metricsErrCh := metricCollector.StartServer(metricsAddr)
	go func() {
		if err := <-metricsErrCh; err != nil {
			log.Warnf("启动 Prometheus 指标服务器失败: %v", err)
		}
	}()

	log.Infof("探针启动中... 配置: %s", cfg.Summary())

	// 验证管理IP配置
	if err := network.ValidateMgmtIP(cfg.Network.MgmtIP); err != nil {
		log.Errorf("管理IP配置无效: %v", err)
		os.Exit(1)
	}

	// 初始化网卡监控器
	netMonitor := network.NewMonitor(cfg.Network.MgmtIP, cfg.EdgeAddr, log)
	netMonitor.Start()
	defer netMonitor.Stop()

	// 获取实际使用的管理IP（可能自动检测）
	mgmtIP := netMonitor.GetMgmtIP()
	if mgmtIP != "" {
		log.Infof("使用管理IP: %s", mgmtIP)
	}

	// 初始化时序存储（如果启用了本地缓存）
	var tsStore *storage.TimeSeriesStore
	if cfg.Storage.Enabled {
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
			ChunkSize:         cfg.Storage.ChunkSize,
			WriteBufferSize:   cfg.Storage.WriteBufferSize,
			CompressionType:   storage.CompressionZSTD,
			IndexEnabled:      cfg.Storage.EnableIndex,
			RetentionInterval: time.Duration(cfg.Storage.RetentionIntervalMin) * time.Minute,
		}
		tsStore, err = storage.NewTimeSeriesStore(opts, log)
		if err != nil {
			log.Warnf("初始化时序存储失败: %v，将禁用本地缓存", err)
			tsStore = nil
		} else {
			log.Infof("时序存储已启用: 目录=%s", cfg.Storage.BaseDir)
			defer tsStore.Close()
		}
	}

	// 连接边缘节点（带重试，Edge 未启动时自动等待）
	// L1 修复: 使用配置的重连参数
	var client *grpcclient.Client
	connectDelay := cfg.ReconnectBaseDelay
	if connectDelay <= 0 {
		connectDelay = 2 * time.Second // 默认值
	}
	maxRetries := cfg.MaxRetries

	for attempt := 1; ; attempt++ {
		if maxRetries > 0 && attempt > maxRetries {
			log.Errorf("连接边缘节点失败: 已达到最大重试次数 %d", maxRetries)
			return
		}

		client, err = grpcclient.NewClient(cfg.EdgeAddr, cfg.APIKey, mgmtIP, grpcclient.TLSConfig{
			Enabled:    cfg.TLS.Enabled,
			ServerName: cfg.TLS.ServerName,
			CACert:     cfg.TLS.CACert,
			ClientCert: cfg.TLS.ClientCert,
			ClientKey:  cfg.TLS.ClientKey,
		}, log)
		if err == nil {
			break
		}
		log.Warnf("连接边缘节点失败 (第 %d 次): %v，%s 后重试...", attempt, err, connectDelay)
		time.Sleep(connectDelay)
		// L1 修复: 使用配置的最大延迟
		maxDelay := cfg.ReconnectMaxDelay
		if maxDelay <= 0 {
			maxDelay = 30 * time.Second
		}
		if connectDelay < maxDelay {
			connectDelay *= 2
		}
	}

	// 创建线程安全的客户端包装器
	safeClient := &safeClient{client: client}

	// 注册探针（携带 region/zone/cluster 元数据）
	hostname, _ := os.Hostname()
	hostIP := getLocalIP()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// FIX: 检查 safeClient 是否为 nil，防止空指针
	client := safeClient.Get()
	if client == nil {
		log.Errorf("gRPC 客户端未初始化")
		os.Exit(1)
	}
	resp, err := client.Register(ctx, cfg.ProbeID, hostIP, hostname, Version)
	if err != nil {
		log.Errorf("注册探针失败: %v", err)
		client.Close()
		os.Exit(1)
	}
	log.Infof("注册成功: %s, 心跳间隔=%ds", resp.GetMessage(), resp.GetHeartbeatInterval())

	// 分布式边缘自治: 记录归属 Edge ID，重连时优先连接
	assignedEdgeID := resp.GetAssignedEdgeId()
	sessionID := resp.GetSessionId()
	if assignedEdgeID != "" {
		log.Infof("归属 Edge: %s, Session: %s", assignedEdgeID, sessionID)
		// 持久化归属关系到本地文件（包含 Edge 地址，用于重连优先连接）
		saveAssignedEdge(cfg.ProbeID, assignedEdgeID, sessionID, cfg.EdgeAddr)
	} else {
		// 尝试从本地文件恢复归属关系
		info := loadAssignedEdge(cfg.ProbeID)
		if info != nil && info.EdgeID != "" {
			assignedEdgeID = info.EdgeID
			sessionID = info.SessionID
			log.Infof("从本地恢复归属 Edge: %s, Session: %s", assignedEdgeID, sessionID)
		}
	}

	// 初始化自监控上报器（注册成功后才能上报）
	if selfMonitorCollector != nil && cfg.EBPF.SelfMonitor.Enabled {
		selfMonitorReporter = selfmonitor.NewReporter(
			selfmonitor.Config{
				Enabled:         cfg.EBPF.SelfMonitor.Enabled,
				ReportInterval:  cfg.EBPF.SelfMonitor.ReportInterval,
			},
			selfMonitorCollector,
			safeClient.Get(),
			cfg.ProbeID,
			log,
		)
		selfMonitorReporter.Start()
		defer selfMonitorReporter.Stop()
		log.Infof("[自监控] 上报器已启动: 上报间隔=%v", cfg.EBPF.SelfMonitor.ReportInterval)
	}

	// 初始化可靠上报器（带校验和、离线缓存、自动重传）
	reliableReporter, err := reliable.NewReporter(
		reliable.Config{
			CacheDir:            filepath.Join(os.TempDir(), "cloud-flow-cache"),
			MaxCacheDuration:    1 * time.Hour,
			RetransmitBatchSize: 100,
			RetransmitInterval:  100 * time.Millisecond,
			SendTimeout:         10 * time.Second,
			EnableChecksum:      true,
			MaxCacheSizeBytes:   100 * 1024 * 1024,
		},
		safeClient.Get(),
		netMonitor,
		log,
	)
	if err != nil {
		log.Warnf("[可靠上报] 初始化失败（将使用基础发送模式）: %v", err)
		reliableReporter = nil
	} else {
		defer reliableReporter.Stop()
		log.Info("[可靠上报] 已启动: 校验和=SHA256, 缓存时长=1h, 缓存上限=100MB")
	}

	// 初始化传统采集器
	c := collector.New(collector.CollectConfig{
		CPU:     cfg.Collect.CPU,
		Memory:  cfg.Collect.Memory,
		Network: cfg.Collect.Network,
		Disk:    cfg.Collect.Disk,
	})

	// 初始化 EBPF 采集器（如果可用）
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
			if ebpfCollector.IsHTTPFullAvailable() {
				log.Info("HTTP全字段解析已启用: 方法、路径、Host、Cookie、User-Agent、状态码等")
			}
			if ebpfCollector.IsDNSFullAvailable() {
				log.Info("DNS全字段解析已启用: 域名、记录类型、TTL、响应码等")
			}
			if ebpfCollector.IsMySQLFullAvailable() {
				log.Info("MySQL全字段解析已启用: 命令、SQL语句、错误码、影响行数等")
			}
			ebpfCollector.Start()
			defer ebpfCollector.Stop()
		}
	} else {
		log.Info("EBPF 采集已禁用，使用传统采集器")
	}

	// 初始化 SQL 聚合分析器（如果启用）
	if cfg.EBPF.SQLAggregator.Enabled {
		sqlAggOpts := &sqlaggregator.SQLAggregatorOptions{
			EnableMySQLSQLAgg:     true,
			SlowQueryThresholdMs: cfg.EBPF.SQLAggregator.SlowQueryThresholdMs,
		}
		sqlAggregator, err = sqlaggregator.NewWithOptions(sqlAggOpts)
		if err != nil {
			log.Warnf("SQL聚合分析器初始化失败: %v", err)
		} else {
			log.Infof("SQL聚合分析器已启用: 慢SQL阈值=%dms, 进程性能关联=%v",
				cfg.EBPF.SQLAggregator.SlowQueryThresholdMs, cfg.EBPF.SQLAggregator.EnableCorrelation)
			sqlAggregator.Start()
			defer sqlAggregator.Stop()
		}
	}

	// 初始化插件化协议解析框架（如果启用）
	var pluginManager *protocol.Manager
	if cfg.EBPF.PluginFramework.Enabled {
		// 注册内置插件（Oracle/PostgreSQL/Redis/Kafka/Dubbo）
		if cfg.EBPF.PluginFramework.EnableBuiltin {
			protocol.RegisterBuiltinPlugins()
			log.Info("[插件框架] 已注册5个内置协议解析插件: Oracle/PostgreSQL/Redis/Kafka/Dubbo")
		}

		pmCfg := protocol.ManagerConfig{
			PluginDir:     cfg.EBPF.PluginFramework.PluginDir,
			AutoDiscovery: cfg.EBPF.PluginFramework.AutoDiscovery,
			CheckInterval: cfg.EBPF.PluginFramework.CheckInterval,
			MaxMemoryMB:   cfg.EBPF.PluginFramework.MaxMemoryMB,
			GRPCTimeout:   cfg.EBPF.PluginFramework.GRPCTimeout,
		}
		pluginManager = protocol.NewManager(pmCfg, log)
		if err := pluginManager.Start(); err != nil {
			log.Warnf("[插件框架] 启动失败: %v", err)
			pluginManager = nil
		} else {
			defer pluginManager.Stop()
			plugins := pluginManager.ListPlugins()
			log.Infof("[插件框架] 已启动: 插件目录=%s, 已加载插件=%d, 自动发现=%v",
				pmCfg.PluginDir, len(plugins), pmCfg.AutoDiscovery)
		}
	}

	// 初始化丢包监控器（如果启用）
	var dropMonitor *dropmonitor.Monitor
	if cfg.EBPF.DropMonitor.Enabled {
		dmCfg := dropmonitor.Config{
			Enabled:          cfg.EBPF.DropMonitor.Enabled,
			EnableKernelDrop: cfg.EBPF.DropMonitor.EnableKernelDrop,
			EnableUserDrop:   cfg.EBPF.DropMonitor.EnableUserDrop,
			RingBufSize:      cfg.EBPF.DropMonitor.RingBufSize,
			SampleRate:       cfg.EBPF.DropMonitor.SampleRate,
			SnapshotInterval: cfg.EBPF.DropMonitor.SnapshotInterval,
			AlertThreshold:   cfg.EBPF.DropMonitor.AlertThreshold,
		}
		dropMonitor = dropmonitor.NewMonitor(dmCfg, log)
		dropMonitor.OnAlert(func(dropRate float64, message string) {
			log.Errorf("[丢包监控告警] 丢包率=%.2f%%, %s", dropRate, message)
		})
		if err := dropMonitor.Start(); err != nil {
			log.Warnf("[丢包监控] 启动失败: %v", err)
			dropMonitor = nil
		} else {
			defer dropMonitor.Stop()
			log.Infof("[丢包监控] 已启动: 内核态=%v, 用户态=%v, 告警阈值=%.1f%%",
				dmCfg.EnableKernelDrop, dmCfg.EnableUserDrop, dmCfg.AlertThreshold)
		}
	}

	// 初始化NTP时钟校准客户端（如果启用）
	var ntpClient *ntp.Client
	if cfg.EBPF.NTP.Enabled {
		// 解析同步模式
		var ntpMode ntp.SyncMode
		switch cfg.EBPF.NTP.Mode {
		case "grpc":
			ntpMode = ntp.SyncModeGRPC
		case "ntp":
			ntpMode = ntp.SyncModeNTP
		default:
			ntpMode = ntp.SyncModeAuto
		}

		ntpCfg := ntp.Config{
			Enabled:      cfg.EBPF.NTP.Enabled,
			Mode:         ntpMode,
			NTPServers:   cfg.EBPF.NTP.NTPServers,
			CenterAddr:   cfg.EdgeAddr,
			SyncInterval: cfg.EBPF.NTP.SyncInterval,
			MaxOffset:    cfg.EBPF.NTP.MaxOffset,
			AdjustStep:   cfg.EBPF.NTP.AdjustStep,
			AdjustSlew:   cfg.EBPF.NTP.AdjustSlew,
		}
		ntpClient = ntp.NewClient(ntpCfg, log)
		ntpClient.OnSync(func(result *ntp.SyncResult) {
			log.Infof("[NTP] 时钟同步成功: 服务器=%s, 偏差=%v, 延迟=%v",
				result.Server, result.Offset, result.Delay)
		})
		ntpClient.OnFail(func(err error) {
			log.Warnf("[NTP] 时钟同步失败: %v", err)
		})
		if err := ntpClient.Start(); err != nil {
			log.Warnf("[NTP] 启动失败: %v", err)
			ntpClient = nil
		} else {
			defer ntpClient.Stop()
			log.Infof("[NTP] 已启动: 模式=%s, 同步间隔=%v, 最大偏差=%v",
				cfg.EBPF.NTP.Mode, ntpCfg.SyncInterval, ntpCfg.MaxOffset)
		}
	}

	stopCh := make(chan struct{})
	var wg sync.WaitGroup
	var stopOnce sync.Once

	// 心跳协程
	heartbeatInterval := time.Duration(resp.GetHeartbeatInterval()) * time.Second
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		failureCount := 0
		const maxFailures = 3 // 连续失败3次触发重连
		const reconnectDelay = 5 * time.Second

		for {
			select {
			case <-ticker.C:
				heartbeatCtx, heartbeatCancel := context.WithTimeout(context.Background(), 5*time.Second)
				heartbeatStart := time.Now()
				// FIX: nil 检查 safeClient.Get() 返回值
				currentClient := safeClient.Get()
				if currentClient == nil {
					heartbeatCancel()
					log.Warn("gRPC 客户端为空，跳过一次心跳")
					failureCount++
					continue
				}
				if err := currentClient.Heartbeat(heartbeatCtx, cfg.ProbeID); err != nil {
					failureCount++
					log.Warnf("心跳失败 (第 %d/%d 次): %v", failureCount, maxFailures, err)
					metricCollector.HeartbeatSent(err)
					// 记录自监控心跳状态
					if selfMonitorCollector != nil {
						selfMonitorCollector.RecordHeartbeat(false, time.Since(heartbeatStart))
					}

					// 连续失败达到阈值，触发重连
					if failureCount >= maxFailures {
						log.Warnf("连续心跳失败 %d 次，开始重连 Edge 节点...", maxFailures)

						// FIX: 先获取并置空，防止其他 goroutine 使用已关闭的客户端
						oldClient := safeClient.Get()
						safeClient.Set(nil)
						if oldClient != nil {
							oldClient.Close()
						}

						// 等待网卡恢复可用
						if !netMonitor.IsAvailable() {
							log.Warn("管理网卡不可用，等待网卡恢复...")
							if !netMonitor.WaitForAvailable(30 * time.Second) {
								log.Warn("等待网卡恢复超时，继续尝试重连...")
							}
						}

						// 重新连接 Edge 节点
					// P1 修复: 优先连接原归属 Edge，失败后回退到默认配置地址
					// M3 修复: 添加最大重试次数，超过后降级为本地缓存模式
					// L1 修复: 使用配置的重连参数
					var newClient *grpcclient.Client
					connectDelay := cfg.ReconnectBaseDelay
					if connectDelay <= 0 {
						connectDelay = 2 * time.Second
					}
					maxReconnectAttempts := cfg.MaxReconnectAttempts
					if maxReconnectAttempts <= 0 {
						maxReconnectAttempts = 10
					}
					reconnectSuccess := false

					// 构建候选地址列表：优先使用已保存的归属 Edge 地址
					candidateAddrs := []string{cfg.EdgeAddr}
					savedInfo := loadAssignedEdge(cfg.ProbeID)
					if savedInfo != nil && savedInfo.Addr != "" && savedInfo.Addr != cfg.EdgeAddr {
						candidateAddrs = []string{savedInfo.Addr, cfg.EdgeAddr}
						log.Infof("重连优先使用归属 Edge: %s (备选: %s)", savedInfo.Addr, cfg.EdgeAddr)
					}

					for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
						// 轮询候选地址（第一轮优先归属 Edge，后续轮次也优先）
						for _, addr := range candidateAddrs {
							// FIX: 关闭上一次循环中创建但未使用的 newClient
							if newClient != nil && !reconnectSuccess {
								newClient.Close()
								newClient = nil
							}
							newClient, err = grpcclient.NewClient(addr, cfg.APIKey, mgmtIP, grpcclient.TLSConfig{
								Enabled:    cfg.TLS.Enabled,
								ServerName: cfg.TLS.ServerName,
								CACert:     cfg.TLS.CACert,
								ClientCert: cfg.TLS.ClientCert,
								ClientKey:  cfg.TLS.ClientKey,
							}, log)
							if err == nil {
								reconnectSuccess = true
								if addr != cfg.EdgeAddr {
									log.Infof("通过归属 Edge 地址重连成功: %s", addr)
								}
								break
							}
							log.Warnf("重连 Edge %s 失败 (第 %d/%d 次): %v", addr, attempt, maxReconnectAttempts, err)
						}
						if reconnectSuccess {
							break
						}
						select {
						case <-stopCh:
							return
						case <-time.After(connectDelay):
						}
						// L1 修复: 使用配置的最大延迟
						maxDelay := cfg.ReconnectMaxDelay
						if maxDelay <= 0 {
							maxDelay = 30 * time.Second
						}
						if connectDelay < maxDelay {
							connectDelay *= 2
						}
					}

					// M3: 重连失败，降级为本地缓存模式
					if !reconnectSuccess {
						log.Errorf("[M3] 重连 Edge 节点失败，已达到最大重试次数 %d，进入本地缓存模式", maxReconnectAttempts)
						metricCollector.RecordDataDropped(0, "edge_unavailable")
						// 等待一段时间后再次尝试重连
						select {
						case <-stopCh:
							return
						case <-time.After(5 * time.Minute):
							log.Info("[M3] 本地缓存模式：尝试重新连接 Edge...")
							failureCount = 0 // 重置失败计数，允许再次重连
							continue
						}
					}

						// 重新注册
						hostname, _ = os.Hostname()
						agentIP := getLocalIP()
						regCtx, regCancel := context.WithTimeout(context.Background(), 10*time.Second)
						newResp, err := newClient.Register(regCtx, cfg.ProbeID, agentIP, hostname, Version)
						regCancel()
						if err != nil {
							log.Errorf("重新注册失败: %v", err)
							failureCount = 0
							continue
						}
						// 检查注册是否成功
						if !newResp.GetSuccess() {
							log.Errorf("重新注册被拒绝: %s，请检查 API Key 配置", newResp.GetMessage())
							failureCount = 0
							// API Key 不匹配时停止重试，避免无限循环
							log.Errorf("API Key 配置错误，探针将停止运行，请修改配置后重启")
							stopOnce.Do(func() {
								close(stopCh)
							})
							return
						}

						// 重连成功，更新客户端和心跳间隔
						safeClient.Set(newClient)
						heartbeatInterval = time.Duration(newResp.GetHeartbeatInterval()) * time.Second
						ticker.Reset(heartbeatInterval)
						failureCount = 0
						// 更新本地归属 Edge 信息
						if newResp.GetAssignedEdgeId() != "" {
							saveAssignedEdge(cfg.ProbeID, newResp.GetAssignedEdgeId(), newResp.GetSessionId(), candidateAddrs[0])
						}
						log.Infof("重连成功: %s, 心跳间隔=%ds", newResp.GetMessage(), newResp.GetHeartbeatInterval())
					}
				} else {
					// 心跳成功，重置失败计数
					failureCount = 0
					metricCollector.HeartbeatSent(nil)
					// 记录自监控心跳状态
					if selfMonitorCollector != nil {
						selfMonitorCollector.RecordHeartbeat(true, time.Since(heartbeatStart))
					}
				}
				heartbeatCancel() // 直接调用而非 defer，因为 defer 在 for 循环内会延迟到函数退出才执行
			case <-stopCh:
				return
			}
		}
	}()

	// 指标采集 + 发送协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(cfg.CollectInterval) * time.Second)
		defer ticker.Stop()
		// L1 修复: 使用配置的刷新间隔
		flushInterval := cfg.FlushInterval
		if flushInterval <= 0 {
			flushInterval = 30 * time.Second // 默认值
		}
		flushTicker := time.NewTicker(flushInterval)
		defer flushTicker.Stop()
		var buf []*edge.MetricData

		// 网卡恢复检测ticker
		netCheckTicker := time.NewTicker(5 * time.Second)
		defer netCheckTicker.Stop()

		// 缓存补发状态
		wasUnavailable := false

		for {
			select {
			case <-ticker.C:
				// 根据过载熔断状态决定采集级别
				if overloadBreaker != nil && overloadBreaker.IsSilent() {
					// 完全静默模式：跳过所有采集
					continue
				}

				// 采集传统指标（非核心指标，降级时跳过）
				if overloadBreaker == nil || overloadBreaker.ShouldCollectExtended() {
					start := metricCollector.CollectStarted()
					metricsData, collectErr := c.Collect()
					metricCollector.CollectFinished(start, collectErr)
					if collectErr != nil {
						log.Warnf("采集传统指标失败: %v", collectErr)
					} else {
						buf = append(buf, metricsData...)
					}
				}

				// 采集 EBPF 网络流量数据
				if ebpfCollector != nil {
					ebpfMetrics := ebpfCollector.Collect()
					metricCollector.EBPFCollect(nil)
					buf = append(buf, ebpfMetrics...)
				}

				// 采集 SQL 聚合分析数据（非核心指标，降级时跳过）
				if sqlAggregator != nil && (overloadBreaker == nil || overloadBreaker.ShouldCollectExtended()) {
					sqlMetrics := sqlAggregator.GetMetrics()
					buf = append(buf, sqlMetrics...)
				}

				if len(buf) >= cfg.BatchSize {
					batch := &edge.MetricsBatch{
						ProbeId: cfg.ProbeID,
						Metrics: buf,
					}

					// 使用可靠上报器（带校验和+离线缓存+自动重传）
					if reliableReporter != nil {
						sendStart := metricCollector.SendStarted()
						sent := reliableReporter.Send(batch)
						if sent {
							metricCollector.SendFinished(sendStart, len(buf)*100, nil)
							buf = nil
						} else {
							metricCollector.SendFinished(sendStart, 0, fmt.Errorf("已缓存"))
							buf = nil // reliableReporter 内部已缓存，清空 buf
						}
					} else {
						// 降级：使用基础发送模式
						if netMonitor.IsAvailable() {
						sendStart := metricCollector.SendStarted()
						sendCtx, sendCancel := context.WithTimeout(context.Background(), 10*time.Second)
						sendClient := safeClient.Get()
						if sendClient == nil {
							log.Warn("gRPC 客户端未初始化，走缓存逻辑")
							sendCancel()
							if tsStore != nil {
								if cacheErr := cacheMetrics(tsStore, buf); cacheErr != nil {
									log.Warnf("写入本地缓存失败: %v", cacheErr)
									metricCollector.RecordDataDropped(len(buf), "cache_failed")
								} else {
									log.Infof("[数据保护] gRPC客户端为空，已将 %d 条指标数据写入本地缓存", len(buf))
								}
							} else {
								metricCollector.RecordDataDropped(len(buf), "no_cache")
							}
							buf = nil
						} else {
							err := sendClient.SendMetrics(sendCtx, batch)
							sendCancel()
							if err != nil {
								log.Errorf("发送指标数据失败: %v", err)
								metricCollector.SendFinished(sendStart, 0, err)
								// C3 修复: 优先写入本地缓存
								if tsStore != nil {
									if cacheErr := cacheMetrics(tsStore, buf); cacheErr != nil {
										log.Warnf("写入本地缓存失败: %v", cacheErr)
										// 缓存失败，记录数据丢失
										metricCollector.RecordDataDropped(len(buf), "cache_failed")
										log.Errorf("[数据丢失] 本地缓存写入失败，丢弃 %d 条指标数据", len(buf))
									} else {
										log.Infof("[数据保护] 已将 %d 条指标数据写入本地缓存", len(buf))
									}
								} else {
									// 无本地缓存，记录数据丢失
									metricCollector.RecordDataDropped(len(buf), "no_cache")
									log.Errorf("[数据丢失] 无本地缓存，丢弃 %d 条指标数据", len(buf))
								}
								// C3 修复: 缓冲区溢出时记录日志和 metrics
								if len(buf) > cfg.BatchSize*2 {
									droppedCount := len(buf) - cfg.BatchSize
									metricCollector.RecordBufOverflow()
									metricCollector.RecordDataDropped(droppedCount, "buffer_overflow")
									log.Warnf("[缓冲区溢出] buf 大小 %d 超过阈值 %d，丢弃最旧的 %d 条数据",
										len(buf), cfg.BatchSize*2, droppedCount)
									buf = buf[len(buf)-cfg.BatchSize:]
								}
							} else {
								metricCollector.SendFinished(sendStart, len(buf)*100, nil)
								buf = nil
							}
						}
						} else {
							// C3 修复: 网卡不可用时优先写入本地缓存
							if tsStore != nil {
								if cacheErr := cacheMetrics(tsStore, buf); cacheErr != nil {
									log.Warnf("写入本地缓存失败: %v", cacheErr)
									metricCollector.RecordDataDropped(len(buf), "cache_failed")
									log.Errorf("[数据丢失] 网卡不可用且本地缓存写入失败，丢弃 %d 条指标数据", len(buf))
								} else {
									log.Infof("[数据保护] 网卡不可用，已将 %d 条指标数据写入本地缓存", len(buf))
								}
							} else {
								metricCollector.RecordDataDropped(len(buf), "network_unavailable")
								log.Errorf("[数据丢失] 网卡不可用且本地缓存未启用，丢弃 %d 条指标数据", len(buf))
							}
							buf = nil
						}
					}
				}
			case <-netCheckTicker.C:
				// 检测网卡恢复，触发缓存补发
				if wasUnavailable && netMonitor.IsAvailable() {
					log.Info("网卡已恢复，开始补发缓存数据...")
					flushClient := safeClient.Get()
					if flushClient == nil {
						log.Warn("gRPC 客户端未初始化，跳过补发缓存数据")
					} else if tsStore != nil {
						if err := flushCachedMetrics(tsStore, flushClient, cfg.ProbeID, metricCollector, log); err != nil {
							log.Warnf("补发缓存数据失败: %v", err)
						}
					}
					wasUnavailable = false
				} else if !netMonitor.IsAvailable() {
					wasUnavailable = true
				}
			case <-flushTicker.C:
				// 每30秒flush一次日志
				log.Sync()
			case <-stopCh:
				// 发送剩余数据
				if len(buf) > 0 {
					if netMonitor.IsAvailable() {
						batch := &edge.MetricsBatch{
							ProbeId: cfg.ProbeID,
							Metrics: buf,
						}
						sendStart := metricCollector.SendStarted()
						sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						stopClient := safeClient.Get()
						if stopClient == nil {
							log.Warn("gRPC 客户端未初始化，将剩余数据写入缓存")
							cancel()
							if tsStore != nil {
								_ = cacheMetrics(tsStore, buf)
							}
						} else if err := stopClient.SendMetrics(sendCtx, batch); err != nil {
							log.Warnf("发送剩余数据失败: %v", err)
							metricCollector.SendFinished(sendStart, 0, err)
							// 尝试写入缓存
							if tsStore != nil {
								_ = cacheMetrics(tsStore, buf)
							}
						} else {
							metricCollector.SendFinished(sendStart, len(buf)*100, nil)
						}
						cancel()
					} else if tsStore != nil {
						// 网卡不可用，写入缓存
						_ = cacheMetrics(tsStore, buf)
					}
				}
				return
			}
		}
	}()

	// 启动 HTTP 健康检查服务器
	http.Version = Version
	healthHandler := http.NewHealthHandler(safeClient, c, ebpfCollector, nil, sqlAggregator, log)
	healthAddr := fmt.Sprintf(":%s", cfg.HealthPort)
	healthServer := http.StartHealthServer(healthAddr, healthHandler)
	log.Infof("健康检查 HTTP 服务监听: %s/health", healthAddr)

	// 初始化 ON-CPU 剖析器（如果启用）
	if cfg.EBPF.CPUProfiler.Enabled {
		log.Infof("ON-CPU 剖析器已启用: 采样频率=%dHz, 目标PID=%d, 输出目录=%s",
			cfg.EBPF.CPUProfiler.SampleFreq, cfg.EBPF.CPUProfiler.TargetPID, cfg.EBPF.CPUProfiler.OutputDir)
	}

	log.Info("探针已启动")

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("收到信号 %v，正在退出...", sig)

	// 优雅退出超时
	const gracefulShutdownTimeout = 30 * time.Second
	ctx, cancel = context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer cancel()

	// 停止所有goroutine
	stopOnce.Do(func() {
		close(stopCh)
	})

	// 等待goroutine退出
	done := make(chan struct{})
	go func() {
		// 等待所有goroutine完成
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("所有goroutine已安全退出")
	case <-ctx.Done():
		log.Warnf("优雅退出超时，强制关闭")
	}

	// 关闭 Prometheus 指标服务器
	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Warnf("关闭 Prometheus 指标服务器失败: %v", err)
		} else {
			log.Info("Prometheus 指标服务器已优雅关闭")
		}
	}

	// 关闭 HTTP 健康检查服务器
	if healthServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			log.Warnf("关闭 HTTP 服务器失败: %v", err)
		} else {
			log.Info("HTTP 服务器已优雅关闭")
		}
	}

	// 关闭客户端连接
	if cl := safeClient.Get(); cl != nil {
		cl.Close()
	}
	log.Info("探针已安全退出")
}

// assignedEdgeInfo 归属 Edge 完整信息（分布式边缘自治）
type assignedEdgeInfo struct {
	EdgeID   string
	Addr     string
	SessionID string
}

// saveAssignedEdge 持久化归属 Edge 到本地文件（分布式边缘自治）
func saveAssignedEdge(probeID, edgeID, sessionID, addr string) {
	dir := filepath.Join(os.TempDir(), "cloud-flow-edge")
	// FIX: 检查目录创建错误
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data := fmt.Sprintf("%s\t%s\t%s\t%s\t%d", probeID, edgeID, sessionID, addr, time.Now().Unix())
	// FIX: 检查文件写入错误
	if err := os.WriteFile(filepath.Join(dir, probeID+".edge"), []byte(data), 0600); err != nil {
		return
	}
}

// loadAssignedEdge 从本地文件恢复归属 Edge（分布式边缘自治）
func loadAssignedEdge(probeID string) *assignedEdgeInfo {
	path := filepath.Join(os.TempDir(), "cloud-flow-edge", probeID+".edge")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parts := strings.Split(string(data), "\t")
	if len(parts) >= 5 {
		return &assignedEdgeInfo{
			EdgeID:   parts[1],
			SessionID: parts[2],
			Addr:     parts[3],
		}
	}
	// 兼容旧格式（4个字段，无 addr）
	if len(parts) >= 3 {
		return &assignedEdgeInfo{
			EdgeID:   parts[1],
			SessionID: parts[2],
		}
	}
	return nil
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		hostname, _ := os.Hostname()
		if hostname != "" {
			return hostname
		}
		return "0.0.0.0" // 最终兜底值
	}

	// 优先返回 IPv4 地址
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}

	// 如果没有 IPv4 地址，返回 IPv6 地址
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			return ipnet.IP.String()
		}
	}

	hostname, _ := os.Hostname()
	if hostname != "" {
		return hostname
	}
	return "0.0.0.0" // 最终兜底值
}

// cacheMetrics 将指标数据写入本地缓存
func cacheMetrics(store *storage.TimeSeriesStore, metrics []*edge.MetricData) error {
	if store == nil || len(metrics) == 0 {
		return nil
	}

	points := make([]storage.DataPoint, 0, len(metrics))
	now := time.Now().UnixNano()

	for _, m := range metrics {
		fields := make(map[string]interface{})
		if m.Value != 0 {
			fields["value"] = m.Value
		}
		if m.Tags != nil {
			for k, v := range m.Tags {
				fields[k] = v
			}
		}

		points = append(points, storage.DataPoint{
			Timestamp: now,
			Tags: map[string]string{
				"probe_id":  m.ProbeId,
				"metric_id": m.MetricId,
				"name":      m.Name,
			},
			Fields:   fields,
			DataType: storage.DataTypeMetric,
			Source:   "agent",
		})
	}

	return store.Write(points...)
}

// flushCachedMetrics 从缓存读取并补发数据
func flushCachedMetrics(store *storage.TimeSeriesStore, client *grpcclient.Client, probeID string, metricCollector *metrics.Collector, log *logger.Logger) error {
	if store == nil || client == nil {
		return nil
	}

	// 查询缓存的指标数据
	query := &storage.Query{
		StartTime: 0,
		EndTime:   time.Now().UnixNano(),
		DataTypes: []storage.DataType{storage.DataTypeMetric},
		UseIndex:  true,
	}

	result, err := store.Query(query)
	if err != nil {
		return fmt.Errorf("查询缓存数据失败: %w", err)
	}

	if len(result.Points) == 0 {
		log.Info("没有需要补发的缓存数据")
		return nil
	}

	log.Infof("开始补发 %d 条缓存数据...", len(result.Points))

	// 将DataPoint转换回MetricData
	var metricsData []*edge.MetricData
	for _, p := range result.Points {
		md := &edge.MetricData{
			ProbeId: probeID,
			MetricId: p.Tags["metric_id"],
			Name:    p.Tags["name"],
			Tags:    make(map[string]string),
		}
		if v, ok := p.Fields["value"].(float64); ok {
			md.Value = v
		}
		// 复制其他标签
		for k, v := range p.Fields {
			if k != "value" {
				if s, ok := v.(string); ok {
					md.Tags[k] = s
				}
			}
		}
		metricsData = append(metricsData, md)
	}

	// 分批发送
	batchSize := 100
	sentCount := 0
	failedCount := 0

	for i := 0; i < len(metricsData); i += batchSize {
		end := i + batchSize
		if end > len(metricsData) {
			end = len(metricsData)
		}

		batch := &edge.MetricsBatch{
			ProbeId: probeID,
			Metrics: metricsData[i:end],
		}

		sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		sendStart := metricCollector.SendStarted()
		err := client.SendMetrics(sendCtx, batch)
		cancel()

		if err != nil {
			log.Warnf("补发批次失败 (%d-%d): %v", i, end, err)
			metricCollector.SendFinished(sendStart, 0, err)
			failedCount += end - i
		} else {
			metricCollector.SendFinished(sendStart, (end-i)*100, nil)
			sentCount += end - i
		}

		// 避免发送过快
		time.Sleep(100 * time.Millisecond)
	}

	log.Infof("缓存数据补发完成: 成功=%d, 失败=%d", sentCount, failedCount)
	return nil
}
