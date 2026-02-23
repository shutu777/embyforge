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

// Feature: emby-webhook-sync, Property 7: 全量同步数据替换
// Validates: Requirements 4.1
//
// 对于任意初始缓存状态和 Emby 媒体库内容，全量同步后 media_caches 应只包含 Emby 返回的数据。
func TestProperty_FullSyncDataReplacement(t *testing.T) {
	tmpDir := t.TempDir()

	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("full_sync_%d.db", time.Now().UnixNano()))
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		cacheService := NewCacheService(db)

		// 预填充一些旧数据
		oldCount := rapid.IntRange(0, 20).Draw(rt, "oldCount")
		for i := 0; i < oldCount; i++ {
			cache := model.MediaCache{
				EmbyItemID: fmt.Sprintf("old-item-%d", i),
				Name:       fmt.Sprintf("OldMedia_%d", i),
				Type:       "Movie",
				CachedAt:   time.Now(),
			}
			db.Create(&cache)
		}

		// 生成 Emby 返回的新数据
		newCount := rapid.IntRange(0, 30).Draw(rt, "newCount")
		embyItems := make([]emby.MediaItem, newCount)
		expectedIDs := make(map[string]bool)

		for i := 0; i < newCount; i++ {
			itemID := fmt.Sprintf("new-item-%d", i)
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(rt, fmt.Sprintf("type_%d", i))
			embyItems[i] = emby.MediaItem{
				ID:        itemID,
				Name:      fmt.Sprintf("NewMedia_%d", i),
				Type:      itemType,
				ImageTags: map[string]string{"Primary": "tag"},
				Path:      fmt.Sprintf("/media/%d", i),
			}
			expectedIDs[itemID] = true
		}

		// 创建 mock Emby 服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			limit := r.URL.Query().Get("Limit")
			if limit == "0" {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{
					Items:            []emby.MediaItem{},
					TotalRecordCount: newCount,
				})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            embyItems,
				TotalRecordCount: newCount,
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestEmbyClient(server)

		ctx := context.Background()
		result, err := cacheService.SyncMediaCacheWithContext(ctx, client)
		if err != nil {
			t.Fatalf("SyncMediaCacheWithContext 失败: %v", err)
		}

		if result.TotalItems != newCount {
			t.Fatalf("同步结果 TotalItems = %d, 期望 %d", result.TotalItems, newCount)
		}

		// 验证 media_caches 只包含新数据
		var allCaches []model.MediaCache
		db.Find(&allCaches)

		if len(allCaches) != newCount {
			t.Fatalf("media_caches 条目数 = %d, 期望 %d", len(allCaches), newCount)
		}

		for _, mc := range allCaches {
			if !expectedIDs[mc.EmbyItemID] {
				t.Fatalf("media_caches 包含非预期条目: %s", mc.EmbyItemID)
			}
		}
	})
}

// Feature: emby-webhook-sync, Property 9: 全量同步期间事件缓冲
// Validates: Requirements 4.3
//
// 当 SyncLock 被 full_sync 持有时，添加到 EventBuffer 的事件应被缓冲而不被处理。
func TestProperty_EventBufferingDuringFullSync(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		syncLock := &SyncLock{}

		eb := NewEventBuffer(syncLock, func(_ context.Context, _ []*BufferedEvent) {
			// 空的 flush handler
		})
		eb.DebounceDelay = 1 * time.Millisecond

		// 模拟全量同步持有锁
		if !syncLock.TryLock("full_sync") {
			t.Fatalf("获取锁失败")
		}

		// 添加随机事件
		eventCount := rapid.IntRange(1, 50).Draw(rt, "eventCount")
		for i := 0; i < eventCount; i++ {
			eb.Add(&BufferedEvent{
				ItemID:    fmt.Sprintf("item-%d", i),
				ItemType:  "Movie",
				ItemName:  fmt.Sprintf("Movie_%d", i),
				Operation: rapid.SampledFrom([]string{"add", "delete"}).Draw(rt, fmt.Sprintf("op_%d", i)),
				Timestamp: time.Now(),
			})
		}

		// 验证事件在缓冲区中
		pending := eb.PendingCount()
		if pending == 0 {
			t.Fatalf("事件应在缓冲区中, 但 PendingCount=0")
		}

		// 使用 DrainEvents 取出事件
		events := eb.DrainEvents()
		if len(events) == 0 {
			t.Fatalf("DrainEvents 应返回缓冲的事件")
		}

		// 验证取出的事件数量（去重后应等于唯一 itemID 数量）
		if len(events) != eventCount {
			t.Fatalf("DrainEvents 返回 %d 个事件, 期望 %d", len(events), eventCount)
		}

		syncLock.Unlock()
	})
}

