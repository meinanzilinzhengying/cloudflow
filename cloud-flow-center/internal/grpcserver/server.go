package grpcserver

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"cloud-flow-center/internal/config"
	"cloud-flow-center/internal/edgeregistry"
	"cloud-flow-center/internal/storage"
	"cloud-flow-center/pkg/logger"
	"cloud-flow/pkg/ratelimit"
	"cloud-flow/pkg/trace"
	edge "cloud-flow/proto"
)

// Server 中心服务 gRPC 服务端
type Server struct {
	edge.UnimplementedCenterServiceServer
	storage  storage.StorageEngine
	logger   *logger.Logger
	apiKey   string
	registry *edgeregistry.Registry // P1: Edge 节点注册表
}

// NewServer 创建服务端
func NewServer(s storage.StorageEngine, log *logger.Logger, apiKey string) *Server {
	return &Server{storage: s, logger: log, apiKey: apiKey}
}

// SetEdgeRegistry 设置 Edge 注册表（P1: Center Edge 注册表）
func (s *Server) SetEdgeRegistry(registry *edgeregistry.Registry) {
	s.registry = registry
}

// GetEdgeRegistry 获取 Edge 注册表
func (s *Server) GetEdgeRegistry() *edgeregistry.Registry {
	return s.registry
}



// TraceIDInterceptor Trace ID 拦截器
func TraceIDInterceptor(log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 从 metadata 中提取 Trace ID
		md, ok := metadata.FromIncomingContext(ctx)
		var traceID string
		if ok {
			vals := md.Get(trace.TraceIDMDKey)
			if len(vals) > 0 {
				traceID = vals[0]
			}
		}

		// 如果没有 Trace ID，生成一个新的
		if traceID == "" {
			traceID = trace.TraceID()
		}

		// 将 Trace ID 添加到 context 中
		ctx = trace.WithTraceID(ctx, traceID)

		// 继续处理请求
		return handler(ctx, req)
	}
}

// APIKeyAuthInterceptor API Key 认证拦截器
func APIKeyAuthInterceptor(apiKey string, log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if apiKey == "" {
			// 未配置 API Key，直接放行
			return handler(ctx, req)
		}

		// 从 metadata 中获取 API Key
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			log.Warnf("API Key 认证失败: 无 metadata")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败")
		}

		vals := md.Get("x-api-key")
		if len(vals) == 0 {
			log.Warnf("API Key 认证失败: 无 x-api-key 头")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败")
		}

		if subtle.ConstantTimeCompare([]byte(vals[0]), []byte(apiKey)) != 1 {
			log.Warnf("API Key 认证失败: 无效的 API Key")
			return nil, status.Errorf(codes.Unauthenticated, "API Key 认证失败")
		}

		// 认证通过，继续处理请求
		return handler(ctx, req)
	}
}

