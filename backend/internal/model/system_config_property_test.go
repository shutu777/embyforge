package model

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	"pgregory.net/rapid"
)

// setupSystemConfigTestDB 创建测试数据库并返回 gorm.DB
func setupSystemConfigTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 DB 失败: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return db
}

// Feature: system-config, Property 1: SystemConfig model round-trip
// Validates: Requirements 1.1
// 对于任意有效的 SystemConfig（非空 key、value、description），保存后再按 key 读取应返回等价的记录。
func TestProperty_SystemConfigRoundTrip(t *testing.T) {
	db := setupSystemConfigTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的配置项
		key := rapid.StringMatching(`[a-z][a-z0-9_]{2,30}`).Draw(t, "key")
		value := rapid.StringMatching(`[a-zA-Z0-9_\-]{1,50}`).Draw(t, "value")
		description := rapid.StringMatching(`[a-zA-Z0-9 ]{0,100}`).Draw(t, "description")

		config := SystemConfig{
			Key:         key,
			Value:       value,
			Description: description,
		}

		// 清理同 key 的旧记录（避免唯一索引冲突）
		db.Where("key = ?", key).Delete(&SystemConfig{})

		// 保存配置
		if err := db.Create(&config).Error; err != nil {
			t.Fatalf("保存配置失败: %v", err)
		}

		// 按 key 读取
		var loaded SystemConfig
		if err := db.Where("key = ?", key).First(&loaded).Error; err != nil {
			t.Fatalf("读取配置失败: %v", err)
		}

		// 验证 round-trip
		if loaded.Key != key {
			t.Fatalf("Key 不匹配: got %q, want %q", loaded.Key, key)
		}
		if loaded.Value != value {
			t.Fatalf("Value 不匹配: got %q, want %q", loaded.Value, value)
		}
		if loaded.Description != description {
			t.Fatalf("Description 不匹配: got %q, want %q", loaded.Description, description)
		}
	})
}

// Feature: system-config, Property 2: Key uniqueness invariant
// Validates: Requirements 1.2
// 对于任意 key，插入一条记录后再插入同 key 的记录应失败，且数据库中该 key 只有一条记录。
func TestProperty_SystemConfigKeyUniqueness(t *testing.T) {
	db := setupSystemConfigTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[a-z][a-z0-9_]{2,30}`).Draw(t, "key")
		value1 := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "value1")
		value2 := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "value2")

		// 清理旧记录
		db.Where("key = ?", key).Delete(&SystemConfig{})

		// 插入第一条
		first := SystemConfig{Key: key, Value: value1, Description: "first"}
		if err := db.Create(&first).Error; err != nil {
			t.Fatalf("插入第一条记录失败: %v", err)
		}

		// 插入第二条同 key 的记录应失败
		second := SystemConfig{Key: key, Value: value2, Description: "second"}
		err := db.Create(&second).Error
		if err == nil {
			t.Fatalf("插入重复 key %q 应失败，但成功了", key)
		}

		// 验证数据库中该 key 只有一条记录
		var count int64
		db.Model(&SystemConfig{}).Where("key = ?", key).Count(&count)
		if count != 1 {
			t.Fatalf("key %q 应只有 1 条记录，实际有 %d 条", key, count)
		}
	})
}
