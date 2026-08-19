package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// callCountingProvider 计数 inner.Chat 调用次数,验证 HIT 时 inner 未被调用。
type callCountingProvider struct {
	calls int
	reply string
	err   error
}

func (p *callCountingProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	p.calls++
	if p.err != nil {
		return "", p.err
	}
	return p.reply, nil
}

// newTestRedis 启动 miniredis 并返回可注入 CachedProvider.store 的 *redis.Client。
// miniredis 是单机 Redis,但 *redis.Client 实现了 redis.Cmdable,满足 CachedProvider 依赖。
func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return client, mr
}

// disableL2L3 在 L1 专属测试中关闭 L2(Milvus)/L3(Neo4j)子构造,避免测试环境 DNS 解析 milvus/neo4j 慢,
// 聚焦验证 L1 精确缓存语义(L2/L3 各有独立测试文件 semantic_test.go/knowledge_test.go)。
func disableL2L3(t *testing.T) {
	t.Helper()
	t.Setenv("ESP_LLM_L2_ENABLED", "false")
	t.Setenv("ESP_LLM_L3_ENABLED", "false")
}

func TestCachedProvider_HitAfterMissAndWriteback(t *testing.T) {
	disableL2L3(t)
	// arrange:inner 固定回复,miniredis 作 store
	inner := &callCountingProvider{reply: "cached-reply"}
	rdb, _ := newTestRedis(t)
	t.Setenv("ESP_LLM_CACHE_ENABLED", "true")
	p := NewCached(inner, rdb, "test-model")

	msgs := []Message{
		{Role: "system", Content: "tutor"},
		{Role: "user", Content: "I'd like to check in for my flight"},
	}

	// act 1:第一次 MISS → inner 调用 + 回写
	got1, err := p.Chat(context.Background(), msgs)
	if err != nil {
		t.Fatalf("first Chat err: %v", err)
	}
	if got1 != "cached-reply" {
		t.Fatalf("first Chat got %q, want cached-reply", got1)
	}
	if inner.calls != 1 {
		t.Fatalf("after first Chat, inner.calls=%d, want 1 (miss should delegate)", inner.calls)
	}

	// act 2:第二次相同输入 → HIT,inner 不应再被调用
	got2, err := p.Chat(context.Background(), msgs)
	if err != nil {
		t.Fatalf("second Chat err: %v", err)
	}
	if got2 != "cached-reply" {
		t.Fatalf("second Chat got %q, want cached-reply", got2)
	}
	if inner.calls != 1 {
		t.Fatalf("after second Chat, inner.calls=%d, want 1 (hit should not delegate)", inner.calls)
	}
}

func TestCachedProvider_DifferentInputIsSeparateCacheEntry(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "reply"}
	rdb, _ := newTestRedis(t)
	p := NewCached(inner, rdb, "m")

	// 两个不同 user 输入应各自 MISS 一次
	_, _ = p.Chat(context.Background(), []Message{{Role: "user", Content: "hello A"}})
	_, _ = p.Chat(context.Background(), []Message{{Role: "user", Content: "hello B"}})
	if inner.calls != 2 {
		t.Fatalf("inner.calls=%d, want 2 (two distinct misses)", inner.calls)
	}

	// 重复第一个 → 应 HIT
	_, _ = p.Chat(context.Background(), []Message{{Role: "user", Content: "hello A"}})
	if inner.calls != 2 {
		t.Fatalf("after repeat, inner.calls=%d, want 2 (hit)", inner.calls)
	}
}

func TestCachedProvider_TTLExpiryCausesReMiss(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "r"}
	rdb, mr := newTestRedis(t)
	p := NewCached(inner, rdb, "m")

	msgs := []Message{{Role: "user", Content: "ttl-check"}}
	_, _ = p.Chat(context.Background(), msgs) // MISS + 回写,inner.calls=1

	// 快进 TTL(miniredis FastForward)
	mr.FastForward(cacheTTL + time.Second)

	_, _ = p.Chat(context.Background(), msgs) // TTL 过期 → 再次 MISS
	if inner.calls != 2 {
		t.Fatalf("after TTL expiry, inner.calls=%d, want 2 (re-miss)", inner.calls)
	}
}

