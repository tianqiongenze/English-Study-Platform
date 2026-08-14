package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// loadTestPrivateKey 加载测试用 RSA 私钥(模拟 BFF 签发)。
func loadTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	data, err := os.ReadFile("testdata/private.pem")
	if err != nil {
		t.Fatalf("read private.pem: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("decode PEM failed")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	return key.(*rsa.PrivateKey)
}

func TestParseUserID_ValidToken(t *testing.T) {
	pubPEM, _ := os.ReadFile("testdata/public.pem")
	v, err := NewValidatorFromPEM(string(pubPEM))
	if err != nil {
		t.Fatalf("NewValidatorFromPEM: %v", err)
	}

	privKey := loadTestPrivateKey(t)
	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject:   "42",
		IssuedAt:  gojwt.NewNumericDate(time.Now()),
		ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	tokenStr, _ := token.SignedString(privKey)

	uid := v.ParseUserID(tokenStr)
	if uid != 42 {
		t.Errorf("expected 42, got %d", uid)
	}
}

func TestParseUserID_TamperedToken(t *testing.T) {
	pubPEM, _ := os.ReadFile("testdata/public.pem")
	v, _ := NewValidatorFromPEM(string(pubPEM))

	privKey := loadTestPrivateKey(t)
	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject: "1",
	})
	tokenStr, _ := token.SignedString(privKey)

	// Tamper with the token
	tampered := tokenStr + "x"
	uid := v.ParseUserID(tampered)
	if uid != 0 {
		t.Errorf("tampered token should return 0, got %d", uid)
	}
}

func TestParseUserID_DifferentKeyPair(t *testing.T) {
	pubPEM, _ := os.ReadFile("testdata/public.pem")
	v, _ := NewValidatorFromPEM(string(pubPEM))

	// Generate a different RSA keypair using crypto/rand
	otherPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	token := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.RegisteredClaims{
		Subject: "99",
	})
	tokenStr, _ := token.SignedString(otherPriv)

	uid := v.ParseUserID(tokenStr)
	if uid != 0 {
		t.Errorf("token signed by different key should return 0, got %d", uid)
	}
}

func TestParseUserID_EmptyAndGarbageToken(t *testing.T) {
	pubPEM, _ := os.ReadFile("testdata/public.pem")
	v, _ := NewValidatorFromPEM(string(pubPEM))

	if uid := v.ParseUserID(""); uid != 0 {
		t.Errorf("empty token should return 0, got %d", uid)
	}
	if uid := v.ParseUserID("garbage"); uid != 0 {
		t.Errorf("garbage token should return 0, got %d", uid)
	}
}
