package com.esp.disruptor;

/**
 * 发布结果。背压语义:队列满时返回 {@link #DROPPED},由调用方决定丢弃/降级/重试。
 */
public enum PublishResult {
    /** 成功入队。 */
    PUBLISHED,
    /** 队列满,背压丢弃(调用方应记指标/降级)。 */
    DROPPED
}
