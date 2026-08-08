package com.esp.teacher.event;

import com.esp.cache.PulsarClientHolder;
import com.esp.cache.event.AutoGradeRequestedEvent;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.TeacherFinalizedEvent;
import com.esp.cache.event.WritingGradedEvent;
import com.esp.teacher.service.TeacherService;
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
import org.springframework.stereotype.Component;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;

/**
 * teacher-java 内部削峰 consumer:监听 {@link EventTopics#AUTO_GRADE_REQUESTED},
 * 异步执行评阅逻辑({@link TeacherService#doAutoGrade})。
 *
 * <p>设计:autoGrade HTTP 入口只发消息立即返回(避免长事务阻塞),
 * consumer 消费执行实际评阅(与 LearningEventConsumer 同模式)。
 */
@Component
@Lazy
public class TeacherEventConsumer {

    private static final Logger log = LoggerFactory.getLogger(TeacherEventConsumer.class);

    private final PulsarClientHolder holder;
    private final String serviceUrl;
    private final boolean consumerEnabled;
    private final TeacherService teacherService;
    private final EventPublisher eventPublisher;

    private volatile Consumer<AutoGradeRequestedEvent> consumer;
    private volatile Consumer<WritingGradedEvent> writingConsumer;
    private volatile boolean running = true;

    public TeacherEventConsumer(@Lazy PulsarClientHolder holder,
                               @Value("${esp.pulsar.service-url:pulsar://pulsar-broker:6650}") String serviceUrl,
                               @Value("${esp.pulsar.consumer-enabled:true}") boolean consumerEnabled,
                               TeacherService teacherService,
                               @Lazy EventPublisher eventPublisher) {
        this.holder = holder;
        this.serviceUrl = serviceUrl;
        this.consumerEnabled = consumerEnabled;
        this.teacherService = teacherService;
        this.eventPublisher = eventPublisher;
    }

    @PostConstruct
    public void start() {
        if (!consumerEnabled) {
            log.info("teacher EDA consumer disabled (esp.pulsar.consumer-enabled=false)");
            return;
        }
        CompletableFuture.runAsync(() -> {
            try {
                PulsarClient client = ensureClient();
                if (client == null) {
                    log.info("pulsar client unavailable, teacher consumer disabled");
                    return;
                }
                consumer = client.newConsumer(Schema.JSON(AutoGradeRequestedEvent.class))
                        .topic(EventTopics.AUTO_GRADE_REQUESTED)
                        .subscriptionName("esp-teacher-autograde")
                        .subscribe();
                log.info("teacher EDA consumer started, topic={}", EventTopics.AUTO_GRADE_REQUESTED);
                while (running && !Thread.currentThread().isInterrupted()) {
                    Message<AutoGradeRequestedEvent> msg = consumer.receive(2, TimeUnit.SECONDS);
                    if (msg == null) continue;
                    try {
                        AutoGradeRequestedEvent ev = msg.getValue();
                        teacherService.doAutoGrade(ev.getAssignmentId(), ev.getClassId());
                        consumer.acknowledge(msg);
                    } catch (Exception ex) {
                        log.warn("teacher consumer handle err err={}", ex.toString());
                        consumer.negativeAcknowledge(msg);
                    }
                }
            } catch (Exception ex) {
                log.warn("teacher consumer start/loop failed: {}", ex.toString());
            }
        });
        startWritingGradedLoop();
    }

    /** writing.graded:writing-py 批改完成 → 记录统计(user_id 透传后可回写 mastery)。 */
    private void startWritingGradedLoop() {
        CompletableFuture.runAsync(() -> {
            try {
                PulsarClient client = ensureClient();
                if (client == null) {
                    log.info("pulsar client unavailable, teacher writing.graded disabled");
                    return;
                }
                writingConsumer = client.newConsumer(Schema.JSON(WritingGradedEvent.class))
                        .topic(EventTopics.WRITING_GRADED)
                        .subscriptionName("esp-teacher-writing-graded")
                        .subscribe();
                log.info("teacher EDA consumer started, topic={}", EventTopics.WRITING_GRADED);
                while (running && !Thread.currentThread().isInterrupted()) {
                    Message<WritingGradedEvent> msg = writingConsumer.receive(2, TimeUnit.SECONDS);
                    if (msg == null) continue;
                    try {
                        WritingGradedEvent ev = msg.getValue();
                        log.info("writing.graded received: status={}, threadId={}, userId={}",
                                ev.getStatus(), ev.getThreadId(), ev.getUserId());
                        // P1-6 断点2:writing.graded → teacher.finalized 桥接,打通 writing→mastery
                        if (ev.getUserId() > 0) {
                            // 用 assignmentId 占位 kpId(与 TeacherService.finalized 同约定;
                            // writing-py 无 assignmentId,暂用 userId 作占位,真实 kpId 待批改回填)
                            TeacherFinalizedEvent fin = new TeacherFinalizedEvent(
                                    ev.getUserId(), (long) ev.getUserId(), ev.getScore());
                            eventPublisher.publish(EventTopics.TEACHER_FINALIZED, fin);
                            log.info("writing.graded → teacher.finalized bridged: userId={}, score={}",
                                    ev.getUserId(), ev.getScore());
                        }
                        writingConsumer.acknowledge(msg);
                    } catch (Exception ex) {
                        log.warn("teacher writing.graded handle err={}", ex.toString());
                        writingConsumer.negativeAcknowledge(msg);
                    }
                }
            } catch (Exception ex) {
                log.warn("teacher writing.graded consumer start/loop failed: {}", ex.toString());
            }
        });
    }

    private PulsarClient ensureClient() {
        PulsarClient existing = holder.get();
        if (existing != null) return existing;
        try {
            PulsarClient client = PulsarClient.builder()
                    .serviceUrl(serviceUrl)
                    .connectionTimeout(5, TimeUnit.SECONDS)
                    .build();
            holder.set(client);
            log.info("pulsar client connected (teacher), serviceUrl={}", serviceUrl);
            return client;
        } catch (Exception ex) {
            log.warn("pulsar client connect failed (teacher consumer disabled): {}", ex.toString());
            return null;
        }
    }

    @PreDestroy
    public void close() {
        running = false;
        try { if (consumer != null) consumer.close(); } catch (Exception ignored) {}
        try { if (writingConsumer != null) writingConsumer.close(); } catch (Exception ignored) {}
    }
}
