-- CloudFlow ClickHouse 幂等性迁移脚本
-- 将 ebpf_events 表从 MergeTree 升级为 ReplacingMergeTree
-- 添加 tenant_id 列（data-ingest-service.py 需要）
--
-- 执行方式：clickhouse-client --user default < scripts/clickhouse_dedup_migration.sql
-- 或在代码中通过 HTTP 接口执行

-- ========================================
-- 第1步：添加 tenant_id 列（如果不存在）
-- ========================================
ALTER TABLE cloudflow.ebpf_events
ADD COLUMN IF NOT EXISTS tenant_id String DEFAULT 'default';

-- ========================================
-- 第2步：创建新表（ReplacingMergeTree + 幂等 ORDER BY）
-- ========================================
CREATE TABLE IF NOT EXISTS cloudflow.ebpf_events_v2
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

-- ========================================
-- 第3步：迁移数据（可选 — 如果需要保留旧数据）
-- ========================================
-- INSERT INTO cloudflow.ebpf_events_v2
-- SELECT * FROM cloudflow.ebpf_events;

-- ========================================
-- 第4步：重命名表（原子操作）
-- ========================================
-- 方法A：RENAME（推荐 — 保留旧表作备份）
-- RENAME TABLE cloudflow.ebpf_events TO cloudflow.ebpf_events_old;
-- RENAME TABLE cloudflow.ebpf_events_v2 TO cloudflow.ebpf_events;

-- 方法B：直接替换（谨慎 — 会丢失旧表数据）
-- DROP TABLE IF EXISTS cloudflow.ebpf_events;
-- RENAME TABLE cloudflow.ebpf_events_v2 TO cloudflow.ebpf_events;

-- ========================================
-- 第5步：定期 OPTIMIZE 清理重复数据
-- ========================================
-- 建议通过 cron 定期执行（每小时）：
-- clickhouse-client --user default --query "OPTIMIZE TABLE cloudflow.ebpf_events FINAL"

-- ========================================
-- 回滚脚本
-- ========================================
-- RENAME TABLE cloudflow.ebpf_events TO cloudflow.ebpf_events_v2;
-- RENAME TABLE cloudflow.ebpf_events_old TO cloudflow.ebpf_events;
