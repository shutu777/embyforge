package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"
	"embyforge/internal/service"
	"embyforge/internal/tmdb"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// defaultScanTimeout 扫描操作默认超时时间
const defaultScanTimeout = 30 * time.Minute

// ScanHandler 扫描处理器
type ScanHandler struct {
	DB          *gorm.DB
	ScanService *service.ScanService
}

// NewScanHandler 创建扫描处理器
func NewScanHandler(db *gorm.DB) *ScanHandler {
	return &ScanHandler{
		DB:          db,
		ScanService: service.NewScanService(db),
	}
}

// getTMDBAPIKey 从数据库读取 TMDB API Key
func (h *ScanHandler) getTMDBAPIKey() (string, error) {
	var config model.SystemConfig
	if err := h.DB.Where("key = ?", "tmdb_api_key").First(&config).Error; err != nil {
		return "", err
	}
	return config.Value, nil
}

// getEmbyClient 从数据库获取 Emby 配置并创建客户端
func (h *ScanHandler) getEmbyClient() (*emby.Client, error) {
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		return nil, err
	}
	return emby.NewClient(config.Host, config.Port, config.APIKey), nil
}

// StartScrapeAnomalyScan 启动刮削异常扫描
func (h *ScanHandler) StartScrapeAnomalyScan(c *gin.Context) {
	log.Printf("🔍 开始刮削异常扫描...")
	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	result, err := h.ScanService.ScanScrapeAnomalies(client)
	if err != nil {
		log.Printf("⚠️ 刮削异常扫描出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "扫描过程中出错",
			"error":   err.Error(),
			"data":    result,
		})
		return
	}

	log.Printf("%s", service.FormatScanSummary("刮削异常", result))
	c.JSON(http.StatusOK, gin.H{
		"message": "扫描完成",
		"data":    result,
	})
}

// StartDuplicateMediaScan 启动重复媒体扫描
func (h *ScanHandler) StartDuplicateMediaScan(c *gin.Context) {
	log.Printf("🔍 开始重复媒体扫描...")
	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	result, err := h.ScanService.ScanDuplicateMedia(client)
	if err != nil {
		log.Printf("⚠️ 重复媒体扫描出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "扫描过程中出错",
			"error":   err.Error(),
			"data":    result,
		})
		return
	}

	log.Printf("%s", service.FormatScanSummary("重复媒体", result))
	c.JSON(http.StatusOK, gin.H{
		"message": "扫描完成",
		"data":    result,
	})
}

// GetDuplicateMedia 分页获取重复媒体结果（按 GroupKey 分组）
func (h *ScanHandler) GetDuplicateMedia(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 获取不同分组的总数
	var totalGroups int64
	h.DB.Model(&model.DuplicateMedia{}).Distinct("group_key").Count(&totalGroups)

	// 分页获取分组键和分组名
	type groupInfo struct {
		GroupKey  string `json:"group_key"`
		GroupName string `json:"group_name"`
		Count     int64  `json:"count"`
	}
	var groups []groupInfo
	offset := (page - 1) * pageSize
	h.DB.Model(&model.DuplicateMedia{}).
		Select("group_key, MAX(group_name) as group_name, COUNT(*) as count").
		Group("group_key").
		Order("count DESC, group_key ASC").
		Offset(offset).
		Limit(pageSize).
		Find(&groups)

	// 获取这些分组下的所有记录
	groupKeys := make([]string, len(groups))
	for i, g := range groups {
		groupKeys[i] = g.GroupKey
	}

	var duplicates []model.DuplicateMedia
	if len(groupKeys) > 0 {
		h.DB.Where("group_key IN ?", groupKeys).Order("group_key ASC, type ASC, name ASC").Find(&duplicates)
	}

	// 按 GroupKey 分组返回，包含分组信息
	type groupResult struct {
		GroupKey  string                 `json:"group_key"`
		GroupName string                 `json:"group_name"`
		Count     int64                  `json:"count"`
		Items     []model.DuplicateMedia `json:"items"`
	}

	// 构建分组映射
	itemsByKey := make(map[string][]model.DuplicateMedia)
	for _, d := range duplicates {
		itemsByKey[d.GroupKey] = append(itemsByKey[d.GroupKey], d)
	}

	var results []groupResult
	for _, g := range groups {
		results = append(results, groupResult{
			GroupKey:  g.GroupKey,
			GroupName: g.GroupName,
			Count:     g.Count,
			Items:     itemsByKey[g.GroupKey],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":         results,
		"total_groups": totalGroups,
		"page":         page,
		"page_size":    pageSize,
	})
}

// GetScrapeAnomalies 分页获取刮削异常结果
func (h *ScanHandler) GetScrapeAnomalies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	h.DB.Model(&model.ScrapeAnomaly{}).Count(&total)

	var anomalies []model.ScrapeAnomaly
	offset := (page - 1) * pageSize
	h.DB.Offset(offset).Limit(pageSize).Order("id ASC").Find(&anomalies)

	c.JSON(http.StatusOK, gin.H{
		"data":      anomalies,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// StartEpisodeMappingScan 启动异常映射扫描
func (h *ScanHandler) StartEpisodeMappingScan(c *gin.Context) {
	log.Printf("🔍 开始异常映射扫描...")
	embyClient, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	tmdbAPIKey, err := h.getTMDBAPIKey()
	if err != nil || tmdbAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请在系统配置页面配置 TMDB API Key",
		})
		return
	}

	tmdbClient := tmdb.NewClient(tmdbAPIKey)

	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultScanTimeout)
	defer cancel()

	result, err := h.ScanService.ScanEpisodeMappingWithContext(ctx, embyClient, tmdbClient)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⚠️ 异常映射扫描超时")
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"code":    504,
				"message": "扫描操作超时",
				"error":   err.Error(),
			})
			return
		}
		log.Printf("⚠️ 异常映射扫描出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "扫描过程中出错",
			"error":   err.Error(),
			"data":    result,
		})
		return
	}

	log.Printf("%s", service.FormatScanSummary("异常映射", result))
	c.JSON(http.StatusOK, gin.H{
		"message": "扫描完成",
		"data":    result,
	})
}

