package com.esp.cache.event;

/**
 * 教师终审完成事件(teacher-java → learning-java)。
 * learning-java 收到后 upsert user_mastery(对应 kp 的 mastery_score = finalScore)。
 * kpId 来源:submission 关联的 assignment/kp(本期 submission 表无 kp 字段,
 * 暂用 assignmentId 作为占位 kpId;真实 kp 映射待 writing-py 批改回填)。
 */
public class TeacherFinalizedEvent {
    private long userId;
    private Long kpId;
    private double finalScore;

    public TeacherFinalizedEvent() {}

    public TeacherFinalizedEvent(long userId, Long kpId, double finalScore) {
        this.userId = userId;
        this.kpId = kpId;
        this.finalScore = finalScore;
    }

    public long getUserId() { return userId; }
    public void setUserId(long userId) { this.userId = userId; }
    public Long getKpId() { return kpId; }
    public void setKpId(Long kpId) { this.kpId = kpId; }
    public double getFinalScore() { return finalScore; }
    public void setFinalScore(double finalScore) { this.finalScore = finalScore; }
}
