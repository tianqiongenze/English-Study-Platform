// content-go -- 统一内容库(功能拓展:四六级/托福/雅思/教材共享内容)
// 四层:course -> lesson -> content_item;支持学习端浏览/作答 + 管理端 CRUD/导入。
// 架构:
//   - 学习端(任意登录用户):浏览课程/课时/题目(不含答案)、提交作答(判分 + 写
//     content_attempts + 发 learning_event_outbox 事件供 XP/掌握度管线,Phase3 消费)。
//   - 管理端(role=admin):课程/课时/题目 CRUD、状态流转(draft/pending/published/archived)、
//     批量导入、编辑建新版本(is_current 快照,不覆盖旧版本)。
//   身份:BFF 注入可信 X-User-Id / X-Role 头(EdgeAuthHandler 已剥离外网伪造)。
// 版权约束:source/license 必填标注;种子内容均为原创/改写/公开领域,不引用受版权保护原文。
// schema: scripts/crdb-schema.sql 的 esp_content 段。
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

const serviceName = "content-go"

// Server 持有依赖,所有 handler 通过方法绑定 -> 可注入 mock DB 做单测。
type Server struct {
	readDB  *sql.DB
	writeDB *sql.DB
}

func NewServer(w, r *sql.DB) *Server {
	return &Server{readDB: r, writeDB: w}
}

func main() {
	port := config.Getenv("PORT", "8096")
	gin.SetMode(config.Getenv("GIN_MODE", "release"))
	log.Printf("[%s] starting on :%s", serviceName, port)

	rDB := db.MustConnectDB(config.Getenv("READ_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_content")), serviceName)
	wDB := db.MustConnectDB(config.Getenv("WRITE_DB_URL", db.DefaultDSN("crdb-public:26257", "esp_content")), serviceName)
	log.Printf("[%s] CRDB pools ready (read+write)", serviceName)
	contentSrv := NewServer(wDB, rDB)
	contentSrv.startOutboxRelay()

	nacosCli := registry.RegisterNacos(serviceName, 8096)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinRateLimit(100, 200))
	r.Use(middleware.GinCircuitBreaker(5, 30*time.Second, serviceName))
	r.GET("/healthz", health.GinHealthz(serviceName))
	r.GET("/livez", health.GinLivez())
	r.GET("/readyz", contentSrv.readyz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	api := r.Group("/api/content")
	api.GET("/courses", contentSrv.listCourses)
	api.GET("/courses/:id", contentSrv.getCourse)
	api.GET("/courses/:id/lessons", contentSrv.listLessons)
	api.GET("/lessons/:id/items", contentSrv.listItems)
	api.POST("/items/:id/answer", contentSrv.submitAnswer)
	api.POST("/courses", contentSrv.requireAdmin(contentSrv.createCourse))
	api.PUT("/courses/:id", contentSrv.requireAdmin(contentSrv.updateCourse))
	api.POST("/lessons", contentSrv.requireAdmin(contentSrv.createLesson))
	api.POST("/items", contentSrv.requireAdmin(contentSrv.createItem))
	api.PUT("/items/:id", contentSrv.requireAdmin(contentSrv.updateItem))
	api.POST("/import", contentSrv.requireAdmin(contentSrv.importBatch))

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
	registry.DeRegister(serviceName, 8096, nacosCli)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func (s *Server) readyz(c *gin.Context) {
	if err := s.readDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(503, gin.H{"status": "DOWN", "db_read": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "UP", "db_read": true})
}

// ---- 身份/鉴权:BFF 注入可信头 ----

func userID(c *gin.Context) int64 {
	v, _ := strconv.ParseInt(c.GetHeader("X-User-Id"), 10, 64)
	return v
}

func roleOf(c *gin.Context) string {
	if r := c.GetHeader("X-Role"); r != "" {
		return r
	}
	return "learner"
}

func (s *Server) requireAdmin(fn gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if roleOf(c) != "admin" {
			c.JSON(403, gin.H{"error": "admin required"})
			c.Abort()
			return
		}
		fn(c)
	}
}

// ---- 学习端:浏览 ----

