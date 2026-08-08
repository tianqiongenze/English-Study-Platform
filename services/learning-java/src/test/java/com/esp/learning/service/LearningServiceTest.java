package com.esp.learning.service;

import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.learning.disruptor.LearningEventDisruptor;
import com.esp.learning.domain.UserMastery;
import com.esp.learning.domain.UserMasteryId;
import com.esp.learning.repository.UserMasteryRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.ObjectProvider;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.function.Supplier;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * LearningService 单元测试:验证 Disruptor 入口、初测、复习、薄弱点逻辑,
 * 以及动态难度/语速委托。依赖全部 Mock,无外部 IO。
 *
 * <p>新增多级缓存验证(ADR-多级缓存):
 * <ul>
 *   <li>连续两次 dueReview 第二次不查 DB(verify repo times(1))。</li>
 *   <li>review 后缓存失效,再查 dueReview 重查 DB。</li>
 *   <li>feature flag(cache.enabled=false → CacheService no-op)走直打 DB。</li>
 * </ul>
 */
class LearningServiceTest {

    private UserMasteryRepository masteryRepo;
    private LearningEventDisruptor disruptor;
    private EbbinghausScheduler scheduler;
    private DifficultyAdjuster adjuster;
    private CacheService cacheService;
    private EventPublisher eventPublisher;
    @SuppressWarnings("unchecked")
    private ObjectProvider<LearningEventDisruptor> disruptorProvider;
    private LearningService service;

    @SuppressWarnings("unchecked")
    @BeforeEach
    void setUp() {
        masteryRepo = mock(UserMasteryRepository.class);
        disruptor = mock(LearningEventDisruptor.class);
        scheduler = new EbbinghausScheduler();
        adjuster = new DifficultyAdjuster(5);
        cacheService = mock(CacheService.class);
        eventPublisher = mock(EventPublisher.class);
        disruptorProvider = mock(ObjectProvider.class);
        when(disruptorProvider.getIfAvailable()).thenReturn(disruptor);
        service = new LearningService(masteryRepo, disruptorProvider, scheduler, adjuster,
                cacheService, eventPublisher, true);
    }

    @Test
    void reportEvent_delegatesToDisruptor() {
        when(disruptor.publish(anyLong(), anyString(), anyString())).thenReturn(true);
        assertTrue(service.reportEvent(1L, "answer", "{}"));
        verify(disruptor).publish(1L, "answer", "{}");
    }

    /**
     * P2-2: worker 模式(disruptor.enabled=false)下,reportEvent 走 Pulsar 而非进程内 Disruptor。
     */
    @Test
    void reportEvent_workerMode_forwardsToPulsar() {
        LearningService workerSvc = new LearningService(masteryRepo, disruptorProvider,
                scheduler, adjuster, cacheService, eventPublisher, false);
        when(disruptorProvider.getIfAvailable()).thenReturn(null);
        assertTrue(workerSvc.reportEvent(1L, "answer", "{}"));
        verify(eventPublisher).publish(eq("learning.event.report"), any());
        verify(disruptor, never()).publish(anyLong(), anyString(), anyString());
    }

    @Test
    void initMastery_writesBaselineForEachKp() {
        service.initMastery(1L, List.of(10L, 20L), 50.0);
        verify(masteryRepo, times(2)).save(any(UserMastery.class));
    }

    @Test
    void review_correct_incrementsScoreAndReviewCount() {
        UserMastery m = new UserMastery(1L, 10L);
        m.setReviewCount(2);
        m.setMasteryScore(BigDecimal.valueOf(3));
        when(masteryRepo.findById(new UserMasteryId(1L, 10L))).thenReturn(Optional.of(m));
        service.review(1L, 10L, true);

        assertEquals(3, m.getReviewCount());
        assertEquals(BigDecimal.valueOf(4), m.getMasteryScore());
        assertNotNull(m.getNextReviewAt()); // EbbinghausScheduler 真实计算
        verify(masteryRepo).save(m);
    }

    @Test
    void review_incorrect_resetsReviewCountZeroScoreUnchanged() {
        UserMastery m = new UserMastery(1L, 10L);
        m.setReviewCount(5);
        m.setMasteryScore(BigDecimal.valueOf(4));
        when(masteryRepo.findById(new UserMasteryId(1L, 10L))).thenReturn(Optional.of(m));

        service.review(1L, 10L, false);

        assertEquals(0, m.getReviewCount());
        assertEquals(BigDecimal.valueOf(4), m.getMasteryScore());
    }

    @Test
    void review_createsNewMasteryWhenAbsent() {
        when(masteryRepo.findById(any())).thenReturn(Optional.empty());

        service.review(9L, 99L, true);

        verify(masteryRepo).save(argThat(m -> m.getUserId() == 9L && m.getKpId() == 99L
                && m.getReviewCount() == 1 && m.getMasteryScore().compareTo(BigDecimal.ONE) == 0));
    }

    @Test
    void nextDifficulty_delegatesToAdjuster() {
        // 窗口未满 -> 保持
        assertEquals(3, service.nextDifficulty(3, true));
    }

    @Test
    void speedMultiplier_returnsStaticFormula() {
        assertEquals(0.8, service.speedMultiplier(1), 1e-4);
        assertEquals(1.4, service.speedMultiplier(5), 1e-4);
    }

    @Test
    void dueReview_delegatesToCacheService() {
        service.dueReview(1L);
        verify(cacheService).getOrLoad(eq("learning:due"), eq("learning:due:1"),
                any(Supplier.class), eq(0L), eq(true));
        // 真实 repo 不应被直接调用(loader 由 CacheService 在 miss 时执行)
        verify(masteryRepo, never()).findDueReview(anyLong(), any());
    }

