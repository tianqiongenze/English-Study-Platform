package com.esp.web.common;

import jakarta.servlet.http.HttpServletRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.client.RestClientException;
import org.springframework.web.method.HandlerMethod;

import java.util.ArrayList;
import java.util.List;

/**
 * 全局异常处理共享类(收编 bff/learning/teacher 三服务重复的 GlobalExceptionHandler)。
 *
 * <p>统一错误 envelope {@code {code, message, traceId}}(响应体类型为 {@link ApiError} 单一真源)。
 *
 * <p><b>扫描范围属性化</b>:Spring 的 {@code @RestControllerAdvice(basePackages = "...")} 注解属性
 * 是编译期常量,无法从 {@code ${...}} 占位符解析。故本类注解为全局 advice,实际包范围在运行时由
 * {@link #shouldHandle(HttpServletRequest, HandlerMethod)} 守卫控制:
 * <ul>
 *   <li>{@code esp.web.common.base-packages}(逗号分隔)—— 只处理控制器类包名前缀匹配该列表的异常;
 *       配置为空则处理全部。actuator 控制器位于 {@code org.springframework.boot.actuate.*},
 *       与业务包(com.esp.bff/com.esp.learning/com.esp.teacher)前缀天然不匹配,自动白名单。</li>
 *   <li>{@code esp.web.common.exclude-path-prefixes}(逗号分隔,默认 {@code /actuator})——
 *       请求路径前缀命中即放行(旁路防线,覆盖 HandlerMethod 为 null 等边界)。</li>
 * </ul>
 * 守卫不命中时<span style="text-decoration:underline">重抛原异常</span>,交回框架默认处理
 * (actuator 自身的错误处理 / 容器 500),避免污染探针/指标端点。
 *
 * <p>处理器:
 * <ul>
 *   <li>{@link RestClientException} -> 503 {@code UPSTREAM_UNAVAILABLE}(下游服务不可达/超时,
 *       保留 bff 版独有 handler)。</li>
 *   <li>{@link Exception} -> 500 {@code INTERNAL_ERROR}(兜底,不泄漏内部堆栈)。</li>
 * </ul>
 */
@RestControllerAdvice
public class GlobalExceptionHandler {

    private static final Logger log = LoggerFactory.getLogger(GlobalExceptionHandler.class);

    private final List<String> basePackages;
    private final List<String> excludePathPrefixes;

    public GlobalExceptionHandler(List<String> basePackages, List<String> excludePathPrefixes) {
        this.basePackages = basePackages == null ? new ArrayList<>() : basePackages;
        this.excludePathPrefixes = excludePathPrefixes == null ? List.of("/actuator") : excludePathPrefixes;
    }

    /** 下游服务不可达/超时(ProxyController 不再内联 catch,REST 调用异常统一落这里 → 503)。 */
    @ExceptionHandler(RestClientException.class)
    public ResponseEntity<ApiError> handleUpstreamUnavailable(RestClientException ex,
                                                              HttpServletRequest request,
                                                              HandlerMethod handlerMethod) {
        if (!shouldHandle(request, handlerMethod)) {
            throw ex;
        }
        String traceId = TraceIdUtil.currentTraceId();
        log.warn("upstream call failed path={} traceId={} err={}", request.getRequestURI(), traceId, ex.getMessage());
        return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE)
                .body(ApiError.of("UPSTREAM_UNAVAILABLE", "upstream unavailable", traceId));
    }

    /** 兜底:未预期异常统一 500 + envelope,不泄漏堆栈。 */
    @ExceptionHandler(Exception.class)
    public ResponseEntity<ApiError> handleException(Exception ex, HttpServletRequest request,
                                                    HandlerMethod handlerMethod) throws Exception {
        if (!shouldHandle(request, handlerMethod)) {
            throw ex;
        }
        String traceId = TraceIdUtil.currentTraceId();
        log.error("unhandled exception path={} traceId={}", request.getRequestURI(), traceId, ex);
        return ResponseEntity.status(HttpStatus.INTERNAL_SERVER_ERROR)
                .body(ApiError.of("INTERNAL_ERROR", "internal server error", traceId));
    }

    /**
     * 判定本次异常是否落在本 advice 的职责范围:
     * 1) 请求路径命中 excludePathPrefixes(如 /actuator)→ 放行;
     * 2) basePackages 非空且控制器类包名前缀不匹配 → 放行;
     * 3) 其余情况 → 处理。
     */
    private boolean shouldHandle(HttpServletRequest request, HandlerMethod handlerMethod) {
        String uri = request.getRequestURI();
        for (String prefix : excludePathPrefixes) {
            if (prefix != null && !prefix.isEmpty() && uri.startsWith(prefix)) {
                return false;
            }
        }
        if (basePackages.isEmpty()) {
            return true;
        }
        if (handlerMethod == null) {
            return false; // 无控制器上下文(interceptor 等)且配置了包范围 → 不处理
        }
        String pkg = handlerMethod.getBeanType().getPackageName();
        for (String bp : basePackages) {
            if (bp != null && !bp.isEmpty() && pkg.startsWith(bp)) {
                return true;
            }
        }
        return false;
    }
}
