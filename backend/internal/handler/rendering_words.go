package handler

import (
	"net/http"
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

	// 构建查询：只筛选"多季异常"类型
	query := h.DB.Model(&model.EpisodeMappingAnomaly{}).
		Where("local_season_count > 1 AND tmdb_season_count = 1")

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
			Where("local_season_count > 1 AND tmdb_season_count = 1").
			Order("emby_item_id ASC, season_number ASC").
			Find(&anomalies)

		// 获取 season_cache 数据
		var seasonCaches []model.SeasonCache
		h.DB.Where("series_emby_item_id IN ?", embyItemIDs).
			Order("series_emby_item_id ASC, season_number ASC").
			Find(&seasonCaches)

		// 构建映射
		anomalyMap := make(map[string][]model.EpisodeMappingAnomaly)
		seasonMap := make(map[string][]model.SeasonCache)
		seriesInfoMap := make(map[string]model.EpisodeMappingAnomaly)

		for _, a := range anomalies {
			if _, exists := seriesInfoMap[a.EmbyItemID]; !exists {
				seriesInfoMap[a.EmbyItemID] = a
			}
			anomalyMap[a.EmbyItemID] = append(anomalyMap[a.EmbyItemID], a)
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
			seasons := seasonMap[embyItemID]
			
			var seasonInfos []SeasonInfo
			var recommendedRules []MappingRule

			// 计算累计偏移
			cumulativeOffset := 0
			
			// 获取 TMDB Season 1 的集数
			tmdbSeason1Episodes := 0
			if tmdbSeasons, exists := tmdbMap[info.TmdbID]; exists {
				if episodes, ok := tmdbSeasons[1]; ok {
					tmdbSeason1Episodes = episodes
				}
			}

			for _, season := range seasons {
				if season.SeasonNumber <= 0 {
					continue // 跳过特别篇
				}

				// 获取 TMDB 集数
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

				// 生成推荐规则
				// 如果本地 Season > 1 且 TMDB 只有 Season 1
				if season.SeasonNumber > 1 && info.TmdbSeasonCount == 1 {
					// 第一个非 Season 1 的季，偏移量 = TMDB Season 1 的集数
					if season.SeasonNumber == 2 {
						cumulativeOffset = tmdbSeason1Episodes
					}
					
					recommendedRules = append(recommendedRules, MappingRule{
						SourceSeason:   season.SeasonNumber,
						SourceEpisodes: "1-" + strconv.Itoa(season.EpisodeCount),
						TargetSeason:   1,
						Offset:         "EP+" + strconv.Itoa(cumulativeOffset),
					})
					
					// 累加偏移量
					cumulativeOffset += season.EpisodeCount
				}
			}

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
