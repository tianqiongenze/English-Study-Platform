package com.esp.teacher.web;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.teacher.service.LessonPlanGenerator;
import com.esp.teacher.service.TeacherService;
import jakarta.servlet.http.HttpServletRequest;
import org.apache.dubbo.config.annotation.DubboReference;
import org.apache.dubbo.rpc.RpcException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.math.BigDecimal;
import java.util.List;
import java.util.Map;
import java.util.Objects;

@RestController
@RequestMapping("/api/teacher")
public class TeacherController {

    private static final Logger log = LoggerFactory.getLogger(TeacherController.class);

    private final TeacherService teacherService;
    private final LearningRpcService learningRpc;

    public TeacherController(TeacherService teacherService, LearningRpcService learningRpc) {
        this.teacherService = teacherService;
        this.learningRpc = learningRpc;
    }

    /** FR-4.1 一键生成教案。 */
    @PostMapping("/lesson-plan")
    public ResponseEntity<LessonPlanGenerator.LessonPlan> lessonPlan(@RequestBody Map<String, Object> body) {
        String topic = (String) body.getOrDefault("topic", "Environment");
        String level = (String) body.getOrDefault("level", "B1");
        return ResponseEntity.ok(teacherService.generateLessonPlan(topic, level));
    }

    /** FR-4.2 班级学情分析。 */
    @GetMapping("/classes/{id}/analytics")
    public ResponseEntity<?> analytics(@PathVariable("id") Long classId) {
        return ResponseEntity.ok(teacherService.classAnalytics(classId));
    }

    /** FR-4.3 作业智能评阅。classId 可选(用于缓存失效;不传则不失效班级缓存)。 */
    @PostMapping("/assignments/{id}/auto-grade")
    public ResponseEntity<Map<String, Object>> autoGrade(@PathVariable("id") Long assignmentId,
                                                          @RequestParam(value = "classId", required = false) Long classId) {
        int graded = teacherService.autoGrade(assignmentId, classId);
        return ResponseEntity.ok(Map.of("graded", graded));
    }

    /** 教师终审。classId 可选(用于缓存失效)。 */
    @PostMapping("/submissions/{id}/finalize")
    public ResponseEntity<Map<String, Object>> finalize(@PathVariable("id") Long submissionId,
                                                         @RequestBody Map<String, Object> body) {
        BigDecimal finalScore = new BigDecimal(body.getOrDefault("final_score", "0").toString());
        Long classId = body.get("class_id") == null ? null : Long.valueOf(body.get("class_id").toString());
        teacherService.finalize(submissionId, finalScore, classId);
        return ResponseEntity.ok(Map.of("finalized", submissionId));
    }

    /** FR-4 学情分析:经 Dubbo Triple 调 learning-java 拉取用户各知识点掌握统计(RPC 失败降级 503)。
     *  P0-3 IDOR 修复:learner 只能查自己的 kp-stats,teacher/admin 角色可查任意学生。 */
    @GetMapping("/{userId}/kp-stats")
    public ResponseEntity<?> kpStats(@PathVariable("userId") Long userId, HttpServletRequest req) {
        String role = Objects.toString(req.getAttribute("role"), "learner");
        if (!("teacher".equals(role) || "admin".equals(role))) {
            Object sub = req.getAttribute("userId");
            if (sub == null || !String.valueOf(userId).equals(sub.toString())) {
                return ResponseEntity.status(HttpStatus.FORBIDDEN).body(Map.of("error", "forbidden"));
            }
        }
        try {
            List<KpStat> stats = learningRpc.userStats(userId);
            return ResponseEntity.ok(stats);
        } catch (RpcException e) {
            log.warn("Dubbo RPC failed for userStats userId={}, msg={}", userId, e.getMessage());
            return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE)
                    .body(Map.of("error", "learning service unavailable", "detail", e.getMessage()));
        }
    }
}
