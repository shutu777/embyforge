package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // 每个时间窗口允许的请求数
	window   time.Duration // 时间窗口
}

// visitor 访问者记录
type visitor struct {
	lastSeen time.Time
	count    int
}

// NewRateLimiter 创建速率限制器
// 参数:
//   - rate: 每个时间窗口允许的请求数
//   - window: 时间窗口（例如：1分钟）
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	
	// 启动清理goroutine，定期清理过期的访问者记录
	go rl.cleanupVisitors()
	
	return rl
}

// cleanupVisitors 定期清理过期的访问者记录
func (rl *RateLimiter) cleanupVisitors() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, v := range rl.visitors {
			if now.Sub(v.lastSeen) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	v, exists := rl.visitors[ip]
	
	if !exists {
		// 新访问者
		rl.visitors[ip] = &visitor{
			lastSeen: now,
			count:    1,
		}
		return true
	}
	
	// 检查时间窗口是否已过
	if now.Sub(v.lastSeen) > rl.window {
		// 重置计数器
		v.lastSeen = now
		v.count = 1
		return true
	}
	
	// 在时间窗口内，检查是否超过限制
	if v.count >= rl.rate {
		return false
	}
	
	// 增加计数
	v.count++
	v.lastSeen = now
	return true
}

// RateLimitMiddleware 返回速率限制中间件
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP
		ip := c.ClientIP()
		
		// 检查是否允许请求
		if !limiter.Allow(ip) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}
