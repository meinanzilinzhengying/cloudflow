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
	"github.com/meinanzilinzhengying/cloudflow/agent/pkg/logger"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mainLoop(ctx, deps)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	deps.Logger.Infof("收到信号 %v，正在退出...", sig)
	cancel()
	wg.Wait()

	deps.Logger.Info("探针已安全退出")
}

// mainLoop 主采集循环
func mainLoop(ctx context.Context, deps *Dependencies) {
	heartbeatInterval := 30 * time.Second
	ticker := time.NewTicker(heartbeatInterval)
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
