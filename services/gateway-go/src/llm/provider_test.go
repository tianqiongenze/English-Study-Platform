package llm

import (
	"context"
	"testing"
)

func TestMockProvider_RespondsToCheckInScenario(t *testing.T) {
	p := &MockProvider{}
	got, err := p.Chat(context.Background(), []Message{
		{Role: "system", Content: "you are airport staff"},
		{Role: "user", Content: "I'd like to check in for my flight"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty reply")
	}
}

func TestMockProvider_HandlesEmptyInput(t *testing.T) {
	p := &MockProvider{}
	got, err := p.Chat(context.Background(), []Message{
		{Role: "system", Content: "tutor"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatal("expected greeting on empty input")
	}
}

func TestNew_ReturnsMockWhenModeMock(t *testing.T) {
	t.Setenv("LLM_MODE", "mock")
	p := New()
	if _, ok := p.(*MockProvider); !ok {
		t.Fatalf("expected MockProvider, got %T", p)
	}
}
