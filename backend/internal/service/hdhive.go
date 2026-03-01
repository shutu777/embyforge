package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ==================== HDHive Next-Action 配置 ====================
// 当 HDHive 前端更新后，可能需要更新以下值
// 获取方式：在 HDHive 网站 F12 Network 中查看对应请求的 Next-Action 请求头
const (
	// 登录接口的 Next-Action
	HDHiveActionLogin = "60a3fc399468c700be8a3ecc69cd86c911899c9c85"
	// 搜索加密接口的 Next-Action
	HDHiveActionEncrypt = "40455ffb676252e2a2c6890de98d69faba06f545a3"
	// 搜索结果解密接口的 Next-Action
	HDHiveActionDecrypt = "402e33395765de5d6b83880fe18ab0830ad2201e2e"
	// 解锁资源接口的 Next-Action
	HDHiveActionUnlock = "40dbca7ab6f555dbd98c40945c8b970185c58e16d3"
)

// ==================================================================

// HDHiveService 封装 HDHive API 调用
type HDHiveService struct {
	BaseURL      string
	LoginClient  *http.Client // 登录专用，不跟随重定向
	BrowseClient *http.Client // 浏览/请求专用，跟随重定向
}

// NewHDHiveService 创建 HDHive 服务实例
func NewHDHiveService() *HDHiveService {
	return &HDHiveService{
		BaseURL: "https://hdhive.com",
		LoginClient: &http.Client{
			Timeout: 30 * time.Second,
			// 登录时不跟随重定向，以便捕获 Set-Cookie
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		BrowseClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HDHiveLoginResult 登录结果
type HDHiveLoginResult struct {
	Token  string `json:"token"`
	Cookie string `json:"cookie"`
}

// HDHiveSearchResult TMDB 搜索结果
type HDHiveSearchResult struct {
	Page         int                    `json:"page"`
	Results      []HDHiveTMDBSearchItem `json:"results"`
	TotalPages   int                    `json:"total_pages"`
	TotalResults int                    `json:"total_results"`
}

// HDHiveTMDBSearchItem 单个 TMDB 搜索结果项
type HDHiveTMDBSearchItem struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	MediaType        string   `json:"media_type"`
	Overview         string   `json:"overview"`
	PosterPath       string   `json:"poster_path"`
	BackdropPath     string   `json:"backdrop_path"`
	VoteAverage      float64  `json:"vote_average"`
	VoteCount        int      `json:"vote_count"`
	Popularity       float64  `json:"popularity"`
	OriginalName     string   `json:"original_name"`
	OriginalTitle    string   `json:"original_title"`
	OriginalLanguage string   `json:"original_language"`
	FirstAirDate     string   `json:"first_air_date"`
	ReleaseDate      string   `json:"release_date"`
	GenreIDs         []int    `json:"genre_ids"`
	OriginCountry    []string `json:"origin_country"`
	Adult            bool     `json:"adult"`
}

// HDHive115Resource 115 网盘资源
type HDHive115Resource struct {
	ID            string   `json:"id"`
	UserName      string   `json:"user_name"`
	UserAvatar    string   `json:"user_avatar"`
	Points        int      `json:"points"`
	Title         string   `json:"title"`
	Remark        string   `json:"remark"`
	Size          string   `json:"size"`
	Tags          []string `json:"tags"`
	IsOfficial    bool     `json:"is_official"`
	UnlockedCount int      `json:"unlocked_count"`
	MediaType     string   `json:"media_type"`
	TmdbID        int      `json:"tmdb_id"`
}

// HDHiveUnlockResult 解锁结果
type HDHiveUnlockResult struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Message string `json:"message"`
}

// Login 登录 HDHive，返回 token 和 cookie
func (s *HDHiveService) Login(username, password string) (*HDHiveLoginResult, error) {
	// 构造 Next.js Server Action 请求体
	body := fmt.Sprintf(`[{"username":"%s","password":"%s"}, "/"]`, username, password)

	req, err := http.NewRequest("POST", s.BaseURL+"/login", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建登录请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Next-Action", HDHiveActionLogin)
	req.Header.Set("Accept", "text/x-component")
	req.Header.Set("Origin", "https://hdhive.com")
	req.Header.Set("Referer", "https://hdhive.com/login")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")

	resp, err := s.LoginClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("登录请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 从 Set-Cookie 中提取 token
	var token string
	var cookieStr string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "token" {
			token = cookie.Value
		}
	}

	if token == "" {
		// 读取响应体以获取错误信息
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("登录失败，未获取到 token，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	// 构造 cookie 字符串
	cookieStr = "token=" + token

	// 尝试提取 csrf_access_token
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "csrf_access_token" {
			cookieStr = "csrf_access_token=" + cookie.Value + "; " + cookieStr
			break
		}
	}

	log.Printf("✅ [HDHive] 登录成功，用户: %s", username)

	return &HDHiveLoginResult{
		Token:  token,
		Cookie: cookieStr,
	}, nil
}

// Search 搜索 HDHive（通过加密 + TMDB 代理）
func (s *HDHiveService) Search(query string, token, cookie string) (*HDHiveSearchResult, error) {
	// 第一步：加密搜索词
	encryptedQuery, err := s.encryptQuery(query, token, cookie)
	if err != nil {
		return nil, fmt.Errorf("加密搜索词失败: %w", err)
	}

	// 第二步：通过 go-api 代理搜索 TMDB
	searchURL := s.BaseURL + "/go-api/proxy/tmdb/3/search/multi?query=" + url.QueryEscape(encryptedQuery)
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建搜索请求失败: %w", err)
	}

	s.setCommonHeaders(req, token, cookie)

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取搜索响应失败: %w", err)
	}

	// 解析响应
	var apiResp struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析搜索响应失败: %w", err)
	}

	if !apiResp.Success {
		return nil, fmt.Errorf("搜索请求返回失败")
	}

	// 第三步：用加密结果再次调用获取解密后的搜索结果
	return s.fetchSearchResults(apiResp.Data, token, cookie)
}

