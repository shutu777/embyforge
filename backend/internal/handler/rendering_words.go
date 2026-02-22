package handler

import (
	"net/http"
	"sort"
	"strconv"

	"embyforge/internal/model"
	"embyforge/internal/tmdb"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RenderingWordsHandler 渲染词处理器
type RenderingWordsHandler struct {
	DB *gorm.DB
}

// NewRenderingWordsHandler 创建渲染词处理器
func NewRenderingWordsHandler(db *gorm.DB) *RenderingWordsHandler {
	return &RenderingWordsHandler{DB: db}
}

// MappingRule 映射规则
type MappingRule struct {
	SourceSeason   int    `json:"source_season"`
	SourceEpisodes string `json:"source_episodes"`
	TargetSeason   int    `json:"target_season"`
	Offset         string `json:"offset"`
}

// ImportCandidate 导入候选
type ImportCandidate struct {
	EmbyItemID       string        `json:"emby_item_id"`
	Name             string        `json:"name"`
	TmdbID           int           `json:"tmdb_id"`
	Seasons          []SeasonInfo  `json:"seasons"`
	RecommendedRules []MappingRule `json:"recommended_rules"`
}

// SeasonInfo 季信息
type SeasonInfo struct {
	SeasonNumber  int `json:"season_number"`
	LocalEpisodes int `json:"local_episodes"`
	TmdbEpisodes  int `json:"tmdb_episodes"`
}

// GetImportCandidates 获取可导入的候选剧集
// GET /api/rendering-words/import-candidates
// 支持三种场景：
// 1. 本地多季 → TMDB 1季：本地 S2/S3... 映射到 TMDB S1，偏移量为正数
// 2. 本地1季 → TMDB多季：本地 S1 的后半部分映射到 TMDB S2/S3...，偏移量为负数
// 3. 其他季数不匹配：本地和 TMDB 都有多季但数量不同
func (h *RenderingWordsHandler) GetImportCandidates(c *gin.Context) {
	search := c.DefaultQuery("search", "")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询：筛选季数不匹配的异常（本地季数 != TMDB季数）
	query := h.DB.Model(&model.EpisodeMappingAnomaly{}).
		Where("local_season_count != tmdb_season_count")

	if search != "" {
		query = query.Where("name LIKE ?", "%"+search+"%")
	}

	// 获取不同剧集的总数
	var totalCount int64
	h.DB.Table("(?) as sub", query.Select("DISTINCT emby_item_id")).Count(&totalCount)

	// 分页获取剧集 ID 列表
	type groupRow struct {
		EmbyItemID string `gorm:"column:emby_item_id"`
	}
	var groupRows []groupRow
	offset := (page - 1) * pageSize

	query.Select("emby_item_id").
		Group("emby_item_id").
		Order("MIN(name) ASC").
		Offset(offset).Limit(pageSize).
		Find(&groupRows)

	embyItemIDs := make([]string, len(groupRows))
	for i, r := range groupRows {
		embyItemIDs[i] = r.EmbyItemID
	}

	var candidates []ImportCandidate

	if len(embyItemIDs) > 0 {
		// 获取异常记录
		var anomalies []model.EpisodeMappingAnomaly
		h.DB.Where("emby_item_id IN ?", embyItemIDs).
			Where("local_season_count != tmdb_season_count").
			Order("emby_item_id ASC, season_number ASC").
			Find(&anomalies)

		// 获取 season_cache 数据（本地季信息）
		var seasonCaches []model.SeasonCache
		h.DB.Where("series_emby_item_id IN ?", embyItemIDs).
			Order("series_emby_item_id ASC, season_number ASC").
			Find(&seasonCaches)

		// 构建映射
		seriesInfoMap := make(map[string]model.EpisodeMappingAnomaly)
		seasonMap := make(map[string][]model.SeasonCache)

		for _, a := range anomalies {
			if _, exists := seriesInfoMap[a.EmbyItemID]; !exists {
				seriesInfoMap[a.EmbyItemID] = a
			}
		}

		for _, sc := range seasonCaches {
			seasonMap[sc.SeriesEmbyItemID] = append(seasonMap[sc.SeriesEmbyItemID], sc)
		}

		// 获取 TMDB 缓存数据
		tmdbIDs := make([]int, 0)
		for _, a := range seriesInfoMap {
			tmdbIDs = append(tmdbIDs, a.TmdbID)
		}
		var tmdbCaches []model.TmdbCache
		if len(tmdbIDs) > 0 {
			h.DB.Where("tmdb_id IN ?", tmdbIDs).Find(&tmdbCaches)
		}

		// 构建 tmdb_id -> season_number -> episode_count 的映射
		tmdbMap := make(map[int]map[int]int)
		for _, tc := range tmdbCaches {
			if _, exists := tmdbMap[tc.TmdbID]; !exists {
				tmdbMap[tc.TmdbID] = make(map[int]int)
			}
			tmdbMap[tc.TmdbID][tc.SeasonNumber] = tc.EpisodeCount
		}

		// 构建候选列表
		for embyItemID, info := range seriesInfoMap {
			localSeasons := seasonMap[embyItemID]

			var seasonInfos []SeasonInfo
			var recommendedRules []MappingRule

			// 收集本地季信息（跳过特别篇）
			for _, season := range localSeasons {
				if season.SeasonNumber <= 0 {
					continue
				}
				tmdbEpisodes := 0
				if tmdbSeasons, exists := tmdbMap[info.TmdbID]; exists {
					if episodes, ok := tmdbSeasons[season.SeasonNumber]; ok {
						tmdbEpisodes = episodes
					}
				}
				seasonInfos = append(seasonInfos, SeasonInfo{
					SeasonNumber:  season.SeasonNumber,
					LocalEpisodes: season.EpisodeCount,
					TmdbEpisodes:  tmdbEpisodes,
				})
			}

			// 根据不同场景生成推荐规则
			if info.LocalSeasonCount > 1 && info.TmdbSeasonCount == 1 {
				// 场景1：本地多季 → TMDB 1季
				// 例：本地 S1(13集)+S2(13集)，TMDB S1(26集)
				// 规则：s=2, e=1-13 => s=1, e=EP+13
				cumulativeOffset := 0
				// 获取 TMDB S1 集数作为初始偏移
				if tmdbSeasons, exists := tmdbMap[info.TmdbID]; exists {
					if episodes, ok := tmdbSeasons[1]; ok {
						cumulativeOffset = episodes
					}
				}

				for _, season := range localSeasons {
					if season.SeasonNumber <= 1 || season.SeasonNumber <= 0 {
						continue
					}
					// 从 S2 开始，偏移量 = TMDB S1 集数
					if season.SeasonNumber == 2 {
						// cumulativeOffset 已经是 TMDB S1 集数
					} else {
						// S3 及以后，需要累加前面本地季的集数
					}

					recommendedRules = append(recommendedRules, MappingRule{
						SourceSeason:   season.SeasonNumber,
						SourceEpisodes: "1-" + strconv.Itoa(season.EpisodeCount),
						TargetSeason:   1,
						Offset:         "EP+" + strconv.Itoa(cumulativeOffset),
					})
					cumulativeOffset += season.EpisodeCount
				}

			} else if info.LocalSeasonCount == 1 && info.TmdbSeasonCount > 1 {
				// 场景2：本地1季 → TMDB多季
				// 例：本地 S1(50集)，TMDB S1(30集)+S2(20集)
				// 规则：s=1, e=31-50 => s=2, e=EP-30
				tmdbSeasons := tmdbMap[info.TmdbID]
				if tmdbSeasons == nil {
					tmdbSeasons = make(map[int]int)
				}

				// 获取本地 S1 的总集数
				localS1Episodes := 0
				for _, season := range localSeasons {
					if season.SeasonNumber == 1 {
						localS1Episodes = season.EpisodeCount
						break
					}
				}

				// 按 TMDB 季号排序
				tmdbSeasonNums := make([]int, 0, len(tmdbSeasons))
				for sn := range tmdbSeasons {
					if sn > 0 {
						tmdbSeasonNums = append(tmdbSeasonNums, sn)
					}
				}
				sort.Ints(tmdbSeasonNums)

				// 计算累计偏移：从 TMDB S2 开始
				cumulativeEpisodes := 0
				for _, tmdbSN := range tmdbSeasonNums {
					tmdbEpCount := tmdbSeasons[tmdbSN]
					if tmdbSN == 1 {
						cumulativeEpisodes = tmdbEpCount
						continue
					}

					// 计算本地 S1 中对应这个 TMDB 季的集数范围
					epStart := cumulativeEpisodes + 1
					epEnd := cumulativeEpisodes + tmdbEpCount
					// 不超过本地总集数
					if epEnd > localS1Episodes {
						epEnd = localS1Episodes
					}
					if epStart > localS1Episodes {
						break // 本地集数已经不够了
					}

					recommendedRules = append(recommendedRules, MappingRule{
						SourceSeason:   1,
						SourceEpisodes: strconv.Itoa(epStart) + "-" + strconv.Itoa(epEnd),
						TargetSeason:   tmdbSN,
						Offset:         "EP-" + strconv.Itoa(cumulativeEpisodes),
					})
					cumulativeEpisodes += tmdbEpCount
				}
			}
			// 场景3：本地多季 vs TMDB多季但数量不同，暂不自动生成规则，用户手动配置

			candidates = append(candidates, ImportCandidate{
				EmbyItemID:       embyItemID,
				Name:             info.Name,
				TmdbID:           info.TmdbID,
				Seasons:          seasonInfos,
				RecommendedRules: recommendedRules,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      candidates,
		"total":     totalCount,
		"page":      page,
		"page_size": pageSize,
	})
}

// ValidateTmdbID 验证 TMDB ID 并返回剧集信息
// GET /api/rendering-words/validate-tmdb/:tmdbId
func (h *RenderingWordsHandler) ValidateTmdbID(c *gin.Context) {
	tmdbIDStr := c.Param("tmdbId")
	tmdbID, err := strconv.Atoi(tmdbIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "TMDB ID 格式错误",
		})
		return
	}

	// 获取 TMDB API Key
	var config model.SystemConfig
	if err := h.DB.Where("key = ?", "tmdb_api_key").First(&config).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 TMDB API Key",
		})
		return
	}

	tmdbClient := tmdb.NewClient(config.Value)

	// 获取 TMDB 数据
	details, err := tmdbClient.GetTVShowDetails(tmdbID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"valid":   false,
			"message": "TMDB ID 不存在或请求失败",
		})
		return
	}

	// 构建季信息
	type seasonInfo struct {
		SeasonNumber int `json:"season_number"`
		EpisodeCount int `json:"episode_count"`
	}
	var seasons []seasonInfo
	for _, s := range details.Seasons {
		if s.SeasonNumber > 0 && s.EpisodeCount > 0 {
			seasons = append(seasons, seasonInfo{
				SeasonNumber: s.SeasonNumber,
				EpisodeCount: s.EpisodeCount,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"name":    details.Name,
		"seasons": seasons,
	})
}
