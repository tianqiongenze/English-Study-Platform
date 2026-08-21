package main

// JWT + 鉴权单测(jwt.go / main.go requireAdmin)。
// CLC 铁律:测真实执行路径——用 stdlib 真实签发 RS256 JWT 走 verifyToken 全链路
// (decode→base64→json claims→exp 检查→RSA 验签),禁裸设 X-Role 头绕过。
// admin-go go.mod 无 gojwt 依赖(代码声明 stdlib only),故用 crypto/rsa 手签保持零依赖。

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testRSAKey 单测专用 RSA 密钥对(仅测试域,每次运行固定生成)。
var testRSAKey *rsa.PrivateKey

func testPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	if testRSAKey == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		testRSAKey = k
	}
	return testRSAKey
}

// testPublicKeyPEM 返回测试私钥对应的 PEM 公钥串(供 loadPubKey)。
func testPublicKeyPEM(t *testing.T) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&testPrivateKey(t).PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// signTestJWT 用 stdlib 手签 RS256 JWT:header.payload.signature。
// 仿 jwt.go:69 的签名构造(sha256(parts[0]+"."+parts[1]) + RSA PKCS1v15)。
func signTestJWT(t *testing.T, sub, role string, exp int64) string {
	t.Helper()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	claims := map[string]any{"sub": sub, "role": role, "exp": exp}
	cb, _ := json.Marshal(claims)
	hEnc := base64.RawURLEncoding.EncodeToString(hb)
	cEnc := base64.RawURLEncoding.EncodeToString(cb)
	signingInput := hEnc + "." + cEnc
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, testPrivateKey(t), crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestLoadPubKey(t *testing.T) {
	// happy:真实 PEM 公钥可加载
	pub, err := loadPubKey(testPublicKeyPEM(t))
	if err != nil {
		t.Fatalf("loadPubKey(valid PEM): %v", err)
	}
	if pub == nil {
		t.Fatal("loadPubKey returned nil pub without error")
	}
	if pub.N == nil || pub.E == 0 {
		t.Fatal("loadPubKey returned unusable key (N/E zero)")
	}
}

func TestLoadPubKey_Errors(t *testing.T) {
	cases := []struct {
		name, pemStr string
	}{
		{"empty", ""},
		{"garbage", "not-a-pem-block"},
		{"private_key_not_public", func() string {
			der := x509.MarshalPKCS1PrivateKey(testPrivateKey(t))
			return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}))
		}()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pub, err := loadPubKey(c.pemStr)
			if err == nil {
				t.Errorf("loadPubKey(%s): expected error, got nil (pub=%v)", c.name, pub)
			}
			if pub != nil {
				t.Errorf("loadPubKey(%s): expected nil pub on error", c.name)
			}
		})
	}
}

func TestVerifyToken_Valid(t *testing.T) {
	pub := &testPrivateKey(t).PublicKey
	uid := int64(1001)
	tok := signTestJWT(t, "1001", "admin", time.Now().Add(1*time.Hour).Unix())

	gotUID, gotRole, err := verifyToken(tok, pub)
	if err != nil {
		t.Fatalf("verifyToken(valid): %v", err)
	}
	if gotUID != uid {
		t.Errorf("uid: got %d, want %d", gotUID, uid)
	}
	if gotRole != "admin" {
		t.Errorf("role: got %q, want admin", gotRole)
	}
}

