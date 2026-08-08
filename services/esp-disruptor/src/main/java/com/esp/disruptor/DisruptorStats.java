package com.esp.disruptor;

/**
 * Disruptor 队列指标快照(不可变)。供 actuator/监控暴露。
 */
public record DisruptorStats(
        String name,
        long bufferSize,
        long published,
        long dropped,
        long consumed) {

    /** 已入队但未消费的滞留量(近似)。 */
    public long backlog() {
        return Math.max(0, published - consumed);
    }
}
