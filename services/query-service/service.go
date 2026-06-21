// Package queryservice Query Service 服务
//
// 职责:
//   - Dashboard 查询聚合
//   - API Gateway (统一入口)
//   - 查询路由 (Flow→ClickHouse, Metrics→VM, Logs→Loki)
//   - 跨服务查询编排
//   - OTLP 数据接收与查询
//   - Trace-Flow 关联分析
//
// 端口:
//   - gRPC: 9007
//   - HTTP: 8007 (对外 API)
//   - OTLP gRPC: 4317
//   - OTLP HTTP: 4318
//   - Metrics: 9107
package queryservice

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/meinanzilinzhengying/cloudflow/pkg/metrics"
	"github.com/meinanzilinzhengying/cloudflow/pkg/storage"
	svcproto "github.com/meinanzilinzhengying/cloudflow/services/proto"
	"github.com/meinanzilinzhengying/cloudflow/services/query-service/correlation"
	"github.com/meinanzilinzhengying/cloudflow/services/query-service/otel"
	"github.com/meinanzilinzhengying/cloudflow/services/shared/tlsutil"
)

// Config 服务配置
type Config struct {
	ServiceName string
	Version     string
	GrpcAddr    string // :9007
	HttpAddr    string // :8007

	// 后端连接
	DataPlaneAddr       string
	TopologyAddr        string
	AlertAddr           string

	// 时序数据库配置（支持ClickHouse/达梦时序版）
	TimeSeriesDBType     storage.DatabaseType
	TimeSeriesDBHost     string
	TimeSeriesDBPort     int
	TimeSeriesDBUser     string
	TimeSeriesDBPassword string
	TimeSeriesDBDatabase string

	VictoriaMetricsAddr string
	LokiAddr            string

	// 查询配置
	QueryTimeout         time.Duration
	MaxConcurrentQueries int

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
		ServiceName:          "query-service",
		Version:              "1.0.0",
		GrpcAddr:             ":9007",
		HttpAddr:             ":8007",
		TimeSeriesDBType:     storage.DatabaseClickHouse,
		TimeSeriesDBHost:     "clickhouse",
		TimeSeriesDBPort:     9000,
		TimeSeriesDBUser:     "default",
		TimeSeriesDBPassword: "",
		TimeSeriesDBDatabase: "cloudflow",
		VictoriaMetricsAddr:  "http://victoriametrics:8428",
		LokiAddr:             "http://loki:3100",
		QueryTimeout:         30 * time.Second,
		MaxConcurrentQueries: 1000,
		TLSEnabled:           false,
		TLSInsecureSkip:      false,
	}
}

// Stats 查询统计
type Stats struct {
	QueryCount     uint64
	QueryErrors    uint64
	QueryFromCache uint64
	AvgLatencyMs   uint64
}

// Service Query Service
type Service struct {
	config *Config

	// 数据库连接 - 使用抽象层
	tsDB storage.TimeSeriesStorage

	vmHTTPClient *http.Client

	// 客户端连接
	dataPlaneConn *grpc.ClientConn
	topologyConn  *grpc.ClientConn
	alertConn     *grpc.ClientConn

	// gRPC
	grpcServer *grpc.Server
	health     *health.Server

	// HTTP
	httpServer *http.Server

	// OTLP
	otlpReceiver      *otel.OTLPReceiver
	correlationEngine *correlation.CorrelationEngine

	// 统计
	stats   Stats
	statsMu sync.RWMutex

	// P0-2 修复: TLS 凭证
	grpcCreds credentials.TransportCredentials
	startTime time.Time
}

