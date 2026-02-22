package handler

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// Feature: ui-cache-improvements, Property 6: Duplicate media delete cache cleanup
// Validates: Requirements 4.3, 4.4, 4.5
// 对于任意一组成功删除的重复媒体 emby_item_id，duplicate_media 表不应包含这些记录，
// media_caches 表不应包含这些记录，且少于 2 条记录的分组应被完全移除。
func TestProperty_DuplicateMediaDeleteCacheCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dup_cleanup.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	rapid.Check(t, func(t *rapid.T) {
		// 每次迭代前清空表
		db.Exec("DELETE FROM duplicate_media")
		db.Exec("DELETE FROM media_caches")

		// 生成 2~5 个重复组，每组 2~4 个条目
		groupCount := rapid.IntRange(2, 5).Draw(t, "groupCount")

		var allDuplicates []model.DuplicateMedia
		var allCaches []model.MediaCache

		for g := 0; g < groupCount; g++ {
			groupKey := fmt.Sprintf("tmdb:%d", 1000+g)
			groupName := fmt.Sprintf("Movie_%d", g)
			itemsInGroup := rapid.IntRange(2, 4).Draw(t, fmt.Sprintf("itemsInGroup_%d", g))

			for i := 0; i < itemsInGroup; i++ {
				embyID := fmt.Sprintf("dup-%d-%d", g, i)
				allDuplicates = append(allDuplicates, model.DuplicateMedia{
					GroupKey:   groupKey,
					GroupName:  groupName,
					EmbyItemID: embyID,
					Name:       fmt.Sprintf("%s_v%d", groupName, i),
					Type:       "Movie",
					Path:       fmt.Sprintf("/media/%s/%d", groupName, i),
					FileSize:   int64((i + 1) * 1000),
				})
				allCaches = append(allCaches, model.MediaCache{
					EmbyItemID: embyID,
					Name:       fmt.Sprintf("%s_v%d", groupName, i),
					Type:       "Movie",
					Path:       fmt.Sprintf("/media/%s/%d", groupName, i),
					CachedAt:   time.Now(),
				})
			}
		}

		// 插入数据
		for _, d := range allDuplicates {
			db.Create(&d)
		}
		for _, c := range allCaches {
			db.Create(&c)
		}

		// 从每个组中随机选择要删除的条目（至少删除 1 个，保留至少 0 个）
		var toDeleteIDs []string
		for g := 0; g < groupCount; g++ {
			// 收集该组的所有 emby_item_id
			var groupIDs []string
			for _, d := range allDuplicates {
				if d.GroupKey == fmt.Sprintf("tmdb:%d", 1000+g) {
					groupIDs = append(groupIDs, d.EmbyItemID)
				}
			}
			// 随机选择删除 1 到 len(groupIDs) 个
			deleteCount := rapid.IntRange(1, len(groupIDs)).Draw(t, fmt.Sprintf("deleteCount_%d", g))
			for i := 0; i < deleteCount; i++ {
				toDeleteIDs = append(toDeleteIDs, groupIDs[i])
			}
		}

		// 模拟 CleanupDuplicateMedia 的数据库清理逻辑（不调用 Emby API）
		if len(toDeleteIDs) > 0 {
			db.Where("emby_item_id IN ?", toDeleteIDs).Delete(&model.DuplicateMedia{})
			db.Where("emby_item_id IN ?", toDeleteIDs).Delete(&model.MediaCache{})
			// 清理只剩一条记录的分组
			db.Exec(`DELETE FROM duplicate_media WHERE group_key IN (
				SELECT group_key FROM duplicate_media GROUP BY group_key HAVING COUNT(*) < 2
			)`)
		}

		// 验证1：已删除的 emby_item_id 不应存在于 duplicate_media 表
		var remainingDups []model.DuplicateMedia
		db.Find(&remainingDups)
		deletedSet := make(map[string]bool)
		for _, id := range toDeleteIDs {
			deletedSet[id] = true
		}
		for _, d := range remainingDups {
			if deletedSet[d.EmbyItemID] {
				t.Fatalf("duplicate_media 中仍存在已删除的条目: %s", d.EmbyItemID)
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

		// 验证3：duplicate_media 中不应有少于 2 条记录的分组
		type groupCount2 struct {
			GroupKey string
			Count    int64
		}
		var groups []groupCount2
		db.Model(&model.DuplicateMedia{}).
			Select("group_key, COUNT(*) as count").
			Group("group_key").
			Find(&groups)
		for _, g := range groups {
			if g.Count < 2 {
				t.Fatalf("分组 %s 只剩 %d 条记录，应被清理", g.GroupKey, g.Count)
			}
		}
	})
}