// encryptQuery 通过 Next.js Server Action 加密搜索词
func (s *HDHiveService) encryptQuery(query string, token, cookie string) (string, error) {
	timestamp := time.Now().UTC().Unix()
	body := fmt.Sprintf(`["{\"query\":\"%s\",\"language\":\"zh-CN\",\"page\":1,\"utctimestamp\":%d}"]`, query, timestamp)

	req, err := http.NewRequest("POST", s.BaseURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Next-Action", HDHiveActionEncrypt)
	req.Header.Set("Accept", "text/x-component")
	s.setCommonHeaders(req, token, cookie)

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 响应格式: 0:{"a":"$@1",...}\n1:"加密字符串"
	respStr := string(respBody)
	lines := strings.Split(respStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "1:") {
			// 去掉 1: 前缀和引号
			encrypted := strings.TrimPrefix(line, "1:")
			encrypted = strings.Trim(encrypted, "\"")
			return encrypted, nil
		}
	}

	return "", fmt.Errorf("未能从加密响应中提取加密查询: %s", respStr)
}

// fetchSearchResults 通过加密数据获取解密后的搜索结果
func (s *HDHiveService) fetchSearchResults(encryptedData string, token, cookie string) (*HDHiveSearchResult, error) {
	body := fmt.Sprintf(`["%s"]`, encryptedData)

	req, err := http.NewRequest("POST", s.BaseURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Next-Action", HDHiveActionDecrypt)
	req.Header.Set("Accept", "text/x-component")
	s.setCommonHeaders(req, token, cookie)

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 响应格式: 0:{"a":"$@1",...}\n1:{搜索结果JSON}
	respStr := string(respBody)
	lines := strings.Split(respStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "1:") {
			jsonStr := strings.TrimPrefix(line, "1:")
			var result HDHiveSearchResult
			if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
				return nil, fmt.Errorf("解析搜索结果失败: %w, 原始数据: %s", err, jsonStr[:min(len(jsonStr), 200)])
			}
			return &result, nil
		}
	}

	return nil, fmt.Errorf("未能从响应中提取搜索结果")
}

// GetTmdbInfo 通过 HDHive 的 TMDB 代理获取 TMDB 详情信息
func (s *HDHiveService) GetTmdbInfo(tmdbID int, mediaType string, token, cookie string) (map[string]interface{}, error) {
	detailURL := fmt.Sprintf("%s/go-api/proxy/tmdb/3/%s/%d?language=zh-CN", s.BaseURL, mediaType, tmdbID)
	req, err := http.NewRequest("GET", detailURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建TMDB详情请求失败: %w", err)
	}

	s.setCommonHeaders(req, token, cookie)

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB详情请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取TMDB详情响应失败: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("解析TMDB详情失败: %w", err)
	}

	return result, nil
}

