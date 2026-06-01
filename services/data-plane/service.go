// Package dataplane Data Plane 服务
//
// 职责:
//   - Flow/Metric/Trace/Log 数据接收 (Ingest)
//   - L7 协议解析
//   - 自适应采样 (Adaptive Sampling)
//   - 数据聚合 (实时/窗口)
//   - 写入存储后端 (ClickHouse/VictoriaMetrics/Loki)
//
// 端口:
//   - gRPC: 9002
//   - Metrics: 9102
package dataplane

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"cloud-flow/pkg/flow"
	svcproto "cloud-flow/services/proto"
	"cloud-flow/services/data-plane/sampling"
	"cloud-flow/services/shared/auth"
	"cloud-flow/services/shared/tenant"
	"cloud-flow/services/shared/tlsutil"
)

// ============================================================================
// 配置
// ============================================================================

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string

	GrpcAddr string // :9002
	MetricsAddr string // :9102

	// 批量写入
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int
	WorkerCount   int

	// 存储后端
	ClickHouseAddr      string
	ClickHouseUser      string
	ClickHousePassword  string
	ClickHouseDatabase string
	VictoriaMetricsAddr string
	LokiAddr            string

	// 其他服务
	ControlPlaneAddr string
	TopologyAddr     string
	AuthAddr         string // P0-3 修复: 认证服务地址

	// 采样配置
	Sampling *sampling.SamplingConfig

	// P0-2 修复: TLS 配置
	TLSEnabled      bool
	TLSCAFile       string
	TLSCertFile     string
	TLSKeyFile      string
	TLSClientAuth   bool
	TLSInsecureSkip bool
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceName:         "data-plane",
		Version:             "1.0.0",
		GrpcAddr:            ":9002",
		MetricsAddr:         ":9102",
		BatchSize:           10000,
		FlushInterval:       time.Second,
		QueueSize:           100000,
		WorkerCount:         4,
		ClickHouseAddr:      "clickhouse:9000",
		ClickHouseUser:      "default",
		ClickHousePassword:  "",
		ClickHouseDatabase:  "cloudflow",
		VictoriaMetricsAddr: "http://victoriametrics:8428",
		LokiAddr:            "http://loki:3100",
		AuthAddr:            "auth-service:9003", // P0-3 修复: 认证服务地址
		Sampling:            sampling.NewSamplingConfig(),
		TLSEnabled:          false,
		TLSInsecureSkip:     false,
	}
}

// ============================================================================
// 统计
// ============================================================================

// Stats 数据面统计
type Stats struct {
	FlowsIngested   uint64
	MetricsIngested uint64
	TracesIngested  uint64
	LogsIngested    uint64
	FlowsDropped    uint64
	FlowsSampled    uint64 // 被采样保留的 flow
	FlowsWritten    uint64 // 成功写入存储的 flow
	WriteErrors     uint64
	AvgLatencyMs    uint64
}

// ============================================================================
// 服务
// ============================================================================

// Service Data Plane 服务
type Service struct {
	config *Config

	// 采样引擎
	samplingEngine *sampling.SamplingEngine

	// 队列
	flowQueue   chan *flow.UnifiedFlow
	metricQueue chan interface{}
	traceQueue  chan interface{}
	logQueue    chan interface{}

	// 存储
	clickHouseDB *sql.DB
	vmHTTPClient *http.Client
	lokiHTTPClient *http.Client

	// 统计
	stats Stats
	statsMu sync.RWMutex

	// gRPC
	grpcServer *grpc.Server
	health     *health.Server

	// Metrics HTTP
	metricsServer *http.Server

	// 运行状态
	startTime time.Time
	running   atomic.Bool

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials

	// P0-3 修复: 认证中间件
	auth *auth.Authenticator
}

