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

// Feature: media-cache-scan, Property 5: 缓存分析等价性（刮削异常）
// Validates: Requirements 4.1, 4.2
// 对于任意一组媒体条目，将其转换为 MediaCache 记录后再从缓存分析，
// 应产生与直接调用 DetectScrapeAnomalies 相同的刮削异常结果。
func TestProperty_CacheAnalysisEquivalence_ScrapeAnomaly(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "scrape_equiv.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目
		count := rapid.IntRange(0, 30).Draw(t, "count")
		items := make([]emby.MediaItem, count)

		for i := 0; i < count; i++ {
			hasPoster := rapid.Bool().Draw(t, fmt.Sprintf("poster_%d", i))

			imageTags := map[string]string{}
			if hasPoster {
				imageTags["Primary"] = "tag"
			}

			providerIds := map[string]string{}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
				providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("tmdb_%d", i)))
			}

			items[i] = emby.MediaItem{
				ID:          fmt.Sprintf("item-%d", i),
				Name:        rapid.StringMatching(`[A-Za-z0-9 ]{1,20}`).Draw(t, fmt.Sprintf("name_%d", i)),
				Type:        rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i)),
				ImageTags:   imageTags,
				Path:        fmt.Sprintf("/media/%d", i),
				ProviderIds: providerIds,
				FileSize:    int64(rapid.IntRange(0, 99999).Draw(t, fmt.Sprintf("size_%d", i))),
			}
		}

		// 直接调用纯逻辑函数得到参考结果
		directAnomalies := DetectScrapeAnomalies(items)

		// 将条目写入缓存
		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM scrape_anomalies")
		for _, item := range items {
			cache := model.NewMediaCacheFromItem(item, "")
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("写入缓存失败: %v", err)
			}
		}

		// 从缓存分析
		_, err := scanService.AnalyzeScrapeAnomaliesFromCache()
		if err != nil {
			t.Fatalf("缓存分析失败: %v", err)
		}

		// 读取分析结果
		var cacheAnomalies []model.ScrapeAnomaly
		db.Find(&cacheAnomalies)

		// 验证：异常数量一致
		if len(cacheAnomalies) != len(directAnomalies) {
			t.Fatalf("异常数量不匹配: 直接=%d, 缓存=%d", len(directAnomalies), len(cacheAnomalies))
		}

		// 按 EmbyItemID 排序后逐条比较
		sort.Slice(directAnomalies, func(i, j int) bool {
			return directAnomalies[i].EmbyItemID < directAnomalies[j].EmbyItemID
		})
		sort.Slice(cacheAnomalies, func(i, j int) bool {
			return cacheAnomalies[i].EmbyItemID < cacheAnomalies[j].EmbyItemID
		})

		for i := range directAnomalies {
			d := directAnomalies[i]
			c := cacheAnomalies[i]
			if d.EmbyItemID != c.EmbyItemID {
				t.Fatalf("第 %d 条异常 EmbyItemID 不匹配: 直接=%s, 缓存=%s", i, d.EmbyItemID, c.EmbyItemID)
			}
			if d.MissingPoster != c.MissingPoster {
				t.Fatalf("条目 %s MissingPoster 不匹配: 直接=%v, 缓存=%v", d.EmbyItemID, d.MissingPoster, c.MissingPoster)
			}
			if d.MissingProvider != c.MissingProvider {
				t.Fatalf("条目 %s MissingProvider 不匹配: 直接=%v, 缓存=%v", d.EmbyItemID, d.MissingProvider, c.MissingProvider)
			}
		}
	})
}


