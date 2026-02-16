-- +goose Up

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username VARCHAR(50) NOT NULL,
    password VARCHAR(255) NOT NULL,
    avatar VARCHAR(500) DEFAULT '',
    created_at DATETIME,
    updated_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- Emby 服务器配置表
CREATE TABLE IF NOT EXISTS emby_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    api_key VARCHAR(255) NOT NULL,
    created_at DATETIME,
    updated_at DATETIME
);

-- 系统配置表（键值对）
CREATE TABLE IF NOT EXISTS system_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key VARCHAR(100) NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    description VARCHAR(500),
    created_at DATETIME,
    updated_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_system_configs_key ON system_configs(key);

INSERT INTO system_configs (key, value, description, created_at, updated_at)
VALUES ('tmdb_api_key', '', 'TMDB API Key，用于异常映射扫描', datetime('now'), datetime('now'));

-- 刮削异常表
CREATE TABLE IF NOT EXISTS scrape_anomalies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    emby_item_id VARCHAR(50) NOT NULL,
    name VARCHAR(500) NOT NULL,
    type VARCHAR(50) NOT NULL,
    missing_poster BOOLEAN NOT NULL DEFAULT 0,
    missing_provider BOOLEAN NOT NULL DEFAULT 0,
    path VARCHAR(1000),
    library_name VARCHAR(255),
    created_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_scrape_anomalies_emby_item_id ON scrape_anomalies(emby_item_id);

-- 重复媒体表
CREATE TABLE IF NOT EXISTS duplicate_media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_key VARCHAR(255) NOT NULL,
    group_name VARCHAR(500) NOT NULL DEFAULT '',
    emby_item_id VARCHAR(50) NOT NULL,
    name VARCHAR(500) NOT NULL,
    type VARCHAR(50) NOT NULL,
    path VARCHAR(1000),
    file_size INTEGER,
    created_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_duplicate_media_group_key ON duplicate_media(group_key);

-- 异常映射表
CREATE TABLE IF NOT EXISTS episode_mapping_anomalies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    emby_item_id VARCHAR(50) NOT NULL,
    name VARCHAR(500) NOT NULL,
    tmdb_id INTEGER NOT NULL,
    season_number INTEGER NOT NULL,
    local_episodes INTEGER NOT NULL,
    tmdb_episodes INTEGER NOT NULL,
    difference INTEGER NOT NULL,
    created_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_episode_mapping_anomalies_emby_item_id ON episode_mapping_anomalies(emby_item_id);

-- 媒体缓存表
CREATE TABLE IF NOT EXISTS media_caches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    emby_item_id VARCHAR(50) NOT NULL,
    name VARCHAR(500) NOT NULL,
    type VARCHAR(50) NOT NULL,
    has_poster BOOLEAN NOT NULL DEFAULT 0,
    path VARCHAR(1000),
    provider_ids TEXT,
    file_size INTEGER DEFAULT 0,
    index_number INTEGER DEFAULT 0,
    parent_index_number INTEGER DEFAULT 0,
    child_count INTEGER DEFAULT 0,
    series_id VARCHAR(50) DEFAULT '',
    series_name VARCHAR(500) DEFAULT '',
    library_name VARCHAR(255),
    cached_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_cache_emby_item_id ON media_caches(emby_item_id);
CREATE INDEX IF NOT EXISTS idx_media_cache_type ON media_caches(type);

-- 季缓存表
CREATE TABLE IF NOT EXISTS season_caches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    series_emby_item_id VARCHAR(50) NOT NULL,
    season_emby_item_id VARCHAR(50) NOT NULL,
    season_number INTEGER NOT NULL,
    episode_count INTEGER NOT NULL DEFAULT 0,
    cached_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_season_cache_series ON season_caches(series_emby_item_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_season_cache_season_id ON season_caches(season_emby_item_id);

-- 扫描日志表
CREATE TABLE IF NOT EXISTS scan_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    module VARCHAR(50) NOT NULL,
    started_at DATETIME NOT NULL,
    finished_at DATETIME NOT NULL,
    total_scanned INTEGER NOT NULL DEFAULT 0,
    anomaly_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_scan_logs_module ON scan_logs(module);

-- +goose Down
DROP TABLE IF EXISTS scan_logs;
DROP TABLE IF EXISTS season_caches;
DROP TABLE IF EXISTS media_caches;
DROP TABLE IF EXISTS episode_mapping_anomalies;
DROP TABLE IF EXISTS duplicate_media;
DROP TABLE IF EXISTS scrape_anomalies;
DROP TABLE IF EXISTS system_configs;
DROP TABLE IF EXISTS emby_configs;
DROP TABLE IF EXISTS users;
