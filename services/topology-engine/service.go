// Package topologyengine Topology Engine 服务
//
// 职责:
//   - Service Graph (服务拓扑图)
//   - Process Graph (进程通信图)
//   - Pod Graph (Pod 网络拓扑)
//   - Namespace Graph (命名空间流量图)
//   - 实时拓扑增量更新
//   - 历史拓扑查询 (ClickHouse)
//   - Latency Heatmap / Error Heatmap
//   - Topology Diff (拓扑变更检测)
//
// 架构:
//
//	UnifiedFlow ──→ IngestFlows ──→ TopologyUpdater ──→ Graph (内存)
//	                                              │
//	                                              ├──→ Cache (LRU+TTL)
//	                                              └──→ HeatmapEngine
//
//	TopologyQuery ──→ Cache.Get ──→ Response
//	HistoricalQuery ──→ ClickHouse ──→ Graph ──→ Response
//	HeatmapQuery ──→ HeatmapEngine ──→ Response
//	DiffQuery ──→ TopologyUpdater.GetDiff ──→ Response
//
// 端口:
//   - gRPC: 9004
//   - HTTP: 8004
//   - Metrics: 9104
package topologyengine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"cloud-flow/pkg/flow"
	svcproto "cloud-flow/services/proto"
	"cloud-flow/services/topology-engine/builder"
	"cloud-flow/services/topology-engine/cache"
	"cloud-flow/services/topology-engine/graph"
	"cloud-flow/services/topology-engine/heatmap"
	"cloud-flow/services/topology-engine/historical"
	"cloud-flow/services/topology-engine/updater"
	"cloud-flow/services/shared/tlsutil"
)

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string
	GrpcAddr    string // :9004
	HttpAddr    string // :8004

	// ClickHouse (历史查询)
	ClickHouseAddr string
	ClickHouseDB   string
	ClickHouseUser string
	ClickHousePass string

	// 拓扑计算
	ComputeInterval time.Duration
	MaxNodes        int
	MaxEdges        int

	// 缓存
	CacheMaxEntries int
	CacheMaxMemoryMB int
	CacheTTL        time.Duration

	// Heatmap
	HeatmapInterval int64 // seconds

	// 增量更新
	UpdateInterval  time.Duration
	StaleThreshold  time.Duration

	// P0-2 修复: TLS 配置
	TLSEnabled      bool
	TLSCAFile       string
	TLSCertFile     string
	TLSKeyFile      string
	TLSClientAuth   bool
	TLSInsecureSkip bool
}

func DefaultConfig() *Config {
	return &Config{
		ServiceName:      "topology-engine",
		Version:          "1.0.0",
		GrpcAddr:         ":9004",
		HttpAddr:         ":8004",
		ClickHouseAddr:   "clickhouse:8123",
		ClickHouseDB:     "cloudflow",
		ClickHouseUser:   "default",
		ClickHousePass:   "",
		ComputeInterval:  30 * time.Second,
		MaxNodes:         50000,
		MaxEdges:         1000000,
		CacheMaxEntries:  1000,
		CacheMaxMemoryMB: 512,
		CacheTTL:         5 * time.Minute,
		HeatmapInterval:  60,
		UpdateInterval:   5 * time.Second,
		StaleThreshold:   60 * time.Second,
		TLSEnabled:       false,
		TLSInsecureSkip:  false,
	}
}

// Service Topology Engine 主服务
type Service struct {
	config *Config

	// 子组件
	graphCache   *cache.Cache
	heatmapEngine *heatmap.HeatmapEngine
	updater       *updater.TopologyUpdater
	historical    *historical.HistoricalQuery
	registry      *builder.BuilderRegistry

	// gRPC
	grpcServer *grpc.Server
	health     *health.Server

	// HTTP
	httpServer *http.Server

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials

	startTime time.Time
}

