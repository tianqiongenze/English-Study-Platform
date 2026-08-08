// Package main 是 vocab-srs-go(S2)的入口。
//
// 职责(ADR §3 第 S2 项,P0):词汇间隔重复(SRS)记忆 + CRUD。
// 端口:8087。
// 依赖:TiDB(esp_vocab schema)读写分离 + Redis Cluster(可选,后续阶段接缓存)。
//
// 配置(env,均带默认值):
//   PORT          监听端口(默认 8087)
//   TIDB_HOST     TiDB 主机(默认 tidb-primary,ExternalName 短名)
//   TIDB_PORT     TiDB 端口(默认 4000)
//   TIDB_USER     TiDB 用户(默认 root)
//   TIDB_PASSWORD TiDB 密码(默认空)
//   TIDB_DB       TiDB 库名(默认 esp_vocab)
//   READ_DB_URL   读库完整 DSN(优先,默认走 tidb-follower)
//   WRITE_DB_URL  写库完整 DSN(优先,默认走 tidb-primary)
//   GIN_MODE      gin 模式 release/debug(默认 release)
//
// 健康检查:
//   /healthz 进程存活(恒 200)
//   /readyz  TiDB 读库可连
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

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const serviceName = "vocab-srs-go"

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar := logger.Sugar()
	sugar.Infof("[%s] starting", serviceName)

	port := getenv("PORT", "8087")
	gin.SetMode(getenv("GIN_MODE", "release"))

	// DSN 拼装:env 完整 DSN 优先,否则用 TIDB_HOST/PORT 组装。
	readDSN := getenv("READ_DB_URL", "")
	writeDSN := getenv("WRITE_DB_URL", "")
	if readDSN == "" {
		readDSN = buildDSN(getenv("TIDB_HOST_READ", "tidb-follower"))
	}
	if writeDSN == "" {
		writeDSN = buildDSN(getenv("TIDB_HOST", "tidb-primary"))
	}

	d, err := dao.NewVocabDAO(readDSN, writeDSN)
	if err != nil {
		sugar.Fatalf("[%s] init dao: %v", serviceName, err)
	}
	defer d.Close()

	h := &handler.Handler{DAO: d, Log: sugar}

	r := gin.New()
	r.Use(gin.Recovery(), traceMiddleware(sugar))

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": serviceName})
	})
	r.GET("/livez", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})
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

// buildDSN 拼装 mysql DSN(默认 root 无密码,db esp_vocab)。
// parseTime=true 让 DATETIME 自动 scan 到 time.Time。
func buildDSN(host string) string {
	user := getenv("TIDB_USER", "root")
	pwd := getenv("TIDB_PASSWORD", "")
	port := getenv("TIDB_PORT", "4000")
	db := getenv("TIDB_DB", "esp_vocab")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=UTC&timeout=5s&readTimeout=3s&writeTimeout=3s",
		user, pwd, host, port, db)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
