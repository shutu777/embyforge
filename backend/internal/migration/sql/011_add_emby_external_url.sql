-- 011_add_emby_external_url.sql
-- 为 emby_configs 表添加外网访问地址字段，解决反向代理场景下图片和链接无法访问的问题

-- +goose Up
ALTER TABLE emby_configs ADD COLUMN external_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE emby_configs DROP COLUMN external_url;
