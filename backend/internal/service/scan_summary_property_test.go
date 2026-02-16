package service

import (
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: backend-logging, Property 1: 扫描结果摘要日志完整性
// Validates: Requirements 3.3
// 对于任意 ScanResult（包含 TotalScanned、AnomalyCount、ErrorCount 三个非负整数），
// 格式化后的扫描完成日志字符串应同时包含这三个数值。
func TestProperty_FormatScanSummaryCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		scanType := rapid.SampledFrom([]string{"刮削异常", "重复媒体", "异常映射"}).Draw(t, "scanType")
		totalScanned := rapid.IntRange(0, 100000).Draw(t, "totalScanned")
		anomalyCount := rapid.IntRange(0, 100000).Draw(t, "anomalyCount")
		errorCount := rapid.IntRange(0, 100000).Draw(t, "errorCount")

		result := &ScanResult{
			TotalScanned: totalScanned,
			AnomalyCount: anomalyCount,
			ErrorCount:   errorCount,
		}

		summary := FormatScanSummary(scanType, result)

		// 验证：日志字符串包含扫描类型名称
		if !strings.Contains(summary, scanType) {
			t.Fatalf("摘要日志缺少扫描类型名称 %q: %s", scanType, summary)
		}

		// 验证：日志字符串包含三个数值
		if !strings.Contains(summary, fmt.Sprintf("%d", totalScanned)) {
			t.Fatalf("摘要日志缺少 TotalScanned=%d: %s", totalScanned, summary)
		}
		if !strings.Contains(summary, fmt.Sprintf("%d", anomalyCount)) {
			t.Fatalf("摘要日志缺少 AnomalyCount=%d: %s", anomalyCount, summary)
		}
		if !strings.Contains(summary, fmt.Sprintf("%d", errorCount)) {
			t.Fatalf("摘要日志缺少 ErrorCount=%d: %s", errorCount, summary)
		}
	})
}

// Feature: scan-performance, Property 5: 扫描摘要格式完整性
// Validates: Requirements 5.2
// 对于任意扫描类型字符串和非负字段的 ScanResult，
// FormatScanSummary 应产生包含扫描类型名称、总扫描数、异常数和错误数的字符串。
func TestProperty_ScanSummaryFormatCompleteness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机扫描类型（包括自定义字符串）
		scanType := rapid.StringMatching(`[A-Za-z\x{4e00}-\x{9fff}]{1,10}`).Draw(t, "scanType")
		totalScanned := rapid.IntRange(0, 100000).Draw(t, "totalScanned")
		anomalyCount := rapid.IntRange(0, totalScanned).Draw(t, "anomalyCount")
		errorCount := rapid.IntRange(0, 1000).Draw(t, "errorCount")

		result := &ScanResult{
			TotalScanned: totalScanned,
			AnomalyCount: anomalyCount,
			ErrorCount:   errorCount,
		}

		summary := FormatScanSummary(scanType, result)

		// 验证：日志字符串包含扫描类型名称
		if !strings.Contains(summary, scanType) {
			t.Fatalf("摘要日志缺少扫描类型名称 %q: %s", scanType, summary)
		}

		// 验证：日志字符串包含总扫描数
		if !strings.Contains(summary, fmt.Sprintf("%d", totalScanned)) {
			t.Fatalf("摘要日志缺少 TotalScanned=%d: %s", totalScanned, summary)
		}

		// 验证：日志字符串包含异常数
		if !strings.Contains(summary, fmt.Sprintf("%d", anomalyCount)) {
			t.Fatalf("摘要日志缺少 AnomalyCount=%d: %s", anomalyCount, summary)
		}

		// 验证：日志字符串包含错误数
		if !strings.Contains(summary, fmt.Sprintf("%d", errorCount)) {
			t.Fatalf("摘要日志缺少 ErrorCount=%d: %s", errorCount, summary)
		}
	})
}
