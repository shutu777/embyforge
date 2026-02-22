package handler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// generateRandomSecret 生成随机 JWT 密钥
func generateRandomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("生成随机 JWT 密钥失败:", err)
	}
	return hex.EncodeToString(b)
}

// ProfileHandler 个人设置处理器
type ProfileHandler struct {
	DB         *gorm.DB
	UploadDir  string // 头像上传目录，如 /data/uploads/avatars
}

// NewProfileHandler 创建个人设置处理器
func NewProfileHandler(db *gorm.DB, dataDir string) *ProfileHandler {
	uploadDir := filepath.Join(dataDir, "uploads", "avatars")
	os.MkdirAll(uploadDir, 0755)
	return &ProfileHandler{DB: db, UploadDir: uploadDir}
}

// GetProfile 获取当前用户信息
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("userID")
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"avatar":   user.Avatar,
		},
	})
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=4"`
}

// ChangePassword 修改密码
func (h *ProfileHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "用户不存在"})
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "原密码错误"})
		return
	}

	// 生成新密码哈希
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "密码加密失败"})
		return
	}

	h.DB.Model(&user).Update("password", string(hashed))

	// 更新数据库中的 JWT secret，使所有旧 token 在下次重启后失效
	newSecret := generateRandomSecret()
	h.DB.Where("`key` = ?", "jwt_secret").Delete(&model.SystemConfig{})
	h.DB.Create(&model.SystemConfig{
		Key:         "jwt_secret",
		Value:       newSecret,
		Description: "JWT 签名密钥（自动生成，修改密码时会更新）",
	})

	log.Printf("🔐 用户 %s 修改了密码，JWT 密钥已更新", user.Username)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "密码修改成功，请重新登录"})
}

// ChangeUsernameRequest 修改用户名请求
type ChangeUsernameRequest struct {
	Username string `json:"username" binding:"required,min=2,max=50"`
}

// ChangeUsername 修改用户名
func (h *ProfileHandler) ChangeUsername(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req ChangeUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	// 检查用户名是否已被占用
	var count int64
	h.DB.Model(&model.User{}).Where("username = ? AND id != ?", req.Username, userID).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "用户名已被占用"})
		return
	}

	h.DB.Model(&model.User{}).Where("id = ?", userID).Update("username", req.Username)
	log.Printf("👤 用户 ID=%v 修改用户名为 %s", userID, req.Username)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "用户名修改成功"})
}

// UploadAvatar 上传头像
func (h *ProfileHandler) UploadAvatar(c *gin.Context) {
	userID, _ := c.Get("userID")

	file, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请选择头像文件"})
		return
	}

	// 限制文件大小 2MB
	if file.Size > 2*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "头像文件不能超过 2MB"})
		return
	}

	// 生成文件名
	ext := filepath.Ext(file.Filename)
	filename := fmt.Sprintf("avatar_%v_%d%s", userID, time.Now().Unix(), ext)
	savePath := filepath.Join(h.UploadDir, filename)

	if err := c.SaveUploadedFile(file, savePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "头像保存失败"})
		return
	}

	// 设置文件权限，确保 nginx 用户可以读取
	os.Chmod(savePath, 0644)

	// 删除旧头像文件
	var user model.User
	if err := h.DB.First(&user, userID).Error; err == nil && user.Avatar != "" {
		oldPath := filepath.Join(h.UploadDir, filepath.Base(user.Avatar))
		os.Remove(oldPath)
	}

	// 更新数据库中的头像路径（存储相对 URL）
	avatarURL := "/uploads/avatars/" + filename
	h.DB.Model(&model.User{}).Where("id = ?", userID).Update("avatar", avatarURL)

	log.Printf("🖼️  用户 ID=%v 上传了新头像", userID)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "头像上传成功", "data": gin.H{"avatar": avatarURL}})
}
