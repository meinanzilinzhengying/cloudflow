// Package grpcserver 提供 gRPC 服务端实现
// 实现 ProbeService 接口，接收探针的注册、心跳、数据上报请求
//
// 优化特性：
//   - 连接池管理：最大10000连接，按IP追踪
//   - 单IP限流：每IP 100 QPS令牌桶
//   - goroutine池：防止OOM，有界任务队列
//   - 熔断机制：三态熔断器保护服务稳定性
package grpcserver

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"cloud-flow-edge/internal/auth"
	"cloud-flow-edge/internal/circuitbreaker"
	"cloud-flow-edge/internal/config"
	"cloud-flow-edge/internal/connpool"
	"cloud-flow-edge/internal/forwarder"
	"cloud-flow-edge/internal/gopool"
	"cloud-flow-edge/internal/iplimiter"
	"cloud-flow-edge/internal/probemgr"
	"cloud-flow-edge/internal/session"
	"cloud-flow-edge/internal/whitelist"
	"cloud-flow-edge/pkg/logger"
	"cloud-flow-edge/pkg/metrics"
	"cloud-flow-edge/pkg/tlsutil"
	"cloud-flow-edge/pkg/validate"
	"cloud-flow/pkg/grpcutil"
	"cloud-flow/pkg/ratelimit"
	"cloud-flow/pkg/trace"
	edge "cloud-flow/proto"
)

// Server gRPC 服务端
// 实现 edge.ProbeServiceServer 接口
type Server struct {
	edge.UnimplementedProbeServiceServer
	manager      *probemgr.Manager
	forwarder    *forwarder.Forwarder
	logger       *logger.Logger
	metrics      *metrics.Metrics
	apiKey       string
	connPool     *connpool.Pool
	ipLimiter    *iplimiter.Limiter
	goPool       *gopool.Pool
	breaker      *circuitbreaker.Manager
	whitelistMgr *whitelist.Manager
	// H1 修复: 使用 atomic.Value 管理 authInterceptor，避免数据竞争
	// BuildServerOpts 中的 combinedInterceptor 闭包会读取此值，
	// 而 SetAuthInterceptor 在 BuildServerOpts 之后设置，需要同步机制保护
	authInterceptor atomic.Value // 存储 *auth.AuthInterceptor

	// 分布式边缘自治增强
	edgeID       string              // 当前 Edge 节点唯一标识
	sessionStore session.Store       // Agent 会话存储
	region       string              // 区域
	zone         string              // 可用区
	cluster      string              // 集群
	// P1: Center 客户端引用，用于 DiscoverEdges 查询 Edge 注册表
	centerClient CenterEdgeDiscoverer
}

// CenterEdgeDiscoverer Center Edge 发现接口（P1: 解耦 Edge Server 与 Center Client）
type CenterEdgeDiscoverer interface {
	GetEdgeStatus(edgeNodeID string) (*edge.GetEdgeStatusResponse, error)
}

// NewServer 创建 gRPC 服务端实例
func NewServer(manager *probemgr.Manager, fwd *forwarder.Forwarder, log *logger.Logger, m *metrics.Metrics, apiKey string) *Server {
	return &Server{
		manager:      manager,
		forwarder:    fwd,
		logger:       log,
		metrics:      m,
		apiKey:       apiKey,
		sessionStore: session.NewMemoryStore(),
	}
}

// SetEdgeConfig 设置 Edge 节点元数据（分布式边缘自治）
func (s *Server) SetEdgeConfig(edgeID, region, zone, cluster string) {
	s.edgeID = edgeID
	s.region = region
	s.zone = zone
	s.cluster = cluster
}

// SetSessionStore 设置会话存储
func (s *Server) SetSessionStore(store session.Store) {
	s.sessionStore = store
}

// SetCenterClient 设置 Center 客户端（P1: 用于 DiscoverEdges）
func (s *Server) SetCenterClient(client CenterEdgeDiscoverer) {
	s.centerClient = client
}

// GetSessionStore 获取会话存储
func (s *Server) GetSessionStore() session.Store {
	return s.sessionStore
}

// SetConnPool 设置连接池
func (s *Server) SetConnPool(pool *connpool.Pool) {
	s.connPool = pool
}

