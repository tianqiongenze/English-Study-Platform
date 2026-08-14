package com.esp.bff.filter;

import com.esp.bff.auth.JwtService;
import com.esp.cache.CacheService;
import io.jsonwebtoken.Claims;
import jakarta.servlet.FilterChain;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * AuthFilter 单测(ADR-12 安全核心,RS256)。
 * 验证:白名单放行(含 /.well-known/jwks.json)、无/非法 Authorization 401、合法 token 注入 X-User-Id/X-Role、
 * 外网伪造的 X-User-Id 被覆盖(剥离后注入可信值)。
 */
class AuthFilterTest {

    private static final String RSA_DIR = "src/test/resources/rsa";

    private JwtService jwt;
    private AuthFilter filter;

    @BeforeEach
    void setUp() throws Exception {
        String priv = Files.readString(Path.of(RSA_DIR, "private.pem"));
        String pub = Files.readString(Path.of(RSA_DIR, "public.pem"));
        CacheService cache = mock(CacheService.class);
        when(cache.isEnabled()).thenReturn(false);
        jwt = new JwtService(priv, pub, cache);
        filter = new AuthFilter(jwt);
    }

    @Test
    void whitelistPassesThroughWithoutAuth() throws Exception {
        for (String path : new String[]{"/api/auth/register", "/healthz", "/actuator/health", "/.well-known/jwks.json"}) {
            MockHttpServletRequest req = new MockHttpServletRequest("GET", path);
            MockHttpServletResponse res = new MockHttpServletResponse();
            FilterChain chain = mock(FilterChain.class);
            filter.doFilter(req, res, chain);
            assertEquals(200, res.getStatus(), "白名单应放行: " + path);
            verify(chain).doFilter(req, res);
        }
    }

    @Test
    void missingAuthorizationHeaderReturns401() throws Exception {
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);
        filter.doFilter(req, res, chain);
        assertEquals(401, res.getStatus());
        verify(chain, never()).doFilter(any(), any());
    }

    @Test
    void nonBearerAuthorizationReturns401() throws Exception {
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        req.addHeader("Authorization", "Basic abc");
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);
        filter.doFilter(req, res, chain);
        assertEquals(401, res.getStatus());
        verify(chain, never()).doFilter(any(), any());
    }

    @Test
    void malformedBearerTokenReturns401() throws Exception {
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        req.addHeader("Authorization", "Bearer not.a.real.jwt");
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);
        filter.doFilter(req, res, chain);
        assertEquals(401, res.getStatus());
        verify(chain, never()).doFilter(any(), any());
    }

    @Test
    void validTokenInjectsTrustedHeaders() throws Exception {
        String token = jwt.issue(77L, "u@x.com", "alice", "teacher");
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        req.addHeader("Authorization", "Bearer " + token);
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);

        filter.doFilter(req, res, chain);

        assertEquals(200, res.getStatus());
        verify(chain).doFilter(any(), eq(res));
        // 注入的 wrapped request 应带可信头。
        org.mockito.ArgumentCaptor<jakarta.servlet.http.HttpServletRequest> cap =
                org.mockito.ArgumentCaptor.forClass(jakarta.servlet.http.HttpServletRequest.class);
        verify(chain).doFilter(cap.capture(), any());
        assertEquals("77", cap.getValue().getHeader("X-User-Id"));
        assertEquals("teacher", cap.getValue().getHeader("X-Role"));
    }

    @Test
    void spoofedXUserIdIsOverwrittenByTrustedValue() throws Exception {
        String token = jwt.issue(77L, "u@x.com", "alice", "learner");
        MockHttpServletRequest req = new MockHttpServletRequest("GET", "/api/learning/due");
        req.addHeader("Authorization", "Bearer " + token);
        req.addHeader("X-User-Id", "9999");   // 外网伪造
        req.addHeader("X-Role", "admin");      // 外网伪造
        MockHttpServletResponse res = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);

        filter.doFilter(req, res, chain);

        org.mockito.ArgumentCaptor<jakarta.servlet.http.HttpServletRequest> cap =
                org.mockito.ArgumentCaptor.forClass(jakarta.servlet.http.HttpServletRequest.class);
        verify(chain).doFilter(cap.capture(), any());
        assertEquals("77", cap.getValue().getHeader("X-User-Id"), "伪造的 X-User-Id 必须被覆盖");
        assertEquals("learner", cap.getValue().getHeader("X-Role"), "伪造的 X-Role 必须被覆盖");
        // getHeaders(复数)也应只有可信值。
        java.util.List<String> ids = java.util.Collections.list(cap.getValue().getHeaders("X-User-Id"));
        assertEquals(1, ids.size());
        assertEquals("77", ids.get(0));
    }
}
