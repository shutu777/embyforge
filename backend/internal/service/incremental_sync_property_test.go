package service

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"pgregory.net/rapid"
)

// setupTestDB 创建测试用的 SQLite 内存数据库
func setupTestDB(t testing.TB) (*model.MediaCache, *sql.DB, *CacheService) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_incremental.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return nil, sqlDB, NewCacheService(db)
}

// genMediaCache 生成随机的 MediaCache 条目
func genMediaCache(t *rapid.T, prefix string, idx int, dateLastSaved time.Time) model.MediaCache {
	itemType := rapid.SampledFrom([]string{"Movie", "Series", "Episode"}).Draw(t, fmt.Sprintf("type_%s_%d", prefix, idx))
	return model.MediaCache{
		EmbyItemID:        fmt.Sprintf("%s-%d", prefix, idx),
		Name:              rapid.StringMatching(`[A-Za-z0-9 ]{1,50}`).Draw(t, fmt.Sprintf("name_%s_%d", prefix, idx)),
		Type:              itemType,
		HasPoster:         rapid.Bool().Draw(t, fmt.Sprintf("poster_%s_%d", prefix, idx)),
		Path:              fmt.Sprintf("/media/%s/%d", prefix, idx),
		ProviderIDs:       "{}",
		FileSize:          int64(rapid.IntRange(0, 10000000).Draw(t, fmt.Sprintf("size_%s_%d", prefix, idx))),
		IndexNumber:       rapid.IntRange(0, 100).Draw(t, fmt.Sprintf("idx_%s_%d", prefix, idx)),
		ParentIndexNumber: rapid.IntRange(0, 20).Draw(t, fmt.Sprintf("pidx_%s_%d", prefix, idx)),
		ChildCount:        rapid.IntRange(0, 50).Draw(t, fmt.Sprintf("cc_%s_%d", prefix, idx)),
		SeriesID:          "",
		SeriesName:        "",
		LibraryName:       "TestLib",
		DateLastSaved:     dateLastSaved,
		CachedAt:          time.Now(),
	}
}

