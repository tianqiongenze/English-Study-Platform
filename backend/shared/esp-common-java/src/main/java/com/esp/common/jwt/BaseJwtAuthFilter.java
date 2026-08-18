package com.esp.common.jwt;

import io.jsonwebtoken.Claims;
import io.jsonwebtoken.JwtException;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.Collections;
import java.util.Enumeration;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

/**
 * JWT 边缘鉴权 Filter(学习/教师服务共用,收编自 learning-java JwtAuthFilter + teacher-java JwtAuthFilter)。
 * <p>
 * 与 bff-java AuthFilter 的差异:本 Filter 仅校验公钥 JWT,不签发;
 * 使用方注入一个实现了 {@link BaseJwtValidator} 的 {@code @Component} 即可。
 * <p>
 * 白名单常量来自 {@link com.esp.common.health.HealthConfig#COMMON_WHITELIST},
 * 服务可追加自有白名单(通过构造器传入 additionalWhitelist)。
 *
 * <p>收编自 learning-java auth/JwtAuthFilter + teacher-java config/JwtAuthFilter,
 * 消除三服务间几乎相同的 Filter 逻辑(白名单/bearer 校验/userId 注入)。
 */
@Order(Ordered.HIGHEST_PRECEDENCE)
public abstract class BaseJwtAuthFilter extends OncePerRequestFilter {

    protected final BaseJwtValidator jwt;
    protected final List<String> whitelist;

    /**
     * @param jwt   公钥校验器
     * @param additionalWhitelist 服务自有白名单路径(可空,在通用白名单基础上追加)
     */
    protected BaseJwtAuthFilter(BaseJwtValidator jwt, List<String> additionalWhitelist) {
        this.jwt = jwt;
        List<String> combined = new java.util.ArrayList<>(com.esp.common.health.HealthConfig.COMMON_WHITELIST);
        if (additionalWhitelist != null) {
            combined.addAll(additionalWhitelist);
        }
        this.whitelist = Collections.unmodifiableList(combined);
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        if (!jwt.enabled()) {
            chain.doFilter(req, res);
            return;
        }
        String path = req.getRequestURI();
        for (String w : whitelist) {
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
            String uid = c.getSubject();
            String role = c.get("role", String.class);
            HttpServletRequest wrapped = new AuthHeaderWrapper(req, uid, role);
            chain.doFilter(wrapped, res);
        } catch (Exception e) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
        }
    }

    /**
     * 注入可信 X-User-Id / X-Role header,覆盖外网伪造值。
     * 同时补充 getHeaderNames() 以支持 ProxyController 枚举转发(坑 BFF X-User-Id 枚举缺失)。
     */
    protected static class AuthHeaderWrapper extends jakarta.servlet.http.HttpServletRequestWrapper {
        private final String uid;
        private final String role;

        AuthHeaderWrapper(HttpServletRequest req, String uid, String role) {
            super(req);
            this.uid = uid;
            this.role = role;
        }

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

        @Override
        public Enumeration<String> getHeaderNames() {
            Set<String> names = new LinkedHashSet<>();
            Enumeration<String> orig = super.getHeaderNames();
            while (orig.hasMoreElements()) names.add(orig.nextElement());
            names.add("X-User-Id");
            names.add("X-Role");
            return Collections.enumeration(names);
        }
    }
}
