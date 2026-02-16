package model

import "time"

// EmbyConfig Emby 服务器配置模型
type EmbyConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Host      string    `gorm:"size:255;not null" json:"host"`   // 如 http://192.168.1.100
	Port      int       `gorm:"not null" json:"port"`            // 如 8096
	APIKey    string    `gorm:"size:255;not null" json:"api_key"` // Emby API Key
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
