package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Pan115Service 115网盘服务
type Pan115Service struct {
	Client *http.Client
}

// NewPan115Service 创建 115 网盘服务
func NewPan115Service() *Pan115Service {
	return &Pan115Service{
		Client: &http.Client{},
	}
}

// Pan115TransferResult 转存结果
type Pan115TransferResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// Pan115TestResult Cookie测试结果
type Pan115TestResult struct {
	Valid    bool   `json:"valid"`
	Message  string `json:"message"`
	UserName string `json:"user_name,omitempty"`
	UserID   string `json:"user_id,omitempty"`
}

// TestCookie 测试115 Cookie是否有效
func (s *Pan115Service) TestCookie(cookie string) (*Pan115TestResult, error) {
	req, err := http.NewRequest("GET", "https://my.115.com/?ct=ajax&ac=nav", nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	s.setHeaders(req, cookie)

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		State bool `json:"state"`
		Data  struct {
			UserID   json.Number `json:"user_id"`
			UserName string      `json:"user_name"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return &Pan115TestResult{Valid: false, Message: "Cookie 无效或已过期"}, nil
	}

	if !result.State || result.Data.UserName == "" {
		return &Pan115TestResult{Valid: false, Message: "Cookie 无效或已过期"}, nil
	}

	log.Printf("✅ [115] Cookie 验证成功，用户: %s", result.Data.UserName)
	return &Pan115TestResult{
		Valid:    true,
		Message:  fmt.Sprintf("验证成功，用户: %s", result.Data.UserName),
		UserName: result.Data.UserName,
		UserID:   result.Data.UserID.String(),
	}, nil
}

// Transfer 将分享链接转存到指定目录
// shareURL 格式: https://115cdn.com/s/{share_code}?password={receive_code}
// cookie: 115网盘的登录cookie
// cid: 目标文件夹ID
func (s *Pan115Service) Transfer(shareURL, cookie, cid string) (*Pan115TransferResult, error) {
	// 解析分享链接，提取 share_code 和 receive_code
	shareCode, receiveCode, err := parseShareURL(shareURL)
	if err != nil {
		return nil, fmt.Errorf("解析分享链接失败: %w", err)
	}

	log.Printf("🔗 [115] 开始转存: share_code=%s, cid=%s", shareCode, cid)

	// 第一步：获取分享快照信息（snap），拿到文件列表
	snapURL := "https://webapi.115.com/share/snap"
	snapParams := url.Values{
		"share_code":   {shareCode},
		"receive_code": {receiveCode},
		"offset":       {"0"},
		"limit":        {"20"},
		"cid":          {""},
	}

	snapReq, err := http.NewRequest("GET", snapURL+"?"+snapParams.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建快照请求失败: %w", err)
	}
	s.setHeaders(snapReq, cookie)

	snapResp, err := s.Client.Do(snapReq)
	if err != nil {
		return nil, fmt.Errorf("快照请求失败: %w", err)
	}
	defer snapResp.Body.Close()

	snapBody, err := io.ReadAll(snapResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取快照响应失败: %w", err)
	}

	var snapResult struct {
		State   bool   `json:"state"`
		Error   string `json:"error"`
		Message string `json:"message"`
		Errno   int    `json:"errno"`
		Data    struct {
			List []struct {
				ShareID json.Number `json:"share_id"`
				FileID  json.Number `json:"file_id"`
				Cid     json.Number `json:"cid"`
			} `json:"list"`
		} `json:"data"`
	}

	if err := json.Unmarshal(snapBody, &snapResult); err != nil {
		return nil, fmt.Errorf("解析快照响应失败: %w", err)
	}

	if !snapResult.State {
		errMsg := snapResult.Error
		if errMsg == "" {
			errMsg = snapResult.Message
		}
		log.Printf("⚠️ [115] 获取分享信息失败: %s (errno: %d)", errMsg, snapResult.Errno)
		return &Pan115TransferResult{
			Success: false,
			Message: errMsg,
		}, nil
	}

	if len(snapResult.Data.List) == 0 {
		return &Pan115TransferResult{
			Success: false,
			Message: "分享链接中没有文件",
		}, nil
	}

	// 收集所有文件ID
	var fileIDs []string
	for _, item := range snapResult.Data.List {
		fileIDs = append(fileIDs, item.FileID.String())
	}

	// 第二步：接收（转存）文件到指定目录
	receiveURL := "https://webapi.115.com/share/receive"
	receiveData := url.Values{
		"share_code":   {shareCode},
		"receive_code": {receiveCode},
		"file_id":      {strings.Join(fileIDs, ",")},
		"cid":          {cid},
	}

	receiveReq, err := http.NewRequest("POST", receiveURL, strings.NewReader(receiveData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("创建转存请求失败: %w", err)
	}
	s.setHeaders(receiveReq, cookie)
	receiveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	receiveResp, err := s.Client.Do(receiveReq)
	if err != nil {
		return nil, fmt.Errorf("转存请求失败: %w", err)
	}
	defer receiveResp.Body.Close()

	receiveBody, err := io.ReadAll(receiveResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取转存响应失败: %w", err)
	}

	var receiveResult struct {
		State   bool   `json:"state"`
		Message string `json:"message"`
		Errno   int    `json:"errno"`
		Error   string `json:"error"`
	}

	if err := json.Unmarshal(receiveBody, &receiveResult); err != nil {
		return nil, fmt.Errorf("解析转存响应失败: %w", err)
	}

	if receiveResult.State {
		log.Printf("✅ [115] 转存成功: %d 个文件", len(fileIDs))
		return &Pan115TransferResult{
			Success: true,
			Message: fmt.Sprintf("转存成功，共 %d 个文件", len(fileIDs)),
		}, nil
	}

	errMsg := receiveResult.Error
	if errMsg == "" {
		errMsg = receiveResult.Message
	}
	// errno 处理常见错误码
	switch receiveResult.Errno {
	case 4000023:
		errMsg = "该文件已转存过"
		return &Pan115TransferResult{Success: true, Message: errMsg}, nil
	case 4000020:
		errMsg = "转存次数已达上限"
	case 4000021:
		errMsg = "分享链接已失效"
	}

	log.Printf("⚠️ [115] 转存失败: %s (errno: %d)", errMsg, receiveResult.Errno)
	return &Pan115TransferResult{
		Success: false,
		Message: errMsg,
	}, nil
}

// parseShareURL 解析 115 分享链接
// 输入: https://115cdn.com/s/swwch453np7?password=knhh
// 输出: share_code=swwch453np7, receive_code=knhh
func parseShareURL(shareURL string) (string, string, error) {
	u, err := url.Parse(shareURL)
	if err != nil {
		return "", "", fmt.Errorf("URL 格式错误: %w", err)
	}

	// 从路径中提取 share_code: /s/{share_code}
	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(pathParts) < 2 || pathParts[0] != "s" {
		return "", "", fmt.Errorf("无效的分享链接路径: %s", u.Path)
	}
	shareCode := pathParts[1]

	// 从查询参数中提取 password 作为 receive_code
	receiveCode := u.Query().Get("password")

	return shareCode, receiveCode, nil
}

// setHeaders 设置 115 WebAPI 请求头
func (s *Pan115Service) setHeaders(req *http.Request, cookie string) {
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://115.com/")
	req.Header.Set("Origin", "https://115.com")
}
