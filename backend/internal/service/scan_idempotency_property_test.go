package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"embyforge/internal/emby"
	"embyforge/internal/model"
	"embyforge/internal/tmdb"

	"pgregory.net/rapid"
)

// Feature: embyforge, Property 5: 扫描幂等性
// Validates: Requirements 4.4, 5.5, 6.5
// 对于任意扫描类型（刮削异常、重复媒体、异常映射），在相同的数据源下连续执行两次扫描，
// 第二次扫描的结果集应与第一次完全相同（表被清空重建）。

// TestProperty_ScrapeIdempotency 测试刮削异常扫描的幂等性
func TestProperty_ScrapeIdempotency(t *testing.T) {
	// 在 rapid.Check 外创建数据库（使用 testing.T 的 TempDir）
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "scrape_idempotency.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目
		count := rapid.IntRange(0, 30).Draw(t, "itemCount")
		items := make([]emby.MediaItem, count)
		for i := 0; i < count; i++ {
			hasPoster := rapid.Bool().Draw(t, fmt.Sprintf("poster_%d", i))
			imageTags := map[string]string{}
			if hasPoster {
				imageTags["Primary"] = "tag"
			}
			items[i] = emby.MediaItem{
				ID:        fmt.Sprintf("item-%d", i),
				Name:      fmt.Sprintf("Media_%d", i),
				Type:      rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i)),
				ImageTags: imageTags,
				Path:      fmt.Sprintf("/media/%d", i),
			}
		}

		// 创建模拟 Emby 服务器
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            items,
				TotalRecordCount: len(items),
			})
		}))
		defer server.Close()
		client := parseEmbyClient(server)

		// 第一次扫描
		result1, err := scanService.ScanScrapeAnomalies(client)
		if err != nil {
			t.Fatalf("第一次扫描失败: %v", err)
		}
		var rows1 []model.ScrapeAnomaly
		scanService.DB.Order("emby_item_id").Find(&rows1)

		// 第二次扫描（相同数据）
		result2, err := scanService.ScanScrapeAnomalies(client)
		if err != nil {
			t.Fatalf("第二次扫描失败: %v", err)
		}
		var rows2 []model.ScrapeAnomaly
		scanService.DB.Order("emby_item_id").Find(&rows2)

		// 验证摘要一致
		if result1.TotalScanned != result2.TotalScanned || result1.AnomalyCount != result2.AnomalyCount {
			t.Fatalf("扫描摘要不一致: 第一次=%+v, 第二次=%+v", result1, result2)
		}

		// 验证记录数和内容一致
		if len(rows1) != len(rows2) {
			t.Fatalf("记录数不一致: %d vs %d", len(rows1), len(rows2))
		}
		for i := range rows1 {
			if rows1[i].EmbyItemID != rows2[i].EmbyItemID ||
				rows1[i].MissingPoster != rows2[i].MissingPoster ||
				rows1[i].MissingProvider != rows2[i].MissingProvider {
				t.Fatalf("第 %d 条记录不一致", i)
			}
		}
	})
}

// TestProperty_DuplicateIdempotency 测试重复媒体扫描的幂等性
func TestProperty_DuplicateIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dup_idempotency.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	scanService := NewScanService(db)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机媒体条目（小范围名称增加重复概率）
		count := rapid.IntRange(0, 30).Draw(t, "itemCount")
		items := make([]emby.MediaItem, count)
		for i := 0; i < count; i++ {
			providerIds := map[string]string{}
			if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
				providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("tid_%d", i)))
			}
			items[i] = emby.MediaItem{
				ID:          fmt.Sprintf("item-%d", i),
				Name:        rapid.SampledFrom([]string{"A", "B", "C"}).Draw(t, fmt.Sprintf("name_%d", i)),
				Type:        "Movie",
				ProviderIds: providerIds,
				Path:        fmt.Sprintf("/media/%d", i),
				FileSize:    int64(rapid.IntRange(100, 9999).Draw(t, fmt.Sprintf("sz_%d", i))),
				ImageTags:   map[string]string{"Primary": "tag"},
			}
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            items,
				TotalRecordCount: len(items),
			})
		}))
		defer server.Close()
		client := parseEmbyClient(server)

		// 第一次扫描
		result1, err := scanService.ScanDuplicateMedia(client)
		if err != nil {
			t.Fatalf("第一次扫描失败: %v", err)
		}
		var rows1 []model.DuplicateMedia
		scanService.DB.Order("group_key, emby_item_id").Find(&rows1)

		// 第二次扫描
		result2, err := scanService.ScanDuplicateMedia(client)
		if err != nil {
			t.Fatalf("第二次扫描失败: %v", err)
		}
		var rows2 []model.DuplicateMedia
		scanService.DB.Order("group_key, emby_item_id").Find(&rows2)

		if result1.TotalScanned != result2.TotalScanned || result1.AnomalyCount != result2.AnomalyCount {
			t.Fatalf("扫描摘要不一致: 第一次=%+v, 第二次=%+v", result1, result2)
		}
		if len(rows1) != len(rows2) {
			t.Fatalf("记录数不一致: %d vs %d", len(rows1), len(rows2))
		}
		for i := range rows1 {
			if rows1[i].GroupKey != rows2[i].GroupKey ||
				rows1[i].EmbyItemID != rows2[i].EmbyItemID ||
				rows1[i].Name != rows2[i].Name {
				t.Fatalf("第 %d 条记录不一致", i)
			}
		}
	})
}

