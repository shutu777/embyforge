package service

import (
	"fmt"
	"testing"

	"embyforge/internal/emby"
	"embyforge/internal/model"
	"embyforge/internal/tmdb"

	"pgregory.net/rapid"
)

// Feature: embyforge, Property 4: 刮削异常检测正确性
// Validates: Requirements 4.3
// 对于任意 Movie/Series 媒体条目集合，当且仅当某个条目缺少封面或缺少外部 ID 时，
// 该条目应被标记为刮削异常。
func TestProperty_ScrapeAnomalyDetection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机数量的媒体条目，每个条目使用唯一 ID
		count := rapid.IntRange(0, 50).Draw(t, "count")
		items := make([]emby.MediaItem, count)

		for i := 0; i < count; i++ {
			hasPoster := rapid.Bool().Draw(t, "hasPoster")
			hasTmdb := rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i))
			hasImdb := rapid.Bool().Draw(t, fmt.Sprintf("hasImdb_%d", i))

			imageTags := map[string]string{}
			if hasPoster {
				imageTags["Primary"] = rapid.StringMatching(`[a-f0-9]{8}`).Draw(t, "posterTag")
			}

			providerIds := map[string]string{}
			if hasTmdb {
				providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 99999).Draw(t, fmt.Sprintf("tmdbId_%d", i)))
			}
			if hasImdb {
				providerIds["Imdb"] = fmt.Sprintf("tt%07d", rapid.IntRange(1, 99999).Draw(t, fmt.Sprintf("imdbId_%d", i)))
			}

			items[i] = emby.MediaItem{
				ID:          fmt.Sprintf("item-%d", i),
				Name:        rapid.StringMatching(`[A-Za-z0-9 ]{1,30}`).Draw(t, "name"),
				Type:        rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, "type"),
				ImageTags:   imageTags,
				ProviderIds: providerIds,
				Path:        rapid.StringMatching(`/media/[a-z]{1,10}/[a-z0-9]{1,20}`).Draw(t, "path"),
			}
		}

		// 调用检测函数
		anomalies := DetectScrapeAnomalies(items)

		// 构建异常条目 ID 集合
		anomalyIDs := make(map[string]bool)
		for _, a := range anomalies {
			anomalyIDs[a.EmbyItemID] = true
		}

		// 验证：只有 Movie 和 Series 会被检测，Episode 应被跳过
		for _, a := range anomalies {
			if a.Type != "Movie" && a.Type != "Series" {
				t.Fatalf("异常条目 %q 类型为 %s，应只检测 Movie 和 Series", a.Name, a.Type)
			}
		}

		// 验证：当且仅当缺少 Poster 或外部 ID 时，Movie/Series 条目应被标记为异常
		for _, item := range items {
			if item.Type != "Movie" && item.Type != "Series" {
				// Episode 不应出现在异常中
				if anomalyIDs[item.ID] {
					t.Fatalf("Episode 条目 %q (ID=%s) 不应被标记为异常", item.Name, item.ID)
				}
				continue
			}

			_, hasPrimary := item.ImageTags["Primary"]
			_, hasTmdbID := item.ProviderIds["Tmdb"]
			_, hasImdbID := item.ProviderIds["Imdb"]
			shouldBeAnomaly := !hasPrimary || (!hasTmdbID && !hasImdbID)
			isAnomaly := anomalyIDs[item.ID]

			if shouldBeAnomaly && !isAnomaly {
				t.Fatalf("条目 %q (ID=%s) 应被标记为异常 (HasPoster=%v, HasTmdb=%v, HasImdb=%v) 但未被标记",
					item.Name, item.ID, hasPrimary, hasTmdbID, hasImdbID)
			}
			if !shouldBeAnomaly && isAnomaly {
				t.Fatalf("条目 %q (ID=%s) 不应被标记为异常 (HasPoster=%v, HasTmdb=%v, HasImdb=%v) 但被标记了",
					item.Name, item.ID, hasPrimary, hasTmdbID, hasImdbID)
			}
		}

		// 验证：异常记录的字段正确
		for _, a := range anomalies {
			for _, item := range items {
				if item.ID == a.EmbyItemID {
					_, hasPrimary := item.ImageTags["Primary"]
					if a.MissingPoster != !hasPrimary {
						t.Fatalf("条目 %q MissingPoster 字段不正确: got %v, want %v",
							a.Name, a.MissingPoster, !hasPrimary)
					}
					_, hasTmdbID := item.ProviderIds["Tmdb"]
					_, hasImdbID := item.ProviderIds["Imdb"]
					expectedMissingProvider := !hasTmdbID && !hasImdbID
					if a.MissingProvider != expectedMissingProvider {
						t.Fatalf("条目 %q MissingProvider 字段不正确: got %v, want %v",
							a.Name, a.MissingProvider, expectedMissingProvider)
					}
					break
				}
			}
		}
	})
}


