package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"
	"embyforge/internal/tmdb"

	"pgregory.net/rapid"
)

// Feature: sync-and-analysis-performance, Property 6: 刮削异常类型过滤
// Validates: Requirements 7.2
//
// 对于任意包含混合类型（Movie、Series、Episode）的缓存数据，
// 刮削异常分析应只检查 Movie 和 Series 记录，
// 绝不应报告 Episode 类型的异常。
func TestProperty_ScrapeAnomalyTypeFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "scrape_filter.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目，包含混合类型
		count := rapid.IntRange(1, 40).Draw(t, "count")

		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM scrape_anomalies")

		episodeIDs := make(map[string]bool)

		for i := 0; i < count; i++ {
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i))
			hasPoster := rapid.Bool().Draw(t, fmt.Sprintf("poster_%d", i))

			providerIDs := map[string]string{}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
				providerIDs["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("tmdb_%d", i)))
			}

			providerJSON := "{}"
			if len(providerIDs) > 0 {
				if data, err := json.Marshal(providerIDs); err == nil {
					providerJSON = string(data)
				}
			}

			itemID := fmt.Sprintf("item-%d", i)
			if itemType == "Episode" {
				episodeIDs[itemID] = true
			}

			cache := model.MediaCache{
				EmbyItemID:  itemID,
				Name:        fmt.Sprintf("Media_%d", i),
				Type:        itemType,
				HasPoster:   hasPoster,
				ProviderIDs: providerJSON,
				Path:        fmt.Sprintf("/media/%d", i),
				CachedAt:    time.Now(),
			}
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("写入缓存失败: %v", err)
			}
		}

		// 执行刮削异常分析
		_, err := scanService.AnalyzeScrapeAnomaliesFromCache()
		if err != nil {
			t.Fatalf("刮削异常分析失败: %v", err)
		}

		// 读取分析结果
		var anomalies []model.ScrapeAnomaly
		db.Find(&anomalies)

		// 验证：不应有 Episode 类型的异常
		for _, a := range anomalies {
			if a.Type == "Episode" {
				t.Fatalf("刮削异常中包含 Episode 类型: ID=%s, Name=%s", a.EmbyItemID, a.Name)
			}
			// 验证只有 Movie 和 Series
			if a.Type != "Movie" && a.Type != "Series" {
				t.Fatalf("刮削异常中包含非 Movie/Series 类型: Type=%s, ID=%s", a.Type, a.EmbyItemID)
			}
		}

		// 验证：所有有异常的 Movie/Series 都应被检测到
		var movieSeriesCaches []model.MediaCache
		db.Where("type IN ?", []string{"Movie", "Series"}).Find(&movieSeriesCaches)

		expectedAnomalyIDs := make(map[string]bool)
		for _, c := range movieSeriesCaches {
			item := c.ToMediaItem()
			_, hasTmdb := item.ProviderIds["Tmdb"]
			_, hasImdb := item.ProviderIds["Imdb"]
			_, hasPrimary := item.ImageTags["Primary"]
			missingPoster := !hasPrimary
			missingProvider := !hasTmdb && !hasImdb
			if missingPoster || missingProvider {
				expectedAnomalyIDs[c.EmbyItemID] = true
			}
		}

		actualAnomalyIDs := make(map[string]bool)
		for _, a := range anomalies {
			actualAnomalyIDs[a.EmbyItemID] = true
		}

		if len(expectedAnomalyIDs) != len(actualAnomalyIDs) {
			t.Fatalf("异常数量不匹配: 期望=%d, 实际=%d", len(expectedAnomalyIDs), len(actualAnomalyIDs))
		}
		for id := range expectedAnomalyIDs {
			if !actualAnomalyIDs[id] {
				t.Fatalf("期望的异常条目 %s 未被检测到", id)
			}
		}
	})
}


