package com.esp.cache.idempotency;

import com.esp.cache.CacheService;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.Ordered;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * 写接口幂等(P1-8,ADR-幂等):Idempotency-Key header + Redis SETNX,仅对 POST 生效。
 *
 * <p>范式对齐 writing-py grade(main.py:54-81):客户端传 Idempotency-Key,服务端用
 * {@code esp:idem:<biz>:<key>} SETNX 占位。首个请求持有 key 执行业务,响应体回写缓存;
 * 重放请求命中缓存,直接返回上次响应体(免重复写库)。
 *
 * <p>多语言统一:learning-java/teacher-java/bff-java 共用本 Filter(esp-cache 包,
 * scanBasePackages 已含 com.esp.cache);gateway-go(sessions)/writing-py(polish) 各自按同范式实现。
 *
 * <p>降级:CacheService 不可用(feature flag off 或 Redis 故障)时跳过幂等放行,
 * 业务照常跑(幂等是防重保护,不是硬性前置条件)。
 */
@Component
@Order(Ordered.HIGHEST_PRECEDENCE + 2) // 在 JwtAuthFilter(鉴权)之后,Sentinel 之前
public class IdempotencyFilter extends OncePerRequestFilter {

    private static final Logger log = LoggerFactory.getLogger(IdempotencyFilter.class);

    private static final int IDEM_TTL_SECONDS = 300; // 幂等窗口 5min(对齐 writing-py grade)
    private static final String HEADER = "Idempotency-Key";
    private static final String IDEM_CACHE = "esp_idem";

    private final CacheService cacheService;

    public IdempotencyFilter(CacheService cacheService) {
        this.cacheService = cacheService;
    }

    @Override
    protected void doFilterInternal(HttpServletRequest req, HttpServletResponse res, FilterChain chain)
            throws ServletException, IOException {
        if (!"POST".equalsIgnoreCase(req.getMethod())) {
            chain.doFilter(req, res);
            return;
        }
        String idemKey = req.getHeader(HEADER);
        if (idemKey == null || idemKey.isBlank()) {
            // 客户端未传幂等键:向后兼容直接放行(无幂等保护但业务可跑)。
            chain.doFilter(req, res);
            return;
        }

        String bizTag = bizTagFromPath(req.getRequestURI());
        String redisKey = "esp:idem:" + bizTag + ":" + idemKey;

        // 缓存不可用 → 跳过幂等放行(不阻塞写)。
        if (!cacheService.isEnabled()) {
            chain.doFilter(req, res);
            return;
        }

        // 1. SETNX 占位:首个请求抢到 key(防并发重复提交双写)。
        boolean acquired = cacheService.tryIdempotent(redisKey, IDEM_TTL_SECONDS);
        if (!acquired) {
            // 已有同 key 在处理或已处理完 → 重放,尝试返回上次响应体。
            Object cached = cacheService.getOrLoad(IDEM_CACHE, redisKey, () -> null,
                    IDEM_TTL_SECONDS, false);
            if (cached != null) {
                log.info("idempotent replay: biz={} key={}", bizTag, idemKey);
                res.setStatus(HttpStatus.OK.value());
                res.setContentType(MediaType.APPLICATION_JSON_VALUE);
                res.setCharacterEncoding(StandardCharsets.UTF_8.name());
                res.getWriter().write(cached.toString());
                return;
            }
            // 占位中(并发处理)但响应体尚未回写 → 让客户端稍后重试。
            res.setStatus(HttpStatus.CONFLICT.value());
            res.setContentType(MediaType.APPLICATION_JSON_VALUE);
            res.getWriter().write("{\"error\":\"idempotency_in_progress\",\"message\":\"retry later\"}");
            return;
        }

        // 2. 捕获响应体,业务成功后回写幂等缓存。
        IdempotentResponseWrapper wrappedRes = new IdempotentResponseWrapper(res);
        chain.doFilter(req, wrappedRes);

        // 3. 业务成功(2xx)回写缓存;失败(4xx/5xx)删占位允许重试。
        int status = wrappedRes.getStatus();
        String bodyText = wrappedText(wrappedRes);
        if (status >= 200 && status < 300) {
            try {
                cacheService.put(IDEM_CACHE, redisKey, bodyText, IDEM_TTL_SECONDS);
            } catch (Exception e) {
                log.warn("idem write failed (non-fatal): {}", e.getMessage());
            }
        } else {
            try { cacheService.evict(IDEM_CACHE, redisKey); } catch (Exception ignored) {}
        }
    }

    private static String bizTagFromPath(String path) {
        String p = path.startsWith("/api/") ? path.substring(5) : path;
        return p.replace("/", "-");
    }

    private static String wrappedText(IdempotentResponseWrapper w) {
        try {
            byte[] b = w.getCapturedBody();
            return b == null ? "{}" : new String(b, StandardCharsets.UTF_8);
        } catch (Exception e) {
            return "{}";
        }
    }
}
