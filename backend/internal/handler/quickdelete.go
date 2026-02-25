package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// QuickDeleteHandler 快速删除处理器
type QuickDeleteHandler struct {
	DB *gorm.DB
}

// seasonDeleteReq 季删除请求（带季号）
type seasonDeleteReq struct {
	ID           string `json:"id"`
	SeasonNumber int    `json:"season_number"`
}

// NewQuickDeleteHandler 创建快速删除处理器
func NewQuickDeleteHandler(db *gorm.DB) *QuickDeleteHandler {
	return &QuickDeleteHandler{DB: db}
}

// getEmbyClient 从数据库获取 Emby 配置并创建客户端
func (h *QuickDeleteHandler) getEmbyClient() (*emby.Client, error) {
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		return nil, err
	}
	return emby.NewClient(config.Host, config.Port, config.APIKey), nil
}

// SearchEmbyMedia GET /api/media-query/search - 搜索 Emby 媒体
func (h *QuickDeleteHandler) SearchEmbyMedia(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入搜索关键字"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 Emby 连接"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	var items []emby.MediaItem

	// 判断是否为 TMDB ID（纯数字）
	if isTmdbID(keyword) {
		// 按 TMDB ID 搜索
		items, err = h.searchByTmdbID(ctx, client, keyword, limit)
	} else {
		// 按关键字搜索
		items, err = client.SearchItems(ctx, keyword, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "搜索失败: " + err.Error()})
		return
	}

	// 获取 serverId 用于前端构建跳转链接
	serverID := ""
	if info, err := client.TestConnection(); err == nil {
		serverID = info.ID
	}

	type SearchResult struct {
		ID                 string `json:"Id"`
		Name               string `json:"Name"`
		Type               string `json:"Type"`
		ProductionYear     int    `json:"ProductionYear"`
		ChildCount         int    `json:"ChildCount"`
		RecursiveItemCount int    `json:"RecursiveItemCount"`
		HasImage           bool   `json:"HasImage"`
		ServerID           string `json:"ServerId"`
		TmdbID             string `json:"TmdbId"`
		ImdbID             string `json:"ImdbId"`
		Path               string `json:"Path"`
	}

	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		_, hasImage := item.ImageTags["Primary"]

		results = append(results, SearchResult{
			ID:                 item.ID,
			Name:               item.Name,
			Type:               item.Type,
			ProductionYear:     item.ProductionYear,
			ChildCount:         item.ChildCount,
			RecursiveItemCount: item.RecursiveItemCount,
			HasImage:           hasImage,
			ServerID:           serverID,
			TmdbID:             item.ProviderIds["Tmdb"],
			ImdbID:             item.ProviderIds["Imdb"],
			Path:               item.Path,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": results})
}

// isTmdbID 判断输入是否为 TMDB ID（纯数字且大于 0）
func isTmdbID(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	// 排除 "0" 或全零的情况，TMDB ID 至少为 1
	n, err := strconv.Atoi(s)
	return err == nil && n > 0
}

// searchByTmdbID 通过 TMDB ID 搜索 Emby 媒体（电影和剧集都搜）
func (h *QuickDeleteHandler) searchByTmdbID(ctx context.Context, client *emby.Client, tmdbID string, limit int) ([]emby.MediaItem, error) {
	baseURL := fmt.Sprintf("%s:%d", client.Host, client.Port)
	url := fmt.Sprintf("%s/emby/Items?AnyProviderIdEquals=Tmdb.%s&IncludeItemTypes=Movie,Series&Recursive=true&Limit=%d&Fields=Path,ProviderIds,ChildCount,RecursiveItemCount,ProductionYear",
		baseURL, tmdbID, limit)

	body, err := embyGet(url, client.APIKey)
	if err != nil {
		return nil, fmt.Errorf("TMDB ID 搜索失败: %w", err)
	}

	var resp emby.MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	return resp.Items, nil
}

// GetSeriesSeasons GET /api/media-query/seasons/:seriesId - 获取剧集的季列表
func (h *QuickDeleteHandler) GetSeriesSeasons(c *gin.Context) {
	seriesID := c.Param("seriesId")
	if seriesID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 seriesId"})
		return
	}

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 Emby 连接"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// 获取 Season 类型的子条目
	seasons, err := client.GetChildItemsWithContext(ctx, seriesID, "Season")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取季列表失败: " + err.Error()})
		return
	}

	// 构建季信息列表，获取每季的集数
	type SeasonInfo struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		SeasonNumber  int    `json:"season_number"`
		EpisodeCount  int    `json:"episode_count"`
	}

	result := make([]SeasonInfo, 0, len(seasons))
	for _, s := range seasons {
		episodeCount := s.EffectiveChildCount()
		// 如果 ChildCount 为 0，尝试通过 API 获取集数
		if episodeCount == 0 {
			count, err := client.GetChildItemCount(ctx, s.ID, "Episode")
			if err == nil {
				episodeCount = count
			}
		}
		result = append(result, SeasonInfo{
			ID:           s.ID,
			Name:         s.Name,
			SeasonNumber: s.IndexNumber,
			EpisodeCount: episodeCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// DeleteMedia POST /api/media-query/delete - 删除媒体
func (h *QuickDeleteHandler) DeleteMedia(c *gin.Context) {
	var req struct {
		EmbyItemID string            `json:"emby_item_id"`
		Type       string            `json:"type"`       // movie, series, season
		SeasonIDs  []string          `json:"season_ids"` // type=season 时使用（兼容旧版）
		Seasons    []seasonDeleteReq `json:"seasons"`    // type=season 时使用（新版，带季号）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	if req.EmbyItemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 emby_item_id"})
		return
	}
	if req.Type != "movie" && req.Type != "series" && req.Type != "season" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的 type，支持: movie, series, season"})
		return
	}

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 Emby 连接"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	switch req.Type {
	case "movie":
		h.deleteMovie(c, ctx, client, req.EmbyItemID)
	case "series":
		h.deleteSeries(c, ctx, client, req.EmbyItemID)
	case "season":
		h.deleteSeasons(c, ctx, client, req.EmbyItemID, req.Seasons, req.SeasonIDs)
	}
}

// deleteMovie 删除电影
func (h *QuickDeleteHandler) deleteMovie(c *gin.Context, ctx context.Context, client *emby.Client, itemID string) {
	// 先从本地缓存获取名称
	var mc model.MediaCache
	movieName := itemID
	if err := h.DB.Where("emby_item_id = ?", itemID).First(&mc).Error; err == nil {
		movieName = mc.Name
	}

	// 调用 Emby API 删除
	if err := client.DeleteItem(ctx, itemID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除失败: " + err.Error()})
		return
	}

	// 清理本地缓存和扫描结果
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.MediaCache{})
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.ScrapeAnomaly{})
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.DuplicateMedia{})
	log.Printf("🗑️ 快速删除电影: %s (ID: %s)", movieName, itemID)

	c.JSON(http.StatusOK, gin.H{"message": "ok", "deleted_count": 1, "failed": []string{}})
}

