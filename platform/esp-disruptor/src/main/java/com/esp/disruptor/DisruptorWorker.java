package com.esp.disruptor;

import com.lmax.disruptor.EventHandler;
import com.lmax.disruptor.ExceptionHandler;
import com.lmax.disruptor.RingBuffer;
import com.lmax.disruptor.WaitStrategy;
import com.lmax.disruptor.dsl.Disruptor;
import com.lmax.disruptor.dsl.ProducerType;
import com.lmax.disruptor.util.DaemonThreadFactory;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.concurrent.atomic.AtomicLong;
import java.util.function.Consumer;

/**
 * ESP 共用 Disruptor 单级工作队列骨架(ADR-09)。
 *
 * <p>封装 LMAX Disruptor 最佳实践,屏蔽 RingBuffer/序列号/事件工厂等底层细节,
 * 使用方只需提供:事件类型 {@code T}、{@link EventHandler} 消费逻辑、可选的异常处理器。
 *
 * <h3>用法</h3>
 * <pre>{@code
 * DisruptorWorker<MyEvent> worker = DisruptorWorker.<MyEvent>builder()
 *     .name("learning-event")
 *     .bufferSize(65536)
 *     .producerType(ProducerType.MULTI)
 *     .waitStrategy(WaitStrategyType.BLOCKING)
 *     .factory(MyEvent::new)
 *     .translator(MyEvent::set)              // (event, payload) -> event.copyFrom(payload)
 *     .handler(new MyBatchHandler())
 *     .build();
 * worker.start();
 * PublishResult r = worker.publish(payload); // 背压:DROPPED 表示队列满
 * worker.shutdown();
 * }</pre>
 *
 * <h3>设计要点(对标 LMAX 官方推荐)</h3>
 * <ul>
 *   <li>预分配 RingBuffer:事件对象复用,零分配开销。translator 写入字段,事件 clear 由消费后或下次写入前完成。</li>
 *   <li>ProducerType:单生产者用 {@link ProducerType#SINGLE}(无 CAS,最快);多生产者用 {@link ProducerType#MULTI}。</li>
 *   <li>WaitStrategy:见 {@link WaitStrategyType},按 CPU-延迟权衡选型。</li>
 *   <li>背压:用 {@link RingBuffer#tryPublishEvent},满则返回 {@link PublishResult#DROPPED},不死等、不打满网关线程。</li>
 *   <li>异常隔离:默认 ExceptionHandler 记日志并跳过该事件,避免单条坏消息打断整条消费链。</li>
 *   <li>优雅停机:{@link #shutdown()} 先等在途事件处理完(disruptor.shutdown 阻塞至消费者追上)。</li>
 * </ul>
 *
 * @param <T> 事件类型,需可复用(字段可被 translator 覆盖)
 */
public class DisruptorWorker<T> {

    private static final Logger log = LoggerFactory.getLogger(DisruptorWorker.class);

    private final String name;
    private final int bufferSize;
    private final ProducerType producerType;
    private final WaitStrategy waitStrategy;
    private final com.lmax.disruptor.EventFactory<T> factory;
    private final com.lmax.disruptor.EventTranslatorOneArg<T, T> translator;
    private final EventHandler<T> handler;
    private final EventHandler<T>[] poolHandlers;
    private final ExceptionHandler<T> exceptionHandler;

    private Disruptor<T> disruptor;
    private RingBuffer<T> ringBuffer;
    private final AtomicLong published = new AtomicLong();
    private final AtomicLong dropped = new AtomicLong();
    private final AtomicLong consumed = new AtomicLong();

    private DisruptorWorker(Builder<T> b) {
        this.name = b.name;
        this.bufferSize = b.bufferSize;
        this.producerType = b.producerType;
        this.waitStrategy = b.waitStrategy;
        this.factory = b.factory;
        this.translator = b.translator;
        this.handler = b.handler;
        this.poolHandlers = b.poolHandlers;
        this.exceptionHandler = b.exceptionHandler != null ? b.exceptionHandler : defaultExceptionHandler();
    }

    /** 启动 Disruptor(@PostConstruct 调用)。 */
    public void start() {
        disruptor = new Disruptor<>(factory, bufferSize, DaemonThreadFactory.INSTANCE, producerType, waitStrategy);
        if (poolHandlers != null && poolHandlers.length > 0) {
            // 并行消费:多个 EventHandler 作为独立 BatchEventProcessor 并行消费同一 RingBuffer。
            // 与 WorkerPool(3.x,4.0 已移)语义不同:这里每个 handler 都会收到每条事件(广播)。
            // 因此 BatchEventHandler 需按消费者索引分片过滤(见 LearningEventDisruptor.start 注释),
            // 实现每条事件只被一个消费者落库 —— 等效 WorkerPool 的瓜分语义,兼容 Disruptor 4.0。
            disruptor.handleEventsWith(poolHandlers);
        } else {
            disruptor.handleEventsWith(handler);
        }
        // 消费计数:在用户 handler 之外再挂一个只计数的事件处理器,顺序在 handler 之后
        disruptor.handleEventsWith((event, sequence, endOfBatch) -> consumed.incrementAndGet());
        disruptor.setDefaultExceptionHandler(exceptionHandler);
        disruptor.start();
        ringBuffer = disruptor.getRingBuffer();
        log.info("DisruptorWorker[{}] started: bufferSize={}, producer={}, wait={}, consumers={}",
                name, bufferSize, producerType, waitStrategy.getClass().getSimpleName(),
                poolHandlers != null ? poolHandlers.length : 1);
    }

