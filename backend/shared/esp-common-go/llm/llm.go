// Package llm 提供统一 Anthropic Claude(兼容 CC Switch 本地代理)客户端。
//
// 共享包铁律 ①:reading-go / listening-go 等 ≥2 服务共用的 LLM 调用下沉到本包,禁止服务内复制。
// 契约:POST {BaseURL}/v1/messages;鉴权 x-api-key + Authorization Bearer 双发兼容
//   (官方端点认 x-api-key;CC Switch 等本地代理认 Bearer/AUTH_TOKEN)。
// 降级:Key 缺失返回显式错误,不静默 mock。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Client struct {
	APIKey    string
	AuthToken string
	Model     string
	BaseURL   string
	hc        *http.Client
}

// New 从环境变量构造(ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN / ANTHROPIC_BASE_URL / ANTHROPIC_MODEL)。
func New() *Client {
	return &Client{
		APIKey:    os.Getenv("ANTHROPIC_API_KEY"),
		AuthToken: os.Getenv("ANTHROPIC_AUTH_TOKEN"),
		Model:     FirstNonEmpty(os.Getenv("ANTHROPIC_MODEL"), "claude-sonnet-4-6"),
		BaseURL:   FirstNonEmpty(os.Getenv("ANTHROPIC_BASE_URL"), "https://api.anthropic.com"),
		hc:        &http.Client{Timeout: 60 * time.Second},
	}
}

// Chat 非流式调用 Claude Messages API,返回拼接文本。
func (c *Client) Chat(ctx context.Context, system, user string) (string, error) {
	if c.APIKey == "" && c.AuthToken == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY/ANTHROPIC_AUTH_TOKEN not set")
	}
	body, err := json.Marshal(map[string]any{
		"model":      c.Model,
		"max_tokens": 1024,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", FirstNonEmpty(c.APIKey, c.AuthToken))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	if c.AuthToken != "" {
		req.Header.Set("authorization", "Bearer "+c.AuthToken)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf := make([]byte, 512)
		n, _ := resp.Body.Read(buf)
		return "", fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(buf[:n]))
	}
	var ar struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return "", err
	}
	var out string
	for _, p := range ar.Content {
		out += p.Text
	}
	return out, nil
}

func FirstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ExtractJSON 从 LLM 可能夹杂 markdown 的输出中提取首个 JSON 对象。
func ExtractJSON(text string) (map[string]any, error) {
	start, end := -1, -1
	// 优先取 ```json 代码块
	for i := 0; i < len(text); i++ {
		if start == -1 {
			if c := text[i]; c == '{' {
				start = i
			}
		} else if text[i] == '}' {
			if end == -1 || i > end {
				// 取最后一个 '}' 作为闭合
				end = i
			}
		}
	}
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object in LLM output")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &m); err != nil {
		return nil, err
	}
	return m, nil
}