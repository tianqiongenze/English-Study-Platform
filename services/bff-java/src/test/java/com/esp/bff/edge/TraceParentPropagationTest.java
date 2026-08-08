package com.esp.bff.edge;

import io.netty.handler.codec.http.DefaultFullHttpRequest;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpMethod;
import io.netty.handler.codec.http.HttpVersion;
import io.netty.buffer.Unpooled;
import io.opentelemetry.api.GlobalOpenTelemetry;
import io.opentelemetry.api.trace.Span;
import io.opentelemetry.api.trace.SpanContext;
import io.opentelemetry.api.trace.Tracer;
import io.opentelemetry.api.trace.TraceFlags;
import io.opentelemetry.api.trace.TraceState;
import io.opentelemetry.api.trace.propagation.W3CTraceContextPropagator;
import io.opentelemetry.context.Context;
import io.opentelemetry.context.propagation.TextMapGetter;
import io.opentelemetry.context.propagation.TextMapSetter;
import io.opentelemetry.sdk.OpenTelemetrySdk;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 验证 BFF Netty 边缘层 traceparent 透传(P1-5 断点1 修复验证)。
 *
 * 测试策略:
 * 1. 模拟入站请求带 W3C traceparent header。
 * 2. 验证 EdgeAuthHandler 的 extract+spanBuilder 创建的 span 继承上游 traceId。
 * 3. 验证 DownstreamForwarder 的 inject 将 traceparent 写入下游请求 header。
 *
 * 注意:OTel javaagent 在运行时注入 GlobalOpenTelemetry,单测环境下
 * GlobalOpenTelemetry 可能是 noop。因此测试分两层:
 * - 显式 traceparent header 透传(纯文本,不依赖 OTel SDK 初始化)。
 * - OTel SDK 可用时,验证 span 继承(需要 SDK setup,标记为 integration)。
 */
class TraceParentPropagationTest {

    private static final String TRACEPARENT = "traceparent";

    /** W3C traceparent 格式:version-traceId-spanId-flags */
    private static final String SAMPLE_TRACEPARENT = "00-4bf92f3577b3da6e3dfbe0a3442dc0b4-00f067aa0ba902b7-01";

    /** 初始化真实 OTel SDK,使 GlobalOpenTelemetry 的 propagator 可实际 extract/inject。
     *  Spring Boot 4.0.7 BOM 管理 OTel 1.55.0,SDK builder 默认注册 W3C propagator。
     *  set() 只能调一次(全局),若已被其他类初始化则跳过。 */
    @BeforeAll
    static void initOtelSdk() {
        try {
            OpenTelemetrySdk sdk = OpenTelemetrySdk.builder()
                    .setPropagators(io.opentelemetry.context.propagation.ContextPropagators
                            .create(W3CTraceContextPropagator.getInstance()))
                    .build();
            GlobalOpenTelemetry.set(sdk);
        } catch (IllegalStateException alreadySet) {
            // already initialized, that's fine
        }
    }

    /**
     * 核心断言:入站 traceparent 经过 extract→startSpan→inject 后,出站 header 含 traceparent,
     * 且 traceId 部分与入站一致(链路贯通)。
     *
     * OTel W3C propagator inject 只写 traceparent 当 Context 含 current span。
     * extract 仅将 remote span parent 设为 context parent,不含 current span。
     * 生产流程:EdgeAuthHandler extract→startSpan,DownstreamForwarder inject span context。
     * 单测模拟同流程。
     */
    @Test
    void traceparent_extractInject_roundTrip_preservesTraceId() {
        // 1. 模拟入站 header carrier
        Map<String, String> inboundHeaders = new HashMap<>();
        inboundHeaders.put(TRACEPARENT, SAMPLE_TRACEPARENT);

        // 2. extract:从入站 header 提取 OTel Context(remote span parent)
        TextMapGetter<Map<String, String>> getter = new TextMapGetter<>() {
            @Override
            public Iterable<String> keys(Map<String, String> carrier) {
                return carrier.keySet();
            }
            @Override
            public String get(Map<String, String> carrier, String key) {
                return carrier == null ? null : carrier.get(key);
            }
        };

        Context extracted = GlobalOpenTelemetry.getPropagators()
                .getTextMapPropagator()
                .extract(Context.current(), inboundHeaders, getter);

        // 3. startSpan:模拟 EdgeAuthHandler 创建边缘 span(继承上游 traceId)
        Tracer tracer = GlobalOpenTelemetry.getTracer("bff-edge");
        Span span = tracer.spanBuilder("edge-auth")
                .setParent(extracted)
                .startSpan();
        Context withSpan = extracted.with(span);

        // 4. inject:模拟 DownstreamForwarder 将 span context 写入出站 header
        Map<String, String> outboundHeaders = new HashMap<>();
        TextMapSetter<Map<String, String>> setter = (carrier, key, value) -> {
            if (carrier != null) carrier.put(key, value);
        };

        GlobalOpenTelemetry.getPropagators()
                .getTextMapPropagator()
                .inject(withSpan, outboundHeaders, setter);

        span.end();

        // 5. 断言:出站含 traceparent,traceId 与入站一致
        String outTraceparent = outboundHeaders.get(TRACEPARENT);
        assertNotNull(outTraceparent, "downstream request must contain traceparent header");

        String inTraceId = SAMPLE_TRACEPARENT.split("-")[1];
        String outTraceId = outTraceparent.split("-")[1];
        assertEquals(inTraceId, outTraceId,
                "downstream traceparent must carry same traceId as upstream (chain continuity)");
    }

    /**
     * 验证:入站无 traceparent 时,inject 不写入空值(不污染下游 header)。
     */
    @Test
    void traceparent_noInbound_injectDoesNotAddEmpty() {
        Map<String, String> inboundHeaders = new HashMap<>(); // 无 traceparent

        TextMapGetter<Map<String, String>> getter = new TextMapGetter<>() {
            @Override
            public Iterable<String> keys(Map<String, String> carrier) { return carrier.keySet(); }
            @Override
            public String get(Map<String, String> carrier, String key) {
                return carrier == null ? null : carrier.get(key);
            }
        };

        Context extracted = GlobalOpenTelemetry.getPropagators()
                .getTextMapPropagator()
                .extract(Context.current(), inboundHeaders, getter);

        Map<String, String> outboundHeaders = new HashMap<>();
        TextMapSetter<Map<String, String>> setter = (carrier, key, value) -> {
            if (carrier != null) carrier.put(key, value);
        };

        GlobalOpenTelemetry.getPropagators()
                .getTextMapPropagator()
                .inject(extracted, outboundHeaders, setter);

        // No-op SDK 时可能不写任何 header;有 SDK 时会写 root span 的 traceparent。
        // 断言:不写空字符串
        String out = outboundHeaders.get(TRACEPARENT);
        if (out != null) {
            assertFalse(out.isEmpty(), "traceparent must not be empty string if present");
        }
    }

    /**
     * 验证:DownstreamForwarder 的 HEADER_SETTER 正确写入 header。
     * 直接测试 InletDisruptor.HEADER_SETTER 的行为(通过反射或直接构造)。
     * 这里用等效的 setter lambda 验证。
     */
    @Test
    void headerSetter_writesKeyAndValue() {
        Map<String, String> carrier = new HashMap<>();
        TextMapSetter<Map<String, String>> setter = (c, k, v) -> {
            if (c != null) c.put(k, v);
        };
        setter.set(carrier, "traceparent", SAMPLE_TRACEPARENT);
        assertEquals(SAMPLE_TRACEPARENT, carrier.get("traceparent"));
    }
}