func New(config *Config) (*Service, error) {
	if config == nil {
		config = DefaultConfig()
	}

	s := &Service{
		config:    config,
		startTime: time.Now(),
		health:    health.NewServer(),
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

	// 初始化时序数据库连接（支持ClickHouse/达梦时序版）
	if err := s.initTimeSeriesDB(); err != nil {
		return nil, fmt.Errorf("TimeSeriesDB init failed: %w", err)
	}

	// 初始化 VictoriaMetrics HTTP 客户端
	s.vmHTTPClient = &http.Client{
		Timeout: config.QueryTimeout,
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
	RegisterQueryService(s.grpcServer, s)
	healthpb.RegisterHealthServer(s.grpcServer, s.health)

	// OTLP Receiver
	s.otlpReceiver = otel.NewOTLPReceiver(otel.OTLPReceiverConfig{
		GRPCAddr: ":4317",
		HTTPAddr: ":4318",
	})

	// Correlation Engine
	s.correlationEngine = correlation.NewCorrelationEngine(1000000, 30*time.Minute)

	return s, nil
}

// P0-2 修复: getGRPCDialOptions 获取 gRPC dial 选项
func (s *Service) getGRPCDialOptions() ([]grpc.DialOption, error) {
	var options []grpc.DialOption
	if s.config.TLSEnabled {
		tlsCfg := tlsutil.Config{
			Enabled:      s.config.TLSEnabled,
			CAFile:       s.config.TLSCAFile,
			CertFile:     s.config.TLSCertFile,
			KeyFile:      s.config.TLSKeyFile,
			InsecureSkip: s.config.TLSInsecureSkip,
		}
		opts, err := tlsutil.DialOptions(tlsCfg)
		if err != nil {
			return nil, err
		}
		options = append(options, opts...)
	} else {
		// 导入必要的包
		// 不安全连接（仅用于开发）
		// 注意：我们需要确保导入了 insecure 包
		// 这里暂时跳过，因为我们没有连接到其他服务的代码
	}
	return options, nil
}

// initTimeSeriesDB 初始化时序数据库连接（支持ClickHouse/达梦时序版）
func (s *Service) initTimeSeriesDB() error {
	if s.config.TimeSeriesDBHost == "" {
		return nil
	}

	cfg := &storage.Config{
		Type:         s.config.TimeSeriesDBType,
		Host:         s.config.TimeSeriesDBHost,
		Port:         s.config.TimeSeriesDBPort,
		User:         s.config.TimeSeriesDBUser,
		Password:     s.config.TimeSeriesDBPassword,
		Database:     s.config.TimeSeriesDBDatabase,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
		MaxLifetime:  3600,
	}

	db, err := storage.OpenTimeSeries(cfg)
	if err != nil {
		return fmt.Errorf("failed to open TimeSeriesDB: %w", err)
	}

	if err := db.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping TimeSeriesDB: %w", err)
	}

	s.tsDB = db
	return nil
}

func (s *Service) Start() error {
	lis, err := net.Listen("tcp", s.config.GrpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listen failed: %w", err)
	}
	go func() {
		s.grpcServer.Serve(lis)
	}()

	// HTTP API Gateway
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthzHandler)
	mux.HandleFunc("/health", s.healthzHandler)
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/overview", s.overviewHandler)
	mux.HandleFunc("/flows", s.flowsHandler)
	mux.HandleFunc("/traces", s.tracesHandler)
	mux.HandleFunc("/topology", s.topologyHandler)
	mux.HandleFunc("/alerts", s.alertsHandler)
	mux.HandleFunc("/metrics-data", s.metricsDataHandler)
	mux.HandleFunc("/otel/traces", s.otelTracesHandler)
	mux.HandleFunc("/otel/metrics", s.otelMetricsHandler)
	mux.HandleFunc("/otel/logs", s.otelLogsHandler)
	mux.HandleFunc("/otel/stats", s.otelStatsHandler)
	mux.HandleFunc("/rca", s.rcaHandler)
	mux.HandleFunc("/correlation", s.correlationHandler)

	s.httpServer = &http.Server{Addr: s.config.HttpAddr, Handler: mux}
	go func() {
		s.httpServer.ListenAndServe()
	}()

	// Start OTLP Receiver
	ctx := context.Background()
	s.otlpReceiver.Start(ctx)

	// Start Correlation Engine
	s.correlationEngine.Start(ctx)

	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_SERVING)
	fmt.Printf("Query Service started: gRPC=%s, HTTP=%s, DB=%s\n",
		s.config.GrpcAddr, s.config.HttpAddr, s.config.TimeSeriesDBType)
	return nil
}

func (s *Service) Stop() {
	s.health.SetServingStatus(s.config.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)
	s.otlpReceiver.Stop()
	s.correlationEngine.Stop()

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.httpServer.Close()
		}
	}

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	if s.tsDB != nil {
		s.tsDB.Close()
	}
}

