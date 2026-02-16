package handler

import (
	"log"
	"net/http"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmbyConfigHandler Emby 配置处理器
type EmbyConfigHandler struct {
	DB *gorm.DB
}

// NewEmbyConfigHandler 创建 Emby 配置处理器
func NewEmbyConfigHandler(db *gorm.DB) *EmbyConfigHandler {
	return &EmbyConfigHandler{DB: db}
}

// EmbyConfigRequest Emby 配置请求体
type EmbyConfigRequest struct {
	Host   string `json:"host" binding:"required"`
	Port   int    `json:"port" binding:"required"`
	APIKey string `json:"api_key" binding:"required"`
}

// GetConfig 获取已保存的 Emby 配置
func (h *EmbyConfigHandler) GetConfig(c *gin.Context) {
	var config model.EmbyConfig
	result := h.DB.First(&config)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取配置失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": config})
}

// SaveConfig 保存 Emby 配置（upsert，只保留一条记录）
func (h *EmbyConfigHandler) SaveConfig(c *gin.Context) {
	var req EmbyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	var existing model.EmbyConfig
	result := h.DB.First(&existing)

	if result.Error == gorm.ErrRecordNotFound {
		// 创建新记录
		config := model.EmbyConfig{
			Host:   req.Host,
			Port:   req.Port,
			APIKey: req.APIKey,
		}
		if err := h.DB.Create(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "保存配置失败"})
			return
		}
		log.Printf("⚙️ Emby 配置已保存: %s:%d", req.Host, req.Port)
		c.JSON(http.StatusOK, gin.H{"data": config, "message": "配置保存成功"})
		return
	}

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询配置失败"})
		return
	}

	// 更新已有记录
	existing.Host = req.Host
	existing.Port = req.Port
	existing.APIKey = req.APIKey
	if err := h.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新配置失败"})
		return
	}

	log.Printf("⚙️ Emby 配置已保存: %s:%d", req.Host, req.Port)
	c.JSON(http.StatusOK, gin.H{"data": existing, "message": "配置更新成功"})
}

// TestConnection 测试 Emby 服务器连接
func (h *EmbyConfigHandler) TestConnection(c *gin.Context) {
	var req EmbyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	client := emby.NewClient(req.Host, req.Port, req.APIKey)
	info, err := client.TestConnection()
	if err != nil {
		log.Printf("⚙️ Emby 连接测试失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "连接失败",
			"error":   err.Error(),
		})
		return
	}

	log.Printf("⚙️ Emby 连接测试成功: %s (版本 %s)", info.ServerName, info.Version)
	c.JSON(http.StatusOK, gin.H{
		"message":     "连接成功",
		"server_name": info.ServerName,
		"version":     info.Version,
	})
}

// GetServerInfo 获取 Emby 服务器信息（包含 serverId，用于前端构建跳转链接）
func (h *EmbyConfigHandler) GetServerInfo(c *gin.Context) {
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取配置失败"})
		return
	}

	client := emby.NewClient(config.Host, config.Port, config.APIKey)
	info, err := client.TestConnection()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "无法连接 Emby 服务器"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"host":        config.Host,
			"port":        config.Port,
			"server_id":   info.ID,
			"server_name": info.ServerName,
		},
	})
}
