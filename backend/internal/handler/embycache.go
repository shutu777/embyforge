package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmbyCacheHandler Emby 缓存管理处理器
type EmbyCacheHandler struct {
	DB *gorm.DB
}

// NewEmbyCacheHandler 创建 Emby 缓存处理器
func NewEmbyCacheHandler(db *gorm.DB) *EmbyCacheHandler {
	return &EmbyCacheHandler{DB: db}
}

// getEmbyClient 从数据库获取 Emby 配置并创建客户端
func (h *EmbyCacheHandler) getEmbyClient() (*emby.Client, error) {
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		return nil, err
	}
	return emby.NewClient(config.Host, config.Port, config.APIKey), nil
}

// GetEmbyCacheList GET /api/emby-cache - 获取 Emby 缓存列表（仅 Movie 和 Series）
func (h *EmbyCacheHandler) GetEmbyCacheList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	search := c.DefaultQuery("search", "")
	typeFilter := c.DefaultQuery("type", "") // "Movie" 或 "Series" 或 ""（全部）

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 基础查询：只查 Movie 和 Series
	baseQuery := func(q *gorm.DB) *gorm.DB {
		q = q.Where("type IN ?", []string{"Movie", "Series"})
		if typeFilter == "Movie" || typeFilter == "Series" {
			q = q.Where("type = ?", typeFilter)
		}
		if search != "" {
			q = q.Where("name LIKE ?", "%"+search+"%")
		}
		return q
	}

	// 查询总数
	var total int64
	baseQuery(h.DB.Model(&model.MediaCache{})).Count(&total)

	// 查询列表
	var items []model.MediaCache
	offset := (page - 1) * pageSize
	baseQuery(h.DB.Model(&model.MediaCache{})).
		Order("cached_at DESC").
		Offset(offset).Limit(pageSize).
		Find(&items)

	c.JSON(http.StatusOK, gin.H{
		"data":  items,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// GetEmbyCacheStatus GET /api/emby-cache/status - 获取 Emby 缓存统计
func (h *EmbyCacheHandler) GetEmbyCacheStatus(c *gin.Context) {
	var totalMovies int64
	h.DB.Model(&model.MediaCache{}).Where("type = ?", "Movie").Count(&totalMovies)

	var totalSeries int64
	h.DB.Model(&model.MediaCache{}).Where("type = ?", "Series").Count(&totalSeries)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"total_movies": totalMovies,
			"total_series": totalSeries,
		},
	})
}

// UpdateEmbyCache PUT /api/emby-cache/:id - 编辑缓存条目
func (h *EmbyCacheHandler) UpdateEmbyCache(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的 ID"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	var cache model.MediaCache
	if err := h.DB.First(&cache, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "未找到"})
		return
	}

	// 只允许编辑 Movie 和 Series
	if cache.Type != "Movie" && cache.Type != "Series" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "只能编辑电影和剧集"})
		return
	}

	cache.Name = req.Name
	if err := h.DB.Save(&cache).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok", "data": cache})
}

// DeleteEmbyCache DELETE /api/emby-cache/:id - 删除缓存条目
func (h *EmbyCacheHandler) DeleteEmbyCache(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的 ID"})
		return
	}

	var cache model.MediaCache
	if err := h.DB.First(&cache, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "未找到"})
		return
	}

	// 只允许删除 Movie 和 Series
	if cache.Type != "Movie" && cache.Type != "Series" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "只能删除电影和剧集"})
		return
	}

	// 如果是 Series，同时删除关联的 Episode 和 SeasonCache
	if cache.Type == "Series" {
		h.DB.Where("series_id = ?", cache.EmbyItemID).Delete(&model.MediaCache{})
		h.DB.Where("series_emby_item_id = ?", cache.EmbyItemID).Delete(&model.SeasonCache{})
	}

	h.DB.Delete(&cache)
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

