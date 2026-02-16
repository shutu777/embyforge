package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"pgregory.net/rapid"
)

// 创建带 JWT 中间件保护的测试路由
func setupJWTTest(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	protected := r.Group("/api")
	protected.Use(JWTAuth(secret))
	protected.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})
	return r
}

// Feature: embyforge, Property 2: JWT 认证中间件保护
// Validates: Requirements 1.4, 7.3
// 对于任意受保护的 API 端点和任意无效/缺失/过期的 JWT 令牌，请求该端点应返回 401 状态码。
func TestProperty_JWTMiddlewareProtection(t *testing.T) {
	secret := "test-jwt-secret"
	r := setupJWTTest(secret)

	rapid.Check(t, func(t *rapid.T) {
		// 生成不同类型的无效令牌场景
		tokenType := rapid.IntRange(0, 4).Draw(t, "tokenType")

		var authHeader string
		switch tokenType {
		case 0:
			// 无 Authorization 头
			authHeader = ""
		case 1:
			// 随机字符串（非 Bearer 格式）
			authHeader = rapid.StringMatching(`[a-zA-Z0-9]{5,30}`).Draw(t, "randomToken")
		case 2:
			// Bearer 前缀但令牌内容随机（无效签名）
			randomToken := rapid.StringMatching(`[a-zA-Z0-9._-]{10,50}`).Draw(t, "invalidToken")
			authHeader = "Bearer " + randomToken
		case 3:
			// 使用错误密钥签名的有效格式令牌
			wrongSecret := rapid.StringMatching(`[a-zA-Z0-9]{8,20}`).Draw(t, "wrongSecret")
			// 确保错误密钥与正确密钥不同
			if wrongSecret == secret {
				wrongSecret = wrongSecret + "x"
			}
			token, err := GenerateToken(1, "admin", wrongSecret)
			if err != nil {
				t.Fatalf("生成令牌失败: %v", err)
			}
			authHeader = "Bearer " + token
		case 4:
			// 已过期的令牌
			claims := Claims{
				UserID:   1,
				Username: "admin",
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
					IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				},
			}
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			tokenStr, err := token.SignedString([]byte(secret))
			if err != nil {
				t.Fatalf("生成过期令牌失败: %v", err)
			}
			authHeader = "Bearer " + tokenStr
		}

		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// 所有无效令牌场景都应返回 401
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("无效令牌类型 %d 应返回 401，实际返回 %d (header: %q)",
				tokenType, w.Code, authHeader)
		}
	})

	// 补充验证：有效令牌应返回 200
	t.Run("有效令牌应通过", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			userID := rapid.Uint().Draw(t, "userID")
			if userID == 0 {
				userID = 1
			}
			username := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9]{2,15}`).Draw(t, "username")

			token, err := GenerateToken(uint(userID), username, secret)
			if err != nil {
				t.Fatalf("生成令牌失败: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("有效令牌 (user=%s) 应返回 200，实际返回 %d", username, w.Code)
			}
		})
	})
}