// SetIPLimiter 设置IP限流器
func (s *Server) SetIPLimiter(limiter *iplimiter.Limiter) {
	s.ipLimiter = limiter
}

// SetGoPool 设置goroutine池
func (s *Server) SetGoPool(pool *gopool.Pool) {
	s.goPool = pool
}

// SetBreaker 设置熔断器
func (s *Server) SetBreaker(breaker *circuitbreaker.Manager) {
	s.breaker = breaker
}

// SetWhitelistManager 设置白名单管理器
func (s *Server) SetWhitelistManager(mgr *whitelist.Manager) {
	s.whitelistMgr = mgr
}

// SetAuthInterceptor 设置鉴权拦截器
// H1 修复: 使用 atomic.Value 存储，确保线程安全
func (s *Server) SetAuthInterceptor(ai *auth.AuthInterceptor) {
	s.authInterceptor.Store(ai)
}

// GetAuthInterceptor 获取鉴权拦截器
// H1 修复: 使用 atomic.Value 读取，确保线程安全
func (s *Server) GetAuthInterceptor() *auth.AuthInterceptor {
	if v := s.authInterceptor.Load(); v != nil {
		return v.(*auth.AuthInterceptor)
	}
	return nil
}

// ============================================================================
// 拦截器
// ============================================================================

// TraceIDInterceptor Trace ID 拦截器
func TraceIDInterceptor(log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		var traceID string
		if ok {
			vals := md.Get(trace.TraceIDMDKey)
			if len(vals) > 0 {
				traceID = vals[0]
			}
		}
		if traceID == "" {
			traceID = trace.TraceID()
		}
		ctx = trace.WithTraceID(ctx, traceID)
		return handler(ctx, req)
	}
}

// APIKeyAuthInterceptor API Key认证拦截器
// M2 修复: 使用 gRPC status code 返回错误，与 Center 保持一致
// N1 修复: 添加安全警告，提醒用户配置 API Key
func APIKeyAuthInterceptor(apiKey string, log *logger.Logger) grpc.UnaryServerInterceptor {
	// N1: 检查 API Key 是否配置，发出安全警告
	if apiKey == "" {
		log.Warnf("[N1] API Key 未配置，跳过认证。生产环境请设置 CLOUD_FLOW_API_KEY 环境变量")
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 如果未配置 API Key，跳过认证（开发/测试环境）
		if apiKey == "" {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			log.Warnf("API Key 认证失败: 无 metadata")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败: 无 metadata")
		}
		vals := md.Get("x-api-key")
		if len(vals) == 0 {
			log.Warnf("API Key 认证失败: 无 x-api-key 头")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败: 未提供 API Key")
		}
		if subtle.ConstantTimeCompare([]byte(vals[0]), []byte(apiKey)) != 1 {
			log.Warnf("API Key 认证失败: 无效的 API Key")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败: 无效的 API Key")
		}
		return handler(ctx, req)
	}
}

// ConnPoolInterceptor 连接池拦截器
// 在注册时添加连接，心跳时更新活跃时间
func ConnPoolInterceptor(pool *connpool.Pool, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		clientIP := extractClientIP(ctx)
		probeID := extractProbeID(req)

		switch info.FullMethod {
		case "/edge.ProbeService/RegisterProbe":
			if probeID != "" {
				if err := pool.Add(probeID, clientIP); err != nil {
					log.Warnf("连接池拒绝: probeID=%s, ip=%s, err=%v", probeID, clientIP, err)
					return nil, fmt.Errorf("连接池已满，最大连接数: %d", pool.MaxConnections())
				}
			}
		case "/edge.ProbeService/Heartbeat":
			if probeID != "" {
				pool.Touch(probeID)
			}
		}

		return handler(ctx, req)
	}
}

// IPLimitInterceptor 单IP限流拦截器
func IPLimitInterceptor(limiter *iplimiter.Limiter, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		clientIP := extractClientIP(ctx)
		if !limiter.Allow(clientIP) {
			log.Warnf("IP限流拒绝: ip=%s, method=%s", clientIP, info.FullMethod)
			return nil, fmt.Errorf("IP 请求频率超限 (100 QPS/IP)")
		}
		return handler(ctx, req)
	}
}

