// Package schema ClickHouse 表结构定义
//
// 设计原则:
//   - MergeTree 引擎，支持高效范围查询
//   - 按天分区，便于 TTL 和数据管理
//   - 排序键: tenant_id + timestamp + flow_id
//   - LowCardinality 优化低基数字段
//   - Bloom Filter 加速高基数字段查询
//   - Skip Index 加速特定查询模式
//   - TTL 自动过期
//   - 物化视图预聚合

package schema

import (
	"fmt"
	"strings"
)

// ============================================================================
// 表名常量
// ============================================================================

const (
	// 主表
	TableFlows        = "flows"
	TableTraces       = "traces"
	TableEvents       = "events"

	// 聚合表
	TableFlows1m      = "flows_1m"      // 1分钟聚合
	TableFlows1h      = "flows_1h"      // 1小时聚合
	TableFlows1d      = "flows_1d"      // 1天聚合

	// 拓扑表
	TableTopology     = "topology"

	// 物化视图
	TableFlowsMV1m    = "flows_mv_1m"
	TableFlowsMV1h    = "flows_mv_1h"
	TableTopologyMV   = "topology_mv"
)

// ============================================================================
// DDL 生成器
// ============================================================================

// SchemaConfig Schema 配置
type SchemaConfig struct {
	// 数据库配置
	Database   string

	// TTL 配置
	FlowTTL    int // Flow 保留天数，默认 30
	TraceTTL   int // Trace 保留天数，默认 7
	EventTTL   int // Event 保留天数，默认 90

	// 冷热分离配置
	HotDays    int // 热数据保留天数，默认 7
	ColdVolume string // 冷存储卷名，默认 'default'

	// 分区配置
	PartitionBy string // 分区表达式，默认 toYYYYMMDD(timestamp)

	// 副本配置
	Replicated    bool
	ReplicaName   string
	ZooKeeper     string
	ClusterName   string // ClickHouse 集群名，默认 'cloudflow_cluster'
	ShardCount    int    // 分片数量，默认 2
	ReplicaCount  int    // 每个分片的副本数，默认 2
}

// DefaultSchemaConfig 返回默认配置
func DefaultSchemaConfig() *SchemaConfig {
	return &SchemaConfig{
		Database:      "cloudflow",
		FlowTTL:       30,
		TraceTTL:      7,
		EventTTL:      90,
		HotDays:       7,
		ColdVolume:    "default",
		PartitionBy:   "toYYYYMMDD(timestamp)",
		Replicated:    false,
		ClusterName:   "cloudflow_cluster",
		ShardCount:    2,
		ReplicaCount:  2,
	}
}

