package com.esp.learning.disruptor;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.*;

/**
 * {@link EventBatchQueue} 单测:独立 DB writer 线程池的
 * 深拷贝正确性 / 队列满 DROP / 优雅排空。
 */
class EventBatchQueueTest {

    @Test
    void offer_deepCopiesBatch() throws Exception {
        List<LearningEvent> original = new ArrayList<>();
        LearningEvent e = new LearningEvent();
        e.setUserId(1);
        e.setEventType("answer");
        e.setPayload("{\"ok\":true}");
        e.setCreatedAt(100L);
        original.add(e);

        CountDownLatch got = new CountDownLatch(1);
        AtomicReference<LearningEvent> received = new AtomicReference<>();
        EventBatchQueue q = new EventBatchQueue((events, offset, length) -> {
            received.set(events[offset]);
            got.countDown();
        }, 10, 1);
        q.start();
        try {
            assertTrue(q.offer(original), "offer should accept when queue not full");

            // 入队后立刻变异原始对象:writer 收到的必须是独立副本,不受影响。
            e.setUserId(999);
            e.setEventType("speak");
            e.setPayload("mutated");
            e.setCreatedAt(999L);

            assertTrue(got.await(2, TimeUnit.SECONDS), "writer should consume within 2s");
            assertNotNull(received.get(), "consumer should receive a batch");
            assertEquals(1, received.get().getUserId(), "userId must be deep-copied value");
            assertEquals("answer", received.get().getEventType(), "eventType must be deep-copied value");
            assertEquals("{\"ok\":true}", received.get().getPayload(), "payload must be deep-copied value");
            assertEquals(100L, received.get().getCreatedAt(), "createdAt must be deep-copied value");
        } finally {
            q.shutdown();
        }
    }

    @Test
    void offer_dropsWhenQueueFull_withoutBlocking() {
        // 不 start() writer 线程:队列空转,容量 1 → 第二批必 DROP,确定性无竞态。
        AtomicInteger consumed = new AtomicInteger();
        EventBatchQueue q = new EventBatchQueue((events, offset, length) -> consumed.addAndGet(length), 1, 1);
        assertTrue(q.offer(single(1, "answer", "{}")), "first batch should be accepted (queue now full)");
        assertFalse(q.offer(single(2, "answer", "{}")), "second batch should DROP when queue full");
        assertEquals(1, q.droppedCount(), "dropped counter should increment");
        assertEquals(2, q.offeredCount(), "offered counter counts both attempts");
        assertEquals(1, q.queueSize(), "queue holds first batch");
        assertEquals(0, consumed.get(), "no writer started → nothing consumed");
        // 未启动的队列 shutdown 应安全 no-op。
        q.shutdown();
    }

    @Test
    void writerConsumesBatches_thenShutdownDrains() throws Exception {
        AtomicInteger consumed = new AtomicInteger();
        EventBatchQueue q = new EventBatchQueue((events, offset, length) -> consumed.addAndGet(length), 10, 1);
        q.start();
        try {
            assertTrue(q.offer(single(1, "answer", "{}")));
            assertTrue(q.offer(single(2, "answer", "{}")));
            // 等 writer 消费完(队列排空)
            long deadline = System.currentTimeMillis() + 3000;
            while (consumed.get() < 2 && System.currentTimeMillis() < deadline) {
                Thread.sleep(20);
            }
            assertEquals(2, consumed.get(), "writer should consume both batches");
            assertEquals(0, q.queueSize(), "queue should be drained");
            assertEquals(0, q.droppedCount(), "no drop expected");
        } finally {
            q.shutdown();
        }
    }

    private static List<LearningEvent> single(long userId, String type, String payload) {
        LearningEvent e = new LearningEvent();
        e.setUserId(userId);
        e.setEventType(type);
        e.setPayload(payload);
        e.setCreatedAt(System.currentTimeMillis());
        List<LearningEvent> l = new ArrayList<>(1);
        l.add(e);
        return l;
    }
}
