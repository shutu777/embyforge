package service

import (
	"fmt"
	"testing"

	"embyforge/internal/emby"
)

// TestBaiJiaJiangTan_PathFallbackFix 验证修复：
// 当 Emby 返回错误的 ParentIndexNumber 和 IndexNumber 时，
// 从 Path 中提取正确的 Season 和 Episode 编号，
// 不同 Season 下的不同集不再被误判为重复。
func TestBaiJiaJiangTan_PathFallbackFix(t *testing.T) {
	seriesID := "tmdb-237020"

	items := []emby.MediaItem{
		{ID: seriesID, Name: "百家讲坛", Type: "Series"},
		// 模拟 Emby 对超长剧集的 bug：
		// ParentIndexNumber 和 IndexNumber 都是错误的（全部返回 20 和 1）
		// 但路径中包含正确的 Season 和 Episode 编号
		{
			ID: "ep1", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/电视剧/综艺/百家讲坛 (2001) (tmdb-237020)/Season 20/百家讲坛.S20E01.第1集.strm",
		},
		{
			ID: "ep2", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/电视剧/综艺/百家讲坛 (2001) (tmdb-237020)/Season 400/百家讲坛.S400E01.第1集.strm",
		},
		{
			ID: "ep3", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/电视剧/综艺/百家讲坛 (2001) (tmdb-237020)/Season 401/百家讲坛.S401E01.第1集.strm",
		},
		{
			ID: "ep4", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/电视剧/综艺/百家讲坛 (2001) (tmdb-237020)/Season 400/百家讲坛.S400E02.第2集.strm",
		},
		{
			ID: "ep5", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/电视剧/综艺/百家讲坛 (2001) (tmdb-237020)/Season 401/百家讲坛.S401E02.第2集.strm",
		},
	}

	duplicates := DetectDuplicateMedia(items)

	if len(duplicates) != 0 {
		t.Errorf("修复后不应检测到重复，但检测到 %d 条:", len(duplicates))
		for _, d := range duplicates {
			t.Errorf("  组: %s | %s | %s", d.GroupKey, d.EmbyItemID, d.Path)
		}
	}
}

// TestBaiJiaJiangTan_RealDuplicateStillDetected 验证真正的重复仍然能被检测到：
// 同一个 Season 下同一集有多个文件
func TestBaiJiaJiangTan_RealDuplicateStillDetected(t *testing.T) {
	seriesID := "tmdb-237020"

	items := []emby.MediaItem{
		{ID: seriesID, Name: "百家讲坛", Type: "Series"},
		// 同一个 Season 400 下的 E01 有两个文件 → 真正的重复
		{
			ID: "ep1", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 400, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E01.第1集.720p.strm",
		},
		{
			ID: "ep2", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 400, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E01.第1集.1080p.strm",
		},
	}

	duplicates := DetectDuplicateMedia(items)

	if len(duplicates) != 2 {
		t.Errorf("真正的重复应检测到 2 条，实际 %d 条", len(duplicates))
	}
}

// TestBaiJiaJiangTan_ParentIndexZeroSameSeasonPath 验证：
// 路径中 Season 相同 + 文件名中集号相同 → 仍然是重复（不管 Emby 元数据如何）
func TestBaiJiaJiangTan_ParentIndexZeroSameSeasonPath(t *testing.T) {
	seriesID := "tmdb-237020"

	items := []emby.MediaItem{
		{ID: seriesID, Name: "百家讲坛", Type: "Series"},
		{
			ID: "ep1", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 0, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E01.720p.strm",
		},
		{
			ID: "ep2", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 0, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E01.1080p.strm",
		},
	}

	duplicates := DetectDuplicateMedia(items)

	if len(duplicates) != 2 {
		t.Errorf("同 Season 同集号应检测到 2 条重复，实际 %d 条", len(duplicates))
	}
}

