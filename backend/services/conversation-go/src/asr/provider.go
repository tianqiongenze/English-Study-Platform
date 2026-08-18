// Package asr 提供语音识别抽象。
package asr

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// New 按 ASR_MODE 构造识别 Provider:
//   - "http"/"openai":HTTPProvider,转发到 ASR_ENDPOINT。真实 ASR 引擎未接入
//     (DEFERRED-需ASR引擎,见多节点清单#4);ASR_ENDPOINT 未配置时显式失败,拒绝静默降级。
//   - "mock"(默认,仅联调/CI):确定性 MockProvider。生产 manifest 不设 ASR_MODE,
//     由就绪度报告核对 DEFERRED 项,不作为上线默认。
func New() Provider {
	mode := os.Getenv("ASR_MODE")
	switch mode {
	case "http", "openai", "real":
		ep := os.Getenv("ASR_ENDPOINT")
		if ep == "" {
			slog.Error("ASR_MODE=" + mode + " 但 ASR_ENDPOINT 未配置(DEFERRED-真实ASR引擎,见多节点清单#4);拒绝静默降级")
			os.Exit(1)
		}
		return &HTTPProvider{Endpoint: ep, Client: &http.Client{Timeout: 10 * time.Second}}
	default:
		slog.Warn("ASR_MODE 未配置或无效,使用 mock ASR(单节点 DEFERRED 占位,见多节点清单#4)")
		return new(MockProvider)
	}
}

// HTTPProvider 调用外部 ASR 引擎(OpenAI Whisper 兼容或自研端点)。
type HTTPProvider struct {
	Endpoint string
	Client   *http.Client
}

type asrReq struct {
	Chunks []string `json:"chunks"`
}

type asrResp struct {
	Text string `json:"text"`
}

// Recognize POST chunks 到 ASR_ENDPOINT,返回识别文本;失败返回空串(调用方按降级处理)。
func (p *HTTPProvider) Recognize(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	body, _ := json.Marshal(asrReq{Chunks: chunks})
	resp, err := p.Client.Post(p.Endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Error("asr recognize request failed", "err", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Error("asr recognize non-200", "status", resp.StatusCode)
		return ""
	}
	var out asrResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		slog.Error("asr recognize decode failed", "err", err)
		return ""
	}
	return out.Text
}

var _ Provider = (*HTTPProvider)(nil)