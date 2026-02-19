-- +goose Up
-- +goose StatementBegin
-- 更新 webhook_configs 表的 file_path 字段，使其可为空
-- SQLite 不支持直接修改列约束，需要重建表

-- 1. 创建新表
CREATE TABLE IF NOT EXISTS webhook_configs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symedia_url TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    file_path TEXT,
    secret TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
-- 2. 复制数据（如果旧表存在）
INSERT INTO webhook_configs_new (id, symedia_url, auth_token, repo_url, branch, file_path, secret, webhook_url, created_at, updated_at)
SELECT id, symedia_url, auth_token, repo_url, branch, file_path, secret, webhook_url, created_at, updated_at
FROM webhook_configs
WHERE EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name='webhook_configs');
-- +goose StatementEnd

-- +goose StatementBegin
-- 3. 删除旧表（如果存在）
DROP TABLE IF EXISTS webhook_configs;
-- +goose StatementEnd

-- +goose StatementBegin
-- 4. 重命名新表
ALTER TABLE webhook_configs_new RENAME TO webhook_configs;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- 回滚：将 file_path 改回 NOT NULL（需要重建表）
CREATE TABLE IF NOT EXISTS webhook_configs_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symedia_url TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    file_path TEXT NOT NULL,
    secret TEXT NOT NULL,
    webhook_url TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO webhook_configs_old (id, symedia_url, auth_token, repo_url, branch, file_path, secret, webhook_url, created_at, updated_at)
SELECT id, symedia_url, auth_token, repo_url, branch, COALESCE(file_path, ''), secret, webhook_url, created_at, updated_at
FROM webhook_configs;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS webhook_configs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE webhook_configs_old RENAME TO webhook_configs;
-- +goose StatementEnd
