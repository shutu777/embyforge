package model

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

// setupTestDB 创建临时测试数据库并返回 gorm.DB
// 所有 model 包的测试共享此 helper，避免重复代码
func setupTestDB(t *testing.T) *gorm.DB {
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
