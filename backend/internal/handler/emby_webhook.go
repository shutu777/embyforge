package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"embyforge/internal/service"

	"github.com/gin-gonic/gin"
)

// EmbyWebhookHandler 处理 Emby 服务器的 Webhook 通知
type EmbyWebhookHandler struct {
	EventBuffer *service.EventBuffer
}

// NewEmbyWebhookHandler 创建 Emby Webhook 处理器
func NewEmbyWebhookHandler(eventBuffer *service.EventBuffer) *EmbyWebhookHandler {
	return &EmbyWebhookHandler{
		EventBuffer: eventBuffer,
	}
}

// HandleEmbyWebhook POST /api/webhook/emby
// 同时兼容 application/json 和 multipart/form-data 两种格式
func (h *EmbyWebhookHandler) HandleEmbyWebhook(c *gin.Context) {
	// 读取原始请求体
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("⚠️ Webhook: 读取请求体失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}
	if len(body) == 0 {
		log.Printf("⚠️ Webhook: 空请求体 (%s)", c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "空请求体"})
		return
	}

	// 从 body 中提取 JSON 数据（自动识别 JSON / multipart）
	jsonData := extractJSON(c.GetHeader("Content-Type"), body)
	if jsonData == nil {
		log.Printf("⚠️ Webhook: 无法提取 JSON 数据 (%s)", c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法解析请求内容"})
		return
	}

	// 检测测试通知
	if isTestNotification(jsonData) {
		log.Printf("📡 Webhook: 收到测试通知，连接正常")
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "test notification received"})
		return
	}

	// 解析 Webhook payload
	event, err := service.ParseEmbyWebhookPayload(jsonData)
	if err != nil {
		log.Printf("⚠️ Webhook: 解析失败: %v (%s)", err, c.ClientIP())
		c.JSON(http.StatusBadRequest, gin.H{"error": "解析 payload 失败"})
		return
	}

	// 过滤非相关 item 类型
	if !service.IsRelevantItemType(event.ItemType) {
		log.Printf("📡 Webhook: 忽略 [%s] %s (Type=%s)", event.EventType, event.ItemName, event.ItemType)
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "irrelevant item type"})
		return
	}

	// 判断操作类型
	var operation string
	if service.IsAddEvent(event.EventType) {
		operation = "add"
	} else if service.IsDeleteEvent(event.EventType) {
		operation = "delete"
	} else {
		log.Printf("📡 Webhook: 忽略 [%s] %s", event.EventType, event.ItemName)
		c.JSON(http.StatusOK, gin.H{"status": "ignored", "reason": "unrecognized event type"})
		return
	}

	// 入队到 EventBuffer
	h.EventBuffer.Add(&service.BufferedEvent{
		ItemID:    event.ItemID,
		ItemType:  event.ItemType,
		ItemName:  event.ItemName,
		Operation: operation,
		Timestamp: time.Now(),
	})

	// 构建日志中的来源描述
	var source string
	switch event.ItemType {
	case "Movie":
		if event.Year > 0 {
			source = fmt.Sprintf("🎬 %s (%d)", event.ItemName, event.Year)
		} else {
			source = fmt.Sprintf("🎬 %s", event.ItemName)
		}
	case "Episode":
		if event.SeriesName != "" && event.ParentIndexNumber > 0 && event.IndexNumber > 0 {
			source = fmt.Sprintf("📺 %s S%02dE%02d %s", event.SeriesName, event.ParentIndexNumber, event.IndexNumber, event.ItemName)
		} else if event.SeriesName != "" {
			source = fmt.Sprintf("📺 %s - %s", event.SeriesName, event.ItemName)
		} else {
			source = fmt.Sprintf("📺 %s", event.ItemName)
		}
	case "Series":
		if event.Year > 0 {
			source = fmt.Sprintf("📺 %s (%d)", event.ItemName, event.Year)
		} else {
			source = fmt.Sprintf("📺 %s", event.ItemName)
		}
	default:
		source = event.ItemName
	}

	log.Printf("📡 Webhook: [%s] %s (ID=%s, 缓冲: %d)", operation, source, event.ItemID, h.EventBuffer.PendingCount())
	c.JSON(http.StatusOK, gin.H{"status": "queued", "operation": operation})
}

// extractJSON 从请求体中提取 JSON 数据
// 优先根据 Content-Type 判断，fallback 到内容嗅探
func extractJSON(contentType string, body []byte) []byte {
	// 尝试根据 Content-Type 解析 multipart
	if strings.Contains(contentType, "multipart/form-data") {
		if data := extractFromMultipart(contentType, body); data != nil {
			return data
		}
	}

	// 尝试直接当 JSON 解析
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed
	}

	// Content-Type 可能不准确，尝试内容嗅探：body 以 "--" 开头说明是 multipart
	if bytes.HasPrefix(trimmed, []byte("--")) {
		// 从 body 中找 boundary
		if data := extractFromMultipartBySniffing(trimmed); data != nil {
			return data
		}
	}

	return nil
}

// extractFromMultipart 使用标准 multipart 解析器从 body 中提取 "data" 字段
func extractFromMultipart(contentType string, body []byte) []byte {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		if part.FormName() == "data" {
			data, err := io.ReadAll(part)
			if err != nil {
				break
			}
			return bytes.TrimSpace(data)
		}
	}
	return nil
}

// extractFromMultipartBySniffing 当 Content-Type 不准确时，从 body 内容嗅探 boundary 并提取 JSON
func extractFromMultipartBySniffing(body []byte) []byte {
	// 第一行就是 boundary（如 --c888c260-77b0-49b0-a9cd-f63193b1d256）
	idx := bytes.IndexByte(body, '\n')
	if idx < 0 {
		idx = bytes.IndexByte(body, '\r')
	}
	if idx < 3 {
		return nil
	}
	boundary := string(bytes.TrimSpace(body[:idx]))
	if strings.HasPrefix(boundary, "--") {
		boundary = boundary[2:] // 去掉前缀 "--"
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		if part.FormName() == "data" {
			data, err := io.ReadAll(part)
			if err != nil {
				break
			}
			return bytes.TrimSpace(data)
		}
	}
	return nil
}

// isTestNotification 检测是否为 Emby 测试通知
// 测试通知的 Event 为 "system.notificationtest"，或者没有 Event/Item 字段
func isTestNotification(data []byte) bool {
	var probe struct {
		Event string `json:"Event"`
		Item  struct {
			ID string `json:"Id"`
		} `json:"Item"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	// Event 包含 "test" 关键字，或者没有 Event 和 Item.Id
	if strings.Contains(strings.ToLower(probe.Event), "test") {
		return true
	}
	return probe.Event == "" && probe.Item.ID == ""
}
