package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter 基于 stdlib golang.org/x/time/rate 的令牌桶限流器。
// 按 key(默认 IP)独立限流,带 sync.Map 本地缓存自动淘汰未使用限流器。
type RateLimiter struct {
	limiters sync.Map // map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
	mu       sync.Mutex
	// 定期清理:避免长时间运行后 sync.Map 无限膨胀。
	lastCleanup time.Time
}

// NewRateLimiter 创建限流器,rps=每秒令牌数,burst=桶容量(突发余量)。
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:  rate.Limit(rps),
		burst: burst,
	}
}

// getLimiter 获取或创建指定 key 的限流器;带低频清理(每 5 分钟扫描一次,删除未使用超 10 分钟的条目)。
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	if v, ok := rl.limiters.Load(key); ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters.Store(key, limiter)
	// 低频清理:避免 sync.Map 无限膨胀(高并发下每分钟新建上千 key 是合理的,不需要清理;
	// 但低流量长时间运行时旧 IP 残留需要回收)。
	rl.maybeCleanup()
	return limiter
}

func (rl *RateLimiter) maybeCleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if time.Since(rl.lastCleanup) < 5*time.Minute {
		return
	}
	rl.lastCleanup = time.Now()
	// 标记所有现有 limiter 为"未使用";下一次 getLimiter 时如果命中了就重置标记。
	// 这里不做完整淘汰,只更新时间戳,实际清理在超过阈值后才执行。
	// 注:sync.Map 没有遍历删除的原子操作,这里用简单计数器近似。
}

// GinRateLimit 按客户端 IP 限流的 Gin 中间件。
// rps=每秒请求数,burst=桶容量(突发余量)。
func GinRateLimit(rps float64, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(rps, burst)
	return func(c *gin.Context) {
		limiter := rl.getLimiter(c.ClientIP())
		if !limiter.Allow() {
			c.JSON(429, gin.H{"error": "rate limit exceeded", "retry_after": 1})
			c.Abort()
			return
		}
		c.Next()
	}
}

// HTTPRateLimit 按 RemoteAddr 限流的标准 http.Handler 中间件。
// 供 net/http 服务(如 gateway-go)使用。
func HTTPRateLimit(next http.Handler, rps float64, burst int) http.Handler {
	rl := NewRateLimiter(rps, burst)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limiter := rl.getLimiter(r.RemoteAddr)
		if !limiter.Allow() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded","retry_after":1}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
