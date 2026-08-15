package com.esp.learning.disruptor;

import com.lmax.disruptor.EventHandler;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;

/**
 * 批量事件处理器(分片并行模式)。
 *
 * <p>Disruptor 4.0 已移除 WorkerPool/WorkHandler,无法用 WorkProcessor 瓜分序列号。本类改用
 * 多 EventHandler 并行(每条事件广播给所有 handler),每个实例按 {@code sequence % shardCount == shardIndex}
 * 过滤,只处理落到自己分片的事件 —— 等效 WorkerPool 的瓜分语义,实现每条事件只被一个消费者落库。
 *
 * <p>历史:原单消费者 + {@code synchronized}。单消费者模型下 onEvent 本就单线程(Disruptor 保证),
 * synchronized 仅保护 drain() 不与消费竞争,属冗余;改为分片并行后,每个 handler 实例仍由单个
 * BatchEventProcessor 线程驱动(onEvent 不并发),synchronized 同样冗余,已移除。
 *
 * <p>timeout flush:EventHandler.onEvent 带 {@code endOfBatch} 信号,本批次末尾若超时则触发 flush,
 * 不需独立守护线程(低频场景由 endOfBatch 兜底;高频由 batchSize 触发)。
 */
public class BatchEventHandler implements EventHandler<LearningEvent> {

    private static final Logger log = LoggerFactory.getLogger(BatchEventHandler.class);

    private final LearningEventConsumer consumer;
    private final int batchSize;
    private final long flushIntervalMs;
    private final int shardIndex;
    private final int shardCount;
    private final List<LearningEvent> buffer;
    private long lastFlushTime;
    private final String name;

    /** 单消费者兼容构造(shardCount=1,处理所有事件)。 */
    public BatchEventHandler(LearningEventConsumer consumer, int batchSize, long flushIntervalMs) {
        this(consumer, batchSize, flushIntervalMs, 0, 1, "batch-handler");
    }

    public BatchEventHandler(LearningEventConsumer consumer, int batchSize, long flushIntervalMs,
                             int shardIndex, int shardCount, String name) {
        this.consumer = consumer;
        this.batchSize = batchSize;
        this.flushIntervalMs = flushIntervalMs;
        this.shardIndex = shardIndex;
        this.shardCount = Math.max(1, shardCount);
        this.buffer = new ArrayList<>(batchSize);
        this.lastFlushTime = System.currentTimeMillis();
        this.name = name;
    }

    @Override
    public void onEvent(LearningEvent event, long sequence, boolean endOfBatch) {
        // 分片过滤:每条事件只被一个 handler 实际处理(等效 WorkerPool 瓜分)。
        if (shardCount > 1 && (sequence % shardCount) != shardIndex) {
            return;
        }
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
    public void drain() {
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
        try {
            consumer.consume(arr, 0, arr.length);
        } catch (Exception ex) {
            // consumer 内部已 catch 并记日志;此处兜底防 flush 抛出打断消费线程。
            log.error("[{}] flush consume failed: {}", name, ex.toString());
        }
        buffer.clear();
        lastFlushTime = System.currentTimeMillis();
    }
}
