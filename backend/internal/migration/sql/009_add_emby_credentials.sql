-- 009_add_emby_credentials.sql
-- 为 emby_configs 表添加用户名和密码字段，用于删除操作的用户认证

-- +goose Up
ALTER TABLE emby_configs ADD COLUMN username TEXT NOT NULL DEFAULT '';
ALTER TABLE emby_configs ADD COLUMN password TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE emby_configs DROP COLUMN username;
ALTER TABLE emby_configs DROP COLUMN password;
