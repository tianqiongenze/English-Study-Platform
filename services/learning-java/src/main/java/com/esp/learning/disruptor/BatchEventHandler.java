package com.esp.learning.disruptor;

import com.lmax.disruptor.EventHandler;

import java.util.ArrayList;
import java.util.List;

/**
 * 批量事件处理器:缓存到 batchSize 后 flush 给下游 consumer。
 * 用 batchSize + 超时 flush 兼顾吞吐与延迟。
 */
public class BatchEventHandler implements EventHandler<LearningEvent> {

    private final LearningEventConsumer consumer;
    private final int batchSize;
    private final long flushIntervalMs;
    private final List<LearningEvent> buffer;
    private long lastFlushTime;

    public BatchEventHandler(LearningEventConsumer consumer, int batchSize, long flushIntervalMs) {
        this.consumer = consumer;
        this.batchSize = batchSize;
        this.flushIntervalMs = flushIntervalMs;
        this.buffer = new ArrayList<>(batchSize);
        this.lastFlushTime = System.currentTimeMillis();
    }

    @Override
    public synchronized void onEvent(LearningEvent event, long sequence, boolean endOfBatch) {
        LearningEvent copy = new LearningEvent();
        copy.setUserId(event.getUserId());
        copy.setEventType(event.getEventType());
        copy.setPayload(event.getPayload());
        copy.setCreatedAt(event.getCreatedAt());
        buffer.add(copy);

        long now = System.currentTimeMillis();
        boolean sizeReached = buffer.size() >= batchSize;
        boolean timeout = (now - lastFlushTime) >= flushIntervalMs;
        if (sizeReached || (endOfBatch && timeout)) {
            flush();
        }
    }

    /** 优雅停机时强制 flush。 */
    public synchronized void drain() {
        if (!buffer.isEmpty()) {
            flush();
        }
    }

    private void flush() {
        if (buffer.isEmpty()) {
            lastFlushTime = System.currentTimeMillis();
            return;
        }
        LearningEvent[] arr = buffer.toArray(new LearningEvent[0]);
        consumer.consume(arr, 0, arr.length);
        buffer.clear();
        lastFlushTime = System.currentTimeMillis();
    }
}
