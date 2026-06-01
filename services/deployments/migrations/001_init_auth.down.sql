-- Rollback Migration 001

USE cloudflow_auth;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;

DROP DATABASE IF EXISTS cloudflow_auth;
