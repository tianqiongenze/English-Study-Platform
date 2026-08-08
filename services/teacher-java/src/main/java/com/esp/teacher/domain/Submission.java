package com.esp.teacher.domain;

import jakarta.persistence.*;

import java.time.Instant;

@Entity
@Table(name = "submissions")
public class Submission {
    @Id
    private Long id;
    private Long assignmentId;
    private Long userId;
    private String content;
    private java.math.BigDecimal autoScore;
    private String autoFeedback;
    private java.math.BigDecimal finalScore;
    private String status;
    private Instant createdAt;

    public Long getId() { return id; }
    public void setId(Long id) { this.id = id; }
    public Long getAssignmentId() { return assignmentId; }
    public void setAssignmentId(Long assignmentId) { this.assignmentId = assignmentId; }
    public Long getUserId() { return userId; }
    public void setUserId(Long userId) { this.userId = userId; }
    public String getContent() { return content; }
    public void setContent(String content) { this.content = content; }
    public java.math.BigDecimal getAutoScore() { return autoScore; }
    public void setAutoScore(java.math.BigDecimal autoScore) { this.autoScore = autoScore; }
    public String getAutoFeedback() { return autoFeedback; }
    public void setAutoFeedback(String autoFeedback) { this.autoFeedback = autoFeedback; }
    public java.math.BigDecimal getFinalScore() { return finalScore; }
    public void setFinalScore(java.math.BigDecimal finalScore) { this.finalScore = finalScore; }
    public String getStatus() { return status; }
    public void setStatus(String status) { this.status = status; }
    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }
}
