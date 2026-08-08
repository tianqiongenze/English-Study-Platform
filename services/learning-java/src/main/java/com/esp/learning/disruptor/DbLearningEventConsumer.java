package com.esp.learning.disruptor;

import com.esp.cache.CacheService;
import com.esp.learning.domain.LearningEventEntity;
import com.esp.learning.repository.LearningEventRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 默认消费者:批量落库 TiDB learning_events(写主库)。
 * 真实环境可叠加 Pulsar 发布触发算法;此处聚焦落库。
 * id 由 TiDB AUTO_RANDOM 生成,故设为 null。
 *
 * <p>落库成功后失效 learning:agg:users 聚合缓存(事件驱动失效,ADR-多级缓存)。
 */
@Component
public class DbLearningEventConsumer implements LearningEventConsumer {

    private static final Logger log = LoggerFactory.getLogger(DbLearningEventConsumer.class);
    private final LearningEventRepository repo;
    private final CacheService cacheService;
    private final AtomicLong consumed = new AtomicLong();

    /** 聚合缓存名 + key(与 LearningEventRepository.aggregateByUser 的读缓存对齐)。 */
    private static final String CACHE_AGG = "learning:agg";
    private static final String AGG_KEY = "learning:agg:users";

    public DbLearningEventConsumer(LearningEventRepository repo, CacheService cacheService) {
        this.repo = repo;
        this.cacheService = cacheService;
    }

    @Override
    public void consume(LearningEvent[] events, int offset, int length) {
        List<LearningEventEntity> batch = new ArrayList<>(length);
        for (int i = 0; i < length; i++) {
            LearningEvent e = events[offset + i];
            LearningEventEntity ent = new LearningEventEntity();
            ent.setId(null); // AUTO_RANDOM
            ent.setUserId(e.getUserId());
            ent.setEventType(e.getEventType());
            ent.setPayload(e.getPayload());
            ent.setCreatedAt(Instant.ofEpochMilli(e.getCreatedAt()));
            batch.add(ent);
        }
        try {
            repo.saveAll(batch);
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

