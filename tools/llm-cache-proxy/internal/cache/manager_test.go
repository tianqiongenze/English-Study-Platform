package cache

import (
	"testing"

	"llm-cache-proxy/internal/config"
	"llm-cache-proxy/internal/protocol"
)

func TestL1KeyProtocolIsolation(t *testing.T) {
	// L1 key 应含协议前缀(协议分键),不同协议同内容不同 key
	keyA := protocol.NormalizeKey{Protocol: protocol.Anthropic, Model: "m", Raw: "anthropic|m|x"}
	keyB := protocol.NormalizeKey{Protocol: protocol.OpenAI, Model: "m", Raw: "openai|m|x"}

	m := &Manager{}
	k1 := m.l1Key(keyA)
	k2 := m.l1Key(keyB)

	if k1 == k2 {
		t.Error("不同协议 L1 key 相同,违反协议分键")
	}
	// 验证 key 格式:lco:1:{protocol}:{sha16}
	if !contains(k1, "lco:1:anthropic:") {
		t.Errorf("L1 key 格式错误: %s", k1)
	}
	if !contains(k2, "lco:1:openai:") {
		t.Errorf("L1 key 格式错误: %s", k2)
	}
}

func TestL1KeyStable(t *testing.T) {
	// 相同输入 L1 key 稳定
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Model: "m", Raw: "stable"}
	m := &Manager{}
	k1 := m.l1Key(key)
	k2 := m.l1Key(key)
	if k1 != k2 {
		t.Error("L1 key 不稳定")
	}
}

func TestClassifyScene(t *testing.T) {
	tests := []struct {
		name string
		user string
		want string
	}{
		{"代码", "write a func main() with ```go", "code"},
		{"长文本", string(make([]byte, 2500)), "code"}, // >2000
		{"知识查询", "what is distributed system", "knowledge"},
		{"中文知识", "什么是微服务", "knowledge"},
		{"普通对话", "hello how are you", "chat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := protocol.NormalizeKey{User: tt.user}
			got := ClassifyScene(key)
			if got != tt.want {
				t.Errorf("ClassifyScene(%q)=%q, want %q", tt.user[:minInt(len(tt.user), 20)], got, tt.want)
			}
		})
	}
}

func TestLookupDisabled(t *testing.T) {
	// 缓存禁用时,Lookup 应返回空(不命中)。
	// 注入最小 cfg(CacheEnabled=false),Manager 零值 rdb=nil,Lookup 第一行检查即返回空。
	m := &Manager{cfg: &config.Config{CacheEnabled: false}}
	key := protocol.NormalizeKey{Protocol: protocol.Anthropic, Raw: "x"}
	// CacheEnabled=false 时 Lookup 第一行即返回空,不会解引用 ctx,故 nil 安全;
	// 此处显式验证禁用路径(不走真实 Redis),保留 nil 以区分正常调用路径。
	r := m.Lookup(nil, key) //nolint:staticcheck // SA1012: 故意 nil ctx,验证禁用分支不解引用 ctx。
	if r.Hit {
		t.Error("缓存禁用时不应命中")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && s[:len(sub)] == sub
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
