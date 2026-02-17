-- +goose Up

-- TMDB 缓存表：缓存 TMDB 电视节目的季集数据，避免重复请求
CREATE TABLE IF NOT EXISTS tmdb_caches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tmdb_id INTEGER NOT NULL,
    name VARCHAR(500) NOT NULL DEFAULT '',
    season_number INTEGER NOT NULL,
    episode_count INTEGER NOT NULL DEFAULT 0,
    season_name VARCHAR(500) NOT NULL DEFAULT '',
    cached_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tmdb_cache_tmdb_season ON tmdb_caches(tmdb_id, season_number);
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_tmdb_id ON tmdb_caches(tmdb_id);

-- +goose Down
DROP TABLE IF EXISTS tmdb_caches;
