-- +goose Up
-- 115 网盘配置项
INSERT OR IGNORE INTO system_configs (key, value, description, created_at, updated_at)
VALUES
    ('pan115_cookie', '', '115 网盘登录 Cookie', datetime('now'), datetime('now')),
    ('pan115_cid', '0', '115 网盘转存目标文件夹 ID', datetime('now'), datetime('now'));

-- +goose Down
DELETE FROM system_configs WHERE key IN ('pan115_cookie', 'pan115_cid');