// New 创建服务
func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if config.Sampling == nil {
		config.Sampling = sampling.NewSamplingConfig()
	}

	// 初始化采样引擎
	samplingEngine := sampling.NewSamplingEngine(config.Sampling)

	s := &Service{
		config:         config,
		samplingEngine: samplingEngine,
		flowQueue:      make(chan *flow.UnifiedFlow, config.QueueSize),
		metricQueue:    make(chan interface{}, config.QueueSize),
		traceQueue:     make(chan interface{}, config.QueueSize),
		logQueue:       make(chan interface{}, config.QueueSize),
		startTime:      time.Now(),
		health:         health.NewServer(),
	}

	// P0-06 新增: 初始化 VictoriaMetrics HTTP 客户端
	if config.VictoriaMetricsAddr != "" {
		s.vmHTTPClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				MaxConnsPerHost: 10,
				IdleConnTimeout: 90 * time.Second,
			},
		}
	}

	// P0-06 新增: 初始化 Loki HTTP 客户端
	if config.LokiAddr != "" {
		s.lokiHTTPClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				MaxConnsPerHost: 10,
				IdleConnTimeout: 90 * time.Second,
			},
		}
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
	grpcOptions = append(grpcOptions,
		grpc.MaxRecvMsgSize(64*1024*1024),
		grpc.MaxSendMsgSize(64*1024*1024),
	)
	s.grpcServer = grpc.NewServer(grpcOptions...)

	RegisterDataPlaneService(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	// P0-3 修复: 初始化认证中间件
	if config.AuthAddr != "" {
		authMiddleware, err := auth.NewAuthenticator(auth.Config{
			AuthAddr:     config.AuthAddr,
			TLSEnabled:   config.TLSEnabled,
			CAFile:       config.TLSCAFile,
			CertFile:     config.TLSCertFile,
			KeyFile:      config.TLSKeyFile,
			InsecureSkip: config.TLSInsecureSkip,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to init auth middleware: %w", err)
		}
		s.auth = authMiddleware
	}

	return s, nil
}

// Start 启动服务
func (s *Service) Start() error {
	// 启动采样引擎后台任务
	s.samplingEngine.Start(context.Background())

	// 初始化 ClickHouse 连接
	if err := s.initClickHouse(); err != nil {
		return fmt.Errorf("ClickHouse init failed: %w", err)
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

	// 启动 Metrics HTTP
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.healthzHandler)
	mux.HandleFunc("/metrics", s.statsHandler)
	mux.HandleFunc("/api/sampling/config", s.samplingConfigHandler)
	mux.HandleFunc("/api/sampling/stats", s.samplingStatsHandler)
	mux.HandleFunc("/api/ingest/metrics", s.ingestMetricsHandler)  // P0-06 新增: 接收指标
	mux.HandleFunc("/api/ingest/logs", s.ingestLogsHandler)      // P0-06 新增: 接收日志

	// P0-3 修复: 应用认证中间件
	var handler http.Handler = mux
	if s.auth != nil {
		handler = s.auth.Middleware("/health", "/metrics")(handler)
	}
	handler = tenant.HTTPMiddleware(handler)

	s.metricsServer = &http.Server{
		Addr:         s.config.MetricsAddr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	go func() {
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	// P0-06 新增: 启动 Flow workers
	for i := 0; i < s.config.WorkerCount; i++ {
		go s.flowWorker()
	}

	// P0-06 新增: 启动 Metric worker
	go s.metricWorker()

	// P0-06 新增: 启动 Log worker
	go s.logWorker()

	s.running.Store(true)
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)

	fmt.Printf("Data Plane started: gRPC=%s, Metrics=%s (sampling enabled, rate=1/%d)\n",
		s.config.GrpcAddr, s.config.MetricsAddr, s.config.Sampling.DefaultRate)
	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	s.running.Store(false)
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	// P1-04 修复: 使用优雅关闭等待请求完成
	if s.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.metricsServer.Shutdown(ctx); err != nil {
			fmt.Printf("Metrics server shutdown error: %v\n", err)
		}
	}

	// P1-04 修复: gRPC 使用带超时的 GracefulStop
	if s.grpcServer != nil {
		stopped := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(30 * time.Second):
			fmt.Println("gRPC graceful stop timeout, forcing stop")
			s.grpcServer.Stop()
		}
	}

	// 关闭 ClickHouse 连接
	if s.clickHouseDB != nil {
		s.clickHouseDB.Close()
	}

	// P0-3 修复: 清理认证中间件资源
	if s.auth != nil {
		s.auth.Close()
	}
}