// Feature: sync-and-analysis-performance, Property 7: 重复检测缓存分析等价性
// Validates: Requirements 7.3
//
// 对于任意一组包含 Movie、Series、Episode 的媒体缓存记录，
// AnalyzeDuplicateMediaFromCache（电影用 SQL + 剧集用 Go）的结果
// 应与内存中的 DetectDuplicateMedia 纯函数产生相同的重复分组。
func TestProperty_DuplicateDetectionSQLEquivalence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dup_sql_equiv.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	// 使用小范围值增加碰撞概率，确保能产生重复
	seriesIDs := []string{"series-1", "series-2", "series-3"}
	seriesNameMap := map[string]string{"series-1": "ShowA", "series-2": "ShowB", "series-3": "ShowC"}

	rapid.Check(t, func(t *rapid.T) {
		count := rapid.IntRange(0, 40).Draw(t, "count")
		items := make([]emby.MediaItem, count)

		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM duplicate_media")

		for i := 0; i < count; i++ {
			// 生成混合类型：Movie、Series、Episode
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i))

			providerIds := map[string]string{}
			var seriesID, seriesName string
			var parentIdx, idx int
			var path string

			switch itemType {
			case "Movie":
				// 电影：随机是否有 TMDB ID，使用小范围增加碰撞
				if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
					providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("tmdb_%d", i)))
				}
				path = fmt.Sprintf("/media/movies/movie_%d.mkv", i)

			case "Episode":
				// 剧集：随机 SeriesID + 季号 + 集号，使用小范围增加碰撞
				seriesID = rapid.SampledFrom(seriesIDs).Draw(t, fmt.Sprintf("seriesId_%d", i))
				seriesName = seriesNameMap[seriesID]
				parentIdx = rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("season_%d", i))
				idx = rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("episode_%d", i))
				// 路径包含 Season/SxxExx 格式，确保 resolveSeasonNumber 和 resolveEpisodeNumber 能正确提取
				path = fmt.Sprintf("/media/tv/%s/Season %d/%s.S%dE%d.ep.mkv", seriesName, parentIdx, seriesName, parentIdx, idx)

			case "Series":
				// Series 本身不参与重复检测
				path = fmt.Sprintf("/media/tv/show_%d", i)
			}

			name := rapid.SampledFrom([]string{"MovieA", "MovieB", "EpX", "EpY"}).Draw(t, fmt.Sprintf("name_%d", i))

			items[i] = emby.MediaItem{
				ID:                fmt.Sprintf("item-%d", i),
				Name:              name,
				Type:              itemType,
				ImageTags:         map[string]string{"Primary": "tag"},
				Path:              path,
				ProviderIds:       providerIds,
				SeriesID:          seriesID,
				SeriesName:        seriesName,
				ParentIndexNumber: parentIdx,
				IndexNumber:       idx,
				FileSize:          int64(rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("size_%d", i))),
			}

			// 写入缓存
			cache := model.NewMediaCacheFromItem(items[i], "")
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("写入缓存失败: %v", err)
			}
		}

		// 纯函数检测（参考结果）
		directDuplicates := DetectDuplicateMedia(items)

		// 缓存分析检测（电影用 SQL + 剧集用 Go）
		_, err := scanService.AnalyzeDuplicateMediaFromCache()
		if err != nil {
			t.Fatalf("缓存重复检测失败: %v", err)
		}

		var cacheDuplicates []model.DuplicateMedia
		db.Find(&cacheDuplicates)

		// 验证数量一致
		if len(cacheDuplicates) != len(directDuplicates) {
			t.Fatalf("重复条目数量不匹配: 纯函数=%d, 缓存分析=%d", len(directDuplicates), len(cacheDuplicates))
		}

		// 按 GroupKey + EmbyItemID 排序后比较
		sort.Slice(directDuplicates, func(i, j int) bool {
			if directDuplicates[i].GroupKey != directDuplicates[j].GroupKey {
				return directDuplicates[i].GroupKey < directDuplicates[j].GroupKey
			}
			return directDuplicates[i].EmbyItemID < directDuplicates[j].EmbyItemID
		})
		sort.Slice(cacheDuplicates, func(i, j int) bool {
			if cacheDuplicates[i].GroupKey != cacheDuplicates[j].GroupKey {
				return cacheDuplicates[i].GroupKey < cacheDuplicates[j].GroupKey
			}
			return cacheDuplicates[i].EmbyItemID < cacheDuplicates[j].EmbyItemID
		})

		for i := range directDuplicates {
			d := directDuplicates[i]
			c := cacheDuplicates[i]
			if d.GroupKey != c.GroupKey {
				t.Fatalf("第 %d 条 GroupKey 不匹配: 纯函数=%s, 缓存分析=%s", i, d.GroupKey, c.GroupKey)
			}
			if d.EmbyItemID != c.EmbyItemID {
				t.Fatalf("第 %d 条 EmbyItemID 不匹配: 纯函数=%s, 缓存分析=%s", i, d.EmbyItemID, c.EmbyItemID)
			}
		}
	})
}

