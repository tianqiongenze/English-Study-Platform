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
     *
     * <p><b>P0-D1(2026-08-21)警告</b>:此方法为<b>非可靠发布</b>——broker 不可达静默丢、
     * sendAsync 失败只 warn 不抛、且若在事务内调用会在提交前触发(事务回滚但消息已发=幽灵事件)。
     * <b>关键业务事件(如 teacher.finalized 驱动 mastery 评分)禁止用此方法,改用
     * {@link #publishReliable(String, Object, String)}</b> 配合调用方在
     * {@code @TransactionalEventListener(AFTER_COMMIT)} 中调用。
     *
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

    /**
     * 可靠发布:P0-D1(2026-08-21)修复关键事件丢失。
     *
     * <p>与 {@link #publish} 区别:
     * <ul>
     *   <li><b>同步发送</b>({@code send} 阻塞至 broker ack,非 sendAsync),确保调用方能感知失败;</li>
     *   <li><b>broker 不可达/send 失败抛 {@link EventPublishException}}</b>,而非静默 warn 丢弃——
     *       调用方据此入重试队列(参见 teacher-java 的 after-commit 桥);</li>
     *   <li><b>blockIfQueueFull(true)</b>:队列满时阻塞等待(有界超时)而非拒绝丢消息。</li>
     * </ul>
     *
     * <p><b>使用范式</b>:调用方须在 {@code @TransactionalEventListener(phase = AFTER_COMMIT)} 中调用此方法,
     * 保证仅事务提交成功后才发消息(消除幽灵事件);失败由调用方降级为入 Redis 重试队列或死信告警。
     *
     * @param topic    短名 topic
     * @param payload  事件体
     * @param idempotencyKey 幂等键(用于重试去重,同一 key 多次发送下游应幂等处理);可为 null
     * @throws EventPublishException broker 不可达或同步发送失败(调用方应捕获并降级为重试/告警)
     */
    public <T> void publishReliable(String topic, T payload, String idempotencyKey) throws EventPublishException {
        try {
            Producer<T> p = ensureProducerReliable(topic, payload.getClass());
            if (p == null) {
                throw new EventPublishException("pulsar producer unavailable for topic=" + topic);
            }
            p.send(payload); // 同步发送:阻塞至 broker ack,失败抛异常让调用方感知
        } catch (EventPublishException e) {
            throw e;
        } catch (Exception ex) {
            throw new EventPublishException("reliable publish failed topic=" + topic + " key=" + idempotencyKey + " err=" + ex, ex);
        }
    }

    /**
     * reliable producer 用 blockIfQueueFull(true)(队列满阻塞而非拒绝丢消息)。
     * 与 {@link #ensureProducer} 缓存隔离(不混用 reliable/unreliable producer 配置)。
     */
    @SuppressWarnings({"unchecked", "rawtypes"})
    private <T> Producer<T> ensureProducerReliable(String topic, Class<?> payloadType) throws Exception {
        synchronized (lock) {
            // reliable producer 用 "reliable:" 前缀 key 与 unreliable 隔离
            String key = "reliable:" + topic;
            Producer<Object> existing = producers.get(key);
            if (existing != null) return (Producer<T>) existing;
            PulsarClient client = ensureClient();
            if (client == null) return null;
            Producer p = client.newProducer(Schema.JSON(payloadType))
                    .topic(topic)
                    .sendTimeout(10, TimeUnit.SECONDS) // LLM/业务事件可稍长,3s→10s
                    .blockIfQueueFull(true)            // 队列满阻塞而非拒绝丢消息(P0-D1)
                    .create();
            producers.put(key, p);
            log.info("pulsar event producer ready(reliable), topic={}, type={}", topic, payloadType.getSimpleName());
            return (Producer<T>) p;
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
