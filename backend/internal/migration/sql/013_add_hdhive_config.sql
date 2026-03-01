-- +goose Up
-- HDHive 配置项
INSERT OR IGNORE INTO system_configs (key, value, description, created_at, updated_at)
VALUES
    ('hdhive_username', '', 'HDHive 账号', datetime('now'), datetime('now')),
    ('hdhive_password', '', 'HDHive 密码', datetime('now'), datetime('now')),
    ('hdhive_token', '', 'HDHive 登录 Token（自动缓存）', datetime('now'), datetime('now')),
    ('hdhive_cookie', '', 'HDHive 登录 Cookie（自动缓存）', datetime('now'), datetime('now'));

-- +goose Down
DELETE FROM system_configs WHERE key IN ('hdhive_username', 'hdhive_password', 'hdhive_token', 'hdhive_cookie');
