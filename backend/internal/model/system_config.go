package model

import (
	"embyforge/internal/util"
	"log"
	"time"

	"gorm.io/gorm"
)

// SystemConfig 系统配置模型，以键值对形式存储应用配置项
type SystemConfig struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Key         string    `gorm:"size:100;uniqueIndex;not null" json:"key"`
	Value       string    `gorm:"type:text;not null;default:''" json:"value"`
	Description string    `gorm:"size:500" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// 需要加密的配置键列表
var encryptedKeys = map[string]bool{
	"symedia_auth_token": true,
}

// BeforeSave GORM钩子：保存前加密敏感字段
func (s *SystemConfig) BeforeSave(tx *gorm.DB) error {
	// 检查是否是需要加密的键
	if encryptedKeys[s.Key] && s.Value != "" {
		encrypted, err := util.Encrypt(s.Value)
		if err != nil {
			log.Printf("❌ 加密配置 %s 失败: %v", s.Key, err)
			return err
		}
		s.Value = encrypted
	}

	return nil
}

// AfterFind GORM钩子：查询后解密敏感字段
func (s *SystemConfig) AfterFind(tx *gorm.DB) error {
	// 检查是否是需要加密的键
	if encryptedKeys[s.Key] && s.Value != "" {
		// 尝试解密，如果失败则认为是明文数据
		decrypted, err := util.Decrypt(s.Value)
		if err != nil {
			// 解密失败，说明是明文数据
			// 静默处理，不输出警告日志（避免日志刷屏）
			// 数据会在下次保存时自动加密
			return nil
		}
		s.Value = decrypted
	}

	return nil
}
