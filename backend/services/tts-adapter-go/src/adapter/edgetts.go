// Package adapter 提供 TTS 适配:把 ESP 自定义契约 {text} 转为 edge-tts 语音合成,
// 生成真实 MP3 音频(base64) + 四态情绪 + Preston Blair 口型时间轴。
//
// 契约(与 conversation-go tts.HTTPProvider 对齐):
//   POST /synthesize  {text:"..."}  ->  {audio:"<base64>", emotion:"happy|neutral|encouraging|sad", visemes:[...]}
//
// SenseEmotion / BuildVisemes 从 conversation-go/src/tts/mock.go 原样移植(关键词→四态情绪、字符→口型)。
// 不依赖 Python/fastapi——纯 Go,单进程完成。
package adapter

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"

	edgetts "github.com/lib-x/edgetts"
	"esp/tts-adapter-go/src/viseme"
)

// SynthesizeReq ESP 自定义契约(与 conversation-go tts.HTTPProvider 对齐)。
type SynthesizeReq struct {
	Text string `json:"text"`
}

// SynthesizeResp 语音合成结果(与 conversation-go tts.Result 对齐)。
type SynthesizeResp struct {
	Audio   string          `json:"audio"`   // base64 音频(MP3)
	Emotion string          `json:"emotion"` // 四态契约:happy/neutral/encouraging/sad
	Visemes []viseme.Viseme `json:"visemes"` // 口型时间轴
}

// EdgeTtsClient 封装 edge-tts 调用。
type EdgeTtsClient struct {
	client *edgetts.Client
}

// NewEdgeTtsClient 从 EDGE_TTS_VOICE env 构造。未设置使用 Jenny(英语教育默认)。
func NewEdgeTtsClient() *EdgeTtsClient {
	voice := os.Getenv("EDGE_TTS_VOICE")
	if voice == "" {
		voice = "en-US-JennyNeural" // Jenny: 英式/美式标准,清晰,适合英语教学
	}
	opts := []edgetts.Option{edgetts.WithVoice(voice)}
	return &EdgeTtsClient{client: edgetts.New(opts...)}
}

// Synthesize 调用 edge-tts 将文本转为 MP3,返回 base64 音频 + 情绪 + 口型。
// 失败返回 neutral 空结果(与 conversation-go tts.HTTPProvider 语义一致)。
func (c *EdgeTtsClient) Synthesize(ctx context.Context, text string) SynthesizeResp {
	if text == "" {
		return SynthesizeResp{Emotion: "neutral"}
	}

	// 调 edge-tts 生成 MP3
	audioBytes, err := c.client.Bytes(ctx, text)
	if err != nil {
		slog.Error("tts-adapter: edge-tts 调用失败", "err", err)
		return SynthesizeResp{Emotion: "neutral"}
	}

	if len(audioBytes) == 0 {
		slog.Error("tts-adapter: edge-tts 返回空音频")
		return SynthesizeResp{Emotion: "neutral"}
	}

	// base64 编码
	audioB64 := base64.StdEncoding.EncodeToString(audioBytes)

	// 情绪感知(关键词→四态)
	emotion := viseme.SenseEmotion(text)

	// 口型时间轴(字符→Preston Blair 口型)
	vs := viseme.BuildVisemes(text)

	return SynthesizeResp{
		Audio:   audioB64,
		Emotion: emotion,
		Visemes: vs,
	}
}
