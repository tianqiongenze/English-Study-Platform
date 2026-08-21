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

// contentTestKey 单测专用 RSA 密钥对(仅测试域)。P1-30 requireAdmin/userID/roleOf 改为
// 零信任 JWT 自校验后,单测必须用真实签发的 JWT 走真实执行路径,禁裸设 X-User-Id/X-Role 头。
var contentTestKey *rsa.PrivateKey

func contentTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	if contentTestKey == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("keygen: %v", err)
		}
		contentTestKey = k
	}
	return contentTestKey
}

func contentTestValidator(t *testing.T) *middleware.JWTValidator {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&contentTestPrivateKey(t).PublicKey)
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

// contentNewTestServer 构造带测试 validator 的 Server(nil DB)。
func contentNewTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{jwt: contentTestValidator(t)}
}

// contentSignJWT 签发 subject=uid + role 的 RS256 JWT(Bearer token)。
// uid<=0 返回空串(模拟无有效 subject → ParseUserID 返 0)。
func contentSignJWT(t *testing.T, uid int64, role string) string {
	t.Helper()
	if uid <= 0 {
		return ""
	}
	return contentSignJWTWithSubject(t, strconv.FormatInt(uid, 10), role)
}

// contentSignJWTNeg 签发 subject 为负数(如 "-1")的 token,用于断言负 uid 被拒。
// 签名仍合法(测试私钥),验证过签名但 ParseUserID 返负数 → 调用方按 uid<=0 走 401。
func contentSignJWTNeg(t *testing.T, uid int64) string {
	t.Helper()
	return contentSignJWTWithSubject(t, strconv.FormatInt(uid, 10), "learner")
}

func contentSignJWTWithSubject(t *testing.T, subject, role string) string {
	t.Helper()
	claims := gojwt.MapClaims{
		"sub":   subject,
		"role":  role,
		"email": "u@example.com",
		"name":  "U",
		"typ":   "access",
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(contentTestPrivateKey(t))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}