// Feature: media-cache-scan, Property 6: 缓存分析等价性（重复媒体）
// Validates: Requirements 5.1, 5.2
// 对于任意一组媒体条目，将其转换为 MediaCache 记录后再从缓存分析，
// 应产生与直接调用 DetectDuplicateMedia 相同的重复媒体分组结果。
func TestProperty_CacheAnalysisEquivalence_DuplicateMedia(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dup_equiv.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目（小范围名称和 ID 增加重复概率）
		count := rapid.IntRange(0, 30).Draw(t, "count")
		items := make([]emby.MediaItem, count)

		for i := 0; i < count; i++ {
			providerIds := map[string]string{}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
				providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("tmdb_%d", i)))
			}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasImdb_%d", i)) {
				providerIds["Imdb"] = fmt.Sprintf("tt%04d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("imdb_%d", i)))
			}

			name := rapid.SampledFrom([]string{"MovieA", "MovieB", "MovieC", "ShowX", "ShowY"}).Draw(t, fmt.Sprintf("name_%d", i))

			items[i] = emby.MediaItem{
				ID:          fmt.Sprintf("item-%d", i),
				Name:        name,
				Type:        rapid.SampledFrom([]string{"Movie", "Series"}).Draw(t, fmt.Sprintf("type_%d", i)),
				ImageTags:   map[string]string{"Primary": "tag"},
				Path:        fmt.Sprintf("/media/%d", i),
				ProviderIds: providerIds,
				FileSize:    int64(rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("size_%d", i))),
			}
		}

		// 直接调用纯逻辑函数得到参考结果
		directDuplicates := DetectDuplicateMedia(items)

		// 将条目写入缓存
		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM duplicate_media")
		for _, item := range items {
			cache := model.NewMediaCacheFromItem(item, "")
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("写入缓存失败: %v", err)
			}
		}

		// 从缓存分析
		_, err := scanService.AnalyzeDuplicateMediaFromCache()
		if err != nil {
			t.Fatalf("缓存分析失败: %v", err)
		}

		// 读取分析结果
		var cacheDuplicates []model.DuplicateMedia
		db.Find(&cacheDuplicates)

		// 验证：重复条目数量一致
		if len(cacheDuplicates) != len(directDuplicates) {
			t.Fatalf("重复条目数量不匹配: 直接=%d, 缓存=%d", len(directDuplicates), len(cacheDuplicates))
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
				t.Fatalf("第 %d 条 GroupKey 不匹配: 直接=%s, 缓存=%s", i, d.GroupKey, c.GroupKey)
			}
			if d.EmbyItemID != c.EmbyItemID {
				t.Fatalf("第 %d 条 EmbyItemID 不匹配: 直接=%s, 缓存=%s", i, d.EmbyItemID, c.EmbyItemID)
			}
		}
	})
}


// Feature: media-cache-scan, Property 7: 缓存分析等价性（异常映射）
// Validates: Requirements 6.1, 6.2
// 对于任意一组 SeriesInfo（含本地季和 TMDB 季），将其转换为缓存数据后再从缓存分析，
// 应产生与直接调用 DetectEpisodeMappingAnomalies 相同的异常映射结果。
func TestProperty_CacheAnalysisEquivalence_EpisodeMapping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ep_equiv.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机电视节目
		seriesCount := rapid.IntRange(0, 5).Draw(t, "seriesCount")

		type testSeriesData struct {
			seriesInfo  SeriesInfo
			tmdbDetails tmdb.TVShowDetails
		}
		var allSeries []testSeriesData

		for i := 0; i < seriesCount; i++ {
			tmdbID := 1000 + i
			seasonCount := rapid.IntRange(1, 4).Draw(t, fmt.Sprintf("sc_%d", i))

			var localSeasons []LocalSeasonInfo
			var tmdbSeasons []tmdb.Season

			// TMDB 特别篇
			tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: 0, EpisodeCount: 5})

			for j := 1; j <= seasonCount; j++ {
				localEp := rapid.IntRange(1, 20).Draw(t, fmt.Sprintf("le_%d_%d", i, j))
				tmdbEp := rapid.IntRange(1, 20).Draw(t, fmt.Sprintf("te_%d_%d", i, j))
				localSeasons = append(localSeasons, LocalSeasonInfo{
					SeasonNumber: j,
					EpisodeCount: localEp,
				})
				tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: j, EpisodeCount: tmdbEp})
			}

			si := SeriesInfo{
				EmbyItemID:   fmt.Sprintf("series-%d", i),
				Name:         fmt.Sprintf("Show_%d", i),
				TmdbID:       tmdbID,
				LocalSeasons: localSeasons,
				TmdbSeasons:  tmdbSeasons,
			}

			allSeries = append(allSeries, testSeriesData{
				seriesInfo: si,
				tmdbDetails: tmdb.TVShowDetails{
					ID:      tmdbID,
					Name:    si.Name,
					Seasons: tmdbSeasons,
				},
			})
		}

		// 直接调用纯逻辑函数得到参考结果
		var allSeriesInfos []SeriesInfo
		for _, s := range allSeries {
			allSeriesInfos = append(allSeriesInfos, s.seriesInfo)
		}
		directAnomalies := DetectEpisodeMappingAnomalies(allSeriesInfos)

		// 将数据写入缓存
		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM season_caches")
		db.Exec("DELETE FROM episode_mapping_anomalies")

		for _, s := range allSeries {
			// 写入 Series 到 media_cache
			providerJSON := fmt.Sprintf(`{"Tmdb":"%d"}`, s.seriesInfo.TmdbID)
			cache := model.MediaCache{
				EmbyItemID:  s.seriesInfo.EmbyItemID,
				Name:        s.seriesInfo.Name,
				Type:        "Series",
				HasPoster:   true,
				ProviderIDs: providerJSON,
				CachedAt:    time.Now(),
			}
			if err := db.Create(&cache).Error; err != nil {
				t.Fatalf("写入 Series 缓存失败: %v", err)
			}

			// 写入季信息到 season_cache
			for j, ls := range s.seriesInfo.LocalSeasons {
				sc := model.SeasonCache{
					SeriesEmbyItemID: s.seriesInfo.EmbyItemID,
					SeasonEmbyItemID: fmt.Sprintf("season-%s-%d", s.seriesInfo.EmbyItemID, j),
					SeasonNumber:     ls.SeasonNumber,
					EpisodeCount:     ls.EpisodeCount,
					CachedAt:         time.Now(),
				}
				if err := db.Create(&sc).Error; err != nil {
					t.Fatalf("写入季缓存失败: %v", err)
				}
			}
		}

		// 创建模拟 TMDB 服务器
		mux := http.NewServeMux()
		for _, s := range allSeries {
			details := s.tmdbDetails
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

		// 从缓存分析
		_, err := scanService.AnalyzeEpisodeMappingFromCache(tmdbClient)
		if err != nil {
			t.Fatalf("缓存分析失败: %v", err)
		}

		// 读取分析结果
		var cacheAnomalies []model.EpisodeMappingAnomaly
		db.Find(&cacheAnomalies)

		// 验证：异常数量一致
		if len(cacheAnomalies) != len(directAnomalies) {
			t.Fatalf("异常数量不匹配: 直接=%d, 缓存=%d", len(directAnomalies), len(cacheAnomalies))
		}

		// 按 EmbyItemID + SeasonNumber 排序后比较
		sort.Slice(directAnomalies, func(i, j int) bool {
			if directAnomalies[i].EmbyItemID != directAnomalies[j].EmbyItemID {
				return directAnomalies[i].EmbyItemID < directAnomalies[j].EmbyItemID
			}
			return directAnomalies[i].SeasonNumber < directAnomalies[j].SeasonNumber
		})
		sort.Slice(cacheAnomalies, func(i, j int) bool {
			if cacheAnomalies[i].EmbyItemID != cacheAnomalies[j].EmbyItemID {
				return cacheAnomalies[i].EmbyItemID < cacheAnomalies[j].EmbyItemID
			}
			return cacheAnomalies[i].SeasonNumber < cacheAnomalies[j].SeasonNumber
		})

		for i := range directAnomalies {
			d := directAnomalies[i]
			c := cacheAnomalies[i]
			if d.EmbyItemID != c.EmbyItemID || d.SeasonNumber != c.SeasonNumber {
				t.Fatalf("第 %d 条异常不匹配: 直接=(%s, S%d), 缓存=(%s, S%d)",
					i, d.EmbyItemID, d.SeasonNumber, c.EmbyItemID, c.SeasonNumber)
			}
			if d.LocalEpisodes != c.LocalEpisodes || d.TmdbEpisodes != c.TmdbEpisodes {
				t.Fatalf("条目 %s S%d 集数不匹配: 直接=(本地=%d, TMDB=%d), 缓存=(本地=%d, TMDB=%d)",
					d.EmbyItemID, d.SeasonNumber, d.LocalEpisodes, d.TmdbEpisodes, c.LocalEpisodes, c.TmdbEpisodes)
			}
		}
	})
}


