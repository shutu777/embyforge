package model

import (
	"log"
	"os"
	"path/filepath"

	"embyforge/internal/migration"
	"embyforge/internal/util"

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

	// 自动加密明文敏感数据
	encryptPlaintextData(db)

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

// encryptPlaintextData 自动加密所有明文存储的敏感数据
func encryptPlaintextData(db *gorm.DB) {
	// 加密 SystemConfig 中的敏感配置
	var configs []SystemConfig
	if err := db.Find(&configs).Error; err != nil {
		log.Printf("⚠️  查询系统配置失败: %v", err)
		return
	}

	encryptedCount := 0
	for _, config := range configs {
		// 检查是否是需要加密的键
		if encryptedKeys[config.Key] && config.Value != "" {
			// 尝试解密，如果失败说明是明文
			if _, err := util.Decrypt(config.Value); err != nil {
				// 是明文，需要加密
				// 直接保存会触发 BeforeSave 钩子自动加密
				if err := db.Save(&config).Error; err != nil {
					log.Printf("⚠️  加密配置 %s 失败: %v", config.Key, err)
				} else {
					encryptedCount++
				}
			}
		}
	}

	if encryptedCount > 0 {
		log.Printf("🔐 已自动加密 %d 个明文配置", encryptedCount)
	}

	// 加密 WebhookConfig 中的敏感字段
	var webhookConfigs []WebhookConfig
	if err := db.Find(&webhookConfigs).Error; err != nil {
		log.Printf("⚠️  查询 Webhook 配置失败: %v", err)
		return
	}

	webhookEncryptedCount := 0
	for _, config := range webhookConfigs {
		needSave := false

		// 检查 AuthToken 是否需要加密
		if config.AuthToken != "" {
			if _, err := util.Decrypt(config.AuthToken); err != nil {
				needSave = true
			}
		}

		// 检查 Secret 是否需要加密
		if config.Secret != "" {
			if _, err := util.Decrypt(config.Secret); err != nil {
				needSave = true
			}
		}

		if needSave {
			// 直接保存会触发 BeforeSave 钩子自动加密
			if err := db.Save(&config).Error; err != nil {
				log.Printf("⚠️  加密 Webhook 配置失败: %v", err)
			} else {
				webhookEncryptedCount++
			}
		}
	}

	if webhookEncryptedCount > 0 {
		log.Printf("🔐 已自动加密 %d 个 Webhook 配置", webhookEncryptedCount)
	}
}
