// Package health —— ESP Go 服务统一健康检查 handler(会话22-3 债1+2)。
// 消除 4 种写法(gin 三处格式不一 + net/http 一处),补 live-tutor-go/course-path-go 缺口。
package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GinHealthz gin 版 /healthz,返回 {"status":"UP","service":<name>}。对齐 gamification/listening/reading 原格式。
func GinHealthz(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "UP", "service": serviceName})
	}
}

// GinLivez gin 版 /livez,返回 {"status":"UP"}。对齐 gamification 原格式。
func GinLivez() gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP"}) }
}

// StdHealthz net/http 版 /healthz(供 gateway-go / live-tutor-go 等非 gin 服务)。
func StdHealthz(serviceName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"UP","service":"` + serviceName + `"}`))
	}
}
