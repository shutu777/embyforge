package model

import (
	"log"
	"os"
	"path/filepath"

	"embyforge/internal/migration"

	"golang.org/x/crypto/bcrypt"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB 初始化数据库连接，执行自动迁移和种子数据
func InitDB(dbPath string) (*gorm.DB, error) {
	// 确保数据库目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// 启用 WAL 模式和性能优化 PRAGMA，提升大批量写入性能
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000", // 64MB 缓存
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			log.Printf("⚠️ 执行 %s 失败: %v", p, err)
		}
	}

	// 执行版本化数据库迁移
	if err := migration.RunMigrations(db); err != nil {
		return nil, err
	}

	// 创建初始管理员账户（如果不存在）
	seedAdmin(db)

	return db, nil
}

// seedAdmin 创建默认管理员账户
func seedAdmin(db *gorm.DB) {
	var count int64
	db.Model(&User{}).Count(&count)
	if count > 0 {
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("❌ 创建默认管理员失败: %v", err)
		return
	}

	admin := User{
		Username: "admin",
		Password: string(hashedPassword),
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Printf("❌ 创建默认管理员失败: %v", err)
		return
	}

	log.Println("👤 已创建默认管理员账户 (admin/admin)")
}