func (s *Server) listCourses(c *gin.Context) {
	examTag := c.DefaultQuery("exam_tag", "")
	status := c.DefaultQuery("status", "published")
	var rows *sql.Rows
	var err error
	q := "SELECT id, code, title, exam_tag, cefr_range, description, status FROM courses"
	args := []any{}
	if status != "all" {
		q += " WHERE status=$1"
		args = append(args, status)
	}
	if examTag != "" {
		if len(args) == 0 {
			q += " WHERE exam_tag=$1"
		} else {
			q += " AND exam_tag=$2"
		}
		args = append(args, examTag)
	}
	q += " ORDER BY id"
	rows, err = s.readDB.QueryContext(c.Request.Context(), q, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	var out []gin.H
	for rows.Next() {
		var id int64
		var code, title, tag, cefr, st string
		var desc sql.NullString
		if err := rows.Scan(&id, &code, &title, &tag, &cefr, &desc, &st); err != nil {
			continue
		}
		out = append(out, gin.H{"id": id, "code": code, "title": title, "exam_tag": tag, "cefr_range": cefr, "description": desc.String, "status": st})
	}
	c.JSON(200, out)
}

func (s *Server) getCourse(c *gin.Context) {
	id := c.Param("id")
	var code, title, tag, cefr string
	var desc sql.NullString
	err := s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT code, title, exam_tag, cefr_range, description FROM courses WHERE id=$1 AND status='published'", id).
		Scan(&code, &title, &tag, &cefr, &desc)
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	c.JSON(200, gin.H{"id": id, "code": code, "title": title, "exam_tag": tag, "cefr_range": cefr, "description": desc.String})
}

func (s *Server) listLessons(c *gin.Context) {
	courseID := c.Param("id")
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		"SELECT id, course_id, seq, title, skill, cefr FROM lessons WHERE course_id=$1 AND status='published' ORDER BY seq",
		courseID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, cid int64
		var seq int
		var title, skill, cefr string
		if err := rows.Scan(&id, &cid, &seq, &title, &skill, &cefr); err != nil {
			continue
		}
		out = append(out, gin.H{"id": id, "course_id": cid, "seq": seq, "title": title, "skill": skill, "cefr": cefr})
	}
	c.JSON(200, out)
}

