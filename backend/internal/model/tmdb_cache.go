package model

import "time"

// TmdbCache TMDB 季集数据缓存模型
type TmdbCache struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TmdbID       int       `gorm:"not null;uniqueIndex:idx_tmdb_cache_tmdb_season" json:"tmdb_id"`
	Name         string    `gorm:"size:500;not null;default:''" json:"name"`           // 节目名称
	SeasonNumber int       `gorm:"not null;uniqueIndex:idx_tmdb_cache_tmdb_season" json:"season_number"`
	EpisodeCount int       `gorm:"not null;default:0" json:"episode_count"`
	SeasonName   string    `gorm:"size:500;not null;default:''" json:"season_name"`    // 季名称
	CachedAt     time.Time `gorm:"not null" json:"cached_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}
