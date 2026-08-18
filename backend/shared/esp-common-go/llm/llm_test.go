package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSendsAuthAndParses(t *testing.T) {
	var gotKey, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("authorization")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"content":[{"type":"text","text":"{\"main_clause\":\"x\"}"}]}`))
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", AuthToken: "t", Model: "m", BaseURL: srv.URL, hc: srv.Client()}
	out, err := c.Chat(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "k" || gotAuth != "Bearer t" {
		t.Errorf("auth headers: key=%q auth=%q", gotKey, gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"m"`) {
		t.Errorf("body=%s", gotBody)
	}
	if out != `{"main_clause":"x"}` {
		t.Errorf("out=%s", out)
	}
}

func TestChatRejectsMissingKey(t *testing.T) {
	c := &Client{BaseURL: "http://example.invalid"}
	if _, err := c.Chat(context.Background(), "s", "u"); err == nil {
		t.Fatal("want error when no key/token")
	}
}

func TestChatNon200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &Client{APIKey: "k", BaseURL: srv.URL, hc: srv.Client()}
	if _, err := c.Chat(context.Background(), "s", "u"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("want 502 error, got %v", err)
	}
}

func TestExtractJSON(t *testing.T) {
	m, err := ExtractJSON("```json\n{\"a\":1}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if m["a"].(float64) != 1 {
		t.Errorf("want 1 got %v", m["a"])
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Fatal("want error")
	}
}