package llm

import (
	"context"
	"fmt"
	"strings"
)

// MockProvider 是确定性 mock,用于压测与离线开发。
// 依据输入关键词生成启发式回复,保证链路可跑通而不依赖外部 API。
type MockProvider struct{}

func (m *MockProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	var lastUser string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUser = strings.ToLower(messages[i].Content)
			break
		}
	}
	switch {
	case strings.Contains(lastUser, "passport") || strings.Contains(lastUser, "check in"):
		return "Sure! May I see your passport and your booking reference, please?", nil
	case strings.Contains(lastUser, "coffee") || strings.Contains(lastUser, "order"):
		return "Welcome! What can I get for you today? We have a great seasonal latte.", nil
	case strings.Contains(lastUser, "interview") || strings.Contains(lastUser, "experience"):
		return "Thank you for having me. Could you tell me more about the team I'd be working with?", nil
	case lastUser == "":
		return "Hello! I'm your AI English tutor. What scenario would you like to practice today?", nil
	default:
		return fmt.Sprintf("That's interesting — let's continue. Could you elaborate on \"%s\"?", truncate(lastUser, 40)), nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
