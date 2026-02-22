package emby

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// ServerInfo Emby 服务器信息
type ServerInfo struct {
	ServerName string `json:"ServerName"`
	Version    string `json:"Version"`
	ID         string `json:"Id"`
}

// MediaItem Emby 媒体条目
type MediaItem struct {
	ID                  string            `json:"Id"`
	Name                string            `json:"Name"`
	Type                string            `json:"Type"`
	ImageTags           map[string]string `json:"ImageTags"`
	Path                string            `json:"Path"`
	ProviderIds         map[string]string `json:"ProviderIds"`
	SeriesID            string            `json:"SeriesId"`
	SeriesName          string            `json:"SeriesName"`
	FileSize            int64             `json:"Size"`
	IndexNumber         int               `json:"IndexNumber"`         // 集号
	ParentIndexNumber   int               `json:"ParentIndexNumber"`   // 季号
	ChildCount          int               `json:"ChildCount"`          // 子条目数量（季的集数）
	RecursiveItemCount  int               `json:"RecursiveItemCount"`  // 递归子条目数量
	ProductionYear      int               `json:"ProductionYear"`      // 制作年份
}

// MediaItemsResponse Emby Items 接口响应
type MediaItemsResponse struct {
	Items            []MediaItem `json:"Items"`
	TotalRecordCount int         `json:"TotalRecordCount"`
}

// EffectiveChildCount 获取有效的子条目数量
// 优先使用 ChildCount，如果为 0 则 fallback 到 RecursiveItemCount
func (m *MediaItem) EffectiveChildCount() int {
	if m.ChildCount > 0 {
		return m.ChildCount
	}
	return m.RecursiveItemCount
}

// Client Emby API 客户端
type Client struct {
	Host       string
	Port       int
	APIKey     string
	HTTPClient *http.Client
}

