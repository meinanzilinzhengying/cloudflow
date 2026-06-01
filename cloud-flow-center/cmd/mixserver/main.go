// Mix Server — 数据统计计算服务
package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"cloud-flow-center/internal/config"
	"cloud-flow-center/internal/mixserver"
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

	port := cfg.MixServer.Port
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = 0
		if _, err := fmt.Sscanf(envPort, "%d", &port); err == nil && port > 0 {
		}
	}
	if port == 0 {
		port = 8081
	}

	authUser := cfg.MixServer.AuthUser
	authPass := cfg.MixServer.AuthPass

	if authUser != "" {
		log.Info("MixServer HTTP Basic Auth 已启用")
	} else {
		log.Warn("MixServer 未配置认证")
	}

	tidbDSN := cfg.Storage.DSN
	if tidbDSN == "" {
		log.Fatal("TiDB DSN 未配置，请设置 center.storage.dsn 配置项")
	}

	db, err := sql.Open("mysql", tidbDSN)
	if err != nil {
		log.Errorf("打开 TiDB 数据库失败: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Errorf("TiDB 数据库连接失败: %v", err)
		os.Exit(1)
	}

	log.Infof("TiDB 数据库连接成功")

	allowedOrigins := []string{"http://localhost:3000"}
	if origins := os.Getenv("CLOUD_FLOW_CORS_ALLOWED_ORIGINS"); origins != "" {
		allowedOrigins = strings.Split(origins, ",")
		log.Infof("从环境变量读取 CORS 允许的来源: %v", allowedOrigins)
	}

	analyzer, err := mixserver.NewAnalyzer(db, log, allowedOrigins)
	if err != nil {
		log.Errorf("创建分析器失败: %v", err)
		os.Exit(1)
	}
	analyzer.Start()

	mux := http.NewServeMux()

	basicAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if authUser == "" || authPass == "" {
				next(w, r)
				return
			}
			user, pass, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(user), []byte(authUser)) != 1 ||
				subtle.ConstantTimeCompare([]byte(pass), []byte(authPass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Cloud Flow MixServer"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/api/summaries", basicAuth(analyzer.SummaryHandler()))
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})

	httpServer := &http.Server{
		Addr:         ":" + fmt.Sprintf("%d", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Infof("MixServer 监听: :%d", port)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Errorf("MixServer 异常: %v，触发优雅关闭", err)
			sigCh <- syscall.SIGTERM
		}
	}()

	log.Info("MixServer 已启动")

	sig := <-sigCh
	log.Infof("收到信号 %v，退出中...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	analyzer.Stop()
	log.Info("MixServer 已停止")
}
