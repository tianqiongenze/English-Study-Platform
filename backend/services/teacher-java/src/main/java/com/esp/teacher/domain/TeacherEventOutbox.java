package com.esp.teacher.domain;

import jakarta.persistence.*;
import java.time.Instant;

/**
 * 教师事件 outbox 行(P0-D1,2026-08-21 transactional outbox)。
 *
 * <p>teacher.finalized 等关键事件在 tryFinalize {@code @Transactional} 内同库原子写入此表
 * (与 submission 行同事务),由 {@link com.esp.teacher.outbox.OutboxRelay} 扫表重投 Pulsar。
 * broker 挂时行留存 relay 兜底;事务回滚则行随之回滚(无幽灵事件)——消除原 sendAsync 三重丢事件。
 *
 * <p>与 content-go 的 esp_learning.learning_event_outbox 同模式(统一 outbox:schema-per-service
 * 各自带 relay),本表属 esp_teacher schema。
 *
 * <p>幂等:message_id 唯一,relay 重投时下游 ON CONFLICT (message_id) DO NOTHING 幂等。
 */
@Entity
@Table(name = "teacher_event_outbox",
        uniqueConstraints = @UniqueConstraint(name = "uk_teacher_outbox_msg", columnNames = "message_id"),
        indexes = {
                @Index(name = "idx_teacher_outbox_status_retry", columnList = "status,next_retry_at"),
                @Index(name = "idx_teacher_outbox_user_time", columnList = "user_id,created_at")
        })
public class TeacherEventOutbox {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    /** 幂等键(唯一),如 "teacher:finalized:<submissionId>"。relay 重投下游幂等去重。 */
    @Column(name = "message_id", nullable = false, length = 128, unique = true)
    private String messageId;

    private Long userId;

    /** 事件类型,如 "teacher.finalized"。 */
    @Column(name = "event_type", nullable = false, length = 32)
    private String eventType;

    /** 事件体 JSON(Jackson 序列化的 TeacherFinalizedEvent 等 POJO)。 */
    @Column(nullable = false, columnDefinition = "jsonb")
    private String payload;

    /** pending / done / failed。 */
    @Column(nullable = false, length = 16)
    private String status;

    @Column(name = "retry_count", nullable = false)
    private int retryCount;

    @Column(name = "next_retry_at", nullable = false)
    private Instant nextRetryAt;

    @Column(name = "created_at", nullable = false, updatable = false)
    private Instant createdAt;

    public TeacherEventOutbox() {}

    /** 工厂:构造 pending 状态的待投递行(payload 由调用方序列化好的 JSON 字符串)。 */
    public static TeacherEventOutbox pending(String eventType, String messageId, Long userId, String payloadJson) {
        TeacherEventOutbox o = new TeacherEventOutbox();
        o.eventType = eventType;
        o.messageId = messageId;
        o.userId = userId;
        o.payload = payloadJson;
        o.status = "pending";
        o.retryCount = 0;
        Instant now = Instant.now();
        o.nextRetryAt = now;
        o.createdAt = now;
        return o;
    }

    /** 重载:接收 POJO,内部 Jackson 序列化为 JSON。 */
    public static TeacherEventOutbox pending(String eventType, String messageId, Long userId, Object payloadPojo) {
        try {
            com.fasterxml.jackson.databind.ObjectMapper mapper = new com.fasterxml.jackson.databind.ObjectMapper();
            return pending(eventType, messageId, userId, mapper.writeValueAsString(payloadPojo));
        } catch (com.fasterxml.jackson.core.JsonProcessingException e) {
            throw new IllegalArgumentException("outbox payload serialize failed: " + e.getMessage(), e);
        }
    }

    public Long getId() { return id; }
    public String getMessageId() { return messageId; }
    public Long getUserId() { return userId; }
    public String getEventType() { return eventType; }
    public String getPayload() { return payload; }
    public String getStatus() { return status; }
    public void setStatus(String status) { this.status = status; }
    public int getRetryCount() { return retryCount; }
    public void setRetryCount(int retryCount) { this.retryCount = retryCount; }
    public Instant getNextRetryAt() { return nextRetryAt; }
    public void setNextRetryAt(Instant nextRetryAt) { this.nextRetryAt = nextRetryAt; }
    public Instant getCreatedAt() { return createdAt; }
}