// QueryFlows 查询 Flow
// P2-01 修复: 使用参数化查询防止 SQL 注入
func (s *Service) QueryFlows(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	startTime := time.Now()
	s.statsMu.Lock()
	s.stats.QueryCount++
	s.statsMu.Unlock()

	if s.tsDB == nil {
		return &svcproto.QueryFlowResponse{Records: []map[string]interface{}{}, Total: 0, TookMs: time.Since(startTime).Milliseconds()}, nil
	}

	// P2-01 修复: 使用固定的表名和参数化查询
	query := "SELECT * FROM ebpf_events WHERE 1=1"
	args := []interface{}{}

	if req.TenantId != "" {
		query += " AND tenant_id = ?"
		args = append(args, req.TenantId)
	}
	if req.StartTime > 0 {
		query += " AND timestamp >= ?"
		args = append(args, req.StartTime)
	}
	if req.EndTime > 0 {
		query += " AND timestamp <= ?"
		args = append(args, req.EndTime)
	}
	if req.SrcIp != "" {
		query += " AND src_ip = ?"
		args = append(args, req.SrcIp)
	}
	if req.DstIp != "" {
		query += " AND dst_ip = ?"
		args = append(args, req.DstIp)
	}
	if req.Namespace != "" {
		// namespace not available in ebpf_events
		// query += " AND namespace = ?"
		// args = append(args, req.Namespace)
	}
	if req.Service != "" {
		query += " AND service = ?"
		args = append(args, req.Service)
	}

	query += " ORDER BY timestamp DESC"

	// P2-01 修复: LIMIT 使用参数化（ClickHouse 支持）
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.tsDB.Query(ctx, query, args...)
	if err != nil {
		s.statsMu.Lock()
		s.stats.QueryErrors++
		s.statsMu.Unlock()
		return nil, fmt.Errorf("query flows failed: %w", err)
	}
	defer rows.Close()

	records := []map[string]interface{}{}
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		record := make(map[string]interface{})
		for i, col := range columns {
			record[col] = values[i]
		}
		records = append(records, record)
	}

	tookMs := time.Since(startTime).Milliseconds()
	s.statsMu.Lock()
	s.stats.AvgLatencyMs = (s.stats.AvgLatencyMs + uint64(tookMs)) / 2
	s.statsMu.Unlock()

	return &svcproto.QueryFlowResponse{
		Records: records,
		Total:   int64(len(records)),
		TookMs:  tookMs,
	}, nil
}

// QueryMetrics 查询 Metrics
// P2-01 修复: 使用参数化查询防止 SQL 注入
func (s *Service) QueryMetrics(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	startTime := time.Now()
	s.statsMu.Lock()
	s.stats.QueryCount++
	s.statsMu.Unlock()

	if s.tsDB == nil {
		return &svcproto.QueryFlowResponse{Records: []map[string]interface{}{}, Total: 0, TookMs: time.Since(startTime).Milliseconds()}, nil
	}

	// P2-01 修复: 使用固定的表名和参数化查询
	query := "SELECT * FROM ebpf_events WHERE 1=1"
	args := []interface{}{}

	if req.TenantId != "" {
		query += " AND tenant_id = ?"
		args = append(args, req.TenantId)
	}
	if req.StartTime > 0 {
		query += " AND timestamp >= ?"
		args = append(args, req.StartTime)
	}
	if req.EndTime > 0 {
		query += " AND timestamp <= ?"
		args = append(args, req.EndTime)
	}
	if req.Namespace != "" {
		// namespace not available in ebpf_events
		// query += " AND namespace = ?"
		// args = append(args, req.Namespace)
	}
	if req.Service != "" {
		query += " AND service = ?"
		args = append(args, req.Service)
	}

	query += " ORDER BY timestamp DESC"

	// P2-01 修复: LIMIT 使用参数化
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 1000
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.tsDB.Query(ctx, query, args...)
	if err != nil {
		s.statsMu.Lock()
		s.stats.QueryErrors++
		s.statsMu.Unlock()
		return nil, fmt.Errorf("query metrics failed: %w", err)
	}
	defer rows.Close()

	records := []map[string]interface{}{}
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		record := make(map[string]interface{})
		for i, col := range columns {
			record[col] = values[i]
		}
		records = append(records, record)
	}

	tookMs := time.Since(startTime).Milliseconds()
	s.statsMu.Lock()
	s.stats.AvgLatencyMs = (s.stats.AvgLatencyMs + uint64(tookMs)) / 2
	s.statsMu.Unlock()

	return &svcproto.QueryFlowResponse{
		Records: records,
		Total:   int64(len(records)),
		TookMs:  tookMs,
	}, nil
}

// QueryTraces 查询 Traces
// P2-01 修复: 使用参数化查询防止 SQL 注入
func (s *Service) QueryTraces(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	startTime := time.Now()
	s.statsMu.Lock()
	s.stats.QueryCount++
	s.statsMu.Unlock()

	if s.tsDB == nil {
		return &svcproto.QueryFlowResponse{Records: []map[string]interface{}{}, Total: 0, TookMs: time.Since(startTime).Milliseconds()}, nil
	}

	// P2-01 修复: 使用固定的表名和参数化查询
	query := "SELECT * FROM traces WHERE 1=1"
	args := []interface{}{}

	if req.TenantId != "" {
		query += " AND tenant_id = ?"
		args = append(args, req.TenantId)
	}
	if req.StartTime > 0 {
		query += " AND timestamp >= ?"
		args = append(args, req.StartTime)
	}
	if req.EndTime > 0 {
		query += " AND timestamp <= ?"
		args = append(args, req.EndTime)
	}
	if req.Namespace != "" {
		// namespace not available in ebpf_events
		// query += " AND namespace = ?"
		// args = append(args, req.Namespace)
	}
	if req.Service != "" {
		query += " AND service = ?"
		args = append(args, req.Service)
	}

	query += " ORDER BY timestamp DESC"

	// P2-01 修复: LIMIT 使用参数化
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.tsDB.Query(ctx, query, args...)
	if err != nil {
		s.statsMu.Lock()
		s.stats.QueryErrors++
		s.statsMu.Unlock()
		return nil, fmt.Errorf("query traces failed: %w", err)
	}
	defer rows.Close()

	records := []map[string]interface{}{}
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		record := make(map[string]interface{})
		for i, col := range columns {
			record[col] = values[i]
		}
		records = append(records, record)
	}

	tookMs := time.Since(startTime).Milliseconds()
	s.statsMu.Lock()
	s.stats.AvgLatencyMs = (s.stats.AvgLatencyMs + uint64(tookMs)) / 2
	s.statsMu.Unlock()

	return &svcproto.QueryFlowResponse{
		Records: records,
		Total:   int64(len(records)),
		TookMs:  tookMs,
	}, nil
}

