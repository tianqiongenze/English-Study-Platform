package com.esp.cache;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.cache.Cache;
import org.springframework.cache.CacheManager;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.stereotype.Component;

import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.function.Supplier;

/**
 * 多级缓存服务(Cache-Aside 手动模式,封装 L1/L2 读写 + 失效广播 + 延迟双删)。
 *
 * <p>读流程 {@link #getOrLoad(String, String, Supplier, long, boolean)}:
 * <pre>
 *   L1 Caffeine → L2 Redis → DB(loader);DB 回填 L2(set TTL)+ L1。
 *   空结果也缓存(短 TTL emptyTtl)防穿透;空结果入 L1 用 null wrapper。
 * </pre>
 *
 * <p>失效流程 {@link #evict(String, String)}:
 * <pre>
 *   1. 删 L2 Redis(del key)
 *   2. 删 L1 Caffeine(evict)
 *   3. 发 Pulsar 广播(其他实例 consumer 删各自 L1)
 *   4. 延迟双删:异步延迟 doubleDeleteDelay ms 后重复 1-3(防并发回填脏数据)
 * </pre>
 *
 * <p>feature flag:esp.cache.enabled=false 时 {@link CacheConfig} 不装配,
 * 本类通过 {@link ObjectProvider} 拿到 null 的 CacheManager/RedisTemplate/broadcast,
 * 所有方法走 no-op(loader 直打 DB),回退安全。本类始终装配(无条件),
 * 这样使用方(LearningService 等)无需 ObjectProvider 包一层,直接 @Autowired 即可。
 */
@Component
public class CacheService {

    private static final Logger log = LoggerFactory.getLogger(CacheService.class);

    /** 空结果 sentinel:Redis 中存此字符串表示"空"(防穿透),L1 用 null wrapper。 */
    private static final String EMPTY_SENTINEL = "__ESP_CACHE_EMPTY__";

    private final CacheManager cacheManager;
    private final RedisTemplate<String, Object> redisTemplate;
    private final CacheBroadcast broadcast;

    @Value("${esp.cache.l2-ttl-seconds:300}")
    private long l2TtlSeconds;

    @Value("${esp.cache.empty-ttl-seconds:30}")
    private long emptyTtlSeconds;

    @Value("${esp.cache.double-delete-delay-ms:100}")
    private long doubleDeleteDelayMs;

    private final ScheduledExecutorService delayPool =
            Executors.newSingleThreadScheduledExecutor(r -> {
                Thread t = new Thread(r, "cache-double-delete");
                t.setDaemon(true);
                return t;
            });

    public CacheService(ObjectProvider<CacheManager> cacheManagerProvider,
                        ObjectProvider<RedisTemplate<String, Object>> redisTemplateProvider,
                        ObjectProvider<CacheBroadcast> broadcastProvider) {
        this.cacheManager = cacheManagerProvider.getIfAvailable();
        this.redisTemplate = redisTemplateProvider.getIfAvailable();
        this.broadcast = broadcastProvider.getIfAvailable();
    }

