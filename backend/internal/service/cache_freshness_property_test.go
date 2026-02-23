package service

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 12: 缓存新鲜度判定
// Validates: Requirements 6.2
//
// 对于任意 localCount、embyCount 和 lastSync 组合，
// IsCacheStale 应在数量不一致 AND 最后同步超过 10 分钟时返回 true，否则返回 false。
func TestProperty_CacheFreshnessJudgment(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		localCount := rapid.Int64Range(0, 100000).Draw(t, "localCount")
		embyCount := rapid.IntRange(0, 100000).Draw(t, "embyCount")
		// 生成 lastSync 时间：距今 0~60 分钟
		minutesAgo := rapid.IntRange(0, 60).Draw(t, "minutesAgo")
		lastSync := time.Now().Add(-time.Duration(minutesAgo) * time.Minute)

		result := IsCacheStale(localCount, embyCount, lastSync)

		countsDiffer := localCount != int64(embyCount)
		olderThan10Min := minutesAgo > 10

		expected := countsDiffer && olderThan10Min

		if result != expected {
			t.Fatalf("IsCacheStale(local=%d, emby=%d, %d分钟前) = %v, 期望 %v (数量不一致=%v, 超过10分钟=%v)",
				localCount, embyCount, minutesAgo, result, expected, countsDiffer, olderThan10Min)
		}
	})
}