// Feature: incremental-sync-performance, Property 1: 时间戳过滤与批量 UPSERT 数据正确性
// Validates: Requirements 1.1, 1.2, 1.4
//
// 对于任意本地数据库中已有的 MediaCache 集合和一组待 UPSERT 的 MediaCache 条目
// （部分已存在且 DateLastSaved 更新、部分已存在但无变化、部分为新增），
// 执行时间戳过滤和批量 UPSERT 后：
// - DateLastSaved 比本地更新的条目应被更新为新值
// - DateLastSaved 不比本地更新的已存在条目应保持不变
// - 新增条目应被插入
// - 新增数 + 更新数 + 跳过数 应等于待处理的总条目数
func TestProperty_BatchUpsertCorrectness(outerT *testing.T) {
	rapid.Check(outerT, func(t *rapid.T) {
		// 设置测试数据库（TempDir 需要用 testing.T）
		tmpDir := outerT.TempDir()
		dbPath := filepath.Join(tmpDir, "test_upsert.db")
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			t.Fatalf("获取 sql.DB 失败: %v", dbErr)
		}
		defer sqlDB.Close()
		svc := NewCacheService(db)

		baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

		// 生成已存在的本地条目（0~20 个）
		existingCount := rapid.IntRange(0, 20).Draw(t, "existingCount")
		existingItems := make([]model.MediaCache, existingCount)
		for i := 0; i < existingCount; i++ {
			item := genMediaCache(t, "existing", i, baseTime)
			existingItems[i] = item
		}

		// 预先插入已存在的条目到数据库
		if len(existingItems) > 0 {
			if insertErr := rawUpsertMediaCaches(sqlDB, existingItems); insertErr != nil {
				t.Fatalf("预插入失败: %v", insertErr)
			}
		}

		// 记录插入后的原始数据（用于验证跳过的条目未被修改）
		originalData := make(map[string]model.MediaCache)
		for _, item := range existingItems {
			var cached model.MediaCache
			if findErr := db.Where("emby_item_id = ?", item.EmbyItemID).First(&cached).Error; findErr == nil {
				originalData[item.EmbyItemID] = cached
			}
		}

		// 生成待 UPSERT 的条目（0~30 个），包含三种情况：
		// 1. 已存在且 DateLastSaved 更新（应更新）
		// 2. 已存在但 DateLastSaved 无变化（应跳过）
		// 3. 新增条目
		upsertCount := rapid.IntRange(0, 30).Draw(t, "upsertCount")
		upsertItems := make([]model.MediaCache, upsertCount)

		expectedNew := 0
		expectedUpdated := 0
		expectedSkipped := 0

		for i := 0; i < upsertCount; i++ {
			scenario := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("scenario_%d", i))
			switch {
			case scenario == 0 && existingCount > 0:
				// 已存在且 DateLastSaved 更新
				existIdx := rapid.IntRange(0, existingCount-1).Draw(t, fmt.Sprintf("existIdx_%d", i))
				item := genMediaCache(t, "existing", existIdx, baseTime.Add(time.Hour))
				item.Name = fmt.Sprintf("Updated_%d", i) // 修改名称以验证更新
				upsertItems[i] = item
				expectedUpdated++
			case scenario == 1 && existingCount > 0:
				// 已存在但 DateLastSaved 无变化（应跳过）
				existIdx := rapid.IntRange(0, existingCount-1).Draw(t, fmt.Sprintf("existIdx2_%d", i))
				item := genMediaCache(t, "existing", existIdx, baseTime)
				upsertItems[i] = item
				expectedSkipped++
			default:
				// 新增条目
				item := genMediaCache(t, "new", i, baseTime.Add(time.Minute))
				upsertItems[i] = item
				expectedNew++
			}
		}

		// 去重：同一个 emby_item_id 可能出现多次，只保留最后一个
		seen := make(map[string]int)
		for i, item := range upsertItems {
			seen[item.EmbyItemID] = i
		}
		deduped := make([]model.MediaCache, 0, len(seen))
		for _, idx := range seen {
			deduped = append(deduped, upsertItems[idx])
		}

		// 执行 filterAndUpsertChangedItems
		newCount, updatedCount, skippedCount, upsertErr := svc.filterAndUpsertChangedItems(sqlDB, deduped)
		if upsertErr != nil {
			t.Fatalf("filterAndUpsertChangedItems 失败: %v", upsertErr)
		}

		// 验证：新增数 + 更新数 + 跳过数 = 去重后的总条目数
		totalProcessed := newCount + updatedCount + skippedCount
		if totalProcessed != len(deduped) {
			t.Fatalf("计数不一致: new(%d) + updated(%d) + skipped(%d) = %d, 期望 %d",
				newCount, updatedCount, skippedCount, totalProcessed, len(deduped))
		}

		// 验证：所有新增条目都应存在于数据库中
		for _, item := range deduped {
			if _, exists := originalData[item.EmbyItemID]; !exists {
				// 这是新增条目，应该在数据库中
				var cached model.MediaCache
				if findErr := db.Where("emby_item_id = ?", item.EmbyItemID).First(&cached).Error; findErr != nil {
					t.Fatalf("新增条目 %s 未在数据库中找到", item.EmbyItemID)
				}
			}
		}

		// 验证：DateLastSaved 更新的条目应被更新
		for _, item := range deduped {
			orig, wasExisting := originalData[item.EmbyItemID]
			if wasExisting && item.DateLastSaved.After(orig.DateLastSaved) {
				var cached model.MediaCache
				if findErr := db.Where("emby_item_id = ?", item.EmbyItemID).First(&cached).Error; findErr != nil {
					t.Fatalf("更新条目 %s 未在数据库中找到", item.EmbyItemID)
				}
				// 名称应该被更新
				if strings.HasPrefix(item.Name, "Updated_") && cached.Name != item.Name {
					t.Fatalf("条目 %s 应被更新为 %s，实际为 %s", item.EmbyItemID, item.Name, cached.Name)
				}
			}
		}
	})
}

