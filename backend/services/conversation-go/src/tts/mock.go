// Package tts 提供语音合成抽象。本期 Mock 返回占位 base64 音频。
package tts

import (
	"context"
	"encoding/base64"

	"esp/conversation-go/src/cache"
)

// Provider 语音合成接口。
type Provider interface {
	// Synthesize 将文本转为音频(base64)。
	Synthesize(ctx context.Context, text string) string
}

// MockProvider 确定性 Mock:返回文本的 base64 作为占位音频。
// 真实环境接 TTS 模型后,相同文本合成结果可复用 —— 走多级缓存(L1 进程内 + L2 Redis),
// 显著省合成成本。本期 Mock 也走缓存以验证链路(对相同输入稳定)。
type MockProvider struct{}

func (m *MockProvider) Synthesize(ctx context.Context, text string) string {
	if text == "" {
		return ""
	}
	val, err := cache.GetOrLoad(ctx, "gateway:tts", []any{text},
		func(ctx context.Context) (any, error) {
			return base64.StdEncoding.EncodeToString([]byte("tts:" + text)), nil
		}, false)
	if err != nil || val == nil {
		return base64.StdEncoding.EncodeToString([]byte("tts:" + text))
	}
	return val.(string)
}
