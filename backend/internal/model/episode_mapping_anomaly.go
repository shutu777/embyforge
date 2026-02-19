package model

import "time"

// EpisodeMappingAnomaly 异常映射模型
type EpisodeMappingAnomaly struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	EmbyItemID       string    `gorm:"size:50;not null;index" json:"emby_item_id"`
	Name             string    `gorm:"size:500;not null" json:"name"`
	TmdbID           int       `gorm:"not null" json:"tmdb_id"`
	SeasonNumber     int       `gorm:"not null" json:"season_number"`
	LocalEpisodes    int       `gorm:"not null" json:"local_episodes"`    // 本地集数
	TmdbEpisodes     int       `gorm:"not null" json:"tmdb_episodes"`     // TMDB 集数
	Difference       int       `gorm:"not null" json:"difference"`        // 差异数
	LocalSeasonCount int       `gorm:"not null;default:0" json:"local_season_count"` // 本地季数
	TmdbSeasonCount  int       `gorm:"not null;default:0" json:"tmdb_season_count"`  // TMDB 季数
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`
}
