package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/middleware"
	"embyforge/internal/model"
	"embyforge/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// defaultSyncTimeout 同步操作默认超时时间（大型媒体库可能有 30 万+ 条目）
const defaultSyncTimeout = 60 * time.Minute

// activeSync 正在运行的同步任务状态
type activeSync struct {
	cancel     context.CancelFunc
	progressCh chan service.SyncProgress // 源通道，由 SyncMediaCacheWithProgress 写入
	mu         sync.Mutex
	listeners  []chan service.SyncProgress // SSE 订阅者列表
	latest     *service.SyncProgress      // 最新的进度快照
	done       bool                       // 同步是否已完成
}

// addListener 添加一个 SSE 订阅者，返回订阅通道
func (a *activeSync) addListener() chan service.SyncProgress {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan service.SyncProgress, 16)
	// 如果有最新进度，先发送给新订阅者
	if a.latest != nil {
		select {
		case ch <- *a.latest:
		default:
		}
	}
	a.listeners = append(a.listeners, ch)
	return ch
}

// removeListener 移除一个 SSE 订阅者
func (a *activeSync) removeListener(ch chan service.SyncProgress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, l := range a.listeners {
		if l == ch {
			a.listeners = append(a.listeners[:i], a.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

// broadcast 向所有订阅者广播进度事件
func (a *activeSync) broadcast(p service.SyncProgress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.latest = &p
	if p.Done || p.Error != "" {
		a.done = true
	}
	for _, ch := range a.listeners {
		select {
		case ch <- p:
		default:
			// 订阅者通道满了，跳过（避免阻塞）
		}
	}
}

// closeAllListeners 关闭所有订阅者通道
func (a *activeSync) closeAllListeners() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.listeners {
		close(ch)
	}
	a.listeners = nil
}

// CacheHandler 缓存处理器
type CacheHandler struct {
	DB           *gorm.DB
	JWTSecret    string
	CacheService *service.CacheService
	SyncLock     *service.SyncLock
	EventBuffer  *service.EventBuffer

	syncMu     sync.Mutex
	activeSync *activeSync
}

// NewCacheHandler 创建缓存处理器
func NewCacheHandler(db *gorm.DB, jwtSecret string, syncLock *service.SyncLock, eventBuffer *service.EventBuffer) *CacheHandler {
	return &CacheHandler{
		DB:           db,
		JWTSecret:    jwtSecret,
		CacheService: service.NewCacheService(db),
		SyncLock:     syncLock,
		EventBuffer:  eventBuffer,
	}
}

// getEmbyClient 从数据库获取 Emby 配置并创建客户端
func (h *CacheHandler) getEmbyClient() (*emby.Client, error) {
	var config model.EmbyConfig
	if err := h.DB.First(&config).Error; err != nil {
		return nil, err
	}
	return emby.NewClient(config.Host, config.Port, config.APIKey), nil
}

// startSync 启动后台同步任务（如果没有正在运行的同步）
// 返回 activeSync、是否是新启动的、错误信息
func (h *CacheHandler) startSync(client *emby.Client) (*activeSync, bool, string) {
	h.syncMu.Lock()
	defer h.syncMu.Unlock()

	// 如果已有正在运行的同步，直接返回
	if h.activeSync != nil && !h.activeSync.done {
		return h.activeSync, false, ""
	}

	// 尝试获取同步锁
	if !h.SyncLock.TryLock("full_sync") {
		return nil, false, fmt.Sprintf("同步锁被占用 (%s)，请稍后再试", h.SyncLock.Holder())
	}

	// 取出缓冲事件（全量同步后协调）
	bufferedEvents := h.EventBuffer.DrainEvents()

	// 创建独立的 context（不绑定任何 HTTP 请求）
	ctx, cancel := context.WithTimeout(context.Background(), defaultSyncTimeout)

	progressCh := make(chan service.SyncProgress, 16)
	as := &activeSync{
		cancel:     cancel,
		progressCh: progressCh,
	}
	h.activeSync = as

	// 启动同步 goroutine
	go func() {
		log.Printf("🔄 启动全量同步模式")
		h.CacheService.SyncMediaCacheWithProgress(ctx, client, progressCh)
		cancel()

		// 全量同步结束后，再取出同步期间新缓冲的事件
		newBuffered := h.EventBuffer.DrainEvents()
		bufferedEvents = append(bufferedEvents, newBuffered...)

		// 释放同步锁
		h.SyncLock.Unlock()

		// 全量同步后协调缓冲事件
		if len(bufferedEvents) > 0 {
			log.Printf("🔄 全量同步后协调 %d 个缓冲事件 (同步前: %d, 同步期间: %d)",
				len(bufferedEvents), len(bufferedEvents)-len(newBuffered), len(newBuffered))
			reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer reconcileCancel()
			h.CacheService.ReconcileBufferedEvents(reconcileCtx, client, h.SyncLock, bufferedEvents)
		}
	}()

	// 启动广播 goroutine：从 progressCh 读取事件并广播给所有订阅者
	go func() {
		for p := range progressCh {
			as.broadcast(p)
		}
		// progressCh 关闭后，关闭所有订阅者
		as.closeAllListeners()

		// 清理 activeSync 引用
		h.syncMu.Lock()
		if h.activeSync == as {
			h.activeSync = nil
		}
		h.syncMu.Unlock()
	}()

	return as, true, ""
}

// SyncCache POST /api/cache/sync - 触发媒体库同步
func (h *CacheHandler) SyncCache(c *gin.Context) {
	log.Printf("🔄 开始同步媒体库缓存...")

	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请先配置 Emby 服务器连接信息",
		})
		return
	}

	// 获取同步锁
	if !h.SyncLock.TryLock("full_sync") {
		c.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": fmt.Sprintf("同步锁被占用 (%s)，请稍后再试", h.SyncLock.Holder()),
		})
		return
	}

	// 取出缓冲事件（全量同步后协调）
	bufferedEvents := h.EventBuffer.DrainEvents()

	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultSyncTimeout)
	defer cancel()

	result, err := h.CacheService.SyncMediaCacheWithContext(ctx, client)

	// 全量同步结束后，再取出同步期间新缓冲的事件
	newBuffered := h.EventBuffer.DrainEvents()
	bufferedEvents = append(bufferedEvents, newBuffered...)

	// 释放同步锁
	h.SyncLock.Unlock()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("⚠️ 媒体库同步超时")
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"code":    504,
				"message": "同步操作超时",
				"error":   err.Error(),
			})
			return
		}
		log.Printf("⚠️ 媒体库同步出错: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "同步过程中出错",
			"error":   err.Error(),
		})
		return
	}

	// 全量同步后协调缓冲事件
	if len(bufferedEvents) > 0 {
		reconcileCtx, reconcileCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer reconcileCancel()
		h.CacheService.ReconcileBufferedEvents(reconcileCtx, client, h.SyncLock, bufferedEvents)
	}

	log.Printf("✅ 媒体库同步完成: %d 个媒体条目, %d 个季, 耗时 %dms",
		result.TotalItems, result.TotalSeasons, result.ElapsedMs)
	c.JSON(http.StatusOK, gin.H{
		"message": "同步完成",
		"data":    result,
	})
}

