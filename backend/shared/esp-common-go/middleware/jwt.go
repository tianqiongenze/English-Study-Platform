// Package middleware 提供 ESP 全服务复用的横切中间件。
//
// jwt.go:零信任 JWT 校验 helper(P1-30,2026-08-21)。
//
// 背景(SCS/零信任):此前 BFF 下游 Go 服务纯信任 BFF 注入的 X-User-Id / X-Role 头,
// 集群内任何 pod 直连服务即可冒充任意用户(IDOR/越权)。本 helper 让服务自身用
// BFF 签发 JWT 的 RS256 公钥校验,取 userID,不轻信 X-User-Id 头。
//
// 对标 gateway-go/src/jwt/Validator(同款 RS256 公钥验签,subject=userID);
// 区别:gateway-go 自持 Validator 实例,本 helper 下沉到共享包供 3 个下游服务复用。
// 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM),与 gateway-go 对齐。
//
// RWT 铁律:JWT 校验失败返 401 不崩;启动期公钥缺失=os.Exit(1)(对齐 gateway-go,
// 生产期拒;开发期可 ESP_JWT_ALLOW_MISSING_KEY=1 降级放行便于本地调试)。

package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt/v5"
)

// JWTValidator 校验 RS256 JWT(公钥验签,ADR-12)。BFF 持私钥签发;下游服务持公钥校验。
type JWTValidator struct {
	pubKey *rsa.PublicKey
}

var (
	sharedValidator     *JWTValidator
	sharedValidatorOnce sync.Once
	sharedValidatorErr  error
)

// NewJWTValidatorFromEnv 从环境变量 JWT_RSA_PUBLIC_KEY_PEM 加载公钥。
// 公钥缺失返 error;调用方决定降级或退出(见 MustInitJWTFromEnv)。
func NewJWTValidatorFromEnv() (*JWTValidator, error) {
	pemStr := os.Getenv("JWT_RSA_PUBLIC_KEY_PEM")
	if pemStr == "" {
		return nil, fmt.Errorf("JWT_RSA_PUBLIC_KEY_PEM env not set")
	}
	return NewJWTValidatorFromPEM(pemStr)
}

// NewJWTValidatorFromPEM 从 PEM 字符串(X.509 SPKI)加载 RSA 公钥。
func NewJWTValidatorFromPEM(pemStr string) (*JWTValidator, error) {
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
	return &JWTValidator{pubKey: rsaPub}, nil
}

// MustInitJWTFromEnv 进程级单例初始化共享 JWTValidator(从 env 加载公钥)。
// 启动期公钥缺失=启动失败(对齐 gateway-go,os.Exit(1)),生产期拒。
// 开发期降级:env ESP_JWT_ALLOW_MISSING_KEY=1 时公钥缺失不退出,返回 nil
// (后续 ParseUserID 返 0,handler 走 401;便于本地无 BFF 调试,严禁生产开)。
func MustInitJWTFromEnv(serviceName string) *JWTValidator {
	sharedValidatorOnce.Do(func() {
		v, err := NewJWTValidatorFromEnv()
		if err != nil {
			if os.Getenv("ESP_JWT_ALLOW_MISSING_KEY") == "1" {
				log.Printf("[%s] WARN: JWT public key missing (%v); running in degraded mode "+
					"(ESP_JWT_ALLOW_MISSING_KEY=1, all authn will 401) — DO NOT use in production",
					serviceName, err)
				sharedValidator = nil
				return
			}
			log.Printf("[%s] FATAL: JWT public key not configured: %v", serviceName, err)
			os.Exit(1)
		}
		sharedValidator = v
		log.Printf("[%s] JWT validator ready (RS256 public key loaded)", serviceName)
	})
	// once 内发生 fatal os.Exit 则不会到达;降级模式 sharedValidator=nil
	return sharedValidator
}

// SharedJWTValidator 返回已初始化的单例(必须先调 MustInitJWTFromEnv)。
// 未初始化返回 nil(调用方应判空,防 nil 解引用,CLC 铁律)。
func SharedJWTValidator() *JWTValidator {
	return sharedValidator
}

// ParseUserID 解析 JWT(RS256 公钥验签)取 userID(subject)。
// 空/非法/无效/签名不符 token 返回 0(调用方应拒绝:HTTP 返 401)。
// validator 为 nil(降级模式或未初始化)返回 0。
func (v *JWTValidator) ParseUserID(tokenStr string) int64 {
	return v.ParseUserClaims(tokenStr).UserID
}

// UserClaims 是从 JWT 解析出的身份信息。
type UserClaims struct {
	UserID int64
	Role   string // BFF 签发的 "role" claim(如 learner/admin/teacher);无则空串
}

// ParseUserClaims 解析 JWT(RS256 公钥验签)取 userID + role。
// 空/非法/无效/签名不符 token 返回零值 UserClaims(调用方应拒绝:HTTP 返 401)。
// validator 为 nil 返回零值(CLC 铲律 nil-safe)。
func (v *JWTValidator) ParseUserClaims(tokenStr string) UserClaims {
	if v == nil || tokenStr == "" {
		return UserClaims{}
	}
	// 用 MapClaims 既含标准 claim(sub/exp/iat/jti)又含自定义 claim(role/email/name)。
	claims := gojwt.MapClaims{}
	tok, err := gojwt.ParseWithClaims(tokenStr, claims, func(t *gojwt.Token) (any, error) {
		if _, ok := t.Method.(*gojwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.pubKey, nil
	}, gojwt.WithLeeway(30*time.Second)) // 容忍跨 pod 时钟偏移,防 exp/nbf 边界误拒
	if err != nil || !tok.Valid {
		return UserClaims{}
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return UserClaims{}
	}
	var uid int64
	fmt.Sscanf(sub, "%d", &uid)
	role, _ := claims["role"].(string)
	return UserClaims{UserID: uid, Role: role}
}

// TokenFromRequest 取 JWT:Authorization Bearer 优先,回落 esp_token httpOnly cookie。
// (对标 gateway-go tokenFromRequest;浏览器同源请求自动携带 cookie,JS 读不到 → 灭 XSS 窃取面;
// 小程序仍走 Bearer)。两者都没有返回空串。
func TokenFromRequest(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		return strings.TrimPrefix(authz, "Bearer ")
	}
	if c, err := r.Cookie("esp_token"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// UserIDFromRequest 解析请求 JWT 取 userID(零信任,不读 X-User-Id 头)。
// validator 为 nil(降级模式)返回 0 → handler 走 401。
func UserIDFromRequest(v *JWTValidator, r *http.Request) int64 {
	if v == nil {
		return 0
	}
	return v.ParseUserID(TokenFromRequest(r))
}

// UserClaimsFromRequest 解析请求 JWT 取 userID + role(零信任,不读 X-User-Id/X-Role 头)。
// validator 为 nil(降级模式)返回零值 → handler 走 401。
func UserClaimsFromRequest(v *JWTValidator, r *http.Request) UserClaims {
	if v == nil {
		return UserClaims{}
	}
	return v.ParseUserClaims(TokenFromRequest(r))
}

// RequireAuth gin 中间件:校验 JWT 取 userID 注入 context(零信任)。
// 失败返 401。用法:router.Use(middleware.RequireAuth(v)) 或单路由挂载。
// context key "uid"(int64)可由 c.MustGet("uid").(int64) 取出。
func RequireAuth(v *JWTValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserIDFromRequest(v, c.Request)
		if uid <= 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("uid", uid)
		c.Next()
	}
}
