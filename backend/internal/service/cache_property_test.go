package service

import (
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

// Feature: media-cache-scan, Property 3: 同步完整性
// Validates: Requirements 2.1, 3.1
// 对于任意一组 Emby 媒体条目（包含 Movie、Series、Episode），同步操作后，
// media_cache 应包含完全相同的条目集合，season_cache 应从 Episode 数据聚合生成。
func TestProperty_SyncCompleteness(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sync_completeness.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	cacheService := NewCacheService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目
		movieCount := rapid.IntRange(0, 10).Draw(t, "movieCount")
		seriesCount := rapid.IntRange(0, 3).Draw(t, "seriesCount")

		var allItems []emby.MediaItem

		// 生成电影条目
		for i := 0; i < movieCount; i++ {
			allItems = append(allItems, emby.MediaItem{
				ID:        fmt.Sprintf("movie-%d", i),
				Name:      rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, fmt.Sprintf("movieName_%d", i)),
				Type:      "Movie",
				ImageTags: map[string]string{},
				Path:      fmt.Sprintf("/media/movies/%d", i),
				ProviderIds: map[string]string{
					"Tmdb": fmt.Sprintf("%d", rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("movieTmdb_%d", i))),
				},
				FileSize:    int64(rapid.IntRange(100, 99999).Draw(t, fmt.Sprintf("movieSize_%d", i))),
				IndexNumber: rapid.IntRange(0, 10).Draw(t, fmt.Sprintf("movieIdx_%d", i)),
				ChildCount:  0,
			})
		}

		// 记录每个 Series 每季的 Episode 数量，用于验证季缓存
		// key: "seriesID:seasonNumber" -> episodeCount
		expectedSeasons := make(map[string]int)

		// 生成 Series 条目及其 Episode 条目
		for i := 0; i < seriesCount; i++ {
			seriesID := fmt.Sprintf("series-%d", i)
			seriesName := rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, fmt.Sprintf("seriesName_%d", i))
			allItems = append(allItems, emby.MediaItem{
				ID:        seriesID,
				Name:      seriesName,
				Type:      "Series",
				ImageTags: map[string]string{},
				Path:      fmt.Sprintf("/media/series/%d", i),
				ProviderIds: map[string]string{
					"Tmdb": fmt.Sprintf("%d", rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("seriesTmdb_%d", i))),
				},
			})

			// 生成该 Series 的 Episode 条目（按季分组）
			seasonCount := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("seasonCount_%d", i))
			for j := 1; j <= seasonCount; j++ {
				epCount := rapid.IntRange(1, 6).Draw(t, fmt.Sprintf("epCount_%d_%d", i, j))
				expectedSeasons[fmt.Sprintf("%s:%d", seriesID, j)] = epCount

				for k := 1; k <= epCount; k++ {
					allItems = append(allItems, emby.MediaItem{
						ID:                fmt.Sprintf("ep-%d-%d-%d", i, j, k),
						Name:              fmt.Sprintf("S%dE%d", j, k),
						Type:              "Episode",
						ImageTags:         map[string]string{},
						Path:              fmt.Sprintf("/media/series/%d/S%d/E%d", i, j, k),
						ProviderIds:       map[string]string{},
						IndexNumber:       k,
						ParentIndexNumber: j,
						SeriesID:          seriesID,
						SeriesName:        seriesName,
					})
				}
			}
		}

		// 创建模拟 Emby 服务器
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            allItems,
				TotalRecordCount: len(allItems),
			})
		}))
		defer server.Close()
		client := parseEmbyClient(server)

		// 执行同步
		syncResult, err := cacheService.SyncMediaCache(client)
		if err != nil {
			t.Fatalf("同步失败: %v", err)
		}

		// 验证1：同步结果的条目数应等于输入条目数
		if syncResult.TotalItems != len(allItems) {
			t.Fatalf("同步条目数不匹配: got %d, want %d", syncResult.TotalItems, len(allItems))
		}

		// 验证2：media_cache 中的条目数应等于输入条目数
		var cachedItems []model.MediaCache
		if err := db.Find(&cachedItems).Error; err != nil {
			t.Fatalf("查询媒体缓存失败: %v", err)
		}
		if len(cachedItems) != len(allItems) {
			t.Fatalf("缓存条目数不匹配: got %d, want %d", len(cachedItems), len(allItems))
		}

		// 验证3：每个输入条目都应在缓存中存在，且关键字段一致
		cachedByID := make(map[string]model.MediaCache)
		for _, c := range cachedItems {
			cachedByID[c.EmbyItemID] = c
		}
		for _, item := range allItems {
			cached, ok := cachedByID[item.ID]
			if !ok {
				t.Fatalf("条目 %q (ID=%s) 未在缓存中找到", item.Name, item.ID)
			}
			if cached.Name != item.Name {
				t.Fatalf("条目 %s Name 不匹配: got %q, want %q", item.ID, cached.Name, item.Name)
			}
			if cached.Type != item.Type {
				t.Fatalf("条目 %s Type 不匹配: got %q, want %q", item.ID, cached.Type, item.Type)
			}
		}

		// 验证4：season_cache 应从 Episode 数据聚合生成
		var cachedSeasons []model.SeasonCache
		if err := db.Find(&cachedSeasons).Error; err != nil {
			t.Fatalf("查询季缓存失败: %v", err)
		}

		if len(cachedSeasons) != len(expectedSeasons) {
			t.Fatalf("季缓存条目数不匹配: got %d, want %d", len(cachedSeasons), len(expectedSeasons))
		}
		if syncResult.TotalSeasons != len(expectedSeasons) {
			t.Fatalf("同步季数不匹配: got %d, want %d", syncResult.TotalSeasons, len(expectedSeasons))
		}

		// 验证5：每个季的 Episode 数量应正确
		for _, sc := range cachedSeasons {
			key := fmt.Sprintf("%s:%d", sc.SeriesEmbyItemID, sc.SeasonNumber)
			expected, ok := expectedSeasons[key]
			if !ok {
				t.Fatalf("季缓存中存在未预期的条目: SeriesID=%s, SeasonNumber=%d", sc.SeriesEmbyItemID, sc.SeasonNumber)
			}
			if sc.EpisodeCount != expected {
				t.Fatalf("季 %s EpisodeCount 不匹配: got %d, want %d", key, sc.EpisodeCount, expected)
			}
		}
	})
}


