package model

import "time"

// ScrapeAnomaly 刮削异常模型
type ScrapeAnomaly struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	EmbyItemID      string    `gorm:"size:50;not null;index" json:"emby_item_id"`
	Name            string    `gorm:"size:500;not null" json:"name"`
	Type            string    `gorm:"size:50;not null" json:"type"` // Movie / Series
	MissingPoster   bool      `gorm:"not null" json:"missing_poster"`
	MissingProvider bool      `gorm:"not null" json:"missing_provider"` // 缺少 TMDB/IMDB 等外部 ID
	Path            string    `gorm:"size:1000" json:"path"`
	LibraryName     string    `gorm:"size:255" json:"library_name"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
}
