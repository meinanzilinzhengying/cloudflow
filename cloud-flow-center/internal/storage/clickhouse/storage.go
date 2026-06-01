// Package clickhouse ClickHouse 存储引擎实现
//
// 性能目标:
//   - 写入: 10 亿 flow/day ≈ 12K flow/s
//   - 查询: 秒级响应
//   - 支持 topology graph query

package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"cloud-flow/cloud-flow-center/internal/storage"
	"cloud-flow/cloud-flow-center/internal/storage/clickhouse/schema"
	"cloud-flow/pkg/flow"
)

// ============================================================================
// 配置
// ============================================================================

// Config ClickHouse 配置
type Config struct {
	// 连接配置
	Addrs       []string // e.g., ["clickhouse-0-0:9000", "clickhouse-0-1:9000"] 用于 HA
	Addr        string   // e.g., "clickhouse:9000" (向后兼容)
	Database    string
	Username    string
	Password    string

	// 连接池
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	DialTimeout     time.Duration
	
	// HA 配置
	EnableHA        bool
	ConnectionRetry int

	// 批量写入
	BatchSize       int
	FlushInterval   time.Duration
	QueueSize       int
	WorkerCount     int

	// 性能
	MaxRetries      int
	RetryInterval   time.Duration

	// Schema
	SchemaConfig    *schema.SchemaConfig
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Addr:            "clickhouse:9000",
		Addrs:           []string{"clickhouse-0-0:9000", "clickhouse-0-1:9000", "clickhouse-1-0:9000", "clickhouse-1-1:9000"},
		Database:        "cloudflow",
		Username:        "default",
		Password:        "",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		DialTimeout:     10 * time.Second,
		EnableHA:        false,
		ConnectionRetry: 3,
		BatchSize:       10000,
		FlushInterval:   time.Second,
		QueueSize:       100000,
		WorkerCount:     4,
		MaxRetries:      3,
		RetryInterval:   100 * time.Millisecond,
		SchemaConfig:    schema.DefaultSchemaConfig(),
	}
}

// ============================================================================
// ClickHouse Storage Engine
// ============================================================================

// Storage ClickHouse 存储引擎
type Storage struct {
	config *Config
	db     *sql.DB

	// HA 连接池 - 备用连接
	alternateDbs []*sql.DB
	
	// 当前连接索引
	currentConnIndex int

	// 批量写入
	flowQueue   chan *flow.UnifiedFlow
	traceQueue  chan *TraceRecord
	eventQueue  chan *EventRecord

	// Worker
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc

	// 统计
	stats Stats

	// 状态
	ready atomic.Bool
	
	// 连接锁
	connMu sync.RWMutex
}

// Stats 统计信息
type Stats struct {
	FlowsWritten   uint64
	TracesWritten  uint64
	EventsWritten  uint64
	FlowsDropped   uint64
	WriteErrors    uint64
	QueryCount     uint64
	QueryErrors    uint64
	AvgWriteTimeNs uint64
}

// TraceRecord Trace 记录
type TraceRecord struct {
	Timestamp     int64
	TraceID       string
	SpanID        string
	ParentID      string
	TraceState    string
	FlowID        uint32
	Name          string
	Kind          uint8
	StartTimeNs   int64
	EndTimeNs     int64
	DurationNs    int64
	StatusCode    uint8
	StatusMessage string
	Attributes    string
	ServiceName   string
	Namespace     string
	Pod           string
	Node          string
	TenantID      string
}

// EventRecord Event 记录
type EventRecord struct {
	Timestamp    int64
	EventID      string
	EventType    string
	Severity     uint8
	Source       string
	SourceID     string
	Title        string
	Message      string
	Details      string
	ResourceType string
	ResourceID   string
	Namespace    string
	Service      string
	Pod          string
	Node         string
	TenantID     string
}

// New 创建 ClickHouse 存储
func New(config *Config) (*Storage, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Storage{
		config:     config,
		flowQueue:  make(chan *flow.UnifiedFlow, config.QueueSize),
		traceQueue: make(chan *TraceRecord, config.QueueSize/10),
		eventQueue: make(chan *EventRecord, config.QueueSize/10),
		ctx:        ctx,
		cancel:     cancel,
	}

	// 连接数据库
	if err := s.connect(); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	// 初始化 Schema
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema failed: %w", err)
	}

	// 启动 workers
	s.startWorkers()

	s.ready.Store(true)
	return s, nil
}

