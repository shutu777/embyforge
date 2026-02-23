package emby

import (
	"context"
	"log"
	"sync"
	"time"
)

// LibraryChangeHandler 媒体库变更回调函数
// items: 变更的完整媒体条目（新增/更新），removed: 删除信号
type LibraryChangeHandler func(items []MediaItem, removed []string)

// LibraryWatcher 媒体库变更轮询监听器
// 定时检查 Emby 是否有新增/修改/删除的媒体条目
type LibraryWatcher struct {
	client   *Client
	handler  LibraryChangeHandler
	interval time.Duration // 轮询间隔

	mu      sync.Mutex
	stopCh  chan struct{}
	running bool

	// 上次检查时间，用于增量查询
	lastCheck time.Time
	// 上次 Emby 总数，用于检测删除
	lastTotal int
	// 手动同步进行中标记，轮询时跳过
	syncActive bool
	// 实时同步处理中标记
	busy bool
	// 轮询计数器，用于定期兜底删除检测
	pollCount int
}

// NewLibraryWatcher 创建媒体库变更轮询监听器
// interval: 轮询间隔，建议 30 秒
// lastSyncAt: 上次同步时间，用于初始化增量查询起点（传零值则用当前时间）
func NewLibraryWatcher(client *Client, handler LibraryChangeHandler, interval time.Duration, lastSyncAt time.Time) *LibraryWatcher {
	if lastSyncAt.IsZero() {
		lastSyncAt = time.Now()
	}
	return &LibraryWatcher{
		client:    client,
		handler:   handler,
		interval:  interval,
		lastCheck: lastSyncAt,
	}
}

// Start 启动轮询监听（非阻塞，后台运行）
func (w *LibraryWatcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	// 获取初始总数
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	total, err := w.client.GetTotalItemCount(ctx)
	cancel()
	if err != nil {
		log.Printf("⚠️ 获取 Emby 初始媒体总数失败: %v", err)
	} else {
		w.lastTotal = total
		log.Printf("📡 媒体库监听已启动，Emby 总数: %d，轮询间隔: %v，起始检查时间: %s",
			total, w.interval, w.lastCheck.Format("2006-01-02 15:04:05"))
	}

	go w.pollLoop()
}

// Stop 停止轮询监听
func (w *LibraryWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)
	log.Printf("🔌 媒体库轮询监听已停止")
}

// IsRunning 返回监听器是否正在运行
func (w *LibraryWatcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// pollLoop 轮询循环
func (w *LibraryWatcher) pollLoop() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// IsBusy 返回实时监控是否正在处理变更（用于互斥手动同步）
func (w *LibraryWatcher) IsBusy() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.busy
}

// SetSyncActive 设置手动同步状态，轮询检查时会跳过
func (w *LibraryWatcher) SetSyncActive(active bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.syncActive = active
	if !active {
		// 手动同步结束后，更新 lastCheck 为当前时间，避免重复检测
		w.lastCheck = time.Now()
	}
}

// check 执行一次变更检查
func (w *LibraryWatcher) check() {
	w.mu.Lock()
	if w.syncActive {
		w.mu.Unlock()
		return // 手动同步进行中，跳过本次轮询
	}
	checkSince := w.lastCheck
	w.pollCount++
	currentPollCount := w.pollCount
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// 1. 检查新增/修改的条目（用 MinDateLastSaved），直接收集完整 MediaItem
	var changedItems []MediaItem
	err := w.client.GetMediaItemsModifiedSince(ctx, checkSince, SyncItemTypes, func(items []MediaItem) error {
		changedItems = append(changedItems, items...)
		return nil
	})
	if err != nil {
		log.Printf("⚠️ 轮询检查变更失败: %v", err)
		return
	}

	// 2. 检查总数变化
	currentTotal, err := w.client.GetTotalItemCount(ctx)
	if err != nil {
		log.Printf("⚠️ 轮询获取总数失败: %v", err)
		return
	}

	w.mu.Lock()
	prevTotal := w.lastTotal
	w.lastCheck = time.Now()
	w.lastTotal = currentTotal
	w.mu.Unlock()

	// 判断是否需要触发删除检测（实时监控场景）：
	// - 总数减少（明确有删除）
	// - 每 3 次轮询兜底检测一次
	// 注意：不再用 len(changedItems) > 0 触发删除检测，因为 294K 条目的全量 ID 对比
	// 需要约 5 分钟，如果每次有变更都触发会导致连续阻塞
	needDeleteDetection := false
	if currentTotal < prevTotal {
		diff := prevTotal - currentTotal
		log.Printf("📡 检测到 Emby 媒体总数减少: %d → %d（减少 %d）", prevTotal, currentTotal, diff)
		needDeleteDetection = true
	} else if currentPollCount%3 == 0 {
		log.Printf("📡 定期兜底删除检测（第 %d 次轮询）", currentPollCount)
		needDeleteDetection = true
	}

	var removedIDs []string
	if needDeleteDetection {
		removedIDs = []string{"__DETECT_DELETIONS__"}
	}

	// 有变更或需要删除检测时触发回调
	if len(changedItems) > 0 || len(removedIDs) > 0 {
		log.Printf("📡 媒体库变更检测: 新增/更新 %d, 删除检测 %v",
			len(changedItems), needDeleteDetection)
		w.mu.Lock()
		w.busy = true
		w.mu.Unlock()
		w.handler(changedItems, removedIDs)
		w.mu.Lock()
		w.busy = false
		w.mu.Unlock()
	} else {
		log.Printf("📡 轮询检查完成: 无变更 (Emby 总数: %d)", currentTotal)
	}
}

// GetTotalItemCount 获取 Emby 媒体总数的便捷方法（供外部使用）
func (w *LibraryWatcher) GetClient() *Client {
	return w.client
}
