package com.esp.teacher.domain;

import jakarta.persistence.*;

import java.math.BigDecimal;
import java.time.Instant;

/**
 * 作业提交聚合根(ADR-DDD,P2-13 充血化)。
 *
 * <p>P2-13: 将原散落在 {@code TeacherService} 的状态机转换下沉为聚合方法。
 * 状态流转:auto(待评)→ graded(自动评分完成)→ finalized(教师终评确认)。
 * 字段联动(autoScore→status,finalScore→status)封装在聚合根内,Service 只加载→调聚合方法→保存。
 */
@Entity
@Table(name = "submissions")
public class Submission {
    @Id
    private Long id;
    private Long assignmentId;
    private Long userId;
    private String content;
    private BigDecimal autoScore;
    private String autoFeedback;
    private BigDecimal finalScore;
    private String status;
    private Instant createdAt;

    /** 提交状态机常量(与现有数据/调用方对齐:auto_graded 为自动评阅完成态)。 */
    public static final String STATUS_PENDING = "pending";
    public static final String STATUS_GRADED = "auto_graded";
    public static final String STATUS_FINALIZED = "finalized";

    /**
     * P2-13: 应用自动评分。状态:非 auto_graded → auto_graded。
     * 封装原 TeacherService.doAutoGrade 里 setter 裸改 autoScore/autoFeedback/status 的逻辑。
     */
    public void grade(BigDecimal score, String feedback) {
        this.autoScore = score;
        this.autoFeedback = feedback;
        this.status = STATUS_GRADED;
    }

    /**
     * P2-13: 教师终评确认。状态:→ finalized。
     * 终评分覆盖 finalScore,状态置 finalized。已 finalized 时幂等忽略(防重复终评)。
     */
    public void confirm(BigDecimal finalScore) {
        if (STATUS_FINALIZED.equals(this.status)) {
            return; // 已终评,幂等忽略
        }
        this.finalScore = finalScore;
        this.status = STATUS_FINALIZED;
    }

    /** P2-13: saga 回滚专用——恢复到快照前的状态/终评分。 */
    public void rollbackStatus(String prevStatus, BigDecimal prevFinalScore) {
        this.status = prevStatus;
        this.finalScore = prevFinalScore;
    }

    public Long getId() { return id; }
    public void setId(Long id) { this.id = id; }
    public Long getAssignmentId() { return assignmentId; }
    public void setAssignmentId(Long assignmentId) { this.assignmentId = assignmentId; }
    public Long getUserId() { return userId; }
    public void setUserId(Long userId) { this.userId = userId; }
    public String getContent() { return content; }
    public void setContent(String content) { this.content = content; }
    public BigDecimal getAutoScore() { return autoScore; }
    public void setAutoScore(BigDecimal autoScore) { this.autoScore = autoScore; }
    public String getAutoFeedback() { return autoFeedback; }
    public void setAutoFeedback(String autoFeedback) { this.autoFeedback = autoFeedback; }
    public BigDecimal getFinalScore() { return finalScore; }
    public void setFinalScore(BigDecimal finalScore) { this.finalScore = finalScore; }
    public String getStatus() { return status; }
    public void setStatus(String status) { this.status = status; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
}