// GenerateCreateFlowsTable 生成 flows 表 DDL
func GenerateCreateFlowsTable(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		// 记录错误并使用默认值
		cfg = DefaultSchemaConfig()
	}

	engine := "MergeTree()"
	if cfg.Replicated {
		sanitizedDB := sanitizeIdentifier(cfg.Database)
		sanitizedTable := sanitizeIdentifier(TableFlows)
		engine = fmt.Sprintf("ReplicatedMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable)
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedTable := sanitizeIdentifier(TableFlows)
	sanitizedPartitionBy := sanitizePartitionExpression(cfg.PartitionBy)

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
    -- 时间戳 (纳秒)
    timestamp           Int64,
    timestamp_dt        DateTime DEFAULT toDateTime(timestamp / 1000000000),

    -- 流标识
    flow_id             UInt32,
    schema_version      UInt32,

    -- L3 网络层
    src_ip              String,
    dst_ip              String,
    src_ip_num          UInt32,
    dst_ip_num          UInt32,
    ip_version          LowCardinality(UInt8),

    -- L4 传输层
    src_port            UInt16,
    dst_port            UInt16,
    protocol            LowCardinality(UInt8),
    tcp_flags           LowCardinality(UInt8),

    -- L7 应用层
    l7_protocol         LowCardinality(UInt8),
    method              LowCardinality(UInt8),
    path                String,
    status_code         LowCardinality(UInt16),
    req_size            UInt64,
    resp_size           UInt64,

    -- L7 扩展 (gRPC)
    grpc_service        LowCardinality(String),
    grpc_method         LowCardinality(String),
    grpc_status         UInt32,

    -- Process
    pid                 UInt32,
    process_name        LowCardinality(String),
    comm                LowCardinality(String),

    -- Container
    container_id        String,
    container_name      LowCardinality(String),
    image               LowCardinality(String),

    -- Kubernetes
    pod                 String,
    namespace           LowCardinality(String),
    deployment          LowCardinality(String),
    service             LowCardinality(String),
    node                LowCardinality(String),

    -- Trace
    trace_id            String,
    span_id             String,
    parent_id           String,

    -- Host
    host_id             LowCardinality(String),
    hostname            LowCardinality(String),

    -- Tenant
    tenant_id           LowCardinality(String),

    -- Metrics
    bytes               UInt64,
    packets             UInt64,
    latency_ns          UInt64,
    direction           LowCardinality(UInt8),

    -- Exception
    exception           String,

    -- Tags (JSON)
    tags                String,

    -- 索引字段
    src_ip_bloom        String MATERIALIZED src_ip,
    dst_ip_bloom        String MATERIALIZED dst_ip,
    trace_id_bloom      String MATERIALIZED trace_id,

    -- 索引标记
    idx_src_ip          UInt32 MATERIALIZED IPv4StringToNumOrDefault(src_ip, 0),
    idx_dst_ip          UInt32 MATERIALIZED IPv4StringToNumOrDefault(dst_ip, 0)
)
ENGINE = %s
PARTITION BY %s
ORDER BY (tenant_id, timestamp, flow_id)
SETTINGS
    index_granularity = 8192,
    min_bytes_for_wide_part = '10M',
    min_rows_for_wide_part = 100000
%s;
`, sanitizedDB, sanitizedTable, engine, sanitizedPartitionBy, generateTTLWithHotCold(cfg.FlowTTL, cfg.HotDays, cfg.ColdVolume))
}

// GenerateCreateTracesTable 生成 traces 表 DDL
func GenerateCreateTracesTable(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	engine := "MergeTree()"
	if cfg.Replicated {
		sanitizedDB := sanitizeIdentifier(cfg.Database)
		sanitizedTable := sanitizeIdentifier(TableTraces)
		engine = fmt.Sprintf("ReplicatedMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable)
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedTable := sanitizeIdentifier(TableTraces)
	sanitizedPartitionBy := sanitizePartitionExpression(cfg.PartitionBy)

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           Int64,
    timestamp_dt        DateTime DEFAULT toDateTime(timestamp / 1000000000),

    trace_id            String,
    span_id             String,
    parent_id           String,
    trace_state         String,

    -- 关联 Flow
    flow_id             UInt32,

    -- Span 信息
    name                String,
    kind                LowCardinality(UInt8),
    start_time_ns       Int64,
    end_time_ns         Int64,
    duration_ns         Int64,

    -- 状态
    status_code         LowCardinality(UInt8),
    status_message      String,

    -- 属性 (JSON)
    attributes          String,

    -- 关联资源
    service_name        LowCardinality(String),
    namespace           LowCardinality(String),
    pod                 String,
    node                LowCardinality(String),

    -- Tenant
    tenant_id           LowCardinality(String)
)
ENGINE = %s
PARTITION BY %s
ORDER BY (tenant_id, trace_id, span_id)
SETTINGS
    index_granularity = 8192
%s;
`, sanitizedDB, sanitizedTable, engine, sanitizedPartitionBy, generateTTLWithHotCold(cfg.TraceTTL, cfg.HotDays, cfg.ColdVolume))
}

