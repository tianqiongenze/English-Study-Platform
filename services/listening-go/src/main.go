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
	_ "github.com/go-sql-driver/mysql"
)

const serviceName = "listening-go"

var (
	readDB  *sql.DB
	writeDB *sql.DB
)

func main() {
	port := getenv("PORT", "8089")
	gin.SetMode(getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	readDB = mustConnectDB(getenv("READ_DB_URL", "root:@tcp(tidb-follower:4000)/esp_listening?parseTime=true"))
	writeDB = mustConnectDB(getenv("WRITE_DB_URL", "root:@tcp(tidb-primary:4000)/esp_listening?parseTime=true"))
	log.Printf("[%s] TiDB pools ready (read+write)", serviceName)

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP", "service": serviceName}) })
	r.GET("/livez", func(c *gin.Context) { c.JSON(200, gin.H{"status": "UP"}) })
	r.GET("/readyz", readyz)

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
		"SELECT id, title, cefr_level, duration_sec FROM listening_exercises ORDER BY id LIMIT ?", n)
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
		"SELECT title, audio_url, cefr_level, duration_sec, question_json FROM listening_exercises WHERE id=?", id).
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
		"INSERT INTO listening_attempts (user_id, exercise_id, answers_json, score, duration_ms) VALUES (?, ?, ?, ?, ?)",
		body.UserID, body.ExerciseID, string(ansJSON), body.Score, body.DurationMs)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"accepted": true, "score": body.Score})
}

// 音频转写流式占位(SSE)
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
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("[%s] db open: %v", serviceName, err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	if err := db.Ping(); err != nil {
		log.Fatalf("[%s] db ping: %v", serviceName, err)
	}
	return db
}

func getenv(k, dft string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return dft
}
