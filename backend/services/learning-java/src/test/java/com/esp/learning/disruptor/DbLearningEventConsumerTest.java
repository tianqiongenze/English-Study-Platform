package com.esp.learning.disruptor;

import com.esp.cache.CacheService;
import org.junit.jupiter.api.Test;
import org.springframework.dao.DataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

import org.mockito.ArgumentCaptor;

/**
 * {@link DbLearningEventConsumer} 单测:G-06 修复 —— batch 落库失败不再仅 log 丢数据,
 * 而是整批改写 learning_event_outbox(status='failed')由 OutboxRelay 重试。
 */
class DbLearningEventConsumerTest {

    @Test
    void batchFailure_routesAllEventsToOutbox() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        // 主写入 learning_events 抛 DB 异常;outbox 写入成功。
        doAnswer(inv -> {
            String sql = inv.getArgument(0);
            if (sql.contains("learning_events ")) {
                throw new DataAccessException("db down") {};
            }
            return new int[0];
        }).when(jdbc).batchUpdate(anyString(), anyList());

        CacheService cache = mock(CacheService.class);
        DbLearningEventConsumer consumer = new DbLearningEventConsumer(jdbc, cache);

        LearningEvent[] events = events(2);
        consumer.consume(events, 0, 2);

        // 主路径应被调用过且失败;缓存不应被失效(未落库成功)。
        verify(jdbc).batchUpdate(eq(DbLearningEventConsumer.INSERT_SQL), anyList());
        verify(cache, never()).evict(anyString(), anyString());
        assertEquals(0, consumer.consumedCount(), "no events counted as consumed");

        // outbox 应收到整批(2 行),status='failed' 由 SQL 常量保证,参数为 (user_id, event_type, payload)。
        @SuppressWarnings("unchecked")
        ArgumentCaptor<List<Object[]>> captor = ArgumentCaptor.forClass(List.class);
        verify(jdbc).batchUpdate(eq(DbLearningEventConsumer.OUTBOX_SQL), captor.capture());
        List<Object[]> outboxArgs = captor.getValue();
        assertNotNull(outboxArgs, "outbox batch args should be captured");
        assertEquals(2, outboxArgs.size(), "all failed events should route to outbox");
        // OUTBOX_SQL 参数顺序: (message_id, user_id, event_type, payload)
        assertEquals("msg-1", outboxArgs.get(0)[0], "message_id");
        assertEquals(1L, outboxArgs.get(0)[1], "user_id");
        assertEquals("answer", outboxArgs.get(0)[2], "event_type");
        assertEquals("{\"u\":1}", outboxArgs.get(0)[3], "payload");
        assertEquals("msg-2", outboxArgs.get(1)[0], "第二个事件的 message_id");
        assertEquals(2L, outboxArgs.get(1)[1], "第二个事件的 user_id");
        assertEquals(2, consumer.outboxRoutedCount(), "outbox routed counter should equal batch size");
    }

    @Test
    void batchSuccess_consumesAndEvictsCache() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        when(jdbc.batchUpdate(eq(DbLearningEventConsumer.INSERT_SQL), anyList())).thenReturn(new int[]{1, 1});
        CacheService cache = mock(CacheService.class);
        DbLearningEventConsumer consumer = new DbLearningEventConsumer(jdbc, cache);

        consumer.consume(events(2), 0, 2);

        assertEquals(2, consumer.consumedCount());
        verify(cache).evict("learning:agg", "learning:agg:users");
        verify(jdbc, never()).batchUpdate(eq(DbLearningEventConsumer.OUTBOX_SQL), anyList());
        assertEquals(0, consumer.outboxRoutedCount());
    }

    @Test
    void outboxAlsoFails_doesNotThrow() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        // 主写入与 outbox 写入都抛异常(DB 双障)→ consume 必须不抛,只记日志。
        doAnswer(inv -> { throw new DataAccessException("db fully down") {}; })
                .when(jdbc).batchUpdate(anyString(), anyList());
        CacheService cache = mock(CacheService.class);
        DbLearningEventConsumer consumer = new DbLearningEventConsumer(jdbc, cache);

        assertDoesNotThrow(() -> consumer.consume(events(2), 0, 2));
        assertEquals(0, consumer.consumedCount());
        assertEquals(0, consumer.outboxRoutedCount(), "outbox fallback also failed → no routed count");
    }

    private static LearningEvent[] events(int n) {
        List<LearningEvent> list = new ArrayList<>(n);
        for (long i = 1; i <= n; i++) {
            LearningEvent e = new LearningEvent();
            e.setUserId(i);
            e.setEventType("answer");
            e.setPayload("{\"u\":" + i + "}");
            e.setCreatedAt(i * 1000L);
            e.setMessageId("msg-" + i);
            list.add(e);
        }
        return list.toArray(new LearningEvent[0]);
    }
}