// GenerateCreateEventsTable 生成 events 表 DDL
func GenerateCreateEventsTable(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	engine := "MergeTree()"
	if cfg.Replicated {
		sanitizedDB := sanitizeIdentifier(cfg.Database)
		sanitizedTable := sanitizeIdentifier(TableEvents)
		engine = fmt.Sprintf("ReplicatedMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable)
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedTable := sanitizeIdentifier(TableEvents)
	sanitizedPartitionBy := sanitizePartitionExpression(cfg.PartitionBy)

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           Int64,
    timestamp_dt        DateTime DEFAULT toDateTime(timestamp / 1000000000),

    event_id            String,
    event_type          LowCardinality(String),
    severity            LowCardinality(UInt8),

    -- 来源
    source              LowCardinality(String),
    source_id           String,

    -- 内容
    title               String,
    message             String,
    details             String,  -- JSON

    -- 关联资源
    resource_type       LowCardinality(String),
    resource_id         String,
    namespace           LowCardinality(String),
    service             LowCardinality(String),
    pod                 String,
    node                LowCardinality(String),

    -- Tenant
    tenant_id           LowCardinality(String)
)
ENGINE = %s
PARTITION BY %s
ORDER BY (tenant_id, timestamp, event_id)
SETTINGS
    index_granularity = 8192
%s;
`, sanitizedDB, sanitizedTable, engine, sanitizedPartitionBy, generateTTLWithHotCold(cfg.EventTTL, cfg.HotDays, cfg.ColdVolume))
}

// GenerateCreateAggregationTables 生成聚合表 DDL
func GenerateCreateAggregationTables(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)

	// 为每个聚合表选择适当的引擎
	engine1m := "SummingMergeTree()"
	engine1h := "SummingMergeTree()"
	engine1d := "SummingMergeTree()"
	if cfg.Replicated {
		sanitizedTable1m := sanitizeIdentifier(TableFlows1m)
		sanitizedTable1h := sanitizeIdentifier(TableFlows1h)
		sanitizedTable1d := sanitizeIdentifier(TableFlows1d)
		engine1m = fmt.Sprintf("ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable1m)
		engine1h = fmt.Sprintf("ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable1h)
		engine1d = fmt.Sprintf("ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable1d)
	}

	sanitizedTable1m := sanitizeIdentifier(TableFlows1m)
	sanitizedTable1h := sanitizeIdentifier(TableFlows1h)
	sanitizedTable1d := sanitizeIdentifier(TableFlows1d)

	return fmt.Sprintf(`
-- 1分钟聚合表
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           DateTime,
    tenant_id           LowCardinality(String),

    -- 维度
    src_ip              String,
    dst_ip              String,
    src_port            UInt16,
    dst_port            UInt16,
    protocol            LowCardinality(UInt8),
    l7_protocol         LowCardinality(UInt8),
    namespace           LowCardinality(String),
    service             LowCardinality(String),
    pod                 String,
    node                LowCardinality(String),

    -- 聚合指标
    flow_count          UInt64,
    bytes_sum           UInt64,
    packets_sum         UInt64,
    latency_avg         Float64,
    latency_p50         Float64,
    latency_p95         Float64,
    latency_p99         Float64,
    error_count         UInt64
)
ENGINE = %s
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (tenant_id, timestamp, src_ip, dst_ip, protocol)
SETTINGS index_granularity = 8192;

-- 1小时聚合表
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           DateTime,
    tenant_id           LowCardinality(String),

    -- 维度
    src_ip              String,
    dst_ip              String,
    protocol            LowCardinality(UInt8),
    l7_protocol         LowCardinality(UInt8),
    namespace           LowCardinality(String),
    service             LowCardinality(String),
    node                LowCardinality(String),

    -- 聚合指标
    flow_count          UInt64,
    bytes_sum           UInt64,
    packets_sum         UInt64,
    latency_avg         Float64,
    latency_p95         Float64,
    latency_p99         Float64,
    error_count         UInt64
)
ENGINE = %s
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (tenant_id, timestamp, src_ip, dst_ip, protocol)
SETTINGS index_granularity = 8192;

