// Package protocol — LLM 协议适配层(Anthropic / OpenAI / Ollama 三协议归一化)
//
// 设计:LCO 代理在本机工具与上游 LLM 之间做透明缓存层。不同工具用不同协议,
// L1 精确缓存按协议分键(用户决策:L1分键+L2跨协议重生成),L2 语义缓存放归一化向量。
package protocol

import (
	"encoding/json"
	"strings"
)

// Protocol LLM 协议标识。
type Protocol string

const (
	// Anthropic /v1/messages 协议。
	Anthropic Protocol = "anthropic" // /v1/messages
	// OpenAI /v1/chat/completions 协议。
	OpenAI  Protocol = "openai" // /v1/chat/completions
	// Ollama /api/chat 协议。
	Ollama  Protocol = "ollama" // /api/chat
	// Unknown 未知协议(降级直穿,不缓存)。
	Unknown Protocol = "unknown"
)

// NormalizeKey 从原始请求体提取归一化缓存键素材(system + user + model)。
// 三协议各自抽 system/user 文本,拼接后供 L1 哈希 + L2 向量化。
type NormalizeKey struct {
	Protocol Protocol
	Model    string
	System   string
	User     string
	// Raw 归一化后的字符串(SHA 输入 + 向量输入共用)
	Raw string
}

// Detect 按路径推断协议。
func Detect(path string) Protocol {
	switch {
	case strings.Contains(path, "/v1/messages"):
		return Anthropic
	case strings.Contains(path, "/v1/chat/completions"):
		return OpenAI
	case strings.Contains(path, "/api/chat"), strings.Contains(path, "/api/generate"):
		return Ollama
	default:
		return Unknown
	}
}

// Normalize 解析请求体,提取归一化键。
// 失败返回 (key, error),调用方按"无法缓存"处理(直穿上游)。
func Normalize(path string, body []byte) (NormalizeKey, error) {
	p := Detect(path)
	switch p {
	case Anthropic:
		return normalizeAnthropic(body)
	case OpenAI:
		return normalizeOpenAI(body)
	case Ollama:
		return normalizeOllama(body)
	default:
		return NormalizeKey{Protocol: Unknown}, nil
	}
}

// --- Anthropic Messages API ---
// {model, system: str|[{text}], messages:[{role,content: str|[{text}]}]}
type anthropicReq struct {
	Model    string          `json:"model"`
	System   json.RawMessage `json:"system"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

func normalizeAnthropic(body []byte) (NormalizeKey, error) {
	var r anthropicReq
	if err := json.Unmarshal(body, &r); err != nil {
		return NormalizeKey{Protocol: Anthropic}, err
	}
	var sb strings.Builder
	sb.WriteString(extractAnthropicText(r.System))
	for _, m := range r.Messages {
		sb.WriteString(m.Role)
		sb.WriteString(":")
		sb.WriteString(extractAnthropicText(m.Content))
		sb.WriteString("\n")
	}
	return NormalizeKey{
		Protocol: Anthropic,
		Model:    r.Model,
		System:   extractAnthropicText(r.System),
		User:     sb.String(),
		Raw:      "anthropic|" + r.Model + "|" + sb.String(),
	}, nil
}

// extractAnthropicText 处理 content 可能是 string 或 [{type:"text",text:...}] 两种形态。
func extractAnthropicText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	// 尝试 array of blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Type == "text" || b.Type == "" {
				sb.WriteString(b.Text)
			}
		}
		return strings.TrimSpace(sb.String())
	}
	return ""
}

// --- OpenAI Chat Completions ---
// {model, messages:[{role,content}]}
type openaiReq struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func normalizeOpenAI(body []byte) (NormalizeKey, error) {
	var r openaiReq
	if err := json.Unmarshal(body, &r); err != nil {
		return NormalizeKey{Protocol: OpenAI}, err
	}
	var system, user strings.Builder
	for _, m := range r.Messages {
		if m.Role == "system" {
			system.WriteString(m.Content)
		} else {
			user.WriteString(m.Role)
			user.WriteString(":")
			user.WriteString(m.Content)
			user.WriteString("\n")
		}
	}
	return NormalizeKey{
		Protocol: OpenAI,
		Model:    r.Model,
		System:   strings.TrimSpace(system.String()),
		User:     user.String(),
		Raw:      "openai|" + r.Model + "|" + system.String() + "|" + user.String(),
	}, nil
}

// --- Ollama /api/chat ---
// {model, messages:[{role,content}]}
type ollamaReq struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func normalizeOllama(body []byte) (NormalizeKey, error) {
	var r ollamaReq
	if err := json.Unmarshal(body, &r); err != nil {
		return NormalizeKey{Protocol: Ollama}, err
	}
	var system, user strings.Builder
	for _, m := range r.Messages {
		if m.Role == "system" {
			system.WriteString(m.Content)
		} else {
			user.WriteString(m.Role)
			user.WriteString(":")
			user.WriteString(m.Content)
			user.WriteString("\n")
		}
	}
	return NormalizeKey{
		Protocol: Ollama,
		Model:    r.Model,
		System:   strings.TrimSpace(system.String()),
		User:     user.String(),
		Raw:      "ollama|" + r.Model + "|" + system.String() + "|" + user.String(),
	}, nil
}
