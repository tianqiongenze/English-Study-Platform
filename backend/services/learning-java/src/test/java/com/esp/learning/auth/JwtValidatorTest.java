package com.esp.learning.auth;

import com.esp.common.jwt.BaseJwtValidator;
import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import io.jsonwebtoken.Jwts;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.util.Base64;

import static org.junit.jupiter.api.Assertions.*;

/**
 * JwtValidator 单测(ADR-12 RS256):验证 learning-java 用公钥校验 BFF 签发的 JWT。
 * 测试策略:用与 bff-java 相同的 RSA keypair 签发 token,learning-java 公钥校验应通过;
 * 用不同密钥对签发的 token 应校验失败。
 *
 * <p>注意:learning-java 的 JwtValidator 现为 thin wrapper(extends BaseJwtValidator)。
 * 本测试验证 wrapper + base 类的完整行为;BaseJwtValidator 自身的单元测试见
 * esp-common-java BaseJwtValidatorTest。
 */
class JwtValidatorTest {

    private static final String RSA_DIR = "src/test/resources/rsa";
    private String publicKeyPem;
    private PrivateKey privateKey;
    private BaseJwtValidator validator;

    @BeforeEach
    void setUp() throws Exception {
        publicKeyPem = Files.readString(Path.of(RSA_DIR, "public.pem"));
        String privateKeyPem = Files.readString(Path.of(RSA_DIR, "private.pem"));
        privateKey = parsePrivateKey(privateKeyPem);
        validator = new BaseJwtValidator(publicKeyPem);
    }

    @Test
    void validTokenSignedByBffIsAccepted() {
        String token = issueToken(42L, "u@x.com", "alice", "learner");
        Claims c = validator.parse(token);
        assertEquals("42", c.getSubject());
        assertEquals("u@x.com", c.get("email", String.class));
        assertEquals("learner", c.get("role", String.class));
    }

    @Test
    void parseUserIdReturnsUserIdForValidToken() {
        String token = issueToken(99L, "a@b.com", "bob", "teacher");
        assertEquals(99L, validator.parseUserId(token));
    }

    @Test
    void parseUserIdReturnsNullForInvalidToken() {
        assertNull(validator.parseUserId("garbage.token.value"));
    }

    @Test
    void tamperedTokenThrows() {
        String token = issueToken(1L, "a@b.com", "n", "teacher");
        String[] parts = token.split("\\.");
        char last = parts[1].charAt(parts[1].length() - 1);
        char swap = last == 'A' ? 'B' : 'A';
        String tampered = parts[0] + "." + parts[1].substring(0, parts[1].length() - 1) + swap + "." + parts[2];
        assertThrows(JwtException.class, () -> validator.parse(tampered));
    }

    @Test
    void tokenSignedByDifferentKeyPairThrows() throws Exception {
        java.security.KeyPairGenerator gen = java.security.KeyPairGenerator.getInstance("RSA");
        gen.initialize(2048);
        java.security.KeyPair otherKp = gen.generateKeyPair();
        String otherToken = Jwts.builder()
                .subject("1")
                .claim("email", "x@y.com")
                .claim("name", "z")
                .claim("role", "admin")
                .signWith(otherKp.getPrivate(), Jwts.SIG.RS256)
                .compact();
        assertThrows(JwtException.class, () -> validator.parse(otherToken), "不同密钥对签的 token 应校验失败");
    }

    @Test
    void garbageTokenThrows() {
        assertThrows(Exception.class, () -> validator.parse("not.a.jwt"));
        assertThrows(Exception.class, () -> validator.parse(""));
    }

    // ---- Helper:用私钥签发(模拟 BFF 签发) ----

    private String issueToken(long userId, String email, String name, String role) {
        return Jwts.builder()
                .subject(String.valueOf(userId))
                .claim("email", email)
                .claim("name", name)
                .claim("role", role)
                .signWith(privateKey, Jwts.SIG.RS256)
                .compact();
    }

    private static PrivateKey parsePrivateKey(String pem) throws Exception {
        String content = pem.replace("-----BEGIN PRIVATE KEY-----", "")
                .replace("-----END PRIVATE KEY-----", "")
                .replaceAll("\\s+", "");
        byte[] der = Base64.getDecoder().decode(content);
        return KeyFactory.getInstance("RSA").generatePrivate(new PKCS8EncodedKeySpec(der));
    }
}
