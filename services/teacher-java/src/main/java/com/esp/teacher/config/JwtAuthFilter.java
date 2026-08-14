package com.esp.teacher.config;

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
 * JWT 边缘校验(D2 修复,ADR-12):teacher-java 教师端接口鉴权。
 * - 公钥未注入(JWT_RSA_PUBLIC_KEY_PEM 空)时**放行**(便于本地/测试环境无密钥运行,避免启动即 401 全挂)。
 * - 公钥已注入:放行 /actuator/** + /healthz;其余无有效 Bearer JWT 一律 401。
 * - 参照 bff-java AuthFilter(OncePerRequestFilter + 白名单 + HIGHEST_PRECEDENCE)。
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
