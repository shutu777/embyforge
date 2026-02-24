package service

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// Feature: emby-webhook-sync, Property 11: Cron 表达式验证
// Validates: Requirements 5.2
//
// 对于任意合法的 cron 表达式，ValidateCronExpr 应返回 true；
// 对于任意非法字符串，ValidateCronExpr 应返回 false。
func TestProperty_ValidateCronExpr(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成合法的 5 段 cron 表达式
		minute := rapid.IntRange(0, 59).Draw(t, "minute")
		hour := rapid.IntRange(0, 23).Draw(t, "hour")
		expr := fmt.Sprintf("%d %d * * *", minute, hour)

		if !ValidateCronExpr(expr) {
			t.Fatalf("ValidateCronExpr(%q) = false，期望 true", expr)
		}
	})
}

func TestProperty_ValidateCronExprRejectsInvalid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机字符串，大概率不是合法 cron 表达式
		s := rapid.StringMatching(`[a-z]{3,10}`).Draw(t, "invalid")
		if ValidateCronExpr(s) {
			t.Fatalf("ValidateCronExpr(%q) = true，期望 false", s)
		}
	})
}

func TestProperty_DefaultCronExprIsValid(t *testing.T) {
	if !ValidateCronExpr(DefaultCronExpr) {
		t.Fatalf("默认 cron 表达式 %q 无效", DefaultCronExpr)
	}
}