// New 创建 Topology Engine 服务
func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// 初始化缓存
	graphCache := cache.NewCache(&cache.CacheConfig{
		MaxEntries:  config.CacheMaxEntries,
		MaxMemoryMB: config.CacheMaxMemoryMB,
		DefaultTTL:  config.CacheTTL,
	})

	// 初始化 Heatmap 引擎
	heatmapEngine := heatmap.NewHeatmapEngine(heatmap.HeatmapConfig{
		TimeInterval:   config.HeatmapInterval,
		MaxBuckets:     1440,
		ErrorThreshold: 0.01,
	})

	// 初始化增量更新器
	topoUpdater := updater.NewTopologyUpdater(
		updater.UpdaterConfig{
			UpdateInterval:    config.UpdateInterval,
			StaleThreshold:    config.StaleThreshold,
			MaxNodesPerTenant: config.MaxNodes,
			MaxEdgesPerTenant: config.MaxEdges,
		},
		graphCache,
		heatmapEngine,
	)

	// 初始化历史查询
	histQuery := historical.NewHistoricalQuery(historical.ClickHouseConfig{
		Addr:     config.ClickHouseAddr,
		Database: config.ClickHouseDB,
		User:     config.ClickHouseUser,
		Password: config.ClickHousePass,
		Timeout:  30 * time.Second,
		MaxRows:  config.MaxEdges,
	})

	// 初始化 Builder Registry
	registry := builder.DefaultRegistry(builder.BuilderConfig{
		MaxNodes:        config.MaxNodes,
		MaxEdges:        config.MaxEdges,
		PruneThreshold:  0.01,
	})

	s := &Service{
		config:        config,
		graphCache:    graphCache,
		heatmapEngine: heatmapEngine,
		updater:       topoUpdater,
		historical:    histQuery,
		registry:      registry,
		startTime:     time.Now(),
		health:        health.NewServer(),
	}

	// P0-2 修复: 初始化 TLS 凭证
	if config.TLSEnabled {
		tlsCfg := tlsutil.Config{
			Enabled:      config.TLSEnabled,
			CAFile:       config.TLSCAFile,
			CertFile:     config.TLSCertFile,
			KeyFile:      config.TLSKeyFile,
			ClientAuth:   config.TLSClientAuth,
			InsecureSkip: config.TLSInsecureSkip,
		}
		var err error
		s.grpcCreds, err = tlsutil.ServerCredentials(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("TLS credentials init failed: %w", err)
		}
	}

	// 初始化 gRPC 服务器
	var grpcOptions []grpc.ServerOption
	if s.grpcCreds != nil {
		grpcOptions = append(grpcOptions, grpc.Creds(s.grpcCreds))
	}
	s.grpcServer = grpc.NewServer(grpcOptions...)
	RegisterTopologyService(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	return s, nil
}

// Start 启动服务
func (s *Service) Start() error {
	// 启动 gRPC
	lis, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listen failed: %w", err)
	}
	go func() { s.grpcServer.Serve(lis) }()

	// 启动 HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.HandleFunc("/api/topology", s.topologyHTTPHandler)
	mux.HandleFunc("/api/topology/service", s.serviceGraphHTTPHandler)
	mux.HandleFunc("/api/topology/process", s.processGraphHTTPHandler)
	mux.HandleFunc("/api/topology/pod", s.podGraphHTTPHandler)
	mux.HandleFunc("/api/topology/namespace", s.namespaceGraphHTTPHandler)
	mux.HandleFunc("/api/topology/heatmap/latency", s.latencyHeatmapHTTPHandler)
	mux.HandleFunc("/api/topology/heatmap/error", s.errorHeatmapHTTPHandler)
	mux.HandleFunc("/api/topology/diff", s.diffHTTPHandler)
	s.httpServer = &http.Server{Addr: s.config.HttpAddr, Handler: mux}
	go func() { s.httpServer.ListenAndServe() }()

	// 启动缓存清理
	go s.graphCache.StartCleanup(context.Background())

	// 启动增量更新器
	s.updater.Start()

	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)
	fmt.Printf("Topology Engine started: gRPC=%s, HTTP=%s\n", s.config.GrpcAddr, s.config.HttpAddr)

	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)
	s.updater.Stop()
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

