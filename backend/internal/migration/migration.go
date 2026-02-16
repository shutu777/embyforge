package migration

// 此文件原为自定义迁移框架，已迁移至 goose。
// 保留 SchemaMigration 类型定义以兼容现有测试。

import "time"

// SchemaMigration 迁移记录模型（goose 使用 goose_db_version 表，此类型仅用于兼容）
type SchemaMigration struct {
	Version    int       `gorm:"primaryKey"`
	Name       string    `gorm:"size:255;not null"`
	ExecutedAt time.Time `gorm:"not null"`
}
