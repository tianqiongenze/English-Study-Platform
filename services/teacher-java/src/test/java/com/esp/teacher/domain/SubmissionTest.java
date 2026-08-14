package com.esp.teacher.domain;

import org.junit.jupiter.api.Test;

import java.math.BigDecimal;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Submission 聚合根行为测试(P2-13 充血化):grade/confirm/rollbackStatus 状态机闭环。
 */
class SubmissionTest {

    @Test
    void grade_setsAutoScoreFeedbackAndGradedStatus() {
        Submission s = new Submission();
        s.setStatus(Submission.STATUS_PENDING);

        s.grade(BigDecimal.valueOf(85), "good");

        assertEquals(BigDecimal.valueOf(85), s.getAutoScore());
        assertEquals("good", s.getAutoFeedback());
        assertEquals(Submission.STATUS_GRADED, s.getStatus());
    }

    @Test
    void confirm_setsFinalScoreAndFinalizedStatus() {
        Submission s = new Submission();
        s.setStatus(Submission.STATUS_GRADED);

        s.confirm(BigDecimal.valueOf(90));

        assertEquals(BigDecimal.valueOf(90), s.getFinalScore());
        assertEquals(Submission.STATUS_FINALIZED, s.getStatus());
    }

    @Test
    void confirm_isIdempotent_whenAlreadyFinalized() {
        Submission s = new Submission();
        s.setStatus(Submission.STATUS_FINALIZED);
        s.setFinalScore(BigDecimal.valueOf(90));

        s.confirm(BigDecimal.valueOf(99)); // 重复终评应被忽略

        assertEquals(BigDecimal.valueOf(90), s.getFinalScore());
        assertEquals(Submission.STATUS_FINALIZED, s.getStatus());
    }

    @Test
    void rollbackStatus_restoresPreviousState() {
        Submission s = new Submission();
        s.setStatus(Submission.STATUS_FINALIZED);
        s.setFinalScore(BigDecimal.valueOf(90));

        s.rollbackStatus(Submission.STATUS_GRADED, null);

        assertEquals(Submission.STATUS_GRADED, s.getStatus());
        assertNull(s.getFinalScore());
    }
}
