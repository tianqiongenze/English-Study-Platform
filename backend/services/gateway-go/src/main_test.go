package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	espjwt "esp/gateway-go/src/jwt"
)

// G-04 修复回归:newID 必须并发唯一(原 UnixMilli 毫秒碰撞)。并发 1000 次无重复。
func TestNewIDConcurrentUnique(t *testing.T) {
	const total = 1000
	const goroutines = 8
	perG := total / goroutines

	ids := make(chan string, total)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				ids <- newID()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]struct{}, total)
	count := 0
	for id := range ids {
		count++
		if !strings.HasPrefix(id, "sess_") {
			t.Fatalf("id %q missing sess_ prefix", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %s", id)
		}
		seen[id] = struct{}{}
	}
	if count != total {
		t.Fatalf("expected %d ids, got %d", total, count)
	}
}

// newID 格式回归:sess_<unixms>_<8hex>。
func TestNewIDFormat(t *testing.T) {
	id := newID()
	if !strings.HasPrefix(id, "sess_") {
		t.Fatalf("id %q missing sess_ prefix", id)
	}
	parts := strings.Split(id, "_")
	if len(parts) != 3 {
		t.Fatalf("id %q should have 3 underscore parts (sess_<ms>_<hex>), got %d parts: %v", id, len(parts), parts)
	}
	hex := parts[2]
	if len(hex) != 8 {
		t.Fatalf("random hex part should be 8 chars, got %q (%d)", hex, len(hex))
	}
}

// tokenFromRequest:Bearer 优先;回落 cookie;都没有返回空串。
func TestTokenFromRequest_BearerPriority(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	if got := tokenFromRequest(r); got != "mytoken123" {
		t.Errorf("Bearer token: got %q, want %q", got, "mytoken123")
	}
}

func TestTokenFromRequest_CookieFallback(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "esp_token", Value: "cookietoken"})
	if got := tokenFromRequest(r); got != "cookietoken" {
		t.Errorf("cookie fallback: got %q, want %q", got, "cookietoken")
	}
}

func TestTokenFromRequest_Empty(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	if got := tokenFromRequest(r); got != "" {
		t.Errorf("no auth: got %q, want empty", got)
	}
}

func TestTokenFromRequest_BearerOverridesCookie(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer bearer_val")
	r.AddCookie(&http.Cookie{Name: "esp_token", Value: "cookie_val"})
	if got := tokenFromRequest(r); got != "bearer_val" {
		t.Errorf("Bearer should override cookie: got %q, want %q", got, "bearer_val")
	}
}

func TestTokenFromRequest_BearerPrefixOnly(t *testing.T) {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Basic abc123") // not Bearer
	if got := tokenFromRequest(r); got != "" {
		t.Errorf("non-Bearer auth should return empty, got %q", got)
	}
}

// env/envInt 委托到 config.Getenv,验证不 panic 即可。
func TestEnvAndEnvInt(t *testing.T) {
	if v := env("NONEXISTENT_KEY_XYZ", "default_val"); v != "default_val" {
		t.Errorf("env default: got %q", v)
	}
	if v := envInt("NONEXISTENT_KEY_XYZ", 42); v != 42 {
		t.Errorf("envInt default: got %d", v)
	}
}

// writeJSON:验证 Content-Type 和序列化。
func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, map[string]any{"ok": true})
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("body: %q", w.Body.String())
	}
}

func TestWriteJSON_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, nil)
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q", ct)
	}
}

// parseUserID:通过 Server jwt 字段直接调用。
func TestParseUserID_EmptyToken(t *testing.T) {
	// Server.parseUserID 只是委托 jwt.ParseUserID,构造一个带 nil jwt 的 Server 测试空 token
	s := &Server{}
	if uid := s.parseUserID(""); uid != 0 {
		t.Errorf("empty token: got %d, want 0", uid)
	}
}

// parseUserID:委托给 jwt.Validator,用已有的 validator 测试。
func TestParseUserID_InvalidToken(t *testing.T) {
	v := jwtTestValidator(t)
	s := &Server{jwt: v}
	if uid := s.parseUserID("not-a-jwt"); uid != 0 {
		t.Errorf("invalid token: got %d, want 0", uid)
	}
}

// jwtTestValidator creates a Validator from an auto-generated RSA key pair for testing.
func jwtTestValidator(t *testing.T) *espjwt.Validator {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	v, err := espjwt.NewValidatorFromPEM(pubPEM)
	if err != nil {
		t.Fatalf("NewValidatorFromPEM: %v", err)
	}
	return v
}

// userIDFromRequest:委托给 parseUserID + tokenFromRequest。
func TestUserIDFromRequest_NoAuth(t *testing.T) {
	v := jwtTestValidator(t)
	s := &Server{jwt: v}
	r, _ := http.NewRequest("GET", "/", nil)
	if uid := s.userIDFromRequest(r); uid != 0 {
		t.Errorf("no auth: got %d, want 0", uid)
	}
}

// TestServerStruct_Fields 确保 Server 结构体可构建(编译时类型检查)。
func TestServerStruct_Fields(t *testing.T) {
	s := &Server{}
	if s.jwt != nil || s.rdb != nil {
		t.Error("new Server should have nil fields")
	}
}
