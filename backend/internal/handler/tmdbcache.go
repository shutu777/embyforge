package handler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TmdbCacheHandler struct {
	DB *gorm.DB
}

func NewTmdbCacheHandler(db *gorm.DB) *TmdbCacheHandler {
	return &TmdbCacheHandler{DB: db}
}

type TmdbCacheGroup struct {
	TmdbID      int               `json:"tmdb_id"`
	Name        string            `json:"name"`
	SeasonCount int               `json:"season_count"`
	CachedAt    time.Time         `json:"cached_at"`
	Seasons     []model.TmdbCache `json:"seasons"`
}

func (h *TmdbCacheHandler) GetTmdbCacheList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	search := c.DefaultQuery("search", "")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建搜索条件
	// 支持：名称关键字（英文/中文）、TMDB ID
	buildSearchCondition := func(q *gorm.DB) *gorm.DB {
		if search == "" {
			return q
		}
		// 如果是纯数字，按 TMDB ID 精确匹配 + 英文名模糊匹配
		if tmdbID, err := strconv.Atoi(search); err == nil {
			return q.Where("name LIKE ? OR tmdb_id = ?", "%"+search+"%", tmdbID)
		}
		// 非数字：先在 tmdb_caches 中搜英文名
		// 同时在 media_caches 中搜中文名（Series 类型），提取 TMDB ID 关联
		var matchedTmdbIDs []int
		h.DB.Model(&model.MediaCache{}).
			Where("type = 'Series' AND name LIKE ?", "%"+search+"%").
			Where("provider_ids LIKE '%Tmdb%'").
			Pluck("DISTINCT CAST(json_extract(provider_ids, '$.Tmdb') AS INTEGER)", &matchedTmdbIDs)

		if len(matchedTmdbIDs) > 0 {
			return q.Where("name LIKE ? OR tmdb_id IN ?", "%"+search+"%", matchedTmdbIDs)
		}
		return q.Where("name LIKE ?", "%"+search+"%")
	}

	var totalCount int64
	cq := buildSearchCondition(h.DB.Model(&model.TmdbCache{}))
	cq.Distinct("tmdb_id").Count(&totalCount)

	type groupRow struct {
		TmdbID      int       `gorm:"column:tmdb_id"`
		SeasonCount int       `gorm:"column:season_count"`
		Name        string    `gorm:"column:name"`
		CachedAt    time.Time `gorm:"column:cached_at"`
	}
	var groupRows []groupRow
	offset := (page - 1) * pageSize
	iq := buildSearchCondition(h.DB.Model(&model.TmdbCache{}))
	iq.Select("tmdb_id, COUNT(*) as season_count, MAX(name) as name, MAX(cached_at) as cached_at").
		Group("tmdb_id").Order("cached_at DESC").Offset(offset).Limit(pageSize).Find(&groupRows)
	tmdbIDs := make([]int, len(groupRows))
	for i, r := range groupRows {
		tmdbIDs[i] = r.TmdbID
	}
	var groups []TmdbCacheGroup
	if len(tmdbIDs) > 0 {
		var caches []model.TmdbCache
		h.DB.Where("tmdb_id IN ?", tmdbIDs).Order("season_number ASC").Find(&caches)
		cacheMap := make(map[int][]model.TmdbCache)
		for _, cc := range caches {
			cacheMap[cc.TmdbID] = append(cacheMap[cc.TmdbID], cc)
		}
		for _, r := range groupRows {
			groups = append(groups, TmdbCacheGroup{
				TmdbID: r.TmdbID, Name: r.Name, SeasonCount: r.SeasonCount,
				CachedAt: r.CachedAt, Seasons: cacheMap[r.TmdbID],
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": groups, "total": totalCount, "page": page, "page_size": pageSize,
	})
}

func (h *TmdbCacheHandler) GetTmdbCacheStatus(c *gin.Context) {
	var totalRecords int64
	h.DB.Model(&model.TmdbCache{}).Count(&totalRecords)
	var totalShows int64
	h.DB.Model(&model.TmdbCache{}).Distinct("tmdb_id").Count(&totalShows)
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"total_records": totalRecords, "total_shows": totalShows},
	})
}

func (h *TmdbCacheHandler) UpdateTmdbCache(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid ID"})
		return
	}
	var req struct {
		EpisodeCount int    `json:"episode_count"`
		SeasonName   string `json:"season_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "bad request"})
		return
	}
	var cache model.TmdbCache
	if err := h.DB.First(&cache, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	cache.EpisodeCount = req.EpisodeCount
	cache.SeasonName = req.SeasonName
	cache.UpdatedAt = time.Now()
	if err := h.DB.Save(&cache).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok", "data": cache})
}

func (h *TmdbCacheHandler) DeleteTmdbCache(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid ID"})
		return
	}
	result := h.DB.Delete(&model.TmdbCache{}, id)
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *TmdbCacheHandler) DeleteTmdbCacheByShow(c *gin.Context) {
	tmdbID, err := strconv.Atoi(c.Param("tmdbId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid TMDB ID"})
		return
	}
	result := h.DB.Where("tmdb_id = ?", tmdbID).Delete(&model.TmdbCache{})
	log.Printf("deleted %d TMDB cache records for TMDB ID=%d", result.RowsAffected, tmdbID)
	c.JSON(http.StatusOK, gin.H{
		"message": "ok", "data": gin.H{"deleted_count": result.RowsAffected},
	})
}

func (h *TmdbCacheHandler) ClearTmdbCache(c *gin.Context) {
	result := h.DB.Exec("DELETE FROM tmdb_caches")
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "clear failed"})
		return
	}
	h.DB.Exec("DELETE FROM sqlite_sequence WHERE name='tmdb_caches'")
	log.Printf("cleared TMDB cache, deleted %d records", result.RowsAffected)
	c.JSON(http.StatusOK, gin.H{
		"message": "ok", "data": gin.H{"deleted_count": result.RowsAffected},
	})
}
