package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// 辅助函数：创建测试数据库和 CacheService
func setupTestDB(t *testing.T) (*model.MediaCache, *CacheService, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	cs := NewCacheService(db)
	return nil, cs, func() { sqlDB.Close() }
}

// 辅助函数：从 httptest.Server URL 创建 emby.Client
func newTestEmbyClient(server *httptest.Server) *emby.Client {
	var serverPort int
	fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &serverPort)
	client := emby.NewClient("http://127.0.0.1", serverPort, "test-key")
	client.HTTPClient = server.Client()
	return client
}

// Feature: emby-webhook-sync, Property 4: Delta Update 新增/更新正确性
// Validates: Requirements 3.1
//
// 对于任意 add/update 事件集合，通过 mock Emby API 处理后，
// 本地 media_caches 应包含每个 item ID 的数据且与 Emby API 返回一致。
func TestProperty_DeltaUpdateAddCorrectness(t *testing.T) {
	tmpDir := t.TempDir()

	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("delta_add_%d.db", time.Now().UnixNano()))
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		cacheService := NewCacheService(db)
		syncLock := &SyncLock{}

		// 生成随机 add 事件
		eventCount := rapid.IntRange(1, 20).Draw(rt, "eventCount")
		events := make([]*BufferedEvent, eventCount)
		embyItems := make(map[string]emby.MediaItem)

		for i := 0; i < eventCount; i++ {
			itemID := fmt.Sprintf("add-item-%d", i)
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(rt, fmt.Sprintf("type_%d", i))
			itemName := fmt.Sprintf("Media_%d", rapid.IntRange(1, 999).Draw(rt, fmt.Sprintf("name_%d", i)))

			events[i] = &BufferedEvent{
				ItemID:    itemID,
				ItemType:  itemType,
				ItemName:  itemName,
				Operation: "add",
				Timestamp: time.Now(),
			}

			seriesID := ""
			if itemType == "Episode" {
				seriesID = fmt.Sprintf("series-%d", rapid.IntRange(1, 5).Draw(rt, fmt.Sprintf("sid_%d", i)))
			}

			embyItems[itemID] = emby.MediaItem{
				ID:        itemID,
				Name:      itemName,
				Type:      itemType,
				SeriesID:  seriesID,
				Path:      fmt.Sprintf("/media/%d", i),
				ImageTags: map[string]string{"Primary": "tag"},
			}
		}

		// 创建 mock Emby 服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			ids := r.URL.Query().Get("Ids")
			item, ok := embyItems[ids]
			if !ok {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{Items: []emby.MediaItem{}})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            []emby.MediaItem{item},
				TotalRecordCount: 1,
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestEmbyClient(server)

		ctx := context.Background()
		err = cacheService.ProcessDeltaEvents(ctx, client, syncLock, events)
		if err != nil {
			t.Fatalf("ProcessDeltaEvents 失败: %v", err)
		}

		// 验证：每个 add 事件对应的 item 应存在于 media_caches
		for _, e := range events {
			var mc model.MediaCache
			if err := db.Where("emby_item_id = ?", e.ItemID).First(&mc).Error; err != nil {
				t.Fatalf("item %s 未在 media_caches 中找到: %v", e.ItemID, err)
			}
			expectedItem := embyItems[e.ItemID]
			if mc.Name != expectedItem.Name {
				t.Fatalf("item %s Name = %q, 期望 %q", e.ItemID, mc.Name, expectedItem.Name)
			}
			if mc.Type != expectedItem.Type {
				t.Fatalf("item %s Type = %q, 期望 %q", e.ItemID, mc.Type, expectedItem.Type)
			}
		}
	})
}

