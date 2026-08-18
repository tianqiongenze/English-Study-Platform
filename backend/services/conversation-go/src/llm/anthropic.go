package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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
	resp, err := http.DefaultClient.Do(req)
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
