package cache

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"

	"llm-cache-proxy/internal/config"
	"llm-cache-proxy/internal/protocol"
)

// newCfg 构造指向 httptest server 的 config(EmbeddingURL/MilvusAddr 由测试注入)。
func newL2Cfg(embeddingURL, milvusHost string) *config.Config {
	return &config.Config{
		EmbeddingURL: embeddingURL,
		MilvusAddr:   milvusHost,
		L2Collection: "llm_cache_prompts",
		L2Threshold:  0.92,
	}
}

// dim2560 构造 2560 维向量(与 Qwen collection 对齐)。
func dim2560() []float32 {
	v := make([]float32, 2560)
	for i := range v {
		v[i] = 0.01
	}
	return v
}

// TestL2StoreSearch_HappyPath mock embedding + Milvus,验证 Store 后 Search 命中,
// 并断言 embedding 请求体含 model=qwen(维度对齐 ADR-23 §2.2)。
func TestL2StoreSearch_HappyPath(t *testing.T) {
	var gotEmbedModel string
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req map[string]string
		_ = json.Unmarshal(b, &req)
		gotEmbedModel = req["model"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"vector": dim2560(),
			"model":  "qwen",
			"dim":    2560,
		})
	}))
	defer emb.Close()

	insertCalled := false
	mil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/vectordb/entities/insert":
			insertCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"insertCount": 1})
		case "/v2/vectordb/entities/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"body":     `{"cached":"yes"}`,
					"protocol": string(protocol.Anthropic),
					"model":    "claude-sonnet-4-6",
					"distance": 0.95, // > threshold 0.92
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mil.Close()

	l := NewL2Semantic(newL2Cfg(emb.URL, mil.Listener.Addr().String()), slog.Default())
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Model: "claude-sonnet-4-6", Raw: "hello world"}

	l.Store(context.Background(), key, []byte(`{"cached":"yes"}`))
	if !insertCalled {
		t.Fatal("Store 未调用 Milvus insert")
	}
	if gotEmbedModel != "qwen" {
		t.Fatalf("embedding 请求 model=%q, want qwen(ADR-23 §2.2 维度对齐)", gotEmbedModel)
	}

	body, proto, hit := l.Search(context.Background(), key)
	if !hit {
		t.Fatal("Search 未命中(distance=0.95 > 0.92 应命中)")
	}
	if proto != protocol.Anthropic {
		t.Errorf("protocol=%s, want anthropic", proto)
	}
	if string(body) != `{"cached":"yes"}` {
		t.Errorf("body=%q", string(body))
	}
}

// TestL2Search_BelowThreshold 降级:distance < threshold 不命中。
func TestL2Search_BelowThreshold(t *testing.T) {
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"vector": dim2560(), "model": "qwen", "dim": 2560})
	}))
	defer emb.Close()
	mil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"body": "x", "protocol": "anthropic", "distance": 0.50}},
		})
	}))
	defer mil.Close()

	l := NewL2Semantic(newL2Cfg(emb.URL, mil.Listener.Addr().String()), slog.Default())
	_, _, hit := l.Search(context.Background(), protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "x"})
	if hit {
		t.Fatal("distance=0.50 < 0.92 不应命中")
	}
}

// TestL2Search_EmbeddingFail_Degrade embedding 服务 500 应降级返回 miss 不 panic。
func TestL2Search_EmbeddingFail_Degrade(t *testing.T) {
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer emb.Close()
	mil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer mil.Close()

	l := NewL2Semantic(newL2Cfg(emb.URL, mil.Listener.Addr().String()), slog.Default())
	_, _, hit := l.Search(context.Background(), protocol.NormalizeKey{Protocol: protocol.OpenAI, Raw: "x"})
	if hit {
		t.Fatal("embedding 失败应降级 miss,不该命中")
	}
}

// TestL2Search_EmptyVector_Degrade embedding 返回空向量应降级。
func TestL2Search_EmptyVector_Degrade(t *testing.T) {
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"vector": []float32{}, "model": "qwen", "dim": 0})
	}))
	defer emb.Close()
	mil := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("embedding 空向量不应走到 Milvus search")
	}))
	defer mil.Close()

	l := NewL2Semantic(newL2Cfg(emb.URL, mil.Listener.Addr().String()), slog.Default())
	_, _, hit := l.Search(context.Background(), protocol.NormalizeKey{Protocol: protocol.Ollama, Raw: "x"})
	if hit {
		t.Fatal("空向量应 miss")
	}
}

// TestL2Health Milvus /v2/vectordb/collections/list 200 → ok。
func TestL2Health(t *testing.T) {
	mil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/vectordb/collections/list" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer mil.Close()
	l := NewL2Semantic(newL2Cfg("http://127.0.0.1:9", mil.Listener.Addr().String()), slog.Default())
	if got := l.Health(context.Background()); got != "ok" {
		t.Errorf("Health=%q want ok", got)
	}
}
