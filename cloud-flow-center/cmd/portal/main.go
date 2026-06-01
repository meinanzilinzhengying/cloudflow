// Portal Server — Web 前端展示服务
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud-flow-center/internal/alerting"
	"cloud-flow-center/internal/config"
	"cloud-flow-center/internal/portal"
	"cloud-flow-center/internal/storage"
	"cloud-flow-center/pkg/audit"
	"cloud-flow-center/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log := logger.New(logger.Config{Level: "info", Format: "json"})
		log.Fatalf("加载配置失败: %v", err)
	}

	log := logger.New(logger.Config{Level: cfg.Log.Level, Format: cfg.Log.Format})
	defer log.Sync()

	log.Infof("配置加载成功: %s", cfg.Summary())

	port := cfg.Portal.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = 0
		if _, err := fmt.Sscanf(envPort, "%d", &port); err == nil && port > 0 {
		}
	}
	if port == 0 {
		port = 8080
	}

	store, err := storage.NewStorageEngine(config.StorageConfig{
		DSN:     cfg.Storage.DSN,
		RetDays: cfg.Storage.RetDays,
	}, log)
	if err != nil {
		log.Warnf("初始化存储失败: %v，将使用模拟数据", err)
	}

	var alertMgr *alerting.AlertManager
	ruleDir := cfg.Alerting.RulesPath
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		log.Warnf("创建规则目录 %s 失败: %v", ruleDir, err)
	}
	if err == nil && store != nil {
		multiNotifier := alerting.NewMultiNotifier(log)
		alertMgr = alerting.NewAlertManager(ruleDir, store, getDBFromStore(store), multiNotifier, log, 0)
		alertMgr.Start()
		defer alertMgr.Stop()
		log.Info("告警管理器已启动")
	} else {
		log.Warn("存储引擎不可用，告警功能已禁用")
		store = nil
		alertMgr = nil
	}

	jwtSecret := cfg.JWT.SecretKey
	if jwtSecret == "" {
		log.Fatal("JWT 密钥未配置，请设置 center.jwt.secret_key 配置项")
	}

	log.Info("Portal 用户认证已启用")

	auditDir := cfg.DataDir + "/audit"
	auditLogger, err := audit.NewLogger(auditDir, log)
	if err != nil {
		log.Warnf("初始化审计日志记录器失败: %v，审计功能已禁用", err)
	} else {
		log.Info("审计日志记录器已初始化")
		defer auditLogger.Stop()
	}

	secureCookie := cfg.TLS.Enabled
	srv := portal.NewServer(store, jwtSecret, auditLogger, alertMgr, log, secureCookie, cfg.RateLimit, time.Duration(cfg.JWT.TokenDuration)*time.Hour, cfg.Portal.RedisAddr, cfg)

	httpServer := &http.Server{
		Addr:         ":" + fmt.Sprintf("%d", port),
		Handler:      srv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Infof("Portal 服务监听: :%d", port)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Errorf("Portal 服务异常: %v，触发优雅关闭", err)
			sigCh <- syscall.SIGTERM
		}
	}()

	log.Info("Portal 服务已启动")

	sig := <-sigCh
	log.Infof("收到信号 %v，退出中...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	if store != nil {
		store.Stop()
	}
	log.Info("Portal 服务已停止")
}

func getDBFromStore(store storage.StorageEngine) *sql.DB {
	if store == nil {
		return nil
	}
	if db, ok := store.DB().(*sql.DB); ok {
		return db
	}
	return nil
}
