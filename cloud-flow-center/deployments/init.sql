-- =============================================================================
-- Cloud Flow Center - 数据库初始化脚本
-- 用于 TiDB (兼容 MySQL 协议)
-- =============================================================================

CREATE DATABASE IF NOT EXISTS cloud_flow DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
USE cloud_flow;

-- =============================================================================
-- 用户管理表
-- =============================================================================
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- =============================================================================
-- 指标数据表
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

-- =============================================================================
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

-- =============================================================================
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

-- =============================================================================
-- 探针信息表
-- =============================================================================
CREATE TABLE IF NOT EXISTS probes (
    edge_node_id VARCHAR(128) PRIMARY KEY,
    payload JSON,
    updated_at BIGINT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- =============================================================================
-- 告警历史记录表
-- =============================================================================
CREATE TABLE IF NOT EXISTS alert_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    alert_id VARCHAR(128) NOT NULL,
    rule_id VARCHAR(128) NOT NULL,
    rule_name VARCHAR(255),
    severity VARCHAR(20),
    message TEXT,
    labels JSON,
    value DOUBLE,
    threshold DOUBLE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP NULL,
    INDEX idx_rule_id (rule_id),
    INDEX idx_severity (severity),
    INDEX idx_resolved (resolved),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE COLUMNS (created_at) (
    PARTITION p_default VALUES LESS THAN ('2027-01-01')
);

-- =============================================================================
-- 用户偏好设置表
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_preferences (
    username VARCHAR(50) PRIMARY KEY,
    theme VARCHAR(20) DEFAULT 'light',
    language VARCHAR(10) DEFAULT 'zh-CN',
    page_size INT DEFAULT 20,
    refresh_interval INT DEFAULT 30,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- =============================================================================
-- 业务管理表
-- =============================================================================
CREATE TABLE IF NOT EXISTS businesses (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) DEFAULT 'active',
    owner VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- =============================================================================
-- 服务管理表
-- =============================================================================
CREATE TABLE IF NOT EXISTS services (
    id VARCHAR(64) PRIMARY KEY,
    business_id VARCHAR(64),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) DEFAULT 'running',
    endpoints JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_business_id (business_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- =============================================================================
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

-- =============================================================================
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
