package service

import (
	"context"
	"log"
	"sync"
	"time"

	"embyforge/internal/emby"

	"github.com/robfig/cron/v3"
)

// DefaultCronExpr 默认 cron 表达式：每 2 小时执行一次
const DefaultCronExpr = "0 */2 * * *"

// CronStatus 调度器状态
type CronStatus struct {
	Running       bool       `json:"running"`
	CronExpr      string     `json:"cron_expr"`
	LastExecution *time.Time `json:"last_execution"`
	NextExecution *time.Time `json:"next_execution"`
}

// ValidateCronExpr 验证 cron 表达式是否合法
func ValidateCronExpr(expr string) bool {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err == nil
}

// CronScheduler 定时全量同步调度器
type CronScheduler struct {
	mu            sync.Mutex
	cronRunner    *cron.Cron
	entryID       cron.EntryID
	running       bool
	cronExpr      string
	lastExecution *time.Time
	syncLock      *SyncLock
	cacheService  *CacheService
	eventBuffer   *EventBuffer
	getEmbyClient func() (*emby.Client, error)
}

// NewCronScheduler 创建调度器
func NewCronScheduler(cronExpr string, syncLock *SyncLock, cacheService *CacheService, eventBuffer *EventBuffer, getClient func() (*emby.Client, error)) *CronScheduler {
	if !ValidateCronExpr(cronExpr) {
		log.Printf("⚠️ 无效的 cron 表达式 %q，使用默认值 %s", cronExpr, DefaultCronExpr)
		cronExpr = DefaultCronExpr
	}
	return &CronScheduler{
		cronExpr:      cronExpr,
		syncLock:      syncLock,
		cacheService:  cacheService,
		eventBuffer:   eventBuffer,
		getEmbyClient: getClient,
	}
}

// Start 启动定时调度
func (cs *CronScheduler) Start() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.running {
		return
	}

	cs.cronRunner = cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)))
	id, err := cs.cronRunner.AddFunc(cs.cronExpr, cs.executeSync)
	if err != nil {
		log.Printf("❌ Cron 表达式解析失败: %v", err)
		return
	}
	cs.entryID = id
	cs.cronRunner.Start()
	cs.running = true

	log.Printf("⏰ Cron 定时同步已启动，表达式: %s", cs.cronExpr)
}

// Stop 停止定时调度
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return
	}

	cs.cronRunner.Stop()
	cs.running = false
	log.Printf("⏰ Cron 定时同步已停止")
}

// UpdateCronExpr 更新 cron 表达式（热更新，无需重启）
func (cs *CronScheduler) UpdateCronExpr(expr string) {
	if !ValidateCronExpr(expr) {
		log.Printf("⚠️ 无效的 cron 表达式 %q，忽略更新", expr)
		return
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.cronExpr == expr {
		return
	}

	cs.cronExpr = expr
	log.Printf("⏰ Cron 表达式已更新为 %s", expr)

	// 如果正在运行，重建调度
	if cs.running && cs.cronRunner != nil {
		cs.cronRunner.Remove(cs.entryID)
		id, err := cs.cronRunner.AddFunc(expr, cs.executeSync)
		if err != nil {
			log.Printf("❌ 更新 cron 表达式失败: %v", err)
			return
		}
		cs.entryID = id
	}
}

// Status 返回调度器状态
func (cs *CronScheduler) Status() CronStatus {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	status := CronStatus{
		Running:       cs.running,
		CronExpr:      cs.cronExpr,
		LastExecution: cs.lastExecution,
	}

	// 从 cron runner 获取下次执行时间
	if cs.running && cs.cronRunner != nil {
		entry := cs.cronRunner.Entry(cs.entryID)
		if !entry.Next.IsZero() {
			next := entry.Next
			status.NextExecution = &next
		}
	}

	return status
}

// executeSync 执行一次全量同步
func (cs *CronScheduler) executeSync() {
	// 检查同步锁
	if cs.syncLock.IsLocked() {
		log.Printf("⏰ Cron: 同步锁被占用 (%s)，跳过本次周期", cs.syncLock.Holder())
		return
	}

	client, err := cs.getEmbyClient()
	if err != nil {
		log.Printf("⏰ Cron: 获取 Emby 客户端失败: %v", err)
		return
	}

	log.Printf("⏰ Cron: 开始定时全量同步")

	// 获取同步锁
	if !cs.syncLock.TryLock("cron_full_sync") {
		log.Printf("⏰ Cron: 获取同步锁失败，跳过本次周期")
		return
	}

	// 取出缓冲事件（全量同步后协调）
	bufferedEvents := cs.eventBuffer.DrainEvents()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	result, err := cs.cacheService.SyncMediaCacheWithContext(ctx, client)
	cs.syncLock.Unlock()

	now := time.Now()
	cs.mu.Lock()
	cs.lastExecution = &now
	cs.mu.Unlock()

	if err != nil {
		log.Printf("⏰ Cron: 全量同步失败: %v", err)
	} else {
		log.Printf("⏰ Cron: 全量同步完成: %d 个媒体条目, %d 个季, 耗时 %dms",
			result.TotalItems, result.TotalSeasons, result.ElapsedMs)
	}

	// 全量同步后协调缓冲事件
	if len(bufferedEvents) > 0 && err == nil {
		cs.cacheService.ReconcileBufferedEvents(ctx, client, cs.syncLock, bufferedEvents)
	}
}
