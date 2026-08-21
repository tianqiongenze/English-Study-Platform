package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// createTestContext 构造挂载真实 *http.Request 的 gin 测试上下文,直接调用 handler(对齐 community-go 范式)。
func createTestContext(t *testing.T, method, path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	ctx.Request = req
	return ctx, w
}

// withNilDBs 保证两包级 DB 指针为 nil 并恢复,隔离 DB 状态(CLC:nil-safe 验证)。
func withNilDBs(t *testing.T, fn func()) {
	t.Helper()
	prevRead, prevWrite := readDB, writeDB
	readDB, writeDB = nil, nil
	t.Cleanup(func() { readDB, writeDB = prevRead, prevWrite })
	fn()
}

// withCleanParseCache 清空 L1 解析缓存并恢复,隔离缓存状态。
func withCleanParseCache(t *testing.T, fn func()) {
	t.Helper()
	parseCacheMu.Lock()
	prev := parseCache
	parseCache = map[string]parseResult{}
	parseCacheMu.Unlock()
	t.Cleanup(func() {
		parseCacheMu.Lock()
		parseCache = prev
		parseCacheMu.Unlock()
	})
	fn()
}

// ---- sval:纯函数,表驱动(CLC:经典惯用法表驱动测试真实返回路径) ----

func TestSval_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", "main", "main"},
		{"empty-string", "", ""},
		{"nil", nil, ""},
		{"int", 42, ""},
		{"float", 3.14, ""},
		{"bool", true, ""},
		{"slice", []any{"a"}, ""},
		{"map", map[string]any{"k": "v"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sval(c.in)
			if got != c.want {
				t.Errorf("sval(%v): got %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ---- readyz:nil-safe 降级(CLC 铁律:零值/未初始化指针不解引用 nil) ----

func TestReadyz_NilDBDoesNotPanic(t *testing.T) {
	withNilDBs(t, func() {
		ctx, w := createTestContext(t, http.MethodGet, "/readyz", "")
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("readyz panicked on nil readDB: %v", r)
			}
		}()
		readyz(ctx)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("readyz nil-db code: got %d, want 503", w.Code)
		}
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
		}
		if got["status"] != "DOWN" {
			t.Errorf("status: got %v, want DOWN", got["status"])
		}
	})
}

// ---- submitAttempt:Bind 校验失败先于 DB 写入返回 ----

func TestSubmitAttempt_InvalidJSON_BindFailsBeforeDB(t *testing.T) {
	withNilDBs(t, func() {
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/attempts", `{bad json`)
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("submitAttempt panicked on invalid json: %v", r)
			}
		}()
		submitAttempt(ctx)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid-json code: got %d, want 400", w.Code)
		}
		var got map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if _, ok := got["error"]; !ok {
			t.Errorf("expected error field, body=%s", w.Body.String())
		}
	})
}

// ---- parseSentence:空 sentence → 400(追踪真实路径:BindJSON 成功但 TrimSpace 空) ----

func TestParseSentence_EmptySentence_400(t *testing.T) {
	withCleanParseCache(t, func() {
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse", `{"sentence":""}`)
		parseSentence(ctx)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("empty-sentence code: got %d, want 400", w.Code)
		}
		if !strings.Contains(w.Body.String(), "sentence required") {
			t.Errorf("expected 'sentence required', got %s", w.Body.String())
		}
	})
}

func TestParseSentence_InvalidJSON_400(t *testing.T) {
	withCleanParseCache(t, func() {
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse", `{bad`)
		parseSentence(ctx)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid-json code: got %d, want 400", w.Code)
		}
	})
}

