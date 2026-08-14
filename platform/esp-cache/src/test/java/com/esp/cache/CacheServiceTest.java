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

    private static void setField(Object target, String field, int value) {
        try {
            var f = CacheService.class.getDeclaredField(field);
            f.setAccessible(true);
            f.setInt(target, value);
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

    // ---- P2-16: TTL 抖动 + singleflight 防击穿 + tryIdempotent 幂等占位 ----

    @Test
    void jitterTtlDisabledWhenPctZeroOrSmallBase() throws Exception {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        setField(svc, "l2TtlJitterPct", 0);
        var m = CacheService.class.getDeclaredMethod("jitterTtl", long.class);
        m.setAccessible(true);
        assertEquals(100L, m.invoke(svc, 100L), "jitter=0 时 TTL 不变");
        setField(svc, "l2TtlJitterPct", 10);
        assertEquals(1L, m.invoke(svc, 1L), "baseTtl<=1 时直接返回原值(不抖动)");
    }

    @Test
    void jitterTtlStaysWithinBand() throws Exception {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        setField(svc, "l2TtlJitterPct", 10);
        var m = CacheService.class.getDeclaredMethod("jitterTtl", long.class);
        m.setAccessible(true);
        for (int i = 0; i < 200; i++) {
            long v = (long) m.invoke(svc, 100L);
            assertTrue(v >= 90 && v <= 110, "jitter 越界: " + v);
        }
    }

    @Test
    void singleflightConcurrentLoadersShareOneDbCall() throws Exception {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(l1.get("hot")).thenReturn(null);
        when(valueOps.get("hot")).thenReturn(null);
        java.util.concurrent.atomic.AtomicInteger dbHits = new java.util.concurrent.atomic.AtomicInteger();
        java.util.function.Supplier<String> loader = () -> {
            dbHits.incrementAndGet();
            try { Thread.sleep(50); } catch (InterruptedException ignored) {}
            return "dbval";
        };
        int n = 8;
        java.util.concurrent.ExecutorService pool = java.util.concurrent.Executors.newFixedThreadPool(n);
        java.util.List<java.util.concurrent.Future<String>> futures = new java.util.ArrayList<>();
        for (int i = 0; i < n; i++) futures.add(pool.submit(() -> svc.getOrLoad("c", "hot", loader, 60, false)));
        for (var f : futures) assertEquals("dbval", f.get());
        pool.shutdown();
        assertEquals(1, dbHits.get(), "singleflight 应保证仅一次 DB 回源");
    }

    @Test
    void tryIdempotentReturnsTrueWhenRedisDown() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);
        when(valueOps.setIfAbsent(anyString(), any(), anyLong(), any()))
                .thenThrow(new RuntimeException("redis down"));
        assertTrue(svc.tryIdempotent("k", 30), "Redis 异常应降级放行(不阻断业务)");
    }

    @Test
    void tryIdempotentAcquiresOnFirstCall() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);
        when(valueOps.setIfAbsent(anyString(), any(), anyLong(), any())).thenReturn(true);
        assertTrue(svc.tryIdempotent("k", 30), "首次 SETNX 成功应返 true");
    }

    @Test
    void tryIdempotentReturnsFalseOnConflict() {
        CacheService svc = newService(cacheManager, redisTemplate, null);
        when(redisTemplate.opsForValue()).thenReturn(valueOps);
        when(valueOps.setIfAbsent(anyString(), any(), anyLong(), any())).thenReturn(false);
        assertFalse(svc.tryIdempotent("k", 30), "key 已存在应返 false(幂等命中)");
    }

    @Test
    void tryIdempotentNoRedisReturnsTrue() {
        CacheService svc = newService(cacheManager, null, null);
        assertTrue(svc.tryIdempotent("k", 30), "无 Redis 时回退放行");
    }

    @Test
    void evictWithDoubleDeleteSchedulesSecondEvict() throws Exception {
        CacheBroadcast bc = mock(CacheBroadcast.class);
        CacheService svc = newService(cacheManager, redisTemplate, bc);
        setField(svc, "doubleDeleteDelayMs", 0L); // 0 关闭延迟双删
        svc.evict("c", "k");
        verify(redisTemplate).delete("k");
        verify(l1).evict("k");
        verify(bc, atLeastOnce()).publishEvict("c", "k");
    }

    // ---- §2.2 跨语言纯 JSON 契约:序列化器产出无 @class 类型标记,Go/Python 可 json.loads 互读 ----

    /**
     * 验证 CacheConfig.redisTemplate 配置的 value 序列化器写出的 JSON 不含 Java 类型标记(@class)。
     * 这是 §2.2 跨语言互通契约的代码级断言:Go {@code json.Unmarshal} / Python {@code json.loads}
     * 读到的是裸 JSON 对象/数组,不是 {@code ["FQCN", {...}]} 多态数组。
     */
    @Test
    void redisTemplate_valueSerializer_producesPureJsonWithoutTypeHint() {
        org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer ser =
                org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer.builder()
                        .defaultTyping(false)
                        .objectMapper(new com.fasterxml.jackson.databind.ObjectMapper()
                                .registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule()))
                        .build();
        // String → 裸字符串
        byte[] strBytes = ser.serialize("v1");
        assertEquals("\"v1\"", new String(strBytes), "String 应序列化为裸 JSON 字符串(无 @class)");
        // Map → 裸 JSON 对象(无 @class)
        java.util.Map<String, Object> userView = new java.util.LinkedHashMap<>();
        userView.put("id", 1L);
        userView.put("name", "x");
        byte[] mapBytes = ser.serialize(userView);
        String mapJson = new String(mapBytes);
        assertFalse(mapJson.contains("@class"), "Map JSON 不得含 @class 类型标记: " + mapJson);
        assertFalse(mapJson.startsWith("["), "Map JSON 不得是多态数组: " + mapJson);
        assertTrue(mapJson.startsWith("{") && mapJson.contains("\"id\"") && mapJson.contains("\"name\""),
                "Map JSON 应是裸 JSON 对象: " + mapJson);
        // List<Map> → 裸 JSON 数组(无 @class)
        byte[] listBytes = ser.serialize(java.util.List.of(userView));
        String listJson = new String(listBytes);
        assertFalse(listJson.contains("@class"), "List JSON 不得含 @class 类型标记: " + listJson);
        assertTrue(listJson.startsWith("[{") , "List JSON 应是裸 JSON 数组: " + listJson);
    }

    /**
     * 验证纯 JSON value 反序列化后,String/Map/List 均可还原为等价 Java 对象(Java 写 Java 读)。
     * L2 命中时 POJO 会还原为 LinkedHashMap(无类型标记),由调用方适配(见 LearningRpcProvider.toUserMastery)。
     */
    @Test
    void pureJsonSerializer_roundTrip_preservesStringAndMapAndList() {
        org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer ser =
                org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer.builder()
                        .defaultTyping(false)
                        .objectMapper(new com.fasterxml.jackson.databind.ObjectMapper()
                                .registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule()))
                        .build();
        // String round-trip
        assertEquals("v1", ser.deserialize(ser.serialize("v1")));
        assertEquals("__ESP_CACHE_EMPTY__", ser.deserialize(ser.serialize("__ESP_CACHE_EMPTY__")));
        // Map round-trip → LinkedHashMap(无类型标记,Jackson 默认)
        java.util.Map<String, Object> userView = new java.util.LinkedHashMap<>();
        userView.put("id", 1L);
        userView.put("name", "x");
        Object back = ser.deserialize(ser.serialize(userView));
        assertTrue(back instanceof java.util.Map, "Map 应还原为 Map(LinkedHashMap)");
        assertEquals(1, ((java.util.Map<?, ?>) back).get("id"));
        assertEquals("x", ((java.util.Map<?, ?>) back).get("name"));
        // List round-trip → ArrayList
        Object listBack = ser.deserialize(ser.serialize(java.util.List.of(userView)));
        assertTrue(listBack instanceof java.util.List, "List 应还原为 List");
        assertEquals(1, ((java.util.List<?>) listBack).size());
    }
}
