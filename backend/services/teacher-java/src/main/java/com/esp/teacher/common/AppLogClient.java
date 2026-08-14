package com.esp.teacher.common;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Map;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * AppLogClient —— teacher-java 结构化业务日志异步写 TiDB app_logs(ADR-observability-rebuild)。
 * 复用 learning-java 同名类的设计:异步 + 降级不阻断业务。traceId 从 OTel span 提取。
 */
@Component
public class AppLogClient {

    private static final Logger log = LoggerFactory.getLogger(AppLogClient.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private final JdbcTemplate writeJdbc;
    private final ThreadPoolExecutor executor;
    private final AtomicLong dropped = new AtomicLong();

    @Value("${esp.app-log.enabled:true}")
    private boolean enabled;

    public AppLogClient(JdbcTemplate writeJdbc) {
        this.writeJdbc = writeJdbc;
        this.executor = new ThreadPoolExecutor(
                2, 4, 60, TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(4096),
                r -> { Thread t = new Thread(r, "app-log-writer"); t.setDaemon(true); return t; },
                (r, e) -> dropped.incrementAndGet()
        );
    }

    public void log(String level, String event, Long userId, Map<String, Object> payload) {
        if (!enabled) return;
        String traceId = currentTraceId();
        try {
            executor.submit(() -> doInsert("teacher", level, event, traceId, userId, payload));
        } catch (Exception ex) {
            dropped.incrementAndGet();
        }
    }

    private void doInsert(String service, String level, String event, String traceId, Long userId, Map<String, Object> payload) {
        try {
            String payloadJson = payload == null ? null : MAPPER.writeValueAsString(payload);
            writeJdbc.update(
                    "INSERT INTO app_logs (ts, service, level, event, trace_id, user_id, payload) " +
                            "VALUES (?, ?, ?, ?, ?, ?, CAST(? AS JSON))",
                    Instant.now(), service, level, event, traceId, userId, payloadJson
            );
        } catch (Exception ex) {
            log.warn("app_logs insert failed (degraded): {}", ex.getMessage());
        }
    }

    private String currentTraceId() {
        try {
            io.opentelemetry.api.trace.SpanContext ctx = io.opentelemetry.api.trace.Span.current().getSpanContext();
            return ctx.isValid() ? ctx.getTraceId() : null;
        } catch (Throwable t) {
            return null;
        }
    }
}
