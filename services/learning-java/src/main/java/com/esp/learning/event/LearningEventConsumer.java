package com.esp.learning.event;

import com.esp.cache.PulsarClientHolder;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.SessionEndedEvent;
import com.esp.cache.event.TeacherFinalizedEvent;
import com.esp.cache.event.UserRegisteredEvent;
import com.esp.learning.domain.UserMastery;
import com.esp.learning.repository.UserMasteryRepository;
import com.esp.learning.service.LearningService;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import org.apache.pulsar.client.api.Consumer;
import org.apache.pulsar.client.api.Message;
import org.apache.pulsar.client.api.PulsarClient;
import org.apache.pulsar.client.api.Schema;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Lazy;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

/**
 * learning-java 侧 EDA consumer:监听 {@link EventTopics#USER_REGISTERED} 与
 * {@link EventTopics#TEACHER_FINALIZED} 两个业务 topic(跨服务事件)。
 *
 * <p>复用 esp-cache PulsarClientHolder 延迟初始化模式;consumer 启动失败不阻断应用
 * (esp.pulsar.consumer-enabled=false 可关)。幂等:
 * <ul>
 *   <li>user.registered:initMastery 前先按 (userId,kpId) 过滤已存在,跳过。</li>
 *   <li>teacher.finalized:user_mastery upsert(存在则更新 score,不存在则插入)。</li>
 * </ul>
 */
@Component
@Lazy
public class LearningEventConsumer {

    private static final Logger log = LoggerFactory.getLogger(LearningEventConsumer.class);

    private final PulsarClientHolder holder;
    private final String serviceUrl;
    private final boolean consumerEnabled;
    private final LearningService learningService;
    private final UserMasteryRepository masteryRepo;
    private final JdbcTemplate jdbc;

    private volatile Consumer<UserRegisteredEvent> regConsumer;
    private volatile Consumer<TeacherFinalizedEvent> finConsumer;
    private volatile Consumer<SessionEndedEvent> endConsumer;
    /** P2-2: 订阅入口投递的 learning.event.report(worker 模式下消费)。 */
    private volatile Consumer<LearningService.LearningEventReport> reportConsumer;
    private volatile boolean running = true;

    public LearningEventConsumer(@Lazy PulsarClientHolder holder,
                                 @Value("${esp.pulsar.service-url:pulsar://pulsar-broker:6650}") String serviceUrl,
                                 @Value("${esp.pulsar.consumer-enabled:true}") boolean consumerEnabled,
                                 LearningService learningService,
                                 UserMasteryRepository masteryRepo,
                                 @org.springframework.beans.factory.annotation.Qualifier("jdbcTemplate") JdbcTemplate jdbc) {
        this.holder = holder;
        this.serviceUrl = serviceUrl;
        this.consumerEnabled = consumerEnabled;
        this.learningService = learningService;
        this.masteryRepo = masteryRepo;
        this.jdbc = jdbc;
    }

    @PostConstruct
    public void start() {
        if (!consumerEnabled) {
            log.info("learning EDA consumer disabled (esp.pulsar.consumer-enabled=false)");
            return;
        }
        startLoop(EventTopics.USER_REGISTERED, "esp-learning-user-registered",
                Schema.JSON(UserRegisteredEvent.class), this::handleRegistered);
        startLoop(EventTopics.TEACHER_FINALIZED, "esp-learning-teacher-finalized",
                Schema.JSON(TeacherFinalizedEvent.class), this::handleFinalized);
        startLoop(EventTopics.SESSION_ENDED, "esp-learning-session-ended",
                Schema.JSON(SessionEndedEvent.class), this::handleSessionEnded);
        // P2-2: 订阅入口投递的学情事件(worker 模式下入口发 Pulsar,worker 消费后进 Disruptor)。
        // 单进程一体化模式下入口直接进程内 Disruptor,本 topic 无消息,订阅无副作用。
        startLoop(EventTopics.LEARNING_EVENT_REPORT, "esp-learning-event-report",
                Schema.JSON(LearningService.LearningEventReport.class), this::handleEventReport);
    }

    /**
     * P2-2: 消费入口投递的学情事件,转交 Disruptor 批量落库。
     * worker 模式下本进程 Disruptor 已启;单进程模式本 topic 无消息。
     */
    private void handleEventReport(LearningService.LearningEventReport ev) {
        try {
            learningService.reportEvent(ev.userId(), ev.eventType(), ev.payload());
        } catch (Exception ex) {
            log.warn("learning.event.report handle failed userId={} err={}", ev.userId(), ex.toString());
        }
    }

