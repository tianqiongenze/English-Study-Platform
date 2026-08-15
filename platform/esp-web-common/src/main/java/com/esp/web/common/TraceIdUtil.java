package com.esp.web.common;

import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanContext;

/**
 * OTel traceId 提取工具(收编三服务 GlobalExceptionHandler/AppLogClient 中重复的 currentTraceId())。
 *
 * <p>从 {@link Span#current()} 取当前 span 的 traceId;上下文无效或 OTel SDK 未注入时返回 null,
 * 不抛异常(调用方按"无 traceId"降级,不阻断业务)。
 */
public final class TraceIdUtil {

    private TraceIdUtil() {
    }

    public static String currentTraceId() {
        try {
            SpanContext ctx = Span.current().getSpanContext();
            return ctx.isValid() ? ctx.getTraceId() : null;
        } catch (Throwable t) {
            return null; // OTel SDK 未注入
        }
    }
}
