package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"embyforge/internal/config"
	"embyforge/internal/emby"
	"embyforge/internal/handler"
	"embyforge/internal/middleware"
	"embyforge/internal/model"
	"embyforge/internal/service"

	"github.com/gin-gonic/gin"
)

// accessLogger 按天轮转的请求日志写入器
type accessLogger struct {
	mu      sync.Mutex
	dir     string
	file    *os.File
	logger  *log.Logger
	curDate string
}

// newAccessLogger 创建请求日志写入器，日志存放在指定目录
func newAccessLogger(logDir string) (*accessLogger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	al := &accessLogger{dir: logDir}
	if err := al.rotate(); err != nil {
		return nil, err
	}
	return al, nil
}

// rotate 按天切换日志文件
func (al *accessLogger) rotate() error {
	today := time.Now().Format("2006-01-02")
	if today == al.curDate && al.file != nil {
		return nil
	}
	if al.file != nil {
		al.file.Close()
	}
	path := filepath.Join(al.dir, fmt.Sprintf("access-%s.log", today))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	al.file = f
	al.logger = log.New(f, "", log.LstdFlags)
	al.curDate = today
	return nil
}

// write 写入一条请求日志
func (al *accessLogger) write(msg string) {
	al.mu.Lock()
	defer al.mu.Unlock()
	al.rotate()
	al.logger.Println(msg)
}

// cleanup 删除超过指定天数的旧日志
func (al *accessLogger) cleanup(maxDays int) {
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	entries, err := os.ReadDir(al.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "access-") {
			continue
		}
		// 从文件名解析日期: access-2006-01-02.log
		name := strings.TrimPrefix(e.Name(), "access-")
		name = strings.TrimSuffix(name, ".log")
		t, err := time.Parse("2006-01-02", name)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			os.Remove(filepath.Join(al.dir, e.Name()))
			log.Printf("🗑️  已清理过期日志: %s", e.Name())
		}
	}
}

