package emby

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTestConnection_Success 测试连接成功场景
func TestTestConnection_Success(t *testing.T) {
	// 模拟 Emby 服务器返回成功响应
	expectedInfo := ServerInfo{
		ServerName: "TestEmbyServer",
		Version:    "4.7.14.0",
		ID:         "abc123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求路径
		if r.URL.Path != "/emby/System/Info" {
			t.Errorf("期望路径 /emby/System/Info，实际 %s", r.URL.Path)
		}

		// 验证 API Key 头
		if r.Header.Get("X-Emby-Token") != "test-api-key" {
			t.Errorf("期望 X-Emby-Token 为 test-api-key，实际 %s", r.Header.Get("X-Emby-Token"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedInfo)
	}))
	defer server.Close()

	// 使用 httptest 服务器地址创建客户端（端口已包含在 URL 中）
	client := &Client{
		Host:       server.URL,
		Port:       0, // httptest 的 URL 已包含端口
		APIKey:     "test-api-key",
		HTTPClient: server.Client(),
	}
	// 覆盖 baseURL 行为：httptest URL 已包含端口，不需要再拼接
	// 需要调整 client 使其兼容 httptest

	info, err := client.testConnectionWithURL(server.URL + "/emby/System/Info")
	if err != nil {
		t.Fatalf("测试连接不应失败: %v", err)
	}

	if info.ServerName != expectedInfo.ServerName {
		t.Errorf("期望服务器名称 %s，实际 %s", expectedInfo.ServerName, info.ServerName)
	}
	if info.Version != expectedInfo.Version {
		t.Errorf("期望版本 %s，实际 %s", expectedInfo.Version, info.Version)
	}
}

// TestTestConnection_Failure 测试连接失败场景
func TestTestConnection_Failure(t *testing.T) {
	// 模拟 Emby 服务器返回 401 未授权
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "Unauthorized"}`))
	}))
	defer server.Close()

	client := &Client{
		Host:       server.URL,
		Port:       0,
		APIKey:     "wrong-key",
		HTTPClient: server.Client(),
	}

	info, err := client.testConnectionWithURL(server.URL + "/emby/System/Info")
	if err == nil {
		t.Fatal("期望连接失败，但返回了成功")
	}
	if info != nil {
		t.Error("连接失败时不应返回服务器信息")
	}
}

// TestTestConnection_NetworkError 测试网络不可达场景
func TestTestConnection_NetworkError(t *testing.T) {
	client := NewClient("http://192.0.2.1", 9999, "test-key")
	// 设置极短超时以快速失败
	client.HTTPClient.Timeout = 1

	_, err := client.TestConnection()
	if err == nil {
		t.Fatal("期望网络错误，但返回了成功")
	}
}
