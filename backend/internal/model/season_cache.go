package model

import "time"

// SeasonCache 季缓存模型
type SeasonCache struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	SeriesEmbyItemID string    `gorm:"size:50;not null;index" json:"series_emby_item_id"`
	SeasonEmbyItemID string    `gorm:"size:50;not null;uniqueIndex" json:"season_emby_item_id"`
	SeasonNumber     int       `gorm:"not null" json:"season_number"`
	EpisodeCount     int       `gorm:"not null;default:0" json:"episode_count"`
	CachedAt         time.Time `gorm:"not null" json:"cached_at"`
}
