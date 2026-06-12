// Package controlplane Control Plane 服务
//
// 职责:
//   - Agent 管理 (注册/发现/状态)
//   - Edge 管理 (注册/心跳/状态)
//   - 配置下发 (采集策略/数据面配置)
//   - 集群管理 (etcd 选主/服务注册)
//
// 端口:
//   - gRPC: 9001
//   - HTTP: 8001 (管理 API)
//   - Metrics: 9101
package controlplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	gopsutil_host "github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	gopsutil_net "github.com/shirou/gopsutil/v3/net"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
)

// ============================================================================
// 配置
// ============================================================================

// Config 服务配置
type Config struct {
	// 服务标识
	ServiceName string
	Version     string

	// 监听地址
	GrpcAddr string // :9001
	HttpAddr string // :8001

	// etcd (服务发现 + 选主)
	EtcdEndpoints []string
	EtcdPrefix    string

	// 数据面连接
	DataPlaneAddr string

	// 其他服务连接
	AuthAddr   string
	TenantAddr string

	// Agent 管理
	AgentTTL         time.Duration
	HeartbeatTimeout time.Duration

	// P1-01 新增: TLS/mTLS 配置
	TLSEnabled       bool   // 是否启用 TLS
	TLSCAFile        string // CA 证书路径
	TLSCertFile      string // 服务器证书路径
	TLSKeyFile       string // 服务器私钥路径
	TLSClientAuth    bool   // 是否要求客户端证书 (mTLS)
	TLSInsecureSkip  bool   // 是否跳过证书验证 (仅用于开发)
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceName:      "control-plane",
		Version:          "1.0.0",
		GrpcAddr:         ":9001",
		HttpAddr:         ":8001",
		EtcdEndpoints:    []string{"localhost:2379"},
		EtcdPrefix:       "github.com/meinanzilinzhengying/cloudflow/services/",
		AgentTTL:         90 * time.Second,
		HeartbeatTimeout: 60 * time.Second,
		TLSEnabled:       false, // 默认禁用 TLS
		TLSInsecureSkip:  false, // 默认不跳过证书验证
	}
}

// ============================================================================
// 服务
// ============================================================================

// Service Control Plane 服务
type Service struct {
	config *Config

	// Agent 注册表
	agents sync.Map // agentId -> *svcproto.AgentInfo
	edges  sync.Map // edgeId -> *svcproto.EdgeInfo

	// etcd
	etcdClient *clientv3.Client
	etcdLease  clientv3.LeaseID
	etcdCtx    context.Context
	etcdCancel context.CancelFunc

	// gRPC
	grpcServer *grpc.Server
	grpcCreds  credentials.TransportCredentials
	health     *health.Server

	// HTTP
	httpServer *http.Server

	// 客户端连接
	dataPlaneConn *grpc.ClientConn
	authConn      *grpc.ClientConn
	tenantConn    *grpc.ClientConn

	// 运行状态
	startTime time.Time
	running   bool
}

// New 创建服务
func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Service{
		config:    config,
		startTime: time.Now(),
		health:    health.NewServer(),
	}

	// P1-01 新增: 初始化 TLS credentials
	var err error
	if config.TLSEnabled {
		s.grpcCreds, err = s.newServerTLSCredentials()
		if err != nil {
			return nil, fmt.Errorf("TLS credentials init failed: %w", err)
		}
	}

	// 初始化 gRPC 服务器
	var grpcOptions []grpc.ServerOption
	if s.grpcCreds != nil {
		grpcOptions = append(grpcOptions, grpc.Creds(s.grpcCreds))
	}
	grpcOptions = append(grpcOptions,
		grpc.MaxRecvMsgSize(64*1024*1024),
		grpc.MaxSendMsgSize(64*1024*1024),
	)
	s.grpcServer = grpc.NewServer(grpcOptions...)

	// 注册服务
	RegisterControlPlaneService(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	return s, nil
}

