package model

import "time"

// WebhookLog Webhook触发的配置刷新日志模型
type WebhookLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Source    string    `gorm:"size:50;not null" json:"source"`     // 触发来源："github" 或 "manual"
	RepoName  string    `gorm:"size:500" json:"repo_name"`          // 仓库名称
	Branch    string    `gorm:"size:100" json:"branch"`             // 分支名称
	CommitSHA string    `gorm:"size:100" json:"commit_sha"`         // 提交SHA
	Success   bool      `gorm:"not null" json:"success"`            // 是否成功
	ErrorMsg  string    `gorm:"type:text" json:"error_msg"`         // 错误消息
	CreatedAt time.Time `gorm:"index" json:"created_at"`            // 创建时间（带索引）
}
