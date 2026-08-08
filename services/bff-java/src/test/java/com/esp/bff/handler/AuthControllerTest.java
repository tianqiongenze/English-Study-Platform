package com.esp.bff.handler;

import com.esp.bff.auth.JwtService;
import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.UserRegisteredEvent;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * AuthController 单测(ADR-12 安全核心,register/login/wx-login 三个端点,RS256)。
 * 用 Mockito mock JdbcTemplate/CacheService/EventPublisher,隔离 DB/缓存/MQ。
 * JwtService 用真实实例(纯内存,无外部依赖,RS256 keypair from test resources)。
 */
class AuthControllerTest {

    private static final String RSA_DIR = "src/test/resources/rsa";

    private JdbcTemplate jdbc;
    private CacheService cache;
    private EventPublisher events;
    private AuthController ctrl;

    @BeforeEach
    void setUp() throws Exception {
        String priv = Files.readString(Path.of(RSA_DIR, "private.pem"));
        String pub = Files.readString(Path.of(RSA_DIR, "public.pem"));
        jwt = new JwtService(priv, pub);
        jdbc = mock(JdbcTemplate.class);
        cache = mock(CacheService.class);
        events = mock(EventPublisher.class);
        ctrl = new AuthController(jdbc, jwt, cache, events);
    }

    // JwtService needs to be accessible for the test
    private JwtService jwt;

    @Test
    @SuppressWarnings("unchecked")
    void registerInsertsUserIssuesTokenCachesAndPublishesEvent() {
        when(jdbc.queryForObject(eq("SELECT id FROM users WHERE email=?"), eq(Long.class), anyString()))
                .thenReturn(123L);

        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.register(
                Map.of("email", "a@b.com", "password", "pw", "name", "alice", "role", "learner"));

        assertEquals(HttpStatus.OK, resp.getStatusCode());
        Map<String, Object> body = resp.getBody();
        assertNotNull(body);
        assertNotNull(body.get("token"), "register 应签发 JWT");
        // 写库(INSERT)被调用。
        verify(jdbc).update(eq("INSERT INTO users (role,name,email,pwd_hash) VALUES (?,?,?,?)"),
                eq("learner"), eq("alice"), eq("a@b.com"), anyString());
        // 缓存用户画像。
        verify(cache).put(eq("bff_user"), eq("user:123"), anyMap(), eq(300L));
        // 发 user.registered 事件(补死代码断链)。
        verify(events).publish(eq(EventTopics.USER_REGISTERED), any(UserRegisteredEvent.class));
    }

    @Test
    @SuppressWarnings("unchecked")
    void registerDuplicateEmailReturns409() {
        when(jdbc.update(anyString(), any(), any(), any(), any()))
                .thenThrow(new RuntimeException("Duplicate entry"));

        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.register(
                Map.of("email", "dup@x.com", "password", "pw"));

        assertEquals(HttpStatus.CONFLICT, resp.getStatusCode());
        verify(cache, never()).put(anyString(), anyString(), any(), anyLong());
        verify(events, never()).publish(anyString(), any());
    }

    @Test
    @SuppressWarnings("unchecked")
    void loginWithValidCredentialsIssuesTokenAndCaches() {
        // BCrypt of "secret-pw".
        String hash = new org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder().encode("secret-pw");
        when(jdbc.queryForMap(eq("SELECT id,name,role,pwd_hash FROM users WHERE email=?"), eq("u@x.com")))
                .thenReturn(Map.of("id", 55L, "name", "bob", "role", "teacher", "pwd_hash", hash));

        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.login(
                Map.of("email", "u@x.com", "password", "secret-pw"));

        assertEquals(HttpStatus.OK, resp.getStatusCode());
        assertNotNull(resp.getBody().get("token"));
        verify(cache).put(eq("bff_user"), eq("user:55"), anyMap(), eq(300L));
    }

    @Test
    @SuppressWarnings("unchecked")
    void loginWithWrongPasswordReturns401() {
        String hash = new org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder().encode("correct");
        when(jdbc.queryForMap(anyString(), anyString()))
                .thenReturn(Map.of("id", 1L, "name", "x", "role", "learner", "pwd_hash", hash));

        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.login(
                Map.of("email", "u@x.com", "password", "wrong"));

        assertEquals(HttpStatus.UNAUTHORIZED, resp.getStatusCode());
        verify(cache, never()).put(anyString(), anyString(), any(), anyLong());
    }

    @Test
    @SuppressWarnings("unchecked")
    void loginWithUnknownEmailReturns401() {
        when(jdbc.queryForMap(anyString(), anyString()))
                .thenThrow(new RuntimeException("EmptyResult"));

        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.login(
                Map.of("email", "no@x.com", "password", "pw"));

        assertEquals(HttpStatus.UNAUTHORIZED, resp.getStatusCode());
    }

    @Test
    @SuppressWarnings("unchecked")
    void wxLoginWithBlankCodeWhenDisabledReturns400() {
        // wxLoginEnabled 默认 false。
        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.wxLogin(
                Map.of("code", ""));

        assertEquals(HttpStatus.BAD_REQUEST, resp.getStatusCode());
    }

    @Test
    @SuppressWarnings("unchecked")
    void wxLoginWithCodeIssuesAnonymousToken() {
        ResponseEntity<Map<String, Object>> resp = (ResponseEntity<Map<String, Object>>) (ResponseEntity<?>) ctrl.wxLogin(
                Map.of("code", "abc123"));

        assertEquals(HttpStatus.OK, resp.getStatusCode());
        Map<String, Object> body = resp.getBody();
        assertNotNull(body.get("token"), "wx-login 应签发匿名 JWT");
        @SuppressWarnings("unchecked")
        Map<String, Object> user = (Map<String, Object>) body.get("user");
        assertEquals("learner", user.get("role"));
    }
}