// GetCacheStatus GET /api/cache/status - 获取缓存状态
func (h *CacheHandler) GetCacheStatus(c *gin.Context) {
	status, err := h.CacheService.GetCacheStatus()
	if err != nil {
		log.Printf("⚠️ 获取缓存状态失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取缓存状态失败",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":           status,
		"pending_events": h.EventBuffer.PendingCount(),
	})
}

// GetSyncStatus GET /api/cache/sync/status - 查询是否有正在进行的同步
func (h *CacheHandler) GetSyncStatus(c *gin.Context) {
	h.syncMu.Lock()
	as := h.activeSync
	h.syncMu.Unlock()

	if as == nil || as.done {
		c.JSON(http.StatusOK, gin.H{"syncing": false})
		return
	}

	as.mu.Lock()
	var latest *service.SyncProgress
	if as.latest != nil {
		cp := *as.latest
		latest = &cp
	}
	as.mu.Unlock()

	resp := gin.H{"syncing": true}
	if latest != nil {
		resp["progress"] = latest
	}
	c.JSON(http.StatusOK, resp)
}

// SyncCacheStream GET /api/cache/sync/stream - SSE 实时推送同步进度
// 使用 URL query parameter 传递 JWT token（因为 EventSource 不支持自定义 header）
func (h *CacheHandler) SyncCacheStream(c *gin.Context) {
	// 从 query parameter 获取 token 并手动验证 JWT
	tokenStr := c.Query("token")
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "缺少认证令牌"})
		return
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证令牌无效或已过期"})
		return
	}

	// 获取 Emby 客户端
	client, err := h.getEmbyClient()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请先配置 Emby 服务器连接信息"})
		return
	}

	// 启动或获取已有的同步任务
	as, isNew, busyMsg := h.startSync(client)
	if busyMsg != "" {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": busyMsg})
		return
	}
	if isNew {
		log.Printf("🔄 SSE 触发全量同步任务 (用户: %s)", claims.Username)
	} else {
		log.Printf("🔄 SSE 连接到已有同步任务 (用户: %s)", claims.Username)
	}

	// 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 获取 http.Flusher 用于立即推送数据
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "不支持 SSE 流式响应"})
		return
	}

	// 订阅进度事件
	listenerCh := as.addListener()
	defer as.removeListener(listenerCh)

	log.Printf("🔄 SSE 同步流已建立 (用户: %s)", claims.Username)

	// 从订阅通道读取事件并推送 SSE
	for {
		select {
		case <-c.Request.Context().Done():
			// 客户端断开 SSE 连接（不影响后台同步）
			log.Printf("⚠️ SSE 连接已断开 (用户: %s)", claims.Username)
			return

		case progress, ok := <-listenerCh:
			if !ok {
				// 通道已关闭，同步结束
				return
			}

			// 根据事件类型推送不同的 SSE 事件
			if progress.Error != "" {
				data, _ := json.Marshal(gin.H{"message": progress.Error})
				fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", data)
				flusher.Flush()
				return
			}

			if progress.Done {
				data, _ := json.Marshal(progress.Result)
				fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", data)
				flusher.Flush()
				return
			}

			// 进度事件
			percent := 0.0
			if progress.Total > 0 {
				percent = float64(progress.Processed) / float64(progress.Total) * 100
			}
			data, _ := json.Marshal(gin.H{
				"phase":     progress.Phase,
				"processed": progress.Processed,
				"total":     progress.Total,
				"percent":   percent,
			})
			fmt.Fprintf(c.Writer, "event: progress\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
