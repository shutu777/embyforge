-- +goose Up
ALTER TABLE media_caches ADD COLUMN date_created DATETIME;
ALTER TABLE duplicate_media ADD COLUMN date_created DATETIME;

-- +goose Down
-- SQLite 不支持 DROP COLUMN（3.35.0 之前），此处简化处理
