package com.esp.learning.web;

import com.esp.cache.CacheService;
import com.esp.learning.common.AppLogClient;
import com.esp.learning.service.LearningService;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.*;

import java.util.*;

/**
 * 用户行为分析与推荐(ADR-observability-rebuild §4)。
 *
 * <p>三端统一行为上报 + 行为聚合分析 + 个性化推荐。所有查询走读副本(tidib-follower)。
 * 行为事件双写:learning_events(原有)+ app_logs(event=behavior.*),以便跨服务统一分析。
 *
 * <p>P2-16: top/funnel/heatmap 三查询接口纳入 CacheService(L1 Caffeine + L2 Redis,
 * TTL 抖动防雪崩 + singleflight 防击穿)。app_logs 聚合查询重,缓存收益高。
 */
@RestController
@RequestMapping("/api/learning")
public class BehaviorController {

    private final LearningService learningService;
    private final AppLogClient appLog;
    private final JdbcTemplate readJdbc;
    private final JdbcTemplate sharedJdbc;  // app_logs 专用(esp_shared)
    private final CacheService cacheService;

    public BehaviorController(LearningService learningService, AppLogClient appLog,
                              @org.springframework.beans.factory.annotation.Qualifier("jdbcTemplate")
                              org.springframework.jdbc.core.JdbcTemplate readJdbc,
                              @org.springframework.beans.factory.annotation.Qualifier("sharedJdbcTemplate")
                              org.springframework.jdbc.core.JdbcTemplate sharedJdbc,
                              CacheService cacheService) {
        this.learningService = learningService;
        this.appLog = appLog;
        // 注入的是 routing DataSource(默认读副本,esp-rw @DataSource AOP 切换)
        this.readJdbc = readJdbc;
        this.sharedJdbc = sharedJdbc;
        this.cacheService = cacheService;
    }

    /** 统一行为上报:点击/停留/完成/退出/语音时长等。落 app_logs(event=behavior.*)。 */
    @PostMapping("/behavior")
    public ResponseEntity<Map<String, Object>> reportBehavior(@RequestBody Map<String, Object> body) {
        long userId = toLong(body.getOrDefault("user_id", 1), 1L);
        String action = (String) body.getOrDefault("action", "unknown");
        String target = body.containsKey("target") ? body.get("target").toString() : null;
        long durationMs = toLong(body.getOrDefault("duration_ms", 0), 0L);
        @SuppressWarnings("unchecked")
        Map<String, Object> context = (Map<String, Object>) body.getOrDefault("context", Map.of());

        Map<String, Object> payload = new HashMap<>();
        payload.put("action", action);
        if (target != null) payload.put("target", target);
        if (durationMs > 0) payload.put("duration_ms", durationMs);
        payload.putAll(context);

        appLog.log("learning", "INFO", "behavior." + action, userId, payload);
        // 上报即行为变更,失效该用户行为聚合缓存(top/funnel 是全局不失效,靠 TTL 自然过期)。
        cacheService.evict("learning:agg", "behavior:heatmap:" + userId);
        return ResponseEntity.accepted().body(Map.of("accepted", true));
    }

    /** 近 N 天活跃 TOP 用户(行为事件计数)。P2-16: 缓存 60s。 */
    @GetMapping("/behavior/top")
    public ResponseEntity<?> topActive(@RequestParam(defaultValue = "7") int days,
                                       @RequestParam(defaultValue = "10") int limit) {
        String cacheKey = "behavior:top:" + days + ":" + limit;
        Object cached = cacheService.getOrLoad("learning:agg", cacheKey,
                () -> {
                    String sql = "SELECT user_id, COUNT(*) cnt FROM app_logs " +
                            "WHERE event LIKE 'behavior.%' AND ts > NOW() - INTERVAL ? DAY " +
                            "GROUP BY user_id ORDER BY cnt DESC LIMIT ?";
                    return sharedJdbc.queryForList(sql, days, limit);
                }, 60, true);
        return ResponseEntity.ok(cached != null ? cached : List.of());
    }

