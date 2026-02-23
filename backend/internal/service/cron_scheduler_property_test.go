package service

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 11: Cron 间隔范围约束
// Validates: Requirements 5.2
//
// 对于任意整数输入，ClampCronInterval 应返回 [1, 168] 范围内的值。
// 小于 1 的值应被 clamp 到 1，大于 168 的值应被 clamp 到 168。
func TestProperty_ClampCronIntervalRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.IntRange(-1000, 1000).Draw(t, "input")
		result := ClampCronInterval(input)

		// 验证结果在 [1, 168] 范围内
		if result < 1 || result > 168 {
			t.Fatalf("ClampCronInterval(%d) = %d, 超出 [1, 168] 范围", input, result)
		}

		// 验证 clamp 逻辑
		if input < 1 && result != 1 {
			t.Fatalf("ClampCronInterval(%d) = %d, 期望 1", input, result)
		}
		if input > 168 && result != 168 {
			t.Fatalf("ClampCronInterval(%d) = %d, 期望 168", input, result)
		}
		if input >= 1 && input <= 168 && result != input {
			t.Fatalf("ClampCronInterval(%d) = %d, 期望 %d（在有效范围内应原样返回）", input, result, input)
		}
	})
}
