package com.esp.learning.disruptor;

import com.esp.disruptor.DisruptorStats;
import com.esp.disruptor.DisruptorWorker;
import com.esp.disruptor.PublishResult;
import com.esp.disruptor.WaitStrategyType;
import com.lmax.disruptor.dsl.ProducerType;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;

/**
 * 学情事件 Disruptor 编排器(基于 ESP 共用 {@link DisruptorWorker},ADR-09)。
 *
 * <p>封装了 LMAX Disruptor 的 RingBuffer/序列号/事件工厂等底层细节,
 * 本类只负责:把学情入口的 (userId,eventType,payload) 三字段投递到共用 worker,
 * 并保留对外 publish()/publishedCount()/droppedCount() API(供 LearningService 调用)。
 *
 * <p>多生产者:Netty worker / REST 线程并发投递 -> {@link ProducerType#MULTI}。
 * 多消费者:分片并行模式,N 个 BatchEventHandler 并行批量落库(打破单消费者串行 INSERT 上限,
 * 参数 {@code esp.disruptor.consumers},默认 4)。Disruptor 4.0 已移除 WorkerPool/WorkHandler,
 * 这里用多 EventHandler(每条事件广播给所有 handler)+ handler 内按 {@code sequence % shardCount == shardIndex}
 * 分片过滤,实现每条事件只被一个消费者落库(等效 WorkerPool 瓜分语义)。
 *
 * <p>原内联 Disruptor 实现已删除,统一复用 com.esp:esp-disruptor:1.0.0。
 *
 * <p>P2-2: 加 {@code @ConditionalOnProperty("esp.disruptor.enabled")} 开关。
 * 默认 true(单进程一体化部署,HTTP 入口与 Disruptor 消费同进程)。
 * 拆分 learning-worker 独立 Deployment 时:HTTP 入口侧设 {@code esp.disruptor.enabled=false}
 * (入口只走 Pulsar 投递,不启 Disruptor 消费);worker 侧设 true(只跑 Disruptor 消费+落库)。
 * 当入口侧 disabled 时,本 Bean 不装配,publish() 不可用 —— 调用方(LearningService.reportEvent)
 * 需改走 Pulsar 投递(见 LearningService.P2-2 说明)。
 */
@Component
@ConditionalOnProperty(name = "esp.disruptor.enabled", havingValue = "true", matchIfMissing = true)
public class LearningEventDisruptor {

    private static final Logger log = LoggerFactory.getLogger(LearningEventDisruptor.class);

    private final int bufferSize;
    private final int batchSize;
    private final long flushIntervalMs;
    private final int consumerCount;
    private final LearningEventConsumer consumer;
    private final JdbcTemplate writeJdbc;

    private DisruptorWorker<LearningEvent> worker;
    private final List<BatchEventHandler> handlers = new ArrayList<>();

    /**
     * translator 输入复用槽:DisruptorWorker.publish(T) 接收一个 payload 对象,
     * translator 把它的字段拷进 RingBuffer 复用槽。学情调用方传的是三个原始字段,
     * 这里用 ThreadLocal 持有一个临时 LearningEvent 避免每次 publish 都 new,
     * 同时保证多生产者并发安全(每个线程独占一份)。
     */
    private final ThreadLocal<LearningEvent> translatorInput = ThreadLocal.withInitial(LearningEvent::new);

    /**
     * P1-10 关键事件白名单:这些 eventType 在 RingBuffer 满时落 outbox 兜底,不可 DROP。
     * 埋点类(page_view/practice_attempt 等)可接受 DROP,不在此列。
     */
    private static final Set<String> CRITICAL_EVENT_TYPES = Set.of(
            "session_ended", "placement_done", "difficulty_changed", "mastery_updated");

    public LearningEventDisruptor(
            @Value("${esp.disruptor.buffer-size:65536}") int bufferSize,
            @Value("${esp.disruptor.batch-size:200}") int batchSize,
            @Value("${esp.disruptor.flush-interval-ms:500}") long flushIntervalMs,
            @Value("${esp.disruptor.consumers:4}") int consumerCount,
            LearningEventConsumer consumer,
            JdbcTemplate writeJdbc) {
        this.bufferSize = bufferSize;
        this.batchSize = batchSize;
        this.flushIntervalMs = flushIntervalMs;
        // 至少 1 个消费者;上限 16(单节点 RD 防线程爆炸,TiDB 连接池容量约束)。
        this.consumerCount = Math.max(1, Math.min(16, consumerCount));
        this.consumer = consumer;
        this.writeJdbc = writeJdbc;
    }

