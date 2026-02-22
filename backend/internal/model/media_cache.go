package model

import (
	"encoding/json"
	"time"

	"embyforge/internal/emby"
)

// MediaCache 媒体缓存模型
type MediaCache struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	EmbyItemID        string    `gorm:"size:50;not null;uniqueIndex" json:"emby_item_id"`
	Name              string    `gorm:"size:500;not null" json:"name"`
	Type              string    `gorm:"size:50;not null;index" json:"type"`
	HasPoster         bool      `gorm:"not null;default:false" json:"has_poster"`
	Path              string    `gorm:"size:1000" json:"path"`
	ProviderIDs       string    `gorm:"type:text" json:"provider_ids"` // JSON string
	FileSize          int64     `gorm:"default:0" json:"file_size"`
	IndexNumber       int       `gorm:"default:0" json:"index_number"`
	ParentIndexNumber int       `gorm:"default:0" json:"parent_index_number"` // 季号
	ChildCount        int       `gorm:"default:0" json:"child_count"`
	SeriesID          string    `gorm:"size:50;default:'';index" json:"series_id"`     // 所属 Series 的 Emby ID
	SeriesName        string    `gorm:"size:500;default:''" json:"series_name"`  // 所属 Series 名称
	LibraryName       string    `gorm:"size:255" json:"library_name"`
	CachedAt          time.Time `gorm:"not null" json:"cached_at"`
}

// CacheStatus 缓存状态
type CacheStatus struct {
	TotalItems   int64      `json:"total_items"`
	TotalSeasons int64      `json:"total_seasons"`
	LastSyncAt   *time.Time `json:"last_sync_at"`
}

// NewMediaCacheFromItem 从 emby.MediaItem 创建 MediaCache
func NewMediaCacheFromItem(item emby.MediaItem, libraryName string) MediaCache {
	providerJSON := "{}"
	if item.ProviderIds != nil {
		if data, err := json.Marshal(item.ProviderIds); err == nil {
			providerJSON = string(data)
		}
	}

	_, hasPrimary := item.ImageTags["Primary"]

	return MediaCache{
		EmbyItemID:        item.ID,
		Name:              item.Name,
		Type:              item.Type,
		HasPoster:         hasPrimary,
		Path:              item.Path,
		ProviderIDs:       providerJSON,
		FileSize:          item.FileSize,
		IndexNumber:       item.IndexNumber,
		ParentIndexNumber: item.ParentIndexNumber,
		ChildCount:        item.ChildCount,
		SeriesID:          item.SeriesID,
		SeriesName:        item.SeriesName,
		LibraryName:       libraryName,
		CachedAt:          time.Now(),
	}
}

// ToMediaItem 将 MediaCache 转换为 emby.MediaItem
func (mc *MediaCache) ToMediaItem() emby.MediaItem {
	providerIds := make(map[string]string)
	if mc.ProviderIDs != "" {
		_ = json.Unmarshal([]byte(mc.ProviderIDs), &providerIds)
	}

	imageTags := make(map[string]string)
	if mc.HasPoster {
		imageTags["Primary"] = "cached"
	}

	return emby.MediaItem{
		ID:                mc.EmbyItemID,
		Name:              mc.Name,
		Type:              mc.Type,
		ImageTags:         imageTags,
		Path:              mc.Path,
		ProviderIds:       providerIds,
		FileSize:          mc.FileSize,
		IndexNumber:       mc.IndexNumber,
		ParentIndexNumber: mc.ParentIndexNumber,
		ChildCount:        mc.ChildCount,
		SeriesID:          mc.SeriesID,
		SeriesName:        mc.SeriesName,
	}
}
