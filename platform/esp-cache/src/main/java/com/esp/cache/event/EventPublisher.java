package com.esp.cache.event;

import com.esp.cache.PulsarClientHolder;
import org.apache.pulsar.client.api.Producer;
import org.apache.pulsar.client.api.PulsarClient;
import org.apache.pulsar.client.api.Schema;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Component;

import jakarta.annotation.PreDestroy;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.TimeUnit;

/**
 * 通用业务事件发布器(EDA 重构 ADR-EDA):复用 {@link PulsarClientHolder} 延迟初始化 +
 * sendAsync + 失败降级(只 warn 不抛,业务无感知)。topic 用短名(见 {@link EventTopics})。
 *
 * <p>Schema:JSON。每个 topic 一个 Producer(按需创建,缓存复用)。
 * <p>设计:与 CacheBroadcast 同模式 —— broker 不可用时 producer 创建/发送均静默降级,
 * 不阻断业务(消息丢失可接受,与缓存失效 TTL 兜底同理)。
 *
 * <p>线程安全:producer 创建走 DCL,发送 sendAsync 线程安全。
 */
@Component
@Lazy
public class EventPublisher {

    private static final Logger log = LoggerFactory.getLogger(EventPublisher.class);

    private final PulsarClientHolder holder;
    private final String serviceUrl;
    private final Object lock = new Object();
    private final Map<String, Producer<Object>> producers = new HashMap<>();

    public EventPublisher(@Lazy PulsarClientHolder holder,
                          @Value("${esp.pulsar.service-url:pulsar://pulsar-broker:6650}") String serviceUrl) {
        this.holder = holder;
        this.serviceUrl = serviceUrl;
    }

    /**
     * 发布事件:sendAsync + 失败降级(只 warn 不抛)。
     * @param topic  短名 topic(见 {@link EventTopics})
     * @param payload 事件体 POJO(可 JSON 序列化)
     */
    public <T> void publish(String topic, T payload) {
        try {
            Producer<T> p = ensureProducer(topic, payload.getClass());
            if (p == null) return;
            p.sendAsync(payload)
                    .thenAccept(id -> { /* ok */ })
                    .exceptionally(ex -> {
                        log.warn("pulsar event publish failed topic={} err={}", topic, ex.toString());
                        return null;
                    });
        } catch (Exception ex) {
            log.warn("pulsar producer unavailable, skip publish topic={} err={}", topic, ex.toString());
        }
    }

    @SuppressWarnings({"unchecked", "rawtypes"})
    private <T> Producer<T> ensureProducer(String topic, Class<?> payloadType) throws Exception {
        synchronized (lock) {
            Producer<Object> existing = producers.get(topic);
            if (existing != null) return (Producer<T>) existing;
            PulsarClient client = ensureClient();
            if (client == null) return null;
            Producer p = client.newProducer(Schema.JSON(payloadType))
                    .topic(topic)
                    .sendTimeout(3, TimeUnit.SECONDS)
                    .blockIfQueueFull(false)
                    .create();
            producers.put(topic, p);
            log.info("pulsar event producer ready, topic={}, type={}", topic, payloadType.getSimpleName());
            return (Producer<T>) p;
        }
    }

    private PulsarClient ensureClient() {
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
                log.info("pulsar client connected (EventPublisher), serviceUrl={}", serviceUrl);
                return client;
            } catch (Exception ex) {
                log.warn("pulsar client connect failed (events degrade to no-op): {}", ex.toString());
                return null;
            }
        }
    }

    @PreDestroy
    public void close() {
        synchronized (lock) {
            producers.values().forEach(p -> {
                try { p.close(); } catch (Exception ignored) {}
            });
            producers.clear();
        }
    }
}
