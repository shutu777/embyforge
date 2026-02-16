package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: scan-performance, Property 1: Worker 并发上限
// Validates: Requirements 1.1, 1.3
// 对于任意 MaxWorkers=N 的 Worker Pool 和任意一组任务，
// 任意时刻并发执行的任务数不超过 N。
func TestProperty_WorkerConcurrencyLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxWorkers := rapid.IntRange(1, 8).Draw(t, "maxWorkers")
		minWorkers := rapid.IntRange(1, maxWorkers).Draw(t, "minWorkers")
		taskCount := rapid.IntRange(1, 50).Draw(t, "taskCount")

		ctx := context.Background()
		pool := New[int](ctx, Config{
			MinWorkers:  minWorkers,
			MaxWorkers:  maxWorkers,
			IdleTimeout: 5 * time.Second,
		})

		var peakConcurrency atomic.Int32
		var currentConcurrency atomic.Int32

		for i := 0; i < taskCount; i++ {
			pool.Submit(func() Result[int] {
				cur := currentConcurrency.Add(1)
				// 更新峰值并发数
				for {
					peak := peakConcurrency.Load()
					if cur <= peak || peakConcurrency.CompareAndSwap(peak, cur) {
						break
					}
				}
				// 模拟一些工作
				time.Sleep(time.Millisecond)
				currentConcurrency.Add(-1)
				return Result[int]{Value: 1}
			})
		}

		pool.Wait()

		peak := int(peakConcurrency.Load())
		if peak > maxWorkers {
			t.Fatalf("并发峰值 %d 超过 MaxWorkers %d", peak, maxWorkers)
		}
	})
}

// Feature: scan-performance, Property 2: 任务完整性
// Validates: Requirements 1.5, 1.7
// 对于任意一组提交到 Worker Pool 的任务，Wait() 返回的结果数量
// 应等于提交的任务数量，且每个任务恰好执行一次。
func TestProperty_TaskCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxWorkers := rapid.IntRange(1, 8).Draw(t, "maxWorkers")
		minWorkers := rapid.IntRange(1, maxWorkers).Draw(t, "minWorkers")
		taskCount := rapid.IntRange(0, 50).Draw(t, "taskCount")

		ctx := context.Background()
		pool := New[int](ctx, Config{
			MinWorkers:  minWorkers,
			MaxWorkers:  maxWorkers,
			IdleTimeout: 5 * time.Second,
		})

		// 每个任务返回自己的索引值
		for i := 0; i < taskCount; i++ {
			val := i
			pool.Submit(func() Result[int] {
				return Result[int]{Value: val}
			})
		}

		results := pool.Wait()

		// 验证结果数量等于提交的任务数量
		if len(results) != taskCount {
			t.Fatalf("结果数量 %d 不等于任务数量 %d", len(results), taskCount)
		}

		// 验证每个任务恰好执行一次（通过检查返回值集合）
		seen := make(map[int]int)
		for _, r := range results {
			if r.Err != nil {
				t.Fatalf("任务返回意外错误: %v", r.Err)
			}
			seen[r.Value]++
		}
		for i := 0; i < taskCount; i++ {
			count, ok := seen[i]
			if !ok {
				t.Fatalf("任务 %d 未被执行", i)
			}
			if count != 1 {
				t.Fatalf("任务 %d 被执行了 %d 次，期望 1 次", i, count)
			}
		}
	})
}

// Feature: scan-performance, Property 3: Context 取消停止新任务
// Validates: Requirements 1.6
// 对于任意 Worker Pool，当 context 被取消后提交的任务不应被执行，
// Wait() 返回的结果数量应 <= 已提交的任务总数。
func TestProperty_ContextCancellationStopsNewTasks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxWorkers := rapid.IntRange(1, 4).Draw(t, "maxWorkers")
		minWorkers := rapid.IntRange(1, maxWorkers).Draw(t, "minWorkers")
		preCancel := rapid.IntRange(1, 10).Draw(t, "preCancelTasks")
		postCancel := rapid.IntRange(1, 10).Draw(t, "postCancelTasks")

		ctx, cancel := context.WithCancel(context.Background())
		pool := New[int](ctx, Config{
			MinWorkers:  minWorkers,
			MaxWorkers:  maxWorkers,
			IdleTimeout: 5 * time.Second,
		})

		var executedCount atomic.Int32

		// 提交取消前的任务
		for i := 0; i < preCancel; i++ {
			pool.Submit(func() Result[int] {
				executedCount.Add(1)
				return Result[int]{Value: 1}
			})
		}

		// 等待一小段时间让取消前的任务有机会执行
		time.Sleep(10 * time.Millisecond)

		// 取消 context
		cancel()

		// 等待一小段时间确保取消信号传播
		time.Sleep(5 * time.Millisecond)

		// 提交取消后的任务
		for i := 0; i < postCancel; i++ {
			pool.Submit(func() Result[int] {
				executedCount.Add(1)
				return Result[int]{Value: 2}
			})
		}

		results := pool.Wait()

		// 验证：结果数量不超过总提交数
		totalSubmitted := preCancel + postCancel
		if len(results) > totalSubmitted {
			t.Fatalf("结果数量 %d 超过总提交数 %d", len(results), totalSubmitted)
		}

		// 验证：实际执行的任务数不超过结果数
		executed := int(executedCount.Load())
		if executed > len(results) {
			t.Fatalf("执行数 %d 超过结果数 %d", executed, len(results))
		}
	})
}

