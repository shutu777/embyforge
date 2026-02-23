package model

import "time"

// EmbyConfig Emby 服务器配置模型
type EmbyConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Host      string    `gorm:"size:255;not null" json:"host"`     // 如 http://192.168.1.100
	Port      int       `gorm:"not null" json:"port"`              // 如 8096
	APIKey    string    `gorm:"size:255;not null" json:"api_key"`  // Emby API Key
	Username  string    `gorm:"size:255;not null;default:''" json:"username"` // Emby 用户名（用于删除操作认证）
	Password  string    `gorm:"size:255;not null;default:''" json:"password,omitempty"` // Emby 密码（响应中不返回）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