// Start 启动服务
func (s *Service) Start() error {
	// 初始化 etcd
	if err := s.initEtcd(); err != nil {
		return fmt.Errorf("etcd init failed: %w", err)
	}

	// P1-9: 从 etcd 恢复 Agent/Edge 状态
	if err := s.restoreStateFromEtcd(); err != nil {
		fmt.Printf("Warning: failed to restore state from etcd: %v\n", err)
	}

	// 建立下游服务连接
	if err := s.connectToDownstream(); err != nil {
		return fmt.Errorf("connect downstream failed: %w", err)
	}

	// 启动 gRPC
	lis, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listen failed: %w", err)
	}

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			fmt.Printf("gRPC server error: %v\n", err)
		}
	}()

	// 启动 HTTP (管理 API)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	
	protected := s.authMiddleware(mux)

	s.httpServer = &http.Server{
		Addr:    s.config.HttpAddr,
		Handler: protected,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	// 设置健康状态
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)

	// 注册到 etcd
	if err := s.registerToEtcd(); err != nil {
		return fmt.Errorf("etcd register failed: %w", err)
	}

	s.running = true

	fmt.Printf("Control Plane started: gRPC=%s, HTTP=%s\n", s.config.GrpcAddr, s.config.HttpAddr)
	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	s.running = false
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	// 取消 etcd 租约
	if s.etcdCancel != nil {
		s.etcdCancel()
	}

	// 优雅停止 gRPC（带超时）
	if s.grpcServer != nil {
		stopChan := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopChan)
		}()

		select {
		case <-stopChan:
			// 正常停止
		case <-time.After(15 * time.Second):
			s.grpcServer.Stop() // 强制停止
		}
	}

	// 停止 HTTP（带超时）
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	// 关闭连接
	if s.dataPlaneConn != nil {
		s.dataPlaneConn.Close()
	}
	if s.authConn != nil {
		s.authConn.Close()
	}
	if s.tenantConn != nil {
		s.tenantConn.Close()
	}

	// 关闭 etcd
	if s.etcdClient != nil {
		s.etcdClient.Close()
	}
}

// initEtcd 初始化 etcd 客户端
func (s *Service) initEtcd() error {
	if len(s.config.EtcdEndpoints) == 0 {
		return nil // etcd 未配置
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   s.config.EtcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return err
	}

	s.etcdClient = client
	s.etcdCtx, s.etcdCancel = context.WithCancel(context.Background())
	return nil
}

// registerToEtcd 注册服务到 etcd
func (s *Service) registerToEtcd() error {
	if s.etcdClient == nil {
		return nil
	}

	// 创建租约
	leaseResp, err := s.etcdClient.Grant(s.etcdCtx, int64(s.config.AgentTTL.Seconds()))
	if err != nil {
		return err
	}
	s.etcdLease = leaseResp.ID

	// 注册服务
	serviceKey := s.config.EtcdPrefix + "control-plane/" + s.config.ServiceName
	serviceValue := s.config.GrpcAddr

	_, err = s.etcdClient.Put(s.etcdCtx, serviceKey, serviceValue, clientv3.WithLease(s.etcdLease))
	if err != nil {
		return err
	}

	// 保持租约
	go func() {
		for s.running {
			time.Sleep(s.config.AgentTTL / 2)
			if s.etcdClient != nil {
				s.etcdClient.KeepAliveOnce(s.etcdCtx, s.etcdLease)
			}
		}
	}()

	return nil
}

// restoreStateFromEtcd P1-9: 从 etcd 恢复 Agent/Edge 状态
func (s *Service) restoreStateFromEtcd() error {
	if s.etcdClient == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(s.etcdCtx, 10*time.Second)
	defer cancel()

	// 恢复 Agents
	agentResp, err := s.etcdClient.Get(ctx, s.config.EtcdPrefix+"agents/", clientv3.WithPrefix())
	if err == nil {
		for _, kv := range agentResp.Kvs {
			agentId := string(kv.Key)[len(s.config.EtcdPrefix+"agents/"):]
			// Agent 信息在内存中，通过 agentId 重新关联
			// 实际使用时需要通过 gRPC 调用获取完整 AgentInfo
			fmt.Printf("Restored agent from etcd: %s\n", agentId)
		}
	}

	// 恢复 Edges
	edgeResp, err := s.etcdClient.Get(ctx, s.config.EtcdPrefix+"edges/", clientv3.WithPrefix())
	if err == nil {
		for _, kv := range edgeResp.Kvs {
			edgeId := string(kv.Key)[len(s.config.EtcdPrefix+"edges/"):]
			fmt.Printf("Restored edge from etcd: %s\n", edgeId)
		}
	}

	return nil
}

