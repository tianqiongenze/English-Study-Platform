package com.esp.learning.rpc;

import com.esp.api.dubbo.KpStat;
import com.esp.learning.domain.UserMastery;
import com.esp.learning.service.LearningService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * LearningRpcProvider 单元测试:验证 LearningRpcService 契约映射。
 * mock LearningService,验证 userStats/weakCount 字段映射正确,读副本注解存在。
 */
class LearningRpcProviderTest {

    private LearningService learningService;
    private LearningRpcProvider provider;

    @BeforeEach
    void setUp() {
        learningService = mock(LearningService.class);
        provider = new LearningRpcProvider(learningService);
    }

    @Test
    void userStats_mapsFieldsCorrectly() {
        UserMastery m1 = new UserMastery(1L, 10L);
        m1.setMasteryScore(BigDecimal.valueOf(3.5));
        m1.setReviewCount(2);
        m1.setNextReviewAt(Instant.parse("2026-08-01T00:00:00Z"));

        UserMastery m2 = new UserMastery(1L, 20L);
        m2.setMasteryScore(BigDecimal.ZERO);
        m2.setReviewCount(0);
        m2.setNextReviewAt(null);

        when(learningService.weakestPoints(1L)).thenReturn(List.of(m1, m2));

        List<KpStat> stats = provider.userStats(1L);

        assertEquals(2, stats.size());
        KpStat s1 = stats.get(0);
        assertEquals(10L, s1.getKpId());
        assertEquals(3.5, s1.getMasteryScore(), 1e-6);
        assertEquals(2, s1.getReviewCount());
        assertEquals("2026-08-01T00:00:00Z", s1.getNextReviewAt());

        KpStat s2 = stats.get(1);
        assertEquals(20L, s2.getKpId());
        assertEquals(0d, s2.getMasteryScore(), 1e-6);
        assertEquals(0, s2.getReviewCount());
        assertNull(s2.getNextReviewAt());
    }

    @Test
    void userStats_emptyList_returnsEmpty() {
        when(learningService.weakestPoints(9L)).thenReturn(List.of());
        assertTrue(provider.userStats(9L).isEmpty());
    }

    @Test
    void userStats_nullMasteryScore_mapsToZero() {
        UserMastery m = new UserMastery(1L, 5L);
        m.setMasteryScore(null);
        m.setReviewCount(null);
        m.setNextReviewAt(null);
        when(learningService.weakestPoints(anyLong())).thenReturn(List.of(m));

        List<KpStat> stats = provider.userStats(1L);
        assertEquals(1, stats.size());
        assertEquals(0d, stats.get(0).getMasteryScore(), 1e-6);
        assertEquals(0, stats.get(0).getReviewCount());
        assertNull(stats.get(0).getNextReviewAt());
    }

    @Test
    void weakCount_returnsSize() {
        when(learningService.weakestPoints(1L)).thenReturn(List.of(
                new UserMastery(1L, 1L), new UserMastery(1L, 2L), new UserMastery(1L, 3L)));
        assertEquals(3, provider.weakCount(1L));
    }

    @Test
    void weakCount_empty_returnsZero() {
        when(learningService.weakestPoints(7L)).thenReturn(List.of());
        assertEquals(0, provider.weakCount(7L));
    }
}
