package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"embyforge/internal/model"
	"embyforge/internal/util"
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

// TransferConfigKeys 归档配置在 SystemConfig 表中的键名
var TransferConfigKeys = map[string]string{
	"rule_id":          "symedia_transfer_rule_id",
	"dest_dir":         "symedia_transfer_dest_dir",
	"transfer_type":    "symedia_transfer_type",
	"category":         "symedia_transfer_category",
	"delete_dir":       "symedia_transfer_delete_dir",
	"extract_metadata": "symedia_transfer_extract_metadata",
	"cache_metadata":   "symedia_transfer_cache_metadata",
	"download_nfo":     "symedia_transfer_download_nfo",
	"download_image":   "symedia_transfer_download_image",
	"path_from":        "symedia_transfer_path_from",
	"path_to":          "symedia_transfer_path_to",
}

// TransferConfigResponse 归档配置响应结构
type TransferConfigResponse struct {
	RuleID          string `json:"rule_id"`
	DestDir         string `json:"dest_dir"`
	TransferType    string `json:"transfer_type"`
	Category        bool   `json:"category"`
	DeleteDir       bool   `json:"delete_dir"`
	ExtractMetadata bool   `json:"extract_metadata"`
	CacheMetadata   bool   `json:"cache_metadata"`
	DownloadNfo     bool   `json:"download_nfo"`
	DownloadImage   bool   `json:"download_image"`
	PathFrom        string `json:"path_from"`
	PathTo          string `json:"path_to"`
}

// GetConfigsResponse 获取配置响应结构
type GetConfigsResponse struct {
	SymediaUrl string                 `json:"symedia_url"`
	AuthToken  string                 `json:"auth_token"`
	Github     *model.WebhookConfig   `json:"github"`
}

// GetConfigs 获取已保存的Symedia和GitHub配置
// GET /api/symedia/config
func (h *SymediaHandler) GetConfigs(c *gin.Context) {
	var response struct {
		SymediaUrl       string                `json:"symedia_url"`
		SymediaAuthToken string                `json:"symedia_auth_token"`
		GithubConfig     *model.WebhookConfig  `json:"github_config"`
		TransferConfig   TransferConfigResponse `json:"transfer_config"`
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
		// 记录未找到，使用空值
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
		// 记录未找到，使用空值
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
		// 记录未找到，返回nil
		response.GithubConfig = nil
	} else {
		response.GithubConfig = &githubConfig
	}

	// 读取归档配置
	response.TransferConfig = h.readTransferConfig()
	
	log.Printf("ℹ️  [Symedia] 获取配置成功")
	c.JSON(http.StatusOK, gin.H{
		"data": response,
	})
}

// boolWithDefault 从配置映射中读取布尔值，键不存在时返回默认值
func boolWithDefault(m map[string]string, key string, defaultVal bool) bool {
	v, ok := m[key]
	if !ok || v == "" {
		return defaultVal
	}
	return v == "true"
}

// readTransferConfig 从 SystemConfig 表读取归档配置
func (h *SymediaHandler) readTransferConfig() TransferConfigResponse {
	cfg := TransferConfigResponse{}

	// 批量读取所有归档配置键
	var configs []model.SystemConfig
	keys := make([]string, 0, len(TransferConfigKeys))
	for _, v := range TransferConfigKeys {
		keys = append(keys, v)
	}
	h.DB.Where("key IN ?", keys).Find(&configs)

	// 构建 key->value 映射
	configMap := make(map[string]string)
	for _, c := range configs {
		configMap[c.Key] = c.Value
	}

	cfg.RuleID = configMap[TransferConfigKeys["rule_id"]]
	cfg.DestDir = configMap[TransferConfigKeys["dest_dir"]]
	// 归档方式默认 cd2_move
	if v, ok := configMap[TransferConfigKeys["transfer_type"]]; ok && v != "" {
		cfg.TransferType = v
	} else {
		cfg.TransferType = "cd2_move"
	}
	// 布尔开关：未保存过的键使用默认值
	cfg.Category = boolWithDefault(configMap, TransferConfigKeys["category"], true)
	cfg.DeleteDir = boolWithDefault(configMap, TransferConfigKeys["delete_dir"], false)
	cfg.ExtractMetadata = boolWithDefault(configMap, TransferConfigKeys["extract_metadata"], true)
	cfg.CacheMetadata = boolWithDefault(configMap, TransferConfigKeys["cache_metadata"], true)
	cfg.DownloadNfo = boolWithDefault(configMap, TransferConfigKeys["download_nfo"], false)
	cfg.DownloadImage = boolWithDefault(configMap, TransferConfigKeys["download_image"], false)
	cfg.PathFrom = configMap[TransferConfigKeys["path_from"]]
	cfg.PathTo = configMap[TransferConfigKeys["path_to"]]

	return cfg
}