// TestResolveSeasonNumber 单独测试 resolveSeasonNumber 的各种场景
func TestResolveSeasonNumber(t *testing.T) {
	tests := []struct {
		name     string
		item     emby.MediaItem
		expected int
	}{
		{
			name:     "路径优先于 ParentIndexNumber",
			item:     emby.MediaItem{ParentIndexNumber: 5, Path: "/media/show/Season 999/ep.strm"},
			expected: 999,
		},
		{
			name:     "ParentIndexNumber=0 从路径提取",
			item:     emby.MediaItem{ParentIndexNumber: 0, Path: "/media/show/Season 400/ep.strm"},
			expected: 400,
		},
		{
			name:     "路径无 Season 时 fallback 到 ParentIndexNumber",
			item:     emby.MediaItem{ParentIndexNumber: 3, Path: "/media/show/ep.strm"},
			expected: 3,
		},
		{
			name:     "ParentIndexNumber=0 路径无 Season 返回 0",
			item:     emby.MediaItem{ParentIndexNumber: 0, Path: "/media/show/ep.strm"},
			expected: 0,
		},
		{
			name:     "路径 Season 大小写不敏感",
			item:     emby.MediaItem{ParentIndexNumber: 0, Path: "/media/show/season 20/ep.strm"},
			expected: 20,
		},
		{
			name:     "Windows 路径",
			item:     emby.MediaItem{ParentIndexNumber: 0, Path: `C:\media\show\Season 3\ep.strm`},
			expected: 3,
		},
		{
			name:     "空路径 fallback 到 ParentIndexNumber",
			item:     emby.MediaItem{ParentIndexNumber: 7, Path: ""},
			expected: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveSeasonNumber(tc.item)
			if got != tc.expected {
				t.Errorf("resolveSeasonNumber() = %d, want %d (path: %s, parentIdx: %d)",
					got, tc.expected, tc.item.Path, tc.item.ParentIndexNumber)
			}
		})
	}
}

// TestResolveEpisodeNumber 单独测试 resolveEpisodeNumber 的各种场景
func TestResolveEpisodeNumber(t *testing.T) {
	tests := []struct {
		name     string
		item     emby.MediaItem
		expected int
	}{
		{
			name:     "从文件名 S01E05 提取集号",
			item:     emby.MediaItem{IndexNumber: 1, Path: "/media/show/Season 1/Show.S01E05.mkv"},
			expected: 5,
		},
		{
			name:     "从文件名 S400E12 提取集号（优先于 IndexNumber）",
			item:     emby.MediaItem{IndexNumber: 1, Path: "/media/百家讲坛/Season 400/百家讲坛.S400E12.第12集.strm"},
			expected: 12,
		},
		{
			name:     "大小写不敏感 s01e03",
			item:     emby.MediaItem{IndexNumber: 1, Path: "/media/show/Season 1/show.s01e03.mkv"},
			expected: 3,
		},
		{
			name:     "文件名无 SxxExx 时 fallback 到 IndexNumber",
			item:     emby.MediaItem{IndexNumber: 7, Path: "/media/show/Season 1/episode_7.mkv"},
			expected: 7,
		},
		{
			name:     "空路径 fallback 到 IndexNumber",
			item:     emby.MediaItem{IndexNumber: 3, Path: ""},
			expected: 3,
		},
		{
			name:     "Windows 路径",
			item:     emby.MediaItem{IndexNumber: 1, Path: `C:\media\show\Season 1\Show.S01E08.mkv`},
			expected: 8,
		},
		{
			name:     "多位数集号 S01E100",
			item:     emby.MediaItem{IndexNumber: 1, Path: "/media/show/Season 1/Show.S01E100.mkv"},
			expected: 100,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveEpisodeNumber(tc.item)
			if got != tc.expected {
				t.Errorf("resolveEpisodeNumber() = %d, want %d (path: %s, indexNumber: %d)",
					got, tc.expected, tc.item.Path, tc.item.IndexNumber)
			}
		})
	}
}

