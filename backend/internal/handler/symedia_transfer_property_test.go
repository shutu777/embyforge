package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"pgregory.net/rapid"
)

// setupSymediaTransferTest 创建测试用的 Gin 引擎和 SymediaHandler
func setupSymediaTransferTest(t *testing.T) (*gin.Engine, *SymediaHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB 失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 DB 失败: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	h := NewSymediaHandler(db, "test-secret")

	r := gin.New()
	r.GET("/api/symedia/config", h.GetConfigs)
	r.POST("/api/symedia/transfer-config", h.SaveTransferConfig)

	return r, h
}

// Feature: symedia-manual-transfer, Property 1: Transfer config round-trip
// Validates: Requirements 2.3, 2.4
// 对于任意一组有效的归档配置值，保存后通过 GetConfigs 读取应返回相同的值。
func TestProperty_TransferConfigRoundTrip(t *testing.T) {
	r, _ := setupSymediaTransferTest(t)

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机归档配置
		ruleID := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).Draw(t, "rule_id")
		destDir := rapid.StringMatching(`/[a-zA-Z0-9/_]{5,50}`).Draw(t, "dest_dir")
		transferType := rapid.SampledFrom([]string{"copy", "move", "cd2_move", "cd2_copy", "link", "softlink"}).Draw(t, "transfer_type")
		category := rapid.Bool().Draw(t, "category")
		deleteDir := rapid.Bool().Draw(t, "delete_dir")
		extractMetadata := rapid.Bool().Draw(t, "extract_metadata")
		cacheMetadata := rapid.Bool().Draw(t, "cache_metadata")
		downloadNfo := rapid.Bool().Draw(t, "download_nfo")
		downloadImage := rapid.Bool().Draw(t, "download_image")

		// 保存配置
		saveReq := SaveTransferConfigRequest{
			RuleID:          ruleID,
			DestDir:         destDir,
			TransferType:    transferType,
			Category:        category,
			DeleteDir:       deleteDir,
			ExtractMetadata: extractMetadata,
			CacheMetadata:   cacheMetadata,
			DownloadNfo:     downloadNfo,
			DownloadImage:   downloadImage,
		}
		body, _ := json.Marshal(saveReq)
		req := httptest.NewRequest(http.MethodPost, "/api/symedia/transfer-config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("保存配置应返回 200，实际返回 %d, body: %s", w.Code, w.Body.String())
		}

		// 读取配置
		getReq := httptest.NewRequest(http.MethodGet, "/api/symedia/config", nil)
		getW := httptest.NewRecorder()
		r.ServeHTTP(getW, getReq)

		if getW.Code != http.StatusOK {
			t.Fatalf("获取配置应返回 200，实际返回 %d", getW.Code)
		}

		// 解析响应
		var resp struct {
			Data struct {
				TransferConfig TransferConfigResponse `json:"transfer_config"`
			} `json:"data"`
		}
		if err := json.Unmarshal(getW.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		tc := resp.Data.TransferConfig

		// 验证 round-trip：保存的值和读取的值一致
		if tc.RuleID != ruleID {
			t.Fatalf("rule_id 不匹配: got %q, want %q", tc.RuleID, ruleID)
		}
		if tc.DestDir != destDir {
			t.Fatalf("dest_dir 不匹配: got %q, want %q", tc.DestDir, destDir)
		}
		if tc.TransferType != transferType {
			t.Fatalf("transfer_type 不匹配: got %q, want %q", tc.TransferType, transferType)
		}
		if tc.Category != category {
			t.Fatalf("category 不匹配: got %v, want %v", tc.Category, category)
		}
		if tc.DeleteDir != deleteDir {
			t.Fatalf("delete_dir 不匹配: got %v, want %v", tc.DeleteDir, deleteDir)
		}
		if tc.ExtractMetadata != extractMetadata {
			t.Fatalf("extract_metadata 不匹配: got %v, want %v", tc.ExtractMetadata, extractMetadata)
		}
		if tc.CacheMetadata != cacheMetadata {
			t.Fatalf("cache_metadata 不匹配: got %v, want %v", tc.CacheMetadata, cacheMetadata)
		}
		if tc.DownloadNfo != downloadNfo {
			t.Fatalf("download_nfo 不匹配: got %v, want %v", tc.DownloadNfo, downloadNfo)
		}
		if tc.DownloadImage != downloadImage {
			t.Fatalf("download_image 不匹配: got %v, want %v", tc.DownloadImage, downloadImage)
		}
	})
}

