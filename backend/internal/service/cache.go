package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"embyforge/internal/emby"
	"embyforge/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SyncResult 同步结果
type SyncResult struct {
	TotalItems   int   `json:"total_items"`
	TotalSeasons int   `json:"total_seasons"`
	ElapsedMs    int64 `json:"elapsed_ms"`
	NewItems     int   `json:"new_items"`     // 增量同步：新增条目数
	UpdatedItems int   `json:"updated_items"` // 增量同步：更新条目数
	DeletedItems int   `json:"deleted_items"` // 增量同步：删除条目数
	IsIncremental bool `json:"is_incremental"` // 是否为增量同步
}

// SyncProgress 同步进度事件
type SyncProgress struct {
	Phase     string      `json:"phase"`               // "media" 或 "season"
	Processed int         `json:"processed"`            // 已处理条目数
	Total     int         `json:"total"`                // 总条目数
	Done      bool        `json:"done"`                 // 是否完成
	Error     string      `json:"error,omitempty"`      // 错误信息
	Result    *SyncResult `json:"result,omitempty"`     // 完成时的结果
}

// CacheService 媒体缓存服务
type CacheService struct {
	DB *gorm.DB
}

// NewCacheService 创建缓存服务
func NewCacheService(db *gorm.DB) *CacheService {
	return &CacheService{DB: db}
}

// SyncMediaCache 从 Emby 同步完整媒体库到本地缓存
// 流程：清空缓存表 → 分页获取 Emby 媒体 → 批量写入 media_cache → 获取 Series 的季信息 → 写入 season_cache
func (s *CacheService) SyncMediaCache(client *emby.Client) (*SyncResult, error) {
	start := time.Now()

	// 清空缓存表
	if err := s.DB.Exec("DELETE FROM media_caches").Error; err != nil {
		return nil, fmt.Errorf("清空媒体缓存表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM season_caches").Error; err != nil {
		return nil, fmt.Errorf("清空季缓存表失败: %w", err)
	}
	log.Printf("🗑️ 已清空缓存表")

	result := &SyncResult{}

	// 分页获取所有媒体条目并写入缓存（只拉取 Movie/Series/Episode）
	err := client.GetMediaItems(emby.SyncItemTypes, func(items []emby.MediaItem) error {
		caches := make([]model.MediaCache, 0, len(items))
		for _, item := range items {
			cache := model.NewMediaCacheFromItem(item, "")
			caches = append(caches, cache)
		}

		if len(caches) > 0 {
			if err := s.DB.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "emby_item_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "type", "has_poster", "path", "provider_ids", "file_size", "index_number", "parent_index_number", "child_count", "series_id", "series_name", "library_name", "cached_at"}),
			}).Create(&caches).Error; err != nil {
				log.Printf("批量写入媒体缓存失败，尝试逐条写入: %v", err)
				for _, c := range caches {
					if err := s.DB.Clauses(clause.OnConflict{
						Columns:   []clause.Column{{Name: "emby_item_id"}},
						DoUpdates: clause.AssignmentColumns([]string{"name", "type", "has_poster", "path", "provider_ids", "file_size", "index_number", "parent_index_number", "child_count", "series_id", "series_name", "library_name", "cached_at"}),
					}).Create(&c).Error; err != nil {
						log.Printf("写入媒体缓存记录失败 (EmbyItemID=%s): %v", c.EmbyItemID, err)
						continue
					}
				}
			}
			result.TotalItems += len(caches)
		}

		log.Printf("📊 媒体缓存同步: 已处理 %d 个条目...", result.TotalItems)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("获取 Emby 媒体条目失败: %w", err)
	}

	// 直接从已同步的 Episode 数据聚合生成季缓存（零额外 HTTP 请求）
	sqlDB, dbErr := s.DB.DB()
	if dbErr != nil {
		log.Printf("⚠️ 获取数据库连接失败: %v", dbErr)
	} else {
		seasonCount, err := s.buildSeasonCacheFromEpisodes(sqlDB)
		if err != nil {
			log.Printf("⚠️ 从 Episode 聚合生成季缓存失败: %v", err)
		} else {
			result.TotalSeasons = seasonCount
		}
	}

	result.ElapsedMs = time.Since(start).Milliseconds()
	log.Printf("✅ 媒体缓存同步完成: %d 个媒体条目, %d 个季, 耗时 %dms",
		result.TotalItems, result.TotalSeasons, result.ElapsedMs)

	return result, nil
}