func TestVerifyToken_Errors(t *testing.T) {
	pub := &testPrivateKey(t).PublicKey
	valid := signTestJWT(t, "5", "admin", time.Now().Add(1*time.Hour).Unix())
	// 用不同密钥签的 token(签名不符)
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	hb, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	cb, _ := json.Marshal(map[string]any{"sub": "5", "role": "admin", "exp": time.Now().Add(1 * time.Hour).Unix()})
	si := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	d := sha256.Sum256([]byte(si))
	badSig, _ := rsa.SignPKCS1v15(rand.Reader, otherKey, crypto.SHA256, d[:])
	badSigTok := si + "." + base64.RawURLEncoding.EncodeToString(badSig)

	// 过期 token(exp + 30s 容忍外)
	expired := signTestJWT(t, "5", "admin", time.Now().Add(-2*time.Minute).Unix())

	cases := []struct {
		name, tok string
		key        *rsa.PublicKey
	}{
		{"empty_token", "", pub},
		{"nil_key", valid, nil},
		{"malformed_parts", "not.three.parts", pub},
		{"two_parts", "only.two", pub},
		{"bad_sig_encoding", "header.payload.!!!not-base64!!!", pub},
		{"signature_mismatch", badSigTok, pub},
		{"expired", expired, pub},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := verifyToken(c.tok, c.key)
			if err == nil {
				t.Errorf("verifyToken(%s): expected error, got nil", c.name)
			}
		})
	}
}

func TestTokenFromRequest(t *testing.T) {
	// Bearer 优先
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok-abc")
	if got := tokenFromRequest(r); got != "tok-abc" {
		t.Errorf("bearer: got %q, want tok-abc", got)
	}
	// 无 Authorization 但有 cookie
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.AddCookie(&http.Cookie{Name: "esp_token", Value: "cookie-tok"})
	if got := tokenFromRequest(r2); got != "cookie-tok" {
		t.Errorf("cookie fallback: got %q, want cookie-tok", got)
	}
	// Bearer 优先于 cookie(两者都有取 Bearer)
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.Header.Set("Authorization", "Bearer bearer-wins")
	r3.AddCookie(&http.Cookie{Name: "esp_token", Value: "cookie-loses"})
	if got := tokenFromRequest(r3); got != "bearer-wins" {
		t.Errorf("bearer precedence: got %q, want bearer-wins", got)
	}
	// 都没有 → 空串
	r4 := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := tokenFromRequest(r4); got != "" {
		t.Errorf("none: got %q, want empty", got)
	}
	// 空 cookie 值 → 当无
	r5 := httptest.NewRequest(http.MethodGet, "/", nil)
	r5.AddCookie(&http.Cookie{Name: "esp_token", Value: ""})
	if got := tokenFromRequest(r5); got != "" {
		t.Errorf("empty cookie: got %q, want empty", got)
	}
}

// TestRequireAdmin 拉真实 mux 链(requireAdmin 包裹一个 sentinel handler),
// 验证 role=admin 放行、非 admin/无 token/坏 token 拒绝 403。
func TestRequireAdmin(t *testing.T) {
	srv := &Server{pubKey: &testPrivateKey(t).PublicKey}
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	protected := srv.requireAdmin(sentinel)

	mkReq := func(tok string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/admin/services", nil)
		if tok != "" {
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		return r
	}

	validAdmin := signTestJWT(t, "1001", "admin", time.Now().Add(1*time.Hour).Unix())
	validLearner := signTestJWT(t, "2002", "learner", time.Now().Add(1*time.Hour).Unix())

	cases := []struct {
		name     string
		tok      string
		wantCode int
	}{
		{"valid_admin", validAdmin, 200},
		{"valid_learner_rejected", validLearner, 403}, // role != admin
		{"no_token", "", 403},
		{"bad_token", "garbage.token.xxx", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			protected.ServeHTTP(w, mkReq(c.tok))
			if w.Code != c.wantCode {
				t.Errorf("%s: status got %d, want %d (body=%s)", c.name, w.Code, c.wantCode, w.Body.String())
			}
			if c.wantCode == 200 && !strings.Contains(w.Body.String(), `"ok":true`) {
				t.Errorf("%s: expected sentinel body, got %q", c.name, w.Body.String())
			}
		})
	}
}

// 防止 fmt 未用导致编译失败(signTestJWT 用 fmt? 否;但 _ 引用以避免 unused import 警告)
var _ = fmt.Sprintf
var _ = strconv.Itoa