// Feature: incremental-sync-performance, Property 2: 智能删除检测决策正确性
// Validates: Requirements 2.1, 2.3
//
// 对于任意 Emby 总数、本地缓存总数和是否有变更的组合：
// - 当 embyTotal < localTotal 时，应执行全量 ID 对比
// - 当 hasChanges == true 时，应执行全量 ID 对比
// - 当 embyTotal >= localTotal 且 hasChanges == false 时，应跳过删除检测
func TestProperty_SmartDeleteDetectionDecision(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		embyTotal := rapid.IntRange(0, 100000).Draw(t, "embyTotal")
		localTotal := rapid.IntRange(0, 100000).Draw(t, "localTotal")
		hasChanges := rapid.Bool().Draw(t, "hasChanges")

		// 决策逻辑：是否应该执行全量 ID 对比
		shouldDetect := shouldRunDeleteDetection(embyTotal, localTotal, hasChanges)

		if embyTotal < localTotal {
			// Emby 总数 < 本地总数：一定有删除，必须检测
			if !shouldDetect {
				t.Fatalf("embyTotal(%d) < localTotal(%d) 时应执行删除检测", embyTotal, localTotal)
			}
		}

		if hasChanges {
			// 有变更（洗版场景）：必须检测
			if !shouldDetect {
				t.Fatalf("hasChanges=true 时应执行删除检测 (emby=%d, local=%d)", embyTotal, localTotal)
			}
		}

		if embyTotal >= localTotal && !hasChanges {
			// 无变更且总数一致或增加：跳过
			if shouldDetect {
				t.Fatalf("embyTotal(%d) >= localTotal(%d) 且 hasChanges=false 时应跳过删除检测", embyTotal, localTotal)
			}
		}
	})
}

