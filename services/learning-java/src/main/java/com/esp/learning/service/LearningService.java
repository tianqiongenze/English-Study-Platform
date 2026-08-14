package com.esp.learning.service;

import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.learning.disruptor.LearningEventDisruptor;
import com.esp.learning.domain.UserMastery;
import com.esp.learning.domain.UserMasteryId;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import com.esp.learning.repository.UserMasteryRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Service;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;

@Service
public class LearningService {

    private static final Logger log = LoggerFactory.getLogger(LearningService.class);

    /** L1 缓存名(与 CacheConfig 常量对齐)。 */
    private static final String CACHE_DUE = "learning:due";
    private static final String CACHE_WEAK = "learning:weak";

    private final UserMasteryRepository masteryRepo;
    private final LearningEventDisruptor disruptor;
    private final EbbinghausScheduler scheduler;
    private final DifficultyAdjuster adjuster;
    private final CacheService cacheService;
    /** P2-2: 跨进程投递用(入口拆 worker 时,Disruptor Bean 不装配,改走 Pulsar)。 */
    private final ObjectProvider<LearningEventDisruptor> disruptorProvider;
    private final EventPublisher eventPublisher;
    /** P2-2: true=进程内 Disruptor(单进程一体化);false=发 Pulsar 给独立 worker。 */
    private final boolean disruptorEnabled;

    public LearningService(UserMasteryRepository masteryRepo,
                           ObjectProvider<LearningEventDisruptor> disruptorProvider,
                           EbbinghausScheduler scheduler,
                           DifficultyAdjuster adjuster,
                           CacheService cacheService,
                           EventPublisher eventPublisher,
                           @Value("${esp.disruptor.enabled:true}") boolean disruptorEnabled) {
        this.masteryRepo = masteryRepo;
        this.disruptorProvider = disruptorProvider;
        this.disruptor = disruptorProvider.getIfAvailable();
        this.scheduler = scheduler;
        this.adjuster = adjuster;
        this.cacheService = cacheService;
        this.eventPublisher = eventPublisher;
        this.disruptorEnabled = disruptorEnabled;
    }

    /**
     * FR-2.4 学情事件入口。
     * P2-2: 单进程一体化(esp.disruptor.enabled=true)→ 进程内 Disruptor 异步入队,立即返回;
     *       拆 worker(esp.disruptor.enabled=false)→ 发 Pulsar topic LEARNING_EVENT_REPORT,
     *       由 learning-worker 订阅后进 Disruptor 落库。两条路径均非阻塞,失败降级只 warn。
     */
    public boolean reportEvent(long userId, String eventType, String payload) {
        if (disruptorEnabled && disruptor != null) {
            return disruptor.publish(userId, eventType, payload);
        }
        // P2-2: 入口无 Disruptor Bean,走 Pulsar 跨进程投递。
        eventPublisher.publish(EventTopics.LEARNING_EVENT_REPORT,
                new LearningEventReport(userId, eventType, payload));
        log.debug("learning event forwarded to pulsar (worker mode): user={} type={}", userId, eventType);
        return true;
    }

    /** P2-2: Pulsar 事件体(与 learning-worker 反序列化对齐)。 */
    public record LearningEventReport(long userId, String eventType, String payload) {}

    /**
     * FR-2.1 初测:批量写入 mastery 基线(写主库)。写后失效 due/weak 缓存。
     * 批量 saveAll(原逐条 save 在 kpIds 多时 N 次 INSERT 往返,坑㉖)。
     */
    @DataSource(DataSourceType.WRITE)
    public void initMastery(long userId, List<Long> kpIds, double initialScore) {
        Instant now = Instant.now();
        java.util.List<UserMastery> batch = new java.util.ArrayList<>(kpIds.size());
        for (Long kpId : kpIds) {
            UserMastery m = new UserMastery(userId, kpId);
            m.setMasteryScore(BigDecimal.valueOf(initialScore));
            m.setReviewCount(0);
            m.setLastReviewAt(now);
            batch.add(m);
        }
        masteryRepo.saveAll(batch);
        evictUserCache(userId);
    }

    /**
     * FR-2.3 今日复习清单。L1 Caffeine → L2 Redis → DB。
     * P2-6: consistent=true 强制走主库。复习提交/初测后前端会立即拉复习清单,
     * 若走从库受 follower lag 影响会读到旧值(刚 review 的 next_review_at 未同步),
     * 故写后立即读的高频入口一律走主库,牺牲少量主库读压力换正确性。
     */
    @DataSource(value = DataSourceType.READ, consistent = true)
    public List<UserMastery> dueReview(long userId) {
        String key = "learning:due:" + userId;
        return cacheService.getOrLoad(CACHE_DUE, key,
                () -> masteryRepo.findDueReview(userId, Instant.now()),
                0, true);
    }

    /**
     * FR-2.1 薄弱知识点(三入口复用热点)。L1 → L2 → DB。
     * P2-6: consistent=true 强制走主库(同 dueReview 理由:写后立即读)。
     */
    @DataSource(value = DataSourceType.READ, consistent = true)
    public List<UserMastery> weakestPoints(long userId) {
        String key = "learning:weak:" + userId;
        return cacheService.getOrLoad(CACHE_WEAK, key,
                () -> masteryRepo.findWeakest(userId),
                0, true);
    }

    /**
     * FR-2.3 复习一次:更新 mastery + 艾宾浩斯下次复习时间(写主库)。写后异步失效 due/weak。
     */
    @DataSource(DataSourceType.WRITE)
    public void review(long userId, long kpId, boolean correct) {
        UserMasteryId id = new UserMasteryId(userId, kpId);
        UserMastery m = masteryRepo.findById(id).orElseGet(() -> new UserMastery(userId, kpId));
        int count = m.getReviewCount() == null ? 0 : m.getReviewCount();
        Instant now = Instant.now();
        m.setLastReviewAt(now);
        m.setNextReviewAt(scheduler.nextReviewAt(count, correct, now));
        m.setReviewCount(correct ? count + 1 : 0);
        BigDecimal score = m.getMasteryScore() == null ? BigDecimal.ZERO : m.getMasteryScore();
        m.setMasteryScore(correct ? score.add(BigDecimal.ONE) : score);
        masteryRepo.save(m);
        evictUserCache(userId);
    }

    /** 失效指定用户的 due/weak 缓存(删 L2 + 删 L1 + Pulsar 广播 + 延迟双删)。 */
    private void evictUserCache(long userId) {
        cacheService.evict(CACHE_DUE, "learning:due:" + userId);
        cacheService.evict(CACHE_WEAK, "learning:weak:" + userId);
    }

    /**
     * FR-2.2 动态难度:记录正误并返回下一难度档位。
     */
    public int nextDifficulty(int current, boolean correct) {
        return adjuster.recordAndAdjust(current, correct);
    }

    public double speedMultiplier(int difficulty) {
        return DifficultyAdjuster.speedMultiplier(difficulty);
    }
}
