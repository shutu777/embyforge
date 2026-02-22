package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"strconv"
)

// Config 应用配置结构体
type Config struct {
	Port      int    // 服务端口
	JWTSecret string // JWT 签名密钥（优先使用环境变量，否则从数据库加载）
	DBPath    string // SQLite 数据库文件路径
}

// GenerateRandomSecret 生成随机 JWT 密钥（导出供外部使用）
func GenerateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("生成随机 JWT 密钥失败:", err)
	}
	return hex.EncodeToString(b)
}

// Load 从环境变量加载配置，未设置时使用默认值
// 注意：JWTSecret 如果未通过环境变量设置，将留空，由 main.go 从数据库加载
func Load() *Config {
	cfg := &Config{
		Port:   8080,
		DBPath: "data/embyforge.db",
	}

	if port := os.Getenv("EMBYFORGE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Port = p
		}
	}

	if secret := os.Getenv("EMBYFORGE_JWT_SECRET"); secret != "" {
		cfg.JWTSecret = secret
	}
	// 不再自动生成随机密钥，由 main.go 从数据库持久化加载

	if dbPath := os.Getenv("EMBYFORGE_DB_PATH"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	return cfg
}
