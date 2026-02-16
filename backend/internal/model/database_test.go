package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	// 使用临时目录创建测试数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}

	// 验证数据库文件已创建
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("数据库文件未创建")
	}

	// 验证所有表已创建
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 DB 失败: %v", err)
	}
	defer sqlDB.Close()

	tables := []string{"users", "emby_configs", "scrape_anomalies", "duplicate_media", "episode_mapping_anomalies", "system_configs", "goose_db_version"}
	for _, table := range tables {
		var count int
		err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("查询表 %s 失败: %v", table, err)
		}
		if count == 0 {
			t.Errorf("表 %s 未创建", table)
		}
	}

	// 验证默认管理员已创建
	var user User
	if err := db.First(&user, "username = ?", "admin").Error; err != nil {
		t.Fatalf("查询默认管理员失败: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("管理员用户名不正确: got %s, want admin", user.Username)
	}
	if user.Password == "" || user.Password == "admin" {
		t.Error("管理员密码未正确哈希")
	}
}

func TestInitDB_SeedAdminIdempotent(t *testing.T) {
	// 验证多次初始化不会创建重复管理员
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db1, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("第一次 InitDB 失败: %v", err)
	}
	sqlDB1, _ := db1.DB()
	sqlDB1.Close()

	db2, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("第二次 InitDB 失败: %v", err)
	}

	var count int64
	db2.Model(&User{}).Count(&count)
	if count != 1 {
		t.Errorf("管理员账户数量不正确: got %d, want 1", count)
	}

	sqlDB2, _ := db2.DB()
	sqlDB2.Close()
}
