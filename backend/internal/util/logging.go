package util

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MaskToken 对令牌进行脱敏处理，仅显示前4位和后4位
// 如果令牌长度小于等于8位，则全部用星号替代
func MaskToken(token string) string {
	if token == "" {
		return ""
	}

	// 使用 rune 切片处理多字节字符
	runes := []rune(token)
	length := len(runes)
	
	if length <= 8 {
		return strings.Repeat("*", length)
	}

	// 显示前4位和后4位，中间用星号替代
	prefix := string(runes[:4])
	suffix := string(runes[length-4:])
	maskedLength := length - 8
	return fmt.Sprintf("%s%s%s", prefix, strings.Repeat("*", maskedLength), suffix)
}

// LogEntry 结构化日志条目
type LogEntry struct {
	Timestamp    time.Time              `json:"timestamp"`
	Level        string                 `json:"level"`
	Source       string                 `json:"source"`
	Action       string                 `json:"action"`
	Result       string                 `json:"result"`
	DurationMs   int64                  `json:"duration_ms,omitempty"`
	SymediaURL   string                 `json:"symedia_url,omitempty"`
	MaskedToken  string                 `json:"masked_token,omitempty"`
	RepoName     string                 `json:"repo_name,omitempty"`
	Branch       string                 `json:"branch,omitempty"`
	CommitSHA    string                 `json:"commit_sha,omitempty"`
	ChangedFiles []string               `json:"changed_files,omitempty"`
	ErrorMsg     string                 `json:"error_msg,omitempty"`
	Extra        map[string]interface{} `json:"extra,omitempty"`
}

// FormatStructuredLog 格式化结构化日志为JSON字符串
func FormatStructuredLog(entry LogEntry) string {
	entry.Timestamp = time.Now()
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Sprintf(`{"error":"日志序列化失败: %v"}`, err)
	}
	return string(jsonBytes)
}

// FormatManualRefreshLog 格式化手动刷新日志
func FormatManualRefreshLog(symediaURL, authToken, result string, durationMs int64, errorMsg string) string {
	entry := LogEntry{
		Level:       "INFO",
		Source:      "manual",
		Action:      "config_refresh",
		Result:      result,
		DurationMs:  durationMs,
		SymediaURL:  symediaURL,
		MaskedToken: MaskToken(authToken),
		ErrorMsg:    errorMsg,
	}
	if errorMsg != "" {
		entry.Level = "ERROR"
	}
	return FormatStructuredLog(entry)
}

// FormatWebhookLog 格式化Webhook日志
func FormatWebhookLog(repoName, branch, commitSHA string, changedFiles []string, result string, durationMs int64, errorMsg string) string {
	entry := LogEntry{
		Level:        "INFO",
		Source:       "github",
		Action:       "webhook_triggered",
		Result:       result,
		DurationMs:   durationMs,
		RepoName:     repoName,
		Branch:       branch,
		CommitSHA:    commitSHA,
		ChangedFiles: changedFiles,
		ErrorMsg:     errorMsg,
	}
	if errorMsg != "" {
		entry.Level = "ERROR"
	}
	return FormatStructuredLog(entry)
}
