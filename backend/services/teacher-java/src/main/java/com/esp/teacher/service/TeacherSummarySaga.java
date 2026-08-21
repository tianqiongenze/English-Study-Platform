package com.esp.teacher.service;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.TeacherFinalizedEvent;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import com.esp.teacher.domain.Submission;
import com.esp.teacher.repository.SubmissionRepository;
import com.esp.teacher.repository.TeacherEventOutboxRepository;
import com.esp.teacher.domain.TeacherEventOutbox;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.context.annotation.Lazy;
import org.springframework.orm.ObjectOptimisticLockingFailureException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.math.BigDecimal;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.ThreadLocalRandom;
import java.util.function.Supplier;

/**
 * 教师学情汇总 Saga(应用层补偿事务)。
 *
 * <p>从 Seata AT(@GlobalTransactional)迁移而来,解锁 schema-per-service Phase1:
 * AT 模式依赖每个参与方本地 undo_log 表(同库),拆 schema 后 teacher/learning 各处独立
 * schema,undo_log 跨 schema 路由不可行,且 learning 是 Dubbo 纯读不持有本地写,
 * AT 全局锁反而是冗余开销。改用应用层 Saga(本地消息表 + 补偿)更契合场景:
 * <ul>
 *   <li>步骤1(可补偿):本地写 finalize(终审 submission)—— 记录前置状态用于补偿回滚。</li>
 *   <li>步骤2(可补偿/可重试):Dubbo 读 learning userStats(纯读,无副作用,无需补偿)。</li>
 *   <li>补偿动作:任何异常 → 把 submission 回滚到前置状态(status + finalScore),
 *       并发 teacher.finalized.compensation 事件告知下游(learning 是纯读消费,幂等由 P1-4 保证)。</li>
 * </ul>
 *
 * <p>不引入 Seata Saga 状态机:teacher 此处仅 1 步可补偿写 + 1 步纯读,状态机是过度设计;
 * 本地消息表 + try/compensate 足够,工业级务实选择。详见
 * {@code docs/design/adr-seata-at-to-saga.md}。
 */
@Service
public class TeacherSummarySaga {

    private static final Logger log = LoggerFactory.getLogger(TeacherSummarySaga.class);

    /** 补偿事件 topic:teacher.finalized 的补偿信号,下游可选消费(learning 纯读,幂等由 P1-4 保证)。 */
    public static final String TEACHER_FINALIZED_COMPENSATION = "teacher.finalized.compensation";

    private final SubmissionRepository submissionRepo;
    private final LearningRpcService learningRpc;
    private final CacheService cacheService;
    private final EventPublisher eventPublisher;
    private final TeacherEventOutboxRepository outboxRepo;

    public TeacherSummarySaga(SubmissionRepository submissionRepo, LearningRpcService learningRpc,
                              CacheService cacheService, @Lazy EventPublisher eventPublisher,
                              TeacherEventOutboxRepository outboxRepo) {
        this.submissionRepo = submissionRepo;
        this.learningRpc = learningRpc;
        this.cacheService = cacheService;
        this.eventPublisher = eventPublisher;
        this.outboxRepo = outboxRepo;
    }

    /**
     * 执行学情汇总 Saga。
     *
     * <p>本地事务包裹步骤1(本地写 finalize);Dubbo 读放在本地事务提交之后(纯读无副作用,
     * 不参与本地事务,避免 DB 连接长时间持有)。若 Dubbo 读失败,触发步骤1补偿。
     *
     * @return weakKp 数(learning userStats.size);补偿发生时返回 0
     */
    public int run(Long userId, Long submissionId, BigDecimal finalScore, Long classId) {
        log.info("saga begin: userId={}, submissionId={}, finalScore={}, classId={}",
                userId, submissionId, finalScore, classId);

        // 步骤1:本地写 finalize(本地事务内),记录前置状态供补偿。
        // P1-D3 OCC(2026-08-21):tryFinalize 是 Submission(@Version)的 read-modify-write,
        // 并发终审同一 submission 时 Hibernate 抛 ObjectOptimisticLockingFailureException。
        // 按 OCC 铁律:重读主库(@DataSource WRITE)→重算 confirm→重 save,指数退避 + jitter,
        // 上限 3 次;耗尽仍冲突则放弃 finalize,转抛 ConflictException(由调用方映射 409),
        // 不无限循环(原 Pulsar nack 重投会无限重试撞锁)。幂等命中(snapshot==null)直接重试无副作用。
        Snapshot snapshot = retryOnConflict("tryFinalize", 3, () -> tryFinalize(submissionId, finalScore, classId));
        // snapshot == null 表示幂等命中(已终审),直接返回 0
        if (snapshot == null) {
            log.info("saga idempotent skip: submissionId={} already finalized", submissionId);
            return 0;
        }

        // 步骤2:Dubbo 读 learning userStats(纯读,无副作用,无需补偿)
        // 放在本地事务提交后执行,避免 DB 连接持有过久;失败则补偿步骤1。
        List<KpStat> stats;
        try {
            stats = learningRpc.userStats(userId);
        } catch (RuntimeException e) {
            log.error("saga step2 learningRpc failed, compensating step1: userId={}, submissionId={}",
                    userId, submissionId, e);
            // P1-D3 OCC:补偿同样是 read-modify-write(findById→rollbackStatus→save),并发冲突同样重试。
            retryOnConflict("compensate", 3, () -> {
                compensate(submissionId, snapshot, userId);
                return null;
            });
            throw e;
        }
        log.info("saga commit: userId={}, weakKp={}, finalized={}", userId, stats.size(), submissionId);
        return stats.size();
    }

