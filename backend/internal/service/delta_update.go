package service

import (
	"context"
	"log"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"
)

// ProcessDeltaEvents 批量处理缓冲的 Webhook 事件
// 1. 获取 SyncLock
// 2. 按操作类型分组（add/update vs delete）
// 3. 批量查询 Emby API 获取最新数据
// 4. 批量 upsert/delete 本地缓存
// 5. 重建受影响的季缓存
// 6. 释放 SyncLock
func (s *CacheService) ProcessDeltaEvents(ctx context.Context, client *emby.Client, syncLock *SyncLock, events []*BufferedEvent) error {
	if len(events) == 0 {
		return nil
	}

	// 尝试获取同步锁
	if !syncLock.TryLock("delta_update") {
		log.Printf("⚠️ 增量同步: 同步锁被占用 (%s)，跳过本次处理", syncLock.Holder())
		return nil
	}
	defer syncLock.Unlock()

	start := time.Now()

	// 按操作类型分组
	var addEvents, deleteEvents []*BufferedEvent
	for _, e := range events {
		switch e.Operation {
		case "add":
			addEvents = append(addEvents, e)
		case "delete":
			deleteEvents = append(deleteEvents, e)
		}
	}

	log.Printf("🔄 增量同步: 处理 %d 个事件 (新增: %d, 删除: %d)",
		len(events), len(addEvents), len(deleteEvents))

	// 获取底层 sql.DB
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}

	// 收集受影响的 SeriesID（用于重建季缓存）
	affectedSeries := make(map[string]bool)

	// 处理新增/更新事件：查询 Emby API 获取最新数据，upsert 到本地
	if len(addEvents) > 0 {
		var caches []model.MediaCache
		for _, e := range addEvents {
			items, err := client.GetItemByID(ctx, e.ItemID)
			if err != nil {
				log.Printf("⚠️ 增量同步: 查询 item %s 失败，跳过: %v", e.ItemID, err)
				// 重试一次
				items, err = client.GetItemByID(ctx, e.ItemID)
				if err != nil {
					continue
				}
			}

			if len(items) == 0 {
				// Emby 返回空，视为已删除
				deleteEvents = append(deleteEvents, &BufferedEvent{
					ItemID:    e.ItemID,
					Operation: "delete",
				})
				continue
			}

			item := items[0]
			if !IsRelevantItemType(item.Type) {
				continue
			}

			cache := model.NewMediaCacheFromItem(item, "")
			caches = append(caches, cache)

			// 收集受影响的 Series
			if item.Type == "Episode" && item.SeriesID != "" {
				affectedSeries[item.SeriesID] = true
			} else if item.Type == "Series" {
				affectedSeries[item.ID] = true
			}
		}

		// 批量 upsert
		if len(caches) > 0 {
			if err := rawUpsertMediaCaches(sqlDB, caches); err != nil {
				log.Printf("⚠️ 增量同步: 批量写入失败: %v", err)
			}
		}
	}

	// 处理删除事件
	if len(deleteEvents) > 0 {
		ids := make([]string, 0, len(deleteEvents))
		for _, e := range deleteEvents {
			ids = append(ids, e.ItemID)

			// 收集受影响的 Series（删除前查询本地缓存获取 SeriesID）
			if e.ItemType == "Episode" {
				var mc model.MediaCache
				if err := s.DB.Where("emby_item_id = ?", e.ItemID).First(&mc).Error; err == nil {
					if mc.SeriesID != "" {
						affectedSeries[mc.SeriesID] = true
					}
				}
			}
		}

		// 批量删除
		const deleteBatch = 500
		for i := 0; i < len(ids); i += deleteBatch {
			end := i + deleteBatch
			if end > len(ids) {
				end = len(ids)
			}
			if err := s.DB.Where("emby_item_id IN ?", ids[i:end]).Delete(&model.MediaCache{}).Error; err != nil {
				log.Printf("⚠️ 增量同步: 批量删除失败: %v", err)
			}
		}
	}

	// 重建受影响的季缓存
	if len(affectedSeries) > 0 {
		seriesIDs := make([]string, 0, len(affectedSeries))
		for sid := range affectedSeries {
			seriesIDs = append(seriesIDs, sid)
		}
		seasonCount, err := s.rebuildSeasonCacheForSeries(sqlDB, seriesIDs)
		if err != nil {
			log.Printf("⚠️ 增量同步: 重建季缓存失败: %v", err)
		} else if seasonCount > 0 {
			log.Printf("📊 增量同步: 重建了 %d 个剧集的季缓存", len(seriesIDs))
		}
	}

	log.Printf("✅ 增量同步完成: 新增 %d, 删除 %d, 耗时 %dms",
		len(addEvents), len(deleteEvents), time.Since(start).Milliseconds())
	return nil
}

// ReconcileBufferedEvents 全量同步后协调缓冲事件
// 过滤已存在的 add 事件，保留 delete 和新 add
func (s *CacheService) ReconcileBufferedEvents(ctx context.Context, client *emby.Client, syncLock *SyncLock, events []*BufferedEvent) {
	if len(events) == 0 {
		return
	}

	log.Printf("🔄 开始协调 %d 个缓冲事件", len(events))

	// 分组
	var reconciled []*BufferedEvent
	for _, e := range events {
		if e.Operation == "delete" {
			// 删除事件：检查本地是否存在，存在则保留
			var count int64
			s.DB.Model(&model.MediaCache{}).Where("emby_item_id = ?", e.ItemID).Count(&count)
			if count > 0 {
				reconciled = append(reconciled, e)
			}
		} else if e.Operation == "add" {
			// 新增事件：检查本地是否已存在，不存在则保留
			var count int64
			s.DB.Model(&model.MediaCache{}).Where("emby_item_id = ?", e.ItemID).Count(&count)
			if count == 0 {
				reconciled = append(reconciled, e)
			}
		}
	}

	if len(reconciled) == 0 {
		log.Printf("✅ 协调完成: 所有缓冲事件已被全量同步覆盖，无需额外处理")
		return
	}

	log.Printf("🔄 协调后需处理 %d 个事件 (原始 %d 个)", len(reconciled), len(events))
	if err := s.ProcessDeltaEvents(ctx, client, syncLock, reconciled); err != nil {
		log.Printf("⚠️ 协调处理失败: %v", err)
	}
}
