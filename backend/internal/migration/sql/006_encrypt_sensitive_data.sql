-- +goose Up
-- +goose StatementBegin
-- 此迁移会在应用启动时通过代码自动执行
-- 用于加密所有明文存储的敏感配置
-- 实际加密逻辑在 BeforeSave 钩子中自动处理
SELECT 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- 回滚操作：无需操作，因为加密是通过代码钩子处理的
SELECT 1;
-- +goose StatementEnd