// Feature: embyforge, Property 6: 重复媒体检测正确性
// Validates: Requirements 5.3
// 电影：同一个 TMDB ID 的 Movie 有多个条目时应被标记为重复
// 剧集：同一部剧的同一集（同 SeriesID + 季号 + 集号）有多个 Episode 条目时应被标记为重复
func TestProperty_DuplicateMediaDetection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机数量的媒体条目
		count := rapid.IntRange(0, 50).Draw(t, "count")
		items := make([]emby.MediaItem, count)

		// 使用小范围值增加碰撞概率
		seriesIDs := []string{"series-1", "series-2", "series-3"}
		seriesNameMap := map[string]string{"series-1": "ShowA", "series-2": "ShowB", "series-3": "ShowC"}

		for i := 0; i < count; i++ {
			itemType := rapid.SampledFrom([]string{"Movie", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i))

			providerIds := map[string]string{}
			var seriesID, seriesName string
			var parentIdx, idx int

			if itemType == "Movie" {
				// 电影：随机是否有 TMDB ID
				if rapid.Bool().Draw(t, fmt.Sprintf("hasTmdb_%d", i)) {
					providerIds["Tmdb"] = fmt.Sprintf("%d", rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("tmdbId_%d", i)))
				}
			} else {
				// Episode：随机 SeriesID + 季号 + 集号
				seriesID = rapid.SampledFrom(seriesIDs).Draw(t, fmt.Sprintf("seriesId_%d", i))
				seriesName = seriesNameMap[seriesID]
				parentIdx = rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("season_%d", i))
				idx = rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("episode_%d", i))
			}

			name := rapid.SampledFrom([]string{"MovieA", "MovieB", "Ep1", "Ep2"}).Draw(t, fmt.Sprintf("name_%d", i))

			// 生成路径：Episode 使用包含 Season/SxxExx 的路径格式，确保 resolve 函数能正确提取
			var path string
			if itemType == "Episode" {
				path = fmt.Sprintf("/media/lib/Season %d/Show.S%dE%d.ep.mkv", parentIdx, parentIdx, idx)
			} else {
				path = fmt.Sprintf("/media/lib/movie_%d.mkv", i)
			}

			items[i] = emby.MediaItem{
				ID:                fmt.Sprintf("item-%d", i),
				Name:              name,
				Type:              itemType,
				ProviderIds:       providerIds,
				SeriesID:          seriesID,
				SeriesName:        seriesName,
				ParentIndexNumber: parentIdx,
				IndexNumber:       idx,
				Path:              path,
				FileSize:          int64(rapid.IntRange(100, 10000).Draw(t, fmt.Sprintf("size_%d", i))),
			}
		}

		// 调用检测函数
		duplicates := DetectDuplicateMedia(items)

		// 参考模型：按相同逻辑分组
		movieGroups := make(map[string][]emby.MediaItem)
		episodeGroups := make(map[string][]emby.MediaItem)
		for _, item := range items {
			switch item.Type {
			case "Movie":
				tmdbID, ok := item.ProviderIds["Tmdb"]
				if !ok || tmdbID == "" {
					continue
				}
				key := "tmdb:movie:" + tmdbID
				movieGroups[key] = append(movieGroups[key], item)
			case "Episode":
				if item.SeriesID == "" {
					continue
				}
				seasonNum := resolveSeasonNumber(item)
				episodeNum := resolveEpisodeNumber(item)
				key := fmt.Sprintf("series:%s:S%dE%d", item.SeriesID, seasonNum, episodeNum)
				episodeGroups[key] = append(episodeGroups[key], item)
			}
		}

		// 构建结果中的 ID 集合
		duplicateIDs := make(map[string]bool)
		for _, d := range duplicates {
			duplicateIDs[d.EmbyItemID] = true
		}

		// 验证：重复条目数量应等于所有 >=2 分组的条目总数
		expectedCount := 0
		for _, groupItems := range movieGroups {
			if len(groupItems) >= 2 {
				expectedCount += len(groupItems)
			}
		}
		for _, groupItems := range episodeGroups {
			if len(groupItems) >= 2 {
				expectedCount += len(groupItems)
			}
		}
		if len(duplicates) != expectedCount {
			t.Fatalf("重复条目数量不匹配: got %d, want %d", len(duplicates), expectedCount)
		}

		// 验证：同一分组至少有 2 个条目
		groupByKey := make(map[string][]model.DuplicateMedia)
		for _, d := range duplicates {
			groupByKey[d.GroupKey] = append(groupByKey[d.GroupKey], d)
		}
		for key, group := range groupByKey {
			if len(group) < 2 {
				t.Fatalf("分组 %q 只有 %d 个条目，重复分组至少应有 2 个", key, len(group))
			}
		}
	})
}