-- 1天聚合表
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           Date,
    tenant_id           LowCardinality(String),

    -- 维度
    src_ip              String,
    dst_ip              String,
    protocol            LowCardinality(UInt8),
    namespace           LowCardinality(String),
    service             LowCardinality(String),
    node                LowCardinality(String),

    -- 聚合指标
    flow_count          UInt64,
    bytes_sum           UInt64,
    packets_sum         UInt64,
    latency_avg         Float64,
    error_count         UInt64
)
ENGINE = %s
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, timestamp, src_ip, dst_ip, protocol)
SETTINGS index_granularity = 8192;
`, sanitizedDB, sanitizedTable1m, engine1m, sanitizedDB, sanitizedTable1h, engine1h, sanitizedDB, sanitizedTable1d, engine1d)
}

// GenerateCreateTopologyTable 生成拓扑表 DDL
func GenerateCreateTopologyTable(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedTable := sanitizeIdentifier(TableTopology)

	engine := "SummingMergeTree()"
	if cfg.Replicated {
		engine = fmt.Sprintf("ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/%s/%s', '{replica}')", sanitizedDB, sanitizedTable)
	}

	return fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s (
    timestamp           DateTime,
    tenant_id           LowCardinality(String),

    -- 源节点
    src_type            LowCardinality(String),
    src_id              String,
    src_name            String,
    src_namespace       LowCardinality(String),

    -- 目标节点
    dst_type            LowCardinality(String),
    dst_id              String,
    dst_name            String,
    dst_namespace       LowCardinality(String),

    -- 连接信息
    protocol            LowCardinality(UInt8),
    port                UInt16,

    -- 统计
    flow_count          UInt64,
    bytes_sum           UInt64,
    packets_sum         UInt64,
    latency_avg         Float64,
    error_count         UInt64
)
ENGINE = %s
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (tenant_id, timestamp, src_id, dst_id, protocol)
SETTINGS index_granularity = 8192;
`, sanitizedDB, sanitizedTable, engine)
}

// GenerateCreateMaterializedViews 生成物化视图 DDL
func GenerateCreateMaterializedViews(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedFlows := sanitizeIdentifier(TableFlows)
	sanitizedFlows1m := sanitizeIdentifier(TableFlows1m)
	sanitizedFlows1h := sanitizeIdentifier(TableFlows1h)
	sanitizedTopology := sanitizeIdentifier(TableTopology)
	sanitizedFlowsMV1m := sanitizeIdentifier(TableFlowsMV1m)
	sanitizedFlowsMV1h := sanitizeIdentifier(TableFlowsMV1h)
	sanitizedTopologyMV := sanitizeIdentifier(TableTopologyMV)

	return fmt.Sprintf(`
-- 1分钟聚合物化视图
CREATE MATERIALIZED VIEW IF NOT EXISTS %s.%s
TO %s.%s AS
SELECT
    toStartOfMinute(timestamp_dt) AS timestamp,
    tenant_id,
    src_ip,
    dst_ip,
    src_port,
    dst_port,
    protocol,
    l7_protocol,
    namespace,
    service,
    pod,
    node,
    count() AS flow_count,
    sum(bytes) AS bytes_sum,
    sum(packets) AS packets_sum,
    avg(latency_ns) AS latency_avg,
    quantile(0.50)(latency_ns) AS latency_p50,
    quantile(0.95)(latency_ns) AS latency_p95,
    quantile(0.99)(latency_ns) AS latency_p99,
    countIf(status_code >= 400 OR grpc_status > 0) AS error_count
FROM %s.%s
GROUP BY
    timestamp, tenant_id, src_ip, dst_ip, src_port, dst_port,
    protocol, l7_protocol, namespace, service, pod, node;

-- 1小时聚合物化视图
CREATE MATERIALIZED VIEW IF NOT EXISTS %s.%s
TO %s.%s AS
SELECT
    toStartOfHour(timestamp_dt) AS timestamp,
    tenant_id,
    src_ip,
    dst_ip,
    protocol,
    l7_protocol,
    namespace,
    service,
    node,
    count() AS flow_count,
    sum(bytes) AS bytes_sum,
    sum(packets) AS packets_sum,
    avg(latency_ns) AS latency_avg,
    quantile(0.95)(latency_ns) AS latency_p95,
    quantile(0.99)(latency_ns) AS latency_p99,
    countIf(status_code >= 400 OR grpc_status > 0) AS error_count
FROM %s.%s
GROUP BY
    timestamp, tenant_id, src_ip, dst_ip, protocol, l7_protocol,
    namespace, service, node;

-- 拓扑物化视图
CREATE MATERIALIZED VIEW IF NOT EXISTS %s.%s
TO %s.%s AS
SELECT
    toStartOfMinute(timestamp_dt) AS timestamp,
    tenant_id,
    'service' AS src_type,
    service AS src_id,
    service AS src_name,
    namespace AS src_namespace,
    'ip' AS dst_type,
    dst_ip AS dst_id,
    dst_ip AS dst_name,
    '' AS dst_namespace,
    protocol,
    dst_port AS port,
    count() AS flow_count,
    sum(bytes) AS bytes_sum,
    sum(packets) AS packets_sum,
    avg(latency_ns) AS latency_avg,
    countIf(status_code >= 400 OR grpc_status > 0) AS error_count
FROM %s.%s
WHERE service != ''
GROUP BY
    timestamp, tenant_id, service, namespace, dst_ip, protocol, dst_port;
`, sanitizedDB, sanitizedFlowsMV1m, sanitizedDB, sanitizedFlows1m, sanitizedDB, sanitizedFlows,
	sanitizedDB, sanitizedFlowsMV1h, sanitizedDB, sanitizedFlows1h, sanitizedDB, sanitizedFlows,
	sanitizedDB, sanitizedTopologyMV, sanitizedDB, sanitizedTopology, sanitizedDB, sanitizedFlows)
}

