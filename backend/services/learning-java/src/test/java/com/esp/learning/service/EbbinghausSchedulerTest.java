package com.esp.learning.service;

import org.junit.jupiter.api.Test;

import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

class EbbinghausSchedulerTest {

    private final EbbinghausScheduler s = new EbbinghausScheduler();

    @Test
    void firstCorrectReview_uses1DayInterval() {
        Instant now = Instant.parse("2026-07-28T00:00:00Z");
        Instant next = s.nextReviewAt(0, true, now);
        assertEquals(now.plus(1, ChronoUnit.DAYS), next);
    }

    @Test
    void progressesThroughIntervals() {
        Instant now = Instant.parse("2026-07-28T00:00:00Z");
        List<Long> intervals = EbbinghausScheduler.INTERVALS_DAYS;
        for (int i = 0; i < intervals.size(); i++) {
            Instant next = s.nextReviewAt(i, true, now);
            assertEquals(now.plus(intervals.get(i), ChronoUnit.DAYS), next,
                    "reviewCount=" + i);
        }
    }

    @Test
    void incorrect_resetsToFirstInterval() {
        Instant now = Instant.parse("2026-07-28T00:00:00Z");
        // 已复习 5 次,答错 -> 重置为 1 天
        Instant next = s.nextReviewAt(5, false, now);
        assertEquals(now.plus(1, ChronoUnit.DAYS), next);
    }

    @Test
    void capsAtLastInterval() {
        Instant now = Instant.parse("2026-07-28T00:00:00Z");
        // reviewCount 超过间隔数,取最后一个(30 天)
        Instant next = s.nextReviewAt(100, true, now);
        assertEquals(now.plus(30, ChronoUnit.DAYS), next);
    }
}