// ============================================================================
// gRPC 服务方法实现
// ============================================================================

// QueryTopology 查询拓扑 (自动判断实时/历史)
func (s *Service) QueryTopology(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.TopologyQueryResponse, error) {
	graphType := req.Type
	if graphType == "" {
		graphType = "service"
	}

	// 判断实时 vs 历史: 如果有明确时间范围且不是最近时间，走历史查询
	now := time.Now().Unix()
	isRealtime := req.StartTime == 0 && req.EndTime == 0 ||
		(req.EndTime > 0 && now-req.EndTime < 300)

	if isRealtime {
		return s.queryRealtimeTopology(ctx, req.TenantId, graphType)
	}
	return s.queryHistoricalTopology(ctx, req)
}

// GetServiceGraph 获取服务拓扑图
func (s *Service) GetServiceGraph(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.TopologyQueryResponse, error) {
	req.Type = "service"
	return s.QueryTopology(ctx, req)
}

// GetDependencyGraph 获取依赖关系图 (带方向的 service graph)
func (s *Service) GetDependencyGraph(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.TopologyQueryResponse, error) {
	req.Type = "service"
	resp, err := s.QueryTopology(ctx, req)
	if err != nil {
		return nil, err
	}
	// Dependency graph = service graph filtered to only external edges
	// (edges crossing namespace boundaries)
	if req.TenantId == "" {
		return resp, nil
	}
	var filteredEdges []*proto.TopologyEdge
	for _, edge := range resp.Edges {
		// 找到 source 和 target 节点的 namespace
		srcNS := findNodeNamespace(resp.Nodes, edge.Source)
		dstNS := findNodeNamespace(resp.Nodes, edge.Target)
		if srcNS != dstNS {
			filteredEdges = append(filteredEdges, edge)
		}
	}
	resp.Edges = filteredEdges
	return resp, nil
}

// GetLatencyHeatmap 获取延迟热力图
func (s *Service) GetLatencyHeatmap(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.HeatmapResponse, error) {
	graphType := req.Type
	if graphType == "" {
		graphType = "service"
	}

	// 实时 heatmap
	if req.StartTime == 0 && req.EndTime == 0 {
		points := s.heatmapEngine.GetLatencyHeatmap("", "", 0, 0)
		return s.heatmapEngine.BuildResponse(points, time.Now().Add(-24*time.Hour).Unix(), time.Now().Unix()), nil
	}

	// 历史 heatmap
	points, err := s.historical.QueryHeatmap(ctx, req.TenantId, req.StartTime, req.EndTime, "latency", 60)
	if err != nil {
		return nil, fmt.Errorf("historical heatmap query failed: %w", err)
	}
	return s.heatmapEngine.BuildResponse(points, req.StartTime, req.EndTime), nil
}

// GetErrorHeatmap 获取错误热力图
func (s *Service) GetErrorHeatmap(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.HeatmapResponse, error) {
	graphType := req.Type
	if graphType == "" {
		graphType = "service"
	}

	// 实时 heatmap
	if req.StartTime == 0 && req.EndTime == 0 {
		points := s.heatmapEngine.GetErrorHeatmap("", "", 0, 0)
		return s.heatmapEngine.BuildResponse(points, time.Now().Add(-24*time.Hour).Unix(), time.Now().Unix()), nil
	}

	// 历史 heatmap
	points, err := s.historical.QueryHeatmap(ctx, req.TenantId, req.StartTime, req.EndTime, "error_rate", 60)
	if err != nil {
		return nil, fmt.Errorf("historical heatmap query failed: %w", err)
	}
	return s.heatmapEngine.BuildResponse(points, req.StartTime, req.EndTime), nil
}

