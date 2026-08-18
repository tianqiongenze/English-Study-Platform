package com.esp.common.jwt;

import io.jsonwebtoken.Jwts;
import io.jsonwebtoken.security.Keys;
import org.junit.jupiter.api.Test;

import java.security.KeyPair;
import java.security.PrivateKey;
import java.util.Date;

import static org.junit.jupiter.api.Assertions.*;

class BaseJwtValidatorTest {

    private static final KeyPair KEY_PAIR = Keys.keyPairFor(Jwts.SIG.RS256);
    private static final String PEM_HEADER = "-----BEGIN PUBLIC KEY-----";
    private static final String PEM_FOOTER = "-----END PUBLIC KEY-----";

    private String toPem(java.security.PublicKey pk) {
        String b64 = java.util.Base64.getEncoder().encodeToString(pk.getEncoded());
        return PEM_HEADER + "\n" + b64 + "\n" + PEM_FOOTER;
    }

    private String createToken(long userId) {
        return Jwts.builder()
                .subject(String.valueOf(userId))
                .claim("role", "student")
                .issuedAt(new Date())
                .expiration(new Date(System.currentTimeMillis() + 3600_000))
                .signWith(KEY_PAIR.getPrivate(), Jwts.SIG.RS256)
                .compact();
    }

    @Test
    void parseValidToken() {
        BaseJwtValidator v = new BaseJwtValidator(toPem(KEY_PAIR.getPublic()));
        assertTrue(v.enabled());
        Long uid = v.parseUserId(createToken(42L));
        assertEquals(42L, uid);
    }

    @Test
    void parseInvalidSignature() {
        KeyPair other = Keys.keyPairFor(Jwts.SIG.RS256);
        BaseJwtValidator v = new BaseJwtValidator(toPem(other.getPublic()));
        assertThrows(Exception.class, () -> v.parse(createToken(1L)));
    }

    @Test
    void disabledWhenEmptyKey() {
        BaseJwtValidator v = new BaseJwtValidator("");
        assertFalse(v.enabled());
    }

    @Test
    void disabledWhenNullKey() {
        BaseJwtValidator v = new BaseJwtValidator(null);
        assertFalse(v.enabled());
    }

    @Test
    void parseExpiredToken() {
        BaseJwtValidator v = new BaseJwtValidator(toPem(KEY_PAIR.getPublic()));
        String expired = Jwts.builder()
                .subject("99")
                .issuedAt(new Date(System.currentTimeMillis() - 7200_000))
                .expiration(new Date(System.currentTimeMillis() - 3600_000))
                .signWith(KEY_PAIR.getPrivate(), Jwts.SIG.RS256)
                .compact();
        assertThrows(Exception.class, () -> v.parse(expired));
    }
}
