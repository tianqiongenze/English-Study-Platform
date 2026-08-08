package com.esp.teacher.service;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.cache.CacheService;
import com.esp.cache.event.AutoGradeRequestedEvent;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.EventTopics;
import com.esp.cache.event.TeacherFinalizedEvent;
import com.esp.teacher.domain.Submission;
import com.esp.rw.DataSource;
import com.esp.rw.DataSourceType;
import com.esp.teacher.repository.SubmissionRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.context.annotation.Lazy;
import org.springframework.stereotype.Service;

import java.math.BigDecimal;
import java.util.List;

@Service
public class TeacherService {

    private static final Logger log = LoggerFactory.getLogger(TeacherService.class);

    private final LessonPlanGenerator lessonPlanGenerator;
    private final SubmissionRepository submissionRepo;
    private final LearningRpcService learningRpc;
    private final CacheService cacheService;
    private final EventPublisher eventPublisher;
    private final TeacherSummarySaga teacherSummarySaga;

    public TeacherService(LessonPlanGenerator lessonPlanGenerator, SubmissionRepository submissionRepo,
                          LearningRpcService learningRpc, CacheService cacheService,
                          @Lazy EventPublisher eventPublisher, TeacherSummarySaga teacherSummarySaga) {
        this.lessonPlanGenerator = lessonPlanGenerator;
        this.submissionRepo = submissionRepo;
        this.learningRpc = learningRpc;
        this.cacheService = cacheService;
        this.eventPublisher = eventPublisher;
        this.teacherSummarySaga = teacherSummarySaga;
    }

    /** FR-4.1 一键生成教案(纯生成,不缓存)。 */
    public LessonPlanGenerator.LessonPlan generateLessonPlan(String topic, String level) {
        return lessonPlanGenerator.generate(topic, level);
    }

    /** FR-4.2 班级共性难题(走 TiFlash 列存 OLAP,读副本)。热点读:同一班级短时间重复查询 → L1+L2 缓存。 */
    @DataSource(value = DataSourceType.READ, tiflash = true)
    public List<Object[]> classAnalytics(Long classId) {
        return cacheService.getOrLoad("teacher:class-weak", "teacher:class-weak:" + classId,
                () -> submissionRepo.classWeakness(classId), 600, true);
    }

    /**
     * FR-4.3 作业智能评阅:削峰重构 —— 只发消息立即返回,
     * 异步评阅由 {@link TeacherEventConsumer} consumer 执行(解耦故障域 + 避免长事务阻塞 HTTP)。
     */
    @DataSource(DataSourceType.WRITE)
    public int autoGrade(Long assignmentId, Long classId) {
        eventPublisher.publish(EventTopics.AUTO_GRADE_REQUESTED,
                new AutoGradeRequestedEvent(assignmentId, classId));
        return 0; // 返回 0 表示异步入队成功;实际数量由 consumer 执行后日志输出
    }

    /**
     * 实际评阅逻辑(原 autoGrade 主体):由 consumer 调用,同步执行评阅循环。
     */
    @DataSource(DataSourceType.WRITE)
    public int doAutoGrade(Long assignmentId, Long classId) {
        List<Submission> subs = submissionRepo.findByAssignmentId(assignmentId);
        int graded = 0;
        for (Submission s : subs) {
            if (!"auto_graded".equals(s.getStatus())) {
                s.setAutoScore(scoreContent(s.getContent()));
                s.setAutoFeedback(feedbackFor(s.getContent()));
                s.setStatus("auto_graded");
                submissionRepo.save(s);
                graded++;
            }
        }
        // 评阅改变 submission.auto_score,班级共性难题聚合结果失效
        if (classId != null) {
            cacheService.evict("teacher:class-weak", "teacher:class-weak:" + classId);
        }
        log.info("teacher autoGrade done: assignmentId={}, classId={}, graded={}", assignmentId, classId, graded);
        return graded;
    }