func TestCachedProvider_CacheKeyCanonicalizationTrimsWhitespace(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "r"}
	rdb, _ := newTestRedis(t)
	p := NewCached(inner, rdb, "m")

	// 输入首尾空白不同,规范化后应命中同一缓存项
	msgA := []Message{{Role: "user", Content: "  hello  "}}
	msgB := []Message{{Role: "user", Content: "hello"}}
	_, _ = p.Chat(context.Background(), msgA) // MISS, calls=1
	_, _ = p.Chat(context.Background(), msgB) // 规范化后 HIT, calls 仍=1
	if inner.calls != 1 {
		t.Fatalf("after trim-normalized repeat, inner.calls=%d, want 1 (canonical hit)", inner.calls)
	}
}

func TestCachedProvider_ModelIsolationSeparatesCache(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "r"}
	rdb, _ := newTestRedis(t)
	pModelA := NewCached(inner, rdb, "model-a")
	pModelB := NewCached(inner, rdb, "model-b")

	msgs := []Message{{Role: "user", Content: "same input"}}
	_, _ = pModelA.Chat(context.Background(), msgs) // model-a MISS, calls=1
	_, _ = pModelB.Chat(context.Background(), msgs) // model-b 不同 key → MISS, calls=2
	if inner.calls != 2 {
		t.Fatalf("model isolation: inner.calls=%d, want 2 (different model = different cache)", inner.calls)
	}
}

func TestCachedProvider_DisabledByEnvPassThroughAndStillCallsInner(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "r"}
	rdb, _ := newTestRedis(t)
	t.Setenv("ESP_LLM_CACHE_ENABLED", "false")
	p := NewCached(inner, rdb, "m")

	// 缓存禁用 → 每次都直穿 inner,不应写 Redis
	msgs := []Message{{Role: "user", Content: "x"}}
	_, _ = p.Chat(context.Background(), msgs)
	_, _ = p.Chat(context.Background(), msgs)
	if inner.calls != 2 {
		t.Fatalf("disabled: inner.calls=%d, want 2 (no cache, each call hits inner)", inner.calls)
	}
	// 验证确实没写 Redis(key 不存在)
	k := p.cacheKey(msgs)
	exists, _ := rdb.Exists(context.Background(), k).Result()
	if exists != 0 {
		t.Fatalf("disabled: cache key should not exist, got exists=%d", exists)
	}
}

func TestCachedProvider_NilStorePassesThrough(t *testing.T) {
	disableL2L3(t)
	inner := &callCountingProvider{reply: "r"}
	t.Setenv("ESP_LLM_CACHE_ENABLED", "true")
	p := NewCached(inner, nil, "m") // store=nil

	msgs := []Message{{Role: "user", Content: "x"}}
	_, err := p.Chat(context.Background(), msgs)
	if err != nil {
		t.Fatalf("nil-store Chat err: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("nil-store: inner.calls=%d, want 1 (pass-through)", inner.calls)
	}
}

func TestCachedProvider_InnerErrorPropagatesAndNoWriteback(t *testing.T) {
	disableL2L3(t)
	// inner error 时不应回写(避免缓存错误),inner.calls 计数仍增加
	wantErr := errors.New("upstream-down")
	inner := &callCountingProvider{err: wantErr}
	rdb, _ := newTestRedis(t)
	p := NewCached(inner, rdb, "m")

	msgs := []Message{{Role: "user", Content: "fail"}}
	_, err := p.Chat(context.Background(), msgs)
	if !errors.Is(err, wantErr) {
		t.Fatalf("inner error not propagated: got %v, want %v", err, wantErr)
	}
	// 验证未回写空/错误值
	k := p.cacheKey(msgs)
	exists, _ := rdb.Exists(context.Background(), k).Result()
	if exists != 0 {
		t.Fatalf("inner error: should not writeback, key exists=%d", exists)
	}
}

func TestCachedProvider_CacheKeyFormat(t *testing.T) {
	disableL2L3(t)
	p := NewCached(&callCountingProvider{}, nil, "claude-sonnet-4")
	k := p.cacheKey([]Message{{Role: "user", Content: "hi"}})
	if !strings.HasPrefix(k, cacheKeyPrefix) {
		t.Fatalf("key %q missing prefix %q", k, cacheKeyPrefix)
	}
	if !strings.Contains(k, "claude-sonnet-4") {
		t.Fatalf("key %q missing model segment", k)
	}
	// sha 段应为 16 hex 字符
	parts := strings.Split(k, ":")
	// esp:cache:llm:exact:{model}:{sha} → 分割后最后一段
	sha := parts[len(parts)-1]
	if len(sha) != cacheShaLen {
		t.Fatalf("sha segment len=%d, want %d", len(sha), cacheShaLen)
	}
}
