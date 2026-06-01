-- Rollback Migration 003

USE cloudflow_alert;

DROP TABLE IF EXISTS notification_history;
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS alert_rules;

DROP DATABASE IF EXISTS cloudflow_alert;
