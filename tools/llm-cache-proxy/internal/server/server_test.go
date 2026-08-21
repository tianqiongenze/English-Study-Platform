// Package server — LCO 代理 HTTP 服务器测试(CLC 铁律:验证 forwardUpstream 流式透传 + 缓存拦截 + 错误路径)
//
// 测试策略:
// - 用 httptest.NewServer 起 mock upstream(模拟 SSE 流式 / 非流式 JSON / 5xx)
// - 缓存层用 &Manager{cfg: CacheEnabled:false} 禁用模式(不依赖真实 Redis/Milvus/Neo4j),
//   聚焦验证转发 + 流式 flush 逻辑;真实缓存命中需三件套起不来,见集成测试。
// - flushRecorder:嵌入 httptest.ResponseRecorder + 覆写 Flush() 记录调用次数
//   (ResponseRecorder 默认不实现 http.Flusher,无法测 flush 时机)
package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"llm-cache-proxy/internal/cache"
	"llm-cache-proxy/internal/config"
)

// flushRecorder 记录 Flush 调用次数,用于断言流式透传边读边 flush。
// httptest.ResponseRecorder 不实现 http.Flusher,必须自行覆写。
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushCalls int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() {
	f.flushCalls++
	// ResponseRecorder 本身无 Flush,不调底层(仅计数)
}

// disabledManager 构造禁用缓存的 Manager(CacheEnabled=false,Lookup 第一行返回空不命中,
// Store 第一行也判 CacheEnabled 不写)。零值安全(CLC:不连真实 Redis/Milvus/Neo4j)。
func disabledManager() *cache.Manager {
	return cache.NewDisabledForTest()
}

// newTestServer 构造测试用 LCO Server,upstream 指向 mock。
func newTestServer(upstreamURL string) *Server {
	return &Server{
		cfg:    &config.Config{UpstreamURL: upstreamURL, CacheEnabled: false},
		cache:  disabledManager(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		hc: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 5 * time.Second, // 测试缩短
				IdleConnTimeout:       10 * time.Second,
			},
		},
	}
}

// --- mock upstream handlers ---

// sseUpstream 模拟 SSE 流式响应:分块写 + Flush + sleep(模拟逐 token 产出)。
func sseUpstream(w http.ResponseWriter, _ *http.Request) {
	flusher, ok := w.(http.Flusher)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	chunks := []string{
		"event: message_start\ndata: {\"type\":\"message_start\"}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"hello\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\" world\"}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for _, c := range chunks {
		fmt.Fprint(w, c)
		if ok {
			flusher.Flush()
		}
		time.Sleep(10 * time.Millisecond) // 模拟 token 产出延迟
	}
}

