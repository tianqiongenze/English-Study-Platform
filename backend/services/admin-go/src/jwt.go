package main

// 最小化 RS256 JWT 校验(stdlib only):验签 + sub/role/exp 解析,零外部依赖。
// 与 gateway-go 共用 esp-rsa-jwt-verify 公钥(env JWT_RSA_PUBLIC_KEY_PEM)。
// 管理门户额外要求 role==admin,故在此独立实现而非复用 jwt.Validator(其只取 subject)。

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
}

// loadPubKey 从 PEM 加载 RSA 公钥。
func loadPubKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}
	return rsaPub, nil
}

// verifyToken 验签 RS256 JWT 并返回 uid + role。空/非法/过期/签名不符返回错误。
func verifyToken(tokenStr string, pub *rsa.PublicKey) (int64, string, error) {
	if tokenStr == "" || pub == nil {
		return 0, "", fmt.Errorf("empty token or key")
	}
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return 0, "", fmt.Errorf("malformed token")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, "", fmt.Errorf("bad signature encoding")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, "", fmt.Errorf("bad payload encoding")
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return 0, "", fmt.Errorf("bad claims: %w", err)
	}
	// 容忍 30s 时钟偏移(与 gateway-jwt.WithLeeway 一致)
	if c.Exp > 0 && c.Exp+30 < time.Now().Unix() {
		return 0, "", fmt.Errorf("token expired")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return 0, "", fmt.Errorf("signature mismatch")
	}
	var uid int64
	fmt.Sscanf(c.Sub, "%d", &uid)
	return uid, c.Role, nil
}

// tokenFromRequest 取 JWT:Authorization Bearer 优先,回落 esp_token cookie。
func tokenFromRequest(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		return strings.TrimPrefix(authz, "Bearer ")
	}
	if c, err := r.Cookie("esp_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}
