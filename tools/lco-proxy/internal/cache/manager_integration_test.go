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
// miniredis 是单节点,L1Pool 只含 1 个 client,无 MOVED 重路由。
func newTestManager(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis 启动失败: %v", err)
	}
	t.Cleanup(mr.Close)
	pool := NewL1Pool(mr.Addr(), "")
	t.Cleanup(pool.Close)
	return &Manager{
		cfg: &config.Config{
			CacheEnabled: true,
			TTLChat:      3600,
		},
		pool: pool,
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
	pool := NewL1Pool(mr.Addr(), "")
	defer pool.Close()
	m := &Manager{
		cfg:  &config.Config{CacheEnabled: false},
		pool: pool,
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
	// Redis 不可达:pool 指向已关闭的 miniredis,Lookup 应降级返回 miss 不 panic
	mr, _ := miniredis.Run()
	pool := NewL1Pool(mr.Addr(), "")
	defer pool.Close()
	m := &Manager{
		cfg:  &config.Config{CacheEnabled: true},
		pool: pool,
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

// TestL1PoolMovedReroute 验证 MOVED 解析逻辑(parseMoved + extractPodIndex)。
// 用真实 miniredis 无法产生 MOVED,这里只单测解析函数。
func TestL1PoolMovedParse(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		wantOk  bool
		wantPod int
	}{
		{"moved-pod3", "MOVED 12000 redis-cluster-new-3.redis-cluster-new-xx.svc:6379", true, 3},
		{"moved-pod0", "MOVED 100 redis-cluster-new-0.redis-cluster-new-xx.svc:6379", true, 0},
		{"moved-pod5", "MOVED 16000 redis-cluster-new-5.redis-cluster-new-xx.svc:6379", true, 5},
		{"non-moved", "GET nil", false, 0},
		{"empty", "", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moved, ok := parseMoved(toErr(tt.errMsg))
			if ok != tt.wantOk {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOk)
			}
			if ok {
				pod := extractPodIndex(moved.host)
				if pod != tt.wantPod {
					t.Errorf("pod=%d, want %d (host=%s)", pod, tt.wantPod, moved.host)
				}
			}
		})
	}
}

// toErr 把字符串包成 error。
type strErr string

func (e strErr) Error() string { return string(e) }
func toErr(s string) error     { return strErr(s) }

// TestL1PoolMovedRerouteSelfHost 验证 CLC 修正:MOVED 指向 startIdx 自身时
// 不应直接返回错误,应继续遍历下一个 pod 直到命中或全部试完。
// 这是 2026-08-19 实测暴露的 bug:原 `podIdx != startIdx` 条件在 MOVED 指向
// startIdx 自身时跳过重试直接 return err,导致 ~50% 命中率(非 100%)。
func TestL1PoolMovedRerouteSelfHost(t *testing.T) {
	// 构造 3 个 client 的池,用闭包模拟 fn 行为:
	// - startIdx=0 (pod-0): 返回 MOVED 指向 pod-0 自身(slot 属于 pod-0 但单节点 Client 不认)
	// - startIdx=1 (pod-1): 返回成功值 "hit-value"
	// 期望:withMovedRetry 遍历过 pod-0 的 MOVED-self 后,继续到 pod-1 拿到 "hit-value"
	pool := &L1Pool{clients: []*redis.Client{nil, nil, nil}} // 真实 client 不用,fn 闭包控制
	callIdx := 0
	fn := func(c *redis.Client) (string, error) {
		callIdx++
		if callIdx == 1 {
			// pod-0 返回 MOVED 指向自身
			return "", toErr("MOVED 4415 redis-cluster-new-0.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379")
		}
		// pod-1 命中
		return "hit-value", nil
	}
	val, err := pool.withMovedRetry(context.Background(), "any-key", fn)
	if err != nil {
		t.Fatalf("MOVED 指向自身时应继续遍历而非返回错误, got err=%v", err)
	}
	if val != "hit-value" {
		t.Errorf("val=%q, want hit-value", val)
	}
}

// TestL1PoolMovedRerouteAllMoved 验证所有 pod 都返回 MOVED 时最终返回 lastErr(不 panic)。
func TestL1PoolMovedRerouteAllMoved(t *testing.T) {
	pool := &L1Pool{clients: []*redis.Client{nil, nil}}
	fn := func(c *redis.Client) (string, error) {
		return "", toErr("MOVED 5000 redis-cluster-new-2.redis-cluster-new.ekb-m2-aws-us-east-1.svc.cluster.local:6379")
	}
	_, err := pool.withMovedRetry(context.Background(), "any-key", fn)
	if err == nil {
		t.Fatal("所有 pod 都 MOVED 应返回错误")
	}
	// 不应是 "no client available"(那是 nil client 情况),应是 MOVED 错误
	if err.Error() == "no client available" {
		t.Error("应返回 MOVED 错误而非 no client available")
	}
}

// TestL1PoolNonMovedErrorContinues 验证非 MOVED 错误(如 i/o timeout)也继续遍历。
func TestL1PoolNonMovedErrorContinues(t *testing.T) {
	pool := &L1Pool{clients: []*redis.Client{nil, nil}}
	callIdx := 0
	fn := func(c *redis.Client) (string, error) {
		callIdx++
		if callIdx == 1 {
			return "", toErr("i/o timeout") // pod-0 超时
		}
		return "recovered", nil // pod-1 恢复
	}
	val, err := pool.withMovedRetry(context.Background(), "any-key", fn)
	if err != nil {
		t.Fatalf("非 MOVED 错误应继续遍历, got err=%v", err)
	}
	if val != "recovered" {
		t.Errorf("val=%q, want recovered", val)
	}
}