// Feature: emby-webhook-sync, Property 10: 同步后事件协调
// Validates: Requirements 4.4
//
// 全量同步后，协调逻辑应丢弃已存在于缓存中的 add 事件，保留 delete 事件和不存在的 add 事件。
func TestProperty_PostSyncEventReconciliation(t *testing.T) {
	tmpDir := t.TempDir()

	rapid.Check(t, func(rt *rapid.T) {
		dbPath := filepath.Join(tmpDir, fmt.Sprintf("reconcile_%d.db", time.Now().UnixNano()))
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, _ := db.DB()
		defer sqlDB.Close()

		cacheService := NewCacheService(db)

		// 模拟全量同步后的缓存状态
		existingCount := rapid.IntRange(1, 15).Draw(rt, "existingCount")
		existingIDs := make(map[string]bool)
		for i := 0; i < existingCount; i++ {
			itemID := fmt.Sprintf("synced-item-%d", i)
			existingIDs[itemID] = true
			cache := model.MediaCache{
				EmbyItemID: itemID,
				Name:       fmt.Sprintf("SyncedMedia_%d", i),
				Type:       "Movie",
				CachedAt:   time.Now(),
			}
			db.Create(&cache)
		}

		// 生成缓冲事件
		eventCount := rapid.IntRange(1, 20).Draw(rt, "eventCount")

		// 使用 map 去重（模拟 EventBuffer 行为）
		latestEvents := make(map[string]*BufferedEvent)
		for i := 0; i < eventCount; i++ {
			useExisting := rapid.Bool().Draw(rt, fmt.Sprintf("existing_%d", i))
			operation := rapid.SampledFrom([]string{"add", "delete"}).Draw(rt, fmt.Sprintf("op_%d", i))

			var itemID string
			if useExisting && existingCount > 0 {
				idx := rapid.IntRange(0, existingCount-1).Draw(rt, fmt.Sprintf("idx_%d", i))
				itemID = fmt.Sprintf("synced-item-%d", idx)
			} else {
				itemID = fmt.Sprintf("new-item-%d", i)
			}

			latestEvents[itemID] = &BufferedEvent{
				ItemID:    itemID,
				ItemType:  "Movie",
				ItemName:  fmt.Sprintf("Media_%s", itemID),
				Operation: operation,
				Timestamp: time.Now(),
			}
		}

		dedupedEvents := make([]*BufferedEvent, 0, len(latestEvents))
		for _, e := range latestEvents {
			dedupedEvents = append(dedupedEvents, e)
		}

		// 创建 mock 服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			ids := r.URL.Query().Get("Ids")
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items: []emby.MediaItem{{
					ID:        ids,
					Name:      "Reconciled_" + ids,
					Type:      "Movie",
					ImageTags: map[string]string{"Primary": "tag"},
				}},
				TotalRecordCount: 1,
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		client := newTestEmbyClient(server)
		syncLock := &SyncLock{}
		ctx := context.Background()

		cacheService.ReconcileBufferedEvents(ctx, client, syncLock, dedupedEvents)

		// 验证协调结果
		for _, e := range dedupedEvents {
			isExisting := existingIDs[e.ItemID]
			var count int64
			db.Model(&model.MediaCache{}).Where("emby_item_id = ?", e.ItemID).Count(&count)

			if e.Operation == "delete" && isExisting {
				// 删除事件 + 本地存在 → 应被删除
				if count != 0 {
					t.Fatalf("协调后 delete 事件的已存在条目 %s 应被删除, 但 count=%d", e.ItemID, count)
				}
			} else if e.Operation == "add" && !isExisting {
				// 新增事件 + 本地不存在 → 应被添加
				if count != 1 {
					t.Fatalf("协调后 add 事件的新条目 %s 应被添加, 但 count=%d", e.ItemID, count)
				}
			} else if e.Operation == "add" && isExisting {
				// add + 已存在 → 应保持不变（被丢弃）
				if count != 1 {
					t.Fatalf("协调后 add 事件的已存在条目 %s 应保持不变, 但 count=%d", e.ItemID, count)
				}
			}
		}
	})
}
