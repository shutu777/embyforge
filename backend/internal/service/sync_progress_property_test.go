package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// Feature: sync-progress, Property 1: 进度事件总数一致性
// Validates: Requirements 3.2
// 对于任意 Emby 媒体库总数 N（由 API 返回），同步过程中推送的所有 Progress_Event 的
// total 字段都应该等于 N。
//
// Feature: sync-progress, Property 2: 进度单调递增
// Validates: Requirements 4.2
// 对于任意同步过程中推送的 Progress_Event 序列，processed 值应该单调递增
// （每个事件的 processed >= 前一个事件的 processed），且最终的 processed 应该等于
// 实际写入数据库的条目数。
func TestProperty_SyncProgressEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sync_progress.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	cacheService := NewCacheService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目（数量在 0~50 之间，足以触发进度事件但不会太慢）
		itemCount := rapid.IntRange(0, 50).Draw(t, "itemCount")
		allItems := make([]emby.MediaItem, itemCount)
		for i := 0; i < itemCount; i++ {
			allItems[i] = emby.MediaItem{
				ID:          fmt.Sprintf("prog-item-%d", i),
				Name:        fmt.Sprintf("Media_%d", i),
				Type:        rapid.SampledFrom([]string{"Movie", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i)),
				ImageTags:   map[string]string{},
				Path:        fmt.Sprintf("/media/%d", i),
				ProviderIds: map[string]string{},
			}
		}

		// 创建模拟 Emby 服务器
		mux := http.NewServeMux()
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			// GetTotalItemCount 使用 Limit=0
			if r.URL.Query().Get("Limit") == "0" {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{
					Items:            nil,
					TotalRecordCount: itemCount,
				})
				return
			}
			// 子条目请求
			if r.URL.Query().Get("ParentId") != "" {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{})
				return
			}
			// 返回所有媒体条目（单页）
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            allItems,
				TotalRecordCount: itemCount,
			})
		})

		server := httptest.NewServer(mux)
		defer server.Close()
		client := parseEmbyClient(server)

		// 收集所有进度事件
		progressCh := make(chan SyncProgress, 100)
		ctx := context.Background()

		go cacheService.SyncMediaCacheWithProgress(ctx, client, progressCh)

		var events []SyncProgress
		for ev := range progressCh {
			events = append(events, ev)
		}

		// 过滤出非错误、非完成的进度事件（phase == "media"）
		var progressEvents []SyncProgress
		var doneEvent *SyncProgress
		for i := range events {
			if events[i].Error != "" {
				t.Fatalf("同步过程中收到错误事件: %s", events[i].Error)
			}
			if events[i].Done {
				doneEvent = &events[i]
				continue
			}
			progressEvents = append(progressEvents, events[i])
		}

		// Property 1: 所有进度事件的 total 字段都应等于 API 返回的总数
		for idx, ev := range progressEvents {
			if ev.Total != itemCount {
				t.Fatalf("Property 1 失败: 进度事件 #%d 的 total=%d，期望=%d",
					idx, ev.Total, itemCount)
			}
		}

		// Property 2: processed 值应单调递增（非递减）
		for i := 1; i < len(progressEvents); i++ {
			if progressEvents[i].Processed < progressEvents[i-1].Processed {
				t.Fatalf("Property 2 失败: 进度事件 #%d 的 processed=%d < 前一个事件 #%d 的 processed=%d",
					i, progressEvents[i].Processed, i-1, progressEvents[i-1].Processed)
			}
		}

		// Property 2 补充: 最终 processed 应等于实际写入数据库的条目数
		if doneEvent != nil {
			var dbCount int64
			db.Model(&model.MediaCache{}).Count(&dbCount)
			if doneEvent.Processed != int(dbCount) {
				t.Fatalf("Property 2 失败: done 事件的 processed=%d，数据库实际条目数=%d",
					doneEvent.Processed, dbCount)
			}
		}

		// 如果有条目，应该至少收到一个进度事件和一个 done 事件
		if itemCount > 0 && doneEvent == nil {
			t.Fatalf("有 %d 个条目但未收到 done 事件", itemCount)
		}
	})
}
