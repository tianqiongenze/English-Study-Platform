package com.esp.learning.service;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

import org.springframework.stereotype.Component;

/**
 * 艾宾浩斯遗忘曲线复习调度。
 * 间隔序列(天):1, 2, 4, 7, 15, 30。
 * 答错重置 reviewCount=0;答对推进到下一间隔。
 */
@Component
public class EbbinghausScheduler {

    /** 间隔天数序列(2 的近似幂次递增,符合遗忘曲线)。 */
    public static final List<Long> INTERVALS_DAYS = List.of(1L, 2L, 4L, 7L, 15L, 30L);

    /**
     * 计算下次复习时间。
     * @param reviewCount 已成功复习次数(从 0 开始;0 表示初次学习)
     * @param correct 本次是否答对
     * @param from 基准时间
     * @return 下次复习时间
     */
    public Instant nextReviewAt(int reviewCount, boolean correct, Instant from) {
        // 答对:reviewCount=0(初次)->1d;之后取 INTERVALS[reviewCount] 向后推进。
        // 答错:重置为 1d(INTERVALS[0])。
        int idx = correct ? Math.min(Math.max(reviewCount, 0), INTERVALS_DAYS.size() - 1) : 0;
        long days = INTERVALS_DAYS.get(idx);
        return from.plus(Duration.ofDays(days));
    }

    /** 当前间隔(天),用于展示。 */
    public long currentIntervalDays(int reviewCount) {
        int idx = Math.min(Math.max(reviewCount, 0), INTERVALS_DAYS.size() - 1);
        return INTERVALS_DAYS.get(idx);
    }
}
