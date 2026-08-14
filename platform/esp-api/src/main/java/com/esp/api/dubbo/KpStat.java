package com.esp.api.dubbo;

import java.io.Serializable;
import java.util.List;

/**
 * 知识点掌握统计(teacher-java 经 Dubbo Triple 调 learning-java 时返回)。
 * 对应 API 契约 §7:`LearningRpcService.userStats(Long userId) -> List<KpStat>`。
 */
public class KpStat implements Serializable {
    private static final long serialVersionUID = 1L;
    private long kpId;
    private double masteryScore;
    private int reviewCount;
    private String nextReviewAt;

    public KpStat() {}
    public KpStat(long kpId, double masteryScore, int reviewCount, String nextReviewAt) {
        this.kpId = kpId; this.masteryScore = masteryScore; this.reviewCount = reviewCount; this.nextReviewAt = nextReviewAt;
    }
    public long getKpId() { return kpId; }
    public void setKpId(long kpId) { this.kpId = kpId; }
    public double getMasteryScore() { return masteryScore; }
    public void setMasteryScore(double masteryScore) { this.masteryScore = masteryScore; }
    public int getReviewCount() { return reviewCount; }
    public void setReviewCount(int reviewCount) { this.reviewCount = reviewCount; }
    public String getNextReviewAt() { return nextReviewAt; }
    public void setNextReviewAt(String nextReviewAt) { this.nextReviewAt = nextReviewAt; }

    public static List<KpStat> empty() { return List.of(); }
}
