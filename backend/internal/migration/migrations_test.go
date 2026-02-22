package migration

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openDB 创建临时 SQLite 数据库并运行所有迁移
func openDB(t *testing.T) *gorm.DB {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations 失败: %v", err)
	}
	return db
}

// tableExists 检查表是否存在
func tableExists(t *testing.T, db *gorm.DB, tableName string) bool {
	var count int
	sqlDB, _ := db.DB()
	err := sqlDB.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?",
		tableName,
	).Scan(&count)
	if err != nil {
		t.Fatalf("查询表 %s 失败: %v", tableName, err)
	}
	return count > 0
}

// TestMigration_CreatesAllTables 验证迁移创建所有表
func TestMigration_CreatesAllTables(t *testing.T) {
	db := openDB(t)

	expectedTables := []string{
		"users",
		"emby_configs",
		"system_configs",
		"scrape_anomalies",
		"duplicate_media",
		"episode_mapping_anomalies",
		"media_caches",
		"season_caches",
		"scan_logs",
	}

	for _, table := range expectedTables {
		if !tableExists(t, db, table) {
			t.Errorf("表 %s 未创建", table)
		}
	}
}

// TestMigration_SeedsTmdbApiKey 验证迁移插入默认 tmdb_api_key 种子数据
func TestMigration_SeedsTmdbApiKey(t *testing.T) {
	db := openDB(t)

	var key, value, description string
	sqlDB, _ := db.DB()
	err := sqlDB.QueryRow(
		"SELECT key, value, description FROM system_configs WHERE key = ?",
		"tmdb_api_key",
	).Scan(&key, &value, &description)
	if err != nil {
		t.Fatalf("查询 tmdb_api_key 失败: %v", err)
	}

	if key != "tmdb_api_key" {
		t.Errorf("key 不匹配: got %q, want %q", key, "tmdb_api_key")
	}
	if value != "" {
		t.Errorf("默认 value 应为空: got %q", value)
	}
	if description == "" {
		t.Error("description 不应为空")
	}
}

// TestMigration_Idempotent 验证迁移幂等性（运行两次不报错）
func TestMigration_Idempotent(t *testing.T) {
	db := openDB(t) // 第一次运行

	// 第二次运行
	if err := RunMigrations(db); err != nil {
		t.Fatalf("第二次 RunMigrations 失败: %v", err)
	}

	// 验证版本号
	sqlDB, _ := db.DB()
	ver, err := GetCurrentVersion(sqlDB)
	if err != nil {
		t.Fatalf("获取版本失败: %v", err)
	}
	if ver != 7 {
		t.Errorf("版本号不匹配: got %d, want 7", ver)
	}
}

// TestMigration_MediaCachesHasSeriesFields 验证 media_caches 表包含 series 相关字段
func TestMigration_MediaCachesHasSeriesFields(t *testing.T) {
	db := openDB(t)
	sqlDB, _ := db.DB()

	// 尝试插入包含 series 字段的记录
	_, err := sqlDB.Exec(`INSERT INTO media_caches 
		(emby_item_id, name, type, has_poster, path, provider_ids, file_size, 
		 index_number, parent_index_number, child_count, series_id, series_name, 
		 library_name, cached_at) 
		VALUES ('test1', 'Test', 'Episode', 0, '/test', '{}', 0, 
		        1, 2, 0, 'series123', 'Test Series', '', datetime('now'))`)
	if err != nil {
		t.Fatalf("插入包含 series 字段的记录失败: %v", err)
	}
}

// TestMigration_DuplicateMediaHasGroupName 验证 duplicate_media 表包含 group_name 字段
func TestMigration_DuplicateMediaHasGroupName(t *testing.T) {
	db := openDB(t)
	sqlDB, _ := db.DB()

	_, err := sqlDB.Exec(`INSERT INTO duplicate_media 
		(group_key, group_name, emby_item_id, name, type, path, file_size) 
		VALUES ('tmdb:movie:123', 'Test Movie', 'item1', 'Test', 'Movie', '/test', 1024)`)
	if err != nil {
		t.Fatalf("插入包含 group_name 字段的记录失败: %v", err)
	}
}
