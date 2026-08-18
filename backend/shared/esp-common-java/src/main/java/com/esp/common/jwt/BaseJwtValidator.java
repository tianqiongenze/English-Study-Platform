package com.esp.common.jwt;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import io.jsonwebtoken.Jwts;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;

/**
 * JWT 校验工具(ADR-12 RS256):learning-java/teacher-java 共用公钥校验逻辑。
 * <p>
 * 使用方注入 {@code jwt.rsa.public-key-pem} 环境变量(X.509 SPKI PEM)后,
 * 由 Spring 管理生命周期:
 * <pre>
 *   {@literal @}Component
 *   public class JwtValidator extends BaseJwtValidator {
 *       public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
 *           super(publicKeyPem);
 *       }
 *   }
 * </pre>
 * <p>
 * 也可以直接 new {@link #BaseJwtValidator(String)} 用于非 Spring 场景(单测/脚本)。
 *
 * <p>收编自 learning-java auth/JwtValidator + teacher-java config/JwtValidator,
 * 消除三服务间几乎相同的 PEM 解析 + RS256 校验 + parseUserId 代码。
 */
public class BaseJwtValidator {

    private static final Logger log = LoggerFactory.getLogger(BaseJwtValidator.class);

    private final PublicKey publicKey;

    /**
     * @param publicKeyPem RS256 公钥 PEM(X.509 SPKI),空串/null 时鉴权不启用。
     */
    public BaseJwtValidator(String publicKeyPem) {
        this.publicKey = (publicKeyPem == null || publicKeyPem.isBlank()) ? null : parsePublicKey(publicKeyPem);
    }

    /** 鉴权是否启用(公钥已注入才启用)。P0-2:公钥未注入时放行(本地/测试无密钥环境避免启动即 401)。 */
    public boolean enabled() {
        return publicKey != null;
    }

    /** 暴露公钥供 JWKS / 子类使用。 */
    public PublicKey getPublicKey() {
        return publicKey;
    }

    /**
     * 解析并校验 JWT(RS256,公钥验)。校验失败抛 JwtException。
     */
    public Claims parse(String token) {
        return Jwts.parser().verifyWith(publicKey).build().parseSignedClaims(token).getPayload();
    }

    /**
     * 校验 token 并返回 subject(userId);校验失败返回 null。
     */
    public Long parseUserId(String token) {
        try {
            return Long.valueOf(parse(token).getSubject());
        } catch (JwtException | IllegalArgumentException e) {
            log.debug("JWT parseUserId failed: {}", e.getMessage());
            return null;
        }
    }

    // ---- PEM 解析工具 ----

    /**
     * 从 X.509 SPKI PEM 解析 RSA 公钥。
     * 支持带/不带头尾行的 PEM 格式。
     */
    static PublicKey parsePublicKey(String pem) {
        String content = pem.replace("-----BEGIN PUBLIC KEY-----", "")
                .replace("-----END PUBLIC KEY-----", "")
                .replaceAll("\\s+", "");
        byte[] der = Base64.getDecoder().decode(content);
        try {
            return KeyFactory.getInstance("RSA").generatePublic(new X509EncodedKeySpec(der));
        } catch (Exception e) {
            throw new IllegalArgumentException("Invalid RSA public key PEM", e);
        }
    }
}
