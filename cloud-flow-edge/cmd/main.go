// 边缘节点服务入口
// 负责探针管理、数据接收、批量转发到中心服务
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"cloudflow-edge/internal/config"
	"cloudflow-edge/internal/forwarder"
	"cloudflow-edge/internal/grpcclient"
	"cloudflow-edge/internal/grpcserver"
	"cloudflow-edge/internal/http"
	"cloudflow-edge/internal/kafkafwd"
	"cloudflow-edge/internal/probemgr"
	"cloudflow-edge/internal/servicediscovery"
	"cloudflow-edge/pkg/logger"
	"cloudflow-edge/pkg/metrics"
	"cloudflow/pkg/utils"
	edge "cloudflow/proto"
)

const gracefulShutdownTimeout = 30 * time.Second

var cfg atomic.Value
var fwd *forwarder.Forwarder
var kafkaFwd *kafkafwd.KafkaForwarder // P0: Kafka 转发器（Flow Ingest Pipeline）
var client *grpcclient.Client
var clientMu sync.RWMutex
var healthServer *http.Server

// loadCfg 安全加载配置一致快照
func loadCfg() *config.Config {
	return cfg.Load().(*config.Config)
}

func main() {
	// 1. 加载配置
	localCfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化全局 atomic.Value
	cfg.Store(localCfg)

	// 2. 初始化日志
	log := logger.New(logger.Config{
		Level:  localCfg.Log.Level,
		Format: localCfg.Log.Format,
	})
	defer log.Sync()

	// 3. 启动配置热加载
	config.StartConfigWatch(func(newCfg *config.Config) {
		log.Infof("配置已更新: %s", newCfg.Summary())
		// 使用 atomic.Value 原子更新配置
		cfg.Store(newCfg)
		// 更新转发器配置（支持热加载）
		if fwd != nil {
			fwd.UpdateConfig(newCfg.BatchSize, newCfg.FlushInterval)
		}
		// API Key 变更时重新创建 Center gRPC 客户端
		clientMu.RLock()
		currentClient := client
		clientMu.RUnlock()
		if currentClient != nil {
			centerAddr := newCfg.CenterAddr
			if err := createCenterClient(centerAddr); err != nil {
				log.Errorf("配置热加载：API Key/TLS 变更后重新创建 Center gRPC 客户端失败: %v", err)
			} else {
				log.Info("配置热加载：Center gRPC 客户端已重新创建（API Key/TLS 配置已更新）")
			}
		}
		// TLS 证书变更时重新加载 TLS 配置
		// 注意：gRPC Server 的 TLS 证书在创建时已固定到 Server 实例，
		// 当前 gRPC Server 不支持动态更换 TLS 证书，需要重启服务才能生效。
		// 此处仅记录日志提示用户。
		// TODO(I-06): 未来可使用 fsnotify (https://github.com/fsnotify/fsnotify)
		// 监听 TLS 证书文件（cert.pem / key.pem）的变更事件，实现证书热更新，
		// 避免因证书轮换而重启整个边缘节点服务。
		if newCfg.TLS.Enabled {
			log.Infof("配置热加载：TLS 配置已更新（新证书将在下次重启后生效）")
		}
		// 注意：以下配置变更需要重启服务才能生效：
		// - gRPC 监听端口（需要重新绑定端口监听）
		// - 服务发现配置（Discovery 实例在启动时创建，不支持动态重建）
		log.Warnf("配置热加载提示：gRPC 监听端口和服务发现配置变更需要重启服务才能生效")
	}, log)

	log.Infof("边缘节点服务启动中... 配置: %s", localCfg.Summary())

	// 3. 启动 Prometheus metrics
	metricCollector := metrics.New(log)
	metricsServer, metricsErrCh := metricCollector.StartServer(localCfg.MetricsPort)
	go func() {
		if err := <-metricsErrCh; err != nil {
			log.Errorf("Prometheus metrics server 错误: %v", err)
		}
	}()

	// 4. 创建探针管理器
	manager := probemgr.NewManager(log)
	manager.StartCleanup(30*time.Second, 60*time.Second)

	// 初始设置探针数量指标
	metricCollector.UpdateProbeCount(manager.GetProbeCount())

	// 5. 初始化服务发现
	var discovery servicediscovery.Discovery
	centerAddr := localCfg.CenterAddr

	// 创建中心服务客户端的函数
	// 注意：更新引用和关闭旧连接的顺序很重要：
	// 先更新 client 引用（加锁保护），再关闭旧连接。
	// 这样可以避免在关闭旧连接期间，其他 goroutine 使用已关闭的连接。
	// 闭包内从 atomic.Value 读取最新配置，确保热加载后 TLS/CenterAPIKey 能更新
	createCenterClient := func(addr string) error {
		currentCfg := loadCfg()
		newClient, err := grpcclient.NewClient(addr, currentCfg.TLS, currentCfg.CenterAPIKey, log)
		if err != nil {
			return err
		}
		clientMu.Lock()
		oldClient := client
		client = newClient
		if fwd != nil {
			fwd.SetClient(client)
		}
		clientMu.Unlock()
		// 关闭旧连接（如果有）
		if oldClient != nil {
			oldClient.Close()
		}
		return nil
	}

	if localCfg.ServiceDiscovery.Enabled {
		discovery, err = servicediscovery.NewDiscovery(localCfg.ServiceDiscovery, log)
		if err != nil {
			log.Warnf("初始化服务发现失败: %v，将使用配置的中心服务地址", err)
		} else {
			discovery.Start()
			defer discovery.Stop()
			// 设置地址更新回调
			discovery.SetUpdateCallback(func(newAddr string) {
				log.Infof("中心服务地址已更新: %s，正在重新连接...", newAddr)
				if err := createCenterClient(newAddr); err != nil {
					log.Errorf("重新连接中心服务失败: %v", err)
				} else {
					log.Info("已成功重新连接到中心服务")
				}
			})
			// 获取服务地址
			if addr, err := discovery.GetServiceAddress(localCfg.ServiceDiscovery.ServiceName); err == nil {
				centerAddr = addr
				log.Infof("通过服务发现获取中心服务地址: %s", centerAddr)
			} else {
				log.Warnf("获取服务地址失败: %v，将使用配置的中心服务地址", err)
			}
		}
	}

	// 6. 连接中心服务（支持 TLS + API Key）
	if err := createCenterClient(centerAddr); err != nil {
		log.Errorf("连接中心服务失败: %v", err)
		os.Exit(1)
	}

	// P0: Flow Ingest Pipeline - 根据配置选择 Kafka 或 gRPC 转发
	if localCfg.Kafka.Enabled && len(localCfg.Kafka.Brokers) > 0 {
		// 使用 Kafka 转发器（异步，零阻塞）
		log.Infof("P0: 启用 Kafka 转发器 (brokers=%v)", localCfg.Kafka.Brokers)
		kafkaFwd, err = kafkafwd.New(localCfg.Kafka.Brokers, &kafkafwd.ProtoSerializer{}, log)
		if err != nil {
			log.Errorf("创建 Kafka 转发器失败: %v", err)
			os.Exit(1)
		}
		// 创建适配器，让 gRPC server 可以调用 Kafka 转发器
		fwd = forwarder.NewKafkaAdapter(kafkaFwd, log)
		fwd.SetMetrics(metricCollector)
		fwd.Start()
	} else {
		// 使用传统 gRPC 转发器（同步）
		log.Info("使用传统 gRPC 转发器（Kafka 未启用）")
		fwd = forwarder.NewForwarder(client, localCfg.BatchSize, localCfg.FlushInterval, localCfg.MaxBufferLimit, log)
		fwd.SetMetrics(metricCollector)
		fwd.Start()
	}

	// 7. 创建 gRPC 服务端并注册探针服务
	srv := grpcserver.NewServer(manager, fwd, log, metricCollector, localCfg.APIKey)
	// 分布式边缘自治: 设置 Edge 节点元数据
	srv.SetEdgeConfig(localCfg.EdgeNodeID, localCfg.CloudPlatform, localCfg.Region, localCfg.Cluster.NodeID)
	log.Infof("Edge 探针认证 API Key 已启用 (key: %s...)", utils.MaskSecret(localCfg.APIKey))
	serverOpts, connPool, ipLimiter, goPool, breakerMgr, err := grpcserver.BuildServerOpts(
		localCfg.TLS, localCfg.RateLimit, localCfg.APIKey,
		localCfg.ConnectionPool, localCfg.IPLimit, localCfg.GoPool, localCfg.CircuitBreaker,
		log,
		srv.GetAuthInterceptor,
	)
	if err != nil {
		log.Errorf("构建 gRPC 服务端选项失败: %v", err)
		os.Exit(1)
	}
	// 将组件注入到Server中（用于监控统计）
	srv.SetConnPool(connPool)
	srv.SetIPLimiter(ipLimiter)
	srv.SetGoPool(goPool)
	srv.SetBreaker(breakerMgr)
	// P1: 注入 Center 客户端，用于 DiscoverEdges 查询 Edge 注册表
	srv.SetCenterClient(client)
	grpcServer := grpc.NewServer(serverOpts...)
	edge.RegisterProbeServiceServer(grpcServer, srv)

	// 8. 监听端口
	listenAddr := fmt.Sprintf(":%d", localCfg.GRPCListenPort)
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Errorf("监听端口失败: %v", err)
		os.Exit(1)
	}

	// 用于通知所有后台协程退出
	stopCh := make(chan struct{})

	// 9. 启动心跳上报协程（H3 修复: 使用 Center 返回的动态间隔）
	// L1 修复: 初始值使用配置，后续根据 Center 响应动态调整
	go func() {
		heartbeatInterval := localCfg.HeartbeatInterval
		if heartbeatInterval <= 0 {
			heartbeatInterval = 30 * time.Second // 默认值
		}
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				probes := manager.GetAllProbes()
				metricCollector.ProbesOnline.Set(float64(len(probes)))
				metricCollector.UpdateProbeCount(manager.GetProbeCount())
				metricCollector.HeartbeatsTotal.Inc()
			clientMu.RLock()
			currentClient := client
			clientMu.RUnlock()
			if currentClient == nil {
				metricCollector.HeartbeatErrorsTotal.Inc()
				log.Warn("gRPC 客户端未初始化，跳过心跳上报")
				continue
			}
			// H3 修复: 获取 Center 下发的心跳间隔
			newInterval, err := currentClient.SendHeartbeat(loadCfg().EdgeNodeID, loadCfg().CloudPlatform, loadCfg().Region, int32(len(probes)))
				if err != nil {
					metricCollector.HeartbeatErrorsTotal.Inc()
					log.Warnf("发送边缘心跳失败: %v", err)
				} else if newInterval > 0 {
					// 如果间隔变化，重置 ticker
					newDuration := time.Duration(newInterval) * time.Second
					if newDuration != heartbeatInterval {
						log.Infof("心跳间隔从 %v 调整为 %v", heartbeatInterval, newDuration)
						ticker.Reset(newDuration)
						heartbeatInterval = newDuration
					}
				}
			case <-stopCh:
				log.Info("心跳上报协程已停止")
				return
			}
		}
	}()

	// 10. 启动探针列表上报协程（L1 修复: 使用配置的间隔）
	go func() {
		probeReportInterval := localCfg.ProbeReportInterval
		if probeReportInterval <= 0 {
			probeReportInterval = 60 * time.Second // 默认值
		}
		ticker := time.NewTicker(probeReportInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				allProbes := manager.GetAllProbes()
				probeInfos := make([]*edge.ProbeInfo, 0, len(allProbes))
				for _, p := range allProbes {
					probeInfos = append(probeInfos, &edge.ProbeInfo{
						ProbeId:       p.ID,
						HostIp:        p.HostIP,
						Hostname:      p.Hostname,
						Status:        p.Status,
						Version:       p.Version,
						LastHeartbeat: p.LastHeartbeat.Unix(),
					})
				}
			clientMu.RLock()
			currentClient := client
			clientMu.RUnlock()
			if currentClient == nil {
				log.Warn("gRPC 客户端未初始化，跳过探针列表上报")
				continue
			}
			if err := currentClient.ReportProbes(loadCfg().EdgeNodeID, loadCfg().CloudPlatform, loadCfg().Region, probeInfos); err != nil {
					log.Warnf("上报探针列表失败: %v", err)
				}
			case <-stopCh:
				log.Info("探针上报协程已停止")
				return
			}
		}
	}()

	// 注册健康检查服务
	healthChecker := grpcserver.NewHealthChecker(srv)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthChecker)

	// 启动 HTTP 健康检查服务器
	healthHandler := http.NewHealthHandler(manager, log)
	healthAddr := fmt.Sprintf(":%d", localCfg.HealthPort)
	healthServer = http.StartHealthServer(healthAddr, healthHandler)
	log.Infof("健康检查 HTTP 服务监听: %s/health", healthAddr)

	// 启动 gRPC 服务
	go func() {
		log.Infof("gRPC 服务监听: %s", listenAddr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Errorf("gRPC 服务异常: %v", err)
		}
	}()

	log.Info("边缘节点服务已启动")

	// 11. 优雅关闭和配置热加载（带超时保护）
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	
	for {
		sig := <-sigCh
		
		// 处理 SIGHUP 信号（配置热加载）
		if sig == syscall.SIGHUP {
			log.Info("收到 SIGHUP 信号，重新加载配置...")
			newCfg, err := config.Load()
			if err != nil {
				log.Errorf("重新加载配置失败: %v", err)
			} else {
				log.Infof("配置已重新加载: %s", newCfg.Summary())
				// 原子更新全局配置
				cfg.Store(newCfg)
				// 更新转发器配置（支持热加载）
				if fwd != nil {
					fwd.UpdateConfig(newCfg.BatchSize, newCfg.FlushInterval)
				}
				// API Key 变更时重新创建 Center gRPC 客户端
				clientMu.RLock()
				currentClient := client
				clientMu.RUnlock()
				if currentClient != nil {
					centerAddr := newCfg.CenterAddr
					if err := createCenterClient(centerAddr); err != nil {
						log.Errorf("SIGHUP 热加载：API Key/TLS 变更后重新创建 Center gRPC 客户端失败: %v", err)
					} else {
						log.Info("SIGHUP 热加载：Center gRPC 客户端已重新创建（API Key/TLS 配置已更新）")
					}
				}
				// TLS 证书变更时重新加载 TLS 配置
				// 注意：gRPC Server 的 TLS 证书在创建时已固定到 Server 实例，
				// 当前 gRPC Server 不支持动态更换 TLS 证书，需要重启服务才能生效。
				// 此处仅记录日志提示用户。
				if newCfg.TLS.Enabled {
					log.Infof("SIGHUP 热加载：TLS 配置已更新（新证书将在下次重启后生效）")
				}
				// 注意：以下配置变更需要重启服务才能生效：
				// - gRPC 监听端口（需要重新绑定端口监听）
				// - 服务发现配置（Discovery 实例在启动时创建，不支持动态重建）
				log.Warnf("SIGHUP 热加载提示：gRPC 监听端口和服务发现配置变更需要重启服务才能生效")
			}
			// 继续运行，不需要重新注册信号，因为 signal.Notify 是持续有效的
			continue
		}
		
		// 处理 SIGINT 和 SIGTERM 信号（优雅关闭）
		log.Infof("收到信号 %v，开始优雅关闭（超时 %s）...", sig, gracefulShutdownTimeout)
		break
	}

	// 注意：shutdownCtx 的超时时间被所有关闭操作共享（停止心跳、转发器、探针管理器等）。
	// 如果某个组件关闭耗时过长，可能导致后续组件来不及优雅关闭。
	// 当前设计中各组件关闭较快（通常 < 1s），共享超时是可接受的简化方案。
	// TODO(AE-L06): 如未来组件关闭耗时增加，考虑为每个组件分配独立的超时预算。
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer shutdownCancel()

	// 停止心跳和上报协程
	close(stopCh)

	// 停止转发器（刷新剩余数据）
	fwd.Stop()

	// 停止探针管理器
	manager.Stop()

	// 停止 gRPC 服务（等待现有请求完成，在独立协程中执行）
	done := make(chan struct{})
	go func() {
		// 先停止子组件（连接池、IP限流器、goroutine池）
		srv.ShutdownComponents()
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Info("gRPC 服务已优雅停止")
	case <-shutdownCtx.Done():
		log.Warnf("优雅关闭超时，强制停止 gRPC 服务")
		grpcServer.Stop()
	}

	// 关闭中心服务连接
	clientMu.RLock()
	currentClient := client
	clientMu.RUnlock()
	if currentClient != nil {
		if err := currentClient.Close(); err != nil {
			log.Warnf("关闭中心服务连接失败: %v", err)
		}
	}

	// 优雅关闭 Prometheus metrics 服务器
	if metricsServer != nil {
		metricsDone := make(chan struct{})
		go func() {
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				log.Warnf("关闭 Prometheus metrics 服务器失败: %v", err)
			} else {
				log.Info("Prometheus metrics 服务器已优雅关闭")
			}
			close(metricsDone)
		}()
		select {
		case <-metricsDone:
		case <-shutdownCtx.Done():
			log.Warnf("Prometheus metrics 服务器关闭超时")
		}
	}

	// 优雅关闭 HTTP 健康检查服务器
	if healthServer != nil {
		healthDone := make(chan struct{})
		go func() {
			if err := healthServer.Shutdown(shutdownCtx); err != nil {
				log.Warnf("关闭 HTTP 健康检查服务器失败: %v", err)
			} else {
				log.Info("HTTP 健康检查服务器已优雅关闭")
			}
			close(healthDone)
		}()
		select {
		case <-healthDone:
		case <-shutdownCtx.Done():
			log.Warnf("HTTP 健康检查服务器关闭超时")
		}
	}

	log.Info("边缘节点服务已安全退出")
}
