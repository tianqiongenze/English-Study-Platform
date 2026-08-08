package com.esp.learning.web;

import com.esp.learning.service.LearningService;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;

@RestController
@RequestMapping("/api/learning")
public class LearningController {

    private final LearningService learningService;

    public LearningController(LearningService learningService) {
        this.learningService = learningService;
    }

    /** FR-2.4 学情事件入口(异步,202)。P1-6 断点3:优先读 X-User-Id header。 */
    @PostMapping("/event")
    public ResponseEntity<Map<String, Object>> reportEvent(
            @RequestHeader(value = "X-User-Id", required = false) Long headerUserId,
            @RequestBody Map<String, Object> body) {
        long userId = headerUserId != null ? headerUserId : toLong(body.getOrDefault("user_id", 1), 1L);
        String type = (String) body.getOrDefault("event_type", "answer");
        String payload = body.containsKey("payload") ? body.get("payload").toString() : "{}";
        boolean ok = learningService.reportEvent(userId, type, payload);
        int status = ok ? 202 : 503;
        return ResponseEntity.status(status).body(Map.of("accepted", ok));
    }

    /** FR-2.1 初测基线。P1-6 断点3:优先读 X-User-Id header。 */
    @PostMapping("/placement")
    public ResponseEntity<Map<String, Object>> placement(
            @RequestHeader(value = "X-User-Id", required = false) Long headerUserId,
            @RequestBody Map<String, Object> body) {
        long userId = headerUserId != null ? headerUserId : toLong(body.getOrDefault("user_id", 1), 1L);
        @SuppressWarnings("unchecked")
        List<Object> kpIdsRaw = (List<Object>) body.getOrDefault("kp_ids", List.of());
        List<Long> kpIds = kpIdsRaw.stream().map(o -> toLong(o, 0L)).toList();
        double score = toDouble(body.getOrDefault("score", 0.0), 0.0);
        learningService.initMastery(userId, kpIds, score);
        return ResponseEntity.ok(Map.of("initialized", kpIds.size()));
    }

    /** FR-2.3 今日复习清单。P1-6 断点3:优先读 X-User-Id header。 */
    @GetMapping("/review")
    public ResponseEntity<?> dueReview(
            @RequestHeader(value = "X-User-Id", required = false) Long headerUserId,
            @RequestParam(value = "user_id", required = false) Long paramUserId) {
        long userId = headerUserId != null ? headerUserId : (paramUserId != null ? paramUserId : 1L);
        return ResponseEntity.ok(learningService.dueReview(userId));
    }

    /** FR-2.1 薄弱知识点。P1-6 断点3:优先读 X-User-Id header。 */
    @GetMapping("/mastery/weak")
    public ResponseEntity<?> weakest(
            @RequestHeader(value = "X-User-Id", required = false) Long headerUserId,
            @RequestParam(value = "user_id", required = false) Long paramUserId) {
        long userId = headerUserId != null ? headerUserId : (paramUserId != null ? paramUserId : 1L);
        return ResponseEntity.ok(learningService.weakestPoints(userId));
    }

    /** FR-2.2 动态难度。 */
    @PostMapping("/difficulty")
    public ResponseEntity<?> difficulty(@RequestBody Map<String, Object> body) {
        int current = toInt(body.getOrDefault("current", 3), 3);
        boolean correct = (boolean) body.getOrDefault("correct", true);
        int next = learningService.nextDifficulty(current, correct);
        double speed = learningService.speedMultiplier(next);
        return ResponseEntity.ok(Map.of(
                "next_difficulty", next,
                "speed_multiplier", speed,
                "scaffolding", next < current));
    }

    private static long toLong(Object o, long dflt) {
        if (o == null) return dflt;
        if (o instanceof Number n) return n.longValue();
        try { return Long.parseLong(o.toString().trim()); } catch (Exception e) { return dflt; }
    }

    private static int toInt(Object o, int dflt) {
        if (o == null) return dflt;
        if (o instanceof Number n) return n.intValue();
        try { return Integer.parseInt(o.toString().trim()); } catch (Exception e) { return dflt; }
    }

    private static double toDouble(Object o, double dflt) {
        if (o == null) return dflt;
        if (o instanceof Number n) return n.doubleValue();
        try { return Double.parseDouble(o.toString().trim()); } catch (Exception e) { return dflt; }
    }
}
