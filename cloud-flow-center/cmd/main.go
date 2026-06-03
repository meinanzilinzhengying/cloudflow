package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"cloudflow-center/internal/alerting"
	"cloudflow-center/internal/cluster"
	"cloudflow-center/internal/config"
	"cloudflow-center/internal/edgeregistry"
	"cloudflow-center/internal/grpcserver"
	"cloudflow-center/internal/kafkaconsumer"
	"cloudflow-center/internal/portal"
	"cloudflow-center/internal/storage"
	"cloudflow-center/pkg/audit"
	"cloudflow-center/pkg/logger"
	"cloudflow-center/pkg/metrics"
	"cloudflow/pkg/safety"
	edge "cloudflow/proto"
)

func main() {
	// 初始化配置管理器
	configMgr := config.NewConfigManager(func(format string, args ...interface{}) {
		fmt.Printf(format+"\n", args...)
	})
	
	if err := configMgr.LoadAndWatch(); err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	defer configMgr.Stop()
	
	cfg := configMgr.GetConfig()

	log := logger.New(logger.Config{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		Output:     cfg.Log.Output,
		LogDir:     cfg.Log.LogDir,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
	})
	defer log.Sync()

	log.Infof("中心服务启动中... 配置: %s", cfg.Summary())

	// 注册配置变更回调
	configMgr.RegisterCallback(func(oldCfg, newCfg *config.Config) {
		log.Infof("配置已更新，旧配置: %s，新配置: %s", oldCfg.Summary(), newCfg.Summary())
		
		// 动态更新日志级别
		if oldCfg.Log.Level != newCfg.Log.Level {
			log.Infof("日志级别变更: %s -> %s", oldCfg.Log.Level, newCfg.Log.Level)
		}
		
		// 可以添加更多配置变更处理逻辑
	})

	// 初始化集群管理器
	clusterMgr, err := cluster.NewManager(cluster.Config{
		EtcdEndpoints: cfg.Cluster.EtcdEndpoints,
		LeaseTTL:      cfg.Cluster.LeaseTTL,
		NodeID:        cfg.Cluster.NodeID,
		NodeAddr:      cfg.Cluster.NodeAddr,
		StoragePath:   cfg.DataDir,
	}, log)
	if err != nil {
		log.Warnf("初始化集群管理器失败: %v，将以单机模式运行", err)
	} else {
		clusterMgr.Start()
		defer clusterMgr.Stop()
	}



	store, err := storage.NewStorageEngine(cfg.Storage, log)
	if err != nil {
		log.Errorf("初始化存储失败: %v", err)
		os.Exit(1)
	}
	store.StartCleanup()

	// 初始化告警管理器
	ruleDir := filepath.Join(cfg.DataDir, "alerting", "rules")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		log.Warnf("创建规则目录 %s 失败: %v", ruleDir, err)
	}

	// 创建多渠道通知器
	multiNotifier := alerting.NewMultiNotifier(log)

	// 添加邮件通知器（如果启用）
	if cfg.Alerting.Email.Enabled {
		emailNotifier := alerting.NewEmailNotifier(
			cfg.Alerting.Email.SMTPHost,
			fmt.Sprintf("%d", cfg.Alerting.Email.SMTPPort),
			cfg.Alerting.Email.SMTPUsername,
			cfg.Alerting.Email.SMTPPassword,
			cfg.Alerting.Email.From,
			cfg.Alerting.Email.To,
			log,
		)
		multiNotifier.AddNotifier(emailNotifier)
		log.Info("邮件通知器已添加")
	}

	// 添加Webhook通知器（如果启用）
	if cfg.Alerting.Webhook.Enabled && cfg.Alerting.Webhook.URL != "" {
		webhookNotifier := alerting.NewWebhookNotifier(
			cfg.Alerting.Webhook.URL,
			log,
		)
		multiNotifier.AddNotifier(webhookNotifier)
		log.Info("Webhook通知器已添加")
	}



	// 初始化告警管理器
	alertMgr := alerting.NewAlertManager(ruleDir, store, getDB(store), multiNotifier, log, 0)
	alertMgr.Start()
	defer alertMgr.Stop()
	log.Info("告警系统已启动")

	srv := grpcserver.NewServer(store, log, cfg.APIKey)

	// P1: 初始化 Edge 注册表，注入到 gRPC Server
	edgeRegistry := edgeregistry.NewRegistry()
	edgeRegistry.StartCleanup(30*time.Second, 90*time.Second) // 每30秒清理，90秒超时
	srv.SetEdgeRegistry(edgeRegistry)
	defer edgeRegistry.Close()
	log.Info("Edge 节点注册表已启动")

	// 构建 gRPC Server 选项（TLS + 限流 + 其他）
	serverOpts, err := grpcserver.BuildServerOpts(cfg.TLS, cfg.RateLimit, cfg.APIKey, log)
	if err != nil {
		log.Errorf("构建 gRPC 服务端选项失败: %v", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer(serverOpts...)
	edge.RegisterCenterServiceServer(grpcServer, srv)

	listenAddr := fmt.Sprintf(":%d", cfg.GRPCListenPort)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Errorf("监听端口失败: %v", err)
		os.Exit(1)
	}

	safety.SafeGo(log, "grpc-server", func() {
		log.Infof("gRPC 服务监听: %s", listenAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Errorf("gRPC 服务异常: %v", err)
		}
	})

	// 启动 Portal HTTP 服务
	auditDir := cfg.DataDir + "/audit"
	auditLogger, err := audit.NewLogger(auditDir, log)
	if err != nil {
		log.Warnf("初始化审计日志记录器失败: %v，审计功能已禁用", err)
	} else {
		log.Info("审计日志记录器已初始化")
		defer auditLogger.Stop()
	}

	// C5 修复: JWT Secret 已在 config.go 中强制要求，此处直接使用
	// N2 修复: 移除冗余安全检查（config.Load() 已验证，此处 jwtSecret 必不为空）
	jwtSecret := cfg.JWT.SecretKey

	secureCookie := cfg.TLS.Enabled
	portalSrv := portal.NewServer(store, jwtSecret, auditLogger, alertMgr, log, secureCookie, cfg.RateLimit, time.Duration(cfg.JWT.TokenDuration)*time.Hour, cfg.Portal.RedisAddr, cfg, configMgr)

	httpListenAddr := fmt.Sprintf(":%d", cfg.Portal.Port)
	httpServer := &http.Server{
		Addr:         httpListenAddr,
		Handler:      portalSrv.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	safety.SafeGo(log, "portal-server", func() {
		log.Infof("Portal HTTP 服务监听: %s", httpListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Portal HTTP 服务异常: %v", err)
		}
	})

	// P0: Flow Ingest Pipeline - 启动 Kafka 消费者（如果启用）
	var kafkaConsumer *kafkaconsumer.Consumer
	if cfg.KafkaConsumer.Enabled && len(cfg.KafkaConsumer.Brokers) > 0 {
		topics := cfg.KafkaConsumer.Topics
		if len(topics) == 0 {
			// 默认订阅所有数据 topic
			topics = []string{"flow.raw", "metrics", "traces", "logs", "profiling"}
		}
		groupID := cfg.KafkaConsumer.GroupID
		if groupID == "" {
			groupID = "cloud-flow-center"
		}
		kafkaConsumer, err = kafkaconsumer.New(cfg.KafkaConsumer.Brokers, groupID, topics, store, log)
		if err != nil {
			log.Errorf("创建 Kafka 消费者失败: %v", err)
		} else {
			if err := kafkaConsumer.Start(); err != nil {
				log.Errorf("启动 Kafka 消费者失败: %v", err)
			} else {
				defer kafkaConsumer.Stop()
				log.Infof("P0: Kafka 消费者已启动 (brokers=%v, topics=%v)", cfg.KafkaConsumer.Brokers, topics)
			}
		}
	} else {
		log.Info("Kafka 消费者未启用（配置中设置 enabled=true 以启用）")
	}

	log.Info("中心服务已启动")

	// 启动 Prometheus metrics HTTP 服务（独立端口，避免与 Portal 冲突）
	metricsPort := 9191
	if p := os.Getenv("METRICS_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			metricsPort = v
		}
	}
	metrics.StartServer(metricsPort)
	log.Infof("Prometheus metrics 服务监听: :%d", metricsPort)

	// 优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("收到信号 %v，开始优雅关闭...", sig)

	// 注意: configMgr.Stop() 已在 defer 中注册（line 43），此处无需重复调用
	// configMgr.Stop() 内部有 running 标志检查，重复调用无害但冗余

	done := make(chan struct{})
	safety.SafeGo(log, "grpc-shutdown", func() {
		grpcServer.GracefulStop()
		close(done)
	})

	// 优雅关闭 HTTP 服务
	httpDone := make(chan struct{})
	safety.SafeGo(log, "portal-shutdown", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
		close(httpDone)
	})

	select {
	case <-done:
		log.Info("gRPC 服务已停止")
	case <-time.After(30 * time.Second):
		log.Warn("优雅关闭超时，强制停止")
		grpcServer.Stop()
	}

	<-httpDone
	log.Info("Portal HTTP 服务已停止")

	store.Stop()
	log.Info("中心服务已安全退出")
}

// getDB 从存储引擎中提取 *sql.DB，用于告警历史持久化
// L2 修复: 使用接口方法而非类型断言，支持多种存储引擎
func getDB(store storage.StorageEngine) *sql.DB {
	if store == nil {
		return nil
	}
	// 使用接口方法获取底层 DB，支持 TiDB 和其他未来可能的数据库实现
	if db, ok := store.DB().(*sql.DB); ok {
		return db
	}
	return nil
}