// GetTopologyDiff 获取拓扑变更
func (s *Service) GetTopologyDiff(ctx context.Context, req *proto.TopologyDiffRequest) (*proto.TopologyDiffResponse, error) {
	graphType := req.Type
	if graphType == "" {
		graphType = "service"
	}

	// 使用历史查询计算两个时间点的 diff
	diff, err := s.historical.QueryTopologyDiff(ctx, req.TenantId, req.BaseTime, req.CompareTime, graphType)
	if err != nil {
		return nil, fmt.Errorf("topology diff query failed: %w", err)
	}

	return convertTopologyDiff(diff, req.BaseTime, req.CompareTime), nil
}

// IngestFlows 接收实时流量数据
func (s *Service) IngestFlows(ctx context.Context, req *proto.FlowIngestRequest) (*proto.FlowIngestResponse, error) {
	if req.Count == 0 || len(req.Flows) == 0 {
		return &proto.FlowIngestResponse{Accepted: 0, Rejected: 0}, nil
	}

	// 反序列化 UnifiedFlow 批次
	flows, err := deserializeFlowBatch(req.Flows)
	if err != nil {
		return nil, fmt.Errorf("flow deserialization failed: %w", err)
	}

	// 投递到增量更新器
	accepted, err := s.updater.IngestFlows(req.TenantId, flows)
	if err != nil {
		return nil, fmt.Errorf("flow ingestion failed: %w", err)
	}

	return &proto.FlowIngestResponse{
		Accepted: accepted,
		Rejected: int(req.Count) - accepted,
	}, nil
}

// GetSnapshot 获取拓扑快照
func (s *Service) GetSnapshot(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.TopologySnapshot, error) {
	graphType := req.Type
	if graphType == "" {
		graphType = "service"
	}

	snapshot, ok := s.updater.GetGraph(req.TenantId, graphType)
	if !ok {
		return &proto.TopologySnapshot{
			TenantId: req.TenantId,
			Type:     graphType,
			Nodes:    []*proto.TopologyNode{},
			Edges:    []*proto.TopologyEdge{},
		}, nil
	}

	return convertSnapshot(snapshot, req.TenantId, graphType), nil
}

// ============================================================================
// 内部方法
// ============================================================================

// queryRealtimeTopology 查询实时拓扑
func (s *Service) queryRealtimeTopology(ctx context.Context, tenantID, graphType string) (*proto.TopologyQueryResponse, error) {
	snapshot, ok := s.updater.GetGraph(tenantID, graphType)
	if !ok {
		return &proto.TopologyQueryResponse{
			Nodes: []*proto.TopologyNode{},
			Edges: []*proto.TopologyEdge{},
		}, nil
	}

	return convertSnapshotToResponse(snapshot), nil
}

// queryHistoricalTopology 查询历史拓扑
func (s *Service) queryHistoricalTopology(ctx context.Context, req *proto.TopologyQueryRequest) (*proto.TopologyQueryResponse, error) {
	g, err := s.historical.QueryTopology(ctx, req.TenantId, req.StartTime, req.EndTime, req.Type)
	if err != nil {
		return nil, fmt.Errorf("historical topology query failed: %w", err)
	}

	snapshot := g.Snapshot()
	return convertSnapshotToResponse(snapshot), nil
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s","version":"%s","uptime":%d}`,
		s.config.ServiceName, s.config.Version, time.Since(s.startTime).Seconds())
}

func (s *Service) topologyHTTPHandler(w http.ResponseWriter, r *http.Request) {
	resp, _ := s.QueryTopology(r.Context(), &proto.TopologyQueryRequest{})
	writeJSON(w, map[string]interface{}{
		"nodes": len(resp.Nodes),
		"edges": len(resp.Edges),
	})
}

func (s *Service) serviceGraphHTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.handleGraphHTTP(w, r, "service")
}

func (s *Service) processGraphHTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.handleGraphHTTP(w, r, "process")
}

func (s *Service) podGraphHTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.handleGraphHTTP(w, r, "pod")
}

func (s *Service) namespaceGraphHTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.handleGraphHTTP(w, r, "namespace")
}

