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

// CleanupDuplicateMedia POST /api/cleanup/duplicate-media - 批量清理重复媒体
// 接收前端传来的待删除 emby_item_id 列表，逐个调用 Emby DeleteVersion 接口
func (h *ScanHandler) CleanupDuplicateMedia(c *gin.Context) {
	var req struct {
		Items []string `json:"items"` // 要删除的 emby_item_id 列表
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请选择要删除的条目",
		})
		return
	}

	log.Printf("🧹 开始批量清理重复媒体，共 %d 个条目...", len(req.Items))

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	deletedCount := 0
	failedCount := 0
	var freedSize int64
	var failedItems []string

	// 查询这些条目的详细信息（用于日志和统计释放空间）
	var toDelete []model.DuplicateMedia
	h.DB.Where("emby_item_id IN ?", req.Items).Find(&toDelete)

	// 构建 emby_item_id -> DuplicateMedia 映射
	itemMap := make(map[string]model.DuplicateMedia)
	for _, d := range toDelete {
		itemMap[d.EmbyItemID] = d
	}

	for _, embyID := range req.Items {
		item, exists := itemMap[embyID]

		// 调用 Emby DeleteVersion 接口
		if err := client.DeleteVersion(ctx, embyID); err != nil {
			log.Printf("❌ 删除版本失败 [%s]: %v", embyID, err)
			failedCount++
			failedItems = append(failedItems, embyID)
			continue
		}

		if exists {
			log.Printf("🗑️  已删除 [%s] %s (%.1f MB)", embyID, item.Path, float64(item.FileSize)/1024/1024)
			freedSize += item.FileSize
		} else {
			log.Printf("🗑️  已删除 [%s]", embyID)
		}
		deletedCount++
	}

	// 从数据库中删除已清理的记录
	if deletedCount > 0 {
		// 排除失败的，只删除成功的
		successIDs := make([]string, 0, deletedCount)
		for _, id := range req.Items {
			isFailed := false
			for _, fid := range failedItems {
				if id == fid {
					isFailed = true
					break
				}
			}
			if !isFailed {
				successIDs = append(successIDs, id)
			}
		}
		if len(successIDs) > 0 {
			h.DB.Where("emby_item_id IN ?", successIDs).Delete(&model.DuplicateMedia{})
		}

		// 清理只剩一条记录的分组（不再是重复）
		h.DB.Exec(`DELETE FROM duplicate_media WHERE group_key IN (
			SELECT group_key FROM duplicate_media GROUP BY group_key HAVING COUNT(*) < 2
		)`)
	}

	log.Printf("✅ 重复媒体清理完成: 删除 %d 个, 释放 %.1f MB, 失败 %d 个",
		deletedCount, float64(freedSize)/1024/1024, failedCount)

	c.JSON(http.StatusOK, gin.H{
		"message": "清理完成",
		"data": gin.H{
			"deleted_count": deletedCount,
			"freed_size":    freedSize,
			"failed_count":  failedCount,
			"failed_items":  failedItems,
		},
	})
}

// PreviewDuplicateCleanup GET /api/cleanup/duplicate-media/preview - 预览待清理的重复媒体
// 返回所有重复组，每组包含全部条目，并标记建议删除的（体积较小的）
func (h *ScanHandler) PreviewDuplicateCleanup(c *gin.Context) {
	// 获取所有重复媒体记录，按分组和文件大小升序排序
	var duplicates []model.DuplicateMedia
	h.DB.Order("group_key ASC, file_size ASC").Find(&duplicates)

	// 按 group_key 分组
	groups := make(map[string][]model.DuplicateMedia)
	var groupOrder []string
	for _, d := range duplicates {
		if _, exists := groups[d.GroupKey]; !exists {
			groupOrder = append(groupOrder, d.GroupKey)
		}
		groups[d.GroupKey] = append(groups[d.GroupKey], d)
	}

	type previewItem struct {
		EmbyItemID    string `json:"emby_item_id"`
		Name          string `json:"name"`
		Type          string `json:"type"`
		Path          string `json:"path"`
		FileSize      int64  `json:"file_size"`
		ShouldDelete  bool   `json:"should_delete"` // 建议删除（体积较小的）
	}

	type previewGroup struct {
		GroupKey  string        `json:"group_key"`
		GroupName string        `json:"group_name"`
		Items     []previewItem `json:"items"`
	}

	var result []previewGroup
	totalDeleteCount := 0
	var totalFreedSize int64

	for _, key := range groupOrder {
		groupItems := groups[key]
		if len(groupItems) < 2 {
			continue
		}

		pg := previewGroup{
			GroupKey:  key,
			GroupName: groupItems[0].GroupName,
		}

		// 按文件大小升序排列，保留最后一个（体积最大的），其余建议删除
		// 大小相同时默认删除排在前面的
		lastIdx := len(groupItems) - 1
		for i, item := range groupItems {
			shouldDelete := i < lastIdx // 最后一个保留，其余删除
			pg.Items = append(pg.Items, previewItem{
				EmbyItemID:   item.EmbyItemID,
				Name:         item.Name,
				Type:         item.Type,
				Path:         item.Path,
				FileSize:     item.FileSize,
				ShouldDelete: shouldDelete,
			})
			if shouldDelete {
				totalDeleteCount++
				totalFreedSize += item.FileSize
			}
		}

		result = append(result, pg)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":               result,
		"total_groups":       len(result),
		"total_delete_count": totalDeleteCount,
		"total_freed_size":   totalFreedSize,
	})
}

