package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestL1Pool_NewL1Pool_MultiAddr 验证 NewL1Pool 按逗号分隔构造多 client,且 password 透传。
func TestL1Pool_NewL1Pool_MultiAddr(t *testing.T) {
	p := NewL1Pool("127.0.0.1:6380, 127.0.0.1:6381 ,127.0.0.1:6382", "secret")
	defer p.Close()
	if got := len(p.clients); got != 3 {
		t.Fatalf("clients 数=%d, want 3", got)
	}
	// 验证 TrimSpace 生效(含空白的地址已清理)
	wantAddrs := []string{"127.0.0.1:6380", "127.0.0.1:6381", "127.0.0.1:6382"}
	for i, c := range p.clients {
		if c == nil {
			t.Fatalf("client[%d] 为 nil", i)
		}
		if c.Options().Addr != wantAddrs[i] {
			t.Errorf("client[%d].Addr=%q, want %q", i, c.Options().Addr, wantAddrs[i])
		}
		if c.Options().Password != "secret" {
			t.Errorf("client[%d].Password 未透传", i)
		}
	}
}

// TestL1Pool_NewL1Pool_EmptyAddr 空地址字符串构造出空池(0 client),不 panic。
// 这是 CLC 边界:NewL1Pool("") 后续 Get/Ping 应安全降级。
func TestL1Pool_NewL1Pool_EmptyAddr(t *testing.T) {
	p := NewL1Pool("", "")
	defer p.Close()
	if got := len(p.clients); got != 1 {
		t.Fatalf("空地址 clients 数=%d, want 1(单空元素)", got)
	}
}

// TestL1Pool_Get_Hit 正常路径:miniredis 写入后 Get 命中。
func TestL1Pool_Get_Hit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	mr.Set("foo", "bar")

	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()

	val, err := p.Get(context.Background(), "foo")
	if err != nil {
		t.Fatalf("Get 命中失败: %v", err)
	}
	if val != "bar" {
		t.Errorf("val=%q, want bar", val)
	}
}

// TestL1Pool_Get_Miss 边界:key 不存在返回 redis.Nil(正常 cache miss,非错误噪音)。
func TestL1Pool_Get_Miss(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()

	_, err = p.Get(context.Background(), "absent")
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("miss 应返回 redis.Nil, got %v", err)
	}
}

// TestL1Pool_Get_EmptyValue 边界:空字符串值不算 miss(Get 命中空串)。
func TestL1Pool_Get_EmptyValue(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	mr.Set("empty", "")

	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()

	val, err := p.Get(context.Background(), "empty")
	if err != nil {
		t.Fatalf("空值 key 应命中, got err=%v", err)
	}
	if val != "" {
		t.Errorf("val=%q, want 空", val)
	}
}

// TestL1Pool_Get_NilPool CLC 铁律(nil 防御):nil *L1Pool 调 Get 必须安全降级,不 panic。
// 起因:CLC 铁律根因区——零值 Manager{}.pool==nil 时 Lookup 经 m.pool.Get 解引用 nil pool。
// 验证:Get 在 nil receiver 下返回 redis.Nil(降级 miss),不 panic。
func TestL1Pool_Get_NilPool(t *testing.T) {
	var p *L1Pool // nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil *L1Pool.Get 不应 panic, recovered: %v", r)
		}
	}()
	_, err := p.Get(context.Background(), "k")
	if !errors.Is(err, redis.Nil) {
		t.Errorf("nil pool Get 应降级返回 redis.Nil, got %v", err)
	}
}

// TestL1Pool_Set_HappyPath 正常路径:Set 后 Get 命中,值与 TTL 一致。
func TestL1Pool_Set_HappyPath(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()

	if err := p.Set(context.Background(), "k1", "v1", 10*time.Second); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}
	val, err := p.Get(context.Background(), "k1")
	if err != nil {
		t.Fatalf("Set 后 Get 失败: %v", err)
	}
	if val != "v1" {
		t.Errorf("val=%q, want v1", val)
	}
	// TTL 落库(miniredis 精确 TTL)
	ttl := mr.TTL("k1")
	if ttl <= 0 || ttl > 10*time.Second {
		t.Errorf("TTL=%v, want (0,10s]", ttl)
	}
}

