package dialog

import (
	"context"
	"testing"
	"time"

	"esp/conversation-go/src/llm"
	"esp/conversation-go/src/session"
	"esp/conversation-go/src/ws"
)

func TestEngine_Process_ReplyAndTurn(t *testing.T) {
	e := &Engine{LLM: &llm.MockProvider{}, HintIdle: 8 * time.Second}
	s := &session.Session{ID: "s", Scenario: "airport", System: "you are staff", Difficulty: 3}
	res, err := e.Process(context.Background(), s, "I'd like to check in.")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if res.Turn.Role != "assistant" || res.Turn.Content == "" {
		t.Fatalf("expected assistant turn, got %+v", res.Turn)
	}
	found := false
	for _, f := range res.Frames {
		if f.Type == ws.TypeAIReply && f.Text != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected an ai_reply frame")
	}
}

func TestEngine_Process_CorrectsSubjectVerbError(t *testing.T) {
	e := &Engine{LLM: &llm.MockProvider{}, HintIdle: 8 * time.Second}
	s := &session.Session{ID: "s", Scenario: "cafe", System: "barista", Difficulty: 3}
	res, err := e.Process(context.Background(), s, "I wants a coffee")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	var corr *ws.Frame
	for i := range res.Frames {
		if res.Frames[i].Type == ws.TypeCorrection {
			corr = &res.Frames[i]
			break
		}
	}
	if corr == nil {
		t.Fatal("expected a correction frame for 'I wants'")
	}
	if corr.Original != "i wants" && corr.Original != "I wants" {
		t.Fatalf("unexpected original: %q", corr.Original)
	}
	if corr.Corrected == "" {
		t.Fatal("expected non-empty corrected")
	}
}

func TestEngine_Process_NoCorrectionForCorrectText(t *testing.T) {
	e := &Engine{LLM: &llm.MockProvider{}, HintIdle: 8 * time.Second}
	s := &session.Session{ID: "s", Scenario: "cafe", System: "barista", Difficulty: 3}
	res, err := e.Process(context.Background(), s, "I want a coffee")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	for _, f := range res.Frames {
		if f.Type == ws.TypeCorrection {
			t.Fatalf("did not expect correction, got %+v", f)
		}
	}
}

func TestEngine_Hint_ByScenario(t *testing.T) {
	e := &Engine{LLM: &llm.MockProvider{}}
	for _, sc := range []string{"airport", "interview", "cafe", "unknown"} {
		f := e.Hint(&session.Session{Scenario: sc})
		if f.Type != ws.TypeHint {
			t.Fatalf("%s: expected hint frame", sc)
		}
		if len(f.Options) < 2 || len(f.Options) > 4 {
			t.Fatalf("%s: expected 2~4 options, got %d", sc, len(f.Options))
		}
	}
}
