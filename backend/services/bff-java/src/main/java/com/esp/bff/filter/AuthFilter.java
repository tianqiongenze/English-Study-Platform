package com.esp.bff.filter;

import com.esp.bff.auth.JwtService;
import com.esp.common.jwt.BaseJwtAuthFilter;
import com.esp.common.jwt.BaseJwtValidator;
import io.jsonwebtoken.Claims;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.Cookie;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.util.List;

/**
 * BFF JWT 边缘校验 Filter(ADR-12 安全,Web MVC 版,替代 SCG GlobalFilter)。
 * <p>
 * 继承 {@link BaseJwtAuthFilter}(esp-common-java 共享包),追加 BFF 特有逻辑:
 * <ul>
 *   <li>esp_token httpOnly cookie 回落(Bearer header 优先)</li>
 *   <li>BFF 特有白名单:/.well-known/、/openapi-go-rust.yaml</li>
 * </ul>
 * <p>
 * 注意:本 Filter 仅服务 Spring MVC 路径(如 /api/auth/**);
 * Netty 边缘路径(转发请求)由 {@link com.esp.bff.edge.EdgeAuthHandler} 处理。
 */
@Component
public class AuthFilter extends BaseJwtAuthFilter {

    /** BFF 特有白名单:在通用白名单(HealthConfig.COMMON_WHITELIST)基础上追加。 */
    private static final List<String> BFF_WHITELIST = List.of(
            "/api/auth/",
            "/.well-known/",
            "/openapi-go-rust.yaml"
    );

    private final JwtService jwt;

    public AuthFilter(JwtService jwt, BaseJwtValidator jwtValidator) {
        super(jwtValidator, BFF_WHITELIST);
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
        // 白名单放行(继承自 BaseJwtAuthFilter 逻辑,提取到方法以便 cookie 回落)
        for (String w : whitelist) {
            if (path.startsWith(w)) {
                chain.doFilter(req, res);
                return;
            }
        }

        // P1-13:Bearer 优先,无 header 时回落 esp_token httpOnly cookie(浏览器同源自动携带)。
        // XSS 防的是前端 JS 读 token;服务端两种凭证等价校验。
        String auth = req.getHeader("Authorization");
        String token = null;
        if (auth != null && auth.startsWith("Bearer ")) {
            token = auth.substring(7);
        } else if (req.getCookies() != null) {
            for (Cookie c : req.getCookies()) {
                if ("esp_token".equals(c.getName()) && !c.getValue().isBlank()) {
                    token = c.getValue();
                    break;
                }
            }
        }
        if (token == null) {
            res.setStatus(401);
            return;
        }
        try {
            Claims c = jwt.parse(token);
            String uid = c.getSubject();
            String role = c.get("role", String.class);
            jakarta.servlet.http.HttpServletRequest wrapped = new AuthHeaderWrapper(req, uid, role);
            chain.doFilter(wrapped, res);
        } catch (Exception e) {
            res.setStatus(401);
        }
    }
}
