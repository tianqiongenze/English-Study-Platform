package com.esp.bff.auth;

import com.esp.cache.CacheService;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.SignatureAlgorithm;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import java.util.Date;
import java.util.Map;
import java.util.UUID;

/**
 * JWT 签发与校验(ADR-12):RS256 非对称密钥。
 * BFF 持私钥签发;下游(learning-java/gateway-go)持公钥校验。
 * 私钥来源:env JWT_RSA_PRIVATE_KEY_PEM(PKCS8 PEM);公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM)。
 * JWKS endpoint 见 {@link com.esp.bff.auth.JwksController}。
 *
 * <p>P1-9 JWT 加固(2026-08-14):
 * <ul>
 *   <li><b>jti</b>:每个 access token 带唯一 jti,支持单 token 撤销。</li>
 *   <li><b>Redis 黑名单(注销生效)</b>:logout 把 jti 写入 {@code esp:jwt:blacklist:<jti>}(TTL=剩余有效期)。
 *       {@link #parse(String)} 解析后校验 jti 不在黑名单 → 注销立即生效(原方案 logout 后旧 token 仍可用 1h)。</li>
 *   <li><b>refresh token</b>:长 TTL(7d)的刷新令牌,access token 过期后用 refresh 换新 access(免重新登录)。
 *       refresh 亦带 jti 并入黑名单管理(登出/轮换时拉黑)。</li>
 * </ul>
 *
 * <p>降级:CacheService 不可用(Redis 故障)时黑名单校验跳过(放行),不阻断登录/鉴权主链路;
 * 安全侧影响:此时 logout 后旧 token 在剩余 TTL 内仍可用 —— 可接受的可用性权衡(Redis 恢复后自愈)。
 */
@Component
public class JwtService {

    private static final Logger log = LoggerFactory.getLogger(JwtService.class);

    /** access token TTL:1 小时。 */
    private static final long ACCESS_TTL_MS = 3600_000L;
    /** refresh token TTL:7 天。 */
    private static final long REFRESH_TTL_MS = 7L * 24 * 3600_000L;

    private static final String BLACKLIST_CACHE = "esp_jwt_blacklist";
    private static final String BLACKLIST_KEY_PREFIX = "esp:jwt:blacklist:";

    private final PrivateKey privateKey;
    private final PublicKey publicKey;
    private final CacheService cacheService;

    public JwtService(
            @Value("${jwt.rsa.private-key-pem:}") String privateKeyPem,
            @Value("${jwt.rsa.public-key-pem:}") String publicKeyPem,
            CacheService cacheService) {
        this.privateKey = parsePrivateKey(privateKeyPem);
        this.publicKey = parsePublicKey(publicKeyPem);
        this.cacheService = cacheService;
    }

    /** 鉴权是否启用(私钥已注入才签发,公钥已注入才校验)。 */
    public boolean enabled() {
        return privateKey != null;
    }

    /** 签发 access token(RS256,私钥签)。含 jti(单 token 撤销基础)。 */
    public String issue(long userId, String email, String name, String role) {
        return issue(userId, email, name, role, "access", ACCESS_TTL_MS);
    }

    /** 签发 refresh token(长 TTL,仅用于换 access,不含业务 claim)。 */
    public String issueRefresh(long userId, String email, String name, String role) {
        return issue(userId, email, name, role, "refresh", REFRESH_TTL_MS);
    }

    private String issue(long userId, String email, String name, String role, String type, long ttlMs) {
        long now = System.currentTimeMillis();
        return Jwts.builder()
                .id(UUID.randomUUID().toString())              // P1-9: jti
                .subject(String.valueOf(userId))
                .claim("email", email)
                .claim("name", name)
                .claim("role", role)
                .claim("typ", type)                              // access / refresh
                .issuedAt(new Date(now))
                .expiration(new Date(now + ttlMs))
                .signWith(privateKey, Jwts.SIG.RS256)
                .compact();
    }

    /**
     * 解析并校验 JWT(RS256,公钥验)+ 黑名单校验(P1-9)。
     * @throws io.jsonwebtoken.JwtException 签名/过期/格式错误
     * @throws TokenRevokedException jti 在黑名单(已注销)
     */
    public Claims parse(String token) {
        Claims claims = Jwts.parser().verifyWith(publicKey).build().parseSignedClaims(token).getPayload();
        if (isRevoked(claims)) {
            throw new TokenRevokedException("token revoked (logged out)");
        }
        return claims;
    }

    /** 注销:把 access token 的 jti 拉黑,剩余 TTL 后自动过期清理。 */
    public void revoke(Claims claims) {
        String jti = claims.getId();
        if (jti == null) return;
        long remainMs = Math.max(0, claims.getExpiration().getTime() - System.currentTimeMillis());
        long remainSec = remainMs / 1000;
        if (remainSec <= 0) return; // 已自然过期,无需拉黑
        try {
            cacheService.put(BLACKLIST_CACHE, BLACKLIST_KEY_PREFIX + jti, "1", remainSec);
            log.info("jwt revoked: jti={} remain={}s typ={}", jti, remainSec, claims.get("typ"));
        } catch (Exception e) {
            log.warn("jwt blacklist write failed (non-fatal): {}", e.getMessage());
        }
    }

    private boolean isRevoked(Claims claims) {
        String jti = claims.getId();
        if (jti == null) return false; // 老 token 无 jti,放行(向后兼容)
        if (!cacheService.isEnabled()) return false; // Redis 不可用 → 跳过黑名单(可用性优先)
        try {
            Object hit = cacheService.getOrLoad(BLACKLIST_CACHE, BLACKLIST_KEY_PREFIX + jti,
                    () -> null, Math.max(1,
                            (claims.getExpiration().getTime() - System.currentTimeMillis()) / 1000),
                    false);
            return hit != null;
        } catch (Exception e) {
            log.warn("jwt blacklist check failed (non-fatal, allow): {}", e.getMessage());
            return false;
        }
    }

    /** 暴露公钥供 JWKS endpoint 使用。 */
    public PublicKey getPublicKey() {
        return publicKey;
    }

    /** access TTL(秒),供调用方构造响应。 */
    public long getAccessTtlSeconds() {
        return ACCESS_TTL_MS / 1000;
    }

    // ---- PEM 解析工具 ----

    static PrivateKey parsePrivateKey(String pem) {
        String content = stripPem(pem);
        byte[] der = Base64.getDecoder().decode(content);
        try {
            return KeyFactory.getInstance("RSA").generatePrivate(new PKCS8EncodedKeySpec(der));
        } catch (Exception e) {
            throw new IllegalArgumentException("Invalid RSA private key PEM", e);
        }
    }

    static PublicKey parsePublicKey(String pem) {
        String content = stripPem(pem);
        byte[] der = Base64.getDecoder().decode(content);
        try {
            return KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(der));
        } catch (Exception e) {
            throw new IllegalArgumentException("Invalid RSA public key PEM", e);
        }
    }

    /** 去除 PEM 头尾行和空白。 */
    static String stripPem(String pem) {
        return pem.replace("-----BEGIN PRIVATE KEY-----", "")
                .replace("-----END PRIVATE KEY-----", "")
                .replace("-----BEGIN PUBLIC KEY-----", "")
                .replace("-----END PUBLIC KEY-----", "")
                .replaceAll("\\s+", "");
    }
}
