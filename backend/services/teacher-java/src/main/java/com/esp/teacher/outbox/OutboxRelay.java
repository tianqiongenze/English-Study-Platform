package com.esp.teacher.outbox;

import com.esp.cache.event.EventPublishException;
import com.esp.cache.event.EventPublisher;
import com.esp.teacher.domain.TeacherEventOutbox;
import com.esp.teacher.repository.TeacherEventOutboxRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.Instant;
import java.util.List;

/**
 * 教师事件 outbox 重投器(P0-D1,2026-08-21 transactional outbox)。
 *
 * <p>每 2s 扫 {@link TeacherEventOutbox} 的 pending/failed 行,用
 * {@link EventPublisher#publishReliable} 同步发送(失败抛 {@link EventPublishException})。
 * 发送成功 → done;失败 → 指数退避(next_retry_at 后延),retry_count>=8 置 failed(死信)。
 *
 * <p>幂等:发送用 message_id 作幂等键,下游 Pulsar consumer 对 message_id 幂等去重
 * (learning-java user_mastery upsert 幂等)。broker 不可达时行留存下次扫到再投,不丢。
 *
 * <p>对照 content-go 的 outbox_relay.go 同模式(统一 outbox:schema-per-service 各自带 relay)。
 * 与原 EventPublisher.publish(sendAsync 静默丢)的区别:本 relay 用 publishReliable(同步+失败感知),
 * 失败行退避重投而非静默丢——P0-D1 核心。
 *
 * <p>事务边界:每次 drain 在独立事务(与 tryFinalize 事务解耦——outbox 行已提交才能被扫到)。
 * 扫 + 投 + 置 done 同事务保证 relay 自身一致性(投 Pulsar 的副作用在事务外,若事务回滚则行
 * 仍是 pending 下次重投——幂等键保护下游不会被重复处理)。
 */
@Component
public class OutboxRelay {

    private static final Logger log = LoggerFactory.getLogger(OutboxRelay.class);

    private static final int BATCH = 200;
    private static final int MAX_RETRY = 8;

    private final TeacherEventOutboxRepository outboxRepo;
    private final EventPublisher eventPublisher;

    public OutboxRelay(TeacherEventOutboxRepository outboxRepo, EventPublisher eventPublisher) {
        this.outboxRepo = outboxRepo;
        this.eventPublisher = eventPublisher;
    }

    @Scheduled(fixedDelay = 2000) // 每 2s 扫一次(与 content-go relayInterval 对齐)
    @Transactional // 默认 TM(用 @Primary RoutingDataSource 走写池;@DataSource(WRITE) 在调用方标记)
    public void drain() {
        List<TeacherEventOutbox> batch;
        try {
            batch = outboxRepo.findPending(Instant.now());
        } catch (Exception e) {
            log.warn("outbox relay poll err: {}", e.toString());
            return;
        }
        if (batch == null || batch.isEmpty()) return;

        for (TeacherEventOutbox row : batch) {
            try {
                // payload 已是 JSON 字符串,直接作 Pulsar 消息体投递(保留 message_id 幂等键)
                eventPublisher.publishReliable(row.getEventType(), row.getPayload(), row.getMessageId());
                row.setStatus("done");
                outboxRepo.save(row);
            } catch (EventPublishException ex) {
                int nextRetry = row.getRetryCount() + 1;
                if (nextRetry >= MAX_RETRY) {
                    row.setStatus("failed");
                    row.setRetryCount(nextRetry);
                    outboxRepo.save(row);
                    log.error("outbox relay mark-dead id={} msgId={} after {} retries: {}",
                            row.getId(), row.getMessageId(), nextRetry, ex.getMessage());
                } else {
                    row.setRetryCount(nextRetry);
                    row.setNextRetryAt(Instant.now().plus(Duration.ofSeconds(backoffSec(nextRetry))));
                    outboxRepo.save(row);
                    if (log.isDebugEnabled()) {
                        log.debug("outbox relay backoff id={} msgId={} retry={} err={}",
                                row.getId(), row.getMessageId(), nextRetry, ex.getMessage());
                    }
                }
            }
        }
    }

    /** 指数退避秒(与 content-go relayBackoffSec 对齐:1/5/30/300)。 */
    private int backoffSec(int retry) {
        switch (retry) {
            case 1: return 1;
            case 2: return 5;
            case 3: return 30;
            default: return 300;
        }
    }
}
