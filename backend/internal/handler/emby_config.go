package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

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

// probeEmbyURL 探测指定 Emby 地址是否可达
// 发送 GET /emby/System/Info 请求，超时 3 秒
// 返回 serverID、serverName 和是否可达
func probeEmbyURL(baseURL, apiKey string) (serverID string, serverName string, ok bool) {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest("GET", baseURL+"/emby/System/Info", nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("X-Emby-Token", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	var info struct {
		ID         string `json:"Id"`
		ServerName string `json:"ServerName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", false
	}
	return info.ID, info.ServerName, true
}

// EmbyConfigRequest Emby 配置请求体
type EmbyConfigRequest struct {
	Host        string `json:"host" binding:"required"`
	Port        int    `json:"port" binding:"required"`
	APIKey      string `json:"api_key" binding:"required"`
	Username    string `json:"username"`     // 可选，用于删除操作认证
	Password    string `json:"password"`     // 可选，用于删除操作认证
	ExternalURL string `json:"external_url"` // 可选，Emby 外网访问地址
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

	// 返回时隐藏密码，只告知是否已配置
	hasPassword := config.Password != ""
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"id":           config.ID,
		"host":         config.Host,
		"port":         config.Port,
		"api_key":      config.APIKey,
		"username":     config.Username,
		"has_password":  hasPassword,
		"external_url": config.ExternalURL,
		"created_at":   config.CreatedAt,
		"updated_at":   config.UpdatedAt,
	}})
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
			Host:        req.Host,
			Port:        req.Port,
			APIKey:      req.APIKey,
			Username:    req.Username,
			Password:    req.Password,
			ExternalURL: req.ExternalURL,
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
	existing.Username = req.Username
	existing.ExternalURL = req.ExternalURL
	// 密码为空时保留原密码（前端不回传密码）
	if req.Password != "" {
		existing.Password = req.Password
	}
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

// GetServerInfo 获取 Emby 服务器信息（后端探测可达性，返回可用的 base_url）
// base_url 选择策略：配置了外网地址就返回外网地址（浏览器用外网访问），否则返回内网地址。
// 探测仅用内网地址（后端在内网），确认连通性和获取 server_id。
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

	internalURL := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// 返回给前端的 base_url：有外网地址就用外网（浏览器能访问），否则用内网
	baseURL := internalURL
	if config.ExternalURL != "" {
		baseURL = config.ExternalURL
	}

	// 用内网地址探测连通性和获取 server_id（后端在内网，不探测外网）
	serverID, _, ok := probeEmbyURL(internalURL, config.APIKey)

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"base_url":  baseURL,
			"server_id": serverID,
			"api_key":   config.APIKey,
			"connected": ok,
		},
	})
}
