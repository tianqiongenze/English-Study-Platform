package com.esp.cache;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.Mockito;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.cache.Cache;
import org.springframework.cache.CacheManager;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.core.ValueOperations;

import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class CacheServiceTest {

    private CacheManager cacheManager;
    private Cache l1;
    private RedisTemplate<String, Object> redisTemplate;
    @SuppressWarnings("unchecked")
    private final ValueOperations<String, Object> valueOps = mock(ValueOperations.class);

    @SuppressWarnings("unchecked")
    private CacheService newService(CacheManager cm, RedisTemplate<String, Object> rt, CacheBroadcast bc) {
        ObjectProvider<CacheManager> cmp = mock(ObjectProvider.class);
        when(cmp.getIfAvailable()).thenReturn(cm);
        ObjectProvider<RedisTemplate<String, Object>> rtp = mock(ObjectProvider.class);
        when(rtp.getIfAvailable()).thenReturn(rt);
        ObjectProvider<CacheBroadcast> bcp = mock(ObjectProvider.class);
        when(bcp.getIfAvailable()).thenReturn(bc);
        CacheService svc = new CacheService(cmp, rtp, bcp);
        // 反射注入 @Value 默认值
        setField(svc, "l2TtlSeconds", 300L);
        setField(svc, "emptyTtlSeconds", 30L);
        setField(svc, "doubleDeleteDelayMs", 0L);  // 关闭延迟双删,简化测试
        return svc;
    }

    private static void setField(Object target, String field, long value) {
        try {
            var f = CacheService.class.getDeclaredField(field);
            f.setAccessible(true);
            f.setLong(target, value);
        } catch (Exception e) { throw new RuntimeException(e); }
    }

    @BeforeEach
    @SuppressWarnings("unchecked")
    void setup() {
        cacheManager = mock(CacheManager.class);
        l1 = mock(Cache.class);
        redisTemplate = mock(RedisTemplate.class);
        when(cacheManager.getCache("c")).thenReturn(l1);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);
    }

    @Test
    void degradeWhenCacheManagerNull() {
        CacheService svc = newService(null, redisTemplate, null);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "v"; }, 0, false);
        assertEquals("v", result);
        assertEquals(1, calls.get());
        assertFalse(svc.isEnabled());
        // put/evict no-op
        svc.put("c", "k", "v", 0);
        svc.evict("c", "k");
        verifyNoInteractions(redisTemplate);
    }

    @Test
    void degradeWhenRedisTemplateNull() {
        CacheService svc = newService(cacheManager, null, null);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "v"; }, 0, false);
        assertEquals("v", result);
        assertEquals(1, calls.get());
    }

    @Test
    void l1HitReturnsValueWithoutLoaderOrL2() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        Cache.ValueWrapper vw = mock(Cache.ValueWrapper.class);
        when(vw.get()).thenReturn("l1val");
        when(l1.get("k")).thenReturn(vw);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "should-not-call"; }, 0, false);
        assertEquals("l1val", result);
        assertEquals(0, calls.get(), "loader 不应被调用");
        verify(valueOps, never()).get(anyString());
    }

    @Test
    void l1NullHitReturnsNullWithoutLoader() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        // L1 中存的是 null wrapper(防穿透回填的 null)
        Cache.ValueWrapper vw = mock(Cache.ValueWrapper.class);
        when(vw.get()).thenReturn(null);
        when(l1.get("k")).thenReturn(vw);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "x"; }, 0, false);
        assertNull(result, "L1 null wrapper 应返回 null");
        assertEquals(0, calls.get());
    }

    @Test
    void l2HitReturnsValueAndBackfillsL1() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("k")).thenReturn(null);  // L1 miss
        when(valueOps.get("k")).thenReturn("l2val");
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "should-not-call"; }, 0, false);
        assertEquals("l2val", result);
        assertEquals(0, calls.get(), "loader 不应被调用");
        verify(l1).put("k", "l2val");
        verify(valueOps, never()).set(anyString(), any(), anyLong(), any());
    }

    @Test
    void l2EmptySentinelReturnsNullAndBackfillsL1() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("k")).thenReturn(null);
        when(valueOps.get("k")).thenReturn("__ESP_CACHE_EMPTY__");
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "x"; }, 0, false);
        assertNull(result, "空 sentinel 应返 null");
        assertEquals(0, calls.get(), "loader 不应被调用");
        verify(l1).put("k", null);
    }

    @Test
    void dbMissFillsL2AndL1() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("k")).thenReturn(null);
        when(valueOps.get("k")).thenReturn(null);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "dbval"; }, 60, false);
        assertEquals("dbval", result);
        assertEquals(1, calls.get());
        verify(valueOps).set(eq("k"), eq("dbval"), eq(60L), eq(TimeUnit.SECONDS));
        verify(l1).put("k", "dbval");
    }

    @Test
    void dbNullWithCacheEmptyStoresSentinel() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("k")).thenReturn(null);
        when(valueOps.get("k")).thenReturn(null);
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return null; }, 0, true);
        assertNull(result);
        verify(valueOps).set(eq("k"), eq("__ESP_CACHE_EMPTY__"), eq(30L), eq(TimeUnit.SECONDS));
        verify(l1).put("k", null);
    }

    @Test
    void l2ReadExceptionDegradesToDb() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("k")).thenReturn(null);
        when(valueOps.get("k")).thenThrow(new RuntimeException("redis down"));
        AtomicInteger calls = new AtomicInteger();
        String result = svc.getOrLoad("c", "k", () -> { calls.incrementAndGet(); return "dbval"; }, 0, false);
        assertEquals("dbval", result);
        assertEquals(1, calls.get(), "L2 异常应降级到 loader");
    }

    @Test
    void evictDeletesL2L1AndBroadcasts() throws Exception {
        CacheBroadcast bc = mock(CacheBroadcast.class);
        CacheService svc = newService(cacheManager, redisTemplate, bc);
        svc.evict("c", "k");
        verify(redisTemplate).delete("k");
        verify(l1).evict("k");
        verify(bc, atLeastOnce()).publishEvict("c", "k");
    }

    @Test
    void putWritesL2AndL1() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        svc.put("c", "k", "v", 120);
        verify(valueOps).set(eq("k"), eq("v"), eq(120L), eq(TimeUnit.SECONDS));
        verify(l1).put("k", "v");
    }

    @Test
    void putNullStoresSentinel() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        svc.put("c", "k", null, 0);
        verify(valueOps).set(eq("k"), eq("__ESP_CACHE_EMPTY__"), eq(30L), eq(TimeUnit.SECONDS));
        verify(l1).put("k", null);
    }

    @Test
    void isEnabledTrueWhenCacheManagerPresent() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        assertTrue(svc.isEnabled());
    }
}