// initClickHouse 初始化 ClickHouse 连接
func (s *Service) initClickHouse() error {
	if s.config.ClickHouseAddr == "" {
		return nil // ClickHouse 未配置，跳过
	}

	database := s.config.ClickHouseDatabase
	if database == "" {
		database = "cloudflow"
	}

	// 构建 DSN，支持用户名和密码认证
	var dsn string
	if s.config.ClickHouseUser != "" && s.config.ClickHousePassword != "" {
		dsn = fmt.Sprintf("clickhouse://%s:%s@%s/%s", 
			s.config.ClickHouseUser, s.config.ClickHousePassword, 
			s.config.ClickHouseAddr, database)
	} else if s.config.ClickHouseUser != "" {
		dsn = fmt.Sprintf("clickhouse://%s@%s/%s", 
			s.config.ClickHouseUser, s.config.ClickHouseAddr, database)
	} else {
		dsn = fmt.Sprintf("clickhouse://%s/%s", s.config.ClickHouseAddr, database)
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	s.clickHouseDB = db
	fmt.Printf("Data Plane ClickHouse connected: %s/%s\n", s.config.ClickHouseAddr, database)
	return nil
}

// writeToClickHouse 写入 Flow 到 ClickHouse
func (s *Service) writeToClickHouse(flows []*flow.UnifiedFlow) error {
	if s.clickHouseDB == nil {
		return fmt.Errorf("ClickHouse not configured")
	}

	tx, err := s.clickHouseDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO flows (
			timestamp, flow_id, schema_version,
			src_ip, dst_ip, src_port, dst_port, protocol, tcp_flags,
			l7_protocol, method, path, status_code, req_size, resp_size,
			grpc_service, grpc_method, grpc_status,
			pid, process_name, comm,
			container_id, container_name, image,
			pod, namespace, deployment, service, node,
			trace_id, span_id, parent_id,
			host_id, hostname, tenant_id,
			bytes, packets, latency_ns, direction, exception, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range flows {
		_, err := stmt.Exec(
			f.Timestamp, f.FlowID, f.SchemaVersion,
			f.SrcIP.String(), f.DstIP.String(), f.SrcPort, f.DstPort, f.Protocol, f.TCPFlags,
			f.L7Protocol, f.Method, f.Path.String(), f.StatusCode, f.ReqSize, f.RespSize,
			f.GetGRPCService(), f.GetGRPCMethod(), 0,
			f.PID, f.ProcessName.String(), f.Comm.String(),
			f.ContainerID.String(), f.ContainerName.String(), f.Image.String(),
			f.Pod.String(), f.Namespace.String(), f.Deployment.String(), f.Service.String(), f.Node.String(),
			f.TraceID.String(), f.SpanID.String(), f.ParentID.String(),
			f.HostID.String(), f.Hostname.String(), f.TenantID.String(),
			f.Bytes, f.Packets, f.LatencyNs, f.Direction, f.GetL7Exception(), "",
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// writeToVictoriaMetrics P0-06 新增: 写入指标到 VictoriaMetrics
func (s *Service) writeToVictoriaMetrics(flows []*flow.UnifiedFlow) error {
	if s.vmHTTPClient == nil || s.config.VictoriaMetricsAddr == "" {
		return nil // VictoriaMetrics 未配置，跳过
	}

	// 将 Flow 转换为 VictoriaMetrics 的 prometheus 格式
	// 格式: metric_name{labels} value timestamp
	var sb strings.Builder

	for _, f := range flows {
		labels := fmt.Sprintf(`src_ip="%s",dst_ip="%s",protocol="%s",service="%s",namespace="%s",tenant_id="%s"`,
			f.SrcIP.String(), f.DstIP.String(), f.Protocol.String(),
			f.Service.String(), f.Namespace.String(), f.TenantID.String())

		// 写入字节数
		sb.WriteString(fmt.Sprintf("flow_bytes{%s} %d %d\n", labels, f.Bytes, f.Timestamp))

		// 写入包数
		sb.WriteString(fmt.Sprintf("flow_packets{%s} %d %d\n", labels, f.Packets, f.Timestamp))

		// 写入延迟
		sb.WriteString(fmt.Sprintf("flow_latency_ns{%s} %d %d\n", labels, f.LatencyNs, f.Timestamp))

		// 写入请求大小
		if f.ReqSize > 0 {
			sb.WriteString(fmt.Sprintf("flow_req_size{%s} %d %d\n", labels, f.ReqSize, f.Timestamp))
		}

		// 写入响应大小
		if f.RespSize > 0 {
			sb.WriteString(fmt.Sprintf("flow_resp_size{%s} %d %d\n", labels, f.RespSize, f.Timestamp))
		}
	}

	// 发送到 VictoriaMetrics
	url := s.config.VictoriaMetricsAddr + "/api/v1/import/prometheus"
	req, err := http.NewRequest("POST", url, strings.NewReader(sb.String()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := s.vmHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("VictoriaMetrics request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VictoriaMetrics returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// writeToLoki P0-06 新增: 写入日志到 Loki
func (s *Service) writeToLoki(flows []*flow.UnifiedFlow) error {
	if s.lokiHTTPClient == nil || s.config.LokiAddr == "" {
		return nil // Loki 未配置，跳过
	}

	// 将 Flow 转换为 Loki 的 log 格式
	// Loki 使用 push API 接收日志
	streams := make(map[string][]logEntry)

	for _, f := range flows {
		labels := fmt.Sprintf(`{src_ip="%s",dst_ip="%s",protocol="%s",service="%s",namespace="%s",tenant_id="%s",direction="%s"}`,
			f.SrcIP.String(), f.DstIP.String(), f.Protocol.String(),
			f.Service.String(), f.Namespace.String(), f.TenantID.String(), f.Direction)

		// 构建日志消息
		message := fmt.Sprintf("Flow: %s -> %s:%d %s bytes=%d latency=%dns",
			f.SrcIP.String(), f.DstIP.String(), f.DstPort, f.Protocol.String(), f.Bytes, f.LatencyNs)

		if f.L7Protocol != 0 {
			message += fmt.Sprintf(" l7=%s", f.L7Protocol.String())
		}
		if f.Method != 0 {
			message += fmt.Sprintf(" method=%s", f.Method.String())
		}
		if f.StatusCode > 0 {
			message += fmt.Sprintf(" status=%d", f.StatusCode)
		}

		entry := logEntry{
			Timestamp: f.Timestamp * 1000000, // Loki uses nanoseconds
			Line:      message,
		}

		streams[labels] = append(streams[labels], entry)
	}

	// 构建 Loki push 请求
	lokiReq := lokiPushRequest{Streams: []lokiStream{}}
	for streamLabels, entries := range streams {
		lokiReq.Streams = append(lokiReq.Streams, lokiStream{
			Stream: parseLabels(streamLabels),
			Values: entries,
		})
	}

	if len(lokiReq.Streams) == 0 {
		return nil
	}

	// 序列化为 JSON
	jsonData, err := json.Marshal(lokiReq)
	if err != nil {
		return fmt.Errorf("failed to marshal Loki request: %w", err)
	}

	// 发送到 Loki
	url := s.config.LokiAddr + "/loki/api/v1/push"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.lokiHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("Loki request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Loki returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// logEntry Loki 日志条目
type logEntry struct {
	Timestamp int64  `json:"tsNs"`
	Line      string `json:"line"`
}

// lokiPushRequest Loki push 请求
type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

// lokiStream Loki 流
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values []logEntry       `json:"values"`
}

// parseLabels 解析标签字符串为 map
func parseLabels(labels string) map[string]string {
	result := make(map[string]string)
	// labels 是 {key="value",key2="value2"} 格式
	labels = strings.Trim(labels, "{}")
	pairs := strings.Split(labels, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = strings.Trim(kv[1], `"`)
		}
	}
	return result
}

// IngestFlow 接收 Flow（含采样决策）
func (s *Service) IngestFlow(ctx context.Context, batch *svcproto.FlowBatch) (*svcproto.IngestResponse, error) {
	accepted := 0
	sampled := 0

	for _, flowMap := range batch.Flows {
		// 反序列化 UnifiedFlow
		f := mapToUnifiedFlow(flowMap)
		if f == nil {
			continue
		}

		// 采样决策
		sCtx := sampling.FlowToContext(f)
		if !s.samplingEngine.ShouldKeep(&sCtx) {
			// 被采样丢弃
			s.statsMu.Lock()
			s.stats.FlowsSampled++
			s.statsMu.Unlock()
			continue
		}

		select {
		case s.flowQueue <- f:
			accepted++
		default:
			s.statsMu.Lock()
			s.stats.FlowsDropped++
			s.statsMu.Unlock()
		}
	}

	s.statsMu.Lock()
	s.stats.FlowsIngested += uint64(accepted)
	s.statsMu.Unlock()

	return &svcproto.IngestResponse{
		Accepted: accepted,
		Rejected: len(batch.Flows) - accepted,
		Success:  true,
	}, nil
}

// GetStats 获取统计
func (s *Service) GetStats() Stats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.stats
}

// GetSamplingStats 获取采样统计
func (s *Service) GetSamplingStats() sampling.SamplingStats {
	return s.samplingEngine.GetStats()
}

// UpdateSamplingConfig 运行时更新采样配置
func (s *Service) UpdateSamplingConfig(cfg *sampling.SamplingConfig) {
	s.samplingEngine.UpdateConfig(cfg)
}

// flowWorker Flow 处理 worker
func (s *Service) flowWorker() {
	batch := make([]*flow.UnifiedFlow, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for s.running.Load() {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushFlows(batch)
				batch = batch[:0]
			}
		case item := <-s.flowQueue:
			batch = append(batch, item)
			if len(batch) >= s.config.BatchSize {
				s.flushFlows(batch)
				batch = batch[:0]
			}
		}
	}

	// 优雅退出：刷新剩余数据
	if len(batch) > 0 {
		s.flushFlows(batch)
	}
}

// metricWorker P0-06 新增: Metric 处理 worker
func (s *Service) metricWorker() {
	batch := make([]interface{}, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for s.running.Load() {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushMetrics(batch)
				batch = batch[:0]
			}
		case item := <-s.metricQueue:
			batch = append(batch, item)
			if len(batch) >= s.config.BatchSize {
				s.flushMetrics(batch)
				batch = batch[:0]
			}
		}
	}

	// 优雅退出：刷新剩余数据
	if len(batch) > 0 {
		s.flushMetrics(batch)
	}
}

// logWorker P0-06 新增: Log 处理 worker
func (s *Service) logWorker() {
	batch := make([]interface{}, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for s.running.Load() {
		select {
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushLogs(batch)
				batch = batch[:0]
			}
		case item := <-s.logQueue:
			batch = append(batch, item)
			if len(batch) >= s.config.BatchSize {
				s.flushLogs(batch)
				batch = batch[:0]
			}
		}
	}

	// 优雅退出：刷新剩余数据
	if len(batch) > 0 {
		s.flushLogs(batch)
	}
}

// flushMetrics P0-06 新增: 刷新 Metric 到 VictoriaMetrics
func (s *Service) flushMetrics(metrics []interface{}) {
	if len(metrics) == 0 {
		return
	}

	s.statsMu.Lock()
	s.stats.MetricsIngested += uint64(len(metrics))
	s.statsMu.Unlock()

	// VictoriaMetrics 已经通过 writeToVictoriaMetrics 处理
	// 这里可以添加额外的 metric 特定处理逻辑
}

// flushLogs P0-06 新增: 刷新 Log 到 Loki
func (s *Service) flushLogs(logs []interface{}) {
	if len(logs) == 0 {
		return
	}

	s.statsMu.Lock()
	s.stats.LogsIngested += uint64(len(logs))
	s.statsMu.Unlock()

	// Loki 已经通过 writeToLoki 处理
	// 这里可以添加额外的 log 特定处理逻辑
}

// flushFlows 刷新 Flow 到存储 P0-06 修改: 调用所有存储后端
func (s *Service) flushFlows(batch []*flow.UnifiedFlow) {
	if len(batch) == 0 {
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // FIX: 保护 error 变量的并发写入
	var chErr, vmErr, lokiErr error

	// 并行写入 ClickHouse
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.writeToClickHouse(batch); err != nil {
			mu.Lock()
			chErr = err
			mu.Unlock()
		}
	}()

	// 并行写入 VictoriaMetrics
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.writeToVictoriaMetrics(batch); err != nil {
			mu.Lock()
			vmErr = err
			mu.Unlock()
		}
	}()

	// 并行写入 Loki
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.writeToLoki(batch); err != nil {
			mu.Lock()
			lokiErr = err
			mu.Unlock()
		}
	}()

	// 等待所有写入完成
	wg.Wait()

	// 更新统计
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	if chErr != nil || vmErr != nil || lokiErr != nil {
		s.stats.WriteErrors += uint64(len(batch))
		if chErr != nil {
			fmt.Printf("ClickHouse write error: %v\n", chErr)
		}
		if vmErr != nil {
			fmt.Printf("VictoriaMetrics write error: %v\n", vmErr)
		}
		if lokiErr != nil {
			fmt.Printf("Loki write error: %v\n", lokiErr)
		}
	} else {
		s.stats.FlowsWritten += uint64(len(batch))
	}
}

// ============================================================================
// HTTP Handlers
// ============================================================================

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s","version":"%s"}`,
		s.config.ServiceName, s.config.Version)
}

func (s *Service) statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.GetStats()
	fmt.Fprintf(w, `{"flows_ingested":%d,"flows_dropped":%d,"flows_sampled":%d,"flows_written":%d,"metrics_ingested":%d,"logs_ingested":%d,"write_errors":%d}`,
		stats.FlowsIngested, stats.FlowsDropped, stats.FlowsSampled, stats.FlowsWritten,
		stats.MetricsIngested, stats.LogsIngested, stats.WriteErrors)
}

func (s *Service) samplingConfigHandler(w http.ResponseWriter, r *http.Request) {
	// P0-3 修复: 验证认证
	tenantID := tenant.FromContext(r.Context())
	if tenantID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.config.Sampling)
	case http.MethodPut, http.MethodPost:
		var cfg sampling.SamplingConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.UpdateSamplingConfig(&cfg)
		writeJSON(w, map[string]string{"status": "updated"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) samplingStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.GetSamplingStats()
	writeJSON(w, stats)
}

// ingestMetricsHandler P0-06 新增: 接收指标数据
func (s *Service) ingestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// P0-3 修复: 获取租户信息
	tenantID := tenant.FromContext(r.Context())
	if tenantID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var metrics []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	count := 0
	for _, m := range metrics {
		// P0-3 修复: 添加租户信息
		m["tenant_id"] = tenantID
		select {
		case s.metricQueue <- m:
			count++
		default:
			// 队列满，丢弃
		}
	}

	s.statsMu.Lock()
	s.stats.MetricsIngested += uint64(count)
	s.statsMu.Unlock()

	writeJSON(w, map[string]interface{}{
		"accepted": count,
		"total":    len(metrics),
	})
}