// syncBatchSize 内存缓冲区满后批量写入数据库的条目数
const syncBatchSize = 10000

// SyncMediaCacheWithContext 使用 Worker Pool 的缓存同步（性能优化版）
// 优化策略：
//   - 增大 API 页面大小减少 HTTP 请求次数
//   - 内存去重避免 Emby API 返回的重复条目
//   - 先 DELETE 再纯 INSERT（无需 ON CONFLICT 开销）
//   - 大批量事务写入减少 SQLite 事务开销
func (s *CacheService) SyncMediaCacheWithContext(ctx context.Context, client *emby.Client) (*SyncResult, error) {
	start := time.Now()

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 清空缓存表
	if err := s.DB.Exec("DELETE FROM media_caches").Error; err != nil {
		return nil, fmt.Errorf("清空媒体缓存表失败: %w", err)
	}
	if err := s.DB.Exec("DELETE FROM season_caches").Error; err != nil {
		return nil, fmt.Errorf("清空季缓存表失败: %w", err)
	}
	log.Printf("🗑️ 已清空缓存表")

	result := &SyncResult{}

	// 内存去重集合，Emby API 跨页可能返回重复 item
	seen := make(map[string]bool, 300000)
	// 内存缓冲区，攒够 syncBatchSize 条后批量写入
	buffer := make([]model.MediaCache, 0, syncBatchSize)

	// flushBuffer 将缓冲区数据批量写入数据库（单个大事务）
	flushBuffer := func(buf []model.MediaCache) error {
		if len(buf) == 0 {
			return nil
		}
		// 使用事务包裹整个批次写入，减少 fsync 次数
		return s.DB.Transaction(func(tx *gorm.DB) error {
			// 分批写入，每批 1000 条，避免 SQLite 变量数限制
			const dbBatch = 1000
			for i := 0; i < len(buf); i += dbBatch {
				end := i + dbBatch
				if end > len(buf) {
					end = len(buf)
				}
				if err := tx.Create(buf[i:end]).Error; err != nil {
					return fmt.Errorf("批量写入媒体缓存失败 (batch %d-%d): %w", i, end, err)
				}
			}
			return nil
		})
	}

	// 分页获取所有媒体条目，使用大页面减少 HTTP 请求（只拉取 Movie/Series/Episode）
	err := client.GetMediaItemsWithContext(ctx, emby.SyncItemTypes, func(items []emby.MediaItem) error {
		for _, item := range items {
			// 内存去重
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true

			cache := model.NewMediaCacheFromItem(item, "")
			buffer = append(buffer, cache)
		}

		// 缓冲区满时批量写入
		if len(buffer) >= syncBatchSize {
			if err := flushBuffer(buffer); err != nil {
				log.Printf("⚠️ 批量写入失败: %v", err)
				return err
			}
			result.TotalItems += len(buffer)
			log.Printf("📊 媒体缓存同步: 已写入 %d 个条目 (去重后)...", result.TotalItems)
			buffer = buffer[:0] // 清空缓冲区，复用底层数组
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("获取 Emby 媒体条目失败: %w", err)
	}

	// 写入剩余缓冲区数据
	if len(buffer) > 0 {
		if err := flushBuffer(buffer); err != nil {
			return nil, fmt.Errorf("写入剩余媒体缓存失败: %w", err)
		}
		result.TotalItems += len(buffer)
	}

	log.Printf("📊 媒体缓存写入完成: 共 %d 个条目 (去重前 %d 个)",
		result.TotalItems, len(seen))

	// 直接从已同步的 Episode 数据聚合生成季缓存（零额外 HTTP 请求）
	sqlDB, err := s.DB.DB()
	if err != nil {
		log.Printf("⚠️ 获取数据库连接失败: %v", err)
	} else {
		seasonCount, err := s.buildSeasonCacheFromEpisodes(sqlDB)
		if err != nil {
			log.Printf("⚠️ 从 Episode 聚合生成季缓存失败: %v", err)
		} else {
			result.TotalSeasons = seasonCount
		}
	}

	result.ElapsedMs = time.Since(start).Milliseconds()
	log.Printf("✅ 媒体缓存同步完成: %d 个媒体条目, %d 个季, 耗时 %dms",
		result.TotalItems, result.TotalSeasons, result.ElapsedMs)

	return result, nil
}

// SyncMediaCacheWithProgress 带进度回调的缓存同步
// 优化策略：
//   - 流水线：API 拉取和 DB 写入在不同 goroutine 并行
//   - 原生 SQL prepared statement 批量写入，绕过 GORM 开销
//   - 内存去重避免 Emby API 跨页返回的重复条目
//   - 同步前 DROP INDEX + 额外 pragma 优化，同步后恢复
//   - 季缓存写入前去重，使用原生 SQL 批量写入
func (s *CacheService) SyncMediaCacheWithProgress(ctx context.Context, client *emby.Client, progressCh chan<- SyncProgress) {
	defer close(progressCh)
	start := time.Now()

	sendError := func(msg string) {
		select {
		case progressCh <- SyncProgress{Phase: "media", Error: msg}:
		case <-ctx.Done():
		}
	}

	select {
	case <-ctx.Done():
		sendError("同步已取消")
		return
	default:
	}

	// 获取底层 *sql.DB 用于原生 SQL 操作
	sqlDB, err := s.DB.DB()
	if err != nil {
		sendError(fmt.Sprintf("获取数据库连接失败: %v", err))
		return
	}

	// 清空缓存表
	if err := s.DB.Exec("DELETE FROM media_caches").Error; err != nil {
		sendError(fmt.Sprintf("清空媒体缓存表失败: %v", err))
		return
	}
	if err := s.DB.Exec("DELETE FROM season_caches").Error; err != nil {
		sendError(fmt.Sprintf("清空季缓存表失败: %v", err))
		return
	}
	log.Printf("🗑️ 已清空缓存表")

	// 同步前删除索引 + 额外写入优化 pragma（写入完成后重建）
	s.DB.Exec("DROP INDEX IF EXISTS idx_media_cache_emby_item_id")
	s.DB.Exec("DROP INDEX IF EXISTS idx_media_caches_type")
	s.DB.Exec("DROP INDEX IF EXISTS idx_media_caches_series_id")
	s.DB.Exec("PRAGMA temp_store=MEMORY")
	s.DB.Exec("PRAGMA mmap_size=268435456") // 256MB mmap

	// 获取媒体总数
	total, err := client.GetTotalItemCount(ctx)
	if err != nil {
		log.Printf("⚠️ 获取媒体总数失败，使用 0: %v", err)
		total = 0
	}

	// 发送初始进度
	select {
	case progressCh <- SyncProgress{Phase: "media", Processed: 0, Total: total}:
	case <-ctx.Done():
		sendError("同步已取消")
		return
	}

	result := &SyncResult{}

	// 内存去重集合
	seen := make(map[string]bool, 300000)

	// 流水线：writeCh 连接 API 拉取和 DB 写入
	type writeBatch struct {
		items []model.MediaCache
	}
	writeCh := make(chan writeBatch, 3) // 缓冲 3 个批次，让 API 拉取不等 DB 写入
	writeErrCh := make(chan error, 1)

	// DB 写入 goroutine
	go func() {
		defer close(writeErrCh)
		for batch := range writeCh {
			if err := rawInsertMediaCaches(sqlDB, batch.items); err != nil {
				writeErrCh <- err
				return
			}
		}
	}()

	// 内存缓冲区
	buffer := make([]model.MediaCache, 0, syncBatchSize)

	// 分页获取所有媒体条目（内存去重，只拉取 Movie/Series/Episode）
	err = client.GetMediaItemsWithContext(ctx, emby.SyncItemTypes, func(items []emby.MediaItem) error {
		for _, item := range items {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true

			cache := model.NewMediaCacheFromItem(item, "")
			buffer = append(buffer, cache)
		}

		// 缓冲区满时发送到写入通道
		if len(buffer) >= syncBatchSize {
			// 复制一份发送，避免数据竞争
			batch := make([]model.MediaCache, len(buffer))
			copy(batch, buffer)

			select {
			case writeCh <- writeBatch{items: batch}:
			case err := <-writeErrCh:
				return fmt.Errorf("DB 写入失败: %w", err)
			case <-ctx.Done():
				return ctx.Err()
			}

			result.TotalItems += len(buffer)
			log.Printf("📊 媒体缓存同步: 已处理 %d 个条目...", result.TotalItems)
			buffer = buffer[:0]

			select {
			case progressCh <- SyncProgress{Phase: "media", Processed: result.TotalItems, Total: total}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		return nil
	})

	if err != nil {
		close(writeCh)
		sendError(fmt.Sprintf("获取 Emby 媒体条目失败: %v", err))
		s.rebuildMediaCacheIndexes()
		return
	}

	// 写入剩余缓冲区
	if len(buffer) > 0 {
		select {
		case writeCh <- writeBatch{items: buffer}:
		case err := <-writeErrCh:
			close(writeCh)
			sendError(fmt.Sprintf("DB 写入失败: %v", err))
			s.rebuildMediaCacheIndexes()
			return
		}
		result.TotalItems += len(buffer)
	}

	// 关闭写入通道，等待写入完成
	close(writeCh)
	if err := <-writeErrCh; err != nil {
		sendError(fmt.Sprintf("DB 写入失败: %v", err))
		s.rebuildMediaCacheIndexes()
		return
	}

	// 发送最终媒体进度
	select {
	case progressCh <- SyncProgress{Phase: "media", Processed: result.TotalItems, Total: total}:
	case <-ctx.Done():
	}

	log.Printf("📊 媒体缓存写入完成: 共 %d 个条目 (去重前 API 返回 %d 个)", result.TotalItems, len(seen))

	// 重建索引
	s.rebuildMediaCacheIndexes()

	// 直接从已同步的 Episode 数据聚合生成季缓存（零额外 HTTP 请求）
	seasonCount, err := s.buildSeasonCacheFromEpisodes(sqlDB)
	if err != nil {
		log.Printf("⚠️ 从 Episode 聚合生成季缓存失败: %v", err)
	} else {
		result.TotalSeasons = seasonCount
	}

	result.ElapsedMs = time.Since(start).Milliseconds()
	log.Printf("✅ 媒体缓存同步完成: %d 个媒体条目, %d 个季, 耗时 %dms",
		result.TotalItems, result.TotalSeasons, result.ElapsedMs)

	select {
	case progressCh <- SyncProgress{Phase: "done", Done: true, Processed: result.TotalItems, Total: total, Result: result}:
	case <-ctx.Done():
	}
}

// rebuildMediaCacheIndexes 重建 media_caches 表的所有索引
func (s *CacheService) rebuildMediaCacheIndexes() {
	s.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_media_cache_emby_item_id ON media_caches(emby_item_id)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_media_caches_type ON media_caches(type)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_media_caches_series_id ON media_caches(series_id)")
}

// rawInsertMediaCaches 使用原生 SQL prepared statement 批量写入媒体缓存
// 比 GORM Create 快 3-5 倍：预编译语句 + 多行 VALUES + 单事务
func rawInsertMediaCaches(db *sql.DB, items []model.MediaCache) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 每批 500 行（14 列 × 500 = 7000 参数，远低于 SQLite 32766 限制）
	const cols = 14
	const batchRows = 500

	for i := 0; i < len(items); i += batchRows {
		end := i + batchRows
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]

		// 构建 INSERT INTO ... VALUES (?,?,...), (?,?,...)
		var sb strings.Builder
		sb.WriteString("INSERT INTO media_caches (emby_item_id,name,type,has_poster,path,provider_ids,file_size,index_number,parent_index_number,child_count,series_id,series_name,library_name,cached_at) VALUES ")
		placeholder := "(?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
		for j := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(placeholder)
		}

		args := make([]interface{}, 0, len(batch)*cols)
		for _, c := range batch {
			args = append(args, c.EmbyItemID, c.Name, c.Type, c.HasPoster,
				c.Path, c.ProviderIDs, c.FileSize, c.IndexNumber, c.ParentIndexNumber, c.ChildCount,
				c.SeriesID, c.SeriesName, c.LibraryName, c.CachedAt)
		}

		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("批量写入失败 (rows %d-%d): %w", i, end, err)
		}
	}

	return tx.Commit()
}

