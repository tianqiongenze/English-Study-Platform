package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strconv"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"esp/common-go/middleware"
)

// testRSAKey 是单测专用的 RSA 密钥对(进程内惰性生成,仅测试域)。
// P1-30 requireAuth 改为零信任 JWT 自校验后,单测必须用真实签发的 JWT 走真实执行路径,
// 不能再裸设 X-User-Id 头(否则 JWT validator 返 0 走 401,违背 CLC 铁律"追踪执行路径")。
var testRSAKey *rsa.PrivateKey

func testPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	if testRSAKey == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("test keygen: %v", err)
		}
		testRSAKey = k
	}
	return testRSAKey
}

// testValidator 用测试公钥构建 JWTValidator(同款 RS256 验签路径)。
func testValidator(t *testing.T) *middleware.JWTValidator {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&testPrivateKey(t).PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	v, err := middleware.NewJWTValidatorFromPEM(pemStr)
	if err != nil {
		t.Fatalf("test validator: %v", err)
	}
	return v
}

// signTestJWT 用测试私钥签发一个 subject=uid 的 RS256 JWT(Bearer token 形态)。
// uid<=0 返回空串(模拟生产:无有效 subject,ParseUserID 返 0)。
func signTestJWT(t *testing.T, uid int64) string {
	t.Helper()
	if uid <= 0 {
		return ""
	}
	claims := gojwt.RegisteredClaims{
		Subject:   strconv.FormatInt(uid, 10),
		ExpiresAt: gojwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  gojwt.NewNumericDate(time.Now()),
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(testPrivateKey(t))
	if err != nil {
		t.Fatalf("sign test jwt: %v", err)
	}
	return s
}
