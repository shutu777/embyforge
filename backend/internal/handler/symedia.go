package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"embyforge/internal/model"
	"embyforge/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SymediaHandler Symedia处理器
type SymediaHandler struct {
	DB        *gorm.DB
	JWTSecret string
}

// NewSymediaHandler 创建Symedia处理器
func NewSymediaHandler(db *gorm.DB, jwtSecret string) *SymediaHandler {
	return &SymediaHandler{
		DB:        db,
		JWTSecret: jwtSecret,
	}
}

// SymediaAPIResponse Symedia API响应结构
type SymediaAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// callSymediaAPI 调用Symedia配置更新API
// 参数:
//   - symediaUrl: Symedia服务地址
//   - authToken: Authorization令牌（会自动添加Bearer前缀）
//
// 返回:
//   - error: 调用失败时返回错误，成功时返回nil
func (h *SymediaHandler) callSymediaAPI(symediaUrl, authToken string) error {
	// 构建完整URL
	apiUrl := strings.TrimRight(symediaUrl, "/") + "/api/v1/archive/update_custom_words"

	// 创建HTTP客户端（设置30秒超时）
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 创建POST请求
	req, err := http.NewRequest("POST", apiUrl, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 处理Authorization头：自动添加Bearer前缀（如果不存在）
	token := authToken
	lowerToken := strings.ToLower(authToken)
	if strings.HasPrefix(lowerToken, "bearer ") {
		// 如果已有Bearer前缀（任何大小写），提取令牌部分并重新格式化
		tokenPart := strings.TrimSpace(authToken[7:]) // 去掉前7个字符 "bearer "
		token = "Bearer " + tokenPart
	} else {
		// 如果没有Bearer前缀，直接添加
		token = "Bearer " + authToken
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	log.Printf("🔄 [Symedia] 调用配置更新API: %s", apiUrl)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var apiResp SymediaAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查success字段
	if !apiResp.Success {
		return fmt.Errorf("Symedia API返回失败: %s", apiResp.Message)
	}

	log.Printf("✅ [Symedia] 配置更新成功")
	return nil
}

// PathReplaceConfig 路径替换配置响应结构
type PathReplaceConfig struct {
	CloudDrivePath string `json:"clouddrive_path"`
	EmbyStrmPath   string `json:"emby_strm_path"`
}

// GetConfigs 获取已保存的Symedia和GitHub配置
// GET /api/symedia/config
func (h *SymediaHandler) GetConfigs(c *gin.Context) {
	var response struct {
		SymediaUrl       string               `json:"symedia_url"`
		SymediaAuthToken string               `json:"symedia_auth_token"`
		GithubConfig     *model.WebhookConfig `json:"github_config"`
		PathReplace      PathReplaceConfig    `json:"path_replace"`
	}

	// 从SystemConfig表读取symedia_url
	var symediaUrlConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("❌ [Symedia] 读取symedia_url配置失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "读取配置失败，请稍后重试",
			})
			return
		}
		response.SymediaUrl = ""
	} else {
		response.SymediaUrl = symediaUrlConfig.Value
	}

	// 从SystemConfig表读取symedia_auth_token
	var authTokenConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("❌ [Symedia] 读取symedia_auth_token配置失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "读取配置失败，请稍后重试",
			})
			return
		}
		response.SymediaAuthToken = ""
	} else {
		response.SymediaAuthToken = authTokenConfig.Value
	}

	// 从WebhookConfig表读取GitHub配置
	var githubConfig model.WebhookConfig
	if err := h.DB.First(&githubConfig).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Printf("❌ [Symedia] 读取GitHub配置失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "读取配置失败，请稍后重试",
			})
			return
		}
		response.GithubConfig = nil
	} else {
		response.GithubConfig = &githubConfig
	}

	// 读取路径替换配置
	response.PathReplace = h.readPathReplaceConfig()

	log.Printf("ℹ️  [Symedia] 获取配置成功")
	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}

// readPathReplaceConfig 从 SystemConfig 表读取路径替换配置
func (h *SymediaHandler) readPathReplaceConfig() PathReplaceConfig {
	cfg := PathReplaceConfig{}

	var configs []model.SystemConfig
	h.DB.Where("key IN ?", []string{"path_replace_clouddrive", "path_replace_emby_strm"}).Find(&configs)

	for _, c := range configs {
		switch c.Key {
		case "path_replace_clouddrive":
			cfg.CloudDrivePath = c.Value
		case "path_replace_emby_strm":
			cfg.EmbyStrmPath = c.Value
		}
	}

	return cfg
}

