package com.esp.learning.domain;

import jakarta.persistence.*;
import org.hibernate.annotations.UpdateTimestamp;

import java.math.BigDecimal;
import java.time.Instant;

/**
 * 用户知识点掌握度聚合根(ADR-DDD,P2-13 充血化)。
 *
 * <p>P2-13: 将原散落在 {@code LearningService.review} 的"读-改-写"状态变更下沉为聚合方法,
 * 字段联动(masteryScore/reviewCount/nextReviewAt/lastReviewAt)封装在聚合根内,Service 只负责
 * 加载→调聚合方法→保存,不再逐字段 setter 裸改。保持 JPA 映射与原表结构不变。
 *
 * <p>不变量:
 * <ul>
 *   <li>masteryScore ≥ 0(review 正确时 +1,错误时不变,不下溢)。</li>
 *   <li>reviewCount:答对累加,答错归零(艾宾浩斯间隔重复策略)。</li>
 *   <li>nextReviewAt 由 {@link #applyReviewEvent} 调用方传入(调度器算出的下次时间),聚合根不自算调度。</li>
 * </ul>
 */
@Entity
@Table(name = "user_mastery")
@IdClass(UserMasteryId.class)
public class UserMastery {
    @Id
    private Long userId;
    @Id
    private Long kpId;

    private BigDecimal masteryScore;
    private Instant lastReviewAt;
    private Integer reviewCount;
    private Instant nextReviewAt;

    /**
     * P2-6: 乐观锁版本号。review 的读-改-写在并发复习(同 user 同 kp 双端同时提交)下
     * 防 lost update,Hibernate 在 UPDATE 追加 WHERE version=? 并自增。
     * 表列 version INT NOT NULL DEFAULT 0,INSERT 时由构造器显式置 0 兜底。
     */
    @Version
    private Integer version;

    /**
     * P2-6: 记录最近更新时间,用于读写分离下检测读后写(follower lag 时从库读出的 updated_at
     * 滞后,业务可据此判断是否需要强制读主)。由 Hibernate 自动写回。
     */
    @UpdateTimestamp
    private Instant updatedAt;

    public UserMastery() {}

    public UserMastery(Long userId, Long kpId) {
        this.userId = userId;
        this.kpId = kpId;
        this.masteryScore = BigDecimal.ZERO;
        this.reviewCount = 0;
        this.version = 0;
    }

    /**
     * P2-13: 应用一次复习事件,联动更新 masteryScore/reviewCount/lastReviewAt/nextReviewAt。
     *
     * <p>封装原 {@code LearningService.review} 里散落的 setter 逻辑,保证字段联动一致性:
     * 答对 → reviewCount+1 且 masteryScore+1;答错 → reviewCount 归零且 masteryScore 不变。
     * nextReviewAt/lastReviewAt 由调用方(持调度器)传入。
     *
     * @param correct        本次是否答对
     * @param nextReviewAt   调度器算出的下次复习时间
     * @param now            本次复习发生时间(写 lastReviewAt)
     */
    public void applyReviewEvent(boolean correct, Instant nextReviewAt, Instant now) {
        int count = reviewCount == null ? 0 : reviewCount;
        this.lastReviewAt = now;
        this.nextReviewAt = nextReviewAt;
        this.reviewCount = correct ? count + 1 : 0;
        BigDecimal score = masteryScore == null ? BigDecimal.ZERO : masteryScore;
        this.masteryScore = correct ? score.add(BigDecimal.ONE) : score;
    }

    /**
     * P2-13: 教师终评覆盖掌握度(teacher.finalized 事件路径)。
     * 直接以教师终评分覆盖 masteryScore,重置 reviewCount 为 0(交由后续复习重新累计)。
     */
    public void applyTeacherFinalized(BigDecimal finalScore, Instant now) {
        this.masteryScore = finalScore;
        this.lastReviewAt = now;
        if (this.reviewCount == null) this.reviewCount = 0;
    }

    /**
     * P2-13: 初始化基线分数(初测路径)。
     */
    public void initBaseline(BigDecimal initialScore, Instant now) {
        this.masteryScore = initialScore;
        this.reviewCount = 0;
        this.lastReviewAt = now;
    }

    public Long getUserId() { return userId; }
    public void setUserId(Long userId) { this.userId = userId; }
    public Long getKpId() { return kpId; }
    public void setKpId(Long kpId) { this.kpId = kpId; }
    public BigDecimal getMasteryScore() { return masteryScore; }
    public void setMasteryScore(BigDecimal masteryScore) { this.masteryScore = masteryScore; }
    public Instant getLastReviewAt() { return lastReviewAt; }
    public void setLastReviewAt(Instant lastReviewAt) { this.lastReviewAt = lastReviewAt; }
    public Integer getReviewCount() { return reviewCount; }
    public void setReviewCount(Integer reviewCount) { this.reviewCount = reviewCount; }
    public Instant getNextReviewAt() { return nextReviewAt; }
    public void setNextReviewAt(Instant nextReviewAt) { this.nextReviewAt = nextReviewAt; }
    public Integer getVersion() { return version; }
    public void setVersion(Integer version) { this.version = version; }
    public Instant getUpdatedAt() { return updatedAt; }
    public void setUpdatedAt(Instant updatedAt) { this.updatedAt = updatedAt; }
}
