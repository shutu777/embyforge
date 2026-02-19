package emby

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
			Timeout: 30 * time.Second,
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


// DeleteVersion 删除 Emby 媒体条目的某个版本文件
// Emby API: POST /emby/Items/{itemId}/DeleteVersion
// 适用于删除重复媒体中体积较小的版本
func (c *Client) DeleteVersion(ctx context.Context, itemID string) error {
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

// DeleteItem 删除 Emby 媒体条目
// Emby API: POST /emby/Items/Delete?Ids={itemId}
// 适用于删除刮削异常等需要完全移除的条目
func (c *Client) DeleteItem(ctx context.Context, itemID string) error {
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
