package handler

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogEntry 日志条目
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// LogBuffer 环形日志缓冲区，捕获系统日志
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

// NewLogBuffer 创建日志缓冲区
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add 添加一条日志
func (b *LogBuffer) Add(level, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry := LogEntry{
		Time:    time.Now().Format("15:04:05.000"),
		Level:   level,
		Message: message,
	}

	if len(b.entries) >= b.maxSize {
		// 移除最旧的
		b.entries = b.entries[1:]
	}
	b.entries = append(b.entries, entry)
}

// GetAll 获取所有日志
func (b *LogBuffer) GetAll() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]LogEntry, len(b.entries))
	copy(result, b.entries)
	return result
}

// Clear 清空日志
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = b.entries[:0]
}

// Write 实现 io.Writer 接口，用于捕获 log 标准库的输出
func (b *LogBuffer) Write(p []byte) (n int, err error) {
	msg := string(p)
	// 去掉末尾换行
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	// 根据内容判断级别
	level := "INFO"
	if containsAny(msg, "❌", "失败", "错误", "error", "fatal") {
		level = "ERROR"
	} else if containsAny(msg, "⚠️", "警告", "warn") {
		level = "WARNING"
	}

	// 去掉 log 标准库自带的时间前缀（格式: "2006/01/02 15:04:05 "）
	cleanMsg := msg
	if len(msg) > 20 && msg[4] == '/' && msg[7] == '/' && msg[10] == ' ' {
		cleanMsg = msg[20:]
	}

	b.Add(level, cleanMsg)
	return len(p), nil
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// LogsHandler 日志处理器
type LogsHandler struct {
	Buffer *LogBuffer
}

// NewLogsHandler 创建日志处理器
func NewLogsHandler(buffer *LogBuffer) *LogsHandler {
	return &LogsHandler{Buffer: buffer}
}

// GetRecentLogs 获取最近的系统日志
func (h *LogsHandler) GetRecentLogs(c *gin.Context) {
	entries := h.Buffer.GetAll()
	c.JSON(http.StatusOK, gin.H{"data": entries})
}
