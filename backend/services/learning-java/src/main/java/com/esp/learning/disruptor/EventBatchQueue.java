package com.esp.learning.disruptor;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 有界事件批次队列 + 独立 DB writer 线程池(架构审查 G-06 修复)。
 *
 * <p>背景:原 Disruptor consumer 线程在 flush 时【同步】调用 {@link LearningEventConsumer#consume}
 * 批量落库,DB 延迟直接反压 RingBuffer → publish DROP 甚至 503。本类把"consumer 入队"与"DB 写"
 * 拆成两个角色:
 * <ul>
 *   <li>Disruptor consumer 线程(在 {@link BatchEventHandler#flush()} 中)把批次【深拷贝成独立
 *       List&lt;LearningEvent&gt;】后经 {@link #offer(List)} 非阻塞投递到有界 {@link BlockingQueue}。</li>
 *   <li>独立 DB writer 线程池(数量 {@code esp.disruptor.writers},默认 4)取出批次异步
 *       {@link LearningEventConsumer#consume} batchUpdate,与 RingBuffer 满 DROP 同语义的背压。</li>
 * </ul>
 *
 * <p>队列满 → {@link #offer} 返回 false 并计数 DROP,不阻塞 consumer 线程(与 RingBuffer 满 DROP
 * 语义一致,保护网关响应时间)。
 *
 * <p>深拷贝铁律:入队批次必须持有独立 LearningEvent 对象,不可持有 Disruptor 复用槽引用
 * (槽会被后续 publish 的 translator 覆盖)。调用方({@link BatchEventHandler#flush})负责在投递前
 * 深拷贝;本类对传入 batch 不再拷贝,语义见 {@link #offer}。
 */
public class EventBatchQueue implements AutoCloseable {

    private static final Logger log = LoggerFactory.getLogger(EventBatchQueue.class);

    private final BlockingQueue<List<LearningEvent>> queue;
    private final LearningEventConsumer consumer;
    private final int capacity;
    private final int writerCount;
    private final List<Thread> writers = new ArrayList<>();
    private final AtomicLong dropped = new AtomicLong();
    private final AtomicLong offered = new AtomicLong();
    private volatile boolean running;

    /**
     * @param consumer  实际落库消费者({@link DbLearningEventConsumer})
     * @param capacity  有界队列容量(单位:批,约 1 批=batchSize 事件;默认 100 ≈ 20000 事件)
     * @param writerCount DB writer 线程数(默认 4;范围 1..16 防单节点线程爆炸)
     */
    public EventBatchQueue(LearningEventConsumer consumer, int capacity, int writerCount) {
        this.consumer = consumer;
        this.capacity = Math.max(1, capacity);
        this.writerCount = Math.max(1, Math.min(16, writerCount));
        this.queue = new LinkedBlockingQueue<>(this.capacity);
        for (int i = 0; i < this.writerCount; i++) {
            Thread t = new Thread(this::run, "learning-db-writer-" + i);
            t.setDaemon(true);
            writers.add(t);
        }
    }

    /** 启动 writer 线程池(幂等)。 */
    public synchronized void start() {
        if (running) return;
        running = true;
        for (Thread t : writers) {
            t.start();
        }
        log.info("EventBatchQueue started: capacity={} batches, writers={}", capacity, writerCount);
    }

    /**
     * 非阻塞入队(与 RingBuffer 满 DROP 同语义)。队列满返回 false(计数 DROP),不阻塞调用方。
     *
     * <p>深拷贝铁律:入队边界再次深拷贝批次 —— 队列/DB writer 线程绝不持有 Disruptor 复用槽
     * 或调用方缓冲区的引用。即便调用方在 {@link #offer} 返回后清空/复用传入批次,队列内
     * 仍是独立对象(不变量:offered batch 与队列零共享引用)。
     *
     * @return true=已入队;false=队列满 DROP(计数 +1)
     */
    public boolean offer(List<LearningEvent> batch) {
        offered.incrementAndGet();
        List<LearningEvent> deepCopy = new ArrayList<>(batch.size());
        for (LearningEvent e : batch) {
            LearningEvent c = new LearningEvent();
            c.setUserId(e.getUserId());
            c.setEventType(e.getEventType());
            c.setPayload(e.getPayload());
            c.setCreatedAt(e.getCreatedAt());
            c.setMessageId(e.getMessageId()); // P1-8 幂等键透传
            deepCopy.add(c);
        }
        if (!queue.offer(deepCopy)) {
            dropped.incrementAndGet();
            log.warn("EventBatchQueue full, batch of {} events DROPPED (queueSize={}/capacity={})",
                    batch.size(), queue.size(), capacity);
            return false;
        }
        return true;
    }

    private void run() {
        while (running) {
            List<LearningEvent> batch;
            try {
                batch = queue.poll(200, TimeUnit.MILLISECONDS);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            }
            if (batch == null) {
                continue;
            }
            try {
                consumer.consume(batch.toArray(new LearningEvent[0]), 0, batch.size());
            } catch (Exception ex) {
                // consumer 内部已 catch(如 DbLearningEventConsumer batch 失败转 outbox);
                // 此处兜底防 writer 线程因未捕获异常挂死。
                log.error("db writer consume failed: {}", ex.toString());
            }
        }
        // 停机排空:优雅关闭时不丢在途批次。
        drain();
    }

    private void drain() {
        List<LearningEvent> batch;
        while ((batch = queue.poll()) != null) {
            try {
                consumer.consume(batch.toArray(new LearningEvent[0]), 0, batch.size());
            } catch (Exception ex) {
                log.error("db writer drain failed: {}", ex.toString());
            }
        }
    }

    /** 停止 writer 线程池并排空剩余批次(阻塞至 writer 退出)。 */
    public void shutdown() {
        if (!running) return;
        running = false;
        for (Thread t : writers) {
            t.interrupt();
        }
        for (Thread t : writers) {
            try {
                t.join(5000);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }
        log.info("EventBatchQueue shutdown: dropped={}, queueRemaining={}", dropped.get(), queue.size());
    }

    public int queueSize() { return queue.size(); }
    public int capacity() { return capacity; }
    public long droppedCount() { return dropped.get(); }
    public long offeredCount() { return offered.get(); }

    @Override
    public void close() { shutdown(); }
}
