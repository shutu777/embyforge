-- +goose Up

-- Webhook配置表
CREATE TABLE IF NOT EXISTS webhook_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symedia_url VARCHAR(500) NOT NULL,
    auth_token TEXT NOT NULL,
    repo_url VARCHAR(500) NOT NULL,
    branch VARCHAR(100) NOT NULL DEFAULT 'main',
    file_path VARCHAR(500) NOT NULL,
    secret VARCHAR(500) NOT NULL,
    webhook_url VARCHAR(500) NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);

-- Webhook日志表
CREATE TABLE IF NOT EXISTS webhook_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source VARCHAR(50) NOT NULL,
    repo_name VARCHAR(500),
    branch VARCHAR(100),
    commit_sha VARCHAR(100),
    success BOOLEAN NOT NULL,
    error_msg TEXT,
    created_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_created_at ON webhook_logs(created_at);

-- 在系统配置表中添加Symedia配置项
INSERT INTO system_configs (key, value, description, created_at, updated_at)
VALUES 
    ('symedia_url', '', 'Symedia服务地址', datetime('now'), datetime('now')),
    ('symedia_auth_token', '', 'Symedia Authorization令牌（加密存储）', datetime('now'), datetime('now'))
ON CONFLICT(key) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS webhook_logs;
DROP TABLE IF EXISTS webhook_configs;
DELETE FROM system_configs WHERE key IN ('symedia_url', 'symedia_auth_token');