// AnalyzeScrapeAnomalies POST /api/analyze/scrape-anomaly - 基于缓存分析刮削异常
func (h *ScanHandler) AnalyzeScrapeAnomalies(c *gin.Context) {
	log.Printf("🔍 开始基于缓存分析刮削异常...")

	// 检查缓存是否为空
	var count int64
	h.DB.Model(&model.MediaCache{}).Count(&count)
	if count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缓存为空，请先到扫描媒体页面同步媒体库",
		})
		return
	}

	result, err := h.ScanService.AnalyzeScrapeAnomaliesFromCache()
	if err != nil {
		log.Printf("⚠️ 刮削异常分析出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "分析过程中出错",
			"error":   err.Error(),
		})
		return
	}

	log.Printf("%s", FormatAnalysisSummary("刮削异常", result))
	c.JSON(http.StatusOK, gin.H{
		"message": "分析完成",
		"data":    result,
	})
}

// AnalyzeDuplicateMedia POST /api/analyze/duplicate-media - 基于缓存分析重复媒体
func (h *ScanHandler) AnalyzeDuplicateMedia(c *gin.Context) {
	log.Printf("🔍 开始基于缓存分析重复媒体...")

	// 检查缓存是否为空
	var count int64
	h.DB.Model(&model.MediaCache{}).Count(&count)
	if count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缓存为空，请先到扫描媒体页面同步媒体库",
		})
		return
	}

	result, err := h.ScanService.AnalyzeDuplicateMediaFromCache()
	if err != nil {
		log.Printf("⚠️ 重复媒体分析出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "分析过程中出错",
			"error":   err.Error(),
		})
		return
	}

	log.Printf("%s", FormatAnalysisSummary("重复媒体", result))
	c.JSON(http.StatusOK, gin.H{
		"message": "分析完成",
		"data":    result,
	})
}

// AnalyzeEpisodeMapping POST /api/analyze/episode-mapping - 基于缓存分析异常映射
func (h *ScanHandler) AnalyzeEpisodeMapping(c *gin.Context) {
	log.Printf("🔍 开始基于缓存分析异常映射...")

	// 检查缓存是否为空
	var count int64
	h.DB.Model(&model.MediaCache{}).Count(&count)
	if count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缓存为空，请先到扫描媒体页面同步媒体库",
		})
		return
	}

	// 获取 TMDB API Key
	tmdbAPIKey, err := h.getTMDBAPIKey()
	if err != nil || tmdbAPIKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请在系统配置页面配置 TMDB API Key",
		})
		return
	}

	tmdbClient := tmdb.NewClient(tmdbAPIKey)

	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultScanTimeout)
	defer cancel()

	result, err := h.ScanService.AnalyzeEpisodeMappingFromCacheWithContext(ctx, tmdbClient)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⚠️ 异常映射分析超时")
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"code":    504,
				"message": "分析操作超时",
				"error":   err.Error(),
			})
			return
		}
		log.Printf("⚠️ 异常映射分析出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "分析过程中出错",
			"error":   err.Error(),
		})
		return
	}

	// 查询去重后的异常节目数（与统计卡片保持一致）
	var distinctCount int64
	h.DB.Model(&model.EpisodeMappingAnomaly{}).Distinct("emby_item_id").Count(&distinctCount)

	log.Printf("✅ 异常映射分析完成: 共分析 %d 个条目, 发现 %d 个异常, %d 个错误",
		result.TotalScanned, distinctCount, result.ErrorCount)

	c.JSON(http.StatusOK, gin.H{
		"message": "分析完成",
		"data":    result,
		"anomaly_show_count": distinctCount,
	})
}