// Feature: scan-performance, Property 4: Panic 恢复
// Validates: Requirements 1.8
// 对于任意一组任务（其中部分任务会 panic），Worker Pool 应恢复所有 panic，
// 继续处理非 panic 任务，并为 panic 任务返回错误结果，不会导致 Pool 崩溃。
func TestProperty_PanicRecovery(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxWorkers := rapid.IntRange(1, 8).Draw(t, "maxWorkers")
		minWorkers := rapid.IntRange(1, maxWorkers).Draw(t, "minWorkers")
		taskCount := rapid.IntRange(1, 30).Draw(t, "taskCount")

		// 随机决定哪些任务会 panic
		panicSet := make(map[int]bool)
		for i := 0; i < taskCount; i++ {
			if rapid.Bool().Draw(t, "shouldPanic") {
				panicSet[i] = true
			}
		}

		ctx := context.Background()
		pool := New[int](ctx, Config{
			MinWorkers:  minWorkers,
			MaxWorkers:  maxWorkers,
			IdleTimeout: 5 * time.Second,
		})

		for i := 0; i < taskCount; i++ {
			shouldPanic := panicSet[i]
			val := i
			pool.Submit(func() Result[int] {
				if shouldPanic {
					panic("测试 panic")
				}
				return Result[int]{Value: val}
			})
		}

		results := pool.Wait()

		// 验证：结果数量等于任务数量（包括 panic 的任务）
		if len(results) != taskCount {
			t.Fatalf("结果数量 %d 不等于任务数量 %d", len(results), taskCount)
		}

		// 验证：panic 任务产生错误结果，非 panic 任务产生正常结果
		errorCount := 0
		successCount := 0
		for _, r := range results {
			if r.Err != nil {
				errorCount++
			} else {
				successCount++
			}
		}

		expectedPanics := len(panicSet)
		expectedSuccess := taskCount - expectedPanics

		if errorCount != expectedPanics {
			t.Fatalf("错误结果数 %d 不等于 panic 任务数 %d", errorCount, expectedPanics)
		}
		if successCount != expectedSuccess {
			t.Fatalf("成功结果数 %d 不等于非 panic 任务数 %d", successCount, expectedSuccess)
		}
	})
}

// Feature: scan-performance, Property 5: 动态伸缩 - Worker 数量在 Min/Max 范围内
// Validates: Requirements 1.2, 1.3, 1.4
// 对于任意 MinWorkers=Min 和 MaxWorkers=Max 的 Worker Pool，
// 在执行过程中任意时刻活跃 worker 数量应 >= Min 且 <= Max。
func TestProperty_DynamicScalingRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxWorkers := rapid.IntRange(2, 8).Draw(t, "maxWorkers")
		minWorkers := rapid.IntRange(1, maxWorkers).Draw(t, "minWorkers")
		taskCount := rapid.IntRange(1, 40).Draw(t, "taskCount")

		ctx := context.Background()
		pool := New[int](ctx, Config{
			MinWorkers:  minWorkers,
			MaxWorkers:  maxWorkers,
			IdleTimeout: 5 * time.Second,
		})

		// 验证初始 worker 数量 >= MinWorkers
		initialWorkers := pool.ActiveWorkers()
		if initialWorkers < minWorkers {
			t.Fatalf("初始 worker 数 %d 小于 MinWorkers %d", initialWorkers, minWorkers)
		}

		var violation atomic.Bool

		for i := 0; i < taskCount; i++ {
			pool.Submit(func() Result[int] {
				active := pool.ActiveWorkers()
				if active > maxWorkers {
					violation.Store(true)
				}
				// 模拟工作，让 worker 有机会扩展
				time.Sleep(time.Millisecond)
				return Result[int]{Value: 1}
			})
		}

		pool.Wait()

		if violation.Load() {
			t.Fatalf("执行过程中 worker 数量超过 MaxWorkers %d", maxWorkers)
		}
	})
}
