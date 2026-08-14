package com.esp.cache.event;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * 口语会话结束事件(gateway-go → learning-java)。
 *
 * <p>gateway-go 会话 End 时异步发出;learning-java 收到后经 Disruptor 记一条 session_ended
 * 学情事件,驱动 mastery 复习调度。字段 snake_case 对齐 gateway-go producer。
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class SessionEndedEvent {

    @JsonProperty("session_id")
    private String sessionId;

    @JsonProperty("user_id")
    private long userId;

    public SessionEndedEvent() {}

    public String getSessionId() { return sessionId; }
    public void setSessionId(String sessionId) { this.sessionId = sessionId; }
    public long getUserId() { return userId; }
    public void setUserId(long userId) { this.userId = userId; }
}
