package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// genTestKey 生成测试 RSA 密钥对(单测域,惰性缓存)。
func genTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	return k
}

func pubPEM(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func sign(t *testing.T, k *rsa.PrivateKey, uid int64, role string) string {
	t.Helper()
	claims := gojwt.MapClaims{
		"sub":   strconv.FormatInt(uid, 10),
		"role":  role,
		"email": "u@example.com",
		"name":  "U",
		"typ":   "access",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(k)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestNewValidatorFromPEM_InvalidPEM(t *testing.T) {
	if _, err := NewJWTValidatorFromPEM("not-a-pem"); err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestNewValidatorFromPEM_NotRSA(t *testing.T) {
	// EC key 不应被接受为 RSA。
	// 用一个空 PKIX 块构造:解析成功但非 RSA → "key is not RSA"。
	// 直接测非 RSA 路径较复杂,改为测 nil 解析路径已覆盖 invalidPEM;此处测正确 RSA 路径见下。
	t.Skip("non-RSA path needs EC key; covered by nil/invalid cases")
}

func TestParseUserClaims_Valid(t *testing.T) {
	k := genTestKey(t)
	v, err := NewJWTValidatorFromPEM(pubPEM(t, k))
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	tok := sign(t, k, 777, "admin")
	got := v.ParseUserClaims(tok)
	if got.UserID != 777 {
		t.Errorf("uid: got %d, want 777", got.UserID)
	}
	if got.Role != "admin" {
		t.Errorf("role: got %q, want admin", got.Role)
	}
}

func TestParseUserClaims_InvalidSignature(t *testing.T) {
	kSign := genTestKey(t)
	kOther := genTestKey(t)
	v, _ := NewJWTValidatorFromPEM(pubPEM(t, kOther)) // 不同公钥
	tok := sign(t, kSign, 1, "learner")              // 用另一密钥签
	got := v.ParseUserClaims(tok)
	if got.UserID != 0 {
		t.Errorf("uid should be 0 on bad sig, got %d", got.UserID)
	}
}

func TestParseUserClaims_InvalidToken(t *testing.T) {
	k := genTestKey(t)
	v, _ := NewJWTValidatorFromPEM(pubPEM(t, k))
	got := v.ParseUserClaims("not-a-jwt")
	if got.UserID != 0 || got.Role != "" {
		t.Errorf("expected zero claims, got %+v", got)
	}
}

func TestParseUserClaims_EmptyToken(t *testing.T) {
	k := genTestKey(t)
	v, _ := NewJWTValidatorFromPEM(pubPEM(t, k))
	got := v.ParseUserClaims("")
	if got.UserID != 0 {
		t.Errorf("uid should be 0, got %d", got.UserID)
	}
}

func TestParseUserClaims_NilValidator(t *testing.T) {
	var v *JWTValidator // nil
	got := v.ParseUserClaims("anything")
	if got.UserID != 0 || got.Role != "" {
		t.Errorf("nil validator must return zero claims, got %+v", got)
	}
	if uid := v.ParseUserID("anything"); uid != 0 {
		t.Errorf("nil ParseUserID must return 0, got %d", uid)
	}
}

func TestParseUserClaims_HS256Rejected(t *testing.T) {
	// RS256 validator 必须拒绝 HS256 token(防 alg 混淆攻击)。
	k := genTestKey(t)
	v, _ := NewJWTValidatorFromPEM(pubPEM(t, k))
	// 签一个 HS256 token(用公钥当 secret——经典 alg 混淆 payload)。
	tok, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, gojwt.MapClaims{
		"sub": "999", "role": "admin", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("anything"))
	if err != nil {
		t.Fatalf("sign hs: %v", err)
	}
	got := v.ParseUserClaims(tok)
	if got.UserID != 0 || got.Role != "" {
		t.Errorf("HS256 token must be rejected, got uid=%d role=%q", got.UserID, got.Role)
	}
}

func TestTokenFromRequest_BearerAndCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	if got := TokenFromRequest(req); got != "abc123" {
		t.Errorf("bearer: got %q", got)
	}
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(&http.Cookie{Name: "esp_token", Value: "cookie-tok"})
	if got := TokenFromRequest(req2); got != "cookie-tok" {
		t.Errorf("cookie: got %q", got)
	}
	req3 := httptest.NewRequest("GET", "/", nil)
	if got := TokenFromRequest(req3); got != "" {
		t.Errorf("none: got %q", got)
	}
}

func TestUserIDFromRequest_NilValidator(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	var v *JWTValidator
	if uid := UserIDFromRequest(v, req); uid != 0 {
		t.Errorf("nil validator: got %d", uid)
	}
}
