// tts-adapter-go —— TTS 适配微服务:把 ESP 自定义契约 {text} 转发到 edge-tts(Go 直连微软),
// 生成真实 MP3 音频(base64) + 四态情绪 + Preston Blair 口型时间轴。
// 纯 Go,零 Python/fastapi,复用 esp-common-go 中间件。
//
// 契约(与 conversation-go tts.HTTPProvider 对齐):
//   POST /synthesize  {text:"..."}  ->  {audio:"<base64>", emotion:"happy|neutral|encouraging|sad", visemes:[...]}
//
// 依赖:EDGE_TTS_VOICE(可选,默认 en-US-JennyNeural)。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"esp/common-go/config"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/tts-adapter-go/src/adapter"
)

const serviceName = "tts-adapter-go"

func main() {
	port := config.Getenv("PORT", "8098")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	edgeTts := adapter.NewEdgeTtsClient() // 无硬依赖,env 可选

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(50, 100))
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP"}) })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.POST("/synthesize", func(c *gin.Context) {
		var req adapter.SynthesizeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		resp := edgeTts.Synthesize(c.Request.Context(), req.Text)
		c.JSON(200, resp)
	})

	httpSrv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[%s] shutting down", serviceName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
