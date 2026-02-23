-- 008_add_date_last_saved.sql
-- 为 media_caches 表添加 Emby 最后保存时间字段

-- +goose Up
ALTER TABLE media_caches ADD COLUMN date_last_saved DATETIME;
CREATE INDEX IF NOT EXISTS idx_media_caches_date_last_saved ON media_caches(date_last_saved);

-- +goose Down
DROP INDEX IF EXISTS idx_media_caches_date_last_saved;
ALTER TABLE media_caches DROP COLUMN date_last_saved;
