package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AuthError 表示 TMDB API 认证失败（401）
type AuthError struct {
	StatusCode int
	Body       string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("TMDB API 认证失败 (状态码 %d): %s", e.StatusCode, e.Body)
}

// IsAuthError 判断错误是否为 TMDB 认证错误（401）
func IsAuthError(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}

// TVShowDetails TMDB 电视节目详情
type TVShowDetails struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Seasons []Season `json:"seasons"`
}

// Season TMDB 季信息
type Season struct {
	ID           int    `json:"id"`
	SeasonNumber int    `json:"season_number"`
	EpisodeCount int    `json:"episode_count"`
	Name         string `json:"name"`
}

// Client TMDB API 客户端
type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient 创建 TMDB API 客户端
func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: "https://api.themoviedb.org",
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// maxRetries 速率限制重试最大次数
const maxRetries = 3

// buildURL 根据 path 是否已包含查询参数，正确拼接 api_key
func (c *Client) buildURL(path string) string {
	separator := "?"
	if strings.Contains(path, "?") || strings.Contains(path, "&") {
		// path 中已有查询参数（以 & 开头），需要先加 ? 再拼接
		// 例如 /3/search/movie&query=xxx -> /3/search/movie?api_key=xxx&query=xxx
		parts := strings.SplitN(path, "&", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("%s%s?api_key=%s&%s", c.BaseURL, parts[0], c.APIKey, parts[1])
		}
		separator = "&"
	}
	return fmt.Sprintf("%s%s%sapi_key=%s", c.BaseURL, path, separator, c.APIKey)
}

// doRequest 执行 HTTP 请求，处理 429 速率限制自动重试
func (c *Client) doRequest(path string) ([]byte, error) {
	url := c.buildURL(path)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求失败: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}

		// 处理速率限制（429 Too Many Requests）
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt >= maxRetries {
				return nil, fmt.Errorf("TMDB API 速率限制，已重试 %d 次仍失败", maxRetries)
			}

			// 从 Retry-After 头获取等待时间，默认等待 2 秒
			waitSeconds := 2
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					waitSeconds = seconds
				}
			}
			time.Sleep(time.Duration(waitSeconds) * time.Second)
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &AuthError{StatusCode: resp.StatusCode, Body: string(body)}
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("TMDB API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("TMDB API 请求失败，超过最大重试次数")
}

// GetTVShowDetails 获取电视节目详情，包含季数信息
// 调用 GET /3/tv/{series_id}
func (c *Client) GetTVShowDetails(tmdbID int) (*TVShowDetails, error) {
	path := fmt.Sprintf("/3/tv/%d", tmdbID)

	body, err := c.doRequest(path)
	if err != nil {
		return nil, fmt.Errorf("获取电视节目详情失败 (TMDB ID=%d): %w", tmdbID, err)
	}

	var details TVShowDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, fmt.Errorf("解析电视节目详情失败 (TMDB ID=%d): %w", tmdbID, err)
	}

	return &details, nil
}

// doRequestWithContext 使用 context 执行 HTTP 请求，处理 429 速率限制自动重试
// 在重试等待期间检查 context 是否已取消
func (c *Client) doRequestWithContext(ctx context.Context, path string) ([]byte, error) {
	url := c.buildURL(path)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求失败: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}

		// 处理速率限制（429 Too Many Requests）
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt >= maxRetries {
				return nil, fmt.Errorf("TMDB API 速率限制，已重试 %d 次仍失败", maxRetries)
			}

			waitSeconds := 2
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if seconds, err := strconv.Atoi(retryAfter); err == nil {
					waitSeconds = seconds
				}
			}

			// 在重试等待期间检查 context 是否已取消
			timer := time.NewTimer(time.Duration(waitSeconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized {
			return nil, &AuthError{StatusCode: resp.StatusCode, Body: string(body)}
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("TMDB API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("TMDB API 请求失败，超过最大重试次数")
}

// GetTVShowDetailsWithContext 带 context 的电视节目详情获取
func (c *Client) GetTVShowDetailsWithContext(ctx context.Context, tmdbID int) (*TVShowDetails, error) {
	path := fmt.Sprintf("/3/tv/%d", tmdbID)

	body, err := c.doRequestWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("获取电视节目详情失败 (TMDB ID=%d): %w", tmdbID, err)
	}

	var details TVShowDetails
	if err := json.Unmarshal(body, &details); err != nil {
		return nil, fmt.Errorf("解析电视节目详情失败 (TMDB ID=%d): %w", tmdbID, err)
	}

	return &details, nil
}

// TmdbDetailResult TMDB 详情结果（电影和剧集通用）
type TmdbDetailResult struct {
	ID            int     `json:"id"`
	Title         string  `json:"title,omitempty"`
	Name          string  `json:"name,omitempty"`
	OriginalTitle string  `json:"original_title,omitempty"`
	OriginalName  string  `json:"original_name,omitempty"`
	Overview      string  `json:"overview"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	VoteAverage   float64 `json:"vote_average"`
	VoteCount     int     `json:"vote_count"`
	ReleaseDate   string  `json:"release_date,omitempty"`
	FirstAirDate  string  `json:"first_air_date,omitempty"`
}

// GetDetail 获取 TMDB 详情（电影或剧集）
func (c *Client) GetDetail(tmdbID int, mediaType, language string) (*TmdbDetailResult, error) {
	path := fmt.Sprintf("/3/%s/%d", mediaType, tmdbID)
	if language != "" {
		path += "&language=" + url.QueryEscape(language)
	}

	body, err := c.doRequest(path)
	if err != nil {
		return nil, fmt.Errorf("获取TMDB详情失败 (ID=%d): %w", tmdbID, err)
	}

	var result TmdbDetailResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析TMDB详情失败 (ID=%d): %w", tmdbID, err)
	}

	return &result, nil
}

// TmdbSearchResult TMDB 搜索结果项（统一格式，电影和剧集共用）
type TmdbSearchResult struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`          // 电影用 title，剧集用 name（映射后统一为 title）
	OriginalTitle string `json:"original_title"` // 电影用 original_title，剧集用 original_name
	ReleaseDate   string `json:"release_date"`   // 电影用 release_date，剧集用 first_air_date
	PosterPath    string `json:"poster_path"`
	Overview      string `json:"overview"`
}

// TmdbSearchRawMovie TMDB 电影搜索原始结果项
type TmdbSearchRawMovie struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	PosterPath    string `json:"poster_path"`
	Overview      string `json:"overview"`
}

// TmdbSearchRawTV TMDB 剧集搜索原始结果项
type TmdbSearchRawTV struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	PosterPath   string `json:"poster_path"`
	Overview     string `json:"overview"`
}

