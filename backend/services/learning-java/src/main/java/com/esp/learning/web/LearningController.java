package com.esp.learning.web;

import com.esp.learning.service.LearningService;
import jakarta.servlet.http.HttpServletRequest;
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

    /** 从 JWT 边缘校验注入的 attribute 取真实 userId(P0-2:不再信任 X-User-Id header/body user_id,防伪造越权)。 */
    private long userIdFrom(HttpServletRequest req) {
        Object attr = req.getAttribute("userId");
        if (attr == null) return 0L;
        try { return Long.parseLong(attr.toString()); } catch (NumberFormatException e) { return 0L; }
    }

    /** FR-2.4 学情事件入口(异步,202)。P0-2:userId 取自 JWT,不再信任 X-User-Id header。 */
    @PostMapping("/event")
    public ResponseEntity<Map<String, Object>> reportEvent(
            HttpServletRequest req,
            @RequestBody Map<String, Object> body) {
        long userId = userIdFrom(req);
        if (userId == 0) return ResponseEntity.status(401).body(Map.of("accepted", false));
        String type = (String) body.getOrDefault("event_type", "answer");
        String payload = body.containsKey("payload") ? body.get("payload").toString() : "{}";
        boolean ok = learningService.reportEvent(userId, type, payload);
        int status = ok ? 202 : 503;
        return ResponseEntity.status(status).body(Map.of("accepted", ok));
    }

    /** FR-2.1 初测基线。P0-2:userId 取自 JWT。 */
    @PostMapping("/placement")
    public ResponseEntity<Map<String, Object>> placement(
            HttpServletRequest req,
            @RequestBody Map<String, Object> body) {
        long userId = userIdFrom(req);
        if (userId == 0) return ResponseEntity.status(401).body(Map.of("error", "unauthorized"));
        @SuppressWarnings("unchecked")
        List<Object> kpIdsRaw = (List<Object>) body.getOrDefault("kp_ids", List.of());
        List<Long> kpIds = kpIdsRaw.stream().map(o -> toLong(o, 0L)).toList();
        double score = toDouble(body.getOrDefault("score", 0.0), 0.0);
        int initialized = learningService.initMastery(userId, kpIds, score);
        return ResponseEntity.ok(Map.of("initialized", initialized));
    }

    /** FR-2.3 今日复习清单。P0-2:userId 取自 JWT,移除 query user_id 回落(防查他人)。 */
    @GetMapping("/review")
    public ResponseEntity<?> dueReview(HttpServletRequest req) {
        long userId = userIdFrom(req);
        if (userId == 0) return ResponseEntity.status(401).body(Map.of("error", "unauthorized"));
        return ResponseEntity.ok(learningService.dueReview(userId));
    }

    /** FR-2.1 薄弱知识点。P0-2:userId 取自 JWT,移除 query user_id 回落(防查他人)。 */
    @GetMapping("/mastery/weak")
    public ResponseEntity<?> weakest(HttpServletRequest req) {
        long userId = userIdFrom(req);
        if (userId == 0) return ResponseEntity.status(401).body(Map.of("error", "unauthorized"));
        return ResponseEntity.ok(learningService.weakestPoints(userId));
    }

    /** FR-2.2 动态难度(纯计算,鉴权已由 Filter 保证,无 userId 依赖)。 */
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