    /** 漏斗分析:按 action 序列统计各阶段用户数。action=enter_speech,practice,complete。P2-16: 缓存 60s。 */
    @GetMapping("/behavior/funnel")
    public ResponseEntity<?> funnel(@RequestParam String action,
                                    @RequestParam(defaultValue = "7") int days) {
        String cacheKey = "behavior:funnel:" + action + ":" + days;
        Object cached = cacheService.getOrLoad("learning:agg", cacheKey,
                () -> {
                    String[] stages = action.split(",");
                    List<Map<String, Object>> result = new ArrayList<>();
                    for (String s : stages) {
                        String sql = "SELECT COUNT(DISTINCT user_id) users FROM app_logs " +
                                "WHERE event = ? AND ts > NOW() - INTERVAL ? DAY";
                        List<Map<String, Object>> rows = sharedJdbc.queryForList(sql, "behavior." + s.trim(), days);
                        Map<String, Object> row = new LinkedHashMap<>();
                        row.put("stage", s.trim());
                        row.put("users", rows.isEmpty() ? 0 : rows.get(0).get("users"));
                        result.add(row);
                    }
                    return result;
                }, 60, true);
        return ResponseEntity.ok(cached != null ? cached : List.of());
    }

    /** 用户行为热力:按 event 分组计数(近 N 天)。P2-16: 缓存 60s,上报即失效。 */
    @GetMapping("/behavior/heatmap")
    public ResponseEntity<?> heatmap(@RequestParam long user_id,
                                     @RequestParam(defaultValue = "30") int days) {
        String cacheKey = "behavior:heatmap:" + user_id + ":" + days;
        Object cached = cacheService.getOrLoad("learning:agg", cacheKey,
                () -> {
                    String sql = "SELECT event, COUNT(*) cnt FROM app_logs " +
                            "WHERE user_id = ? AND event LIKE 'behavior.%' AND ts > NOW() - INTERVAL ? DAY " +
                            "GROUP BY event ORDER BY cnt DESC";
                    return sharedJdbc.queryForList(sql, user_id, days);
                }, 60, true);
        return ResponseEntity.ok(cached != null ? cached : List.of());
    }

    /** 个性化推荐:复习清单 + 相似用户场景 + 写作练习推荐。 */
    @GetMapping("/recommend")
    public ResponseEntity<?> recommend(@RequestParam long user_id) {
        Map<String, Object> result = new LinkedHashMap<>();

        // 1. 复习清单(复用 Ebbinghaus 调度,已有)
        result.put("review", learningService.dueReview(user_id));

        // 2. 薄弱知识点(已有)
        result.put("weak_points", learningService.weakestPoints(user_id));

        // 3. 相似用户练习过的场景(基于 user_mastery 协同过滤,轻量 SQL)
        try {
            String sql = "SELECT s.id, s.code, s.title, s.difficulty FROM scenarios s " +
                    "WHERE s.id IN (" +
                    "  SELECT DISTINCT ses.scenario_id FROM sessions ses " +
                    "  WHERE ses.user_id IN (" +
                    "    SELECT m1.user_id FROM user_mastery m1 " +
                    "    WHERE m1.kp_id IN (SELECT m2.kp_id FROM user_mastery m2 WHERE m2.user_id = ? AND m2.mastery_score < 60) " +
                    "    AND m1.user_id <> ? AND m1.mastery_score > 70" +
                    "  ) AND ses.scenario_id NOT IN (SELECT scenario_id FROM sessions WHERE user_id = ?)" +
                    ") LIMIT 5";
            result.put("similar_scenarios", readJdbc.queryForList(sql, user_id, user_id, user_id));
        } catch (Exception ex) {
            result.put("similar_scenarios", List.of());
        }

        return ResponseEntity.ok(result);
    }

    private static long toLong(Object o, long dflt) {
        if (o == null) return dflt;
        if (o instanceof Number n) return n.longValue();
        try { return Long.parseLong(o.toString().trim()); } catch (Exception e) { return dflt; }
    }
}
