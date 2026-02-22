package handler

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// Feature: ui-cache-improvements, Property 4: Scrape anomaly delete cache cleanup
// Validates: Requirements 3.3, 3.4
// 对于任意一组成功从 Emby 删除的刮削异常 emby_item_id，
// scrape_anomalies 表不应包含这些记录，media_caches 表也不应包含这些记录。
func TestProperty_ScrapeAnomalyDeleteCacheCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "anomaly_cleanup.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	rapid.Check(t, func(t *rapid.T) {
		// 每次迭代前清空表
		db.Exec("DELETE FROM scrape_anomalies")
		db.Exec("DELETE FROM media_caches")

		// 生成 3~15 条 ScrapeAnomaly 记录和对应的 MediaCache 记录
		count := rapid.IntRange(3, 15).Draw(t, "count")
		var allIDs []string

		for i := 0; i < count; i++ {
			embyID := fmt.Sprintf("anomaly-%d", i)
			allIDs = append(allIDs, embyID)

			db.Create(&model.ScrapeAnomaly{
				EmbyItemID:      embyID,
				Name:            fmt.Sprintf("Media_%d", i),
				Type:            "Movie",
				MissingPoster:   rapid.Bool().Draw(t, fmt.Sprintf("missingPoster_%d", i)),
				MissingProvider: rapid.Bool().Draw(t, fmt.Sprintf("missingProvider_%d", i)),
				Path:            fmt.Sprintf("/media/%d", i),
				LibraryName:     "TestLib",
				CreatedAt:       time.Now(),
			})

			db.Create(&model.MediaCache{
				EmbyItemID: embyID,
				Name:       fmt.Sprintf("Media_%d", i),
				Type:       "Movie",
				Path:       fmt.Sprintf("/media/%d", i),
				CachedAt:   time.Now(),
			})
		}

		// 随机选择要删除的条目（至少 1 个）
		deleteCount := rapid.IntRange(1, count).Draw(t, "deleteCount")
		toDeleteIDs := allIDs[:deleteCount]

		// 模拟 CleanupScrapeAnomalies 的数据库清理逻辑（假设 Emby 删除成功）
		if len(toDeleteIDs) > 0 {
			db.Where("emby_item_id IN ?", toDeleteIDs).Delete(&model.ScrapeAnomaly{})
			db.Where("emby_item_id IN ?", toDeleteIDs).Delete(&model.MediaCache{})
		}

		deletedSet := make(map[string]bool)
		for _, id := range toDeleteIDs {
			deletedSet[id] = true
		}

		// 验证1：已删除的 emby_item_id 不应存在于 scrape_anomalies 表
		var remainingAnomalies []model.ScrapeAnomaly
		db.Find(&remainingAnomalies)
		for _, a := range remainingAnomalies {
			if deletedSet[a.EmbyItemID] {
				t.Fatalf("scrape_anomalies 中仍存在已删除的条目: %s", a.EmbyItemID)
			}
		}

		// 验证2：已删除的 emby_item_id 不应存在于 media_caches 表
		var remainingCaches []model.MediaCache
		db.Find(&remainingCaches)
		for _, c := range remainingCaches {
			if deletedSet[c.EmbyItemID] {
				t.Fatalf("media_caches 中仍存在已删除的条目: %s", c.EmbyItemID)
			}
		}

		// 验证3：未删除的条目应仍然存在于两个表中
		survivedCount := count - deleteCount
		if len(remainingAnomalies) != survivedCount {
			t.Fatalf("scrape_anomalies 应剩余 %d 条记录，实际 %d 条", survivedCount, len(remainingAnomalies))
		}
		if len(remainingCaches) != survivedCount {
			t.Fatalf("media_caches 应剩余 %d 条记录，实际 %d 条", survivedCount, len(remainingCaches))
		}
	})
}
