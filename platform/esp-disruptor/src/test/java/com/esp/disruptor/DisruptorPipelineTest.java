package com.esp.disruptor;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

import static org.junit.jupiter.api.Assertions.*;

class StringSlot {
    String payload;
    static com.lmax.disruptor.EventFactory<StringSlot> factory() { return StringSlot::new; }
    static com.lmax.disruptor.EventTranslatorOneArg<StringSlot, StringSlot> translator() {
        return (slot, sequence, src) -> slot.payload = src.payload;
    }
    static StringSlot of(String s) { StringSlot h = new StringSlot(); h.payload = s; return h; }
}

class DisruptorPipelineTest {

    @Test
    void emptyStagesBuildThrows() {
        assertThrows(IllegalStateException.class, () -> DisruptorPipeline.named("empty").build());
    }

    @Test
    void twoStagePipelineFlowsThrough() throws Exception {
        List<String> stage0Seen = new CopyOnWriteArrayList<>();
        List<String> stage1Seen = new CopyOnWriteArrayList<>();
        // 用数组持有 pipeline 引用,handler 闭包内用 forward
        DisruptorPipeline[] ref = new DisruptorPipeline[1];

        DisruptorPipeline pipe = DisruptorPipeline.named("two-stage")
                .stage("s0", 256,
                        com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(),
                        StringSlot.translator(),
                        (event, sequence, endOfBatch) -> {
                            stage0Seen.add(event.payload);
                            // forward 派生事件到下一级
                            ref[0].forward(0, StringSlot.of("stage1:" + event.payload));
                        },
                        WaitStrategyType.BLOCKING)
                .stage("s1", 256,
                        com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(),
                        StringSlot.translator(),
                        (event, sequence, endOfBatch) -> stage1Seen.add(event.payload),
                        WaitStrategyType.BLOCKING)
                .build();
        ref[0] = pipe;

        pipe.start();
        pipe.publish(StringSlot.of("hello"));
        // stage0 同步消费后 forward 到 stage1,但 stage1 消费异步;shutdown stage0 后给 stage1 短暂 drain。
        // 先等 stage0 收到(它消费后才会 forward),再 shutdown(逆序:先停 stage1 再停 stage0——
        // 但 stage0 forward 依赖 stage1 仍存活,故这里等 stage1 收到后再 shutdown)。
        long deadline = System.nanoTime() + 2_000_000_000L;
        while (!stage0Seen.contains("hello") && System.nanoTime() < deadline) Thread.sleep(5);
        long deadline1 = System.nanoTime() + 2_000_000_000L;
        while (!stage1Seen.contains("stage1:hello") && System.nanoTime() < deadline1) Thread.sleep(5);
        pipe.shutdown();

        assertTrue(stage0Seen.contains("hello"), "stage0=" + stage0Seen);
        assertTrue(stage1Seen.contains("stage1:hello"), "stage1=" + stage1Seen);
        assertEquals(2, pipe.stageCount());
        assertEquals(2, pipe.stats().size());
        assertEquals("two-stage", pipe.name());
    }

    @Test
    void shutdownReverseOrderDoesNotThrow() throws Exception {
        DisruptorPipeline pipe = DisruptorPipeline.named("rev")
                .stage("a", 128, com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(), StringSlot.translator(),
                        (e, s, b) -> {}, WaitStrategyType.BLOCKING)
                .stage("b", 128, com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(), StringSlot.translator(),
                        (e, s, b) -> {}, WaitStrategyType.BLOCKING)
                .build();
        pipe.start();
        assertDoesNotThrow(pipe::shutdown);
    }

    @Test
    void statsByIndexReturnsCorrectStage() {
        DisruptorPipeline pipe = DisruptorPipeline.named("idx")
                .stage("a", 128, com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(), StringSlot.translator(),
                        (e, s, b) -> {}, WaitStrategyType.BLOCKING)
                .stage("b", 128, com.lmax.disruptor.dsl.ProducerType.SINGLE,
                        StringSlot.factory(), StringSlot.translator(),
                        (e, s, b) -> {}, WaitStrategyType.BLOCKING)
                .build();
        pipe.start();
        assertEquals("idx:a", pipe.stats(0).name());
        assertEquals("idx:b", pipe.stats(1).name());
        pipe.shutdown();
    }
}
