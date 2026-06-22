-- CloudFlow ClickHouse 全新初始化脚本
-- VM1:192.168.58.130

CREATE DATABASE IF NOT EXISTS cloudflow;

-- ============================================
-- 核心表：ebpf_events (ReplacingMergeTree 幂等)
-- ============================================
CREATE TABLE IF NOT EXISTS cloudflow.ebpf_events
(
    timestamp   DateTime,
    probe_id    String,
    category    String,
    event_type  String,
    src_ip      String,
    dst_ip      String,
    src_port    UInt16,
    dst_port    UInt16,
    protocol    String,
    bytes       UInt64,
    packets     UInt64,
    latency_ms  Float64,
    service     String,
    details     String,
    tags        String,
    tenant_id   String DEFAULT 'default'
)
ENGINE = ReplacingMergeTree()
ORDER BY (probe_id, category, event_type, src_ip, dst_ip, src_port, dst_port, protocol, timestamp)
TTL timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- ============================================
-- 物化视图：按类别分表 (自动从 ebpf_events 派生)
-- ============================================
CREATE TABLE IF NOT EXISTS cloudflow.network_events
(
    timestamp   DateTime,
    probe_id    String,
    event_type  String,
    src_ip      String,
    dst_ip      String,
    src_port    UInt16,
    dst_port    UInt16,
    protocol    String,
    bytes       UInt64,
    packets     UInt64,
    latency_ms  Float64,
    service     String,
    tenant_id   String
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS cloudflow.file_events
(
    timestamp   DateTime,
    probe_id    String,
    event_type  String,
    src_ip      String,
    service     String,
    details     String,
    tenant_id   String
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS cloudflow.process_events
(
    timestamp   DateTime,
    probe_id    String,
    event_type  String,
    src_ip      String,
    service     String,
    details     String,
    tenant_id   String
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

CREATE TABLE IF NOT EXISTS cloudflow.syscall_events
(
    timestamp   DateTime,
    probe_id    String,
    event_type  String,
    src_ip      String,
    details     String,
    tenant_id   String
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

-- ============================================
-- 物化视图定义 (自动分流)
-- ============================================
DROP VIEW IF EXISTS cloudflow.mv_network_events;
CREATE MATERIALIZED VIEW cloudflow.mv_network_events
TO cloudflow.network_events
AS SELECT timestamp, probe_id, event_type, src_ip, dst_ip, src_port, dst_port, protocol, bytes, packets, latency_ms, service, tenant_id
FROM cloudflow.ebpf_events
WHERE category IN ('network', 'protocol');

DROP VIEW IF EXISTS cloudflow.mv_file_events;
CREATE MATERIALIZED VIEW cloudflow.mv_file_events
TO cloudflow.file_events
AS SELECT timestamp, probe_id, event_type, src_ip, service, details, tenant_id
FROM cloudflow.ebpf_events
WHERE category = 'file';

DROP VIEW IF EXISTS cloudflow.mv_process_events;
CREATE MATERIALIZED VIEW cloudflow.mv_process_events
TO cloudflow.process_events
AS SELECT timestamp, probe_id, event_type, src_ip, service, details, tenant_id
FROM cloudflow.ebpf_events
WHERE category = 'process';

DROP VIEW IF EXISTS cloudflow.mv_syscall_events;
CREATE MATERIALIZED VIEW cloudflow.mv_syscall_events
TO cloudflow.syscall_events
AS SELECT timestamp, probe_id, event_type, src_ip, details, tenant_id
FROM cloudflow.ebpf_events
WHERE category = 'syscall';

-- ============================================
-- flows 表 (query-service /flows 端点使用)
-- ============================================
CREATE TABLE IF NOT EXISTS cloudflow.flows
(
    timestamp       DateTime,
    probe_id        String,
    src_ip          String,
    dst_ip          String,
    src_port        UInt16,
    dst_port        UInt16,
    protocol        String,
    bytes           UInt64,
    packets         UInt64,
    syn_flag        UInt8 DEFAULT 0,
    fin_flag        UInt8 DEFAULT 0,
    rst_flag        UInt8 DEFAULT 0,
    http_method     String DEFAULT '',
    http_host       String DEFAULT '',
    http_url        String DEFAULT '',
    http_status     UInt16 DEFAULT 0,
    dns_query       String DEFAULT '',
    dns_type        String DEFAULT '',
    service         String DEFAULT '',
    tenant_id       String DEFAULT 'default'
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

-- ============================================
-- host_metrics 表 (query-service /metrics-data 端点使用)
-- ============================================
CREATE TABLE IF NOT EXISTS cloudflow.host_metrics
(
    timestamp       DateTime,
    probe_id        String,
    cpu_percent     Float64,
    memory_percent  Float64,
    disk_percent    Float64,
    net_rx_bytes    UInt64,
    net_tx_bytes    UInt64,
    disk_read_bytes UInt64,
    disk_write_bytes UInt64,
    tenant_id       String DEFAULT 'default'
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp)
TTL timestamp + INTERVAL 30 DAY;

-- ============================================
-- metrics 表 (兼容旧 query-service)
-- ============================================
CREATE TABLE IF NOT EXISTS cloudflow.metrics
(
    timestamp   DateTime,
    probe_id    String,
    name        String,
    value       Float64,
    tags        String DEFAULT '',
    tenant_id   String DEFAULT 'default'
)
ENGINE = MergeTree()
ORDER BY (probe_id, timestamp, name)
TTL timestamp + INTERVAL 30 DAY;

-- ============================================
-- 定期清理重复数据 (建议配置 cron)
-- ============================================
SELECT 'ClickHouse setup complete!' AS status;
