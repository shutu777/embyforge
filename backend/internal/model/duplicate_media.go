package model

import "time"

// DuplicateMedia 重复媒体模型
type DuplicateMedia struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	GroupKey   string    `gorm:"size:255;not null;index" json:"group_key"` // 分组键（tmdb:ID）
	GroupName  string    `gorm:"size:500;not null;default:''" json:"group_name"` // 分组显示名（媒体名称）
	EmbyItemID string    `gorm:"size:50;not null" json:"emby_item_id"`
	Name       string    `gorm:"size:500;not null" json:"name"`
	Type       string    `gorm:"size:50;not null" json:"type"`
	Path       string    `gorm:"size:1000" json:"path"`
	FileSize   int64     `json:"file_size"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}
