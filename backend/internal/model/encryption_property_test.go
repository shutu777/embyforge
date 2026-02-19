package model

import (
	"testing"

	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// Feature: symedia-config-refresh, Property 6: 敏感信息加密存储（WebhookConfig）
// Validates: Requirements 5.5, 8.2
// 对于任何 WebhookConfig，AuthToken 和 Secret 在数据库中应该是加密的，读取后应该自动解密
func TestProperty_WebhookConfigEncryption(t *testing.T) {
	db := setupWebhookConfigTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的敏感信息
		authToken := rapid.StringMatching(`[a-zA-Z0-9\-_]{20,100}`).Draw(t, "authToken")
		secret := rapid.StringMatching(`[a-zA-Z0-9]{16,64}`).Draw(t, "secret")

		config := WebhookConfig{
			SymediaUrl: "https://example.com:8080",
			AuthToken:  authToken,
			RepoUrl:    "https://github.com/test/repo",
			Branch:     "main",
			FilePath:   "config/rules.txt",
			Secret:     secret,
			WebhookUrl: "https://example.com:8080/api/webhook/test",
		}

		// 清理旧记录
		db.Where("1=1").Delete(&WebhookConfig{})

		// 保存配置（应该触发 BeforeSave 钩子加密）
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("保存 WebhookConfig 失败: %v", err)
		}

		// 直接从数据库读取原始数据（绕过 AfterFind 钩子）
		var rawConfig WebhookConfig
		if err := db.Session(&gorm.Session{}).Raw("SELECT * FROM webhook_configs WHERE id = ?", config.ID).Scan(&rawConfig).Error; err != nil {
			t.Fatalf("读取原始数据失败: %v", err)
		}

		// 验证数据库中的值已加密（与原始值不同）
		if rawConfig.AuthToken == authToken {
			t.Fatalf("AuthToken 在数据库中未加密")
		}
		if rawConfig.Secret == secret {
			t.Fatalf("Secret 在数据库中未加密")
		}

		// 通过 GORM 读取配置（应该触发 AfterFind 钩子解密）
		var loaded WebhookConfig
		if err := db.First(&loaded, config.ID).Error; err != nil {
			t.Fatalf("读取 WebhookConfig 失败: %v", err)
		}

		// 验证解密后的值与原始值一致
		if loaded.AuthToken != authToken {
			t.Fatalf("AuthToken 解密后不匹配: got %q, want %q", loaded.AuthToken, authToken)
		}
		if loaded.Secret != secret {
			t.Fatalf("Secret 解密后不匹配: got %q, want %q", loaded.Secret, secret)
		}
	})
}

// Feature: symedia-config-refresh, Property 6: 敏感信息加密存储（SystemConfig）
// Validates: Requirements 5.5, 8.2
// 对于 SystemConfig 中的 symedia_auth_token，在数据库中应该是加密的，读取后应该自动解密
func TestProperty_SystemConfigEncryption(t *testing.T) {
	db := setupWebhookConfigTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的 auth token
		authToken := rapid.StringMatching(`[a-zA-Z0-9\-_]{20,100}`).Draw(t, "authToken")

		config := SystemConfig{
			Key:         "symedia_auth_token",
			Value:       authToken,
			Description: "Symedia Authorization Token",
		}

		// 清理旧记录
		db.Where("key = ?", "symedia_auth_token").Delete(&SystemConfig{})

		// 保存配置（应该触发 BeforeSave 钩子加密）
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("保存 SystemConfig 失败: %v", err)
		}

		// 直接从数据库读取原始数据（绕过 AfterFind 钩子）
		var rawConfig SystemConfig
		if err := db.Session(&gorm.Session{}).Raw("SELECT * FROM system_configs WHERE id = ?", config.ID).Scan(&rawConfig).Error; err != nil {
			t.Fatalf("读取原始数据失败: %v", err)
		}

		// 验证数据库中的值已加密（与原始值不同）
		if rawConfig.Value == authToken {
			t.Fatalf("symedia_auth_token 在数据库中未加密")
		}

		// 通过 GORM 读取配置（应该触发 AfterFind 钩子解密）
		var loaded SystemConfig
		if err := db.First(&loaded, config.ID).Error; err != nil {
			t.Fatalf("读取 SystemConfig 失败: %v", err)
		}

		// 验证解密后的值与原始值一致
		if loaded.Value != authToken {
			t.Fatalf("symedia_auth_token 解密后不匹配: got %q, want %q", loaded.Value, authToken)
		}
	})
}

// Feature: symedia-config-refresh, Property 6: 敏感信息加密存储（非敏感配置不加密）
// Validates: Requirements 5.5, 8.2
// 对于 SystemConfig 中的非敏感配置（如 symedia_url），不应该加密
func TestProperty_SystemConfigNoEncryptionForNonSensitive(t *testing.T) {
	db := setupWebhookConfigTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的 URL
		symediaUrl := rapid.StringMatching(`https?://[a-z0-9\-\.]+:[0-9]{4,5}`).Draw(t, "symediaUrl")

		config := SystemConfig{
			Key:         "symedia_url",
			Value:       symediaUrl,
			Description: "Symedia Service URL",
		}

		// 清理旧记录
		db.Where("key = ?", "symedia_url").Delete(&SystemConfig{})

		// 保存配置
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("保存 SystemConfig 失败: %v", err)
		}

		// 直接从数据库读取原始数据
		var rawConfig SystemConfig
		if err := db.Session(&gorm.Session{}).Raw("SELECT * FROM system_configs WHERE id = ?", config.ID).Scan(&rawConfig).Error; err != nil {
			t.Fatalf("读取原始数据失败: %v", err)
		}

		// 验证数据库中的值未加密（与原始值相同）
		if rawConfig.Value != symediaUrl {
			t.Fatalf("symedia_url 不应该加密，但值不匹配: got %q, want %q", rawConfig.Value, symediaUrl)
		}

		// 通过 GORM 读取配置
		var loaded SystemConfig
		if err := db.First(&loaded, config.ID).Error; err != nil {
			t.Fatalf("读取 SystemConfig 失败: %v", err)
		}

		// 验证值与原始值一致
		if loaded.Value != symediaUrl {
			t.Fatalf("symedia_url 读取后不匹配: got %q, want %q", loaded.Value, symediaUrl)
		}
	})
}
