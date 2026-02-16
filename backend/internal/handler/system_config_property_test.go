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

// setupSystemConfigTest 创建测试用的 Gin 引擎和 SystemConfigHandler
func setupSystemConfigTest(t *testing.T) (*gin.Engine, *SystemConfigHandler) {
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

	h := NewSystemConfigHandler(db)

	r := gin.New()
	r.GET("/api/system-config", h.GetAllConfigs)
	r.PUT("/api/system-config/:key", h.UpdateConfig)

	return r, h
}

// Feature: system-config, Property 3: GET returns all configs
// Validates: Requirements 2.1
// 对于任意一组 SystemConfig 记录，GET 端点应返回包含所有记录的列表，key 和 value 均匹配。
func TestProperty_GetAllConfigs(t *testing.T) {
	r, h := setupSystemConfigTest(t)

	rapid.Check(t, func(t *rapid.T) {
		// 清理非种子数据（保留 tmdb_api_key）
		h.DB.Where("key != ?", "tmdb_api_key").Delete(&model.SystemConfig{})

		// 生成随机配置项并插入
		numConfigs := rapid.IntRange(0, 5).Draw(t, "numConfigs")
		inserted := make(map[string]string)
		// 种子数据 tmdb_api_key 始终存在
		var seedConfig model.SystemConfig
		h.DB.Where("key = ?", "tmdb_api_key").First(&seedConfig)
		inserted["tmdb_api_key"] = seedConfig.Value

		for i := 0; i < numConfigs; i++ {
			key := fmt.Sprintf("test_key_%d_%s", i, rapid.StringMatching(`[a-z]{3,8}`).Draw(t, fmt.Sprintf("key_%d", i)))
			value := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, fmt.Sprintf("value_%d", i))

			// 确保 key 不重复
			if _, exists := inserted[key]; exists {
				continue
			}

			config := model.SystemConfig{Key: key, Value: value, Description: "test"}
			if err := h.DB.Create(&config).Error; err != nil {
				t.Fatalf("插入配置失败: %v", err)
			}
			inserted[key] = value
		}

		// 调用 GET 端点
		req := httptest.NewRequest(http.MethodGet, "/api/system-config", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("GET 应返回 200，实际返回 %d", w.Code)
		}

		// 解析响应
		var resp struct {
			Data []model.SystemConfig `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		// 验证返回的记录数量
		if len(resp.Data) != len(inserted) {
			t.Fatalf("返回记录数不匹配: got %d, want %d", len(resp.Data), len(inserted))
		}

		// 验证每条记录的 key 和 value
		returned := make(map[string]string)
		for _, c := range resp.Data {
			returned[c.Key] = c.Value
		}
		for key, value := range inserted {
			if got, ok := returned[key]; !ok {
				t.Fatalf("缺少配置项: %q", key)
			} else if got != value {
				t.Fatalf("配置项 %q 的值不匹配: got %q, want %q", key, got, value)
			}
		}

		// 清理本轮插入的测试数据
		h.DB.Where("key != ?", "tmdb_api_key").Delete(&model.SystemConfig{})
	})
}

// Feature: system-config, Property 4: PUT update round-trip
// Validates: Requirements 2.2
// 对于任意已存在的 key 和新 value，PUT 更新后从数据库读取应返回更新后的值。
func TestProperty_PutUpdateRoundTrip(t *testing.T) {
	r, h := setupSystemConfigTest(t)

	rapid.Check(t, func(t *rapid.T) {
		// 创建一个测试配置项
		key := rapid.StringMatching(`[a-z][a-z0-9_]{2,20}`).Draw(t, "key")
		initialValue := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "initialValue")
		newValue := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "newValue")

		// 清理并插入初始记录
		h.DB.Where("key = ?", key).Delete(&model.SystemConfig{})
		config := model.SystemConfig{Key: key, Value: initialValue, Description: "test"}
		if err := h.DB.Create(&config).Error; err != nil {
			t.Fatalf("插入初始配置失败: %v", err)
		}

		// 发送 PUT 请求
		body, _ := json.Marshal(UpdateConfigRequest{Value: newValue})
		req := httptest.NewRequest(http.MethodPut, "/api/system-config/"+key, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("PUT 应返回 200，实际返回 %d, body: %s", w.Code, w.Body.String())
		}

		// 从数据库读取验证
		var loaded model.SystemConfig
		if err := h.DB.Where("key = ?", key).First(&loaded).Error; err != nil {
			t.Fatalf("读取更新后的配置失败: %v", err)
		}

		if loaded.Value != newValue {
			t.Fatalf("更新后的值不匹配: got %q, want %q", loaded.Value, newValue)
		}

		// 清理
		h.DB.Where("key = ?", key).Delete(&model.SystemConfig{})
	})
}

// TestUpdateConfig_NotFound 验证 PUT 不存在的 key 返回 404
func TestUpdateConfig_NotFound(t *testing.T) {
	r, _ := setupSystemConfigTest(t)

	body, _ := json.Marshal(UpdateConfigRequest{Value: "some-value"})
	req := httptest.NewRequest(http.MethodPut, "/api/system-config/nonexistent_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("不存在的 key 应返回 404，实际返回 %d", w.Code)
	}
}