// FormatAnalysisSummary 格式化分析结果摘要日志字符串
func FormatAnalysisSummary(analysisType string, result *service.ScanResult) string {
	return fmt.Sprintf("✅ %s分析完成: 共分析 %d 个条目, 发现 %d 个异常, %d 个错误",
		analysisType, result.TotalScanned, result.AnomalyCount, result.ErrorCount)
}

// CleanupScrapeAnomalies POST /api/cleanup/scrape-anomaly - 批量删除刮削异常条目
// 接收前端传来的待删除 emby_item_id 列表，逐个调用 Emby DeleteItem 接口
func (h *ScanHandler) CleanupScrapeAnomalies(c *gin.Context) {
	var req struct {
		Items []string `json:"items"` // 要删除的 emby_item_id 列表
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请选择要删除的条目",
		})
		return
	}

	log.Printf("🧹 开始批量删除刮削异常条目，共 %d 个...", len(req.Items))

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	deletedCount := 0
	failedCount := 0
	var failedItems []string

	// 查询这些条目的详细信息（用于日志）
	var toDelete []model.ScrapeAnomaly
	h.DB.Where("emby_item_id IN ?", req.Items).Find(&toDelete)

	// 构建 emby_item_id -> ScrapeAnomaly 映射
	itemMap := make(map[string]model.ScrapeAnomaly)
	for _, d := range toDelete {
		itemMap[d.EmbyItemID] = d
	}

	for _, embyID := range req.Items {
		item, exists := itemMap[embyID]

		// 调用 Emby DeleteItem 接口
		if err := client.DeleteItem(ctx, embyID); err != nil {
			log.Printf("❌ 删除条目失败 [%s]: %v", embyID, err)
			failedCount++
			failedItems = append(failedItems, embyID)
			continue
		}

		if exists {
			log.Printf("🗑️  已删除 [%s] %s", embyID, item.Name)
		} else {
			log.Printf("🗑️  已删除 [%s]", embyID)
		}
		deletedCount++
	}

	// 从数据库中删除已清理的记录
	if deletedCount > 0 {
		successIDs := make([]string, 0, deletedCount)
		for _, id := range req.Items {
			isFailed := false
			for _, fid := range failedItems {
				if id == fid {
					isFailed = true
					break
				}
			}
			if !isFailed {
				successIDs = append(successIDs, id)
			}
		}
		if len(successIDs) > 0 {
			h.DB.Where("emby_item_id IN ?", successIDs).Delete(&model.ScrapeAnomaly{})
		}
	}

	// 同时清理 media_cache 中对应的缓存记录
	if deletedCount > 0 {
		successIDs := make([]string, 0, deletedCount)
		for _, id := range req.Items {
			isFailed := false
			for _, fid := range failedItems {
				if id == fid {
					isFailed = true
					break
				}
			}
			if !isFailed {
				successIDs = append(successIDs, id)
			}
		}
		if len(successIDs) > 0 {
			h.DB.Where("emby_item_id IN ?", successIDs).Delete(&model.MediaCache{})
		}
	}

	log.Printf("✅ 刮削异常清理完成: 删除 %d 个, 失败 %d 个", deletedCount, failedCount)

	c.JSON(http.StatusOK, gin.H{
		"message": "清理完成",
		"data": gin.H{
			"deleted_count": deletedCount,
			"failed_count":  failedCount,
			"failed_items":  failedItems,
		},
	})
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
