package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DashboardHandler 仪表盘处理器
type DashboardHandler struct {
	DB *gorm.DB
}

// NewDashboardHandler 创建仪表盘处理器
func NewDashboardHandler(db *gorm.DB) *DashboardHandler {
	return &DashboardHandler{DB: db}
}

// RecentMedia 最近入库媒体
type RecentMedia struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
}

// DashboardData 仪表盘数据
type DashboardData struct {
	EmbyConnected  bool   `json:"emby_connected"`
	EmbyServerName string `json:"emby_server_name"`
	EmbyVersion    string `json:"emby_version"`
	EmbyError      string `json:"emby_error,omitempty"`

	MovieCount   int `json:"movie_count"`
	SeriesCount  int `json:"series_count"`
	EpisodeCount int `json:"episode_count"`

	ScrapeAnomalyCount  int64 `json:"scrape_anomaly_count"`
	DuplicateGroupCount int64 `json:"duplicate_group_count"`
	EpisodeAnomalyCount int64 `json:"episode_anomaly_count"`

	RecentItems []RecentMedia `json:"recent_items"`

	// 图表数据（最近7天）
	DailyMediaStats   []DailyStat `json:"daily_media_stats"`
	DailyAnomalyStats []DailyStat `json:"daily_anomaly_stats"`
}

// DailyStat 每日统计
type DailyStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// embyItemsResp Emby Items 接口的精简响应
type embyItemsResp struct {
	Items            []embyItemBrief `json:"Items"`
	TotalRecordCount int             `json:"TotalRecordCount"`
}

type embyItemBrief struct {
	ID        string            `json:"Id"`
	Name      string            `json:"Name"`
	Type      string            `json:"Type"`
	ImageTags map[string]string `json:"ImageTags"`
}

// GetDashboard GET /api/dashboard
func (h *DashboardHandler) GetDashboard(c *gin.Context) {
	data := DashboardData{}

	// 异常统计（数据库查询，很快）
	h.DB.Model(&model.ScrapeAnomaly{}).Count(&data.ScrapeAnomalyCount)
	h.DB.Model(&model.DuplicateMedia{}).Distinct("group_key").Count(&data.DuplicateGroupCount)
	h.DB.Model(&model.EpisodeMappingAnomaly{}).Count(&data.EpisodeAnomalyCount)

	// 获取 Emby 配置
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		data.EmbyConnected = false
		data.EmbyError = "未配置 Emby 服务器"
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}

	client := emby.NewClient(config.Host, config.Port, config.APIKey)
	baseURL := fmt.Sprintf("%s:%d", config.Host, config.Port)

	// 测试连接
	info, err := client.TestConnection()
	if err != nil {
		data.EmbyConnected = false
		data.EmbyError = "Emby 连接失败"
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}
	data.EmbyConnected = true
	data.EmbyServerName = info.ServerName
	data.EmbyVersion = info.Version

	// 媒体数量统计
	data.MovieCount = h.fetchCount(baseURL, config.APIKey, "Movie")
	data.SeriesCount = h.fetchCount(baseURL, config.APIKey, "Series")
	data.EpisodeCount = h.fetchCount(baseURL, config.APIKey, "Episode")

	// 最近入库
	data.RecentItems = h.fetchRecentItems(baseURL, config.APIKey)

	// 图表数据：最近7天每日入库统计
	data.DailyMediaStats = h.fetchDailyMediaStats(baseURL, config.APIKey)

	// 图表数据：最近7天每日异常统计（从数据库）
	data.DailyAnomalyStats = h.fetchDailyAnomalyStats()

	c.JSON(http.StatusOK, gin.H{"data": data})
}

// fetchCount 获取指定类型的媒体总数
func (h *DashboardHandler) fetchCount(baseURL, apiKey, itemType string) int {
	url := fmt.Sprintf("%s/emby/Items?IncludeItemTypes=%s&Recursive=true&Limit=0", baseURL, itemType)
	body, err := embyGet(url, apiKey)
	if err != nil {
		log.Printf("获取 %s 数量失败: %v", itemType, err)
		return 0
	}
	var resp embyItemsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0
	}
	return resp.TotalRecordCount
}

// fetchRecentItems 获取最近入库的媒体
func (h *DashboardHandler) fetchRecentItems(baseURL, apiKey string) []RecentMedia {
	url := fmt.Sprintf("%s/emby/Items?SortBy=DateCreated&SortOrder=Descending&Recursive=true&Limit=5&IncludeItemTypes=Movie,Series&Fields=PrimaryImageTag", baseURL)
	body, err := embyGet(url, apiKey)
	if err != nil {
		log.Printf("获取最近入库失败: %v", err)
		return nil
	}
	var resp embyItemsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}

	items := make([]RecentMedia, 0, len(resp.Items))
	for _, item := range resp.Items {
		imgURL := ""
		if _, ok := item.ImageTags["Primary"]; ok {
			imgURL = fmt.Sprintf("%s/emby/Items/%s/Images/Primary?maxHeight=160&api_key=%s", baseURL, item.ID, apiKey)
		}
		typeName := item.Type
		if typeName == "Movie" {
			typeName = "电影"
		} else if typeName == "Series" {
			typeName = "剧集"
		}
		items = append(items, RecentMedia{
			ID:       item.ID,
			Name:     item.Name,
			Type:     typeName,
			ImageURL: imgURL,
		})
	}
	return items
}

// embyGet 对 Emby 发起 GET 请求
func embyGet(url, apiKey string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Emby-Token", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Emby API 返回 %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// fetchDailyMediaStats 获取最近7天每日入库数量（通过 Emby API）
func (h *DashboardHandler) fetchDailyMediaStats(baseURL, apiKey string) []DailyStat {
	now := time.Now()
	stats := make([]DailyStat, 7)

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		dayEnd := dayStart.AddDate(0, 0, 1)

		dateLabel := dayStart.Format("01/02")

		// 查询当天入库的电影+剧集数量
		url := fmt.Sprintf("%s/emby/Items?Recursive=true&Limit=0&IncludeItemTypes=Movie,Series&MinDateCreated=%s&MaxDateCreated=%s",
			baseURL,
			dayStart.Format("2006-01-02T15:04:05"),
			dayEnd.Format("2006-01-02T15:04:05"),
		)

		body, err := embyGet(url, apiKey)
		count := 0
		if err == nil {
			var resp embyItemsResp
			if json.Unmarshal(body, &resp) == nil {
				count = resp.TotalRecordCount
			}
		}

		stats[6-i] = DailyStat{Date: dateLabel, Count: count}
	}

	return stats
}

// fetchDailyAnomalyStats 获取最近7天每日异常数量（从数据库）
func (h *DashboardHandler) fetchDailyAnomalyStats() []DailyStat {
	now := time.Now()
	stats := make([]DailyStat, 7)

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		dayEnd := dayStart.AddDate(0, 0, 1)

		dateLabel := dayStart.Format("01/02")

		var scrapeCount, dupCount, epCount int64
		h.DB.Model(&model.ScrapeAnomaly{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&scrapeCount)
		h.DB.Model(&model.DuplicateMedia{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&dupCount)
		h.DB.Model(&model.EpisodeMappingAnomaly{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).Count(&epCount)

		stats[6-i] = DailyStat{Date: dateLabel, Count: int(scrapeCount + dupCount + epCount)}
	}

	return stats
}