// Feature: symedia-manual-transfer, Property 2: Rule ID validation rejects invalid input
// Validates: Requirements 2.5
// 对于任意纯空白字符串（包括空字符串），保存为 rule_id 应被拒绝，已有配置不受影响。
func TestProperty_RuleIDValidationRejectsInvalid(t *testing.T) {
	r, h := setupSymediaTransferTest(t)

	rapid.Check(t, func(t *rapid.T) {
		// 先保存一份有效配置作为基线
		validRuleID := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}`).Draw(t, "valid_rule_id")
		validReq := SaveTransferConfigRequest{
			RuleID:       validRuleID,
			DestDir:      "/test/dir",
			TransferType: "copy",
		}
		body, _ := json.Marshal(validReq)
		req := httptest.NewRequest(http.MethodPost, "/api/symedia/transfer-config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("保存有效配置应返回 200，实际返回 %d", w.Code)
		}

		// 生成纯空白字符串作为无效 rule_id
		numSpaces := rapid.IntRange(0, 20).Draw(t, "numSpaces")
		numTabs := rapid.IntRange(0, 5).Draw(t, "numTabs")
		whitespace := ""
		for i := 0; i < numSpaces; i++ {
			whitespace += " "
		}
		for i := 0; i < numTabs; i++ {
			whitespace += "\t"
		}

		// 尝试用空白 rule_id 保存
		invalidReq := SaveTransferConfigRequest{
			RuleID:       whitespace,
			DestDir:      "/new/dir",
			TransferType: "move",
		}
		body2, _ := json.Marshal(invalidReq)
		req2 := httptest.NewRequest(http.MethodPost, "/api/symedia/transfer-config", bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		// 应被拒绝
		if w2.Code != http.StatusBadRequest {
			t.Fatalf("空白 rule_id 应返回 400，实际返回 %d, body: %s", w2.Code, w2.Body.String())
		}

		// 验证原有配置未被修改
		var savedConfig model.SystemConfig
		if err := h.DB.Where("key = ?", "symedia_transfer_rule_id").First(&savedConfig).Error; err != nil {
			t.Fatalf("读取 rule_id 配置失败: %v", err)
		}
		if savedConfig.Value != validRuleID {
			t.Fatalf("原有 rule_id 被修改: got %q, want %q", savedConfig.Value, validRuleID)
		}
	})
}

// Feature: symedia-manual-transfer, Property 3: Symedia payload assembly correctness
// Validates: Requirements 4.2, 4.3, 5.2, 5.3
// 对于任意有效的归档请求和配置值，组装后的 Symedia payload 应包含正确的 items 和 transferForm。
func TestProperty_SymediaPayloadAssemblyCorrectness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 生成随机归档请求
		name := rapid.StringMatching(`[a-zA-Z0-9\x{4e00}-\x{9fff}]{2,20}`).Draw(t, "name")
		path := rapid.StringMatching(`/[a-zA-Z0-9/_]{5,50}`).Draw(t, "path")
		tmdbID := rapid.IntRange(1, 9999999).Draw(t, "tmdbid")
		mediaType := rapid.SampledFrom([]string{"movie", "tv"}).Draw(t, "media_type")

		var season *int
		if mediaType == "tv" {
			s := rapid.IntRange(1, 20).Draw(t, "season")
			season = &s
		}

		req := TransferRequest{
			Name:      name,
			Path:      path,
			TmdbID:    tmdbID,
			MediaType: mediaType,
			Season:    season,
		}

		// 生成随机归档配置
		ruleID := rapid.StringMatching(`[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).Draw(t, "rule_id")
		destDir := rapid.StringMatching(`/[a-zA-Z0-9/_]{5,50}`).Draw(t, "dest_dir")
		transferType := rapid.SampledFrom([]string{"copy", "move", "cd2_move", "cd2_copy"}).Draw(t, "transfer_type")
		category := rapid.Bool().Draw(t, "category")
		deleteDir := rapid.Bool().Draw(t, "delete_dir")
		extractMetadata := rapid.Bool().Draw(t, "extract_metadata")
		cacheMetadata := rapid.Bool().Draw(t, "cache_metadata")
		downloadNfo := rapid.Bool().Draw(t, "download_nfo")
		downloadImage := rapid.Bool().Draw(t, "download_image")

		cfg := TransferConfigResponse{
			RuleID:          ruleID,
			DestDir:         destDir,
			TransferType:    transferType,
			Category:        category,
			DeleteDir:       deleteDir,
			ExtractMetadata: extractMetadata,
			CacheMetadata:   cacheMetadata,
			DownloadNfo:     downloadNfo,
			DownloadImage:   downloadImage,
		}

		// 组装 payload（测试中不使用路径映射）
		payload := BuildSymediaPayload(req, cfg, []SyncMapping{})

		// 验证 items 数组
		if len(payload.Items) != 1 {
			t.Fatalf("items 数组长度应为 1，实际为 %d", len(payload.Items))
		}
		item := payload.Items[0]
		if item.Name != name {
			t.Fatalf("item.Name 不匹配: got %q, want %q", item.Name, name)
		}
		if item.Path != path {
			t.Fatalf("item.Path 不匹配: got %q, want %q", item.Path, path)
		}

		// 验证 transferForm 中来自请求的字段
		form := payload.TransferForm
		if form.TmdbID != tmdbID {
			t.Fatalf("transferForm.TmdbID 不匹配: got %d, want %d", form.TmdbID, tmdbID)
		}
		if form.MediaType != mediaType {
			t.Fatalf("transferForm.MediaType 不匹配: got %q, want %q", form.MediaType, mediaType)
		}
		if mediaType == "tv" {
			if form.Season == nil || *form.Season != *season {
				t.Fatalf("transferForm.Season 不匹配")
			}
		}

		// 验证 transferForm 中来自配置的字段
		if form.RuleID != ruleID {
			t.Fatalf("transferForm.RuleID 不匹配: got %q, want %q", form.RuleID, ruleID)
		}
		if form.DestDir != destDir {
			t.Fatalf("transferForm.DestDir 不匹配: got %q, want %q", form.DestDir, destDir)
		}
		if form.TransferType != transferType {
			t.Fatalf("transferForm.TransferType 不匹配: got %q, want %q", form.TransferType, transferType)
		}
		if form.Category != category {
			t.Fatalf("transferForm.Category 不匹配: got %v, want %v", form.Category, category)
		}
		if form.DeleteDir != deleteDir {
			t.Fatalf("transferForm.DeleteDir 不匹配: got %v, want %v", form.DeleteDir, deleteDir)
		}
		if form.ExtractMetadata != extractMetadata {
			t.Fatalf("transferForm.ExtractMetadata 不匹配: got %v, want %v", form.ExtractMetadata, extractMetadata)
		}
		if form.CacheMetadata != cacheMetadata {
			t.Fatalf("transferForm.CacheMetadata 不匹配: got %v, want %v", form.CacheMetadata, cacheMetadata)
		}
		if form.DownloadNfo != downloadNfo {
			t.Fatalf("transferForm.DownloadNfo 不匹配: got %v, want %v", form.DownloadNfo, downloadNfo)
		}
		if form.DownloadImage != downloadImage {
			t.Fatalf("transferForm.DownloadImage 不匹配: got %v, want %v", form.DownloadImage, downloadImage)
		}
	})
}

