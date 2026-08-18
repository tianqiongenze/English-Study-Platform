package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"esp/conversation-go/src/ws"
)

func TestHTTPProviderSynthesizeOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ttsReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode req: %v", err)
		}
		if req.Text != "hello" {
			t.Errorf("text = %q, want hello", req.Text)
		}
		out := ttsResp{
			Audio:   base64.StdEncoding.EncodeToString([]byte("audio-bytes")),
			Emotion: "happy",
			Visemes: []ws.Viseme{{ID: "AA", Start: 0, End: 200}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	res := p.Synthesize(context.Background(), "hello")
	if res.Emotion != "happy" {
		t.Errorf("Emotion = %q, want happy", res.Emotion)
	}
	if len(res.Visemes) != 1 || res.Visemes[0].ID != "AA" {
		t.Errorf("Visemes = %+v, want [AA]", res.Visemes)
	}
	wantAudio := base64.StdEncoding.EncodeToString([]byte("audio-bytes"))
	if res.Audio != wantAudio {
		t.Errorf("Audio = %q, want %q", res.Audio, wantAudio)
	}
}

func TestHTTPProviderSynthesizeNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	res := p.Synthesize(context.Background(), "x")
	if res.Emotion != "neutral" {
		t.Errorf("Emotion on 502 = %q, want neutral", res.Emotion)
	}
}

func TestHTTPProviderSynthesizeInvalidAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		out := ttsResp{Emotion: "sad", Audio: "not-base64!!"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	res := p.Synthesize(context.Background(), "y")
	if res.Emotion != "neutral" {
		t.Errorf("Emotion on invalid audio = %q, want neutral", res.Emotion)
	}
	if res.Audio != "" {
		t.Errorf("Audio = %q, want empty", res.Audio)
	}
}

func TestHTTPProviderSynthesizeEmptyEmotionDefaultsNeutral(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		out := ttsResp{Audio: base64.StdEncoding.EncodeToString([]byte("z"))}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	if res := p.Synthesize(context.Background(), "w"); res.Emotion != "neutral" {
		t.Errorf("Emotion = %q, want neutral default", res.Emotion)
	}
}