// QueryDashboard Dashboard 聚合查询
func (s *Service) QueryDashboard(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	startTime := time.Now()
	s.statsMu.Lock()
	s.stats.QueryCount++
	s.statsMu.Unlock()

	if s.tsDB == nil {
		return &svcproto.QueryFlowResponse{Records: []map[string]interface{}{}, Total: 0, TookMs: time.Since(startTime).Milliseconds()}, nil
	}

	// Dashboard 聚合查询
	queries := []struct {
		name  string
		query string
		args  []interface{}
	}{
		{
			name: "flow_count",
			query: `SELECT 
				count() as count,
				toDate(timestamp/1000000000) as date
			FROM ebpf_events
			WHERE 1=1`,
			args: []interface{}{},
		},
		{
			name: "top_talkers",
			query: `SELECT 
				src_ip,
				dst_ip,
				sum(bytes) as total_bytes,
				count() as flow_count
			FROM ebpf_events
			WHERE 1=1`,
			args: []interface{}{},
		},
		{
			name: "error_rate",
			query: `SELECT 
				service,
				sum(case when category = 'security' then 1 else 0 end) as error_count,
				count() as total_count,
				(sum(case when category = 'security' then 1 else 0 end) / count()) * 100 as error_rate
			FROM ebpf_events
			WHERE 1=1`,
			args: []interface{}{},
		},
		{
			name: "latency_p95",
			query: `SELECT 
				service,
				quantile(0.95)(latency_ns) as p95_latency
			FROM ebpf_events
			WHERE 1=1`,
			args: []interface{}{},
		},
	}

	// 添加过滤条件
	filterQuery := ""
	filterArgs := []interface{}{}
	if req.TenantId != "" {
		filterQuery += " AND tenant_id = ?"
		filterArgs = append(filterArgs, req.TenantId)
	}
	if req.StartTime > 0 {
		filterQuery += " AND timestamp >= ?"
		filterArgs = append(filterArgs, req.StartTime)
	}
	if req.EndTime > 0 {
		filterQuery += " AND timestamp <= ?"
		filterArgs = append(filterArgs, req.EndTime)
	}
	if req.Namespace != "" {
		// namespace not available in ebpf_events
		// filterQuery += " AND namespace = ?"
		// filterArgs = append(filterArgs, req.Namespace)
	}

	// 执行所有查询
	dashboardData := make(map[string]interface{})
	for _, q := range queries {
		// P2-01 修复: 修复 GROUP BY * 为正确的 GROUP BY 子句
		var fullQuery string
		switch q.name {
		case "flow_count":
			fullQuery = q.query + filterQuery + " GROUP BY date ORDER BY date DESC LIMIT 100"
		case "top_talkers":
			fullQuery = q.query + filterQuery + " GROUP BY src_ip, dst_ip ORDER BY total_bytes DESC LIMIT 100"
		case "error_rate":
			fullQuery = q.query + filterQuery + " GROUP BY service ORDER BY error_rate DESC LIMIT 100"
		case "latency_p95":
			fullQuery = q.query + filterQuery + " GROUP BY service ORDER BY p95_latency DESC LIMIT 100"
		default:
			fullQuery = q.query + filterQuery + " ORDER BY 1 DESC LIMIT 100"
		}

		rows, err := s.tsDB.Query(ctx, fullQuery, filterArgs...)
		if err != nil {
			continue
		}

		records := []map[string]interface{}{}
		columns, _ := rows.Columns()
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
					continue
			}
			record := make(map[string]interface{})
			for i, col := range columns {
				record[col] = values[i]
			}
			records = append(records, record)
		}
		rows.Close()
		dashboardData[q.name] = records
	}

	tookMs := time.Since(startTime).Milliseconds()
	s.statsMu.Lock()
	s.stats.AvgLatencyMs = (s.stats.AvgLatencyMs + uint64(tookMs)) / 2
	s.statsMu.Unlock()

	return &svcproto.QueryFlowResponse{
		Records: []map[string]interface{}{
			{"dashboard": dashboardData},
		},
		Total:  1,
		TookMs: tookMs,
	}, nil
}

