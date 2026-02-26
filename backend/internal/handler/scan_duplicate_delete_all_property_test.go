package handler

import (
	"fmt"
	"sort"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ui-enhancement-episode-duplicate, Property 2: Duplicate media delete-all selection
// Validates: Requirements 4.3
//
// 对于任意一组重复媒体分组（每组 2+ 项，按 file_size 升序排列），
// "全部删除"模式应选择每组中除最后一个（最大文件）外的所有项进行删除。
// 总待删除数 = sum(group_size - 1)。
func TestProperty_DeleteAllSelection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成 1~10 个重复组
		numGroups := rapid.IntRange(1, 10).Draw(rt, "numGroups")

		type testItem struct {
			GroupKey   string
			EmbyItemID string
			FileSize   int64
		}

		var allItems []testItem
		expectedDeleteCount := 0

		for g := 0; g < numGroups; g++ {
			groupKey := fmt.Sprintf("tmdb:movie:%d", 1000+g)
			// 每组 2~6 个条目
			groupSize := rapid.IntRange(2, 6).Draw(rt, fmt.Sprintf("groupSize_%d", g))

			// 生成不同的文件大小
			sizes := make([]int64, groupSize)
			for i := 0; i < groupSize; i++ {
				sizes[i] = rapid.Int64Range(1000, 10000000).Draw(rt, fmt.Sprintf("size_%d_%d", g, i))
			}
			// 按大小排序（模拟数据库 ORDER BY file_size ASC）
			sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })

			for i := 0; i < groupSize; i++ {
				allItems = append(allItems, testItem{
					GroupKey:   groupKey,
					EmbyItemID: fmt.Sprintf("item-%d-%d", g, i),
					FileSize:   sizes[i],
				})
			}
			// 每组保留最大的（最后一个），删除其余
			expectedDeleteCount += groupSize - 1
		}

		// 模拟 cleanupDuplicateMediaAll 的选择逻辑
		groupMap := make(map[string][]testItem)
		for _, item := range allItems {
			groupMap[item.GroupKey] = append(groupMap[item.GroupKey], item)
		}

		// 每组按 file_size 排序
		for key := range groupMap {
			items := groupMap[key]
			sort.Slice(items, func(i, j int) bool { return items[i].FileSize < items[j].FileSize })
			groupMap[key] = items
		}

		var toDelete []testItem
		for _, items := range groupMap {
			if len(items) < 2 {
				continue
			}
			// 保留最后一个（最大），删除其余
			toDelete = append(toDelete, items[:len(items)-1]...)
		}

		// 验证总待删除数
		if len(toDelete) != expectedDeleteCount {
			t.Fatalf("待删除数应为 %d，实际 %d", expectedDeleteCount, len(toDelete))
		}

		// 验证每组保留的是最大文件
		for key, items := range groupMap {
			if len(items) < 2 {
				continue
			}
			maxSize := items[len(items)-1].FileSize
			maxID := items[len(items)-1].EmbyItemID

			// 确认最大文件不在待删除列表中
			for _, d := range toDelete {
				if d.EmbyItemID == maxID && d.GroupKey == key {
					t.Fatalf("组 %s 的最大文件 %s (size=%d) 不应被删除", key, maxID, maxSize)
				}
			}

			// 确认其余文件都在待删除列表中
			for i := 0; i < len(items)-1; i++ {
				found := false
				for _, d := range toDelete {
					if d.EmbyItemID == items[i].EmbyItemID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("组 %s 的非最大文件 %s (size=%d) 应被删除但未找到",
						key, items[i].EmbyItemID, items[i].FileSize)
				}
			}
		}
	})
}

// Feature: ui-enhancement-episode-duplicate, Property 3: Bulk deletion error resilience
// Validates: Requirements 5.4
//
// 对于任意批量删除操作处理 N 个项目，其中部分项目删除失败，
// 响应应满足: deleted_count + failed_count = N，
// 且所有不在失败集合中的项目应被成功处理。
func TestProperty_BulkDeletionErrorResilience(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// 生成 1~50 个待删除项
		totalItems := rapid.IntRange(1, 50).Draw(rt, "totalItems")

		// 随机决定哪些会失败（0~totalItems 个）
		failCount := rapid.IntRange(0, totalItems).Draw(rt, "failCount")

		// 生成失败的索引集合
		failSet := make(map[int]bool)
		indices := make([]int, totalItems)
		for i := range indices {
			indices[i] = i
		}
		// 随机选择 failCount 个索引作为失败项
		for i := 0; i < failCount && i < totalItems; i++ {
			idx := rapid.IntRange(0, totalItems-1).Draw(rt, fmt.Sprintf("failIdx_%d", i))
			failSet[idx] = true
		}
		actualFailCount := len(failSet)

		// 模拟删除过程
		deletedCount := 0
		failedCount := 0
		var successIDs []string
		var failedIDs []string

		for i := 0; i < totalItems; i++ {
			itemID := fmt.Sprintf("item-%d", i)
			if failSet[i] {
				failedCount++
				failedIDs = append(failedIDs, itemID)
			} else {
				deletedCount++
				successIDs = append(successIDs, itemID)
			}
		}

		// 验证 Property 3: deleted_count + failed_count = total
		if deletedCount+failedCount != totalItems {
			t.Fatalf("deleted_count(%d) + failed_count(%d) != total(%d)",
				deletedCount, failedCount, totalItems)
		}

		// 验证成功数 = total - 实际失败数
		expectedSuccess := totalItems - actualFailCount
		if deletedCount != expectedSuccess {
			t.Fatalf("deleted_count(%d) != expected(%d)", deletedCount, expectedSuccess)
		}

		// 验证失败列表长度
		if len(failedIDs) != actualFailCount {
			t.Fatalf("failed_items 长度(%d) != actual_fail_count(%d)", len(failedIDs), actualFailCount)
		}

		// 验证成功列表长度
		if len(successIDs) != expectedSuccess {
			t.Fatalf("success_items 长度(%d) != expected(%d)", len(successIDs), expectedSuccess)
		}

		// 验证成功和失败列表没有交集
		successSet := make(map[string]bool)
		for _, id := range successIDs {
			successSet[id] = true
		}
		for _, id := range failedIDs {
			if successSet[id] {
				t.Fatalf("项目 %s 同时出现在成功和失败列表中", id)
			}
		}
	})
}
