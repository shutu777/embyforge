package handler

import (
	"log"
	"net/http"

	"embyforge/internal/middleware"
	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	DB        *gorm.DB
	JWTSecret string
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(db *gorm.DB, jwtSecret string) *AuthHandler {
	return &AuthHandler{DB: db, JWTSecret: jwtSecret}
}

// LoginRequest 登录请求体
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应体
type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
}

// Login 处理用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	// 查询用户
	var user model.User
	if err := h.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		log.Printf("🔐 用户 %s 登录失败: 用户不存在", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户名或密码错误"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		log.Printf("🔐 用户 %s 登录失败: 密码错误", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "用户名或密码错误"})
		return
	}

	// 生成 JWT 令牌
	token, err := middleware.GenerateToken(user.ID, user.Username, h.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "令牌生成失败"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:    token,
		Username: user.Username,
	})
	log.Printf("🔐 用户 %s 登录成功", user.Username)
}