// TestProperty_EpisodeMappingIdempotency 测试异常映射扫描的幂等性
func TestProperty_EpisodeMappingIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ep_idempotency.db")
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

		type seriesData struct {
			item        emby.MediaItem
			seasons     []emby.MediaItem
			tmdbDetails tmdb.TVShowDetails
		}
		var allSeries []seriesData

		for i := 0; i < seriesCount; i++ {
			tmdbID := 1000 + i
			seasonCount := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("sc_%d", i))
			var localSeasons []emby.MediaItem
			var tmdbSeasons []tmdb.Season

			// 特别篇
			tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: 0, EpisodeCount: 5})

			for j := 1; j <= seasonCount; j++ {
				localEp := rapid.IntRange(1, 20).Draw(t, fmt.Sprintf("le_%d_%d", i, j))
				tmdbEp := rapid.IntRange(1, 20).Draw(t, fmt.Sprintf("te_%d_%d", i, j))
				localSeasons = append(localSeasons, emby.MediaItem{
					ID:          fmt.Sprintf("season-%d-%d", i, j),
					Name:        fmt.Sprintf("Season %d", j),
					Type:        "Season",
					IndexNumber: j,
					ChildCount:  localEp,
				})
				tmdbSeasons = append(tmdbSeasons, tmdb.Season{SeasonNumber: j, EpisodeCount: tmdbEp})
			}

			allSeries = append(allSeries, seriesData{
				item: emby.MediaItem{
					ID:   fmt.Sprintf("series-%d", i),
					Name: fmt.Sprintf("Show_%d", i),
					Type: "Series",
					ProviderIds: map[string]string{
						"Tmdb": fmt.Sprintf("%d", tmdbID),
					},
				},
				seasons: localSeasons,
				tmdbDetails: tmdb.TVShowDetails{
					ID:      tmdbID,
					Name:    fmt.Sprintf("Show_%d", i),
					Seasons: tmdbSeasons,
				},
			})
		}

		// 创建模拟服务器
		mux := http.NewServeMux()

		// Emby Items 端点
		seriesItems := make([]emby.MediaItem, len(allSeries))
		for i, s := range allSeries {
			seriesItems[i] = s.item
		}
		mux.HandleFunc("/emby/Items", func(w http.ResponseWriter, r *http.Request) {
			parentID := r.URL.Query().Get("ParentId")
			if parentID != "" {
				for _, s := range allSeries {
					if s.item.ID == parentID {
						json.NewEncoder(w).Encode(emby.MediaItemsResponse{
							Items:            s.seasons,
							TotalRecordCount: len(s.seasons),
						})
						return
					}
				}
				json.NewEncoder(w).Encode(emby.MediaItemsResponse{})
				return
			}
			json.NewEncoder(w).Encode(emby.MediaItemsResponse{
				Items:            seriesItems,
				TotalRecordCount: len(seriesItems),
			})
		})

		// TMDB 端点
		for _, s := range allSeries {
			details := s.tmdbDetails
			mux.HandleFunc(fmt.Sprintf("/3/tv/%d", details.ID), func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(details)
			})
		}

		server := httptest.NewServer(mux)
		defer server.Close()

		embyClient := parseEmbyClient(server)
		tmdbClient := &tmdb.Client{
			APIKey:     "test-key",
			BaseURL:    server.URL,
			HTTPClient: server.Client(),
		}

		// 第一次扫描
		result1, err := scanService.ScanEpisodeMapping(embyClient, tmdbClient)
		if err != nil {
			t.Fatalf("第一次扫描失败: %v", err)
		}
		var rows1 []model.EpisodeMappingAnomaly
		scanService.DB.Order("emby_item_id, season_number").Find(&rows1)

		// 第二次扫描
		result2, err := scanService.ScanEpisodeMapping(embyClient, tmdbClient)
		if err != nil {
			t.Fatalf("第二次扫描失败: %v", err)
		}
		var rows2 []model.EpisodeMappingAnomaly
		scanService.DB.Order("emby_item_id, season_number").Find(&rows2)

		if result1.TotalScanned != result2.TotalScanned || result1.AnomalyCount != result2.AnomalyCount {
			t.Fatalf("扫描摘要不一致: 第一次=%+v, 第二次=%+v", result1, result2)
		}
		if len(rows1) != len(rows2) {
			t.Fatalf("记录数不一致: %d vs %d", len(rows1), len(rows2))
		}
		for i := range rows1 {
			if rows1[i].EmbyItemID != rows2[i].EmbyItemID ||
				rows1[i].SeasonNumber != rows2[i].SeasonNumber ||
				rows1[i].LocalEpisodes != rows2[i].LocalEpisodes ||
				rows1[i].TmdbEpisodes != rows2[i].TmdbEpisodes {
				t.Fatalf("第 %d 条记录不一致", i)
			}
		}
	})
}

// parseEmbyClient 从 httptest.Server 创建 Emby 客户端
// 正确解析 host 和 port，使 Client.baseURL() 生成正确的 URL
func parseEmbyClient(server *httptest.Server) *emby.Client {
	u, _ := url.Parse(server.URL)
	host := fmt.Sprintf("%s://%s", u.Scheme, u.Hostname())
	port, _ := strconv.Atoi(u.Port())
	return &emby.Client{
		Host:       host,
		Port:       port,
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	}
}
