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
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * P1-10 outbox 重投器:异步轮询 {@code learning_event_outbox} 表 pending/failed 行,补投到 learning_events。
 *
 * <p>触发场景:
 * <ol>
 *   <li>Disruptor RingBuffer 满时,关键事件(security/教务类)落 outbox 兜底(见
 *       {@link LearningEventDisruptor#fallbackToOutbox},status='pending')。</li>
 *   <li>G-06:DbLearningEventConsumer batch 落库失败时,整批改写 outbox(status='failed',见
 *       {@link DbLearningEventConsumer#routeToOutbox})。</li>
 * </ol>
 * 本 Relay 周期扫描两状态行,直接批量 INSERT learning_events(绕过 Disruptor,避免再次 DROP 形成
 * 兜底死循环),成功后批量标记 done,失败递增 retry_count 并指数退避 next_retry_at。
 *
 * <p>装配条件:{@code esp.disruptor.enabled=true}(与 LearningEventDisruptor 同进程);worker 模式下
 * 入口侧 disruptor disabled,本 Relay 不装配(outbox 由 worker 侧重投 —— 单节点 RD 入口仍带 Disruptor,
 * 即入口承担重投;多节点可按 ESP_ROLE 拆分)。
 *
 * <p>退避策略:retry_count 0→1s,1→5s,2→30s,3+→5min(封顶);超过 MAX_RETRY(8)标记 dead 防无限重试。
 *
 * <p>G-06 修复:原逐行 2 次 DB 往返(INSERT + UPDATE)改批量 batchUpdate,BATCH=200 行由 400 次
 * 往返降到 ~3 次(批量 INSERT + 批量 done + 批量退避/死信)。
 */
@Component
@ConditionalOnProperty(name = "esp.disruptor.enabled", havingValue = "true", matchIfMissing = true)
public class OutboxRelay {

    private static final Logger log = LoggerFactory.getLogger(OutboxRelay.class);

    private static final int BATCH = 200;
    private static final int MAX_RETRY = 8;

    /** P1-8:message_id 透传到 learning_events,ON CONFLICT 去重(防止 outbox 重投产生重复行)。 */
    private static final String INSERT_EVENTS_SQL =
            "INSERT INTO learning_events (user_id, event_type, payload, created_at, message_id) " +
                    "VALUES (?,?,?,?,?) ON CONFLICT (message_id) DO NOTHING";
    private static final String MARK_DONE_SQL =
            "UPDATE learning_event_outbox SET status='done' WHERE id=?";
    private static final String MARK_DEAD_SQL =
            "UPDATE learning_event_outbox SET status='dead', retry_count=? WHERE id=?";
    private static final String BACKOFF_SQL =
            "UPDATE learning_event_outbox SET retry_count=?, next_retry_at=? WHERE id=?";

    private final JdbcTemplate writeJdbc;

    public OutboxRelay(@Value("${esp.disruptor.enabled:true}") boolean enabled,
                       JdbcTemplate writeJdbc) {
        this.writeJdbc = writeJdbc;
    }

    /** 每 2s 扫一次 pending/failed(单节点 RD,2s 平衡及时性与 TiDB 负载)。 */
    @Scheduled(fixedDelayString = "${esp.outbox.relay-interval-ms:2000}")
    public void drain() {
        List<Map<String, Object>> rows;
        try {
            rows = writeJdbc.queryForList(
                    "SELECT id, message_id, user_id, event_type, payload, retry_count FROM learning_event_outbox " +
                            "WHERE status IN ('pending','failed') AND next_retry_at <= ? ORDER BY id LIMIT ?",
                    Timestamp.from(Instant.now()), BATCH);
        } catch (Exception ex) {
            log.warn("outbox poll failed (non-fatal): {}", ex.getMessage());
            return;
        }
        if (rows.isEmpty()) return;

        // 1) 批量 INSERT learning_events(替代逐行 N+1)。
        List<Object[]> insertArgs = new ArrayList<>(rows.size());
        for (Map<String, Object> row : rows) {
            insertArgs.add(new Object[]{
                    ((Number) row.get("user_id")).longValue(),
                    String.valueOf(row.get("event_type")),
                    row.get("payload"),
                    Timestamp.from(Instant.now()),
                    row.get("message_id")
            });
        }
        boolean inserted;
        try {
            writeJdbc.batchUpdate(INSERT_EVENTS_SQL, insertArgs);
            inserted = true;
        } catch (Exception ex) {
            inserted = false;
            log.warn("outbox batch insert failed: {} rows err={}", rows.size(), ex.getMessage());
        }

        // 2) 成功行批量标记 done;失败行批量退避(指数退避)或 dead(超 MAX_RETRY)。
        List<Object[]> doneArgs = new ArrayList<>();
        List<Object[]> deadArgs = new ArrayList<>();
        List<Object[]> backoffArgs = new ArrayList<>();
        for (Map<String, Object> row : rows) {
            long id = ((Number) row.get("id")).longValue();
            if (inserted) {
                doneArgs.add(new Object[]{id});
                continue;
            }
            int retry = row.get("retry_count") == null ? 0 : ((Number) row.get("retry_count")).intValue();
            int nextRetry = retry + 1;
            if (nextRetry >= MAX_RETRY) {
                deadArgs.add(new Object[]{nextRetry, id});
                log.error("outbox row dead (max retry): id={}", id);
            } else {
                long backoffMs = backoffMs(nextRetry);
                backoffArgs.add(new Object[]{nextRetry, Timestamp.from(Instant.now().plusMillis(backoffMs)), id});
                log.warn("outbox row retry id={} nextRetry={}", id, nextRetry);
            }
        }
        if (!doneArgs.isEmpty()) {
            try {
                writeJdbc.batchUpdate(MARK_DONE_SQL, doneArgs);
            } catch (Exception ex) {
                // done 标记失败 → 下周期该行仍 pending 会重复 INSERT(duplicate 风险,原逐行实现同隐患);
                // 记日志,不打断本周期其余处理。
                log.error("outbox mark-done batch FAILED (duplicate risk next cycle): {} rows err={}",
                        doneArgs.size(), ex.toString());
            }
        }
        if (!deadArgs.isEmpty()) {
            try {
                writeJdbc.batchUpdate(MARK_DEAD_SQL, deadArgs);
            } catch (Exception ex) {
                log.error("outbox mark-dead batch FAILED: {} rows err={}", deadArgs.size(), ex.toString());
            }
        }
        if (!backoffArgs.isEmpty()) {
            try {
                writeJdbc.batchUpdate(BACKOFF_SQL, backoffArgs);
            } catch (Exception ex) {
                log.error("outbox backoff batch FAILED: {} rows err={}", backoffArgs.size(), ex.toString());
            }
        }
        log.info("outbox relay: ok={} fail={} pending_scanned={}", doneArgs.size(), deadArgs.size() + backoffArgs.size(), rows.size());
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
