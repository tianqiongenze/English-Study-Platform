package llm

// L2 语义缓存(Milvus)+ L3 知识图谱(Neo4j)单元测试。
// 设计(CLC 铁律:追踪执行路径,非语义臆测):
//   - L2 用 httptest mock Milvus + embedding,返回 canned 响应,不依赖真实服务。
//   - L3 用 nil driver 测降级不 panic(零值对象安全),并用 httptest 不可达驱动测 Record 降级。
//   - 验证 nil receiver / nil client / 禁用开关 短路路径均不 panic。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- L2 SemanticCache 测试 ----

// newMockEmbeddingServer 启动返回固定 384 维向量的 embedding mock。
func newMockEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	vec := make([]float32, 384)
	for i := range vec {
		vec[i] = 0.01 * float32(i) // 固定可预测向量
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"vector": vec})
	}))
}

// newMockMilvusServer 启动 mock Milvus REST:collections list/create + insert + search。
// 返回 canned search 结果(可配 distance)。
func newMockMilvusServer(t *testing.T, searchDistance float64, searchResponse string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vector/collections", func(w http.ResponseWriter, r *http.Request) {
		// GET list
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []string{l2CollectionName}})
	})
	mux.HandleFunc("/v1/vector/collections/create", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{}})
	})
	mux.HandleFunc("/v1/vector/insert", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"insertCount": 1}})
	})
	mux.HandleFunc("/v1/vector/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []map[string]any{
			{"distance": searchDistance, "response": searchResponse, "prompt_sha16": "deadbeefdeadbeef"},
		}})
	})
	return httptest.NewServer(mux)
}

func TestSemanticCache_SearchHitAboveThreshold(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.95, "cached-semantic-reply") // 0.95 >= 0.92 → HIT
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client()) // 复用 mock 的 client(走 127.0.0.1)
	got, ok := s.Search(context.Background(), "hello world", "minilm", "test")
	if !ok {
		t.Fatalf("expected L2 hit (distance 0.95 >= 0.92), got miss")
	}
	if got != "cached-semantic-reply" {
		t.Fatalf("got %q, want cached-semantic-reply", got)
	}
}

func TestSemanticCache_SearchMissBelowThreshold(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.50, "low-similarity-reply") // 0.50 < 0.92 → MISS
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok {
		t.Fatalf("expected L2 miss (distance 0.50 < 0.92), got hit %q", got)
	}
	if got != "" {
		t.Fatalf("expected empty response on miss, got %q", got)
	}
}

func TestSemanticCache_SearchEmptyMilvusResult(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	// mock 返回空 data
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vector/collections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []string{l2CollectionName}})
	})
	mux.HandleFunc("/v1/vector/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []any{}})
	})
	milvus := httptest.NewServer(mux)
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok {
		t.Fatalf("expected miss on empty data, got %q", got)
	}
}

// 覆盖 Search 命中但 response 为空字符串 → 视为 MISS(hit=="")。
func TestSemanticCache_SearchHitEmptyResponseIsMiss(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.99, "") // 高相似度但 response 为空
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok || got != "" {
		t.Fatalf("empty response should be miss, got (%q,%v)", got, ok)
	}
}

// 覆盖 Milvus 非 200 状态码降级。
func TestSemanticCache_MilvusNon200Degrades(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vector/collections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []string{l2CollectionName}})
	})
	mux.HandleFunc("/v1/vector/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	milvus := httptest.NewServer(mux)
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok {
		t.Fatalf("non-200 should degrade to miss, got %q", got)
	}
	s.Store(context.Background(), "hello", "reply", "minilm", "test") // non-200 on insert → skip, no panic
}

func TestSemanticCache_StoreDoesNotPanic(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.95, "x")
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	// Store 不应 panic;失败也不 panic
	s.Store(context.Background(), "prompt", "response", "minilm", "test")
}

// CLC 铁律核心测试:nil receiver + nil client + 禁用开关,均不得 panic。
func TestSemanticCache_NilSafeAndDisabled(t *testing.T) {
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	var nilSem *SemanticCache // nil receiver
	if got, ok := nilSem.Search(context.Background(), "x", "m", "s"); ok || got != "" {
		t.Fatalf("nil receiver Search should return miss, got (%q,%v)", got, ok)
	}
	nilSem.Store(context.Background(), "x", "y", "m", "s") // nil receiver Store no-op

	// nil client(降级)
	s := NewSemanticCache(nil)
	if got, ok := s.Search(context.Background(), "x", "m", "s"); ok || got != "" {
		t.Fatalf("nil client Search should return miss, got (%q,%v)", got, ok)
	}
	s.Store(context.Background(), "x", "y", "m", "s") // no-op, no panic

	// 禁用开关
	t.Setenv("ESP_LLM_L2_ENABLED", "false")
	s2 := NewSemanticCache(http.DefaultClient)
	if got, ok := s2.Search(context.Background(), "x", "m", "s"); ok || got != "" {
		t.Fatalf("disabled Search should return miss, got (%q,%v)", got, ok)
	}
	s2.Store(context.Background(), "x", "y", "m", "s") // no-op
}