// Feature: embyforge, Property 7: 异常映射检测正确性
// Validates: Requirements 6.3
// 对于任意电视节目及其本地季集数据和 TMDB 季集数据，
// 当且仅当某季的本地集数与 TMDB 集数不一致时，该季应被标记为异常映射。
func TestProperty_EpisodeMappingDetection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机数量的电视节目
		seriesCount := rapid.IntRange(0, 10).Draw(t, "seriesCount")
		seriesList := make([]SeriesInfo, seriesCount)

		for i := 0; i < seriesCount; i++ {
			// 生成本地季数据（1-5 季，排除特别篇 season 0）
			localSeasonCount := rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("localSeasonCount_%d", i))
			localSeasons := make([]LocalSeasonInfo, localSeasonCount)
			for j := 0; j < localSeasonCount; j++ {
				localSeasons[j] = LocalSeasonInfo{
					SeasonNumber: j + 1, // 从第 1 季开始
					EpisodeCount: rapid.IntRange(1, 30).Draw(t, fmt.Sprintf("localEp_%d_%d", i, j)),
				}
			}

			// 生成 TMDB 季数据（可能包含特别篇 season 0）
			tmdbSeasonCount := rapid.IntRange(0, 6).Draw(t, fmt.Sprintf("tmdbSeasonCount_%d", i))
			tmdbSeasons := make([]tmdb.Season, tmdbSeasonCount)
			for j := 0; j < tmdbSeasonCount; j++ {
				tmdbSeasons[j] = tmdb.Season{
					SeasonNumber: j, // 从第 0 季（特别篇）开始
					EpisodeCount: rapid.IntRange(1, 30).Draw(t, fmt.Sprintf("tmdbEp_%d_%d", i, j)),
				}
			}

			seriesList[i] = SeriesInfo{
				EmbyItemID:   fmt.Sprintf("series-%d", i),
				Name:         fmt.Sprintf("Show_%d", i),
				TmdbID:       rapid.IntRange(1000, 9999).Draw(t, fmt.Sprintf("tmdbID_%d", i)),
				LocalSeasons: localSeasons,
				TmdbSeasons:  tmdbSeasons,
			}
		}

		// 调用检测函数
		anomalies := DetectEpisodeMappingAnomalies(seriesList)

		// 构建异常集合用于查找：key = "embyItemID:seasonNumber"
		anomalySet := make(map[string]model.EpisodeMappingAnomaly)
		for _, a := range anomalies {
			key := fmt.Sprintf("%s:%d", a.EmbyItemID, a.SeasonNumber)
			anomalySet[key] = a
		}

		// 验证：对每个节目的每个本地季，当且仅当与 TMDB 不一致时应被标记
		for _, series := range seriesList {
			// 构建 TMDB 季集数映射（排除特别篇）
			tmdbMap := make(map[int]int)
			for _, s := range series.TmdbSeasons {
				if s.SeasonNumber > 0 {
					tmdbMap[s.SeasonNumber] = s.EpisodeCount
				}
			}

			for _, local := range series.LocalSeasons {
				if local.SeasonNumber <= 0 {
					continue
				}

				key := fmt.Sprintf("%s:%d", series.EmbyItemID, local.SeasonNumber)
				tmdbEp, tmdbExists := tmdbMap[local.SeasonNumber]

				shouldBeAnomaly := !tmdbExists || local.EpisodeCount != tmdbEp
				_, isAnomaly := anomalySet[key]

				if shouldBeAnomaly && !isAnomaly {
					t.Fatalf("节目 %q 第 %d 季应被标记为异常 (本地=%d, TMDB=%d, TMDB存在=%v) 但未被标记",
						series.Name, local.SeasonNumber, local.EpisodeCount, tmdbEp, tmdbExists)
				}
				if !shouldBeAnomaly && isAnomaly {
					t.Fatalf("节目 %q 第 %d 季不应被标记为异常 (本地=%d, TMDB=%d) 但被标记了",
						series.Name, local.SeasonNumber, local.EpisodeCount, tmdbEp)
				}

				// 验证异常记录的字段正确性
				if isAnomaly {
					a := anomalySet[key]
					if a.LocalEpisodes != local.EpisodeCount {
						t.Fatalf("节目 %q 第 %d 季 LocalEpisodes 不正确: got %d, want %d",
							series.Name, local.SeasonNumber, a.LocalEpisodes, local.EpisodeCount)
					}
					expectedTmdbEp := 0
					if tmdbExists {
						expectedTmdbEp = tmdbEp
					}
					if a.TmdbEpisodes != expectedTmdbEp {
						t.Fatalf("节目 %q 第 %d 季 TmdbEpisodes 不正确: got %d, want %d",
							series.Name, local.SeasonNumber, a.TmdbEpisodes, expectedTmdbEp)
					}
					expectedDiff := local.EpisodeCount - expectedTmdbEp
					if expectedDiff < 0 {
						expectedDiff = -expectedDiff
					}
					if a.Difference != expectedDiff {
						t.Fatalf("节目 %q 第 %d 季 Difference 不正确: got %d, want %d",
							series.Name, local.SeasonNumber, a.Difference, expectedDiff)
					}
				}
			}
		}

		// 验证：异常总数应等于所有不一致季的数量
		expectedAnomalyCount := 0
		for _, series := range seriesList {
			tmdbMap := make(map[int]int)
			for _, s := range series.TmdbSeasons {
				if s.SeasonNumber > 0 {
					tmdbMap[s.SeasonNumber] = s.EpisodeCount
				}
			}
			for _, local := range series.LocalSeasons {
				if local.SeasonNumber <= 0 {
					continue
				}
				tmdbEp, exists := tmdbMap[local.SeasonNumber]
				if !exists || local.EpisodeCount != tmdbEp {
					expectedAnomalyCount++
				}
			}
		}
		if len(anomalies) != expectedAnomalyCount {
			t.Fatalf("异常总数不匹配: got %d, want %d", len(anomalies), expectedAnomalyCount)
		}
	})
}
