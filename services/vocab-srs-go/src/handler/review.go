// Package handler 实现 vocab-srs-go 的 HTTP handler。
//
// 路由:
//   POST /api/vocab/review          提交一次复习(SM-2+ 调度)
//   GET  /api/vocab/due?uid=&limit= 查询到期词列表
//   POST /api/vocab/word            新建词条
//   GET  /api/vocab/word/:id        查词条
//   DELETE /api/vocab/word/:id      删词条
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"esp/vocab-srs-go/src/dao"
	srs "esp/vocab-srs-go/src/srs"

	"github.com/gin-gonic/gin"
)

// Handler 持有 DAO 与 logger 抽象。
type Handler struct {
	DAO *dao.VocabDAO
	Log Logger
}

// Logger 最小日志接口(由 main 注入 *zap.SugaredLogger 或 slog 适配)。
type Logger interface {
	Infof(template string, args ...any)
	Errorf(template string, args ...any)
}

// traceKey 存放 X-Trace-Id 透传。
type traceKey struct{}

// ReviewReq 提交复习请求体。
type ReviewReq struct {
	UserID   int64 `json:"user_id" binding:"required"`
	WordID   int64 `json:"word_id" binding:"required"`
	Quality  int   `json:"quality" binding:"required"` // 0-5
}

// ReviewResp 提交复习响应。
type ReviewResp struct {
	WordID       int64  `json:"word_id"`
	IntervalDays int    `json:"interval_days"`
	EaseFactor   float64 `json:"ease_factor"`
	DueAt        string `json:"due_at"`
	ReviewCount  int    `json:"review_count"`
	LapseCount   int    `json:"lapse_count"`
	Status       string `json:"status"`
}

// Review 处理一次 SM-2+ 复习调度。
// 流程:取 user_vocabulary 当前状态 → sm2.Compute → upsert → 返回新调度。
// 若用户尚未学过该词,初始化为 learning(interval=1, ease=2.5)。
func (h *Handler) Review(c *gin.Context) {
	ctx := withTrace(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	var req ReviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Quality < srs.MinQuality || req.Quality > srs.MaxQuality {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quality must be in [0,5]"})
		return
	}

	now := time.Now()
	cur, err := h.DAO.GetUserVocab(ctx, req.UserID, req.WordID)
	if err != nil {
		h.Log.Errorf("GetUserVocab uid=%d wid=%d err=%v", req.UserID, req.WordID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}

	var in srs.Input
	if cur != nil {
		in = srs.Input{
			Quality:      req.Quality,
			IntervalDays: cur.IntervalDays,
			EaseFactor:   cur.EaseFactor,
			ReviewCount:  cur.ReviewCount,
			LapseCount:   cur.LapseCount,
			Status:       cur.Status,
		}
	} else {
		in = srs.Input{
			Quality:      req.Quality,
			IntervalDays: 1,
			EaseFactor:   srs.DefaultEase,
			ReviewCount:  0,
			LapseCount:   0,
			Status:       srs.StatusLearning,
		}
	}

	res := srs.Compute(in, now)

	upsert := &dao.UserVocabRow{
		UserID:        req.UserID,
		WordID:        req.WordID,
		SrsStage:      0,
		IntervalDays:  res.IntervalDays,
		EaseFactor:    res.EaseFactor,
		DueAt:         res.DueAt,
		ReviewCount:   res.ReviewCount,
		LapseCount:    res.LapseCount,
		Status:        res.Status,
		LastReviewed:  sqlNullTime(now),
	}
	if err := h.DAO.UpsertUserVocab(ctx, upsert); err != nil {
		h.Log.Errorf("UpsertUserVocab uid=%d wid=%d err=%v", req.UserID, req.WordID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, ReviewResp{
		WordID:       req.WordID,
		IntervalDays: res.IntervalDays,
		EaseFactor:   res.EaseFactor,
		DueAt:        res.DueAt.Format(time.RFC3339),
		ReviewCount:  res.ReviewCount,
		LapseCount:   res.LapseCount,
		Status:       res.Status,
	})
}

// Due 查询某用户到期词列表。
func (h *Handler) Due(c *gin.Context) {
	ctx := withTrace(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	uid, err := strconv.ParseInt(c.Query("uid"), 10, 64)
	if err != nil || uid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid uid"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	words, err := h.DAO.ListDueWords(ctx, uid, time.Now(), limit)
	if err != nil {
		h.Log.Errorf("ListDueWords uid=%d err=%v", uid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": uid, "due": words, "count": len(words)})
}

// CreateWordReq 新建词条请求。
type CreateWordReq struct {
	Word         string          `json:"word" binding:"required"`
	Phonetic     string          `json:"phonetic"`
	Pos          string          `json:"pos"`
	Definition   string          `json:"definition" binding:"required"`
	DefinitionZh string          `json:"definition_zh"`
	CefrLevel    string          `json:"cefr_level"`
	Examples     json.RawMessage `json:"examples"`
}

// CreateWord 新建词条。
func (h *Handler) CreateWord(c *gin.Context) {
	ctx := withTrace(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	var req CreateWordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.DAO.CreateVocab(ctx, req.Word, req.Phonetic, req.Pos,
		req.Definition, req.DefinitionZh, req.CefrLevel, req.Examples)
	if err != nil {
		h.Log.Errorf("CreateVocab word=%s err=%v", req.Word, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "word": req.Word})
}

// GetWord 查询单个词条。
func (h *Handler) GetWord(c *gin.Context) {
	ctx := withTrace(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	v, err := h.DAO.GetVocabByID(ctx, id)
	if err != nil {
		h.Log.Errorf("GetVocabByID id=%d err=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, v)
}

// DeleteWord 删除词条。
func (h *Handler) DeleteWord(c *gin.Context) {
	ctx := withTrace(c.Request.Context(), c.GetHeader("X-Trace-Id"))
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DAO.DeleteVocab(ctx, id); err != nil {
		h.Log.Errorf("DeleteVocab id=%d err=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// withTrace 注入 traceId 到 context(若 header 无则生成)。
func withTrace(parent context.Context, traceID string) context.Context {
	if traceID == "" {
		traceID = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return context.WithValue(parent, traceKey{}, traceID)
}

// sqlNullTime 返回 sql.NullTime 包装(已 reviewed 时用)。
func sqlNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