// connect 连接数据库（支持 HA）
func (s *Storage) connect() error {
	// 首先，检查是否启用 HA 模式
	if s.config.EnableHA && len(s.config.Addrs) > 0 {
		// HA 模式：连接所有节点
		var primaryDb *sql.DB
		var alts []*sql.DB
		
		for i, addr := range s.config.Addrs {
			dsn := fmt.Sprintf("clickhouse://%s:%s@%s/%s?dial_timeout=%s",
				s.config.Username,
				s.config.Password,
				addr,
				s.config.Database,
				s.config.DialTimeout,
			)
			
			db, err := sql.Open("clickhouse", dsn)
			if err != nil {
				continue
			}
			
			db.SetMaxOpenConns(s.config.MaxOpenConns)
			db.SetMaxIdleConns(s.config.MaxIdleConns)
			db.SetConnMaxLifetime(s.config.ConnMaxLifetime)
			
			// 测试连接
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			
			if err := db.PingContext(ctx); err != nil {
				db.Close()
				continue
			}
			
			if primaryDb == nil {
				primaryDb = db
			} else {
				alts = append(alts, db)
			}
		}
		
		if primaryDb == nil {
			return fmt.Errorf("could not connect to any ClickHouse node in HA mode")
		}
		
		s.db = primaryDb
		s.alternateDbs = alts
		return nil
	}
	
	// 非 HA 模式（向后兼容）
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s/%s?dial_timeout=%s",
		s.config.Username,
		s.config.Password,
		s.config.Addr,
		s.config.Database,
		s.config.DialTimeout,
	)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(s.config.MaxOpenConns)
	db.SetMaxIdleConns(s.config.MaxIdleConns)
	db.SetConnMaxLifetime(s.config.ConnMaxLifetime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return err
	}

	s.db = db
	return nil
}

// getDB 获取当前可用的数据库连接
func (s *Storage) getDB() *sql.DB {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.db
}

// failover 尝试故障转移到备用连接
func (s *Storage) failover() bool {
	if !s.config.EnableHA || len(s.alternateDbs) == 0 {
		return false
	}
	
	s.connMu.Lock()
	defer s.connMu.Unlock()
	
	// 尝试所有备用连接
	for i, alt := range s.alternateDbs {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := alt.PingContext(ctx)
		cancel()
		if err == nil {
			// 切换到这个备用连接
			oldPrimary := s.db
			s.db = alt
			
			// 重新构建备用连接列表（移除已使用的，添加原来的主连接）
			newAlts := make([]*sql.DB, 0, len(s.alternateDbs))
			for j, db := range s.alternateDbs {
				if j != i {
					newAlts = append(newAlts, db)
				}
			}
			if oldPrimary != nil {
				newAlts = append(newAlts, oldPrimary)
			}
			s.alternateDbs = newAlts
			
			return true
		}
	}
	
	return false
}

// execWithRetry 带重试的执行（支持故障转移）
func (s *Storage) execWithRetry(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var lastErr error
	
	for retries := 0; retries < s.config.ConnectionRetry; retries++ {
		db := s.getDB()
		if db == nil {
			return nil, fmt.Errorf("no available database connection")
		}
		
		res, err := db.ExecContext(ctx, query, args...)
		if err == nil {
			return res, nil
		}
		
		lastErr = err
		if s.config.EnableHA {
			if s.failover() {
				continue
			}
		}
		
		time.Sleep(s.config.RetryInterval)
	}
	
	return nil, lastErr
}

// queryWithRetry 带重试的查询（支持故障转移）
func (s *Storage) queryWithRetry(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	var lastErr error
	
	for retries := 0; retries < s.config.ConnectionRetry; retries++ {
		db := s.getDB()
		if db == nil {
			return nil, fmt.Errorf("no available database connection")
		}
		
		rows, err := db.QueryContext(ctx, query, args...)
		if err == nil {
			return rows, nil
		}
		
		lastErr = err
		if s.config.EnableHA {
			if s.failover() {
				continue
			}
		}
		
		time.Sleep(s.config.RetryInterval)
	}
	
	return nil, lastErr
}

// initSchema 初始化 Schema
func (s *Storage) initSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 执行 DDL
	ddl := schema.GenerateAllDDL(s.config.SchemaConfig)
	statements := strings.Split(ddl, ";")

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}

		if _, err := s.execWithRetry(ctx, stmt); err != nil {
			// 忽略 "already exists" 错误
			if !strings.Contains(err.Error(), "already exists") {
				return fmt.Errorf("execute DDL failed: %w, statement: %s", err, stmt[:min(100, len(stmt))])
			}
		}
	}

	return nil
}