func (s *Server) listItems(c *gin.Context) {
	lessonID := c.Param("id")
	rows, err := s.readDB.QueryContext(c.Request.Context(),
		"SELECT id, lesson_id, item_type, prompt, difficulty, cefr, COALESCE(array_to_string(tags, ','), '') FROM content_items WHERE lesson_id=$1 AND status='published' AND is_current ORDER BY id",
		lessonID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := make([]gin.H, 0)
	for rows.Next() {
		var id, lid int64
		var it string
		var prompt []byte
		var diff int
		var cefr, tags string
		if err := rows.Scan(&id, &lid, &it, &prompt, &diff, &cefr, &tags); err != nil {
			continue
		}
		var promptJSON any
		_ = json.Unmarshal(prompt, &promptJSON)
		var tagList []string
		if tags != "" {
			tagList = strings.Split(tags, ",")
		}
		out = append(out, gin.H{"id": id, "lesson_id": lid, "item_type": it, "prompt": promptJSON, "difficulty": diff, "cefr": cefr, "tags": tagList})
	}
	c.JSON(200, out)
}

// ---- 学习端:提交作答(判分 + attempts + 事件) ----

type answerBody struct {
	Answers    any `json:"answers"`
	DurationMs int `json:"duration_ms"`
}

func (s *Server) submitAnswer(c *gin.Context) {
	uid := userID(c)
	if uid <= 0 {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	itemID := c.Param("id")
	var body answerBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var itemType string
	var promptB, answerB []byte
	var explanation sql.NullString
	err := s.readDB.QueryRowContext(c.Request.Context(),
		"SELECT item_type, prompt, answer, explanation FROM content_items WHERE id=$1 AND status='published' AND is_current", itemID).
		Scan(&itemType, &promptB, &answerB, &explanation)
	if err != nil {
		c.JSON(404, gin.H{"error": "item not found"})
		return
	}

	correct, score := grade(itemType, body.Answers, answerB)

	answersJSON, _ := json.Marshal(body.Answers)
	_, err = s.writeDB.ExecContext(c.Request.Context(),
		"INSERT INTO content_attempts (user_id, item_id, answers_json, score, correct, duration_ms) VALUES ($1,$2,$3::jsonb,$4,$5,$6)",
		uid, itemID, string(answersJSON), score, correct, body.DurationMs)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	itemIDN, _ := strconv.ParseInt(itemID, 10, 64)
	s.emitContentEvent(uid, itemIDN, itemType, correct, score)

	c.JSON(200, gin.H{"correct": correct, "score": score, "explanation": explanation.String})
}

func grade(itemType string, submitted any, answerB []byte) (bool, float64) {
	var ans struct {
		CorrectOption int `json:"correct_option"`
	}
	_ = json.Unmarshal(answerB, &ans)
	if itemType == "vocab" {
		return true, 1
	}
	sub, ok := submitted.(map[string]any)
	if !ok {
		return false, 0
	}
	oi, ok := sub["option_index"]
	if !ok {
		return false, 0
	}
	n, _ := oi.(float64)
	if int(n) == ans.CorrectOption {
		return true, 100
	}
	return false, 0
}

func (s *Server) emitContentEvent(userID, itemID int64, itemType string, correct bool, score float64) {
	payload, _ := json.Marshal(gin.H{
		"user_id":     userID,
		"item_id":     itemID,
		"item_type":   itemType,
		"category":    itemType,
		"correct":     correct,
		"score":       score,
		"source":      "content-library",
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
	})
	_, err := s.writeDB.ExecContext(context.Background(),
		`INSERT INTO esp_learning.learning_event_outbox (message_id, user_id, event_type, payload)
		 VALUES ($1,$2,$3,$4::jsonb)`,
		randID(), userID, "content.item_answered", string(payload))
	if err != nil {
		log.Printf("[%s] outbox write warn: %v", serviceName, err)
	}
}

// ---- 管理端 CRUD ----

func (s *Server) createCourse(c *gin.Context) {
	var b struct {
		Code      string `json:"code"`
		Title     string `json:"title"`
		ExamTag   string `json:"exam_tag"`
		CefrRange string `json:"cefr_range"`
		Desc      string `json:"description"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, err := s.insertReturnID(c,
		"INSERT INTO courses (code, title, exam_tag, cefr_range, description) VALUES ($1,$2,$3,$4,$5) RETURNING id",
		b.Code, b.Title, strings.ToUpper(b.ExamTag), b.CefrRange, b.Desc)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": id})
}

func (s *Server) updateCourse(c *gin.Context) {
	id := c.Param("id")
	var b struct {
		Title     string `json:"title"`
		Status    string `json:"status"`
		CefrRange string `json:"cefr_range"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	_, err := s.writeDB.ExecContext(c.Request.Context(),
		"UPDATE courses SET title=$1, status=$2, cefr_range=$3 WHERE id=$4", b.Title, b.Status, b.CefrRange, id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": id})
}

func (s *Server) createLesson(c *gin.Context) {
	var b struct {
		CourseID int64  `json:"course_id"`
		Seq      int    `json:"seq"`
		Title    string `json:"title"`
		Skill    string `json:"skill"`
		Cefr     string `json:"cefr"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, err := s.insertReturnID(c,
		"INSERT INTO lessons (course_id, seq, title, skill, cefr) VALUES ($1,$2,$3,$4,$5) RETURNING id",
		b.CourseID, b.Seq, b.Title, b.Skill, b.Cefr)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": id})
}

type itemInput struct {
	LessonID    int64    `json:"lesson_id"`
	ItemType    string   `json:"item_type"`
	Prompt      any      `json:"prompt"`
	Answer      any      `json:"answer"`
	Explanation string   `json:"explanation"`
	Difficulty  int      `json:"difficulty"`
	Cefr        string   `json:"cefr"`
	Tags        []string `json:"tags"`
	Source      string   `json:"source"`
	License     string   `json:"license"`
}

func (s *Server) createItem(c *gin.Context) {
	var b itemInput
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, err := s.insertItem(c, b)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"id": id})
}

func (s *Server) updateItem(c *gin.Context) {
	id := c.Param("id")
	var b struct {
		Status string     `json:"status"`
		Edit   *itemInput `json:"edit,omitempty"`
	}
	if err := c.BindJSON(&b); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if b.Status != "" {
		if _, err := s.writeDB.ExecContext(c.Request.Context(),
			"UPDATE content_items SET status=$1 WHERE id=$2", b.Status, id); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	if b.Edit != nil {
		newID, err := s.insertItem(c, *b.Edit)
		if err == nil {
			if _, err := s.writeDB.ExecContext(c.Request.Context(),
				`UPDATE content_items SET is_current=false WHERE id=$1`, id); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			if _, err := s.writeDB.ExecContext(c.Request.Context(),
				`UPDATE content_items SET version = (SELECT COALESCE(MAX(version),0)+1 FROM content_items WHERE lesson_id=(SELECT lesson_id FROM content_items WHERE id=$1) AND is_current=false), is_current=true WHERE id=$2`,
				id, newID); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			c.JSON(200, gin.H{"id": newID, "superseded": id})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": id})
}

func (s *Server) insertItem(c *gin.Context, b itemInput) (int64, error) {
	promptB, _ := json.Marshal(b.Prompt)
	answerB, _ := json.Marshal(b.Answer)
	return s.insertReturnID(c,
		"INSERT INTO content_items (lesson_id, item_type, prompt, answer, explanation, difficulty, cefr, tags, source, license) VALUES ($1,$2,$3::jsonb,$4::jsonb,$5,$6,$7,$8::string[],$9,$10) RETURNING id",
		b.LessonID, b.ItemType, string(promptB), string(answerB), b.Explanation, b.Difficulty, b.Cefr, b.Tags, b.Source, b.License)
}

func (s *Server) importBatch(c *gin.Context) {
	var items []itemInput
	if err := c.BindJSON(&items); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if len(items) == 0 || len(items) > 5000 {
		c.JSON(400, gin.H{"error": "batch size must be 1..5000"})
		return
	}
	created := 0
	for i := range items {
		if _, err := s.insertItem(c, items[i]); err != nil {
			c.JSON(500, gin.H{"error": err.Error(), "index": i})
			return
		}
		created++
	}
	c.JSON(201, gin.H{"created": created})
}

// ---- helpers ----

func (s *Server) insertReturnID(c *gin.Context, q string, args ...any) (int64, error) {
	var id int64
	err := s.writeDB.QueryRowContext(c.Request.Context(), q, args...).Scan(&id)
	return id, err
}

func randID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}