// deleteSeries 删除整个剧集
func (h *QuickDeleteHandler) deleteSeries(c *gin.Context, ctx context.Context, client *emby.Client, itemID string) {
	// 先从本地缓存获取名称
	var mc model.MediaCache
	seriesName := itemID
	if err := h.DB.Where("emby_item_id = ?", itemID).First(&mc).Error; err == nil {
		seriesName = mc.Name
	}

	// 统计关联的 Episode 和 Season 数量
	var episodeCount, seasonCount int64
	h.DB.Model(&model.MediaCache{}).Where("series_id = ?", itemID).Count(&episodeCount)
	h.DB.Model(&model.SeasonCache{}).Where("series_emby_item_id = ?", itemID).Count(&seasonCount)

	// 调用 Emby API 删除整个 Series
	if err := client.DeleteItem(ctx, itemID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除失败: " + err.Error()})
		return
	}

	// 清理本地缓存：Series 本身 + 关联的 Episode + SeasonCache
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.MediaCache{})
	h.DB.Where("series_id = ?", itemID).Delete(&model.MediaCache{})
	h.DB.Where("series_emby_item_id = ?", itemID).Delete(&model.SeasonCache{})
	// 清理扫描结果表
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.ScrapeAnomaly{})
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.DuplicateMedia{})
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.EpisodeMappingAnomaly{})
	log.Printf("🗑️ 快速删除剧集: %s (ID: %s, 清理: %d 集 %d 季)", seriesName, itemID, episodeCount, seasonCount)

	c.JSON(http.StatusOK, gin.H{"message": "ok", "deleted_count": 1, "failed": []string{}})
}

