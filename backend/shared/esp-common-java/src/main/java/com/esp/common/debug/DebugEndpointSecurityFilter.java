package com.esp.common.debug;

import com.esp.common.jwt.BaseJwtValidator;
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
import java.util.Set;

/**
 * Actuator 深度诊断端点鉴权 Filter(全语言统一深度诊断面板 · Step2 Java 侧)。
 * <p>
 * 背景:Spring Boot Actuator 的 {@code env / loggers / threaddump / heapdump / beans / mappings}
 * 属敏感运行时诊断信息,默认即便 {@code exposure.include} 暴露也不应匿名可达。
 * {@link com.esp.common.health.HealthConfig#COMMON_WHITELIST} 当前对整个 {@code /actuator/} 前缀放行,
 * 是为 k8s 探针 + Prometheus 抓取的 {@code /actuator/health*} / {@code /actuator/prometheus} 之便。
 * <p>
 * 本 Filter 在通用 Jwt Filter 之前(HIGHEST_PRECEDENCE - 1)收口:
 * <ul>
 *   <li>命中 {@link #SENSITIVE_ACTUATOR_PATHS} 的请求必须携带有效 admin-role JWT,否则 401/403;</li>
 *   <li>其余 actuator 路径(health/prometheus/info)保持现有白名单语义,不受影响。</li>
 * </ul>
 * <p>鉴权策略与 admin-go {@code requireAdmin} 对齐:校验公钥 RS256 JWT 且 {@code role=admin}。
 * BFF 代理(admin-go / bff-java debug 路由)转发时会携带原始 admin token,故内部服务侧二次校验合法。
 *
 * <p>ASV 铁律落实:Spring Boot 4.0.7 actuator 端点真实存在性已校验(javap HeapDumpWebEndpoint /
 * ThreadDumpEndpoint / LoggersEndpoint / EnvironmentEndpoint / BeansEndpoint / MappingsEndpoint),
 * 路径 {@code /actuator/<id>} 由 EndpointWebMapping 自动注册,无需手写 controller。
 */
@Order(Ordered.HIGHEST_PRECEDENCE - 1)
@Component
public class DebugEndpointSecurityFilter extends OncePerRequestFilter {

    /** Actuator 深度诊断敏感端点(health/prometheus/info 不在此列,保持匿名可达)。 */
    public static final Set<String> SENSITIVE_ACTUATOR_PATHS = Set.of(
            "/actuator/env",
            "/actuator/loggers",
            "/actuator/threaddump",
            "/actuator/heapdump",
            "/actuator/beans",
            "/actuator/mappings"
    );

    private final BaseJwtValidator jwt;

    public DebugEndpointSecurityFilter(BaseJwtValidator jwt) {
        this.jwt = jwt;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        String path = req.getRequestURI();
        // 仅对敏感 actuator 深度诊断端点强制 admin JWT;其余请求直通下游 Jwt Filter。
        if (!SENSITIVE_ACTUATOR_PATHS.contains(path)) {
            chain.doFilter(req, res);
            return;
        }
        // 若鉴权关闭(本地/测试),放行由下游白名单语义决定(与 BaseJwtAuthFilter.enabled() 一致)。
        if (!jwt.enabled()) {
            chain.doFilter(req, res);
            return;
        }
        String auth = req.getHeader("Authorization");
        if (auth == null || !auth.startsWith("Bearer ")) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
            res.setContentType("application/json");
            res.getWriter().write("{\"error\":\"unauthorized: admin token required\"}");
            return;
        }
        try {
            Claims c = jwt.parse(auth.substring(7));
            String role = c.get("role", String.class);
            if (!"admin".equals(role)) {
                res.setStatus(HttpStatus.FORBIDDEN.value());
                res.setContentType("application/json");
                res.getWriter().write("{\"error\":\"forbidden: admin role required\"}");
                return;
            }
        } catch (Exception e) {
            res.setStatus(HttpStatus.UNAUTHORIZED.value());
            res.setContentType("application/json");
            res.getWriter().write("{\"error\":\"invalid token\"}");
            return;
        }
        chain.doFilter(req, res);
    }
}
