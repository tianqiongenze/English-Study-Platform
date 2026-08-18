package asr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProviderRecognizeOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		var req asrReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode req: %v", err)
		}
		if len(req.Chunks) != 2 || req.Chunks[0] != "abc" {
			t.Errorf("chunks = %v, want [abc def]", req.Chunks)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(asrResp{Text: "hello world"})
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	got := p.Recognize([]string{"abc", "def"})
	if got != "hello world" {
		t.Errorf("Recognize = %q, want %q", got, "hello world")
	}
}

func TestHTTPProviderRecognizeEmptyChunks(t *testing.T) {
	p := &HTTPProvider{Endpoint: "http://unused.invalid", Client: &http.Client{}}
	if got := p.Recognize(nil); got != "" {
		t.Errorf("Recognize(nil) = %q, want empty", got)
	}
}

func TestHTTPProviderRecognizeNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	if got := p.Recognize([]string{strings.Repeat("a", 8)}); got != "" {
		t.Errorf("Recognize on 500 = %q, want empty", got)
	}
}

func TestHTTPProviderRecognizeBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	p := &HTTPProvider{Endpoint: srv.URL, Client: srv.Client()}
	if got := p.Recognize([]string{strings.Repeat("b", 8)}); got != "" {
		t.Errorf("Recognize on bad json = %q, want empty", got)
	}
}

var _ = context.Background