// FormatAnalysisSummary 格式化分析结果摘要日志字符串
func FormatAnalysisSummary(analysisType string, result *service.ScanResult) string {
	return fmt.Sprintf("✅ %s分析完成: 共分析 %d 个条目, 发现 %d 个异常, %d 个错误",
		analysisType, result.TotalScanned, result.AnomalyCount, result.ErrorCount)
}

// GetAnalysisStatus 获取各分析模块的最后分析时间和异常数量
func (h *ScanHandler) GetAnalysisStatus(c *gin.Context) {
	type moduleStatus struct {
		LastAnalyzedAt *time.Time `json:"last_analyzed_at"`
		AnomalyCount   int64     `json:"anomaly_count"`
	}

	status := make(map[string]moduleStatus)

	modules := []struct {
		key   string
		model interface{}
		countDistinct string // 如果需要 distinct 计数
	}{
		{"scrape_anomaly", &model.ScrapeAnomaly{}, ""},
		{"duplicate_media", &model.DuplicateMedia{}, "group_key"},
		{"episode_mapping", &model.EpisodeMappingAnomaly{}, "emby_item_id"},
	}

	for _, m := range modules {
		var count int64
		if m.countDistinct != "" {
			h.DB.Model(m.model).Distinct(m.countDistinct).Count(&count)
		} else {
			h.DB.Model(m.model).Count(&count)
		}

		// 从 scan_logs 获取最后执行时间
		var lastLog model.ScanLog
		var lastTime *time.Time
		if err := h.DB.Where("module = ?", m.key).Order("finished_at DESC").First(&lastLog).Error; err == nil {
			lastTime = &lastLog.FinishedAt
		}

		status[m.key] = moduleStatus{LastAnalyzedAt: lastTime, AnomalyCount: count}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": status,
	})
}

// EpisodeMappingGroup 按节目聚合的异常映射组
type EpisodeMappingGroup struct {
	EmbyItemID string                        `json:"emby_item_id"`
	Name       string                        `json:"name"`
	TmdbID     int                           `json:"tmdb_id"`
	Seasons    []model.EpisodeMappingAnomaly `json:"seasons"`
}

// GetEpisodeMappingAnomalies 分页获取异常映射结果（按节目聚合）
func (h *ScanHandler) GetEpisodeMappingAnomalies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 按 emby_item_id 分组计算总节目数
	var total int64
	h.DB.Model(&model.EpisodeMappingAnomaly{}).Distinct("emby_item_id").Count(&total)

	// 分页获取当前页的节目 ID 列表（按名称排序）
	var embyItemIDs []string
	offset := (page - 1) * pageSize
	h.DB.Model(&model.EpisodeMappingAnomaly{}).
		Select("emby_item_id").
		Group("emby_item_id").
		Order("MIN(name) ASC").
		Offset(offset).Limit(pageSize).
		Pluck("emby_item_id", &embyItemIDs)

	var groups []EpisodeMappingGroup
	if len(embyItemIDs) > 0 {
		// 获取这些节目的所有异常季数据，按季号排序
		var anomalies []model.EpisodeMappingAnomaly
		h.DB.Where("emby_item_id IN ?", embyItemIDs).
			Order("season_number ASC").
			Find(&anomalies)

		// 按 emby_item_id 聚合
		groupMap := make(map[string]*EpisodeMappingGroup)
		groupOrder := make([]string, 0) // 保持顺序
		for _, a := range anomalies {
			g, exists := groupMap[a.EmbyItemID]
			if !exists {
				g = &EpisodeMappingGroup{
					EmbyItemID: a.EmbyItemID,
					Name:       a.Name,
					TmdbID:     a.TmdbID,
					Seasons:    []model.EpisodeMappingAnomaly{},
				}
				groupMap[a.EmbyItemID] = g
				groupOrder = append(groupOrder, a.EmbyItemID)
			}
			g.Seasons = append(g.Seasons, a)
		}

		// 按原始顺序输出
		for _, id := range groupOrder {
			groups = append(groups, *groupMap[id])
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      groups,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
