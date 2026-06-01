-- CloudFlow Alert Service Database Migrations
-- Target: TiDB (cloudflow_alert)

-- Migration 001: Initial schema for alert rules and history
CREATE DATABASE IF NOT EXISTS cloudflow_alert CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE cloudflow_alert;

-- Alert Rules table
CREATE TABLE IF NOT EXISTS alert_rules (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(256) NOT NULL,
    description TEXT,
    tenant_id VARCHAR(36) NOT NULL,
    project_id VARCHAR(36),
    alert_type VARCHAR(64) NOT NULL,
    severity ENUM('critical', 'high', 'medium', 'low', 'info') NOT NULL DEFAULT 'medium',
    condition JSON NOT NULL,
    duration INT NOT NULL DEFAULT 60,
    threshold DOUBLE NOT NULL,
    comparator ENUM('gt', 'gte', 'lt', 'lte', 'eq', 'neq') NOT NULL DEFAULT 'gt',
    metric_name VARCHAR(128),
    target_scope JSON,
    tags JSON,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    notify_channels JSON,
    created_by VARCHAR(36),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_project_id (project_id),
    INDEX idx_is_active (is_active),
    INDEX idx_alert_type (alert_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Alert Events table (firing/resolved alerts)
CREATE TABLE IF NOT EXISTS alert_events (
    id VARCHAR(36) PRIMARY KEY,
    rule_id VARCHAR(36) NOT NULL,
    tenant_id VARCHAR(36) NOT NULL,
    project_id VARCHAR(36),
    status ENUM('firing', 'resolved', 'acknowledged', 'silenced') NOT NULL DEFAULT 'firing',
    severity ENUM('critical', 'high', 'medium', 'low', 'info') NOT NULL,
    title VARCHAR(512) NOT NULL,
    message TEXT,
    metric_value DOUBLE,
    threshold_value DOUBLE,
    evaluation_time TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP NULL,
    acknowledged_at TIMESTAMP NULL,
    acknowledged_by VARCHAR(36),
    notified_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_rule_id (rule_id),
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_status (status),
    INDEX idx_severity (severity),
    INDEX idx_evaluation_time (evaluation_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Notification History table
CREATE TABLE IF NOT EXISTS notification_history (
    id VARCHAR(36) PRIMARY KEY,
    event_id VARCHAR(36) NOT NULL,
    channel_type ENUM('email', 'webhook', 'slack', 'dingtalk', 'feishu') NOT NULL,
    channel_target VARCHAR(512) NOT NULL,
    status ENUM('sent', 'failed', 'pending') NOT NULL DEFAULT 'pending',
    response TEXT,
    sent_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_event_id (event_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
