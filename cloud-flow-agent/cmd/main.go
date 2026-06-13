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
	shutdownComponents(deps)

	deps.Logger.Info("探针已安全退出")
}

// shutdownComponents 关闭所有组件，确保优雅退出
func shutdownComponents(deps *Dependencies) {
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
	case <-time.After(30 * time.Second):
		// 超时
	}
}

// mainLoop 主采集循环
func mainLoop(ctx context.Context, deps *Dependencies) {
	heartbeatInterval := 30
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndReport(ctx, deps)
		}
	}
}

// collectAndReport 采集并上报数据
func collectAndReport(ctx context.Context, deps *Dependencies) {
	// 实现采集和上报逻辑
}
