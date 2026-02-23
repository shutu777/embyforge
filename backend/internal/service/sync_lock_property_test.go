package service

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 8: 同步锁互斥性
// Validates: Requirements 4.2
//
// 对于任意 TryLock/Unlock 操作序列，同一时刻最多只有一个持有者能持有锁。
// TryLock 在锁已被持有时应返回 false。
func TestProperty_SyncLockMutualExclusion(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		sl := &SyncLock{}

		// 生成随机操作序列
		opCount := rapid.IntRange(1, 100).Draw(t, "opCount")
		holders := []string{"full_sync", "delta_update", "cron_full_sync", "test_holder"}

		locked := false
		currentHolder := ""

		for i := 0; i < opCount; i++ {
			// 随机选择操作：0=TryLock, 1=Unlock, 2=IsLocked, 3=Holder
			op := rapid.IntRange(0, 3).Draw(t, "op")

			switch op {
			case 0: // TryLock
				holder := rapid.SampledFrom(holders).Draw(t, "holder")
				got := sl.TryLock(holder)

				if locked {
					// 锁已被持有，TryLock 应返回 false
					if got {
						t.Fatalf("锁已被 %q 持有，但 TryLock(%q) 返回 true", currentHolder, holder)
					}
				} else {
					// 锁未被持有，TryLock 应返回 true
					if !got {
						t.Fatalf("锁未被持有，但 TryLock(%q) 返回 false", holder)
					}
					locked = true
					currentHolder = holder
				}

			case 1: // Unlock
				sl.Unlock()
				locked = false
				currentHolder = ""

			case 2: // IsLocked
				got := sl.IsLocked()
				if got != locked {
					t.Fatalf("IsLocked() = %v, 期望 %v", got, locked)
				}

			case 3: // Holder
				got := sl.Holder()
				if got != currentHolder {
					t.Fatalf("Holder() = %q, 期望 %q", got, currentHolder)
				}
			}
		}
	})
}
