package com.esp.disruptor;

import com.lmax.disruptor.EventHandler;
import com.lmax.disruptor.ExceptionHandler;
import com.lmax.disruptor.dsl.ProducerType;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/** 可复用事件槽:translator 把 payload 字段拷进复用槽,handler 读 payload。 */
class StringHolder {
    String payload;

    static com.lmax.disruptor.EventFactory<StringHolder> factory() { return StringHolder::new; }

    /** translator:(slot, sequence, payloadHolder) -> 拷贝 payload 字段进复用槽。 */
    static com.lmax.disruptor.EventTranslatorOneArg<StringHolder, StringHolder> translator() {
        return (slot, sequence, src) -> slot.payload = src.payload;
    }

    static StringHolder of(String s) {
        StringHolder h = new StringHolder();
        h.payload = s;
        return h;
    }
}

class DisruptorWorkerTest {

    @Test
    void buildWithoutFactoryThrows() {
        assertThrows(IllegalStateException.class, () -> DisruptorWorker.<StringHolder>builder()
                .translator(StringHolder.translator())
                .handler((e, s, b) -> {})
                .build());
    }

    @Test
    void buildWithoutTranslatorThrows() {
        assertThrows(IllegalStateException.class, () -> DisruptorWorker.<StringHolder>builder()
                .factory(StringHolder.factory())
                .handler((e, s, b) -> {})
                .build());
    }

    @Test
    void buildWithoutHandlerThrows() {
        assertThrows(IllegalStateException.class, () -> DisruptorWorker.<StringHolder>builder()
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .build());
    }

    @Test
    void endToEnd() throws Exception {
        List<String> received = new CopyOnWriteArrayList<>();
        DisruptorWorker<StringHolder> worker = DisruptorWorker.<StringHolder>builder()
                .name("e2e")
                .bufferSize(1024)
                .producerType(ProducerType.SINGLE)
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .handler((event, sequence, endOfBatch) -> received.add(event.payload))
                .build();
        worker.start();
        worker.publish(StringHolder.of("hello"));
        // 等消费完成再 shutdown:shutdown() 会阻塞至消费者追上发布点,但若消费线程调度延迟,
        // 立即 shutdown 可能出现竞态。短暂 await 保证稳定。
        long deadline = System.nanoTime() + 2_000_000_000L;
        while (received.isEmpty() && System.nanoTime() < deadline) Thread.sleep(2);
        worker.shutdown();
        assertTrue(received.contains("hello"), "received=" + received);
        DisruptorStats stats = worker.stats();
        assertEquals(1, stats.published());
        assertTrue(stats.consumed() >= 1);
    }

    @Test
    void backpressureDropsWhenFull() throws Exception {
        List<String> received = new CopyOnWriteArrayList<>();
        DisruptorWorker<StringHolder> worker = DisruptorWorker.<StringHolder>builder()
                .name("bp")
                .bufferSize(2)
                .producerType(ProducerType.SINGLE)
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .handler((event, sequence, endOfBatch) -> {
                    try { Thread.sleep(50); } catch (InterruptedException ignored) {}  // 慢消费让 ring buffer 满
                    received.add(event.payload);
                })
                .build();
        worker.start();
        boolean dropped = false;
        for (int i = 0; i < 10000; i++) {
            if (worker.publish(StringHolder.of("m" + i)) == PublishResult.DROPPED) {
                dropped = true;
                break;
            }
        }
        worker.shutdown();
        assertTrue(dropped, "should have at least one DROPPED under backpressure");
        assertTrue(worker.stats().dropped() > 0);
    }

    @org.junit.jupiter.params.ParameterizedTest
    @org.junit.jupiter.params.provider.EnumSource(WaitStrategyType.class)
    void allWaitStrategiesWork(WaitStrategyType type) throws Exception {
        List<String> received = new CopyOnWriteArrayList<>();
        DisruptorWorker<StringHolder> worker = DisruptorWorker.<StringHolder>builder()
                .name("ws-" + type)
                .bufferSize(256)
                .producerType(ProducerType.SINGLE)
                .waitStrategy(type)
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .handler((event, sequence, endOfBatch) -> received.add(event.payload))
                .build();
        worker.start();
        worker.publish(StringHolder.of("x"));
        // 等消费者处理完事件后再 shutdown(各 waitStrategy 的消费线程调度时机不同,
        // BUSY_SPIN/YIELDING 在 shutdown 前若消费线程未运行,事件可能滞留)。
        long deadline = System.nanoTime() + 2_000_000_000L;
        while (received.isEmpty() && System.nanoTime() < deadline) {
            Thread.sleep(2);
        }
        worker.shutdown();
        assertTrue(received.contains("x"), "waitStrategy=" + type + " 应消费到事件");
    }

    @Test
    void exceptionHandlerCatchesHandlerExceptionAndContinues() throws Exception {
        List<String> received = new CopyOnWriteArrayList<>();
        AtomicReference<Throwable> caught = new AtomicReference<>();
        ExceptionHandler<StringHolder> eh = new ExceptionHandler<>() {
            @Override public void handleEventException(Throwable ex, long sequence, StringHolder event) { caught.set(ex); }
            @Override public void handleOnStartException(Throwable ex) {}
            @Override public void handleOnShutdownException(Throwable ex) {}
        };
        DisruptorWorker<StringHolder> worker = DisruptorWorker.<StringHolder>builder()
                .name("ex")
                .bufferSize(256)
                .producerType(ProducerType.SINGLE)
                .waitStrategy(WaitStrategyType.BLOCKING)
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .handler((event, sequence, endOfBatch) -> {
                    if ("boom".equals(event.payload)) throw new RuntimeException("boom");
                    received.add(event.payload);
                })
                .exceptionHandler(eh)
                .build();
        worker.start();
        worker.publish(StringHolder.of("boom"));
        worker.publish(StringHolder.of("good"));
        worker.shutdown();
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5);
        while (caught.get() == null && System.nanoTime() < deadline) Thread.sleep(10);
        assertNotNull(caught.get(), "exception handler should catch the boom");
        assertTrue(received.contains("good"), "subsequent event should be consumed");
    }

    @Test
    void nameReturnsBuilderName() {
        DisruptorWorker<StringHolder> w = DisruptorWorker.<StringHolder>builder()
                .name("my-name")
                .factory(StringHolder.factory())
                .translator(StringHolder.translator())
                .handler((e, s, b) -> {})
                .build();
        assertEquals("my-name", w.name());
    }
}
