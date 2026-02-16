package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Seasons  []Season `json:"seasons"`
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

// doRequest 执行 HTTP 请求，处理 429 速率限制自动重试
func (c *Client) doRequest(path string) ([]byte, error) {
	url := fmt.Sprintf("%s%s?api_key=%s", c.BaseURL, path, c.APIKey)

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
	url := fmt.Sprintf("%s%s?api_key=%s", c.BaseURL, path, c.APIKey)

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
