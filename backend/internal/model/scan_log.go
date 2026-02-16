package model

import "time"

// ScanLog 扫描/分析执行记录
type ScanLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Module     string    `gorm:"size:50;not null;index" json:"module"` // scrape_anomaly / duplicate_media / episode_mapping
	StartedAt  time.Time `gorm:"not null" json:"started_at"`
	FinishedAt time.Time `gorm:"not null" json:"finished_at"`
	TotalScanned int     `gorm:"not null;default:0" json:"total_scanned"`
	AnomalyCount int     `gorm:"not null;default:0" json:"anomaly_count"`
	ErrorCount   int     `gorm:"not null;default:0" json:"error_count"`
}