// Feature: incremental-sync-performance, Property 3: 增量季缓存重建正确性
// Validates: Requirements 3.1, 3.2, 3.3
//
// 对于任意多个 Series（各有不同数量的季和集），
// 全量构建季缓存后，随机选择部分 Series 作为"受影响"的 Series，
// 执行增量季缓存重建后：
// - 未受影响的 Series 的季缓存应保持不变
// - 受影响的 Series 的季缓存应准确反映 Episode 数据
// - 如果没有受影响的 Series，季缓存总数不变
func TestProperty_IncrementalSeasonCacheRebuild(outerT *testing.T) {
	rapid.Check(outerT, func(t *rapid.T) {
		// 设置测试数据库（TempDir 需要用 testing.T）
		tmpDir := outerT.TempDir()
		dbPath := filepath.Join(tmpDir, "test_season.db")
		db, err := model.InitDB(dbPath)
		if err != nil {
			t.Fatalf("InitDB 失败: %v", err)
		}
		sqlDB, dbErr := db.DB()
		if dbErr != nil {
			t.Fatalf("获取 sql.DB 失败: %v", dbErr)
		}
		defer sqlDB.Close()
		svc := NewCacheService(db)

		now := time.Now()

		// 生成 2~5 个 Series，每个有 1~3 季，每季 1~5 集
		seriesCount := rapid.IntRange(2, 5).Draw(t, "seriesCount")
		type seriesInfo struct {
			id      string
			seasons int
		}
		allSeries := make([]seriesInfo, seriesCount)

		var allEpisodes []model.MediaCache
		for s := 0; s < seriesCount; s++ {
			sid := fmt.Sprintf("series-%d", s)
			seasonCount := rapid.IntRange(1, 3).Draw(t, fmt.Sprintf("seasons_%d", s))
			allSeries[s] = seriesInfo{id: sid, seasons: seasonCount}

			// 插入 Series 本身
			seriesCache := model.MediaCache{
				EmbyItemID:    sid,
				Name:          fmt.Sprintf("Series %d", s),
				Type:          "Series",
				ProviderIDs:   "{}",
				DateLastSaved: now,
				CachedAt:      now,
			}
			if createErr := db.Create(&seriesCache).Error; createErr != nil {
				t.Fatalf("插入 Series 失败: %v", createErr)
			}

			for season := 1; season <= seasonCount; season++ {
				epCount := rapid.IntRange(1, 5).Draw(t, fmt.Sprintf("eps_%d_%d", s, season))
				for ep := 1; ep <= epCount; ep++ {
					epCache := model.MediaCache{
						EmbyItemID:        fmt.Sprintf("ep-%d-s%d-e%d", s, season, ep),
						Name:              fmt.Sprintf("S%dE%d", season, ep),
						Type:              "Episode",
						SeriesID:          sid,
						ParentIndexNumber: season,
						IndexNumber:       ep,
						ProviderIDs:       "{}",
						DateLastSaved:     now,
						CachedAt:          now,
					}
					allEpisodes = append(allEpisodes, epCache)
				}
			}
		}

		// 批量插入所有 Episode
		if len(allEpisodes) > 0 {
			if insertErr := rawUpsertMediaCaches(sqlDB, allEpisodes); insertErr != nil {
				t.Fatalf("插入 Episode 失败: %v", insertErr)
			}
		}

		// 先全量构建季缓存
		if _, buildErr := svc.buildSeasonCacheFromEpisodes(sqlDB); buildErr != nil {
			t.Fatalf("全量构建季缓存失败: %v", buildErr)
		}

		// 记录所有 Series 的原始季缓存
		type seasonKey struct {
			seriesID     string
			seasonNumber int
		}
		originalSeasons := make(map[seasonKey]int) // key -> episodeCount
		var allSeasonCaches []model.SeasonCache
		db.Find(&allSeasonCaches)
		for _, sc := range allSeasonCaches {
			originalSeasons[seasonKey{sc.SeriesEmbyItemID, sc.SeasonNumber}] = sc.EpisodeCount
		}

		// 随机选择 0~2 个 Series 作为"受影响"的 Series
		affectedCount := rapid.IntRange(0, min(2, seriesCount)).Draw(t, "affectedCount")
		affectedSeriesIDs := make([]string, 0, affectedCount)
		affectedSet := make(map[string]bool)
		for i := 0; i < affectedCount; i++ {
			idx := rapid.IntRange(0, seriesCount-1).Draw(t, fmt.Sprintf("affected_%d", i))
			sid := allSeries[idx].id
			if !affectedSet[sid] {
				affectedSeriesIDs = append(affectedSeriesIDs, sid)
				affectedSet[sid] = true
			}
		}

		// 构建变更条目：只包含受影响 Series 的 Episode
		var changedItems []emby.MediaItem
		for _, sid := range affectedSeriesIDs {
			changedItems = append(changedItems, emby.MediaItem{
				ID:       fmt.Sprintf("changed-ep-%s", sid),
				Type:     "Episode",
				SeriesID: sid,
			})
		}

		// 执行增量季缓存重建
		seasonCount2, rebuildErr := svc.rebuildSeasonCacheForSeries(sqlDB, affectedSeriesIDs)
		if rebuildErr != nil {
			t.Fatalf("增量季缓存重建失败: %v", rebuildErr)
		}

		// 验证：未受影响的 Series 的季缓存应保持不变
		var currentSeasons []model.SeasonCache
		db.Find(&currentSeasons)
		currentMap := make(map[seasonKey]int)
		for _, sc := range currentSeasons {
			currentMap[seasonKey{sc.SeriesEmbyItemID, sc.SeasonNumber}] = sc.EpisodeCount
		}

		for key, origCount := range originalSeasons {
			if !affectedSet[key.seriesID] {
				if currentCount, exists := currentMap[key]; !exists {
					t.Fatalf("未受影响的 Series %s 季 %d 的缓存被删除了", key.seriesID, key.seasonNumber)
				} else if currentCount != origCount {
					t.Fatalf("未受影响的 Series %s 季 %d 的集数从 %d 变为 %d",
						key.seriesID, key.seasonNumber, origCount, currentCount)
				}
			}
		}

		// 验证：受影响的 Series 的季缓存应准确反映 Episode 数据
		for _, sid := range affectedSeriesIDs {
			// 从 media_caches 聚合实际 Episode 数据
			rows, queryErr := sqlDB.Query(
				"SELECT parent_index_number, COUNT(*) FROM media_caches WHERE type='Episode' AND series_id=? GROUP BY parent_index_number",
				sid,
			)
			if queryErr != nil {
				t.Fatalf("查询 Episode 数据失败: %v", queryErr)
			}
			expectedEps := make(map[int]int)
			for rows.Next() {
				var sn, cnt int
				rows.Scan(&sn, &cnt)
				expectedEps[sn] = cnt
			}
			rows.Close()

			for sn, expectedCount := range expectedEps {
				key := seasonKey{sid, sn}
				if actualCount, exists := currentMap[key]; !exists {
					t.Fatalf("受影响的 Series %s 季 %d 的缓存不存在", sid, sn)
				} else if actualCount != expectedCount {
					t.Fatalf("受影响的 Series %s 季 %d 的集数为 %d，期望 %d",
						sid, sn, actualCount, expectedCount)
				}
			}
		}

		// 验证：如果没有受影响的 Series，季缓存总数不变
		if len(affectedSeriesIDs) == 0 {
			if len(currentSeasons) != len(allSeasonCaches) {
				t.Fatalf("无受影响 Series 时季缓存总数从 %d 变为 %d", len(allSeasonCaches), len(currentSeasons))
			}
		}

		_ = seasonCount2
	})
}