    /**
     * 读:L1 → L2 → DB。
     *
     * @param cacheName  L1 缓存名(如 "learning:due")
     * @param key        完整 key(如 "learning:due:1")
     * @param loader     DB 加载函数(L1/L2 miss 时调用)
     * @param ttlSec     L2 TTL(秒);传 0 用默认 l2TtlSeconds
     * @param cacheEmpty 是否缓存空结果(防穿透)
     * @return 缓存或 DB 的值;无缓存能力时直接返回 loader 结果
     */
    @SuppressWarnings("unchecked")
    public <T> T getOrLoad(String cacheName, String key, Supplier<T> loader, long ttlSec, boolean cacheEmpty) {
        if (cacheManager == null || redisTemplate == null) return loader.get();
        Cache l1 = cacheManager.getCache(cacheName);
        if (l1 == null) return loader.get();

        // 1. L1
        Cache.ValueWrapper w = l1.get(key);
        if (w != null) {
            Object v = w.get();
            return v == null ? null : (T) v;
        }

        // 2. L2
        T val = null;
        boolean l2Hit = false;
        try {
            Object raw = redisTemplate.opsForValue().get(key);
            if (raw != null) {
                l2Hit = true;
                if (EMPTY_SENTINEL.equals(raw)) {
                    l1.put(key, null); // 空结果回填 L1
                    return null;
                }
                val = (T) raw;
            }
        } catch (Exception ex) {
            log.warn("L2 redis read failed for {}, degrade to DB: {}", key, ex.toString());
        }

        // 3. DB
        if (!l2Hit) {
            val = loader.get();
            try {
                long ttl = ttlSec > 0 ? ttlSec : l2TtlSeconds;
                if (val == null) {
                    if (cacheEmpty) {
                        redisTemplate.opsForValue().set(key, EMPTY_SENTINEL, emptyTtlSeconds, TimeUnit.SECONDS);
                        l1.put(key, null);
                    }
                } else {
                    redisTemplate.opsForValue().set(key, val, ttl, TimeUnit.SECONDS);
                    l1.put(key, val);
                }
            } catch (Exception ex) {
                log.warn("L2 redis write-back failed for {}: {}", key, ex.toString());
            }
        } else {
            l1.put(key, val); // L2 命中,回填 L1
        }
        return val;
    }

    public void put(String cacheName, String key, Object value, long ttlSec) {
        if (cacheManager == null || redisTemplate == null) return;
        try {
            long ttl = ttlSec > 0 ? ttlSec : l2TtlSeconds;
            if (value == null) {
                redisTemplate.opsForValue().set(key, EMPTY_SENTINEL, emptyTtlSeconds, TimeUnit.SECONDS);
            } else {
                redisTemplate.opsForValue().set(key, value, ttl, TimeUnit.SECONDS);
            }
        } catch (Exception ex) {
            log.warn("L2 redis put failed for {}: {}", key, ex.toString());
        }
        Cache l1 = cacheManager.getCache(cacheName);
        if (l1 != null) l1.put(key, value);
    }

    /**
     * 失效:删 L2 + 删 L1 + Pulsar 广播 + 延迟双删(异步,不阻塞写路径)。
     */
    public void evict(String cacheName, String key) {
        if (cacheManager == null || redisTemplate == null) return;
        doEvict(cacheName, key);
        if (doubleDeleteDelayMs > 0) {
            delayPool.schedule(() -> doEvict(cacheName, key),
                    doubleDeleteDelayMs, TimeUnit.MILLISECONDS);
        }
    }

    private void doEvict(String cacheName, String key) {
        try {
            redisTemplate.delete(key);
        } catch (Exception ex) {
            log.warn("L2 redis delete failed for {}: {}", key, ex.toString());
        }
        Cache l1 = cacheManager.getCache(cacheName);
        if (l1 != null) l1.evict(key);
        if (broadcast != null) broadcast.publishEvict(cacheName, key);
    }

    /** 是否启用(测试/回退判断用)。 */
    public boolean isEnabled() {
        return cacheManager != null;
    }

    /**
     * 幂等占位(SETNX + TTL):尝试用 key 占位,仅当 key 不存在时成功。
     * 用于核心写接口的请求级幂等(eg esp:idem:teacher:summary:&lt;submissionId&gt;)。
     * 命中返回 false(已被处理过);首次/无缓存能力返回 true(本次应执行业务)。
     *
     * @param key     完整 Redis key(如 esp:idem:teacher:summary:123)
     * @param ttlSec  TTL(秒);&lt;=1 时按 1 处理
     * @return true=本次应执行业务(已占位/无缓存);false=幂等命中,应跳过
     */
    public boolean tryIdempotent(String key, long ttlSec) {
        if (redisTemplate == null) return true; // 回退:无 Redis 不阻断业务
        try {
            long ttl = Math.max(1, ttlSec);
            Boolean ok = redisTemplate.opsForValue()
                    .setIfAbsent(key, "1", ttl, TimeUnit.SECONDS);
            return Boolean.TRUE.equals(ok);
        } catch (Exception ex) {
            log.warn("idempotent SETNX failed for {}, degrade to execute: {}", key, ex.toString());
            return true; // 异常时放行,业务优先(避免缓存故障阻断写)
        }
    }
}