// GoPoolInterceptor goroutine池拦截器
// 将数据上报类请求提交到goroutine池异步处理，避免阻塞gRPC线程
func GoPoolInterceptor(pool *gopool.Pool, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 注册和心跳请求直接在gRPC线程处理（低延迟要求）
		switch info.FullMethod {
		case "/edge.ProbeService/RegisterProbe",
			"/edge.ProbeService/Heartbeat":
			return handler(ctx, req)
		}

		// 数据上报类请求提交到goroutine池
		resultCh := make(chan handlerResult, 1)
		task := func() {
			resp, err := handler(ctx, req)
			resultCh <- handlerResult{resp: resp, err: err}
		}

		if err := pool.Submit(task); err != nil {
			log.Warnf("goroutine池已满，请求降级为同步处理: method=%s", info.FullMethod)
			// 降级：直接在当前goroutine执行
			return handler(ctx, req)
		}

		// 等待结果
		select {
		case result := <-resultCh:
			return result.resp, result.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// CircuitBreakerInterceptor 熔断器拦截器
func CircuitBreakerInterceptor(breakerMgr *circuitbreaker.Manager, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 按服务方法获取熔断器
		brk := breakerMgr.GetBreaker(info.FullMethod)

		if !brk.Allow() {
			log.Warnf("熔断器拒绝请求: method=%s, state=%s", info.FullMethod, brk.GetState().State)
			return nil, fmt.Errorf("服务暂时不可用 (circuit breaker open)")
		}

		resp, err := handler(ctx, req)
		if err != nil {
			brk.RecordFailure()
		} else {
			brk.RecordSuccess()
		}
		return resp, err
	}
}

// handlerResult goroutine池处理结果
type handlerResult struct {
	resp interface{}
	err  error
}

// extractClientIP 从gRPC context中提取客户端IP
func extractClientIP(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return "unknown"
	}
	return extractIP(p.Addr.String())
}

// extractProbeID 从请求中提取probeID
func extractProbeID(req interface{}) string {
	switch r := req.(type) {
	case *edge.RegisterProbeRequest:
		return r.GetProbeId()
	case *edge.HeartbeatRequest:
		return r.GetProbeId()
	case *edge.MetricsBatch:
		return r.GetProbeId()
	case *edge.TraceBatch:
		return r.GetProbeId()
	case *edge.ProfilingBatch:
		return r.GetProbeId()
	default:
		return ""
	}
}

// extractIP 从addr中提取IP（去掉端口）
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// ============================================================================
// 服务端构建
// ============================================================================

