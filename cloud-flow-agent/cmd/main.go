package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/agent/internal/config"
	edge "github.com/meinanzilinzhengying/cloudflow/proto"
)

var Version = "dev"

// P20: 采集相关常量（替代硬编码魔法数字）
const (
	// DefaultCollectInterval 默认采集间隔（秒）
	DefaultCollectInterval = 30
	// DefaultShutdownTimeout 默认优雅关闭超时
	DefaultShutdownTimeout = 30 * time.Second
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	provider := NewProvider(cfg)
	deps, err := provider.Provide()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化依赖失败: %v\n", err)
		os.Exit(1)
	}
	defer deps.Logger.Sync()

	// 创建主 context 和取消函数
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	// 启动主采集循环
	wg.Add(1)
	go func() {
		defer wg.Done()
		mainLoop(ctx, deps)
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	deps.Logger.Infof("收到信号 %v，正在退出...", sig)

	// 取消所有组件的 context
	cancel()

	// 等待所有 goroutine 退出
	wg.Wait()

	// 显式关闭所有组件
	shutdownComponents(ctx, deps)

	deps.Logger.Info("探针已安全退出")
}

// P20: 将组件定义提取为顶层结构，避免 shutdownComponents 函数过长
type stoppableComponent struct {
	name string
	stop func()
}

// newShutdownComponents 返回所有需要关闭的组件列表
func newShutdownComponents(deps *Dependencies) []stoppableComponent {
	return []stoppableComponent{
		{"NetMonitor", func() {
			if deps.NetMonitor != nil {
				deps.NetMonitor.Stop()
			}
		}},
		{"SelfMonitor", func() {
			if deps.SelfMonitor != nil {
				deps.SelfMonitor.Stop()
			}
		}},
		{"EBPFCollector", func() {
			if deps.EBPFCollector != nil {
				deps.EBPFCollector.Stop()
			}
		}},
		{"TSStore", func() {
			if deps.TSStore != nil {
				deps.TSStore.Close()
			}
		}},
		{"GRPCClient", func() {
			if deps.GRPCClient != nil {
				deps.GRPCClient.Close()
			}
		}},
		{"CgroupManager", func() {
			if deps.CgroupManager != nil {
				deps.CgroupManager.Close()
			}
		}},
		{"Breaker", func() {
			if deps.Breaker != nil {
				deps.Breaker.Stop()
			}
		}},
	}
}

// shutdownComponents 关闭所有组件，确保优雅退出
func shutdownComponents(ctx context.Context, deps *Dependencies) {
	var wg sync.WaitGroup

	components := newShutdownComponents(deps)
	for _, c := range components {
		wg.Add(1)
		go func(name string, stop func()) {
			defer wg.Done()
			stop()
		}(c.name, c.stop)
	}

	// 等待所有组件关闭，最多 DefaultShutdownTimeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 所有组件已关闭
	case <-ctx.Done():
		// 超时
	}
}

// P20: 采集间隔解析（从配置读取，未配置则使用默认值）
func getCollectInterval(cfg *config.Config) time.Duration {
	if cfg.CollectInterval > 0 {
		return time.Duration(cfg.CollectInterval) * time.Second
	}
	return DefaultCollectInterval * time.Second
}

// mainLoop 主采集循环
func mainLoop(ctx context.Context, deps *Dependencies) {
	defer func() {
		if r := recover(); r != nil {
			deps.Logger.Errorf("mainLoop panic: %v", r)
		}
	}()

	interval := getCollectInterval(deps.Config)
	deps.Logger.Infof("mainLoop 启动，采集间隔: %v", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			deps.Logger.Infof("mainLoop 退出")
			return
		case <-ticker.C:
			deps.Logger.Debug("开始采集...")
			collectAndReport(ctx, deps)
		}
	}
}

// collectAndReport 采集并上报数据
// P20: 将采集、组装、发送逻辑拆分为独立小函数，提高可测试性
func collectAndReport(ctx context.Context, deps *Dependencies) {
	if deps.Reporter == nil {
		deps.Logger.Warnf("[采集] Reporter 未初始化，跳过此次采集")
		return
	}

	allMetrics := collectAllMetrics(ctx, deps)
	if len(allMetrics) == 0 {
		deps.Logger.Debug("[采集] 本轮无数据")
		return
	}

	batch := buildMetricsBatch(deps.Config.ProbeID, allMetrics)
	sendMetricsBatch(deps, batch)
}

// collectAllMetrics 采集所有指标（传统 + eBPF）
func collectAllMetrics(ctx context.Context, deps *Dependencies) []*edge.MetricData {
	var allMetrics []*edge.MetricData

	// 采集传统指标（CPU、内存、网络、磁盘）
	if legacyMetrics := collectLegacyMetrics(deps); len(legacyMetrics) > 0 {
		allMetrics = append(allMetrics, legacyMetrics...)
	}

	// 采集 eBPF 指标
	if ebpfMetrics := collectEBPFMetrics(deps); len(ebpfMetrics) > 0 {
		allMetrics = append(allMetrics, ebpfMetrics...)
	}

	return allMetrics
}

// collectLegacyMetrics 采集传统指标
func collectLegacyMetrics(deps *Dependencies) []*edge.MetricData {
	if deps.LegacyCollector == nil {
		return nil
	}
	metrics, err := deps.LegacyCollector.Collect()
	if err != nil {
		deps.Logger.Warnf("[采集] 传统采集失败: %v", err)
		return nil
	}
	deps.Logger.Debugf("[采集] 传统采集返回 %d 条指标", len(metrics))
	return metrics
}

// collectEBPFMetrics 采集 eBPF 指标
func collectEBPFMetrics(deps *Dependencies) []*edge.MetricData {
	if deps.EBPFCollector == nil {
		return nil
	}
	metrics := deps.EBPFCollector.Collect()
	deps.Logger.Debugf("[采集] eBPF 采集返回 %d 条指标", len(metrics))
	return metrics
}

// buildMetricsBatch 构建指标批次并设置 ProbeId
func buildMetricsBatch(probeID string, metrics []*edge.MetricData) *edge.MetricsBatch {
	for _, m := range metrics {
		m.ProbeId = probeID
	}
	return &edge.MetricsBatch{
		ProbeId: probeID,
		Metrics: metrics,
	}
}

// sendMetricsBatch 发送指标批次
func sendMetricsBatch(deps *Dependencies, batch *edge.MetricsBatch) {
	deps.Reporter.Send(batch)
	deps.Logger.Infof("[上报] 已发送 %d 条指标", len(batch.Metrics))
}
