package tts

import (
	"context"
	"strings"
	"testing"
)

func TestSenseEmotion(t *testing.T) {
	cases := map[string]string{
		"I feel so tired today":  "sad",
		"Great job, amazing!":    "encouraging", // "great job" 命中鼓励词(先于 positive)
		"You are doing great":    "happy",
		"The cat sat":            "neutral",
		"":                       "neutral",
	}
	for in, want := range cases {
		if got := SenseEmotion(in); got != want {
			t.Errorf("SenseEmotion(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuildVisemes(t *testing.T) {
	v := BuildVisemes("aba")
	if len(v) != 3 {
		t.Fatalf("want 3 visemes got %d", len(v))
	}
	if v[0].ID != "AA" || v[1].ID != "MBP" || v[2].ID != "AA" {
		t.Errorf("ids: %+v", v)
	}
	if v[0].Start != 0 || v[0].End != 60 || v[2].Start != 120 {
		t.Errorf("timeline: %+v", v)
	}
	// 非字母不产生口型
	if n := len(BuildVisemes("!!! ...")); n != 0 {
		t.Errorf("punct produced %d visemes", n)
	}
}

func TestMockSynthesize(t *testing.T) {
	r := (&MockProvider{}).Synthesize(context.Background(), "you are doing great")
	if r.Emotion != "happy" {
		t.Errorf("emotion=%q", r.Emotion)
	}
	if !strings.HasPrefix(r.Audio, "dHRz") { // base64("tts:")
		t.Errorf("audio prefix wrong")
	}
	if len(r.Visemes) == 0 {
		t.Errorf("no visemes")
	}
}