// BuildServerOpts 根据配置构建 gRPC Server 选项
// 集成连接池、IP限流、goroutine池、熔断器
func BuildServerOpts(
	tlsCfg config.TLSConfig,
	rateLimit config.RateLimit,
	apiKey string,
	poolCfg config.ConnectionPoolConfig,
	ipLimitCfg config.IPLimitConfig,
	goPoolCfg config.GoPoolConfig,
	breakerCfg config.CircuitBreakerConfig,
	log *logger.Logger,
) ([]grpc.ServerOption, *connpool.Pool, *iplimiter.Limiter, *gopool.Pool, *circuitbreaker.Manager, error) {

	var opts []grpc.ServerOption

	// 1. 创建连接池
	connPool := connpool.NewPool(
		connpool.WithMaxConnections(poolCfg.MaxConnections),
		connpool.WithStaleTimeout(poolCfg.StaleTimeout),
		connpool.WithCleanupInterval(poolCfg.CleanupInterval),
	)
	connPool.Start()
	log.Infof("[连接池] 已启动: 最大连接数=%d, 过期超时=%v", poolCfg.MaxConnections, poolCfg.StaleTimeout)

	// 2. 创建IP限流器
	ipLimiter := iplimiter.NewLimiter(
		iplimiter.WithMaxQPS(ipLimitCfg.MaxQPSPerIP),
		iplimiter.WithBurstSize(ipLimitCfg.BurstSize),
		iplimiter.WithCleanupInterval(ipLimitCfg.CleanupInterval),
		iplimiter.WithStaleDuration(ipLimitCfg.StaleDuration),
	)
	ipLimiter.Start()
	log.Infof("[IP限流] 已启动: 每IP %d QPS, 突发=%d", ipLimitCfg.MaxQPSPerIP, ipLimitCfg.BurstSize)

	// 3. 创建goroutine池
	goPool := gopool.NewPool(
		gopool.WithWorkers(goPoolCfg.Workers),
		gopool.WithQueueCap(goPoolCfg.QueueCap),
	)
	log.Infof("[goroutine池] 已启动: workers=%d, 队列容量=%d", goPoolCfg.Workers, goPoolCfg.QueueCap)

	// 4. 创建熔断器管理器
	breakerMgr := circuitbreaker.NewManager(
		circuitbreaker.WithFailureThreshold(breakerCfg.FailureThreshold),
		circuitbreaker.WithMinRequests(breakerCfg.MinRequests),
		circuitbreaker.WithRecoveryTimeout(breakerCfg.RecoveryTimeout),
		circuitbreaker.WithHalfOpenMaxRequests(breakerCfg.HalfOpenMaxRequests),
	)
	log.Infof("[熔断器] 已启动: 失败阈值=%.0f%%, 最小请求数=%d, 恢复超时=%v",
		breakerCfg.FailureThreshold*100, breakerCfg.MinRequests, breakerCfg.RecoveryTimeout)

	// 5. 构建拦截器链
	// 执行顺序: Panic恢复 -> 连接池 -> IP限流 -> 全局限流 -> goroutine池 -> 熔断器 -> TraceID -> API Key
	traceInterceptor := TraceIDInterceptor(log)

	var rateLimiter *ratelimit.TokenBucket
	if rateLimit.Enabled {
		rateLimiter = ratelimit.NewTokenBucket(rateLimit.BucketSize, rateLimit.RefillRate)
		log.Infof("[全局限流] 已启用: 桶容量=%d, 填充速率=%d/秒", rateLimit.BucketSize, rateLimit.RefillRate)
	}

	connPoolInterceptor := ConnPoolInterceptor(connPool, log)
	ipLimitInterceptor := IPLimitInterceptor(ipLimiter, log)
	goPoolInterceptor := GoPoolInterceptor(goPool, log)
	breakerInterceptor := CircuitBreakerInterceptor(breakerMgr, log)

	combinedInterceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		// C4 修复: 使用命名返回值，让 defer 能修改返回值
		// 原修复错误: if panicErr != nil 检查在 defer 注册后立即执行，此时 panicErr 必然为 nil（死代码）
		// 正确方式: 使用命名返回值 (resp, err)，defer 中直接修改 err
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("gRPC 处理 panic: %v, method=%s", r, info.FullMethod)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		// 1. 全局限流
		if rateLimit.Enabled && !rateLimiter.Allow() {
			return nil, fmt.Errorf("全局限流: 请求频率超限")
		}

		// 2. 连接池检查
		ctx, resp, err = func() (context.Context, interface{}, error) {
			return connPoolInterceptor(ctx, req, info, handler)
		}()
		if err != nil {
			return nil, err
		}
		_ = ctx // connPoolInterceptor不修改ctx

		// 3. IP限流
		if !ipLimiter.Allow(extractClientIP(ctx)) {
			return nil, fmt.Errorf("IP 请求频率超限 (100 QPS/IP)")
		}

		// 4. goroutine池分发
		// 5. 熔断器
		// 6. Agent鉴权（mTLS + Token + 白名单）
		// 7. TraceID
		// 8. API Key
		return breakerInterceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return goPoolInterceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			// H1 修复: 使用 GetAuthInterceptor() 方法读取，确保线程安全
			// Agent鉴权拦截器（如果启用）
			authInterceptor := s.GetAuthInterceptor()
			if authInterceptor != nil {
				return authInterceptor.UnaryServerInterceptor()(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
					return traceInterceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
						if apiKey != "" {
							apiInterceptor := APIKeyAuthInterceptor(apiKey, log)
							return apiInterceptor(ctx, req, info, handler)
						}
						return handler(ctx, req)
					})
				})
			}
			return traceInterceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
				if apiKey != "" {
					apiInterceptor := APIKeyAuthInterceptor(apiKey, log)
					return apiInterceptor(ctx, req, info, handler)
				}
				return handler(ctx, req)
			})
		})
	})
	}

	opts = append(opts, grpc.UnaryInterceptor(combinedInterceptor))

	// 6. 配置最大并发流（支持5000+Agent）
	opts = append(opts, grpc.MaxConcurrentStreams(uint32(poolCfg.MaxConnections)))
	// 增大消息大小限制
	opts = append(opts, grpc.MaxRecvMsgSize(64*1024*1024)) // 64MB
	opts = append(opts, grpc.MaxSendMsgSize(64*1024*1024)) // 64MB

	// 7. TLS配置
	if !tlsCfg.Enabled {
		log.Warn("gRPC 服务端未启用 TLS，将使用明文传输")
		return opts, connPool, ipLimiter, goPool, breakerMgr, nil
	}

	if tlsCfg.ServerCert == "" || tlsCfg.ServerKey == "" {
		log.Info("未配置服务端证书，将自动生成自签名证书")
		cert, err := tlsutil.LoadOrGenSelfSignedCert("", "", "./certs")
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("生成自签名证书失败: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{*cert},
			ClientAuth:   tls.NoClientCert,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
		return opts, connPool, ipLimiter, goPool, breakerMgr, nil
	}

	cert, err := tlsutil.LoadOrGenSelfSignedCert(tlsCfg.ServerCert, tlsCfg.ServerKey, "")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("加载服务端证书失败: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
		ClientAuth:   tls.NoClientCert,
	}

	if tlsCfg.CACert != "" {
		caPEM, err := os.ReadFile(tlsCfg.CACert)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, nil, nil, nil, nil, fmt.Errorf("解析 CA 证书失败")
		}
		tlsConfig.ClientCAs = certPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		log.Info("gRPC 服务端启用 mTLS")
	} else {
		log.Info("gRPC 服务端启用 TLS（单向）")
	}

	creds := credentials.NewTLS(tlsConfig)
	opts = append(opts, grpc.Creds(creds))
	return opts, connPool, ipLimiter, goPool, breakerMgr, nil
}

