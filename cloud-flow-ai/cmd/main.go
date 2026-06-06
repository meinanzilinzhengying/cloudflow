package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meinanzilinzhengying/cloudflow/ai/internal/config"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/llm"
	"github.com/meinanzilinzhengying/cloudflow/ai/internal/server"
	"github.com/meinanzilinzhengying/cloudflow/ai/pkg/logger"
)

func main() {
	fmt.Println("=== Cloud Flow AI Service ===")

	// 初始化配置
	configMgr := config.NewConfigManager(config.WithConfigPath(""))
	if err := configMgr.LoadAndWatch(); err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	defer configMgr.Stop()

	cfg := configMgr.GetConfig()

	// 初始化日志
	log := logger.New(logger.Config{
		Level:      cfg.AI.Log.Level,
		Format:     cfg.AI.Log.Format,
		Output:     cfg.AI.Log.Output,
		LogDir:     cfg.AI.Log.LogDir,
		MaxSize:    cfg.AI.Log.MaxSize,
		MaxBackups: cfg.AI.Log.MaxBackups,
		MaxAge:     cfg.AI.Log.MaxAge,
	})
	defer log.Sync()

	log.Infof("AI 服务启动中... 配置: %s", cfg.Summary())

	// 初始化 LLM 客户端
	llmClient := llm.NewClient(cfg)
	llmClient.SetModels(cfg.AI.LLM.Models)

	// 初始化 HTTP 服务器
	svr := server.NewServer(cfg, log, llmClient)
	addr := fmt.Sprintf(":%d", cfg.AI.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      svr.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 启动 HTTP 服务
	go func() {
		log.Infof("HTTP 服务监听: %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("HTTP 服务异常: %v", err)
		}
	}()

	// 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Infof("收到信号，开始优雅关闭...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Errorf("关闭 HTTP 服务失败: %v", err)
	}
	log.Infof("AI 服务已安全退出")
}
