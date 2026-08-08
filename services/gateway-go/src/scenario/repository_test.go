package scenario

import "testing"

func TestRepository_ListHasDefaults(t *testing.T) {
	r := NewRepository()
	got := r.List()
	if len(got) < 3 {
		t.Fatalf("expected >=3 scenarios, got %d", len(got))
	}
}

func TestRepository_GetKnown(t *testing.T) {
	r := NewRepository()
	s, ok := r.Get("airport")
	if !ok {
		t.Fatal("expected airport scenario")
	}
	if s.System == "" {
		t.Fatal("system prompt should not be empty")
	}
}

func TestRepository_GetUnknown(t *testing.T) {
	r := NewRepository()
	if _, ok := r.Get("nope"); ok {
		t.Fatal("expected not found")
	}
}

func TestRepository_AddCustom(t *testing.T) {
	r := NewRepository()
	r.Add(Scenario{Code: "custom", Title: "Custom", System: "x", Difficulty: 3})
	s, ok := r.Get("custom")
	if !ok || s.Title != "Custom" {
		t.Fatalf("custom scenario not stored: %+v", s)
	}
}
