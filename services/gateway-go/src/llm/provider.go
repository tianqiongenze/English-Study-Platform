// Package llm 提供统一的 LLM Provider 抽象。
// 默认实现 Anthropic Claude;支持 mock 模式(LLM_MODE=mock)用于压测避免费用与限流。
package llm

import (
	"context"
	"os"
)

// Message 是对话消息。
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// Provider 是 LLM 提供方抽象。
type Provider interface {
	// Chat 非流式对话,返回完整回复。
	Chat(ctx context.Context, messages []Message) (string, error)
}

// New 根据环境变量构造 Provider。
// LLM_MODE=mock → MockProvider;否则 AnthropicProvider(需 ANTHROPIC_API_KEY)。
func New() Provider {
	if os.Getenv("LLM_MODE") == "mock" {
		return &MockProvider{}
	}
	return &AnthropicProvider{
		APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		Model:   firstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), "claude-sonnet-4-6"),
		BaseURL: firstNonEmpty(os.Getenv("ANTHROPIC_BASE_URL"), "https://api.anthropic.com"),
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
