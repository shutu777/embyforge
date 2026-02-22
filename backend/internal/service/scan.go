package service

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"
	"embyforge/internal/tmdb"
	"embyforge/internal/workerpool"

	"gorm.io/gorm"
)

// maxConsecutiveAuthErrors 连续认证失败（401）的最大次数，超过后中止分析
const maxConsecutiveAuthErrors = 5

// ScanResult 扫描结果摘要
type ScanResult struct {
	TotalScanned int `json:"total_scanned"` // 扫描的总条目数
	AnomalyCount int `json:"anomaly_count"` // 发现的异常数量
	ErrorCount   int `json:"error_count"`   // 扫描过程中的错误数量
}

// ScanService 扫描服务
type ScanService struct {
	DB *gorm.DB
}

// NewScanService 创建扫描服务
func NewScanService(db *gorm.DB) *ScanService {
	return &ScanService{DB: db}
}

// FormatScanSummary 格式化扫描结果摘要日志字符串
// 接收扫描类型名称和 ScanResult，返回格式化的日志字符串
func FormatScanSummary(scanType string, result *ScanResult) string {
	return fmt.Sprintf("✅ %s扫描完成: 共扫描 %d 个条目, 发现 %d 个异常, %d 个错误",
		scanType, result.TotalScanned, result.AnomalyCount, result.ErrorCount)
}

// ScanScrapeAnomalies 扫描刮削异常
// 检查每个媒体条目是否缺少封面图片或外部 ID
func (s *ScanService) ScanScrapeAnomalies(client *emby.Client) (*ScanResult, error) {
	// 清空刮削异常表并重置主键
	if err := s.DB.Exec("DELETE FROM scrape_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空刮削异常表失败: %w", err)
	}
	log.Printf("🗑️ 已清空刮削异常表")
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='scrape_anomalies'").Error; err != nil {
		// sqlite_sequence 可能不存在（表从未插入过数据），忽略此错误
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	result := &ScanResult{}

	// 分页获取 Movie 和 Series 媒体条目并检测异常（Episode 的外部 ID 在 Series 级别）
	err := client.GetMediaItems("Movie,Series", func(items []emby.MediaItem) error {
		var anomalies []model.ScrapeAnomaly

		for _, item := range items {
			result.TotalScanned++

			_, hasPrimary := item.ImageTags["Primary"]
			missingPoster := !hasPrimary

			// 检查是否缺少外部 ID（TMDB 或 IMDB）
			_, hasTmdb := item.ProviderIds["Tmdb"]
			_, hasImdb := item.ProviderIds["Imdb"]
			missingProvider := !hasTmdb && !hasImdb

			if missingPoster || missingProvider {
				anomalies = append(anomalies, model.ScrapeAnomaly{
					EmbyItemID:      item.ID,
					Name:            item.Name,
					Type:            item.Type,
					MissingPoster:   missingPoster,
					MissingProvider: missingProvider,
					Path:            item.Path,
				})
			}
		}

		// 分批写入异常记录
		if len(anomalies) > 0 {
			if err := batchCreateInDB(s.DB, anomalies, 500); err != nil {
				log.Printf("⚠️ 分批写入刮削异常失败: %v", err)
				result.ErrorCount++
				return nil
			}
			result.AnomalyCount += len(anomalies)
		}

		log.Printf("📊 刮削异常扫描: 已处理 %d 个条目...", result.TotalScanned)
		return nil
	})

	if err != nil {
		log.Printf("扫描刮削异常过程中出错: %v", err)
		result.ErrorCount++
	}

	return result, err
}