// SavePathReplaceRequest 保存路径替换配置请求
type SavePathReplaceRequest struct {
	CloudDrivePath string `json:"clouddrive_path"`
	EmbyStrmPath   string `json:"emby_strm_path"`
}

// SavePathReplaceConfig 保存路径替换配置
// POST /api/symedia/path-replace-config
func (h *SymediaHandler) SavePathReplaceConfig(c *gin.Context) {
	var req SavePathReplaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，请检查输入"})
		return
	}

	configPairs := map[string]string{
		"path_replace_clouddrive": req.CloudDrivePath,
		"path_replace_emby_strm":  req.EmbyStrmPath,
	}

	for key, value := range configPairs {
		sc := model.SystemConfig{
			Key:         key,
			Value:       value,
			Description: "路径替换配置",
		}
		if err := h.DB.Where("key = ?", key).Assign(sc).FirstOrCreate(&sc).Error; err != nil {
			log.Printf("❌ [Symedia] 保存路径替换配置 %s 失败: %v", key, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败，请稍后重试"})
			return
		}
	}

	log.Printf("✅ [Symedia] 路径替换配置保存成功")
	c.JSON(http.StatusOK, gin.H{"message": "路径替换配置保存成功"})
}

// CheckPathRequest 检查路径请求
type CheckPathRequest struct {
	Path string `json:"path" binding:"required"`
}

// CheckPath 检查 Symedia 路径是否存在文件
// POST /api/symedia/check-path
func (h *SymediaHandler) CheckPath(c *gin.Context) {
	var req CheckPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效"})
		return
	}

	// 读取 Symedia URL
	var symediaUrlConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址"})
		return
	}

	symediaUrl := strings.TrimSpace(symediaUrlConfig.Value)
	if symediaUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址"})
		return
	}

	// 读取 auth token
	var authTokenConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 认证令牌"})
		return
	}
	authToken := strings.TrimSpace(authTokenConfig.Value)

	// 构建 listfile API URL
	apiUrl := strings.TrimRight(symediaUrl, "/") + "/api/v1/finder/listfile?path=" + url.QueryEscape(req.Path)

	client := &http.Client{Timeout: 15 * time.Second}
	httpReq, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建请求失败"})
		return
	}

	// 设置认证头
	if authToken != "" {
		token := authToken
		if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = "Bearer " + token
		}
		httpReq.Header.Set("Authorization", token)
	}

	log.Printf("🔄 [Symedia] 检查路径: %s", req.Path)
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("❌ [Symedia] 检查路径请求失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法连接到 Symedia 服务"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取响应失败"})
		return
	}

	// 判断返回是否为非空数组
	var result []json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil || len(result) == 0 {
		log.Printf("ℹ️  [Symedia] 路径为空: %s", req.Path)
		c.JSON(http.StatusOK, gin.H{
			"exists":      false,
			"symedia_url": symediaUrl,
		})
		return
	}

	log.Printf("✅ [Symedia] 路径存在: %s", req.Path)
	c.JSON(http.StatusOK, gin.H{
		"exists":      true,
		"symedia_url": symediaUrl,
	})
}

// SaveConfigRequest 保存配置请求结构（不触发刷新）
type SaveConfigRequest struct {
	SymediaUrl string `json:"symedia_url" binding:"required"`
	AuthToken  string `json:"auth_token" binding:"required"`
}

// SaveConfig 保存Symedia配置（不触发刷新）
// POST /api/symedia/save-config
func (h *SymediaHandler) SaveConfig(c *gin.Context) {
	var req SaveConfigRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("⚠️  [Symedia] 请求参数验证失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数无效，请检查输入",
		})
		return
	}

	// 验证URL格式
	if !isValidURL(req.SymediaUrl) {
		log.Printf("⚠️  [Symedia] URL格式无效: %s", req.SymediaUrl)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Symedia地址格式无效，必须是有效的HTTP/HTTPS URL",
		})
		return
	}

	// 验证令牌非空
	if strings.TrimSpace(req.AuthToken) == "" {
		log.Printf("⚠️  [Symedia] Authorization令牌为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Authorization令牌不能为空",
		})
		return
	}

	// 保存配置到SystemConfig表
	// 保存symedia_url
	symediaUrlConfig := model.SystemConfig{
		Key:         "symedia_url",
		Value:       req.SymediaUrl,
		Description: "Symedia服务地址",
	}
	if err := h.DB.Where("key = ?", "symedia_url").Assign(symediaUrlConfig).FirstOrCreate(&symediaUrlConfig).Error; err != nil {
		log.Printf("❌ [Symedia] 保存symedia_url配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存配置失败，请稍后重试",
		})
		return
	}

	// 保存symedia_auth_token
	authTokenConfig := model.SystemConfig{
		Key:         "symedia_auth_token",
		Value:       req.AuthToken,
		Description: "Symedia Authorization令牌",
	}
	if err := h.DB.Where("key = ?", "symedia_auth_token").Assign(authTokenConfig).FirstOrCreate(&authTokenConfig).Error; err != nil {
		log.Printf("❌ [Symedia] 保存symedia_auth_token配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "保存配置失败，请稍后重试",
		})
		return
	}

	log.Printf("✅ [Symedia] 配置保存成功")

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "配置保存成功",
	})
}

