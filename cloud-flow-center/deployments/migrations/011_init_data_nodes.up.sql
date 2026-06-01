-- =============================================================================
-- 011_init_data_nodes.up.sql
-- 数据节点管理表
-- =============================================================================

CREATE TABLE IF NOT EXISTS data_nodes (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    host VARCHAR(255),
    port INT,
    type VARCHAR(50) DEFAULT 'tidb',
    status VARCHAR(20) DEFAULT 'online',
    config JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
