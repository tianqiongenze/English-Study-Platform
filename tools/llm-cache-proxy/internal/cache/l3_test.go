package cache

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"log/slog"

	"llm-cache-proxy/internal/config"
	"llm-cache-proxy/internal/protocol"
)

// setupL3 启动 mock Neo4j REST server 并设置 LCO_NEO4J_REST_URL env,返回 cleanup。
func setupL3(t *testing.T, handler http.HandlerFunc) (*L3Knowledge, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	// l3.neo4jTx 优先读 LCO_NEO4J_REST_URL
	prev := os.Getenv("LCO_NEO4J_REST_URL")
	os.Setenv("LCO_NEO4J_REST_URL", srv.URL)
	cfg := &config.Config{
		Neo4jURL:      "bolt://127.0.0.1:7688",
		Neo4jUser:     "neo4j",
		Neo4jPassword: "test",
	}
	l := NewL3Knowledge(cfg, slog.Default())
	return l, func() {
		os.Setenv("LCO_NEO4J_REST_URL", prev)
		srv.Close()
	}
}

// TestL3Record_Payload Record 应向 Neo4j /db/neo4j/tx/commit 发 MERGE 语句,
// 断言:语句含 MERGE (p:Prompt)+ANSWERED_BY,参数含 hash/text/proto/model/rid。
func TestL3Record_Payload(t *testing.T) {
	var gotStmt string
	var gotParams map[string]any
	l, cleanup := setupL3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/db/neo4j/tx/commit" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var body struct {
			Statements []struct {
				Statement  string         `json:"statement"`
				Parameters map[string]any `json:"parameters"`
			} `json:"statements"`
		}
		_ = json.Unmarshal(b, &body)
		if len(body.Statements) > 0 {
			gotStmt = body.Statements[0].Statement
			gotParams = body.Statements[0].Parameters
		}
		// 正常返回空结果
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "errors": []any{}})
	})
	defer cleanup()

	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Model: "claude-sonnet-4-6", Raw: "hello graph"}
	l.Record(context.Background(), key, []byte(`{"r":"body"}`))

	if gotStmt == "" {
		t.Fatal("Record 未发出 Cypher 语句")
	}
	if want := "MERGE (p:Prompt"; !containsStmt(gotStmt, want) {
		t.Errorf("stmt 缺 %q: %s", want, gotStmt)
	}
	if want := "ANSWERED_BY"; !containsStmt(gotStmt, want) {
		t.Errorf("stmt 缺 %q", want)
	}
	if gotParams["proto"] != string(protocol.Anthropic) {
		t.Errorf("proto=%v want anthropic", gotParams["proto"])
	}
	if gotParams["model"] != "claude-sonnet-4-6" {
		t.Errorf("model=%v", gotParams["model"])
	}
	if _, ok := gotParams["hash"].(string); !ok || gotParams["hash"] == "" {
		t.Errorf("hash 缺失/空: %v", gotParams["hash"])
	}
	if _, ok := gotParams["rid"].(string); !ok || gotParams["rid"] == "" {
		t.Errorf("rid 缺失/空: %v", gotParams["rid"])
	}
}

// TestL3Enhance_Hit Enhance 应解析 Neo4j 返回的 row[0] 作为 body。
func TestL3Enhance_Hit(t *testing.T) {
	l, cleanup := setupL3(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []any{
				map[string]any{
					"data": []any{
						map[string]any{"row": []any{"enhanced-body"}},
					},
				},
			},
			"errors": []any{},
		})
	})
	defer cleanup()

	got := l.Enhance(context.Background(), protocol.NormalizeKey{Protocol: protocol.OpenAI, Raw: "some prompt text"})
	if got != "enhanced-body" {
		t.Errorf("Enhance=%q want enhanced-body", got)
	}
}

// TestL3Enhance_NoResult 无结果返回空串,不 panic。
func TestL3Enhance_NoResult(t *testing.T) {
	l, cleanup := setupL3(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "errors": []any{}})
	})
	defer cleanup()
	if got := l.Enhance(context.Background(), protocol.NormalizeKey{Protocol: protocol.OpenAI, Raw: "x"}); got != "" {
		t.Errorf("Enhance=%q want empty", got)
	}
}

// TestL3_Neo4jError_Degrade Neo4j 返回 cypher error 应降级不 panic。
func TestL3_Neo4jError_Degrade(t *testing.T) {
	l, cleanup := setupL3(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []any{map[string]any{"code": "Neo.ClientError", "message": "boom"}},
		})
	})
	defer cleanup()
	// Enhance 失败应返回空不 panic
	if got := l.Enhance(context.Background(), protocol.NormalizeKey{Protocol: protocol.OpenAI, Raw: "x"}); got != "" {
		t.Errorf("error 应降级返回空, got %q", got)
	}
}

// TestL3_NilSafe 零值/nil L3Knowledge 调用 Record/Enhance/Health 不 panic(CLC 铁律)。
func TestL3_NilSafe(t *testing.T) {
	var l *L3Knowledge
	// nil receiver
	l.Record(context.Background(), protocol.NormalizeKey{Raw: "x"}, []byte("b"))
	if got := l.Enhance(context.Background(), protocol.NormalizeKey{Raw: "x"}); got != "" {
		t.Errorf("nil Enhance 应返回空")
	}
	// 零值 cfg=nil(非 nil receiver)
	l2 := &L3Knowledge{cfg: nil, logger: slog.Default()}
	l2.Record(context.Background(), protocol.NormalizeKey{Raw: "x"}, []byte("b"))
	if got := l2.Enhance(context.Background(), protocol.NormalizeKey{Raw: "x"}); got != "" {
		t.Errorf("nil cfg Enhance 应返回空")
	}
}

// TestL3Health Ok Down。
func TestL3Health(t *testing.T) {
	l, cleanup := setupL3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/db/neo4j/tx/commit" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "errors": []any{}})
			return
		}
		http.NotFound(w, r)
	})
	defer cleanup()
	if got := l.Health(context.Background()); got != "ok" {
		t.Errorf("Health=%q want ok", got)
	}
}

// TestTruncate_UTF8Safe truncate 按 rune 截不破坏 UTF-8。
func TestTruncate_UTF8Safe(t *testing.T) {
	s := "你好world"
	if got := truncate(s, 5); len([]rune(got)) != 5 {
		t.Errorf("truncate rune 数=%d want 5", len([]rune(got)))
	}
	// 短串原样返回
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short=%q want abc", got)
	}
}

func containsStmt(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
