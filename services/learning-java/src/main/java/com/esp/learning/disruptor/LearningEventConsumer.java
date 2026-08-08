package com.esp.learning.disruptor;

/**
 * 事件消费者接口(批量落库 + 触发算法)。
 * 实现可替换为:TiDB 批量 insert / Pulsar 发布 / 算法触发。
 */
public interface LearningEventConsumer {
    /**
     * 批量处理事件。
     * @param events 事件数组
     * @param offset  起始下标
     * @param length  数量
     */
    void consume(LearningEvent[] events, int offset, int length);
}
