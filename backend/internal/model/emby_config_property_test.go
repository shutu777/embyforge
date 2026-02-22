package model

import (
	"testing"

	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// Feature: embyforge, Property 3: Emby 配置 Round-Trip
// Validates: Requirements 3.2
// 对于任意有效的 Emby 配置（Host、Port、APIKey），保存配置后再读取应返回等价的配置对象。
func TestProperty_EmbyConfigRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的 Emby 配置
		host := rapid.StringMatching(`https?://[a-z0-9]{1,20}(\.[a-z0-9]{1,10}){0,3}`).Draw(t, "host")
		port := rapid.IntRange(1, 65535).Draw(t, "port")
		apiKey := rapid.StringMatching(`[a-f0-9]{16,32}`).Draw(t, "apiKey")

		config := EmbyConfig{
			Host:   host,
			Port:   port,
			APIKey: apiKey,
		}

		// 保存配置（upsert：先删除旧的，再创建新的）
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&EmbyConfig{})
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("保存配置失败: %v", err)
		}

		// 读取配置
		var loaded EmbyConfig
		if err := db.First(&loaded).Error; err != nil {
			t.Fatalf("读取配置失败: %v", err)
		}

		// 验证 round-trip：保存后读取应等价
		if loaded.Host != host {
			t.Fatalf("Host 不匹配: got %q, want %q", loaded.Host, host)
		}
		if loaded.Port != port {
			t.Fatalf("Port 不匹配: got %d, want %d", loaded.Port, port)
		}
		if loaded.APIKey != apiKey {
			t.Fatalf("APIKey 不匹配: got %q, want %q", loaded.APIKey, apiKey)
		}
	})
}
