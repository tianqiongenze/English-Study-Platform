package com.esp.learning.common;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.scheduling.annotation.Async;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.Map;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * AppLogClient —— 结构化业务日志异步写入 TiDB app_logs 表(ADR-observability-rebuild)。
 *
 * <p>设计:
 * <ul>
 *   <li>异步:专用 ThreadPoolExecutor(核心 2 / 最大 4 / 队列 4096),不阻塞业务主路径。</li>
 *   <li>降级:TiDB 不可用时仅 log.warn,不抛异常(参考 esp-cache 降级模式)。</li>
 *   <li>traceId:从 OTel 当前 span 提取(若 OTel agent 已注入);未启用时为 null。</li>
 *   <li>写主库:app_logs 走 WRITE 数据源(JdbcTemplate 注入 primary)。</li>
 * </ul>
 *
 * <p>典型用法:
 * <pre>
 *   appLogClient.log("learning", "INFO", "learning.event", userId, Map.of("kp_id", kpId, "correct", true, "duration_ms", 1200));
 * </pre>
 */
@Component
public class AppLogClient {

    private static final Logger log = LoggerFactory.getLogger(AppLogClient.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private final org.springframework.jdbc.core.JdbcTemplate writeJdbc;
    private final ThreadPoolExecutor executor;
    private final AtomicLong dropped = new AtomicLong();

    @Value("${esp.app-log.enabled:true}")
    private boolean enabled;

    public AppLogClient(@Qualifier("sharedJdbcTemplate") org.springframework.jdbc.core.JdbcTemplate writeJdbc) {
        this.writeJdbc = writeJdbc;
        this.executor = new ThreadPoolExecutor(
                2, 4, 60, TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(4096),
                r -> {
                    Thread t = new Thread(r, "app-log-writer");
                    t.setDaemon(true);
                    return t;
                },
                (r, e) -> { dropped.incrementAndGet(); }  // 队列满丢弃,保业务
        );
    }

    /** 异步写一条结构化日志。level: INFO/WARN/ERROR;event: auth.login/writing.grade/... */
    @Async
    public void log(String service, String level, String event, Long userId, Map<String, Object> payload) {
        if (!enabled) return;
        String traceId = currentTraceId();
        try {
            executor.submit(() -> doInsert(service, level, event, traceId, userId, payload));
        } catch (Exception ex) {
            // 提交失败(队列满)—— 降级
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
            // 降级:仅 warn,不阻断业务
            log.warn("app_logs insert failed (degraded): {}", ex.getMessage());
        }
    }

    /** 提取 OTel 当前 span 的 traceId;未启用 agent 时返回 null。 */
    private String currentTraceId() {
        try {
            io.opentelemetry.api.trace.Span span = io.opentelemetry.api.trace.Span.current();
            io.opentelemetry.api.trace.SpanContext ctx = span.getSpanContext();
            return ctx.isValid() ? ctx.getTraceId() : null;
        } catch (Throwable t) {
            return null;  // OTel SDK 未注入
        }
    }

    public long getDroppedCount() { return dropped.get(); }
}