// ManualRefreshRequest 手动刷新请求结构
type ManualRefreshRequest struct {
	SymediaUrl string `json:"symedia_url" binding:"required"`
	AuthToken  string `json:"auth_token" binding:"required"`
}

// ManualRefresh 手动触发Symedia配置刷新
// POST /api/symedia/refresh
func (h *SymediaHandler) ManualRefresh(c *gin.Context) {
	var req ManualRefreshRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("⚠️  [Symedia] 请求参数验证失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数无效，请检查输入",
		})
		return
	}

	// 验证URL格式
	if !isValidURL(req.SymediaUrl) {
		log.Printf("⚠️  [Symedia] URL格式无效: %s", req.SymediaUrl)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Symedia地址格式无效，必须是有效的HTTP/HTTPS URL",
		})
		return
	}

	// 验证令牌非空（已通过binding验证，这里是双重检查）
	if strings.TrimSpace(req.AuthToken) == "" {
		log.Printf("⚠️  [Symedia] Authorization令牌为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Authorization令牌不能为空",
		})
		return
	}

	// 记录操作开始时间
	startTime := time.Now()

	// 调用Symedia API
	log.Printf("🔄 [Symedia] 手动刷新配置: url=%s", maskUrl(req.SymediaUrl))
	err := h.callSymediaAPI(req.SymediaUrl, req.AuthToken)

	// 计算耗时
	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		// 记录失败日志（结构化）
		structuredLog := util.FormatManualRefreshLog(
			req.SymediaUrl,
			req.AuthToken,
			"failure",
			duration,
			err.Error(),
		)
		log.Printf("❌ [Symedia] 手动刷新失败: %s", structuredLog)

		// 根据错误类型返回友好的错误消息
		errorMsg := "配置刷新失败，请稍后重试"
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "超时") {
			errorMsg = "无法连接到Symedia服务，请检查地址和网络"
		} else if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "连接被拒绝") {
			errorMsg = "无法连接到Symedia服务，请检查地址和网络"
		} else if strings.Contains(err.Error(), "Symedia API返回失败") {
			// 提取API返回的具体错误消息
			errorMsg = err.Error()
		} else if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			errorMsg = "认证失败，请检查令牌是否正确"
		}

		// 返回错误响应
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": errorMsg,
		})
		return
	}

	// 保存配置到SystemConfig表
	// 保存symedia_url
	symediaUrlConfig := model.SystemConfig{
		Key:         "symedia_url",
		Value:       req.SymediaUrl,
		Description: "Symedia服务地址",
	}
	if err := h.DB.Where("key = ?", "symedia_url").Assign(symediaUrlConfig).FirstOrCreate(&symediaUrlConfig).Error; err != nil {
		log.Printf("⚠️  [Symedia] 保存symedia_url配置失败: %v", err)
		// 不影响主流程，继续执行
	}

	// 保存symedia_auth_token
	authTokenConfig := model.SystemConfig{
		Key:         "symedia_auth_token",
		Value:       req.AuthToken,
		Description: "Symedia Authorization令牌",
	}
	if err := h.DB.Where("key = ?", "symedia_auth_token").Assign(authTokenConfig).FirstOrCreate(&authTokenConfig).Error; err != nil {
		log.Printf("⚠️  [Symedia] 保存symedia_auth_token配置失败: %v", err)
		// 不影响主流程，继续执行
	}

	// 记录成功日志（结构化）
	structuredLog := util.FormatManualRefreshLog(
		req.SymediaUrl,
		req.AuthToken,
		"success",
		duration,
		"",
	)
	log.Printf("✅ [Symedia] 手动刷新成功: %s", structuredLog)

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "配置刷新成功",
	})
}