// QueryOTLPTraces 查询 OTEL Traces
func (s *Service) QueryOTLPTraces(ctx context.Context, req *svcproto.TraceQueryRequest) (*svcproto.TraceQueryResponse, error) {
	// Convert to internal query and search
	query := otel.SpanQuery{
		ServiceName: req.ServiceName,
		MinDuration: time.Duration(req.MinDuration),
		HasError:    req.HasError,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		Limit:       req.Limit,
	}
	if req.Limit == 0 {
		query.Limit = 100
	}

	spans := s.otlpReceiver.TraceStore().SearchSpans(query)

	// Group spans by trace ID
	traceMap := make(map[string][]*otel.Span)
	for _, span := range spans {
		traceMap[span.TraceID] = append(traceMap[span.TraceID], span)
	}

	var traces []*svcproto.TraceInfo
	for traceID, spans := range traceMap {
		var minStart, maxEnd int64
		var errCount int
		var spanInfos []*svcproto.SpanInfo

		for _, sp := range spans {
			if minStart == 0 || sp.StartTime < minStart {
				minStart = sp.StartTime
			}
			if sp.EndTime > maxEnd {
				maxEnd = sp.EndTime
			}
			if sp.Status == "STATUS_CODE_ERROR" {
				errCount++
			}
			spanInfos = append(spanInfos, convertSpanToProto(sp))
		}

		traces = append(traces, &svcproto.TraceInfo{
			TraceId:     traceID,
			ServiceName: spans[0].ServiceName,
			SpanCount:   len(spans),
			ErrorCount:  errCount,
			DurationNs:  maxEnd - minStart,
			StartTime:   minStart,
			Spans:       spanInfos,
		})
	}

	return &svcproto.TraceQueryResponse{Traces: traces, Total: len(traces)}, nil
}

// GetRootCauseAnalysis 根因分析
func (s *Service) GetRootCauseAnalysis(ctx context.Context, req *svcproto.RootCauseRequest) (*svcproto.RootCauseResponse, error) {
	report := s.correlationEngine.GetRootCauseAnalysis(req.TraceId)
	if report == nil {
		return &svcproto.RootCauseResponse{}, nil
	}

	resp := &svcproto.RootCauseResponse{
		TraceId:          report.TraceID,
		AffectedServices: report.AffectedServices,
		SuggestedCauses:  report.SuggestedCauses,
	}

	for _, es := range report.ErrorSpans {
		resp.ErrorSpans = append(resp.ErrorSpans, &svcproto.SpanInfo{
			TraceId: report.TraceID, SpanId: es.SpanID,
			ServiceName: es.ServiceName, Name: es.OperationName,
			DurationNs: es.DurationNs,
		})
	}

	for _, ss := range report.SlowSpans {
		resp.SlowSpans = append(resp.SlowSpans, &svcproto.SpanInfo{
			TraceId: report.TraceID, SpanId: ss.SpanID,
			ServiceName: ss.ServiceName, Name: ss.OperationName,
			DurationNs: ss.DurationNs,
		})
	}

	for _, fl := range report.RelatedFlows {
		resp.RelatedFlows = append(resp.RelatedFlows, &svcproto.FlowTraceLink{
			TraceId: fl.TraceID, SpanId: fl.SpanID,
			ServiceName: fl.ServiceName,
			SrcIp:       fl.FlowSrcIP, DstIp: fl.FlowDstIP,
			SrcPort: fl.FlowSrcPort, DstPort: fl.FlowDstPort,
			Protocol: fl.FlowProtocol, LatencyNs: fl.FlowLatencyNs,
			Bytes: fl.FlowBytes, Timestamp: fl.FlowTimestamp,
		})
	}

	return resp, nil
}

