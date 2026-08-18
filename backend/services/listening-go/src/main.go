// listening-go —— 精听/泛听/听写/语速自适应 + 音频转写流式(S4, P1)
// scaffold 级:健康检查 + 听力素材/进度/听写提交路由桩 + esp_listening 表读写
// ADR: docs/design/adr-new-services.md / PRD §四 / schema esp_listening
//
// 配置(env):
//   PORT/READ_DB_URL/WRITE_DB_URL/GIN_MODE
// 健康检查:/healthz /readyz /livez
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/health"
	"esp/common-go/middleware"
	"esp/common-go/registry"
)

const serviceName = "listening-go"

var (
	readDB  *sql.DB
	writeDB *sql.DB
)

func main() {
	port := config.Getenv("PORT", "8089")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	readDB = db.MustConnectDB(config.Getenv("READ_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_listening")), serviceName)
	writeDB = db.MustConnectDB(config.Getenv("WRITE_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_listening")), serviceName)
	log.Printf("[%s] CRDB pools ready (read+write)", serviceName)

	// Nacos 服务注册(优雅退出时注销)
	nacosCli := registry.RegisterNacos(serviceName, 8089)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(100, 200))                                           // 100 rps,桶容量200(突发余量)
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))                // 5次失败触发30s熔断
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", readyz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api/listening")
	api.GET("/materials", listMaterials)          // 听力素材列表(桩)
	api.GET("/materials/:id", getMaterial)         // 单素材(桩)
	api.POST("/attempts", submitAttempt)           // 听写/做题提交(写 listening_attempts)
	api.POST("/transcribe", transcribe)            // 音频转写流式占位(SSE)

	srv := &http.Server{Addr: ":" + port, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[%s] server error: %v", serviceName, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[%s] shutting down", serviceName)
	registry.DeRegister(serviceName, 8089, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func readyz(c *gin.Context) {
	if err := readDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "db_read": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "UP", "db_read": true})
}

// 听力素材列表(桩):返回 listening_exercises 表前 N 条
// 表:id,title,audio_url,duration_sec,cefr_level,question_json,scene_tags,created_at
func listMaterials(c *gin.Context) {
	limit := c.DefaultQuery("limit", "10")
	n, err := strconv.Atoi(limit)
	if err != nil || n <= 0 {
		n = 10
	}
	rows, err := readDB.QueryContext(c.Request.Context(),
		"SELECT id, title, cefr_level, duration_sec FROM listening_exercises ORDER BY id LIMIT $1", n)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var id int64
		var title, level string
		var duration int
		if err := rows.Scan(&id, &title, &level, &duration); err != nil {
			continue
		}
		out = append(out, gin.H{"id": id, "title": title, "cefr_level": level, "duration_sec": duration})
	}
	c.JSON(200, out)
}

func getMaterial(c *gin.Context) {
	id := c.Param("id")
	var title, audioURL, level string
	var duration int
	var questionJSON sql.NullString
	err := readDB.QueryRowContext(c.Request.Context(),
		"SELECT title, audio_url, cefr_level, duration_sec, question_json FROM listening_exercises WHERE id=$1", id).
		Scan(&title, &audioURL, &level, &duration, &questionJSON)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"id": id, "title": title, "audio_url": audioURL, "cefr_level": level, "duration_sec": duration, "question_json": questionJSON.String})
}

// 听力做题/听写提交(写 listening_attempts)
// 表:id,user_id,exercise_id,answers_json,score,duration_ms,created_at
func submitAttempt(c *gin.Context) {
	var body struct {
		UserID     int64  `json:"user_id"`
		ExerciseID int64  `json:"exercise_id"`
		Answers    map[string]any `json:"answers"`
		Score      float64 `json:"score"`
		DurationMs int    `json:"duration_ms"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	ansJSON, _ := json.Marshal(body.Answers)
	_, err := writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO listening_attempts (user_id, exercise_id, answers_json, score, duration_ms) VALUES ($1, $2, $3::jsonb, $4, $5)",
		body.UserID, body.ExerciseID, string(ansJSON), body.Score, body.DurationMs)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"accepted": true, "score": body.Score})
}

// 音频转写(DEFERRED-真实ASR:需ASR引擎接入,见多节点清单#4;现状回声占位仅验证 SSE 通道)
func transcribe(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	flusher, _ := c.Writer.(http.Flusher)
	// scaffold:推送 3 个占位片段后结束
	chunks := []string{"Hello", ", this is", " a placeholder transcript."}
	for _, ch := range chunks {
		data, _ := json.Marshal(gin.H{"text": ch})
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func mustConnectDB(dsn string) *sql.DB {
	// 已迁移至 esp/common-go/db.MustConnectDB(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return db.MustConnectDB(dsn, serviceName)
}

func getenv(k, dft string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, dft)
}
