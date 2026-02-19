package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"embyforge/internal/model"
	"embyforge/internal/util"
)

// WebhookHandler Webhook处理器
type WebhookHandler struct {
	DB             *gorm.DB
	SymediaHandler *SymediaHandler
}

// NewWebhookHandler 创建Webhook处理器
func NewWebhookHandler(db *gorm.DB, symediaHandler *SymediaHandler) *WebhookHandler {
	return &WebhookHandler{
		DB:             db,
		SymediaHandler: symediaHandler,
	}
}

// GitHubPushEvent GitHub推送事件结构
type GitHubPushEvent struct {
	Ref        string `json:"ref"` // 分支引用，格式：refs/heads/main
	Repository struct {
		FullName string `json:"full_name"` // 仓库全名，格式：owner/repo
	} `json:"repository"`
	HeadCommit struct {
		ID       string   `json:"id"`       // 提交SHA
		Modified []string `json:"modified"` // 修改的文件列表
		Added    []string `json:"added"`    // 新增的文件列表
	} `json:"head_commit"`
}

// verifyGitHubSignature 验证GitHub Webhook签名
// 参数:
//   - secret: Webhook密钥
//   - payload: 请求体原始字节
//   - signature: X-Hub-Signature-256 头的值
// 返回:
//   - bool: 签名是否有效
func (h *WebhookHandler) verifyGitHubSignature(secret string, payload []byte, signature string) bool {
	if signature == "" {
		return false
	}
	
	// GitHub使用HMAC SHA256算法
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := mac.Sum(nil)
	expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC)
	
	// 使用恒定时间比较防止时序攻击
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// shouldTriggerRefresh 判断是否应该触发配置刷新
// 参数:
//   - event: GitHub推送事件
//   - config: Webhook配置
// 返回:
//   - bool: 是否应该触发刷新
func (h *WebhookHandler) shouldTriggerRefresh(event *GitHubPushEvent, config *model.WebhookConfig) bool {
	// 检查分支是否匹配
	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	if branch != config.Branch {
		log.Printf("ℹ️  [Webhook] 分支不匹配: 期望=%s, 实际=%s", config.Branch, branch)
		return false
	}
	
	// 如果配置的文件路径为空或为"*"，则监听所有文件变化
	if config.FilePath == "" || config.FilePath == "*" {
		// 合并修改和新增的文件列表
		allFiles := append(event.HeadCommit.Modified, event.HeadCommit.Added...)
		if len(allFiles) > 0 {
			log.Printf("✅ [Webhook] 监听所有文件，检测到 %d 个文件变更", len(allFiles))
			return true
		}
		log.Printf("ℹ️  [Webhook] 未检测到文件变更")
		return false
	}
	
	// 检查特定文件路径是否匹配
	// 合并修改和新增的文件列表
	allFiles := append(event.HeadCommit.Modified, event.HeadCommit.Added...)
	
	for _, file := range allFiles {
		if strings.Contains(file, config.FilePath) {
			log.Printf("✅ [Webhook] 文件路径匹配: %s 包含 %s", file, config.FilePath)
			return true
		}
	}
	
	log.Printf("ℹ️  [Webhook] 文件路径不匹配: 监听路径=%s, 变更文件=%v", config.FilePath, allFiles)
	return false
}