// RefreshEmbyCache POST /api/emby-cache/:id/refresh - 刷新单个条目（从 Emby 重新拉取）
func (h *EmbyCacheHandler) RefreshEmbyCache(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "无效的 ID"})
		return
	}

	var cache model.MediaCache
	if err := h.DB.First(&cache, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "未找到"})
		return
	}

	if cache.Type != "Movie" && cache.Type != "Series" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "只能刷新电影和剧集"})
		return
	}

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 Emby 服务器连接信息"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// 通过 Emby API 直接获取单个条目
	embyItemID := cache.EmbyItemID
	refreshedItems, err := client.GetItemByID(ctx, embyItemID)
	if err != nil {
		log.Printf("⚠️ 从 Emby 获取条目失败 (ID=%s): %v", embyItemID, err)
	}

	if len(refreshedItems) == 0 {
		// Emby 中已不存在该条目，删除本地缓存
		if cache.Type == "Series" {
			h.DB.Where("series_id = ?", cache.EmbyItemID).Delete(&model.MediaCache{})
			h.DB.Where("series_emby_item_id = ?", cache.EmbyItemID).Delete(&model.SeasonCache{})
		}
		h.DB.Delete(&cache)
		c.JSON(http.StatusOK, gin.H{"message": "Emby 中已不存在该条目，已删除本地缓存", "deleted": true})
		return
	}

	// 更新本地缓存（Series/Movie 本身）
	newCache := model.NewMediaCacheFromItem(refreshedItems[0], cache.LibraryName)
	newCache.ID = cache.ID
	if err := h.DB.Save(&newCache).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新缓存失败"})
		return
	}

	// 如果是 Series，还需要刷新其下所有 Episode
	if cache.Type == "Series" {
		// 1. 删除该 Series 下所有旧 Episode 缓存和季缓存
		h.DB.Where("series_id = ? AND type = ?", embyItemID, "Episode").Delete(&model.MediaCache{})
		h.DB.Where("series_emby_item_id = ?", embyItemID).Delete(&model.SeasonCache{})

		// 2. 从 Emby 重新拉取该 Series 下所有 Episode
		episodePath := fmt.Sprintf("/emby/Items?ParentId=%s&Recursive=true&IncludeItemTypes=Episode&Fields=Path,ProviderIds,ImageTags,ParentIndexNumber,SeriesId,SeriesName,MediaSources&Limit=2000", embyItemID)
		body, err := client.DoRequestWithContext(ctx, episodePath)
		if err != nil {
			log.Printf("⚠️ 从 Emby 获取 Series Episode 失败 (SeriesID=%s): %v", embyItemID, err)
		} else {
			var resp emby.MediaItemsResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				log.Printf("⚠️ 解析 Episode 响应失败: %v", err)
			} else {
				// 去重写入
				seen := make(map[string]bool, len(resp.Items))
				for _, item := range resp.Items {
					if seen[item.ID] {
						continue
					}
					seen[item.ID] = true
					epCache := model.NewMediaCacheFromItem(item, newCache.LibraryName)
					h.DB.Create(&epCache)
				}

				// 3. 重建该 Series 的季缓存
				var seasonAggs []struct {
					SeasonNumber int
					EpisodeCount int
				}
				h.DB.Model(&model.MediaCache{}).
					Select("parent_index_number as season_number, COUNT(*) as episode_count").
					Where("series_id = ? AND type = ?", embyItemID, "Episode").
					Group("parent_index_number").
					Find(&seasonAggs)

				for _, agg := range seasonAggs {
					seasonEmbyID := fmt.Sprintf("%s_S%d", embyItemID, agg.SeasonNumber)
					h.DB.Create(&model.SeasonCache{
						SeriesEmbyItemID: embyItemID,
						SeasonEmbyItemID: seasonEmbyID,
						SeasonNumber:     agg.SeasonNumber,
						EpisodeCount:     agg.EpisodeCount,
						CachedAt:         time.Now(),
					})
				}

				log.Printf("🔄 已刷新 Series 缓存: %s (%s)，Episode: %d 个，季: %d 个",
					newCache.Name, embyItemID, len(resp.Items), len(seasonAggs))
			}
		}
	} else {
		log.Printf("🔄 已刷新 Emby 缓存: %s (%s)", newCache.Name, newCache.EmbyItemID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "ok", "data": newCache})
}
