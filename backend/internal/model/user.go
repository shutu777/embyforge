package model

import "time"

// User 用户模型
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"uniqueIndex;size:50;not null" json:"username"`
	Password  string    `gorm:"size:255;not null" json:"-"` // bcrypt 哈希，JSON 序列化时隐藏
	Avatar    string    `gorm:"size:500" json:"avatar"`     // 头像文件路径
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
