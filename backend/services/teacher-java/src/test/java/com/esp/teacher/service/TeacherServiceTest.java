package com.esp.teacher.service;

import com.esp.cache.CacheService;
import com.esp.cache.event.EventPublisher;
import com.esp.cache.event.TeacherFinalizedEvent;
import com.esp.teacher.domain.Submission;
import com.esp.teacher.repository.SubmissionRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.math.BigDecimal;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

class LessonPlanGeneratorTest {

    private final LessonPlanGenerator gen = new LessonPlanGenerator();

    @Test
    void generatesCompleteLessonPlan() {
        var plan = gen.generate("环保", "B2");
        assertNotNull(plan.passage());
        assertTrue(plan.exercises().size() >= 3, "exercises >= 3");
        assertFalse(plan.games().isEmpty(), "games not empty");
        assertTrue(plan.discussion().size() >= 2, "discussion >= 2");
        assertTrue(plan.passage().contains("环保"));
    }

    @Test
    void defaultsTopicWhenBlank() {
        var plan = gen.generate("  ", "");
        assertTrue(plan.passage().contains("Environment"));
    }
}

class TeacherServiceScoringTest {

    @Test
    void scoreContent_emptyIsZero() {
        assertEquals(BigDecimal.ZERO, TeacherService.scoreContent(""));
        assertEquals(BigDecimal.ZERO, TeacherService.scoreContent(null));
    }

    @Test
    void scoreContent_withLinkingWordsGetsHigher() {
        BigDecimal plain = TeacherService.scoreContent("I like cats. They are nice. I have one.");
        BigDecimal linked = TeacherService.scoreContent("I like cats. However, they are noisy. Therefore I keep them indoors. Because.");
        assertTrue(linked.compareTo(plain) > 0, "linking words should raise score");
    }

    @Test
    void feedbackFor_shortContentSuggestsDevelopment() {
        String fb = TeacherService.feedbackFor("short.");
        assertTrue(fb.toLowerCase().contains("short") || fb.toLowerCase().contains("develop"));
    }

    @Test
    void feedbackFor_missingLinkingWordsSuggestsThem() {
        String fb = TeacherService.feedbackFor("This is a longer paragraph with enough words but no transition words at all here.");
        assertTrue(fb.toLowerCase().contains("linking"));
    }
}

/**
 * TeacherService 单元测试:doAutoGrade / finalize 业务逻辑。
 * Mock 所有外部依赖(SubmissionRepository / CacheService / EventPublisher)。
 */
class TeacherServiceUnitTest {

    private SubmissionRepository submissionRepo;
    private CacheService cacheService;
    private EventPublisher eventPublisher;
    private TeacherService service;

    @BeforeEach
    void setUp() {
        submissionRepo = mock(SubmissionRepository.class);
        cacheService = mock(CacheService.class);
        eventPublisher = mock(EventPublisher.class);
        // 其余依赖不需要精确 mock,用 nice mock 即可
        LessonPlanGenerator generator = new LessonPlanGenerator();
        var learningRpc = mock(com.esp.api.dubbo.LearningRpcService.class);
        var saga = mock(TeacherSummarySaga.class);
        service = new TeacherService(generator, submissionRepo, learningRpc, cacheService, eventPublisher, saga);
    }

    @Test
    void doAutoGrade_scoresAndSavesPendingSubmissions() {
        Submission s1 = new Submission();
        s1.setId(1L);
        s1.setStatus(Submission.STATUS_PENDING);
        s1.setContent("I like cats. However they are noisy. Therefore I keep them indoors. Because.");
        Submission s2 = new Submission();
        s2.setId(2L);
        s2.setStatus(Submission.STATUS_PENDING);
        s2.setContent("Short.");

        when(submissionRepo.findByAssignmentId(10L)).thenReturn(List.of(s1, s2));

        int graded = service.doAutoGrade(10L, 5L);

        assertEquals(2, graded, "两个 pending submission 都应被评阅");
        verify(submissionRepo, times(2)).save(any(Submission.class));
        // classId=5 时应失效班级缓存
        verify(cacheService).evict("teacher:class-weak", "teacher:class-weak:5");
    }

    @Test
    void doAutoGrade_skipsAlreadyGraded() {
        Submission graded = new Submission();
        graded.setId(3L);
        graded.setStatus(Submission.STATUS_GRADED);
        graded.setContent("Already graded content with enough words.");

        when(submissionRepo.findByAssignmentId(20L)).thenReturn(List.of(graded));

        int result = service.doAutoGrade(20L, null);

        assertEquals(0, result, "已评阅的 submission 应被跳过");
        verify(submissionRepo, never()).save(any());
        verify(cacheService, never()).evict(anyString(), anyString());
    }

    @Test
    void doAutoGrade_emptyList_returnsZero() {
        when(submissionRepo.findByAssignmentId(99L)).thenReturn(List.of());
        assertEquals(0, service.doAutoGrade(99L, null));
    }

    @Test
    void finalize_savesAndPublishesEvent() {
        Submission s = new Submission();
        s.setId(7L);
        s.setUserId(42L);
        s.setAssignmentId(10L);
        s.setStatus(Submission.STATUS_GRADED);
        when(submissionRepo.findById(7L)).thenReturn(java.util.Optional.of(s));
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(true);

        service.finalize(7L, BigDecimal.valueOf(88), 5L);

        assertEquals(Submission.STATUS_FINALIZED, s.getStatus());
        assertEquals(BigDecimal.valueOf(88), s.getFinalScore());
        verify(submissionRepo).save(s);
        verify(cacheService).evict("teacher:class-weak", "teacher:class-weak:5");
        verify(eventPublisher).publish(eq("teacher.finalized"), any(TeacherFinalizedEvent.class));
    }

    @Test
    void finalize_idempotent_skipsReplay() {
        when(cacheService.tryIdempotent(anyString(), anyLong())).thenReturn(false);

        service.finalize(7L, BigDecimal.valueOf(99), 5L);

        verify(submissionRepo, never()).findById(anyLong());
        verify(submissionRepo, never()).save(any());
        verify(eventPublisher, never()).publish(anyString(), any());
    }
}
