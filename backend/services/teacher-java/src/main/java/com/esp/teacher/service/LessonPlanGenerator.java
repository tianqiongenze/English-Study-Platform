package com.esp.teacher.service;

import org.springframework.stereotype.Component;

/**
 * 教案生成器:基于主题确定性生成结构化教案(无 LLM 依赖,保证可测可压)。
 * 真实环境可替换为调用 writing-py 的 LLM 生成。
 */
@Component
public class LessonPlanGenerator {

    /**
     * 按 FR-4.1 生成教案:passage + exercises(>=3) + games(>=1) + discussion(>=2)。
     */
    public LessonPlan generate(String topic, String level) {
        String t = topic == null || topic.isBlank() ? "Environment" : topic.trim();
        String lvl = level == null || level.isBlank() ? "B1" : level.trim();
        return new LessonPlan(
                "Passage about " + t + " (level " + lvl + "). The text introduces key vocabulary and ideas.",
                java.util.List.of(
                        "Match these words about " + t + " with their definitions.",
                        "Fill in the blanks using target vocabulary from the passage.",
                        "Write a short paragraph (50 words) about " + t + ".",
                        "Choose the best summary of the passage."
                ),
                java.util.List.of(
                        "Vocabulary Bingo: students mark target words as they hear them."
                ),
                java.util.List.of(
                        "Why is " + t + " important in your daily life?",
                        "What can individuals do to help with " + t + "?",
                        "Debate: government vs. personal responsibility for " + t + "."
                )
        );
    }

    public record LessonPlan(String passage, java.util.List<String> exercises,
                             java.util.List<String> games, java.util.List<String> discussion) {}
}