// Feature: emby-webhook-sync, Property 5: Delta Update 删除正确性
// Validates: Requirements 3.2
//
// 对于任意 delete 事件集合，处理后目标条目应从 media_caches 消失，其他条目不变。
func TestProperty_DeltaUpdateDeleteCorrectness(t *testing.T) {
	tmpDir := t.TempDir()

	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("delta_del_%d.db", time.Now().UnixNano()))
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		cacheService := NewCacheService(db)
		syncLock := &SyncLock{}

		// 预填充缓存
		totalItems := rapid.IntRange(5, 30).Draw(rt, "totalItems")
		deleteCount := rapid.IntRange(1, totalItems).Draw(rt, "deleteCount")

		allIDs := make([]string, totalItems)
		for i := 0; i < totalItems; i++ {
			itemID := fmt.Sprintf("pre-item-%d", i)
			allIDs[i] = itemID
			cache := model.MediaCache{
				EmbyItemID: itemID,
				Name:       fmt.Sprintf("PreMedia_%d", i),
				Type:       "Movie",
				CachedAt:   time.Now(),
			}
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("预填充失败: %v", err)
			}
		}

		// 选择要删除的条目
		deleteIDs := make(map[string]bool)
		events := make([]*BufferedEvent, 0, deleteCount)
		for i := 0; i < deleteCount; i++ {
			id := allIDs[i]
			deleteIDs[id] = true
			events = append(events, &BufferedEvent{
				ItemID:    id,
				ItemType:  "Movie",
				Operation: "delete",
				Timestamp: time.Now(),
			})
		}

		// 创建 mock 服务器
		server := httptest.NewServer(http.NewServeMux())
		defer server.Close()

		client := newTestEmbyClient(server)

		ctx := context.Background()
		err = cacheService.ProcessDeltaEvents(ctx, client, syncLock, events)
		if err != nil {
			t.Fatalf("ProcessDeltaEvents 失败: %v", err)
		}

		// 验证：删除的条目不存在
		for id := range deleteIDs {
			var count int64
			db.Model(&model.MediaCache{}).Where("emby_item_id = ?", id).Count(&count)
			if count != 0 {
				t.Fatalf("已删除的 item %s 仍存在于 media_caches", id)
			}
		}

		// 验证：未删除的条目仍存在
		for _, id := range allIDs {
			if deleteIDs[id] {
				continue
			}
			var count int64
			db.Model(&model.MediaCache{}).Where("emby_item_id = ?", id).Count(&count)
			if count != 1 {
				t.Fatalf("未删除的 item %s 应仍存在, count=%d", id, count)
			}
		}
	})
}

// Feature: emby-webhook-sync, Property 6: Episode 变更后季缓存一致性
// Validates: Requirements 3.3, 3.4
//
// 添加/删除 Episode 后，受影响 Series 的 season_caches 的 episode_count 应正确反映实际数量。
func TestProperty_EpisodeChangeSeasonCacheConsistency(t *testing.T) {
	tmpDir := t.TempDir()

	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("delta_season_%d.db", time.Now().UnixNano()))
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		cacheService := NewCacheService(db)
		syncLock := &SyncLock{}

		seriesID := "series-100"
		seasonNumber := rapid.IntRange(1, 5).Draw(rt, "seasonNumber")

		// 预填充一些 Episode
		existingEpCount := rapid.IntRange(0, 10).Draw(rt, "existingEpCount")
		for i := 0; i < existingEpCount; i++ {
			cache := model.MediaCache{
				EmbyItemID:        fmt.Sprintf("existing-ep-%d", i),
				Name:              fmt.Sprintf("Episode %d", i+1),
				Type:              "Episode",
				SeriesID:          seriesID,
				ParentIndexNumber: seasonNumber,
				CachedAt:          time.Now(),
			}
			db.Create(&cache)
		}

		// 生成要添加的新 Episode
		addEpCount := rapid.IntRange(1, 5).Draw(rt, "addEpCount")
		addEvents := make([]*BufferedEvent, addEpCount)
		embyItems := make(map[string]emby.MediaItem)

		for i := 0; i < addEpCount; i++ {
			itemID := fmt.Sprintf("new-ep-%d", i)
			item := emby.MediaItem{
				ID:                itemID,
				Name:              fmt.Sprintf("New Episode %d", i+1),
				Type:              "Episode",
				SeriesID:          seriesID,
				ParentIndexNumber: seasonNumber,
				ImageTags:         map[string]string{"Primary": "tag"},
			}
			embyItems[itemID] = item
			addEvents[i] = &BufferedEvent{
				ItemID:    itemID,
				ItemType:  "Episode",
				ItemName:  item.Name,
				Operation: "add",
				Timestamp: time.Now(),
			}
		}

		// 创建 mock Emby 服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			ids := r.URL.Query().Get("Ids")
			item, ok := embyItems[ids]
			if !ok {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{Items: []emby.MediaItem{}})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            []emby.MediaItem{item},
				TotalRecordCount: 1,
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestEmbyClient(server)

		ctx := context.Background()
		err = cacheService.ProcessDeltaEvents(ctx, client, syncLock, addEvents)
		if err != nil {
			t.Fatalf("ProcessDeltaEvents 失败: %v", err)
		}

		// 验证季缓存的 episode_count
		var seasonCache model.SeasonCache
		err = db.Where("series_emby_item_id = ? AND season_number = ?", seriesID, seasonNumber).
			First(&seasonCache).Error
		if err != nil {
			t.Fatalf("未找到 series %s season %d 的季缓存: %v", seriesID, seasonNumber, err)
		}

		expectedCount := existingEpCount + addEpCount
		if seasonCache.EpisodeCount != expectedCount {
			t.Fatalf("season_caches episode_count = %d, 期望 %d (已有 %d + 新增 %d)",
				seasonCache.EpisodeCount, expectedCount, existingEpCount, addEpCount)
		}
	})
}
