package com.esp.learning.disruptor;

/**
 * 学情事件(Disruptor 载荷)。
 * 高频写入场景:每次答题/口语片段产生一条。
 */
public class LearningEvent {
    private long userId;
    private String eventType; // answer | speak | review ...
    private String payload;   // JSON
    private long createdAt;   // epoch millis

    public long getUserId() { return userId; }
    public void setUserId(long userId) { this.userId = userId; }

    public String getEventType() { return eventType; }
    public void setEventType(String eventType) { this.eventType = eventType; }

    public String getPayload() { return payload; }
    public void setPayload(String payload) { this.payload = payload; }

    public long getCreatedAt() { return createdAt; }
    public void setCreatedAt(long createdAt) { this.createdAt = createdAt; }

    /** Disruptor 复用槽位时清空,避免脏数据。 */
    public void clear() {
        userId = 0;
        eventType = null;
        payload = null;
        createdAt = 0;
    }
}
