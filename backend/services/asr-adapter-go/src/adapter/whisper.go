// Package adapter 提供 Whisper ASR 引擎适配:把 ESP 自定义契约 {chunks:[base64]}
// 转换为 OpenAI Whisper 兼容 multipart,转发到集群内 Whisper 容器,解析回 {text}。
package adapter

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

// WhisperClient 封装对 OpenAI Whisper 兼容端点的调用。
type WhisperClient struct {
	Endpoint string        // Whisper /v1/audio/transcriptions 完整 URL
	Client   *http.Client
}

// NewWhisperClient 从 WHISPER_ENDPOINT env 构造客户端。未配置时显式失败(拒绝静默降级)。
//
// GPU/本地模型预留(2026-08-19):
//   ASR_MODE=local → 未来直调本机 Whisper-MPS(零 HTTP,Apple M1 Max GPU 推理)
//   ASR_MODE=http  → 集群内 Whisper 容器(默认,当前阶段)
// 当前 local 分支仅打印告警 + 走 http fallback,真实 local 实现待 Whisper-MPS 安装后补(DEFERRED:多节点清单#GPU)。
func NewWhisperClient() *WhisperClient {
	mode := os.Getenv("ASR_MODE")
	if mode == "local" {
		slog.Warn("asr-adapter: ASR_MODE=local 预留分支,本机 Whisper-MPS 未就绪,降级 http(DEFERRED:多节点清单#GPU)")
		// 未来:返回 &WhisperClient{Local:true, MpsDevice:"mps"} 走本地进程调用,不走 HTTP
	}
	ep := os.Getenv("WHISPER_ENDPOINT")
	if ep == "" {
		slog.Error("WHISPER_ENDPOINT 未配置;真实 ASR 引擎缺失,拒绝静默降级")
		os.Exit(1)
	}
	return &WhisperClient{
		Endpoint: ep,
		Client:   &http.Client{Timeout: 60 * time.Second}, // CPU 推理较慢,给足超时
	}
}

// TranscribeReq ESP 自定义契约(与 conversation-go asr.HTTPProvider 对齐)。
type TranscribeReq struct {
	Chunks []string `json:"chunks"` // base64 音频帧序列
}

// TranscribeResp 识别结果(与 conversation-go asr.asrResp 对齐)。
type TranscribeResp struct {
	Text string `json:"text"`
}

// whisperResp OpenAI Whisper /v1/audio/transcriptions 响应。
type whisperResp struct {
	Text string `json:"text"`
}

// Transcribe 将 base64 音频帧拼成单个音频字节流,以 multipart 转发到 Whisper,返回识别文本。
// 失败返回空 text(由调用方/上游按降级处理,与 conversation-go Recognize 语义一致)。
func (w *WhisperClient) Transcribe(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}

	// 拼接 base64 音频帧为连续字节流
	var audio bytes.Buffer
	for _, c := range chunks {
		b, err := base64.StdEncoding.DecodeString(c)
		if err != nil {
			slog.Warn("asr-adapter: 非法 base64 chunk,跳过", "err", err)
			continue
		}
		audio.Write(b)
	}
	if audio.Len() == 0 {
		slog.Error("asr-adapter: 所有 chunk 解码失败,无可识别音频")
		return ""
	}

	// 构造 multipart/form-data: file=<audio>, model=<model>
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "audio.wav")
	if err != nil {
		slog.Error("asr-adapter: multipart file 创建失败", "err", err)
		return ""
	}
	if _, err := fw.Write(audio.Bytes()); err != nil {
		slog.Error("asr-adapter: multipart 写入失败", "err", err)
		return ""
	}
	model := os.Getenv("WHISPER_MODEL")
	if model == "" {
		model = "base"
	}
	_ = mw.WriteField("model", model)
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, w.Endpoint, &body)
	if err != nil {
		slog.Error("asr-adapter: 构造 Whisper 请求失败", "err", err)
		return ""
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := w.Client.Do(req)
	if err != nil {
		slog.Error("asr-adapter: Whisper 请求失败", "err", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		slog.Error("asr-adapter: Whisper 非 200", "status", resp.StatusCode, "body", string(b))
		return ""
	}

	var out whisperResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		slog.Error("asr-adapter: Whisper 响应解析失败", "err", err)
		return ""
	}
	return out.Text
}
