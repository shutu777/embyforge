package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"embyforge/internal/service"

	"github.com/gin-gonic/gin"
	"pgregory.net/rapid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// Feature: emby-webhook-sync, Property 1: Webhook 事件路由正确性
// Validates: Requirements 1.2, 1.3
//
// 对于任意 valid Emby Webhook payload（包含已识别的事件类型和相关 item 类型），
// Webhook handler 应返回 200 且 operation 正确（"add" 或 "delete"）。
func TestProperty_WebhookEventRoutingCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 创建 EventBuffer 和 Handler
		syncLock := &service.SyncLock{}
		eb := service.NewEventBuffer(syncLock, nil)
		handler := NewEmbyWebhookHandler(eb)

		// 随机选择事件类型和对应的期望 operation
		type eventCase struct {
			eventType string
			operation string
		}
		cases := []eventCase{
			{"library.new", "add"},
			{"item.add", "add"},
			{"item.update", "add"},
			{"library.update", "add"},
			{"library.deleted", "delete"},
			{"item.remove", "delete"},
			{"item.delete", "delete"},
			{"library.remove", "delete"},
		}
		chosen := rapid.SampledFrom(cases).Draw(t, "eventCase")

		itemID := fmt.Sprintf("item-%d", rapid.IntRange(1, 99999).Draw(t, "itemID"))
		itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, "itemType")
		itemName := fmt.Sprintf("Media_%d", rapid.IntRange(1, 999).Draw(t, "itemName"))

		payload := map[string]interface{}{
			"Title": "Emby Server Notification",
			"Event": chosen.eventType,
			"Item": map[string]interface{}{
				"Id":   itemID,
				"Name": itemName,
				"Type": itemType,
			},
			"Server": map[string]interface{}{
				"Id":   "server-1",
				"Name": "Test Server",
			},
		}
		data, _ := json.Marshal(payload)

		// 发送请求
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/webhook/emby", bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.HandleEmbyWebhook(c)

		// 验证返回 200
		if w.Code != http.StatusOK {
			t.Fatalf("期望 HTTP 200, 实际 %d, body: %s", w.Code, w.Body.String())
		}

		// 验证 operation 正确
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		if resp["status"] != "queued" {
			t.Fatalf("期望 status=queued, 实际 %v", resp["status"])
		}
		if resp["operation"] != chosen.operation {
			t.Fatalf("事件 %q 期望 operation=%q, 实际 %v",
				chosen.eventType, chosen.operation, resp["operation"])
		}

		// 验证事件已入队
		if eb.PendingCount() == 0 {
			t.Fatalf("事件未入队到 EventBuffer")
		}

		// 清空缓冲区供下次迭代
		eb.DrainEvents()
	})
}

// Feature: emby-webhook-sync, Property 2: 畸形 Payload 拒绝
// Validates: Requirements 1.5
//
// 对于任意非 valid JSON 或缺少必要字段的 payload，Webhook handler 应返回 400。
func TestProperty_MalformedPayloadRejection(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		syncLock := &service.SyncLock{}
		eb := service.NewEventBuffer(syncLock, nil)
		handler := NewEmbyWebhookHandler(eb)

		// 生成各种畸形 payload
		malformType := rapid.IntRange(0, 4).Draw(t, "malformType")
		var data []byte

		switch malformType {
		case 0:
			// 非 JSON 数据
			data = []byte(rapid.SampledFrom([]string{
				"not json", "{invalid", "12345", "<xml>", "null",
			}).Draw(t, "invalidJSON"))
		case 1:
			// 缺少 Event 字段
			payload := map[string]interface{}{
				"Item": map[string]interface{}{
					"Id":   "item-1",
					"Type": "Movie",
				},
			}
			data, _ = json.Marshal(payload)
		case 2:
			// 缺少 Item.Id 字段
			payload := map[string]interface{}{
				"Event": "library.new",
				"Item": map[string]interface{}{
					"Type": "Movie",
				},
			}
			data, _ = json.Marshal(payload)
		case 3:
			// 缺少 Item.Type 字段
			payload := map[string]interface{}{
				"Event": "library.new",
				"Item": map[string]interface{}{
					"Id": "item-1",
				},
			}
			data, _ = json.Marshal(payload)
		case 4:
			// 空请求体
			data = []byte{}
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/webhook/emby", bytes.NewReader(data))
		c.Request.Header.Set("Content-Type", "application/json")

		handler.HandleEmbyWebhook(c)

		// 验证返回 400
		if w.Code != http.StatusBadRequest {
			t.Fatalf("畸形 payload (type=%d) 期望 HTTP 400, 实际 %d, body: %s",
				malformType, w.Code, w.Body.String())
		}

		// 验证没有事件入队
		if eb.PendingCount() != 0 {
			t.Fatalf("畸形 payload 不应有事件入队, 但 PendingCount=%d", eb.PendingCount())
		}
	})
}