// TestL1Pool_Set_NilPool CLC nil 防御:nil pool Set 安全降级返回 error,不 panic。
func TestL1Pool_Set_NilPool(t *testing.T) {
	var p *L1Pool // nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil *L1Pool.Set 不应 panic, recovered: %v", r)
		}
	}()
	err := p.Set(context.Background(), "k", "v", time.Second)
	if err == nil {
		t.Error("nil pool Set 应返回 error 而非 nil")
	}
}

// TestL1Pool_Set_EmptyTTL 边界:零 TTL(不过期),Set 后应永久存活(miniredis 不快进)。
func TestL1Pool_Set_EmptyTTL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()

	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()

	if err := p.Set(context.Background(), "noexpire", "v", 0); err != nil {
		t.Fatalf("Set(0 TTL) 失败: %v", err)
	}
	if !mr.Exists("noexpire") {
		t.Error("0 TTL 的 key 应存在")
	}
}

// TestL1Pool_Ping_OK 正常路径:可达后端 Ping 返回 nil。
func TestL1Pool_Ping_OK(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()
	if err := p.Ping(context.Background()); err != nil {
		t.Errorf("Ping 应 nil, got %v", err)
	}
}

// TestL1Pool_Ping_AllDown 异常:所有 client 不可达,Ping 返回错误。
func TestL1Pool_Ping_AllDown(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	p := NewL1Pool(mr.Addr(), "")
	defer p.Close()
	mr.Close() // 后端关掉

	if err := p.Ping(context.Background()); err == nil {
		t.Error("全部 client down 时 Ping 应返回错误")
	}
}

// TestL1Pool_Ping_NilPool CLC nil 防御:nil pool Ping 安全降级,不 panic。
func TestL1Pool_Ping_NilPool(t *testing.T) {
	var p *L1Pool
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil *L1Pool.Ping 不应 panic, recovered: %v", r)
		}
	}()
	if err := p.Ping(context.Background()); err == nil {
		t.Error("nil pool Ping 应返回错误")
	}
}

// TestL1Pool_Ping_EmptyClients CLC 防御:空 clients 切片 Ping 安全降级,不 panic。
func TestL1Pool_Ping_EmptyClients(t *testing.T) {
	p := &L1Pool{clients: nil}
	if err := p.Ping(context.Background()); err == nil {
		t.Error("空 clients Ping 应返回错误")
	}
}

// TestL1Pool_Close_Idempotent 边界:多次 Close 不 panic(Close 应可重入)。
func TestL1Pool_Close_Idempotent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	p := NewL1Pool(mr.Addr(), "")
	p.Close()
	p.Close() // 第二次不应 panic
}

// TestL1Pool_Close_NilPool CLC nil 防御:nil pool Close 不 panic。
func TestL1Pool_Close_NilPool(t *testing.T) {
	var p *L1Pool
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil *L1Pool.Close 不应 panic, recovered: %v", r)
		}
	}()
	p.Close()
}

// TestL1Pool_WithMovedRetry_NilPool CLC:nil pool 直接调 withMovedRetry 安全降级。
func TestL1Pool_WithMovedRetry_NilPool(t *testing.T) {
	var p *L1Pool
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil pool withMovedRetry 不应 panic, recovered: %v", r)
		}
	}()
	fn := func(*redis.Client) (string, error) { return "x", nil }
	val, err := p.withMovedRetry(context.Background(), "k", fn)
	if err == nil {
		t.Errorf("nil pool 应返回 error, got val=%q", val)
	}
}

// TestL1Pool_WithMovedRetry_EmptyClients CLC:空 clients 切片返回 "no client available" 不 panic。
func TestL1Pool_WithMovedRetry_EmptyClients(t *testing.T) {
	p := &L1Pool{clients: nil}
	fn := func(*redis.Client) (string, error) { return "x", nil }
	_, err := p.withMovedRetry(context.Background(), "k", fn)
	if err == nil {
		t.Fatal("空 clients 应返回 error")
	}
	if err.Error() != "no client available" {
		t.Errorf("err=%q, want 'no client available'", err.Error())
	}
}

