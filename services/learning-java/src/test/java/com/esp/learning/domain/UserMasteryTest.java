package com.esp.learning.domain;

import org.junit.jupiter.api.Test;

import java.math.BigDecimal;
import java.time.Instant;

import static org.junit.jupiter.api.Assertions.*;

/**
 * UserMastery 聚合根行为测试(P2-13 充血化):applyReviewEvent/applyTeacherFinalized/initBaseline 联动闭环。
 */
class UserMasteryTest {

    @Test
    void applyReviewEvent_correct_incrementsScoreAndCount() {
        UserMastery m = new UserMastery(1L, 10L);
        Instant now = Instant.parse("2026-08-14T10:00:00Z");
        Instant next = Instant.parse("2026-08-15T10:00:00Z");

        m.applyReviewEvent(true, next, now);

        assertEquals(BigDecimal.ONE, m.getMasteryScore());
        assertEquals(1, m.getReviewCount());
        assertEquals(now, m.getLastReviewAt());
        assertEquals(next, m.getNextReviewAt());
    }

    @Test
    void applyReviewEvent_incorrect_resetsCountZeroScoreUnchanged() {
        UserMastery m = new UserMastery(1L, 10L);
        m.setReviewCount(5);
        m.setMasteryScore(BigDecimal.valueOf(4));
        Instant now = Instant.parse("2026-08-14T10:00:00Z");
        Instant next = Instant.parse("2026-08-15T10:00:00Z");

        m.applyReviewEvent(false, next, now);

        assertEquals(0, m.getReviewCount());
        assertEquals(BigDecimal.valueOf(4), m.getMasteryScore());
        assertEquals(next, m.getNextReviewAt());
    }

    @Test
    void applyReviewEvent_toleratesNullCountAndScore() {
        UserMastery m = new UserMastery();
        m.setUserId(1L);
        m.setKpId(10L);
        // reviewCount / masteryScore 留 null(旧数据)
        Instant now = Instant.parse("2026-08-14T10:00:00Z");

        m.applyReviewEvent(true, null, now);

        assertEquals(BigDecimal.ONE, m.getMasteryScore());
        assertEquals(1, m.getReviewCount());
    }

    @Test
    void applyTeacherFinalized_overridesScoreAndKeepsCount() {
        UserMastery m = new UserMastery(1L, 10L);
        m.setReviewCount(3);
        Instant now = Instant.parse("2026-08-14T10:00:00Z");

        m.applyTeacherFinalized(BigDecimal.valueOf(95), now);

        assertEquals(BigDecimal.valueOf(95), m.getMasteryScore());
        assertEquals(3, m.getReviewCount(), "终评不重置 reviewCount");
        assertEquals(now, m.getLastReviewAt());
    }

    @Test
    void initBaseline_setsScoreAndZeroCount() {
        UserMastery m = new UserMastery(1L, 10L);
        Instant now = Instant.parse("2026-08-14T10:00:00Z");

        m.initBaseline(BigDecimal.valueOf(50), now);

        assertEquals(BigDecimal.valueOf(50), m.getMasteryScore());
        assertEquals(0, m.getReviewCount());
        assertEquals(now, m.getLastReviewAt());
    }
}
