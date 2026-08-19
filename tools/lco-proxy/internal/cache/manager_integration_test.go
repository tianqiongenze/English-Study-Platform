package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"lco-proxy/internal/config"
	"lco-proxy/internal/protocol"
)

// newTestManager 用 miniredis 构造真实 Redis 后端的 Manager(L1 可测,L2/L3 禁用)。
func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis 启动失败: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &Manager{
		cfg: &config.Config{
			CacheEnabled: true,
			TTLChat:      3600,
		},
		rdb: rdb,
	}, mr
}

func TestL1StoreAndLookupHit(t *testing.T) {
	m, mr := newTestManager(t)
	ctx := context.Background()
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Model: "m", Raw: "anthropic|m|hello"}
	resp := []byte(`{"content":[{"text":"cached response"}]}`)

	// 初始 Lookup:miss
	r := m.Lookup(ctx, key)
	if r.Hit {
		t.Fatal("首次查询不应命中")
	}

	// Store 后再查:应命中 L1
	m.Store(ctx, key, resp, "chat")
	r = m.Lookup(ctx, key)
	if !r.Hit {
		t.Fatal("Store 后应命中")
	}
	if r.Layer != "L1" {
		t.Errorf("Layer=%s, want L1", r.Layer)
	}
	if string(r.Body) != string(resp) {
		t.Errorf("Body 不匹配")
	}

	// 验证 Redis 确实写入
	keys := mr.Keys()
	if len(keys) == 0 {
		t.Error("miniredis 无 key 写入")
	}
}

func TestL1TTLExpiry(t *testing.T) {
	m, mr := newTestManager(t)
	ctx := context.Background()
	key := protocol.NormalizeKey{Protocol: protocol.OpenAI, Model: "gpt", Raw: "openai|gpt|q"}
	m.Store(ctx, key, []byte(`resp`), "chat")

	// 快进 TTL 过期(TTLChat=3600s,快进 3700s)
	mr.FastForward(3700 * time.Second)

	r := m.Lookup(ctx, key)
	if r.Hit {
		t.Error("TTL 过期后不应命中")
	}
}

func TestL1ProtocolIsolationRealRedis(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()
	// 同内容不同协议
	keyA := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "isolation-test"}
	keyB := protocol.NormalizeKey{Protocol: protocol.OpenAI, Raw: "openai-isolation-test"}

	m.Store(ctx, keyA, []byte("anthropic-resp"), "chat")
	m.Store(ctx, keyB, []byte("openai-resp"), "chat")

	ra := m.Lookup(ctx, keyA)
	rb := m.Lookup(ctx, keyB)
	if !ra.Hit || !rb.Hit {
		t.Fatal("两协议都应命中")
	}
	if string(ra.Body) != "anthropic-resp" {
		t.Errorf("anthropic 响应串台: %s", ra.Body)
	}
	if string(rb.Body) != "openai-resp" {
		t.Errorf("openai 响应串台: %s", rb.Body)
	}
}

func TestL1Disabled(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	m := &Manager{
		cfg: &config.Config{CacheEnabled: false},
		rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}
	ctx := context.Background()
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "disabled-test"}

	m.Store(ctx, key, []byte("resp"), "chat")
	r := m.Lookup(ctx, key)
	if r.Hit {
		t.Error("缓存禁用时不应命中也不应写入")
	}
}

func TestL1RedisDownDegradesGracefully(t *testing.T) {
	// Redis 不可达:rdb 指向已关闭的 miniredis,Lookup 应降级返回 miss 不 panic
	mr, _ := miniredis.Run()
	m := &Manager{
		cfg: &config.Config{CacheEnabled: true},
		rdb: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}
	mr.Close() // 关闭后端
	ctx := context.Background()
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "down-test"}

	// Lookup 不应 panic,返回 miss
	r := m.Lookup(ctx, key)
	if r.Hit {
		t.Error("Redis 挂了不应命中")
	}
	// Store 也不应 panic
	m.Store(ctx, key, []byte("resp"), "chat")
}

func TestL1KeyDifferentContent(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()
	k1 := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "content-1"}
	k2 := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "content-2"}

	m.Store(ctx, k1, []byte("resp1"), "chat")
	// k2 未 store,应 miss
	r := m.Lookup(ctx, k2)
	if r.Hit {
		t.Error("不同内容不应命中")
	}
	// k1 应命中
	r = m.Lookup(ctx, k1)
	if !r.Hit {
		t.Error("k1 应命中")
	}
}