// TestParseSentence_RealLLM_OK:env 注入 httptest LLM 桩 → 走真实 llm.New()+Chat+ExtractJSON 路径,
// 解析后写入 L1 缓存,响应含 main_clause/translation。
func TestParseSentence_RealLLM_OK(t *testing.T) {
	withCleanParseCache(t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Anthropic Messages API 契约:返回 {"content":[{"type":"text","text":"..."}]}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"main_clause\":\"He runs\",\"subordinate\":[\"because he is late\"],\"translation\":\"他跑\"}"}]}`))
		}))
		defer srv.Close()
		t.Setenv("ANTHROPIC_API_KEY", "test-key")
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		t.Setenv("ANTHROPIC_MODEL", "test-model")

		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse",
			`{"sentence":"He runs because he is late"}`)
		parseSentence(ctx)
		if w.Code != http.StatusOK {
			t.Fatalf("real-llm code: got %d, want 200 body=%s", w.Code, w.Body.String())
		}
		var got parseResult
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
		}
		if got.MainClause != "He runs" {
			t.Errorf("main_clause: got %q, want 'He runs'", got.MainClause)
		}
		if got.Translation != "他跑" {
			t.Errorf("translation: got %q, want '他跑'", got.Translation)
		}
		if len(got.Subordinate) != 1 || got.Subordinate[0] != "because he is late" {
			t.Errorf("subordinate: got %v, want [because he is late]", got.Subordinate)
		}
	})
}

// TestParseSentence_CacheHit:首次解析写缓存,第二次相同 sentence 命中缓存不调 LLM。
func TestParseSentence_CacheHit(t *testing.T) {
	withCleanParseCache(t, func() {
		callCount := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"{\"main_clause\":\"MC\",\"translation\":\"T\"}"}]}`))
		}))
		defer srv.Close()
		t.Setenv("ANTHROPIC_API_KEY", "test-key")
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		t.Setenv("ANTHROPIC_MODEL", "test-model")

		body := `{"sentence":"cached sentence xyz"}`
		// 首次:miss → 调 LLM → 写缓存
		ctx1, w1 := createTestContext(t, http.MethodPost, "/api/reading/parse", body)
		parseSentence(ctx1)
		if w1.Code != http.StatusOK {
			t.Fatalf("first call code: got %d, want 200", w1.Code)
		}
		if callCount != 1 {
			t.Fatalf("first call should hit LLM once, got %d", callCount)
		}
		// 第二次:相同 sentence → 命中缓存,不调 LLM
		ctx2, w2 := createTestContext(t, http.MethodPost, "/api/reading/parse", body)
		parseSentence(ctx2)
		if w2.Code != http.StatusOK {
			t.Fatalf("second call code: got %d, want 200", w2.Code)
		}
		if callCount != 1 {
			t.Fatalf("cache hit should NOT call LLM, callCount=%d", callCount)
		}
		var got parseResult
		_ = json.Unmarshal(w2.Body.Bytes(), &got)
		if got.MainClause != "MC" {
			t.Errorf("cached main_clause: got %q, want MC", got.MainClause)
		}
	})
}

// TestParseSentence_LLMNoKey_502:无 API key → llm.Chat 返回 error → 502(RWT:降级不崩)。
func TestParseSentence_LLMNoKey_502(t *testing.T) {
	withCleanParseCache(t, func() {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse", `{"sentence":"some sentence"}`)
		parseSentence(ctx)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("no-key code: got %d, want 502", w.Code)
		}
		if !strings.Contains(w.Body.String(), "LLM unavailable") {
			t.Errorf("expected 'LLM unavailable', got %s", w.Body.String())
		}
	})
}

// TestParseSentence_LLMNonJSONOutput_502:LLM 返回非 JSON → ExtractJSON 失败 → 502。
func TestParseSentence_LLMNonJSONOutput_502(t *testing.T) {
	withCleanParseCache(t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"this is not json"}]}`))
		}))
		defer srv.Close()
		t.Setenv("ANTHROPIC_API_KEY", "test-key")
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		t.Setenv("ANTHROPIC_MODEL", "test-model")
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse", `{"sentence":"x"}`)
		parseSentence(ctx)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("non-json code: got %d, want 502", w.Code)
		}
		if !strings.Contains(w.Body.String(), "LLM output parse failed") {
			t.Errorf("expected parse failed, got %s", w.Body.String())
		}
	})
}

// TestParseSentence_LLMNon200_502:LLM 返回非 200 → Chat 返回 error → 502。
func TestParseSentence_LLMNon200_502(t *testing.T) {
	withCleanParseCache(t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		t.Setenv("ANTHROPIC_API_KEY", "test-key")
		t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
		t.Setenv("ANTHROPIC_MODEL", "test-model")
		ctx, w := createTestContext(t, http.MethodPost, "/api/reading/parse", `{"sentence":"x"}`)
		parseSentence(ctx)
		if w.Code != http.StatusBadGateway {
			t.Fatalf("non-200 code: got %d, want 502", w.Code)
		}
		if !strings.Contains(w.Body.String(), "LLM unavailable") {
			t.Errorf("expected LLM unavailable, got %s", w.Body.String())
		}
	})
}
