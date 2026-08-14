package com.esp.disruptor;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class DisruptorStatsTest {

    @Test
    void backlogReturnsDiffWhenPublishedExceedsConsumed() {
        DisruptorStats s = new DisruptorStats("n", 1024, 100L, 5L, 70L);
        assertEquals(30L, s.backlog(), "published-consumed");
        assertEquals("n", s.name());
        assertEquals(1024L, s.bufferSize());
        assertEquals(100L, s.published());
        assertEquals(5L, s.dropped());
        assertEquals(70L, s.consumed());
    }

    @Test
    void backlogReturnsZeroWhenConsumedExceedsPublished() {
        DisruptorStats s = new DisruptorStats("n", 1024, 10L, 0L, 50L);
        assertEquals(0L, s.backlog(), "max(0, published-consumed)");
    }

    @Test
    void backlogZeroWhenEqual() {
        DisruptorStats s = new DisruptorStats("n", 1024, 10L, 0L, 10L);
        assertEquals(0L, s.backlog());
    }
}