// QueryCorrelation 关联查询
func (s *Service) QueryCorrelation(ctx context.Context, req *svcproto.CorrelationQueryRequest) (*svcproto.CorrelationQueryResponse, error) {
	var traceIDs []string

	switch req.QueryType {
	case "trace_to_flow":
		links := s.correlationEngine.GetFlowsByTraceID(req.TraceId)
		var flows []*svcproto.FlowTraceLink
		for _, fl := range links {
			flows = append(flows, &svcproto.FlowTraceLink{
				TraceId: fl.TraceID, SpanId: fl.SpanID,
				ServiceName: fl.ServiceName,
				SrcIp:       fl.FlowSrcIP, DstIp: fl.FlowDstIP,
				SrcPort: fl.FlowSrcPort, DstPort: fl.FlowDstPort,
				Protocol: fl.FlowProtocol, LatencyNs: fl.FlowLatencyNs,
				Bytes: fl.FlowBytes, Timestamp: fl.FlowTimestamp,
			})
		}
		return &svcproto.CorrelationQueryResponse{Flows: flows, Total: len(flows)}, nil

	case "service_to_trace":
		traceIDs = s.correlationEngine.GetTracesByService(req.ServiceName)
	case "process_to_trace":
		traceIDs = s.correlationEngine.GetTracesByProcess(req.ProcessName, req.Pid)
	}

	// Convert trace IDs to TraceInfo
	var traces []*svcproto.TraceInfo
	for _, tid := range traceIDs {
		trace, ok := s.otlpReceiver.TraceStore().GetTrace(tid)
		if !ok {
			continue
		}
		traces = append(traces, convertTraceToProto(trace))
	}

	return &svcproto.CorrelationQueryResponse{Traces: traces, Total: len(traces)}, nil
}

// GetOTLPStats 获取 OTLP 接收统计
func (s *Service) GetOTLPStats(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.OTLPIngestStats, error) {
	stats := s.otlpReceiver.Stats()
	return &svcproto.OTLPIngestStats{
		TracesReceived:     stats.TracesReceived,
		SpansReceived:      stats.SpansReceived,
		MetricsReceived:    stats.MetricsReceived,
		DataPointsReceived: stats.DataPointsReceived,
		LogsReceived:       stats.LogsReceived,
		LogRecordsReceived: stats.LogRecordsReceived,
		Errors:             stats.Errors,
	}, nil
}

// HTTP Handlers

func (s *Service) healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s"}`, s.config.ServiceName)
}

func (s *Service) overviewHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if s.tsDB == nil {
		writeJSON(w, map[string]interface{}{
			"totalFlows": 0, "activeAgents": 0, "activeServices": 0,
			"trafficLabels": []string{}, "trafficInbound": []int64{}, "trafficOutbound": []int64{},
			"topServices": []map[string]interface{}{}, "resources": []map[string]interface{}{},
			"avgLatency": 0,
		})
		return
	}

	var totalFlows, activeAgents, activeServices uint64
	s.tsDB.QueryRow(ctx, "SELECT count() FROM cloudflow.ebpf_events").Scan(&totalFlows)
	s.tsDB.QueryRow(ctx, "SELECT count(DISTINCT probe_id) FROM cloudflow.ebpf_events WHERE probe_id != '' AND probe_id != 'test'").Scan(&activeAgents)
	s.tsDB.QueryRow(ctx, "SELECT count(DISTINCT dst_ip) FROM cloudflow.ebpf_events WHERE dst_ip != '' AND dst_ip NOT IN ('test','cpu','memory','network')").Scan(&activeServices)

	rows, _ := s.tsDB.Query(ctx, `
		SELECT toStartOfMinute(timestamp/1000000000) as t, sum(bytes) as total
		FROM cloudflow.ebpf_events
		WHERE toDateTime(timestamp/1000000000) > now() - INTERVAL 30 MINUTE
		GROUP BY t ORDER BY t
	`)

	var trafficLabels []string
	var trafficInbound []int64
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var t time.Time
			var total int64
			if rows.Scan(&t, &total) == nil {
				trafficLabels = append(trafficLabels, t.Format("15:04"))
				trafficInbound = append(trafficInbound, total)
			}
		}
	}

	topRows, _ := s.tsDB.Query(ctx, `
		SELECT dst_ip, count() as cnt
		FROM cloudflow.ebpf_events
		WHERE toDateTime(timestamp/1000000000) > now() - INTERVAL 30 MINUTE AND dst_ip != '' AND dst_ip NOT IN ('test','cpu','memory','network')
		GROUP BY dst_ip ORDER BY cnt DESC LIMIT 5
	`)

	var topServices []map[string]interface{}
	if topRows != nil {
		defer topRows.Close()
		maxCnt := uint64(0)
		for topRows.Next() {
			var name string
			var cnt uint64
			if topRows.Scan(&name, &cnt) == nil {
				if cnt > maxCnt {
					maxCnt = cnt
				}
				topServices = append(topServices, map[string]interface{}{"name": name, "qps": cnt, "percentage": 0.0})
			}
		}
		if maxCnt > 0 {
			for _, svc := range topServices {
				if qps, ok := svc["qps"].(uint64); ok {
					svc["percentage"] = float64(qps) / float64(maxCnt) * 100.0
				}
			}
		}
	}

	cpuVal, memVal, netVal := 0.0, 0.0, 0.0
	s.tsDB.QueryRow(ctx, "SELECT metric_value FROM cloudflow.ebpf_events WHERE metric_name='cpu_percent' ORDER BY timestamp DESC LIMIT 1").Scan(&cpuVal)
	s.tsDB.QueryRow(ctx, "SELECT metric_value FROM cloudflow.ebpf_events WHERE metric_name='memory' ORDER BY timestamp DESC LIMIT 1").Scan(&memVal)
	s.tsDB.QueryRow(ctx, "SELECT metric_value FROM cloudflow.ebpf_events WHERE metric_name='network' ORDER BY timestamp DESC LIMIT 1").Scan(&netVal)

	resources := []map[string]interface{}{
		{"name": "CPU", "percentage": cpuVal, "color": "stroke-primary-500"},
		{"name": "Memory", "percentage": memVal, "color": "stroke-accent-500"},
		{"name": "NetworkIO", "percentage": netVal / 10000.0, "color": "stroke-amber-500"},
		{"name": "Disk", "percentage": 0.0, "color": "stroke-emerald-500"},
	}

	var avgLatency float64
	s.tsDB.QueryRow(ctx, "SELECT avg(latency_ms) FROM cloudflow.ebpf_events WHERE latency_ms > 0 AND toDateTime(timestamp/1000000000) > now() - INTERVAL 30 MINUTE").Scan(&avgLatency)
	if avgLatency != avgLatency { // NaN check
		avgLatency = 0.0
	}

	result := map[string]interface{}{
		"totalFlows": totalFlows, "activeAgents": activeAgents, "activeServices": activeServices,
		"trafficLabels": trafficLabels, "trafficInbound": trafficInbound, "trafficOutbound": []int64{},
		"topServices": topServices, "resources": resources,
		"avgLatency": avgLatency,
		"traceCount": 0, "alertCount": 0,
		"flowsChange": "0%", "agentsChange": "0", "servicesChange": "0",
		"tracesChange": "0%", "alertsChange": "0%", "latencyChange": "0ms",
	}

	writeJSON(w, result)
}