func (s *Service) handleGraphHTTP(w http.ResponseWriter, r *http.Request, graphType string) {
	tenantID := r.URL.Query().Get("tenant_id")
	resp, _ := s.QueryTopology(r.Context(), &proto.TopologyQueryRequest{
		TenantId: tenantID,
		Type:     graphType,
	})
	writeJSON(w, resp)
}

func (s *Service) latencyHeatmapHTTPHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	resp, _ := s.GetLatencyHeatmap(r.Context(), &proto.TopologyQueryRequest{
		TenantId: tenantID,
	})
	writeJSON(w, resp)
}

func (s *Service) errorHeatmapHTTPHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	resp, _ := s.GetErrorHeatmap(r.Context(), &proto.TopologyQueryRequest{
		TenantId: tenantID,
	})
	writeJSON(w, resp)
}

func (s *Service) diffHTTPHandler(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	graphType := r.URL.Query().Get("type")
	resp, _ := s.GetTopologyDiff(r.Context(), &proto.TopologyDiffRequest{
		TenantId:     tenantID,
		Type:         graphType,
		BaseTime:     time.Now().Add(-10 * time.Minute).Unix(),
		CompareTime:  time.Now().Unix(),
	})
	writeJSON(w, resp)
}

// ============================================================================
// 转换函数
// ============================================================================

// convertSnapshotToResponse 将 GraphSnapshot 转换为 proto Response
func convertSnapshotToResponse(snapshot *graph.GraphSnapshot) *proto.TopologyQueryResponse {
	resp := &proto.TopologyQueryResponse{
		Nodes: make([]*proto.TopologyNode, 0, len(snapshot.Nodes)),
		Edges: make([]*proto.TopologyEdge, 0, len(snapshot.Edges)),
	}

	for _, n := range snapshot.Nodes {
		resp.Nodes = append(resp.Nodes, &proto.TopologyNode{
			Id:        string(n.ID),
			Name:      n.Name,
			Type:      n.Type,
			Namespace: n.Namespace,
			Metadata:  n.Metadata,
			BytesIn:   n.BytesIn,
			BytesOut:  n.BytesOut,
			Errors:    n.Errors,
		})
	}

	for _, e := range snapshot.Edges {
		resp.Edges = append(resp.Edges, &proto.TopologyEdge{
			Source:   string(e.Source),
			Target:   string(e.Target),
			Protocol: e.Protocol,
			Port:     e.Port,
			Bytes:    e.Bytes,
			Latency:  e.Latency,
			Errors:   e.Errors,
		})
	}

	return resp
}

// convertSnapshot 将 GraphSnapshot 转换为 proto TopologySnapshot
func convertSnapshot(snapshot *graph.GraphSnapshot, tenantID, graphType string) *proto.TopologySnapshot {
	resp := &proto.TopologySnapshot{
		Version:   snapshot.Version,
		Timestamp: snapshot.Timestamp,
		TenantId:  tenantID,
		Type:      graphType,
		Nodes:     make([]*proto.TopologyNode, 0, len(snapshot.Nodes)),
		Edges:     make([]*proto.TopologyEdge, 0, len(snapshot.Edges)),
	}

	for _, n := range snapshot.Nodes {
		resp.Nodes = append(resp.Nodes, &proto.TopologyNode{
			Id:        string(n.ID),
			Name:      n.Name,
			Type:      n.Type,
			Namespace: n.Namespace,
			Metadata:  n.Metadata,
			BytesIn:   n.BytesIn,
			BytesOut:  n.BytesOut,
			Errors:    n.Errors,
		})
	}

	for _, e := range snapshot.Edges {
		resp.Edges = append(resp.Edges, &proto.TopologyEdge{
			Source:   string(e.Source),
			Target:   string(e.Target),
			Protocol: e.Protocol,
			Port:     e.Port,
			Bytes:    e.Bytes,
			Latency:  e.Latency,
			Errors:   e.Errors,
		})
	}

	return resp
}

