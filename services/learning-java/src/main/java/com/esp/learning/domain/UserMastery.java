package com.esp.learning.domain;

import jakarta.persistence.*;
import org.hibernate.annotations.UpdateTimestamp;

import java.math.BigDecimal;
import java.time.Instant;

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
