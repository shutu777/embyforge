package service

import (
	"sync"
	"time"
)

// SyncLock 同步操作互斥锁，确保同一时刻只有一个同步操作在执行
type SyncLock struct {
	mu       sync.Mutex
	locked   bool
	holder   string    // "full_sync" 或 "delta_update"
	lockedAt time.Time
}

// TryLock 尝试获取锁，返回是否成功
func (sl *SyncLock) TryLock(holder string) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.locked {
		return false
	}
	sl.locked = true
	sl.holder = holder
	sl.lockedAt = time.Now()
	return true
}

// Unlock 释放锁
func (sl *SyncLock) Unlock() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.locked = false
	sl.holder = ""
	sl.lockedAt = time.Time{}
}

// IsLocked 检查是否已锁定
func (sl *SyncLock) IsLocked() bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.locked
}

// Holder 返回当前持有者
func (sl *SyncLock) Holder() string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.holder
}
