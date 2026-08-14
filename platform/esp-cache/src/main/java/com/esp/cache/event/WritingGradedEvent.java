package com.esp.cache.event;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * 写作批改完成事件(writing-py → teacher-java)。
 *
 * <p>writing-py 批改完成异步发出;teacher-java 收到后记录/统计。
 * P1-6:新增 userId 字段(writing-py 从 GradeRequest.user_id 注入),
 * teacher-java 收到后据此回写 mastery(桥接 writing.graded → teacher.finalized)。
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class WritingGradedEvent {

    private String status;

    @JsonProperty("thread_id")
    private String threadId;

    @JsonProperty("user_id")
    private long userId;

    /** 批改分数(0~30,来自 GradeResponse.score)。 */
    private double score;

    public WritingGradedEvent() {}

    public String getStatus() { return status; }
    public void setStatus(String status) { this.status = status; }
    public String getThreadId() { return threadId; }
    public void setThreadId(String threadId) { this.threadId = threadId; }
    public long getUserId() { return userId; }
    public void setUserId(long userId) { this.userId = userId; }
    public double getScore() { return score; }
    public void setScore(double score) { this.score = score; }
}