// GenerateCreateIndexes 生成索引 DDL
func GenerateCreateIndexes(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)
	sanitizedFlows := sanitizeIdentifier(TableFlows)

	return fmt.Sprintf(`
-- Bloom Filter 索引 (加速高基数字段查询)
ALTER TABLE %s.%s ADD INDEX idx_src_ip_bloom src_ip_bloom TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_dst_ip_bloom dst_ip_bloom TYPE bloom_filter(0.01) GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_trace_id_bloom trace_id_bloom TYPE bloom_filter(0.01) GRANULARITY 4;

-- Skip Index (加速特定查询模式)
ALTER TABLE %s.%s ADD INDEX idx_status_code status_code TYPE set(100) GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_l7_protocol l7_protocol TYPE set(20) GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_namespace namespace TYPE set(100) GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_service service TYPE set(500) GRANULARITY 4;

-- MinMax 索引 (加速范围查询)
ALTER TABLE %s.%s ADD INDEX idx_latency_minmax latency_ns TYPE minmax GRANULARITY 4;
ALTER TABLE %s.%s ADD INDEX idx_bytes_minmax bytes TYPE minmax GRANULARITY 4;
`, sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows,
		sanitizedDB, sanitizedFlows)
}

// GenerateAllDDL 生成所有 DDL
func GenerateAllDDL(cfg *SchemaConfig) string {
	if cfg == nil {
		cfg = DefaultSchemaConfig()
	}

	// P2-02 修复: 验证配置参数，防止 SQL 注入
	if err := validateSchemaConfig(cfg); err != nil {
		cfg = DefaultSchemaConfig()
	}

	sanitizedDB := sanitizeIdentifier(cfg.Database)

	var ddl strings.Builder

	ddl.WriteString("-- CloudFlow ClickHouse Schema\n")
	ddl.WriteString("-- Generated by schema generator\n\n")

	ddl.WriteString("-- 创建数据库\n")
	ddl.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;\n\n", sanitizedDB))

	ddl.WriteString("-- 主表\n")
	ddl.WriteString(GenerateCreateFlowsTable(cfg))
	ddl.WriteString("\n")
	ddl.WriteString(GenerateCreateTracesTable(cfg))
	ddl.WriteString("\n")
	ddl.WriteString(GenerateCreateEventsTable(cfg))
	ddl.WriteString("\n")

	ddl.WriteString("-- 聚合表\n")
	ddl.WriteString(GenerateCreateAggregationTables(cfg))
	ddl.WriteString("\n")

	ddl.WriteString("-- 拓扑表\n")
	ddl.WriteString(GenerateCreateTopologyTable(cfg))
	ddl.WriteString("\n")

	ddl.WriteString("-- 物化视图\n")
	ddl.WriteString(GenerateCreateMaterializedViews(cfg))
	ddl.WriteString("\n")

	ddl.WriteString("-- 索引\n")
	ddl.WriteString(GenerateCreateIndexes(cfg))

	return ddl.String()
}