// TestL1Pool_WithMovedRetry_NilErrorInFn redis.Nil 短路:fn 返回 redis.Nil 应直接返回,不遍历其他 pod。
// CLC 修正(2026-08-20)路径:正常 miss 不应计入 error,不浪费多次 Redis 调用。
func TestL1Pool_WithMovedRetry_NilErrorInFn(t *testing.T) {
	p := &L1Pool{clients: []*redis.Client{nil, nil}}
	callCount := 0
	fn := func(*redis.Client) (string, error) {
		callCount++
		return "", redis.Nil
	}
	_, err := p.withMovedRetry(context.Background(), "k", fn)
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("应返回 redis.Nil, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("redis.Nil 应短路返回,只调用 1 次 fn,实际 %d 次(应不遍历其他 pod)", callCount)
	}
}

// TestL1Pool_ParseMoved_Table parseMoved 边界:正常 MOVED / 非 MOVED / 畸形消息。
func TestL1Pool_ParseMoved_Table(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantOk   bool
		wantSlot string
		wantHost string
	}{
		{"正常 MOVED", toErr("MOVED 12000 redis-cluster-new-3.svc:6379"), true, "12000", "redis-cluster-new-3.svc"},
		{"无前缀", toErr("GET nil"), false, "", ""},
		{"nil error", nil, false, "", ""},
		{"MOVED 但少于3段", toErr("MOVED 12000"), false, "", ""},
		{"空字符串", toErr(""), false, "", ""},
		{"MOVED 无端口", toErr("MOVED 5 redis-cluster-new-2.svc"), true, "5", "redis-cluster-new-2.svc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moved, ok := parseMoved(tt.err)
			if ok != tt.wantOk {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOk)
			}
			if ok {
				if moved.slot != tt.wantSlot {
					t.Errorf("slot=%q, want %q", moved.slot, tt.wantSlot)
				}
				if moved.host != tt.wantHost {
					t.Errorf("host=%q, want %q", moved.host, tt.wantHost)
				}
			}
		})
	}
}

// TestL1Pool_ExtractPodIndex_Table extractPodIndex 边界:正常 FQDN / 畸形 / 无数字。
func TestL1Pool_ExtractPodIndex_Table(t *testing.T) {
	tests := []struct {
		name string
		fqdn string
		want int
	}{
		{"pod3", "redis-cluster-new-3.redis-cluster-new.svc", 3},
		{"pod0", "redis-cluster-new-0.ekb-m2-aws-us-east-1.svc.cluster.local", 0},
		{"pod5", "redis-cluster-new-5.svc:6379", 5},
		{"无前缀", "some-other-host.svc", -1},
		{"空串", "", -1},
		{"前缀但无数字", "redis-cluster-new-.svc", -1},
		{"多段数字取首段", "redis-cluster-new-12abc.svc", 12},
		{"仅前缀片段", "redis-cluster-new-", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPodIndex(tt.fqdn)
			if got != tt.want {
				t.Errorf("extractPodIndex(%q)=%d, want %d", tt.fqdn, got, tt.want)
			}
		})
	}
}

// TestL1Pool_GetSet_RoundTrip_RealRedis 端到端:Set→Get 往返值正确(单 pod miniredis)。
func TestL1Pool_GetSet_RoundTrip_RealRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	p := NewL1Pool(mr.Addr(), "ignored-pwd")
	defer p.Close()

	pairs := map[string]string{"a": "1", "b": "long-value-222", "c": ""}
	for k, v := range pairs {
		if err := p.Set(context.Background(), k, v, 60*time.Second); err != nil {
			t.Fatalf("Set(%s)=%v", k, err)
		}
	}
	for k, want := range pairs {
		got, err := p.Get(context.Background(), k)
		if err != nil {
			t.Fatalf("Get(%s)=%v", k, err)
		}
		if got != want {
			t.Errorf("Get(%s)=%q, want %q", k, got, want)
		}
	}
}
