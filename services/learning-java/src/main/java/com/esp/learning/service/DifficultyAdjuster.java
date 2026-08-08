package com.esp.learning.service;

import java.util.ArrayDeque;
import java.util.Deque;

/**
 * 动态难度调整:基于滑动窗口正确率。
 * 每填满一个窗口计算一次正确率并据此调整一档:
 * - 正确率 > 80% -> 难度上调(上限 5)
 * - 正确率 < 40% -> 难度下调(下限 1)并附降维解析
 * - 否则保持
 * 窗口未满时不调整(样本不足)。
 */
public class DifficultyAdjuster {

    public static final int MIN = 1;
    public static final int MAX = 5;
    private static final double UP_THRESHOLD = 0.8;
    private static final double DOWN_THRESHOLD = 0.4;

    private final int windowSize;
    private final Deque<Boolean> window;

    public DifficultyAdjuster(int windowSize) {
        this.windowSize = Math.max(windowSize, 1);
        this.window = new ArrayDeque<>(windowSize);
    }

    /** 记录一题正误,返回建议的下一难度档位(1~5)。 */
    public int recordAndAdjust(int currentDifficulty, boolean correct) {
        window.addLast(correct);
        if (window.size() < windowSize) {
            return currentDifficulty; // 样本不足,保持
        }
        // 窗口满,计算并清空(滑动窗口逐窗口评估)
        double rate = correctRate();
        int next = currentDifficulty;
        if (rate > UP_THRESHOLD) {
            next = Math.min(MAX, currentDifficulty + 1);
        } else if (rate < DOWN_THRESHOLD) {
            next = Math.max(MIN, currentDifficulty - 1);
        }
        window.clear();
        return next;
    }

    /** 是否需要降维解析(难度被下调时)。 */
    public boolean needsScaffolding(int currentDifficulty, int nextDifficulty) {
        return nextDifficulty < currentDifficulty;
    }

    public double correctRate() {
        if (window.isEmpty()) return 0.0;
        long ok = window.stream().filter(b -> b).count();
        return (double) ok / window.size();
    }

    /** 听力语速倍率:难度越高语速越快(0.8x~1.4x)。 */
    public static double speedMultiplier(int difficulty) {
        int d = Math.max(MIN, Math.min(MAX, difficulty));
        return 0.8 + (d - MIN) * (0.6 / (MAX - MIN));
    }
}
