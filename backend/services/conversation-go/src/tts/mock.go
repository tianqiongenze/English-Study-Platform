// Package tts 提供语音合成抽象。Mock 返回占位 base64 音频 + 情绪驱动口型序列(#121)。
package tts

import (
	"context"
	"encoding/base64"
	"strings"
	"unicode"

	"esp/conversation-go/src/cache"
	"esp/conversation-go/src/ws"
)

// Provider 语音合成接口。
type Provider interface {
	// Synthesize 将文本转为情绪驱动的合成结果(音频 base64 + 情绪 + 口型序列)。
	Synthesize(ctx context.Context, text string) Result
}

// Result 数字人 TTS 合成结果:音频 + 情绪标签 + viseme 口型时间轴。
type Result struct {
	Audio   string      `json:"audio"`             // base64 占位音频
	Emotion string      `json:"emotion"`           // positive / negative / neutral
	Visemes []ws.Viseme `json:"visemes"`           // 口型时间轴(相对音频起点 ms)
}

// 情绪词表与 emotion-svc sense_emotion 对齐(同一份关键词规则的双语种实现)。
var (
	negativeWords = []string{"sad", "tired", "frustrated", "angry", "anxious", "nervous", "worried", "afraid", "stressed", "difficult", "hard", "fail", "bad"}
	positiveWords = []string{"happy", "excited", "great", "good", "love", "confident", "proud", "easy", "fun", "wonderful", "amazing"}
)

// SenseEmotion 关键词情绪感知(与 emotion-svc NEGATIVE/POSITIVE 同源)。
func SenseEmotion(text string) string {
	low := strings.ToLower(text)
	for _, w := range negativeWords {
		if strings.Contains(low, w) {
			return "negative"
		}
	}
	for _, w := range positiveWords {
		if strings.Contains(low, w) {
			return "positive"
		}
	}
	return "neutral"
}

// visemeForChar 字符 → Preston Blair 口型编号(简化映射)。
func visemeForChar(r rune) string {
	switch {
	case unicode.ToLower(r) == 'o' || unicode.ToLower(r) == 'u':
		return "O"
	case unicode.ToLower(r) == 'e' || unicode.ToLower(r) == 'i':
		return "E"
	case unicode.ToLower(r) == 'a':
		return "AA"
	case r == 'm' || r == 'b' || r == 'p':
		return "MBP"
	case r == 'f' || r == 'v':
		return "FV"
	case unicode.IsLetter(r):
		return "etc"
	default:
		return ""
	}
}

// BuildVisemes 文本 → 口型时间轴。语速按 60ms/viseme 均匀近似(Mock 无真实时长)。
// 连续相同口型合并,序列长度上限 400(超长文本截断,防 WS 帧膨胀)。
func BuildVisemes(text string) []ws.Viseme {
	var out []ws.Viseme
	t := 0
	var prev string
	for _, r := range strings.ToLower(text) {
		v := visemeForChar(r)
		if v == "" {
			t += 30 // 非字母(空格/标点)短停顿
			continue
		}
		if v == prev && len(out) > 0 {
			out[len(out)-1].End = t + 60
			t += 60
			continue
		}
		out = append(out, ws.Viseme{ID: v, Start: t, End: t + 60})
		prev = v
		t += 60
		if len(out) >= 400 {
			break
		}
	}
	return out
}

// MockProvider 确定性 Mock:返回文本的 base64 占位音频 + 情绪 + 口型。
// 真实环境接 TTS 模型后,相同文本合成结果可复用 —— 走多级缓存(L1 进程内 + L2 Redis),
// 显著省合成成本。本期 Mock 也走缓存以验证链路(对相同输入稳定)。
type MockProvider struct{}

func (m *MockProvider) Synthesize(ctx context.Context, text string) Result {
	if text == "" {
		return Result{}
	}
	val, err := cache.GetOrLoad(ctx, "gateway:tts:v2", []any{text},
		func(ctx context.Context) (any, error) {
			return Result{
				Audio:   base64.StdEncoding.EncodeToString([]byte("tts:" + text)),
				Emotion: SenseEmotion(text),
				Visemes: BuildVisemes(text),
			}, nil
		}, false)
	if err != nil || val == nil {
		return Result{
			Audio:   base64.StdEncoding.EncodeToString([]byte("tts:" + text)),
			Emotion: SenseEmotion(text),
			Visemes: BuildVisemes(text),
		}
	}
	return val.(Result)
}
