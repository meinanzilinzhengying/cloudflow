-- =============================================================================
-- 004_init_profiling.up.sql
-- 性能分析数据表
-- =============================================================================

CREATE TABLE IF NOT EXISTS profiling (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    probe_id VARCHAR(64) NOT NULL,
    ts TIMESTAMP NOT NULL,
    payload JSON,
    type VARCHAR(50),
    INDEX idx_probe_id (probe_id),
    INDEX idx_ts (ts),
    INDEX idx_type (type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS (ts) (
    PARTITION p_default VALUES LESS THAN ('2027-01-01')
);