// ScanDuplicateMedia 扫描重复媒体
// 按名称和 TMDB/IMDB ID 分组，找出重复条目
func (s *ScanService) ScanDuplicateMedia(client *emby.Client) (*ScanResult, error) {
	// 清空重复媒体表并重置主键
	if err := s.DB.Exec("DELETE FROM duplicate_media").Error; err != nil {
		return nil, fmt.Errorf("清空重复媒体表失败: %w", err)
	}
	log.Printf("🗑️ 已清空重复媒体表")
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='duplicate_media'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	result := &ScanResult{}

	// 收集所有媒体条目（需要全量数据才能分组判断重复）
	var allItems []emby.MediaItem
	err := client.GetMediaItems("", func(items []emby.MediaItem) error {
		for _, item := range items {
			result.TotalScanned++
			allItems = append(allItems, item)
		}
		log.Printf("📊 重复媒体扫描: 已处理 %d 个条目...", result.TotalScanned)
		return nil
	})

	if err != nil {
		log.Printf("扫描重复媒体过程中出错: %v", err)
		result.ErrorCount++
		return result, err
	}

	// 检测重复并分批写入数据库
	duplicates := DetectDuplicateMedia(allItems)
	if len(duplicates) > 0 {
		if err := batchCreateInDB(s.DB, duplicates, 500); err != nil {
			log.Printf("⚠️ 分批写入重复媒体失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(duplicates)
	}

	return result, nil
}

// seasonFromPathRe 从路径中提取 Season 编号，如 "/Season 400/" → 400
var seasonFromPathRe = regexp.MustCompile(`(?i)[/\\]Season\s+(\d+)[/\\]`)

// episodeFromFilenameRe 从文件名中提取集号，如 "S400E05" → 5
// 支持常见命名格式：S01E02、s01e02
var episodeFromFilenameRe = regexp.MustCompile(`(?i)S\d+E(\d+)`)

// resolveSeasonNumber 获取 Episode 的有效季号
// 优先从路径中提取 Season 编号（最可靠），因为 Emby 对超长剧集（如百家讲坛）
// 可能返回错误的 ParentIndexNumber（例如 Season 400+ 的 Episode 全部返回 ParentIndexNumber=20）。
// 仅当路径中无法提取时，才 fallback 到 ParentIndexNumber。
func resolveSeasonNumber(item emby.MediaItem) int {
	if matches := seasonFromPathRe.FindStringSubmatch(item.Path); len(matches) == 2 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return item.ParentIndexNumber
}

// resolveEpisodeNumber 获取 Episode 的有效集号
// 优先从文件名中提取集号（如 S400E05 → 5），因为 Emby 对超长剧集
// 可能返回错误的 IndexNumber（例如百家讲坛 Season 400+ 的所有 Episode 都返回 IndexNumber=1）。
// 仅当文件名中无法提取时，才 fallback 到 IndexNumber。
func resolveEpisodeNumber(item emby.MediaItem) int {
	if matches := episodeFromFilenameRe.FindStringSubmatch(item.Path); len(matches) == 2 {
		if n, err := strconv.Atoi(matches[1]); err == nil {
			return n
		}
	}
	return item.IndexNumber
}

// DetectDuplicateMedia 纯逻辑函数：检测媒体条目中的重复媒体
// 电影：同一个 TMDB ID 的 Movie 有多个条目 → 重复（多版本）
// 剧集：同一部剧（同 SeriesID）的同一集（同季号+集号）有多个 Episode 条目 → 重复
// 不依赖数据库，便于属性测试
func DetectDuplicateMedia(items []emby.MediaItem) []model.DuplicateMedia {
	// 电影按 TMDB ID 分组
	movieGroups := make(map[string][]emby.MediaItem)
	// 剧集按 SeriesID + S季E集 分组
	episodeGroups := make(map[string][]emby.MediaItem)
	// 记录 Series 名称映射（SeriesID -> SeriesName）
	seriesNames := make(map[string]string)

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
			// 用 SeriesID + 季号 + 集号 作为分组键
			if item.SeriesID == "" {
				continue
			}
			if item.SeriesName != "" {
				seriesNames[item.SeriesID] = item.SeriesName
			}
			seasonNum := resolveSeasonNumber(item)
			episodeNum := resolveEpisodeNumber(item)
			key := fmt.Sprintf("series:%s:S%dE%d", item.SeriesID, seasonNum, episodeNum)
			episodeGroups[key] = append(episodeGroups[key], item)

		case "Series":
			// Series 本身不参与重复检测，但记录名称
			if item.ID != "" && item.Name != "" {
				seriesNames[item.ID] = item.Name
			}
		}
	}

	var duplicates []model.DuplicateMedia

	// 处理电影重复
	for key, groupItems := range movieGroups {
		if len(groupItems) < 2 {
			continue
		}
		groupName := groupItems[0].Name
		for _, item := range groupItems {
			duplicates = append(duplicates, model.DuplicateMedia{
				GroupKey:    key,
				GroupName:   groupName,
				EmbyItemID:  item.ID,
				Name:        item.Name,
				Type:        item.Type,
				Path:        item.Path,
				FileSize:    item.FileSize,
			})
		}
	}

	// 处理剧集重复
	for key, groupItems := range episodeGroups {
		if len(groupItems) < 2 {
			continue
		}
		// 分组名用 Series 名称 + 季集号
		first := groupItems[0]
		sName := seriesNames[first.SeriesID]
		if sName == "" {
			sName = first.SeriesName
		}
		seasonNum := resolveSeasonNumber(first)
		episodeNum := resolveEpisodeNumber(first)
		groupName := fmt.Sprintf("%s S%dE%d", sName, seasonNum, episodeNum)
		for _, item := range groupItems {
			duplicates = append(duplicates, model.DuplicateMedia{
				GroupKey:    key,
				GroupName:   groupName,
				EmbyItemID:  item.ID,
				Name:        item.Name,
				Type:        item.Type,
				Path:        item.Path,
				FileSize:    item.FileSize,
			})
		}
	}

	return duplicates
}