    /**
     * 步骤1(可补偿):本地事务写 finalize。
     *
     * <p>幂等:SETNX esp:idem:teacher:summary:&lt;submissionId&gt; TTL 600s;命中返回 null(跳过)。
     * 记录前置状态(status + finalScore)用于补偿。
     *
     * @return submission 前置快照(用于补偿);幂等命中返回 null
     */
    @DataSource(DataSourceType.WRITE)
    @Transactional(rollbackFor = Exception.class)
    public Snapshot tryFinalize(Long submissionId, BigDecimal finalScore, Long classId) {
        // 幂等:SETNX esp:idem:teacher:summary:<submissionId> TTL 600s;命中则跳过
        String idemKey = "esp:idem:teacher:summary:" + submissionId;
        if (!cacheService.tryIdempotent(idemKey, 600)) {
            log.info("tryFinalize idempotent replay skipped: submissionId={}", submissionId);
            return null;
        }
        Submission s = submissionRepo.findById(submissionId)
                .orElseThrow(() -> new IllegalArgumentException("submission not found: " + submissionId));
        // 记录前置状态(供补偿回滚)
        String prevStatus = s.getStatus();
        BigDecimal prevFinalScore = s.getFinalScore();

        s.confirm(finalScore);
        submissionRepo.save(s);
        if (classId != null) {
            cacheService.evict("teacher:class-weak", "teacher:class-weak:" + classId);
        }
        // P0-D1(2026-08-21):teacher.finalized 改 transactional outbox(本地原子写 + relay 异步重投)。
        // 原实现 eventPublisher.publish 在 @Transactional 内 sendAsync:① 事务回滚但消息已发=幽灵事件;
        // ② sendAsync 失败只 warn 丢消息,mastery 评分永久缺失;③ broker 不可达静默丢。
        // 正解(用户 2026-08-21 确认走 DB outbox 统一路线):事务内同 esp_teacher schema 写 outbox 行
        // (与 submission 同库同事务原子),由独立 OutboxRelay 扫表重投 Pulsar(幂等键 submissionId)。
        // broker 挂时 outbox 行留存,relay 兜底重投;事务回滚则 outbox 行随之回滚(无幽灵事件)。
        // 与 content-go outbox_relay.go 同模式,统一 outbox(schema-per-service 各自带 relay)。
        TeacherFinalizedEvent evt = new TeacherFinalizedEvent(s.getUserId(), s.getAssignmentId(), finalScore.doubleValue());
        String messageKey = "teacher:finalized:" + submissionId;
        outboxRepo.save(TeacherEventOutbox.pending(
                "teacher.finalized", messageKey, s.getUserId(), evt));
        log.info("teacher.finalized enqueued to local outbox: submissionId={}, userId={}, kpId={}, score={}",
                submissionId, s.getUserId(), s.getAssignmentId(), finalScore);

        // 返回前置快照(本地事务提交后,Spring 会把返回值交给调用方;补偿时用)
        return new Snapshot(submissionId, prevStatus, prevFinalScore, s.getUserId(), s.getAssignmentId());
    }

