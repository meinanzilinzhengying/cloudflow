-- =============================================================================
-- 005_init_probes.up.sql
-- 探针（边缘节点）信息表
-- =============================================================================

CREATE TABLE IF NOT EXISTS probes (
    edge_node_id VARCHAR(128) PRIMARY KEY,
    payload JSON,
    updated_at BIGINT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
