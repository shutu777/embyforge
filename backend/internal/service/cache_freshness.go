package service

import (
	"context"
	"log"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"
)

// CacheFreshnessResult 缓存新鲜度检查结果
type CacheFreshnessResult struct {
	IsStale    bool       `json:"is_stale"`
	LocalCount int64      `json:"local_count"`
	EmbyCount  int        `json:"emby_count"`
	LastSyncAt *time.Time `json:"last_sync_at"`
}

// IsCacheStale 判断缓存是否过期（纯函数，便于测试）
// 规则：数量不一致 AND 最后同步超过 10 分钟 → 过期
func IsCacheStale(localCount int64, embyCount int, lastSync time.Time) bool {
	if localCount == int64(embyCount) {
		return false
	}
	// 数量不一致，检查时间
	return time.Since(lastSync) > 10*time.Minute
}

// CheckCacheFreshness 检查缓存新鲜度
func (s *CacheService) CheckCacheFreshness(ctx context.Context, client *emby.Client) *CacheFreshnessResult {
	result := &CacheFreshnessResult{}

	// 查询本地 media_caches 总数
	if err := s.DB.Model(&model.MediaCache{}).Count(&result.LocalCount).Error; err != nil {
		log.Printf("⚠️ 缓存新鲜度检查: 查询本地总数失败: %v", err)
		return result
	}

	// 查询最后同步时间
	var lastCache model.MediaCache
	if err := s.DB.Order("cached_at DESC").First(&lastCache).Error; err == nil {
		result.LastSyncAt = &lastCache.CachedAt
	}

	// 查询 Emby 总数（轻量 API 调用，Limit=0）
	embyCount, err := client.GetTotalItemCount(ctx)
	if err != nil {
		log.Printf("⚠️ 缓存新鲜度检查: 查询 Emby 总数失败: %v", err)
		return result
	}
	result.EmbyCount = embyCount

	// 判断是否过期
	if result.LastSyncAt != nil {
		result.IsStale = IsCacheStale(result.LocalCount, result.EmbyCount, *result.LastSyncAt)
	} else if result.LocalCount == 0 && embyCount > 0 {
		// 从未同步过
		result.IsStale = true
	}

	return result
}
