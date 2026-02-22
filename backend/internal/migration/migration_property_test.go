package migration

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"pgregory.net/rapid"
)

// closeDB 关闭 GORM 数据库连接，释放 SQLite 文件锁
func closeDB(t *rapid.T, db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 DB 失败: %v", err)
	}
	sqlDB.Close()
}

// Feature: system-config, Property 6: Migration idempotency
// Validates: Requirements 5.3
// 对于任意次数的迁移运行，结果应与运行一次相同（幂等性）。
func TestProperty_MigrationIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbIndex := 0
	rapid.Check(t, func(t *rapid.T) {
		// 随机运行 1~5 次迁移
		runCount := rapid.IntRange(1, 5).Draw(t, "runCount")

		dbIndex++
		dbPath := filepath.Join(tmpDir, "test_idempotent.db")
		db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			t.Fatalf("打开测试数据库失败: %v", err)
		}
		defer closeDB(t, db)

		// 运行 N 次迁移
		for i := 0; i < runCount; i++ {
			if err := RunMigrations(db); err != nil {
				t.Fatalf("第 %d 次 RunMigrations 失败: %v", i+1, err)
			}
		}

		// 验证版本号始终为 1
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("获取底层 DB 失败: %v", err)
		}
		ver, err := GetCurrentVersion(sqlDB)
		if err != nil {
			t.Fatalf("获取版本失败: %v", err)
		}
		if ver != 7 {
			t.Fatalf("幂等性违反: 运行 %d 次后版本为 %d, 期望 7", runCount, ver)
		}
	})
}