// SaveTransferConfigRequest 保存归档配置请求结构
type SaveTransferConfigRequest struct {
	RuleID          string `json:"rule_id"`
	DestDir         string `json:"dest_dir"`
	TransferType    string `json:"transfer_type"`
	Category        bool   `json:"category"`
	DeleteDir       bool   `json:"delete_dir"`
	ExtractMetadata bool   `json:"extract_metadata"`
	CacheMetadata   bool   `json:"cache_metadata"`
	DownloadNfo     bool   `json:"download_nfo"`
	DownloadImage   bool   `json:"download_image"`
	PathFrom        string `json:"path_from"`
	PathTo          string `json:"path_to"`
}

// SaveTransferConfig 保存归档配置
// POST /api/symedia/transfer-config
func (h *SymediaHandler) SaveTransferConfig(c *gin.Context) {
	var req SaveTransferConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，请检查输入"})
		return
	}

	// 验证 rule_id 非空
	if strings.TrimSpace(req.RuleID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule_id 不能为空"})
		return
	}

	// 构建要保存的配置键值对
	boolStr := func(b bool) string {
		if b {
			return "true"
		}
		return "false"
	}

	configPairs := map[string]string{
		TransferConfigKeys["rule_id"]:          req.RuleID,
		TransferConfigKeys["dest_dir"]:         req.DestDir,
		TransferConfigKeys["transfer_type"]:    req.TransferType,
		TransferConfigKeys["category"]:         boolStr(req.Category),
		TransferConfigKeys["delete_dir"]:       boolStr(req.DeleteDir),
		TransferConfigKeys["extract_metadata"]: boolStr(req.ExtractMetadata),
		TransferConfigKeys["cache_metadata"]:   boolStr(req.CacheMetadata),
		TransferConfigKeys["download_nfo"]:     boolStr(req.DownloadNfo),
		TransferConfigKeys["download_image"]:   boolStr(req.DownloadImage),
		TransferConfigKeys["path_from"]:        req.PathFrom,
		TransferConfigKeys["path_to"]:          req.PathTo,
	}

	// 逐个保存到 SystemConfig 表
	for key, value := range configPairs {
		sc := model.SystemConfig{
			Key:         key,
			Value:       value,
			Description: "Symedia 归档配置",
		}
		if err := h.DB.Where("key = ?", key).Assign(sc).FirstOrCreate(&sc).Error; err != nil {
			log.Printf("❌ [Symedia] 保存归档配置 %s 失败: %v", key, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置失败，请稍后重试"})
			return
		}
	}

	log.Printf("✅ [Symedia] 归档配置保存成功")
	c.JSON(http.StatusOK, gin.H{"message": "归档配置保存成功"})
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
	RepoUrl   string `json:"repo_url" binding:"required"`
	Branch    string `json:"branch" binding:"required"`
	FilePath  string `json:"file_path"` // 可选字段，为空或"*"表示监听所有文件
	Secret    string `json:"secret" binding:"required"`
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
	RepoUrl   string `json:"repo_url" binding:"required"`
	Branch    string `json:"branch" binding:"required"`
	FilePath  string `json:"file_path"` // 可选字段，为空或"*"表示监听所有文件
	Secret    string `json:"secret" binding:"required"`
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

// TransferRequest 前端提交的归档请求
type TransferRequest struct {
	Name      string `json:"name" binding:"required"`
	Path      string `json:"path" binding:"required"`
	TmdbID    int    `json:"tmdbid" binding:"required"`
	MediaType string `json:"media_type" binding:"required"` // movie 或 tv
	Season    *int   `json:"season"`                        // 剧集的季号，电影为 nil
}

// SymediaTransferPayload 发送给 Symedia 的完整请求体
type SymediaTransferPayload struct {
	Items        []SymediaTransferItem `json:"items"`
	HistoryIDs   interface{}           `json:"history_ids"`
	TransferForm SymediaTransferForm   `json:"transferForm"`
}

// SymediaTransferItem 归档条目
type SymediaTransferItem struct {
	Name     string                `json:"name"`
	Path     string                `json:"path"`
	Type     string                `json:"type"`
	Size     int                   `json:"size"`
	Mtime    int64                 `json:"mtime"`
	Children []SymediaTransferItem `json:"children"`
}

// SymediaTransferForm 归档表单参数
type SymediaTransferForm struct {
	DestDir         string `json:"dest_dir"`
	RuleID          string `json:"rule_id"`
	TmdbID          int    `json:"tmdbid"`
	DoubanID        *int   `json:"doubanid"`
	Season          *int   `json:"season"`
	MediaType       string `json:"media_type"`
	TransferType    string `json:"transfer_type"`
	EpisodeFormat   string `json:"episode_format"`
	EpisodeDetail   string `json:"episode_detail"`
	EpisodePart     string `json:"episode_part"`
	EpisodeOffset   *int   `json:"episode_offset"`
	MinFilesize     int    `json:"min_filesize"`
	Suffix          string `json:"suffix"`
	DownloadNfo     bool   `json:"download_nfo"`
	DownloadImage   bool   `json:"download_image"`
	Category        bool   `json:"category"`
	DeleteDir       bool   `json:"delete_dir"`
	ExtractMetadata bool   `json:"extract_metadata"`
	CacheMetadata   bool   `json:"cache_metadata"`
}

// SyncMapping 软链接同步映射关系（从 Symedia sync_list 获取）
type SyncMapping struct {
	MediaDir   string `json:"media_dir"`   // 云盘路径，如 /CloudNAS/CloudDrive/115open/影库
	SymlinkDir string `json:"symlink_dir"` // Emby 挂载路径，如 /media
}

// mapItemPath 使用 sync_list 映射表将 Emby 路径转换为云盘路径
// 匹配规则：找到 Emby 路径前缀匹配的 symlink_dir，替换为对应的 media_dir
// 优先匹配最长的 symlink_dir（最精确匹配）
func mapItemPath(embyPath string, syncMappings []SyncMapping) string {
	if embyPath == "" || len(syncMappings) == 0 {
		return embyPath
	}

	bestMatch := ""
	bestMediaDir := ""
	for _, m := range syncMappings {
		symlinkDir := strings.TrimRight(m.SymlinkDir, "/")
		// Emby 路径必须以 symlink_dir 开头（精确到路径段边界）
		if strings.HasPrefix(embyPath, symlinkDir+"/") || embyPath == symlinkDir {
			if len(symlinkDir) > len(bestMatch) {
				bestMatch = symlinkDir
				bestMediaDir = strings.TrimRight(m.MediaDir, "/")
			}
		}
	}

	if bestMatch != "" {
		return bestMediaDir + embyPath[len(bestMatch):]
	}

	return embyPath
}

// BuildSymediaPayload 根据前端请求和保存的配置组装 Symedia 请求体
// syncMappings 为 Symedia sync_list 返回的路径映射表
func BuildSymediaPayload(req TransferRequest, cfg TransferConfigResponse, syncMappings []SyncMapping) SymediaTransferPayload {
	// 路径映射：使用 sync_list 映射表将 Emby 路径转换为云盘路径
	itemPath := mapItemPath(req.Path, syncMappings)
	// name 只是文件夹名，不需要路径映射
	itemName := req.Name

	item := SymediaTransferItem{
		Name:     itemName,
		Path:     itemPath,
		Type:     "dir",
		Size:     0,
		Mtime:    time.Now().Unix(),
		Children: []SymediaTransferItem{},
	}

	form := SymediaTransferForm{
		DestDir:         cfg.DestDir,
		RuleID:          cfg.RuleID,
		TmdbID:          req.TmdbID,
		DoubanID:        nil,
		Season:          req.Season,
		MediaType:       req.MediaType,
		TransferType:    cfg.TransferType,
		EpisodeFormat:   "",
		EpisodeDetail:   "",
		EpisodePart:     "",
		EpisodeOffset:   nil,
		MinFilesize:     0,
		Suffix:          "",
		DownloadNfo:     cfg.DownloadNfo,
		DownloadImage:   cfg.DownloadImage,
		Category:        cfg.Category,
		DeleteDir:       cfg.DeleteDir,
		ExtractMetadata: cfg.ExtractMetadata,
		CacheMetadata:   cfg.CacheMetadata,
	}

	return SymediaTransferPayload{
		Items:        []SymediaTransferItem{item},
		HistoryIDs:   nil,
		TransferForm: form,
	}
}

// fetchSyncList 从 Symedia 获取软链接同步映射列表
func (h *SymediaHandler) fetchSyncList(symediaUrl, authToken string) ([]SyncMapping, error) {
	apiUrl := strings.TrimRight(symediaUrl, "/") + "/api/v1/autosymlink/sync_list"

	client := &http.Client{Timeout: 15 * time.Second}
	httpReq, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	// 设置认证头
	token := authToken
	lowerToken := strings.ToLower(authToken)
	if strings.HasPrefix(lowerToken, "bearer ") {
		token = "Bearer " + strings.TrimSpace(authToken[7:])
	} else {
		token = "Bearer " + authToken
	}
	httpReq.Header.Set("Authorization", token)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Symedia 服务: %w", err)
	}
	defer resp.Body.Close()

	var rawList []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	mappings := make([]SyncMapping, 0, len(rawList))
	for _, raw := range rawList {
		var m SyncMapping
		if err := json.Unmarshal(raw, &m); err == nil && m.MediaDir != "" && m.SymlinkDir != "" {
			mappings = append(mappings, m)
		}
	}
	return mappings, nil
}