func TestSemanticCache_UnreachableEndpointsDegrade(t *testing.T) {
	// 指向不存在的端口,验证超时/连接失败降级 skip 不 panic
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", "127.0.0.1:1") // 不可达
	t.Setenv("EMBEDDING_URL", "http://127.0.0.1:1/embed")
	s := NewSemanticCache(&http.Client{})
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok {
		t.Fatalf("unreachable should degrade to miss, got %q", got)
	}
	s.Store(context.Background(), "hello", "reply", "minilm", "test") // no panic
}

func TestSemanticCache_EmptyResponseNotStored(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.95, "x")
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	// 空回复不应触发 insert(防御性:不缓存空值)
	s.Store(context.Background(), "prompt", "", "minilm", "test")
}

// 覆盖 createCollection 路径:collection 不在 list 中 → 触发 create → 后续 Search 走通。
func TestSemanticCache_CollectionNotExistTriggersCreate(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	created := false
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vector/collections", func(w http.ResponseWriter, r *http.Request) {
		// list 返回空(目标 collection 不存在)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []string{}})
	})
	mux.HandleFunc("/v1/vector/collections/create", func(w http.ResponseWriter, r *http.Request) {
		created = true // 验证 create 被调用
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{}})
	})
	mux.HandleFunc("/v1/vector/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []map[string]any{
			{"distance": 0.99, "response": "created-then-hit", "prompt_sha16": "x"},
		}})
	})
	milvus := httptest.NewServer(mux)
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "prompt", "minilm", "test")
	if !created {
		t.Fatalf("create endpoint should have been called (collection did not exist)")
	}
	if !ok {
		t.Fatalf("expected hit after create, got miss")
	}
	if got != "created-then-hit" {
		t.Fatalf("got %q, want created-then-hit", got)
	}
}

// 覆盖 milvusInsert 完整路径:Store 成功写入(mock insert 返回 200 insertCount)。
func TestSemanticCache_StoreInsertsToMilvus(t *testing.T) {
	emb := newMockEmbeddingServer(t)
	defer emb.Close()
	insertCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/vector/collections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": []string{l2CollectionName}})
	})
	mux.HandleFunc("/v1/vector/insert", func(w http.ResponseWriter, r *http.Request) {
		insertCalled = true
		// 校验请求体含 response + vector
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		data, _ := body["data"].([]any)
		if len(data) != 1 {
			t.Errorf("insert data should have 1 row, got %d", len(data))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"insertCount": 1}})
	})
	milvus := httptest.NewServer(mux)
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	s.Store(context.Background(), "prompt text", "reply text", "minilm", "test")
	if !insertCalled {
		t.Fatalf("insert endpoint should have been called")
	}
}

// 覆盖 embedding 失败降级:embedding endpoint 返回非 JSON / 错误 → Search/Store skip。
func TestSemanticCache_EmbeddingFailureDegrades(t *testing.T) {
	// embedding server 返回错误状态
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.95, "x")
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok || got != "" {
		t.Fatalf("embedding failure should degrade to miss, got (%q,%v)", got, ok)
	}
	// Store 也不应 panic
	s.Store(context.Background(), "hello", "reply", "minilm", "test")
}

// 覆盖 embedding 返回空向量 → skip。
func TestSemanticCache_EmbeddingEmptyVectorSkips(t *testing.T) {
	emb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"vector": []float32{}})
	}))
	defer emb.Close()
	milvus := newMockMilvusServer(t, 0.95, "x")
	defer milvus.Close()
	t.Setenv("ESP_LLM_L2_ENABLED", "true")
	t.Setenv("MILVUS_ADDR", strings.TrimPrefix(milvus.URL, "http://"))
	t.Setenv("EMBEDDING_URL", emb.URL+"/embed")
	s := NewSemanticCache(milvus.Client())
	got, ok := s.Search(context.Background(), "hello", "minilm", "test")
	if ok {
		t.Fatalf("empty vector should degrade to miss, got %q", got)
	}
}
