package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// BufferedEvent 缓冲中的单个事件
type BufferedEvent struct {
	ItemID    string
	ItemType  string
	ItemName  string
	Operation string    // "add" 或 "delete"
	Timestamp time.Time
}

// EventBuffer 事件缓冲器，负责去抖和批量处理
type EventBuffer struct {
	mu            sync.Mutex
	events        map[string]*BufferedEvent // key: itemID，去重保留最新
	debounceTimer *time.Timer
	maxWaitTimer  *time.Timer
	firstEventAt  *time.Time // 当前窗口第一个事件的时间
	syncLock      *SyncLock
	cacheService  *CacheService
	getClient     func() (interface{}, error) // 延迟获取 emby client

	// 可配置参数
	DebounceDelay time.Duration // 默认 30s
	MaxWaitTime   time.Duration // 默认 5min
	MaxEvents     int           // 默认 5000

	// flush 回调，由外部注入实际处理逻辑
	flushHandler func(ctx context.Context, events []*BufferedEvent)
}

// NewEventBuffer 创建事件缓冲器
func NewEventBuffer(syncLock *SyncLock, flushHandler func(ctx context.Context, events []*BufferedEvent)) *EventBuffer {
	return &EventBuffer{
		events:        make(map[string]*BufferedEvent),
		syncLock:      syncLock,
		DebounceDelay: 30 * time.Second,
		MaxWaitTime:   5 * time.Minute,
		MaxEvents:     5000,
		flushHandler:  flushHandler,
	}
}

// Add 添加事件到缓冲区（线程安全）
func (eb *EventBuffer) Add(event *BufferedEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// 去重：同一 itemID 只保留最新事件
	eb.events[event.ItemID] = event

	// 记录当前窗口第一个事件的时间
	if eb.firstEventAt == nil {
		now := time.Now()
		eb.firstEventAt = &now
	}

	// 事件数量达到上限，立即触发处理
	if len(eb.events) >= eb.MaxEvents {
		log.Printf("📦 事件缓冲区达到上限 (%d)，立即触发处理", eb.MaxEvents)
		eb.triggerFlushLocked()
		return
	}

	// 重置去抖定时器
	if eb.debounceTimer != nil {
		eb.debounceTimer.Stop()
	}
	eb.debounceTimer = time.AfterFunc(eb.DebounceDelay, func() {
		eb.triggerFlush()
	})

	// 设置最大等待定时器（只在第一个事件时设置）
	if eb.maxWaitTimer == nil {
		eb.maxWaitTimer = time.AfterFunc(eb.MaxWaitTime, func() {
			log.Printf("📦 事件缓冲区达到最大等待时间 (%v)，触发处理", eb.MaxWaitTime)
			eb.triggerFlush()
		})
	}
}

// triggerFlush 触发 flush（从定时器回调调用）
func (eb *EventBuffer) triggerFlush() {
	eb.mu.Lock()
	events := eb.drainLocked()
	eb.mu.Unlock()

	if len(events) == 0 {
		return
	}

	log.Printf("📦 缓冲处理: %d 个事件", len(events))
	if eb.flushHandler != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		eb.flushHandler(ctx, events)
	}
}

// triggerFlushLocked 在已持有锁的情况下触发 flush
func (eb *EventBuffer) triggerFlushLocked() {
	events := eb.drainLocked()

	if len(events) == 0 {
		return
	}

	// 异步处理，避免在锁内阻塞
	go func() {
		log.Printf("📦 开始处理 %d 个缓冲事件", len(events))
		if eb.flushHandler != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			eb.flushHandler(ctx, events)
		}
	}()
}

// drainLocked 在已持有锁的情况下取出所有事件并重置状态
func (eb *EventBuffer) drainLocked() []*BufferedEvent {
	if len(eb.events) == 0 {
		return nil
	}

	events := make([]*BufferedEvent, 0, len(eb.events))
	for _, e := range eb.events {
		events = append(events, e)
	}

	// 重置状态
	eb.events = make(map[string]*BufferedEvent)
	eb.firstEventAt = nil
	if eb.debounceTimer != nil {
		eb.debounceTimer.Stop()
		eb.debounceTimer = nil
	}
	if eb.maxWaitTimer != nil {
		eb.maxWaitTimer.Stop()
		eb.maxWaitTimer = nil
	}

	return events
}

// Flush 立即处理所有缓冲事件（外部调用，如全量同步后协调）
func (eb *EventBuffer) Flush(ctx context.Context) {
	eb.mu.Lock()
	events := eb.drainLocked()
	eb.mu.Unlock()

	if len(events) == 0 {
		return
	}

	log.Printf("📦 手动 Flush: 处理 %d 个缓冲事件", len(events))
	if eb.flushHandler != nil {
		eb.flushHandler(ctx, events)
	}
}

// DrainEvents 取出所有缓冲事件但不处理（用于全量同步后协调）
func (eb *EventBuffer) DrainEvents() []*BufferedEvent {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.drainLocked()
}

// PendingCount 返回当前缓冲事件数量
func (eb *EventBuffer) PendingCount() int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return len(eb.events)
}