// isValidURL 验证URL格式是否有效
func isValidURL(urlStr string) bool {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return false
	}

	// 检查是否以http://或https://开头
	if !strings.HasPrefix(strings.ToLower(urlStr), "http://") &&
		!strings.HasPrefix(strings.ToLower(urlStr), "https://") {
		return false
	}

	// 基本格式检查：至少包含协议和域名
	// 移除协议前缀
	withoutProtocol := urlStr
	if strings.HasPrefix(strings.ToLower(urlStr), "https://") {
		withoutProtocol = urlStr[8:]
	} else if strings.HasPrefix(strings.ToLower(urlStr), "http://") {
		withoutProtocol = urlStr[7:]
	}

	// 检查是否有域名部分
	if withoutProtocol == "" || withoutProtocol == "/" {
		return false
	}

	return true
}

// maskUrl 对URL进行脱敏处理（用于日志）
func maskUrl(urlStr string) string {
	// 简单处理：只显示协议和域名，隐藏路径
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		if idx2 := strings.Index(urlStr[idx+3:], "/"); idx2 != -1 {
			return urlStr[:idx+3+idx2] + "/***"
		}
	}
	return urlStr
}

// SaveGithubConfigOnlyRequest 只保存GitHub配置请求结构（不刷新Webhook URL）
type SaveGithubConfigOnlyRequest struct {
	RepoUrl  string `json:"repo_url" binding:"required"`
	Branch   string `json:"branch" binding:"required"`
	FilePath string `json:"file_path"` // 可选字段，为空或"*"表示监听所有文件
	Secret   string `json:"secret" binding:"required"`
}

// SaveGithubConfigOnly 只保存GitHub Webhook配置（不刷新Webhook URL）
// POST /api/symedia/github-config-save
func (h *SymediaHandler) SaveGithubConfigOnly(c *gin.Context) {
	var req SaveGithubConfigOnlyRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("⚠️  [Symedia] GitHub配置请求参数验证失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数无效，请检查输入",
		})
		return
	}

	// 验证仓库URL格式
	if !isValidURL(req.RepoUrl) {
		log.Printf("⚠️  [Symedia] 仓库URL格式无效: %s", req.RepoUrl)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "仓库URL格式无效，必须是有效的HTTP/HTTPS URL",
		})
		return
	}

	// 验证分支名称非空
	if strings.TrimSpace(req.Branch) == "" {
		log.Printf("⚠️  [Symedia] 分支名称为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "分支名称不能为空",
		})
		return
	}

	// 文件路径为可选，如果为空则设置为"*"表示监听所有文件
	filePath := strings.TrimSpace(req.FilePath)
	if filePath == "" {
		filePath = "*"
	}

	// 验证密钥非空
	if strings.TrimSpace(req.Secret) == "" {
		log.Printf("⚠️  [Symedia] Webhook密钥为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Webhook密钥不能为空",
		})
		return
	}

	// 从SystemConfig获取Symedia配置
	var symediaUrlConfig model.SystemConfig
	var authTokenConfig model.SystemConfig

	symediaUrl := ""
	authToken := ""

	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err == nil {
		symediaUrl = symediaUrlConfig.Value
	}

	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err == nil {
		authToken = authTokenConfig.Value
	}

	// 查找现有配置
	var existingConfig model.WebhookConfig
	err := h.DB.First(&existingConfig).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在，返回错误（需要先刷新Webhook URL）
		log.Printf("⚠️  [Symedia] 未找到Webhook配置，请先刷新Webhook URL")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "未找到Webhook配置，请先点击'刷新 Webhook URL'按钮",
		})
		return
	} else if err != nil {
		// 查询出错
		log.Printf("❌ [Symedia] 查询GitHub配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询配置失败，请稍后重试",
		})
		return
	}

	// 更新配置（保留原有的 WebhookUrl）
	existingConfig.SymediaUrl = symediaUrl
	existingConfig.AuthToken = authToken
	existingConfig.RepoUrl = req.RepoUrl
	existingConfig.Branch = req.Branch
	existingConfig.FilePath = filePath
	existingConfig.Secret = req.Secret

	if err := h.DB.Save(&existingConfig).Error; err != nil {
		log.Printf("❌ [Symedia] 更新GitHub配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "更新配置失败，请稍后重试",
		})
		return
	}

	log.Printf("✅ [Symedia] GitHub配置保存成功: repo=%s, branch=%s", req.RepoUrl, req.Branch)

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "GitHub配置保存成功",
	})
}

