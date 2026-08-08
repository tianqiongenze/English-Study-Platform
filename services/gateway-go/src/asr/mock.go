// Package asr 提供语音识别抽象。本期 Mock 将音频帧映射为预设文本。
package asr

import (
	"encoding/base64"
	"strings"
)

// Provider 语音识别接口。
type Provider interface {
	// Recognize 将音频帧(base64)序列识别为文本。
	Recognize(chunks []string) string
}

// MockProvider 确定性 Mock。
// 解码 base64 后,若包含关键字则返回对应场景文本,否则返回默认问候。
type MockProvider struct{}

func (m *MockProvider) Recognize(chunks []string) string {
	if len(chunks) == 0 {
		return ""
	}
	// 解码第一帧探测内容
	raw, err := base64.StdEncoding.DecodeString(chunks[0])
	if err == nil {
		low := strings.ToLower(string(raw))
		switch {
		case strings.Contains(low, "check") || strings.Contains(low, "airport"):
			return "I'd like to check in for my flight."
		case strings.Contains(low, "coffee") || strings.Contains(low, "latte"):
			return "Can I get a latte, please?"
		case strings.Contains(low, "interview") || strings.Contains(low, "job"):
			return "I'm here for the interview."
		}
	}
	// 默认:取第一帧 base64 长度作为确定性输出
	if len(chunks[0]) > 0 {
		return "Hello, I'd like to practice English."
	}
	return ""
}
