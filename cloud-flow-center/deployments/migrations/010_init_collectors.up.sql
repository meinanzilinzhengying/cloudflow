-- =============================================================================
-- 010_init_collectors.up.sql
-- 采集器管理表
-- =============================================================================

CREATE TABLE IF NOT EXISTS collectors (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    host VARCHAR(255),
    port INT,
    status VARCHAR(20) DEFAULT 'running',
    config JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