// SaveGithubConfigRequest 保存GitHub配置请求结构
type SaveGithubConfigRequest struct {
	RepoUrl  string `json:"repo_url" binding:"required"`
	Branch   string `json:"branch" binding:"required"`
	FilePath string `json:"file_path"` // 可选字段，为空或"*"表示监听所有文件
	Secret   string `json:"secret" binding:"required"`
}

// SaveGithubConfigResponse 保存GitHub配置响应结构
type SaveGithubConfigResponse struct {
	WebhookUrl string `json:"webhook_url"`
}

// SaveGithubConfig 保存GitHub Webhook配置
// POST /api/symedia/github-config
func (h *SymediaHandler) SaveGithubConfig(c *gin.Context) {
	var req SaveGithubConfigRequest

	// 绑定并验证请求参数
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("⚠️  [Symedia] GitHub配置请求参数验证失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "请求参数无效，请检查输入",
		})
		return
	}

	// 验证仓库URL格式
	if !isValidURL(req.RepoUrl) {
		log.Printf("⚠️  [Symedia] 仓库URL格式无效: %s", req.RepoUrl)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "仓库URL格式无效，必须是有效的HTTP/HTTPS URL",
		})
		return
	}

	// 验证分支名称非空
	if strings.TrimSpace(req.Branch) == "" {
		log.Printf("⚠️  [Symedia] 分支名称为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "分支名称不能为空",
		})
		return
	}

	// 文件路径为可选，如果为空则设置为"*"表示监听所有文件
	filePath := strings.TrimSpace(req.FilePath)
	if filePath == "" {
		filePath = "*"
	}

	// 验证密钥非空
	if strings.TrimSpace(req.Secret) == "" {
		log.Printf("⚠️  [Symedia] Webhook密钥为空")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Webhook密钥不能为空",
		})
		return
	}

	// 生成唯一的Webhook URL
	// 使用时间戳和随机字符串确保唯一性
	webhookUrl := generateWebhookUrl()

	// 从SystemConfig获取Symedia配置
	var symediaUrlConfig model.SystemConfig
	var authTokenConfig model.SystemConfig

	symediaUrl := ""
	authToken := ""

	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err == nil {
		symediaUrl = symediaUrlConfig.Value
	}

	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err == nil {
		authToken = authTokenConfig.Value
	}

	// 创建或更新WebhookConfig记录
	webhookConfig := model.WebhookConfig{
		SymediaUrl: symediaUrl,
		AuthToken:  authToken,
		RepoUrl:    req.RepoUrl,
		Branch:     req.Branch,
		FilePath:   filePath, // 使用处理后的文件路径
		Secret:     req.Secret,
		WebhookUrl: webhookUrl,
	}

	// 查找是否已存在配置
	var existingConfig model.WebhookConfig
	err := h.DB.First(&existingConfig).Error

	if err == gorm.ErrRecordNotFound {
		// 不存在，创建新记录
		if err := h.DB.Create(&webhookConfig).Error; err != nil {
			log.Printf("❌ [Symedia] 创建GitHub配置失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "保存配置失败，请稍后重试",
			})
			return
		}
		log.Printf("✅ [Symedia] 创建GitHub配置成功: repo=%s, branch=%s", req.RepoUrl, req.Branch)
	} else if err != nil {
		// 查询出错
		log.Printf("❌ [Symedia] 查询GitHub配置失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询配置失败，请稍后重试",
		})
		return
	} else {
		// 已存在，更新记录
		webhookConfig.ID = existingConfig.ID
		if err := h.DB.Save(&webhookConfig).Error; err != nil {
			log.Printf("❌ [Symedia] 更新GitHub配置失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "更新配置失败，请稍后重试",
			})
			return
		}
		log.Printf("✅ [Symedia] 更新GitHub配置成功: repo=%s, branch=%s", req.RepoUrl, req.Branch)
	}

	// 返回生成的Webhook URL
	c.JSON(http.StatusOK, gin.H{
		"message": "GitHub配置保存成功",
		"data": gin.H{
			"webhook_url": webhookUrl,
		},
	})
}

// generateWebhookUrl 生成唯一的Webhook URL
func generateWebhookUrl() string {
	// 使用时间戳（纳秒）+ 随机字符串确保唯一性
	timestamp := time.Now().UnixNano()

	// 生成一个简单的随机字符串（基于时间戳）
	// 在生产环境中，可以使用更强的随机生成器
	randomPart := fmt.Sprintf("%x", timestamp)

	// 构建完整的Webhook URL路径
	// 格式: /api/webhook/github/{unique_id}
	webhookPath := fmt.Sprintf("/api/webhook/github/%s", randomPart)

	return webhookPath
}
