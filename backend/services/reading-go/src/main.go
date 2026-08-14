// reading-go —— 分级阅读 + 长难句解析 + 题目自动生成(S6, P1)
// scaffold 级:健康检查 + 文章/题目/答题提交路由桩 + esp_reading 表读写
// ADR: docs/design/adr-new-services.md / PRD §六 / schema esp_reading
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"

	"esp/common-go/config"
	"esp/common-go/db"
	"esp/common-go/health"
)

const serviceName = "reading-go"

var (
	readDB  *sql.DB
	writeDB *sql.DB
)

func main() {
	port := config.Getenv("PORT", "8091")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	readDB = db.MustConnectDB(config.Getenv("READ_DB_URL", db.DefaultDSN("tidb-follower:4000", "esp_reading")), serviceName)
	writeDB = db.MustConnectDB(config.Getenv("WRITE_DB_URL", db.DefaultDSN("tidb-primary:4000", "esp_reading")), serviceName)
	log.Printf("[%s] TiDB pools ready (read+write)", serviceName)

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", readyz)

	api := r.Group("/api/reading")
	api.GET("/passages", listPassages)        // 分级文章列表
	api.GET("/passages/:id", getPassage)      // 单文章(含 content)
	api.POST("/attempts", submitAttempt)      // 答题提交(写 reading_attempts)
	api.POST("/parse", parseSentence)         // 长难句解析(桩)

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

// 分级文章列表(表:id,title,content,word_count,cefr_level,source_type,questions_json,created_at)
func listPassages(c *gin.Context) {
	level := c.DefaultQuery("level", "")
	// TiDB 不接受字符串参数传给 LIMIT,必须转整型(同 listening-go 的坑)
	limitN, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limitN < 1 {
		limitN = 10
	}
	var rows *sql.Rows
	if level != "" {
		rows, err = readDB.QueryContext(c.Request.Context(),
			"SELECT id, title, cefr_level, word_count FROM reading_passages WHERE cefr_level=? ORDER BY id LIMIT ?", level, limitN)
	} else {
		rows, err = readDB.QueryContext(c.Request.Context(),
			"SELECT id, title, cefr_level, word_count FROM reading_passages ORDER BY id LIMIT ?", limitN)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var id int64
		var title, lvl string
		var wc int
		if err := rows.Scan(&id, &title, &lvl, &wc); err != nil {
			continue
		}
		out = append(out, gin.H{"id": id, "title": title, "cefr_level": lvl, "word_count": wc})
	}
	c.JSON(200, out)
}

func getPassage(c *gin.Context) {
	id := c.Param("id")
	var title, content, level string
	var wc int
	var sourceType sql.NullString
	var questionsJSON sql.NullString
	err := readDB.QueryRowContext(c.Request.Context(),
		"SELECT title, content, word_count, cefr_level, source_type, questions_json FROM reading_passages WHERE id=?", id).
		Scan(&title, &content, &wc, &level, &sourceType, &questionsJSON)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"id": id, "title": title, "content": content, "cefr_level": level, "word_count": wc, "source_type": sourceType.String, "questions_json": questionsJSON.String})
}

// 答题提交(写 reading_attempts)
// 表:id,user_id,passage_id,answers_json,score,duration_ms,created_at
func submitAttempt(c *gin.Context) {
	var body struct {
		UserID     int64        `json:"user_id"`
		PassageID  int64        `json:"passage_id"`
		Answers    map[string]any `json:"answers"`
		Score      float64      `json:"score"`
		DurationMs int          `json:"duration_ms"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	ansJSON, _ := json.Marshal(body.Answers)
	_, err := writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO reading_attempts (user_id, passage_id, answers_json, score, duration_ms) VALUES (?, ?, ?, ?, ?)",
		body.UserID, body.PassageID, string(ansJSON), body.Score, body.DurationMs)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"accepted": true, "score": body.Score})
}

// 长难句解析(桩)
func parseSentence(c *gin.Context) {
	var body struct {
		Sentence string `json:"sentence"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"sentence":   body.Sentence,
		"main_clause": "占位主句(LLM 长难句解析待接入)",
		"subordinate": []string{},
		"translation": "占位翻译",
	})
}

func mustConnectDB(dsn string) *sql.DB {
	// 已迁移至 esp/common-go/db.MustConnectDB(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return db.MustConnectDB(dsn, serviceName)
}

func getenv(k, dft string) string {
	// 已迁移至 esp/common-go/config.Getenv(会话22-3 债1+2)。保留空壳避免外部引用断裂。
	return config.Getenv(k, dft)
}