// Feature: media-cache-scan, Property 8: 分析幂等性
// Validates: Requirements 4.3, 5.3, 6.3
// 对于任意缓存数据，连续执行两次相同的分析操作应产生完全相同的结果。
func TestProperty_AnalysisIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "analysis_idempotency.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目并写入缓存
		count := rapid.IntRange(0, 20).Draw(t, "count")
		db.Exec("DELETE FROM media_caches")
		db.Exec("DELETE FROM season_caches")

		seriesCount := 0
		for i := 0; i < count; i++ {
			hasPoster := rapid.Bool().Draw(t, fmt.Sprintf("poster_%d", i))
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i))

			providerIds := map[string]string{}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
				providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("tmdb_%d", i)))
			}

			item := emby.MediaItem{
				ID:          fmt.Sprintf("item-%d", i),
				Name:        rapid.SampledFrom([]string{"A", "B", "C", "D"}).Draw(t, fmt.Sprintf("name_%d", i)),
				Type:        itemType,
				ImageTags:   map[string]string{},
				Path:        fmt.Sprintf("/media/%d", i),
				ProviderIds: providerIds,
				FileSize:    int64(rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("size_%d", i))),
			}
			if hasPoster {
				item.ImageTags["Primary"] = "tag"
			}

			cache := model.NewMediaCacheFromItem(item, "")
			db.Create(&cache)

			if itemType == "Series" {
				seriesCount++
				// 为 Series 添加季缓存
				seasonCount := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("sc_%d", i))
				for j := 1; j <= seasonCount; j++ {
					sc := model.SeasonCache{
						SeriesEmbyItemID: item.ID,
						SeasonEmbyItemID: fmt.Sprintf("season-%d-%d", i, j),
						SeasonNumber:     j,
						EpisodeCount:     rapid.IntRange(1, 20).Draw(t, fmt.Sprintf("ep_%d_%d", i, j)),
						CachedAt:         time.Now(),
					}
					db.Create(&sc)
				}
			}
		}

		// 测试刮削异常分析幂等性
		r1, err := scanService.AnalyzeScrapeAnomaliesFromCache()
		if err != nil {
			t.Fatalf("第一次刮削分析失败: %v", err)
		}
		var scrape1 []model.ScrapeAnomaly
		db.Order("emby_item_id").Find(&scrape1)

		r2, err := scanService.AnalyzeScrapeAnomaliesFromCache()
		if err != nil {
			t.Fatalf("第二次刮削分析失败: %v", err)
		}
		var scrape2 []model.ScrapeAnomaly
		db.Order("emby_item_id").Find(&scrape2)

		if r1.AnomalyCount != r2.AnomalyCount || r1.TotalScanned != r2.TotalScanned {
			t.Fatalf("刮削分析摘要不一致: 第一次=%+v, 第二次=%+v", r1, r2)
		}
		if len(scrape1) != len(scrape2) {
			t.Fatalf("刮削异常记录数不一致: %d vs %d", len(scrape1), len(scrape2))
		}
		for i := range scrape1 {
			if scrape1[i].EmbyItemID != scrape2[i].EmbyItemID ||
				scrape1[i].MissingPoster != scrape2[i].MissingPoster ||
				scrape1[i].MissingProvider != scrape2[i].MissingProvider {
				t.Fatalf("刮削异常第 %d 条记录不一致", i)
			}
		}

		// 测试重复媒体分析幂等性
		d1, err := scanService.AnalyzeDuplicateMediaFromCache()
		if err != nil {
			t.Fatalf("第一次重复分析失败: %v", err)
		}
		var dup1 []model.DuplicateMedia
		db.Order("group_key, emby_item_id").Find(&dup1)

		d2, err := scanService.AnalyzeDuplicateMediaFromCache()
		if err != nil {
			t.Fatalf("第二次重复分析失败: %v", err)
		}
		var dup2 []model.DuplicateMedia
		db.Order("group_key, emby_item_id").Find(&dup2)

		if d1.AnomalyCount != d2.AnomalyCount || d1.TotalScanned != d2.TotalScanned {
			t.Fatalf("重复分析摘要不一致: 第一次=%+v, 第二次=%+v", d1, d2)
		}
		if len(dup1) != len(dup2) {
			t.Fatalf("重复媒体记录数不一致: %d vs %d", len(dup1), len(dup2))
		}
		for i := range dup1 {
			if dup1[i].GroupKey != dup2[i].GroupKey || dup1[i].EmbyItemID != dup2[i].EmbyItemID {
				t.Fatalf("重复媒体第 %d 条记录不一致", i)
			}
		}

		// 测试异常映射分析幂等性（仅当有 Series 时）
		if seriesCount > 0 {
			// 创建模拟 TMDB 服务器（返回空季数据，确保产生异常）
			tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(tmdb.TVShowDetails{
					ID:      1,
					Name:    "Test",
					Seasons: []tmdb.Season{},
				})
			}))
			defer tmdbServer.Close()

			tmdbClient := &tmdb.Client{
				APIKey:     "test-key",
				BaseURL:    tmdbServer.URL,
				HTTPClient: tmdbServer.Client(),
			}

			e1, err := scanService.AnalyzeEpisodeMappingFromCache(tmdbClient)
			if err != nil {
				t.Fatalf("第一次映射分析失败: %v", err)
			}
			var ep1 []model.EpisodeMappingAnomaly
			db.Order("emby_item_id, season_number").Find(&ep1)

			e2, err := scanService.AnalyzeEpisodeMappingFromCache(tmdbClient)
			if err != nil {
				t.Fatalf("第二次映射分析失败: %v", err)
			}
			var ep2 []model.EpisodeMappingAnomaly
			db.Order("emby_item_id, season_number").Find(&ep2)

			if e1.AnomalyCount != e2.AnomalyCount || e1.TotalScanned != e2.TotalScanned {
				t.Fatalf("映射分析摘要不一致: 第一次=%+v, 第二次=%+v", e1, e2)
			}
			if len(ep1) != len(ep2) {
				t.Fatalf("映射异常记录数不一致: %d vs %d", len(ep1), len(ep2))
			}
			for i := range ep1 {
				if ep1[i].EmbyItemID != ep2[i].EmbyItemID ||
					ep1[i].SeasonNumber != ep2[i].SeasonNumber ||
					ep1[i].LocalEpisodes != ep2[i].LocalEpisodes ||
					ep1[i].TmdbEpisodes != ep2[i].TmdbEpisodes {
					t.Fatalf("映射异常第 %d 条记录不一致", i)
				}
			}
		}
	})
}
