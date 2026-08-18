package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	payload := map[string]any{"status": "UP", "count": 3}
	writeJSON(w, payload)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["status"] != "UP" {
		t.Errorf("status: got %v, want UP", got["status"])
	}
	if got["count"] != float64(3) {
		t.Errorf("count: got %v, want 3", got["count"])
	}
}

func TestGeneratePath_MethodNotAllowed(t *testing.T) {
	// GET 应返回 405
	req := httptest.NewRequest(http.MethodGet, "/api/path/generate", nil)
	w := httptest.NewRecorder()
	generatePath(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET generatePath: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestGeneratePath_Post_Scaffold(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/path/generate", nil)
	w := httptest.NewRecorder()
	generatePath(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST generatePath: got %d, want %d", w.Code, http.StatusOK)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["status"] != "scaffold" {
		t.Errorf("status: got %v, want scaffold", got["status"])
	}
	if got["generated"] != false {
		t.Errorf("generated: got %v, want false", got["generated"])
	}
	if pid, ok := got["path_id"].(string); !ok || pid == "" {
		t.Errorf("path_id missing or empty: %v", got["path_id"])
	}
}

func TestUnlockNode_Scaffold(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/path/unlock", nil)
	w := httptest.NewRecorder()
	unlockNode(w, req)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["unlocked"] != false {
		t.Errorf("unlocked: got %v, want false", got["unlocked"])
	}
	if got["status"] != "scaffold" {
		t.Errorf("status: got %v, want scaffold", got["status"])
	}
}

func TestGetPath_Scaffold(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/path/abc", nil)
	w := httptest.NewRecorder()
	getPath(w, req)
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["path"] != nil {
		t.Errorf("path: got %v, want nil", got["path"])
	}
	if got["status"] != "scaffold" {
		t.Errorf("status: got %v, want scaffold", got["status"])
	}
}

func TestReadiness_Liveness(t *testing.T) {
	for _, h := range []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"readiness", readiness},
		{"liveness", liveness},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		h.fn(w, req)
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s unmarshal: %v", h.name, err)
		}
		if got["status"] != "UP" {
			t.Errorf("%s status: got %v, want UP", h.name, got["status"])
		}
	}
}

// getenv:委托到 config.Getenv,验证不 panic。
func TestGetenv(t *testing.T) {
	if v := getenv("NONEXISTENT_KEY_XYZ", "fallback"); v != "fallback" {
		t.Errorf("getenv default: got %q", v)
	}
}

// writeJSON:额外边界情况。
func TestWriteJSON_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, nil)
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
	// nil JSON encode = "null"
	if w.Body.String() != "null\n" {
		t.Errorf("nil JSON body: got %q", w.Body.String())
	}
}

func TestWriteJSON_Slice(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, []string{"a", "b"})
	var got []string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("slice: got %v", got)
	}
}
