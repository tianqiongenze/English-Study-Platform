package com.esp.learning.disruptor;

import com.esp.cache.CacheService;
import com.esp.learning.disruptor.LearningEvent;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.time.Instant;
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
 */
@Component
public class DbLearningEventConsumer implements LearningEventConsumer {

    private static final Logger log = LoggerFactory.getLogger(DbLearningEventConsumer.class);
    private final JdbcTemplate writeJdbc;
    private final CacheService cacheService;
    private final AtomicLong consumed = new AtomicLong();

    /** 聚合缓存名 + key(与 LearningEventRepository.aggregateByUser 的读缓存对齐)。 */
    private static final String CACHE_AGG = "learning:agg";
    private static final String AGG_KEY = "learning:agg:users";

    /** 不含 id 列,由 TiDB AUTO_RANDOM 自动生成。 */
    private static final String INSERT_SQL =
            "INSERT INTO learning_events (user_id, event_type, payload, created_at) VALUES (?,?,?,?)";

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
                    Instant.ofEpochMilli(e.getCreatedAt())
            });
        }
        try {
            writeJdbc.batchUpdate(INSERT_SQL, batchArgs);
            consumed.addAndGet(length);
            // 落库成功后失效聚合缓存(删 L2 + 删 L1 + Pulsar 广播 + 延迟双删)
            cacheService.evict(CACHE_AGG, AGG_KEY);
        } catch (Exception ex) {
            // 生产环境:转死信队列/重试。此处仅记错,保证 Disruptor 不阻塞。
            log.error("batch persist failed: {}", ex.getMessage());
        }
    }

    public long consumedCount() { return consumed.get(); }
}

