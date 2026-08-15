package com.esp.learning.disruptor;

import com.esp.cache.CacheService;
import com.esp.learning.disruptor.LearningEvent;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.sql.Timestamp;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 默认消费者:批量落库 TiDB learning_events(写主库)。
 * 真实环境可叠加 Pulsar 发布触发算法;此处聚焦落库。
 *
 * <p>坑㉙-修复:learning_events.id 为 TiDB AUTO_RANDOM,Hibernate persist() 要求应用分配
 * 非空主键,setId(null) 必抛 "ids for this class must be manually assigned before persist"。
 * 改用 JdbcTemplate 原生 INSERT 且**不含 id 列**,让 TiDB AUTO_RANDOM 自动生成随机 id
 * (与种子灌入 100M 行方式一致)。批量逐行 batchUpdate 降低往返开销。
 *
 * <p>落库成功后失效 learning:agg:users 聚合缓存(事件驱动失效,ADR-多级缓存)。
 *
 * <p>G-06 修复(batch 失败不丢数据):本消费者由独立 DB writer 线程池异步调用(见
 * {@link EventBatchQueue});当 batchUpdate 失败时,把整个批次改写进 {@code learning_event_outbox}
 * (status='failed'),由 {@link OutboxRelay} 轮询重试 —— 替代原"仅 log 丢数据"。写入 outbox 本身
 * 也可能失败(DB 双障),此时仍尽力记日志(数据丢失风险,与 Disruptor DROP 同语义兜底)。
 */
@Component
public class DbLearningEventConsumer implements LearningEventConsumer {

    private static final Logger log = LoggerFactory.getLogger(DbLearningEventConsumer.class);
    private final JdbcTemplate writeJdbc;
    private final CacheService cacheService;
    private final AtomicLong consumed = new AtomicLong();
    private final AtomicLong outboxRouted = new AtomicLong();

    /** 聚合缓存名 + key(与 LearningEventRepository.aggregateByUser 的读缓存对齐)。 */
    private static final String CACHE_AGG = "learning:agg";
    private static final String AGG_KEY = "learning:agg:users";

    /** 不含 id 列,由数据库自动生成。message_id 为 P1-8 幂等键:ON CONFLICT 去重(Shared 订阅重投递)。 */
    static final String INSERT_SQL =
            "INSERT INTO learning_events (user_id, event_type, payload, created_at, message_id) " +
                    "VALUES (?,?,?,?,?) ON CONFLICT (message_id) DO NOTHING";

    /** batch 失败兜底:改写 outbox(status='failed'),由 OutboxRelay 重试。message_id 透传保留幂等键。 */
    static final String OUTBOX_SQL =
            "INSERT INTO learning_event_outbox (message_id, user_id, event_type, payload, status, retry_count, next_retry_at) " +
                    "VALUES (?,?,?,?, 'failed', 0, CURRENT_TIMESTAMP)";

    public DbLearningEventConsumer(JdbcTemplate writeJdbc, CacheService cacheService) {
        this.writeJdbc = writeJdbc;
        this.cacheService = cacheService;
    }

    @Override
    public void consume(LearningEvent[] events, int offset, int length) {
        List<Object[]> batchArgs = new ArrayList<>(length);
        for (int i = 0; i < length; i++) {
            LearningEvent e = events[offset + i];
            batchArgs.add(new Object[]{
                    e.getUserId(),
                    e.getEventType(),
                    e.getPayload(),
                    // Timestamp 而非 Instant:PG JDBC 42.7.x setObject(Instant) 无法推断类型(SQLState 07006),
                    // CRDB 模式整批落库必失败;Timestamp 双模式通用。
                    new Timestamp(e.getCreatedAt()),
                    e.getMessageId()
            });
        }
        try {
            writeJdbc.batchUpdate(INSERT_SQL, batchArgs);
            consumed.addAndGet(length);
            // 落库成功后失效聚合缓存(删 L2 + 删 L1 + Pulsar 广播 + 延迟双删)
            cacheService.evict(CACHE_AGG, AGG_KEY);
        } catch (Exception ex) {
            // G-06:不再仅 log 丢数据 —— 整个批次改写 outbox,由 OutboxRelay 异步重试。
            log.error("batch persist failed, routing {} events to outbox: {}", length, ex.getMessage());
            routeToOutbox(events, offset, length, ex);
        }
    }

    /** 批次落库失败 → 整批写入 learning_event_outbox(status='failed'),交给 OutboxRelay 重试。 */
    private void routeToOutbox(LearningEvent[] events, int offset, int length, Exception cause) {
        try {
            List<Object[]> outboxArgs = new ArrayList<>(length);
            for (int i = 0; i < length; i++) {
                LearningEvent e = events[offset + i];
                outboxArgs.add(new Object[]{e.getMessageId(), e.getUserId(), e.getEventType(), e.getPayload()});
            }
            writeJdbc.batchUpdate(OUTBOX_SQL, outboxArgs);
            outboxRouted.addAndGet(length);
            log.warn("outbox routed: {} events (cause={})", length, cause.getMessage());
        } catch (Exception outboxEx) {
            // outbox 也失败(DB 双障)→ 尽力而为记日志,数据不可恢复(与 RingBuffer 满 DROP 同语义兜底)。
            log.error("outbox fallback FAILED (data loss risk): {} events dropped err={}",
                    length, outboxEx.toString());
        }
    }

    public long consumedCount() { return consumed.get(); }

    /** 因 batch 失败被改写进 outbox 的事件总数(可观测:该值 >0 说明主写入路径有间歇性故障)。 */
    public long outboxRoutedCount() { return outboxRouted.get(); }
}
