-- +goose Up
-- 清理旧版轮询机制遗留的配置项（已被 Webhook + Cron 替代）
DELETE FROM system_configs WHERE key = 'watcher_interval_minutes';

-- +goose Down
INSERT INTO system_configs (key, value, description, created_at, updated_at)
VALUES ('watcher_interval_minutes', '10', '定时监听轮询间隔（分钟，范围 1-180）', datetime('now'), datetime('now'));