    @PostConstruct
    public void start() {
        com.lmax.disruptor.EventHandler<LearningEvent>[] pool =
                new com.lmax.disruptor.EventHandler[consumerCount];
        for (int i = 0; i < consumerCount; i++) {
            // 每个 handler 持有自己的分片索引(i)与总分片数(consumerCount),
            // onEvent 内按 sequence % shardCount == shardIndex 过滤,实现瓜分语义。
            BatchEventHandler h = new BatchEventHandler(consumer, batchSize, flushIntervalMs,
                    i, consumerCount, "learning-batch-" + i);
            handlers.add(h);
            pool[i] = h;
        }
        worker = DisruptorWorker.<LearningEvent>builder()
                .name("learning-event")
                .bufferSize(bufferSize)
                .producerType(ProducerType.MULTI)
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(LearningEvent::new)
                .translator(new com.lmax.disruptor.EventTranslatorOneArg<LearningEvent, LearningEvent>() {
                    @Override
                    public void translateTo(LearningEvent slot, long sequence, LearningEvent src) {
                        slot.clear();
                        slot.setUserId(src.getUserId());
                        slot.setEventType(src.getEventType());
                        slot.setPayload(src.getPayload());
                        slot.setCreatedAt(src.getCreatedAt());
                    }
                })
                .poolHandlers(pool)
                .build();
        worker.start();
        log.info("DisruptorWorker[learning-event] started: bufferSize={}, batchSize={}, flushMs={}, consumers={}",
                bufferSize, batchSize, flushIntervalMs, consumerCount);
    }

    /**
     * 发布事件(非阻塞,背压丢弃)。RingBuffer 满时返回 false(记 dropped)。
     * 保护网关响应时间,不死等。
     *
     * <p>P1-10 outbox 兜底:关键事件(security/教务类,见 {@link #CRITICAL_EVENT_TYPES})
     * 在 RingBuffer 满(DROP)时落 {@code learning_event_outbox} 表,由 OutboxRelay 异步重投,
     * 防关键业务数据丢失;埋点类可接受 DROP(不影响业务正确性)。
     */
    public boolean publish(long userId, String eventType, String payload) {
        LearningEvent src = translatorInput.get();
        src.setUserId(userId);
        src.setEventType(eventType);
        src.setPayload(payload);
        src.setCreatedAt(System.currentTimeMillis());
        PublishResult r = worker.publish(src);
        if (r == PublishResult.PUBLISHED) {
            return true;
        }
        // DROP:关键事件落 outbox 兜底,非关键丢弃。
        if (CRITICAL_EVENT_TYPES.contains(eventType)) {
            fallbackToOutbox(userId, eventType, payload);
        }
        return false;
    }

    /** P1-10:关键事件 DROP 时落 outbox(异步,不阻塞发布线程)。 */
    private void fallbackToOutbox(long userId, String eventType, String payload) {
        try {
            writeJdbc.update(
                    "INSERT INTO learning_event_outbox (user_id, event_type, payload, status) VALUES (?,?,?, 'pending')",
                    userId, eventType, payload);
            log.warn("critical event DROP-fallback to outbox: user={} type={}", userId, eventType);
        } catch (Exception ex) {
            // outbox 也失败(Redis/DB 双障)→ 尽力而为记日志,不抛(不阻断入口响应)。
            log.error("outbox fallback FAILED (data loss risk): user={} type={} err={}",
                    userId, eventType, ex.toString());
        }
    }

    public long publishedCount() {
        return worker == null ? 0L : worker.stats().published();
    }

    public long droppedCount() {
        return worker == null ? 0L : worker.stats().dropped();
    }

    /** 暴露完整指标快照(供 actuator/监控)。 */
    public DisruptorStats stats() {
        return worker == null ? null : worker.stats();
    }

    @PreDestroy
    public void shutdown() {
        log.info("DisruptorWorker[learning-event] shutting down, draining batch buffers...");
        // 先 flush 所有并行消费者的批处理缓存,确保在途事件落库;worker.shutdown() 会阻塞至消费者追上。
        for (BatchEventHandler h : handlers) {
            h.drain();
        }
        if (worker != null) worker.shutdown();
    }
}
