package com.esp.cache;

import org.apache.pulsar.client.api.Consumer;
import org.apache.pulsar.client.api.Message;
import org.apache.pulsar.client.api.Producer;
import org.apache.pulsar.client.api.PulsarClient;
import org.apache.pulsar.client.api.Schema;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.cache.CacheManager;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

/**
 * Pulsar 失效广播器 + 跨实例 L1 evict(ADR-多级缓存)。
 *
 * <p>producer:写后失效时 {@link #publishEvict(String, String)} 发 {@link InvalidateMsg}
 * 到 topic esp-cache-invalidate。sendAsync 失败仅记 log 不抛(降级为单实例 L1 失效 + TTL 兜底)。
 *
 * <p>consumer(本实例启动一个):订阅同一 topic,收到消息后对本地 L1 Caffeine 执行 invalidate(key)。
 * 多副本:A 实例 review → A 删自己 L1 + 发广播 → B 实例 consumer 收到 → B 删自己 L1。
 * consumer 启动失败不阻断应用启动(esp.pulsar.consumer-enabled=false 可关)。
 *
 * <p>延迟双删不在此处做(由 {@link CacheService} 编排:立即删 → 发广播 → 100ms 后再删 + 再广播)。
 */
public class CacheBroadcast {

    private static final Logger log = LoggerFactory.getLogger(CacheBroadcast.class);

    private final PulsarClientHolder holder;
    private final String serviceUrl;
    private final String topic;
    private final boolean consumerEnabled;
    private final CacheManager cacheManager;

    private volatile Producer<InvalidateMsg> producer;
    private volatile Consumer<InvalidateMsg> consumer;
    private final Object lock = new Object();

    public CacheBroadcast(PulsarClientHolder holder, String serviceUrl, String topic,
                          boolean consumerEnabled, CacheManager cacheManager) {
        this.holder = holder;
        this.serviceUrl = serviceUrl;
        this.topic = topic;
        this.consumerEnabled = consumerEnabled;
        this.cacheManager = cacheManager;
        if (consumerEnabled) {
            startConsumer();
        }
    }

    /** 发广播:写后失效路径调用。失败静默(降级为本地失效 + TTL 兜底)。 */
    public void publishEvict(String cacheName, String key) {
        try {
            Producer<InvalidateMsg> p = ensureProducer();
            if (p == null) return;
            p.sendAsync(new InvalidateMsg(cacheName, key))
                    .thenAccept(id -> { /* ok */ })
                    .exceptionally(ex -> {
                        log.warn("pulsar broadcast evict failed for {}: {}", key, ex.toString());
                        return null;
                    });
        } catch (Exception ex) {
            log.warn("pulsar producer unavailable, skip broadcast for {}: {}", key, ex.toString());
        }
    }

    private Producer<InvalidateMsg> ensureProducer() throws Exception {
        if (producer != null) return producer;
        synchronized (lock) {
            if (producer != null) return producer;
            PulsarClient client = ensureClient();
            if (client == null) return null;
            producer = client.newProducer(Schema.JSON(InvalidateMsg.class))
                    .topic(topic)
                    .sendTimeout(3, TimeUnit.SECONDS)
                    .blockIfQueueFull(false)
                    .create();
            log.info("pulsar cache-invalidate producer ready, topic={}", topic);
            return producer;
        }
    }

    private PulsarClient ensureClient() throws Exception {
        PulsarClient existing = holder.get();
        if (existing != null) return existing;
        synchronized (lock) {
            if (holder.get() != null) return holder.get();
            try {
                PulsarClient client = PulsarClient.builder()
                        .serviceUrl(serviceUrl)
                        .connectionTimeout(5, TimeUnit.SECONDS)
                        .build();
                holder.set(client);
                log.info("pulsar client connected, serviceUrl={}", serviceUrl);
                return client;
            } catch (Exception ex) {
                log.warn("pulsar client connect failed (degrade to local-only L1 evict): {}",
                        ex.toString());
                return null;
            }
        }
    }

    private void startConsumer() {
        CompletableFuture.runAsync(() -> {
            try {
                PulsarClient client = ensureClient();
                if (client == null) {
                    log.info("pulsar client unavailable, cache-invalidate consumer disabled");
                    return;
                }
                synchronized (lock) {
                    if (consumer != null) return;
                    consumer = client.newConsumer(Schema.JSON(InvalidateMsg.class))
                            .topic(topic)
                            .subscriptionName("esp-cache-l1-evict")
                            .subscribe();
                }
                log.info("pulsar cache-invalidate consumer started, topic={}", topic);
                while (!Thread.currentThread().isInterrupted() && consumer != null) {
                    Message<InvalidateMsg> msg = consumer.receive(2, TimeUnit.SECONDS);
                    if (msg == null) continue;
                    InvalidateMsg body = msg.getValue();
                    if (body != null) {
                        evictLocal(body.getCacheName(), body.getKey());
                    }
                    consumer.acknowledge(msg);
                }
            } catch (Exception ex) {
                log.warn("pulsar consumer start/loop failed (L1 仅本地失效,跨实例靠 TTL 兜底): {}",
                        ex.toString());
            }
        });
    }

    /** 收到广播后 evict 本地 L1(只删 Caffeine,不删 L2:广播发起方已删 L2)。 */
    private void evictLocal(String cacheName, String key) {
        org.springframework.cache.Cache cache = cacheManager.getCache(cacheName);
        if (cache != null) {
            cache.evict(key);
            log.debug("L1 evicted via pulsar broadcast: {}:{}", cacheName, key);
        }
    }

    public void close() {
        try { if (consumer != null) consumer.close(); } catch (Exception ignored) {}
        try { if (producer != null) producer.close(); } catch (Exception ignored) {}
    }
}
