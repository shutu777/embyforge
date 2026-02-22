package emby

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// newTestClient 从 httptest.Server 创建一个正确配置的 Emby Client
// 解析 httptest 服务器 URL 的 host 和 port，避免 baseURL() 拼接错误
func newTestClient(server *httptest.Server) *Client {
	u, _ := url.Parse(server.URL)
	host := u.Scheme + "://" + u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	return &Client{
		Host:       host,
		Port:       port,
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	}
}

// Feature: ui-cache-improvements, Property 3: Delete item API fallback
// Validates: Requirements 3.2

// TestDeleteItem_PrimarySuccess 主端点成功时不应调用备用端点
func TestDeleteItem_PrimarySuccess(t *testing.T) {
	fallbackCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/emby/Items/Delete" {
			// 主端点成功
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/emby/Items/") {
			fallbackCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteItem(context.Background(), "item123")
	if err != nil {
		t.Fatalf("主端点成功时不应返回错误: %v", err)
	}
	if fallbackCalled {
		t.Error("主端点成功时不应调用备用端点")
	}
}

// TestDeleteItem_PrimaryFailsFallbackSuccess 主端点失败时应 fallback 到备用端点
func TestDeleteItem_PrimaryFailsFallbackSuccess(t *testing.T) {
	primaryCalled := false
	fallbackCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/emby/Items/Delete" {
			primaryCalled = true
			// 主端点返回 404（模拟不支持此端点的 Emby 版本）
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "not found"}`))
			return
		}
		if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/emby/Items/") {
			fallbackCalled = true
			// 备用端点成功
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteItem(context.Background(), "item123")
	if err != nil {
		t.Fatalf("备用端点成功时不应返回错误: %v", err)
	}
	if !primaryCalled {
		t.Error("应先尝试主端点")
	}
	if !fallbackCalled {
		t.Error("主端点失败后应调用备用端点")
	}
}

// TestDeleteItem_BothFail 两个端点都失败时应返回错误
func TestDeleteItem_BothFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 所有端点都返回 500
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "server error"}`))
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteItem(context.Background(), "item123")
	if err == nil {
		t.Fatal("两个端点都失败时应返回错误")
	}
}


// Feature: ui-cache-improvements, Property 5: Delete version API fallback
// Validates: Requirements 4.2

// TestDeleteVersion_PrimarySuccess 主端点成功时不应调用备用端点
func TestDeleteVersion_PrimarySuccess(t *testing.T) {
	fallbackCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/DeleteVersion") {
			// 主端点成功
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == "DELETE" {
			fallbackCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteVersion(context.Background(), "item456")
	if err != nil {
		t.Fatalf("主端点成功时不应返回错误: %v", err)
	}
	if fallbackCalled {
		t.Error("主端点成功时不应调用备用端点")
	}
}

// TestDeleteVersion_PrimaryFailsFallbackSuccess 主端点失败时应 fallback 到备用端点
func TestDeleteVersion_PrimaryFailsFallbackSuccess(t *testing.T) {
	primaryCalled := false
	fallbackCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/DeleteVersion") {
			primaryCalled = true
			// 主端点返回 405（模拟不支持此端点的 Emby 版本）
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error": "method not allowed"}`))
			return
		}
		if r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/emby/Items/") {
			fallbackCalled = true
			// 备用端点成功
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteVersion(context.Background(), "item456")
	if err != nil {
		t.Fatalf("备用端点成功时不应返回错误: %v", err)
	}
	if !primaryCalled {
		t.Error("应先尝试主端点")
	}
	if !fallbackCalled {
		t.Error("主端点失败后应调用备用端点")
	}
}

// TestDeleteVersion_BothFail 两个端点都失败时应返回错误
func TestDeleteVersion_BothFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 所有端点都返回 500
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "server error"}`))
	}))
	defer server.Close()

	client := newTestClient(server)

	err := client.DeleteVersion(context.Background(), "item456")
	if err == nil {
		t.Fatal("两个端点都失败时应返回错误")
	}
}
