package com.esp.bff.filter;

import com.esp.bff.auth.JwtService;
import io.jsonwebtoken.Claims;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletRequestWrapper;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.Collections;
import java.util.Enumeration;
import java.util.List;

/**
 * JWT 边缘校验 + X-User-Id/X-Role 注入(ADR-12 安全,Web MVC 版,替代 SCG GlobalFilter)。
 * - 放行:/api/auth/**、/healthz、/actuator/**。
 * - 剥离外网伪造的 X-User-Id/X-Role 头,由 BFF 注入可信值。
 * - 校验失败 401。
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class AuthFilter extends OncePerRequestFilter {

    // 放行:认证、健康、actuator、API 门户(springdoc /v3/api-docs + /docs 聚合 Swagger UI +
    // /swagger-ui 静态资源 + /openapi-go-rust.yaml Go/Rust 手写清单)。门户为只读 API 文档,无敏感数据;
    // 实际业务接口仍受本 Filter 保护。
    private static final List<String> WHITELIST = List.of(
            "/api/auth/", "/healthz", "/actuator/", "/.well-known/",
            "/v3/api-docs", "/docs", "/swagger-ui", "/openapi-go-rust.yaml");
    private final JwtService jwt;

    public AuthFilter(JwtService jwt) { this.jwt = jwt; }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        String path = req.getRequestURI();
        for (String w : WHITELIST) if (path.startsWith(w)) { chain.doFilter(req, res); return; }

        String auth = req.getHeader("Authorization");
        if (auth == null || !auth.startsWith("Bearer ")) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
            return;
        }
        try {
            Claims c = jwt.parse(auth.substring(7));
            String uid = c.getSubject();
            String role = c.get("role", String.class);
            HttpServletRequest wrapped = new AuthHeaderWrapper(req, uid, role);
            chain.doFilter(wrapped, res);
        } catch (Exception e) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
        }
    }

    /** 注入可信 X-User-Id/X-Role,覆盖外网伪造值。 */
    private static class AuthHeaderWrapper extends HttpServletRequestWrapper {
        private final String uid;
        private final String role;
        AuthHeaderWrapper(HttpServletRequest req, String uid, String role) { super(req); this.uid = uid; this.role = role; }

        @Override
        public String getHeader(String name) {
            if ("X-User-Id".equalsIgnoreCase(name)) return uid;
            if ("X-Role".equalsIgnoreCase(name)) return role;
            return super.getHeader(name);
        }
        @Override
        public Enumeration<String> getHeaders(String name) {
            if ("X-User-Id".equalsIgnoreCase(name)) return Collections.enumeration(List.of(uid));
            if ("X-Role".equalsIgnoreCase(name)) return Collections.enumeration(List.of(role));
            return super.getHeaders(name);
        }
    }
}