// startWorkers 启动写入 workers
func (s *Storage) startWorkers() {
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.flowWriter()
	}

	s.wg.Add(1)
	go s.traceWriter()

	s.wg.Add(1)
	go s.eventWriter()
}

// ============================================================================
// StorageBackend 接口实现
// ============================================================================

// Name 返回名称
func (s *Storage) Name() string {
	return "clickhouse"
}

// Type 返回类型
func (s *Storage) Type() storage.DataType {
	return storage.DataTypeFlow
}

// Ready 检查是否就绪
func (s *Storage) Ready() bool {
	return s.ready.Load()
}

// Write 写入数据
func (s *Storage) Write(ctx context.Context, data interface{}) error {
	switch v := data.(type) {
	case *flow.UnifiedFlow:
		return s.WriteFlow(ctx, v)
	case *TraceRecord:
		return s.WriteTrace(ctx, v)
	case *EventRecord:
		return s.WriteEvent(ctx, v)
	default:
		return storage.ErrUnknownDataType
	}
}

// WriteBatch 批量写入
func (s *Storage) WriteBatch(ctx context.Context, batch []interface{}) error {
	if len(batch) == 0 {
		return nil
	}

	switch batch[0].(type) {
	case *flow.UnifiedFlow:
		flows := make([]*flow.UnifiedFlow, len(batch))
		for i, d := range batch {
			flows[i] = d.(*flow.UnifiedFlow)
		}
		return s.WriteFlows(ctx, flows)
	case *TraceRecord:
		traces := make([]*TraceRecord, len(batch))
		for i, d := range batch {
			traces[i] = d.(*TraceRecord)
		}
		return s.WriteTraces(ctx, traces)
	case *EventRecord:
		events := make([]*EventRecord, len(batch))
		for i, d := range batch {
			events[i] = d.(*EventRecord)
		}
		return s.WriteEvents(ctx, events)
	default:
		return storage.ErrUnknownDataType
	}
}

// WriteFlow 写入单条 Flow
func (s *Storage) WriteFlow(ctx context.Context, f *flow.UnifiedFlow) error {
	select {
	case s.flowQueue <- f:
		return nil
	default:
		s.stats.FlowsDropped++
		return storage.ErrWriteFailed
	}
}