// Feature: sync-and-analysis-performance, Property 8: 集数映射类型过滤
// Validates: Requirements 7.4
//
// 对于任意包含混合类型（Movie、Series、Episode）的缓存数据，
// 集数映射分析应只使用 Series 记录，不受 Movie 记录影响。
// 即：添加或移除 Movie 记录不应改变集数映射分析的结果。
func TestProperty_EpisodeMappingTypeFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ep_filter.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机 Series 数据
		seriesCount := rapid.IntRange(0, 3).Draw(t, "seriesCount")

		type testSeries struct {
			embyItemID string
			name       string
			tmdbID     int
			seasons    []LocalSeasonInfo
			tmdbDetail tmdb.TVShowDetails
		}
		var allSeries []testSeries

		for i := 0; i < seriesCount; i++ {
			tmdbID := 1000 + i
			seasonCount := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("sc_%d", i))

			var localSeasons []LocalSeasonInfo
			var tmdbSeasons []tmdb.Season
			tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: 0, EpisodeCount: 5})

			for j := 1; j <= seasonCount; j++ {
				localEp := rapid.IntRange(1, 15).Draw(t, fmt.Sprintf("le_%d_%d", i, j))
				tmdbEp := rapid.IntRange(1, 15).Draw(t, fmt.Sprintf("te_%d_%d", i, j))
				localSeasons = append(localSeasons, LocalSeasonInfo{
					SeasonNumber: j,
					EpisodeCount: localEp,
				})
				tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: j, EpisodeCount: tmdbEp})
			}

			allSeries = append(allSeries, testSeries{
				embyItemID: fmt.Sprintf("series-%d", i),
				name:       fmt.Sprintf("Show_%d", i),
				tmdbID:     tmdbID,
				seasons:    localSeasons,
				tmdbDetail: tmdb.TVShowDetails{
					ID:      tmdbID,
					Name:    fmt.Sprintf("Show_%d", i),
					Seasons: tmdbSeasons,
				},
			})
		}

		// 创建 TMDB mock 服务器
		mux := http.NewServeMux()
		for _, s := range allSeries {
			details := s.tmdbDetail
			mux.HandleFunc(fmt.Sprintf("/3/tv/%d", details.ID), func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(details)
			})
		}
		server := httptest.NewServer(mux)
		defer server.Close()

		tmdbClient := &tmdb.Client{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		}

		// === 第一次运行：只有 Series 数据（无 Movie）===
		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM season_caches")
		db.Exec("DELETE FROM episode_mapping_anomalies")
		db.Exec("DELETE FROM tmdb_caches")

		for _, s := range allSeries {
			providerJSON := fmt.Sprintf(`{"Tmdb":"%d"}`, s.tmdbID)
			cache := model.MediaCache{
				EmbyItemID:  s.embyItemID,
				Name:        s.name,
				Type:        "Series",
				HasPoster:   true,
				ProviderIDs: providerJSON,
				CachedAt:    time.Now(),
			}
			db.Create(&cache)

			for j, ls := range s.seasons {
				sc := model.SeasonCache{
					SeriesEmbyItemID: s.embyItemID,
					SeasonEmbyItemID: fmt.Sprintf("season-%s-%d", s.embyItemID, j),
					SeasonNumber:     ls.SeasonNumber,
					EpisodeCount:     ls.EpisodeCount,
					CachedAt:         time.Now(),
				}
				db.Create(&sc)
			}
		}

		result1, err := scanService.AnalyzeEpisodeMappingFromCache(tmdbClient)
		if err != nil {
			t.Fatalf("第一次分析失败: %v", err)
		}
		var anomalies1 []model.EpisodeMappingAnomaly
		db.Order("emby_item_id, season_number").Find(&anomalies1)

		// === 第二次运行：添加随机 Movie 记录 ===
		db.Exec("DELETE FROM episode_mapping_anomalies")
		db.Exec("DELETE FROM tmdb_caches")

		movieCount := rapid.IntRange(1, 10).Draw(t, "movieCount")
		for i := 0; i < movieCount; i++ {
			cache := model.MediaCache{
				EmbyItemID:  fmt.Sprintf("movie-%d", i),
				Name:        fmt.Sprintf("Movie_%d", i),
				Type:        "Movie",
				HasPoster:   true,
				ProviderIDs: fmt.Sprintf(`{"Tmdb":"%d"}`, 5000+i),
				CachedAt:    time.Now(),
			}
			db.Create(&cache)
		}

		result2, err := scanService.AnalyzeEpisodeMappingFromCache(tmdbClient)
		if err != nil {
			t.Fatalf("第二次分析失败: %v", err)
		}
		var anomalies2 []model.EpisodeMappingAnomaly
		db.Order("emby_item_id, season_number").Find(&anomalies2)

		// 验证：两次结果应完全一致
		if result1.AnomalyCount != result2.AnomalyCount {
			t.Fatalf("异常数量不一致: 无Movie=%d, 有Movie=%d", result1.AnomalyCount, result2.AnomalyCount)
		}
		if result1.TotalScanned != result2.TotalScanned {
			t.Fatalf("扫描数量不一致: 无Movie=%d, 有Movie=%d（应只扫描 Series）",
				result1.TotalScanned, result2.TotalScanned)
		}
		if len(anomalies1) != len(anomalies2) {
			t.Fatalf("异常记录数不一致: 无Movie=%d, 有Movie=%d", len(anomalies1), len(anomalies2))
		}
		for i := range anomalies1 {
			a1 := anomalies1[i]
			a2 := anomalies2[i]
			if a1.EmbyItemID != a2.EmbyItemID || a1.SeasonNumber != a2.SeasonNumber ||
				a1.LocalEpisodes != a2.LocalEpisodes || a1.TmdbEpisodes != a2.TmdbEpisodes {
				t.Fatalf("第 %d 条异常不一致: 无Movie=(%s, S%d, %d/%d), 有Movie=(%s, S%d, %d/%d)",
					i, a1.EmbyItemID, a1.SeasonNumber, a1.LocalEpisodes, a1.TmdbEpisodes,
					a2.EmbyItemID, a2.SeasonNumber, a2.LocalEpisodes, a2.TmdbEpisodes)
			}
		}
	})
}
