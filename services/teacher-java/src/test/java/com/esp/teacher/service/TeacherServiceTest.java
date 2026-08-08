package com.esp.teacher.service;

import org.junit.jupiter.api.Test;

import java.math.BigDecimal;

import static org.junit.jupiter.api.Assertions.*;

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