// rawInsertSeasonCaches 使用原生 SQL 批量写入季缓存
func rawInsertSeasonCaches(db *sql.DB, items []model.SeasonCache) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	const cols = 5
	const batchRows = 500

	for i := 0; i < len(items); i += batchRows {
		end := i + batchRows
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO season_caches (series_emby_item_id,season_emby_item_id,season_number,episode_count,cached_at) VALUES ")
		placeholder := "(?,?,?,?,?)"
		for j := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(placeholder)
		}

		args := make([]interface{}, 0, len(batch)*cols)
		for _, c := range batch {
			args = append(args, c.SeriesEmbyItemID, c.SeasonEmbyItemID, c.SeasonNumber, c.EpisodeCount, c.CachedAt)
		}

		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return fmt.Errorf("批量写入季缓存失败 (rows %d-%d): %w", i, end, err)
		}
	}

	return tx.Commit()
}

// buildSeasonCacheFromEpisodes 从 media_caches 中的 Episode 数据聚合生成季缓存
// 按 series_id + parent_index_number 分组统计 Episode 数量，直接写入 season_caches
// 不需要任何额外的 HTTP 请求，因为 Episode 数据在媒体同步时已经拉取
func (s *CacheService) buildSeasonCacheFromEpisodes(sqlDB *sql.DB) (int, error) {
	// 用一条 SQL 聚合出每个 Series 每季的 Episode 数量
	// series_id 对应 season_caches 的 series_emby_item_id
	// parent_index_number 对应 season_number
	// season_emby_item_id 用 series_id + '_S' + parent_index_number 生成（因为没有真实的 Season Emby ID）
	rows, err := sqlDB.Query(`
		SELECT series_id, parent_index_number, COUNT(*) as episode_count
		FROM media_caches
		WHERE type = 'Episode' AND series_id != ''
		GROUP BY series_id, parent_index_number
	`)
	if err != nil {
		return 0, fmt.Errorf("聚合 Episode 数据失败: %w", err)
	}
	defer rows.Close()

	// 收集聚合结果
	type seasonAgg struct {
		seriesID     string
		seasonNumber int
		episodeCount int
	}
	var aggs []seasonAgg
	for rows.Next() {
		var a seasonAgg
		if err := rows.Scan(&a.seriesID, &a.seasonNumber, &a.episodeCount); err != nil {
			return 0, fmt.Errorf("读取聚合结果失败: %w", err)
		}
		aggs = append(aggs, a)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("遍历聚合结果失败: %w", err)
	}

	if len(aggs) == 0 {
		return 0, nil
	}

	// 批量写入 season_caches
	tx, err := sqlDB.Begin()
	if err != nil {
		return 0, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback()

	const batchRows = 500
	now := time.Now()

	for i := 0; i < len(aggs); i += batchRows {
		end := i + batchRows
		if end > len(aggs) {
			end = len(aggs)
		}
		batch := aggs[i:end]

		var sb strings.Builder
		sb.WriteString("INSERT INTO season_caches (series_emby_item_id, season_emby_item_id, season_number, episode_count, cached_at) VALUES ")
		placeholder := "(?,?,?,?,?)"
		for j := range batch {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(placeholder)
		}

		args := make([]interface{}, 0, len(batch)*5)
		for _, a := range batch {
			// 生成虚拟的 season_emby_item_id（因为不再从 Emby API 获取真实 Season ID）
			seasonEmbyID := fmt.Sprintf("%s_S%d", a.seriesID, a.seasonNumber)
			args = append(args, a.seriesID, seasonEmbyID, a.seasonNumber, a.episodeCount, now)
		}

		if _, err := tx.Exec(sb.String(), args...); err != nil {
			return 0, fmt.Errorf("批量写入季缓存失败 (rows %d-%d): %w", i, end, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交事务失败: %w", err)
	}

	log.Printf("📊 从 Episode 聚合生成了 %d 条季缓存记录", len(aggs))
	return len(aggs), nil
}

// IncrementalSyncMediaCacheWithProgress 增量同步：只同步新增和修改的条目，检测并删除已移除的条目
// 流程：
//  1. 获取上次同步时间（last_sync_at）
//  2. 如果没有上次同步记录 → 回退到全量同步
//  3. 通过 MinDateLastSaved 获取修改过的条目 → UPSERT 到本地缓存
//  4. 获取 Emby 当前所有 ID → 删除本地有但 Emby 已移除的条目
//  5. 重建季缓存
func (s *CacheService) IncrementalSyncMediaCacheWithProgress(ctx context.Context, client *emby.Client, progressCh chan<- SyncProgress) {
	// 注意：不使用 defer close(progressCh)，因为可能回退到全量同步（由全量方法负责 close）

	// 获取上次同步时间
	status, err := s.GetCacheStatus()
	if err != nil || status.LastSyncAt == nil || status.TotalItems == 0 {
		// 没有上次同步记录，回退到全量同步（全量方法会负责 close progressCh）
		log.Printf("📊 没有上次同步记录，回退到全量同步")
		s.SyncMediaCacheWithProgress(ctx, client, progressCh)
		return
	}

	// 走增量逻辑，由本方法负责 close
	defer close(progressCh)
	start := time.Now()

	sendError := func(msg string) {
		select {
		case progressCh <- SyncProgress{Phase: "media", Error: msg}:
		case <-ctx.Done():
		}
	}

	sendProgress := func(phase string, processed, total int) {
		select {
		case progressCh <- SyncProgress{Phase: phase, Processed: processed, Total: total}:
		case <-ctx.Done():
		}
	}

	select {
	case <-ctx.Done():
		sendError("同步已取消")
		return
	default:
	}

	lastSyncAt := *status.LastSyncAt
	log.Printf("🔄 开始增量同步，上次同步时间: %s", lastSyncAt.Format(time.RFC3339))

	result := &SyncResult{IsIncremental: true}

	// 阶段 1：获取修改过的条目并 UPSERT
	sendProgress("media", 0, 0)

	processed := 0
	err = client.GetMediaItemsModifiedSince(ctx, lastSyncAt, emby.SyncItemTypes, func(items []emby.MediaItem) error {
		if len(items) == 0 {
			return nil
		}

		caches := make([]model.MediaCache, 0, len(items))
		for _, item := range items {
			cache := model.NewMediaCacheFromItem(item, "")
			caches = append(caches, cache)
		}

		// UPSERT：存在则更新，不存在则插入
		for _, c := range caches {
			var existing model.MediaCache
			dbResult := s.DB.Where("emby_item_id = ?", c.EmbyItemID).First(&existing)
			if dbResult.Error == nil {
				// 已存在，更新
				if err := s.DB.Model(&existing).Updates(map[string]interface{}{
					"name":               c.Name,
					"type":               c.Type,
					"has_poster":         c.HasPoster,
					"path":               c.Path,
					"provider_ids":       c.ProviderIDs,
					"file_size":          c.FileSize,
					"index_number":       c.IndexNumber,
					"parent_index_number": c.ParentIndexNumber,
					"child_count":        c.ChildCount,
					"series_id":          c.SeriesID,
					"series_name":        c.SeriesName,
					"library_name":       c.LibraryName,
					"cached_at":          c.CachedAt,
				}).Error; err != nil {
					log.Printf("⚠️ 更新缓存记录失败 (EmbyItemID=%s): %v", c.EmbyItemID, err)
					continue
				}
				result.UpdatedItems++
			} else {
				// 不存在，插入
				if err := s.DB.Create(&c).Error; err != nil {
					log.Printf("⚠️ 插入缓存记录失败 (EmbyItemID=%s): %v", c.EmbyItemID, err)
					continue
				}
				result.NewItems++
			}
		}

		processed += len(items)
		sendProgress("media", processed, 0)
		log.Printf("📊 增量同步: 已处理 %d 个变更条目 (新增: %d, 更新: %d)",
			processed, result.NewItems, result.UpdatedItems)

		return nil
	})

	if err != nil {
		sendError(fmt.Sprintf("获取增量媒体条目失败: %v", err))
		return
	}

	log.Printf("📊 增量变更处理完成: 新增 %d, 更新 %d", result.NewItems, result.UpdatedItems)

	// 阶段 2：删除检测已由 WebSocket 实时监听处理，增量同步不再需要
	// 如需精确清理，请使用全量同步模式

	// 阶段 3：重建季缓存
	sendProgress("season", 0, 0)

	// 清空并重建季缓存
	if err := s.DB.Exec("DELETE FROM season_caches").Error; err != nil {
		log.Printf("⚠️ 清空季缓存表失败: %v", err)
	} else {
		sqlDB, err := s.DB.DB()
		if err != nil {
			log.Printf("⚠️ 获取数据库连接失败: %v", err)
		} else {
			seasonCount, err := s.buildSeasonCacheFromEpisodes(sqlDB)
			if err != nil {
				log.Printf("⚠️ 从 Episode 聚合生成季缓存失败: %v", err)
			} else {
				result.TotalSeasons = seasonCount
			}
		}
	}

	// 统计最终总数
	var totalCount int64
	s.DB.Model(&model.MediaCache{}).Count(&totalCount)
	result.TotalItems = int(totalCount)

	result.ElapsedMs = time.Since(start).Milliseconds()
	log.Printf("✅ 增量同步完成: 总计 %d 条目 (新增 %d, 更新 %d, 删除 %d), %d 个季, 耗时 %dms",
		result.TotalItems, result.NewItems, result.UpdatedItems, result.DeletedItems,
		result.TotalSeasons, result.ElapsedMs)

	select {
	case progressCh <- SyncProgress{Phase: "done", Done: true, Processed: result.TotalItems, Total: result.TotalItems, Result: result}:
	case <-ctx.Done():
	}
}

// GetCacheStatus 获取缓存状态信息
func (s *CacheService) GetCacheStatus() (*model.CacheStatus, error) {
	status := &model.CacheStatus{}

	// 查询媒体缓存条目数
	if err := s.DB.Model(&model.MediaCache{}).Count(&status.TotalItems).Error; err != nil {
		return nil, fmt.Errorf("查询媒体缓存条目数失败: %w", err)
	}

	// 查询季缓存条目数
	if err := s.DB.Model(&model.SeasonCache{}).Count(&status.TotalSeasons).Error; err != nil {
		return nil, fmt.Errorf("查询季缓存条目数失败: %w", err)
	}

	// 查询最后同步时间
	var lastCache model.MediaCache
	if err := s.DB.Order("cached_at DESC").First(&lastCache).Error; err == nil {
		status.LastSyncAt = &lastCache.CachedAt
	}

	return status, nil
}

// HandleLibraryChanged 处理媒体库变更事件
// 由 LibraryWatcher 回调触发，直接接收完整的 MediaItem（无需二次请求）
func (s *CacheService) HandleLibraryChanged(ctx context.Context, client *emby.Client, items []emby.MediaItem, removed []string) {
	// 处理删除检测信号
	if len(removed) == 1 && removed[0] == "__DETECT_DELETIONS__" {
		s.detectAndRemoveDeletedItems(ctx, client)
		removed = nil
	}

	// 处理普通删除：直接从本地缓存中删除
	if len(removed) > 0 {
		const deleteBatch = 500
		for i := 0; i < len(removed); i += deleteBatch {
			end := i + deleteBatch
			if end > len(removed) {
				end = len(removed)
			}
			if err := s.DB.Where("emby_item_id IN ?", removed[i:end]).Delete(&model.MediaCache{}).Error; err != nil {
				log.Printf("⚠️ 实时删除缓存记录失败: %v", err)
			}
		}
		log.Printf("🗑️ 实时同步: 已删除 %d 个缓存条目", len(removed))
	}

	// 处理新增和更新：直接使用传入的完整 MediaItem，无需再调用 GetItemByID
	if len(items) > 0 {
		newCount, updateCount := 0, 0
		for _, item := range items {
			// 只处理我们关心的类型
			if item.Type != "Movie" && item.Type != "Series" && item.Type != "Episode" {
				continue
			}

			cache := model.NewMediaCacheFromItem(item, "")
			var existing model.MediaCache
			if s.DB.Where("emby_item_id = ?", cache.EmbyItemID).First(&existing).Error == nil {
				// 已存在，更新
				s.DB.Model(&existing).Updates(map[string]interface{}{
					"name":                cache.Name,
					"type":                cache.Type,
					"has_poster":          cache.HasPoster,
					"path":                cache.Path,
					"provider_ids":        cache.ProviderIDs,
					"file_size":           cache.FileSize,
					"index_number":        cache.IndexNumber,
					"parent_index_number": cache.ParentIndexNumber,
					"child_count":         cache.ChildCount,
					"series_id":           cache.SeriesID,
					"series_name":         cache.SeriesName,
					"library_name":        cache.LibraryName,
					"cached_at":           cache.CachedAt,
				})
				updateCount++
			} else {
				// 不存在，插入
				if err := s.DB.Create(&cache).Error; err != nil {
					log.Printf("⚠️ 实时同步插入缓存失败 (EmbyItemID=%s): %v", cache.EmbyItemID, err)
				}
				newCount++
			}
		}
		if newCount > 0 || updateCount > 0 {
			log.Printf("📡 实时同步: 新增 %d, 更新 %d 个缓存条目", newCount, updateCount)
		}
	}
}

// detectAndRemoveDeletedItems 检测并删除 Emby 中已不存在的本地缓存条目
// 通过分页获取 Emby 所有 ID，与本地缓存对比，删除本地多余的条目
func (s *CacheService) detectAndRemoveDeletedItems(ctx context.Context, client *emby.Client) {
	log.Printf("🔍 开始检测已删除的条目...")

	embyIDs, total, err := client.GetAllItemIDs(ctx, emby.SyncItemTypes)
	if err != nil {
		log.Printf("⚠️ 获取 Emby ID 列表失败: %v", err)
		return
	}

	// 获取本地所有 emby_item_id
	var localIDs []string
	if err := s.DB.Model(&model.MediaCache{}).Pluck("emby_item_id", &localIDs).Error; err != nil {
		log.Printf("⚠️ 获取本地缓存 ID 列表失败: %v", err)
		return
	}

	// 找出本地有但 Emby 没有的条目
	var toDelete []string
	for _, id := range localIDs {
		if !embyIDs[id] {
			toDelete = append(toDelete, id)
		}
	}

	if len(toDelete) > 0 {
		const deleteBatch = 500
		for i := 0; i < len(toDelete); i += deleteBatch {
			end := i + deleteBatch
			if end > len(toDelete) {
				end = len(toDelete)
			}
			s.DB.Where("emby_item_id IN ?", toDelete[i:end]).Delete(&model.MediaCache{})
		}
		log.Printf("🗑️ 删除检测完成: 删除了 %d 个本地多余条目 (Emby 总数: %d, 本地原有: %d)",
			len(toDelete), total, len(localIDs))
	} else {
		log.Printf("✅ 删除检测完成: 无需删除 (Emby: %d, 本地: %d)", total, len(localIDs))
	}
}
