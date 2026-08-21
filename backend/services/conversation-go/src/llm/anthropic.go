package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// httpClient 外部 LLM 调用专用 client(RWT 铁律:外部 LLM 超时分级 60s)。
// 替代 http.DefaultClient(无 Timeout,裸调可永久挂起致主链阻塞)。
// 超时分级:外部 LLM 30-60s;此处取 60s 上限,覆盖 CSDN GLM 长尾(实测 14-22s)。
var httpClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second, // 首字节超时(上游多久开始吐响应头)
		IdleConnTimeout:       90 * time.Second,
	},
}

// AnthropicProvider 调用 Anthropic Claude Messages API。
type AnthropicProvider struct {
	APIKey    string
	AuthToken string
	Model     string
	BaseURL   string
}

type anthropicReq struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []aMessage `json:"messages"`
}
type aMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type anthropicResp struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	if p.APIKey == "" && p.AuthToken == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN not set")
	}
	var sys string
	var msgs []aMessage
	for _, m := range messages {
		if m.Role == "system" {
			sys += m.Content + "\n"
			continue
		}
		msgs = append(msgs, aMessage{Role: m.Role, Content: m.Content})
	}
	body, _ := json.Marshal(anthropicReq{
		Model: p.Model, MaxTokens: 1024, System: sys, Messages: msgs,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", p.BaseURL+"/v1/messages", bytes.NewReader(body))
	req.Header.Set("x-api-key", firstNonEmpty(p.APIKey, p.AuthToken))
	if p.AuthToken != "" {
		// CC Switch 等本地代理按 Authorization: Bearer 鉴权;官方端点按 x-api-key。两者都发兼容。
		req.Header.Set("authorization", "Bearer "+p.AuthToken)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	// P0-R1(RWT 铁律,2026-08-21):改用包级带超时 httpClient(60s Timeout + 30s ResponseHeaderTimeout),
	// 替代 http.DefaultClient.Do(无 Timeout,LLM 推理 8-34s+长尾时上游 hang 致 goroutine 永久挂起耗尽 worker)。
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var ar anthropicResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", err
	}
	if ar.Error != nil {
		return "", fmt.Errorf("anthropic: %s", ar.Error.Message)
	}
	if len(ar.Content) == 0 {
		return "", nil
	}
	return ar.Content[0].Text, nil
}