// BuildServerOpts 根据配置构建 gRPC Server 选项
func BuildServerOpts(tlsCfg config.TLSConfig, rateLimit config.RateLimitConfig, apiKey string, log *logger.Logger) ([]grpc.ServerOption, error) {
	var opts []grpc.ServerOption
	
	// 添加 Trace ID 拦截器
	traceInterceptor := TraceIDInterceptor(log)
	
	// 添加限流拦截器
	var rateLimiter *ratelimit.TokenBucket
	if rateLimit.Enabled {
		rateLimiter = ratelimit.NewTokenBucket(rateLimit.BucketSize, rateLimit.RefillRate)
		log.Infof("Center gRPC 服务端启用速率限制: 桶容量=%d, 填充速率=%d/秒", rateLimit.BucketSize, rateLimit.RefillRate)
	} else {
		log.Info("Center gRPC 服务端未启用速率限制")
	}
	
	// 公共拦截逻辑：panic 恢复 + 限流 + Trace ID
	runWithCommonInterceptors := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, finalHandler grpc.UnaryHandler) (interface{}, error) {
		var panicErr error
		var result interface{}
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("gRPC 处理 panic: %v", r)
					panicErr = status.Errorf(codes.Internal, "internal server error")
				}
			}()
			// 先处理限流
			if rateLimit.Enabled && !rateLimiter.Allow() {
				panicErr = status.Errorf(codes.ResourceExhausted, "rate limit exceeded")
				return
			}
			// 再处理 Trace ID
			result, panicErr = traceInterceptor(ctx, req, info, finalHandler)
		}()
		if panicErr != nil {
			return nil, panicErr
		}
		return result, nil
	}

	// H2 修复: 强制要求 API Key，不允许绕过认证
	// 原因: 未配置 API Key 时，虽然启用了限流等拦截器，但缺少认证是严重的安全隐患
	// N1 修复: 更正错误提示中的环境变量名称（实际绑定的是 CLOUD_FLOW_API_KEY）
	if apiKey == "" {
		return nil, fmt.Errorf("API Key 未配置。请设置 CLOUD_FLOW_API_KEY 环境变量或配置文件 center.api_key")
	}

	// 组合拦截器: Trace ID + 限流 + API Key 认证
	combinedInterceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return runWithCommonInterceptors(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			apiInterceptor := APIKeyAuthInterceptor(apiKey, log)
			return apiInterceptor(ctx, req, info, handler)
		})
	}
	opts = append(opts, grpc.UnaryInterceptor(combinedInterceptor))
	log.Info("Center gRPC 服务端启用 Trace ID、API Key 认证和速率限制")
	
	if !tlsCfg.Enabled {
		log.Warn("Center gRPC 服务端未启用 TLS，将使用明文传输")
		return opts, nil
	}

	if tlsCfg.ServerCert == "" || tlsCfg.ServerKey == "" {
		log.Info("未配置 Center 服务端证书，将自动生成自签名证书")
		cert, certPath, keyPath, err := genSelfSignedCert("./certs")
		if err != nil {
			return nil, fmt.Errorf("生成自签名证书失败: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.NoClientCert,
		}
		log.Infof("Center TLS 自签名证书已生成: cert=%s, key=%s", certPath, keyPath)
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
		return opts, nil
	}

	cert, err := tls.LoadX509KeyPair(tlsCfg.ServerCert, tlsCfg.ServerKey)
	if err != nil {
		return nil, fmt.Errorf("加载 Center 服务端证书失败: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.NoClientCert,
	}

	if tlsCfg.CACert != "" {
		caPEM, err := os.ReadFile(tlsCfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("读取 CA 证书失败: %w", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("解析 CA 证书失败")
		}
		tlsConfig.ClientCAs = certPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		log.Info("Center gRPC 服务端启用 mTLS")
	} else {
		log.Info("Center gRPC 服务端启用 TLS（单向）")
	}

	opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	return opts, nil
}

// RegisterHealthChecks 注册 gRPC 健康检查服务
func RegisterHealthChecks(s *grpc.Server, log *logger.Logger) {
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)
	log.Info("gRPC 健康检查服务已注册")
}

func (s *Server) ReportProbes(ctx context.Context, req *edge.ReportProbesRequest) (*edge.ReportProbesResponse, error) {
	if s.storage == nil {
		return nil, status.Errorf(codes.Unavailable, "存储服务不可用")
	}
	s.logger.Infof("收到探针上报: edgeNode=%s, platform=%s, region=%s, probes=%d",
		req.GetEdgeNodeId(), req.GetCloudPlatform(), req.GetRegion(), len(req.GetProbes()))

	probes := req.GetProbes()
	var updatedAt int64
	if len(probes) > 0 {
		updatedAt = probes[0].GetLastHeartbeat()
	}
	if err := s.storage.SaveProbeInfo(req.GetEdgeNodeId(), map[string]interface{}{
		"edge_node_id":   req.GetEdgeNodeId(),
		"cloud_platform": req.GetCloudPlatform(),
		"region":         req.GetRegion(),
		"probes":         probes,
		"updated_at":     updatedAt,
	}); err != nil {
		s.logger.Errorf("保存探针信息失败: %v", err)
	}
	return &edge.ReportProbesResponse{Success: true}, nil
}

func (s *Server) ForwardMetrics(ctx context.Context, batch *edge.MetricsBatch) (*edge.ForwardResponse, error) {
	if s.storage == nil {
		return nil, status.Errorf(codes.Unavailable, "存储服务不可用")
	}
	// 创建 slice 副本后再修改，避免修改原始请求中的数据
	metrics := batch.GetMetrics()
	metricsCopy := make([]*edge.MetricData, len(metrics))
	copy(metricsCopy, metrics)
	for _, metric := range metricsCopy {
		metric.ProbeId = batch.GetProbeId()
	}
	
	if err := s.storage.SaveMetrics(batch.GetProbeId(), metricsCopy); err != nil {
		s.logger.Errorf("保存指标数据失败: %v", err)
		return &edge.ForwardResponse{Success: false}, nil
	}
	return &edge.ForwardResponse{Success: true}, nil
}

func (s *Server) ForwardTraces(ctx context.Context, batch *edge.TraceBatch) (*edge.ForwardResponse, error) {
	if s.storage == nil {
		return nil, status.Errorf(codes.Unavailable, "存储服务不可用")
	}
	// 创建 slice 副本后再修改，避免修改原始请求中的数据
	spans := batch.GetSpans()
	spansCopy := make([]*edge.TraceSpanData, len(spans))
	copy(spansCopy, spans)
	for _, span := range spansCopy {
		span.ProbeId = batch.GetProbeId()
	}
	
	if err := s.storage.SaveTraces(batch.GetProbeId(), spansCopy); err != nil {
		s.logger.Errorf("保存链路追踪数据失败: %v", err)
		return &edge.ForwardResponse{Success: false}, nil
	}
	return &edge.ForwardResponse{Success: true}, nil
}

func (s *Server) ForwardProfiling(ctx context.Context, batch *edge.ProfilingBatch) (*edge.ForwardResponse, error) {
	if s.storage == nil {
		return nil, status.Errorf(codes.Unavailable, "存储服务不可用")
	}
	// 创建 slice 副本后再修改，避免修改原始请求中的数据
	profiles := batch.GetProfiles()
	profilesCopy := make([]*edge.ProfilingData, len(profiles))
	copy(profilesCopy, profiles)
	for _, profile := range profilesCopy {
		profile.ProbeId = batch.GetProbeId()
	}
	
	if err := s.storage.SaveProfiling(batch.GetProbeId(), profilesCopy); err != nil {
		s.logger.Errorf("保存性能分析数据失败: %v", err)
		return &edge.ForwardResponse{Success: false}, nil
	}
	return &edge.ForwardResponse{Success: true}, nil
}

func (s *Server) Heartbeat(ctx context.Context, req *edge.EdgeHeartbeatRequest) (*edge.EdgeHeartbeatResponse, error) {
	s.logger.Debugf("收到边缘心跳: edgeNode=%s, probes=%d",
		req.GetEdgeNodeId(), req.GetProbeCount())

	// P1: 更新 Edge 注册表
	if s.registry != nil && req.GetEdgeNodeId() != "" {
		s.registry.RegisterOrUpdate(&edgeregistry.EdgeNode{
			ID:            req.GetEdgeNodeId(),
			Address:       req.GetEdgeAddress(),
			Region:        req.GetRegion(),
			CloudPlatform: req.GetCloudPlatform(),
			ProbeCount:    req.GetProbeCount(),
			Version:       req.GetVersion(),
		})
	}

	// H3 修复: 返回心跳间隔，允许 Center 动态控制 Edge 心跳频率
	return &edge.EdgeHeartbeatResponse{
		Success:           true,
		HeartbeatInterval: 30, // 默认 30 秒，可从配置读取
	}, nil
}

// GetEdgeStatus 获取 Edge 节点状态（P1: 基于 Edge 注册表实现）
func (s *Server) GetEdgeStatus(ctx context.Context, req *edge.GetEdgeStatusRequest) (*edge.GetEdgeStatusResponse, error) {
	if s.registry == nil {
		return &edge.GetEdgeStatusResponse{
			Success: false,
			Message: "Edge 注册表未启用",
		}, nil
	}

	// 查询单个 Edge
	if req.GetEdgeNodeId() != "" {
		node, ok := s.registry.Get(req.GetEdgeNodeId())
		if !ok {
			return &edge.GetEdgeStatusResponse{
				Success: false,
				Message: fmt.Sprintf("Edge 节点不存在: %s", req.GetEdgeNodeId()),
			}, nil
		}
		return &edge.GetEdgeStatusResponse{
			Success: true,
			Total:   1,
			Statuses: []*edge.EdgeStatus{
				{
					EdgeNodeId:    node.ID,
					EdgeAddress:   node.Address,
					CloudPlatform: node.CloudPlatform,
					Region:        node.Region,
					Version:       node.Version,
					Status:        "online",
					ProbeCount:    node.ProbeCount,
				},
			},
		}, nil
	}

	// 查询所有健康 Edge
	nodes := s.registry.ListHealthy()
	statuses := make([]*edge.EdgeStatus, 0, len(nodes))
	for _, node := range nodes {
		statuses = append(statuses, &edge.EdgeStatus{
			EdgeNodeId:    node.ID,
			EdgeAddress:   node.Address,
			CloudPlatform: node.CloudPlatform,
			Region:        node.Region,
			Version:       node.Version,
			Status:        "online",
			ProbeCount:    node.ProbeCount,
		})
	}

	return &edge.GetEdgeStatusResponse{
		Success:  true,
		Total:    int32(len(statuses)),
		Statuses: statuses,
	}, nil
}
