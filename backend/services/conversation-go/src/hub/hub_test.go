package hub

import (
	"testing"
)

func TestFnv1a(t *testing.T) {
	cases := map[string]uint32{
		"":       2166136261, // offset basis
		"a":      0xe40c292c,
		"foobar": 0xbf9cf968,
	}
	for s, want := range cases {
		if got := fnv1a(s); got != want {
			t.Errorf("fnv1a(%q)=%#x want %#x", s, got, want)
		}
	}
}

func TestShardChannelFor(t *testing.T) {
	n := 8
	// 同一 sessionID 恒定映射到同一 shard(幂等)。
	s := "sess_12345"
	c1 := shardChannelFor(s, n)
	c2 := shardChannelFor(s, n)
	if c1 != c2 {
		t.Fatalf("shard not stable: %s vs %s", c1, c2)
	}
	// channel 名格式正确。
	if len(c1) <= len("ws:shard:") {
		t.Fatalf("bad channel name: %s", c1)
	}
	// 不同 sessionID 分散到多个 shard(分布性)。
	seen := map[string]int{}
	for i := 0; i < 1000; i++ {
		seen[shardChannelFor(id(i), n)]++
	}
	if len(seen) < n/2 {
		t.Fatalf("poor shard distribution: only %d/%d shards used", len(seen), n)
	}
}

func TestShardChannelsDistinct(t *testing.T) {
	n := 8
	got := map[string]bool{}
	for i := 0; i < n; i++ {
		got[shardChannel(i, n)] = true
	}
	if len(got) != n {
		t.Fatalf("expected %d distinct channels, got %d", n, len(got))
	}
}

func id(i int) string {
	// 模拟 session id 生成(前缀+数字)。
	return "sess_" + string(rune('a'+i%26)) + string(rune('0'+i%10))
}
