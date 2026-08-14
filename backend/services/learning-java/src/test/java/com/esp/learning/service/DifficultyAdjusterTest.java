package com.esp.learning.service;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class DifficultyAdjusterTest {

    private DifficultyAdjuster a;

    @BeforeEach
    void setUp() {
        a = new DifficultyAdjuster(5);
    }

    @Test
    void windowNotFull_keepsDifficulty() {
        int d = 3;
        d = a.recordAndAdjust(d, true);
        d = a.recordAndAdjust(d, true);
        assertEquals(3, d); // 样本不足,保持
    }

    @Test
    void fullWindowHighRate_increasesDifficulty_upToMax() {
        int d = 3;
        // 满 5 全对 -> 上调一档到 4
        for (int i = 0; i < 5; i++) d = a.recordAndAdjust(d, true);
        assertEquals(4, d);
        // 再满 5 全对 -> 到 5
        for (int i = 0; i < 5; i++) d = a.recordAndAdjust(d, true);
        assertEquals(DifficultyAdjuster.MAX, d);
        // 已封顶
        for (int i = 0; i < 5; i++) d = a.recordAndAdjust(d, true);
        assertEquals(DifficultyAdjuster.MAX, d);
    }

    @Test
    void fullWindowLowRate_decreasesDifficulty_downToMin() {
        int d = 3;
        for (int i = 0; i < 5; i++) d = a.recordAndAdjust(d, false);
        assertEquals(2, d);
        for (int i = 0; i < 5; i++) d = a.recordAndAdjust(d, false);
        assertEquals(DifficultyAdjuster.MIN, d);
    }

    @Test
    void fullWindowMidRate_keepsDifficulty() {
        int d = 3;
        // 3 对 2 错 = 60%,保持
        d = a.recordAndAdjust(d, true);
        d = a.recordAndAdjust(d, true);
        d = a.recordAndAdjust(d, false);
        d = a.recordAndAdjust(d, true);
        d = a.recordAndAdjust(d, false);
        assertEquals(3, d);
    }

    @Test
    void scaffoldingNeededWhenDifficultyDecreased() {
        assertTrue(a.needsScaffolding(3, 2));
        assertFalse(a.needsScaffolding(3, 3));
        assertFalse(a.needsScaffolding(3, 4));
    }

    @Test
    void speedMultiplier_monotonicByDifficulty() {
        double d1 = DifficultyAdjuster.speedMultiplier(1);
        double d5 = DifficultyAdjuster.speedMultiplier(5);
        assertEquals(0.8, d1, 0.0001);
        assertEquals(1.4, d5, 0.0001);
        assertTrue(d1 < DifficultyAdjuster.speedMultiplier(3));
        assertTrue(DifficultyAdjuster.speedMultiplier(3) < d5);
    }
}
