package com.esp.web.common;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.Map;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 结构化业务日志异步写 app_logs 共享客户端(收编 bff/learning/teacher 三份重复 AppLogClient)。
 *
 * <p>设计(合并三份实现的最大公共语义):
 * <ul>
 *   <li>异步:专用 ThreadPoolExecutor(核心 2 / 最大 4 / 队列 4096),不阻塞业务主路径。</li>
 *   <li>降级:提交失败(队列满)仅计数 dropped;TiDB/CRDB 不可用时仅 log.warn,不抛异常。</li>
 *   <li>traceId:从 OTel 当前 span 提取(未启用 agent 时为 null),统一走 {@link TraceIdUtil}。</li>
 *   <li>service 名:由 {@code esp.web.common.app-log.service} 属性化;未配置时回退
 *       {@code spring.application.name}。提供 5 参重载供调用方临时覆盖(learning 现有调用)。</li>
 *   <li>DB 连接:默认注入主库 JdbcTemplate(写库方式);可通过
 *       {@code esp.web.common.app-log.jdbc-template-name} 指定命名 bean(如 learning 的 sharedJdbcTemplate)。</li>
 *   <li>SQL 模板:默认 {@value #MYSQL_INSERT_SQL}(TiDB/MySQL wire,CAST AS JSON);
 *       CRDB(PG wire)自动选 {@value #CRDB_INSERT_SQL}(CAST AS JSONB);
 *       也可经 {@code esp.web.common.app-log.sql} 整体覆盖。app_logs 恒在 esp_shared 库,
 *       用两段式 esp_shared.app_logs,使 bff/teacher 走路由池(esp_auth/esp_teacher)时跨库可达。</li>
 * </ul>
 *
 * <p>典型用法:
 * <pre>
 *   appLogClient.log("INFO", "auth.login", userId, Map.of("ip", ip));            // service 取属性
 *   appLogClient.log("learning", "INFO", "behavior.click", userId, payload);     // 显式 service 覆盖
 * </pre>
 */
public class AppLogClient {

    private static final Logger log = LoggerFactory.getLogger(AppLogClient.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    /** 缺省 INSERT(TiDB/MySQL wire):CAST(? AS JSON);app_logs 用两段式 esp_shared.app_logs 跨库可达。 */
    public static final String MYSQL_INSERT_SQL =
            "INSERT INTO esp_shared.app_logs (ts, service, level, event, trace_id, user_id, payload) " +
                    "VALUES (?, ?, ?, ?, ?, ?, CAST(? AS JSON))";

    /** CockroachDB(PG wire):JSON -> JSONB。 */
    public static final String CRDB_INSERT_SQL =
            "INSERT INTO esp_shared.app_logs (ts, service, level, event, trace_id, user_id, payload) " +
                    "VALUES (?, ?, ?, ?, ?, ?, CAST(? AS JSONB))";

    private final JdbcTemplate writeJdbc;
    private final String insertSql;
    private final ThreadPoolExecutor executor;
    private final AtomicLong dropped = new AtomicLong();
    private final String defaultService;
    private final boolean enabled;

    public AppLogClient(JdbcTemplate writeJdbc, String defaultService, boolean enabled) {
        this(writeJdbc, MYSQL_INSERT_SQL, defaultService, enabled);
    }

    public AppLogClient(JdbcTemplate writeJdbc, String insertSql, String defaultService, boolean enabled) {
        this.writeJdbc = writeJdbc;
        this.insertSql = insertSql;
        this.defaultService = defaultService;
        this.enabled = enabled;
        this.executor = new ThreadPoolExecutor(
                2, 4, 60, TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(4096),
                r -> {
                    Thread t = new Thread(r, "app-log-writer");
                    t.setDaemon(true);
                    return t;
                },
                (r, e) -> dropped.incrementAndGet() // 队列满丢弃,保业务
        );
    }

    /** 异步写一条结构化日志,service 取 {@code esp.web.common.app-log.service}(或 spring.application.name)。 */
    public void log(String level, String event, Long userId, Map<String, Object> payload) {
        log(defaultService, level, event, userId, payload);
    }

    /** 异步写一条结构化日志,显式指定 service(learning 现有调用形态)。level: INFO/WARN/ERROR;event: auth.login/behavior.xxx/... */
    public void log(String service, String level, String event, Long userId, Map<String, Object> payload) {
        if (!enabled || writeJdbc == null) {
            return;
        }
        String traceId = TraceIdUtil.currentTraceId();
        try {
            executor.submit(() -> doInsert(service, level, event, traceId, userId, payload));
        } catch (Exception ex) {
            // 提交失败(队列满)—— 降级
            dropped.incrementAndGet();
        }
    }

    private void doInsert(String service, String level, String event, String traceId, Long userId,
                          Map<String, Object> payload) {
        try {
            String payloadJson = payload == null ? null : MAPPER.writeValueAsString(payload);
            // Timestamp 而非 Instant:PG JDBC 42.7.x setObject(Instant) 无法推断类型(SQLState 07006),
            // CRDB(PG wire)模式下 app_logs 必失败;Timestamp 双模式通用。
            writeJdbc.update(
                    insertSql,
                    Timestamp.from(Instant.now()), service, level, event, traceId, userId, payloadJson
            );
        } catch (Exception ex) {
            // 降级:仅 warn,不阻断业务
            log.warn("app_logs insert failed (degraded): {}", ex.getMessage());
        }
    }

    /** 队列满被丢弃的日志条数(可观测)。 */
    public long getDroppedCount() {
        return dropped.get();
    }
}
