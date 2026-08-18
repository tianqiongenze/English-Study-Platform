package handler

import (
	"context"
	"testing"
	"time"
)

func TestWithTrace_WithHeader(t *testing.T) {
	ctx := withTrace(context.Background(), "trace-123")
	v := ctx.Value(traceKey{})
	if s, ok := v.(string); !ok || s != "trace-123" {
		t.Errorf("traceId = %v, want trace-123", v)
	}
}

func TestWithTrace_GeneratesWhenEmpty(t *testing.T) {
	ctx := withTrace(context.Background(), "")
	v := ctx.Value(traceKey{})
	s, ok := v.(string)
	if !ok || s == "" {
		t.Errorf("traceId should be generated, got %v", v)
	}
}

func TestSqlNullTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	nt := sqlNullTime(now)
	if !nt.Valid {
		t.Error("NullTime should be valid")
	}
	// The stored time should be UTC-normalized
	if nt.Time.Year() != 2026 || nt.Time.Month() != 1 || nt.Time.Day() != 1 {
		t.Errorf("NullTime = %v, want 2026-01-01", nt.Time)
	}
}

// TestReviewReq_Fields 验证 ReviewReq 结构体字段(编译时类型检查)。
func TestReviewReq_Fields(t *testing.T) {
	r := ReviewReq{UserID: 1, WordID: 2, Quality: 3}
	if r.UserID != 1 || r.WordID != 2 || r.Quality != 3 {
		t.Errorf("ReviewReq fields: %+v", r)
	}
}

// TestReviewResp_Fields 验证 ReviewResp 结构体字段。
func TestReviewResp_Fields(t *testing.T) {
	r := ReviewResp{
		WordID: 1, IntervalDays: 6, EaseFactor: 2.5,
		DueAt: "2026-01-07T00:00:00Z", ReviewCount: 2,
		LapseCount: 0, Status: "review",
	}
	if r.Status != "review" {
		t.Errorf("ReviewResp.Status = %q", r.Status)
	}
}

// TestCreateWordReq_Fields 验证 CreateWordReq 结构体字段。
func TestCreateWordReq_Fields(t *testing.T) {
	r := CreateWordReq{Word: "hello", Definition: "greeting"}
	if r.Word != "hello" || r.Definition != "greeting" {
		t.Errorf("CreateWordReq: %+v", r)
	}
}