// TestPathReplacement_ManualConfig 测试 path_from/path_to 手动路径替换逻辑
func TestPathReplacement_ManualConfig(t *testing.T) {
	tests := []struct {
		name       string
		embyPath   string
		pathFrom   string
		pathTo     string
		syncList   []SyncMapping
		wantPath   string
	}{
		{
			name:     "手动替换：正常前缀匹配",
			embyPath: "/volume1/Video/Strm-Sa/电视剧/东南亚/冰冻情人节 (2026) {tmdb-307833}",
			pathFrom: "/volume1/Video/Strm-Sa",
			pathTo:   "/CloudNAS/CloudDrive/115open/Video",
			wantPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/东南亚/冰冻情人节 (2026) {tmdb-307833}",
		},
		{
			name:     "手动替换：path_from 带尾部斜杠",
			embyPath: "/volume1/Video/Strm-Sa/电视剧/韩剧/废柴舅舅 (2021) {tmdb-135605}",
			pathFrom: "/volume1/Video/Strm-Sa/",
			pathTo:   "/CloudNAS/CloudDrive/115open/Video/",
			wantPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/韩剧/废柴舅舅 (2021) {tmdb-135605}",
		},
		{
			name:     "手动替换：路径完全等于 path_from",
			embyPath: "/volume1/Video/Strm-Sa",
			pathFrom: "/volume1/Video/Strm-Sa",
			pathTo:   "/CloudNAS/CloudDrive/115open/Video",
			wantPath: "/CloudNAS/CloudDrive/115open/Video",
		},
		{
			name:     "手动替换优先于 sync_list",
			embyPath: "/volume1/Video/Strm-Sa/电视剧/测试",
			pathFrom: "/volume1/Video/Strm-Sa",
			pathTo:   "/CloudNAS/正确路径",
			syncList: []SyncMapping{
				{SymlinkDir: "/volume1/Video/Strm-Sa", MediaDir: "/错误路径"},
			},
			wantPath: "/CloudNAS/正确路径/电视剧/测试",
		},
		{
			name:     "都留空：回退到 sync_list",
			embyPath: "/media/电视剧/测试",
			pathFrom: "",
			pathTo:   "",
			syncList: []SyncMapping{
				{SymlinkDir: "/media", MediaDir: "/CloudNAS/CloudDrive/115open/Video"},
			},
			wantPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/测试",
		},
		{
			name:     "都留空且无 sync_list：路径不变",
			embyPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/测试",
			pathFrom: "",
			pathTo:   "",
			syncList: []SyncMapping{},
			wantPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/测试",
		},
		{
			name:     "只填 path_from 没填 path_to：回退到 sync_list",
			embyPath: "/media/电视剧/测试",
			pathFrom: "/media",
			pathTo:   "",
			syncList: []SyncMapping{
				{SymlinkDir: "/media", MediaDir: "/CloudNAS/Video"},
			},
			wantPath: "/CloudNAS/Video/电视剧/测试",
		},
		{
			name:     "只填 path_to 没填 path_from：回退到 sync_list",
			embyPath: "/media/电视剧/测试",
			pathFrom: "",
			pathTo:   "/CloudNAS/Video",
			syncList: []SyncMapping{
				{SymlinkDir: "/media", MediaDir: "/CloudNAS/Video"},
			},
			wantPath: "/CloudNAS/Video/电视剧/测试",
		},
		{
			name:     "手动配置了但前缀不匹配：路径不变，不走 sync_list",
			embyPath: "/other/path/电视剧/测试",
			pathFrom: "/volume1/Video/Strm-Sa",
			pathTo:   "/CloudNAS/CloudDrive/115open/Video",
			syncList: []SyncMapping{
				{SymlinkDir: "/other/path", MediaDir: "/不应该匹配"},
			},
			wantPath: "/other/path/电视剧/测试",
		},
		{
			name:     "path_from 带空格：trim 后正常工作",
			embyPath: "/volume1/Video/Strm-Sa/电视剧/测试",
			pathFrom: "  /volume1/Video/Strm-Sa  ",
			pathTo:   "  /CloudNAS/CloudDrive/115open/Video  ",
			wantPath: "/CloudNAS/CloudDrive/115open/Video/电视剧/测试",
		},
		{
			name:     "防止部分路径段匹配：path_from 是另一个路径的前缀子串",
			embyPath: "/volume1/Video/Strm-Sa2/电视剧/测试",
			pathFrom: "/volume1/Video/Strm-Sa",
			pathTo:   "/CloudNAS/Video",
			syncList: []SyncMapping{},
			wantPath: "/volume1/Video/Strm-Sa2/电视剧/测试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TransferRequest{
				Name:      "测试",
				Path:      tt.embyPath,
				TmdbID:    12345,
				MediaType: "tv",
			}
			cfg := TransferConfigResponse{
				RuleID:       "test-rule",
				DestDir:      "/dest",
				TransferType: "cd2_move",
				PathFrom:     tt.pathFrom,
				PathTo:       tt.pathTo,
			}
			payload := BuildSymediaPayload(req, cfg, tt.syncList)
			got := payload.Items[0].Path
			if got != tt.wantPath {
				t.Errorf("路径不匹配:\n  got:  %q\n  want: %q", got, tt.wantPath)
			}
		})
	}
}
