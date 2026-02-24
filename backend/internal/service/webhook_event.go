package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// EmbyWebhookEvent 解析后的 Emby Webhook 事件
type EmbyWebhookEvent struct {
	EventType         string `json:"event_type"`          // "library.new", "library.deleted", "item.add", "item.remove" 等
	ItemID            string `json:"item_id"`             // Emby 媒体条目 ID
	ItemType          string `json:"item_type"`           // "Movie", "Series", "Season", "Episode"
	ItemName          string `json:"item_name"`           // 媒体名称（集名/电影名）
	SeriesID          string `json:"series_id"`           // 所属 Series 的 Emby ID（Episode/Season 有值）
	SeriesName        string `json:"series_name"`         // 所属剧集名称（Episode/Season 有值）
	ParentIndexNumber int    `json:"parent_index_number"` // 季号（Episode 有值）
	IndexNumber       int    `json:"index_number"`        // 集号（仅 Episode 有值）
	Year              int    `json:"year"`                // 年份（电影/剧集）
}

// embyWebhookPayload Emby Webhook 原始 payload 结构
type embyWebhookPayload struct {
	Title string `json:"Title"`
	Event string `json:"Event"`
	Item  struct {
		ID                string `json:"Id"`
		Name              string `json:"Name"`
		Type              string `json:"Type"`
		SeriesID          string `json:"SeriesId"`
		SeriesName        string `json:"SeriesName"`
		ParentIndexNumber int    `json:"ParentIndexNumber"`
		IndexNumber       int    `json:"IndexNumber"`
		ProductionYear    int    `json:"ProductionYear"`
		Path              string `json:"Path"`
	} `json:"Item"`
	Server struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	} `json:"Server"`
}

// addEventTypes 新增/更新类型的事件
var addEventTypes = map[string]bool{
	"library.new":    true,
	"item.add":       true,
	"item.update":    true,
	"library.update": true,
}

// deleteEventTypes 删除类型的事件
var deleteEventTypes = map[string]bool{
	"library.deleted": true,
	"item.remove":     true,
	"item.delete":     true,
	"library.remove":  true,
}

// ParseEmbyWebhookPayload 从 Emby Webhook JSON payload 解析事件
func ParseEmbyWebhookPayload(payload []byte) (*EmbyWebhookEvent, error) {
	var p embyWebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}

	if p.Event == "" {
		return nil, fmt.Errorf("缺少 Event 字段")
	}
	if p.Item.ID == "" {
		return nil, fmt.Errorf("缺少 Item.Id 字段")
	}
	if p.Item.Type == "" {
		return nil, fmt.Errorf("缺少 Item.Type 字段")
	}

	return &EmbyWebhookEvent{
		EventType:         p.Event,
		ItemID:            p.Item.ID,
		ItemType:          p.Item.Type,
		ItemName:          p.Item.Name,
		SeriesID:          p.Item.SeriesID,
		SeriesName:        p.Item.SeriesName,
		ParentIndexNumber: p.Item.ParentIndexNumber,
		IndexNumber:       p.Item.IndexNumber,
		Year:              p.Item.ProductionYear,
	}, nil
}

// FormatEmbyWebhookEvent 将事件格式化为人类可读的日志字符串
func FormatEmbyWebhookEvent(event *EmbyWebhookEvent) string {
	return fmt.Sprintf("[%s] %s (ID=%s, Type=%s)", event.EventType, event.ItemName, event.ItemID, event.ItemType)
}

// IsRelevantItemType 判断 item 类型是否需要处理（Movie/Series/Season/Episode）
func IsRelevantItemType(itemType string) bool {
	switch itemType {
	case "Movie", "Series", "Season", "Episode":
		return true
	default:
		return false
	}
}

// IsAddEvent 判断事件是否为新增/更新类型
func IsAddEvent(eventType string) bool {
	return addEventTypes[strings.ToLower(eventType)] || addEventTypes[eventType]
}

// IsDeleteEvent 判断事件是否为删除类型
func IsDeleteEvent(eventType string) bool {
	return deleteEventTypes[strings.ToLower(eventType)] || deleteEventTypes[eventType]
}
