// Package tts 提供语音合成抽象。
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"esp/conversation-go/src/ws"
)

// New 按 TTS_MODE 构造合成 Provider:
//   - "http"/"openai":HTTPProvider,转发到 TTS_ENDPOINT。真实 TTS 引擎未接入
//     (DEFERRED-需TTS引擎,见多节点清单#4);TTS_ENDPOINT 未配置时显式失败,拒绝静默降级。
//   - "mock"(默认,仅联调/CI):确定性 MockProvider。生产 manifest 不设 TTS_MODE,
//     由就绪度报告核对 DEFERRED 项,不作为上线默认。
func New() Provider {
	mode := os.Getenv("TTS_MODE")
	switch mode {
	case "http", "openai", "real":
		ep := os.Getenv("TTS_ENDPOINT")
		if ep == "" {
			slog.Error("TTS_MODE=" + mode + " 但 TTS_ENDPOINT 未配置(DEFERRED-真实TTS引擎,见多节点清单#4);拒绝静默降级")
			os.Exit(1)
		}
		return &HTTPProvider{Endpoint: ep, Client: &http.Client{Timeout: 15 * time.Second}}
	default:
		slog.Warn("TTS_MODE 未配置或无效,使用 mock TTS(单节点 DEFERRED 占位,见多节点清单#4)")
		return new(MockProvider)
	}
}

var _ Provider = (*HTTPProvider)(nil)

// HTTPProvider 调用外部 TTS 引擎,返回 base64 音频 + 四态情绪 + 口型序列。
type HTTPProvider struct {
	Endpoint string
	Client   *http.Client
}

type ttsReq struct {
	Text string `json:"text"`
}

type ttsResp struct {
	Audio   string      `json:"audio"`   // base64 音频
	Emotion string      `json:"emotion"` // 四态契约:happy/neutral/encouraging/sad
	Visemes []ws.Viseme `json:"visemes"` // 口型时间轴
}

// Synthesize POST text 到 TTS_ENDPOINT;失败/非 200/非法 base64 返回 neutral 空结果(调用方降级)。
func (p *HTTPProvider) Synthesize(ctx context.Context, text string) Result {
	body, _ := json.Marshal(ttsReq{Text: text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(body))
	if err != nil {
		slog.Error("tts synth request build failed", "err", err)
		return Result{Emotion: "neutral"}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		slog.Error("tts synth request failed", "err", err)
		return Result{Emotion: "neutral"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Error("tts synth non-200", "status", resp.StatusCode)
		return Result{Emotion: "neutral"}
	}
	var out ttsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		slog.Error("tts synth decode failed", "err", err)
		return Result{Emotion: "neutral"}
	}
	if out.Emotion == "" {
		out.Emotion = "neutral"
	}
	if _, err := base64.StdEncoding.DecodeString(out.Audio); err != nil {
		slog.Error("tts synth invalid audio base64", "err", err)
		return Result{Emotion: "neutral"}
	}
	return Result{Audio: out.Audio, Emotion: out.Emotion, Visemes: out.Visemes}
}