// TestNormalEpisodeDuplicateUnaffected 验证正常剧集的重复检测不受影响
func TestNormalEpisodeDuplicateUnaffected(t *testing.T) {
	items := []emby.MediaItem{
		{ID: "series-1", Name: "普通剧", Type: "Series"},
		// 正常剧集：ParentIndexNumber 正确，同季同集两个文件
		{
			ID: "ep1", Name: "第一集", Type: "Episode",
			SeriesID: "series-1", SeriesName: "普通剧",
			ParentIndexNumber: 1, IndexNumber: 1,
			Path: "/media/普通剧/Season 1/S01E01.720p.mkv",
		},
		{
			ID: "ep2", Name: "第一集", Type: "Episode",
			SeriesID: "series-1", SeriesName: "普通剧",
			ParentIndexNumber: 1, IndexNumber: 1,
			Path: "/media/普通剧/Season 1/S01E01.1080p.mkv",
		},
		// 不同季不同集，不重复
		{
			ID: "ep3", Name: "第二集", Type: "Episode",
			SeriesID: "series-1", SeriesName: "普通剧",
			ParentIndexNumber: 1, IndexNumber: 2,
			Path: "/media/普通剧/Season 1/S01E02.mkv",
		},
		{
			ID: "ep4", Name: "第一集", Type: "Episode",
			SeriesID: "series-1", SeriesName: "普通剧",
			ParentIndexNumber: 2, IndexNumber: 1,
			Path: "/media/普通剧/Season 2/S02E01.mkv",
		},
	}

	duplicates := DetectDuplicateMedia(items)

	if len(duplicates) != 2 {
		t.Errorf("应检测到 2 条重复（S01E01 的两个版本），实际 %d 条", len(duplicates))
		for _, d := range duplicates {
			t.Logf("  %s: %s", d.GroupKey, d.Path)
		}
	}

	// 验证重复的是 ep1 和 ep2
	ids := map[string]bool{}
	for _, d := range duplicates {
		ids[d.EmbyItemID] = true
	}
	if !ids["ep1"] || !ids["ep2"] {
		t.Errorf("重复条目应该是 ep1 和 ep2，实际: %v", ids)
	}
}

// TestMovieDuplicateUnaffected 验证电影重复检测完全不受影响
func TestMovieDuplicateUnaffected(t *testing.T) {
	items := []emby.MediaItem{
		{
			ID: "m1", Name: "电影A", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "12345"},
			Path:        "/media/电影A.720p.mkv",
		},
		{
			ID: "m2", Name: "电影A", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "12345"},
			Path:        "/media/电影A.1080p.mkv",
		},
		{
			ID: "m3", Name: "电影B", Type: "Movie",
			ProviderIds: map[string]string{"Tmdb": "99999"},
			Path:        "/media/电影B.mkv",
		},
	}

	duplicates := DetectDuplicateMedia(items)

	if len(duplicates) != 2 {
		t.Errorf("应检测到 2 条电影重复，实际 %d 条", len(duplicates))
	}

	for _, d := range duplicates {
		if d.GroupKey != "tmdb:movie:12345" {
			t.Errorf("重复组键应为 tmdb:movie:12345，实际: %s", d.GroupKey)
		}
	}
}

// TestBaiJiaJiangTanGroupKeyAnalysis 打印分组键帮助分析
func TestBaiJiaJiangTanGroupKeyAnalysis(t *testing.T) {
	seriesID := "tmdb-237020"

	items := []emby.MediaItem{
		{
			ID: "ep-s20e01", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 20/百家讲坛.S20E01.strm",
		},
		{
			ID: "ep-s400e01", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E01.strm",
		},
		{
			ID: "ep-s400e05", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 400/百家讲坛.S400E05.strm",
		},
		{
			ID: "ep-s401e01", Name: "趣读天文学", Type: "Episode",
			SeriesID: seriesID, SeriesName: "百家讲坛",
			ParentIndexNumber: 20, IndexNumber: 1,
			Path: "/media/百家讲坛/Season 401/百家讲坛.S401E01.strm",
		},
	}

	t.Log("=== 修复后的分组键（季号+集号均从路径提取） ===")
	for _, item := range items {
		seasonNum := resolveSeasonNumber(item)
		episodeNum := resolveEpisodeNumber(item)
		key := fmt.Sprintf("series:%s:S%dE%d", item.SeriesID, seasonNum, episodeNum)
		t.Logf("%-15s ParentIdx=%d Idx=%d → season=%d episode=%d → key=%s",
			item.ID, item.ParentIndexNumber, item.IndexNumber, seasonNum, episodeNum, key)
	}

	duplicates := DetectDuplicateMedia(items)
	t.Logf("检测到重复: %d 条（应为 0）", len(duplicates))
	if len(duplicates) != 0 {
		t.Errorf("不应检测到重复，但检测到 %d 条", len(duplicates))
	}
}
