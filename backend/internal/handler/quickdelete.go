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

// SearchEmbyMedia GET /api/quick-delete/search - 搜索 Emby 媒体
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

	// 构建带海报 URL 的结果
	baseURL := fmt.Sprintf("%s:%d", client.Host, client.Port)

	// 获取 serverId 用于 Emby Web 跳转链接
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
		ImageURL           string `json:"ImageUrl"`
		EmbyURL            string `json:"EmbyUrl"`
		TmdbID             string `json:"TmdbId"`
		ImdbID             string `json:"ImdbId"`
		Path               string `json:"Path"`
	}

	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		imgURL := ""
		if _, ok := item.ImageTags["Primary"]; ok {
			imgURL = fmt.Sprintf("%s/emby/Items/%s/Images/Primary?maxHeight=300&api_key=%s", baseURL, item.ID, client.APIKey)
		}
		// 构建 Emby Web 跳转链接
		embyURL := fmt.Sprintf("%s/web/index.html#!/item?id=%s&serverId=%s", baseURL, item.ID, serverID)

		results = append(results, SearchResult{
			ID:                 item.ID,
			Name:               item.Name,
			Type:               item.Type,
			ProductionYear:     item.ProductionYear,
			ChildCount:         item.ChildCount,
			RecursiveItemCount: item.RecursiveItemCount,
			ImageURL:           imgURL,
			EmbyURL:            embyURL,
			TmdbID:             item.ProviderIds["Tmdb"],
			ImdbID:             item.ProviderIds["Imdb"],
			Path:               item.Path,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": results})
}

// isTmdbID 判断输入是否为 TMDB ID（纯数字）
func isTmdbID(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
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

// GetSeriesSeasons GET /api/quick-delete/seasons/:seriesId - 获取剧集的季列表
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

// DeleteMedia POST /api/quick-delete/delete - 删除媒体
func (h *QuickDeleteHandler) DeleteMedia(c *gin.Context) {
	var req struct {
		EmbyItemID string   `json:"emby_item_id"`
		Type       string   `json:"type"`       // movie, series, season
		SeasonIDs  []string `json:"season_ids"` // type=season 时使用
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
		h.deleteSeasons(c, ctx, client, req.EmbyItemID, req.SeasonIDs)
	}
}

// deleteMovie 删除电影
func (h *QuickDeleteHandler) deleteMovie(c *gin.Context, ctx context.Context, client *emby.Client, itemID string) {
	// 调用 Emby API 删除
	if err := client.DeleteItem(ctx, itemID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除失败: " + err.Error()})
		return
	}

	// 清理本地缓存
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.MediaCache{})
	log.Printf("🗑️ 快速删除电影: %s", itemID)

	c.JSON(http.StatusOK, gin.H{"message": "ok", "deleted_count": 1, "failed": []string{}})
}

// deleteSeries 删除整个剧集
func (h *QuickDeleteHandler) deleteSeries(c *gin.Context, ctx context.Context, client *emby.Client, itemID string) {
	// 调用 Emby API 删除整个 Series
	if err := client.DeleteItem(ctx, itemID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "删除失败: " + err.Error()})
		return
	}

	// 清理本地缓存：Series 本身 + 关联的 Episode + SeasonCache
	h.DB.Where("emby_item_id = ?", itemID).Delete(&model.MediaCache{})
	h.DB.Where("series_id = ?", itemID).Delete(&model.MediaCache{})
	h.DB.Where("series_emby_item_id = ?", itemID).Delete(&model.SeasonCache{})
	log.Printf("🗑️ 快速删除剧集: %s（含关联 Episode 和 Season 缓存）", itemID)

	c.JSON(http.StatusOK, gin.H{"message": "ok", "deleted_count": 1, "failed": []string{}})
}

// deleteSeasons 删除指定的季
func (h *QuickDeleteHandler) deleteSeasons(c *gin.Context, ctx context.Context, client *emby.Client, seriesID string, seasonIDs []string) {
	if len(seasonIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请选择要删除的季"})
		return
	}

	deletedCount := 0
	failed := make([]string, 0)

	for _, seasonID := range seasonIDs {
		// 调用 Emby API 删除该季
		if err := client.DeleteItem(ctx, seasonID); err != nil {
			log.Printf("❌ 删除季失败 [%s]: %v", seasonID, err)
			failed = append(failed, seasonID)
			continue
		}

		// 清理本地缓存：SeasonCache + 该季下的 Episode
		h.DB.Where("season_emby_item_id = ?", seasonID).Delete(&model.SeasonCache{})
		// Episode 的 ParentIndexNumber 对应季号，但我们用 SeriesID + 季的 Emby ID 来关联
		// 由于 Episode 缓存中没有直接的 SeasonID 字段，通过 Emby API 获取该季下的 Episode 再删除
		// 简化处理：直接通过 series_id 和 parent_index_number 来匹配
		// 先获取该季的季号
		var seasonCache model.SeasonCache
		if err := h.DB.Where("season_emby_item_id = ?", seasonID).First(&seasonCache).Error; err == nil {
			h.DB.Where("series_id = ? AND parent_index_number = ?", seriesID, seasonCache.SeasonNumber).Delete(&model.MediaCache{})
		}

		deletedCount++
		log.Printf("🗑️ 快速删除季: %s (Series: %s)", seasonID, seriesID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "ok",
		"deleted_count": deletedCount,
		"failed":        failed,
	})
}