// DetectScrapeAnomalies 纯逻辑函数：检测媒体条目中的刮削异常
// 检测缺少封面图和缺少外部 ID（TMDB/IMDB）的条目
// 不依赖数据库，便于属性测试
func DetectScrapeAnomalies(items []emby.MediaItem) []model.ScrapeAnomaly {
	var anomalies []model.ScrapeAnomaly

	for _, item := range items {
		// 只检查 Movie 和 Series
		if item.Type != "Movie" && item.Type != "Series" {
			continue
		}

		_, hasPrimary := item.ImageTags["Primary"]
		missingPoster := !hasPrimary

		// 检查是否缺少外部 ID（TMDB 或 IMDB）
		_, hasTmdb := item.ProviderIds["Tmdb"]
		_, hasImdb := item.ProviderIds["Imdb"]
		missingProvider := !hasTmdb && !hasImdb

		if missingPoster || missingProvider {
			anomalies = append(anomalies, model.ScrapeAnomaly{
				EmbyItemID:      item.ID,
				Name:            item.Name,
				Type:            item.Type,
				MissingPoster:   missingPoster,
				MissingProvider: missingProvider,
				Path:            item.Path,
			})
		}
	}

	return anomalies
}

// LocalSeasonInfo 本地季信息（用于纯逻辑函数）
type LocalSeasonInfo struct {
	SeasonNumber int
	EpisodeCount int
}

// SeriesInfo 电视节目信息（用于纯逻辑函数）
type SeriesInfo struct {
	EmbyItemID   string
	Name         string
	TmdbID       int
	LocalSeasons []LocalSeasonInfo
	TmdbSeasons  []tmdb.Season
}

// DetectEpisodeMappingAnomalies 纯逻辑函数：检测异常映射
// 对比本地季集数据与 TMDB 季集数据，找出不一致的季
// 不依赖数据库和外部 API，便于属性测试
func DetectEpisodeMappingAnomalies(seriesList []SeriesInfo) []model.EpisodeMappingAnomaly {
	var anomalies []model.EpisodeMappingAnomaly

	for _, series := range seriesList {
		// 计算本地季数（排除特别篇 season_number=0）
		localSeasonCount := 0
		for _, local := range series.LocalSeasons {
			if local.SeasonNumber > 0 {
				localSeasonCount++
			}
		}

		// 计算 TMDB 有效季数（排除特别篇 season_number=0，且 EpisodeCount > 0）
		// 注意：只统计有集数的季，不统计空季
		tmdbSeasonCount := 0
		tmdbSeasonMap := make(map[int]int) // seasonNumber -> episodeCount
		for _, s := range series.TmdbSeasons {
			if s.SeasonNumber > 0 && s.EpisodeCount > 0 {
				tmdbSeasonCount++
				tmdbSeasonMap[s.SeasonNumber] = s.EpisodeCount
			}
		}
		
		// 对比每个本地季
		for _, local := range series.LocalSeasons {
			if local.SeasonNumber <= 0 {
				continue // 跳过特别篇
			}

			tmdbEpisodes, exists := tmdbSeasonMap[local.SeasonNumber]
			if !exists {
				// TMDB 中不存在该季（或该季没有集数），标记为异常
				anomalies = append(anomalies, model.EpisodeMappingAnomaly{
					EmbyItemID:       series.EmbyItemID,
					Name:             series.Name,
					TmdbID:           series.TmdbID,
					SeasonNumber:     local.SeasonNumber,
					LocalEpisodes:    local.EpisodeCount,
					TmdbEpisodes:     0,
					Difference:       local.EpisodeCount,
					LocalSeasonCount: localSeasonCount,
					TmdbSeasonCount:  tmdbSeasonCount,
				})
				continue
			}

			if local.EpisodeCount != tmdbEpisodes {
				diff := local.EpisodeCount - tmdbEpisodes
				if diff < 0 {
					diff = -diff
				}
				anomalies = append(anomalies, model.EpisodeMappingAnomaly{
					EmbyItemID:       series.EmbyItemID,
					Name:             series.Name,
					TmdbID:           series.TmdbID,
					SeasonNumber:     local.SeasonNumber,
					LocalEpisodes:    local.EpisodeCount,
					TmdbEpisodes:     tmdbEpisodes,
					Difference:       diff,
					LocalSeasonCount: localSeasonCount,
					TmdbSeasonCount:  tmdbSeasonCount,
				})
			}
		}
	}

	return anomalies
}