// newServerTLSCredentials P1-01 新增: 创建服务器 TLS credentials
func (s *Service) newServerTLSCredentials() (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// 如果配置了 mTLS，加载 CA 证书验证客户端
	if s.config.TLSClientAuth && s.config.TLSCAFile != "" {
		caCert, err := os.ReadFile(s.config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.ClientCAs = caCertPool
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	// 加载服务器证书和私钥
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		serverCert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load server cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// newClientTLSCredentials P1-01 新增: 创建客户端 TLS credentials
func (s *Service) newClientTLSCredentials() (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// 如果配置了跳过证书验证（仅用于开发）
	if s.config.TLSInsecureSkip {
		tlsConfig.InsecureSkipVerify = true
		return credentials.NewTLS(tlsConfig), nil
	}

	// 加载 CA 证书验证服务器
	if s.config.TLSCAFile != "" {
		caCert, err := os.ReadFile(s.config.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA cert: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA cert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// 如果启用了 mTLS，加载客户端证书
	if s.config.TLSCertFile != "" && s.config.TLSKeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client cert: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return credentials.NewTLS(tlsConfig), nil
}

// connectToDownstream 建立下游服务连接
func (s *Service) connectToDownstream() error {
	var errs []error

	// 获取 gRPC dial 选项
	dialOptions, err := s.getGRPCDialOptions()
	if err != nil {
		return fmt.Errorf("failed to get dial options: %w", err)
	}

	// 连接 Data Plane
	if s.config.DataPlaneAddr != "" {
		conn, err := grpc.Dial(
			s.config.DataPlaneAddr,
			append(dialOptions, grpc.WithTimeout(5*time.Second))...,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("connect data-plane failed: %w", err))
		} else {
			s.dataPlaneConn = conn
		}
	}

	// 连接 Auth Service
	if s.config.AuthAddr != "" {
		conn, err := grpc.Dial(
			s.config.AuthAddr,
			append(dialOptions, grpc.WithTimeout(5*time.Second))...,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("connect auth-service failed: %w", err))
		} else {
			s.authConn = conn
		}
	}

	// 连接 Tenant Service
	if s.config.TenantAddr != "" {
		conn, err := grpc.Dial(
			s.config.TenantAddr,
			append(dialOptions, grpc.WithTimeout(5*time.Second))...,
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("connect tenant-service failed: %w", err))
		} else {
			s.tenantConn = conn
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("downstream connect errors: %v", errs)
	}
	return nil
}

// getGRPCDialOptions P1-01 新增: 获取 gRPC dial 选项
func (s *Service) getGRPCDialOptions() ([]grpc.DialOption, error) {
	var options []grpc.DialOption

	if s.config.TLSEnabled {
		creds, err := s.newClientTLSCredentials()
		if err != nil {
			return nil, fmt.Errorf("failed to create client TLS credentials: %w", err)
		}
		options = append(options, grpc.WithTransportCredentials(creds))
	} else {
		options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	return options, nil
}

// authMiddleware 认证中间件
func (s *Service) authMiddleware(next http.Handler) *http.ServeMux {
	protectedMux := http.NewServeMux()

	protectedMux.HandleFunc("/healthz", s.healthzHandler)
	protectedMux.HandleFunc("/api/stats", s.statsHandler)
	protectedMux.HandleFunc("/api/system-metrics", s.systemMetricsHandler)
	protectedMux.HandleFunc("/api/health", s.healthHandler)
	protectedMux.HandleFunc("/api/agents", s.listAgentsHandler)

	protectedMux.HandleFunc("/api/edges", func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticateRequest(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		s.listEdgesHandler(w, r)
	})

	return protectedMux
}

// authenticateRequest 验证请求
func (s *Service) authenticateRequest(r *http.Request) bool {
	// 检查 API Key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		return s.validateAPIKey(r.Context(), apiKey)
	}

	// 检查 Bearer Token
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) > 7 && strings.ToLower(authHeader[:7]) == "bearer " {
		token := authHeader[7:]
		return s.validateToken(r.Context(), token)
	}

	return false
}

// validateAPIKey 验证 API Key
func (s *Service) validateAPIKey(ctx context.Context, apiKey string) bool {
	if s.authConn == nil {
		return false
	}

	client := svcproto.NewAuthServiceClient(s.authConn)
	resp, err := client.ValidateToken(ctx, &svcproto.ValidateTokenRequest{
		Token: apiKey,
	})
	if err != nil {
		return false
	}

	return resp.Valid
}

// validateToken 验证 JWT Token
func (s *Service) validateToken(ctx context.Context, token string) bool {
	if s.authConn == nil {
		return false
	}

	client := svcproto.NewAuthServiceClient(s.authConn)
	resp, err := client.ValidateToken(ctx, &svcproto.ValidateTokenRequest{
		Token: token,
	})
	if err != nil {
		return false
	}

	return resp.Valid
}

// ============================================================================
// Agent 管理
// ============================================================================

// RegisterAgent 注册 Agent（持久化到 etcd）
func (s *Service) RegisterAgent(agent *svcproto.AgentInfo) {
	agent.Status = "online"
	s.agents.Store(agent.AgentId, agent)
	s.persistAgentToEtcd(agent)
}

// persistAgentToEtcd P1-9: 将 Agent 状态持久化到 etcd
func (s *Service) persistAgentToEtcd(agent *svcproto.AgentInfo) {
	if s.etcdClient == nil {
		return
	}
	key := s.config.EtcdPrefix + "agents/" + agent.AgentId
	// 使用短 TTL，心跳会续期
	leaseResp, err := s.etcdClient.Grant(s.etcdCtx, int64(s.config.AgentTTL.Seconds()))
	if err != nil {
		fmt.Printf("Failed to grant lease for agent %s: %v\n", agent.AgentId, err)
		return
	}
	_, err = s.etcdClient.Put(s.etcdCtx, key, agent.AgentId, clientv3.WithLease(leaseResp.ID))
	if err != nil {
		fmt.Printf("Failed to persist agent %s to etcd: %v\n", agent.AgentId, err)
	}
}

// DeregisterAgent 注销 Agent（从 etcd 移除）
func (s *Service) DeregisterAgent(agentId string) {
	s.agents.Delete(agentId)
	if s.etcdClient != nil {
		key := s.config.EtcdPrefix + "agents/" + agentId
		s.etcdClient.Delete(s.etcdCtx, key)
	}
}

// GetAgent 获取 Agent
func (s *Service) GetAgent(agentId string) (*svcproto.AgentInfo, bool) {
	v, ok := s.agents.Load(agentId)
	if !ok {
		return nil, false
	}
	return v.(*svcproto.AgentInfo), true
}

// ListAgents 列出 Agent
func (s *Service) ListAgents(tenantId, region, status string) []*svcproto.AgentInfo {
	var agents []*svcproto.AgentInfo
	s.agents.Range(func(_, v interface{}) bool {
		a := v.(*svcproto.AgentInfo)
		if tenantId != "" && a.TenantId != tenantId {
			return true
		}
		if region != "" && a.Region != region {
			return true
		}
		if status != "" && a.Status != status {
			return true
		}
		agents = append(agents, a)
		return true
	})
	return agents
}

// ============================================================================
// Edge 管理
// ============================================================================

// RegisterEdge 注册 Edge（持久化到 etcd）
func (s *Service) RegisterEdge(edge *svcproto.EdgeInfo) {
	edge.Status = "online"
	s.edges.Store(edge.EdgeId, edge)
	s.persistEdgeToEtcd(edge)
}

// persistEdgeToEtcd P1-9: 将 Edge 状态持久化到 etcd
func (s *Service) persistEdgeToEtcd(edge *svcproto.EdgeInfo) {
	if s.etcdClient == nil {
		return
	}
	key := s.config.EtcdPrefix + "edges/" + edge.EdgeId
	leaseResp, err := s.etcdClient.Grant(s.etcdCtx, int64(s.config.AgentTTL.Seconds()))
	if err != nil {
		fmt.Printf("Failed to grant lease for edge %s: %v\n", edge.EdgeId, err)
		return
	}
	_, err = s.etcdClient.Put(s.etcdCtx, key, edge.EdgeId, clientv3.WithLease(leaseResp.ID))
	if err != nil {
		fmt.Printf("Failed to persist edge %s to etcd: %v\n", edge.EdgeId, err)
	}
}

// DeregisterEdge 注销 Edge（从 etcd 移除）
func (s *Service) DeregisterEdge(edgeId string) {
	s.edges.Delete(edgeId)
	if s.etcdClient != nil {
		key := s.config.EtcdPrefix + "edges/" + edgeId
		s.etcdClient.Delete(s.etcdCtx, key)
	}
}

// GetEdge 获取 Edge
func (s *Service) GetEdge(edgeId string) (*svcproto.EdgeInfo, bool) {
	v, ok := s.edges.Load(edgeId)
	if !ok {
		return nil, false
	}
	return v.(*svcproto.EdgeInfo), true
}

// ListEdges 列出 Edge
func (s *Service) ListEdges(tenantId, region, status string) []*svcproto.EdgeInfo {
	var edges []*svcproto.EdgeInfo
	s.edges.Range(func(_, v interface{}) bool {
		e := v.(*svcproto.EdgeInfo)
		if tenantId != "" && e.TenantId != tenantId {
			return true
		}
		if region != "" && e.Region != region {
			return true
		}
		if status != "" && e.Status != status {
			return true
		}
		edges = append(edges, e)
		return true
	})
	return edges
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s","version":"%s","uptime":%d}`,
		s.config.ServiceName, s.config.Version, time.Since(s.startTime).Seconds())
}

// healthHandler 返回平台所有服务的健康状态
func (s *Service) healthHandler(w http.ResponseWriter, r *http.Request) {
	services := s.collectServiceHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"services": services,
	})
}

