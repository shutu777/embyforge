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
	JWTSecret string // JWT 签名密钥
	DBPath    string // SQLite 数据库文件路径
}

// generateRandomSecret 生成随机 JWT 密钥
func generateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("生成随机 JWT 密钥失败:", err)
	}
	return hex.EncodeToString(b)
}

// Load 从环境变量加载配置，未设置时使用默认值
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
	} else {
		cfg.JWTSecret = generateRandomSecret()
		log.Println("未设置 EMBYFORGE_JWT_SECRET，已自动生成随机密钥")
	}

	if dbPath := os.Getenv("EMBYFORGE_DB_PATH"); dbPath != "" {
		cfg.DBPath = dbPath
	}

	return cfg
}
