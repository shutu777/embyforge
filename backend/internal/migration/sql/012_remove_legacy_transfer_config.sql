-- +goose Up
-- 清理旧版归档配置项（归档功能已移除，替换为路径替换）
DELETE FROM system_configs WHERE key LIKE 'symedia_transfer_%';

-- +goose Down
-- 无法恢复已删除的数据，仅提供空操作
