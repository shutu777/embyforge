package handler

import (
	"log"
	"net/http"

	"embyforge/internal/model"
	"embyforge/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Pan115Handler 115网盘处理器
type Pan115Handler struct {
	DB      *gorm.DB
	Service *service.Pan115Service
}

// NewPan115Handler 创建 115 网盘处理器
func NewPan115Handler(db *gorm.DB) *Pan115Handler {
	return &Pan115Handler{
		DB:      db,
		Service: service.NewPan115Service(),
	}
}

// 获取配置
func (h *Pan115Handler) getConfig(key string) string {
	var config model.SystemConfig
	if err := h.DB.Where("`key` = ?", key).First(&config).Error; err != nil {
		return ""
	}
	return config.Value
}

// TestCookie POST /api/pan115/test-cookie
// 测试 115 Cookie 是否有效
func (h *Pan115Handler) TestCookie(c *gin.Context) {
	cookie := h.getConfig("pan115_cookie")
	if cookie == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先保存 115 Cookie"})
		return
	}

	result, err := h.Service.TestCookie(cookie)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "测试失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// Transfer POST /api/pan115/transfer
// 转存分享链接到 115 网盘
func (h *Pan115Handler) Transfer(c *gin.Context) {
	var req struct {
		ShareURL string `json:"share_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ShareURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "share_url 不能为空"})
		return
	}

	cookie := h.getConfig("pan115_cookie")
	cid := h.getConfig("pan115_cid")

	if cookie == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 115 网盘 Cookie"})
		return
	}
	if cid == "" {
		cid = "0" // 默认根目录
	}

	result, err := h.Service.Transfer(req.ShareURL, cookie, cid)
	if err != nil {
		log.Printf("❌ [115] 转存失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "转存失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}
