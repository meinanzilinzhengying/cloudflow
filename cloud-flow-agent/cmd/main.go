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

// shutdownComponents 关闭所有组件，确保优雅退出
func shutdownComponents(ctx context.Context, deps *Dependencies) {
	var wg sync.WaitGroup

	components := []struct {
		name string
		stop func()
	}{
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

	for _, c := range components {
		wg.Add(1)
		go func(name string, stop func()) {
			defer wg.Done()
			stop()
		}(c.name, c.stop)
	}

	// 等待所有组件关闭，最多 30 秒
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

// mainLoop 主采集循环
func mainLoop(ctx context.Context, deps *Dependencies) {
	defer func() {
		if r := recover(); r != nil {
			deps.Logger.Errorf("mainLoop panic: %v", r)
		}
	}()
	deps.Logger.Infof("mainLoop 启动，采集间隔: %ds", 30)
	heartbeatInterval := 30
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
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
func collectAndReport(ctx context.Context, deps *Dependencies) {
	// 调试：写入文件确认函数被调用
	f, _ := os.OpenFile("/tmp/agent-collect.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		f.WriteString(fmt.Sprintf("%s: collectAndReport called\n", time.Now().Format("15:04:05")))
		f.Close()
	}
	if deps.Reporter == nil {
		deps.Logger.Warnf("[采集] Reporter 未初始化，跳过此次采集")
		return
	}

	var allMetrics []*edge.MetricData

	// 采集传统指标（CPU、内存、网络、磁盘）
	if deps.LegacyCollector != nil {
		metrics, err := deps.LegacyCollector.Collect()
		if err != nil {
			deps.Logger.Warnf("[采集] 传统采集失败: %v", err)
		} else {
			deps.Logger.Debugf("[采集] 传统采集返回 %d 条指标", len(metrics))
			if len(metrics) > 0 {
				allMetrics = append(allMetrics, metrics...)
			}
		}
	}

	// 采集 eBPF 指标
	if deps.EBPFCollector != nil {
		ebpfMetrics := deps.EBPFCollector.Collect()
		if len(ebpfMetrics) > 0 {
			allMetrics = append(allMetrics, ebpfMetrics...)
		}
	}

	if len(allMetrics) == 0 {
		deps.Logger.Debug("[采集] 本轮无数据")
		return
	}

	// 设置 ProbeId
	for _, m := range allMetrics {
		m.ProbeId = deps.Config.ProbeID
	}

	// 创建批次
	batch := &edge.MetricsBatch{
		ProbeId: deps.Config.ProbeID,
		Metrics: allMetrics,
	}

	// 通过可靠上报器发送
	deps.Reporter.Send(batch)
	deps.Logger.Infof("[上报] 已发送 %d 条指标", len(allMetrics))
}
