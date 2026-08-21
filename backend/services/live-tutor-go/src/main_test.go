package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteJSON —— 纯函数 writeJSON 真实执行路径:写 Content-Type 头 + JSON 编码 body。
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

// TestWriteJSON_Nil —— 边界:nil 值 JSON 编码为 "null",不 panic(CLC nil-safe)。
func TestWriteJSON_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, nil)
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	// nil JSON encode = "null"
	if w.Body.String() != "null\n" {
		t.Errorf("nil JSON body: got %q, want \"null\\n\"", w.Body.String())
	}
}

// TestWriteJSON_Slice —— 边界:切片类型编码(非 map 路径)。
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

// TestReadiness_Liveness —— 表驱动:两个健康端点均返 {"status":"UP"}。
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
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s Content-Type: got %q", h.name, ct)
		}
	}
}

// TestBook_MethodNotAllowed —— 错误路径:非 POST 返 405(CLC 追踪 r.Method 分支)。
func TestBook_MethodNotAllowed(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/tutor/book", nil)
		w := httptest.NewRecorder()
		book(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("book %s: got %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// TestBook_Post_Scaffold —— happy 路径:POST 返 scaffold 契约字段(booking_id/status/message)。
func TestBook_Post_Scaffold(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/tutor/book", nil)
	w := httptest.NewRecorder()
	book(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("book POST: got %d, want %d", w.Code, http.StatusOK)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["status"] != "scaffold" {
		t.Errorf("status: got %v, want scaffold", got["status"])
	}
	if bid, ok := got["booking_id"].(string); !ok || bid == "" {
		t.Errorf("booking_id missing or empty: %v", got["booking_id"])
	}
	if msg, ok := got["message"].(string); !ok || msg == "" {
		t.Errorf("message missing or empty: %v", got["message"])
	}
}

// TestAvailability_Scaffold —— happy 路径:返可用时段桩契约(slots 空 + status scaffold)。
func TestAvailability_Scaffold(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/tutor/availability", nil)
	w := httptest.NewRecorder()
	availability(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("availability: got %d, want %d", w.Code, http.StatusOK)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["status"] != "scaffold" {
		t.Errorf("status: got %v, want scaffold", got["status"])
	}
	slots, ok := got["slots"].([]any)
	if !ok {
		t.Fatalf("slots not a slice: %T", got["slots"])
	}
	if len(slots) != 0 {
		t.Errorf("slots: got %d items, want 0 (scaffold empty)", len(slots))
	}
}

// TestSignaling_NonWebSocketRequest —— 真实执行路径:非 WS 握手请求 gorilla upgrader.Upgrade 返 error,
// handler 走 if err != nil { log; return } 不 panic + 不建立连接(CLC nil-safe,禁 panic 控制流)。
func TestSignaling_NonWebSocketRequest(t *testing.T) {
	// 普通 GET 无 Upgrade header -> Upgrade 返 BadRequest error,handler 应直接 return 不 panic。
	req := httptest.NewRequest(http.MethodGet, "/ws/signaling/abc", nil)
	w := httptest.NewRecorder()
	// 不应 panic
	signaling(w, req)
	// 非 WS 握手时 gorilla 写 400,handler return;无连接建立。仅断言未 panic(到此处即通过)。
}

// TestGetenv —— 真实执行路径:委托 config.Getenv,缺省回退(CLC 不 panic)。
func TestGetenv(t *testing.T) {
	if v := getenv("NONEXISTENT_KEY_XYZ", "fallback"); v != "fallback" {
		t.Errorf("getenv default: got %q, want fallback", v)
	}
}