// WriteFlows 批量写入 Flow
func (s *Storage) WriteFlows(ctx context.Context, flows []*flow.UnifiedFlow) error {
	for _, f := range flows {
		if err := s.WriteFlow(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

// WriteTrace 写入 Trace
func (s *Storage) WriteTrace(ctx context.Context, t *TraceRecord) error {
	select {
	case s.traceQueue <- t:
		return nil
	default:
		return storage.ErrWriteFailed
	}
}

// WriteTraces 批量写入 Trace
func (s *Storage) WriteTraces(ctx context.Context, traces []*TraceRecord) error {
	return s.writeTracesBatch(ctx, traces)
}

// WriteEvent 写入 Event
func (s *Storage) WriteEvent(ctx context.Context, e *EventRecord) error {
	select {
	case s.eventQueue <- e:
		return nil
	default:
		return storage.ErrWriteFailed
	}
}

// WriteEvents 批量写入 Event
func (s *Storage) WriteEvents(ctx context.Context, events []*EventRecord) error {
	return s.writeEventsBatch(ctx, events)
}

// ============================================================================
// 查询实现
// ============================================================================

// Query 查询数据
func (s *Storage) Query(ctx context.Context, req *storage.QueryRequest) (*storage.QueryResult, error) {
	s.stats.QueryCount++

	switch req.DataType {
	case storage.DataTypeFlow:
		return s.queryFlows(ctx, req)
	case storage.DataTypeTrace:
		return s.queryTraces(ctx, req)
	case storage.DataTypeEvent:
		return s.queryEvents(ctx, req)
	default:
		return nil, storage.ErrUnknownDataType
	}
}

// queryFlows 查询 Flow
func (s *Storage) queryFlows(ctx context.Context, req *storage.QueryRequest) (*storage.QueryResult, error) {
	start := time.Now()

	// 构建查询
	var query strings.Builder
	var args []interface{}

	query.WriteString(`
		SELECT
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
		FROM flows
		WHERE 1=1
	`)

	// 添加条件
	if req.TenantID != "" {
		query.WriteString(" AND tenant_id = ?")
		args = append(args, req.TenantID)
	}

	if !req.StartTime.IsZero() {
		query.WriteString(" AND timestamp >= ?")
		args = append(args, req.StartTime.UnixNano())
	}

	if !req.EndTime.IsZero() {
		query.WriteString(" AND timestamp <= ?")
		args = append(args, req.EndTime.UnixNano())
	}

	if req.SrcIP != "" {
		query.WriteString(" AND src_ip = ?")
		args = append(args, req.SrcIP)
	}

	if req.DstIP != "" {
		query.WriteString(" AND dst_ip = ?")
		args = append(args, req.DstIP)
	}

	if req.Namespace != "" {
		query.WriteString(" AND namespace = ?")
		args = append(args, req.Namespace)
	}

	if req.Service != "" {
		query.WriteString(" AND service = ?")
		args = append(args, req.Service)
	}

	if req.Pod != "" {
		query.WriteString(" AND pod = ?")
		args = append(args, req.Pod)
	}

	// 排序 - FIX: 白名单校验 OrderBy 字段，防止 SQL 注入
	if req.OrderBy != "" {
		if !isValidOrderByColumn(req.OrderBy) {
			return nil, fmt.Errorf("invalid order_by column: %s", req.OrderBy)
		}
		query.WriteString(fmt.Sprintf(" ORDER BY %s", req.OrderBy))
		if req.OrderDesc {
			query.WriteString(" DESC")
		}
	} else {
		query.WriteString(" ORDER BY timestamp DESC")
	}

	// 限制
	if req.Limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", req.Limit))
	}

	if req.Offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", req.Offset))
	}

	// 执行查询
	rows, err := s.queryWithRetry(ctx, query.String(), args...)
	if err != nil {
		s.stats.QueryErrors++
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	// 解析结果
	records := make([]map[string]interface{}, 0)
	for rows.Next() {
		record := make(map[string]interface{})
		var (
			timestamp, reqSize, respSize, bytes, packets, latencyNs uint64
			flowID, schemaVersion, pid                               uint32
			srcPort, dstPort, statusCode                             uint16
			ipVersion, protocol, tcpFlags, l7Protocol, method, direction uint8
			srcIP, dstIP, path, grpcService, grpcMethod               string
			grpcStatus                                                uint32
			processName, comm, containerID, containerName, image      string
			pod, namespace, deployment, service, node                 string
			traceID, spanID, parentID                                 string
			hostID, hostname, tenantID                                string
			exception, tags                                           string
		)

		err := rows.Scan(
			&timestamp, &flowID, &schemaVersion,
			&srcIP, &dstIP, &srcPort, &dstPort, &protocol, &tcpFlags,
			&l7Protocol, &method, &path, &statusCode, &reqSize, &respSize,
			&grpcService, &grpcMethod, &grpcStatus,
			&pid, &processName, &comm,
			&containerID, &containerName, &image,
			&pod, &namespace, &deployment, &service, &node,
			&traceID, &spanID, &parentID,
			&hostID, &hostname, &tenantID,
			&bytes, &packets, &latencyNs, &direction, &exception, &tags,
		)
		if err != nil {
			continue
		}

		record["timestamp"] = timestamp
		record["flow_id"] = flowID
		record["src_ip"] = srcIP
		record["dst_ip"] = dstIP
		record["src_port"] = srcPort
		record["dst_port"] = dstPort
		record["protocol"] = protocol
		record["bytes"] = bytes
		record["packets"] = packets
		record["latency_ns"] = latencyNs
		record["tenant_id"] = tenantID

		records = append(records, record)
	}

	return &storage.QueryResult{
		Records: records,
		Total:   int64(len(records)),
		TookMs:  time.Since(start).Milliseconds(),
	}, nil
}

// queryTraces 查询 Trace
func (s *Storage) queryTraces(ctx context.Context, req *storage.QueryRequest) (*storage.QueryResult, error) {
	// TODO: 实现 trace 查询
	return &storage.QueryResult{}, nil
}

// queryEvents 查询 Event
func (s *Storage) queryEvents(ctx context.Context, req *storage.QueryRequest) (*storage.QueryResult, error) {
	// TODO: 实现 event 查询
	return &storage.QueryResult{}, nil
}

// QueryTopology 查询拓扑
func (s *Storage) QueryTopology(ctx context.Context, req *storage.QueryRequest) (*storage.TopologyResult, error) {
	start := time.Now()

	if req.Topology == nil {
		return nil, errors.New("topology config required")
	}

	// 从拓扑表查询
	query := `
		SELECT
			src_type, src_id, src_name, src_namespace,
			dst_type, dst_id, dst_name, dst_namespace,
			protocol, port,
			sum(flow_count) as flow_count,
			sum(bytes_sum) as bytes_sum,
			sum(packets_sum) as packets_sum,
			avg(latency_avg) as latency_avg,
			sum(error_count) as error_count
		FROM topology
		WHERE tenant_id = ?
			AND timestamp >= ?
			AND timestamp <= ?
		GROUP BY
			src_type, src_id, src_name, src_namespace,
			dst_type, dst_id, dst_name, dst_namespace,
			protocol, port
	`

	args := []interface{}{
		req.TenantID,
		req.StartTime,
		req.EndTime,
	}

	rows, err := s.queryWithRetry(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query topology failed: %w", err)
	}
	defer rows.Close()

	// 构建拓扑图
	nodeMap := make(map[string]*storage.TopologyNode)
	var edges []*storage.TopologyEdge

	for rows.Next() {
		var (
			srcType, srcID, srcName, srcNS string
			dstType, dstID, dstName, dstNS string
			protocol                       uint8
			port                           uint16
			flowCount, bytesSum, packetsSum, errorCount uint64
			latencyAvg                     float64
		)

		if err := rows.Scan(
			&srcType, &srcID, &srcName, &srcNS,
			&dstType, &dstID, &dstName, &dstNS,
			&protocol, &port,
			&flowCount, &bytesSum, &packetsSum, &latencyAvg, &errorCount,
		); err != nil {
			continue
		}

		// 添加源节点
		srcKey := srcType + ":" + srcID
		if _, exists := nodeMap[srcKey]; !exists {
			nodeMap[srcKey] = &storage.TopologyNode{
				ID:        srcID,
				Name:      srcName,
				Type:      srcType,
				Namespace: srcNS,
			}
		}
		nodeMap[srcKey].BytesOut += bytesSum

		// 添加目标节点
		dstKey := dstType + ":" + dstID
		if _, exists := nodeMap[dstKey]; !exists {
			nodeMap[dstKey] = &storage.TopologyNode{
				ID:        dstID,
				Name:      dstName,
				Type:      dstType,
				Namespace: dstNS,
			}
		}
		nodeMap[dstKey].BytesIn += bytesSum

		// 添加边
		edges = append(edges, &storage.TopologyEdge{
			Source:     srcID,
			Target:     dstID,
			Protocol:   flow.Protocol(protocol).String(),
			Port:       port,
			Bytes:      bytesSum,
			Packets:    packetsSum,
			LatencyNs:  uint64(latencyAvg),
			ErrorCount: errorCount,
		})
	}

	// 转换节点 map 为列表
	nodes := make([]*storage.TopologyNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, node)
	}

	return &storage.TopologyResult{
		Nodes: nodes,
		Edges: edges,
		Stats: storage.TopologyStats{
			NodeCount:  len(nodes),
			EdgeCount:  len(edges),
			TotalBytes: 0, // TODO: 计算
		},
	}, nil
}

// ============================================================================
// 批量写入 Workers
// ============================================================================

func (s *Storage) flowWriter() {
	defer s.wg.Done()

	batch := make([]*flow.UnifiedFlow, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			// 刷新剩余数据
			if len(batch) > 0 {
				s.writeFlowsBatch(context.Background(), batch)
			}
			return

		case f := <-s.flowQueue:
			batch = append(batch, f)
			if len(batch) >= s.config.BatchSize {
				s.writeFlowsBatch(context.Background(), batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.writeFlowsBatch(context.Background(), batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Storage) traceWriter() {
	defer s.wg.Done()

	batch := make([]*TraceRecord, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.writeTracesBatch(context.Background(), batch)
			}
			return

		case t := <-s.traceQueue:
			batch = append(batch, t)
			if len(batch) >= s.config.BatchSize {
				s.writeTracesBatch(context.Background(), batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.writeTracesBatch(context.Background(), batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Storage) eventWriter() {
	defer s.wg.Done()

	batch := make([]*EventRecord, 0, s.config.BatchSize)
	ticker := time.NewTicker(s.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				s.writeEventsBatch(context.Background(), batch)
			}
			return

		case e := <-s.eventQueue:
			batch = append(batch, e)
			if len(batch) >= s.config.BatchSize {
				s.writeEventsBatch(context.Background(), batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.writeEventsBatch(context.Background(), batch)
				batch = batch[:0]
			}
		}
	}
}

// writeFlowsBatch 批量写入 Flow
func (s *Storage) writeFlowsBatch(ctx context.Context, flows []*flow.UnifiedFlow) error {
	if len(flows) == 0 {
		return nil
	}

	start := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
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
		_, err := stmt.ExecContext(ctx,
			f.Timestamp, f.FlowID, f.SchemaVersion,
			f.SrcIP.String(), f.DstIP.String(), f.SrcPort, f.DstPort, f.Protocol, f.TCPFlags,
			f.L7Protocol, f.Method, f.Path.String(), f.StatusCode, f.ReqSize, f.RespSize,
			f.GetGRPCService(), f.GetGRPCMethod(), 0, // grpc_status
			f.PID, f.ProcessName.String(), f.Comm.String(),
			f.ContainerID.String(), f.ContainerName.String(), f.Image.String(),
			f.Pod.String(), f.Namespace.String(), f.Deployment.String(), f.Service.String(), f.Node.String(),
			f.TraceID.String(), f.SpanID.String(), f.ParentID.String(),
			f.HostID.String(), f.Hostname.String(), f.TenantID.String(),
			f.Bytes, f.Packets, f.LatencyNs, f.Direction, f.GetL7Exception(), "", // tags
		)
		if err != nil {
			s.stats.WriteErrors++
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.stats.FlowsWritten += uint64(len(flows))
	duration := time.Since(start).Nanoseconds()
	s.stats.AvgWriteTimeNs = (s.stats.AvgWriteTimeNs*9 + uint64(duration)) / 10

	return nil
}

// writeTracesBatch 批量写入 Trace
func (s *Storage) writeTracesBatch(ctx context.Context, traces []*TraceRecord) error {
	if len(traces) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO traces (
			timestamp, trace_id, span_id, parent_id, trace_state, flow_id,
			name, kind, start_time_ns, end_time_ns, duration_ns,
			status_code, status_message, attributes,
			service_name, namespace, pod, node, tenant_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range traces {
		_, err := stmt.ExecContext(ctx,
			t.Timestamp, t.TraceID, t.SpanID, t.ParentID, t.TraceState, t.FlowID,
			t.Name, t.Kind, t.StartTimeNs, t.EndTimeNs, t.DurationNs,
			t.StatusCode, t.StatusMessage, t.Attributes,
			t.ServiceName, t.Namespace, t.Pod, t.Node, t.TenantID,
		)
		if err != nil {
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.stats.TracesWritten += uint64(len(traces))
	return nil
}

// writeEventsBatch 批量写入 Event
func (s *Storage) writeEventsBatch(ctx context.Context, events []*EventRecord) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (
			timestamp, event_id, event_type, severity,
			source, source_id, title, message, details,
			resource_type, resource_id, namespace, service, pod, node, tenant_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range events {
		_, err := stmt.ExecContext(ctx,
			e.Timestamp, e.EventID, e.EventType, e.Severity,
			e.Source, e.SourceID, e.Title, e.Message, e.Details,
			e.ResourceType, e.ResourceID, e.Namespace, e.Service, e.Pod, e.Node, e.TenantID,
		)
		if err != nil {
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	s.stats.EventsWritten += uint64(len(events))
	return nil
}

// Close 关闭连接
func (s *Storage) Close() error {
	s.ready.Store(false)
	s.cancel()
	s.wg.Wait()

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GetStats 获取统计
func (s *Storage) GetStats() Stats {
	return s.stats
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// validOrderByColumns 定义允许排序的列名白名单，防止 SQL 注入
var validOrderByColumns = map[string]bool{
	"timestamp":     true,
	"src_ip":        true,
	"dst_ip":        true,
	"src_port":      true,
	"dst_port":      true,
	"protocol":      true,
	"bytes_sent":    true,
	"bytes_recv":    true,
	"packets_sent":  true,
	"packets_recv":  true,
	"duration":      true,
	"service":       true,
	"pod":           true,
	"namespace":     true,
	"src_service":   true,
	"dst_service":   true,
}

// isValidOrderByColumn 检查 order_by 列名是否在白名单中
func isValidOrderByColumn(column string) bool {
	return validOrderByColumns[column]
}
