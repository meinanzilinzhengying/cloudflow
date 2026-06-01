-- =============================================================================
-- 007_init_user_preferences.up.sql
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
