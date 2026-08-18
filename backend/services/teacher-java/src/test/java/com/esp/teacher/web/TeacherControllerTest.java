package com.esp.teacher.web;

import com.esp.api.dubbo.KpStat;
import com.esp.api.dubbo.LearningRpcService;
import com.esp.teacher.service.LessonPlanGenerator;
import com.esp.teacher.service.TeacherService;
import com.esp.common.jwt.BaseJwtValidator;
import com.esp.cache.CacheService;
import org.apache.dubbo.rpc.RpcException;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.webmvc.test.autoconfigure.WebMvcTest;
import org.springframework.test.context.bean.override.mockito.MockitoBean;
import org.springframework.http.MediaType;
import org.springframework.test.context.TestPropertySource;
import org.springframework.test.web.servlet.MockMvc;

import java.util.List;

import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.*;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

/**
 * TeacherController Web 层测试:验证教案生成、班级分析、批量评阅、终审端点契约,
 * 以及新增 Dubbo Triple RPC 学情端点(成功 + RPC 失败降级 503)。
 */
@WebMvcTest(TeacherController.class)
@TestPropertySource("classpath:application-test.properties")
class TeacherControllerTest {

    @Autowired
    MockMvc mvc;

    @MockitoBean TeacherService teacherService;
    @MockitoBean LearningRpcService learningRpc;
    @MockitoBean CacheService cacheService;
    @MockitoBean BaseJwtValidator jwtValidator;

    @Test
    void lessonPlan_returnsGeneratedPlan() throws Exception {
        var plan = new LessonPlanGenerator.LessonPlan(
                "Passage about Environment.", List.of("ex1", "ex2", "ex3"),
                List.of("game1"), List.of("d1", "d2"));
        when(teacherService.generateLessonPlan(anyString(), anyString())).thenReturn(plan);

        mvc.perform(post("/api/teacher/lesson-plan")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"topic\":\"Environment\",\"level\":\"B2\"}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.passage").value("Passage about Environment."))
                .andExpect(jsonPath("$.exercises[0]").value("ex1"))
                .andExpect(jsonPath("$.discussion.length()").value(2));
    }

    @Test
    void analytics_returnsList() throws Exception {
        when(teacherService.classAnalytics(5L)).thenAnswer(inv -> List.of(new Object[]{"kp", 3L}));

        mvc.perform(get("/api/teacher/classes/5/analytics"))
                .andExpect(status().isOk());
        verify(teacherService).classAnalytics(5L);
    }

    @Test
    void autoGrade_returnsGradedCount() throws Exception {
        when(teacherService.autoGrade(eq(7L), any())).thenReturn(3);

        mvc.perform(post("/api/teacher/assignments/7/auto-grade"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.graded").value(3));
    }

    @Test
    void finalize_returnsFinalizedId() throws Exception {
        doNothing().when(teacherService).finalize(eq(9L), any(), any());

        mvc.perform(post("/api/teacher/submissions/9/finalize")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{\"final_score\":88}"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.finalized").value(9));
    }

    @Test
    void kpStats_returnsLearningRpcResult() throws Exception {
        when(learningRpc.userStats(1L)).thenReturn(List.of(
                new KpStat(10L, 3.5, 2, "2026-08-01T00:00:00Z")));

        mvc.perform(get("/api/teacher/1/kp-stats")
                        .requestAttr("userId", "1")
                        .requestAttr("role", "teacher"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].kpId").value(10))
                .andExpect(jsonPath("$[0].masteryScore").value(3.5))
                .andExpect(jsonPath("$[0].reviewCount").value(2))
                .andExpect(jsonPath("$[0].nextReviewAt").value("2026-08-01T00:00:00Z"));
    }

    @Test
    void kpStats_returns503_whenRpcFails() throws Exception {
        when(learningRpc.userStats(anyLong()))
                .thenThrow(new RpcException("learning service unavailable"));

        mvc.perform(get("/api/teacher/2/kp-stats")
                        .requestAttr("userId", "2")
                        .requestAttr("role", "teacher"))
                .andExpect(status().isServiceUnavailable())
                .andExpect(jsonPath("$.error").value("learning service unavailable"));
    }

    /** P0-3 IDOR:learner 查他人 kp-stats → 403。 */
    @Test
    void kpStats_forbidden_whenLenderQueriesOtherUser() throws Exception {
        mvc.perform(get("/api/teacher/2/kp-stats")
                        .requestAttr("userId", "1")
                        .requestAttr("role", "learner"))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.error").value("forbidden"));
        verifyNoInteractions(learningRpc);
    }
}