// HandleGitHubWebhook 处理GitHub Webhook推送事件
// POST /api/webhook/github
func (h *WebhookHandler) HandleGitHubWebhook(c *gin.Context) {
	// 读取请求体
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("❌ [Webhook] 无法读取请求体: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无法读取请求体",
		})
		return
	}
	
	// 处理不同的 Content-Type
	var jsonPayload []byte
	contentType := c.GetHeader("Content-Type")
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// URL 编码格式：payload={"ref":"..."}
		// 需要解析表单数据，提取 payload 字段
		payloadStr := string(payload)
		if strings.HasPrefix(payloadStr, "payload=") {
			// 去掉 "payload=" 前缀，然后 URL 解码
			jsonStr := strings.TrimPrefix(payloadStr, "payload=")
			decoded, err := url.QueryUnescape(jsonStr)
			if err != nil {
				log.Printf("❌ [Webhook] URL 解码失败: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "无法解码请求数据",
				})
				return
			}
			jsonPayload = []byte(decoded)
		} else {
			log.Printf("❌ [Webhook] URL 编码格式错误，缺少 payload= 前缀")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "请求格式错误",
			})
			return
		}
	} else {
		// 直接是 JSON 格式
		jsonPayload = payload
	}
	
	// 获取签名头
	signature := c.GetHeader("X-Hub-Signature-256")
	
	// 查询Webhook配置
	var config model.WebhookConfig
	if err := h.DB.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("❌ [Webhook] 未找到Webhook配置")
			c.JSON(http.StatusNotFound, gin.H{
				"error": "未配置Webhook",
			})
			return
		}
		log.Printf("❌ [Webhook] 查询配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询配置失败，请稍后重试",
		})
		return
	}
	
	// 验证签名（使用原始 payload，不是解码后的）
	if !h.verifyGitHubSignature(config.Secret, payload, signature) {
		log.Printf("⚠️  [Webhook] GitHub签名验证失败")
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "签名验证失败",
		})
		return
	}
	
	// 解析GitHub推送事件
	var event GitHubPushEvent
	if err := json.Unmarshal(jsonPayload, &event); err != nil {
		log.Printf("❌ [Webhook] 无法解析事件: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无法解析事件数据",
		})
		return
	}
	
	// 检查是否应该触发刷新
	if !h.shouldTriggerRefresh(&event, &config) {
		log.Printf("ℹ️  [Webhook] 事件不匹配监听条件，跳过")
		c.JSON(http.StatusOK, gin.H{
			"message": "事件已接收，但不触发刷新",
		})
		return
	}
	
	// 触发配置刷新
	branch := strings.TrimPrefix(event.Ref, "refs/heads/")
	commitSHA := event.HeadCommit.ID
	shortCommitSHA := commitSHA
	if len(commitSHA) > 7 {
		shortCommitSHA = commitSHA[:7]
	}
	
	// 合并变更文件列表
	changedFiles := append(event.HeadCommit.Modified, event.HeadCommit.Added...)
	
	log.Printf("🔄 [Webhook] GitHub推送触发配置刷新: repo=%s, branch=%s, commit=%s", 
		event.Repository.FullName, branch, shortCommitSHA)
	
	// 记录开始时间
	startTime := time.Now()
	
	// 调用Symedia API
	err = h.SymediaHandler.callSymediaAPI(config.SymediaUrl, config.AuthToken)
	
	// 计算耗时
	duration := time.Since(startTime).Milliseconds()
	
	// 记录日志到WebhookLog表
	logEntry := model.WebhookLog{
		Source:    "github",
		RepoName:  event.Repository.FullName,
		Branch:    branch,
		CommitSHA: commitSHA,
		Success:   err == nil,
		ErrorMsg:  "",
	}
	if err != nil {
		logEntry.ErrorMsg = err.Error()
	}
	
	if dbErr := h.DB.Create(&logEntry).Error; dbErr != nil {
		log.Printf("⚠️  [Webhook] 记录日志失败: %v", dbErr)
		// 不影响主流程，继续执行
	}
	
	// 记录结构化日志
	result := "success"
	errorMsg := ""
	if err != nil {
		result = "failure"
		errorMsg = err.Error()
	}
	
	structuredLog := util.FormatWebhookLog(
		event.Repository.FullName,
		branch,
		commitSHA,
		changedFiles,
		result,
		duration,
		errorMsg,
	)
	
	// 返回响应
	if err != nil {
		log.Printf("❌ [Webhook] 配置刷新失败: %s", structuredLog)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "配置刷新失败，请稍后重试",
		})
		return
	}
	
	log.Printf("✅ [Webhook] 配置刷新成功: %s", structuredLog)
	c.JSON(http.StatusOK, gin.H{
		"message": "配置刷新成功",
	})
}