// deleteSeasons 删除指定的季
func (h *QuickDeleteHandler) deleteSeasons(c *gin.Context, ctx context.Context, client *emby.Client, seriesID string, seasons []seasonDeleteReq, legacySeasonIDs []string) {
	// 兼容旧版：如果 seasons 为空但 season_ids 有值，转换为新格式
	if len(seasons) == 0 && len(legacySeasonIDs) > 0 {
		for _, id := range legacySeasonIDs {
			seasons = append(seasons, seasonDeleteReq{ID: id})
		}
	}

	if len(seasons) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请选择要删除的季"})
		return
	}

	// 获取剧集名称
	var seriesMC model.MediaCache
	seriesName := seriesID
	if err := h.DB.Where("emby_item_id = ?", seriesID).First(&seriesMC).Error; err == nil {
		seriesName = seriesMC.Name
	}

	deletedCount := 0
	failed := make([]string, 0)

	for _, season := range seasons {
		seasonID := season.ID
		seasonNumber := season.SeasonNumber

		// 如果前端没传季号，尝试从 SeasonCache 获取
		if seasonNumber == 0 {
			var sc model.SeasonCache
			// 先用真实 Emby Season ID 查
			if err := h.DB.Where("season_emby_item_id = ?", seasonID).First(&sc).Error; err == nil {
				seasonNumber = sc.SeasonNumber
			}
		}

		// 统计该季下的 Episode 数量
		var episodeCount int64
		if seasonNumber > 0 {
			h.DB.Model(&model.MediaCache{}).Where("series_id = ? AND parent_index_number = ?", seriesID, seasonNumber).Count(&episodeCount)
		}

		// 调用 Emby API 删除该季
		if err := client.DeleteItem(ctx, seasonID); err != nil {
			log.Printf("❌ 删除季失败: %s S%02d (ID: %s): %v", seriesName, seasonNumber, seasonID, err)
			failed = append(failed, seasonID)
			continue
		}

		// 清理本地缓存
		if seasonNumber > 0 {
			// 删除该季下的所有 Episode
			h.DB.Where("series_id = ? AND parent_index_number = ?", seriesID, seasonNumber).Delete(&model.MediaCache{})
		}
		// 删除 SeasonCache（兼容真实 ID 和合成 ID 两种格式）
		h.DB.Where("season_emby_item_id = ?", seasonID).Delete(&model.SeasonCache{})
		if seasonNumber > 0 {
			syntheticID := fmt.Sprintf("%s_S%d", seriesID, seasonNumber)
			h.DB.Where("season_emby_item_id = ?", syntheticID).Delete(&model.SeasonCache{})
			// 清理该季的异常映射记录
			h.DB.Where("emby_item_id = ? AND season_number = ?", seriesID, seasonNumber).Delete(&model.EpisodeMappingAnomaly{})
		}

		deletedCount++
		log.Printf("🗑️ 快速删除季: %s S%02d (ID: %s, 清理: %d 集)", seriesName, seasonNumber, seasonID, episodeCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "ok",
		"deleted_count": deletedCount,
		"failed":        failed,
	})
}