// jsonUpstream 模拟非流式 JSON 响应。
func jsonUpstream(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"id":"msg-1","type":"message","role":"assistant","content":[{"type":"text","text":"Pong!"}]}`)
}

// errorUpstream 模拟 5xx 错误。
func errorUpstream(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprint(w, `{"error":"upstream error"}`)
}

// makeBody 构造 /v1/messages 请求体。
func makeBody(userContent string) []byte {
	return []byte(fmt.Sprintf(`{"model":"glm_for_coding","max_tokens":20,"messages":[{"role":"user","content":%q}]}`, userContent))
}

// --- 测试用例 ---

// TestForwardUpstream_Stream_ChunkedFlush:流式转发,断言 flush 次数 >= 3(逐 chunk flush,非全量缓冲)。
func TestForwardUpstream_Stream_ChunkedFlush(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(sseUpstream))
	defer upstream.Close()

	srv := newTestServer(upstream.URL)
	rec := newFlushRecorder()
	body := makeBody("count to 5")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	srv.forwardUpstream(rec, req, body, "anthropic|test|key")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if rec.flushCalls < 3 {
		t.Fatalf("flushCalls=%d, want >=3(流式应逐 chunk flush)", rec.flushCalls)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("Content-Type=%q, want text/event-stream", rec.Header().Get("Content-Type"))
	}
	// 响应体应含全部 SSE 帧
	if !strings.Contains(rec.Body.String(), "message_stop") {
		t.Errorf("响应体缺 message_stop, body=%q", rec.Body.String())
	}
}

// TestForwardUpstream_NonStream_NoFlush:非流式 JSON 响应不应触发 flush(全量缓冲后一次性写)。
func TestForwardUpstream_NonStream_NoFlush(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(jsonUpstream))
	defer upstream.Close()

	srv := newTestServer(upstream.URL)
	rec := newFlushRecorder()
	body := makeBody("ping")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	srv.forwardUpstream(rec, req, body, "anthropic|test|key")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if rec.flushCalls != 0 {
		t.Errorf("非流式不应 flush, flushCalls=%d", rec.flushCalls)
	}
	if !strings.Contains(rec.Body.String(), "Pong!") {
		t.Errorf("响应体缺 Pong!, body=%q", rec.Body.String())
	}
}

// TestForwardUpstream_5xxPassthrough:upstream 5xx 透传给客户端。
func TestForwardUpstream_5xxPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(errorUpstream))
	defer upstream.Close()

	srv := newTestServer(upstream.URL)
	rec := newFlushRecorder()
	body := makeBody("ping")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	srv.forwardUpstream(rec, req, body, "")

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502 透传", rec.Code)
	}
}

// TestForwardUpstream_UpstreamUnreachable:upstream 不可达返回 502。
func TestForwardUpstream_UpstreamUnreachable(t *testing.T) {
	// 用一个未监听的端口模拟不可达
	srv := newTestServer("http://127.0.0.1:1") // port 1 不可达
	rec := newFlushRecorder()
	body := makeBody("ping")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))

	srv.forwardUpstream(rec, req, body, "")

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502(upstream unreachable)", rec.Code)
	}
}

// TestIsStreamByHeader:响应头 content-type 含 text/event-stream 判流式。
func TestIsStreamByHeader(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/event-stream", true},
		{"text/event-stream; charset=utf-8", true},
		{"application/json", false},
		{"", false},
	}
	for _, tt := range tests {
		resp := &http.Response{Header: http.Header{}}
		if tt.ct != "" {
			resp.Header.Set("Content-Type", tt.ct)
		}
		if got := isStreamByHeader(resp); got != tt.want {
			t.Errorf("isStreamByHeader(ct=%q)=%v, want %v", tt.ct, got, tt.want)
		}
	}
}

// TestIsStreamBody:body 前缀 data: 判流式(兜底)。
func TestIsStreamBody(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"data: {\"type\":\"message_start\"}", true},
		{"  data: hello", true},
		{"{\"id\":\"msg-1\"}", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isStreamBody([]byte(tt.body)); got != tt.want {
			t.Errorf("isStreamBody(%q)=%v, want %v", tt.body, got, tt.want)
		}
	}
}

// TestStreamProxy_FlusherDegrade:ResponseWriter 不支持 Flush 时降级为纯 Write 不 panic。
// 用 httptest.ResponseRecorder(不实现 http.Flusher)直接调 streamProxy。
func TestStreamProxy_FlusherDegrade(t *testing.T) {
	// 构造一个 SSE upstream
	upstream := httptest.NewServer(http.HandlerFunc(sseUpstream))
	defer upstream.Close()

	srv := newTestServer(upstream.URL)
	// 普通 ResponseRecorder(不实现 Flusher)
	rec := httptest.NewRecorder()
	body := makeBody("hi")
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// 不应 panic,应正常写完全部 body
	srv.forwardUpstream(rec, req, body, "anthropic|test|key")

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "message_stop") {
		t.Errorf("降级路径应仍写完响应体, body=%q", rec.Body.String())
	}
}

// TestForwardUpstream_IndependentContext:验证 upstream context 独立于 r.Context()。
// 客户端 context cancel 后,upstream 请求不应被立即取消(独立 600s context)。
func TestForwardUpstream_IndependentContext(t *testing.T) {
	// upstream 响应慢一点(100ms),客户端 context 立即 cancel
	slowUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		jsonUpstream(w, r)
	}))
	defer slowUpstream.Close()

	srv := newTestServer(slowUpstream.URL)
	rec := newFlushRecorder()
	body := makeBody("ping")

	// 客户端 context 在请求前就 cancel(模拟客户端断开)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req = req.WithContext(ctx)

	srv.forwardUpstream(rec, req, body, "anthropic|test|key")

	// 独立 context:upstream 仍应完成(200),不被客户端 cancel 误杀
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200(独立 context 不受客户端 cancel 影响)", rec.Code)
	}
}
