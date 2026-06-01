-- Rollback Migration 002

USE cloudflow_tenant;

DROP TABLE IF EXISTS quotas;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS tenants;

DROP DATABASE IF EXISTS cloudflow_tenant;
