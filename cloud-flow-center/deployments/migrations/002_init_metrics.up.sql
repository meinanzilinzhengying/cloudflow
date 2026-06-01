-- =============================================================================
-- 002_init_metrics.up.sql
-- 指标数据表（网络流量、CPU、内存、磁盘等）
-- =============================================================================

CREATE TABLE IF NOT EXISTS metrics (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    probe_id VARCHAR(64) NOT NULL,
    ts TIMESTAMP NOT NULL,
    src_ip VARCHAR(45),
    dst_ip VARCHAR(45),
    src_port INT,
    dst_port INT,
    protocol VARCHAR(20),
    bytes BIGINT DEFAULT 0,
    packets INT DEFAULT 0,
    latency BIGINT DEFAULT 0,
    cpu_usage DOUBLE DEFAULT 0,
    memory_usage DOUBLE DEFAULT 0,
    disk_usage DOUBLE DEFAULT 0,
    INDEX idx_probe_id (probe_id),
    INDEX idx_ts (ts),
    INDEX idx_protocol (protocol),
    INDEX idx_src_ip (src_ip),
    INDEX idx_dst_ip (dst_ip)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS (ts) (
    PARTITION p_default VALUES LESS THAN ('2027-01-01')
);
