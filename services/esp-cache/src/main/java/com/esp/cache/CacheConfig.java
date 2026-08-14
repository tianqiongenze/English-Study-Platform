package com.esp.cache;

import com.fasterxml.jackson.annotation.JsonAutoDetect;
import com.fasterxml.jackson.annotation.PropertyAccessor;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.jsontype.impl.LaissezFaireSubTypeValidator;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.cache.CacheManager;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.cache.caffeine.CaffeineCacheManager;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Lazy;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.serializer.GenericJackson2JsonRedisSerializer;
import org.springframework.data.redis.serializer.StringRedisSerializer;

/**
 * 多级缓存基建配置(ADR-多级缓存,公共包 esp-cache)。
 *
 * <p>三级一致性架构:
 * <ol>
 *   <li>L1 Caffeine(进程内,~1ms):maximumSize=10000 + expireAfterWrite=60s(TTL 兜底防 Pulsar 丢消息)。</li>
 *   <li>L2 Redis(跨实例,~1ms 内网):L2 TTL 5min,聚合类 30min;空结果 30s 防穿透。</li>
 *   <li>跨实例 L1 失效:Pulsar topic esp-cache-invalidate,所有实例 consumer 收到后 invalidate 本地 L1。</li>
 * </ol>
 *
 * <p>失效策略(写路径,见 {@link CacheService}):先删 L2 → 删 L1 → 发 Pulsar 广播 → 延迟双删(100ms 后再删一次,异步)。
 *
 * <p>feature flag:esp.cache.enabled=false 时本配置类不装配 → {@link CacheService} 通过 ObjectProvider
 * 拿不到 CacheManager/broadcast,所有方法走 no-op 直打 DB(回退安全)。
 *
 * <p>Pulsar bean 用 @Lazy + @ConditionalOnProperty,测试环境(无 Pulsar broker)可设
 * esp.pulsar.consumer-enabled=false 跳过 consumer 启动;producer 延迟到首次发消息时才连接,
 * 失败不阻断启动(由 {@link CacheService} 兜底降级)。
 */
@Configuration
@EnableCaching
@ConditionalOnProperty(prefix = "esp.cache", name = "enabled", havingValue = "true", matchIfMissing = true)
public class CacheConfig {

    private static final Logger log = LoggerFactory.getLogger(CacheConfig.class);

    @Value("${esp.cache.l1-spec:maximumSize=10000,expireAfterWrite=60s}")
    private String l1Spec;

    @Value("${esp.pulsar.service-url:pulsar://pulsar-broker:6650}")
    private String pulsarServiceUrl;

    @Value("${esp.pulsar.topic:esp-cache-invalidate}")
    private String pulsarTopic;

    @Value("${esp.pulsar.consumer-enabled:true}")
    private boolean consumerEnabled;

    /**
     * L1 Caffeine CacheManager:每个 cacheName 一个 Caffeine,共用同一 spec。
     * 手动 Cache-Aside(不用 @Cacheable 注解,因要精确控制 L1/L2 两级 + 失效广播)。
     */
    @Bean
    public CacheManager cacheManager() {
        // P1-12 缓存可观测:Caffeine spec 追加 recordStats(),
        // Spring Boot CacheMetricsAutoConfiguration 自动绑 Micrometer cache.gets{result=hit,miss}/cache.loads/cache.evictions。
        // 先量化命中率,再据此调 maximumSize/TTL(防凭感觉调参)。
        String specWithStats = l1Spec.contains("recordStats") ? l1Spec : l1Spec + ",recordStats";
        CaffeineCacheManager mgr = new CaffeineCacheManager();
        mgr.setCaffeine(com.github.benmanes.caffeine.cache.Caffeine.from(specWithStats));
        mgr.setAllowNullValues(true); // 空结果也缓存(防穿透,TTL 由 CacheService 控制)
        // P1-12:Caffeine cache 惰性创建,而 CacheMetricsRegistrar 只绑启动时已存在的 cache;
        // 预注册全部业务 cacheName,保证启动期即完成 Micrometer 绑定(否则首访后才建的 cache 无指标)。
        mgr.setCacheNames(java.util.List.of(
                "bff_user", "esp_jwt_blacklist", "esp_idem",
                "learning:due", "learning:weak", "learning:agg",
                "teacher:class-weak"));
        return mgr;
    }

    /**
     * L2 Redis 模板:String key + Jackson value(支持 POJO 与 JavaTime)。
     */
    @Bean
    public RedisTemplate<String, Object> redisTemplate(RedisConnectionFactory cf) {
        RedisTemplate<String, Object> tpl = new RedisTemplate<>();
        tpl.setConnectionFactory(cf);
        tpl.setKeySerializer(new StringRedisSerializer());

        ObjectMapper om = new ObjectMapper();
        om.registerModule(new JavaTimeModule());
        om.setVisibility(PropertyAccessor.ALL, JsonAutoDetect.Visibility.ANY);
        om.activateDefaultTyping(LaissezFaireSubTypeValidator.instance,
                ObjectMapper.DefaultTyping.NON_FINAL);
        GenericJackson2JsonRedisSerializer jsonSer = new GenericJackson2JsonRedisSerializer(om);

        tpl.setValueSerializer(jsonSer);
        tpl.setHashKeySerializer(new StringRedisSerializer());
        tpl.setHashValueSerializer(jsonSer);
        tpl.afterPropertiesSet();
        return tpl;
    }

    /**
     * PulsarClient 持有者(延迟初始化:首次 producer/consumer 创建时才连 broker)。
     * 由 {@link CacheBroadcast} 按需调用,连接失败时上层降级为不发广播 + TTL 兜底。
     */
    @Bean
    @Lazy
    public PulsarClientHolder pulsarClientHolder() {
        return new PulsarClientHolder();
    }

    /**
     * Pulsar 失效广播器(producer + consumer)。@Lazy 避免测试环境无 broker 时启动失败。
     */
    @Bean
    @Lazy
    public CacheBroadcast cacheBroadcast(@Lazy PulsarClientHolder holder, CacheManager cacheManager) {
        return new CacheBroadcast(holder, pulsarServiceUrl, pulsarTopic, consumerEnabled, cacheManager);
    }

    @PreDestroy
    public void shutdown() {
        log.info("CacheConfig destroyed (pulsar resources managed by CacheBroadcast)");
    }
}
