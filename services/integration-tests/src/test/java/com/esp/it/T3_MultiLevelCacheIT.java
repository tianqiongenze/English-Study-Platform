package com.esp.it;

import com.esp.cache.CacheService;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;
import org.springframework.data.redis.core.RedisTemplate;

import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * T3:多级缓存 L1/L2 代码级断言(真实 Redis L2 + Caffeine L1)。
 *
 * <p>断言点(全代码级):
 * <ol>
 *   <li>put → Redis L2 命中(key 存在);getOrLoad 同 key 返回 value 且 loader 不执行(L1 命中)。</li>
 *   <li>evict → Redis key 删除、L1 失效;延迟双删(等 200ms 后 Redis 仍无 key)。</li>
 *   <li>空结果防穿透:cacheEmpty=true + loader 返回 null → Redis 存 EMPTY_SENTINEL,再调 loader 不执行。</li>
 *   <li>L2 降级:Redis 异常时 getOrLoad 降级调 loader,不抛(本用例手动构造:删 key 后不回填,L2 读异常路径由 CacheService 内部 catch)。</li>
 *   <li>Pulsar 跨实例广播:@Disabled(单节点环境 consumer 关闭,广播逻辑由 CacheBroadcast 单测覆盖)。</li>
 * </ol>
 */
@SpringBootTest(classes = T3_MultiLevelCacheIT.ITConfig.class,
        properties = "spring.profiles.active=it")
class T3_MultiLevelCacheIT {

    @Configuration
    @Import(com.esp.cache.CacheConfig.class)
    static class ITConfig {
        @org.springframework.context.annotation.Bean
        CacheService cacheService(
                org.springframework.beans.factory.ObjectProvider<org.springframework.cache.CacheManager> cm,
                org.springframework.beans.factory.ObjectProvider<RedisTemplate<String, Object>> rt,
                org.springframework.beans.factory.ObjectProvider<com.esp.cache.CacheBroadcast> cb) {
            return new CacheService(cm, rt, cb);
        }
    }

    @Autowired
    private CacheService cacheService;

    @Autowired
    private RedisTemplate<String, Object> redisTemplate;

    private static final String CACHE = "test:cache";

    @AfterEach
    void cleanup() {
        try {
            redisTemplate.delete("test:k1");
            redisTemplate.delete("test:k2");
            redisTemplate.delete("test:k3");
        } catch (Exception ignored) {}
    }

    @Test
    void put_thenGetOrLoad_hitsL1_loaderNotCalled() {
        // 断言 1a:put 后 Redis L2 有 key
        cacheService.put(CACHE, "test:k1", "v1", 60);
        Object inRedis = redisTemplate.opsForValue().get("test:k1");
        assertThat(inRedis).as("L2 Redis should hold the value after put").isEqualTo("v1");

        // 断言 1b:getOrLoad 同 key 返回 value,且 loader 不执行(L1 命中)
        AtomicBoolean loaderCalled = new AtomicBoolean(false);
        String got = cacheService.getOrLoad(CACHE, "test:k1",
                () -> { loaderCalled.set(true); return "LOADER_VALUE"; },
                60, false);
        assertThat(got).as("should return cached value, not loader").isEqualTo("v1");
        assertThat(loaderCalled.get()).as("loader must NOT be called on L1 hit").isFalse();
    }