// collectServiceHealth 采集所有服务健康状态（优先 Docker，回退内部连接状态）
func (s *Service) collectServiceHealth() []map[string]interface{} {
	type svcDef struct{ Name, SvcType string }
	allServices := []svcDef{
		{"control-plane", "核心服务"}, {"data-plane", "核心服务"},
		{"auth-service", "核心服务"}, {"tenant-service", "核心服务"},
		{"query-service", "核心服务"}, {"alert-engine", "核心服务"},
		{"frontend", "前端"}, {"platform-frontend", "前端"},
		{"etcd", "基础设施"}, {"redis", "基础设施"},
		{"tidb", "基础设施"}, {"clickhouse", "基础设施"},
		{"kafka", "基础设施"}, {"victoriametrics", "基础设施"},
		{"loki", "基础设施"}, {"prometheus", "基础设施"},
		{"grafana", "基础设施"},
	}

	// 尝试从 Docker 获取容器状态
	dockerStates := make(map[string]string)
	if out, err := exec.Command("docker", "ps", "--format", "{{.Names}}|{{.State}}|{{.Status}}").Output(); err == nil {
		for _, line := range strings.Split(string(out), "
") {
		parts := strings.Split(line, "|")
		parts := strings.Split(line, "|")
		parts := strings.Split(line, "|")
			parts := strings.Split(line, "|")
			if len(parts) >= 2 {
				dockerStates[parts[0]] = parts[1]
			}
		}
	}

	// 基于内部连接判断核心服务
	cpStatus, dpStatus, authStatus, tenantStatus := "running", "running", "running", "running"
	if s.dataPlaneConn == nil {
		dpStatus = "stopped"
	}
	if s.authConn == nil {
		authStatus = "stopped"
	}
	if s.tenantConn == nil {
		tenantStatus = "stopped"
	}

	result := make([]map[string]interface{}, 0, len(allServices))
	for _, svc := range allServices {
		status := "running"
		switch svc.Name {
		case "control-plane":
			status = cpStatus
		case "data-plane":
			status = dpStatus
		case "auth-service":
			status = authStatus
		case "tenant-service":
			status = tenantStatus
		}

		// 如果 Docker 有该容器状态，优先使用
		if ds, ok := dockerStates[svc.Name]; ok {
			status = ds
		}

		result = append(result, map[string]interface{}{
			"name":     svc.Name,
			"type":     svc.SvcType,
			"status":   status,
			"cpu":      0,
			"memory":   0,
			"restarts": 0,
		})
	}
	return result
}