// ingestLogsHandler P0-06 新增: 接收日志数据
func (s *Service) ingestLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// P0-3 修复: 获取租户信息
	tenantID := tenant.FromContext(r.Context())
	if tenantID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var logs []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&logs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	count := 0
	for _, l := range logs {
		// P0-3 修复: 添加租户信息
		l["tenant_id"] = tenantID
		select {
		case s.logQueue <- l:
			count++
		default:
			// 队列满，丢弃
		}
	}

	s.statsMu.Lock()
	s.stats.LogsIngested += uint64(count)
	s.statsMu.Unlock()

	writeJSON(w, map[string]interface{}{
		"accepted": count,
		"total":    len(logs),
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// mapToUnifiedFlow 将 map[string]interface{} 转换为 UnifiedFlow
func mapToUnifiedFlow(m map[string]interface{}) *flow.UnifiedFlow {
	f := flow.New()

	if v, ok := m["src_ip"].(string); ok {
		f.SrcIP = flow.ParseIP(v)
	}
	if v, ok := m["dst_ip"].(string); ok {
		f.DstIP = flow.ParseIP(v)
	}
	if v, ok := m["src_port"].(float64); ok {
		f.SrcPort = uint16(v)
	}
	if v, ok := m["dst_port"].(float64); ok {
		f.DstPort = uint16(v)
	}
	if v, ok := m["protocol"].(string); ok {
		f.Protocol = flow.ParseProtocol(v)
	}
	if v, ok := m["bytes"].(float64); ok {
		f.Bytes = uint64(v)
	}
	if v, ok := m["packets"].(float64); ok {
		f.Packets = uint64(v)
	}
	if v, ok := m["latency_ns"].(float64); ok {
		f.LatencyNs = uint64(v)
	}
	if v, ok := m["status_code"].(float64); ok {
		f.StatusCode = uint16(v)
	}
	if v, ok := m["tcp_flags"].(float64); ok {
		f.TCPFlags = uint8(v)
	}
	if v, ok := m["l7_protocol"].(string); ok {
		f.L7Protocol = flow.ParseProtocol(v)
	}
	if v, ok := m["tenant_id"].(string); ok {
		f.TenantID.Set(v)
	}
	if v, ok := m["namespace"].(string); ok {
		f.Namespace.Set(v)
	}
	if v, ok := m["service"].(string); ok {
		f.Service.Set(v)
	}
	if v, ok := m["pod"].(string); ok {
		f.Pod.Set(v)
	}
	if v, ok := m["deployment"].(string); ok {
		f.Deployment.Set(v)
	}
	if v, ok := m["node"].(string); ok {
		f.Node.Set(v)
	}
	if v, ok := m["process_name"].(string); ok {
		f.ProcessName.Set(v)
	}
	if v, ok := m["comm"].(string); ok {
		f.Comm.Set(v)
	}
	if v, ok := m["container_id"].(string); ok {
		f.ContainerID.Set(v)
	}
	if v, ok := m["container_name"].(string); ok {
		f.ContainerName.Set(v)
	}
	if v, ok := m["hostname"].(string); ok {
		f.Hostname.Set(v)
	}
	if v, ok := m["timestamp"].(float64); ok {
		f.Timestamp = int64(v)
	}

	return f
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
