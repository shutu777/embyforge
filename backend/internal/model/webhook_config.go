package model

import (
	"embyforge/internal/util"
	"log"
	"time"

	"gorm.io/gorm"
)

// WebhookConfig GitHub Webhook配置模型
type WebhookConfig struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SymediaUrl string    `gorm:"size:500;not null" json:"symedia_url"`       // Symedia服务地址
	AuthToken  string    `gorm:"type:text;not null" json:"auth_token"`       // Authorization令牌（加密存储）
	RepoUrl    string    `gorm:"size:500;not null" json:"repo_url"`          // GitHub仓库URL
	Branch     string    `gorm:"size:100;not null;default:'main'" json:"branch"` // 监听的分支
	FilePath   string    `gorm:"size:500" json:"file_path"`                  // 监听的文件路径（可选，为空或"*"表示监听所有文件）
	Secret     string    `gorm:"size:500;not null" json:"secret"`            // Webhook密钥（加密存储）
	WebhookUrl string    `gorm:"size:500;not null" json:"webhook_url"`       // 生成的Webhook URL
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BeforeSave GORM钩子：保存前加密敏感字段
func (w *WebhookConfig) BeforeSave(tx *gorm.DB) error {
	// 加密 AuthToken
	if w.AuthToken != "" {
		encrypted, err := util.Encrypt(w.AuthToken)
		if err != nil {
			log.Printf("❌ 加密 AuthToken 失败: %v", err)
			return err
		}
		w.AuthToken = encrypted
	}

	// 加密 Secret
	if w.Secret != "" {
		encrypted, err := util.Encrypt(w.Secret)
		if err != nil {
			log.Printf("❌ 加密 Secret 失败: %v", err)
			return err
		}
		w.Secret = encrypted
	}

	return nil
}

// AfterFind GORM钩子：查询后解密敏感字段
func (w *WebhookConfig) AfterFind(tx *gorm.DB) error {
	// 解密 AuthToken
	if w.AuthToken != "" {
		decrypted, err := util.Decrypt(w.AuthToken)
		if err != nil {
			// 解密失败，说明是明文数据
			// 静默处理，不输出警告日志（避免日志刷屏）
			// 数据会在下次保存时自动加密
		} else {
			w.AuthToken = decrypted
		}
	}

	// 解密 Secret
	if w.Secret != "" {
		decrypted, err := util.Decrypt(w.Secret)
		if err != nil {
			// 解密失败，说明是明文数据
			// 静默处理，不输出警告日志（避免日志刷屏）
			// 数据会在下次保存时自动加密
		} else {
			w.Secret = decrypted
		}
	}

	return nil
}
