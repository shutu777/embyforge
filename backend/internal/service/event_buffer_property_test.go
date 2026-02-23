package service

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 3: 事件缓冲去重与分组
// Validates: Requirements 2.2, 2.3
//
// 对于任意 BufferedEvent 序列，DrainEvents 后每个 itemID 只保留一条（最新的），
// 且 add/delete 分组正确。
func TestProperty_EventBufferDeduplicationAndGrouping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 创建 EventBuffer（不需要 flushHandler，我们直接测试 Add + DrainEvents）
		eb := NewEventBuffer(&SyncLock{}, nil)

		// 生成随机事件序列
		eventCount := rapid.IntRange(1, 200).Draw(t, "eventCount")
		// 使用有限的 itemID 池来制造重复
		itemIDPool := rapid.IntRange(3, 20).Draw(t, "poolSize")

		// 记录每个 itemID 的最新事件
		expectedLatest := make(map[string]*BufferedEvent)

		for i := 0; i < eventCount; i++ {
			itemIdx := rapid.IntRange(1, itemIDPool).Draw(t, fmt.Sprintf("itemIdx_%d", i))
			itemID := fmt.Sprintf("item-%d", itemIdx)
			operation := rapid.SampledFrom([]string{"add", "delete"}).Draw(t, fmt.Sprintf("op_%d", i))
			itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%d", i))

			event := &BufferedEvent{
				ItemID:    itemID,
				ItemType:  itemType,
				ItemName:  fmt.Sprintf("Media_%d", itemIdx),
				Operation: operation,
				Timestamp: time.Now().Add(time.Duration(i) * time.Millisecond),
			}

			eb.Add(event)
			// 记录最新事件（后添加的覆盖先添加的）
			expectedLatest[itemID] = event
		}

		// DrainEvents 取出所有事件
		drained := eb.DrainEvents()

		// 验证 1: 每个 itemID 只出现一次
		seenIDs := make(map[string]bool)
		for _, e := range drained {
			if seenIDs[e.ItemID] {
				t.Fatalf("DrainEvents 中 itemID %q 出现多次（应去重）", e.ItemID)
			}
			seenIDs[e.ItemID] = true
		}

		// 验证 2: 事件数量等于唯一 itemID 数量
		if len(drained) != len(expectedLatest) {
			t.Fatalf("DrainEvents 返回 %d 个事件, 期望 %d 个唯一 itemID",
				len(drained), len(expectedLatest))
		}

		// 验证 3: 每个事件的 operation 与最新添加的一致
		for _, e := range drained {
			expected, ok := expectedLatest[e.ItemID]
			if !ok {
				t.Fatalf("DrainEvents 包含未预期的 itemID %q", e.ItemID)
			}
			if e.Operation != expected.Operation {
				t.Fatalf("itemID %q 的 Operation = %q, 期望 %q（应保留最新）",
					e.ItemID, e.Operation, expected.Operation)
			}
		}

		// 验证 4: DrainEvents 后缓冲区为空
		if eb.PendingCount() != 0 {
			t.Fatalf("DrainEvents 后 PendingCount() = %d, 期望 0", eb.PendingCount())
		}

		// 验证 5: 事件可以正确分组为 add 和 delete
		var addCount, deleteCount int
		for _, e := range drained {
			switch e.Operation {
			case "add":
				addCount++
			case "delete":
				deleteCount++
			default:
				t.Fatalf("未知 Operation: %q", e.Operation)
			}
		}
		if addCount+deleteCount != len(drained) {
			t.Fatalf("add(%d) + delete(%d) != total(%d)", addCount, deleteCount, len(drained))
		}
	})
}
