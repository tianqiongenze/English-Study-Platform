package com.esp.bff.auth;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.SignatureAlgorithm;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;
import java.util.Date;

/**
 * JWT 签发与校验(ADR-12):RS256 非对称密钥。
 * BFF 持私钥签发;下游(learning-java/gateway-go)持公钥校验。
 * 私钥来源:env JWT_RSA_PRIVATE_KEY_PEM(PKCS8 PEM);公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM)。
 * JWKS endpoint 见 {@link com.esp.bff.auth.JwksController}。
 *
 * <p>HS256 回退点:若需降级,将构造器改回 Keys.hmacShaKeyFor(secret) + signWith(key) + verifyWith(key),
 * 见 git history 或 docs/PROGRESS.md。</p>
 */
@Component
public class JwtService {

    private final PrivateKey privateKey;
    private final PublicKey publicKey;

    public JwtService(
            @Value("${jwt.rsa.private-key-pem:}") String privateKeyPem,
            @Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        this.privateKey = parsePrivateKey(privateKeyPem);
        this.publicKey = parsePublicKey(publicKeyPem);
    }

    /** 签发 JWT(RS256,私钥签)。设 iat+exp:gateway-go 的 golang-jwt/v5 默认要求 exp 存在且未过期,缺失则静默拒。 */
    public String issue(long userId, String email, String name, String role) {
        long now = System.currentTimeMillis();
        return Jwts.builder()
                .subject(String.valueOf(userId))
                .claim("email", email)
                .claim("name", name)
                .claim("role", role)
                .issuedAt(new Date(now))
                .expiration(new Date(now + 3600_000))  // 1h TTL(可抽 @Value 配置,当前最小修复)
                .signWith(privateKey, Jwts.SIG.RS256)
                .compact();
    }

    /** 解析 JWT(RS256,公钥验)。 */
    public Claims parse(String token) {
        return Jwts.parser().verifyWith(publicKey).build().parseSignedClaims(token).getPayload();
    }

    /** 暴露公钥供 JWKS endpoint 使用。 */
    public PublicKey getPublicKey() {
        return publicKey;
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
