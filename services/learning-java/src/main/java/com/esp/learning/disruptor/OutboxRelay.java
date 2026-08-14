package com.esp.learning.disruptor;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

import java.sql.Timestamp;
import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * P1-10 outbox 重投器:异步轮询 {@code learning_event_outbox} 表 pending 行,补投到 learning_events。
 *
 * <p>触发场景:Disruptor RingBuffer 满时,关键事件(security/教务类)落 outbox 兜底(见
 * {@link LearningEventDisruptor#fallbackToOutbox})。本 Relay 周期扫描 pending 行,直接 INSERT
 * learning_events(绕过 Disruptor,避免再次 DROP 形成兜底死循环),成功后标记 done,失败递增 retry_count
 * 并指数退避 next_retry_at。
 *
 * <p>装配条件:{@code esp.disruptor.enabled=true}(与 LearningEventDisruptor 同进程);worker 模式下
 * 入口侧 disruptor disabled,本 Relay 不装配(outbox 由 worker 侧重投 —— 单节点 RD 入口仍带 Disruptor,
 * 即入口承担重投;多节点可按 ESP_ROLE 拆分)。
 *
 * <p>退避策略:retry_count 0→1s,1→5s,2→30s,3+→5min(封顶);超过 MAX_RETRY(8)标记 dead 防无限重试。
 */
@Component
@ConditionalOnProperty(name = "esp.disruptor.enabled", havingValue = "true", matchIfMissing = true)
public class OutboxRelay {

    private static final Logger log = LoggerFactory.getLogger(OutboxRelay.class);

    private static final int BATCH = 200;
    private static final int MAX_RETRY = 8;

    private final JdbcTemplate writeJdbc;

    public OutboxRelay(@Value("${esp.disruptor.enabled:true}") boolean enabled,
                       JdbcTemplate writeJdbc) {
        this.writeJdbc = writeJdbc;
    }

    /** 每 2s 扫一次 pending(单节点 RD,2s 平衡及时性与 TiDB 负载)。 */
    @Scheduled(fixedDelayString = "${esp.outbox.relay-interval-ms:2000}")
    public void drain() {
        List<Map<String, Object>> rows;
        try {
            rows = writeJdbc.queryForList(
                    "SELECT id, user_id, event_type, payload, retry_count FROM learning_event_outbox " +
                            "WHERE status = 'pending' AND next_retry_at <= ? ORDER BY id LIMIT ?",
                    Timestamp.from(Instant.now()), BATCH);
        } catch (Exception ex) {
            log.warn("outbox poll failed (non-fatal): {}", ex.getMessage());
            return;
        }
        if (rows.isEmpty()) return;
        int ok = 0, fail = 0;
        for (Map<String, Object> row : rows) {
            long id = ((Number) row.get("id")).longValue();
            long userId = ((Number) row.get("user_id")).longValue();
            String eventType = String.valueOf(row.get("event_type"));
            Object payload = row.get("payload");
            int retry = row.get("retry_count") == null ? 0 : ((Number) row.get("retry_count")).intValue();
            try {
                writeJdbc.update(
                        "INSERT INTO learning_events (user_id, event_type, payload, created_at) VALUES (?,?,?,?)",
                        userId, eventType, payload, Timestamp.from(Instant.now()));
                writeJdbc.update(
                        "UPDATE learning_event_outbox SET status='done' WHERE id=?", id);
                ok++;
            } catch (Exception ex) {
                fail++;
                int nextRetry = retry + 1;
                if (nextRetry >= MAX_RETRY) {
                    writeJdbc.update(
                            "UPDATE learning_event_outbox SET status='dead', retry_count=? WHERE id=?",
                            nextRetry, id);
                    log.error("outbox row dead (max retry): id={} type={}", id, eventType);
                } else {
                    long backoffMs = backoffMs(nextRetry);
                    Timestamp nextAt = Timestamp.from(Instant.now().plusMillis(backoffMs));
                    writeJdbc.update(
                            "UPDATE learning_event_outbox SET retry_count=?, next_retry_at=? WHERE id=?",
                            nextRetry, nextAt, id);
                }
            }
        }
        if (ok > 0 || fail > 0) {
            log.info("outbox relay: ok={} fail={} pending_scanned={}", ok, fail, rows.size());
        }
    }

    /** 指数退避:1→1s,2→5s,3→30s,4+→5min。 */
    private long backoffMs(int retry) {
        return switch (retry) {
            case 1 -> 1_000L;
            case 2 -> 5_000L;
            case 3 -> 30_000L;
            default -> 300_000L;
        };
    }
}
