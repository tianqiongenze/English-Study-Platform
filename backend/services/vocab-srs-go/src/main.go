// Package main 是 vocab-srs-go(S2)的入口。
//
// 职责(ADR §3 第 S2 项,P0):词汇间隔重复(SRS)记忆 + CRUD。
// 端口:8087。
// 依赖:CRDB(esp_vocab schema,postgres 协议)读写分离 + Redis Cluster(可选,后续阶段接缓存)。
//
// 配置(env,均带默认值):
//   PORT          监听端口(默认 8087)
//   READ_DB_URL   读库完整 postgres DSN(优先,默认 crdb-public:26257/esp_vocab)
//   WRITE_DB_URL  写库完整 postgres DSN(优先,默认 crdb-public:26257/esp_vocab)
//   GIN_MODE      gin 模式 release/debug(默认 release)
//
// 健康检查:
//   /healthz 进程存活(恒 200)
//   /readyz  CRDB 读库可连
//   /livez   进程存活(恒 200)
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"esp/vocab-srs-go/src/dao"
	"esp/vocab-srs-go/src/handler"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/common-go/registry"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const serviceName = "vocab-srs-go"

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Infof("[%s] starting", serviceName)

	port := config.Getenv("PORT", "8087")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))

	// DSN 拼装:env 完整 postgres DSN 优先,否则默认 CRDB(esp_vocab)。
	readDSN := config.Getenv("READ_DB_URL", "")
	writeDSN := config.Getenv("WRITE_DB_URL", "")
	if readDSN == "" {
		readDSN = db.DefaultDSN("crdb-public:26257", "esp_vocab")
	}
	if writeDSN == "" {
		writeDSN = db.DefaultDSN("crdb-public:26257", "esp_vocab")
	}

	d, err := dao.NewVocabDAO(readDSN, writeDSN)
	if err != nil {
		sugar.Fatalf("[%s] init dao: %v", serviceName, err)
	}
	defer d.Close()

	h := &handler.Handler{DAO: d, Log: sugar}

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(serviceName, 8087)

	r := gin.New()
	r.Use(gin.Recovery(), traceMiddleware(sugar))
	r.Use(middleware.GinRateLimit(100, 200))                                           // 100 rps,桶容量200(突发余量)
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))                // 5次失败触发30s熔断

	// 健康检查
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "DOWN", "err": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	// 业务路由
	v1 := r.Group("/api/vocab")
	{
		v1.POST("/review", h.Review)
		v1.GET("/due", h.Due)
		v1.POST("/word", h.CreateWord)
		v1.GET("/word/:id", h.GetWord)
		v1.DELETE("/word/:id", h.DeleteWord)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 优雅关闭
	go func() {
		sugar.Infof("[%s] listening on :%s", serviceName, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sugar.Fatalf("[%s] serve: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	sugar.Infof("[%s] shutting down", serviceName)
	registry.DeRegister(serviceName, 8087, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		sugar.Errorf("[%s] shutdown: %v", serviceName, err)
	}
}

// traceMiddleware 从 X-Trace-Id 透传/生成 traceId,写到 response header 与日志。
func traceMiddleware(log *zap.SugaredLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader("X-Trace-Id")
		if tid == "" {
			tid = fmt.Sprintf("v-%d", time.Now().UnixNano())
		}
		c.Set("traceId", tid)
		c.Writer.Header().Set("X-Trace-Id", tid)
		start := time.Now()
		c.Next()
		log.Infof("trace=%s method=%s path=%s status=%d dur=%dms",
			tid, c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start).Milliseconds())
	}
}

func getenv(k, d string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, d)
}