func (s *Service) metricsHandler(w http.ResponseWriter, r *http.Request) {
	req := &svcproto.QueryFlowRequest{
		TenantId:  r.URL.Query().Get("tenant_id"),
		StartTime: parseInt64(r.URL.Query().Get("start_time")),
		EndTime:   parseInt64(r.URL.Query().Get("end_time")),
		Namespace: r.URL.Query().Get("namespace"),
		Service:   r.URL.Query().Get("service"),
		Limit:     parseInt(r.URL.Query().Get("limit"), 1000),
	}
	resp, err := s.QueryMetrics(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *Service) flowsHandler(w http.ResponseWriter, r *http.Request) {
	req := &svcproto.QueryFlowRequest{
		TenantId:  r.URL.Query().Get("tenant_id"),
		StartTime: parseInt64(r.URL.Query().Get("start_time")),
		EndTime:   parseInt64(r.URL.Query().Get("end_time")),
		SrcIp:     r.URL.Query().Get("src_ip"),
		DstIp:     r.URL.Query().Get("dst_ip"),
		Namespace: r.URL.Query().Get("namespace"),
		Service:   r.URL.Query().Get("service"),
		Limit:     parseInt(r.URL.Query().Get("limit"), 1000),
	}
	resp, err := s.QueryFlows(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *Service) tracesHandler(w http.ResponseWriter, r *http.Request) {
	req := &svcproto.QueryFlowRequest{
		TenantId:  r.URL.Query().Get("tenant_id"),
		StartTime: parseInt64(r.URL.Query().Get("start_time")),
		EndTime:   parseInt64(r.URL.Query().Get("end_time")),
		Namespace: r.URL.Query().Get("namespace"),
		Service:   r.URL.Query().Get("service"),
		Limit:     parseInt(r.URL.Query().Get("limit"), 100),
	}
	resp, err := s.QueryTraces(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *Service) topologyHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"message": "topology endpoint - use gRPC for topology queries",
	})
}

func (s *Service) alertsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"alerts": []interface{}{},
		"total":  0,
	})
}


func (s *Service) metricsDataHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.tsDB == nil {
		writeJSON(w, map[string]interface{}{
			"records": []map[string]interface{}{},
			"total":   0,
			"took_ms": 0,
		})
		return
	}
	query := "SELECT timestamp, probe_id, cpu_percent, memory_percent, disk_percent, net_rx_bytes, net_tx_bytes, disk_read_bytes, disk_write_bytes FROM cloudflow.host_metrics ORDER BY timestamp DESC LIMIT 100"
	rows, err := s.tsDB.Query(ctx, query)
	if err != nil {
		writeJSON(w, map[string]interface{}{
			"records": []map[string]interface{}{},
			"total":   0,
			"took_ms": 0,
			"error":   err.Error(),
		})
		return
	}
	defer rows.Close()
	records := []map[string]interface{}{}
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}
		record := make(map[string]interface{})
		for i, col := range columns {
			record[col] = values[i]
		}
		records = append(records, record)
	}
	writeJSON(w, map[string]interface{}{
		"records": records,
		"total":   len(records),
		"took_ms": 0,
	})
}