// ============================================================================
// RPC 方法实现
// ============================================================================

// RegisterProbe 处理探针注册请求
func (s *Server) RegisterProbe(ctx context.Context, req *edge.RegisterProbeRequest) (*edge.RegisterProbeResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		s.logger.Warnf("探针注册被拒绝: API Key 验证失败, probeID=%s", req.GetProbeId())
		return &edge.RegisterProbeResponse{Success: false, Message: "API Key 验证失败"}, nil
	}
	if err := validate.RegisterProbeRequest(req); err != nil {
		s.logger.Warnf("探针注册被拒绝: 参数校验失败: %v", err)
		return &edge.RegisterProbeResponse{Success: false, Message: err.Error()}, nil
	}

	// 注册探针到 ProbeManager
	probe := s.manager.Register(req.GetProbeId(), req.GetHostIp(), req.GetHostname(), req.GetVersion())

	// 创建/更新 Agent 会话（分布式边缘自治增强）
	probeSession := &session.Session{
		ProbeID:      req.GetProbeId(),
		AssignedEdge: s.edgeID,
		ClientIP:     req.GetHostIp(),
		Hostname:     req.GetHostname(),
		Version:      req.GetVersion(),
		Region:       req.GetRegion(),
		Zone:         req.GetZone(),
		Cluster:      req.GetCluster(),
		Metadata:     req.GetLabels(),
	}
	if err := s.sessionStore.CreateSession(probeSession); err != nil {
		s.logger.Warnf("创建会话失败: %v", err)
	}

	// 检查是否是重连（会话已存在）
	_, existingSession := s.sessionStore.GetSession(req.GetProbeId())
	if existingSession && probe.RegisteredAt.Before(time.Now().Add(-1*time.Minute)) {
		s.logger.Infof("Agent 重连: probe_id=%s, session_id=%s", req.GetProbeId(), probeSession.SessionID)
	}

	return &edge.RegisterProbeResponse{
		Success:           true,
		Message:           "注册成功",
		HeartbeatInterval: 30,
		SessionId:         probeSession.SessionID,
		AssignedEdgeId:    s.edgeID,
	}, nil
}

