-- 007_add_season_counts.sql
-- 为 episode_mapping_anomalies 表添加本地季数和 TMDB 季数字段

-- +goose Up
ALTER TABLE episode_mapping_anomalies ADD COLUMN local_season_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE episode_mapping_anomalies ADD COLUMN tmdb_season_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE episode_mapping_anomalies DROP COLUMN local_season_count;
ALTER TABLE episode_mapping_anomalies DROP COLUMN tmdb_season_count;
