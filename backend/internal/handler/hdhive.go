package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"embyforge/internal/model"
	"embyforge/internal/service"
	"embyforge/internal/tmdb"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HDHiveHandler HDHive 功能处理器
type HDHiveHandler struct {
	DB      *gorm.DB
	Service *service.HDHiveService
}

// NewHDHiveHandler 创建 HDHive 处理器
func NewHDHiveHandler(db *gorm.DB) *HDHiveHandler {
	return &HDHiveHandler{
		DB:      db,
		Service: service.NewHDHiveService(),
	}
}

// getConfig 从数据库获取 HDHive 配置
func (h *HDHiveHandler) getConfig(key string) string {
	var config model.SystemConfig
	if err := h.DB.Where("`key` = ?", key).First(&config).Error; err != nil {
		return ""
	}
	return config.Value
}

// saveConfig 保存 HDHive 配置到数据库
func (h *HDHiveHandler) saveConfig(key, value string) error {
	var config model.SystemConfig
	err := h.DB.Where("`key` = ?", key).First(&config).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return h.DB.Create(&model.SystemConfig{
				Key:   key,
				Value: value,
			}).Error
		}
		return err
	}
	config.Value = value
	return h.DB.Save(&config).Error
}

// isTokenExpired 检查 JWT token 是否已过期
// 提前 5 分钟视为过期，避免请求时刚好失效
func (h *HDHiveHandler) isTokenExpired(token string) bool {
	if token == "" {
		return true
	}

	// JWT 格式: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}

	// 解码 payload（Base64URL）
	payload := parts[1]
	// 补齐 Base64 padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		log.Printf("⚠️ [HDHive] 解析 token payload 失败: %v", err)
		return true
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		log.Printf("⚠️ [HDHive] 解析 token claims 失败: %v", err)
		return true
	}

	if claims.Exp == 0 {
		return true
	}

	// 提前 5 分钟视为过期
	return time.Now().Unix() > claims.Exp-300
}

// ensureLoggedIn 确保 HDHive 已登录且 token 未过期
// 如果 token 过期，自动使用已保存的账号密码重新登录
// 返回 (token, cookie, error)
func (h *HDHiveHandler) ensureLoggedIn() (string, string, error) {
	token := h.getConfig("hdhive_token")
	cookie := h.getConfig("hdhive_cookie")

	// token 存在且未过期，直接使用
	if token != "" && !h.isTokenExpired(token) {
		return token, cookie, nil
	}

	// token 为空或已过期，尝试自动重新登录
	username := h.getConfig("hdhive_username")
	password := h.getConfig("hdhive_password")
	if username == "" || password == "" {
		return "", "", fmt.Errorf("请先配置 HDHive 账号和密码")
	}

	if token == "" {
		log.Printf("🔄 [HDHive] 未登录，自动登录中...")
	} else {
		log.Printf("🔄 [HDHive] Token 已过期，自动重新登录中...")
	}

	result, err := h.Service.Login(username, password)
	if err != nil {
		return "", "", fmt.Errorf("自动登录失败: %w", err)
	}

	// 缓存新的 token 和 cookie
	if err := h.saveConfig("hdhive_token", result.Token); err != nil {
		log.Printf("⚠️ [HDHive] 保存 token 失败: %v", err)
	}
	if err := h.saveConfig("hdhive_cookie", result.Cookie); err != nil {
		log.Printf("⚠️ [HDHive] 保存 cookie 失败: %v", err)
	}

	log.Printf("✅ [HDHive] 自动登录成功")
	return result.Token, result.Cookie, nil
}

// Login POST /api/hdhive/login
// 使用配置的账号密码登录 HDHive，缓存 token 和 cookie
func (h *HDHiveHandler) Login(c *gin.Context) {
	username := h.getConfig("hdhive_username")
	password := h.getConfig("hdhive_password")

	if username == "" || password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 HDHive 账号和密码"})
		return
	}

	result, err := h.Service.Login(username, password)
	if err != nil {
		log.Printf("❌ [HDHive] 登录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "HDHive 登录失败: " + err.Error()})
		return
	}

	// 缓存 token 和 cookie 到数据库
	if err := h.saveConfig("hdhive_token", result.Token); err != nil {
		log.Printf("⚠️ [HDHive] 保存 token 失败: %v", err)
	}
	if err := h.saveConfig("hdhive_cookie", result.Cookie); err != nil {
		log.Printf("⚠️ [HDHive] 保存 cookie 失败: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    result,
		"message": "HDHive 登录成功",
	})
}

// Search GET /api/hdhive/search?query=xxx
// 搜索 HDHive 资源
func (h *HDHiveHandler) Search(c *gin.Context) {
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请输入搜索关键字"})
		return
	}

	token, cookie, err := h.ensureLoggedIn()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	result, err := h.Service.Search(query, token, cookie)
	if err != nil {
		log.Printf("❌ [HDHive] 搜索失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "HDHive 搜索失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetTmdbInfo GET /api/hdhive/tmdb-info?tmdb_id=xxx&media_type=tv|movie
// 获取 TMDB 详情信息（海报、背景图、简介等）
func (h *HDHiveHandler) GetTmdbInfo(c *gin.Context) {
	tmdbIDStr := c.Query("tmdb_id")
	mediaType := c.Query("media_type")

	if tmdbIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "tmdb_id 不能为空"})
		return
	}
	if mediaType != "tv" && mediaType != "movie" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "media_type 必须为 tv 或 movie"})
		return
	}

	var tmdbID int
	if _, err := fmt.Sscanf(tmdbIDStr, "%d", &tmdbID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "tmdb_id 格式错误"})
		return
	}

	// 从 SystemConfig 读取 TMDB API Key
	apiKey := h.getConfig("tmdb_api_key")
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 TMDB API Key"})
		return
	}

	client := tmdb.NewClient(apiKey)
	info, err := client.GetDetail(tmdbID, mediaType, "zh-CN")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取TMDB信息失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": info})
}

// GetDetail GET /api/hdhive/detail?tmdb_id=xxx&media_type=tv|movie
// 获取资源详情（115 资源列表）
func (h *HDHiveHandler) GetDetail(c *gin.Context) {
	tmdbIDStr := c.Query("tmdb_id")
	mediaType := c.Query("media_type")

	if tmdbIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "tmdb_id 不能为空"})
		return
	}
	if mediaType != "tv" && mediaType != "movie" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "media_type 必须为 tv 或 movie"})
		return
	}

	var tmdbID int
	if _, err := fmt.Sscanf(tmdbIDStr, "%d", &tmdbID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "tmdb_id 格式错误"})
		return
	}

	token, cookie, err := h.ensureLoggedIn()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	resources, err := h.Service.GetResourceDetail(tmdbID, mediaType, token, cookie)
	if err != nil {
		log.Printf("❌ [HDHive] 获取详情失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取详情失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": resources})
}

// UnlockResource POST /api/hdhive/unlock
// 解锁 115 资源
func (h *HDHiveHandler) UnlockResource(c *gin.Context) {
	var req struct {
		ResourceID string `json:"resource_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ResourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "resource_id 不能为空"})
		return
	}

	token, cookie, err := h.ensureLoggedIn()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	result, err := h.Service.UnlockResource(req.ResourceID, token, cookie)
	if err != nil {
		log.Printf("❌ [HDHive] 解锁资源失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "解锁资源失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