// Transfer 执行归档操作
// POST /api/symedia/transfer
func (h *SymediaHandler) Transfer(c *gin.Context) {
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效，请检查输入"})
		return
	}

	// 验证 media_type
	if req.MediaType != "movie" && req.MediaType != "tv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media_type 必须为 movie 或 tv"})
		return
	}

	// 读取 Symedia URL 和 auth token
	var symediaUrlConfig, authTokenConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}
	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}

	symediaUrl := strings.TrimSpace(symediaUrlConfig.Value)
	authToken := strings.TrimSpace(authTokenConfig.Value)
	if symediaUrl == "" || authToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}

	// 读取归档固定配置
	transferCfg := h.readTransferConfig()
	if strings.TrimSpace(transferCfg.RuleID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置归档规则 ID"})
		return
	}

	// 校验 rule_id 在 Symedia 端是否仍然有效
	rules, err := h.fetchTransferRules(symediaUrl, authToken)
	if err != nil {
		log.Printf("⚠️ [Symedia] 校验规则 ID 时获取规则列表失败: %v", err)
		// 获取规则列表失败不阻塞归档，只记录日志
	} else {
		ruleValid := false
		for _, r := range rules {
			if r.RuleID == transferCfg.RuleID {
				ruleValid = true
				break
			}
		}
		if !ruleValid {
			log.Printf("❌ [Symedia] 归档规则 ID %s 在 Symedia 端不存在", transferCfg.RuleID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "当前配置的归档规则在 Symedia 中已不存在，请前往「配置管理」重新选择归档规则"})
			return
		}
	}

	// 组装 Symedia 请求体
	// 从 Symedia 获取软链接同步映射表，用于路径转换
	syncMappings, err := h.fetchSyncList(symediaUrl, authToken)
	if err != nil {
		log.Printf("⚠️ [Symedia] 获取同步映射列表失败，路径将不做转换: %v", err)
		syncMappings = []SyncMapping{}
	}
	payload := BuildSymediaPayload(req, transferCfg, syncMappings)
	log.Printf("🔍 [Symedia] 路径映射: %q → %q (映射规则 %d 条)", req.Path, payload.Items[0].Path, len(syncMappings))

	// 序列化请求体
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("❌ [Symedia] 序列化归档请求体失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建请求失败"})
		return
	}

	// 构建 Symedia API URL
	apiUrl := strings.TrimRight(symediaUrl, "/") + "/api/v1/transfer/manual"

	// 创建 HTTP 客户端（60 秒超时）
	client := &http.Client{Timeout: 60 * time.Second}

	httpReq, err := http.NewRequest("POST", apiUrl, bytes.NewReader(payloadBytes))
	if err != nil {
		log.Printf("❌ [Symedia] 创建归档请求失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}

	// 设置请求头
	token := authToken
	lowerToken := strings.ToLower(authToken)
	if strings.HasPrefix(lowerToken, "bearer ") {
		tokenPart := strings.TrimSpace(authToken[7:])
		token = "Bearer " + tokenPart
	} else {
		token = "Bearer " + authToken
	}
	httpReq.Header.Set("Authorization", token)
	httpReq.Header.Set("Content-Type", "application/json")

	log.Printf("🔄 [Symedia] 发送归档请求: name=%s, tmdbid=%d, media_type=%s", req.Name, req.TmdbID, req.MediaType)

	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("❌ [Symedia] 归档请求失败: %v", err)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline") {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Symedia 服务响应超时"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "无法连接到 Symedia 服务"})
		}
		return
	}
	defer resp.Body.Close()

	// 解析 Symedia 响应
	var symResp SymediaAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&symResp); err != nil {
		log.Printf("❌ [Symedia] 解析归档响应失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解析 Symedia 响应失败"})
		return
	}

	if !symResp.Success {
		log.Printf("❌ [Symedia] 归档失败: %s", symResp.Message)
		c.JSON(http.StatusInternalServerError, gin.H{"error": symResp.Message})
		return
	}

	log.Printf("✅ [Symedia] 归档成功: name=%s", req.Name)
	c.JSON(http.StatusOK, gin.H{
		"message": "归档请求已提交",
		"data": gin.H{
			"success": true,
			"message": symResp.Message,
		},
	})
}