// GetResourceDetail 获取 HDHive 详情页中的 115 资源列表
func (s *HDHiveService) GetResourceDetail(tmdbID int, mediaType string, token, cookie string) ([]HDHive115Resource, error) {
	// 构造 TMDB 详情页 URL
	var tmdbURL string
	if mediaType == "tv" {
		tmdbURL = fmt.Sprintf("%s/tmdb/tv/%d", s.BaseURL, tmdbID)
	} else {
		tmdbURL = fmt.Sprintf("%s/tmdb/movie/%d", s.BaseURL, tmdbID)
	}

	// 第一步：通过 RSC 请求获取 NEXT_REDIRECT，提取 HDHive 内部 ID
	req, err := http.NewRequest("GET", tmdbURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建详情请求失败: %w", err)
	}

	s.setCommonHeaders(req, token, cookie)
	req.Header.Set("RSC", "1")
	req.Header.Set("Next-Router-State-Tree", "%5B%22%22%2C%7B%22children%22%3A%5B%22__PAGE__%22%2C%7B%7D%5D%7D%5D")
	req.Header.Set("Accept", "text/x-component")

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RSC 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 RSC 响应失败: %w", err)
	}

	rscStr := string(respBody)

	// 从 RSC 响应中提取 NEXT_REDIRECT 中的内部 URL
	// 格式: NEXT_REDIRECT;replace;/tv/321df25beb4411ed8d4e0242ac190003;307;
	redirectRegex := regexp.MustCompile(`NEXT_REDIRECT;[^;]*;(/(?:tv|movie)/[a-f0-9]{32});`)
	redirectMatch := redirectRegex.FindStringSubmatch(rscStr)
	if redirectMatch == nil || len(redirectMatch) < 2 {
		log.Printf("⚠️ [HDHive] 未找到内部重定向，该资源可能在 HDHive 中不存在 (TMDB %d)", tmdbID)
		return []HDHive115Resource{}, nil
	}

	internalPath := redirectMatch[1]
	internalURL := s.BaseURL + internalPath
	log.Printf("🔗 [HDHive] TMDB %d → %s", tmdbID, internalPath)

	// 第二步：对内部详情页发 RSC 请求，获取结构化资源数据
	req2, err := http.NewRequest("GET", internalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建内部详情请求失败: %w", err)
	}

	s.setCommonHeaders(req2, token, cookie)
	req2.Header.Set("RSC", "1")
	req2.Header.Set("Accept", "text/x-component")

	resp2, err := s.BrowseClient.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("内部详情请求失败: %w", err)
	}
	defer resp2.Body.Close()

	respBody2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, fmt.Errorf("读取内部详情响应失败: %w", err)
	}

	rscData := string(respBody2)
	// 内部RSC响应调试日志（正常运行时可注释）
	_ = resp2.StatusCode

	// 第三步：从 RSC 数据中解析 115 资源
	resources := s.parse115Resources(rscData, tmdbID, mediaType)

	log.Printf("📦 [HDHive] 找到 %d 个 115 资源", len(resources))
	return resources, nil
}

