package service

import (
	"context"
	"log"
	"sync"
	"time"

	"embyforge/internal/emby"
)

// CronStatus 调度器状态
type CronStatus struct {
	Running       bool       `json:"running"`
	IntervalHours int        `json:"interval_hours"`
	LastExecution *time.Time `json:"last_execution"`
	NextExecution *time.Time `json:"next_execution"`
}

// CronScheduler 定时全量同步调度器
type CronScheduler struct {
	mu            sync.Mutex
	ticker        *time.Ticker
	stopCh        chan struct{}
	running       bool
	lastExecution *time.Time
	nextExecution *time.Time
	intervalHours int
	syncLock      *SyncLock
	cacheService  *CacheService
	eventBuffer   *EventBuffer
	getEmbyClient func() (*emby.Client, error)
}

// ClampCronInterval 将 cron 间隔限制在有效范围 [1, 168] 小时
func ClampCronInterval(hours int) int {
	if hours < 1 {
		return 1
	}
	if hours > 168 {
		return 168
	}
	return hours
}

// NewCronScheduler 创建调度器
func NewCronScheduler(intervalHours int, syncLock *SyncLock, cacheService *CacheService, eventBuffer *EventBuffer, getClient func() (*emby.Client, error)) *CronScheduler {
	return &CronScheduler{
		intervalHours: ClampCronInterval(intervalHours),
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

	cs.running = true
	cs.stopCh = make(chan struct{})
	interval := time.Duration(cs.intervalHours) * time.Hour
	cs.ticker = time.NewTicker(interval)

	next := time.Now().Add(interval)
	cs.nextExecution = &next

	log.Printf("⏰ Cron 定时同步已启动，间隔: %d 小时", cs.intervalHours)

	go cs.loop()
}

// Stop 停止定时调度
func (cs *CronScheduler) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.running {
		return
	}

	cs.running = false
	close(cs.stopCh)
	cs.ticker.Stop()
	cs.nextExecution = nil
	log.Printf("⏰ Cron 定时同步已停止")
}

// UpdateInterval 更新调度间隔（热更新，无需重启）
func (cs *CronScheduler) UpdateInterval(hours int) {
	hours = ClampCronInterval(hours)

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.intervalHours == hours {
		return
	}

	cs.intervalHours = hours
	log.Printf("⏰ Cron 间隔已更新为 %d 小时", hours)

	// 如果正在运行，重置 ticker
	if cs.running && cs.ticker != nil {
		cs.ticker.Reset(time.Duration(hours) * time.Hour)
		next := time.Now().Add(time.Duration(hours) * time.Hour)
		cs.nextExecution = &next
	}
}

// Status 返回调度器状态
func (cs *CronScheduler) Status() CronStatus {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	return CronStatus{
		Running:       cs.running,
		IntervalHours: cs.intervalHours,
		LastExecution: cs.lastExecution,
		NextExecution: cs.nextExecution,
	}
}

// loop 调度循环
func (cs *CronScheduler) loop() {
	for {
		select {
		case <-cs.stopCh:
			return
		case <-cs.ticker.C:
			cs.executeSync()
		}
	}
}

// executeSync 执行一次全量同步
func (cs *CronScheduler) executeSync() {
	// 检查同步锁
	if cs.syncLock.IsLocked() {
		log.Printf("⏰ Cron: 同步锁被占用 (%s)，跳过本次周期", cs.syncLock.Holder())
		cs.mu.Lock()
		next := time.Now().Add(time.Duration(cs.intervalHours) * time.Hour)
		cs.nextExecution = &next
		cs.mu.Unlock()
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
	next := now.Add(time.Duration(cs.intervalHours) * time.Hour)
	cs.nextExecution = &next
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
