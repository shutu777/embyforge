package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"pgregory.net/rapid"
)

// setupEpisodeMappingDeleteTest 创建测试用的 Gin 引擎和 ScanHandler
func setupEpisodeMappingDeleteTest(t *testing.T) (*gin.Engine, *ScanHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_ep_delete.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })

	h := NewScanHandler(db)
	r := gin.New()
	r.DELETE("/api/scan/episode-mapping", h.DeleteEpisodeMappingAnomaly)

	return r, h
}

// Feature: ui-enhancement-episode-duplicate, Property 1: Delete anomaly precision
// Validates: Requirements 3.2, 3.6
//
// 对于任意一组 EpisodeMappingAnomaly 记录和任意删除请求（整组或单季），
// 删除操作完成后，只有目标记录被移除，其他记录保持不变。
func TestProperty_DeleteAnomalyPrecision(t *testing.T) {
	r, h := setupEpisodeMappingDeleteTest(t)

	rapid.Check(t, func(rt *rapid.T) {
		// 清空表
		h.DB.Exec("DELETE FROM episode_mapping_anomalies")

		// 生成 2~5 个不同的 emby_item_id
		numShows := rapid.IntRange(2, 5).Draw(rt, "numShows")
		type showInfo struct {
			embyItemID string
			seasons    []int
		}
		shows := make([]showInfo, numShows)

		var allRecords []model.EpisodeMappingAnomaly
		for i := 0; i < numShows; i++ {
			embyID := fmt.Sprintf("show-%d", i)
			numSeasons := rapid.IntRange(1, 4).Draw(rt, fmt.Sprintf("numSeasons_%d", i))
			seasons := make([]int, numSeasons)
			for j := 0; j < numSeasons; j++ {
				seasons[j] = j + 1
				rec := model.EpisodeMappingAnomaly{
					EmbyItemID:    embyID,
					Name:          fmt.Sprintf("Show %d", i),
					TmdbID:        1000 + i,
					SeasonNumber:  j + 1,
					LocalEpisodes: rapid.IntRange(1, 30).Draw(rt, fmt.Sprintf("local_%d_%d", i, j)),
					TmdbEpisodes:  rapid.IntRange(1, 30).Draw(rt, fmt.Sprintf("tmdb_%d_%d", i, j)),
					Difference:    1,
				}
				allRecords = append(allRecords, rec)
			}
			shows[i] = showInfo{embyItemID: embyID, seasons: seasons}
		}

		// 插入所有记录
		for _, rec := range allRecords {
			if err := h.DB.Create(&rec).Error; err != nil {
				t.Fatalf("插入记录失败: %v", err)
			}
		}

		// 随机选择一个 show 作为删除目标
		targetIdx := rapid.IntRange(0, numShows-1).Draw(rt, "targetIdx")
		target := shows[targetIdx]

		// 随机决定是删除整组还是单季
		deleteSingleSeason := rapid.Bool().Draw(rt, "deleteSingleSeason")

		var body map[string]interface{}
		if deleteSingleSeason && len(target.seasons) > 0 {
			seasonIdx := rapid.IntRange(0, len(target.seasons)-1).Draw(rt, "seasonIdx")
			body = map[string]interface{}{
				"emby_item_id":  target.embyItemID,
				"season_number": target.seasons[seasonIdx],
			}
		} else {
			body = map[string]interface{}{
				"emby_item_id": target.embyItemID,
			}
			deleteSingleSeason = false // 确保一致
		}

		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodDelete, "/api/scan/episode-mapping", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d, body: %s", w.Code, w.Body.String())
		}

		// 验证：其他 show 的记录不受影响
		for i, show := range shows {
			if i == targetIdx {
				continue
			}
			var count int64
			h.DB.Model(&model.EpisodeMappingAnomaly{}).
				Where("emby_item_id = ?", show.embyItemID).
				Count(&count)
			expectedCount := int64(len(show.seasons))
			if count != expectedCount {
				t.Fatalf("show %s 的记录数应为 %d，实际 %d", show.embyItemID, expectedCount, count)
			}
		}

		// 验证：目标 show 的记录被正确删除
		var targetCount int64
		h.DB.Model(&model.EpisodeMappingAnomaly{}).
			Where("emby_item_id = ?", target.embyItemID).
			Count(&targetCount)

		if deleteSingleSeason {
			// 单季删除：应该少了一条
			expectedRemaining := int64(len(target.seasons) - 1)
			if targetCount != expectedRemaining {
				t.Fatalf("单季删除后 show %s 应剩余 %d 条，实际 %d 条",
					target.embyItemID, expectedRemaining, targetCount)
			}

			// 验证被删除的季确实不存在
			seasonNum := body["season_number"].(int)
			var seasonCount int64
			h.DB.Model(&model.EpisodeMappingAnomaly{}).
				Where("emby_item_id = ? AND season_number = ?", target.embyItemID, seasonNum).
				Count(&seasonCount)
			if seasonCount != 0 {
				t.Fatalf("被删除的季 %d 仍然存在", seasonNum)
			}
		} else {
			// 整组删除：应该全部删除
			if targetCount != 0 {
				t.Fatalf("整组删除后 show %s 应剩余 0 条，实际 %d 条",
					target.embyItemID, targetCount)
			}
		}
	})
}
