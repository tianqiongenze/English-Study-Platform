package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// createTestContext 构造一个挂载了真实 *http.Request 的 gin 测试上下文,
// 供 handler 单测直接调用 handler(c),无需经整 gin 路由树(对齐 community-go 范式)。
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

// withNilDBs 保证两包级 DB 指针为 nil 并在测试后恢复,隔离 DB 状态(CLC:nil-safe 验证)。
func withNilDBs(t *testing.T, fn func()) {
	t.Helper()
	prevRead, prevWrite := readDB, writeDB
	readDB, writeDB = nil, nil
	t.Cleanup(func() { readDB, writeDB = prevRead, prevWrite })
	fn()
}

// ---- readyz:nil-safe 降级(CLC 铁律:零值 Manager nil 解引用 panic 起因区同型问题) ----
// 修复前 readyz 在 readDB==nil 时直接 readDB.PingContext → nil 解引用 panic;
// 生产期 main 中 readDB 必初始化,但单测/未初始化路径必须 nil-safe 返回 503 而非 panic。

func TestReadyz_NilDBDoesNotPanic(t *testing.T) {
	withNilDBs(t, func() {
		ctx, w := createTestContext(t, http.MethodGet, "/readyz", "")
		// 必须 NOT panic,返回 503 DOWN(走真实执行路径,非语义假设)。
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

// ---- submitAttempt:Bind 校验失败先于 DB 写入返回(CLC:追踪真实路径) ----
// BindJSON 失败在 ExecContext 之前,故 writeDB==nil 仍安全返回 400(真实执行路径证明)。

func TestSubmitAttempt_InvalidJSON_BindFailsBeforeDB(t *testing.T) {
	withNilDBs(t, func() {
		ctx, w := createTestContext(t, http.MethodPost, "/api/listening/attempts",
			`{bad json`)
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
			t.Errorf("expected error field in 400 response, body=%s", w.Body.String())
		}
	})
}

// ---- transcribe:env-gating(MPB 铁律:无 ASR_ENDPOINT 返回 501 拒绝假数据) ----

func TestTranscribe_NoEndpoint_Returns501(t *testing.T) {
	t.Setenv("ASR_ENDPOINT", "")
	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "")
	transcribe(ctx)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("transcribe no-endpoint code: got %d, want 501", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["code"] != "ASR_NOT_IMPLEMENTED" {
		t.Errorf("code: got %v, want ASR_NOT_IMPLEMENTED", got["code"])
	}
}

func TestTranscribe_EmptyBody_Returns400(t *testing.T) {
	// 有 endpoint 但 body 空 → 400(追踪真实路径:读 body 后判空)。
	t.Setenv("ASR_ENDPOINT", "http://127.0.0.1:1/transcribe") // 不可达,但 body 空先返回不发起请求
	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "")
	transcribe(ctx)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty-body code: got %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty audio body") {
		t.Errorf("expected empty audio body error, got %s", w.Body.String())
	}
}

// TestTranscribe_RealAdapter_OK:真实 ASR adapter 桩返回 text → SSE 单帧含该文本。
// 走真实执行路径:env 注入 httptest server URL → 读 body → POST adapter → 解码 → SSE 输出。
func TestTranscribe_RealAdapter_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// adapter 契约:接收 {"chunks":[base64...]} 返回 {"text":"..."}
		var in struct {
			Chunks []string `json:"chunks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Chunks) == 0 {
			http.Error(w, "bad", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello world recognized"}`))
	}))
	defer srv.Close()
	t.Setenv("ASR_ENDPOINT", srv.URL)

	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "raw-audio-bytes")
	transcribe(ctx)
	if w.Code != http.StatusOK {
		t.Fatalf("real-adapter code: got %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type: got %q, want text/event-stream*", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"text":"hello world recognized"`) {
		t.Errorf("SSE body missing recognized text: %s", body)
	}
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("SSE body should start with 'data: ', got: %s", body[:min(len(body), 40)])
	}
}

// TestTranscribe_AdapterUnreachable_502:adapter 不可达 → 502 降级(RWT 铁律:失败降级不崩)。
func TestTranscribe_AdapterUnreachable_502(t *testing.T) {
	// 127.0.0.1:1 端口无监听 → 连接拒绝 → 502
	t.Setenv("ASR_ENDPOINT", "http://127.0.0.1:1/transcribe")
	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "audio-bytes")
	transcribe(ctx)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("unreachable-adapter code: got %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ASR adapter unreachable") {
		t.Errorf("expected unreachable error, got %s", w.Body.String())
	}
}

// TestTranscribe_AdapterEmptyText_502:adapter 返回空 text → 502(契约:空/非法即降级)。
func TestTranscribe_AdapterEmptyText_502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"text":""}`)) // 空 text
	}))
	defer srv.Close()
	t.Setenv("ASR_ENDPOINT", srv.URL)
	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "audio-bytes")
	transcribe(ctx)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("empty-text code: got %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty/invalid") {
		t.Errorf("expected empty/invalid error, got %s", w.Body.String())
	}
}

// TestTranscribe_AdapterInvalidJSON_502:adapter 返回非 JSON → 解码失败 → 502(契约:非法输出降级)。
func TestTranscribe_AdapterInvalidJSON_502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`not-json-at-all`))
	}))
	defer srv.Close()
	t.Setenv("ASR_ENDPOINT", srv.URL)
	ctx, w := createTestContext(t, http.MethodPost, "/api/listening/transcribe", "audio-bytes")
	transcribe(ctx)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("invalid-json code: got %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "empty/invalid") {
		t.Errorf("expected empty/invalid error, got %s", w.Body.String())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