// generateTTL 生成 TTL 子句（向后兼容）
func generateTTL(days int) string {
	if days <= 0 {
		return ""
	}
	return fmt.Sprintf("TTL timestamp_dt + INTERVAL %d DAY DELETE", days)
}

// generateTTLWithHotCold 生成包含冷热分离的 TTL 子句
func generateTTLWithHotCold(totalDays int, hotDays int, coldVolume string) string {
	if totalDays <= 0 {
		return ""
	}
	
	ttls := []string{}
	
	// 热数据 → 冷数据的移动策略
	if hotDays > 0 && hotDays < totalDays {
		if coldVolume != "default" {
			ttls = append(ttls, fmt.Sprintf("timestamp_dt + INTERVAL %d DAY TO VOLUME '%s'", hotDays, sanitizeIdentifier(coldVolume)))
		}
	}
	
	// 删除策略
	ttls = append(ttls, fmt.Sprintf("timestamp_dt + INTERVAL %d DAY DELETE", totalDays))
	
	return fmt.Sprintf("TTL %s", strings.Join(ttls, ", "))
}

// ============================================================================
// P2-02 修复: SQL 注入防护辅助函数
// ============================================================================

// validateSchemaConfig 验证 SchemaConfig 配置参数，防止 SQL 注入
func validateSchemaConfig(cfg *SchemaConfig) error {
	if cfg == nil {
		return fmt.Errorf("cfg 不能为 nil")
	}

	// 验证 Database 名称（只允许字母、数字、下划线）
	if !isValidIdentifier(cfg.Database) {
		return fmt.Errorf("无效的数据库名: %s", cfg.Database)
	}

	// 验证 PartitionBy 表达式（只允许合法的 ClickHouse 函数调用）
	if !isValidPartitionExpression(cfg.PartitionBy) {
		return fmt.Errorf("无效的分区表达式: %s", cfg.PartitionBy)
	}

	// 验证 ColdVolume 名称
	if cfg.ColdVolume != "" && !isValidIdentifier(cfg.ColdVolume) {
		return fmt.Errorf("无效的冷存储卷名: %s", cfg.ColdVolume)
	}

	return nil
}

// isValidIdentifier 验证标识符是否合法（只允许字母、数字、下划线）
func isValidIdentifier(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// sanitizeIdentifier 清理标识符，移除非法字符
func sanitizeIdentifier(name string) string {
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result.WriteRune(c)
		}
	}
	sanitized := result.String()
	if sanitized == "" {
		return "invalid"
	}
	return sanitized
}

// isValidPartitionExpression 验证分区表达式是否合法
func isValidPartitionExpression(expr string) bool {
	if len(expr) == 0 || len(expr) > 100 {
		return false
	}
	// 只允许特定的 ClickHouse 分区函数
	allowedPatterns := []string{
		"toYYYYMMDD(timestamp)",
		"toYYYYMM(timestamp)",
		"toMonday(timestamp)",
		"toStartOfMonth(timestamp)",
		"toStartOfQuarter(timestamp)",
		"toYear(timestamp)",
	}
	for _, pattern := range allowedPatterns {
		if expr == pattern {
			return true
		}
	}
	return false
}

// sanitizePartitionExpression 清理分区表达式，返回安全的默认值
func sanitizePartitionExpression(expr string) string {
	if isValidPartitionExpression(expr) {
		return expr
	}
	// 返回默认的安全表达式
	return "toYYYYMMDD(timestamp)"
}
