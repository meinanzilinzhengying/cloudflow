-- CloudFlow Tenant Service Database Migrations
-- Target: TiDB (cloudflow_tenant)

-- Migration 001: Initial schema for tenants and projects
CREATE DATABASE IF NOT EXISTS cloudflow_tenant CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE cloudflow_tenant;

-- Tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id VARCHAR(36) PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    display_name VARCHAR(256),
    plan ENUM('free', 'pro', 'enterprise') NOT NULL DEFAULT 'free',
    status ENUM('active', 'suspended', 'deleted') NOT NULL DEFAULT 'active',
    max_projects INT NOT NULL DEFAULT 3,
    max_users INT NOT NULL DEFAULT 5,
    max_agents INT NOT NULL DEFAULT 10,
    settings JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_name (name),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Projects table
CREATE TABLE IF NOT EXISTS projects (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL,
    name VARCHAR(128) NOT NULL,
    display_name VARCHAR(256),
    description TEXT,
    status ENUM('active', 'archived') NOT NULL DEFAULT 'active',
    cluster_config JSON,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Quotas table (per tenant)
CREATE TABLE IF NOT EXISTS quotas (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id VARCHAR(36) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    limit_value BIGINT NOT NULL DEFAULT 0,
    used_value BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_tenant_resource (tenant_id, resource_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
