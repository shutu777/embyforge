package handler

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"embyforge/internal/model"
	"embyforge/internal/tmdb"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TmdbSearchHandler TMDB 搜索处理器
type TmdbSearchHandler struct {
	DB *gorm.DB
}

// NewTmdbSearchHandler 创建 TMDB 搜索处理器
func NewTmdbSearchHandler(db *gorm.DB) *TmdbSearchHandler {
	return &TmdbSearchHandler{DB: db}
}

// SearchTmdb GET /api/tmdb/search?query=xxx&media_type=movie|tv
// 搜索 TMDB 电影或剧集
func (h *TmdbSearchHandler) SearchTmdb(c *gin.Context) {
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入搜索关键字"})
		return
	}

	mediaType := strings.TrimSpace(c.Query("media_type"))
	if mediaType != "movie" && mediaType != "tv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media_type 必须为 movie 或 tv"})
		return
	}

	// 从 SystemConfig 读取 TMDB API Key
	var apiKeyConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "tmdb_api_key").First(&apiKeyConfig).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 TMDB API Key"})
			return
		}
		log.Printf("❌ [TMDB] 读取 API Key 配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取配置失败"})
		return
	}

	if strings.TrimSpace(apiKeyConfig.Value) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 TMDB API Key"})
		return
	}

	// 创建 TMDB 客户端并搜索
	client := tmdb.NewClient(apiKeyConfig.Value)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	var results []tmdb.TmdbSearchResult
	var err error

	if mediaType == "movie" {
		results, err = client.SearchMovies(ctx, query, "zh-CN")
	} else {
		results, err = client.SearchTV(ctx, query, "zh-CN")
	}

	if err != nil {
		log.Printf("❌ [TMDB] 搜索失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TMDB 搜索失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": results})
}
