package handler

import (
	"log"
	"net/http"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SystemConfigHandler 系统配置处理器
type SystemConfigHandler struct {
	DB *gorm.DB
}

// NewSystemConfigHandler 创建系统配置处理器
func NewSystemConfigHandler(db *gorm.DB) *SystemConfigHandler {
	return &SystemConfigHandler{DB: db}
}

// UpdateConfigRequest 更新配置请求体
type UpdateConfigRequest struct {
	Value string `json:"value"`
}

// GetAllConfigs GET /api/system-config
// 返回所有系统配置项
func (h *SystemConfigHandler) GetAllConfigs(c *gin.Context) {
	var configs []model.SystemConfig
	if err := h.DB.Order("id ASC").Find(&configs).Error; err != nil {
		log.Printf("❌ 查询系统配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

// UpdateConfig PUT /api/system-config/:key
// 更新指定 key 的配置值
func (h *SystemConfigHandler) UpdateConfig(c *gin.Context) {
	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "配置键名不能为空"})
		return
	}

	var req UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	var config model.SystemConfig
	if err := h.DB.Where("key = ?", key).First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "配置项不存在"})
			return
		}
		log.Printf("❌ 查询配置项失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询配置失败"})
		return
	}

	config.Value = req.Value
	if err := h.DB.Save(&config).Error; err != nil {
		log.Printf("❌ 更新配置项失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新配置失败"})
		return
	}

	log.Printf("⚙️ 系统配置已更新: %s", key)
	c.JSON(http.StatusOK, gin.H{"data": config, "message": "配置更新成功"})
}
