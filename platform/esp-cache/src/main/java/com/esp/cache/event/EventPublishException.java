package com.esp.cache.event;

/**
 * P0-D1(2026-08-21):可靠事件发布失败异常。
 *
 * <p>{@link EventPublisher#publishReliable} 在 broker 不可达或同步发送失败时抛出,
 * 调用方据此降级为重试/死信告警,而非静默丢消息(原 {@code publish} 只 warn 不抛致
 * teacher.finalized 等关键事件丢失,mastery 评分永久缺失)。
 */
public class EventPublishException extends Exception {

    public EventPublishException(String message) {
        super(message);
    }

    public EventPublishException(String message, Throwable cause) {
        super(message, cause);
    }
}