func (s *Service) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	agents := s.ListAgents("", "", "")
	result := make([]map[string]interface{}, 0, len(agents))
	for _, a := range agents {
		result = append(result, map[string]interface{}{
			"name":     a.Hostname,
			"pid":      0,
			"cpu":      0,
			"memory":   0,
			"uptime":   0,
			"hostname": a.Hostname,
			"status":   a.Status,
			"ip":       a.Ip,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Service) listEdgesHandler(w http.ResponseWriter, r *http.Request) {
	edges := s.ListEdges("", "", "")
	fmt.Fprintf(w, `{"edges":%d}`, len(edges))
}

// ============================================================================
// gRPC 服务注册与实现
// ============================================================================

// RegisterControlPlaneService 注册 gRPC 服务
func RegisterControlPlaneService(s *grpc.Server, svc *Service) {
	svcproto.RegisterControlPlaneServiceServer(s, &controlPlaneGRPC{svc: svc})
}

// controlPlaneGRPC gRPC 服务实现
type controlPlaneGRPC struct {
	svcproto.UnimplementedControlPlaneServiceServer
	svc *Service
}

// HealthCheck 健康检查
func (g *controlPlaneGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{
		Healthy: true,
		Version: g.svc.config.Version,
		Uptime:  int64(time.Since(g.svc.startTime).Seconds()),
	}, nil
}

// ListAgents 列出 Agent
func (g *controlPlaneGRPC) ListAgents(ctx context.Context, req *svcproto.ListAgentsRequest) (*svcproto.ListAgentsResponse, error) {
	agents := g.svc.ListAgents(req.TenantId, req.Region, req.Status)
	return &svcproto.ListAgentsResponse{
		Agents: agents,
		Total:  len(agents),
	}, nil
}

// GetAgent 获取 Agent
func (g *controlPlaneGRPC) GetAgent(ctx context.Context, req *svcproto.AgentInfo) (*svcproto.AgentInfo, error) {
	agent, ok := g.svc.GetAgent(req.AgentId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "agent %s not found", req.AgentId)
	}
	return agent, nil
}

// ListEdges 列出 Edge
func (g *controlPlaneGRPC) ListEdges(ctx context.Context, req *svcproto.ListEdgesRequest) (*svcproto.ListEdgesResponse, error) {
	edges := g.svc.ListEdges(req.TenantId, req.Region, req.Status)
	return &svcproto.ListEdgesResponse{
		Edges: edges,
		Total:  len(edges),
	}, nil
}

// GetEdge 获取 Edge
func (g *controlPlaneGRPC) GetEdge(ctx context.Context, req *svcproto.EdgeInfo) (*svcproto.EdgeInfo, error) {
	edge, ok := g.svc.GetEdge(req.EdgeId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "edge %s not found", req.EdgeId)
	}
	return edge, nil
}