// RuleItem 规则列表项（提取 id、name、rule_id 和 dest_dir）
type RuleItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	RuleID  string `json:"rule_id"`
	DestDir string `json:"dest_dir"`
}

// fetchTransferRules 从 Symedia 获取归档规则列表（内部方法）
func (h *SymediaHandler) fetchTransferRules(symediaUrl, authToken string) ([]RuleItem, error) {
	apiUrl := strings.TrimRight(symediaUrl, "/") + "/api/v1/system/settings/transfer_list"

	client := &http.Client{Timeout: 15 * time.Second}
	httpReq, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}

	// 设置认证头
	token := authToken
	lowerToken := strings.ToLower(authToken)
	if strings.HasPrefix(lowerToken, "bearer ") {
		token = "Bearer " + strings.TrimSpace(authToken[7:])
	} else {
		token = "Bearer " + authToken
	}
	httpReq.Header.Set("Authorization", token)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("无法连接到 Symedia 服务: %w", err)
	}
	defer resp.Body.Close()

	var rawList []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawList); err != nil {
		return nil, fmt.Errorf("解析 Symedia 响应失败: %w", err)
	}

	rules := make([]RuleItem, 0, len(rawList))
	for _, raw := range rawList {
		var item RuleItem
		if err := json.Unmarshal(raw, &item); err == nil && item.ID != "" {
			rules = append(rules, item)
		}
	}
	return rules, nil
}

// GetTransferRules 从 Symedia 获取归档规则列表
// GET /api/symedia/transfer-rules
func (h *SymediaHandler) GetTransferRules(c *gin.Context) {
	// 读取 Symedia URL 和 auth token
	var symediaUrlConfig, authTokenConfig model.SystemConfig
	if err := h.DB.Where("key = ?", "symedia_url").First(&symediaUrlConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}
	if err := h.DB.Where("key = ?", "symedia_auth_token").First(&authTokenConfig).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}

	symediaUrl := symediaUrlConfig.Value
	authToken := authTokenConfig.Value

	if strings.TrimSpace(symediaUrl) == "" || strings.TrimSpace(authToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先配置 Symedia 服务地址和认证令牌"})
		return
	}

	rules, err := h.fetchTransferRules(symediaUrl, authToken)
	if err != nil {
		log.Printf("❌ [Symedia] 获取规则列表失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rules})
}
