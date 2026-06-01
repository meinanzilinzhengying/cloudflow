-- =============================================================================
-- 006_init_alert_history.up.sql
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