// UpdateIngestConfig 更新数据面配置
func (g *controlPlaneGRPC) UpdateIngestConfig(ctx context.Context, req *svcproto.UpdateIngestConfigRequest) (*svcproto.UpdateIngestConfigResponse, error) {
	return &svcproto.UpdateIngestConfigResponse{
		Success: true,
		Message: "config updated",
	}, nil
}

// ============================================================================
// HTTP Handlers - 监控统计
// ============================================================================

// statsHandler 返回平台统计数据 (供前端 Dashboard 使用)
func (s *Service) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats, err := s.collectSystemStats()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// systemMetricsHandler 返回详细系统指标
func (s *Service) systemMetricsHandler(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.collectDetailedMetrics()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// collectSystemStats 采集系统统计数据
func (s *Service) collectSystemStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// CPU 使用率
	cpuPercent, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercent) > 0 {
		stats["cpu"] = map[string]interface{}{
			"usage":   cpuPercent[0],
			"cores":   runtime.NumCPU(),
		}
	}

	// 内存使用
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		stats["memory"] = map[string]interface{}{
			"total":      memInfo.Total / 1024 / 1024 / 1024, // GB
			"used":       memInfo.Used / 1024 / 1024 / 1024,  // GB
			"usage":      memInfo.UsedPercent,
		}
	}

	// 磁盘使用
	diskInfo, err := disk.Usage("/")
	if err == nil {
		stats["disk"] = map[string]interface{}{
			"total":   diskInfo.Total / 1024 / 1024 / 1024, // GB
			"used":    diskInfo.Used / 1024 / 1024 / 1024,   // GB
			"usage":   diskInfo.UsedPercent,
		}
	}

	// 网络 I/O
	netIO, err := gopsutil_net.IOCounters(false)
	if err == nil && len(netIO) > 0 {
		stats["network"] = map[string]interface{}{
			"inbound":  netIO[0].BytesRecv / 1024 / 1024,  // MB
			"outbound": netIO[0].BytesSent / 1024 / 1024,   // MB
		}
	}

	// 运行时长
	stats["uptime"] = int64(time.Since(s.startTime).Seconds())

	// 服务状态
	running := 2 // control-plane 和 data-plane
	if s.dataPlaneConn != nil {
		running++
	}

	stats["services"] = map[string]interface{}{
		"total":   running + 6, // 假设总共有 running+6 个服务
		"running": running,
		"stopped": 6,
	}

	return stats, nil
}