// NewClient 创建 Emby API 客户端
func NewClient(host string, port int, apiKey string) *Client {
	return &Client{
		Host:   host,
		Port:   port,
		APIKey: apiKey,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

// baseURL 返回 Emby 服务器基础 URL
func (c *Client) baseURL() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// doRequest 执行 HTTP 请求并返回响应体
func (c *Client) doRequest(path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.baseURL(), path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 使用 API Key 认证
	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Emby API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// TestConnection 测试与 Emby 服务器的连接
func (c *Client) TestConnection() (*ServerInfo, error) {
	return c.testConnectionWithURL(c.baseURL() + "/emby/System/Info")
}

// testConnectionWithURL 使用指定 URL 测试连接（便于测试）
func (c *Client) testConnectionWithURL(url string) (*ServerInfo, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Emby API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	var info ServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("解析服务器信息失败: %w", err)
	}

	return &info, nil
}

// PageSize 每页获取的媒体条目数量（大页面减少 HTTP 请求次数）
const PageSize = 2000

// GetMediaItems 分页获取媒体条目，使用回调函数逐页处理，避免全量加载到内存
// itemType 可选，如 "Movie"、"Series"、"Episode"，为空则获取所有类型
func (c *Client) GetMediaItems(itemType string, callback func(items []MediaItem) error) error {
	startIndex := 0

	for {
		path := fmt.Sprintf("/emby/Items?StartIndex=%d&Limit=%d&Recursive=true&Fields=Path,ProviderIds,ImageTags,ParentIndexNumber,SeriesId,SeriesName,MediaSources",
			startIndex, PageSize)

		if itemType != "" {
			path += "&IncludeItemTypes=" + itemType
		}

		body, err := c.doRequest(path)
		if err != nil {
			return fmt.Errorf("获取媒体条目失败 (StartIndex=%d): %w", startIndex, err)
		}

		var resp MediaItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("解析媒体条目响应失败: %w", err)
		}

		// 没有更多数据时退出
		if len(resp.Items) == 0 {
			break
		}

		// 通过回调函数处理当前页数据
		if err := callback(resp.Items); err != nil {
			return fmt.Errorf("处理媒体条目回调失败: %w", err)
		}

		startIndex += len(resp.Items)

		// 已获取所有数据时退出
		if startIndex >= resp.TotalRecordCount {
			break
		}
	}

	return nil
}

// GetChildItems 获取指定父条目的子条目（如获取某个 Series 的所有 Season）
func (c *Client) GetChildItems(parentID string, itemType string) ([]MediaItem, error) {
	path := fmt.Sprintf("/emby/Items?ParentId=%s&Recursive=false&Fields=Path,ProviderIds,ChildCount,RecursiveItemCount",
		parentID)

	if itemType != "" {
		path += "&IncludeItemTypes=" + itemType
	}

	body, err := c.doRequest(path)
	if err != nil {
		return nil, fmt.Errorf("获取子条目失败 (ParentID=%s): %w", parentID, err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析子条目响应失败: %w", err)
	}

	return resp.Items, nil
}

// DoRequestWithContext 使用 context 执行 HTTP 请求并返回响应体（导出供外部使用）
func (c *Client) DoRequestWithContext(ctx context.Context, path string) ([]byte, error) {
	return c.doRequestWithContext(ctx, path)
}

// doRequestWithContext 使用 context 执行 HTTP 请求并返回响应体
func (c *Client) doRequestWithContext(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s", c.baseURL(), path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Emby API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// GetMediaItemsWithContext 带 context 的分页获取媒体条目
func (c *Client) GetMediaItemsWithContext(ctx context.Context, itemType string, callback func(items []MediaItem) error) error {
	startIndex := 0

	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		path := fmt.Sprintf("/emby/Items?StartIndex=%d&Limit=%d&Recursive=true&Fields=Path,ProviderIds,ImageTags,ParentIndexNumber,SeriesId,SeriesName,MediaSources",
			startIndex, PageSize)

		if itemType != "" {
			path += "&IncludeItemTypes=" + itemType
		}

		body, err := c.doRequestWithContext(ctx, path)
		if err != nil {
			return fmt.Errorf("获取媒体条目失败 (StartIndex=%d): %w", startIndex, err)
		}

		var resp MediaItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("解析媒体条目响应失败: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		if err := callback(resp.Items); err != nil {
			return fmt.Errorf("处理媒体条目回调失败: %w", err)
		}

		startIndex += len(resp.Items)

		if startIndex >= resp.TotalRecordCount {
			break
		}
	}

	return nil
}

// SyncItemTypes 同步时只拉取的媒体类型：电影、剧集、单集
const SyncItemTypes = "Movie,Series,Episode"

// GetTotalItemCount 获取媒体总条目数（使用 Limit=0 只返回 TotalRecordCount）
// 只统计 Movie、Series、Episode 三种类型
func (c *Client) GetTotalItemCount(ctx context.Context) (int, error) {
	path := "/emby/Items?Limit=0&Recursive=true&IncludeItemTypes=" + SyncItemTypes

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("获取媒体总数失败: %w", err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("解析媒体总数响应失败: %w", err)
	}

	return resp.TotalRecordCount, nil
}

// GetChildItemsWithContext 带 context 的子条目获取
func (c *Client) GetChildItemsWithContext(ctx context.Context, parentID string, itemType string) ([]MediaItem, error) {
	path := fmt.Sprintf("/emby/Items?ParentId=%s&Recursive=false&Fields=Path,ProviderIds,ChildCount,RecursiveItemCount",
		parentID)

	if itemType != "" {
		path += "&IncludeItemTypes=" + itemType
	}

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("获取子条目失败 (ParentID=%s): %w", parentID, err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析子条目响应失败: %w", err)
	}

	return resp.Items, nil
}

// GetChildItemCount 获取指定父条目下子条目的数量（使用 Limit=0 只返回 TotalRecordCount）
// 用于获取 Season 下的 Episode 数量，因为 Emby 的 Season 项目不返回 ChildCount
func (c *Client) GetChildItemCount(ctx context.Context, parentID string, itemType string) (int, error) {
	path := fmt.Sprintf("/emby/Items?ParentId=%s&Recursive=false&Limit=0", parentID)

	if itemType != "" {
		path += "&IncludeItemTypes=" + itemType
	}

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("获取子条目数量失败 (ParentID=%s): %w", parentID, err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("解析子条目数量响应失败: %w", err)
	}

	return resp.TotalRecordCount, nil
}


// GetItemByID 通过 Emby Item ID 获取单个媒体条目
func (c *Client) GetItemByID(ctx context.Context, itemID string) ([]MediaItem, error) {
	path := fmt.Sprintf("/emby/Items?Ids=%s&Fields=Path,ProviderIds,ImageTags,ParentIndexNumber,SeriesId,SeriesName,MediaSources", itemID)

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("获取条目失败 (ID=%s): %w", itemID, err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析条目响应失败: %w", err)
	}

	return resp.Items, nil
}

// GetMediaItemsModifiedSince 获取指定时间之后修改的媒体条目（增量同步用）
// 使用 Emby 的 MinDateLastSaved 参数过滤
func (c *Client) GetMediaItemsModifiedSince(ctx context.Context, since time.Time, itemType string, callback func(items []MediaItem) error) error {
	startIndex := 0
	sinceStr := since.UTC().Format("2006-01-02T15:04:05.0000000Z")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		path := fmt.Sprintf("/emby/Items?StartIndex=%d&Limit=%d&Recursive=true&Fields=Path,ProviderIds,ImageTags,ParentIndexNumber,SeriesId,SeriesName,MediaSources&MinDateLastSaved=%s",
			startIndex, PageSize, sinceStr)

		if itemType != "" {
			path += "&IncludeItemTypes=" + itemType
		}

		body, err := c.doRequestWithContext(ctx, path)
		if err != nil {
			return fmt.Errorf("获取增量媒体条目失败 (StartIndex=%d): %w", startIndex, err)
		}

		var resp MediaItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("解析增量媒体条目响应失败: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		if err := callback(resp.Items); err != nil {
			return fmt.Errorf("处理增量媒体条目回调失败: %w", err)
		}

		startIndex += len(resp.Items)

		if startIndex >= resp.TotalRecordCount {
			break
		}
	}

	return nil
}

// GetAllItemIDs 获取 Emby 中所有媒体条目的 ID 列表（用于增量同步时检测已删除的条目）
// 使用大页面分页获取，只取 ID 字段以减少数据量
func (c *Client) GetAllItemIDs(ctx context.Context, itemType string) (map[string]bool, int, error) {
	ids := make(map[string]bool, 300000)
	startIndex := 0

	for {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		default:
		}

		// 只请求最少的字段，减少传输量
		path := fmt.Sprintf("/emby/Items?StartIndex=%d&Limit=%d&Recursive=true&Fields=&EnableImages=false",
			startIndex, PageSize)

		if itemType != "" {
			path += "&IncludeItemTypes=" + itemType
		}

		body, err := c.doRequestWithContext(ctx, path)
		if err != nil {
			return nil, 0, fmt.Errorf("获取 Emby ID 列表失败 (StartIndex=%d): %w", startIndex, err)
		}

		var resp MediaItemsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, 0, fmt.Errorf("解析 Emby ID 列表响应失败: %w", err)
		}

		if len(resp.Items) == 0 {
			break
		}

		for _, item := range resp.Items {
			ids[item.ID] = true
		}

		startIndex += len(resp.Items)

		if startIndex >= resp.TotalRecordCount {
			break
		}
	}

	return ids, len(ids), nil
}

