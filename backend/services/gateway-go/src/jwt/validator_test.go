package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// 测试密钥对进程内一次性生成,不依赖 testdata/*.pem 文件
// (坑:.gitignore 的 **/*.pem 会把测试密钥排除在仓库外,CI checkout 后缺失导致 PEM decode 失败)。
var (
	testKeysOnce sync.Once
	testPriv     *rsa.PrivateKey
	testPubPEM   string
	testErr      error
)

func testKeyPair(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	testKeysOnce.Do(func() {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			testErr = fmt.Errorf("generate test RSA key: %w", err)
			return
		}
		testPriv = priv
		pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
		if err != nil {
			testErr = fmt.Errorf("marshal public key: %w", err)
			return
		}
		testPubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	})
	if testErr != nil {
		t.Fatalf("init test keypair: %v", testErr)
	}
	return testPriv, testPubPEM
}

// newTestValidator 构建以自生成密钥对为信任锚的 Validator(模拟 BFF 签发 + gateway 验签)。
func newTestValidator(t *testing.T) *Validator {
	t.Helper()
	_, pubPEM := testKeyPair(t)
	v, err := NewValidatorFromPEM(pubPEM)
	if err != nil {
		t.Fatalf("NewValidatorFromPEM: %v", err)
	}
	return v
}

func TestParseUserID_ValidToken(t *testing.T) {
	v := newTestValidator(t)
	privKey, _ := testKeyPair(t)

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject:   "42",
		IssuedAt:  gojwt.NewNumericDate(time.Now()),
		ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	tokenStr, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if uid := v.ParseUserID(tokenStr); uid != 42 {
		t.Errorf("expected 42, got %d", uid)
	}
}

func TestParseUserID_TamperedToken(t *testing.T) {
	v := newTestValidator(t)
	privKey, _ := testKeyPair(t)

	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject: "1",
	})
	tokenStr, _ := token.SignedString(privKey)

	// Tamper with the token
	if uid := v.ParseUserID(tokenStr + "x"); uid != 0 {
		t.Errorf("tampered token should return 0, got %d", uid)
	}
}

func TestParseUserID_DifferentKeyPair(t *testing.T) {
	v := newTestValidator(t)

	otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject: "99",
	})
	tokenStr, _ := token.SignedString(otherPriv)

	if uid := v.ParseUserID(tokenStr); uid != 0 {
		t.Errorf("token signed by different key should return 0, got %d", uid)
	}
}

func TestParseUserID_EmptyAndGarbageToken(t *testing.T) {
	v := newTestValidator(t)

	if uid := v.ParseUserID(""); uid != 0 {
		t.Errorf("empty token should return 0, got %d", uid)
	}
	if uid := v.ParseUserID("garbage"); uid != 0 {
		t.Errorf("garbage token should return 0, got %d", uid)
	}
}