// convertTopologyDiff 将 graph.TopologyDiff 转换为 proto.TopologyDiffResponse
func convertTopologyDiff(diff *graph.TopologyDiff, baseTime, compareTime int64) *proto.TopologyDiffResponse {
	resp := &proto.TopologyDiffResponse{
		BaseTime:    baseTime,
		CompareTime: compareTime,
		Summary: &proto.DiffSummary{
			AddedNodes:   len(diff.AddedNodes),
			RemovedNodes: len(diff.RemovedNodes),
			AddedEdges:   len(diff.AddedEdges),
			RemovedEdges: len(diff.RemovedEdges),
			ChangedEdges: len(diff.ChangedEdges),
		},
		Diffs: make([]*proto.TopologyDiff, 0,
			len(diff.AddedNodes)+len(diff.RemovedNodes)+
				len(diff.AddedEdges)+len(diff.RemovedEdges)+
				len(diff.ChangedEdges)),
	}

	for _, n := range diff.AddedNodes {
		resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
			DiffType: "added_node",
			NodeId:   string(n.ID),
		})
	}
	for _, n := range diff.RemovedNodes {
		resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
			DiffType: "removed_node",
			NodeId:   string(n.ID),
		})
	}
	for _, e := range diff.AddedEdges {
		resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
			DiffType: "added_edge",
			Source:   string(e.Source),
			Target:   string(e.Target),
		})
	}
	for _, e := range diff.RemovedEdges {
		resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
			DiffType: "removed_edge",
			Source:   string(e.Source),
			Target:   string(e.Target),
		})
	}
	for _, c := range diff.ChangedEdges {
		if c.BytesDelta != 0 {
			resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
				DiffType: "weight_change",
				Source:   string(c.Source),
				Target:   string(c.Target),
				Field:    "bytes",
				OldValue: c.OldBytes,
				NewValue: c.NewBytes,
			})
		}
		if c.LatencyDelta != 0 {
			resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
				DiffType: "weight_change",
				Source:   string(c.Source),
				Target:   string(c.Target),
				Field:    "latency",
				OldValue: c.OldLatency,
				NewValue: c.NewLatency,
			})
		}
		if c.ErrorsDelta != 0 {
			resp.Diffs = append(resp.Diffs, &proto.TopologyDiff{
				DiffType: "weight_change",
				Source:   string(c.Source),
				Target:   string(c.Target),
				Field:    "errors",
				OldValue: c.OldErrors,
				NewValue: c.NewErrors,
			})
		}
	}

	return resp
}

// findNodeNamespace 从节点列表中查找节点的 namespace
func findNodeNamespace(nodes []*proto.TopologyNode, nodeID string) string {
	for _, n := range nodes {
		if n.Id == nodeID {
			return n.Namespace
		}
	}
	return ""
}

// deserializeFlowBatch 反序列化 UnifiedFlow 批次
func deserializeFlowBatch(data []byte) ([]*flow.UnifiedFlow, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// 尝试 JSON 格式 (兼容 HTTP API)
	var flows []*flow.UnifiedFlow
	if err := json.Unmarshal(data, &flows); err == nil {
		return flows, nil
	}

	// 二进制格式: [count:4][flow1_len:4][flow1_data][flow2_len:4][flow2_data]...
	if len(data) < 4 {
		return nil, fmt.Errorf("invalid flow batch: too short")
	}

	// 简单的二进制反序列化: 逐个 flow
	offset := 0
	for offset < len(data) {
		f := &flow.UnifiedFlow{}
		remaining := data[offset:]
		if len(remaining) < 10 {
			break // 最小 flow 大小
		}
		if err := f.Deserialize(remaining); err != nil {
			break
		}
		flows = append(flows, f)
		// 估算下一个 flow 的偏移 (简化: 使用序列化大小)
		serialized := f.Serialize()
		offset += len(serialized)
		if offset >= len(data) {
			break
		}
	}

	return flows, nil
}

// writeJSON 写 JSON 响应
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
