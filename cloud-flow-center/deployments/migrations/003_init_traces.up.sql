-- =============================================================================
-- 003_init_traces.up.sql
-- 链路追踪数据表
-- =============================================================================

CREATE TABLE IF NOT EXISTS traces (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    probe_id VARCHAR(64) NOT NULL,
    ts TIMESTAMP NOT NULL,
    payload JSON,
    span_id VARCHAR(128),
    INDEX idx_probe_id (probe_id),
    INDEX idx_ts (ts),
    INDEX idx_span_id (span_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS (ts) (
    PARTITION p_default VALUES LESS THAN ('2027-01-01')
);
