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

// gamifTestKey 单测专用 RSA 密钥对(仅测试域)。P1-30 requireUID 改为零信任 JWT 自校验后,
// 单测必须用真实签发的 JWT 走真实执行路径,禁裸设 X-User-Id 头。
var gamifTestKey *rsa.PrivateKey

func gamifTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	if gamifTestKey == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		gamifTestKey = k
	}
	return gamifTestKey
}

func gamifTestValidator(t *testing.T) *middleware.JWTValidator {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&gamifTestPrivateKey(t).PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	v, err := middleware.NewJWTValidatorFromPEM(pemStr)
	if err != nil {
		t.Fatalf("validator: %v", err)
	}
	return v
}

// gamifNewTestServer 构造带测试 validator 的 Server(nil DB/Redis)。
func gamifNewTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{jwt: gamifTestValidator(t)}
}

// gamifSignJWT 签发 subject=uid 的 RS256 JWT(Bearer token)。uid<=0 返回空串。
func gamifSignJWT(t *testing.T, uid int64) string {
	t.Helper()
	if uid <= 0 {
		return ""
	}
	claims := gojwt.MapClaims{
		"sub":   strconv.FormatInt(uid, 10),
		"role":  "learner",
		"email": "u@example.com",
		"name":  "U",
		"typ":   "access",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(gamifTestPrivateKey(t))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}
