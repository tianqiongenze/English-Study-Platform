// Package dialog 是口语会话的核心编排逻辑(纯函数式,便于 TDD)。
package dialog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"esp/conversation-go/src/llm"
	"esp/conversation-go/src/session"
	"esp/conversation-go/src/ws"
)

// Engine 编排一次对话:纠错 → LLM → 启发式引导。
type Engine struct {
	LLM      llm.Provider
	HintIdle time.Duration // 卡壳判定阈值
}

// Result 是一次用户输入处理后产出的一组帧。
type Result struct {
	Frames []ws.Frame
	Turn   session.Turn // 需追加到历史的 AI 回复轮
}

// Process 处理用户文本输入,返回应推送的帧与需落库的 AI 轮。
func (e *Engine) Process(ctx context.Context, s *session.Session, userText string) (Result, error) {
	if e.LLM == nil {
		return Result{}, fmt.Errorf("llm provider not set")
	}
	out := Result{}

	// 1) 即时纠错(轻量规则)
	if c := correctGrammar(userText); c != nil {
		out.Frames = append(out.Frames, *c)
	}

	// 2) 拼上下文调 LLM
	msgs := []llm.Message{{Role: "system", Content: s.System}}
	for _, t := range s.History {
		msgs = append(msgs, llm.Message{Role: t.Role, Content: t.Content})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: userText})

	reply, err := e.LLM.Chat(ctx, msgs)
	if err != nil {
		return out, err
	}
	out.Frames = append(out.Frames, ws.Frame{Type: ws.TypeAIReply, Text: reply})
	out.Turn = session.Turn{Role: "assistant", Content: reply}
	return out, nil
}

// Hint 生成启发式引导帧。
func (e *Engine) Hint(s *session.Session) ws.Frame {
	opts := hintsFor(s.Scenario)
	return ws.Frame{Type: ws.TypeHint, Options: opts, Hints: opts}
}

// correctGrammar 用规则匹配常见错误,返回 correction 帧(无错返回 nil)。
var reWantsS = regexp.MustCompile(`(?i)\b(I|you|we|they)\s+(wants|needs|likes|goes)\b`)

func correctGrammar(text string) *ws.Frame {
	if loc := reWantsS.FindStringIndex(text); loc != nil {
		bad := text[loc[0]:loc[1]]
		fixed := reWantsS.ReplaceAllStringFunc(bad, func(s string) string {
			parts := strings.Fields(s)
			verb := strings.ToLower(parts[1])
			verb = strings.TrimSuffix(verb, "s")
			return parts[0] + " " + verb
		})
		return &ws.Frame{
			Type:      ws.TypeCorrection,
			Original:  bad,
			Corrected: fixed,
		}
	}
	return nil
}

// hintsFor 按场景返回启发式选项。
func hintsFor(scenario string) []string {
	switch strings.ToLower(scenario) {
	case "airport":
		return []string{"Show your passport", "Ask for a window seat", "Confirm your destination"}
	case "interview":
		return []string{"Talk about a past project", "Mention a key strength", "Ask about the team"}
	case "cafe":
		return []string{"Order a latte", "Ask for oat milk", "Choose a size"}
	default:
		return []string{"Introduce yourself", "Ask a follow-up question", "Describe your goal"}
	}
}