// Feature: media-cache-scan, Property 4: 同步幂等性
// Validates: Requirements 2.2
// 对于任意两次连续同步操作（使用不同数据集），media_cache 应只包含最近一次同步的数据，
// 不应有前一次同步的残留数据。
func TestProperty_SyncIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sync_idempotency.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	cacheService := NewCacheService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成第一批数据
		count1 := rapid.IntRange(1, 10).Draw(t, "count1")
		items1 := make([]emby.MediaItem, count1)
		for i := 0; i < count1; i++ {
			items1[i] = emby.MediaItem{
				ID:          fmt.Sprintf("batch1-item-%d", i),
				Name:        fmt.Sprintf("Batch1_Media_%d", i),
				Type:        rapid.SampledFrom([]string{"Movie", "Series"}).Draw(t, fmt.Sprintf("type1_%d", i)),
				ImageTags:   map[string]string{},
				Path:        fmt.Sprintf("/media/batch1/%d", i),
				ProviderIds: map[string]string{},
			}
		}

		// 生成第二批数据（不同的 ID 前缀，确保与第一批不重叠）
		count2 := rapid.IntRange(1, 10).Draw(t, "count2")
		items2 := make([]emby.MediaItem, count2)
		for i := 0; i < count2; i++ {
			items2[i] = emby.MediaItem{
				ID:          fmt.Sprintf("batch2-item-%d", i),
				Name:        fmt.Sprintf("Batch2_Media_%d", i),
				Type:        rapid.SampledFrom([]string{"Movie", "Series"}).Draw(t, fmt.Sprintf("type2_%d", i)),
				ImageTags:   map[string]string{},
				Path:        fmt.Sprintf("/media/batch2/%d", i),
				ProviderIds: map[string]string{},
			}
		}

		// 第一次同步（使用第一批数据）
		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("ParentId") != "" {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            items1,
				TotalRecordCount: len(items1),
			})
		}))
		client1 := parseEmbyClient(server1)

		_, err := cacheService.SyncMediaCache(client1)
		server1.Close()
		if err != nil {
			t.Fatalf("第一次同步失败: %v", err)
		}

		// 验证第一次同步后的数据
		var afterFirst []model.MediaCache
		db.Find(&afterFirst)
		if len(afterFirst) != count1 {
			t.Fatalf("第一次同步后条目数不匹配: got %d, want %d", len(afterFirst), count1)
		}

		// 第二次同步（使用第二批数据）
		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("ParentId") != "" {
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            items2,
				TotalRecordCount: len(items2),
			})
		}))
		client2 := parseEmbyClient(server2)

		_, err = cacheService.SyncMediaCache(client2)
		server2.Close()
		if err != nil {
			t.Fatalf("第二次同步失败: %v", err)
		}

		// 验证：缓存中只包含第二批数据
		var afterSecond []model.MediaCache
		db.Find(&afterSecond)
		if len(afterSecond) != count2 {
			t.Fatalf("第二次同步后条目数不匹配: got %d, want %d", len(afterSecond), count2)
		}

		// 验证：没有第一批数据的残留
		for _, cached := range afterSecond {
			for _, item1 := range items1 {
				if cached.EmbyItemID == item1.ID {
					t.Fatalf("第二次同步后仍存在第一批数据: EmbyItemID=%s", cached.EmbyItemID)
				}
			}
		}

		// 验证：所有第二批数据都在缓存中
		cachedByID := make(map[string]bool)
		for _, c := range afterSecond {
			cachedByID[c.EmbyItemID] = true
		}
		for _, item := range items2 {
			if !cachedByID[item.ID] {
				t.Fatalf("第二批数据 %q (ID=%s) 未在缓存中找到", item.Name, item.ID)
			}
		}
	})
}
