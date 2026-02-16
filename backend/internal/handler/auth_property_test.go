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
	"golang.org/x/crypto/bcrypt"
	"pgregory.net/rapid"
)

// 创建测试用的 Gin 引擎和 AuthHandler
func setupAuthTest(t *testing.T) (*gin.Engine, *AuthHandler) {
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

	h := NewAuthHandler(db, "test-secret-key")

	r := gin.New()
	r.POST("/api/auth/login", h.Login)

	return r, h
}

// 在数据库中创建一个测试用户
func createTestUser(t *testing.T, h *AuthHandler, username, password string) {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("密码哈希失败: %v", err)
	}
	user := model.User{
		Username: username,
		Password: string(hashed),
	}
	if err := h.DB.Create(&user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
}

// Feature: embyforge, Property 1: 登录凭据验证一致性
// Validates: Requirements 1.1, 1.2
// 对于任意用户名和密码组合，如果该凭据与数据库中某个用户的用户名匹配且密码哈希验证通过，
// 则登录接口返回有效的 JWT 令牌；否则返回 401 状态码。
func TestProperty_LoginCredentialConsistency(t *testing.T) {
	r, h := setupAuthTest(t)

	// 创建一个已知用户（除了 seedAdmin 创建的 admin/admin）
	createTestUser(t, h, "testuser", "testpass123")

	rapid.Check(t, func(t *rapid.T) {
		// 生成随机用户名和密码
		username := rapid.OneOf(
			// 有时使用已知存在的用户名
			rapid.Just("admin"),
			rapid.Just("testuser"),
			// 有时使用随机用户名（大概率不存在）
			rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{2,15}`),
		).Draw(t, "username")

		password := rapid.OneOf(
			// 有时使用正确密码
			rapid.Just("admin"),
			rapid.Just("testpass123"),
			// 有时使用随机密码
			rapid.StringMatching(`[a-zA-Z0-9!@#]{3,20}`),
		).Draw(t, "password")

		// 判断预期结果：凭据是否有效
		isValidCredential := (username == "admin" && password == "admin") ||
			(username == "testuser" && password == "testpass123")

		// 发送登录请求
		body, _ := json.Marshal(LoginRequest{
			Username: username,
			Password: password,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if isValidCredential {
			// 有效凭据应返回 200 和 JWT 令牌
			if w.Code != http.StatusOK {
				t.Fatalf("有效凭据 (%s/%s) 应返回 200，实际返回 %d", username, password, w.Code)
			}
			var resp LoginResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if resp.Token == "" {
				t.Fatalf("有效凭据应返回非空令牌")
			}
			if resp.Username != username {
				t.Fatalf("响应用户名不匹配: got %q, want %q", resp.Username, username)
			}
		} else {
			// 无效凭据应返回 401
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("无效凭据 (%s/%s) 应返回 401，实际返回 %d", username, password, w.Code)
			}
		}
	})
}