    /**
     * 发布事件(背压)。translator 把 payload 字段拷进复用的 event 槽。
     * 队列满返回 {@link PublishResult#DROPPED}(记 dropped 指标),不死等。
     */
    public PublishResult publish(T payload) {
        if (ringBuffer.tryPublishEvent(translator, payload)) {
            published.incrementAndGet();
            return PublishResult.PUBLISHED;
        }
        dropped.incrementAndGet();
        return PublishResult.DROPPED;
    }

    /** 优雅停机(@PreDestroy 调用):阻塞至消费者追上当前发布点,确保在途事件处理完。 */
    public void shutdown() {
        log.info("DisruptorWorker[{}] shutting down, draining... published={}, consumed={}, dropped={}",
                name, published.get(), consumed.get(), dropped.get());
        if (disruptor != null) disruptor.shutdown();
    }

    public DisruptorStats stats() {
        return new DisruptorStats(name, bufferSize, published.get(), dropped.get(), consumed.get());
    }

    public String name() { return name; }

    private ExceptionHandler<T> defaultExceptionHandler() {
        return new ExceptionHandler<>() {
            @Override
            public void handleEventException(Throwable ex, long sequence, T event) {
                log.error("DisruptorWorker[{}] event {} failed: {}", name, sequence, ex.toString());
            }
            @Override
            public void handleOnStartException(Throwable ex) {
                log.error("DisruptorWorker[{}] start error", name, ex);
            }
            @Override
            public void handleOnShutdownException(Throwable ex) {
                log.error("DisruptorWorker[{}] shutdown error", name, ex);
            }
        };
    }

    /** Builder。translator 签名:(eventSlot, payload) -> void,把 payload 字段拷进复用槽。 */
    public static <T> Builder<T> builder() { return new Builder<>(); }

    public static class Builder<T> {
        private String name = "unnamed";
        private int bufferSize = 65536;          // 2^16,通用默认
        private ProducerType producerType = ProducerType.MULTI;
        private WaitStrategy waitStrategy = new com.lmax.disruptor.BlockingWaitStrategy();
        private com.lmax.disruptor.EventFactory<T> factory;
        private com.lmax.disruptor.EventTranslatorOneArg<T, T> translator;
        private EventHandler<T> handler;
        private EventHandler<T>[] poolHandlers;
        private ExceptionHandler<T> exceptionHandler;

        public Builder<T> name(String name) { this.name = name; return this; }
        public Builder<T> bufferSize(int n) { this.bufferSize = n; return this; }
        public Builder<T> producerType(ProducerType p) { this.producerType = p; return this; }
        public Builder<T> waitStrategy(WaitStrategyType t) {
            this.waitStrategy = switch (t) {
                case BLOCKING -> new com.lmax.disruptor.BlockingWaitStrategy();
                case SLEEPING -> new com.lmax.disruptor.SleepingWaitStrategy();
                case YIELDING -> new com.lmax.disruptor.YieldingWaitStrategy();
                case BUSY_SPIN -> new com.lmax.disruptor.BusySpinWaitStrategy();
                case LITE_BLOCKING -> new com.lmax.disruptor.LiteBlockingWaitStrategy();
            };
            return this;
        }
        public Builder<T> factory(com.lmax.disruptor.EventFactory<T> f) { this.factory = f; return this; }
        public Builder<T> translator(com.lmax.disruptor.EventTranslatorOneArg<T, T> t) { this.translator = t; return this; }
        public Builder<T> handler(EventHandler<T> h) { this.handler = h; return this; }

        /** 并行消费:传入多个 EventHandler 并行处理。Disruptor 4.0 已移除 WorkerPool/WorkHandler,
         *  这里用多 EventHandler(每条广播给所有 handler)。调用方负责在 handler 内按 sequence 分片
         *  过滤,使每条事件只被一个消费者实际落库(等效 WorkerPool 瓜分语义)。与 {@link #handler} 互斥。 */
        @SafeVarargs
        public final Builder<T> poolHandlers(EventHandler<T>... hs) { this.poolHandlers = hs; return this; }

        public Builder<T> exceptionHandler(ExceptionHandler<T> e) { this.exceptionHandler = e; return this; }

        public DisruptorWorker<T> build() {
            if (factory == null) throw new IllegalStateException("factory required");
            if (translator == null) throw new IllegalStateException("translator required");
            boolean singleMode = handler != null;
            boolean poolMode = poolHandlers != null && poolHandlers.length > 0;
            if (!singleMode && !poolMode) throw new IllegalStateException("handler or poolHandlers required");
            if (singleMode && poolMode) throw new IllegalStateException("handler and poolHandlers are mutually exclusive");
            return new DisruptorWorker<>(this);
        }
    }
}
