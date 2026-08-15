package com.esp.web.common;

import io.opentelemetry.api.trace.Span;
import io.opentelemetry.context.Scope;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import io.opentelemetry.sdk.trace.SdkTracerProvider;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;

/**
 * TraceIdUtil 单测:有效 OTel span 返回 traceId;无上下文返回 null;异常不抛出。
 */
class TraceIdUtilTest {

    private static OpenTelemetrySdk sdk;

    @BeforeAll
    static void initSdk() {
        sdk = OpenTelemetrySdk.builder()
                .setTracerProvider(SdkTracerProvider.builder().build())
                .build();
    }

    @AfterAll
    static void closeSdk() {
        if (sdk != null) {
            sdk.close();
        }
    }

    @Test
    void currentTraceId_withActiveSpan_returnsTraceId() {
        Span span = sdk.getTracer("test").spanBuilder("test-span").startSpan();
        try (Scope scope = span.makeCurrent()) {
            String traceId = TraceIdUtil.currentTraceId();
            assertNotNull(traceId);
            assertEquals(span.getSpanContext().getTraceId(), traceId);
        } finally {
            span.end();
        }
    }

    @Test
    void currentTraceId_withoutActiveSpan_returnsNull() {
        assertNull(TraceIdUtil.currentTraceId());
    }

    @Test
    void currentTraceId_neverThrows() {
        // Span.current() 在无 TracerProvider 时返回 invalid span → isValid false → null,不抛异常
        assertNull(TraceIdUtil.currentTraceId());
    }
}