// DeleteVersion 删除 Emby 媒体条目的某个版本文件（带 fallback 兼容）
// 主端点: POST /emby/Items/{itemId}/DeleteVersion
// 备用端点: DELETE /emby/Items/{itemId}
// 主端点失败时自动尝试备用端点，兼容不同版本的 Emby 服务器
func (c *Client) DeleteVersion(ctx context.Context, itemID string) error {
	// 尝试主端点
	err := c.deleteVersionPrimary(ctx, itemID)
	if err == nil {
		return nil
	}
	log.Printf("主删除版本端点失败，尝试备用端点: %v", err)
	// 尝试备用端点
	return c.deleteItemFallback(ctx, itemID)
}

// deleteVersionPrimary 使用主端点删除版本
// POST /emby/Items/{itemId}/DeleteVersion
func (c *Client) deleteVersionPrimary(ctx context.Context, itemID string) error {
	url := fmt.Sprintf("%s/emby/Items/%s/DeleteVersion", c.baseURL(), itemID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建删除版本请求失败: %w", err)
	}

	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除版本请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Emby 删除版本失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteItem 删除 Emby 媒体条目（带 fallback 兼容）
// 主端点: POST /emby/Items/Delete?Ids={itemId}
// 备用端点: DELETE /emby/Items/{itemId}
// 主端点失败时自动尝试备用端点，兼容不同版本的 Emby 服务器
func (c *Client) DeleteItem(ctx context.Context, itemID string) error {
	// 尝试主端点
	err := c.deleteItemPrimary(ctx, itemID)
	if err == nil {
		return nil
	}
	log.Printf("主删除端点失败，尝试备用端点: %v", err)
	// 尝试备用端点
	return c.deleteItemFallback(ctx, itemID)
}

// deleteItemPrimary 使用主端点删除条目
// POST /emby/Items/Delete?Ids={itemId}
func (c *Client) deleteItemPrimary(ctx context.Context, itemID string) error {
	url := fmt.Sprintf("%s/emby/Items/Delete?Ids=%s", c.baseURL(), itemID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建删除条目请求失败: %w", err)
	}

	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("删除条目请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Emby 删除条目失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// deleteItemFallback 使用备用端点删除条目
// DELETE /emby/Items/{itemId}
func (c *Client) deleteItemFallback(ctx context.Context, itemID string) error {
	url := fmt.Sprintf("%s/emby/Items/%s", c.baseURL(), itemID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("创建备用删除条目请求失败: %w", err)
	}

	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("备用删除条目请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Emby 备用删除条目失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// RemoteImageInfo 远程图片信息
type RemoteImageInfo struct {
	ProviderName string  `json:"ProviderName"`
	URL          string  `json:"Url"`
	ThumbnailURL string  `json:"ThumbnailUrl"`
	Height       int     `json:"Height"`
	Width        int     `json:"Width"`
	Language     string  `json:"Language"`
	Type         string  `json:"Type"` // Primary, Backdrop, etc.
	RatingType   string  `json:"RatingType"`
	CommunityRating float64 `json:"CommunityRating"`
}

// RemoteImagesResponse 远程图片列表响应
type RemoteImagesResponse struct {
	Images       []RemoteImageInfo `json:"Images"`
	TotalRecordCount int           `json:"TotalRecordCount"`
	Providers    []string          `json:"Providers"`
}

// GetRemoteImages 获取媒体项的远程图片列表
// Emby API: GET /emby/Items/{itemId}/RemoteImages
func (c *Client) GetRemoteImages(ctx context.Context, itemID string, imageType string) (*RemoteImagesResponse, error) {
	path := fmt.Sprintf("/emby/Items/%s/RemoteImages?Type=%s", itemID, imageType)

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("获取远程图片失败: %w", err)
	}

	var resp RemoteImagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析远程图片响应失败: %w", err)
	}

	return &resp, nil
}

// DownloadRemoteImage 下载并设置远程图片为媒体项的封面
// Emby API: POST /emby/Items/{itemId}/RemoteImages/Download
func (c *Client) DownloadRemoteImage(ctx context.Context, itemID string, imageType string, imageURL string, providerName string) error {
	url := fmt.Sprintf("%s/emby/Items/%s/RemoteImages/Download?Type=%s&ImageUrl=%s&ProviderName=%s",
		c.baseURL(), itemID, imageType, imageURL, providerName)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("创建下载图片请求失败: %w", err)
	}

	req.Header.Set("X-Emby-Token", c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("下载图片请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Emby 下载图片失败，状态码 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SearchItems 通过关键字搜索 Emby 媒体条目
// 只返回 Movie 和 Series 类型，支持 limit 参数控制返回数量
func (c *Client) SearchItems(ctx context.Context, keyword string, limit int) ([]MediaItem, error) {
	if limit <= 0 {
		limit = 50
	}

	path := fmt.Sprintf("/emby/Items?SearchTerm=%s&IncludeItemTypes=Movie,Series&Recursive=true&Limit=%d&Fields=Path,ProviderIds,ChildCount,RecursiveItemCount,ProductionYear",
		url.QueryEscape(keyword), limit)

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("搜索媒体条目失败: %w", err)
	}

	var resp MediaItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}

	return resp.Items, nil
}
