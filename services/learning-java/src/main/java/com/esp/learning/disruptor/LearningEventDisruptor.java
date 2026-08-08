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
import org.springframework.stereotype.Component;

/**
 * 学情事件 Disruptor 编排器(基于 ESP 共用 {@link DisruptorWorker},ADR-09)。
 *
 * <p>封装了 LMAX Disruptor 的 RingBuffer/序列号/事件工厂等底层细节,
 * 本类只负责:把学情入口的 (userId,eventType,payload) 三字段投递到共用 worker,
 * 并保留对外 publish()/publishedCount()/droppedCount() API(供 LearningService 调用)。
 *
 * <p>多生产者:Netty worker / REST 线程并发投递 -> {@link ProducerType#MULTI}。
 * 单消费者:BatchEventHandler 顺序批量落库(保持原有顺序语义)。
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
    private final LearningEventConsumer consumer;

    private DisruptorWorker<LearningEvent> worker;
    private BatchEventHandler handler;

    /**
     * translator 输入复用槽:DisruptorWorker.publish(T) 接收一个 payload 对象,
     * translator 把它的字段拷进 RingBuffer 复用槽。学情调用方传的是三个原始字段,
     * 这里用 ThreadLocal 持有一个临时 LearningEvent 避免每次 publish 都 new,
     * 同时保证多生产者并发安全(每个线程独占一份)。
     */
    private final ThreadLocal<LearningEvent> translatorInput = ThreadLocal.withInitial(LearningEvent::new);

    public LearningEventDisruptor(
            @Value("${esp.disruptor.buffer-size:65536}") int bufferSize,
            @Value("${esp.disruptor.batch-size:200}") int batchSize,
            @Value("${esp.disruptor.flush-interval-ms:500}") long flushIntervalMs,
            LearningEventConsumer consumer) {
        this.bufferSize = bufferSize;
        this.batchSize = batchSize;
        this.flushIntervalMs = flushIntervalMs;
        this.consumer = consumer;
    }

    @PostConstruct
    public void start() {
        handler = new BatchEventHandler(consumer, batchSize, flushIntervalMs);
        worker = DisruptorWorker.<LearningEvent>builder()
                .name("learning-event")
                .bufferSize(bufferSize)
                .producerType(ProducerType.MULTI)
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(LearningEvent::new)
                .translator((LearningEvent slot, long sequence, LearningEvent src) -> {
                    slot.clear();
                    slot.setUserId(src.getUserId());
                    slot.setEventType(src.getEventType());
                    slot.setPayload(src.getPayload());
                    slot.setCreatedAt(src.getCreatedAt());
                })
                .handler(handler)
                .build();
        worker.start();
        log.info("DisruptorWorker[learning-event] started: bufferSize={}, batchSize={}, flushMs={}",
                bufferSize, batchSize, flushIntervalMs);
    }

    /**
     * 发布事件(非阻塞,背压丢弃)。RingBuffer 满时返回 false(记 dropped)。
     * 保护网关响应时间,不死等。
     */
    public boolean publish(long userId, String eventType, String payload) {
        LearningEvent src = translatorInput.get();
        src.setUserId(userId);
        src.setEventType(eventType);
        src.setPayload(payload);
        src.setCreatedAt(System.currentTimeMillis());
        return worker.publish(src) == PublishResult.PUBLISHED;
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
        log.info("DisruptorWorker[learning-event] shutting down, draining batch buffer...");
        // 先 flush 批处理缓存,确保在途事件落库;worker.shutdown() 会阻塞至消费者追上。
        if (handler != null) handler.drain();
        if (worker != null) worker.shutdown();
    }
}