    @Test
    void weakestPoints_delegatesToCacheService() {
        service.weakestPoints(1L);
        verify(cacheService).getOrLoad(eq("learning:weak"), eq("learning:weak:1"),
                any(Supplier.class), eq(0L), eq(true));
        verify(masteryRepo, never()).findWeakest(anyLong());
    }

    /**
     * 验证缓存命中(端到端):真实 CacheService + 真实 Caffeine L1,
     * mock repo + mock RedisTemplate(L2 始终 miss)。
     * 连续两次 dueReview 第二次应命中 L1,repo 只被调一次。
     */
    @Test
    void dueReview_secondCall_hitsL1_noDbQuery() {
        org.springframework.cache.caffeine.CaffeineCacheManager cm =
                new org.springframework.cache.caffeine.CaffeineCacheManager("learning:due", "learning:weak");
        cm.setCaffeineSpec(com.github.benmanes.caffeine.cache.CaffeineSpec.parse(
                "maximumSize=1000,expireAfterWrite=60s"));

        com.esp.cache.CacheService realCache = buildRealCacheService(cm);

        when(disruptorProvider.getIfAvailable()).thenReturn(disruptor);
        LearningService svc = new LearningService(masteryRepo, disruptorProvider, scheduler, adjuster,
                realCache, eventPublisher, true);
        when(masteryRepo.findDueReview(eq(1L), any())).thenReturn(List.of(new UserMastery(1L, 10L)));

        svc.dueReview(1L);
        svc.dueReview(1L); // 第二次应命中 L1

        verify(masteryRepo, times(1)).findDueReview(eq(1L), any());
    }

    /**
     * 验证 review 后缓存失效:第一次查 DB → review(evict) → 第二次重查 DB(L1 已失效)。
     */
    @Test
    void review_invalidatesCache_soNextQueryRehitsDb() {
        org.springframework.cache.caffeine.CaffeineCacheManager cm =
                new org.springframework.cache.caffeine.CaffeineCacheManager("learning:due", "learning:weak");
        cm.setCaffeineSpec(com.github.benmanes.caffeine.cache.CaffeineSpec.parse(
                "maximumSize=1000,expireAfterWrite=60s"));

        com.esp.cache.CacheService realCache = buildRealCacheService(cm);

        LearningService svc = new LearningService(masteryRepo, disruptorProvider, scheduler, adjuster,
                realCache, eventPublisher, true);
        when(masteryRepo.findDueReview(eq(1L), any())).thenReturn(List.of(new UserMastery(1L, 10L)));
        when(masteryRepo.findById(any())).thenReturn(Optional.of(new UserMastery(1L, 10L)));

        svc.dueReview(1L);            // 第一次:查 DB
        svc.review(1L, 10L, true);    // 写 + 失效
        svc.dueReview(1L);            // 第二次:L1 已 evict,重查 DB

        verify(masteryRepo, times(2)).findDueReview(eq(1L), any());
    }

    /**
     * feature flag 关闭:CacheService.getOrLoad 直接执行 loader(模拟 cache.enabled=false no-op)。
     * 连续两次 dueReview 每次都查 DB。
     */
    @Test
    void dueReview_cacheDisabled_fallsThroughToDb() {
        when(cacheService.getOrLoad(anyString(), anyString(), any(Supplier.class), anyLong(), anyBoolean()))
                .thenAnswer(inv -> {
                    @SuppressWarnings("unchecked")
                    Supplier<List<UserMastery>> loader = (Supplier<List<UserMastery>>) inv.getArgument(2);
                    return loader.get();
                });
        when(masteryRepo.findDueReview(eq(1L), any())).thenReturn(List.of());

        service.dueReview(1L);
        service.dueReview(1L); // 缓存关闭,每次都查 DB

        verify(masteryRepo, times(2)).findDueReview(eq(1L), any());
    }

    // ---- 构造真实 CacheService(真实 Caffeine L1 + mock RedisTemplate L2 miss + 无 broadcast) ----

    @SuppressWarnings("unchecked")
    private com.esp.cache.CacheService buildRealCacheService(org.springframework.cache.CacheManager cm) {
        org.springframework.data.redis.core.RedisTemplate<String, Object> redis =
                mock(org.springframework.data.redis.core.RedisTemplate.class);
        org.springframework.data.redis.core.ValueOperations<String, Object> vops =
                mock(org.springframework.data.redis.core.ValueOperations.class);
        when(redis.opsForValue()).thenReturn(vops);
        when(vops.get(anyString())).thenReturn(null); // L2 miss

        org.springframework.beans.factory.ObjectProvider<org.springframework.cache.CacheManager> cmP =
                mock(org.springframework.beans.factory.ObjectProvider.class);
        when(cmP.getIfAvailable()).thenReturn(cm);

        org.springframework.beans.factory.ObjectProvider<org.springframework.data.redis.core.RedisTemplate<String, Object>> rP =
                mock(org.springframework.beans.factory.ObjectProvider.class);
        when(rP.getIfAvailable()).thenReturn(redis);

        org.springframework.beans.factory.ObjectProvider<com.esp.cache.CacheBroadcast> bP =
                mock(org.springframework.beans.factory.ObjectProvider.class);
        when(bP.getIfAvailable()).thenReturn(null);

        return new com.esp.cache.CacheService(cmP, rP, bP);
    }
}