// rscResource RSC 响应中的 115 资源结构
type rscResource struct {
	ID                 int      `json:"id"`
	Slug               string   `json:"slug"`
	Title              string   `json:"title"`
	ShareSize          *string  `json:"share_size"`
	VideoResolution    []string `json:"video_resolution"`
	Source             []string `json:"source"`
	SubtitleLanguage   []string `json:"subtitle_language"`
	SubtitleType       []string `json:"subtitle_type"`
	Remark             string   `json:"remark"`
	UnlockPoints       int      `json:"unlock_points"`
	UnlockedUsersCount int      `json:"unlocked_users_count"`
	IsOfficial         bool     `json:"is_official"`
	User               *struct {
		ID        int    `json:"id"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// parse115Resources 从 HDHive RSC 响应中解析 115 资源信息
// RSC 数据格式: ...,{"websites":["115","aliPan"],"groupData":{"115":[{资源数组}]}}
func (s *HDHiveService) parse115Resources(body string, tmdbID int, mediaType string) []HDHive115Resource {
	var resources []HDHive115Resource

	// 查找 groupData 中的 115 资源数组
	// 格式: "groupData":{"115":[...]}
	groupDataIdx := strings.Index(body, `"groupData"`)
	if groupDataIdx < 0 {
		log.Printf("⚠️ [HDHive] RSC 响应中未找到 groupData")
		return resources
	}

	// 从 groupData 位置开始，查找 "115":[ 的起始位置
	searchFrom := body[groupDataIdx:]
	key115Idx := strings.Index(searchFrom, `"115":[`)
	if key115Idx < 0 {
		log.Printf("⚠️ [HDHive] groupData 中未找到 115 资源")
		return resources
	}

	// 定位到 115 数组的开始 [
	arrayStart := groupDataIdx + key115Idx + len(`"115":`)
	if arrayStart >= len(body) {
		return resources
	}

	// 找到匹配的 ] 来提取完整的 JSON 数组
	bracketCount := 0
	arrayEnd := -1
	for i := arrayStart; i < len(body); i++ {
		switch body[i] {
		case '[':
			bracketCount++
		case ']':
			bracketCount--
			if bracketCount == 0 {
				arrayEnd = i + 1
				break
			}
		}
		if arrayEnd >= 0 {
			break
		}
	}

	if arrayEnd < 0 {
		log.Printf("⚠️ [HDHive] 无法找到 115 数组的结束位置")
		return resources
	}

	arrayJSON := body[arrayStart:arrayEnd]
	// 调试日志已隐藏

	// 解析 JSON 数组
	var rscResources []rscResource
	if err := json.Unmarshal([]byte(arrayJSON), &rscResources); err != nil {
		log.Printf("⚠️ [HDHive] 解析 115 资源 JSON 失败: %v", err)
		// 尝试打印前200字符帮助调试
		preview := arrayJSON
		if len(preview) > 200 {
			preview = preview[:200]
		}
		log.Printf("🐛 [HDHive] JSON 预览: %s", preview)
		return resources
	}

	// 转换为 HDHive115Resource
	for _, r := range rscResources {
		size := ""
		if r.ShareSize != nil {
			size = *r.ShareSize
		}
		userName := ""
		avatarURL := ""
		if r.User != nil {
			userName = r.User.Nickname
			avatarURL = r.User.AvatarURL
		}
		res := HDHive115Resource{
			ID:            r.Slug,
			Title:         r.Title,
			Remark:        r.Remark,
			UserName:      userName,
			UserAvatar:    avatarURL,
			Size:          size,
			MediaType:     mediaType,
			TmdbID:        tmdbID,
			Points:        r.UnlockPoints,
			IsOfficial:    r.IsOfficial,
			UnlockedCount: r.UnlockedUsersCount,
		}
		// 拼接标签：分辨率、来源、字幕语言、字幕类型
		for _, v := range r.VideoResolution {
			res.Tags = append(res.Tags, v)
		}
		for _, s := range r.Source {
			res.Tags = append(res.Tags, s)
		}
		for _, sl := range r.SubtitleLanguage {
			res.Tags = append(res.Tags, sl)
		}
		for _, st := range r.SubtitleType {
			res.Tags = append(res.Tags, st)
		}
		resources = append(resources, res)
	}

	return resources
}

// UnlockResource 解锁 115 资源，返回网盘链接
func (s *HDHiveService) UnlockResource(resourceID string, token, cookie string) (*HDHiveUnlockResult, error) {
	body := fmt.Sprintf(`["%s"]`, resourceID)

	reqURL := fmt.Sprintf("%s/resource/115/%s", s.BaseURL, resourceID)
	req, err := http.NewRequest("POST", reqURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建解锁请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("Next-Action", HDHiveActionUnlock)
	req.Header.Set("Accept", "text/x-component")
	s.setCommonHeaders(req, token, cookie)

	resp, err := s.BrowseClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("解锁请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取解锁响应失败: %w", err)
	}

	// 响应格式: 0:{"a":"$@1",...}\n1:{解锁结果JSON}
	respStr := string(respBody)
	// 解锁响应调试日志已隐藏
	lines := strings.Split(respStr, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "1:") {
			jsonStr := strings.TrimPrefix(line, "1:")
			var result struct {
				Response struct {
					Success bool `json:"success"`
					Data    struct {
						FullURL string `json:"full_url"`
						URL     string `json:"url"`
					} `json:"data"`
					Message string `json:"message"`
					Code    string `json:"code"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
				return nil, fmt.Errorf("解析解锁响应失败: %w", err)
			}

			unlockURL := result.Response.Data.FullURL
			if unlockURL == "" {
				unlockURL = result.Response.Data.URL
			}
			// 统一替换旧域名为 115cdn.com
			unlockURL = normalize115URL(unlockURL)

			if result.Response.Success {
				log.Printf("✅ [HDHive] 解锁成功: %s", result.Response.Message)
			} else {
				log.Printf("⚠️ [HDHive] 解锁失败: %s", result.Response.Message)
			}

			return &HDHiveUnlockResult{
				Success: result.Response.Success,
				URL:     unlockURL,
				Message: result.Response.Message,
			}, nil
		}
	}

	return nil, fmt.Errorf("未能从响应中提取解锁结果")
}

// normalize115URL 将旧115域名统一替换为 115cdn.com
func normalize115URL(u string) string {
	// 常见旧域名列表
	oldDomains := []string{
		"https://anxia.com/s/",
		"http://anxia.com/s/",
		"https://www.anxia.com/s/",
		"http://www.anxia.com/s/",
	}
	for _, old := range oldDomains {
		if strings.HasPrefix(u, old) {
			return "https://115cdn.com/s/" + strings.TrimPrefix(u, old)
		}
	}
	return u
}

// setCommonHeaders 设置公共请求头
func (s *HDHiveService) setCommonHeaders(req *http.Request, token, cookie string) {
	req.Header.Set("Origin", "https://hdhive.com")
	req.Header.Set("Referer", "https://hdhive.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")

	// 设置 Cookie
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	} else if token != "" {
		req.Header.Set("Cookie", "token="+token)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
