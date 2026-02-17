package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"embyforge/internal/config"
	"embyforge/internal/handler"
	"embyforge/internal/middleware"
	"embyforge/internal/model"

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
	cacheHandler := handler.NewCacheHandler(db, cfg.JWTSecret)
	dashboardHandler := handler.NewDashboardHandler(db)
	profileHandler := handler.NewProfileHandler(db, filepath.Dir(cfg.DBPath))
	systemConfigHandler := handler.NewSystemConfigHandler(db)
	logsHandler := handler.NewLogsHandler(logBuffer)

	// 初始化 Gin 引擎
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginLogger(accessLog, logBuffer))

	// 静态文件服务：头像等上传文件
	uploadsDir := filepath.Join(filepath.Dir(cfg.DBPath), "uploads")
	os.MkdirAll(uploadsDir, 0755)
	r.Static("/uploads", uploadsDir)

	// 公开路由（无需认证）
	public := r.Group("/api")
	{
		public.POST("/auth/login", authHandler.Login)
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

		protected.GET("/scan/scrape-anomaly", scanHandler.GetScrapeAnomalies)
		protected.GET("/scan/duplicate-media", scanHandler.GetDuplicateMedia)
		protected.GET("/scan/episode-mapping", scanHandler.GetEpisodeMappingAnomalies)
		protected.GET("/scan/analysis-status", scanHandler.GetAnalysisStatus)
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

		// 写入内存缓冲区（过滤掉日志接口自身的请求，避免刷屏）
		if c.Request.URL.Path != "/api/logs/recent" {
			logBuffer.Add(level, msg)
		}
	}
}
