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
 *
 * <p>G-06 修复:flush 不再【同步】调用 consumer.consume,而是把批次【深拷贝成独立
 * List&lt;LearningEvent&gt;】后非阻塞投递到 {@link EventBatchQueue},由独立 DB writer 线程池异步
 * batchUpdate。深拷贝必须在此边界完成 —— Disruptor 槽位会被后续 publish 复用,队列/DB writer
 * 线程绝不能持有槽引用。
 */
public class BatchEventHandler implements EventHandler<LearningEvent> {

    private static final Logger log = LoggerFactory.getLogger(BatchEventHandler.class);

    private final EventBatchQueue batchQueue;
    private final int batchSize;
    private final long flushIntervalMs;
    private final int shardIndex;
    private final int shardCount;
    private final List<LearningEvent> buffer;
    private long lastFlushTime;
    private final String name;

    /** 单消费者兼容构造(shardCount=1,处理所有事件)。 */
    public BatchEventHandler(EventBatchQueue batchQueue, int batchSize, long flushIntervalMs) {
        this(batchQueue, batchSize, flushIntervalMs, 0, 1, "batch-handler");
    }

    public BatchEventHandler(EventBatchQueue batchQueue, int batchSize, long flushIntervalMs,
                             int shardIndex, int shardCount, String name) {
        this.batchQueue = batchQueue;
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
        // 深拷贝 RingBuffer 槽位:槽会被后续 publish 复用,缓冲绝不能持有槽引用。
        LearningEvent copy = new LearningEvent();
        copy.setUserId(event.getUserId());
        copy.setEventType(event.getEventType());
        copy.setPayload(event.getPayload());
        copy.setCreatedAt(event.getCreatedAt());
        copy.setMessageId(event.getMessageId()); // P1-8 幂等键透传
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
        // 生成独立批次列表(元素为 onEvent 深拷贝的独立对象),投递后 buffer.clear()。
        // 入队边界 EventBatchQueue.offer 还会再深拷贝一次,确保队列/DB writer 线程绝不
        // 持有 Disruptor 复用槽或本 handler 缓冲区的引用。
        List<LearningEvent> batch = new ArrayList<>(buffer);
        // 非阻塞投递:队列满 → 计数 DROP(EventBatchQueue 内部记 dropped),不阻塞 consumer 线程。
        batchQueue.offer(batch);
        buffer.clear();
        lastFlushTime = System.currentTimeMillis();
    }
}
