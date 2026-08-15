package com.esp.learning.disruptor;

import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.mock;

class LearningEventDisruptorTest {

    @Test
    void publishAndConsumeBatched() throws Exception {
        AtomicInteger total = new AtomicInteger();
        List<LearningEvent> sink = new ArrayList<>();
        LearningEventConsumer consumer = (events, offset, length) -> {
            for (int i = 0; i < length; i++) {
                sink.add(events[offset + i]);
                total.incrementAndGet();
            }
        };

        // 用小 buffer 与 batch 加速 flush 触发。writeJdbc 传 mock(outbox 兜底路径不被命中)。
        JdbcTemplate mockJdbc = mock(JdbcTemplate.class);
        LearningEventDisruptor d = new LearningEventDisruptor(1024, 50, 100, 1, consumer, mockJdbc);
        d.start();
        try {
            int n = 500;
            for (int i = 0; i < n; i++) {
                assertTrue(d.publish(i, "answer", "{\"ok\":true}"));
            }
            // 等待消费者 flush(batch + 超时)
            long deadline = System.currentTimeMillis() + 5000;
            while (total.get() < n && System.currentTimeMillis() < deadline) {
                Thread.sleep(50);
            }
            assertEquals(n, total.get(), "all events should be consumed");
            assertEquals(n, d.publishedCount());
            assertEquals(0, d.droppedCount());
        } finally {
            d.shutdown();
        }
    }

    @Test
    void highThroughput_meetsTarget() throws Exception {
        AtomicInteger total = new AtomicInteger();
        LearningEventConsumer consumer = (events, offset, length) -> total.addAndGet(length);

        JdbcTemplate mockJdbc = mock(JdbcTemplate.class);
        LearningEventDisruptor d = new LearningEventDisruptor(65536, 500, 200, 4, consumer, mockJdbc);
        d.start();
        try {
            int threads = 8;
            int perThread = 5000; // 共 40000
            ExecutorService pool = Executors.newFixedThreadPool(threads);
            CountDownLatch start = new CountDownLatch(1);
            CountDownLatch done = new CountDownLatch(threads);
            for (int t = 0; t < threads; t++) {
                final int base = t * perThread;
                pool.submit(() -> {
                    try {
                        start.await();
                        for (int i = 0; i < perThread; i++) {
                            d.publish(base + i, "answer", "{}");
                        }
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                    } finally {
                        done.countDown();
                    }
                });
            }
            long t0 = System.nanoTime();
            start.countDown();
            boolean finished = done.await(30, TimeUnit.SECONDS);
            long publishMs = (System.nanoTime() - t0) / 1_000_000;
            assertTrue(finished, "publish should finish quickly");

            // 发布吞吐
            long totalEvents = (long) threads * perThread;
            double publishThroughput = totalEvents * 1000.0 / publishMs;
            assertTrue(publishThroughput >= 5000,
                    "publish throughput should >= 5000 ev/s, got " + publishThroughput);

            // 等待消费完成
            long deadline = System.currentTimeMillis() + 15000;
            while (total.get() < totalEvents && System.currentTimeMillis() < deadline) {
                Thread.sleep(100);
            }
            assertEquals(totalEvents, total.get(), "all consumed");
        } finally {
            d.shutdown();
        }
    }
}
