package migration

import (
	"database/sql"
	"embed"
	"fmt"
	"log"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

//go:embed sql/*.sql
var embedMigrations embed.FS

// RunMigrations 使用 goose 执行所有待执行的数据库迁移
// SQL 迁移文件嵌入在二进制中，无需额外分发文件
func RunMigrations(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	goose.SetBaseFS(embedMigrations)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("设置 goose 方言失败: %w", err)
	}

	if err := goose.Up(sqlDB, "sql"); err != nil {
		return fmt.Errorf("执行数据库迁移失败: %w", err)
	}

	// 打印迁移状态
	current, err := goose.GetDBVersion(sqlDB)
	if err != nil {
		log.Printf("⚠️ 获取迁移版本失败: %v", err)
	} else {
		log.Printf("📦 数据库迁移完成，当前版本: %d", current)
	}

	return nil
}

// GetCurrentVersion 获取当前数据库迁移版本
func GetCurrentVersion(sqlDB *sql.DB) (int64, error) {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return 0, err
	}
	return goose.GetDBVersion(sqlDB)
}