    /** user.registered:查全量 kpId → initMastery(幂等:已存在跳过)。 */
    private void handleRegistered(UserRegisteredEvent ev) {
        try {
            long userId = ev.getUserId();
            List<Long> kpIds = jdbc.queryForList("SELECT id FROM knowledge_points", Long.class);
            // 幂等:过滤已存在的 (userId, kpId)
            List<UserMastery> exist = masteryRepo.findAll().stream()
                    .filter(m -> m.getUserId() == userId).toList();
            if (exist.size() >= kpIds.size()) {
                log.info("user.registered already inited, skip userId={}", userId);
                return;
            }
            learningService.initMastery(userId, kpIds, 0.0);
            log.info("user.registered handled: userId={}, kpCount={}", userId, kpIds.size());
        } catch (Exception ex) {
            log.warn("user.registered handle failed userId={} err={}", ev.getUserId(), ex.toString());
        }
    }

    /** teacher.finalized:upsert user_mastery(对应 kp mastery_score = finalScore)。 */
    private void handleFinalized(TeacherFinalizedEvent ev) {
        try {
            if (ev.getKpId() == null) {
                log.debug("teacher.finalized no kpId, skip userId={}", ev.getUserId());
                return;
            }
            Long kpId = ev.getKpId();
            BigDecimal score = BigDecimal.valueOf(ev.getFinalScore());
            UserMastery m = masteryRepo.findById(
                    new com.esp.learning.domain.UserMasteryId(ev.getUserId(), kpId)).orElse(null);
            if (m == null) {
                m = new UserMastery(ev.getUserId(), kpId);
                m.setReviewCount(0);
            }
            m.setMasteryScore(score);
            m.setLastReviewAt(Instant.now());
            masteryRepo.save(m); // upsert(幂等:同主键覆盖)
            log.info("teacher.finalized handled: userId={}, kpId={}, score={}",
                    ev.getUserId(), kpId, score);
        } catch (Exception ex) {
            log.warn("teacher.finalized handle failed err={}", ex.toString());
        }
    }

    /** session.ended:口语会话结束 → 经 Disruptor 记 session_ended 学情事件(驱动复习调度)。 */
    private void handleSessionEnded(SessionEndedEvent ev) {
        try {
            learningService.reportEvent(ev.getUserId(), "session_ended", ev.getSessionId());
            log.info("session.ended handled: userId={}, sessionId={}", ev.getUserId(), ev.getSessionId());
        } catch (Exception ex) {
            log.warn("session.ended handle failed err={}", ex.toString());
        }
    }

    @FunctionalInterface
    private interface Handler<T> { void handle(T payload); }

    private <T> void startLoop(String topic, String subName, Schema<T> schema, Handler<T> handler) {
        CompletableFuture.runAsync(() -> {
            try {
                PulsarClient client = ensureClient();
                if (client == null) {
                    log.info("pulsar client unavailable, learning consumer({}) disabled", topic);
                    return;
                }
                Consumer<T> c = client.newConsumer(schema)
                        .topic(topic)
                        .subscriptionName(subName)
                        .subscribe();
                if (topic.equals(EventTopics.USER_REGISTERED)) regConsumer = uncheckedCast(c);
                else if (topic.equals(EventTopics.TEACHER_FINALIZED)) finConsumer = uncheckedCast(c);
                else endConsumer = uncheckedCast(c);
                log.info("learning EDA consumer started, topic={}", topic);
                while (running && !Thread.currentThread().isInterrupted()) {
                    Message<T> msg = c.receive(2, TimeUnit.SECONDS);
                    if (msg == null) continue;
                    try {
                        handler.handle(msg.getValue());
                        c.acknowledge(msg);
                    } catch (Exception ex) {
                        log.warn("learning consumer handle err topic={} err={}", topic, ex.toString());
                        c.negativeAcknowledge(msg);
                    }
                }
            } catch (Exception ex) {
                log.warn("learning consumer({}) start/loop failed: {}", topic, ex.toString());
            }
        });
    }

    @SuppressWarnings("unchecked")
    private static <T> Consumer<T> uncheckedCast(Consumer<?> c) { return (Consumer<T>) c; }

    private PulsarClient ensureClient() {
        PulsarClient existing = holder.get();
        if (existing != null) return existing;
        try {
            PulsarClient client = PulsarClient.builder()
                    .serviceUrl(serviceUrl)
                    .connectionTimeout(5, TimeUnit.SECONDS)
                    .build();
            holder.set(client);
            log.info("pulsar client connected (learning), serviceUrl={}", serviceUrl);
            return client;
        } catch (Exception ex) {
            log.warn("pulsar client connect failed (learning consumer disabled): {}", ex.toString());
            return null;
        }
    }

    @PreDestroy
    public void close() {
        running = false;
        try { if (regConsumer != null) regConsumer.close(); } catch (Exception ignored) {}
        try { if (finConsumer != null) finConsumer.close(); } catch (Exception ignored) {}
        try { if (endConsumer != null) endConsumer.close(); } catch (Exception ignored) {}
        try { if (reportConsumer != null) reportConsumer.close(); } catch (Exception ignored) {}
    }
}