func main() {
	// 初始化日志缓冲区，捕获系统日志到内存（最多保留200条）
	logBuffer := handler.NewLogBuffer(200)
	// 同时输出到 stdout 和缓冲区
	multiWriter := io.MultiWriter(os.Stdout, logBuffer)
	log.SetOutput(multiWriter)

	log.Println("🚀 EmbyForge 正在启动...")

	// 加载配置
	cfg := config.Load()
	log.Println("⚙️  配置加载完成")

	// 设置 Gin 为 release 模式
	gin.SetMode(gin.ReleaseMode)

	// 初始化数据库
	db, err := model.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}
	log.Println("📦 数据库初始化完成")

	// 如果环境变量未设置 JWT secret，从数据库加载持久化密钥
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		var sc model.SystemConfig
		if err := db.Where("`key` = ?", "jwt_secret").First(&sc).Error; err != nil {
			// 数据库中没有，生成并持久化
			jwtSecret = config.GenerateRandomSecret()
			db.Create(&model.SystemConfig{
				Key:         "jwt_secret",
				Value:       jwtSecret,
				Description: "JWT 签名密钥（自动生成，修改密码时会更新）",
			})
			log.Println("🔑 已生成并持久化 JWT 密钥")
		} else {
			jwtSecret = sc.Value
			log.Println("🔑 已从数据库加载 JWT 密钥")
		}
	}
	cfg.JWTSecret = jwtSecret

	// 初始化请求日志（写入文件，不输出到终端）
	logDir := filepath.Join(filepath.Dir(cfg.DBPath), "logs")
	accessLog, err := newAccessLogger(logDir)
	if err != nil {
		log.Printf("⚠️  请求日志初始化失败，将不记录请求日志: %v", err)
	} else {
		// 启动时清理超过7天的旧日志
		accessLog.cleanup(7)
		log.Printf("📋 请求日志目录: %s（保留7天）", logDir)
	}

	// 初始化处理器
	authHandler := handler.NewAuthHandler(db, cfg.JWTSecret)
	embyConfigHandler := handler.NewEmbyConfigHandler(db)
	scanHandler := handler.NewScanHandler(db)

	// 创建共享的同步锁和缓存服务
	syncLock := &service.SyncLock{}
	cacheService := service.NewCacheService(db)

	// 创建 getEmbyClient 辅助函数
	getEmbyClient := func() (*emby.Client, error) {
		var embyConfig model.EmbyConfig
		if err := db.First(&embyConfig).Error; err != nil {
			return nil, err
		}
		return emby.NewClient(embyConfig.Host, embyConfig.Port, embyConfig.APIKey), nil
	}

	// 创建 EventBuffer（flush 时执行 Delta Update）
	var eventBuffer *service.EventBuffer
	eventBuffer = service.NewEventBuffer(syncLock, func(ctx context.Context, events []*service.BufferedEvent) {
		client, err := getEmbyClient()
		if err != nil {
			log.Printf("⚠️ 增量同步: 获取 Emby 客户端失败: %v", err)
			return
		}
		if err := cacheService.ProcessDeltaEvents(ctx, client, syncLock, events); err != nil {
			if errors.Is(err, service.ErrSyncLockBusy) {
				// 锁被占用（全量同步中），将事件放回缓冲区等待协调
				eventBuffer.RequeueEvents(events)
			} else {
				log.Printf("⚠️ 增量同步失败: %v", err)
			}
		}
	})

	cacheHandler := handler.NewCacheHandler(db, cfg.JWTSecret, syncLock, eventBuffer)
	embyWebhookHandler := handler.NewEmbyWebhookHandler(eventBuffer)

	// 从数据库读取 cron 表达式配置
	cronExpr := service.DefaultCronExpr
	var cronConfig model.SystemConfig
	if err := db.Where("`key` = ?", "cron_sync_expr").First(&cronConfig).Error; err != nil {
		// 不存在则创建默认配置；同时清理旧的 interval_hours 配置
		db.Create(&model.SystemConfig{
			Key:         "cron_sync_expr",
			Value:       service.DefaultCronExpr,
			Description: "定时全量同步 Cron 表达式（如 0 */2 * * * 表示每2小时）",
		})
		db.Where("`key` = ?", "cron_sync_interval_hours").Delete(&model.SystemConfig{})
	} else {
		if cronConfig.Value != "" {
			cronExpr = cronConfig.Value
		}
	}

	// 创建并启动 CronScheduler
	cronScheduler := service.NewCronScheduler(cronExpr, syncLock, cacheService, eventBuffer, getEmbyClient)
	cronScheduler.Start()

	dashboardHandler := handler.NewDashboardHandler(db)
	profileHandler := handler.NewProfileHandler(db, filepath.Dir(cfg.DBPath))
	systemConfigHandler := handler.NewSystemConfigHandler(db)
	// 配置更新回调：cron 表达式变更时热更新调度器
	systemConfigHandler.OnConfigUpdate = func(key, value string) {
		if key == "cron_sync_expr" {
			cronScheduler.UpdateCronExpr(value)
		}
	}
	logsHandler := handler.NewLogsHandler(logBuffer)
	tmdbCacheHandler := handler.NewTmdbCacheHandler(db)
	symediaHandler := handler.NewSymediaHandler(db, cfg.JWTSecret)
	webhookHandler := handler.NewWebhookHandler(db, symediaHandler)
	renderingWordsHandler := handler.NewRenderingWordsHandler(db)
	embyCacheHandler := handler.NewEmbyCacheHandler(db)
	quickDeleteHandler := handler.NewQuickDeleteHandler(db)

	// 初始化 Gin 引擎
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginLogger(accessLog, logBuffer))

	// 确保上传目录存在（在 Docker 中由 Nginx 提供静态文件服务）
	uploadsDir := filepath.Join(filepath.Dir(cfg.DBPath), "uploads")
	os.MkdirAll(uploadsDir, 0755)

	// 创建速率限制器
	webhookRateLimiter := middleware.NewRateLimiter(10, time.Minute)
	embyWebhookRateLimiter := middleware.NewRateLimiter(60, time.Minute)

	// 公开路由（无需认证）
	public := r.Group("/api")
	{
		public.POST("/auth/login", authHandler.Login)
		// GitHub Webhook 公开端点（带速率限制）
		public.POST("/webhook/github", 
			middleware.RateLimitMiddleware(webhookRateLimiter),
			webhookHandler.HandleGitHubWebhook)
		public.POST("/webhook/github/:id", 
			middleware.RateLimitMiddleware(webhookRateLimiter),
			webhookHandler.HandleGitHubWebhook)
		// Emby Webhook 公开端点（带速率限制，每分钟最多 60 个请求）
		public.POST("/webhook/emby",
			middleware.RateLimitMiddleware(embyWebhookRateLimiter),
			embyWebhookHandler.HandleEmbyWebhook)
	}

	// 受保护路由（需要 JWT 认证）
	protected := r.Group("/api")
	protected.Use(middleware.JWTAuth(cfg.JWTSecret))

	// SSE 路由（handler 内部通过 query parameter 验证 JWT，不使用中间件）
	r.GET("/api/cache/sync/stream", cacheHandler.SyncCacheStream)

	{
		protected.GET("/dashboard", dashboardHandler.GetDashboard)

		protected.GET("/profile", profileHandler.GetProfile)
		protected.PUT("/profile/username", profileHandler.ChangeUsername)
		protected.PUT("/profile/password", profileHandler.ChangePassword)
		protected.POST("/profile/avatar", profileHandler.UploadAvatar)

		protected.GET("/system-config", systemConfigHandler.GetAllConfigs)
		protected.PUT("/system-config/:key", systemConfigHandler.UpdateConfig)

		protected.GET("/logs/recent", logsHandler.GetRecentLogs)

		protected.GET("/emby-config", embyConfigHandler.GetConfig)
		protected.POST("/emby-config", embyConfigHandler.SaveConfig)
		protected.POST("/emby-config/test", embyConfigHandler.TestConnection)
		protected.GET("/emby-config/server-info", embyConfigHandler.GetServerInfo)

		protected.POST("/cache/sync", cacheHandler.SyncCache)
		protected.GET("/cache/status", cacheHandler.GetCacheStatus)
		protected.GET("/cache/sync/status", cacheHandler.GetSyncStatus)

		protected.POST("/analyze/scrape-anomaly", scanHandler.AnalyzeScrapeAnomalies)
		protected.POST("/analyze/duplicate-media", scanHandler.AnalyzeDuplicateMedia)
		protected.POST("/analyze/episode-mapping", scanHandler.AnalyzeEpisodeMapping)

		protected.POST("/cleanup/duplicate-media", scanHandler.CleanupDuplicateMedia)
		protected.GET("/cleanup/duplicate-media/preview", scanHandler.PreviewDuplicateCleanup)
		protected.POST("/cleanup/scrape-anomaly", scanHandler.CleanupScrapeAnomalies)
		protected.GET("/cleanup/missing-poster-items", scanHandler.GetMissingPosterItems)
		protected.POST("/cleanup/batch-find-posters", scanHandler.BatchFindPosters)
		protected.POST("/cleanup/find-single-poster", scanHandler.FindSinglePoster)

		protected.GET("/scan/scrape-anomaly", scanHandler.GetScrapeAnomalies)
		protected.GET("/scan/duplicate-media", scanHandler.GetDuplicateMedia)
		protected.GET("/scan/episode-mapping", scanHandler.GetEpisodeMappingAnomalies)
		protected.GET("/scan/analysis-status", scanHandler.GetAnalysisStatus)

		protected.GET("/tmdb-cache", tmdbCacheHandler.GetTmdbCacheList)
		protected.GET("/tmdb-cache/status", tmdbCacheHandler.GetTmdbCacheStatus)
		protected.PUT("/tmdb-cache/:id", tmdbCacheHandler.UpdateTmdbCache)
		protected.DELETE("/tmdb-cache/:id", tmdbCacheHandler.DeleteTmdbCache)
		protected.DELETE("/tmdb-cache/show/:tmdbId", tmdbCacheHandler.DeleteTmdbCacheByShow)
		protected.POST("/tmdb-cache/clear", tmdbCacheHandler.ClearTmdbCache)

		// Symedia 配置管理
		protected.GET("/symedia/config", symediaHandler.GetConfigs)
		protected.POST("/symedia/save-config", symediaHandler.SaveConfig)
		protected.POST("/symedia/refresh", symediaHandler.ManualRefresh)
		protected.POST("/symedia/github-config-save", symediaHandler.SaveGithubConfigOnly)
		protected.POST("/symedia/github-config", symediaHandler.SaveGithubConfig)

		// 渲染词生成器
		protected.GET("/rendering-words/import-candidates", renderingWordsHandler.GetImportCandidates)
		protected.GET("/rendering-words/validate-tmdb/:tmdbId", renderingWordsHandler.ValidateTmdbID)

		// Emby 缓存管理
		protected.GET("/emby-cache", embyCacheHandler.GetEmbyCacheList)
		protected.GET("/emby-cache/status", embyCacheHandler.GetEmbyCacheStatus)
		protected.PUT("/emby-cache/:id", embyCacheHandler.UpdateEmbyCache)
		protected.DELETE("/emby-cache/:id", embyCacheHandler.DeleteEmbyCache)
		protected.POST("/emby-cache/:id/refresh", embyCacheHandler.RefreshEmbyCache)

		// 快速删除
		protected.GET("/quick-delete/search", quickDeleteHandler.SearchEmbyMedia)
		protected.GET("/quick-delete/seasons/:seriesId", quickDeleteHandler.GetSeriesSeasons)
		protected.POST("/quick-delete/delete", quickDeleteHandler.DeleteMedia)
	}

	// 启动服务
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("✅ EmbyForge 已启动，监听端口 %s", addr)
	log.Println("📝 默认账户: admin / admin")
	if err := r.Run(addr); err != nil {
		log.Fatalf("❌ 服务启动失败: %v", err)
	}
}

