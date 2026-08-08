package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Validator 校验 RS256 JWT(公钥验签,ADR-12)。
// BFF 持私钥签发;gateway-go 持公钥校验。
// 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM)。
// JWKS 动态获取为后续增强(当前走 PEM 直校验)。
type Validator struct {
	pubKey *rsa.PublicKey
}

// NewValidatorFromEnv 从环境变量 JWT_RSA_PUBLIC_KEY_PEM 加载公钥。
func NewValidatorFromEnv() (*Validator, error) {
	pemStr := os.Getenv("JWT_RSA_PUBLIC_KEY_PEM")
	if pemStr == "" {
		return nil, fmt.Errorf("JWT_RSA_PUBLIC_KEY_PEM env not set")
	}
	return NewValidatorFromPEM(pemStr)
}

// NewValidatorFromPEM 从 PEM 字符串加载公钥。
func NewValidatorFromPEM(pemStr string) (*Validator, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}
	return &Validator{pubKey: rsaPub}, nil
}

// ParseUserID 解析 JWT(RS256 公钥验签)取 userID(subject)。
// 空/非法/无效/签名不符 token 返回 0。
func (v *Validator) ParseUserID(tokenStr string) int64 {
	if tokenStr == "" {
		return 0
	}
	claims := &gojwt.RegisteredClaims{}
	tok, err := gojwt.ParseWithClaims(tokenStr, claims, func(t *gojwt.Token) (any, error) {
		if _, ok := t.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.pubKey, nil
	}, gojwt.WithLeeway(30*time.Second)) // 容忍跨 pod 时钟偏移,防 exp/nbf 边界误拒
	if err != nil || !tok.Valid || claims.Subject == "" {
		return 0
	}
	var uid int64
	fmt.Sscanf(claims.Subject, "%d", &uid)
	return uid
}