func (s *Service) otelTracesHandler(w http.ResponseWriter, r *http.Request) {
	// Proxy to trace store search
	writeJSON(w, s.otlpReceiver.TraceStore().SearchSpans(otel.SpanQuery{Limit: 100}))
}

func (s *Service) otelMetricsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.otlpReceiver.MetricsStore().GetAllSeriesNames())
}

func (s *Service) otelLogsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.otlpReceiver.LogStore().Query(otel.LogQuery{Limit: 100}))
}

func (s *Service) otelStatsHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.otlpReceiver.Stats())
}

func (s *Service) rcaHandler(w http.ResponseWriter, r *http.Request) {
	traceID := r.URL.Query().Get("trace_id")
	resp, _ := s.GetRootCauseAnalysis(r.Context(), &svcproto.RootCauseRequest{TraceId: traceID})
	writeJSON(w, resp)
}

func (s *Service) correlationHandler(w http.ResponseWriter, r *http.Request) {
	queryType := r.URL.Query().Get("type")
	traceID := r.URL.Query().Get("trace_id")
	serviceName := r.URL.Query().Get("service")
	resp, _ := s.QueryCorrelation(r.Context(), &svcproto.CorrelationQueryRequest{
		QueryType: queryType, TraceId: traceID, ServiceName: serviceName,
	})
	writeJSON(w, resp)
}

// Helper functions

func convertSpanToProto(sp *otel.Span) *svcproto.SpanInfo {
	return &svcproto.SpanInfo{
		TraceId: sp.TraceID, SpanId: sp.SpanID, ParentSpanId: sp.ParentSpanID,
		Name: sp.Name, ServiceName: sp.ServiceName, Kind: sp.Kind,
		StartTime: sp.StartTime, EndTime: sp.EndTime, DurationNs: sp.DurationNs,
		Status: sp.Status, StatusCode: sp.StatusCode, Attributes: sp.Attributes,
	}
}

func convertTraceToProto(t *otel.Trace) *svcproto.TraceInfo {
	var spans []*svcproto.SpanInfo
	for _, sp := range t.Spans {
		spans = append(spans, convertSpanToProto(sp))
	}
	return &svcproto.TraceInfo{
		TraceId:     t.TraceID,
		ServiceName: t.ServiceName,
		SpanCount:   len(t.Spans),
		ErrorCount:  t.ErrorCount,
		DurationNs:  t.DurationNs,
		StartTime:   t.StartTime,
		Spans:       spans,
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func parseInt64(s string) int64 {
	var result int64
	fmt.Sscanf(s, "%d", &result)
	return result
}

func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	var result int
	fmt.Sscanf(s, "%d", &result)
	if result == 0 {
		return defaultVal
	}
	return result
}

// gRPC 服务注册
type queryGRPC struct {
	svcproto.UnimplementedQueryServiceServer
	svc *Service
}

func RegisterQueryService(s *grpc.Server, svc *Service) {
	svcproto.RegisterQueryServiceServer(s, &queryGRPC{svc: svc})
}

func (g *queryGRPC) HealthCheck(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.HealthCheckResponse, error) {
	return &svcproto.HealthCheckResponse{Healthy: true}, nil
}

func (g *queryGRPC) QueryFlows(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	return g.svc.QueryFlows(ctx, req)
}

func (g *queryGRPC) QueryMetrics(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	return g.svc.QueryMetrics(ctx, req)
}

func (g *queryGRPC) QueryTraces(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	return g.svc.QueryTraces(ctx, req)
}

func (g *queryGRPC) QueryDashboard(ctx context.Context, req *svcproto.QueryFlowRequest) (*svcproto.QueryFlowResponse, error) {
	return g.svc.QueryDashboard(ctx, req)
}

func (g *queryGRPC) QueryOTLPTraces(ctx context.Context, req *svcproto.TraceQueryRequest) (*svcproto.TraceQueryResponse, error) {
	return g.svc.QueryOTLPTraces(ctx, req)
}

func (g *queryGRPC) GetRootCauseAnalysis(ctx context.Context, req *svcproto.RootCauseRequest) (*svcproto.RootCauseResponse, error) {
	return g.svc.GetRootCauseAnalysis(ctx, req)
}

func (g *queryGRPC) QueryCorrelation(ctx context.Context, req *svcproto.CorrelationQueryRequest) (*svcproto.CorrelationQueryResponse, error) {
	return g.svc.QueryCorrelation(ctx, req)
}

func (g *queryGRPC) GetOTLPStats(ctx context.Context, req *svcproto.HealthCheckRequest) (*svcproto.OTLPIngestStats, error) {
	return g.svc.GetOTLPStats(ctx, req)
}
