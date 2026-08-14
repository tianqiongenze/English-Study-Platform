package com.esp.teacher.config;

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
 * JWT 校验(ADR-12 RS256):teacher-java 用公钥校验 BFF 签发的 JWT。
 * 公钥来源:env JWT_RSA_PUBLIC_KEY_PEM(X.509 SPKI PEM,由 secret esp-rsa-jwt envFrom 注入)。
 * D2 修复:此前 teacher 端口完全裸奔,本类 + JwtAuthFilter 补齐鉴权。
 */
@Component
public class JwtValidator {

    private final PublicKey publicKey;

    public JwtValidator(@Value("${jwt.rsa.public-key-pem:}") String publicKeyPem) {
        this.publicKey = publicKeyPem.isBlank() ? null : parsePublicKey(publicKeyPem);
    }

    /** 鉴权是否启用(公钥已注入才启用)。 */
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
        } catch (JwtException | IllegalArgumentException e) {
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
