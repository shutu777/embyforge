package tmdb

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: symedia-manual-transfer, Property 4: TMDB search URL construction
// Validates: Requirements 3.2, 6.3
// 对于任意有效的搜索查询和 media_type（"movie" 或 "tv"），
// 构建的 URL 路径应为 /3/search/movie（movie）或 /3/search/tv（tv）。
func TestProperty_TmdbSearchURLConstruction(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 随机选择 media_type
		mediaType := rapid.SampledFrom([]string{"movie", "tv"}).Draw(t, "mediaType")
		// 生成随机搜索词（非空）
		query := rapid.StringMatching(`[a-zA-Z0-9\x{4e00}-\x{9fff}]{1,20}`).Draw(t, "query")
		// 可选的 language 参数
		language := rapid.SampledFrom([]string{"", "zh-CN", "en-US", "ja-JP"}).Draw(t, "language")

		url := BuildSearchURL(mediaType, query, language)

		// 验证 URL 路径前缀
		if mediaType == "movie" {
			if !strings.HasPrefix(url, "/3/search/movie") {
				t.Fatalf("movie 类型应以 /3/search/movie 开头，实际: %s", url)
			}
		} else {
			if !strings.HasPrefix(url, "/3/search/tv") {
				t.Fatalf("tv 类型应以 /3/search/tv 开头，实际: %s", url)
			}
		}

		// 验证包含 query 参数
		if !strings.Contains(url, "query=") {
			t.Fatalf("URL 应包含 query 参数，实际: %s", url)
		}

		// 验证 language 参数
		if language != "" {
			if !strings.Contains(url, "language=") {
				t.Fatalf("指定 language 时 URL 应包含 language 参数，实际: %s", url)
			}
		} else {
			if strings.Contains(url, "language=") {
				t.Fatalf("未指定 language 时 URL 不应包含 language 参数，实际: %s", url)
			}
		}
	})
}


// Feature: symedia-manual-transfer, Property 5: TMDB response field mapping
// Validates: Requirements 6.4
// 对于任意 TMDB 原始搜索响应（电影或剧集），映射后的结果应包含
// id、title、release_date、overview 和 poster_path，且值与原始数据一致。
func TestProperty_TmdbResponseFieldMapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 随机选择测试电影还是剧集映射
		isMovie := rapid.Bool().Draw(t, "isMovie")

		if isMovie {
			// 生成随机电影原始结果
			numResults := rapid.IntRange(0, 10).Draw(t, "numResults")
			raw := make([]TmdbSearchRawMovie, numResults)
			for i := 0; i < numResults; i++ {
				raw[i] = TmdbSearchRawMovie{
					ID:            rapid.IntRange(1, 999999).Draw(t, "id"),
					Title:         rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "title"),
					OriginalTitle: rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "originalTitle"),
					ReleaseDate:   rapid.StringMatching(`[0-9]{4}-[0-9]{2}-[0-9]{2}`).Draw(t, "releaseDate"),
					PosterPath:    rapid.StringMatching(`/[a-zA-Z0-9]{5,10}\.jpg`).Draw(t, "posterPath"),
					Overview:      rapid.StringMatching(`[a-zA-Z0-9 ]{0,50}`).Draw(t, "overview"),
				}
			}

			mapped := MapMovieResults(raw)

			if len(mapped) != len(raw) {
				t.Fatalf("映射结果数量不匹配: got %d, want %d", len(mapped), len(raw))
			}

			for i, m := range mapped {
				if m.ID != raw[i].ID {
					t.Fatalf("[%d] ID 不匹配: got %d, want %d", i, m.ID, raw[i].ID)
				}
				if m.Title != raw[i].Title {
					t.Fatalf("[%d] Title 不匹配: got %q, want %q", i, m.Title, raw[i].Title)
				}
				if m.OriginalTitle != raw[i].OriginalTitle {
					t.Fatalf("[%d] OriginalTitle 不匹配: got %q, want %q", i, m.OriginalTitle, raw[i].OriginalTitle)
				}
				if m.ReleaseDate != raw[i].ReleaseDate {
					t.Fatalf("[%d] ReleaseDate 不匹配: got %q, want %q", i, m.ReleaseDate, raw[i].ReleaseDate)
				}
				if m.PosterPath != raw[i].PosterPath {
					t.Fatalf("[%d] PosterPath 不匹配: got %q, want %q", i, m.PosterPath, raw[i].PosterPath)
				}
				if m.Overview != raw[i].Overview {
					t.Fatalf("[%d] Overview 不匹配: got %q, want %q", i, m.Overview, raw[i].Overview)
				}
			}
		} else {
			// 生成随机剧集原始结果
			numResults := rapid.IntRange(0, 10).Draw(t, "numResults")
			raw := make([]TmdbSearchRawTV, numResults)
			for i := 0; i < numResults; i++ {
				raw[i] = TmdbSearchRawTV{
					ID:           rapid.IntRange(1, 999999).Draw(t, "id"),
					Name:         rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "name"),
					OriginalName: rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "originalName"),
					FirstAirDate: rapid.StringMatching(`[0-9]{4}-[0-9]{2}-[0-9]{2}`).Draw(t, "firstAirDate"),
					PosterPath:   rapid.StringMatching(`/[a-zA-Z0-9]{5,10}\.jpg`).Draw(t, "posterPath"),
					Overview:     rapid.StringMatching(`[a-zA-Z0-9 ]{0,50}`).Draw(t, "overview"),
				}
			}

			mapped := MapTVResults(raw)

			if len(mapped) != len(raw) {
				t.Fatalf("映射结果数量不匹配: got %d, want %d", len(mapped), len(raw))
			}

			for i, m := range mapped {
				if m.ID != raw[i].ID {
					t.Fatalf("[%d] ID 不匹配: got %d, want %d", i, m.ID, raw[i].ID)
				}
				// 剧集的 Name 应映射到 Title
				if m.Title != raw[i].Name {
					t.Fatalf("[%d] Title 应映射自 Name: got %q, want %q", i, m.Title, raw[i].Name)
				}
				// 剧集的 OriginalName 应映射到 OriginalTitle
				if m.OriginalTitle != raw[i].OriginalName {
					t.Fatalf("[%d] OriginalTitle 应映射自 OriginalName: got %q, want %q", i, m.OriginalTitle, raw[i].OriginalName)
				}
				// 剧集的 FirstAirDate 应映射到 ReleaseDate
				if m.ReleaseDate != raw[i].FirstAirDate {
					t.Fatalf("[%d] ReleaseDate 应映射自 FirstAirDate: got %q, want %q", i, m.ReleaseDate, raw[i].FirstAirDate)
				}
				if m.PosterPath != raw[i].PosterPath {
					t.Fatalf("[%d] PosterPath 不匹配: got %q, want %q", i, m.PosterPath, raw[i].PosterPath)
				}
				if m.Overview != raw[i].Overview {
					t.Fatalf("[%d] Overview 不匹配: got %q, want %q", i, m.Overview, raw[i].Overview)
				}
			}
		}
	})
}