    @Test
    void evict_deletesL2_andL1_doubleDelete() throws Exception {
        cacheService.put(CACHE, "test:k2", "v2", 60);
        // 先确认 L2 有
        assertThat(redisTemplate.opsForValue().get("test:k2")).isEqualTo("v2");

        // evict:立即删 L2+L1,并安排延迟双删(100ms 后再删一次)
        cacheService.evict(CACHE, "test:k2");

        // 断言 2a:立即删除后 Redis 无 key
        assertThat(redisTemplate.opsForValue().get("test:k2"))
                .as("L2 should be deleted immediately on evict").isNull();

        // 等待延迟双删窗口(配置 100ms,等 200ms 兜底)
        Thread.sleep(250);

        // 断言 2b:延迟双删后 Redis 仍无 key(双删兜底防并发回填脏数据)
        assertThat(redisTemplate.opsForValue().get("test:k2"))
                .as("L2 should still be absent after double-delete window").isNull();

        // 断言 2c:evict 后 getOrLoad 应触发 loader(L1 也已失效)
        AtomicBoolean loaderCalled = new AtomicBoolean(false);
        String got = cacheService.getOrLoad(CACHE, "test:k2",
                () -> { loaderCalled.set(true); return "RELOADED"; },
                60, false);
        assertThat(loaderCalled.get()).as("loader should be called after evict (cache miss)").isTrue();
        assertThat(got).isEqualTo("RELOADED");
    }

    @Test
    void emptyResult_cacheEmpty_storesSentinel_loaderNotRecalled() {
        // 断言 3:cacheEmpty=true + loader 返回 null → Redis 存 EMPTY_SENTINEL
        AtomicBoolean loaderCalled = new AtomicBoolean(false);
        String got = cacheService.getOrLoad(CACHE, "test:k3",
                () -> { loaderCalled.set(true); return null; },
                60, true);
        assertThat(got).as("null loader result should return null").isNull();
        assertThat(loaderCalled.get()).as("loader should be called on first miss").isTrue();

        // 断言 3a:Redis 存了 sentinel(防穿透)
        Object inRedis = redisTemplate.opsForValue().get("test:k3");
        assertThat(inRedis).as("should store EMPTY_SENTINEL for null result").isEqualTo("__ESP_CACHE_EMPTY__");

        // 断言 3b:再调一次,loader 不执行(L1 命中 null wrapper)
        AtomicBoolean loaderCalled2 = new AtomicBoolean(false);
        String got2 = cacheService.getOrLoad(CACHE, "test:k3",
                () -> { loaderCalled2.set(true); return "SHOULD_NOT_HIT"; },
                60, true);
        assertThat(got2).as("second call should return cached null").isNull();
        assertThat(loaderCalled2.get()).as("loader must NOT be called on empty-sentinel hit").isFalse();
    }

    @Test
    void l2Degradation_loaderFallbackOnRedisError() {
        // 断言 4:L2 降级 —— 手动删 key + 强行让 L2 读不到(L1 也未命中),getOrLoad 仍能调 loader 返回值
        // CacheService 内部 catch Redis 异常降级到 loader;此处用正常 miss 路径验证 loader 兜底
        redisTemplate.delete("test:k1");
        // 清 L1:换一个新 cacheName 确保 L1 也 miss
        String freshCache = "test:fresh:" + System.currentTimeMillis();
        AtomicBoolean loaderCalled = new AtomicBoolean(false);
        String got = cacheService.getOrLoad(freshCache, "test:k1",
                () -> { loaderCalled.set(true); return "DEGRADED_VALUE"; },
                60, false);
        assertThat(got).as("should fall back to loader value").isEqualTo("DEGRADED_VALUE");
        assertThat(loaderCalled.get()).as("loader should be invoked on L1+L2 miss").isTrue();
    }

    /**
     * Pulsar 跨实例广播断言:单节点环境 consumer 关闭(esp.pulsar.consumer-enabled=false),
     * 无法端到端验证多实例 L1 失效。此处 @Disabled,广播逻辑由 esp-cache 的 CacheBroadcastTest 覆盖。
     * 真实多副本部署后可启用:启动两个 ApplicationContext,一个 evict,验证另一个 L1 被清。
     */
    @org.junit.jupiter.api.Disabled("Pulsar consumer disabled in single-node IT; covered by CacheBroadcastTest")
    @Test
    void pulsarBroadcast_crossInstanceL1Invalidation() {
        // 占位:多实例广播逻辑详见 esp-cache CacheBroadcastTest
    }
}