// batchCreateInDB 分批写入数据库，每批 batchSize 条记录
// 避免 SQLite "too many SQL variables" 错误
func batchCreateInDB[T any](db *gorm.DB, records []T, batchSize int) error {
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]
		if err := db.Create(&batch).Error; err != nil {
			return fmt.Errorf("批次 %d-%d 写入失败: %w", i, end, err)
		}
	}
	return nil
}

// AnalyzeScrapeAnomaliesFromCache 基于缓存数据分析刮削异常
// 从 media_cache 读取数据，转换为 MediaItem，调用 DetectScrapeAnomalies
// 只检查 Movie 和 Series，Episode 的外部 ID 在 Series 级别，不单独检查
func (s *ScanService) AnalyzeScrapeAnomaliesFromCache() (*ScanResult, error) {
	startedAt := time.Now()

	// 清空刮削异常表并重置主键
	if err := s.DB.Exec("DELETE FROM scrape_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空刮削异常表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='scrape_anomalies'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	// 从缓存读取 Movie 和 Series 条目（Episode 的外部 ID 在 Series 级别，不单独检查）
	var caches []model.MediaCache
	if err := s.DB.Where("type IN ?", []string{"Movie", "Series"}).Find(&caches).Error; err != nil {
		return nil, fmt.Errorf("读取媒体缓存失败: %w", err)
	}

	// 转换为 MediaItem
	items := make([]emby.MediaItem, len(caches))
	for i, c := range caches {
		items[i] = c.ToMediaItem()
	}

	// 调用纯逻辑函数检测异常
	anomalies := DetectScrapeAnomalies(items)

	result := &ScanResult{
		TotalScanned: len(items),
	}

	// 分批写入数据库（每批 500 条，避免 SQLite 变量数限制）
	if len(anomalies) > 0 {
		if err := batchCreateInDB(s.DB, anomalies, 500); err != nil {
			log.Printf("⚠️ 分批写入刮削异常失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(anomalies)
	}

	// 记录执行日志
	s.saveScanLog("scrape_anomaly", startedAt, result)

	return result, nil
}

// AnalyzeDuplicateMediaFromCache 基于缓存数据分析重复媒体
// 从 media_cache 读取数据，转换为 MediaItem，调用 DetectDuplicateMedia
func (s *ScanService) AnalyzeDuplicateMediaFromCache() (*ScanResult, error) {
	startedAt := time.Now()

	// 清空重复媒体表并重置主键
	if err := s.DB.Exec("DELETE FROM duplicate_media").Error; err != nil {
		return nil, fmt.Errorf("清空重复媒体表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='duplicate_media'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	// 从缓存读取所有媒体条目
	var caches []model.MediaCache
	if err := s.DB.Find(&caches).Error; err != nil {
		return nil, fmt.Errorf("读取媒体缓存失败: %w", err)
	}

	// 转换为 MediaItem
	items := make([]emby.MediaItem, len(caches))
	for i, c := range caches {
		items[i] = c.ToMediaItem()
	}

	// 调用纯逻辑函数检测重复
	duplicates := DetectDuplicateMedia(items)

	result := &ScanResult{
		TotalScanned: len(items),
	}

	// 分批写入数据库（每批 500 条）
	if len(duplicates) > 0 {
		if err := batchCreateInDB(s.DB, duplicates, 500); err != nil {
			log.Printf("⚠️ 分批写入重复媒体失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(duplicates)
	}

	// 记录执行日志
	s.saveScanLog("duplicate_media", startedAt, result)

	return result, nil
}

// AnalyzeEpisodeMappingFromCache 基于缓存数据+TMDB分析异常映射
// 从 media_cache + season_cache 读取数据，构建 SeriesInfo，调用 DetectEpisodeMappingAnomalies
func (s *ScanService) AnalyzeEpisodeMappingFromCache(tmdbClient *tmdb.Client) (*ScanResult, error) {
	// 清空异常映射表并重置主键
	if err := s.DB.Exec("DELETE FROM episode_mapping_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空异常映射表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='episode_mapping_anomalies'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	// 从缓存读取所有 Series 类型条目
	var seriesCaches []model.MediaCache
	if err := s.DB.Where("type = ?", "Series").Find(&seriesCaches).Error; err != nil {
		return nil, fmt.Errorf("读取 Series 缓存失败: %w", err)
	}

	result := &ScanResult{}
	var allAnomalies []model.EpisodeMappingAnomaly

	for _, sc := range seriesCaches {
		result.TotalScanned++

		// 获取 TMDB ID
		item := sc.ToMediaItem()
		tmdbIDStr, ok := item.ProviderIds["Tmdb"]
		if !ok || tmdbIDStr == "" {
			log.Printf("电视节目 %q (ID=%s) 没有 TMDB ID，跳过", sc.Name, sc.EmbyItemID)
			continue
		}
		tmdbID, err := strconv.Atoi(tmdbIDStr)
		if err != nil {
			log.Printf("电视节目 %q (ID=%s) TMDB ID 格式错误: %s", sc.Name, sc.EmbyItemID, tmdbIDStr)
			result.ErrorCount++
			continue
		}

		// 从 season_cache 读取该 Series 的季信息
		var seasonCaches []model.SeasonCache
		if err := s.DB.Where("series_emby_item_id = ?", sc.EmbyItemID).Find(&seasonCaches).Error; err != nil {
			log.Printf("读取 Series %q 的季缓存失败: %v", sc.Name, err)
			result.ErrorCount++
			continue
		}

		var localSeasons []LocalSeasonInfo
		for _, season := range seasonCaches {
			localSeasons = append(localSeasons, LocalSeasonInfo{
				SeasonNumber: season.SeasonNumber,
				EpisodeCount: season.EpisodeCount,
			})
		}

		// 获取 TMDB 数据（仍需请求 TMDB API）
		tmdbDetails, err := tmdbClient.GetTVShowDetails(tmdbID)
		if err != nil {
			log.Printf("获取电视节目 %q 的 TMDB 数据失败: %v", sc.Name, err)
			result.ErrorCount++
			continue
		}

		// 使用纯逻辑函数检测异常
		seriesInfo := SeriesInfo{
			EmbyItemID:   sc.EmbyItemID,
			Name:         sc.Name,
			TmdbID:       tmdbID,
			LocalSeasons: localSeasons,
			TmdbSeasons:  tmdbDetails.Seasons,
		}
		anomalies := DetectEpisodeMappingAnomalies([]SeriesInfo{seriesInfo})
		allAnomalies = append(allAnomalies, anomalies...)
	}

	// 分批写入异常记录（每批 500 条）
	if len(allAnomalies) > 0 {
		if err := batchCreateInDB(s.DB, allAnomalies, 500); err != nil {
			log.Printf("⚠️ 分批写入异常映射失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(allAnomalies)
	}

	return result, nil
}

// ScanEpisodeMapping 扫描异常映射
// 获取电视节目的本地季集数据，与 TMDB 数据对比
func (s *ScanService) ScanEpisodeMapping(embyClient *emby.Client, tmdbClient *tmdb.Client) (*ScanResult, error) {
	// 清空异常映射表并重置主键
	if err := s.DB.Exec("DELETE FROM episode_mapping_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空异常映射表失败: %w", err)
	}
	log.Printf("🗑️ 已清空异常映射表")
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='episode_mapping_anomalies'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	result := &ScanResult{}

	// 获取所有电视节目
	var allSeries []emby.MediaItem
	err := embyClient.GetMediaItems("Series", func(items []emby.MediaItem) error {
		allSeries = append(allSeries, items...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("获取电视节目列表失败: %w", err)
	}

	var allAnomalies []model.EpisodeMappingAnomaly

	for _, series := range allSeries {
		result.TotalScanned++
		log.Printf("📊 异常映射扫描: 已处理 %d 个条目...", result.TotalScanned)

		// 获取 TMDB ID
		tmdbIDStr, ok := series.ProviderIds["Tmdb"]
		if !ok || tmdbIDStr == "" {
			log.Printf("电视节目 %q (ID=%s) 没有 TMDB ID，跳过", series.Name, series.ID)
			continue
		}
		tmdbID, err := strconv.Atoi(tmdbIDStr)
		if err != nil {
			log.Printf("电视节目 %q (ID=%s) TMDB ID 格式错误: %s", series.Name, series.ID, tmdbIDStr)
			result.ErrorCount++
			continue
		}

		// 获取本地季信息
		seasons, err := embyClient.GetChildItems(series.ID, "Season")
		if err != nil {
			log.Printf("获取电视节目 %q 的季信息失败: %v", series.Name, err)
			result.ErrorCount++
			continue
		}

		var localSeasons []LocalSeasonInfo
		for _, season := range seasons {
			localSeasons = append(localSeasons, LocalSeasonInfo{
				SeasonNumber: season.IndexNumber,
				EpisodeCount: season.EffectiveChildCount(),
			})
		}

		// 获取 TMDB 数据
		tmdbDetails, err := tmdbClient.GetTVShowDetails(tmdbID)
		if err != nil {
			log.Printf("获取电视节目 %q 的 TMDB 数据失败: %v", series.Name, err)
			result.ErrorCount++
			continue
		}

		// 使用纯逻辑函数检测异常
		seriesInfo := SeriesInfo{
			EmbyItemID:   series.ID,
			Name:         series.Name,
			TmdbID:       tmdbID,
			LocalSeasons: localSeasons,
			TmdbSeasons:  tmdbDetails.Seasons,
		}
		anomalies := DetectEpisodeMappingAnomalies([]SeriesInfo{seriesInfo})
		allAnomalies = append(allAnomalies, anomalies...)
	}

	// 分批写入异常记录（每批 500 条）
	if len(allAnomalies) > 0 {
		if err := batchCreateInDB(s.DB, allAnomalies, 500); err != nil {
			log.Printf("⚠️ 分批写入异常映射失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(allAnomalies)
	}

	return result, nil
}

// tmdbResult 用于 Worker Pool 的 TMDB 查询结果
type tmdbResult struct {
	SeriesInfo SeriesInfo
	Err        error
}

// ScanEpisodeMappingWithContext 并发扫描异常映射
// 使用 Worker Pool 并发获取 TMDB 数据
func (s *ScanService) ScanEpisodeMappingWithContext(ctx context.Context, embyClient *emby.Client, tmdbClient *tmdb.Client) (*ScanResult, error) {
	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 清空异常映射表并重置主键
	if err := s.DB.Exec("DELETE FROM episode_mapping_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空异常映射表失败: %w", err)
	}
	log.Printf("🗑️ 已清空异常映射表")
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='episode_mapping_anomalies'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	result := &ScanResult{}

	// 获取所有电视节目（使用带 context 的方法）
	var allSeries []emby.MediaItem
	err := embyClient.GetMediaItemsWithContext(ctx, "Series", func(items []emby.MediaItem) error {
		allSeries = append(allSeries, items...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("获取电视节目列表失败: %w", err)
	}

	result.TotalScanned = len(allSeries)

	// 连续认证失败计数器（用于快速中止无效 API Key 的情况）
	var consecutiveAuthErrors atomic.Int32
	// 用 WithCancel 包装 context，以便在连续 401 时主动取消
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// 使用 Worker Pool 并发获取 TMDB 数据
	pool := workerpool.New[tmdbResult](cancelCtx, workerpool.Config{
		MinWorkers:  2,
		MaxWorkers:  5,
		IdleTimeout: 5 * time.Second,
	})

	for _, series := range allSeries {
		s := series
		pool.Submit(func() workerpool.Result[tmdbResult] {
			// 获取 TMDB ID
			tmdbIDStr, ok := s.ProviderIds["Tmdb"]
			if !ok || tmdbIDStr == "" {
				log.Printf("电视节目 %q (ID=%s) 没有 TMDB ID，跳过", s.Name, s.ID)
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: fmt.Errorf("无 TMDB ID")}}
			}
			tmdbID, err := strconv.Atoi(tmdbIDStr)
			if err != nil {
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: fmt.Errorf("TMDB ID 格式错误: %s", tmdbIDStr)}}
			}

			// 获取本地季信息（使用带 context 的方法）
			seasons, err := embyClient.GetChildItemsWithContext(cancelCtx, s.ID, "Season")
			if err != nil {
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: err}}
			}

			var localSeasons []LocalSeasonInfo
			for _, season := range seasons {
				localSeasons = append(localSeasons, LocalSeasonInfo{
					SeasonNumber: season.IndexNumber,
					EpisodeCount: season.EffectiveChildCount(),
				})
			}

			// 获取 TMDB 数据（使用带 context 的方法）
			tmdbDetails, err := tmdbClient.GetTVShowDetailsWithContext(cancelCtx, tmdbID)
			if err != nil {
				// 检测是否为认证错误（401）
				if tmdb.IsAuthError(err) {
					count := consecutiveAuthErrors.Add(1)
					log.Printf("🔑 TMDB 认证失败 (401): %q (TMDB ID=%d), 连续失败 %d 次", s.Name, tmdbID, count)
					if int(count) >= maxConsecutiveAuthErrors {
						log.Printf("🚫 连续 %d 次 TMDB 认证失败，API Key 可能无效，中止扫描", count)
						cancelFunc()
					}
				} else {
					// 非 401 错误，重置连续计数
					consecutiveAuthErrors.Store(0)
				}
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: err}}
			}

			// 请求成功，重置连续 401 计数
			consecutiveAuthErrors.Store(0)

			return workerpool.Result[tmdbResult]{
				Value: tmdbResult{
					SeriesInfo: SeriesInfo{
						EmbyItemID:   s.ID,
						Name:         s.Name,
						TmdbID:       tmdbID,
						LocalSeasons: localSeasons,
						TmdbSeasons:  tmdbDetails.Seasons,
					},
				},
			}
		})
	}

	poolResults := pool.Wait()

	// 收集所有成功的 SeriesInfo 并检测异常
	var seriesList []SeriesInfo
	for _, r := range poolResults {
		if r.Err != nil {
			result.ErrorCount++
			continue
		}
		if r.Value.Err != nil {
			result.ErrorCount++
			continue
		}
		seriesList = append(seriesList, r.Value.SeriesInfo)
	}

	allAnomalies := DetectEpisodeMappingAnomalies(seriesList)

	// 分批写入异常记录（每批 500 条）
	if len(allAnomalies) > 0 {
		if err := batchCreateInDB(s.DB, allAnomalies, 500); err != nil {
			log.Printf("⚠️ 分批写入异常映射失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(allAnomalies)
	}

	return result, nil
}

// AnalyzeEpisodeMappingFromCacheWithContext 并发分析异常映射（基于缓存）
// 使用 Worker Pool 并发获取 TMDB 数据
func (s *ScanService) AnalyzeEpisodeMappingFromCacheWithContext(ctx context.Context, tmdbClient *tmdb.Client) (*ScanResult, error) {
	startedAt := time.Now()

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 清空异常映射表并重置主键
	if err := s.DB.Exec("DELETE FROM episode_mapping_anomalies").Error; err != nil {
		return nil, fmt.Errorf("清空异常映射表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM sqlite_sequence WHERE name='episode_mapping_anomalies'").Error; err != nil {
		log.Printf("重置主键序列（可忽略）: %v", err)
	}

	// 从缓存读取所有 Series 类型条目
	var seriesCaches []model.MediaCache
	if err := s.DB.Where("type = ?", "Series").Find(&seriesCaches).Error; err != nil {
		return nil, fmt.Errorf("读取 Series 缓存失败: %w", err)
	}

	result := &ScanResult{
		TotalScanned: len(seriesCaches),
	}

	log.Printf("📊 异常映射分析: 共 %d 个 Series，开始请求 TMDB...", len(seriesCaches))

	// 连续认证失败计数器（用于快速中止无效 API Key 的情况）
	var consecutiveAuthErrors atomic.Int32
	// 用 WithCancel 包装 context，以便在连续 401 时主动取消
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	// 进度计数器
	var progressMu sync.Mutex
	progressCount := 0

	// 使用 Worker Pool 并发获取 TMDB 数据
	pool := workerpool.New[tmdbResult](cancelCtx, workerpool.Config{
		MinWorkers:  2,
		MaxWorkers:  5,
		IdleTimeout: 5 * time.Second,
	})

	for _, sc := range seriesCaches {
		cache := sc
		pool.Submit(func() workerpool.Result[tmdbResult] {
			// 获取 TMDB ID
			item := cache.ToMediaItem()
			tmdbIDStr, ok := item.ProviderIds["Tmdb"]
			if !ok || tmdbIDStr == "" {
				progressMu.Lock()
				progressCount++
				current := progressCount
				progressMu.Unlock()
				log.Printf("⏭️ [%d/%d] 跳过（无 TMDB ID）: %q", current, len(seriesCaches), cache.Name)
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: fmt.Errorf("无 TMDB ID")}}
			}
			tmdbID, err := strconv.Atoi(tmdbIDStr)
			if err != nil {
				progressMu.Lock()
				progressCount++
				current := progressCount
				progressMu.Unlock()
				log.Printf("⏭️ [%d/%d] 跳过（TMDB ID 格式错误）: %q, ID=%s", current, len(seriesCaches), cache.Name, tmdbIDStr)
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: fmt.Errorf("TMDB ID 格式错误")}}
			}

			// 从 season_cache 读取该 Series 的季信息
			var seasonCaches []model.SeasonCache
			if err := s.DB.Where("series_emby_item_id = ?", cache.EmbyItemID).Find(&seasonCaches).Error; err != nil {
				log.Printf("❌ 读取季缓存失败: %q: %v", cache.Name, err)
				progressMu.Lock()
				progressCount++
				progressMu.Unlock()
				return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: err}}
			}

			var localSeasons []LocalSeasonInfo
			for _, season := range seasonCaches {
				localSeasons = append(localSeasons, LocalSeasonInfo{
					SeasonNumber: season.SeasonNumber,
					EpisodeCount: season.EpisodeCount,
				})
			}

			// 先查询 TMDB 缓存
			var tmdbCaches []model.TmdbCache
			s.DB.Where("tmdb_id = ?", tmdbID).Find(&tmdbCaches)

			var tmdbSeasons []tmdb.Season
			if len(tmdbCaches) > 0 {
				// 使用缓存数据
				for _, tc := range tmdbCaches {
					tmdbSeasons = append(tmdbSeasons, tmdb.Season{
						SeasonNumber: tc.SeasonNumber,
						EpisodeCount: tc.EpisodeCount,
						Name:         tc.SeasonName,
					})
				}
				progressMu.Lock()
				progressCount++
				current := progressCount
				progressMu.Unlock()
				log.Printf("📦 [%d/%d] 使用 TMDB 缓存: %q (TMDB ID=%d, %d 季)",
					current, len(seriesCaches), cache.Name, tmdbID, len(tmdbSeasons))
			} else {
				// 缓存未命中，请求 TMDB API
				tmdbDetails, err := tmdbClient.GetTVShowDetailsWithContext(cancelCtx, tmdbID)
				if err != nil {
					progressMu.Lock()
					progressCount++
					current := progressCount
					progressMu.Unlock()

					// 检测是否为认证错误（401）
					if tmdb.IsAuthError(err) {
						count := consecutiveAuthErrors.Add(1)
						log.Printf("🔑 [%d/%d] TMDB 认证失败 (401): %q (TMDB ID=%d), 连续失败 %d 次",
							current, len(seriesCaches), cache.Name, tmdbID, count)
						if int(count) >= maxConsecutiveAuthErrors {
							log.Printf("🚫 连续 %d 次 TMDB 认证失败，API Key 可能无效，中止分析", count)
							cancelFunc()
						}
					} else {
						// 非 401 错误，重置连续计数
						consecutiveAuthErrors.Store(0)
						log.Printf("❌ [%d/%d] TMDB 请求失败: %q (TMDB ID=%d): %v",
							current, len(seriesCaches), cache.Name, tmdbID, err)
					}

					return workerpool.Result[tmdbResult]{Value: tmdbResult{Err: err}}
				}

				// 请求成功，重置连续 401 计数
				consecutiveAuthErrors.Store(0)
				tmdbSeasons = tmdbDetails.Seasons

				// 写入 TMDB 缓存
				now := time.Now()
				for _, season := range tmdbDetails.Seasons {
					tc := model.TmdbCache{
						TmdbID:       tmdbID,
						Name:         tmdbDetails.Name,
						SeasonNumber: season.SeasonNumber,
						EpisodeCount: season.EpisodeCount,
						SeasonName:   season.Name,
						CachedAt:     now,
						UpdatedAt:    now,
					}
					s.DB.Where("tmdb_id = ? AND season_number = ?", tmdbID, season.SeasonNumber).
						Assign(tc).FirstOrCreate(&tc)
				}

				progressMu.Lock()
				progressCount++
				current := progressCount
				progressMu.Unlock()
				log.Printf("✅ [%d/%d] TMDB 请求成功并已缓存: %q (TMDB ID=%d, %d 季)",
					current, len(seriesCaches), cache.Name, tmdbID, len(tmdbSeasons))
			}

			return workerpool.Result[tmdbResult]{
				Value: tmdbResult{
					SeriesInfo: SeriesInfo{
						EmbyItemID:   cache.EmbyItemID,
						Name:         cache.Name,
						TmdbID:       tmdbID,
						LocalSeasons: localSeasons,
						TmdbSeasons:  tmdbSeasons,
					},
				},
			}
		})
	}

	poolResults := pool.Wait()

	// 收集所有成功的 SeriesInfo 并检测异常
	var seriesList []SeriesInfo
	for _, r := range poolResults {
		if r.Err != nil {
			result.ErrorCount++
			continue
		}
		if r.Value.Err != nil {
			result.ErrorCount++
			continue
		}
		seriesList = append(seriesList, r.Value.SeriesInfo)
	}

	allAnomalies := DetectEpisodeMappingAnomalies(seriesList)

	// 分批写入异常记录（每批 500 条）
	if len(allAnomalies) > 0 {
		if err := batchCreateInDB(s.DB, allAnomalies, 500); err != nil {
			log.Printf("⚠️ 分批写入异常映射失败: %v", err)
			result.ErrorCount++
			return result, err
		}
		result.AnomalyCount = len(allAnomalies)
	}

	// 记录执行日志
	s.saveScanLog("episode_mapping", startedAt, result)

	return result, nil
}

// saveScanLog 保存扫描/分析执行记录
func (s *ScanService) saveScanLog(module string, startedAt time.Time, result *ScanResult) {
	scanLog := model.ScanLog{
		Module:       module,
		StartedAt:    startedAt,
		FinishedAt:   time.Now(),
		TotalScanned: result.TotalScanned,
		AnomalyCount: result.AnomalyCount,
		ErrorCount:   result.ErrorCount,
	}
	if err := s.DB.Create(&scanLog).Error; err != nil {
		log.Printf("⚠️ 保存扫描日志失败: %v", err)
	}
}
