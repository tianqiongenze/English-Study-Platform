package com.esp.learning.auth;

import io.jsonwebtoken.Claims;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.List;

/**
 * JWT 边缘校验(P0-2,ADR-12):learning-java 学情接口鉴权。
 * 此前 LearningController 信任 @RequestHeader X-User-Id,伪造即越权(无任何 Filter)。
 * - 公钥未注入(enabled()==false)时放行(本地/测试无密钥环境)。
 * - 公钥已注入:放行白名单(/actuator/、/healthz、/v3/api-docs、/swagger-ui);其余无有效 Bearer JWT 一律 401。
 * - 校验通过后将 userId 存入 request attribute "userId",Controller 从此取,不再信任 X-User-Id header。
 * 范式对齐 teacher-java JwtAuthFilter。
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE)
public class JwtAuthFilter extends OncePerRequestFilter {

    private static final List<String> WHITELIST = List.of(
            "/actuator/", "/healthz", "/v3/api-docs", "/swagger-ui", "/docs");

    private final JwtValidator jwt;

    public JwtAuthFilter(JwtValidator jwt) {
        this.jwt = jwt;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        if (!jwt.enabled()) {
            chain.doFilter(req, res);
            return;
        }
        String path = req.getRequestURI();
        for (String w : WHITELIST) {
            if (path.startsWith(w)) {
                chain.doFilter(req, res);
                return;
            }
        }
        String auth = req.getHeader("Authorization");
        if (auth == null || !auth.startsWith("Bearer ")) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
            return;
        }
        try {
            Claims c = jwt.parse(auth.substring(7));
            req.setAttribute("userId", c.getSubject());
            req.setAttribute("role", c.get("role", String.class));
            chain.doFilter(req, res);
        } catch (Exception e) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
        }
    }
}
