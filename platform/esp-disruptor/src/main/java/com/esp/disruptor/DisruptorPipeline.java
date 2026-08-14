package com.esp.disruptor;

import com.lmax.disruptor.EventHandler;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;

/**
 * 多级 Disruptor 流水线(ADR-09)。
 *
 * <p>把多个 {@link DisruptorWorker} 串联成流水线:上一级的消费者在处理完事件后,
 * 把(派生的)事件投递到下一级。每级独立 RingBuffer + 独立消费线程,级间解耦、
 * 各自背压,扛住不同阶段的速度差异(经典 LMAX 流水线模式)。
 *
 * <h3>典型场景:BFF 边缘 → 业务处理 → 落库/转发</h3>
 * <pre>{@code
 * DisruptorPipeline pipe = DisruptorPipeline.named("bff-flow")
 *     .stage("ingest",  InletEvent::new,  InletEvent::copyFrom, new IngestHandler(),  WaitStrategyType.YIELDING)
 *     .stage("process", ProcEvent::new,   ProcEvent::copyFrom,  new ProcessHandler(), WaitStrategyType.BLOCKING)
 *     .stage("sink",    SinkEvent::new,   SinkEvent::copyFrom,  new SinkHandler(),    WaitStrategyType.BLOCKING)
 *     .build();
 * pipe.start();
 * pipe.publish(stage0Payload);
 * pipe.shutdown();
 * }</pre>
 *
 * <h3>设计要点</h3>
 * <ul>
 *   <li>每级独立 bufferSize/producerType/waitStrategy,按该级负载特征选型(接入级低延迟,落库级吞吐)。</li>
 *   <li>级间衔接:用户在 stage 的 handler 内调用 {@code next.publish(...)} 即可;
 *       本类提供 {@link #forward(int, Object)} 便捷方法把事件推到指定下一级。</li>
 *   <li>整条流水线共享一个名称前缀,便于指标归类。</li>
 *   <li>停机按级逆序 shutdown,保证上游不再投递后下游能 drain 完。</li>
 * </ul>
 */
public class DisruptorPipeline {

    private static final Logger log = LoggerFactory.getLogger(DisruptorPipeline.class);

    private final String name;
    private final List<DisruptorWorker<?>> stages;

    private DisruptorPipeline(String name, List<DisruptorWorker<?>> stages) {
        this.name = name;
        this.stages = stages;
    }

    /** 启动所有级(顺序)。 */
    public void start() {
        for (DisruptorWorker<?> w : stages) w.start();
        log.info("DisruptorPipeline[{}] started, stages={}", name, stages.size());
    }

    /** 向第 0 级投递事件。 */
    public <T> PublishResult publish(T payload) {
        return publish(0, payload);
    }

    /** 向指定级投递事件(供级间衔接使用)。 */
    @SuppressWarnings({"unchecked", "rawtypes"})
    public <T> PublishResult publish(int stageIndex, T payload) {
        return ((DisruptorWorker<T>) stages.get(stageIndex)).publish(payload);
    }

    /** 便捷:在某级 handler 内把派生事件转发到下一级。 */
    public <T> PublishResult forward(int fromStage, T payload) {
        return publish(fromStage + 1, payload);
    }

    /** 逆序停机:先停上游,再 drain 下游。 */
    public void shutdown() {
        log.info("DisruptorPipeline[{}] shutting down (reverse order)", name);
        for (int i = stages.size() - 1; i >= 0; i--) stages.get(i).shutdown();
    }

    public DisruptorStats stats(int stageIndex) {
        return stages.get(stageIndex).stats();
    }

    public List<DisruptorStats> stats() {
        List<DisruptorStats> all = new ArrayList<>(stages.size());
        for (DisruptorWorker<?> w : stages) all.add(w.stats());
        return all;
    }

    public String name() { return name; }
    public int stageCount() { return stages.size(); }

    public static Builder named(String name) { return new Builder(name); }

    /** 流水线 Builder:按 stage 顺序追加。 */
    public static class Builder {
        private final String name;
        private final List<DisruptorWorker<?>> stages = new ArrayList<>();

        Builder(String name) { this.name = name; }

        /**
         * 追加一级。默认 ProducerType.MULTI、bufferSize=65536。
         * @param stageName  级名(指标用)
         * @param factory    事件工厂(预分配槽)
         * @param translator (slot, payload) -> void,字段拷贝
         * @param handler    该级消费者;如需衔接下一级,在 handler 内调用 pipeline.forward(i, ...)
         * @param wait       该级等待策略
         */
        public <T> Builder stage(String stageName,
                                 com.lmax.disruptor.EventFactory<T> factory,
                                 com.lmax.disruptor.EventTranslatorOneArg<T, T> translator,
                                 EventHandler<T> handler,
                                 WaitStrategyType wait) {
            return stage(stageName, 65536, com.lmax.disruptor.dsl.ProducerType.MULTI, factory, translator, handler, wait);
        }

        public <T> Builder stage(String stageName, int bufferSize,
                                 com.lmax.disruptor.dsl.ProducerType producerType,
                                 com.lmax.disruptor.EventFactory<T> factory,
                                 com.lmax.disruptor.EventTranslatorOneArg<T, T> translator,
                                 EventHandler<T> handler,
                                 WaitStrategyType wait) {
            DisruptorWorker<T> w = DisruptorWorker.<T>builder()
                    .name(name + ":" + stageName)
                    .bufferSize(bufferSize)
                    .producerType(producerType)
                    .waitStrategy(wait)
                    .factory(factory)
                    .translator(translator)
                    .handler(handler)
                    .build();
            stages.add(w);
            return this;
        }

        public DisruptorPipeline build() {
            if (stages.isEmpty()) throw new IllegalStateException("pipeline needs at least one stage");
            return new DisruptorPipeline(name, stages);
        }
    }
}