    /**
     * 补偿动作:把 submission 回滚到前置状态(status + finalScore),并发补偿事件。
     *
     * <p>幂等:补偿前检查 submission 是否还在 finalized 状态(已被补偿则跳过),防止重复补偿。
     * 补偿失败仅记 ERROR 日志 + 告警(后续人工介入),不再向上抛——避免补偿链无限循环。
     *
     * <p>补偿在独立本地事务中执行(不与 tryFinalize 同事务:tryFinalize 已提交,补偿是新事务)。
     */
    @DataSource(DataSourceType.WRITE)
    @Transactional(rollbackFor = Exception.class)
    public void compensate(Long submissionId, Snapshot snapshot, Long userId) {
        Submission s = submissionRepo.findById(submissionId).orElse(null);
        if (s == null) {
            log.warn("compensate: submission not found, skip: submissionId={}", submissionId);
            return;
        }
        // 幂等:仅当仍为 finalized 时才回滚(已被补偿则跳过)
        if (!"finalized".equals(s.getStatus())) {
            log.info("compensate: submission not in finalized state, skip idempotently: submissionId={}, status={}",
                    submissionId, s.getStatus());
            return;
        }
        s.rollbackStatus(snapshot.prevStatus, snapshot.prevFinalScore);
        submissionRepo.save(s);
        // 失效幂等占位,允许下次重试(占位已被消费,可重新执行)
        cacheService.evict("esp:idem", "esp:idem:teacher:summary:" + submissionId);
        // 发补偿事件:告知下游已回滚(learning 纯读消费,幂等由 P1-4 保证;此处仅为信号)
        eventPublisher.publish(TEACHER_FINALIZED_COMPENSATION,
                new TeacherFinalizedEvent(userId, snapshot.assignmentId, 0));
        log.warn("compensate done: submissionId={}, restored status={}, finalScore={}",
                submissionId, snapshot.prevStatus, snapshot.prevFinalScore);
    }

    /**
     * P1-D3 OCC(2026-08-21):乐观锁冲突重试包装。
     *
     * <p>Submission(@Version) read-modify-write 在并发终审/补偿时,Hibernate flush 的 UPDATE
     * 追加 WHERE version=?,并发冲突返 0 行抛 {@link ObjectOptimisticLockingFailureException}。
     * 按 OCC 铁律:捕获后重读主库(被包装方法内部 {@code findById} 已是最新主库读,
     * 因 @DataSource WRITE 走主库 strong read)→重算→重 save;指数退避 + jitter(20ms→40ms→80ms
     * 上下浮动)防惊群,上限 maxAttempts 次;耗尽仍冲突抛 {@link ConflictException} 由上层映射 409,
     * 不无限循环(违反 BRR/CLC 铁律的"裸重试无上限")。
     *
     * @param step         步骤名(日志用)
     * @param maxAttempts  最大尝试次数(含首次,建议 3)
     * @param action        返回非空快照或 null(幂等命中);返回值被首次成功的值采用
     * @param <T>           返回类型
     * @return 首次成功执行的返回值
     * @throws ConflictException 重试耗尽仍冲突
     */
    private <T> T retryOnConflict(String step, int maxAttempts, Supplier<T> action) {
        ObjectOptimisticLockingFailureException last = null;
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                return action.get();
            } catch (ObjectOptimisticLockingFailureException e) {
                last = e;
                if (attempt >= maxAttempts) {
                    break;
                }
                long backoff = (20L * (1L << (attempt - 1))) + ThreadLocalRandom.current().nextLong(0, 20);
                log.warn("saga {} optimistic lock conflict, retry {}/{} after {}ms: {}",
                        step, attempt, maxAttempts, backoff, e.toString());
                try {
                    Thread.sleep(backoff);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    throw new ConflictException("saga " + step + " interrupted during OCC backoff", ie);
                }
            }
        }
        log.error("saga {} optimistic lock conflict exhausted after {} attempts, giving up", step, maxAttempts, last);
        throw new ConflictException("saga " + step + " optimistic lock conflict after " + maxAttempts + " attempts", last);
    }

    /** P1-D3 OCC:乐观锁重试耗尽,转 409 语义(交上层映射)。 */
    public static class ConflictException extends RuntimeException {
        public ConflictException(String message, Throwable cause) { super(message, cause); }
    }

    /** Saga 前置快照(供补偿回滚)。独立静态内部类避免污染 Submission 实体。 */
    public static final class Snapshot extends Submission {
        final Long submissionId;
        final String prevStatus;
        final BigDecimal prevFinalScore;
        final Long assignmentId;

        Snapshot(Long submissionId, String prevStatus, BigDecimal prevFinalScore,
                 Long userId, Long assignmentId) {
            this.submissionId = submissionId;
            this.prevStatus = prevStatus;
            this.prevFinalScore = prevFinalScore;
            this.assignmentId = assignmentId;
            // Submission 透传字段(便于补偿事件回填)
            setUserId(userId);
            setAssignmentId(assignmentId);
        }
    }
}
