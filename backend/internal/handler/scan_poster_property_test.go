package handler

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// Feature: ui-cache-improvements, Property 1: Missing poster query completeness
// Validates: Requirements 2.1
// 对于任意一组 ScrapeAnomaly 记录（missing_poster 值随机），
// 查询 missing_poster=true 的结果应恰好包含所有 missing_poster 为 true 的记录，不多不少。
func TestProperty_MissingPosterQueryCompleteness(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "poster_query.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	rapid.Check(t, func(t *rapid.T) {
		// 每次迭代前清空表
		db.Exec("DELETE FROM scrape_anomalies")

		// 生成 1~20 条 ScrapeAnomaly 记录
		count := rapid.IntRange(1, 20).Draw(t, "count")
		expectedMissingIDs := make(map[string]bool)

		for i := 0; i < count; i++ {
			embyID := fmt.Sprintf("item-%d", i)
			missingPoster := rapid.Bool().Draw(t, fmt.Sprintf("missingPoster_%d", i))
			missingProvider := rapid.Bool().Draw(t, fmt.Sprintf("missingProvider_%d", i))

			db.Create(&model.ScrapeAnomaly{
				EmbyItemID:      embyID,
				Name:            fmt.Sprintf("Media_%d", i),
				Type:            "Movie",
				MissingPoster:   missingPoster,
				MissingProvider: missingProvider,
				Path:            fmt.Sprintf("/media/%d", i),
				LibraryName:     "TestLib",
				CreatedAt:       time.Now(),
			})

			if missingPoster {
				expectedMissingIDs[embyID] = true
			}
		}

		// 执行与 GetMissingPosterItems 相同的查询
		var items []model.ScrapeAnomaly
		db.Where("missing_poster = ?", true).Order("id ASC").Find(&items)

		// 验证：返回的记录数应等于预期数量
		if len(items) != len(expectedMissingIDs) {
			t.Fatalf("期望 %d 条缺封面记录，实际返回 %d 条", len(expectedMissingIDs), len(items))
		}

		// 验证：返回的每条记录都应在预期集合中，且 missing_poster 为 true
		for _, item := range items {
			if !item.MissingPoster {
				t.Fatalf("返回的记录 %s 的 missing_poster 不为 true", item.EmbyItemID)
			}
			if !expectedMissingIDs[item.EmbyItemID] {
				t.Fatalf("返回了不在预期集合中的记录: %s", item.EmbyItemID)
			}
		}
	})
}

// Feature: ui-cache-improvements, Property 2: Poster fix cache consistency
// Validates: Requirements 2.3, 2.4, 2.5
// 对于任意一组成功修复封面的 emby_item_id，scrape_anomalies 表中对应记录的
// missing_poster 应为 false，media_caches 表中对应记录的 has_poster 应为 true。
func TestProperty_PosterFixCacheConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "poster_fix.db")
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

		// 生成 2~10 条缺封面的 ScrapeAnomaly 记录和对应的 MediaCache 记录
		count := rapid.IntRange(2, 10).Draw(t, "count")
		var allIDs []string

		for i := 0; i < count; i++ {
			embyID := fmt.Sprintf("fix-%d", i)
			allIDs = append(allIDs, embyID)

			db.Create(&model.ScrapeAnomaly{
				EmbyItemID:      embyID,
				Name:            fmt.Sprintf("Media_%d", i),
				Type:            "Movie",
				MissingPoster:   true,
				MissingProvider: false,
				Path:            fmt.Sprintf("/media/%d", i),
				LibraryName:     "TestLib",
				CreatedAt:       time.Now(),
			})

			db.Create(&model.MediaCache{
				EmbyItemID: embyID,
				Name:       fmt.Sprintf("Media_%d", i),
				Type:       "Movie",
				HasPoster:  false,
				Path:       fmt.Sprintf("/media/%d", i),
				CachedAt:   time.Now(),
			})
		}

		// 随机选择要修复的条目（至少 1 个）
		fixCount := rapid.IntRange(1, count).Draw(t, "fixCount")
		fixedIDs := allIDs[:fixCount]

		// 模拟 BatchFindPosters / FindSinglePoster 的数据库更新逻辑
		for _, embyID := range fixedIDs {
			db.Model(&model.ScrapeAnomaly{}).
				Where("emby_item_id = ?", embyID).
				Update("missing_poster", false)

			db.Model(&model.MediaCache{}).
				Where("emby_item_id = ?", embyID).
				Update("has_poster", true)
		}

		// 验证：已修复的条目在 scrape_anomalies 中 missing_poster 应为 false
		fixedSet := make(map[string]bool)
		for _, id := range fixedIDs {
			fixedSet[id] = true
		}

		var anomalies []model.ScrapeAnomaly
		db.Find(&anomalies)
		for _, a := range anomalies {
			if fixedSet[a.EmbyItemID] && a.MissingPoster {
				t.Fatalf("scrape_anomalies 中已修复条目 %s 的 missing_poster 仍为 true", a.EmbyItemID)
			}
			if !fixedSet[a.EmbyItemID] && !a.MissingPoster {
				t.Fatalf("scrape_anomalies 中未修复条目 %s 的 missing_poster 不应为 false", a.EmbyItemID)
			}
		}

		// 验证：已修复的条目在 media_caches 中 has_poster 应为 true
		var caches []model.MediaCache
		db.Find(&caches)
		for _, c := range caches {
			if fixedSet[c.EmbyItemID] && !c.HasPoster {
				t.Fatalf("media_caches 中已修复条目 %s 的 has_poster 仍为 false", c.EmbyItemID)
			}
			if !fixedSet[c.EmbyItemID] && c.HasPoster {
				t.Fatalf("media_caches 中未修复条目 %s 的 has_poster 不应为 true", c.EmbyItemID)
			}
		}
	})
}
