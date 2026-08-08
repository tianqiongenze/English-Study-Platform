package com.esp.cache.event;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

class EventPojosTest {

    @Test
    void autoGradeRequestedEvent() {
        AutoGradeRequestedEvent e = new AutoGradeRequestedEvent();
        e.setAssignmentId(7L);
        e.setClassId(42L);
        assertEquals(7L, e.getAssignmentId());
        assertEquals(42L, e.getClassId());
        AutoGradeRequestedEvent e2 = new AutoGradeRequestedEvent(1L, 2L);
        assertEquals(1L, e2.getAssignmentId());
        assertEquals(2L, e2.getClassId());
        e2.setClassId(null);
        assertNull(e2.getClassId());
    }

    @Test
    void sessionEndedEvent() {
        SessionEndedEvent e = new SessionEndedEvent();
        e.setSessionId("s1");
        e.setUserId(9L);
        assertEquals("s1", e.getSessionId());
        assertEquals(9L, e.getUserId());
    }

    @Test
    void teacherFinalizedEvent() {
        TeacherFinalizedEvent e = new TeacherFinalizedEvent();
        e.setUserId(1L);
        e.setKpId(2L);
        e.setFinalScore(0.95);
        assertEquals(1L, e.getUserId());
        assertEquals(2L, e.getKpId());
        assertEquals(0.95, e.getFinalScore(), 1e-9);
        TeacherFinalizedEvent e2 = new TeacherFinalizedEvent(3L, 4L, 0.5);
        assertEquals(3L, e2.getUserId());
        e2.setKpId(null);
        assertNull(e2.getKpId());
    }

    @Test
    void userRegisteredEvent() {
        UserRegisteredEvent e = new UserRegisteredEvent(1L, "a@b.c", "student");
        assertEquals(1L, e.getUserId());
        assertEquals("a@b.c", e.getEmail());
        assertEquals("student", e.getRole());
        e.setEmail("x@y.z");
        e.setRole("teacher");
        assertEquals("x@y.z", e.getEmail());
        assertEquals("teacher", e.getRole());
        // 用 no-arg 构造 + setter 覆盖所有分支
        UserRegisteredEvent e2 = new UserRegisteredEvent();
        e2.setUserId(5L);
        assertEquals(5L, e2.getUserId());
    }

    @Test
    void writingGradedEvent() {
        WritingGradedEvent e = new WritingGradedEvent();
        e.setStatus("done");
        e.setThreadId("t1");
        assertEquals("done", e.getStatus());
        assertEquals("t1", e.getThreadId());
    }

    @Test
    void eventTopicsConstants() {
        assertEquals("user.registered", EventTopics.USER_REGISTERED);
        assertEquals("teacher.finalized", EventTopics.TEACHER_FINALIZED);
        assertEquals("teacher.autograde.requested", EventTopics.AUTO_GRADE_REQUESTED);
        assertEquals("session.ended", EventTopics.SESSION_ENDED);
        assertEquals("writing.graded", EventTopics.WRITING_GRADED);
    }
}
