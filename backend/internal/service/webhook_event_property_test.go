package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 13: Webhook 事件解析正确性
// Validates: Requirements 8.1
//
// 对于任意 valid Emby Webhook JSON payload，解析器应正确提取 event type、item ID、item type 和 item name。
func TestProperty_WebhookEventParseCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机有效 payload 字段
		eventType := rapid.SampledFrom([]string{
			"library.new", "library.deleted", "item.add", "item.remove",
			"item.update", "library.update", "item.delete", "library.remove",
		}).Draw(t, "eventType")
		itemID := fmt.Sprintf("item-%d", rapid.IntRange(1, 99999).Draw(t, "itemID"))
		itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode", "Season", "Audio"}).Draw(t, "itemType")
		itemName := rapid.SampledFrom([]string{
			"测试电影", "Test Movie", "剧集名称", "Episode Title", "",
		}).Draw(t, "itemName")

		// 构建 JSON payload
		payload := map[string]interface{}{
			"Title": "Emby Server Notification",
			"Event": eventType,
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
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("JSON 序列化失败: %v", err)
		}

		event, err := ParseEmbyWebhookPayload(data)
		if err != nil {
			t.Fatalf("解析有效 payload 失败: %v", err)
		}

		// 验证字段提取正确
		if event.EventType != eventType {
			t.Fatalf("EventType = %q, 期望 %q", event.EventType, eventType)
		}
		if event.ItemID != itemID {
			t.Fatalf("ItemID = %q, 期望 %q", event.ItemID, itemID)
		}
		if event.ItemType != itemType {
			t.Fatalf("ItemType = %q, 期望 %q", event.ItemType, itemType)
		}
		if event.ItemName != itemName {
			t.Fatalf("ItemName = %q, 期望 %q", event.ItemName, itemName)
		}
	})
}

// Feature: emby-webhook-sync, Property 14: 非相关 Item 类型过滤
// Validates: Requirements 8.3
//
// 对于任意非 "Movie"/"Series"/"Episode" 的类型字符串，IsRelevantItemType 应返回 false。
func TestProperty_IrrelevantItemTypeFilter(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成非相关类型
		irrelevantType := rapid.SampledFrom([]string{
			"Season", "Audio", "MusicAlbum", "MusicArtist", "Photo",
			"Folder", "BoxSet", "Playlist", "Person", "Studio",
			"Genre", "MusicGenre", "Book", "Trailer", "LiveTvChannel",
			"", "movie", "series", "episode", "MOVIE", "SERIES",
		}).Draw(t, "irrelevantType")

		result := IsRelevantItemType(irrelevantType)
		if result {
			t.Fatalf("IsRelevantItemType(%q) = true, 期望 false（非相关类型）", irrelevantType)
		}
	})

	// 同时验证相关类型返回 true
	for _, validType := range []string{"Movie", "Series", "Episode"} {
		if !IsRelevantItemType(validType) {
			t.Fatalf("IsRelevantItemType(%q) = false, 期望 true", validType)
		}
	}
}

// Feature: emby-webhook-sync, Property 15: 事件格式化包含必要字段
// Validates: Requirements 8.4
//
// 对于任意 valid EmbyWebhookEvent，格式化后的日志字符串应包含 event type、item ID、item type 和 item name。
func TestProperty_EventFormatContainsRequiredFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := &EmbyWebhookEvent{
			EventType: rapid.SampledFrom([]string{
				"library.new", "library.deleted", "item.add", "item.remove",
			}).Draw(t, "eventType"),
			ItemID:   fmt.Sprintf("id-%d", rapid.IntRange(1, 99999).Draw(t, "itemID")),
			ItemType: rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, "itemType"),
			ItemName: fmt.Sprintf("Media_%d", rapid.IntRange(1, 999).Draw(t, "itemName")),
		}

		formatted := FormatEmbyWebhookEvent(event)

		// 验证格式化字符串包含所有必要字段
		if !strings.Contains(formatted, event.EventType) {
			t.Fatalf("格式化字符串 %q 不包含 EventType %q", formatted, event.EventType)
		}
		if !strings.Contains(formatted, event.ItemID) {
			t.Fatalf("格式化字符串 %q 不包含 ItemID %q", formatted, event.ItemID)
		}
		if !strings.Contains(formatted, event.ItemType) {
			t.Fatalf("格式化字符串 %q 不包含 ItemType %q", formatted, event.ItemType)
		}
		if !strings.Contains(formatted, event.ItemName) {
			t.Fatalf("格式化字符串 %q 不包含 ItemName %q", formatted, event.ItemName)
		}
	})
}


// Feature: emby-webhook-sync, Property 16: 解析-格式化 Round Trip
// Validates: Requirements 8.5
//
// 对于任意 valid EmbyWebhookEvent，将其 marshal 为 JSON payload 再 parse 回来，
// 应产生等价的事件对象。
func TestProperty_ParseFormatRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机事件
		eventType := rapid.SampledFrom([]string{
			"library.new", "library.deleted", "item.add", "item.remove",
			"item.update", "library.update",
		}).Draw(t, "eventType")
		itemID := fmt.Sprintf("item-%d", rapid.IntRange(1, 99999).Draw(t, "itemID"))
		itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, "itemType")
		itemName := fmt.Sprintf("Media_%d", rapid.IntRange(1, 999).Draw(t, "itemName"))

		// 构建 Emby Webhook payload JSON
		payload := map[string]interface{}{
			"Title": "Emby Server Notification",
			"Event": eventType,
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
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("JSON 序列化失败: %v", err)
		}

		// Parse
		parsed, err := ParseEmbyWebhookPayload(data)
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}

		// 验证 round trip：parse 后的字段应与原始值一致
		if parsed.EventType != eventType {
			t.Fatalf("Round trip EventType: %q != %q", parsed.EventType, eventType)
		}
		if parsed.ItemID != itemID {
			t.Fatalf("Round trip ItemID: %q != %q", parsed.ItemID, itemID)
		}
		if parsed.ItemType != itemType {
			t.Fatalf("Round trip ItemType: %q != %q", parsed.ItemType, itemType)
		}
		if parsed.ItemName != itemName {
			t.Fatalf("Round trip ItemName: %q != %q", parsed.ItemName, itemName)
		}
	})
}
