package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"embyforge/internal/model"

	"github.com/gin-gonic/gin"
	"pgregory.net/rapid"
)

// setupEmbyConfigTest 创建测试用的 Gin 引擎和 EmbyConfigHandler
func setupEmbyConfigTest(t *testing.T) (*gin.Engine, *EmbyConfigHandler) {
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

	h := NewEmbyConfigHandler(db)

	r := gin.New()
	r.GET("/api/emby-config/server-info", h.GetServerInfo)

	return r, h
}

// newFakeEmbyServer 创建一个模拟 Emby 服务器，返回指定的 serverID
// 如果 reachable 为 false，返回 500 状态码模拟不可达
func newFakeEmbyServer(serverID string, reachable bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !reachable {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"Id":         serverID,
			"ServerName": "TestEmby",
		})
	}))
}

// Feature: emby-url-simplification, Property 1: URL 选择优先级
// Validates: Requirements 1.2, 1.4, 1.5
//
// 对于任意 Emby 配置和可达性状态组合，GetServerInfo 返回的 base_url 应遵循：
// 内网可达 → 返回内网地址；内网不可达但外网可达 → 返回外网地址；都不可达 → 回退内网地址且 connected=false
func TestProperty_URLSelectionPriority(t *testing.T) {
	r, h := setupEmbyConfigTest(t)

	rapid.Check(t, func(rt *rapid.T) {
		// 随机生成可达性状态
		internalReachable := rapid.Bool().Draw(rt, "internalReachable")
		externalReachable := rapid.Bool().Draw(rt, "externalReachable")
		hasExternalURL := rapid.Bool().Draw(rt, "hasExternalURL")

		serverID := rapid.StringMatching(`[a-f0-9]{16}`).Draw(rt, "serverID")

		// 创建模拟 Emby 服务器
		internalServer := newFakeEmbyServer(serverID, internalReachable)
		defer internalServer.Close()

		var externalServer *httptest.Server
		externalURL := ""
		if hasExternalURL {
			externalServer = newFakeEmbyServer(serverID, externalReachable)
			defer externalServer.Close()
			externalURL = externalServer.URL
		}

		// httptest.Server 的 URL 格式为 http://127.0.0.1:PORT，需要拆分为 host 和 port
		// 但 probeEmbyURL 接受完整的 baseURL，而 GetServerInfo 用 fmt.Sprintf("%s:%d", host, port) 构建
		// 所以我们需要让 host 包含 scheme，port 为实际端口
		// 解析 internalServer.URL
		internalURL := internalServer.URL // 如 http://127.0.0.1:12345

		// 插入配置：使用 host=internalServer.URL, port=0 的方式不行
		// 因为 GetServerInfo 用 fmt.Sprintf("%s:%d", config.Host, config.Port) 构建 URL
		// 我们需要让构建出的 URL 等于 internalServer.URL
		// internalServer.URL 格式为 http://127.0.0.1:PORT
		// 所以设置 Host = "http://127.0.0.1", Port = PORT
		var host string
		var port int
		// 解析 URL 获取端口
		for i := len(internalURL) - 1; i >= 0; i-- {
			if internalURL[i] == ':' {
				host = internalURL[:i]
				p := internalURL[i+1:]
				for _, c := range p {
					port = port*10 + int(c-'0')
				}
				break
			}
		}

		config := model.EmbyConfig{
			Host:        host,
			Port:        port,
			APIKey:      "test-api-key",
			ExternalURL: externalURL,
		}
		h.DB.Delete(&model.EmbyConfig{}, "1=1")
		if err := h.DB.Create(&config).Error; err != nil {
			t.Fatalf("插入配置失败: %v", err)
		}

		// 调用 GetServerInfo
		req := httptest.NewRequest(http.MethodGet, "/api/emby-config/server-info", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d, body: %s", w.Code, w.Body.String())
		}

		var resp struct {
			Data struct {
				BaseURL   string `json:"base_url"`
				ServerID  string `json:"server_id"`
				APIKey    string `json:"api_key"`
				Connected bool   `json:"connected"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		expectedInternalURL := internalURL

		// 验证 URL 选择优先级
		if internalReachable {
			// 内网可达 → 返回内网地址
			if resp.Data.BaseURL != expectedInternalURL {
				t.Fatalf("内网可达时应返回内网地址 %q，实际 %q", expectedInternalURL, resp.Data.BaseURL)
			}
			if !resp.Data.Connected {
				t.Fatalf("内网可达时 connected 应为 true")
			}
		} else if hasExternalURL && externalReachable {
			// 内网不可达但外网可达 → 返回外网地址
			if resp.Data.BaseURL != externalURL {
				t.Fatalf("外网可达时应返回外网地址 %q，实际 %q", externalURL, resp.Data.BaseURL)
			}
			if !resp.Data.Connected {
				t.Fatalf("外网可达时 connected 应为 true")
			}
		} else {
			// 都不可达 → 回退内网地址，connected=false
			if resp.Data.BaseURL != expectedInternalURL {
				t.Fatalf("都不可达时应回退内网地址 %q，实际 %q", expectedInternalURL, resp.Data.BaseURL)
			}
			if resp.Data.Connected {
				t.Fatalf("都不可达时 connected 应为 false")
			}
		}
	})
}

// Feature: emby-url-simplification, Property 2: 响应完整性
// Validates: Requirements 2.1, 2.2, 2.3
//
// 对于任意有效的 Emby 配置，GetServerInfo 的响应应始终包含 base_url、server_id、api_key、connected 四个字段，
// 且 base_url 和 api_key 不为空字符串。
func TestProperty_ResponseCompleteness(t *testing.T) {
	r, h := setupEmbyConfigTest(t)

	rapid.Check(t, func(rt *rapid.T) {
		// 随机生成可达性
		reachable := rapid.Bool().Draw(rt, "reachable")
		hasExternalURL := rapid.Bool().Draw(rt, "hasExternalURL")
		apiKey := rapid.StringMatching(`[a-f0-9]{8,32}`).Draw(rt, "apiKey")

		// 创建模拟 Emby 服务器
		internalServer := newFakeEmbyServer("server-123", reachable)
		defer internalServer.Close()

		externalURL := ""
		if hasExternalURL {
			extServer := newFakeEmbyServer("server-123", rapid.Bool().Draw(rt, "extReachable"))
			defer extServer.Close()
			externalURL = extServer.URL
		}

		// 解析 internalServer.URL 为 host:port
		internalURL := internalServer.URL
		var host string
		var port int
		for i := len(internalURL) - 1; i >= 0; i-- {
			if internalURL[i] == ':' {
				host = internalURL[:i]
				p := internalURL[i+1:]
				for _, c := range p {
					port = port*10 + int(c-'0')
				}
				break
			}
		}

		h.DB.Delete(&model.EmbyConfig{}, "1=1")
		config := model.EmbyConfig{
			Host:        host,
			Port:        port,
			APIKey:      apiKey,
			ExternalURL: externalURL,
		}
		if err := h.DB.Create(&config).Error; err != nil {
			t.Fatalf("插入配置失败: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/emby-config/server-info", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("期望 200，实际 %d", w.Code)
		}

		// 解析为 raw map 验证字段存在性
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		// data 字段必须存在
		dataRaw, ok := raw["data"]
		if !ok {
			t.Fatalf("响应缺少 data 字段")
		}

		var data map[string]json.RawMessage
		if err := json.Unmarshal(dataRaw, &data); err != nil {
			t.Fatalf("解析 data 字段失败: %v", err)
		}

		// 验证四个必需字段存在
		requiredFields := []string{"base_url", "server_id", "api_key", "connected"}
		for _, field := range requiredFields {
			if _, exists := data[field]; !exists {
				t.Fatalf("响应 data 缺少字段: %q", field)
			}
		}

		// 验证 base_url 不为空
		var baseURL string
		json.Unmarshal(data["base_url"], &baseURL)
		if baseURL == "" {
			t.Fatalf("base_url 不应为空字符串")
		}

		// 验证 api_key 不为空
		var respAPIKey string
		json.Unmarshal(data["api_key"], &respAPIKey)
		if respAPIKey == "" {
			t.Fatalf("api_key 不应为空字符串")
		}

		// 验证 api_key 与配置一致
		if respAPIKey != apiKey {
			t.Fatalf("api_key 不匹配: got %q, want %q", respAPIKey, apiKey)
		}
	})
}
