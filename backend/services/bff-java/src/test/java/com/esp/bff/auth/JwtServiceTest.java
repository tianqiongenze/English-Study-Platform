package com.esp.bff.auth;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import com.esp.cache.CacheService;

import java.nio.file.Files;
import java.nio.file.Path;
import java.security.PrivateKey;
import java.security.PublicKey;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;
import static org.mockito.ArgumentMatchers.*;

/**
 * JwtService 单测(ADR-12 安全核心,RS256 非对称密钥)。
 * 验证:issue/parse 往返、claim 透传、篡改/伪造 token 抛异常、错误公钥抛错、垃圾 token 抛错。
 * 测试用 RSA keypair 由 scripts/gen-rsa-keypair.sh 生成,存 test/resources/rsa/。
 */
class JwtServiceTest {

    private static final String RSA_DIR = "src/test/resources/rsa";
    private String privateKeyPem;
    private String publicKeyPem;
    private JwtService jwt;

    @BeforeEach
    void setUp() throws Exception {
        privateKeyPem = Files.readString(Path.of(RSA_DIR, "private.pem"));
        publicKeyPem = Files.readString(Path.of(RSA_DIR, "public.pem"));
        // P1-9: JwtService 现需 CacheService(黑名单)。单测用 mock,且 stub 为 disabled,
        // 跳过黑名单校验(关注签发/验签往返,不测注销场景)。
        CacheService cache = mock(CacheService.class);
        when(cache.isEnabled()).thenReturn(false);
        jwt = new JwtService(privateKeyPem, publicKeyPem, cache);
    }

    @Test
    void issueAndParseRoundTripPreservesAllClaims() {
        String token = jwt.issue(42L, "u@x.com", "alice", "learner");
        Claims c = jwt.parse(token);
        assertEquals("42", c.getSubject(), "sub 应为 userId");
        assertEquals("u@x.com", c.get("email", String.class));
        assertEquals("alice", c.get("name", String.class));
        assertEquals("learner", c.get("role", String.class));
    }

    @Test
    void parseTamperedSignatureThrows() {
        String token = jwt.issue(1L, "a@b.com", "n", "teacher");
        // 篡改 payload 段(第二段)的最后一个字符。
        String[] parts = token.split("\\.");
        char last = parts[1].charAt(parts[1].length() - 1);
        char swap = last == 'A' ? 'B' : 'A';
        String tampered = parts[0] + "." + parts[1].substring(0, parts[1].length() - 1) + swap + "." + parts[2];
        assertThrows(JwtException.class, () -> jwt.parse(tampered), "篡改 payload 后签名校验应失败");
    }

    @Test
    void parseTokenSignedByDifferentKeyPairThrows() throws Exception {
        // 生成一对不同的 key 来签发,用本 jwt 的公钥解析应失败。
        String otherPrivPem = generateInMemoryKeyPairPem("private");
        String otherPubPem = generateInMemoryKeyPairPem("public");
        CacheService otherCache = mock(CacheService.class);
        when(otherCache.isEnabled()).thenReturn(false);
        JwtService otherIssuer = new JwtService(otherPrivPem, otherPubPem, otherCache);
        String foreignToken = otherIssuer.issue(9L, "x@y.com", "z", "admin");
        assertThrows(JwtException.class, () -> jwt.parse(foreignToken), "不同密钥对签发的 token 应校验失败");
    }

    @Test
    void parseGarbageThrows() {
        assertThrows(Exception.class, () -> jwt.parse("not.a.jwt"));
        assertThrows(Exception.class, () -> jwt.parse(""));
    }

    @Test
    void issueProducesCompactThreePartToken() {
        String token = jwt.issue(7L, "e@f.com", "g", "admin");
        assertNotNull(token);
        assertEquals(3, token.split("\\.").length, "JWT 应为 header.payload.signature 三段");
        assertEquals("7", jwt.parse(token).getSubject());
    }

    @Test
    void algorithmIsRS256() {
        String token = jwt.issue(1L, "a@b.com", "n", "learner");
        // 解码 header 看 alg
        String header = token.split("\\.")[0];
        String headerJson = new String(java.util.Base64.getDecoder().decode(header));
        assertTrue(headerJson.contains("\"RS256\""), "签发算法应为 RS256,实际 header: " + headerJson);
    }

    @Test
    void issueRefresh_andParse_roundtripPreservesTyp() {
        String token = jwt.issueRefresh(42L, "u@x.com", "alice", "learner");
        Claims c = jwt.parse(token);
        assertEquals("42", c.getSubject());
        assertEquals("refresh", c.get("typ", String.class), "refresh token 的 typ 应为 refresh");
        assertEquals("u@x.com", c.get("email", String.class));
    }

    @Test
    void revoke_throwsTokenRevoked_whenCacheEnabled() {
        // 构造一个 cache enabled 的 JwtService
        CacheService enabledCache = mock(CacheService.class);
        when(enabledCache.isEnabled()).thenReturn(true);
        // 初始:不在黑名单(null); revoke 后返回"1"(在黑名单中)
        java.util.concurrent.atomic.AtomicBoolean revoked = new java.util.concurrent.atomic.AtomicBoolean(false);
        when(enabledCache.getOrLoad(anyString(), anyString(), any(), anyLong(), anyBoolean()))
                .thenAnswer(inv -> revoked.get() ? "1" : null);
        JwtService svc = new JwtService(privateKeyPem, publicKeyPem, enabledCache);

        String token = svc.issue(1L, "a@b.com", "n", "learner");
        // 首次解析应成功(未拉黑)
        Claims c = svc.parse(token);
        assertEquals("1", c.getSubject());
        // 拉黑
        svc.revoke(c);
        revoked.set(true);
        // 再解析同一 token 应被拒绝(token revoked)
        assertThrows(TokenRevokedException.class, () -> svc.parse(token), "revoked token 应抛 TokenRevokedException");
    }

    @Test
    void getAccessTtlSeconds_returns3600() {
        assertEquals(3600, jwt.getAccessTtlSeconds(), "access TTL 应为 3600s(1h)");
    }

    /** 内存生成 RSA keypair 并返回 PEM(仅用于测试"不同密钥对互斥")。 */
    private static String generateInMemoryKeyPairPem(String type) throws Exception {
        java.security.KeyPairGenerator gen = java.security.KeyPairGenerator.getInstance("RSA");
        gen.initialize(2048);
        java.security.KeyPair kp = gen.generateKeyPair();
        java.util.Base64.Encoder b64 = java.util.Base64.getMimeEncoder(64, "\n".getBytes());
        if ("private".equals(type)) {
            byte[] der = kp.getPrivate().getEncoded();
            return "-----BEGIN PRIVATE KEY-----\n" + b64.encodeToString(der) + "\n-----END PRIVATE KEY-----\n";
        } else {
            byte[] der = kp.getPublic().getEncoded();
            return "-----BEGIN PUBLIC KEY-----\n" + b64.encodeToString(der) + "\n-----END PUBLIC KEY-----\n";
        }
    }
}
