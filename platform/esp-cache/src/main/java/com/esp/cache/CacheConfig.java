package com.esp.cache;

import com.fasterxml.jackson.annotation.JsonAutoDetect;
import com.fasterxml.jackson.annotation.PropertyAccessor;
import com.fasterxml.jackson.databind.ObjectMapper;
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
     * L2 Redis 模板:String key + 纯 JSON value(跨语言互通,兑现 §2.2 契约)。
     *
     * <p>序列化决策(2026-08-15 会话22-28,兑现多级缓存架构对齐文档 §2.2 承诺):
     * <ul>
     *   <li><b>去掉 activateDefaultTyping</b>:原 {@code DefaultTyping.NON_FINAL} 会在 JSON 中嵌入
     *       Java FQCN 类型标记(形如 {@code ["com.esp.xxx.UserView", {...}]})。Go/Python 读到后
     *       {@code json.Unmarshal}/{@code json.loads} 得到的是数组而非预期对象,跨语言互读失败。
     *       这是 §2.2 标注的"定时炸弹"。</li>
     *   <li><b>方案选型</b>:经探查,Java 侧缓存 value 类型有限——String(sentinel/"1" 幂等占位/
     *       JWT 黑名单)、Map(bff user view、behavior 聚合 {@code List<Map<String,Object>>})、
     *       POJO 列表({@code List<UserMastery>} / {@code List<Object[]>})。所有 value 的下游消费方
     *       均不依赖 Java 类型标记:① HTTP 入口直接把缓存值交给 Spring 的 Jackson 序列化写响应体,
     *       不强类型;② {@code LearningRpcProvider.weakestPoints()} 会缓存命中后对每个元素调
     *       getter,要求元素是 {@code UserMastery} —— 但 L1 Caffeine 命中时直接是原对象(不经
     *       Redis 序列化),只有 L2 命中回填路径会拿 {@code LinkedHashMap}。经查 L2 命中回填后
     *       返回的元素是 {@code LinkedHashMap}(Jackson 无类型标记反序列化默认结果),
     *       {@code getKpId()} 等 getter 调用会抛 ClassCastException。
     *       <b>故对 POJO 列表缓存点加防御性类型还原</b>(见各调用方注释/适配),
     *       这是去掉 DefaultTyping 的唯一需要适配的代价。</li>
     *   <li><b>选用 GenericJackson2JsonRedisSerializer.builder().defaultTyping(false)</b>:
     *       Spring Data Redis 3.x 新增 builder API,显式关 defaultTyping 即写出无类型标记的纯 JSON。
     *       相比自定义 StringRedisSerializer + 调用方自行 ObjectMapper 的方案,本方案对调用方零侵入
     *       (CacheService 的 {@code opsForValue().set/get} 签名不变),且 ObjectMapper 配 JavaTimeModule
     *       支持 Instant/BigDecimal 等 Java 时间类型。</li>
     *   <li><b>存量缓存兼容</b>:Redis 中旧格式 key(带 {@code @class}/类型数组)在新序列化器下反序列化
     *       会失败。因所有缓存 TTL ≤ 30min,旧 key 在滚动部署后 30min 内自然过期;CacheService 内部
     *       L2 读异常已 catch 降级到 loader(见 {@link CacheService#getOrLoad}),故旧 key 读失败不报错
     *       只触发一次回源。滚动部署窗口建议避开高峰,或对热点前缀执行 {@code DEL}(见 PROGRESS 说明)。</li>
     * </ul>
     */
    @Bean
    public RedisTemplate<String, Object> redisTemplate(RedisConnectionFactory cf) {
        RedisTemplate<String, Object> tpl = new RedisTemplate<>();
        tpl.setConnectionFactory(cf);
        tpl.setKeySerializer(new StringRedisSerializer());

        ObjectMapper om = new ObjectMapper();
        om.registerModule(new JavaTimeModule());
        om.setVisibility(PropertyAccessor.ALL, JsonAutoDetect.Visibility.ANY);
        // §2.2 契约:关闭 defaultTyping,value 为纯 JSON(无 Java 类型标记),Go/Python 可直接 json 解析。
        // 原 activateDefaultTyping(NON_FINAL) 会写 ["FQCN", {...}],跨语言互读失败。
        GenericJackson2JsonRedisSerializer jsonSer = GenericJackson2JsonRedisSerializer.builder()
                .defaultTyping(false)
                .objectMapper(om)
                .build();

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
