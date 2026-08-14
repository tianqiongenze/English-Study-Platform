package session

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewManager(rdb, time.Hour), mr
}

func TestManager_CreateAndGet(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()
	s := &Session{ID: "s1", UserID: 1, Scenario: "airport", System: "you are staff", Difficulty: 3}
	if err := m.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := m.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Scenario != "airport" || got.Difficulty != 3 {
		t.Fatalf("unexpected session: %+v", got)
	}
	if got.StartedAt.IsZero() {
		t.Fatal("StartedAt should be set")
	}
}

func TestManager_GetNotFound(t *testing.T) {
	m, _ := newTestManager(t)
	_, err := m.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestManager_AppendTurn_PersistsAndTrimsHistory(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()
	s := &Session{ID: "s2", UserID: 1, Scenario: "cafe", System: "barista", Difficulty: 3}
	_ = m.Create(ctx, s)
	for i := 0; i < 60; i++ {
		_, err := m.AppendTurn(ctx, "s2", Turn{Role: "user", Content: "hi"})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	got, err := m.Get(ctx, "s2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.History) != 50 {
		t.Fatalf("expected history trimmed to 50, got %d", len(got.History))
	}
}

func TestManager_End_RemovesKey(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()
	_ = m.Create(ctx, &Session{ID: "s3", UserID: 1, Scenario: "x"})
	if err := m.End(ctx, "s3"); err != nil {
		t.Fatalf("end: %v", err)
	}
	if _, err := m.Get(ctx, "s3"); err == nil {
		t.Fatal("expected session removed after End")
	}
}
