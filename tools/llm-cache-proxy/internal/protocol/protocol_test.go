package protocol

import (
	"encoding/json"
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		path string
		want Protocol
	}{
		{"/v1/messages", Anthropic},
		{"/v1/messages?foo=bar", Anthropic},
		{"/v1/chat/completions", OpenAI},
		{"/api/chat", Ollama},
		{"/api/generate", Ollama},
		{"/unknown", Unknown},
	}
	for _, tt := range tests {
		got := Detect(tt.path)
		if got != tt.want {
			t.Errorf("Detect(%q)=%q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestNormalizeAnthropic(t *testing.T) {
	// system 为字符串
	body := []byte(`{"model":"claude-sonnet-4-6","system":"you are helpful","messages":[{"role":"user","content":"hello"}]}`)
	key, err := Normalize("/v1/messages", body)
	if err != nil {
		t.Fatal(err)
	}
	if key.Protocol != Anthropic {
		t.Errorf("protocol=%s, want anthropic", key.Protocol)
	}
	if key.Model != "claude-sonnet-4-6" {
		t.Errorf("model=%s", key.Model)
	}
	if key.System != "you are helpful" {
		t.Errorf("system=%s", key.System)
	}
	if key.User == "" {
		t.Error("user 为空")
	}
	if key.Raw == "" {
		t.Error("raw 为空")
	}
}

func TestNormalizeAnthropicBlockContent(t *testing.T) {
	// system + content 为 block 数组形态(ZCode 实际格式)
	body := []byte(`{"model":"m","system":[{"type":"text","text":"sys"}],"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	key, err := Normalize("/v1/messages", body)
	if err != nil {
		t.Fatal(err)
	}
	if key.System != "sys" {
		t.Errorf("system=%s, want sys", key.System)
	}
	if key.User == "" {
		t.Error("user 为空(block content 未提取)")
	}
}

func TestNormalizeOpenAI(t *testing.T) {
	body := []byte(`{"model":"gpt-4","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}]}`)
	key, err := Normalize("/v1/chat/completions", body)
	if err != nil {
		t.Fatal(err)
	}
	if key.Protocol != OpenAI {
		t.Errorf("protocol=%s", key.Protocol)
	}
	if key.System != "sys" {
		t.Errorf("system=%s", key.System)
	}
}

func TestNormalizeOllama(t *testing.T) {
	body := []byte(`{"model":"llama3","messages":[{"role":"user","content":"hi"}]}`)
	key, err := Normalize("/api/chat", body)
	if err != nil {
		t.Fatal(err)
	}
	if key.Protocol != Ollama {
		t.Errorf("protocol=%s", key.Protocol)
	}
}

func TestNormalizeSameInputStableKey(t *testing.T) {
	// 相同输入应产生相同 Raw(缓存键稳定性)
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"stable-test"}]}`)
	k1, _ := Normalize("/v1/chat/completions", body)
	k2, _ := Normalize("/v1/chat/completions", body)
	if k1.Raw != k2.Raw {
		t.Errorf("相同输入 Raw 不稳定: %q vs %q", k1.Raw, k2.Raw)
	}
}

func TestNormalizeProtocolIsolation(t *testing.T) {
	// 同内容不同协议应产生不同 Raw(L1 协议分键要求)
	content := `{"model":"m","messages":[{"role":"user","content":"x"}]}`
	ka, _ := Normalize("/v1/messages", []byte(content))
	ko, _ := Normalize("/v1/chat/completions", []byte(content))
	if ka.Raw == ko.Raw {
		t.Error("不同协议 Raw 相同,违反 L1 协议分键")
	}
}

func TestNormalizeInvalidJSON(t *testing.T) {
	_, err := Normalize("/v1/messages", []byte(`{invalid`))
	if err == nil {
		t.Error("非法 JSON 应返回 error")
	}
}

func TestExtractAnthropicTextString(t *testing.T) {
	s := extractAnthropicText(json.RawMessage(`"hello"`))
	if s != "hello" {
		t.Errorf("got %q", s)
	}
}

func TestExtractAnthropicTextEmpty(t *testing.T) {
	s := extractAnthropicText(nil)
	if s != "" {
		t.Errorf("empty input should return empty, got %q", s)
	}
}
