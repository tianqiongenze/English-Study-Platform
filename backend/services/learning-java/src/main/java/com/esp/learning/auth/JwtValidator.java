package com.esp.learning.auth;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import io.jsonwebtoken.Jwts;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.security.KeyFactory;
import java.security.PublicKey;
import java.security.spec.X509EncodedKeySpec;
import java.util.Base64;

/**
 * JWT 校验(ADR-12 RS256):learning-java 用公钥校验 BFF 签发的 JWT。
 * 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM)。
 * JWKS 动态获取为后续增强(当前走 PEM 直校验,注释说明 JWKS fetch 为后续)。
 */
@Component
public class JwtValidator {

    private final PublicKey publicKey;

    public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        this.publicKey = publicKeyPem == null || publicKeyPem.isBlank() ? null : parsePublicKey(publicKeyPem);
    }

    /** 鉴权是否启用(公钥已注入才启用)。P0-2:公钥未注入时放行(本地/测试无密钥环境避免启动即 401 全挂)。 */
    public boolean enabled() {
        return publicKey != null;
    }

    /** 解析并校验 JWT(RS256,公钥验)。校验失败抛 JwtException。 */
    public Claims parse(String token) {
        return Jwts.parser().verifyWith(publicKey).build().parseSignedClaims(token).getPayload();
    }

    /** 校验 token 并返回 subject(userId);校验失败返回 null。 */
    public Long parseUserId(String token) {
        try {
            return Long.valueOf(parse(token).getSubject());
        } catch (JwtException e) {
            return null;
        }
    }

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