// TmdbSearchResponse TMDB 搜索 API 原始响应（电影）
type TmdbSearchMovieResponse struct {
	Page         int                  `json:"page"`
	Results      []TmdbSearchRawMovie `json:"results"`
	TotalResults int                  `json:"total_results"`
	TotalPages   int                  `json:"total_pages"`
}

// TmdbSearchTVResponse TMDB 搜索 API 原始响应（剧集）
type TmdbSearchTVResponse struct {
	Page         int               `json:"page"`
	Results      []TmdbSearchRawTV `json:"results"`
	TotalResults int               `json:"total_results"`
	TotalPages   int               `json:"total_pages"`
}

// BuildSearchURL 构建 TMDB 搜索 URL 路径（含查询参数，不含 BaseURL 和 api_key）
// mediaType: "movie" 或 "tv"
// 注意：返回的路径中额外查询参数以 & 开头，需要在已有 ?api_key=xxx 之后拼接
func BuildSearchURL(mediaType, query, language string) string {
	path := "/3/search/movie"
	if mediaType == "tv" {
		path = "/3/search/tv"
	}
	q := url.QueryEscape(query)
	result := fmt.Sprintf("%s&query=%s", path, q)
	if language != "" {
		result += "&language=" + url.QueryEscape(language)
	}
	return result
}

// MapMovieResults 将电影原始结果映射为统一的 TmdbSearchResult
func MapMovieResults(raw []TmdbSearchRawMovie) []TmdbSearchResult {
	results := make([]TmdbSearchResult, len(raw))
	for i, r := range raw {
		results[i] = TmdbSearchResult{
			ID:            r.ID,
			Title:         r.Title,
			OriginalTitle: r.OriginalTitle,
			ReleaseDate:   r.ReleaseDate,
			PosterPath:    r.PosterPath,
			Overview:      r.Overview,
		}
	}
	return results
}

// MapTVResults 将剧集原始结果映射为统一的 TmdbSearchResult
func MapTVResults(raw []TmdbSearchRawTV) []TmdbSearchResult {
	results := make([]TmdbSearchResult, len(raw))
	for i, r := range raw {
		results[i] = TmdbSearchResult{
			ID:            r.ID,
			Title:         r.Name,
			OriginalTitle: r.OriginalName,
			ReleaseDate:   r.FirstAirDate,
			PosterPath:    r.PosterPath,
			Overview:      r.Overview,
		}
	}
	return results
}

// SearchMovies 搜索电影
// 调用 TMDB GET /3/search/movie 接口
func (c *Client) SearchMovies(ctx context.Context, query, language string) ([]TmdbSearchResult, error) {
	searchPath := BuildSearchURL("movie", query, language)

	body, err := c.doRequestWithContext(ctx, searchPath)
	if err != nil {
		return nil, fmt.Errorf("搜索电影失败: %w", err)
	}

	var resp TmdbSearchMovieResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析电影搜索结果失败: %w", err)
	}

	return MapMovieResults(resp.Results), nil
}

// SearchTV 搜索剧集
// 调用 TMDB GET /3/search/tv 接口
func (c *Client) SearchTV(ctx context.Context, query, language string) ([]TmdbSearchResult, error) {
	searchPath := BuildSearchURL("tv", query, language)

	body, err := c.doRequestWithContext(ctx, searchPath)
	if err != nil {
		return nil, fmt.Errorf("搜索剧集失败: %w", err)
	}

	var resp TmdbSearchTVResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("解析剧集搜索结果失败: %w", err)
	}

	return MapTVResults(resp.Results), nil
}
