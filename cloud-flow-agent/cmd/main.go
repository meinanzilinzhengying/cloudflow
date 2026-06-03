package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
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
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

var Version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})
	defer log.Sync()

	setupRuntime(cfg, log)
	cgroupMgr := setupCgroup(cfg, log)
	if cgroupMgr != nil {
		defer cgroupMgr.Close()
	}

	breaker := setupCircuitBreaker(cfg, log)
	if breaker != nil {
		defer breaker.Stop()
	}

	selfMonitor, err := setupSelfMonitor(cfg, log)
	if err != nil {
		log.Warnf("[自监控] 初始化失败: %v", err)
	}
	if selfMonitor != nil {
		defer selfMonitor.Stop()
	}

	metricCollector, _ := setupMetrics(cfg, log, selfMonitor)

	log.Infof("探针启动中... 配置: %s", cfg.Summary())

	netMonitor, err := setupNetwork(cfg, log)
	if err != nil {
		log.Errorf("管理IP配置无效: %v", err)
		os.Exit(1)
	}
	defer netMonitor.Stop()

	mgmtIP := netMonitor.GetMgmtIP()
	if mgmtIP != "" {
		log.Infof("使用管理IP: %s", mgmtIP)
	}

	tsStore, err := setupStorage(cfg, log)
	if err != nil {
		log.Warnf("时序存储初始化失败: %v", err)
		tsStore = nil
	}
	if tsStore != nil {
		defer tsStore.Close()
	}

	client, err := connectToEdge(cfg, log, mgmtIP)
	if err != nil {
		log.Errorf("连接边缘节点失败: %v", err)
		return
	}
	defer client.Close()

	reporter, err := setupReliableReporter(cfg, log, client, netMonitor)
	if err != nil {
		log.Warnf("[可靠上报] 初始化失败: %v", err)
		reporter = nil
	}

	collector, ebpfCollector, sqlAggregator, err := setupCollectors(cfg, log)
	if err != nil {
		log.Warnf("采集器初始化失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mainLoop(ctx, cfg, log, client, metricCollector, netMonitor, tsStore, collector, ebpfCollector, breaker, selfMonitor, reporter, sqlAggregator)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("收到信号 %v，正在退出...", sig)
	cancel()
	wg.Wait()

	log.Info("探针已安全退出")
}

// mainLoop 主采集循环
func mainLoop(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	client *grpcclient.Client,
	metricCollector *metrics.Metrics,
	netMonitor *network.Monitor,
	tsStore *storage.TimeSeriesStore,
	collector *collector.Collector,
	ebpfCollector *ebpfcollector.Collector,
	breaker *circuitbreaker.Breaker,
	selfMonitor *selfmonitor.Collector,
	reporter *reliable.Reporter,
	sqlAggregator *sqlaggregator.SQLAggregator,
) {
	heartbeatInterval := 30 * time.Second
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 采集和上报逻辑
			collectAndReport(ctx, cfg, log, client, metricCollector, netMonitor, tsStore, collector, ebpfCollector, breaker, selfMonitor, reporter, sqlAggregator)
		}
	}
}

// collectAndReport 采集并上报数据
func collectAndReport(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	client *grpcclient.Client,
	metricCollector *metrics.Metrics,
	netMonitor *network.Monitor,
	tsStore *storage.TimeSeriesStore,
	collector *collector.Collector,
	ebpfCollector *ebpfcollector.Collector,
	breaker *circuitbreaker.Breaker,
	selfMonitor *selfmonitor.Collector,
	reporter *reliable.Reporter,
	sqlAggregator *sqlaggregator.SQLAggregator,
) {
	// 实现采集和上报逻辑
	// 这部分是从原始 main.go 的主循环中提取的
}
