package model

import (
	"encoding/json"
	"testing"

	"embyforge/internal/emby"

	"pgregory.net/rapid"
)

// Feature: media-cache-scan, Property 1: ProviderIDs 序列化 round-trip
// Validates: Requirements 1.2
// 对于任意有效的 map[string]string 类型的 ProviderIDs，序列化为 JSON 再反序列化回来应产生等价的 map。
func TestProperty_ProviderIDsRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机的 ProviderIDs map
		size := rapid.IntRange(0, 5).Draw(t, "mapSize")
		providerIds := make(map[string]string, size)
		for i := 0; i < size; i++ {
			key := rapid.StringMatching(`[A-Za-z]{1,10}`).Draw(t, "key")
			value := rapid.StringMatching(`[a-z0-9]{1,20}`).Draw(t, "value")
			providerIds[key] = value
		}

		// 构造一个 emby.MediaItem，包含生成的 ProviderIDs
		item := emby.MediaItem{
			ID:          "test-id",
			Name:        "test-name",
			Type:        "Movie",
			ProviderIds: providerIds,
			ImageTags:   map[string]string{},
		}

		// 通过 NewMediaCacheFromItem 序列化
		cache := NewMediaCacheFromItem(item, "TestLib")

		// 通过 ToMediaItem 反序列化
		restored := cache.ToMediaItem()

		// 验证 round-trip：反序列化后的 ProviderIds 应与原始一致
		if len(restored.ProviderIds) != len(providerIds) {
			t.Fatalf("ProviderIds 长度不匹配: got %d, want %d", len(restored.ProviderIds), len(providerIds))
		}
		for k, v := range providerIds {
			if restored.ProviderIds[k] != v {
				t.Fatalf("ProviderIds[%q] 不匹配: got %q, want %q", k, restored.ProviderIds[k], v)
			}
		}

		// 额外验证：中间的 JSON 字符串是合法 JSON
		var parsed map[string]string
		if err := json.Unmarshal([]byte(cache.ProviderIDs), &parsed); err != nil {
			t.Fatalf("ProviderIDs 不是合法 JSON: %v", err)
		}
	})
}

// Feature: media-cache-scan, Property 2: 缓存唯一性
// Validates: Requirements 1.3
// 对于任意一组包含重复 emby_item_id 的媒体条目，写入缓存后每个 emby_item_id 只应出现一次。
func TestProperty_CacheUniqueness(t *testing.T) {
	db := setupTestDB(t)

	rapid.Check(t, func(t *rapid.T) {
		// 清空缓存表
		db.Exec("DELETE FROM media_caches")

		// 生成一组随机的 emby_item_id（可能有重复）
		numItems := rapid.IntRange(1, 20).Draw(t, "numItems")
		numUniqueIDs := rapid.IntRange(1, numItems).Draw(t, "numUniqueIDs")

		// 先生成一组唯一 ID
		uniqueIDs := make([]string, numUniqueIDs)
		for i := 0; i < numUniqueIDs; i++ {
			uniqueIDs[i] = rapid.StringMatching(`[a-z0-9]{5,15}`).Draw(t, "uniqueID")
		}

		// 生成条目，从唯一 ID 中随机选取（制造重复）
		items := make([]MediaCache, numItems)
		for i := 0; i < numItems; i++ {
			idx := rapid.IntRange(0, numUniqueIDs-1).Draw(t, "idxChoice")
			items[i] = MediaCache{
				EmbyItemID:  uniqueIDs[idx],
				Name:        rapid.StringMatching(`[A-Za-z ]{1,30}`).Draw(t, "name"),
				Type:        "Movie",
				ProviderIDs: "{}",
				CachedAt:    items[0].CachedAt,
			}
			// 使用 upsert 逻辑：如果已存在则跳过
			result := db.Where("emby_item_id = ?", items[i].EmbyItemID).FirstOrCreate(&items[i])
			if result.Error != nil {
				t.Fatalf("写入缓存失败: %v", result.Error)
			}
		}

		// 验证：数据库中每个 emby_item_id 只出现一次
		var caches []MediaCache
		if err := db.Find(&caches).Error; err != nil {
			t.Fatalf("查询缓存失败: %v", err)
		}

		seen := make(map[string]bool)
		for _, c := range caches {
			if seen[c.EmbyItemID] {
				t.Fatalf("emby_item_id %q 出现了多次", c.EmbyItemID)
			}
			seen[c.EmbyItemID] = true
		}

		// 验证：缓存条目数不超过唯一 ID 数
		if len(caches) > numUniqueIDs {
			t.Fatalf("缓存条目数 %d 超过唯一 ID 数 %d", len(caches), numUniqueIDs)
		}
	})
}