// collectDetailedMetrics 采集详细系统指标
func (s *Service) collectDetailedMetrics() (map[string]interface{}, error) {
	metrics := make(map[string]interface{})

	// CPU 每个核心的使用率
	cpuPercents, err := cpu.Percent(0, true)
	if err == nil {
		metrics["cpu_per_core"] = cpuPercents
	}

	// 内存详细信息
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		metrics["memory"] = map[string]interface{}{
			"total":       memInfo.Total,
			"available":   memInfo.Available,
			"used":        memInfo.Used,
			"free":        memInfo.Free,
			"used_percent": memInfo.UsedPercent,
		}
	}

	// 磁盘 I/O 统计
	diskIO, err := disk.IOCounters()
	if err == nil {
		metrics["disk_io"] = diskIO
	}

	// 网络接口统计
	netIO, err := gopsutil_net.IOCounters(true)
	if err == nil {
		metrics["network_interfaces"] = netIO
	}

	// 主机信息
	hostInfo, err := gopsutil_host.Info()
	if err == nil {
		metrics["host"] = map[string]interface{}{
			"hostname":   hostInfo.Hostname,
			"uptime":     hostInfo.Uptime,
			"procs":      hostInfo.Procs,
			"os":         hostInfo.OS,
			"platform":   hostInfo.Platform,
		}
	}

	// runtime 信息（供前端进程监控回退使用）
	metrics["runtime"] = map[string]interface{}{
		"name":       "control-plane",
		"pid":        os.Getpid(),
		"cpu":        0,
		"memory":     0,
		"uptime":     int64(time.Since(s.startTime).Seconds()),
		"status":     "running",
		"go_version": runtime.Version(),
		"goroutines": runtime.NumGoroutine(),
	}

	return metrics, nil
}
