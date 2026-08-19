// asr-adapter-go —— ASR 适配微服务:把 ESP 自定义契约 {chunks:[base64]} 转发到集群内
// Whisper 引擎,解析回 {text}。纯 Go,无状态转发层(不跑模型),复用 esp-common-go 中间件。
//
// 契约(与 conversation-go asr.HTTPProvider 对齐):
//   POST /transcribe  {chunks:[base64...]}  ->  {text}
//
// 依赖:WHISPER_ENDPOINT(必填,指向 whisper 容器 /v1/audio/transcriptions)。
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

	"esp/asr-adapter-go/src/adapter"
	"esp/common-go/config"
	"esp/common-go/health"
	"esp/common-go/middleware"
)

const serviceName = "asr-adapter-go"

func main() {
	port := config.Getenv("PORT", "8097")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	whisper := adapter.NewWhisperClient() // WHISPER_ENDPOINT 未配置则 os.Exit(1)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(50, 100))
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP"}) })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.POST("/transcribe", func(c *gin.Context) {
		var req adapter.TranscribeReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid body"})
			return
		}
		text := whisper.Transcribe(req.Chunks)
		c.JSON(200, adapter.TranscribeResp{Text: text})
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