    /** 教师终审:写入 final_score,status -> finalized + 发事件。写操作:该班级共性难题缓存失效。
     *  幂等:SETNX esp:idem:teacher:finalize:&lt;submissionId&gt; TTL 600s,同 submissionId 重复终审被拦截
     *  (复用 CacheService.tryIdempotent,与 summarizeStudentLearning 同款机制)。 */
    @DataSource(DataSourceType.WRITE)
    public void finalize(Long submissionId, BigDecimal finalScore, Long classId) {
        // 幂等占位:同 submissionId 重复终审(并发重试/客户端重放)直接跳过,避免覆盖已有终审分数
        String idemKey = "esp:idem:teacher:finalize:" + submissionId;
        if (!cacheService.tryIdempotent(idemKey, 600)) {
            log.info("finalize idempotent replay skipped: submissionId={} (already finalized)", submissionId);
            return;
        }
        Submission s = submissionRepo.findById(submissionId)
                .orElseThrow(() -> new IllegalArgumentException("submission not found: " + submissionId));
        s.setFinalScore(finalScore);
        s.setStatus("finalized");
        submissionRepo.save(s);
        if (classId != null) {
            cacheService.evict("teacher:class-weak", "teacher:class-weak:" + classId);
        }
        // EDA:发 teacher.finalized 事件 → learning-java 更新 mastery 分数
        eventPublisher.publish(EventTopics.TEACHER_FINALIZED,
                new TeacherFinalizedEvent(s.getUserId(), s.getAssignmentId(), finalScore.doubleValue()));
        log.info("teacher.finalized published: userId={}, assignmentId(as kpId)={}, score={}",
                s.getUserId(), s.getAssignmentId(), finalScore);
    }

    /**
     * FR-4 学情汇总(应用层 Saga 补偿事务):跨 teacher -> learning-java 拉薄弱知识点 +
     * 本地写终审。已从 Seata AT(@GlobalTransactional)迁移到应用层 Saga
     * (见 {@link TeacherSummarySaga}),解锁 schema-per-service Phase1
     * (undo_log 跨 schema 阻塞;learning 是 Dubbo 纯读,AT 全局锁冗余)。
     *
     * <p>步骤1(可补偿):本地写 finalize,记录前置状态供补偿。
     * 步骤2(可补偿/可重试):Dubbo 读 learning userStats(纯读,无副作用)。
     * 补偿:任何异常 → 回滚 submission 到前置状态 + 发 teacher.finalized.compensation 事件。
     *
     * <p>幂等:SETNX esp:idem:teacher:summary:&lt;submissionId&gt; TTL 600s(在 saga 内)。
     *
     * @return weakKp 数(learning userStats.size);幂等命中或补偿发生时返回 0
     */
    @DataSource(DataSourceType.WRITE)
    public int summarizeStudentLearning(Long userId, Long submissionId, BigDecimal finalScore, Long classId) {
        return teacherSummarySaga.run(userId, submissionId, finalScore, classId);
    }

    /** 评分:长度 + 句点 + 关键词命中(0~100)。 */
    static BigDecimal scoreContent(String content) {
        if (content == null || content.isBlank()) return BigDecimal.ZERO;
        int len = content.trim().length();
        long sentences = content.chars().filter(c -> c == '.' || c == '!' || c == '?').count();
        int lenPts = Math.min(40, len / 3);            // 最多 40
        int sentPts = (int) Math.min(30, sentences * 10); // 最多 30
        int kwPts = content.toLowerCase().matches(".*(however|therefore|because|although|moreover).*") ? 30 : 0;
        return BigDecimal.valueOf(Math.min(100, lenPts + sentPts + kwPts));
    }

    static String feedbackFor(String content) {
        if (content == null || content.isBlank()) return "Empty submission.";
        if (content.length() < 50) return "Too short; develop your ideas further.";
        if (!content.toLowerCase().matches(".*(however|therefore|because|although).*")) {
            return "Add linking words (however, therefore, because) to improve coherence.";
        }
        return "Well-structured. Check grammar and vocabulary variety.";
    }
}