// ginLogger 请求日志中间件，写入文件和日志缓冲区
func ginLogger(accessLog *accessLogger, logBuffer *handler.LogBuffer) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		icon := "✅"
		level := "INFO"
		if status >= 400 {
			icon = "⚠️"
			level = "WARNING"
		}
		if status >= 500 {
			icon = "❌"
			level = "ERROR"
		}

		msg := fmt.Sprintf("%s %d | %s | %s %s",
			icon, status, duration.Round(time.Millisecond),
			c.Request.Method, c.Request.URL.Path)

		// 写入文件
		if accessLog != nil {
			accessLog.write(msg)
		}

		// 写入内存缓冲区（过滤掉高频 GET 请求，避免刷屏）
		// 只过滤成功的 GET 请求，非 200 的仍然显示
		if !shouldHideFromRealtimeLog(c.Request.Method, c.Request.URL.Path, status) {
			logBuffer.Add(level, msg)
		}
	}
}

// shouldHideFromRealtimeLog 判断请求是否应从实时日志面板中隐藏
// 隐藏所有成功的 GET 请求（这些都是前端页面加载和状态查询，信息价值低）
// POST/PUT/DELETE 和所有错误请求始终显示
func shouldHideFromRealtimeLog(method, path string, status int) bool {
	if method == "GET" && status < 400 {
		return true
	}
	return false
}