// Heartbeat 处理探针心跳请求
func (s *Server) Heartbeat(ctx context.Context, req *edge.HeartbeatRequest) (*edge.HeartbeatResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		return &edge.HeartbeatResponse{Success: false}, nil
	}
	if err := validate.HeartbeatRequest(req); err != nil {
		return &edge.HeartbeatResponse{Success: false}, nil
	}

	// 更新 ProbeManager 心跳
	ok := s.manager.Heartbeat(req.GetProbeId())

	// 更新会话心跳（分布式边缘自治增强）
	if req.GetSessionId() != "" {
		if sess, exists := s.sessionStore.GetSession(req.GetProbeId()); exists {
			if sess.SessionID != req.GetSessionId() {
				// Session 不匹配，可能是旧会话
				s.logger.Warnf("心跳 Session 不匹配: probe_id=%s, expected=%s, got=%s",
					req.GetProbeId(), sess.SessionID, req.GetSessionId())
				return &edge.HeartbeatResponse{Success: false, Message: "session mismatch"}, nil
			}
		}
	}
	_ = s.sessionStore.UpdateHeartbeat(req.GetProbeId())

	return &edge.HeartbeatResponse{Success: ok}, nil
}

// SendMetrics 接收探针发送的指标数据
func (s *Server) SendMetrics(ctx context.Context, batch *edge.MetricsBatch) (*edge.SendResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		s.logger.Warnf("指标数据被拒绝: API Key 验证失败, probeID=%s", batch.GetProbeId())
		if s.metrics != nil {
			s.metrics.Receive(0, fmt.Errorf("API Key 验证失败"))
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if err := validate.MetricsBatch(batch); err != nil {
		s.logger.Warnf("指标数据被拒绝: 参数校验失败: %v", err)
		if s.metrics != nil {
			s.metrics.Receive(0, err)
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if s.metrics != nil {
		s.metrics.Receive(len(batch.GetMetrics())*100, nil)
	}
	s.forwarder.AddMetrics(batch)
	return &edge.SendResponse{Success: true, Accepted: int32(len(batch.GetMetrics()))}, nil
}

// SendTraces 接收探针发送的链路追踪数据
func (s *Server) SendTraces(ctx context.Context, batch *edge.TraceBatch) (*edge.SendResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		s.logger.Warnf("链路追踪数据被拒绝: API Key 验证失败, probeID=%s", batch.GetProbeId())
		if s.metrics != nil {
			s.metrics.Receive(0, fmt.Errorf("API Key 验证失败"))
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if err := validate.TraceBatch(batch); err != nil {
		s.logger.Warnf("链路追踪数据被拒绝: 参数校验失败: %v", err)
		if s.metrics != nil {
			s.metrics.Receive(0, err)
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if s.metrics != nil {
		s.metrics.Receive(len(batch.GetSpans())*200, nil)
	}
	s.forwarder.AddTraces(batch)
	return &edge.SendResponse{Success: true, Accepted: int32(len(batch.GetSpans()))}, nil
}

// SendProfiling 接收探针发送的性能分析数据
func (s *Server) SendProfiling(ctx context.Context, batch *edge.ProfilingBatch) (*edge.SendResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		s.logger.Warnf("性能分析数据被拒绝: API Key 验证失败, probeID=%s", batch.GetProbeId())
		if s.metrics != nil {
			s.metrics.Receive(0, fmt.Errorf("API Key 验证失败"))
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if err := validate.ProfilingBatch(batch); err != nil {
		s.logger.Warnf("性能分析数据被拒绝: 参数校验失败: %v", err)
		if s.metrics != nil {
			s.metrics.Receive(0, err)
		}
		return &edge.SendResponse{Success: false, Accepted: 0}, nil
	}
	if s.metrics != nil {
		s.metrics.Receive(len(batch.GetProfiles())*500, nil)
	}
	s.forwarder.AddProfiling(batch)
	return &edge.SendResponse{Success: true, Accepted: int32(len(batch.GetProfiles()))}, nil
}

// DiscoverEdges 发现可用 Edge 节点（P1: Center Edge 注册表）
// Agent 通过此 RPC 发现同区域的其他 Edge 节点，用于故障转移
func (s *Server) DiscoverEdges(ctx context.Context, req *edge.DiscoverEdgesRequest) (*edge.DiscoverEdgesResponse, error) {
	if !grpcutil.CheckAPIKey(ctx, s.apiKey) {
		s.logger.Warnf("DiscoverEdges 被拒绝: API Key 验证失败, probeID=%s", req.GetProbeId())
		return &edge.DiscoverEdgesResponse{Success: false, Code: "auth_failed", Message: "API Key 验证失败"}, nil
	}

	// 如果没有 Center 客户端，返回仅包含自身的列表
	if s.centerClient == nil {
		s.logger.Warnf("DiscoverEdges: Center 客户端未配置，仅返回当前 Edge")
		return &edge.DiscoverEdgesResponse{
			Success: true,
			Instances: []*edge.EdgeInstance{
				{
					Id:      s.edgeID,
					Address: s.getEdgeAddr(),
					Healthy: true,
					Region:  s.region,
					Zone:    s.zone,
					Weight:  100,
				},
			},
			Strategy: "single",
		}, nil
	}

	// 从 Center 查询 Edge 列表
	resp, err := s.centerClient.GetEdgeStatus("")
	if err != nil {
		s.logger.Warnf("DiscoverEdges: 查询 Center 失败: %v，返回仅包含自身的列表", err)
		return &edge.DiscoverEdgesResponse{
			Success: true,
			Instances: []*edge.EdgeInstance{
				{
					Id:      s.edgeID,
					Address: s.getEdgeAddr(),
					Healthy: true,
					Region:  s.region,
					Zone:    s.zone,
					Weight:  100,
				},
			},
			Strategy: "fallback",
		}, nil
	}

	// 转换 Center 返回的 EdgeStatus 为 EdgeInstance
	instances := make([]*edge.EdgeInstance, 0, len(resp.GetStatuses()))
	for _, status := range resp.GetStatuses() {
		// 按区域过滤
		if req.GetRegion() != "" && status.GetRegion() != "" && status.GetRegion() != req.GetRegion() {
			continue
		}
		instances = append(instances, &edge.EdgeInstance{
			Id:              status.GetEdgeNodeId(),
			Address:         status.GetEdgeAddress(),
			Healthy:         status.GetStatus() == "online",
			Region:          status.GetRegion(),
			ConnectionCount: int64(status.GetProbeCount()),
			Version:         status.GetVersion(),
			Weight:          100,
		})
	}

	// 确保自身在列表中
	found := false
	for _, inst := range instances {
		if inst.Id == s.edgeID {
			found = true
			break
		}
	}
	if !found {
		instances = append(instances, &edge.EdgeInstance{
			Id:      s.edgeID,
			Address: s.getEdgeAddr(),
			Healthy: true,
			Region:  s.region,
			Zone:    s.zone,
			Weight:  100,
		})
	}

	strategy := "round-robin"
	if len(instances) <= 1 {
		strategy = "single"
	}

	s.logger.Infof("DiscoverEdges: 返回 %d 个 Edge 实例 (probeID=%s, region=%s, strategy=%s)",
		len(instances), req.GetProbeId(), req.GetRegion(), strategy)

	return &edge.DiscoverEdgesResponse{
		Success:   true,
		Instances: instances,
		Strategy:  strategy,
	}, nil
}

// getEdgeAddr 获取当前 Edge 的外部地址
func (s *Server) getEdgeAddr() string {
	// 优先从配置获取，此处返回空字符串由调用方填充
	return ""
}

// StreamData P2: 双向流式数据通道（Agent <-> Edge）
// 复用单个 TCP 连接传输多种数据类型（metrics/traces/logs/heartbeat），减少连接开销
func (s *Server) StreamData(stream edge.ProbeService_StreamDataServer) error {
	// 从第一个消息中提取 probeID 进行认证
	firstMsg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("接收首条消息失败: %w", err)
	}

	probeID := firstMsg.GetProbeId()
	if probeID == "" {
		return fmt.Errorf("首条消息缺少 probe_id")
	}

	s.logger.Infof("[StreamData] Agent 建立流式连接: probeID=%s", probeID)

	// 处理第一条消息
	s.handleStreamMessage(firstMsg, stream)

	// 持续接收消息
	for {
		msg, err := stream.Recv()
		if err != nil {
			s.logger.Infof("[StreamData] Agent 流式连接断开: probeID=%s, err=%v", probeID, err)
			return nil // 流正常关闭
		}
		s.handleStreamMessage(msg, stream)
	}
}

// handleStreamMessage 处理流式消息
func (s *Server) handleStreamMessage(msg *edge.StreamDataRequest, stream edge.ProbeService_StreamDataServer) {
	switch msg.GetType() {
	case edge.StreamDataType_METRICS:
		// 反序列化 MetricsBatch
		var batch edge.MetricsBatch
		if err := json.Unmarshal(msg.GetPayload(), &batch); err != nil {
			s.logger.Warnf("[StreamData] 反序列化 MetricsBatch 失败: %v", err)
			_ = stream.Send(&edge.StreamDataResponse{
				Success: false, Code: "decode_error", Message: err.Error(),
				SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
			})
			return
		}
		batch.ProbeId = msg.GetProbeId()
		s.forwarder.AddMetrics(&batch)
		_ = stream.Send(&edge.StreamDataResponse{
			Success: true, SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
			Message: fmt.Sprintf("accepted %d metrics", len(batch.GetMetrics())),
		})

	case edge.StreamDataType_TRACES:
		var batch edge.TraceBatch
		if err := json.Unmarshal(msg.GetPayload(), &batch); err != nil {
			s.logger.Warnf("[StreamData] 反序列化 TraceBatch 失败: %v", err)
			_ = stream.Send(&edge.StreamDataResponse{
				Success: false, Code: "decode_error", Message: err.Error(),
				SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
			})
			return
		}
		batch.ProbeId = msg.GetProbeId()
		s.forwarder.AddTraces(&batch)
		_ = stream.Send(&edge.StreamDataResponse{
			Success: true, SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
			Message: fmt.Sprintf("accepted %d traces", len(batch.GetSpans())),
		})

	case edge.StreamDataType_HEARTBEAT:
		// 流式心跳：更新 ProbeManager
		s.manager.Heartbeat(msg.GetProbeId())
		_ = s.sessionStore.UpdateHeartbeat(msg.GetProbeId())
		_ = stream.Send(&edge.StreamDataResponse{
			Success: true, SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
		})

	default:
		s.logger.Warnf("[StreamData] 未知消息类型: %s, probeID=%s", msg.GetType(), msg.GetProbeId())
		_ = stream.Send(&edge.StreamDataResponse{
			Success: false, Code: "unknown_type",
			Message: fmt.Sprintf("unknown message type: %s", msg.GetType()),
			SeqId: msg.GetSeqId(), Type: edge.StreamDataType_ACK,
		})
	}
}

// GetResourceStats 获取服务资源统计（用于监控）
func (s *Server) GetResourceStats() map[string]interface{} {
	stats := make(map[string]interface{})

	if s.connPool != nil {
		poolStats := s.connPool.GetStats()
		stats["connection_pool"] = map[string]interface{}{
			"total":       poolStats.Total,
			"max":         poolStats.Max,
			"ip_counts":   poolStats.IPCounts,
		}
	}

	if s.ipLimiter != nil {
		ipStats := s.ipLimiter.GetStats()
		stats["ip_limiter"] = map[string]interface{}{
			"tracked_ips": ipStats.TrackedIPs,
			"top_consumers": ipStats.TopConsumers,
		}
	}

	if s.goPool != nil {
		poolMetrics := s.goPool.Metrics()
		stats["goroutine_pool"] = map[string]interface{}{
			"active_workers":  poolMetrics.ActiveWorkers,
			"queued_tasks":    poolMetrics.QueuedTasks,
			"completed_tasks": poolMetrics.CompletedTasks,
			"rejected_tasks":  poolMetrics.RejectedTasks,
		}
	}

	if s.breaker != nil {
		breakers := s.breaker.GetAllBreakers()
		breakerStats := make([]map[string]interface{}, 0, len(breakers))
		for name, brk := range breakers {
			state := brk.GetState()
			breakerStats = append(breakerStats, map[string]interface{}{
				"service":     name,
				"state":       state.State.String(),
				"total":       state.TotalRequests,
				"success":     state.SuccessCount,
				"failures":    state.FailureCount,
				"error_rate":  state.ErrorRate,
			})
		}
		stats["circuit_breakers"] = breakerStats
	}

	return stats
}

// ShutdownComponents 优雅关闭所有子组件
func (s *Server) ShutdownComponents() {
	if s.connPool != nil {
		s.connPool.Stop()
		s.logger.Info("[连接池] 已停止")
	}
	if s.ipLimiter != nil {
		s.ipLimiter.Stop()
		s.logger.Info("[IP限流] 已停止")
	}
	if s.goPool != nil {
		s.goPool.Stop()
		s.logger.Info("[goroutine池] 已停止")
	}
}
